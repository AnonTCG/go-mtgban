package gundam

import (
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// Rules implements matcher.GameRules for the Gundam Card Game. A card is
// identified by name + collector number, and where several printings share
// one number it is the rarity that tells them apart: this game marks a
// parallel run by suffixing the rarity rather than by lettering the number
// or renaming the card. 454 of the game's numbers are shared that way, and
// 439 of those groups differ in rarity and in nothing else; the rest wear
// a label.
type Rules struct{ matcher.DefaultRules }

// rejoinMaxLen bounds the spelling rejoinName normalizes, far past any name.
// A longer spelling comes from an uploaded cell, not a scraper's title, and
// would need hundreds of bytes Normalize drops to be a name.
const rejoinMaxLen = 512

// fullNumberRe matches the game's collector number shapes: "GD01-001",
// "ST01-011", "EB01-023", "RP-001", "T-024". Every number the datastore
// carries matches it.
var fullNumberRe = regexp.MustCompile(`^[A-Za-z]+[0-9]*-[0-9]+[a-zA-Z]*$`)

// trailingCodeRe matches the collector number a storefront writes behind a
// name in parentheses of its own ("Gundam (GD01-001)"). It only matches at
// the end, and only a full code: a parenthetical anywhere else may be part
// of the name.
var trailingCodeRe = regexp.MustCompile(`\s*\(([A-Za-z]+[0-9]*-[0-9]+[a-zA-Z]*)\)$`)

// dashNumberRe matches the collector number hung inside a name after a dash
// ("Wing Gundam - GD01-001").
var dashNumberRe = regexp.MustCompile(`\s+-\s+([A-Za-z]+[0-9]*-[0-9]+[a-zA-Z]*)\b`)

// Prefilter splits the parenthetical decorations off the name before the
// canonical-name lookup: storefronts write "Gundam (GD01-001) (SP)" and
// "Char Aznable - GD01-093". A full name that is itself canonical stays as
// it is, since a card of this game may carry a parenthetical of its own.
func (Rules) Prefilter(b *matcher.Backend, inCard *matcher.InputCard) {
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; found {
		// A name that is a card may still be the head of another: "Delta
		// Plus" is GD01-006, and "Delta Plus" beside "Waverider Mode" is
		// GD02-017.
		rejoinName(b, inCard)
		return
	}
	if m := dashNumberRe.FindStringSubmatch(inCard.Name); m != nil {
		inCard.Name = strings.Replace(inCard.Name, m[0], "", 1)
		inCard.Variation = strings.TrimSpace(m[1] + " " + inCard.Variation)
	}
	// A card whose own name ends in a parenthetical wears two of them once
	// the storefront writes the number behind it, and the split below cannot
	// tell which is which. The number is the one parenthetical never part of
	// a name, so peel that alone and ask again, leaving the name untouched
	// unless the answer is yes.
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
	rejoinName(b, inCard)
}

// rejoinName gives a name back the parenthetical that was split off it. The
// datastore names a card the way the game's list does - "Delta Plus
// (Waverider Mode)", "Freedom Gundam (METEOR)", "Elan Ceres (Enhanced
// Person Number 4)" - and a storefront writes the head plain with the rest
// beside it as a qualifier, which the split above produces too. The head is
// a card of this game on its own often enough (GD01-006 is a Delta Plus)
// that the canonical lookup would stop there and read the qualifier as a
// variant no printing of it has; and where the head is no card at all the
// qualifier was still left behind once AdjustName found the spelling.
//
// The collector numbers in the variation are set aside first, since a
// listing writes its number wherever it likes and no name holds one. Then
// the leading words go back onto the name, longest run first, where the two
// spell a canonical name, and what is left stays the variation: "Freedom
// Gundam" and "GD03-076 Meteor Boost Kit 01" is "Freedom Gundam (METEOR)"
// at GD03-076 from the Boost Kit. Where the card puts its parenthetical in
// the middle - "Guncannon (108) & Guncannon (109)", "Zaku (Four Snake
// Eyes') [YETI] (GQ)" - the split hands the words back in another order, so
// the one canonical name holding every word of the head and made only of
// words the listing wrote is taken, and the words it did not use - the
// rarity of a parallel - stay the variation.
//
// Only the canonical names are consulted, never the qualified spellings a
// printing is searchable under: "Gundam" beside "SP" must stay the suit
// and its rarity, which FilterCards reads, and not become a name.
func rejoinName(b *matcher.Backend, inCard *matcher.InputCard) {
	if inCard.Variation == "" {
		return
	}
	var numbers, words []string
	for _, word := range strings.Fields(inCard.Variation) {
		if fullNumberRe.MatchString(word) {
			numbers = append(numbers, word)
		} else {
			words = append(words, word)
		}
	}
	if len(words) == 0 || headHoldsNumber(b, inCard.Name, numbers) {
		return
	}
	variation := func(left []string) string {
		return strings.Join(append(slices.Clone(numbers), left...), " ")
	}
	// Only a run short enough to be a name is normalized: each one is the
	// whole spelling again, so asking every run of a long variation would be
	// quadratic. Normalize drops only spaces, punctuation and joining words.
	i := len(words)
	for size := len(inCard.Name) + len(strings.Join(words, " ")) + 1; size > rejoinMaxLen && i > 0; i-- {
		size -= len(words[i-1]) + 1
	}
	for ; i > 0; i-- {
		key := matcher.Normalize(inCard.Name + " " + strings.Join(words[:i], " "))
		if canonical, found := b.CanonicalNames[key]; found {
			inCard.Name = canonical
			inCard.Variation = variation(words[i:])
			return
		}
	}
	head := bagOfWords(inCard.Name)
	have := bagOfWords(inCard.Name + " " + strings.Join(words, " "))
	var match string
	var leftover map[string]int
	for _, canonical := range b.CanonicalNames {
		if !strings.ContainsAny(canonical, "([") {
			continue
		}
		bag := bagOfWords(canonical)
		if _, holdsHead := subtract(bag, head); !holdsHead {
			continue
		}
		left, spelled := subtract(have, bag)
		if !spelled {
			continue
		}
		if match != "" && match != canonical {
			return
		}
		match, leftover = canonical, left
	}
	if match == "" {
		return
	}
	var left []string
	for _, word := range words {
		if key := matcher.Normalize(word); leftover[key] > 0 {
			leftover[key]--
			left = append(left, word)
		}
	}
	inCard.Name = match
	inCard.Variation = variation(left)
}

// headHoldsNumber reports whether the name as written is a card with a
// printing at one of the numbers the listing carries. Then the listing means
// that card and the words beside it are its own qualifier, not a name's:
// "GN Armor" at GD03-057 beside "Type-E" is the card the datastore files
// under that head, wherever it also files a "GN Armor Type-E". A listing
// carrying no number says nothing here.
func headHoldsNumber(b *matcher.Backend, name string, numbers []string) bool {
	if len(numbers) == 0 {
		return false
	}
	for _, uuid := range b.Hashes[matcher.Normalize(name)] {
		co, found := b.UUIDs[uuid]
		if !found {
			continue
		}
		for _, number := range numbers {
			if matcher.Equals(co.Number, number) {
				return true
			}
		}
	}
	return false
}

// bagOfWords is the words of a name, normalized one by one and sorted, so
// two spellings that place a parenthetical differently compare equal.
func bagOfWords(s string) []string {
	var out []string
	for _, field := range strings.Fields(s) {
		if word := matcher.Normalize(field); word != "" {
			out = append(out, word)
		}
	}
	sort.Strings(out)
	return out
}

// subtract takes one bag of words out of another, and says whether every
// word was there to take. What is left is counted by word.
func subtract(have, take []string) (map[string]int, bool) {
	left := map[string]int{}
	for _, word := range have {
		left[word]++
	}
	for _, word := range take {
		if left[word] == 0 {
			return nil, false
		}
		left[word]--
	}
	return left, true
}

// AdjustName reaches the printings the catalog files under a qualified
// spelling the storefront wrote plain, and provides a prefix fallback for
// the feeds that truncate. Both only run once the canonical lookup has
// already failed, so a name that resolves on its own is never touched.
func (Rules) AdjustName(b *matcher.Backend, inCard *matcher.InputCard) {
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; found {
		return
	}
	// The qualified spellings are in the name hashes, so a storefront
	// writing "Gundam SP" reaches "Gundam (SP)", whether it wrote the
	// qualifier into the name or beside it: the name becomes the
	// printing's own and the qualifier its labels, spelled into the
	// variation for the tiering to read. The qualified spelling is never
	// canonical, which is why CanonicalNames cannot answer here.
	for _, needle := range []string{
		matcher.Normalize(inCard.Name),
		matcher.Normalize(inCard.Name + " " + inCard.Variation),
	} {
		uuids := b.Hashes[needle]
		if len(uuids) == 0 {
			continue
		}
		co := b.UUIDs[uuids[0]]
		inCard.Name = co.Name
		for _, promoType := range co.PromoTypes {
			inCard.AddToVariant(b.PromoTypeLabel(promoType))
		}
		return
	}
	// A truncated feed keeps the head of the name; accept it when exactly
	// one canonical name starts with what was written. Names compare
	// normalized, so punctuation variants are not read as ambiguity.
	prefix := matcher.Normalize(inCard.Name)
	if len(prefix) < 4 {
		return
	}
	var match string
	for normalized, canonical := range b.CanonicalNames {
		if !strings.HasPrefix(normalized, prefix) {
			continue
		}
		if match != "" && match != canonical {
			return
		}
		match = canonical
	}
	if match != "" {
		inCard.Name = match
	}
}

// AliasEdition resolves the headings storefronts file this game under onto
// the set names the datastore carries. The catalog names a set for the
// product line it belongs to ("Newtype Rising", "Starter Deck 01: Heroic
// Beginnings"), and a storefront usually writes the game in front of it.
// The whole fixup reads only the edition, so it is the fixup entire and
// AdjustEdition delegates to it.
func (Rules) AliasEdition(b *matcher.Backend, edition string) string {
	edition = strings.TrimSpace(edition)
	// An edition already naming a set verbatim needs no normalization, and
	// must not be trimmed out of matching: the promotional sets are named
	// "Gundam Promotional Cards" and would lose their heading below.
	for _, set := range b.Sets {
		if matcher.Equals(set.Name, edition) {
			return edition
		}
	}
	for _, prefix := range []string{"Gundam Card Game", "Gundam"} {
		if strings.HasPrefix(edition, prefix) {
			edition = strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(edition, prefix), ":-"))
			break
		}
	}
	edition = strings.TrimSpace(strings.TrimSuffix(edition, "Singles"))
	// Every promo heading collapses onto one, the way the other games do it:
	// the heading spans several promotional sets and agrees with none of
	// their names, so the tiers in FilterCards choose rather than whichever
	// set name happens to contain the heading's words.
	if edition != "" && (matcher.IsPromoHeading(edition) || endsInPromo(edition)) {
		edition = "Promos"
	}
	return edition
}

// AdjustEdition normalizes the edition a storefront published toward a set
// name, and unpins it where the listing's own wording names a printing that
// set does not hold.
func (Rules) AdjustEdition(b *matcher.Backend, inCard *matcher.InputCard) {
	inCard.Edition = (Rules{}).AliasEdition(b, inCard.Edition)
	if pointedElsewhere(b, inCard) {
		inCard.PromoWildcard = true
	}
}

// pointedElsewhere reports whether the listing's wording names a printing of
// its number that the edition it published excludes.
//
// A storefront shelves a card under the set it is numbered for, and this game
// prints elsewhere at that same number: the promo sets reprint a card for
// every event it was handed out at, and a later booster carries the SP and
// the second parallel of a card a starter deck introduced. Both are shelved
// by the storefront under the original set, so the edition names a set that
// holds only the base printing while the wording beside it names one of the
// others - and the edition gate then deletes the only right answer, leaving
// the base printing to be priced at the parallel's price. Cool Stuff Inc
// bought Gundam Dynames ST07-005 at $585 against a $1.99 ask that way: the
// Newtype Challenge stamp is in the promo set, the shelf said ST07.
//
// PromoWildcard is the flag Match already reads to skip edition selection,
// leaving FilterCards to choose on the label and the rarity - which is what
// the wording named in the first place. It is set only where the wording
// picks a printing outside the edition and none inside it, so a listing the
// edition was answering correctly keeps its answer.
func pointedElsewhere(b *matcher.Backend, inCard *matcher.InputCard) bool {
	number := extractNumber(inCard.Variation)
	if number == "" || inCard.Edition == "" {
		return false
	}
	inside, outside := numberedByEdition(b, inCard, number)
	if len(inside) == 0 || len(outside) == 0 {
		return false
	}
	// The label first: a wording naming an event names the printing handed
	// out at it, and the base printing carries no label to be named by.
	if labelled(inCard.Variation, outside) && !labelled(inCard.Variation, inside) {
		return true
	}
	// Then the rarity, which is this game's own way of telling a parallel
	// from the run it parallels. A rarity the edition's printings do not
	// carry is the wording saying the printing is filed elsewhere: the
	// second parallel of a starter deck's card is sold in the booster that
	// follows it, so ST10-006 is Legend Rare and LR+ under Generation Pulse
	// and LR++ only under Eternal Nexus.
	return saysAnyRarity(b, inCard.Variation, outside) && !saysAnyRarity(b, inCard.Variation, inside)
}

// saysAnyRarity reports whether the wording spells the rarity of any of these
// printings. It differs from rarityNamed in narrowing nothing: a group of one
// still answers, which is the whole question being asked here.
func saysAnyRarity(b *matcher.Backend, wording string, cards []matcher.Card) bool {
	for _, card := range cards {
		if matcher.SaysWord(wording, b.RarityLabel(card.Rarity), "+") {
			return true
		}
	}
	return false
}

// numberedByEdition splits the printings of the input's name at the given
// number into those the edition names and those it does not, reading the
// edition the way Match itself does: an exact set name where one answers,
// and a contained one otherwise.
func numberedByEdition(b *matcher.Backend, inCard *matcher.InputCard, number string) (inside, outside []matcher.Card) {
	var numbered []matcher.Card
	for _, uuid := range b.Hashes[matcher.Normalize(inCard.Name)] {
		co, found := b.UUIDs[uuid]
		if !found || co.Sealed || !strings.EqualFold(number, co.Number) {
			continue
		}
		numbered = append(numbered, co.Card)
	}
	var exact bool
	for _, card := range numbered {
		if matcher.Equals(b.Sets[card.SetCode].Name, inCard.Edition) {
			exact = true
			break
		}
	}
	for _, card := range numbered {
		setName := b.Sets[card.SetCode].Name
		named := matcher.Equals(setName, inCard.Edition)
		if !exact {
			named = matcher.Contains(setName, inCard.Edition)
		}
		if named {
			inside = append(inside, card)
		} else {
			outside = append(outside, card)
		}
	}
	return inside, outside
}

// labelled reports whether the wording describes the labels of any of these
// printings, which is what says the listing means a labelled one.
func labelled(wording string, cards []matcher.Card) bool {
	for _, card := range cards {
		if len(card.PromoTypes) > 0 && wordsDescribe(wording, card.PromoTypes) {
			return true
		}
	}
	return false
}

// endsInPromo reports whether a heading ends in the game's own word for a
// promotional run, which is what a storefront writes where the datastore
// says "Gundam Promotional Cards".
func endsInPromo(edition string) bool {
	lower := strings.ToLower(strings.TrimSpace(edition))
	return strings.HasSuffix(lower, "promo") || strings.HasSuffix(lower, "promos") ||
		strings.HasSuffix(lower, "promotional cards")
}

// PlainNumber implements matcher.GameRules. A number carries its set code and pads the ordinal behind it, where a person writes the ordinal alone: GD01-001 is card 1.
func (Rules) PlainNumber(number string) string {
	return matcher.PlainOrdinal(number)
}

// FilterCards narrows candidates by edition, collector number and then by
// rarity, which is what tells this game's parallel runs apart. The variant
// label carries the event printings and the alternate forms, and is tiered
// first where the storefront's wording describes one.
func (Rules) FilterCards(b *matcher.Backend, inCard *matcher.InputCard, sets []matcher.CandidateSet) []matcher.Card {
	number := extractNumber(inCard.Variation)

	var candidates []matcher.Card
	var cards []matcher.Card
	for _, set := range sets {
		cards = append(cards, set.Cards...)
	}
	for _, card := range cards {
		if number != "" && !strings.EqualFold(number, card.Number) {
			continue
		}
		candidates = append(candidates, card)
	}
	if len(candidates) <= 1 {
		return candidates
	}

	// The variant label is the storefront's own words where it carries one.
	// The labels nest - "World Championship Regionals 26-27 Season 1
	// Finalist" says everything "... Season 1" does - so the longest label
	// the wording describes is the one it means, the way the rarities are
	// read below. Taking every label described would call the two an
	// ambiguity and price neither.
	var described []matcher.Card
	longest := 0
	for _, card := range candidates {
		if len(card.PromoTypes) > 0 && wordsDescribe(inCard.Variation, card.PromoTypes) &&
			labelLength(card.PromoTypes) > longest {
			longest = labelLength(card.PromoTypes)
		}
	}
	for _, card := range candidates {
		if len(card.PromoTypes) > 0 && labelLength(card.PromoTypes) == longest &&
			wordsDescribe(inCard.Variation, card.PromoTypes) {
			described = append(described, card)
		}
	}
	if len(described) > 0 {
		candidates = described
	} else if plain := unlabelled(candidates); len(plain) > 0 {
		// A wording naming no event means the printing that was handed out
		// at none. The promo set reprints a card once per event it appeared
		// in, all at the main set's number and rarity, and one of those
		// reprints carries no event of its own - that is the one a bare
		// listing names.
		candidates = plain
	}
	if len(candidates) <= 1 {
		return candidates
	}

	// Then the rarity. A wording naming one picks the printings carrying it;
	// a wording naming none keeps the base run, since the plain rarity is
	// what a listing without a qualifier means. Both tiers only narrow -
	// a rarity nothing carries leaves the candidates as they were, so an
	// ambiguity is reported rather than silently resolved.
	named := rarityNamed(b, inCard.Variation, candidates)
	if len(named) > 0 {
		candidates = named
	} else if base := baseRarity(candidates); len(base) > 0 {
		candidates = base
	}
	return candidates
}

// unlabelled keeps the printings carrying no variant label, for the wording
// that described none. It narrows or it does nothing: where every candidate
// is labelled, or none is, there is nothing here to choose on.
func unlabelled(cards []matcher.Card) []matcher.Card {
	var out []matcher.Card
	for _, card := range cards {
		if len(card.PromoTypes) == 0 {
			out = append(out, card)
		}
	}
	if len(out) == len(cards) {
		return nil
	}
	return out
}

// rarityNamed keeps the candidates whose rarity the wording spells, by the
// catalog's own name for it. The comparison is word-bounded on the wording
// as written rather than on a normalized form: normalizing drops the "+"
// that marks a parallel, and "C+" reduced to "c" is a substring of very
// nearly every listing there is.
//
// The longest spelling wins, because the rarity names nest - "Rare" is a
// tail of "Legend Rare", and a listing saying the latter says the former
// too.
func rarityNamed(b *matcher.Backend, variation string, cards []matcher.Card) []matcher.Card {
	if strings.TrimSpace(variation) == "" {
		return nil
	}
	longest := 0
	for _, card := range cards {
		rarity := b.RarityLabel(card.Rarity)
		if matcher.SaysWord(variation, rarity, "+") && len(rarity) > longest {
			longest = len(rarity)
		}
	}
	if longest == 0 {
		return nil
	}
	var out []matcher.Card
	for _, card := range cards {
		rarity := b.RarityLabel(card.Rarity)
		if len(rarity) == longest && matcher.SaysWord(variation, rarity, "+") {
			out = append(out, card)
		}
	}
	// A tier that keeps everything has chosen nothing.
	if len(out) == len(cards) {
		return nil
	}
	return out
}

// baseRarity keeps the printings of the plain run: the rarities that carry
// no parallel marker. It is what a listing saying nothing but the number
// means, the parallels being the printings a storefront names.
func baseRarity(cards []matcher.Card) []matcher.Card {
	var out []matcher.Card
	for _, card := range cards {
		if !strings.HasSuffix(card.Rarity, "+") {
			out = append(out, card)
		}
	}
	if len(out) == len(cards) {
		return nil
	}
	return out
}

// labelLength is how much of a wording a printing's labels account for,
// which is what makes the longest of two nested labels the one meant.
func labelLength(promoTypes []string) int {
	var total int
	for _, promoType := range promoTypes {
		total += len(promoType)
	}
	return total
}

// wordsDescribe reports whether the storefront's wording says every promo
// type the printing carries, each as a run of whole words. The containment
// runs that way round so a wording naming one event cannot answer for a
// printing of another, and it is whole words rather than a substring so a
// note saying "Special" does not name the sp printing, nor "Ahead" the
// head one - which #536 let reach across editions.
func wordsDescribe(wording string, promoTypes []string) bool {
	if len(promoTypes) == 0 {
		return false
	}
	for _, promoType := range promoTypes {
		if !matcher.SlugDescribes(wording, promoType) {
			return false
		}
	}
	return true
}

// extractNumber reads the collector number out of a storefront's wording,
// taking the game's full code where one is written and nothing otherwise: a
// bare number names no set here, every collector number of this game
// carrying the code of the run it belongs to.
func extractNumber(variation string) string {
	for _, field := range strings.Fields(variation) {
		field = strings.Trim(field, "()[],.")
		if fullNumberRe.MatchString(field) {
			return field
		}
	}
	return ""
}
