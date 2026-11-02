package yugioh

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestPromoFlag pins what "is:promo" answers with. Yu-Gi-Oh rarities name
// the foil treatment ("Secret Rare", "Starfoil Rare") and never the
// promotion, so the flag can only come from the set: the 732 Duelist League
// promos carry no promotional rarity between them. The collector tins are
// the trap in the other direction, reprinting at retail rather than handing
// out.
func TestPromoFlag(t *testing.T) {
	b := gametest.Backend(t, matcher.GameYuGiOh)

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
		// The league and magazine cases name a season and a volume: the
		// shelf spanning a whole league or magazine holds only sealed.
		{"Duelist League 13 participation cards", true},
		{"Judge Promotional Cards", true},
		{"Shonen Jump Magazine Promos (JUMP)", true},
		{"McDonald's Promo", true},
		{"2014 Mega-Tins", false},
		{"The Legend of Blue Eyes White Dragon", false},
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
