package fleshandblood

import (
	"flag"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

var updateFleshandblood = flag.Bool("update-fleshandblood", false,
	"rewrite testdata/fleshandblood_test_data.json from what Match returns")

// The seeds cover the storefront shapes the scrapers feed: the canonical
// pitch-color parenthetical beside a full collector number (and its dashed
// and truncated spellings), the treatment and print-run wording that routes
// to its printing's entry without ever gating, cardtrader's marvel letter
// tail and print-run edition suffixes, the fused-card pair numbers, and the
// identifier index the TCGplayer feed resolves through.
var fleshandbloodSeeds = []gametest.Case{
	{
		Desc: "canonical pitch name with the full collector number",
		In:   matcher.InputCard{Name: "Sink Below (Red)", Variation: "WTR215"},
	},
	{
		Desc: "dashed pitch color normalizes onto the parenthetical name",
		In:   matcher.InputCard{Name: "Sink Below - Red", Variation: "WTR215"},
	},
	{
		Desc: "bare truncated name adopts the pitch parenthetical carrying the number",
		In:   matcher.InputCard{Name: "Breakneck Battery", Variation: "WTR011"},
	},
	{
		Desc: "rainbow wording routes to its plainest print run",
		In:   matcher.InputCard{Name: "Sink Below (Red)", Variation: "WTR215 Rainbow Foil", Edition: "Flesh & Blood: Welcome to Rathe Singles"},
	},
	{
		Desc: "cold foil wording picks the cold entry over the rainbow default",
		In:   matcher.InputCard{Name: "Frost Hex", Variation: "UPR126 Cold Foil", Foil: true},
	},
	{
		Desc: "tcgplayer printing name routes both finish axes",
		In:   matcher.InputCard{Name: "Sink Below (Red)", Variation: "WTR215 1st Edition Rainbow Foil", Foil: true},
	},
	{
		Desc: "storefront wording picks the variant it describes",
		In:   matcher.InputCard{Name: "Enigma, New Moon (Marvel)"},
	},
	{
		Desc: "cardtrader marvel letter tail demands the variant",
		In:   matcher.InputCard{Name: "Enigma, New Moon", Variation: "MST238m"},
	},
	{
		Desc: "bare number with the marvel tail still demands the variant",
		In:   matcher.InputCard{Name: "Enigma, New Moon", Variation: "238m"},
	},
	{
		Desc: "letter label beside the treatment wording picks its product",
		In:   matcher.InputCard{Name: "Lightning Flow (A)", Variation: "OMN203 Cold Foil", Foil: true},
	},
	{
		Desc: "treatment wording spelling a label defers to the named variant",
		In:   matcher.InputCard{Name: "Ash // Aether Ashwing (Marvel)", Variation: "UPR043 Cold Foil", Foil: true},
	},
	{
		Desc: "pair collector number resolves the fused card",
		In:   matcher.InputCard{Name: "Anothos // Bravo", Variation: "WTR040 // WTR039"},
	},
	{
		Desc: "front half of a pair number still matches",
		In:   matcher.InputCard{Name: "Anothos // Bravo", Variation: "WTR040"},
	},
	{
		Desc: "cardtrader print-run suffix strips off the edition",
		In:   matcher.InputCard{Name: "Barraging Beatdown (Yellow)", Variation: "18", Edition: "Welcome to Rathe - 1st Edition"},
	},
	{
		Desc: "alpha print run rides the 1st edition entry",
		In:   matcher.InputCard{Name: "Barraging Beatdown (Yellow)", Variation: "18", Edition: "Welcome to Rathe - Alpha Print Run"},
	},
	{
		Desc: "tcgplayer product id resolves through the identifier index",
		In:   matcher.InputCard{ID: "225309"},
	},
	{
		Desc: "the printing named beside the id prices that printing's entry",
		In:   matcher.InputCard{ID: "225015", Finish: "Unlimited Edition Normal"},
	},
	{
		Desc: "a bare treatment reaches the print run the product sold it in",
		In:   matcher.InputCard{ID: "225015", Finish: "Cold Foil"},
	},
	{
		Desc: "negative: a treatment the product was never priced in refuses the sibling",
		In:   matcher.InputCard{ID: "225501", Name: "Fyendal's Spring Tunic", Variation: "FAB001", Finish: "Rainbow Foil"},
	},
	{
		Desc: "negative: unknown card name",
		In:   matcher.InputCard{Name: "Nonexistent Imaginary Hero", Variation: "WTR001"},
	},
	{
		Desc: "negative: known name with wrong collector number",
		In:   matcher.InputCard{Name: "Sink Below (Red)", Variation: "WTR999"},
	},
	{
		Desc: "negative: a name reprinted across sets stays ambiguous unaided",
		In:   matcher.InputCard{Name: "Brutal Assault"},
	},
	// The only extended art of OMN133 wears the words behind a colour, so
	// the storefront naming the treatment names the second tag and nothing
	// else.
	{
		Desc: "a tag written behind another still names its printing",
		In:   matcher.InputCard{Name: "Tome of Quandaries", Variation: "OMN133 Extended Art"},
	},
	// The datastore mirrors the storefront each set was catalogued from,
	// and the storefronts disagree about the pitch parenthetical: the
	// number is what says which printing was meant, in both directions.
	{
		Desc: "pitch qualifier drops when the numbered printing spells it without",
		In:   matcher.InputCard{Name: "Hyper Driver (Red)", Variation: "ARC036"},
	},
	{
		Desc: "pitch qualifier drops for the yellow spelling too",
		In:   matcher.InputCard{Name: "Bittering Thorns (Yellow)", Variation: "CRU072"},
	},
	{
		Desc: "pitch qualifier is adopted when the numbered printing spells it",
		In:   matcher.InputCard{Name: "Bloodrot Trap", Variation: "OUT171"},
	},
	{
		Desc: "marvel qualifier is adopted from the numbered printing",
		In:   matcher.InputCard{Name: "Kassai", Variation: "HVY091"},
	},
	// The re-spelling swaps one parenthetical for another, and the same
	// shape spells both a piece of the name and a treatment label, so what
	// it drops has to survive as wording or the base printing wins the
	// number.
	{
		Desc: "marvel label survives a respelling onto the plain name",
		In:   matcher.InputCard{Name: "Enigma, Ledger of Ancestry (Marvel)", Variation: "MST025"},
	},
	{
		Desc: "marvel label survives a respelling onto a pitch-qualified name",
		In:   matcher.InputCard{Name: "Golden Skull (Marvel)", Variation: "OMN240"},
	},
	{
		Desc: "pitch label survives when the number wears the color as a label",
		In:   matcher.InputCard{Name: "Staunch Response (Red)", Variation: "TNP019"},
	},
	// Cardtrader writes a fused card's faces in the opposite order from the
	// datastore, and its Monarch hero numbers disagree outright; the
	// unordered face set identifies the card, the pair number picks among
	// the spellings sharing it, and a pair no printing wears is replaced by
	// the one they agree on.
	{
		Desc: "reversed fused spelling adopts the datastore face order",
		In:   matcher.InputCard{Name: "Spectral Shield // Soul Shackle", Variation: "MON104//MON186", Edition: "Monarch - Unlimited"},
	},
	{
		Desc: "pair number matches in either face order",
		In:   matcher.InputCard{Name: "Soul Shackle // Spectral Shield", Variation: "MON104//MON186", Edition: "Monarch - Unlimited"},
	},
	{
		Desc: "pair number picks among the spellings sharing the faces",
		In:   matcher.InputCard{Name: "Quicken // Harmonized Kodachi", Variation: "WTR225//WTR078 Unlimited", Edition: "Welcome to Rathe"},
	},
	{
		Desc: "canonical fused spelling keeps its own printing at its number",
		In:   matcher.InputCard{Name: "Quicken // Harmonized Kodachi", Variation: "XXX009 // XXX008"},
	},
	{
		Desc: "a pair no printing wears adopts the number the printings agree on",
		In:   matcher.InputCard{Name: "Prism // Iris of Reality", Variation: "MON220//MON068", Edition: "Monarch - Unlimited"},
	},
	{
		Desc: "doubled fused spelling folds onto its pairing",
		In:   matcher.InputCard{Name: "Gold // Golden Cog // Gold // Golden Cog", Variation: "SEA244//SEA042", Edition: "High Seas"},
	},
	// Two fused claimants refuse a bare face name, but a listing naming a
	// treatment no pair was sold in has said which printing it prices.
	{
		Desc: "a treatment no fused pair wears reaches the label at the number",
		In:   matcher.InputCard{Name: "Cindra", Variation: "HNT055", Finish: "Cold Foil", Foil: true},
	},
	{
		Desc: "the prefix fallback reaches it too",
		In:   matcher.InputCard{Name: "Kayo, Strong-arm", Variation: "SUP064", Finish: "Cold Foil", Foil: true},
	},
	{
		Desc: "negative: a treatment the label never wore refuses all the same",
		In:   matcher.InputCard{Name: "Cindra", Variation: "HNT055", Finish: "Rainbow Foil", Foil: true},
	},
	{
		Desc: "negative: a plain listing of a face still refuses two claimants",
		In:   matcher.InputCard{Name: "Iyslander", Variation: "UPR103", Finish: "Normal"},
	},
	// The datastore spells the promo marvels with both parentheticals, and
	// a name carrying only the pitch one has to be able to grow the label.
	{
		Desc: "a pitch-qualified name grows the treatment label at its number",
		In:   matcher.InputCard{Name: "Wild Ride (Red)", Variation: "TNP001"},
	},
	// The treatment siblings of a number are one name, one number and two
	// products, so the finish the caller names is the only thing that
	// tells them apart - and a finish the product was never sold in names
	// no product at all.
	{
		Desc: "the named finish picks its treatment sibling at the number",
		In:   matcher.InputCard{Name: "Will of Arcana", Edition: "Rosetta", Variation: "ROS000", Finish: "Cold Foil", Foil: true},
	},
	{
		Desc: "the other treatment of the same number picks the other sibling",
		In:   matcher.InputCard{Name: "Will of Arcana", Edition: "Rosetta", Variation: "ROS000", Finish: "Rainbow Foil", Foil: true},
	},
	{
		Desc: "negative: the name path refuses a treatment the product never sold",
		In:   matcher.InputCard{Name: "Fyendal's Spring Tunic", Variation: "FAB001", Finish: "Rainbow Foil", Foil: true},
	},
	{
		Desc: "negative: a bare number never licenses a pitch respelling",
		In:   matcher.InputCard{Name: "Hyper Driver (Red)", Variation: "036"},
	},
	{
		Desc: "negative: a respelling still has to name a printing at the number",
		In:   matcher.InputCard{Name: "Hyper Driver (Red)", Variation: "ARC999"},
	},
}

func TestFleshandbloodMatch(t *testing.T) {
	b := gametest.Backend(t, matcher.GameFleshAndBlood)
	gametest.Replay(t, b, fleshandbloodSeeds, *updateFleshandblood)
}

// TestFleshandbloodSealed pins the sealed namespace: products load, resolve
// by name, and never shadow the card index.
func TestFleshandbloodSealed(t *testing.T) {
	b := gametest.Backend(t, matcher.GameFleshAndBlood)

	if len(b.AllSealedUUIDs) == 0 {
		t.Fatal("no sealed products loaded")
	}
	productMap := b.BuildSealedProductMap("tcgplayerProductId")
	if len(productMap) == 0 {
		t.Fatal("no sealed product ids")
	}
	for id, uuids := range productMap {
		if len(uuids) != 1 {
			t.Errorf("tcgplayer id %d shared by %d sealed products", id, len(uuids))
		}
	}

	uuid, err := b.ResolveSealed("Welcome to Rathe Booster Pack [1st Edition]")
	if err != nil {
		t.Fatalf("bracket-edition booster pack did not resolve: %s", err)
	}
	co, err := b.GetUUID(uuid)
	if err != nil || !co.Sealed {
		t.Fatalf("resolved uuid %s is not a sealed product", uuid)
	}

	// The WTR booster pack is two real products (the 1st Edition and
	// Unlimited Edition print runs); a wording naming neither must stay
	// unresolved rather than pick one.
	if uuid, err := b.ResolveSealed("Welcome to Rathe Booster Pack"); err == nil {
		t.Fatalf("print-run-ambiguous booster pack resolved to %s", uuid)
	}
}
