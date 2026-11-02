package pokemon

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestSetSymbol pins representative marks a set's cards print. The builder
// carries these from whatever upstream source currently has them. Not every
// set has one, so anything rendering these has to be ready to draw the set
// code instead: a missing symbol is a state to handle, not a gap to fill.
//
// The URL is opaque to this package. The source and URL shape belong to the
// datastore builder, and may change without changing the loader contract.
func TestSetSymbol(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	for _, tt := range []struct {
		code string
		want string
	}{
		{"JU", "https://assets.tcgdex.net/en/base/base2/symbol.png"},
		{"N1", "https://assets.tcgdex.net/en/neo/neo1/symbol.png"},
		{"PRC", "https://assets.tcgdex.net/en/xy/xy5/symbol.png"},
		{"SVI", "https://assets.tcgdex.net/en/sv/sv01/symbol.png"},
	} {
		set, found := b.Sets[tt.code]
		if !found {
			t.Errorf("%s is not a set", tt.code)
			continue
		}
		if set.Symbol != tt.want {
			t.Errorf("%s: Symbol is %q, want %q", tt.code, set.Symbol, tt.want)
		}
	}

}
