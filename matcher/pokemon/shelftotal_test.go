package pokemon

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestShelfTotalDoesNotVeto pins the one total a storefront may write that
// the card's face does not carry: the size of the shelf it files the card
// under. A total the shelf could not have produced still names another
// set's printing and still vetoes.
func TestShelfTotalDoesNotVeto(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	for _, tt := range []struct {
		desc string
		in   matcher.InputCard
		want string
	}{
		{"the shelf's card count over a card that keeps its original total", matcher.InputCard{
			Name: "Blastoise", Edition: "Celebrations: Classic Collection", Variation: "2/25"}, "2-102_250319_holofoil"},
		{"the total most of the shelf prints over a subset numbered apart", matcher.InputCard{
			Name: "Unown", Edition: "EX Unseen Forces", Variation: "Z/115"}, "z-28_90193_holofoil"},
		{"the Unown numbered by a mark rather than a letter", matcher.InputCard{
			Name: "Unown", Edition: "EX Unseen Forces", Variation: "?/115"}, "28_90167_holofoil"},
		{"the whole run's figure over one year of it", matcher.InputCard{
			Name: "AZ", Edition: "World Championship Decks", Variation: "91/100 2015 Patrick Martinez"}, "91-119_481163"},
		{"the parent's own card count, without the subset admitted beside it", matcher.InputCard{
			Name: "Luxio", Edition: "SWSH09: Brilliant Stars", Variation: "050/206"}, "050-172_263756"},
		{"the face itself still lands", matcher.InputCard{
			Name: "Blastoise", Edition: "Celebrations: Classic Collection", Variation: "2/102"}, "2-102_250319_holofoil"},
		{"another set's total is not the shelf's and still vetoes", matcher.InputCard{
			Name: "Cascoon", Edition: "Platinum", Variation: "44/130"}, ""},
		{"and the set's own total keeps telling the reprints apart", matcher.InputCard{
			Name: "Cascoon", Edition: "Platinum", Variation: "44/127"}, "44-127_84122"},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			in := tt.in
			id, err := b.Match(&in)
			if tt.want == "" {
				if err == nil {
					t.Fatalf("Match(%v) = %s (%v), want an error", tt.in, id, b.UUIDs[id])
				}
				return
			}
			if err != nil {
				t.Fatalf("Match(%v) = %v", tt.in, err)
			}
			if id != tt.want {
				t.Errorf("Match(%v) = %s (%v), want %s", tt.in, id, b.UUIDs[id], tt.want)
			}
		})
	}
}
