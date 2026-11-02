package yugioh

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestPossessiveRarity pins the three ways a storefront writes a tier the
// catalog spells possessively. Dropping the possessive is not a near miss:
// every tier this game has ends in "Rare", so a wording the premium tier
// cannot answer is still answered in full by the plain one, and King's
// Court sells both at one number - the listing that says "Collector Rare"
// was being handed the printing worth a fortieth of it.
func TestPossessiveRarity(t *testing.T) {
	b := gametest.Backend(t, matcher.GameYuGiOh)

	for _, tt := range []struct {
		desc       string
		in         matcher.InputCard
		wantSet    string
		wantNumber string
		wantRarity string
	}{
		{
			desc:    "the possessive dropped entirely still names the tier",
			in:      matcher.InputCard{Name: "Queen's Knight", Edition: "King's Court", Variation: "KICO-EN026 Collector Rare"},
			wantSet: "KICO", wantNumber: "KICO-EN026", wantRarity: "Collector's Rare",
		},
		{
			desc:    "and so does the possessive written without its mark",
			in:      matcher.InputCard{Name: "Queen's Knight", Edition: "King's Court", Variation: "KICO-EN026 Collectors Rare"},
			wantSet: "KICO", wantNumber: "KICO-EN026", wantRarity: "Collector's Rare",
		},
		{
			desc:    "and the catalog's own spelling",
			in:      matcher.InputCard{Name: "Queen's Knight", Edition: "King's Court", Variation: "KICO-EN026 Collector's Rare"},
			wantSet: "KICO", wantNumber: "KICO-EN026", wantRarity: "Collector's Rare",
		},
		{
			desc:    "the plain tier at that same number keeps the plain printing",
			in:      matcher.InputCard{Name: "Queen's Knight", Edition: "King's Court", Variation: "KICO-EN026 Rare"},
			wantSet: "KICO", wantNumber: "KICO-EN026", wantRarity: "Rare",
		},
		{
			desc:    "the tier named in the card's own name reads alike",
			in:      matcher.InputCard{Name: "Queen's Knight (Collector Rare)", Edition: "King's Court"},
			wantSet: "KICO", wantNumber: "KICO-EN026", wantRarity: "Collector's Rare",
		},
		{
			desc:    "the possessive tier the catalog spells with a mark",
			in:      matcher.InputCard{Name: "Obelisk the Tormentor", Edition: "King's Court", Variation: "KICO-EN064 Pharaoh's Rare with Secret Rare text"},
			wantSet: "KICO", wantNumber: "KICO-EN064", wantRarity: "Secret Pharaoh's Rare",
		},
		{
			desc:    "a possessive further into a tier keeps its s, so a stray word cannot upgrade the plain tier",
			in:      matcher.InputCard{Name: "Gravekeeper's Trap", Edition: "Magnificent Mavens", Variation: "Pharaoh Ultra Rare"},
			wantSet: "MAMA", wantNumber: "MAMA-EN029", wantRarity: "Ultra Rare",
		},
		{
			desc:    "a tier the set prints only decorated is reached without the possessive too",
			in:      matcher.InputCard{Name: "Alpha, the Master of Beasts", Edition: "25th Anniversary Rarity Collection", Variation: "Collector Rare"},
			wantSet: "RA01", wantNumber: "RA01-EN022", wantRarity: "Prismatic Collector's Rare",
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
			rarity := b.RarityLabel(co.Rarity)
			if co.SetCode != tt.wantSet || co.Number != tt.wantNumber || rarity != tt.wantRarity {
				t.Errorf("Match(%v) = %s|%s|%s, want %s|%s|%s", tt.in,
					co.SetCode, co.Number, rarity, tt.wantSet, tt.wantNumber, tt.wantRarity)
			}
		})
	}
}
