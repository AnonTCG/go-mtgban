package onepiece

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestPromoFlag pins what "is:promo" answers with. The datastore types the
// sets it knows to be promotional and the name answers where it says
// nothing, so both paths are pinned here; the Premium Booster sets are the
// trap, their codes beginning "PRB" while they are an ordinary product.
func TestPromoFlag(t *testing.T) {
	b := gametest.Backend(t, matcher.GameOnePiece)

	for _, tt := range []struct {
		name string
		kind string
		want bool
	}{
		{"One Piece Promotion Cards", "promo", true},
		{"Paramount War Pre-Release Cards", "promo", true},
		// The type answers for a set whose name says nothing.
		{"Royal Blood Release Event Cards", "promo", true},
		// And the name still answers for a datastore that types nothing.
		{"One Piece Promotion Cards", "", true},
		{"Paramount War Pre-Release Cards", "", true},
		{"Super Pre-Release Starter Deck 1: Straw Hat Crew", "", true},
		{"Premium Booster -The Best-", "", false},
		{"Extra Booster: Memorial Collection", "", false},
		{"Starter Deck 31: RED Monkey.D.Luffy", "", false},
	} {
		set := &matcher.Set{Name: tt.name, Type: tt.kind}
		if got := setIsPromotional(set); got != tt.want {
			t.Errorf("setIsPromotional(%q, type %q) = %v, want %v", tt.name, tt.kind, got, tt.want)
		}
	}
	if setIsPromotional(nil) {
		t.Error("setIsPromotional(nil) is true; a set the backend does not hold is not promotional")
	}

	// A promo Nami no bare name reaches is flagged promotional too.
	uuids, err := b.SearchEquals("Nami (Premium Card Collection -Best Selection Vol. 6-)")
	if err != nil {
		t.Fatal(err)
	}
	for _, uuid := range uuids {
		co, err := b.GetUUID(uuid)
		if err != nil {
			t.Fatal(err)
		}
		if !co.IsPromo {
			t.Errorf("%s %s is in %q and is not flagged promotional", co.SetCode, co.Number, co.Edition)
		}
	}

	var promo int
	for _, uuid := range b.AllUUIDs {
		co, err := b.GetUUID(uuid)
		if err == nil && co.IsPromo {
			promo++
		}
	}
	if promo == 0 {
		t.Error("no printing is flagged promotional, so is:promo answers with nothing")
	}
}
