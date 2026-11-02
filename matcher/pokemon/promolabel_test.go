package pokemon

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestPromoLabelDepth pins that a printing sharing a label with its siblings
// is told from them by the labels it does not share. Four Cosmos Holo
// printings of one number differ only in the retailer that stamped them, so
// the wording naming a retailer has to outrank the wording naming none.
func TestPromoLabelDepth(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	for _, tt := range []struct{ desc, name, edition, variation, want string }{
		{"the retailer is what tells the stampings apart", "Hop's Snorlax", "", "117 GameStop Cosmos Holo", "117-159_626640_holofoil"},
		{"and the other retailer likewise", "Hop's Snorlax", "", "117 EB Games Cosmos Holo", "117-159_629648_holofoil"},
		// A placement is never named by accident, so a wording naming none
		// means the copy wearing none.
		{"no placement named is not the staff copy", "Toxtricity", "ME: Mega Evolution Promo", "017", "017_663193_holofoil"},
		// Naming only the shared label names no one of them, and saying so
		// beats answering with whichever came first.
		{"the shared label alone still aliases", "Hop's Snorlax", "", "117 Cosmos Holo", ""},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			id, err := b.Match(&matcher.InputCard{Name: tt.name, Edition: tt.edition, Variation: tt.variation})
			if id != tt.want {
				t.Errorf("Match(%q) = %q (err %v), want %q", tt.variation, id, err, tt.want)
			}
		})
	}
}
