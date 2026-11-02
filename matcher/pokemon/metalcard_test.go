package pokemon

import (
	"errors"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestMetalCardListingHeldToTheMetalPrinting pins what a listing naming a
// metal card is held to: the metal printing at its number, past the ordinary
// card of the set its shelf names, the one metal printing when it gives no
// number, and nothing at all when the catalog sells no metal printing of the
// card.
func TestMetalCardListingHeldToTheMetalPrinting(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	for _, tt := range []struct {
		desc string
		in   matcher.InputCard
		want string
	}{
		{"the metal card, not the set's own", matcher.InputCard{
			Name: "Arceus V", Edition: "Brilliant Stars", Variation: "122/172 Metal Card"}, "122-172_454372_holofoil"},
		{"not the Classic Collection reprint", matcher.InputCard{
			Name: "Charizard", Edition: "Celebrations", Variation: "4/102 Metal Card"}, "004-102_252517_holofoil"},
		{"no number, one metal card", matcher.InputCard{
			Name: "Pikachu", Edition: "Celebrations", Variation: "Metal Card"}, "058-102_252516_holofoil"},
		{"already on its set's shelf", matcher.InputCard{
			Name: "Mew ex", Edition: "Scarlet & Violet 151", Variation: "205 Metal Card"}, "205-165_519481"},
		{"not the Mew ex its set's name numbers", matcher.InputCard{
			Name: "Mew ex", Edition: "SV: Scarlet & Violet 151", Variation: "205 151 Metal Card"}, "205-165_519481"},
		{"a letter hung off the number", matcher.InputCard{
			Name: "Mew ex", Edition: "SV: Scarlet & Violet 151", Variation: "205Z 151 Metal Card"}, "205-165_519481"},
		{"no metal printing sold", matcher.InputCard{
			Name: "Charizard ex", Edition: "Scarlet & Violet 151", Variation: "199 Metal Card"}, ""},
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
