package yugioh

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestSiblingRarity pins the edition a tier names when the number does not.
// A set family splits into editions filed as sets of their own, each printing
// the same cards under its own numbers - MVP1-EN054, MVP1-ENG54, MVP1-ENS54 -
// and a storefront that numbers every one of them with the base edition's
// number has said which tier and never which edition. All three answered with
// the base edition's Ultra, so the $2.50 Gold and the $1.50 Secret were both
// sold at the $1.25 Ultra's price.
func TestSiblingRarity(t *testing.T) {
	b := gametest.Backend(t, matcher.GameYuGiOh)

	const family = "The Dark Side of Dimensions Movie Pack"
	for _, tt := range []struct {
		desc       string
		in         matcher.InputCard
		wantSet    string
		wantNumber string
		wantRarity string
	}{
		{
			desc:    "a tier only a sibling prints reaches the sibling",
			in:      matcher.InputCard{Name: "Dark Magician", Edition: family, Variation: "MVP1-EN054 Gold Rare"},
			wantSet: "MVP1-ENG", wantNumber: "MVP1-ENG54", wantRarity: "Gold Rare",
		},
		{
			desc:    "and so does the other sibling's",
			in:      matcher.InputCard{Name: "Dark Magician", Edition: family, Variation: "MVP1-EN054 Secret Rare"},
			wantSet: "MVP1-ENS", wantNumber: "MVP1-ENS54", wantRarity: "Secret Rare",
		},
		{
			desc:    "the tier the named edition prints keeps it",
			in:      matcher.InputCard{Name: "Dark Magician", Edition: family, Variation: "MVP1-EN054 Ultra Rare"},
			wantSet: "MVP1", wantNumber: "MVP1-EN054", wantRarity: "Ultra Rare",
		},
		{
			desc:    "a tier whose words another tier's contain is not read as it",
			in:      matcher.InputCard{Name: "Dark Magician", Edition: family, Variation: "Gold Secret Rare"},
			wantSet: "MVP1-ENG", wantNumber: "MVP1-ENGV3", wantRarity: "Gold Secret Rare",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			in := tt.in
			id, err := b.Match(&in)
			if err != nil {
				t.Fatalf("Match(%v) = %v", tt.in, err)
			}
			co := b.UUIDs[id]
			rarity := b.RarityLabel(co.Rarity)
			if co.SetCode != tt.wantSet || co.Number != tt.wantNumber || rarity != tt.wantRarity {
				t.Errorf("Match(%v) = %s|%s|%s, want %s|%s|%s", tt.in,
					co.SetCode, co.Number, rarity, tt.wantSet, tt.wantNumber, tt.wantRarity)
			}
		})
	}
}
