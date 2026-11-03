package lorcana

import (
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/mtgban/go-mtgban/matcher"
)

// Rules implements matcher.GameRules for Lorcana. Lorcana has no edition
// aliases, variant tables, or promo types: a card is identified by name +
// collector number + foil. So most hooks are no-ops and the real work is the
// number disambiguation in FilterCards (foil is honored downstream by output).
type Rules struct {
	matcher.DefaultRules

	// treatments are the promo types each printing carries, by uuid: the
	// Rainbow Pillars of a card sold in it beside its cold foil is one
	// printing, and a listing naming it names that printing.
	treatments map[string][]string
}

// Prefilter splits a trailing parenthetical variant off the name before the
// canonical-name lookup. Unlike Magic it leaves " - " intact, since Lorcana
// names are "Character - Title".
func (Rules) Prefilter(b *matcher.Backend, inCard *matcher.InputCard) {
	// A name that is itself canonical stays whole: two catalog names carry
	// their qualifier inside them - the Errata Version of Bucky and of Elsa,
	// Gloves Off - and splitting those looked up the bare name instead, so
	// the errata printing was unreachable even when a listing spelled it out
	// exactly and answered with the original standing at the same number.
	_, canonical := b.CanonicalNames[matcher.Normalize(inCard.Name)]
	if !canonical && strings.Contains(inCard.Name, "(") {
		vars := matcher.SplitVariants(inCard.Name)
		if len(vars) > 1 {
			inCard.Name = vars[0]
			inCard.AddToVariant(strings.Join(vars[1:], " "))
		}
	}
	adoptQualifiedName(b, inCard)
}

// qualifiedNameRe matches the trailing parenthetical the catalog keeps inside
// a name instead of distilling it into a label of its own.
var qualifiedNameRe = regexp.MustCompile(`\s*\(([^()]*)\)$`)

// promoPoolSet is the code the catalog carries every otherwise-unbound
// promotional printing under, whatever heading a storefront files it under
// instead. Cardmarket's "Promos Year N" and "Lorcana Challenge Promos Year
// N", and Cool Stuff Inc's plain "Promo", all name no set of their own; the
// printings they cover still have to live somewhere, and this is where.
const promoPoolSet = "DLPC"

// adoptQualifiedName adopts the catalog's decorated spelling of a name a
// storefront wrote bare, when a collector number is given and exactly one
// decorated candidate answers it: with a named set, one whose wording states
// a word of the decoration; with no set (a promo heading), any one candidate
// in the DLPC pool. Two decorated siblings, or a plain printing already at
// that number, refuse rather than guess.
func adoptQualifiedName(b *matcher.Backend, inCard *matcher.InputCard) {
	number := extractNumber(inCard.Variation)
	if number == "" {
		return
	}
	set, err := b.GetSetByName(inCard.Edition)
	if err == nil && set != nil {
		adoptFromSet(set, inCard, number, true)
		return
	}
	pool, err := b.GetSet(promoPoolSet)
	if err == nil && pool != nil {
		adoptFromSet(pool, inCard, number, false)
	}
}

// adoptFromSet performs the scan and the swap for adoptQualifiedName, over
// whichever set the caller resolved the edition to. requireWording is the
// named-set gate, off for the pool search that has no wording to gate on.
func adoptFromSet(set *matcher.Set, inCard *matcher.InputCard, number string, requireWording bool) {
	wording := strings.ToLower(inCard.Variation)
	var adopt string
	for i := range set.Cards {
		card := &set.Cards[i]
		if card.Number != number {
			continue
		}
		match := qualifiedNameRe.FindStringSubmatchIndex(card.Name)
		if match == nil {
			if !requireWording && matcher.Equals(card.Name, inCard.Name) {
				return
			}
			continue
		}
		if !matcher.Equals(card.Name[:match[0]], inCard.Name) {
			continue
		}
		if requireWording && !statesQualifier(wording, card.Name[match[2]:match[3]]) {
			continue
		}
		if adopt != "" && !matcher.Equals(adopt, card.Name) {
			return
		}
		adopt = card.Name
	}
	if adopt != "" {
		inCard.Name = adopt
	}
}

// statesQualifier reports whether the wording names the qualifier: one of
// its distinguishing words, written whole. The category noun a multi-word
// qualifier ends on - the Version of "Errata Version" - licenses nothing
// alone, being the word a note about the other printing carries just as
// well; and a word is read whole, so a note's own longer words cannot
// smuggle one in.
func statesQualifier(wording, qualifier string) bool {
	words := strings.Fields(strings.ToLower(qualifier))
	if len(words) > 1 {
		words = words[:len(words)-1]
	}
	tokens := strings.FieldsFunc(wording, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, word := range words {
		if slices.Contains(tokens, word) {
			return true
		}
	}
	return false
}

// nameHit is one (name, set) pair an AdjustName tier holds. A reprinted name
// carries one hit per set it survives in, rather than one hit overall, so
// narrowing a tier down to an edition's set can never lose a set the name is
// genuinely also printed under.
type nameHit struct {
	name    string
	setCode string
}

// addHit appends a (name, set) pair not already in hits.
func addHit(hits []nameHit, name, setCode string) []nameHit {
	if slices.ContainsFunc(hits, func(h nameHit) bool { return h.name == name && h.setCode == setCode }) {
		return hits
	}
	return append(hits, nameHit{name, setCode})
}

// distinctNames returns the names hits holds, each once.
func distinctNames(hits []nameHit) []string {
	var names []string
	for _, h := range hits {
		if !slices.Contains(names, h.name) {
			names = append(names, h.name)
		}
	}
	return names
}

// AdjustName provides a prefix fallback: scraper feeds sometimes truncate the
// "Character - Title" name. When the exact name is unknown, scan for cards
// whose name has the input as a prefix and let the collector number and finish
// narrow them; adopt the name only when exactly one survives, or - failing
// that - when the edition names the one set among them. If several distinct
// names still survive, the input stays unresolved and Match reports an
// unknown name (without a single name there is nothing to hand the pipeline).
func (Rules) AdjustName(b *matcher.Backend, inCard *matcher.InputCard) {
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; found {
		return
	}
	number := extractNumber(inCard.Variation)
	// A name no printing opens with has no prefix matches to tier, and the
	// number is all that is left to read it by.
	uuids, err := b.SearchHasPrefix(inCard.Name)
	if err != nil {
		uuids = nil
	}

	var fits, wrongFinish []nameHit
	for _, uuid := range uuids {
		co, err := b.GetUUID(uuid)
		if err != nil {
			continue
		}
		// A sealed product shares the name buckets but is never the card
		// the truncated name is reaching for.
		if co.Sealed {
			continue
		}
		if number != "" && number != co.Number {
			continue
		}
		// A foil claim is a claim, and a printing sold in no foil cannot
		// answer it: the flag stands. Saying nothing is not the same claim,
		// so an unflagged input only sets a foil-only name aside.
		if inCard.Foil && !co.HasFinish(matcher.FinishFoil) {
			continue
		}
		if !inCard.Foil && !co.HasFinish(matcher.FinishNonfoil) {
			wrongFinish = addHit(wrongFinish, co.Name, co.SetCode)
			continue
		}
		fits = addHit(fits, co.Name, co.SetCode)
	}
	// The first tier holding anything decides, the shape FilterCards uses:
	// the names set aside only ever answer where the plain finish had
	// nothing to offer.
	setCode := soleSet(b, inCard.Edition)
	for _, tier := range [][]nameHit{fits, wrongFinish} {
		if len(tier) == 0 {
			continue
		}
		// A multi-name tier is narrowed to the edition's sole set, the same
		// way FilterCards reads it; naming none or several leaves the tier
		// exactly as ambiguous as before.
		names := distinctNames(tier)
		if len(names) > 1 && setCode != "" {
			names = distinctNames(slices.DeleteFunc(slices.Clone(tier), func(h nameHit) bool {
				return h.setCode != setCode
			}))
		}
		if len(names) == 1 {
			inCard.Name = names[0]
		}
		return
	}

	if name := nameAtNumber(b, inCard, number); name != "" {
		inCard.Name = name
	}
}

// nameAtNumber returns the datastore's spelling of the card a storefront
// misspelled, read off the set and collector number it wrote beside the name.
//
// A storefront that cannot spell a name still files the listing in a set and
// numbers it, and a number names one card of a set. Coolstuffinc doubles a
// letter that is single and singles one that is double ("Metalic Leader",
// "Gepetto"), drops one altogether ("Somone Will Lose His Head", "Valourous
// General") or swaps two ("Ambitious Entreperneur"), and every one of those
// listings carries the number that says which card it is in the same record.
//
// Two things gate it. The edition has to name one set, since a number is only
// an identifier within one. And exactly one of the cards wearing that number
// there has to be close to the misspelling - a piece missing off one end, or
// up to two letters wrong - because the number is the storefront's own claim
// and is sometimes another card's, so without the closeness test a stray
// number would rename a card into whatever it pointed at.
//
// A number is not an identifier on its own even inside a set: a promo shelf
// numbers each of its runs from one, so Disney Lorcana Promo Cards holds seven
// cards numbered 2. Closeness is what tells them apart, and demanding it pick
// one is what keeps a stray number from reaching the others.
func nameAtNumber(b *matcher.Backend, inCard *matcher.InputCard, number string) string {
	if number == "" {
		return ""
	}
	setCode := soleSet(b, inCard.Edition)
	if setCode == "" {
		return ""
	}
	var match, matchNorm string
	for _, uuid := range b.AllUUIDs {
		co, err := b.GetUUID(uuid)
		if err != nil || co.Sealed || co.SetCode != setCode || co.Number != number {
			continue
		}
		norm := matcher.Normalize(co.Name)
		if norm == matchNorm || !matcher.CloseName(inCard.Name, co.Name) {
			continue
		}
		if match != "" {
			return ""
		}
		match, matchNorm = co.Name, norm
	}
	return match
}

// soleSet returns the code of the one set an edition names, or "" where none
// or several answer. The name is read the way Match reads it, whole or as a
// part, with the decorations a storefront hangs off it trimmed first.
func soleSet(b *matcher.Backend, edition string) string {
	edition = trimEdition(edition)
	if matcher.Normalize(edition) == "" {
		return ""
	}
	var found string
	for code, set := range b.Sets {
		if !matcher.Contains(set.Name, edition) {
			continue
		}
		if found != "" {
			return ""
		}
		found = code
	}
	return found
}

// AliasEdition spells an edition string toward a LorcanaJSON set name using
// the string alone, and is the game's whole AdjustEdition: storefronts
// commonly prefix the game name ("Disney Lorcana: Rise of the Floodborn") or
// append catalog suffixes ("... Singles"). An edition that still matches no
// set name simply does not narrow the candidates (the Match skeleton falls
// back to every printing), so trimming can only help.
func (Rules) AliasEdition(b *matcher.Backend, edition string) string {
	edition = trimEdition(edition)
	if matcher.IsPromoHeading(edition) {
		edition = ""
	}
	return edition
}

// trimEdition strips the game name a storefront prefixes a set with and the
// catalog suffix it appends.
func trimEdition(edition string) string {
	edition = strings.TrimSpace(edition)
	for _, prefix := range []string{"Disney Lorcana", "Lorcana"} {
		if strings.HasPrefix(edition, prefix) {
			edition = strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(edition, prefix), ":-"))
			break
		}
	}
	return strings.TrimSpace(strings.TrimSuffix(edition, "Singles"))
}

// IsUnsupported drops the non-card products TCGplayer files under its "Cards"
// product type, so they are skipped instead of reported as unknown names: the
// puzzle-piece inserts bundled with booster displays (sold per piece and as
// whole sets), the story inserts a set packs beside them, and the multi-card
// promo lots. No Lorcana card carries any of those wordings and none ever
// will, since none of them is a card.
//
// Both tests are literal and read the name alone. Normalize erases every "s",
// which turns the lots' "Set of" into "etof" - a substring of "The Queen -
// Cruelest of All" and four more real names - so the normalized Contains is
// too lossy here. And the promotion behind the lot also names a real card's
// variant, "Mickey Mouse - True Friend (Disney Cruise Promo)", which the
// prefilter leaves in the variation: anchoring at the start of the name keeps
// the rule off every listing that merely mentions the promotion.
//
// The story inserts are named for the set that packs them and end on the
// word - "Reign of Jafar - Lore Story Insert", "Azurite Sea Insert" - so the
// suffix is what reads them all without naming each set. No printing the
// datastore carries ends there: the puzzle inserts it does carry spell which
// piece they are after it, "Puzzle Insert (Top Left)".
//
// The discard-pile markers sold as singles ("White "Discard" Card", the
// misquoted "Enchanted ""Discard"" Card") are read the same way, by the
// quoted "Discard" their name always ends on: a suffix rather than a
// Contains, so the real card "Discarded Armor" keeps matching.
func (Rules) IsUnsupported(b *matcher.Backend, inCard *matcher.InputCard, co *matcher.CardObject, stage matcher.Stage) bool {
	return stage == matcher.StageWording && (strings.Contains(inCard.Name, "Puzzle Insert") ||
		strings.HasSuffix(inCard.Name, "Insert") ||
		strings.HasPrefix(inCard.Name, "Disney Cruise Promos") ||
		strings.HasSuffix(inCard.Name, `"Discard" Card`) ||
		strings.HasSuffix(inCard.Name, `""Discard"" Card`))
}

// plainNumberTail are the letters a variant is spelled with behind a number.
const plainNumberTail = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// PlainNumber implements matcher.GameRules. The letter behind the number
// names the variant, and the number it varies is the number the card prints;
// the ★ marks the loader's twin for a foil sold as a product of its own.
func (Rules) PlainNumber(number string) string {
	plain := strings.TrimRight(strings.TrimSuffix(number, "★"), plainNumberTail)
	if plain == "" {
		return number
	}
	return plain
}

// FilterCards narrows candidates by edition, collector number, and finish.
// sets holds those matching the input edition when one was supplied
// and resolves, and every printing otherwise, so it disambiguates a
// name+number shared across sets ("Let It Go" #163) while a missing or
// unrecognized edition changes nothing.
func (r Rules) FilterCards(b *matcher.Backend, inCard *matcher.InputCard, sets []matcher.CandidateSet) []matcher.Card {
	number := extractNumber(inCard.Variation)
	// A letter hung off the end of the number may be the storefront's own
	// promo-series marker - Strikezone numbers its promos "010B", "003C" -
	// or the letter the data itself gives a same-numbered sibling ("4a"
	// through "4e"). Only the number as written can tell which, so the
	// stripped form is kept as a fallback and never as a substitute.
	bare := strings.TrimRight(number, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	if bare == number {
		bare = ""
	}
	total := extractTotal(inCard.Variation)

	var out, wrongFinish, bareOut, bareWrongFinish, chaseOut, chaseWrongFinish []matcher.Card
	chased := false
	var cards []matcher.Card
	for _, set := range sets {
		cards = append(cards, set.Cards...)
	}
	for _, card := range cards {
		exact := number == "" || number == card.Number || number == card.PlainNumber
		bareFits := !exact && bare != "" && bare == card.Number
		// A named chase tier is a claim about the printing, and it holds
		// wherever the number does not: a storefront that writes one down
		// has said which of two same-named printings the listing is even
		// when the number beside it reaches neither.
		chase := namesChaseRarity(inCard.Variation, card.Rarity)
		chased = chased || chase
		if !exact && !bareFits && !chase {
			continue
		}
		// Set aside the candidates sold in no plain finish. A card with both
		// passes either way; output() picks the uuid.
		finishFits := card.HasFinish(matcher.FinishNonfoil)
		if inCard.Foil {
			// A foil claim is a claim, and a printing sold in no foil
			// cannot answer it: the flag stands.
			if !card.HasFinish(matcher.FinishFoil) {
				continue
			}
			finishFits = true
		}
		// A listing naming a foil sub-type re-keys the copy's FoilUUIDs so
		// the flag-driven resolution downstream lands on that sub-type's uuid
		// instead of the primary foil's.
		if uuid := selectFinish(b, inCard, &card, r.treatments); uuid != "" {
			foilUUIDs := maps.Clone(card.FoilUUIDs)
			foilUUIDs[matcher.FinishFoil] = uuid
			card.FoilUUIDs = foilUUIDs
		}
		switch {
		case exact && finishFits:
			out = append(out, card)
		case exact:
			wrongFinish = append(wrongFinish, card)
		case bareFits && finishFits:
			bareOut = append(bareOut, card)
		case bareFits:
			bareWrongFinish = append(bareWrongFinish, card)
		case finishFits:
			chaseOut = append(chaseOut, card)
		default:
			chaseWrongFinish = append(chaseWrongFinish, card)
		}
	}
	// A feed that never says "foil" must not lose a foil-only card outright:
	// the promotional printings are sold foil only, and a storefront listing
	// them plain would have every one of them deleted here. Saying nothing
	// is not the same claim as saying nonfoil, which is why only this
	// direction falls back. The number as written and the plain finish both
	// still decide whenever they have candidates to choose between, so each
	// fallback only ever answers where the alternative answers nothing.
	tiers := [][]matcher.Card{out, wrongFinish, bareOut, bareWrongFinish, chaseOut, chaseWrongFinish}
	// Once a tier is named by a printing that carries it, the printings that
	// do not are contradicted rather than merely unmentioned, and go out of
	// the tiers a number did not decide: the letter a storefront hangs off a
	// chase card's number is the one thing standing between the stripped
	// form and the plain card it numbers, and stripping it is a guess where
	// the named tier is a statement.
	//
	// A number that was written and landed exactly is the more specific
	// claim of the two and keeps its tiers, so a wording that names the tier
	// beside a number naming the printing outright never overrules it. A
	// number that was not written at all decided nothing, and there the
	// wording is all the listing said.
	first := 0
	if number != "" {
		first = 2
	}
	if chased {
		for i := first; i < len(tiers); i++ {
			tiers[i] = keepChaseRarity(inCard.Variation, tiers[i])
		}
	}
	for _, tier := range tiers {
		if len(tier) > 0 {
			return promoOrdinalTiebreak(b, inCard, totalTiebreak(total, tier))
		}
	}
	return nil
}

// chaseRarities are the tiers Lorcana keeps for the alternate art of a card
// already printed in the set, as the datastore spells them. They are the only
// rarities read: the rest of the ladder is never how a storefront tells two
// printings of one name apart, and "Promo" is a word half the promo wordings
// carry anyway.
var chaseRarities = []string{"enchanted", "epic", "iconic"}

// namesChaseRarity reports whether the variation claims the printing is the
// chase tier given. "Alternate Art" is the nickname a storefront writes for
// the same claim, and naming a tier no candidate carries reaches nothing, so
// a nickname read onto the wrong one of the three costs no printing.
func namesChaseRarity(variation, rarity string) bool {
	if !slices.Contains(chaseRarities, rarity) {
		return false
	}
	return spellsOut(variation, rarity) || spellsOut(variation, "alternate art")
}

// keepChaseRarity drops the printings whose tier the variation contradicts.
func keepChaseRarity(variation string, cards []matcher.Card) []matcher.Card {
	var kept []matcher.Card
	for _, card := range cards {
		if namesChaseRarity(variation, card.Rarity) {
			kept = append(kept, card)
		}
	}
	return kept
}

// spellsOut reports whether the text writes the phrase as consecutive whole
// words, so a tier spelled inside a longer word is not read as the tier.
func spellsOut(text, phrase string) bool {
	words := strings.Fields(strings.ToLower(text))
	wanted := strings.Fields(phrase)
	for i := 0; i+len(wanted) <= len(words); i++ {
		if slices.Equal(words[i:i+len(wanted)], wanted) {
			return true
		}
	}
	return false
}

// totalTiebreak narrows a tier to the printings whose face prints the
// denominator the storefront wrote behind the number.
//
// The datastore numbers each promo pool from one, so a card promoted twice
// carries the same number in both: "Maleficent - Monstrous Dragon" is card 5
// of the P1 pool and card 5 of the P3 one, and the number alone cannot tell
// the two apart. The denominator can, and the storefront writes it where a
// set card writes its set size - which is exactly what it is. SetTotal is the
// denominator the face prints, "P3" on a promo where a card of the set
// prints "204", so the tier is narrowed on the card's own total rather than
// on a promo type: the pool is not a promotion.
//
// A count separates the same way a pool does, and for the same reason: a set
// card and a promo of it stand at one number and print different totals under
// it. Cool Stuff Inc buys Fabled's "Stitch - Rock Star" at "3/204", where the
// set's Super Rare prints 204 and the Disney Parks printing beside it prints
// DIS, and without the count the two answer together as an aliasing.
//
// A total no candidate carries keeps the whole tier. The storefront's
// spelling of a total is its own - it prints the one the card came from,
// which is not always the one the datastore numbered it in - and refusing a
// printing over it would price nothing where the number alone was answering.
func totalTiebreak(total string, cards []matcher.Card) []matcher.Card {
	if total == "" || len(cards) <= 1 {
		return cards
	}
	var totalled []matcher.Card
	for _, card := range cards {
		if strings.EqualFold(card.SetTotal, total) {
			totalled = append(totalled, card)
		}
	}
	if len(totalled) == 0 {
		return cards
	}
	return totalled
}

// extractOrdinal reads the position Cardmarket's own "(V.N)" wording claims
// among same-named, same-numbered siblings, which Core Match has already
// split off the name and appended to Variation by the time this runs. A
// storefront that names no position at all returns 0, which is Cardmarket's
// own way of saying the first - it never suffixes the wave it lists first.
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

// promoOrdinalTiebreak narrows same-named, same-numbered siblings left tied
// once totalTiebreak has had its say, using the position Cardmarket's own
// "(V.N)" wording claims. Cardmarket sends a bare number, no denominator, so
// totalTiebreak is always a no-op for it: "Maleficent - Monstrous Dragon",
// card 5 of the P1 pool and card 5 of the P3 one per totalTiebreak's own
// example, is exactly the tie a Cardmarket listing reaches with nothing left
// to read off Number - Cardmarket is the one side still holding the answer,
// in the wording rather than in Number.
//
// The tie is trusted only where every candidate is Special: that is the
// rarity Lorcana's own promo waves carry, and nothing about a Cardmarket
// wave count is a claim about a tie some other cause produced - the number
// and name already decided everywhere else, so this never overrides a claim
// the datastore itself could make. Candidates are sorted by the release date
// of the set housing each printing, not always a set named for the promo
// itself but the closest thing every wave publishes, and the wording's own
// count indexes into that order the way it indexed into Cardmarket's own
// listing order.
//
// An unsuffixed wording only counts as Cardmarket's own "V.1" where nothing
// else in it claims a denominator - a storefront naming a pool of its own,
// recognized or not, has made its own claim about the tie, and a total
// totalTiebreak could not place must not be overridden by a wave count read
// into the silence around it - and where the edition itself says which
// promo wave this is. Silence alone is not a claim any storefront's bare
// number could not equally send, so it is read as Cardmarket's own "V.1"
// only where the edition confirms the shape: mentionsPromoWave is what
// keeps a same-numbered tie at some other storefront, that happens to land
// on this same pair of Special printings, from being guessed at instead of
// refused.
func promoOrdinalTiebreak(b *matcher.Backend, inCard *matcher.InputCard, cards []matcher.Card) []matcher.Card {
	if len(cards) <= 1 {
		return cards
	}
	for _, card := range cards {
		if !strings.EqualFold(card.Rarity, "special") {
			return cards
		}
	}
	ordinal := extractOrdinal(inCard.Variation)
	if ordinal == 0 {
		// An unsuffixed wording only counts as Cardmarket's own "V.1" where
		// the edition itself says which promo wave it is: any storefront
		// can send a bare number, but only Cardmarket's "Promos Year N"
		// heading is a claim this tiebreak has verified means what it
		// answers with. Elsewhere, silence stays silence.
		if extractTotal(inCard.Variation) != "" || !mentionsPromoWave(inCard.Edition) {
			return cards
		}
		ordinal = 1
	}
	if ordinal > len(cards) {
		return cards
	}
	sorted := slices.Clone(cards)
	slices.SortFunc(sorted, func(a, c matcher.Card) int {
		ta, tc := setReleaseDate(b, a.SetCode), setReleaseDate(b, c.SetCode)
		if !ta.Equal(tc) {
			if ta.Before(tc) {
				return -1
			}
			return 1
		}
		return strings.Compare(a.SetCode, c.SetCode)
	})
	return []matcher.Card{sorted[ordinal-1]}
}

// setReleaseDate is the zero time for a code the datastore does not carry,
// which sorts first rather than panicking - a candidate FilterCards already
// pulled from its sets is never actually missing.
func setReleaseDate(b *matcher.Backend, code string) time.Time {
	set := b.Sets[code]
	if set == nil {
		return time.Time{}
	}
	return set.ReleaseDateTime
}

// mentionsPromoWave reports whether an edition names a Cardmarket yearly
// promo heading - "Promos Year 1", "Promos Year 3" - the one shape
// promoOrdinalTiebreak has verified an unsuffixed wording still means
// Cardmarket's own "V.1" over. Any other edition, known or not, decides
// nothing here.
func mentionsPromoWave(edition string) bool {
	return strings.Contains(strings.ToLower(edition), "promos year")
}

// numberField answers the field of a variation the collector number is
// written in.
//
// A Lorcana number is written over what it is one of, and a field written
// that way is the number wherever the variation holds one. Only where none
// does is the first digit-leading field all that is left to read, which is
// what Core Match leaves behind when it appends parenthetical chunks split
// off the input name ("205 Enchanted", or just "Enchanted" when no number
// was supplied).
//
// The two are not the same field wherever a storefront writes prose in front
// of the number, and prose carries digits: Cool Stuff Inc sells The First
// Chapter's Donald Duck as "Chapter 1 version - 177/204", where the leading
// field numbers the chapter and the card is 177.
func numberField(variation string) string {
	var first string
	for field := range strings.FieldsSeq(variation) {
		if field[0] < '0' || field[0] > '9' {
			continue
		}
		if strings.Contains(field, "/") {
			return field
		}
		if first == "" {
			first = field
		}
	}
	return first
}

// extractNumber pulls the collector number out of the scraper-supplied
// Variation. The number is the part before '/' with leading zeros stripped -
// except a number the zeros are the whole of stays "0", with any letter it
// carries, so the genuine 0-numbered promo stays reachable.
func extractNumber(variation string) string {
	number := strings.Split(numberField(variation), "/")[0]
	trimmed := strings.TrimLeft(number, "0")
	// TrimLeft stops at the first non-zero character of any kind, so a result
	// that is empty or no longer leads with a digit means the zeros were the
	// whole numeric part and one has to come back: "000B" is the 0-numbered
	// card written the way a storefront hangs its promo-series marker off a
	// number, not a number named "B".
	if number != "" && (trimmed == "" || trimmed[0] < '0' || trimmed[0] > '9') {
		trimmed = "0" + trimmed
	}
	return trimmed
}

// extractTotal pulls the denominator out of the number a storefront wrote.
//
// A Lorcana number is written over what it is one of: "87/204" for the
// eighty-seventh of a set of two hundred and four, "5/P3" for the fifth card
// of the third promo pool. Either tail says the same thing about the printing
// and is read as written: the card's SetTotal holds the same denominator in
// the datastore's own spelling, and totalTiebreak compares the two without
// regard to case.
func extractTotal(variation string) string {
	field := numberField(variation)
	if field == "" {
		return ""
	}
	_, tail, found := strings.Cut(strings.TrimRight(field, ","), "/")
	if !found {
		return ""
	}
	return tail
}

// selectFinish maps the foil a listing names onto the uuid of the printing
// sold in it, so a sub-typed printing resolves to its own uuid instead of
// folding onto the primary foil. The caller's own finish answers first; a
// storefront that sends none still spells the sub-type in its wording, and
// the names to look for there are the finishes the printing is sold in, then
// the treatments its printings carry. Anything else, a nonfoil included,
// keeps the flag-driven resolution.
func selectFinish(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card, treatments map[string][]string) string {
	if inCard.Finish != "" {
		uuid := b.FinishUUID(card, inCard.Finish)
		if uuid != "" && uuid != card.FoilUUIDs[matcher.FinishNonfoil] {
			return uuid
		}
	}

	// One wording can hold two of the names and map iteration is random, so
	// they are visited in sorted order.
	variation := matcher.PromoTypeSlug(inCard.Variation)
	for _, finish := range slices.Sorted(maps.Keys(card.FoilUUIDs)) {
		if finish == matcher.FinishNonfoil || finish == matcher.FinishFoil {
			continue
		}
		if strings.Contains(variation, finish) {
			return card.FoilUUIDs[finish]
		}
	}
	for _, uuid := range slices.Compact(slices.Sorted(maps.Values(card.FoilUUIDs))) {
		for _, treatment := range treatments[uuid] {
			if strings.Contains(variation, treatment) {
				return uuid
			}
		}
	}
	return ""
}
