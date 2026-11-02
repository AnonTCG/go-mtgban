package gundam

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestPromoTypeLabels pins that every token the loader declares reads back
// as words. A row for a token no datastore declares is not checked: nothing
// reads it, and TCGplayer retiring a promo is no reason for this to fail.
func TestPromoTypeLabels(t *testing.T) {
	b := gametest.Backend(t, matcher.GameGundam)

	for _, tag := range b.AllPromoTypes {
		if b.PromoTypeLabel(tag) == "" {
			t.Errorf("tag %q reads back as nothing", tag)
		}
	}
}
