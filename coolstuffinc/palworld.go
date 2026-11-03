package coolstuffinc

import (
	"regexp"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// palworldNumberTail is a collector number with a rarity code written onto
// its end, "EBP01-025RR".
var palworldNumberTail = regexp.MustCompile(`^([A-Za-z]+[0-9]*-[0-9]+)([A-Za-z]+)$`)

// palworldBaseRarities are the rarity codes this game numbers no printing
// apart by, which is the whole of what the storefront may have glued to a
// number: the codes left out are the six the catalog files a collector
// number of its own under - OSR, SP, SR, SSP, TSP, TSR - and a number
// ending in one of those names that printing rather than restating the
// card's rarity, so the two must not be pulled apart.
//
// The list is the rarity facet this storefront publishes for the game,
// minus those six. It is enumerated rather than inverted so that a tier
// added to a later set is left alone: a code nobody here has heard of is a
// number's own tail until the catalog says otherwise, and reading it as a
// rarity would answer the base printing for a parallel.
var palworldBaseRarities = map[string]bool{
	"C":   true,
	"PO":  true,
	"PR":  true,
	"R":   true,
	"RR":  true,
	"SSS": true,
	"TD":  true,
	"TDR": true,
	"U":   true,
}

// palworldNoteFixes corrects a Notes field this storefront has typed with
// the wrong collector number for one listing: Chillet's Demo Caravan Promo
// carries "ESOUL-000", a real but different, non-foil Soul promo, where the
// listing itself sells foil and CSI's own product image is ESOUL003. CSI
// has worded the art both ways round.
var palworldNoteFixes = strings.NewReplacer(
	"ESOUL-000 PR Long Blue Creature in Pool Ring", "ESOUL-003 PR",
	"ESOUL-000 PR Blue Long Creature in Pool Ring", "ESOUL-003 PR",
)

// palworldNotes writes a note's collector number and the rarity code this
// storefront sometimes glues to it back apart.
//
// The glue is this storefront's alone, and not even consistently its own:
// it files Victor's Strategy at "EBP01-046 RR" and Chillet at
// "EBP01-025RR" on the same shelf. Nobody else writes the code into the
// number either - Bushiroad's own card list numbers BP01-025 and carries
// RR in a rarity field, CardTrader splits the same two into
// collector_number and palworld_rarity, and TCGplayer, which the catalog
// takes its identity from, only ever suffixes the parallel tiers.
func palworldNotes(notes string) string {
	notes = palworldNoteFixes.Replace(notes)
	fields := strings.Fields(notes)
	for i, field := range fields {
		m := palworldNumberTail.FindStringSubmatch(field)
		if m == nil || !palworldBaseRarities[strings.ToUpper(m[2])] {
			continue
		}
		fields[i] = m[1] + " " + m[2]
		return strings.Join(fields, " ")
	}
	return notes
}

// palworldSpellings corrects the Palworld names this storefront misspells.
// The storefront's own shelves are what say it is a slip rather than a
// name: it sells EBP01-032 as "Fuack - Manic Wave Ripper" and the promo of
// that same Pal as "Fuak", where Bushiroad's list and the catalog both
// spell it Fuack.
//
// A replacer rather than a lookup because the name reaches here decorated -
// the storefront hangs the pack a promo came out of behind it - and spelled
// out rather than found by nearest match, for the reason csiSpellings
// gives.
var palworldSpellings = strings.NewReplacer(
	"Fuak - Manic Wave Ripper", "Fuack - Manic Wave Ripper",
	// The catalog names every Soul promo "Soul" alone and, where it names
	// the Pal at all, carries it as a variant of that card rather than
	// part of the name; Chillet's is the one listing this storefront
	// names epithet-style.
	"Soul - Chillet (Demo Caravan Promo)", "Soul (Demo Caravan Promo)",
	"Soul - Dawn of Palpagos G/P", "Soul",
	"Soul - Dawn of Palpagos R/B", "Soul",
	"Soul - Dawn of Palpagos", "Soul",
	"Soul - Palworld Logo", "Soul",
)

// palworldPrototypeSuffix is the tail this storefront appends to the
// Prototype cards it files on its "Souls & Misc." shelf - the same shelf
// also carries a plain "Soul (TD)" listing with no suffix. The catalog
// holds the fifteen Prototype printings unnumbered, under their bare
// names, in one set of their own - so the suffix names nothing the
// catalog also names and has to come off before the name can be looked
// up.
const palworldPrototypeSuffix = " - Prototype"

// palworldName spells a Palworld card the way the catalog does, where this
// storefront has typed it wrong or decorated it with wording of its own.
func palworldName(name string) string {
	name = palworldSpellings.Replace(name)
	return strings.TrimSuffix(name, palworldPrototypeSuffix)
}

// retailPalworld reads an offer's note on its own: Palworld numbers a
// parallel apart from the card it parallels, so the note names one printing,
// once the rarity code this storefront sometimes types onto the number is
// written back beside it.
func retailPalworld(csi *Coolstuffinc, l sellListing) (*matcher.InputCard, string, error) {
	theCard := &matcher.InputCard{Name: palworldName(l.name), Edition: l.edition, Variation: palworldNotes(l.notes), Foil: l.foil}
	cardID, err := l.match(csi.backend, theCard)
	return theCard, cardID, err
}

// buylistPalworld reads a buylist row's number, the rarity riding in its own
// tail, so the plain reading names one printing and nothing has to be read
// out of the wording. The feed sometimes leaves the number empty and writes
// it in the note, and it is the same hands typing the sell listings that
// glue the rarity onto one.
func buylistPalworld(csi *Coolstuffinc, p CSIPriceEntry) (*matcher.InputCard, string, error) {
	theCard := &matcher.InputCard{Name: palworldName(p.Name), Edition: p.ItemSet, Variation: palworldNotes(buylistVariation(p)), Foil: p.IsFoil == 1}
	cardID, err := csi.backend.Match(theCard)
	return theCard, cardID, err
}
