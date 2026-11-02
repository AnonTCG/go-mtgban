package pokemon

import (
	"errors"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestOversizedListingHeldToTheJumbo pins what a listing saying oversized is
// held to: the Jumbo Cards printing at its number, past the ordinary card of
// the set its shelf names and past a pooled shelf, and nothing at all when
// the catalog has no jumbo at that number, none wearing the placement the
// listing names, or the listing carries no number.
func TestOversizedListingHeldToTheJumbo(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	for _, tt := range []struct {
		desc string
		in   matcher.InputCard
		want string
	}{
		{"the jumbo, not the ordinary card", matcher.InputCard{
			Name: "Koraidon ex", Edition: "Scarlet & Violet", Variation: "125 Jumbo Oversized | 125"}, "125-198_475654_holofoil"},
		{"past a pooled shelf", matcher.InputCard{
			Name: "Salamence ex", Edition: "Theme Deck & Blisters Exclusives", Variation: "114 Jumbo Oversized | 114/159"}, "114-159_662000_holofoil"},
		{"a letter hung off the number", matcher.InputCard{
			Name: "Darkrai & Cresselia Legend", Edition: "Jumbo Cards", Variation: "099Z Single Oversized Promo"}, "099-102-100-102_211448_holofoil"},
		{"no jumbo at the number", matcher.InputCard{
			Name: "Raging Bolt ex", Edition: "SV Black Star Promos", Variation: "145 Jumbo Oversized | 145"}, ""},
		{"a placement no jumbo wears", matcher.InputCard{
			Name: "Metal Energy", Edition: "SM Black Star Promos", Variation: "043 Jumbo Oversized | 094 Winner"}, ""},
		{"no number to tell two jumbos apart", matcher.InputCard{
			Name: "Pikachu", Edition: "XY Promos", Variation: "XY-P Jumbo Oversized | XY-P"}, ""},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			in := tt.in
			got, err := b.Match(&in)
			if tt.want == "" {
				if !errors.Is(err, matcher.ErrUnsupported) {
					t.Errorf("Match(%+v) = %q, %v, want %v", tt.in, got, err, matcher.ErrUnsupported)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("Match(%+v) = %q, %v, want %s", tt.in, got, err, tt.want)
			}
		})
	}
}
