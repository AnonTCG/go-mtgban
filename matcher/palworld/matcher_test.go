package palworld

import (
	"flag"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

var updatePalworld = flag.Bool("update-palworld", false,
	"rewrite testdata/palworld_test_data.json from what Match returns")

// palworldSeeds pin the contract edges this game turns on - the rarity that
// tells same-numbered printings apart, the shelves storefronts spell the sets
// with, and the names carrying a parenthetical of their own.
var palworldSeeds = []gametest.Case{
	{
		// A reprint run shares the number of the card it reprints, and
		// only the wording naming the run reaches it.
		Desc: "a 2nd Edition reprint is the printing its run names",
		In:   matcher.InputCard{Name: "Rooby - Brave Sparks", Variation: "EBP01-011 2nd Edition"},
	},
	{
		Desc: "and so it is where the run rides in the name",
		In:   matcher.InputCard{Name: "Rooby - Brave Sparks (2nd Edition)", Variation: "EBP01-011"},
	},
	{
		Desc: "and beside a parallel's tail",
		In:   matcher.InputCard{Name: "Suzaku - Hellfire Wings", Variation: "EBP01-002SP 2nd Edition"},
	},
	{
		// A run named on a card never reprinted in it names no printing,
		// rather than the original's
		Desc: "negative: a 2nd Edition of a card printed in none",
		In:   matcher.InputCard{Name: "Jolthog - Shocking Yet Innocent", Variation: "ETD01-002 2nd Edition"},
	},
	{
		// This game numbers a parallel apart from the card it parallels,
		// the rarity riding in the number's own tail, so the number names
		// one printing on its own.
		Desc: "the rarity tail is part of the number",
		In:   matcher.InputCard{Name: "Grizzbolt - Rumbling Tank", Variation: "ETD01-001TSR"},
	},
	{
		Desc: "the plain number is a printing of its own",
		In:   matcher.InputCard{Name: "Grizzbolt - Rumbling Tank", Variation: "ETD01-001"},
	},
	{
		// TCGplayer writes the rarity code in parentheses after the name as
		// well as in the number, so it reaches the rules from either side.
		Desc: "the rarity code written after the name",
		In:   matcher.InputCard{Name: "Grizzbolt - Rumbling Tank (TSR)", Variation: "ETD01-001"},
	},
	{
		Desc: "a super special parallel names its own printing",
		In:   matcher.InputCard{Name: "Jormuntide Ignis - Savage Lava Dragon", Variation: "EBP01-001SSP"},
	},
	{
		// Numbers are unique across the game, so one answers without a set.
		Desc: "a number answers without an edition",
		In:   matcher.InputCard{Name: "Suzaku - Hellfire Wings", Variation: "EBP01-002"},
	},
	{
		// The same card at the same base number in three rarities, each
		// its own printing rather than a treatment of one.
		Desc: "the over super rare of that same number",
		In:   matcher.InputCard{Name: "Suzaku - Hellfire Wings", Variation: "EBP01-002OSR"},
	},
	{
		Desc: "and its super parallel",
		In:   matcher.InputCard{Name: "Suzaku - Hellfire Wings", Variation: "EBP01-002SP"},
	},
	{
		// The catalog holds this name in its own Prototype set only, so
		// "Prototype" answers it even with no number at all.
		Desc: "a wording naming no number, saying Prototype instead",
		In:   matcher.InputCard{Name: "Cattiva", Variation: "Prototype"},
	},
	{
		// This name also has a numbered TD01 printing; "Prototype" is what
		// tells the two apart when neither carries a number.
		Desc: "Prototype picks the Prototype set over a same-named TD01 card",
		In:   matcher.InputCard{Name: "Soul", Variation: "Prototype"},
	},
	{
		Desc: "negative: a number belonging to no set of this game",
		In:   matcher.InputCard{Name: "Grizzbolt - Rumbling Tank", Variation: "ZZ99-999"},
	},
	{
		Desc: "negative: a name the catalog does not carry",
		In:   matcher.InputCard{Name: "Not A Palworld Card At All", Variation: "EBP01-001"},
	},
}

func TestPalworldMatch(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePalworld)
	gametest.Replay(t, b, palworldSeeds, *updatePalworld)
}
