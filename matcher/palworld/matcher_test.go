package palworld

import (
	"encoding/json"
	"flag"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// matchTest mirrors the Riftbound and Lorcana matcher harnesses: an input
// card and the expected Match() verdict, uuid or error string. The seeds
// below pin the contract edges this game turns on - the rarity that tells
// same-numbered printings apart, the shelves storefronts spell the sets
// with, and the names carrying a parenthetical of their own. Run with
// -update-palworld to re-bake the verdicts after a deliberate change; the
// regeneration refuses to flip a case between success and error, so a
// regression cannot be blessed silently.
type matchTest struct {
	Desc string            `json:"description"`
	In   matcher.InputCard `json:"input"`
	ID   string            `json:"uuid,omitempty"`
	Err  string            `json:"error,omitempty"`
}

const palworldTestData = "testdata/palworld_test_data.json"

var updatePalworld = flag.Bool("update-palworld", false,
	"re-run Match over every test input and rewrite "+palworldTestData)

// palworldSeeds are hand-authored cases ensured present by -update-palworld.
// Verdicts are baked by the regeneration rather than typed, so they always
// state what Match actually returns; a "negative:" description declares the
// author's intent and the regeneration fails loudly if the class disagrees.
var palworldSeeds = []matchTest{
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

	data, err := os.ReadFile(palworldTestData)
	if err != nil {
		if *updatePalworld && os.IsNotExist(err) {
			data = []byte("[]")
		} else {
			t.Fatal(err)
		}
	}
	var tests []matchTest
	err = json.Unmarshal(data, &tests)
	if err != nil {
		t.Fatal(err)
	}

	if *updatePalworld {
		regeneratePalworldTestData(t, b, tests)
		return
	}

	if len(tests) == 0 {
		t.Fatal("no Palworld test cases")
	}

	for _, tt := range tests {
		t.Run(tt.Desc, func(t *testing.T) {
			t.Parallel()
			in := tt.In
			id, err := b.Match(&in)
			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if id != tt.ID || gotErr != tt.Err {
				t.Errorf("Match(%q ed=%q num=%q) = (%q, %q), want (%q, %q)",
					tt.In.Name, tt.In.Edition, tt.In.Variation, id, gotErr, tt.ID, tt.Err)
			}
		})
	}
}

// regeneratePalworldTestData re-runs Match over every committed input plus the
// seeds, bakes the verdict, and rewrites the golden file sorted by
// description. A verdict may change in detail - each change is logged - but
// flipping between success and error aborts the rewrite: a change of that
// size has to be acknowledged by editing the entry or the seed by hand.
func regeneratePalworldTestData(t *testing.T, b *matcher.Backend, tests []matchTest) {
	isSeed := map[string]bool{}
	for _, seed := range palworldSeeds {
		isSeed[seed.Desc] = true
	}
	kept := tests[:0]
	for _, tt := range tests {
		if !isSeed[tt.Desc] {
			kept = append(kept, tt)
		}
	}
	tests = append(kept, palworldSeeds...)

	for i := range tests {
		in := tests[i].In
		id, err := b.Match(&in)
		gotErr := ""
		if err != nil {
			gotErr = err.Error()
		}

		wantNegative := tests[i].Err != ""
		if isSeed[tests[i].Desc] {
			wantNegative = strings.HasPrefix(tests[i].Desc, "negative:")
		}
		if wantNegative != (gotErr != "") {
			t.Errorf("refusing to flip %q: (%q, %q) -> (%q, %q); edit the entry or seed by hand",
				tests[i].Desc, tests[i].ID, tests[i].Err, id, gotErr)
			continue
		}

		if tests[i].ID != id || tests[i].Err != gotErr {
			t.Logf("updating %q: (%q, %q) -> (%q, %q)",
				tests[i].Desc, tests[i].ID, tests[i].Err, id, gotErr)
		}
		tests[i].ID = id
		tests[i].Err = gotErr
	}
	if t.Failed() {
		t.Fatal("verdict-class flips detected; golden file left untouched")
	}

	sort.Slice(tests, func(i, j int) bool {
		return tests[i].Desc < tests[j].Desc
	})

	out, err := os.Create(palworldTestData)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	enc := json.NewEncoder(out)
	enc.SetIndent("", "    ")
	err = enc.Encode(tests)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d Palworld test cases to %s", len(tests), palworldTestData)
}
