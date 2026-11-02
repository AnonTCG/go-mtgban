package yugioh

import (
	"slices"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestOriginalArtworkWording pins Harpie Lady MRD-008, filed twice under one
// name, number and rarity - once original-artwork, once new-art - which Card
// Trader tells apart only by spelling the copyright line at a physical
// card's bottom into the Version it would otherwise use for the rarity. The
// scraper turns that line into the wording below; this is what the wording
// has to reach once it gets here.
func TestOriginalArtworkWording(t *testing.T) {
	b := gametest.Backend(t, matcher.GameYuGiOh)

	in := matcher.InputCard{Name: "Harpie Lady", Edition: "Metal Raiders", Variation: "008 Original Artwork"}
	id, err := b.Match(&in)
	if err != nil {
		t.Fatalf("Match(%v) = %v", in, err)
	}
	co, err := b.GetUUID(id)
	if err != nil {
		t.Fatalf("GetUUID(%s) = %v", id, err)
	}
	if !slices.Contains(co.PromoTypes, "originalartwork") {
		t.Errorf("Match(%v) = %s, promo types %v, want originalartwork", in, id, co.PromoTypes)
	}

	// The bare rarity, wearing neither artwork's word, still names both and
	// stays refused: a listing that never mentions the artwork is not the
	// scraper's business to guess for.
	in = matcher.InputCard{Name: "Harpie Lady", Edition: "Metal Raiders", Variation: "008 Common"}
	if id, err := b.Match(&in); err == nil {
		t.Errorf("Match(%v) = %q, want a refusal between the two prints", in, id)
	}
}
