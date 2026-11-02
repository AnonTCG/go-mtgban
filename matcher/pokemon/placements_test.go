package pokemon

import (
	"errors"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestLabelsBeforeMarks pins which copy a wording names when it names a
// label beside a mark, a number, or another label. Every case is a product
// the catalog sells under its own TCGplayer id, and the id is what the
// answer is graded by.
func TestLabelsBeforeMarks(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	for _, tt := range []struct {
		desc string
		in   matcher.InputCard
		want string
	}{
		{"a placement outranks the year the plain copy is marked with", matcher.InputCard{
			Name: "Champions Festival", Edition: "XY Promos", Variation: "XY91 2015 Quarter Finalist"},
			"xy91_199317"},
		{"a placement wearing the year too is still the placement", matcher.InputCard{
			Name: "Champions Festival - XY27", Edition: "XY Promos", Variation: "XY27 2014 Staff"},
			"xy27_96485_holofoil"},
		{"a placement spelled out is the figure its label carries", matcher.InputCard{
			Name: "Champions Festival - BW95 [Top Thirty-Two]", Edition: "Black and White Promos", Variation: "BW95 Worlds 13"},
			"bw95_96567"},
		{"a label is named by its words with a year between them", matcher.InputCard{
			Name: "Fighting Energy", Edition: "League & Championship Cards", Variation: "WotC 2002 League Promo"},
			"125568_holofoil"},
		{"a label named keeps the copy the number tier would drop", matcher.InputCard{
			Name: "Leafeon - 11/116", Edition: "League & Championship Cards", Variation: "11 Regional Championship Promo"},
			"11_183200_reverseholofoil"},
		{"the mark named most fully is the one meant", matcher.InputCard{
			Name: "Code Card - Legends of Galar Tin [Zacian V]", Edition: "SWSH02: Rebel Clash", Variation: "International Version"},
			"279260"},
		{"a wording writing no number means the unnumbered twin", matcher.InputCard{
			Name: "Metal Energy - 2008", Edition: "World Championship Decks", Variation: "Tristan Robinson"},
			"479797"},
		{"a stamp names the stamped copy before its mark or label", matcher.InputCard{
			Name: "Archaludon - 107/142", Edition: "Miscellaneous Cards & Products", Variation: "107/142 Cosmos Holo Stellar Crown Stamp"},
			"107-142_588382_holofoil"},
		{"an accent does not hide a label", matcher.InputCard{
			Name: "Magneton - 159", Edition: "SV: Scarlet & Violet Promo Cards", Variation: "159 Pokémon Center Exclusive"},
			"159_594468_holofoil"},
		{"two words opening a label name it", matcher.InputCard{
			Name: "Flutter Mane - 097", Edition: "SV: Scarlet & Violet Promo Cards", Variation: "097 Pokemon Center"},
			"097_543947_holofoil"},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			in := tt.in
			got, err := b.Match(&in)
			if err != nil {
				t.Fatalf("Match(%+v) = %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("Match(%+v) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

// TestPlacementNoSurvivorWears pins the refusal: a wording naming a
// placement prices a copy the plain printing is not, so where the copies
// the number reaches wear none of it the plain one must not answer. The
// main set sells this card only plain.
func TestPlacementNoSurvivorWears(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	in := matcher.InputCard{
		Name: "Professor Turo's Scenario - 171/182", Edition: "SV04: Paradox Rift", Variation: "171/182 Staff"}
	got, err := b.Match(&in)
	if err == nil {
		t.Fatalf("Match(%+v) = %s, want a refusal", in, got)
	}
	if !errors.Is(err, matcher.ErrCardWrongVariant) {
		t.Errorf("Match(%+v) = %v, want %v", in, err, matcher.ErrCardWrongVariant)
	}
}
