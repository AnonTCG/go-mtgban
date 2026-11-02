package onepiece

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/mtgban/go-mtgban/matcher"
)

// Rules implements matcher.GameRules for One Piece. A card is identified
// by name + collector number, with the variant label separating the
// printings that share a number (alternate arts, parallels, event
// printings). Foil never gates anything: a product is one printing whatever
// its stamping, and both flag values resolve to the same uuid.
type Rules struct{ matcher.DefaultRules }

// fullNumberRe matches the game's collector number shapes: "OP01-001",
// "ST01-001", "EB01-023", "P-001", with an optional letter tail
// (cardtrader suffixes alternate arts "OP01-001a").
//
// The tail takes the Greek letters as well, which cardtrader numbers the
// corrected runs with ("OP01-002β", "OP01-016βa"). Leaving them out does not
// leave the number unread - it leaves it read as something else, because a
// number this fails on falls through to the first digit-leading word, and
// behind one of these numbers that is a digit quoted out of the card's own
// rules text.
var fullNumberRe = regexp.MustCompile(`^[A-Za-z]+[0-9]*-[0-9]+[a-zA-Z\x{03b1}\x{03b2}]*$`)

// dashNumberRe matches the collector number hung inside a name after a
// dash ("Monkey.D.Luffy - P-043").
var dashNumberRe = regexp.MustCompile(`\s+-\s+([A-Za-z]+[0-9]*-[0-9]+[a-zA-Z]*)\b`)

// dashTailRe matches the bare number coolstuffinc hangs off a name in
// place of the full code ("Trafalgar Law - 008"). It only matches at the
// end: two cards are named with a dash of their own, and no card name ends
// in one followed by a number.
var dashTailRe = regexp.MustCompile(`\s+-\s+([0-9]+[a-zA-Z]?)$`)

// trailingCodeRe matches the collector number a storefront writes behind a
// name in parentheses of its own ("Gloriosa (Grandma Nyon) (OP07-041)"). It
// only matches at the end, and only a full code: a parenthetical anywhere
// else may be part of the name.
var trailingCodeRe = regexp.MustCompile(`\s*\(([A-Za-z]+[0-9]*-[0-9]+[a-zA-Z]*)\)$`)

// Prefilter splits the parenthetical decorations off the name before the
// canonical-name lookup: storefronts write "Roronoa Zoro (OP01-001) (V.2)",
// "Shanks (001) (Parallel)" and "Monkey.D.Luffy - P-043 (Convention Promo
// 2024)". A full name that is itself canonical stays as it is - the epithet
// parentheticals ("Mr.2.Bon.Kurei (Bentham)") are part of the name.
func (Rules) Prefilter(b *matcher.Backend, inCard *matcher.InputCard) {
	splitDecorations(b, inCard)
	adoptNumberedName(b, inCard)
}

// splitDecorations moves the parenthetical qualifiers and the dash-hung
// collector number out of the name, leaving the bare spelling behind.
func splitDecorations(b *matcher.Backend, inCard *matcher.InputCard) {
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; found {
		return
	}
	// The catalog joins two DON!! cards to their label with " // " ("DON!!
	// Card // Green Compass"), which a datastore naming the card alone files
	// as the variant.
	head, label, joined := strings.Cut(inCard.Name, " // ")
	if joined {
		inCard.Name = head
		inCard.AddToVariant(label)
	}
	split := splitDashNumber(inCard)
	// A card whose own name ends in a parenthetical wears two of them once
	// the storefront writes the number behind it, and the split below cannot
	// tell which is which: it keeps the head and sweeps the rest into the
	// variation, so the epithet goes with the number and what is left is
	// another card's name ("Gloriosa" is OP17-046, not OP07-041). The number
	// is the one parenthetical never part of a name, so peel that alone and
	// ask again, and leave the name untouched unless the answer is yes.
	if fields := trailingCodeRe.FindStringSubmatch(inCard.Name); fields != nil {
		peeled := strings.TrimSuffix(inCard.Name, fields[0])
		if _, found := b.CanonicalNames[matcher.Normalize(peeled)]; found {
			inCard.Name = peeled
			inCard.AddToVariant(fields[1])
			return
		}
	}
	if strings.Contains(inCard.Name, "(") {
		vars := matcher.SplitVariants(inCard.Name)
		if len(vars) > 1 {
			inCard.Name = vars[0]
			inCard.AddToVariant(strings.Join(vars[1:], " "))
		}
	}
	// The same feed writes its qualifiers behind the number ("Trafalgar Law
	// - 008 (Parallel)"), where the bare tail is no longer at the end of the
	// name to be read; the parentheticals are off by now, so try again, on
	// the same terms - a name that is itself canonical is left alone.
	if split {
		return
	}
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; found {
		return
	}
	splitDashNumber(inCard)
}

// adoptNumberedName adopts the catalog's decorated spelling of a name the
// storefront wrote plain, where the number says the plain spelling is not
// the one meant.
//
// A handful of printings carry their occasion inside the name - both P-110
// printings are "Monkey.D.Luffy (4th Anniversary)", and the pre-release
// tournament cards read the same way - while the storefront writes the bare
// "Monkey.D.Luffy". That bare name is perfectly canonical, and 270 other
// printings answer to it, so the lookup succeeds and the number then deletes
// every one of them: the listing dies as an unknown variant with nothing
// having gone wrong with its variant. AdjustName's own reading of the number
// never runs, because it is only asked once the canonical lookup has failed.
//
// nameAtNumber does the reading, and its guards are what make it safe here: a
// code several names answer says nothing about which was meant, and the
// spelling adopted has to be close to the one written. A name with a printing
// at the number is already right, so it is never asked about.
func adoptNumberedName(b *matcher.Backend, inCard *matcher.InputCard) {
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; !found {
		return
	}
	number := extractNumber(inCard.Variation)
	if number == "" {
		return
	}
	for _, uuid := range b.Hashes[matcher.Normalize(inCard.Name)] {
		co, found := b.UUIDs[uuid]
		if !found || co.Sealed {
			continue
		}
		if numberMatches(number, co.Number) {
			return
		}
	}
	if name := nameAtNumber(b, inCard.Name, number); name != "" {
		inCard.Name = name
	}
}

// splitDashNumber moves the collector number a storefront hangs off a name
// after a dash into the variation, reporting whether it found one. The
// number goes in front of whatever the variation already holds: the retry
// runs with the parenthetical qualifiers moved there, and those carry
// ordinals of their own ("Judge Pack Vol. 5", "1st Anniversary Set") that
// extractNumber's digit-leading fallback would read as the number instead.
func splitDashNumber(inCard *matcher.InputCard) bool {
	m := dashNumberRe.FindStringSubmatch(inCard.Name)
	if m != nil {
		inCard.Name = strings.Replace(inCard.Name, m[0], "", 1)
		inCard.Variation = strings.TrimSpace(m[1] + " " + inCard.Variation)
		return true
	}
	m = dashTailRe.FindStringSubmatch(inCard.Name)
	if m != nil {
		inCard.Name = strings.TrimSuffix(inCard.Name, m[0])
		inCard.Variation = strings.TrimSpace(m[1] + " " + inCard.Variation)
		return true
	}
	return false
}

// nameAliases maps a storefront's spelling of a card onto the name the
// datastore files it under, keyed by the normalized input name. Only
// spellings no canonical name answers belong here - the table is consulted
// after the canonical lookup has already failed.
var nameAliases = map[string]string{
	// The game's resource card is "DON!! Card" on the card and in the
	// catalog the datastore is built from; several storefronts call it by
	// the shout alone. The prefix fallback below cannot rescue it: every
	// character named Don reads as a prefix match too.
	"don": "DON!! Card",
}

// AdjustName provides a prefix fallback for truncated feeds, adopting the
// one name among the prefix matches that carries the input's number. Names
// compare normalized, so punctuation variants of one name are not read as
// ambiguity.
func (Rules) AdjustName(b *matcher.Backend, inCard *matcher.InputCard) {
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; found {
		return
	}
	if alias, found := nameAliases[matcher.Normalize(inCard.Name)]; found {
		if _, known := b.CanonicalNames[matcher.Normalize(alias)]; known {
			inCard.Name = alias
			return
		}
	}
	number := extractNumber(inCard.Variation)
	uuids, err := b.SearchHasPrefix(inCard.Name)
	if err == nil {
		var match, matchNorm string
		for _, uuid := range uuids {
			co, err := b.GetUUID(uuid)
			if err != nil || co.Sealed {
				continue
			}
			if number != "" && !numberMatches(number, co.Number) {
				continue
			}
			norm := matcher.Normalize(co.Name)
			if match != "" && matchNorm != norm {
				return
			}
			match, matchNorm = co.Name, norm
		}
		if match != "" {
			inCard.Name = match
			return
		}
	}
	if name := nameAtNumber(b, inCard.Name, number); name != "" {
		inCard.Name = name
	}
}

// nameAtNumber returns the datastore's spelling of the card a storefront
// misspelled, read off the collector number it wrote beside the name.
//
// A storefront that cannot spell a name still writes the number, and a full
// code names one printing outright, so the code says which card the listing
// is. Cardmarket types an acute accent where the card has an apostrophe
// ("Who´s.Who"), drops a letter ("Artifical Devil Fruit SMILE"), swaps two
// ("Crone Oli"), or names a character by the last part of their name alone
// ("Garp" for "Monkey.D.Garp").
//
// Two things have to hold before the spelling is adopted. Every printing
// wearing the code has to share one name: a code several names answer says
// nothing about which was meant. And the misspelling has to be close to
// that name - a prefix or a suffix of it, or within two edits, one typo or
// a transposition. The number is the storefront's own claim and it is
// sometimes another card's, so without the closeness test a stray number
// would rename a card into whatever it pointed at.
//
// Only a full code is read. A bare tail ("051") is the same number in every
// set of the game, and the edition that would say which set has not been
// resolved yet when a name is being fixed up.
func nameAtNumber(b *matcher.Backend, name, number string) string {
	if number == "" || !fullNumberRe.MatchString(number) {
		return ""
	}
	var match, matchNorm string
	for _, uuid := range b.GetUUIDs() {
		co, err := b.GetUUID(uuid)
		if err != nil || co.Sealed || !numberMatches(number, co.Number) {
			continue
		}
		norm := matcher.Normalize(co.Name)
		if match != "" && matchNorm != norm {
			return ""
		}
		match, matchNorm = co.Name, norm
	}
	if match == "" || !matcher.CloseName(name, match) {
		return ""
	}
	return match
}

// setCodePrefixRe matches a set code worn as an edition prefix: cardtrader
// spells its expansions "OP-01: Romance Dawn", coolstuffinc "OP03 - Pillars
// of Strength", with the compound codes ("OP15-EB04 - ...") in the same
// shape.
var setCodePrefixRe = regexp.MustCompile(`^[A-Za-z]+-?[0-9]+(?:-[A-Za-z]+[0-9]+)?\s*[-:]\s*`)

// AdjustEdition trims the game-name and set-code prefixes storefronts
// decorate set names with. An edition that still matches no set simply does
// not narrow the candidates.
//
// When the rest of the wording asks for a variant printing, the edition
// stops narrowing them: a variant usually shares its collector number with
// the base card while being filed in another set - the alternate arts in
// PRB-01, the event printings in OP-PR - and storefronts name the base
// card's set for all of them. Narrowing on it would delete the printing
// being asked for before FilterCards ever tiers the candidates, pricing a
// promo as the base common. The edition still reaches the tiering as
// wording, so it goes on describing a variant rather than selecting one.
func (r Rules) AdjustEdition(b *matcher.Backend, inCard *matcher.InputCard) {
	edition := r.AliasEdition(b, inCard.Edition)
	// A wording abbreviating an event's name is the storefront saying which
	// set the listing is in, however plainly its edition says the other one.
	named := false
	promo := promoSetBegun(b, inCard)
	if promo != "" {
		edition = promo
		named = true
	}
	// The same, for the sets a base set's event printings are filed in. A
	// storefront that stocks one writes the event on the card and the base
	// set in the edition field, so neither field names the event set alone:
	// Cool Stuff Inc sells "(Super Pre-Release)" against an edition saying
	// "Starter Deck: Straw Hat Crew", where the catalog files the printing
	// in "Super Pre-Release Starter Deck 1: Straw Hat Crew". Reading the two
	// together is the only thing that reaches it - an event printing wears
	// no label of its own, so nothing but the set name tells it from the
	// card it reprints, and the edition alone names that card.
	event := eventSetNamed(b, inCard, edition)
	if event != "" {
		edition = event
		named = true
	}

	// And for the reprints a later set carries. A treasure rare is printed
	// in the set after the one the card is numbered for, and this
	// storefront writes that set's code in front of the treatment -
	// "OP07-109 (OP08 Treasure Rare)" against an edition still saying OP07
	// - so the edition names the set the printing is not in and the gate
	// below deletes the only right answer.
	coded := codedSetNamed(b, inCard, edition)
	if coded != "" {
		edition = coded
		named = true
	}

	// TCGplayer files two Learn Together Deck Set cards under the Premium
	// Booster Vol. 2 reprint block instead: see foldedPrintings.
	set, err := b.GetSetByName(edition)
	if err == nil {
		folded, ok := foldedPrintings[set.Code]
		if ok {
			number := inputNumber(b, inCard)
			code, ok := folded[number]
			if ok {
				target, err := b.GetSet(code)
				if err == nil {
					edition = target.Name
					named = true
					inCard.Foil = false
				}
			}
		}
	}
	inCard.Edition = edition

	// PromoWildcard is the flag Match already reads to skip edition
	// selection entirely, both the exact match and the looser one: a
	// storefront's "EB01 - Memorial Collection" does not equal the set's
	// "Extra Booster: Memorial Collection" but is contained in it, and
	// that alone was enough to delete the promo printing.
	if variantPointedAt(b, inCard, named) {
		inCard.PromoWildcard = true
	}
}

// editionAliases maps a storefront's shelf spelling onto the code of the set
// the datastore files it under, keyed by the normalized input; the set's
// name is read off the datastore, since TCGplayer renames sets and keeps
// their codes. Only spellings canonicalEdition's word-for-word scoring
// cannot resolve belong here.
var editionAliases = map[string]string{
	// Cool Stuff Inc drops the space in "Gear 5", so canonicalEdition never
	// scores this far enough to read the leading ST21 code.
	"st21starterdeckgear5": "ST-21",
}

// AliasEdition spells an edition string toward a set name using the string
// alone. See matcher.GameRules.
func (Rules) AliasEdition(b *matcher.Backend, edition string) string {
	edition = strings.TrimSpace(edition)
	norm := matcher.Normalize(edition)
	if code, found := editionAliases[norm]; found {
		set, err := b.GetSet(code)
		if err == nil {
			return set.Name
		}
	}
	for _, prefix := range []string{"One Piece Card Game", "One Piece TCG", "One Piece"} {
		if strings.HasPrefix(edition, prefix) {
			edition = strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(edition, prefix), ":-"))
			break
		}
	}
	// The code the prefix spells is the storefront saying which set it
	// means, and it survives the trim: it is the only thing telling the
	// members of a set family apart once the shared name is all that is
	// left of the wording.
	code := strings.Trim(setCodePrefixRe.FindString(edition), " -:")
	edition = setCodePrefixRe.ReplaceAllString(edition, "")
	edition = strings.TrimSpace(strings.TrimSuffix(edition, "Singles"))

	// Storefronts spell a set's name their own way - dropping the ordinal
	// out of "Starter Deck 1: Straw Hat Crew", writing "500 Years into the
	// Future" for the set called "500 Years in the Future" - and a name
	// that is neither equal to nor contained in the datastore's selects
	// nothing at all. Snap it back to the set it describes when one set
	// describes it better than every other, keeping any promo line in
	// front so the event printings are still the ones being named.
	prefix := promoLineRe.FindString(edition)
	base := edition[len(prefix):]
	if prefix == "" {
		base = shelfLineRe.ReplaceAllString(base, "")
	}
	canon := canonicalEdition(b, base, code)
	if canon != "" {
		edition = prefix + canon
	}
	return edition
}

// eventSetNamed answers the event set a listing's wording names when its
// edition names that set's base, and "" when it names none or more than one.
//
// The catalog builds an event set's name on its base set's - "Super
// Pre-Release Starter Deck 1: Straw Hat Crew" against "Starter Deck 1: Straw
// Hat Crew" - so what is left once the base name is taken out is the marker
// naming the event. The variation has to spell that marker whole: the
// printings this chooses between share a name, a number and an empty label,
// so a listing merely mentioning a word of the marker has said nothing that
// tells them apart.
func eventSetNamed(b *matcher.Backend, inCard *matcher.InputCard, base string) string {
	if inCard.Variation == "" || base == "" {
		return ""
	}
	var name string
	for _, uuid := range b.Hashes[matcher.Normalize(inCard.Name)] {
		co, found := b.UUIDs[uuid]
		if !found || co.Sealed || !isEventSet(co.SetCode) {
			continue
		}
		set, found := b.Sets[co.SetCode]
		if !found || !matcher.Contains(set.Name, base) {
			continue
		}
		marker := strings.ReplaceAll(strings.ToLower(set.Name), strings.ToLower(base), " ")
		// "(Super Pre-Release Edition)": the edition is the catalog's word
		// for the run, which no listing writes.
		slug := strings.TrimSuffix(matcher.PromoTypeSlug(marker), "edition")
		if slug == "" || !matcher.SlugDescribes(inCard.Variation, slug) {
			continue
		}
		if name != "" && name != set.Name {
			return ""
		}
		name = set.Name
	}
	return name
}

// promoSetBegun answers the promotional set a listing's wording names by the
// opening words of a printing's label, and "" when it names none.
//
// A storefront shortens the event a promo was handed out at - Cool Stuff Inc
// writes "(Championship 25-26)" for the catalog's "Championship 25 26
// Regionals Season 1" - and files the listing under the base card's set,
// which is the one set that printing is not in. The abbreviation is too short
// for the variant tiering to read as a label, so nothing at all says the
// listing is a promo and the base common answers it, wearing the promo's
// $1250 buy price. Nothing but the edition reaches the right card: the
// printing shares its number with the base card, so the number cannot pick
// it, and a tiering that never read the label cannot prefer it either.
//
// Four things hold the rule to that case. Only a full collector number is
// read, since a bare tail is the same number in every set of the game. The
// run has to open the label rather than sit anywhere in it, because a
// storefront shortening a name keeps its front. It has to be two words long,
// because one word is what a wording lands on by coincidence. And a wording
// that spells some label out in full is left alone, along with a run opening
// the labels of two different sets: the first is already the tiering's to
// answer and the second names nothing.
func promoSetBegun(b *matcher.Backend, inCard *matcher.InputCard) string {
	number := inputNumber(b, inCard)
	if !fullNumberRe.MatchString(number) {
		return ""
	}
	var promoSet *matcher.Set
	var code string
	for _, uuid := range b.Hashes[matcher.Normalize(inCard.Name)] {
		co, found := b.UUIDs[uuid]
		tokens := labelTokens(co.Card)
		if !found || co.Sealed || len(tokens) == 0 ||
			!numberMatches(number, co.Number) {
			continue
		}
		if matcher.SlugDescribesAny(inCard.Variation, tokens) {
			return ""
		}
		if !slugsRunOf(inCard.Variation, tokens) {
			continue
		}
		set, found := b.Sets[co.SetCode]
		if !found {
			continue
		}
		if code != "" && code != co.SetCode {
			return ""
		}
		promoSet, code = set, co.SetCode
	}
	if !setIsPromotional(promoSet) {
		return ""
	}
	return promoSet.Name
}

// slugsRunOf reports whether a wording spells a run of any of the labels a
// printing wears, as a run of at least two whole words and short of the whole
// label. It is SlugDescribes stopped short of the end: the slug has lost its
// spaces, so the wording's words are joined back up a run at a time and the
// label asked whether it holds them. A wording spelling a label out in full
// holds every shorter run too, so it is promoSetBegun that tells the
// abbreviation from the full name.
//
// The run is looked for anywhere in the label rather than at its front. A
// storefront shortening a name keeps a run of it, but not always the first:
// it drops the collection the promo was sold in ("Best Selection Vol. 5" for
// "Premium Card Collection Best Selection Vol. 5"), the year it was handed
// out in ("PSA Magazine promo" for "2025 PSA Magazine promo"), or the
// treatment the signature is printed on. Two words is what keeps that
// generous reading honest: one word is what a wording lands on by
// coincidence, and the prose a storefront writes about the artwork - "Sword",
// "Strings, Foil" - is exactly what would land there.
func slugsRunOf(wording string, slugs []string) bool {
	words := strings.Fields(strings.ToLower(wording))
	for _, slug := range slugs {
		if slug == "" {
			continue
		}
		for i := range words {
			var joined string
			for j := i; j < len(words); j++ {
				joined += matcher.PromoTypeSlug(words[j])
				if len(joined) >= len(slug) || !strings.Contains(slug, joined) {
					break
				}
				if j > i {
					return true
				}
			}
		}
	}
	return false
}

// variantPointedAt reports whether the input's own wording asks for a
// variant printing: a letter tail or "(V.n)" index, or words naming the
// variant label of some printing of this card. Only the variation is read,
// never the edition - the edition is what this decides whether to trust.
func variantPointedAt(b *matcher.Backend, inCard *matcher.InputCard, named bool) bool {
	number := inputNumber(b, inCard)
	if wantsVariant(inCard, number) {
		return true
	}
	// The guard below reads "the number being asked about", and a bare
	// number is not one: it is the same number in every set of the game, so
	// a card that only shares its tail passes as the card being asked
	// about. The edition says which set the listing is in, so the whole
	// number is read out of it first. The printings this is meant to unpin
	// the edition for wear the base card's own number and pass; a different
	// set's card wears its own and does not.
	if full := fullNumberInEdition(b, inCard, number); full != "" {
		number = full
	}
	wording := strings.ToLower(inCard.Variation)
	for _, uuid := range b.Hashes[matcher.Normalize(inCard.Name)] {
		co, found := b.UUIDs[uuid]
		tokens := labelTokens(co.Card)
		if !found || co.Sealed || len(tokens) == 0 {
			continue
		}
		// Only a printing of the number being asked about can say that this
		// input wants a variant. A label another number wears - the card
		// reprinted elsewhere under its own - would otherwise unpin the
		// edition for an input the edition was answering perfectly, and the
		// widened pool then hands back the base printing.
		if number != "" && !numberMatches(number, co.Number) {
			continue
		}
		// A run of a label points at the variant as plainly as the whole
		// of one: the storefront that writes "Gold-Stamped Signature" for
		// the catalog's "Alternate Art Gold-Stamped Signature" has said
		// which printing it means, and pinning the edition to the set the
		// card was first printed in deletes that printing outright.
		if matcher.SlugDescribesAny(wording, tokens) {
			return true
		}
		// A run of a label points at the variant as plainly as the whole of
		// one: the storefront writing "Gold-Stamped Signature" for the
		// catalog's "Alternate Art Gold-Stamped Signature" has said which
		// printing it means, and pinning the edition to the set the card
		// was first printed in deletes that printing outright. Not where
		// the same run already named the set the listing is filed in,
		// though - that wording has said where to look, and unpinning the
		// edition throws the answer away.
		if !named && slugsRunOf(wording, tokens) {
			return true
		}
		// A set's own word for a treatment points at it as plainly as the
		// game's: the wording says manga and the printing is the one
		// Premium Booster Vol. 2 files that art under, which the edition
		// naming the set the card was first printed in would delete.
		if setVocabularyNames(wording, co.SetCode, tokens) {
			return true
		}
	}
	return false
}

// PlainNumber implements matcher.GameRules. A number carries its set code and pads the ordinal behind it, where a person writes the ordinal alone: OP01-007 is card 7.
func (Rules) PlainNumber(number string) string {
	return matcher.PlainOrdinal(number)
}

// FilterCards narrows candidates by edition, collector number and variant.
// The variant tiering mirrors the number sharing in the data: when the
// input's wording describes a variant label, the printings it describes
// win; a bare input keeps the base printing when one exists; cardmarket's
// positional "(V.n)" wording keeps the base for V.1 and the variants
// otherwise.
func (Rules) FilterCards(b *matcher.Backend, inCard *matcher.InputCard, sets []matcher.CandidateSet) []matcher.Card {
	number := inputNumber(b, inCard)

	var candidates []matcher.Card
	var cards []matcher.Card
	for _, set := range sets {
		cards = append(cards, set.Cards...)
	}
	for _, card := range cards {
		if number != "" && !numberMatches(number, card.Number) {
			continue
		}
		candidates = append(candidates, card)
	}

	// A Premium Booster volume's V.1 is the plain row: untagged where the
	// volume prints one, "reprint"-tagged where it prints that instead of
	// an unlabelled row. No volume prints both, so keeping either is safe.
	if numbered(b, inCard.Name) && positionalIndex(inCard.Variation) == 1 {
		set, ok := singlePremiumBoosterSet(b, inCard.Edition)
		if ok {
			var plain []matcher.Card
			for _, card := range candidates {
				if card.SetCode == set.Code && (len(card.PromoTypes) == 0 || slices.Contains(card.PromoTypes, "reprint")) {
					plain = append(plain, card)
				}
			}
			return plain
		}
	}

	if len(candidates) <= 1 {
		return candidates
	}

	// A name whose printings wear no collector number has nothing but the
	// set and the label telling them apart, and the labels are prose the
	// catalog wrote rather than tags, so both are read here instead of by
	// the tiering below. Every other name is left to the tiering on
	// purpose: a variant shares its base card's number while being filed in
	// another set, and storefronts name the base card's set for it. The
	// DON!! cards are not reprinted that way - each set prints its own - so
	// an edition naming one of them names the set the listing is in, and a
	// wording that happens to describe another set's DON!! must not reach
	// past it.
	if !numbered(b, inCard.Name) {
		candidates = editionTiebreak(b, inCard, candidates)
		if len(candidates) <= 1 || !editionNamesSet(b, inCard.Edition) {
			return candidates
		}
		return labelOverlap(b, inCard, candidates, false)
	}

	// A corrected printing is filed on the base card's number with the
	// correction in its label, so every printing of that number answers a
	// listing selling one - the base card first among them, which is the
	// one thing the listing said it is not. The tiering below cannot settle
	// it either: the storefront writes "Alternate Art" where the catalog
	// writes "Parallel", so the plainer label is the one a wording naming
	// both the run and the art describes, and the bare printing wins a
	// listing that is not it.
	narrowed := errataNarrow(b, inCard, candidates)
	if len(narrowed) == 1 {
		return narrowed
	}
	candidates = narrowed

	// A tournament printing is handed out for a finishing place, and the
	// catalog spells that place at the end of the label beside the event
	// that awarded it. A listing naming no place is not asking for one.
	// A wording naming the manga art names a printing the catalog spells
	// another way, and the plainer label beside it answers first.
	if picked := mangaChosen(b, inCard, candidates); picked != nil {
		return picked
	}
	if picked := placeChosen(b, inCard, candidates); picked != nil {
		return picked
	}
	narrowed = placeNarrow(b, inCard, candidates)
	if len(narrowed) == 1 {
		return narrowed
	}
	candidates = narrowed

	// The letter tail cardtrader appends to a number ("OP01-001a") means a
	// variant printing without saying which; the V.n index says the same.
	// Either demand drops the base printing from consideration.
	described, base, variants := tierByVariant(b, inCard, candidates)
	described = packPairNamed(b, strings.ToLower(inCard.Variation+" "+inCard.Edition), described, variants)
	if len(described) > 0 {
		narrowed := editionTiebreak(b, inCard, described)
		narrowed = lastNamedTiebreak(b, inCard.Variation, narrowed)
		narrowed = unnumberedTiebreak(number, narrowed)
		return finishTiebreak(inCard, narrowed)
	}
	if wantsVariant(inCard, number) {
		if len(variants) == 0 && wantsUnnamedVariant(inCard) {
			// The listing said it is not the base printing, and the
			// catalog holds no other at this number: answering with the
			// base prices the card the listing went out of its way to
			// say it is not.
			return nil
		}
		if len(variants) > 0 {
			// The edition decides first and the index only speaks for what
			// it leaves: cardmarket's index counts one shelf's products, so
			// reading it across sets would answer a storefront naming a
			// promo set with a booster set's parallel.
			narrowed := editionTiebreak(b, inCard, variants)
			if picked := variantAtIndex(inCard, editionTiebreak(b, inCard, base), narrowed); picked != nil {
				return picked
			}
			// A bare unnamed-variant word crosses several labels at one
			// number; the plain one alone is what it asked for.
			if len(narrowed) > 1 && wantsUnnamedVariant(inCard) {
				picked := plainUnnamedVariant(narrowed)
				if picked != nil {
					return picked
				}
			}
			return narrowed
		}
		// No printing at this number wears a label at all, so the demand
		// is for something the catalog does not tell these apart by: an
		// event's printings carry no label, and the set they are filed in
		// is the whole of what separates them from the card they reprint.
		// The letter tail is still the storefront saying this is not the
		// plain printing, and the edition is what can say which it is, so
		// the question falls to it rather than being given up on.
	}
	// The wording spelled no label out, but a storefront shortens one the
	// same way it shortens an event's name: it drops the treatment the
	// signature is printed on and keeps the signature. Where a run of one
	// candidate's label is all the wording says, and no other candidate's
	// label it says any of, that candidate is the printing being named -
	// and the base card standing beside it is the one thing it is not.
	runNamed := runNamedVariants(b, inCard, number, variants)
	if len(runNamed) == 1 {
		return runNamed
	}
	if len(base) > 0 {
		return finishNamedTiebreak(inCard, editionTiebreak(b, inCard, base))
	}
	// Every candidate wears a label, the wording spelled none of them out,
	// and the number has already said which card this is - so the label is
	// the only thing left the listing can be asking about. The catalog
	// prefixes an event label with the base set's code ("OP16 Release
	// Event", "OP16 Release Event Winner") and the storefront never writes
	// it, so the whole-label test the tiering above runs answers nothing and
	// both printings come back aliased. Counting the label's own words the
	// wording did say separates them, and refuses whatever it cannot.
	candidates = editionTiebreak(b, inCard, candidates)
	if len(candidates) <= 1 {
		return candidates
	}
	candidates = finishNamedTiebreak(inCard, candidates)
	if len(candidates) <= 1 {
		return candidates
	}
	return labelOverlap(b, inCard, candidates, true)
}

// namedFinish reads the finish a wording states outright, and whether it
// stated one at all.
//
// Cardtrader sends no finish property for One Piece: a listing carries a
// language and a rarity and nothing else, so the flag beside the wording is
// the zero value for every listing of the game. The wording is the only
// place a listing can say which of two printings at one number it is.
//
// Non-foil is asked first because the word for it contains the other.
func namedFinish(inCard *matcher.InputCard) (string, bool) {
	if matcher.Contains(inCard.Variation, "nonfoil") {
		return matcher.FinishNonfoil, true
	}
	if matcher.Contains(inCard.Variation, "foil") {
		return matcher.FinishFoil, true
	}
	return "", false
}

// finishNamedTiebreak narrows to the printing whose finish the wording named,
// when exactly one candidate carries it. A wording naming none is left alone:
// the flag it would fall back on reads nonfoil for every listing of the game,
// so answering with it would hand every tie to the nonfoil printing on the
// strength of a field cardtrader never sent.
func finishNamedTiebreak(inCard *matcher.InputCard, cards []matcher.Card) []matcher.Card {
	if len(cards) <= 1 {
		return cards
	}
	finish, named := namedFinish(inCard)
	if !named {
		return cards
	}
	var kept []matcher.Card
	for _, card := range cards {
		if card.HasFinish(finish) {
			kept = append(kept, card)
		}
	}
	if len(kept) != 1 {
		return cards
	}
	return kept
}

// variantAtIndex narrows the variants tier to the one printing cardmarket's
// "(V.n)" index points at, when the datastore's own numbering says which that
// is. Bandai numbers a card's alternate printings by appending "_p1", "_p2"
// to its collector number, in the order cardmarket files them behind the base
// printing, so V.2 is "_p1" and V.3 is "_p2".
//
// The index is a position in cardmarket's own shelf, not a name for anything,
// so it only means this where the shelf is the plain one it describes: one
// base printing for V.1 to have been, and exactly one variant wearing the
// suffix asked for. Cardmarket files an un-indexed product beside indexed
// ones on its promo shelves, where V.1 is already an alternate rather than
// the base - and on those shelves every printing of ours carries a promo
// label, so there is no base tier and the index is left unread rather than
// spending V.2 on the printing V.1 stands for.
func variantAtIndex(inCard *matcher.InputCard, base, variants []matcher.Card) []matcher.Card {
	index := positionalIndex(inCard.Variation)
	if index < 2 || len(base) != 1 || base[0].Identifiers == nil {
		return nil
	}
	bandaiID := base[0].Identifiers["bandaiId"]
	if bandaiID == "" {
		return nil
	}
	want := bandaiID + "_p" + strconv.Itoa(index-1)
	var picked []matcher.Card
	for _, card := range variants {
		if card.Identifiers["bandaiId"] == want {
			picked = append(picked, card)
		}
	}
	if len(picked) != 1 {
		return nil
	}
	return picked
}

// finishTiebreak narrows a tier the wording described in full to the
// printings sold in the finish the storefront priced.
//
// The event printings a promo number carries are one card issued several
// times over, and a storefront names them all at once: "P-115 (OP15 Release
// Event - Winner)" says both the set's release-event card and the winner's
// copy of it, because the winner's label is the plain one with a word
// appended. What separates them is the stamping - the participation card is
// sold plain, the winner's foil - so the finish being priced is the last
// thing left that says which one the listing is.
//
// Only a tier the wording named throughout is narrowed this way. A wording
// naming none of them has not said the listing is one of these at all, and
// answering it with whichever printing happens to carry the right stamping
// would price a product the catalog does not hold as one it does.
func finishTiebreak(inCard *matcher.InputCard, cards []matcher.Card) []matcher.Card {
	if len(cards) <= 1 {
		return cards
	}
	finish := matcher.FinishNonfoil
	if inCard.Foil {
		finish = matcher.FinishFoil
	}
	var stamped []matcher.Card
	for _, card := range cards {
		if card.HasFinish(finish) {
			stamped = append(stamped, card)
		}
	}
	if len(stamped) == 0 {
		return cards
	}
	return stamped
}

// promoRemainderNamesSet reports whether what a promo line leaves behind
// names a set. Pure punctuation names nothing ("Promos: -"), and so does the
// possessive a storefront sometimes trails ("Promo: 's"), which is punctuation
// wearing an s rather than a name. Normalizing keeps the s, so the
// possessive is named here.
func promoRemainderNamesSet(base string) bool {
	trimmed := strings.Trim(base, " '\u2019\u02bc-:.,")
	if strings.EqualFold(trimmed, "s") {
		return false
	}
	return matcher.Normalize(trimmed) != ""
}

// editionNamesSet reports whether a storefront's edition is a set of the
// game rather than a shelf it files a whole game under.
//
// The labels tell apart the printings of one set, and the set has to have
// been named for them to be read: cardmarket files half its promos under
// "One Piece Products", where the pool is every DON!! the game ever printed
// and one character name is spelled by half a dozen of them across as many
// sets. A listing naming a card the catalog does not hold - a convention
// DON!! nobody catalogued, an event set a year newer - would answer with a
// booster's card of the same character.
//
// The question is asked the way Match asks it, on the name or a part of it,
// with the promo line a storefront hangs a set name off read through.
func editionNamesSet(b *matcher.Backend, edition string) bool {
	if matcher.Normalize(edition) == "" {
		return false
	}
	base := promoLineRe.ReplaceAllString(edition, "")
	if !promoRemainderNamesSet(base) {
		base = edition
	}
	for _, set := range b.Sets {
		if matcher.Contains(set.Name, edition) || matcher.Contains(set.Name, base) {
			return true
		}
	}
	return false
}

// editionTiebreak narrows a tier still holding several printings to the
// ones filed in the set the edition describes, with the exact name match
// preferred over the contained-in one, mirroring how Match itself selects
// editions. PromoWildcard trades the edition's pre-filtering for a wider
// pool so a cross-set variant survives to the tiering; this hands the
// edition back its say once the wording has done its work, so the same
// label printed in several sets - the starter-deck and Premium Booster
// Reprints share one number and one label - resolves instead of aliasing.
// An edition matching none of the tier keeps the whole tier: storefronts
// name the base card's set for printings filed elsewhere, and failing safe
// on genuine ambiguity is the point of the tiering.
func editionTiebreak(b *matcher.Backend, inCard *matcher.InputCard, cards []matcher.Card) []matcher.Card {
	if len(cards) <= 1 {
		return cards
	}
	if inCard.Edition == "" {
		return cards
	}

	// A storefront files a set's event printings under that set's promo
	// line: cardmarket's "Promos: Pillars of Strength" is the set the
	// datastore calls "Pillars of Strength Pre-Release Cards". Neither name
	// contains the other, so the marker has to be read as the selection it
	// is or the whole promo line prices as the base commons.
	// A marker with nothing behind it - a bare "Promos:" bucket - names no
	// set, and every set name contains the empty string: reading it as a
	// selection would hand back the event printing of whatever card it
	// decorated. Only a line naming a set selects one, and the emptiness to
	// refuse is the one Contains sees, so a remainder of pure punctuation
	// ("Promos: -", "Promo: 's") reads as the same empty needle.
	base := promoLineRe.ReplaceAllString(inCard.Edition, "")
	if promoRemainderNamesSet(base) && base != inCard.Edition {
		var event []matcher.Card
		for _, card := range cards {
			set, found := b.Sets[card.SetCode]
			if !found || !isEventSet(card.SetCode) {
				continue
			}
			if matcher.Contains(set.Name, base) {
				event = append(event, card)
			}
		}
		if len(event) > 0 {
			return event
		}
	}

	var equal, contained []matcher.Card
	for _, card := range cards {
		set, found := b.Sets[card.SetCode]
		if !found {
			continue
		}
		if matcher.Equals(set.Name, inCard.Edition) {
			equal = append(equal, card)
		} else if matcher.Contains(set.Name, inCard.Edition) {
			contained = append(contained, card)
		}
	}
	if len(equal) > 0 {
		return equal
	}
	if len(contained) > 0 {
		return contained
	}

	// The event printings a set's cards receive are filed in a set of their
	// own instead of wearing a variant label, so they share a name, a
	// number and an empty label with the card they reprint and nothing but
	// the set name tells them apart. A storefront names an event printing
	// when it stocks one, so an edition matching neither set is still a
	// claim that excludes them - unlike no edition at all, which claims
	// nothing and is left to alias.
	var regular []matcher.Card
	for _, card := range cards {
		if !isEventSet(card.SetCode) {
			regular = append(regular, card)
		}
	}
	if len(regular) > 0 {
		return regular
	}
	return cards
}

// promoLineRe matches the promo-line prefix a storefront hangs a set name
// off to name that set's event printings.
var promoLineRe = regexp.MustCompile(`(?i)^promos?\s*[:-]\s*`)

// shelfLineRe matches the shelf a storefront files a set under, written in
// front of the set's own name ("Reprints - Revision Pack" for the set the
// catalog calls "Revision Pack Cards").
//
// It is the promo line's sibling and is kept apart from it because that one
// carries a second meaning: a wording behind a promo line is naming an
// event's printing, and the tiering reads it that way. A shelf says only
// where the storefront filed the set, so it is dropped rather than kept -
// and dropped only where the name is snapped to a set, which is the one
// place the extra word is what stops the set being found. "Revision Pack"
// leaves nothing of "Revision Pack Cards" unaccounted for, where "Reprints
// - Revision Pack" leaves a word over and is refused for it.
var shelfLineRe = regexp.MustCompile(`(?i)^reprints?\s*[:-]\s*`)

// canonicalEdition returns the name of the one set an edition describes
// better than every other, or "" when the wording picks no clear winner.
//
// The wording has to leave nothing of the set name unaccounted for, or
// share three words with it when it does: two words in common is the
// coincidence a vendor bucket like "One Piece Products" lands on, and
// answering that with a set would price a whole shelf as one card.
//
// The set code the storefront wore in front of the name overrides the
// wording: a family shares its name across volumes ("Premium Booster -The
// Best-" and its Vol. 2), so the code is the only thing that says which
// one the listing is filed in. An event set's code opens with its base
// set's - "OP14 RE" against "OP14" - and that opening field is the whole
// of what a storefront prefix spells, so the code cannot tell the pair
// apart and the wording has to. Only the wording being a name the
// storefront cut short unseats the code, and which words it holds does
// not say that: the markers the event sets append come out of one
// vocabulary, so a wording sharing every word with one of them is as
// likely a storefront hanging a decoration off the base name. A
// truncation loses a name's tail, so the wording that is one spells the
// winner's leading words in the winner's order - "The Azure Sea's Seven
// Release" against "The Azure Sea's Seven Release Event Cards", where
// "Pillars of Strength Cards" reaches past the marker of "Pillars of
// Strength Pre-Release Cards" and is decoration on the coded set instead.
func canonicalEdition(b *matcher.Backend, edition, code string) string {
	want := editionTokens(edition)
	if len(want) == 0 {
		return ""
	}
	// A set already naming itself needs no rewriting, and the set index
	// answers that without scoring a single name. The code still has its
	// say: it names the set outright when it disagrees with the wording.
	wantCode := foldSetCode(code)
	named, found := b.NormalizedSets[matcher.Normalize(edition)]
	if found && (wantCode == "" || foldSetCode(named.Code) == wantCode) {
		return ""
	}

	var best, runner, coded editionScore
	for setCode, set := range b.Sets {
		cur := scoreEdition(want, set.Name)
		cur.event = isEventSet(setCode)
		if cur.shared < 2 || cur.missing > 1 || (cur.missing > 0 && cur.shared < 3) {
			continue
		}
		if wantCode != "" && foldSetCode(setCode) == wantCode {
			coded = cur
		}
		if best.name == "" || cur.beats(best) {
			runner, best = best, cur
		} else if runner.name == "" || cur.beats(runner) {
			runner = cur
		}
	}
	// The coded set answers unless the winner both accounts for more of the
	// wording than it does and spells that wording as its own leading
	// words: only then is the wording a name the storefront cut short.
	if coded.name != "" {
		cut := best.missing < coded.missing && truncates(edition, best.name)
		if !cut {
			return coded.name
		}
	}
	if best.name == "" || !best.beats(runner) {
		return ""
	}
	return best.name
}

// foldSetCode drops the punctuation a storefront spells a set code with:
// cardtrader writes "OP-01" for the datastore's "OP01", coolstuffinc
// "EB01" for its "EB-01". The event sets keep their marker ("OP03 PRE"
// folds to OP03PRE), which no storefront prefix spells.
func foldSetCode(code string) string {
	return strings.Map(func(r rune) rune {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return -1
		}
		return unicode.ToUpper(r)
	}, code)
}

// editionScore counts how much of a set name an edition accounts for: the
// words the two share and the ones the set name carries that the edition
// left out, with the event sets flagged for the tie those two cannot
// break. Nothing else orders two sets - the words a set name spells beyond
// the wording say nothing about which one was meant, and answering a
// family's shared name with its shortest member invents a volume the
// storefront never wrote.
type editionScore struct {
	name            string
	shared, missing int
	event           bool
}

func scoreEdition(want map[string]bool, name string) editionScore {
	have := editionTokens(name)
	cur := editionScore{name: name}
	for token := range want {
		if have[token] {
			cur.shared++
		} else {
			cur.missing++
		}
	}
	return cur
}

func (s editionScore) beats(other editionScore) bool {
	if s.shared != other.shared {
		return s.shared > other.shared
	}
	if s.missing != other.missing {
		return s.missing < other.missing
	}
	// An event set wears its base set's whole name with a marker appended,
	// so a wording spelling the base name alone accounts for both exactly
	// as well and they would tie forever. A wording that spells the marker
	// too wins the event set the counts above, so a tie left here is the
	// marker gone unspelled, and the base set is the one being named.
	return !s.event && other.event
}

// editionFields splits a set name into the words that carry its identity,
// in the order it spells them. Normalize is no help here: it drops the
// spaces the words are counted by.
func editionFields(edition string) []string {
	return strings.FieldsFunc(strings.ToLower(edition), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func editionTokens(edition string) map[string]bool {
	out := map[string]bool{}
	for _, field := range editionFields(edition) {
		out[field] = true
	}
	return out
}

// truncates reports whether the wording is the set name with a tail cut
// off: its words are that name's leading ones, in the order the name
// spells them.
func truncates(edition, name string) bool {
	want := editionFields(edition)
	have := editionFields(name)
	if len(want) == 0 || len(want) > len(have) {
		return false
	}
	for i, field := range want {
		if field != have[i] {
			return false
		}
	}
	return true
}

// isEventSet reports whether a set code files the event printings of
// another set's cards. The datastore spells them as the base code with a
// marker appended - "OP03-PRE" for the pre-release cards, "OP10-RE" for the
// release event ones, "OP05-ANN" for the anniversary tournament ones.
//
// The marker is read off either separator: a set code cannot carry a space,
// since a search query is split on whitespace before a filter sees it, but
// a datastore that has not been rebuilt still spells these with one.
func isEventSet(code string) bool {
	fields := strings.FieldsFunc(code, func(r rune) bool {
		return r == ' ' || r == '-'
	})
	if len(fields) < 2 {
		return false
	}
	switch fields[len(fields)-1] {
	case "PRE", "RE", "ANN":
		return true
	}
	return false
}

// tierByVariant splits the candidates into the ones whose variant label the
// input's wording describes, the base printings, and the variant printings.
func tierByVariant(b *matcher.Backend, inCard *matcher.InputCard, candidates []matcher.Card) (described, base, variants []matcher.Card) {
	wording := strings.ToLower(inCard.Variation + " " + inCard.Edition)
	for _, card := range candidates {
		if promoLabel(b, card) == "" {
			base = append(base, card)
			continue
		}
		variants = append(variants, card)
	}
	wording = markWording(wording, variants)
	described = matcher.DescribedVariants(wording, variants)
	// A storefront that named no label at all may have named one in its own
	// words. Asking again in the catalog's is a fallback and never more: a
	// wording that already named a label has said which printing it means,
	// and the one number filing a manga printing beside a super alternate
	// art one is told apart by exactly that.
	if len(described) == 0 {
		aliased := catalogWording(wording)
		if aliased != wording {
			described = matcher.DescribedVariants(aliased, variants)
		}
	}
	// The edition is in the wording because a storefront files a promo
	// under its base set and spells the event in the shelf name, but it
	// describes the shelf and the variation describes the card. Where the
	// shelf's own words name a label - Cool Stuff Inc buckets a Wanted
	// Poster printing under "One Piece: SP", which names the sp label worn
	// by another printing of the same number - the two tie and the listing
	// aliases away. Let the variation answer alone when it names one
	// printing and nothing else does.
	if len(described) > 1 {
		alone := matcher.DescribedVariants(markWording(strings.ToLower(inCard.Variation), variants), variants)
		if len(alone) == 1 {
			described = alone
		}
	}
	return
}

// markWording adds the mark of every candidate whose words the wording says,
// spelled the way the mark's tag is, and returns it unchanged when it says
// none whole.
//
// A mark of several words is one tag, and the tag is a run of whole words
// to a wording - which the catalog's own wording cannot spell where it wrote
// the words apart: "CS 25-26 Finalist Card Set 2" is marked "25-26 2", the
// season at one end of the label and the set's number at the other. A
// wording that says every word of the mark has named it, whatever it wrote
// between them, and the copy the mark tells from the others at this number
// answers a wording that could not have been more specific.
func markWording(wording string, candidates []matcher.Card) string {
	for _, card := range candidates {
		if markSaid(wording, card.Watermark) {
			wording += " " + strings.ToLower(card.Watermark)
		}
	}
	return wording
}

// markSaid reports whether a wording says every word of a mark of several
// words, in any order. A mark of one word is a run of one whole word, which
// SlugDescribes already reads.
func markSaid(wording, mark string) bool {
	words := labelWords(mark)
	if len(words) < 2 {
		return false
	}
	said := labelWords(wording)
	for _, word := range words {
		if !slices.Contains(said, word) {
			return false
		}
	}
	return true
}

// errataWord is what both sides call the printings a game corrected: an early
// run whose text was wrong, which the catalog files under the base card's
// number and the storefronts sell as a printing of its own.
const errataWord = "errata"

// errataRuns are the two corrected runs a card can have been printed in,
// each in the two spellings a wording carries it in: cardtrader letters the
// collector number with the Greek letter and writes the word out in the
// version text beside it. The order is fixed so a wording somehow naming
// both still reads the same way twice.
var errataRuns = []struct{ letter, run string }{
	{"α", "alpha"},
	{"β", "beta"},
}

// errataRun reads which corrected run a text names, "" for one naming
// neither. Only the corrected printings are labelled with these words, so
// reading them off a wording asks nothing of any other printing.
func errataRun(text string) string {
	lower := strings.ToLower(text)
	for _, gen := range errataRuns {
		if strings.Contains(lower, gen.letter) || matcher.SlugDescribes(lower, gen.run) {
			return gen.run
		}
	}
	return ""
}

// errataArt reads the art cardtrader letters a corrected printing's number
// with: an "a" among the letters after the digits is the alternate art
// ("OP01-070ae", "OP01-016βa"), and letters without one are the plain
// printing ("OP01-070e"). found is false for a number carrying no Latin
// letter, which says nothing about the art.
func errataArt(number string) (alternate, found bool) {
	_, tail := splitNumber(number)
	for _, r := range strings.TrimLeft(tail, "0123456789") {
		if r > unicode.MaxASCII || !unicode.IsLetter(r) {
			continue
		}
		found = true
		if unicode.ToLower(r) == 'a' {
			alternate = true
		}
	}
	return alternate, found
}

// errataSkipped are the words every corrected printing's label shares - the
// correction itself and the run it names - which say nothing about which of
// them a listing means.
var errataSkipped = map[string]bool{
	"pre": true, "errata": true, "preerrata": true, "alpha": true, "beta": true,
}

// errataTreatment is what a corrected printing's label says beyond the
// correction and the run: the treatment that printing wears. The catalog
// spells it several ways - "Parallel", "Box Topper", "Demo Deck" - so the
// words are read off the label rather than looked for, and a label saying
// nothing else is the plain corrected printing.
func errataTreatment(label string) string {
	var kept []string
	for _, field := range strings.Fields(label) {
		if errataSkipped[matcher.PromoTypeSlug(field)] {
			continue
		}
		kept = append(kept, field)
	}
	return strings.Join(kept, " ")
}

// errataNarrow keeps the corrected printings a wording naming one can mean,
// and answers with the single printing where the two axes the shelf tells
// them apart by leave one.
//
// The axes are the run and the treatment, and cardtrader letters both into
// the collector number: the Greek letter names the run, and an "a" marks the
// alternate art (OP01-070ae, OP01-025a, OP01-016βa) where a number lettered
// without one is plain (OP01-070e). The letters are read before the version
// text, which says "Alternate Art" over OP01-070e's plain art and nothing
// over OP01-070ae's or OP01-025a's alternate one. A number with no letter
// leaves the text to say it, and OP01-003's alternate art has neither, so
// cardtrader resolves that one by its Cardmarket id. The treatment is
// compared as the catalog's own words, never built out of them, because the
// catalog files one number's alternate art as a Box Topper and another's as
// a Demo Deck.
//
// Nothing here fires unless the wording says the correction and some
// printing of the number wears it, and the second half of that is what
// holds it. The storefront calls the corrected run "Pre-Errata Card" on one
// card and "Alpha Errata Card" on another, so the bare word has to be what
// is read - and it also writes "1st Print Errata Card" on twelve cards the
// catalog files no corrected printing of, which is the same printing said
// another way. Asking the catalog rather than the wording is what leaves
// those alone: a number holding no corrected printing has nothing here to
// answer with, whatever the listing called itself.
func errataNarrow(b *matcher.Backend, inCard *matcher.InputCard, candidates []matcher.Card) []matcher.Card {
	wording := strings.ToLower(inCard.Variation + " " + inCard.Edition)
	if !strings.Contains(wording, errataWord) {
		return candidates
	}
	var errata []matcher.Card
	for _, card := range candidates {
		if strings.Contains(strings.ToLower(promoLabel(b, card)), errataWord) {
			errata = append(errata, card)
		}
	}
	if len(errata) == 0 {
		return candidates
	}

	// The run the listing names is the one the printing has to have been
	// in, and a listing naming none is asking for the printing that names
	// none either.
	run := errataRun(wording)
	var sameRun []matcher.Card
	for _, card := range errata {
		if errataRun(promoLabel(b, card)) == run {
			sameRun = append(sameRun, card)
		}
	}
	if len(sameRun) == 0 {
		return errata
	}
	if len(sameRun) == 1 {
		return sameRun
	}

	// A wording spelling a treatment out has named the printing outright,
	// which answers before anything inferred from the art word does.
	var named []matcher.Card
	for _, card := range sameRun {
		treatment := errataTreatment(promoLabel(b, card))
		if treatment == "" {
			continue
		}
		if matcher.SlugDescribes(wording, matcher.PromoTypeSlug(treatment)) {
			named = append(named, card)
		}
	}
	if len(named) == 1 {
		return named
	}

	// Otherwise the listing has only said whether it is the base art or not,
	// and one printing of the run wears a treatment while the other does
	// not, so that is enough to tell them apart. The number's letters say it
	// before the words do.
	treated := wantsUnnamedVariant(inCard)
	if alternate, found := errataArt(extractNumber(inCard.Variation)); found {
		treated = alternate
	}
	var picked []matcher.Card
	for _, card := range sameRun {
		if (errataTreatment(promoLabel(b, card)) != "") == treated {
			picked = append(picked, card)
		}
	}
	if len(picked) == 1 {
		return picked
	}
	return sameRun
}

// promoPlaces are the finishing places a tournament printing is awarded for,
// which the catalog writes as the last word of the label.
var promoPlaces = []string{"winner", "finalist", "participant"}

// placeSpellings are the storefronts' other words for a place, keyed by
// their slug. The catalog tags the printing handed to everyone who played
// as the participant's and names the product it came in a "Participation
// Pack", and the storefronts write the second word for the first.
var placeSpellings = map[string]string{
	"participation": "participant",
}

// placeWord reads the finishing place a word names, "" for a word naming
// none. The word has to be a word: "winner" inside "winners" is not this
// storefront naming a place.
func placeWord(word string) string {
	slug := matcher.PromoTypeSlug(word)
	if slices.Contains(promoPlaces, slug) {
		return slug
	}
	return placeSpellings[slug]
}

// placeMentioned reports whether a wording carries any place word, in the
// catalog's spelling or a storefront's. Only presence is asked: "winners"
// mentions a place as surely as "winner" does.
func placeMentioned(wording string) bool {
	for _, place := range promoPlaces {
		if strings.Contains(wording, place) {
			return true
		}
	}
	for spelling := range placeSpellings {
		if strings.Contains(wording, spelling) {
			return true
		}
	}
	return false
}

// placeAwarded reads the finishing place a label is awarded for, "" for a
// label naming none. Only the last word is read: the catalog also sells a
// "Winner Pack" and a "Finalist Card Set", products named after a place
// rather than awarded for one, and those carry their own noun behind it.
func placeAwarded(label string) string {
	fields := strings.Fields(label)
	if len(fields) == 0 {
		return ""
	}
	last := matcher.PromoTypeSlug(fields[len(fields)-1])
	if slices.Contains(promoPlaces, last) {
		return last
	}
	return ""
}

// placeNarrow drops the printings awarded for a finishing place the wording
// never mentions.
//
// The catalog files the places of one event as labels sharing a stem - "CS
// 2023 Top Players Pack", and the same with "Winner" or "Finalist" behind it
// - and the stem is no help in telling them apart, because this storefront
// writes that event as "Championship 2023". So a listing of the plain
// printing describes no label at all and every place ties with it.
//
// Only the absence of the word is read, never its presence. Where the word
// is there, what it means is genuinely unclear - "Beginners Deck Party |
// Winner Pack" is the printing awarded for winning, while "Winner Pack Vol.
// 6" is a product named after one - and every rule tried on that wording
// moved listings onto the wrong side. Where the word is absent the listing
// has said nothing about a place, so the printings awarded for one are the
// answers it cannot have, and refusing them is the half that matters: the
// places are the scarce printings, and answering a plain listing with one
// prices a common card as a rare.
func placeNarrow(b *matcher.Backend, inCard *matcher.InputCard, candidates []matcher.Card) []matcher.Card {
	wording := strings.ToLower(inCard.Variation + " " + inCard.Edition)
	if placeMentioned(wording) {
		return candidates
	}
	var kept []matcher.Card
	for _, card := range candidates {
		if placeAwarded(promoLabel(b, card)) == "" {
			kept = append(kept, card)
		}
	}
	if len(kept) == 0 {
		return candidates
	}
	return kept
}

// packPairNamed answers the printing a wording asked for where an event's
// two cards carry two product names and the storefront writes only one.
//
// An event hands out a card for playing and a card for winning, and the
// catalog names them "Tournament Pack Vol. 5" and "Winner Pack Vol. 5" -
// except for two volumes of nine, where the winner's is "Tournament Pack
// Vol. 2 Winner" instead. Cool Stuff Inc writes one wording for all of them,
// "Tournament Pack Vol. 5 - Winner". Where the catalog spelled that volume
// the second way the wording names the label outright and this does nothing.
// Where it spelled it the first way the wording describes the playing card,
// the word saying otherwise is left over, and a $56 winner's card was
// answered with the $2 one handed to everybody who turned up.
//
// The word is not read as a place, which placeNarrow says cannot be done:
// "Winner Pack Vol. 6" is a product named after a place and "Beginners Deck
// Party | Winner Pack" is a printing awarded for one, and no rule on the
// word alone told those apart. This asks a different question - whether the
// catalog holds a pack of the same event named for the place the wording
// names - and answers only then. The event has to match to the word, so
// "Winner Pack Vol. 5" is reachable from "Tournament Pack Vol. 5" and from
// no other wording.
func packPairNamed(b *matcher.Backend, wording string, described, variants []matcher.Card) []matcher.Card {
	if len(described) != 1 {
		return described
	}
	label := promoLabel(b, described[0])
	event, isPlaying := strings.CutPrefix(label, "Tournament Pack ")
	if !isPlaying {
		return described
	}
	place := placeAsked(wording)
	// A label already naming the place has answered for itself, which is
	// how the two volumes the catalog spells the other way keep working.
	if place == "" || strings.Contains(strings.ToLower(label), place) {
		return described
	}
	want := strings.ToUpper(place[:1]) + place[1:] + " Pack " + event
	for _, card := range variants {
		if strings.EqualFold(promoLabel(b, card), want) {
			return []matcher.Card{card}
		}
	}
	return described
}

// placeStem is a label with its finishing place taken off, which is the
// event that awarded it.
func placeStem(label string) string {
	if placeAwarded(label) == "" {
		return label
	}
	fields := strings.Fields(label)
	return strings.Join(fields[:len(fields)-1], " ")
}

// placeAsked reads the one finishing place a wording names, "" for a wording
// naming none or more than one.
func placeAsked(wording string) string {
	said := ""
	for field := range strings.FieldsSeq(wording) {
		slug := placeWord(field)
		if slug == "" {
			continue
		}
		if said != "" && said != slug {
			return ""
		}
		said = slug
	}
	return said
}

// placeChosen answers a wording that names a place with the printing awarded
// for it, where the catalog files that place beside the same event's others.
//
// The word on its own says too little to act on. The catalog sells a "Winner
// Pack" and a "Finalist Card Set", products named after a place rather than
// awarded for one, and a wording carrying either says "winner" as surely as
// a wording naming the place does - reading the word wherever it falls moved
// listings onto the wrong side of that, in both directions.
//
// What makes it safe is asking only where the catalog has already said the
// place is what tells these apart: a family of labels sharing one event and
// differing in nothing but the place behind it. There the place is the only
// question the wording can be answering, whatever else its words are doing,
// and one member of the family carrying the place the wording named is an
// answer no other member has a claim on. Where no such family stands at the
// number, or where more than one member answers, this says nothing.
func placeChosen(b *matcher.Backend, inCard *matcher.InputCard, candidates []matcher.Card) []matcher.Card {
	wording := strings.ToLower(inCard.Variation + " " + inCard.Edition)
	asked := placeAsked(wording)
	if asked == "" || placeLabelled(b, wording, asked, candidates) {
		return nil
	}
	families := map[string][]matcher.Card{}
	for _, card := range candidates {
		label := promoLabel(b, card)
		if label == "" {
			continue
		}
		stem := placeStem(label)
		families[stem] = append(families[stem], card)
	}
	var kept []matcher.Card
	for _, family := range families {
		if len(family) < 2 {
			continue
		}
		for _, card := range family {
			if placeAwarded(promoLabel(b, card)) == asked {
				kept = append(kept, card)
			}
		}
	}
	if len(kept) == 0 {
		return nil
	}
	// Two events can share a place at one number; break the tie by whichever
	// family the wording's other words name more of, unique winner only.
	if len(kept) > 1 {
		best := kept[0]
		bestNamed := tagsNamed(wording, best)
		unique := true
		for _, card := range kept[1:] {
			named := tagsNamed(wording, card)
			if named > bestNamed {
				best, bestNamed, unique = card, named, true
			} else if named == bestNamed {
				unique = false
			}
		}
		if !unique {
			return nil
		}
		kept = []matcher.Card{best}
	}
	// The place is the only question the wording can be answering where
	// its other words name nothing better. "CS 2024 Participation" names
	// the championship's own participant printing by two of its tags, and
	// the regional family standing at the same number, whose participant
	// the place alone would pick, by one; the wording has said more than
	// the place, and the tiering reads what it said.
	named := tagsNamed(wording, kept[0])
	for _, card := range candidates {
		if tagsNamed(wording, card) > named {
			return nil
		}
	}
	return kept
}

// tagsNamed counts the tags of a printing a wording names.
func tagsNamed(wording string, card matcher.Card) int {
	named := 0
	for _, promoType := range card.PromoTypes {
		if matcher.SlugDescribes(wording, promoType) {
			named++
		}
	}
	return named
}

// placeLabelled reports whether the place a wording names is a word of a
// label the wording spells out whole, worn by one of the candidates.
//
// The catalog names products after a place as well as awarding printings
// for one, and a wording spelling "Finalist Card Set" has said the word
// "finalist" without asking for the finalist's printing of the event pack
// filed beside it. The whole label is the storefront saying which product
// it means, and the place inside it is that product's name rather than a
// question of its own; the tiering reads the label, and the place is left
// to the wordings that say it on its own.
//
// The tag naming that label is not enough by itself: a Store Championship's
// own "Participation Pack" tag also names the word an unrelated family's
// participant printing is asked for by. See otherTagsDescribed.
func placeLabelled(b *matcher.Backend, wording, place string, candidates []matcher.Card) bool {
	for _, card := range candidates {
		for _, promoType := range card.PromoTypes {
			if slices.Contains(promoPlaces, promoType) || !matcher.SlugDescribes(wording, promoType) {
				continue
			}
			if !otherTagsDescribed(wording, card, promoType) {
				continue
			}
			for word := range strings.FieldsSeq(b.PromoTypeLabel(promoType)) {
				if placeWord(word) == place {
					return true
				}
			}
		}
	}
	return false
}

// otherTagsDescribed reports whether a wording describes every tag of a
// card besides the one already being checked and the place tags, which
// carry no words of their own to spell out. The wording is read with the
// card's own mark joined back up first, the same way tierByVariant reads
// it: a label split across two ends, "CS 25-26 Finalist Card Set 2", is one
// tag's slug, "25262", which nothing but the mark rejoins.
func otherTagsDescribed(wording string, card matcher.Card, skip string) bool {
	wording = markWording(wording, []matcher.Card{card})
	for _, promoType := range card.PromoTypes {
		if promoType == skip || slices.Contains(promoPlaces, promoType) {
			continue
		}
		if !matcher.SlugDescribes(wording, promoType) {
			return false
		}
	}
	return true
}

// mangaWord is what every storefront calls the printings drawn as manga
// panels. The catalog spells the same treatment "Super Alternate Art" on the
// sets that print it alone, and writes the word out on the sets that cross it
// with something else.
const mangaWord = "manga"

// mangaLabelWords are a label's words with the catalog's spelling of the
// manga art said the way the storefronts say it, so the two can be compared
// word for word.
//
// The substitution is made on the label, never on the wording. Adding
// "alternate" and "art" to what a listing said would let them answer for a
// plain Alternate Art printing standing beside the manga one, which is a
// different card at the same number - EB02-061 and OP12-118 both hold the
// pair, and both move to the wrong half of it that way.
func mangaLabelWords(b *matcher.Backend, card matcher.Card) []string {
	label := promoLabel(b, card)
	if label == "" {
		return nil
	}
	return labelWords(superAlternateArt.Replace(label))
}

var superAlternateArt = strings.NewReplacer("Super Alternate Art", "Manga")

// mangaChosen answers a wording naming the manga art with the printing that
// carries it, where the catalog crosses that art with something else.
//
// The catalog files the crossed printings under one label - "Red Super
// Alternate Art", "Parallel Manga Alternate Art" - while this storefront
// names the parts separately and in its own order, "(Alternate Art) (Manga)
// (Red Parallel)". No wording of one describes a label of the other, so the
// tiering answers with the plainest label the wording happens to contain,
// which is "Parallel": the base parallel, standing beside a manga printing
// worth two orders of magnitude more.
//
// Only labels carrying the manga art are considered, and only those whose
// every word the listing said. That second half is what keeps the plain
// Alternate Art printing out of it, and the first is what keeps a wording
// naming the art from being answered by a label that does not have it.
// Where the longest such label is not unique the wording has not chosen,
// and neither does this.
func mangaChosen(b *matcher.Backend, inCard *matcher.InputCard, candidates []matcher.Card) []matcher.Card {
	wording := strings.ToLower(inCard.Variation + " " + inCard.Edition)
	if !matcher.SlugDescribes(wording, mangaWord) {
		return nil
	}
	said := map[string]bool{}
	for _, word := range labelWords(wording) {
		said[word] = true
	}
	var pool []matcher.Card
	for _, card := range candidates {
		words := mangaLabelWords(b, card)
		if len(words) == 0 || !slices.Contains(words, mangaWord) {
			continue
		}
		whole := true
		for _, word := range words {
			if !said[word] {
				whole = false
				break
			}
		}
		if whole {
			pool = append(pool, card)
		}
	}
	if len(pool) == 0 {
		return nil
	}
	longest := 0
	for _, card := range pool {
		if n := len(mangaLabelWords(b, card)); n > longest {
			longest = n
		}
	}
	var best []matcher.Card
	for _, card := range pool {
		if len(mangaLabelWords(b, card)) == longest {
			best = append(best, card)
		}
	}
	if len(best) != 1 {
		return nil
	}
	return best
}

// premiumBoosterPrefix is the words every Premium Booster set's name opens
// with; what follows is that volume's own marker.
const premiumBoosterPrefix = "Premium Booster "

// singlePremiumBoosterSet answers the one Premium Booster set an edition
// names by its own marker, and false where it names none or more than one.
//
// The marker, not the whole set name, is what an unresolved edition spells
// in full: Cardmarket writes "The Best" for Vol. 1's shelf and "The Best
// Vol.2" for Vol. 2's, and "The Best" alone is contained in both names
// equally, so a whole-name or Contains match cannot tell them apart. Both
// forms are matched because AdjustEdition already resolves an unambiguous
// marker, like Vol. 2's, to the whole name before this runs.
func singlePremiumBoosterSet(b *matcher.Backend, edition string) (*matcher.Set, bool) {
	if edition == "" {
		return nil, false
	}
	var found *matcher.Set
	for _, set := range b.Sets {
		marker, isPremiumBooster := strings.CutPrefix(set.Name, premiumBoosterPrefix)
		if !isPremiumBooster {
			continue
		}
		if !matcher.Equals(marker, edition) && !matcher.Equals(set.Name, edition) {
			continue
		}
		if found != nil {
			return nil, false
		}
		found = set
	}
	return found, found != nil
}

// variationSetCodeRe matches the set code a storefront writes inside the
// variation, ahead of the treatment it names.
var variationSetCodeRe = regexp.MustCompile(`^(?:OP|EB|ST|PRB|LT)[0-9]{2}$`)

// starterDeckRunRe reads "Starter Deck NN" as the "STnn" code it names.
var starterDeckRunRe = regexp.MustCompile(`(?i)starter\s+deck\s+([0-9]{1,2})\b`)

// learnTogetherRe reads TCGplayer's own name for LT-01 as the "LT01" code
// no storefront actually prints on the card.
var learnTogetherRe = regexp.MustCompile(`(?i)learn\s+together\s+deck\s+set`)

// codedSetNamed returns the set a variation names by its code.
//
// A code naming one printing of this card's number has said which card the
// listing is outright, and the edition is worth overruling. A Premium
// Booster code is worth overruling even where it names several: PRB-01
// always holds an Alternate Art beside a plain Reprint at one number, and
// demanding uniqueness there deleted both rows before the tiering below
// could read "PRB01 Alternate Art" and choose between them. Every other
// code still demands uniqueness.
//
// The code is matched as a prefix of the set's own, because a set's code
// carries what the storefront leaves off: "OP15" is written for the cards
// the catalog files in OP15-EB04.
func codedSetNamed(b *matcher.Backend, inCard *matcher.InputCard, edition string) string {
	number := inputNumber(b, inCard)
	if number == "" || inCard.Variation == "" {
		return ""
	}
	variation := starterDeckRunRe.ReplaceAllStringFunc(inCard.Variation, func(run string) string {
		digits := starterDeckRunRe.FindStringSubmatch(run)[1]
		if len(digits) == 1 {
			digits = "0" + digits
		}
		return "ST" + digits
	})
	variation = learnTogetherRe.ReplaceAllString(variation, "LT01")
	var name string
	for field := range strings.FieldsSeq(variation) {
		code := strings.ToUpper(field)
		if !variationSetCodeRe.MatchString(code) {
			continue
		}
		var found []matcher.Card
		for _, uuid := range b.Hashes[matcher.Normalize(inCard.Name)] {
			co, ok := b.UUIDs[uuid]
			if !ok || co.Sealed || !numberMatches(number, co.Number) {
				continue
			}
			if !strings.HasPrefix(foldSetCode(co.SetCode), foldSetCode(code)) {
				continue
			}
			if len(found) > 0 && found[0].SetCode != co.SetCode {
				return ""
			}
			found = append(found, co.Card)
		}
		if len(found) == 0 {
			continue
		}
		set, ok := b.Sets[found[0].SetCode]
		if !ok || set.Name == edition {
			continue
		}
		// Two printings in the named set leave the wording to choose, which
		// is safe only for the family that always holds several: every
		// other coded set still demands the code alone settle it.
		seen := map[string]bool{}
		for _, card := range found {
			seen[card.UUID] = true
		}
		if len(seen) != 1 && !strings.HasPrefix(code, "PRB") {
			return ""
		}
		if name != "" && name != set.Name {
			return ""
		}
		name = set.Name
	}
	return name
}

// fullNumberInEdition spells a bare collector number the way the edition's
// own set writes it, "" where the wording already wrote it in full, the
// edition names no set, or that set holds this card at no single number.
func fullNumberInEdition(b *matcher.Backend, inCard *matcher.InputCard, number string) string {
	if number == "" {
		return ""
	}
	if set, _ := splitNumber(number); set != "" {
		return ""
	}
	edition, found := b.NormalizedSets[matcher.Normalize(inCard.Edition)]
	if !found {
		return ""
	}
	full := ""
	for _, uuid := range b.Hashes[matcher.Normalize(inCard.Name)] {
		co, ok := b.UUIDs[uuid]
		if !ok || co.Sealed || co.SetCode != edition.Code || !numberMatches(number, co.Number) {
			continue
		}
		if full != "" && full != co.Number {
			return ""
		}
		full = co.Number
	}
	return full
}

// runNamedVariants keeps the printings whose set the variation names and
// whose label it spells a run of. It is the tiering's whole-label test
// loosened the way promoSetBegun loosens its own, and it answers only where
// it names one printing: two printings sharing a run are told apart by what
// the wording did not say.
//
// Two things hold the run to the printings it can speak for, because the run
// alone says too little. The number has to be a whole one: a bare tail is
// the same number in every set of the game, so it cannot say which printing
// a label belongs to. And the printing has to be one a storefront names the
// base card's set for - a card handed out at an event, which is filed in the
// promo set and sold under the set it was printed in - or else one whose set
// the wording names outright. A card reprinted in an ordinary set wears a
// label there that a storefront selling the original may write as a note,
// "Sanji (Best Selection)" against a Premium Booster reprint, and reading
// that as the reprint prices one card as another. Cool Stuff Inc's "(OP05
// 1st Anniversary Gold-Stamped Signature)" on a starter deck card says the
// set out loud, and its "(World Tour 23-24)" names an event printing.
func runNamedVariants(b *matcher.Backend, inCard *matcher.InputCard, number string, cards []matcher.Card) []matcher.Card {
	if !fullNumberRe.MatchString(number) {
		return nil
	}
	wording := strings.ToLower(inCard.Variation)
	var out []matcher.Card
	for _, card := range cards {
		if !slugsRunOf(wording, labelTokens(card)) {
			continue
		}
		set, found := b.Sets[card.SetCode]
		if found && setIsPromotional(set) {
			out = append(out, card)
			continue
		}
		for field := range strings.FieldsSeq(wording) {
			if strings.EqualFold(strings.Trim(field, ":-"), card.SetCode) {
				out = append(out, card)
				break
			}
		}
	}
	return out
}

// setVocabulary spells a treatment the way one set files it, for the sets
// whose word for it differs from the game's own.
//
// Premium Booster -The Best- Vol. 2 reprints cards in manga art and files
// every one of them as an alternate art: the catalog carries 46 of those and
// no manga at all, where Vol. 1 carries both as separate things. The art is
// manga - line work, screentone and a panel's speech bubble - and every
// storefront sells it by that name, so the two words are naming one printing
// and only in this set.
var setVocabulary = map[string]map[string]string{
	"PRB-02": {"manga": "Alternate Art"},
}

// foldedPrintings names, for a set whose cards TCGplayer sells under
// another set's product, the set that product is really filed in, keyed by
// set code since GetSet is by code. TCGplayer's own description on the
// Premium Booster Vol. 2 products (656132, 656164) says each was
// "originally reprinted from the Learn Together Deck Set".
var foldedPrintings = map[string]map[string]string{
	"LT-01": {"OP06-118": "PRB-02", "OP10-109": "PRB-02"},
}

// setVocabularyNames reports whether a set's own word for a treatment is what
// the wording says, and this printing wears the label that word names.
func setVocabularyNames(wording, setCode string, promoTypes []string) bool {
	spellings, found := setVocabulary[setCode]
	if !found {
		return false
	}
	for storefront, catalog := range spellings {
		if !matcher.SlugDescribes(wording, matcher.PromoTypeSlug(storefront)) {
			continue
		}
		if slices.Contains(promoTypes, matcher.PromoTypeSlug(catalog)) {
			return true
		}
	}
	return false
}

// catalogVocabulary spells a treatment the way the catalog files it, for the
// words the storefronts use instead. Both sides are the names of one thing:
// the catalog calls the manga-art printings "Super Alternate Art" where every
// storefront calls them manga, and files the treasure rares under the two
// letters their rarity is printed as.
var catalogVocabulary = map[string]string{
	"manga":         "Super Alternate Art",
	"treasure rare": "TR",
	"textured art":  "Textured Foil",
}

// catalogWording adds the catalog's spelling for every storefront word the
// wording says, and returns it unchanged when it says none. The storefront's
// own words stay: a wording naming two treatments still names both.
func catalogWording(wording string) string {
	for storefront, catalog := range catalogVocabulary {
		if matcher.SlugDescribes(wording, matcher.PromoTypeSlug(storefront)) {
			wording += " " + catalog
		}
	}
	return wording
}

// unnumberedTiebreak keeps the one printing numbered by a word where the
// wording wrote no number and the printings beside it wear one.
//
// The 3rd Anniversary Tournament pack holds a leader card the catalog files
// under "LEADER" and a card filed under ST01-012, both wearing the pack's
// label and nothing else, and the catalog names the second by its number and
// the first by nothing: the number is what tells the one from the other,
// and a listing that wrote none is the card there was no number to write.
// A wording carrying a number has already chosen and is left alone.
func unnumberedTiebreak(number string, cards []matcher.Card) []matcher.Card {
	if number != "" || len(cards) < 2 {
		return cards
	}
	var worded []matcher.Card
	for _, card := range cards {
		if !strings.ContainsFunc(card.Number, unicode.IsDigit) {
			worded = append(worded, card)
		}
	}
	if len(worded) != 1 {
		return cards
	}
	return worded
}

// lastNamedTiebreak keeps the printings whose label the wording names last.
//
// A storefront writes the treatment it means behind the category it belongs
// to: Cool Stuff Inc sells "St. Marcus Mars (Alternate Art)" for $12 and
// "St. Marcus Mars (Alternate Art) (Red Parallel)" for $345, and the catalog
// files those as a printing wearing alternateart and one wearing parallel.
// A wording naming both names one tag on each with none left over, which is
// the tie DescribedVariants leaves to its caller, and the words in front are
// the ones both listings share. What the second listing added is what tells
// it from the first.
func lastNamedTiebreak(b *matcher.Backend, wording string, cards []matcher.Card) []matcher.Card {
	if len(cards) < 2 {
		return cards
	}
	var out []matcher.Card
	best := -1
	for _, card := range cards {
		at := -1
		for _, promoType := range labelTokens(card) {
			said := slugSaidAt(wording, promoType)
			if said > at {
				at = said
			}
		}
		if at > best {
			best, out = at, nil
		}
		if at == best {
			out = append(out, card)
		}
	}
	return out
}

// slugSaidAt answers the word a wording last begins spelling a slug at, and
// -1 when it never does. It is SlugDescribes keeping the position it found.
func slugSaidAt(wording, slug string) int {
	at := -1
	for first := range slugRuns(strings.Fields(strings.ToLower(wording)), slug) {
		at = max(at, first)
	}
	return at
}

// doublePackRe matches the product line a Double Pack Set's two DON!!
// cards are filed under, in the storefront's spelling or the catalog's
// ("Double Pack Set Vol. 4", "Double Pack Volume 2", "Double Pack Vol 7").
var doublePackRe = regexp.MustCompile(`(?i)\bdouble\s*pack\b`)

// labelOverlap picks, among printings told apart by nothing but a label,
// the one whose label the storefront's wording names.
//
// The DON!! cards are the game's only such printings, and their labels are
// prose the catalog wrote rather than tags: "Trafalgar Law, Eustass Kid and
// Monkey.D.Luffy Double Pack Set Vol. 4" beside a plain "Alternate Art" in
// the same set. A storefront never spells one of those in full - it writes
// "(Law, Kid, Luffy) (Double Pack Set Vol. 4)" - so asking whether the
// wording names a whole label answers "Alternate Art" for every one of them,
// because every such listing also says the words "alternate art". Counting
// the label's own words the wording says asks the question the other way
// round and lets the fuller label win.
//
// The wording is not all identification, though. Beside the decorations a
// storefront hangs off the name it publishes a sentence describing the art -
// "Gold Hook", "Big Eye", "Sand Tornado" - and by the time a filter runs the
// two have been folded into one variation with nothing to tell them apart.
// Every guard below is there because that prose reached a label it had no
// business naming.
//
// A label word counts as said only when the wording spells it as a run of
// whole words, which is what lets a catalog's "GEAR5 Luffy" hear a
// storefront's "Gear 5 Luffy" and refuse the "Gear 4 Luffy" filed beside it.
// The winner must say more of its label than it leaves out, or an art
// sentence saying "Big Eye" would answer with the set's "Big Mom". And a
// winner that says no more words than the runner-up, only fewer wrong ones,
// has to say a word no other label carries: without it a wording naming a
// Double Pack volume and no character - "Sand Tornado", "Purple Smoke" -
// answers with whichever of the volume's two cards has the shorter name,
// and both of the volume's listings price as the same card.
//
// Only the variation is read. The edition is the same string for every
// candidate here - the set is what narrowed to them - so it can say nothing
// about which of their labels was meant, while the words of the set's own
// name would count as label evidence: "OP15 - Adventure On Kami's Island"
// hands the word "Island" to a "Sky Island Map" label no listing named.
func labelOverlap(b *matcher.Backend, inCard *matcher.InputCard, candidates []matcher.Card, mustName bool) []matcher.Card {
	wording := inCard.Variation

	// A Double Pack Set's DON!! cards are filed under a label naming the
	// set, so a listing naming a volume is naming one of them and nothing
	// else in the set answers. Where the catalog holds no such card - Vol.
	// 1 is missing from it - the listing has no printing to price and must
	// not settle for the set's plain alternate art.
	if doublePackRe.MatchString(wording) {
		var packs []matcher.Card
		for _, card := range candidates {
			if doublePackRe.MatchString(promoLabel(b, card)) {
				packs = append(packs, card)
			}
		}
		if len(packs) == 0 {
			return nil
		}
		candidates = packs
	}

	groups, order := groupByIdentity(b, candidates)
	// One identity is one card printed plain and gold, and nothing has to
	// name it: the set was what narrowed to it, and the treatment is all
	// that is left to decide. A caller that demands the wording name
	// something is asking about the label rather than the treatment, so it
	// goes on to the scoring below, where a wording naming nothing settles
	// nothing.
	if len(order) == 1 && !mustName {
		return groups[order[0]].pick(inCard, treatmentSaid(inCard.Variation))
	}

	// A word two labels share says nothing about which of them was meant.
	carried := map[string]int{}
	for _, key := range order {
		for _, word := range uniqueWords(groups[key].words) {
			carried[word]++
		}
	}

	var best, runner *identityGroup
	for _, key := range order {
		group := groups[key]
		for _, word := range group.words {
			if matcher.SlugDescribes(wording, word) {
				group.named++
				if carried[word] == 1 {
					group.only = true
				}
			}
		}
		if best == nil || group.beats(best) {
			runner, best = best, group
		} else if runner == nil || group.beats(runner) {
			runner = group
		}
	}
	if best == nil || best.named <= len(best.words)-best.named {
		return candidates
	}
	if runner != nil && (!best.beats(runner) || (best.named == runner.named && !best.only)) {
		return candidates
	}
	return best.pick(inCard, treatmentSaid(inCard.Variation))
}

// identityGroup collects the printings of one label, the gold-treated half
// of a pair filed beside the plain one it is a treatment of, along with how
// much of that shared identity a wording said.
type identityGroup struct {
	words []string
	plain []matcher.Card
	gold  []matcher.Card
	named int
	only  bool
}

// beats orders two identities by the words of theirs a wording said, the
// label leaving fewest unsaid winning a tie. That tie-break is what keeps a
// bare wording on the bare printing where the treatment is not what tells
// the two apart: "Nami" says all of "Nami" and half of "Nami Manga".
func (g *identityGroup) beats(other *identityGroup) bool {
	if g.named != other.named {
		return g.named > other.named
	}
	return len(g.words)-g.named < len(other.words)-other.named
}

// pick answers with the treated half of the identity when the storefront
// asked for the treatment and the catalog holds one, and with the plain half
// otherwise. An identity holding only the half that was not asked for hands
// back what it has: the treatment is the storefront's claim, and refusing its
// own printing over it would price nothing at all. Handing back both is how
// this refuses to answer, leaving Match to report the aliasing.
//
// A storefront that indexes its products positionally has already said which
// of an identity's halves it means, and its index outranks any wording: this
// is cardmarket, which does not describe a DON!! beyond naming the character
// and files the plain printing as V.1 with the treated one behind it, the
// same reading the numbered cards get. An index past the pair names a product
// the catalog does not hold as a half of anything, and pricing it as either
// half would be a guess.
func (g *identityGroup) pick(inCard *matcher.InputCard, treated bool) []matcher.Card {
	both := append(append([]matcher.Card{}, g.plain...), g.gold...)
	if len(both) == 1 {
		return both
	}
	switch positionalIndex(inCard.Variation) {
	case 0:
	case 1:
		treated = false
	case 2:
		treated = true
	default:
		return both
	}
	if treated && len(g.gold) > 0 {
		return g.gold
	}
	if !treated && len(g.plain) > 0 {
		return g.plain
	}
	return both
}

// positionalIndex reads the "(V.n)" index cardmarket synthesizes to tell the
// products sharing a name apart, 0 where none was written.
func positionalIndex(variation string) int {
	for field := range strings.FieldsSeq(strings.ToLower(variation)) {
		tail, found := strings.CutPrefix(field, "v.")
		if !found {
			continue
		}
		index, err := strconv.Atoi(tail)
		if err != nil {
			continue
		}
		return index
	}
	return 0
}

// goldTreatment is the word a DON!! label ends in when the printing is the
// gold-bordered half of a pair: a third of the catalog's DON!! labels are
// another label of the same set with this word appended.
const goldTreatment = "gold"

// groupByIdentity buckets the candidates by the label they wear with the
// gold treatment taken off, so that the pair a set prints of one card is
// scored once, on the identity both halves share. Returned with the order
// the identities were first seen, so that nothing depends on map iteration.
func groupByIdentity(b *matcher.Backend, candidates []matcher.Card) (map[string]*identityGroup, []string) {
	groups := map[string]*identityGroup{}
	var order []string
	for _, card := range candidates {
		words := labelWords(promoLabel(b, card))
		// A label that is the treatment word alone is not a treated half of
		// anything: those are the coloured DON!! promos, filed under the name
		// of their colour.
		//
		// Where the word falls is not read. The catalog wrote it last -
		// "Buggy Gold" - and a datastore that publishes the character as a
		// mark says the same printing as "Gold Buggy", the promotion in
		// front of the mark it marks. Either way the treatment is the word
		// and the rest is the identity it treats.
		treated := len(words) > 1 && slices.Contains(words, goldTreatment)
		if treated {
			words = slices.DeleteFunc(words, func(word string) bool {
				return word == goldTreatment
			})
		}
		key := strings.Join(words, " ")
		group, found := groups[key]
		if !found {
			group = &identityGroup{words: words}
			groups[key] = group
			order = append(order, key)
		}
		if treated {
			group.gold = append(group.gold, card)
		} else {
			group.plain = append(group.plain, card)
		}
	}
	return groups, order
}

// treatmentNeighbours are the words a storefront writes beside "gold" when
// it means the gold-bordered printing: the finish, the border, the text the
// border is drawn around, or the version of the card.
var treatmentNeighbours = map[string]bool{
	"foil": true, "border": true, "text": true, "ver": true, "version": true,
}

// treatmentSaid reports whether a wording claims the gold treatment rather
// than merely saying the word.
//
// A storefront claims it beside one of the words a treatment is named with
// ("Gold Foil", "Gold Border", "Gold text/Border", "Gold Ver."), or with the
// word standing at the end of what it wrote, which is where the decorations
// hung off a name end up. The art sentences say gold of something in the
// picture instead - a hook, a bell, a logo - and reading those as the claim
// priced a set's plain DON!! as its gold-bordered twin.
//
// Which word comes next is read off the punctuation as well as the spaces:
// a storefront runs its decorations together ("Gold text/Border", "(Gold
// Border)(We'll Have To...") and joining those into one word hid the
// treatment behind it.
func treatmentSaid(wording string) bool {
	words := strings.FieldsFunc(strings.ToLower(wording), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for i, word := range words {
		if word != goldTreatment {
			continue
		}
		if i == len(words)-1 || treatmentNeighbours[words[i+1]] {
			return true
		}
	}
	return false
}

// labelTokens are the tokens a printing answers a wording with, which are
// its promo types: the register puts the mark among them, because a mark is
// a fact a listing names even though it is no promotion.
func labelTokens(card matcher.Card) []string {
	return card.PromoTypes
}

// promoLabel builds a printing's wording out of the facts the datastore
// publishes: the words its promo types read as, the mark saying which copy
// of the number it is, and on the end the place it was awarded for, which is
// where the catalog wrote it and where placeAwarded reads it.
func promoLabel(b *matcher.Backend, card matcher.Card) string {
	mark := matcher.PromoTypeSlug(card.Watermark)
	when := promoDate(card.OriginalReleaseDate)
	dated := matcher.PromoTypeSlug(when)
	var event, place []string
	for _, promoType := range card.PromoTypes {
		switch {
		case promoType == mark, promoType == dated:
			// The mark and the date ride among the tags so that a listing
			// can name them; they are written out below, in the place the
			// catalog wrote them, rather than twice.
			continue
		case slices.Contains(promoPlaces, promoType):
			place = append(place, b.PromoTypeLabel(promoType))
		default:
			event = append(event, b.PromoTypeLabel(promoType))
		}
	}
	if card.Watermark != "" {
		event = append(event, matcher.Title(card.Watermark))
	}
	// The date the promotion ran, where the datastore publishes one the set
	// does not: the catalog writes it into the label - "Treasure Cup August
	// 2025" beside a plain "Treasure Cup" - and the two are told apart by
	// nothing else.
	if when != "" {
		event = append(event, when)
	}
	return strings.Join(append(event, place...), " ")
}

// labelWords cuts a label or a wording into the words a comparison can be
// made of, each stripped of the punctuation the slugging drops so that
// "Law," and "Monkey.D.Luffy" read as they are written.
func labelWords(text string) []string {
	fields := strings.Fields(text)
	words := make([]string, 0, len(fields))
	for _, field := range fields {
		if word := matcher.PromoTypeSlug(field); word != "" {
			words = append(words, word)
		}
	}
	return words
}

// uniqueWords drops the repeats out of a label's words, so that a word a
// label spells twice is still one label carrying it.
func uniqueWords(words []string) []string {
	seen := map[string]bool{}
	out := words[:0:0]
	for _, word := range words {
		if seen[word] {
			continue
		}
		seen[word] = true
		out = append(out, word)
	}
	return out
}

// wantsVariant reports whether the input demands some variant printing
// without describing which: cardmarket's "(V.n)" with n past 1, or a letter
// tail on the collector number ("OP01-001a").
func wantsVariant(inCard *matcher.InputCard, number string) bool {
	if number != "" && number[len(number)-1] >= 'a' {
		return true
	}
	if wantsUnnamedVariant(inCard) {
		return true
	}
	for field := range strings.FieldsSeq(strings.ToLower(inCard.Variation)) {
		if strings.HasPrefix(field, "v.") && field != "v.1" {
			return true
		}
	}
	return false
}

// unnamedVariantWords are the ways a storefront says a printing is not the
// base one without saying which it is. The catalog labels the same printing
// several ways - 177 of them "Parallel", 515 "Alternate Art" - so a word here
// names no label in particular, only that the listing is asking past the base.
// A wording that does name a label is answered by that label before any of
// this is reached.
var unnamedVariantWords = []string{"parallel", "alternate art", "alt art"}

// wantsUnnamedVariant reports whether the wording asks for a printing other
// than the base without naming which. Cool Stuff Inc sells three Kouzuki
// Hiyori at EB01-013 and calls the second one "(Parallel)" where the catalog
// calls it "Alternate Art", so the word cannot be matched against a label -
// what it says is that the base is the wrong answer.
func wantsUnnamedVariant(inCard *matcher.InputCard) bool {
	wording := strings.ToLower(inCard.Variation)
	for _, word := range unnamedVariantWords {
		if strings.Contains(wording, word) {
			return true
		}
	}
	return false
}

// plainUnnamedVariant narrows to the single candidate whose only tag is the
// plain treatment ("alternateart" or "parallel") a bare unnamed-variant
// wording asks for, nil where none or several answer.
func plainUnnamedVariant(cards []matcher.Card) []matcher.Card {
	var plain []matcher.Card
	for _, card := range cards {
		if len(card.PromoTypes) == 1 && (card.PromoTypes[0] == "alternateart" || card.PromoTypes[0] == "parallel") {
			plain = append(plain, card)
		}
	}
	if len(plain) != 1 {
		return nil
	}
	return plain
}

// inputNumber is extractNumber with the card being asked about in hand: the
// number a storefront wrote only narrows when the printings of that name
// wear collector numbers it could be one of, and the words it spent on a
// label are not the number.
func inputNumber(b *matcher.Backend, inCard *matcher.InputCard) string {
	wording := unlabelled(b, inCard.Name, inCard.Variation)
	if number := slashedNumber(b, inCard.Name, wording); number != "" {
		return number
	}
	number := extractNumber(wording)
	if number == "" || numbered(b, inCard.Name) {
		return number
	}
	return ""
}

// slashedNumber is a word of the wording that is, slash and all, the number
// a printing of the name wears: the crew certificate's "1/1000" is its
// number, not card 1 of a thousand, which is how extractNumber reads it.
func slashedNumber(b *matcher.Backend, name, wording string) string {
	for _, field := range strings.Fields(wording) {
		if !strings.Contains(field, "/") {
			continue
		}
		for _, uuid := range b.Hashes[matcher.Normalize(name)] {
			co, found := b.UUIDs[uuid]
			if found && !co.Sealed && strings.EqualFold(co.Number, field) {
				return field
			}
		}
	}
	return ""
}

// unlabelled is a wording with every label it spells out whole taken off,
// among the labels the printings of a name wear, and every mark it says
// every word of.
//
// extractNumber falls back on the first word opening with a digit, and the
// labels carry words that do: the ordinal of an anniversary, the count in
// "3 Brothers Pack", the year a promotion ran, the instalment of a pack. A
// storefront writing no number for the leader card of the 3rd Anniversary
// Tournament pack was read as asking for number 3, which every set's third
// card answers and the leader does not. A word the wording spent on a label
// the catalog wears is that label's word; what is left is where a number
// could be. The mark is read as markWording reads it, a word at a time,
// because the catalog writes its words apart.
func unlabelled(b *matcher.Backend, name, wording string) string {
	words := strings.Fields(wording)
	kept := make([]bool, len(words))
	for i := range kept {
		kept[i] = true
	}
	for _, uuid := range b.Hashes[matcher.Normalize(name)] {
		co, found := b.UUIDs[uuid]
		if !found || co.Sealed {
			continue
		}
		for _, promoType := range co.PromoTypes {
			for first, last := range slugRuns(words, promoType) {
				for i := first; i <= last; i++ {
					kept[i] = false
				}
			}
		}
		if !markSaid(wording, co.Watermark) {
			continue
		}
		for _, mark := range labelWords(co.Watermark) {
			for i, word := range words {
				if matcher.PromoTypeSlug(word) == mark {
					kept[i] = false
				}
			}
		}
	}
	var out []string
	for i, word := range words {
		if kept[i] {
			out = append(out, word)
		}
	}
	return strings.Join(out, " ")
}

// slugRuns finds the runs of whole words in a wording that spell a slug, as
// SlugDescribes reads them, keyed by the first word's index and holding the
// last's.
//
// Punctuation words are skipped as SlugDescribes skips them, and none begins
// a run, so unlabelled clears a stretch once rather than once per dash before
// a label; it only reads a number out of what is left.
func slugRuns(words []string, slug string) map[int]int {
	runs := map[int]int{}
	if slug == "" {
		return runs
	}
	slugs := make([]string, len(words))
	var spelled []int
	for i, word := range words {
		slugs[i] = matcher.PromoTypeSlug(word)
		if slugs[i] != "" {
			spelled = append(spelled, i)
		}
	}
	for k, first := range spelled {
		rest := slug
		for _, j := range spelled[k:] {
			if !strings.HasPrefix(rest, slugs[j]) {
				break
			}
			rest = rest[len(slugs[j]):]
			if rest == "" {
				runs[first] = j
				break
			}
		}
	}
	return runs
}

// numbered reports whether any printing of a name wears a collector number
// with a digit in it. The DON!! cards are the game's one exception: every
// printing of them is filed under the literal code "DON" instead of a
// number, so nothing a storefront writes in its number field is one of
// theirs - coolstuffinc writes the set code there ("PRB-01"), cardmarket the
// number of the card the DON!! was packed with - and comparing it deleted
// every candidate the name had. A name with even one numbered printing is
// not that case: "Monkey.D.Luffy" wears "LEADER" on a few event printings
// and a real number everywhere else, and the number still has to tell those
// apart.
func numbered(b *matcher.Backend, name string) bool {
	for _, uuid := range b.Hashes[matcher.Normalize(name)] {
		co, found := b.UUIDs[uuid]
		if !found || co.Sealed {
			continue
		}
		if strings.ContainsFunc(co.Number, unicode.IsDigit) {
			return true
		}
	}
	return false
}

// extractNumber pulls the collector number out of the scraper-supplied
// Variation: the full "OP01-001" shape when present, else the first
// digit-leading field ("001", "001/121").
func extractNumber(variation string) string {
	fields := strings.Fields(variation)
	for _, field := range fields {
		if fullNumberRe.MatchString(field) {
			return strings.Split(field, "/")[0]
		}
	}
	for _, field := range fields {
		if field[0] >= '0' && field[0] <= '9' {
			return strings.Split(field, "/")[0]
		}
	}
	return ""
}

// numberMatches compares an input number against a printing's full
// collector number: equal full codes, or a bare input matching the code's
// numeric tail. Leading zeros never decide ("1" matches "OP01-001"), and a
// letter tail on the input ("OP01-001a", "OP01-002β") matches its base
// number - the catalog files a corrected run on the base number too.
func numberMatches(input, full string) bool {
	if strings.EqualFold(input, full) {
		return true
	}
	input = strings.TrimRight(input, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZαβ")
	if strings.EqualFold(input, full) {
		return true
	}
	inSet, inTail := splitNumber(input)
	fullSet, fullTail := splitNumber(full)
	// An input spelling its own set code has already named which printing's
	// number it is, and only that code may answer it: dropping to the tail
	// alone would let "OP07-002" take every card numbered -002 in the game,
	// aliasing a set's card against its every same-numbered reprint. A bare
	// input carries no code to disagree with, so it still matches on the
	// tail: that is the whole identification cardmarket's number field has.
	if inSet != "" && !strings.EqualFold(inSet, fullSet) {
		return false
	}
	return inTail != "" && matcher.CanonicalTail(inTail) == matcher.CanonicalTail(fullTail)
}

// splitNumber cuts a collector number into its set code and numeric tail,
// a number without a code being all tail.
func splitNumber(number string) (set, tail string) {
	idx := strings.LastIndexByte(number, '-')
	if idx >= 0 {
		return number[:idx], number[idx+1:]
	}
	return "", number
}
