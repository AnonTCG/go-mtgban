package palworld

import (
	"regexp"
	"slices"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// Rules implements matcher.GameRules for Palworld. A card is identified
// by name + collector number: this game numbers a parallel printing apart
// from the card it parallels, the rarity's code being the number's tail.
// What the rules have to do is read that tail out of a storefront's wording
// where the number was written without it - TCGplayer names the products
// that way too, "Grizzbolt - Rumbling Tank (TSR)" for ETD01-001TSR. Only a
// reprint run, such as BP01's 2nd Edition, shares a number with the
// original, and its promo type tells the two apart.
type Rules struct{ matcher.DefaultRules }

// fullNumberRe matches the game's collector number shapes, the rarity tail
// included: "EBP01-001", "ETD01-001TSR", "EPR-004", and the newer
// hyphenated tails such as "EPR-008-S". Every number the datastore carries
// matches it.
var fullNumberRe = regexp.MustCompile(`^[A-Za-z]+[0-9]*-[0-9]+(?:-?[A-Za-z]+)?$`)

// numberTailRe splits a collector number into the run's number and the
// rarity code it ends in, the tail empty for a plain printing. The separator
// before a tail is optional because the catalog uses both "001SSP" and
// "008-S".
var numberTailRe = regexp.MustCompile(`^([A-Za-z]+[0-9]*-[0-9]+)(?:-?([A-Za-z]+))?$`)

// trailingCodeRe matches the collector number a storefront writes behind a
// name in parentheses of its own ("Grizzbolt - Rumbling Tank (ETD01-001)").
var trailingCodeRe = regexp.MustCompile(`\s*\(([A-Za-z]+[0-9]*-[0-9]+[A-Za-z]*)\)$`)

// rarityTails is every rarity code a number ends in, mapped to the rarity
// the catalog spells it out as. TCGplayer writes the code in parentheses
// after the name as well, so it reaches the matcher from either side.
var rarityTails = map[string]string{
	"SR":  "Super Rare",
	"OSR": "Over Super Rare",
	"SP":  "Super Parallel",
	"SSP": "Super Special Parallel",
	"TSR": "Trial Deck Super Rare",
	"TSP": "Trial Deck Super Parallel",
}

// Prefilter splits the parenthetical decorations off the name before the
// canonical-name lookup. The card names of this game carry a dash of their
// own - every Pal is "Name - Epithet" - so nothing is split on one; only
// the parentheses are read, and only when the whole name is not itself a
// card.
func (Rules) Prefilter(b *matcher.Backend, inCard *matcher.InputCard) {
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; found {
		return
	}
	// The number is the one parenthetical never part of a name, so peel it
	// alone first and leave the name untouched unless what is left is a card.
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
}

// AdjustName provides a prefix fallback for the feeds that truncate a name,
// accepting the head they kept when exactly one canonical name starts with
// it. It only runs once the canonical lookup has failed, so a name that
// resolves on its own is never touched.
func (Rules) AdjustName(b *matcher.Backend, inCard *matcher.InputCard) {
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; found {
		return
	}
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
// the set names the datastore carries. The catalog names a set by its code
// and title both ("BP01: Dawn of Palpagos"), which a storefront usually
// writes one half of. The whole fixup reads only the edition, so it is the
// fixup entire and AdjustEdition delegates to it.
func (Rules) AliasEdition(b *matcher.Backend, edition string) string {
	edition = strings.TrimSpace(edition)
	// An edition already naming a set verbatim needs no normalization, and
	// must not be trimmed out of matching: the promo set is named "Palworld
	// Promo Cards" and would lose its heading below.
	for _, set := range b.Sets {
		if matcher.Equals(set.Name, edition) {
			return edition
		}
	}
	for _, prefix := range []string{"Palworld OFFICIAL CARD GAME", "Palworld Official Card Game", "Palworld"} {
		if strings.HasPrefix(edition, prefix) {
			edition = strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(edition, prefix), ":-"))
			break
		}
	}
	edition = strings.TrimSpace(strings.TrimSuffix(edition, "Singles"))
	// A heading naming a set's code alone reaches the set that carries it,
	// the datastore keying its sets by that code.
	set, found := b.Sets[strings.ToUpper(edition)]
	if found {
		return set.Name
	}
	if edition != "" && (matcher.IsPromoHeading(edition) || endsInPromo(edition)) {
		edition = "Promos"
	}
	return edition
}

// AdjustEdition normalizes the edition a storefront published toward a set
// name; the fixup reads nothing but the edition, so the alias is all of it.
func (Rules) AdjustEdition(b *matcher.Backend, inCard *matcher.InputCard) {
	inCard.Edition = (Rules{}).AliasEdition(b, inCard.Edition)
}

// endsInPromo reports whether a heading ends in the game's own word for a
// promotional run, which is what a storefront writes where the datastore
// says "Palworld Promo Cards".
func endsInPromo(edition string) bool {
	lower := strings.ToLower(strings.TrimSpace(edition))
	return strings.HasSuffix(lower, "promo") || strings.HasSuffix(lower, "promos") ||
		strings.HasSuffix(lower, "promo cards")
}

// plainNumberTail are the treatment codes a number is suffixed with.
const plainNumberTail = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// PlainNumber implements matcher.GameRules. The letters behind a number
// name the treatment rather than number it - EBP01-001OSR, EBP01-001SSP and
// EPR-008-S are the cards whose base number is the part before the tail - so
// the number they carry is the plain one.
func (Rules) PlainNumber(number string) string {
	if plain, _ := splitNumber(number); plain != "" {
		return plain
	}
	return strings.TrimRight(number, plainNumberTail)
}

// palworldPrototypeSet is the set code for the catalog's fifteen bare-named
// Prototype cards. Five of those names - Primitive Furnace, Mounted Machine
// Gun, Pump-Action Shotgun, Single-Shot Rifle and Soul - also exist,
// numbered, in BP01 or TD01, so a wording naming no number needs the set
// settled explicitly rather than by name alone.
const palworldPrototypeSet = "PW-PC"

// FilterCards narrows candidates by edition and collector number. The
// number's tail is the rarity's code and the whole of what tells a parallel
// from the card it parallels, so the run's number narrows first and the
// tail chooses: written onto the number ("ETD01-001TSR"), written beside it
// ("ETD01-001 TSR"), or spelled out as the rarity ("Trial Deck Super Rare").
// A wording naming no tail means the plain card.
func (Rules) FilterCards(b *matcher.Backend, inCard *matcher.InputCard, sets []matcher.CandidateSet) []matcher.Card {
	number := extractNumber(inCard.Variation)

	var candidates []matcher.Card
	for _, set := range sets {
		candidates = append(candidates, set.Cards...)
	}

	if number == "" {
		// A wording naming no number but saying "Prototype" settles the
		// set outright, deciding the five of these names that also exist,
		// numbered, elsewhere.
		if matcher.SaysWord(inCard.Variation, "Prototype", "") {
			return withSet(candidates, palworldPrototypeSet)
		}
		return withPromoTypes(b, candidates, inCard.Variation)
	}

	run, writtenTail := splitNumber(number)
	var sameRun []matcher.Card
	for _, card := range candidates {
		if cardRun, _ := splitNumber(card.Number); cardRun != "" && strings.EqualFold(run, cardRun) {
			sameRun = append(sameRun, card)
		}
	}
	if len(sameRun) == 0 {
		return nil
	}
	candidates = sameRun
	if len(candidates) == 1 {
		return withPromoTypes(b, candidates, inCard.Variation)
	}

	// A tail written onto the number says which printing outright.
	// Otherwise the wording may name one beside the number, by the code or
	// by the rarity it stands for, and saying nothing means the plain card:
	// the parallels are the printings a storefront names.
	tail := writtenTail
	if tail == "" {
		tail = tailSaid(inCard.Variation)
	}
	return withPromoTypes(b, withTail(candidates, tail), inCard.Variation)
}

// withPromoTypes keeps the printings wearing a promo type the wording names,
// and nothing where no candidate wears it, as withTail treats a tail: a
// "2nd Edition" listing is not the original's to price. A wording naming
// none keeps the printings wearing none, a reprint run being the printing a
// storefront names, and leaves alone a candidate set it settles neither way.
func withPromoTypes(b *matcher.Backend, cards []matcher.Card, wording string) []matcher.Card {
	var said []string
	for _, promoType := range b.AllPromoTypes {
		if matcher.SlugDescribes(wording, promoType) {
			said = append(said, promoType)
		}
	}
	var out []matcher.Card
	for _, card := range cards {
		if len(said) > 0 && slices.ContainsFunc(said, func(promoType string) bool { return slices.Contains(card.PromoTypes, promoType) }) {
			out = append(out, card)
		} else if len(said) == 0 && len(card.PromoTypes) == 0 {
			out = append(out, card)
		}
	}
	if len(said) == 0 && len(out) == 0 {
		return cards
	}
	return out
}

// withTail keeps the candidates whose number ends in the given rarity code,
// the empty code naming the plain printings. It returns nothing rather than
// everything when no candidate carries the tail, so a code that names no
// printing of this run is a miss rather than a wrong answer.
func withTail(cards []matcher.Card, tail string) []matcher.Card {
	var out []matcher.Card
	for _, card := range cards {
		if _, cardTail := splitNumber(card.Number); strings.EqualFold(cardTail, tail) {
			out = append(out, card)
		}
	}
	return out
}

// withSet keeps the candidates printed in the given set.
func withSet(cards []matcher.Card, setCode string) []matcher.Card {
	var out []matcher.Card
	for _, card := range cards {
		if card.SetCode == setCode {
			out = append(out, card)
		}
	}
	return out
}

// tailSaid reads the rarity code a wording names beside the number, by the
// code itself or by the words it stands for, and answers "" when it names
// none. The comparison is word-bounded on the wording as written: "SR" is a
// substring of "TSR" and of any number ending in one, so a normalized
// contains would read every trial-deck listing as a Super Rare.
//
// The spelled rarities contain one another as words do: "Over Super Rare"
// says "Super Rare" and "Trial Deck Super Parallel" says "Super Parallel",
// and a wording naming the longer has not named the shorter, so the longest
// spelling said wins, the way Gundam reads its rarities. A wording naming
// two codes neither contains names neither, there being no saying which
// was meant.
func tailSaid(variation string) string {
	if strings.TrimSpace(variation) == "" {
		return ""
	}
	// Which codes the wording said, and whether by spelling the rarity
	// out rather than by the code itself: only a spelling can sit inside
	// a longer one.
	spelled := map[string]bool{}
	for tail, rarity := range rarityTails {
		if matcher.SaysWord(variation, tail, "") {
			spelled[tail] = false
		} else if matcher.SaysWord(variation, rarity, "") {
			spelled[tail] = true
		}
	}
	var found string
	for tail, bySpelling := range spelled {
		inside := false
		for other := range spelled {
			if other != tail && bySpelling && strings.Contains(rarityTails[other], rarityTails[tail]) {
				inside = true
			}
		}
		if inside {
			continue
		}
		if found != "" && found != tail {
			return ""
		}
		found = tail
	}
	return found
}

// splitNumber cuts a collector number into the run's number and the rarity
// code it ends in. A number the game does not shape this way - the one
// printing carrying none - answers with two empty strings.
func splitNumber(number string) (string, string) {
	m := numberTailRe.FindStringSubmatch(number)
	if m == nil {
		return "", ""
	}
	return m[1], m[2]
}

// extractNumber reads the collector number out of a storefront's wording,
// taking the game's full code where one is written and nothing otherwise: a
// bare number names no set here, every collector number carrying the code
// of the run it belongs to.
func extractNumber(variation string) string {
	for _, field := range strings.Fields(variation) {
		field = strings.Trim(field, "()[],.")
		if fullNumberRe.MatchString(field) {
			return field
		}
	}
	return ""
}
