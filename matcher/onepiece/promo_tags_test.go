package onepiece

import (
	"slices"
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestPromoTagsAreSlugs pins the stored form for the game that needs it
// most: every One Piece card is named after a character, so the qualifier is
// the only thing that says which printing, and there are 464 of them.
func TestPromoTagsAreSlugs(t *testing.T) {
	b := gametest.Backend(t, matcher.GameOnePiece)

	for _, tag := range b.AllPromoTypes {
		if slug := matcher.PromoTypeSlug(tag); slug != tag {
			t.Errorf("declared tag %q is not its own slug (%q)", tag, slug)
		}
	}
	for _, tag := range []string{"alternateart", "parallel", "manga"} {
		if !slices.Contains(b.AllPromoTypes, tag) {
			t.Errorf("tag %q is not declared", tag)
		}
	}

	// A printing is reachable by the event that issued
	// it, whichever way the datastore spells that event. One publishing a
	// shelf's whole product name files it under a single long tag,
	// "premiumcardcollectionbestselectionvol6"; one that takes the
	// instalment off files it under the collection with "Vol. 6" beside it
	// as the mark. The tag is not the same in the two, and the event it
	// reads back as is.
	const issuedBy = "premium card collection"
	var hits int
	for _, uuid := range b.AllUUIDs {
		co, err := b.GetUUID(uuid)
		if err != nil {
			continue
		}
		for _, tag := range co.PromoTypes {
			if strings.Contains(strings.ToLower(b.PromoTypeLabel(tag)), issuedBy) {
				hits++
				break
			}
		}
	}
	if hits == 0 {
		t.Error("no printing is reachable by the Premium Card Collection that issued it")
	}
}
