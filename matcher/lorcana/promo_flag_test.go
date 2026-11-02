package lorcana

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestIsPromoIsSet pins that the game names its promotional printings at all.
// Upstream publishes no nonPromoId, so reading it would answer false for
// every card in the game - and a promo filter that matches nothing looks
// exactly like a game with no promos in it.
func TestIsPromoIsSet(t *testing.T) {
	b := gametest.Backend(t, matcher.GameLorcana)

	var promos, total, inPromoSet, promoSetFlagged int
	for _, uuid := range b.GetUUIDs() {
		co, err := b.GetUUID(uuid)
		if err != nil || co.Sealed {
			continue
		}
		total++
		if co.IsPromo {
			promos++
		}
		set, err := b.GetSet(co.SetCode)
		if err != nil || set.Type != "promo" {
			continue
		}
		inPromoSet++
		if co.IsPromo {
			promoSetFlagged++
		}
	}
	if promos == 0 {
		t.Fatalf("no printing of %d is promotional, which is what the dropped field said", total)
	}
	// Every rarity the game calls Promo is one, whatever else says so: a
	// minted printing has no upstream entry to carry a field or a
	// relationship, and its rarity is all that is left to read.
	for _, uuid := range b.GetUUIDs() {
		co, err := b.GetUUID(uuid)
		if err != nil || co.Sealed || co.Rarity != "promo" || co.IsPromo {
			continue
		}
		t.Errorf("%s is rarity promo and not flagged: %v", uuid, co)
	}
	// A set that is wholly promotional has no card in it that is not.
	if inPromoSet > 0 && promoSetFlagged != inPromoSet {
		t.Errorf("%d of %d printings in promo-typed sets are flagged", promoSetFlagged, inPromoSet)
	}
	t.Logf("%d of %d printings are promotional, %d of them in promo-typed sets", promos, total, inPromoSet)
}
