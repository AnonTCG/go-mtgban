package pokemon

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestFoilFlagAgreesWithFinish pins that the flag and the finish name say the
// same thing. The flag is what a caller with no finish vocabulary reads - the
// arbitrage filters and the website's foil column among them - and the loader
// set the finish without ever setting the flag, so every holo printing in the
// game was filed as a plain one.
func TestFoilFlagAgreesWithFinish(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	var foil, total, disagree int
	for _, uuid := range b.GetUUIDs() {
		co, err := b.GetUUID(uuid)
		if err != nil || co.Sealed {
			continue
		}
		total++
		if co.Foil {
			foil++
		}
		if co.Foil != matcher.IsFoilFinish(co.Finish) {
			if disagree < 5 {
				t.Errorf("%s is %q with Foil=%v", uuid, co.Finish, co.Foil)
			}
			disagree++
		}
	}
	if disagree > 0 {
		t.Errorf("%d of %d printings disagree with their own finish", disagree, total)
	}
	if foil == 0 {
		t.Errorf("no printing of %d is foil, and the game is mostly holo", total)
	}
	t.Logf("%d of %d printings are foil", foil, total)
}
