package riftbound

import (
	"slices"
	"strconv"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// Rules implements matcher.GameRules for Riftbound. Like Lorcana, a card
// is identified by name + collector number + foil, with the edition breaking
// ties when it resolves; there are no variant tables or promo types, so most
// hooks are no-ops and the real work is the number disambiguation in
// FilterCards (foil is honored downstream by output).
type Rules struct{ matcher.DefaultRules }

// Prefilter splits a trailing parenthetical variant off the name before the
// canonical-name lookup - unless the full name is itself a known card: a few
// real names carry a parenthetical ("Recruit (DE)"), and the promotional
// printings keep their storefront names verbatim, qualifiers included.
// Dashes stay intact, since they occur in real names too ("Dark Child -
// Starter").
func (Rules) Prefilter(b *matcher.Backend, inCard *matcher.InputCard) {
	// Names that only promotional printings carry act as unknown unless the
	// input targets a promo set: promos keep their storefront names verbatim
	// (qualifiers included), and those shapes must keep resolving to the
	// main printings for everyone else.
	targetsPromo := editionIsPromo(b, inCard.Edition)
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; found {
		if (!promoOnlyName(b, inCard.Name) || targetsPromo) &&
			!editionConflictsName(b, inCard.Name, inCard.Edition) {
			return
		}
		// The current gallery shortened several main-set names to the
		// champion alone, while the promo records still carry the old
		// qualified spelling. When that promo spelling is canonical, an
		// ordinary listing must be aimed at the main-set name before the
		// promo gate can exclude the promotional sibling.
		if fixed := qualifiedBaseName(b, inCard.Name, inCard.Variation); fixed != "" {
			if extractNumber(inCard.Variation) == "" {
				if number := qualifiedNameNumber(b, inCard.Name); number != "" {
					if inCard.Variation == "" {
						inCard.Variation = number
					} else {
						inCard.Variation = number + " " + inCard.Variation
					}
				}
			}
			inCard.Name = fixed
			return
		}
	}
	if strings.Contains(inCard.Name, "(") {
		vars := matcher.SplitVariants(inCard.Name)
		if len(vars) > 1 {
			inCard.Name = vars[0]
			qualifier := strings.Join(vars[1:], " ")
			inCard.AddToVariant(qualifier)
			// The gallery qualifies a promo in the name itself, and the
			// storefront wording is close but rarely equal ("Champion Stamp"
			// against "(Champion)"). Sibling promos share the base name and
			// the number, so only the qualifier tells them apart.
			if targetsPromo {
				if fixed := qualifiedPromoName(b, inCard.Name, qualifier); fixed != "" {
					inCard.Name = fixed
					return
				}
			}
		}
	}
	// A promo printing can still share the storefront shape of a main-set
	// legend name ("Teemo - Swift Scout" is a promo entry while the Origins
	// card is "Swift Scout"): re-aim at the gallery name here, or the
	// canonical lookup would stop at the promo, whose printings the promo
	// gate in FilterCards then rightly refuses. The same retry also covers
	// a direct match that isn't promo-only but still conflicts with the
	// stated edition (see editionConflictsName) - a small organized-play or
	// special-release card can share a storefront's dash spelling of an
	// unrelated main-set card's epithet-only name without either side being
	// a promo.
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; found &&
		((promoOnlyName(b, inCard.Name) && !targetsPromo) ||
			editionConflictsName(b, inCard.Name, inCard.Edition)) {
		if fixed := legendName(b, inCard.Name, targetsPromo); fixed != "" {
			inCard.Name = fixed
		}
	}
}

// editionIsPromo reports whether the input edition targets the promotional
// sets, either by naming one or by saying only that the card is a promo.
// Storefronts file every promotional printing under a single heading rather
// than under the set that issued it - CoolStuffInc calls the lot "Promo",
// mixing judge, release event and championship cards - and that heading names
// no set. Reading it as an ordinary edition leaves the gate shut, and the
// promo-only names then resolve to the main printing: the champion-stamped
// Edge of Night comes back as the Spiritforged card of the same number, a
// promo sold at many times its price.
func editionIsPromo(b *matcher.Backend, edition string) bool {
	if edition == "" {
		return false
	}
	if matcher.IsPromoHeading(edition) {
		return true
	}
	set, err := b.GetSetByName(edition)
	if err == nil {
		return set.Type == "promo"
	}
	// Storefronts also file promos under the set whose printing they
	// reprint - Cardmarket's "Origins: Promos" holds organized-play
	// cards - which names no set of ours but still says promo.
	return endsInPromo(edition)
}

// endsInPromo reports whether an edition ends in the word a storefront hangs
// a set name off. Both spellings count, since storefronts write either.
func endsInPromo(edition string) bool {
	norm := matcher.Normalize(edition)
	return strings.HasSuffix(norm, "promos") || strings.HasSuffix(norm, "promo")
}

// qualifiedPromoName returns the promotional printing named "<base>
// (<qualifier>)" that the storefront's own qualifier describes, or "" when
// none does or several might. Siblings share the base name and the collector
// number - Edge of Night is 139 in the gallery both as (Champion) and as
// (Top 8) - so the qualifier is the only thing that separates them.
//
// The gallery's qualifier has to be contained in the storefront's, word for
// word, rather than merely overlap it: storefronts qualify these more fully
// than the gallery does ("Champion Stamp" for "(Champion)"), while the
// reverse would let "(Top 8)" answer for a plain "Top" and let one sibling
// stand in for another.
func qualifiedPromoName(b *matcher.Backend, base, qualifier string) string {
	if qualifier == "" {
		return ""
	}
	uuids, err := b.SearchHasPrefix(base)
	if err != nil {
		return ""
	}

	// Compared word by word in lower case rather than through Normalize,
	// which folds a whole string into one token and would collapse
	// "Champion Stamp" into a single word matching nothing.
	said := map[string]bool{}
	for word := range strings.FieldsSeq(strings.ToLower(qualifier)) {
		said[word] = true
	}

	var match string
	for _, uuid := range uuids {
		co, err := b.GetUUID(uuid)
		if err != nil || co.Name == base || !promoOnlyName(b, co.Name) {
			continue
		}
		vars := matcher.SplitVariants(co.Name)
		if len(vars) < 2 || vars[0] != base {
			continue
		}
		describes := true
		for word := range strings.FieldsSeq(strings.ToLower(strings.Join(vars[1:], " "))) {
			if !said[word] {
				describes = false
				break
			}
		}
		if !describes {
			continue
		}
		if match != "" && match != co.Name {
			// Two qualifiers both fit: let the ordinary path decide.
			return ""
		}
		match = co.Name
	}
	return match
}

// promoOnlyName reports whether every printing hashed under the name lives
// in a promotional set.
func promoOnlyName(b *matcher.Backend, name string) bool {
	uuids := b.Hashes[matcher.Normalize(name)]
	if len(uuids) == 0 {
		return false
	}
	for _, uuid := range uuids {
		co, found := b.UUIDs[uuid]
		if !found {
			continue
		}
		if co.Sealed {
			continue
		}
		set, found := b.Sets[co.SetCode]
		if !found || set.Type != "promo" {
			return false
		}
	}
	return true
}

// editionConflictsName reports whether the storefront's edition names a real
// set that this name's own printings never appear in. Most storefront
// dashes and commas are genuinely part of the name ("Dark Child - Starter"),
// but the gallery has also, separately, reused a comma-joined champion
// spelling ("Ivern, Green Father") for an unrelated card in a small
// organized-play or special-release set, while the storefront's own dash
// spelling of that same text names a main-set card filed under its epithet
// alone ("Green Father"). Nothing about the name says which is meant; the
// edition does. A listing whose stated edition holds none of the direct
// match's own printings is the signal that the direct match is the wrong
// one, and it is safe to keep looking rather than trust it - an edition
// that does not resolve to a set at all proves nothing either way, so it
// does not count as a conflict.
//
// This only disqualifies the direct match; it does not itself vouch for
// whatever legendName or qualifiedBaseName retries with next. A retry
// candidate that also lacks a printing in the stated edition is left to
// the ordinary downstream number/edition filtering to refuse, the same as
// any other wrong candidate - not silently accepted for having merely
// passed this check.
func editionConflictsName(b *matcher.Backend, name, edition string) bool {
	set, err := b.GetSetByName(edition)
	if err != nil {
		return false
	}
	for _, uuid := range b.Hashes[matcher.Normalize(name)] {
		co, err := b.GetUUID(uuid)
		if err == nil && co.SetCode == set.Code {
			return false
		}
	}
	return true
}

// AdjustName reconciles storefront name shapes with the gallery's. Legend
// cards are exported title-only ("Daughter of the Void") while storefronts
// list them champion-first ("Kai'Sa - Daughter of the Void"), so an unknown
// name containing " - " retries as its title segment; the collector number
// and finish still validate the outcome downstream. Failing that, a prefix
// fallback covers feeds that truncate a "Champion, Title" name: first try
// the current gallery's champion base, then scan for cards whose name has the
// input as a prefix and let the number and finish narrow them, adopting the
// name only when exactly one survives.
func (Rules) AdjustName(b *matcher.Backend, inCard *matcher.InputCard) {
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; found {
		return
	}

	// The gallery shortened several champion cards from "Champion, Title" to
	// just the champion name. Storefronts still publish the older qualified
	// spelling, so use the base name when it is canonical and has a printing
	// matching the listing's number. Edition and finish remain the authority
	// for choosing among that champion's printings below.
	if fixed := qualifiedBaseName(b, inCard.Name, inCard.Variation); fixed != "" {
		inCard.Name = fixed
		return
	}

	if fixed := legendName(b, inCard.Name, editionIsPromo(b, inCard.Edition)); fixed != "" {
		inCard.Name = fixed
		return
	}

	uuids, err := b.SearchHasPrefix(inCard.Name)
	if err != nil {
		return
	}

	number := extractNumber(inCard.Variation)
	var match, matchNorm string
	for _, uuid := range uuids {
		co, err := b.GetUUID(uuid)
		if err != nil {
			continue
		}
		if number != "" && !strings.EqualFold(number, co.Number) {
			continue
		}
		// The finish deliberately does not narrow here, for the same reason
		// collectPrintings does not gate on it: half the game is sold in a
		// single finish, so a wrong or missing flag would drop the true
		// candidate, and here that loses the name rather than a printing.
		//
		// Names compare normalized because the promo sets spell the epithet
		// with a dash where the main sets use a comma ("Annie - Fiery" for
		// OGS's "Annie, Fiery"). Match already resolves both spellings to one
		// card, so meeting both is not the ambiguity it looks like.
		norm := matcher.Normalize(co.Name)
		if match != "" && matchNorm != norm {
			// Different names survive the filters: genuinely ambiguous.
			return
		}
		match, matchNorm = co.Name, norm
	}
	if match != "" {
		inCard.Name = match
	}
}

// qualifiedBaseName maps a storefront's old qualified spelling ("Champion,
// Title" or "Champion - Title") to a current gallery name when the champion
// itself is canonical. Requiring the number to occur under that base name
// prevents an unrelated qualified name from being silently reinterpreted as a
// champion listing.
func qualifiedBaseName(b *matcher.Backend, name, variation string) string {
	number := extractNumber(variation)
	for _, separator := range []string{", ", " - "} {
		// Dashes are valid in real Riftbound names, so the storefront dash
		// spelling is only ever considered in two cases: this exact name is
		// already known to be promo-only, or it names no card at all - the
		// current gallery's own vocabulary says nothing either way - and the
		// listing carries a collector number, so the number-must-occur-under-
		// base check below is what ties it to the champion rather than the
		// wording alone. Either way, a name the datastore already resolves
		// to a real, non-promotional card (a genuine dashed name like "Dark
		// Child - Starter") is left for the ordinary AdjustName fallback to
		// handle, never reinterpreted here.
		if separator == " - " && !promoOnlyName(b, name) &&
			!(number != "" && len(b.Hashes[matcher.Normalize(name)]) == 0) {
			continue
		}
		base, title, found := strings.Cut(name, separator)
		if !found || title == "" {
			continue
		}

		canonical, found := b.CanonicalNames[matcher.Normalize(base)]
		if !found {
			continue
		}

		for _, uuid := range b.Hashes[matcher.Normalize(canonical)] {
			co, err := b.GetUUID(uuid)
			if err != nil || co.Sealed {
				continue
			}
			if number == "" || strings.EqualFold(number, co.Number) {
				return canonical
			}
		}
	}
	return ""
}

// qualifiedNameNumber returns the one collector number carried by a
// promo-only qualified name, or "" when that name covers more than one
// number. The number is useful when the current gallery shortened the main
// name and the storefront omitted the number: the qualified promo spelling
// is then the only evidence tying it back to the main printing.
//
// Numbers compare through stripNumberLetter's reduction rather than as
// written, so a lettered organized-play sibling still agrees with the rest
// of the name's plain printings (see the commit message).
func qualifiedNameNumber(b *matcher.Backend, name string) string {
	var number string
	for _, uuid := range b.Hashes[matcher.Normalize(name)] {
		co, err := b.GetUUID(uuid)
		if err != nil || co.Sealed {
			continue
		}
		plain := co.Number
		stripped := stripNumberLetter(plain)
		if stripped != "" {
			plain = stripped
		}
		if number == "" {
			number = plain
			continue
		}
		if !strings.EqualFold(number, plain) {
			return ""
		}
	}
	return number
}

// AdjustEdition normalizes scraper edition strings toward the gallery set
// names: storefronts commonly prefix the game name ("Riftbound: Origins")
// or append catalog suffixes ("... Singles"). An edition that still matches
// no set name simply does not narrow the candidates (the Match skeleton
// falls back to every printing), so trimming can only help.
func (r Rules) AdjustEdition(b *matcher.Backend, inCard *matcher.InputCard) {
	inCard.Edition = r.AliasEdition(b, inCard.Edition)
}

// AliasEdition spells an edition string toward a set name using the string
// alone. See matcher.GameRules.
func (Rules) AliasEdition(b *matcher.Backend, edition string) string {
	edition = strings.TrimSpace(edition)
	// An edition already naming a set verbatim needs no normalization - and
	// must not be trimmed out of matching: the promotional sets themselves
	// are named "Riftbound ... Promotional Cards".
	for _, set := range b.Sets {
		if matcher.Equals(set.Name, edition) {
			return edition
		}
	}
	for _, prefix := range []string{"Riftbound: League of Legends", "Riftbound", "League of Legends"} {
		if strings.HasPrefix(edition, prefix) {
			edition = strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(edition, prefix), ":-"))
			break
		}
	}
	edition = strings.TrimSpace(strings.TrimSuffix(edition, "Singles"))
	if name, found := storefrontEditions[matcher.Normalize(edition)]; found {
		edition = name
	}
	// Every promo heading collapses onto one canonical heading - editions
	// naming a real set returned at the top, and no set name ends in
	// "Promos". "Promos" also occurs in no set name, so the core name
	// heuristics cannot narrow the candidates on it: the heading spans
	// several promotional sets, and the promo tiers in FilterCards choose
	// instead of whichever set name happens to contain the heading's
	// words. Checked without GetSetByName, which runs this very hook.
	if matcher.IsPromoHeading(edition) || endsInPromo(edition) {
		edition = "Promos"
	}
	return edition
}

// storefrontEditions renames the editions storefronts carry that the gallery
// names differently. Without them the edition narrows nothing and the number
// alone decides, which quietly answers with the base-set printing: a listing
// of the Top 8 Guardian Angel, numbered 51 in the promotional set, comes back
// as the Spiritforged card of the same number.
//
// Only the renames the data agrees on are here. Every product id that
// resolves under these headings lands in one set apiece - 31, 58, 2 and 24 of
// them - and the release-event heading, thin on ids, has an organized-play
// printing at the number of all 17 of its listings. CardTrader's plain
// "Promos" splits across two promotional sets, so there is no single answer
// to give it and it is deliberately absent.
var storefrontEditions = map[string]string{
	matcher.Normalize("Organized Play"):          "Riftbound Organized Play Promotional Cards",
	matcher.Normalize("Nexus Night Promos"):      "Riftbound Organized Play Promotional Cards",
	matcher.Normalize("Release Event Promos"):    "Riftbound Organized Play Promotional Cards",
	matcher.Normalize("Origins Proving Grounds"): "Proving Grounds",
}

// legendName maps a storefront "Champion - Title" name onto the gallery name
// it stands for, or returns "" when no unambiguous mapping exists. The
// gallery exports Legend cards title-only ("Daughter of the Void"), may
// shorten the champion ("Master Yi - Meditative" is "Yi, Meditative"), and
// names starter legends title-first ("Lux - Lady of Luminosity (Starter)" is
// "Lady of Luminosity - Starter"): accept the bare title, a "Champion,
// Title" name whose title matches and whose champion ends the input's
// champion, or a dashed name led by the title - as long as exactly one
// candidate does overall.
func legendName(b *matcher.Backend, name string, targetsPromo bool) string {
	champion, title, found := strings.Cut(name, " - ")
	if !found {
		// Cardmarket writes the champion off a comma where the rest of the
		// feeds use a dash ("Ahri, Nine-Tailed Fox"). Only an unknown name
		// gets here, so this cannot cut a gallery name that owns its comma.
		champion, title, found = strings.Cut(name, ", ")
	}
	if !found {
		return ""
	}
	if _, known := b.CanonicalNames[matcher.Normalize(title)]; known {
		return title
	}
	var match string
	for _, canonical := range b.AllCanonicalNames {
		c, t, ok := strings.Cut(canonical, ", ")
		matches := ok && matcher.Equals(t, title) &&
			strings.HasSuffix(matcher.Normalize(champion), matcher.Normalize(c))
		if !matches {
			t2, _, ok2 := strings.Cut(canonical, " - ")
			matches = ok2 && matcher.Equals(t2, title)
		}
		if !matches {
			continue
		}
		// The promotional sets carry the champion-first spelling too - "Lux,
		// Lady of Luminosity" sits beside the Proving Grounds "Lady of
		// Luminosity - Starter" - so one storefront name describes two
		// gallery names at once. Calling that ambiguous loses the card
		// altogether, and the promo gate in FilterCards would refuse the
		// promo for an input naming no promo edition anyway, so the same
		// line decides here.
		if promoOnlyName(b, canonical) != targetsPromo {
			continue
		}
		if match != "" && match != canonical {
			return ""
		}
		match = canonical
	}
	return match
}

// PlainNumber implements matcher.GameRules. The star marks a variant
// printing rather than a different number.
func (Rules) PlainNumber(number string) string {
	return strings.TrimRight(number, "*")
}

// FilterCards narrows candidates by edition, collector number, and finish,
// mirroring the Lorcana rules: sets holds those matching the input
// edition when one was supplied and resolves (falling back to every printing
// otherwise), and the number comparison is case-insensitive because real
// numbers carry letter affixes ("66a", "T5", "SP3").
func (Rules) FilterCards(b *matcher.Backend, inCard *matcher.InputCard, sets []matcher.CandidateSet) []matcher.Card {
	number := extractNumber(inCard.Variation)

	// Promotional printings reuse the main sets' collector numbers, so they
	// never match implicitly: only an edition that resolves to a promo set
	// reaches them, and everything else keeps matching the main printings.
	allowPromo := editionIsPromo(b, inCard.Edition)

	out := collectPrintings(b, inCard, sets, allowPromo, number)

	// A storefront may letter a number the gallery does not: CardTrader files
	// the champion-stamped Lillia as 058c where the gallery keeps 58 and says
	// "champion" in the promo types. Retry on the base number, but keep only
	// the printings the storefront's own wording describes - the letter marks
	// a variant, so answering with a plain sibling would misprice it, and the
	// champion Poppy the gallery does not carry has to stay a miss rather
	// than land on the plain 178.
	if len(out) == 0 {
		if stripped := stripNumberLetter(number); stripped != "" {
			for _, card := range collectPrintings(b, inCard, sets, allowPromo, stripped) {
				if len(card.PromoTypes) > 0 && wordsDescribe(inCard.Variation, card.PromoTypes) {
					out = append(out, card)
				}
			}
		}
	}

	// A promotion whose name opens with something number-shaped leaves
	// extractNumber holding a word. TCGplayer files the five T1 Worlds
	// Champion cards as "(T1 Worlds Champion Player Bundle)" and "T1" is the
	// esports team the cards commemorate, not a collector number: nothing in
	// the game is numbered T1, and the printings themselves are at T1A001
	// and T1S001. The number filter then admits nothing and all fifteen
	// would miss.
	//
	// So when the number named no printing of the name at all, ask the
	// wording on its own and keep only the printings whose every promo type
	// it says. A wording that was only a number still misses, since a bare
	// number describes no promo types.
	if len(out) == 0 {
		for _, card := range collectPrintings(b, inCard, sets, allowPromo, "") {
			if len(card.PromoTypes) > 0 && wordsDescribe(inCard.Variation, card.PromoTypes) {
				out = append(out, card)
			}
		}
	}

	out = promoOrdinalTiebreak(out, inCard.Variation)

	// Sibling promos share one clean name - and, for the organized-play
	// cards, even the main set's collector number - so the number alone
	// can leave several candidates. They rank in tiers:
	//
	//  1. A variant whose promo types are all said by the storefront's own
	//     wording. The containment direction qualified names used, so
	//     "(Top 8)" cannot answer for a plain "Top".
	//  2. A promotional printing without promo types: the input targeted a
	//     promo, so under equal numbers the promo outranks the main-set
	//     sibling, and its plain variant outranks the decorated ones.
	//  3. A typed variant the wording never described, but only when it
	//     said nothing at all beyond the number and the finish: the number
	//     alone reaches a variant a storefront lists by number, while a
	//     qualifier that matched no variant must not pick one anyway.
	//  4. The cards without promo types - the whole candidate list of a
	//     datastore from before the types were recorded, and the main-set
	//     fallback for a qualifier no printing carries.
	var described, promoPlain, promoTyped, untyped []matcher.Card
	for _, card := range out {
		promo := false
		if set, found := b.Sets[card.SetCode]; found && set.Type == "promo" {
			promo = true
		}
		switch {
		case len(card.PromoTypes) > 0 && wordsDescribe(inCard.Variation, card.PromoTypes):
			described = append(described, card)
		case promo && len(card.PromoTypes) == 0:
			promoPlain = append(promoPlain, card)
		case promo:
			promoTyped = append(promoTyped, card)
		default:
			untyped = append(untyped, card)
		}
	}
	tier := out
	switch {
	case len(described) > 0:
		tier = longestDescribed(described)
	case len(promoPlain) > 0:
		tier = promoPlain
	case len(promoTyped) > 0 && len(qualifierWords(inCard.Variation)) == 0:
		tier = promoTyped
	case len(untyped) > 0:
		tier = untyped
	}
	tier = preferBasePrinting(b, inCard, number, promoOrigin(b, inCard.Variation, tier))
	return preferListedFinish(inCard, tier)
}

// promoOrdinalTiebreak breaks a same-numbered organized-play pair that
// differs only by the champion promo type, using the position Cardmarket's
// own "V.N" wording claims - the one storefront that ever sends it, glued
// onto the number by its resolver ("051 V.1 - Rare"). V.1 is the
// non-champion row and V.2 the champion one on all six such pairs today (see
// the commit message); anything else - no "V.N" at all, or more than one
// card on either side - is left for the tiers below to settle.
func promoOrdinalTiebreak(cards []matcher.Card, variation string) []matcher.Card {
	ordinal := extractOrdinal(variation)
	if ordinal != 1 && ordinal != 2 {
		return cards
	}
	var champion, other matcher.Card
	var champCount, otherCount int
	for _, card := range cards {
		if slices.Contains(card.PromoTypes, "champion") {
			champion, champCount = card, champCount+1
		} else {
			other, otherCount = card, otherCount+1
		}
	}
	if champCount != 1 || otherCount != 1 {
		return cards
	}
	if ordinal == 2 {
		return []matcher.Card{champion}
	}
	return []matcher.Card{other}
}

// extractOrdinal reads the position a "V.N" field of the variation claims,
// 0 when none is there. Only Cardmarket's resolver ever writes one.
func extractOrdinal(variation string) int {
	for field := range strings.FieldsSeq(variation) {
		tail, found := strings.CutPrefix(field, "V.")
		if !found {
			continue
		}
		n, err := strconv.Atoi(tail)
		if err != nil || n < 1 {
			continue
		}
		return n
	}
	return 0
}

// promoOrigin narrows a tier of promotional printings to the ones issued
// where the storefront says its copy came from.
//
// A storefront files every promo under one heading, so the edition names no
// set and the promotional sets are told apart by nothing: the judge printing
// of Heimerdinger and the promotional one share a name and a number, and both
// listings aliased. But the wording says where the card came from, and it can
// be read two ways.
//
// A wording spelling a word one promotional set's name owns is naming that
// set - "Judge Promo" against "Riftbound Judge Promotional Cards", where no
// other promotional set says "judge". A wording naming a product instead -
// "Arcane Box Set Promo" - names the set the catalog files that product in,
// which is what makes the reading verifiable rather than a guess: the box is
// a sealed product of its own, filed in the promotional set whose cards it
// holds. Both are exact claims about a set, and neither answers unless one
// set alone fits.
func promoOrigin(b *matcher.Backend, wording string, tier []matcher.Card) []matcher.Card {
	if len(tier) <= 1 || wording == "" {
		return tier
	}
	codes := map[string]bool{}
	for _, card := range tier {
		codes[card.SetCode] = true
	}
	if len(codes) < 2 {
		return tier
	}

	// A word several of the tier's set names carry says nothing about which
	// of them was meant.
	owned := map[string]string{}
	for code := range codes {
		set, found := b.Sets[code]
		if !found {
			continue
		}
		for word := range strings.FieldsSeq(strings.ToLower(set.Name)) {
			if seen, found := owned[word]; found && seen != code {
				owned[word] = ""
				continue
			}
			owned[word] = code
		}
	}
	said := ""
	for word := range strings.FieldsSeq(strings.ToLower(wording)) {
		code := owned[word]
		if code == "" {
			continue
		}
		if said != "" && said != code {
			return tier
		}
		said = code
	}
	if said == "" {
		said = sealedSet(b, wording, codes)
	}
	if said == "" {
		return tier
	}
	var issued []matcher.Card
	for _, card := range tier {
		if card.SetCode == said {
			issued = append(issued, card)
		}
	}
	if len(issued) == 0 {
		return tier
	}
	return issued
}

// sealedSet returns the code of the one set among the given ones that holds a
// sealed product the wording names, or "" where none or several do.
//
// The wording is the storefront's whole note, which says the promotion as
// well as the product ("Arcane Box Set Promo"), so the product's name has to
// contain it rather than the other way round, with the promotion word cut
// off first. A note down to a single word is not read: every set has a
// product whose name holds "box" or "set".
func sealedSet(b *matcher.Backend, wording string, codes map[string]bool) string {
	var words []string
	for word := range strings.FieldsSeq(wording) {
		if !matcher.IsPromoHeading(word) {
			words = append(words, word)
		}
	}
	if len(words) < 2 {
		return ""
	}
	needle := strings.Join(words, " ")

	found := ""
	for code := range codes {
		for _, uuid := range b.SetSealedUUIDs[code] {
			co, err := b.GetUUID(uuid)
			if err != nil || !matcher.Contains(co.Name, needle) {
				continue
			}
			if found != "" && found != code {
				return ""
			}
			found = code
			break
		}
	}
	return found
}

// stripNumberLetter drops the letters a collector number trails off its
// digits ("058c" -> "058"), and returns "" when there are none to drop or
// nothing but letters ahead of them, leaving the letter-led token and rune
// numbers ("T05", "R01") to answer for themselves.
func stripNumberLetter(number string) string {
	i := len(number)
	for i > 0 && isLetter(number[i-1]) {
		i--
	}
	if i == len(number) || i == 0 || number[i-1] < '0' || number[i-1] > '9' {
		return ""
	}
	return number[:i]
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// collectPrintings gathers the printings in sets that the promo gate and
// the given collector number admit.
func collectPrintings(b *matcher.Backend, inCard *matcher.InputCard, sets []matcher.CandidateSet, allowPromo bool, number string) []matcher.Card {
	var out []matcher.Card
	var cards []matcher.Card
	for _, set := range sets {
		cards = append(cards, set.Cards...)
	}
	for _, card := range cards {
		set, known := b.Sets[card.SetCode]
		if known && !allowPromo && set.Type == "promo" {
			continue
		}
		// A heading saying only "Promos" names every promo set and no main
		// one, but it agrees with none of our set names - ours are spelled
		// "Riftbound Organized Play Promotional Cards" - so core Match
		// finds no edition to narrow on and hands over every printing of
		// the name. The main-set sibling shares the organized-play card's
		// collector number, and answering with it would file a promo's
		// price under the plain printing's uuid.
		if known && set.Type != "promo" && matcher.IsPromoHeading(inCard.Edition) {
			continue
		}
		if number != "" && !strings.EqualFold(number, card.Number) {
			continue
		}
		// The finish deliberately does not filter here. Dropping candidates
		// on it would lose the roughly half of the game sold in a single
		// finish to any storefront that reports the flag wrongly, or does
		// not report it at all; where it does tell two printings of one
		// number apart, preferListedFinish reads it at the end, on a tie the
		// wording could not break.
		out = append(out, card)
	}
	return out
}

// preferBasePrinting keeps the base printings when the input said nothing
// that could choose among a card's alternate arts. Riftbound files those
// under a number of their own rather than under a variant label, so nothing
// tiers them: a feed that lists a card by name alone, as CoolStuffInc's
// does, aliases every art against the base card and prices none of them.
//
// Two things mark an art as an alternate. It carries a letter or star on the
// number the base printing owns plainly - 66a beside 66, 303* beside 303 -
// or it is numbered past the end of the set, the gallery's own public code
// spelling the boundary out ("SFD-224/221" against "SFD-049/221"). An input
// carrying a number, or a word that might name an art, has already chosen
// and is left alone.
func preferBasePrinting(b *matcher.Backend, inCard *matcher.InputCard, number string, cards []matcher.Card) []matcher.Card {
	if len(cards) <= 1 || number != "" || len(qualifierWords(inCard.Variation)) > 0 {
		return cards
	}
	var base []matcher.Card
	for _, card := range cards {
		if card.Number == "" {
			continue
		}
		if last := card.Number[len(card.Number)-1]; last < '0' || last > '9' {
			continue
		}
		set, found := b.Sets[card.SetCode]
		if found && set.BaseSetSize > 0 && leadingNumber(card.Number) > set.BaseSetSize {
			continue
		}
		base = append(base, card)
	}
	if len(base) > 0 {
		return base
	}
	return cards
}

// leadingNumber reads the number a collector number opens with, 0 when it
// opens with a letter as the token and rune numbers do.
func leadingNumber(number string) int {
	out := 0
	for i := 0; i < len(number) && number[i] >= '0' && number[i] <= '9'; i++ {
		out = out*10 + int(number[i]-'0')
	}
	return out
}

// preferListedFinish keeps the printings sold in the finish the listing
// prices, and only when everything above left more than one candidate.
//
// A collector number almost always names one printing, so the number and the
// storefront's wording settle the rest. Two shapes escape them, and in both
// the tied printings are sold in one finish each, so the flag the listing
// already carries names one of them outright: Vendetta numbers the plain
// "Shadow Clone // Tentacle" and its full-art sibling alike, in a set no
// promo tier reaches; and the organized-play promos pair a Best Of foil with
// a Prize Wall nonfoil under one number, typed so differently that a plain
// variation describes neither.
//
// Running last and only on a tie costs no printing: a finish no candidate
// carries leaves the list alone, so the roughly half of the game sold in a
// single finish still answers a storefront that reports the flag wrongly, or
// does not report it at all.
func preferListedFinish(inCard *matcher.InputCard, cards []matcher.Card) []matcher.Card {
	if len(cards) <= 1 {
		return cards
	}
	finish := matcher.FinishNonfoil
	if inCard.Foil {
		finish = matcher.FinishFoil
	}
	var sold []matcher.Card
	for _, card := range cards {
		if card.HasFinish(finish) {
			sold = append(sold, card)
		}
	}
	if len(sold) == 0 {
		return cards
	}
	return sold
}

// qualifierWords are the storefront's qualifying words: what the variation
// says beyond the collector number and the finish vocabulary that rides
// along with it.
func qualifierWords(variation string) []string {
	var out []string
	for field := range strings.FieldsSeq(strings.ToLower(variation)) {
		if strings.ContainsAny(field, "0123456789") {
			continue
		}
		switch field {
		case "foil", "nonfoil", "non-foil", "normal":
			continue
		}
		out = append(out, field)
	}
	return out
}

// longestDescribed keeps the printings wearing the longest label the
// storefront's wording said.
//
// The labels nest. "T1 Worlds Champion Signature Edition Bundle Serial
// Numbered" says everything the same bundle without the serial says, so a
// wording spelling the longer one describes both printings and the pair
// reads as an ambiguity that prices neither - which is how the five
// serial-numbered printings read. The longest label a wording describes is
// the one it meant, the way gundam reads its nested event labels.
func longestDescribed(cards []matcher.Card) []matcher.Card {
	longest := 0
	for _, card := range cards {
		if n := labelLength(card.PromoTypes); n > longest {
			longest = n
		}
	}
	var out []matcher.Card
	for _, card := range cards {
		if labelLength(card.PromoTypes) == longest {
			out = append(out, card)
		}
	}
	return out
}

// labelLength is how much of a wording a printing's promo types account for.
func labelLength(promoTypes []string) int {
	var total int
	for _, promoType := range promoTypes {
		total += len(promoType)
	}
	return total
}

// wordsDescribe reports whether every word of every promo type is said in
// the storefront wording, compared word by word in lower case like
// qualifiedPromoName does.
func wordsDescribe(wording string, promoTypes []string) bool {
	for _, promoType := range promoTypes {
		if !matcher.SlugDescribes(wording, promoType) {
			return false
		}
	}
	return true
}

// extractNumber pulls the collector number out of the scraper-supplied
// Variation. Core Match may append parenthetical chunks split off the input
// name, so only the first field containing a digit counts - numbers may be
// letter-prefixed ("T1", "SP3"), letter-suffixed ("66a"), or starred
// ("227*"). A full public code ("OGN-066a/298") reduces to its number, and
// the result is canonicalized exactly like the loader's numbers so
// zero-padded feeds compare equal.
//
// A trailing "s" becomes the star: storefronts number the signed showcase
// printings "302s" where the gallery numbers them "302*". The two never
// collide, since no published number ends in a letter other than a, b or c.
func extractNumber(variation string) string {
	number := ""
	for field := range strings.FieldsSeq(variation) {
		if strings.ContainsAny(field, "0123456789") {
			number = field
			break
		}
	}
	if idx := strings.LastIndexByte(number, '-'); idx >= 0 {
		number = number[idx+1:]
	}
	number = strings.Split(number, "/")[0]
	number = canonicalNumber(number)
	if len(number) > 1 && (number[len(number)-1] == 's' || number[len(number)-1] == 'S') {
		if last := number[len(number)-2]; last >= '0' && last <= '9' {
			number = number[:len(number)-1] + "*"
		}
	}
	return number
}
