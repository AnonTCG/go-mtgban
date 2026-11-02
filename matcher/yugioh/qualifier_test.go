package yugioh

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestQualifierTellsTheProduct pins what the catalog's own name-qualifier
// decides where the datastore does not publish it as a promo type: a
// listing that says nothing means the product sold under the bare name, a
// wording that spells a qualifier whole means that product before any word
// of it is read as a piece of a rarity, and the qualifier saying the most
// wins.
func TestQualifierTellsTheProduct(t *testing.T) {
	b := gametest.Backend(t, matcher.GameYuGiOh)

	for _, tt := range []struct {
		desc string
		in   matcher.InputCard
		want string
	}{
		{"a plain listing means the plain product", matcher.InputCard{
			Name: "Mayhem Fur Hire", Variation: "BLGG-EN116", Edition: "Battles of Legend: Glorious Gallery"}, "blgg-en116_695673_1stedition"},
		{"and the rarity still reaches the other", matcher.InputCard{
			Name: "Mayhem Fur Hire", Variation: "BLGG-EN116 Starlight Rare", Edition: "Battles of Legend: Glorious Gallery"}, "blgg-en116_696518_1stedition"},
		{"a qualifier spelled whole beats a rarity spelled in part", matcher.InputCard{
			Name: "Monster Reborn", Variation: "26LP-EN001 Emblazoned", Edition: "Limited Pack World Championship 2026"}, "26lp-en001_713318_1stedition"},
		{"and the rarity spelled whole is the rarity", matcher.InputCard{
			Name: "Monster Reborn", Variation: "26LP-EN001 Emblazoned Secret Rare", Edition: "Limited Pack World Championship 2026"}, "26lp-en001_713317_1stedition"},
		{"an artwork qualifier among five arts", matcher.InputCard{
			Name: "Dark Magician", Variation: "RA04-EN106 Arkana Platinum Secret Rare", Edition: "Quarter Century Stampede"}, "ra04-en106_627267_1stedition"},
		{"an ink alone is the plain printing in that ink", matcher.InputCard{
			Name: "Dark Magician Girl the Dragon Knight", Variation: "DLCS-EN006 Blue", Edition: "Dragons of Legend: The Complete Series"}, "dlcs-en006_222244_1stedition"},
		{"and with the alternate art named, that one", matcher.InputCard{
			Name: "Dark Magician Girl the Dragon Knight", Variation: "DLCS-EN006 Alternate Art Blue", Edition: "Dragons of Legend: The Complete Series"}, "dlcs-en006_222248_1stedition"},
		{"nothing named among inks means the uninked printing", matcher.InputCard{
			Name: "Dark Magician Girl the Dragon Knight", Variation: "DLCS-EN006", Edition: "Dragons of Legend: The Complete Series"}, "dlcs-en006_221528_1stedition"},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			in := tt.in
			id, err := b.Match(&in)
			if err != nil {
				t.Fatalf("Match(%v) = %v", tt.in, err)
			}
			if id != tt.want {
				t.Errorf("Match(%v) = %s (%v), want %s", tt.in, id, b.UUIDs[id], tt.want)
			}
		})
	}
}
