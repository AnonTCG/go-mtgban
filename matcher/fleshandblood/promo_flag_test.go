package fleshandblood

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestPromoFlag pins what "is:promo" answers with. Two things say a set is
// promotional and they cover different ground: TCGplayer names one group
// outright, and the welcome deck hands its cards out without saying so,
// which the products' own rarity records.
func TestPromoFlag(t *testing.T) {
	b := gametest.Backend(t, matcher.GameFleshAndBlood)

	byName := map[string]bool{}
	for _, co := range b.UUIDs {
		// Sealed product hangs off the same set and is never a promo
		// printing, so reading it would answer for the cards.
		if co.Sealed {
			continue
		}
		set := b.Sets[co.SetCode]
		if set == nil {
			continue
		}
		byName[set.Name] = co.IsPromo
	}

	for _, tt := range []struct {
		set  string
		want bool
	}{
		{"Flesh and Blood: Promo Cards", true},
		{"Welcome Deck: Ira", true},
		{"Welcome to Rathe", false},
		{"Rosetta", false},
	} {
		got, found := byName[tt.set]
		if !found {
			t.Errorf("set %q carries no printing", tt.set)
			continue
		}
		if got != tt.want {
			t.Errorf("set %q is:promo = %v, want %v", tt.set, got, tt.want)
		}
	}
}
