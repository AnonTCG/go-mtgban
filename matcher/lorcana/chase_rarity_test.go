package lorcana

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestChaseRarity pins the tier a storefront writes beside a name. Lorcana
// prints its chase cards as a second copy of a card already in the set,
// numbered past the set's own count, and a storefront that publishes the
// number of the first while naming the second has said two things that
// cannot both be true. Every case below is a listing a live feed publishes,
// or the same listing with the wording taken away.
func TestChaseRarity(t *testing.T) {
	b := gametest.Backend(t, matcher.GameLorcana)

	for _, tt := range []struct {
		desc    string
		in      matcher.InputCard
		wantSet string
		wantNum string
		wantRar string
	}{
		{
			desc:    "the enchanted card Strike Zone numbers as the plain one",
			in:      matcher.InputCard{Name: "Kuzco's Palace - Home of the Emperor (Alternate Art)", Edition: "Into the Inklands", Variation: "102A", Foil: true},
			wantSet: "3", wantNum: "213", wantRar: "enchanted",
		},
		{
			desc:    "and the plain one, which says no tier, stays where its number puts it",
			in:      matcher.InputCard{Name: "Kuzco's Palace - Home of the Emperor", Edition: "Into the Inklands", Variation: "102"},
			wantSet: "3", wantNum: "102", wantRar: "uncommon",
		},
		{
			desc:    "an epic card carrying another epic card's number",
			in:      matcher.InputCard{Name: "Alice - Accidentally Adrift (Epic)", Edition: "Fabled", Variation: "210", Foil: true},
			wantSet: "9", wantNum: "217", wantRar: "epic",
		},
		{
			desc:    "the epic card that number belongs to, whose own tier agrees with it",
			in:      matcher.InputCard{Name: "Elsa - Snow Queen (Epic)", Edition: "Fabled", Variation: "210E", Foil: true},
			wantSet: "9", wantNum: "210", wantRar: "epic",
		},
		{
			desc:    "a tier named with no number at all",
			in:      matcher.InputCard{Name: "Hades - King of Olympus (Alternate Art)", Edition: "The First Chapter", Foil: true},
			wantSet: "1", wantNum: "205", wantRar: "enchanted",
		},
		{
			desc:    "a number that lands exactly is the more specific claim and keeps its card",
			in:      matcher.InputCard{Name: "Hades - King of Olympus (Alternate Art)", Edition: "The First Chapter", Variation: "5"},
			wantSet: "1", wantNum: "5", wantRar: "rare",
		},
		{
			desc:    "the alternate art of a promo names no tier the card is filed under",
			in:      matcher.InputCard{Name: "Maui - Demigod (Alternate Art)", Edition: "Disney 100 Promos", Variation: "23", Foil: true},
			wantSet: "D100", wantNum: "23", wantRar: "special",
		},
		{
			desc:    "a tier spelled out rather than nicknamed",
			in:      matcher.InputCard{Name: "I2I (Enchanted)", Edition: "Fabled", Foil: true},
			wantSet: "9", wantNum: "234", wantRar: "enchanted",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			in := tt.in
			id, err := b.Match(&in)
			if err != nil {
				t.Fatalf("Match(%v) = %v", tt.in, err)
			}
			co, err := b.GetUUID(id)
			if err != nil {
				t.Fatalf("GetUUID(%s) = %v", id, err)
			}
			if co.SetCode != tt.wantSet || co.Number != tt.wantNum || co.Rarity != tt.wantRar {
				t.Errorf("Match(%v) = %s|%s|%s, want %s|%s|%s", tt.in,
					co.SetCode, co.Number, co.Rarity, tt.wantSet, tt.wantNum, tt.wantRar)
			}
		})
	}
}
