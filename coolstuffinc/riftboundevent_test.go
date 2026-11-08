package coolstuffinc

import (
	"testing"

	"github.com/mtgban/go-mtgban/matcher"

	_ "github.com/mtgban/go-mtgban/matcher/games"
)

// TestRiftboundShelfFollowsTheEventBracket pins the promos whose name carries
// the event that handed them out: the promo shelf holds a printing of its own
// at the same number, so the listing has to be asked on the event shelf.
func TestRiftboundShelfFollowsTheEventBracket(t *testing.T) {
	b := readGameDatastore(t, "riftbound", "RIFTBOUND_PATH")

	for _, tt := range []struct {
		name, notes, wantSet string
	}{
		{"Jinx - Rebel - (Origins Stamp)", "Gold Origins Stamp", "OPP"},
		{"Riven - Shattered (Prerelease)", "", "OPP"},
		{"Jinx - Rebel - (Origins Stamp)", "OGN-202", "OPP"},
		{"Lux - Iluminated (Organized Play Promo)", "006 - Organized Play Promo", "OPP"},
		{"Guardian Angel (Organized Play Promo)", "051 - Organized Play Promo", "OPP"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			name := riftboundSpellings.Replace(tt.name)
			shelf := riftboundShelf(b, "Promo", tt.notes, name, tt.notes, true)
			card := &matcher.InputCard{Name: name, Edition: shelf, Variation: tt.notes, Foil: true}
			id, err := b.Match(card)
			if err != nil {
				t.Fatalf("Match(%v) = %v", card, err)
			}
			co, _ := b.GetUUID(id)
			if co.SetCode != tt.wantSet {
				t.Errorf("Match = %q (%s %s), want set %s", id, co.SetCode, co.Number, tt.wantSet)
			}
		})
	}
}
