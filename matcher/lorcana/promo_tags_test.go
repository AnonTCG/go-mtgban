package lorcana

import (
	"slices"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestPromoTagsAreSlugs pins the stored form: the builder's own labels,
// slugged like every other game's. Lorcana writes almost none of this into
// the name, so most of them come from the datastore's promo and finish
// fields; the handful it does write in a parenthesis - "(Errata Version)",
// "(Puzzle Promo)" - the builder reads out of the name for us.
func TestPromoTagsAreSlugs(t *testing.T) {
	b := gametest.Backend(t, matcher.GameLorcana)

	for _, tag := range b.AllPromoTypes {
		if slug := matcher.PromoTypeSlug(tag); slug != tag {
			t.Errorf("declared tag %q is not its own slug (%q)", tag, slug)
		}
	}
	// A source category, a treatment the builder moved over from foilTypes,
	// and a label it read out of a card's name. "highgloss" is deliberately
	// not among them: every Legendary of its sets wears it, so it says what
	// the rarity says and the builder leaves it off.
	for _, tag := range []string{"d23", "organizedplay", "satin", "puzzle"} {
		if !slices.Contains(b.AllPromoTypes, tag) {
			t.Errorf("tag %q is not declared", tag)
		}
	}
	var tagged int
	for _, uuid := range b.AllUUIDs {
		co, err := b.GetUUID(uuid)
		if err == nil && co.HasPromoType("d23") {
			tagged++
		}
	}
	if tagged == 0 {
		t.Error("no printing carries d23, so is:d23 answers with nothing")
	}
}
