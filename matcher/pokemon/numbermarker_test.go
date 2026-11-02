package pokemon

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestNumberMarker pins the letter a storefront hangs off a collector number
// the catalog carries without one. Strikezone numbers the Master Ball
// patterns "074M" beside the plain "074" and the run-marked reprints "001A",
// and the catalog numbers neither that way, so the number as written reaches
// nothing and the listing would go unpriced. Each case below is a listing
// the storefront publishes, beside the same card numbered as the catalog
// writes it.
func TestNumberMarker(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	for _, tt := range []struct {
		desc    string
		in      matcher.InputCard
		wantSet string
		wantNum string
		wantVar string
		wantID  string
	}{
		{
			desc:    "the marked number reaches the pattern the wording names",
			in:      matcher.InputCard{Name: "Umbreon", Edition: "SV: Prismatic Evolutions", Variation: "059M 1st Edition Master Ball Pattern"},
			wantSet: "PRE", wantNum: "059", wantVar: "Master Ball Pattern",
		},
		{
			desc:    "the same number written plain still reaches the plain printing",
			in:      matcher.InputCard{Name: "Umbreon", Edition: "SV: Prismatic Evolutions", Variation: "059 1st Edition"},
			wantSet: "PRE", wantNum: "059", wantVar: "",
		},
		{
			desc:    "and the pattern is reachable without the marker too",
			in:      matcher.InputCard{Name: "Umbreon", Edition: "SV: Prismatic Evolutions", Variation: "059 Master Ball Pattern"},
			wantSet: "PRE", wantNum: "059", wantVar: "Master Ball Pattern",
		},
		{
			desc:    "a marked number beside the run the printing was reprinted in",
			in:      matcher.InputCard{Name: "Alakazam", Edition: "Base Set Shadowless", Variation: "001A Unlimited"},
			wantSet: "BSS", wantNum: "001",
		},
		{
			// The letter is the catalog's own here, not the storefront's:
			// the alternate art promos are numbered for the set they
			// reprint and lettered apart from it. Cool Stuff Inc sells
			// them under that set's name, which is the one set the
			// printing is not in.
			desc:    "a lettered promo the edition admits none of",
			in:      matcher.InputCard{Name: "Garbodor", Edition: "SM Guardians Rising", Variation: "51a/145 Alt Art"},
			wantSet: "PR-1938", wantNum: "051a", wantVar: "Cosmos Holo",
		},
		{
			// The catalog also files this card in the set it came from,
			// as a row TCGplayer never priced, and the edition holds it.
			desc:    "a lettered promo beside a tcgdex row of its own edition",
			in:      matcher.InputCard{Name: "Professor Sycamore", Edition: "XY BREAKpoint", Variation: "107a/122 Alt Art HOLO", Foil: true},
			wantSet: "PR-1938", wantNum: "107a", wantID: "107a-122_133813_holofoil",
		},
		{
			desc:    "and its non-holo twin, which the catalog prices apart",
			in:      matcher.InputCard{Name: "Professor Sycamore", Edition: "XY BREAKpoint", Variation: "107a/122 Alt Art Non-Holo"},
			wantSet: "PR-1938", wantNum: "107a", wantID: "107a-122_133814",
		},
		{
			desc:    "the same, for a number the set code is written into",
			in:      matcher.InputCard{Name: "Tapu Koko", Edition: "SM Promos", Variation: "SM30a Alt Art"},
			wantSet: "PR-1938", wantNum: "SM30a",
		},
		{
			desc:    "and the unlettered number still reaches the set it names",
			in:      matcher.InputCard{Name: "Garbodor", Edition: "SM Guardians Rising", Variation: "51/145"},
			wantSet: "SM02", wantNum: "51",
		},
		{
			desc:    "a marked number in a gallery, whose prefix the storefront rebuilds",
			in:      matcher.InputCard{Name: "Abomasnow", Edition: "SWSH10: Astral Radiance Trainer Gallery", Variation: "TG001C 1st Edition"},
			wantSet: "SWSH10-TG", wantNum: "TG01",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			in := tt.in
			id, err := b.Match(&in)
			if err != nil {
				t.Fatalf("Match(%v) = %v", tt.in, err)
			}
			if tt.wantID != "" && id != tt.wantID {
				t.Errorf("Match(%v) = %s, want %s", tt.in, id, tt.wantID)
			}
			co, err := b.GetUUID(id)
			if err != nil {
				t.Fatalf("GetUUID(%s) = %v", id, err)
			}
			if co.SetCode != tt.wantSet || co.Number != tt.wantNum {
				t.Errorf("Match(%v) = %s|%s, want %s|%s", tt.in, co.SetCode, co.Number, tt.wantSet, tt.wantNum)
			}
			gotVar := ""
			if len(co.PromoTypes) > 0 {
				gotVar = co.PromoTypes[0]
			}
			wantVar := matcher.PromoTypeSlug(tt.wantVar)
			if tt.wantVar == "" {
				wantVar = ""
			}
			if gotVar != wantVar {
				t.Errorf("Match(%v) = %s|%s with promo types %v, want %q", tt.in, co.SetCode, co.Number, co.PromoTypes, wantVar)
			}
		})
	}
}

// TestLetteredPromoAmbiguous pins what happens where the catalog carries a
// lettered number twice. TCGplayer sells several of these promos both at
// their own size and as an oversized jumbo, filing them under the same name,
// number and finish with nothing on the card to tell them apart - the set is
// what does, and a wording naming it picks. One that names neither gets no
// answer: dropping the letter would reach the plain card the promo reprints,
// which is a third card again.
func TestLetteredPromoAmbiguous(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	t.Run("the wording names the set and picks it", func(t *testing.T) {
		in := matcher.InputCard{Name: "Darkrai-GX", Edition: "SM Burning Shadows", Variation: "88a/147 Alt Art"}
		id, err := b.Match(&in)
		if err != nil {
			t.Fatalf("Match(%v) = %v", in, err)
		}
		co, err := b.GetUUID(id)
		if err != nil {
			t.Fatal(err)
		}
		if co.SetCode != "PR-1938" || co.Number != "088a" {
			t.Errorf("Match = %s|%s, want PR-1938|088a", co.SetCode, co.Number)
		}
	})

	t.Run("a wording naming neither set gets no answer", func(t *testing.T) {
		in := matcher.InputCard{Name: "Rayquaza-GX", Edition: "SM Celestial Storm", Variation: "177a/168 Shiny"}
		id, err := b.Match(&in)
		if err == nil {
			co, _ := b.GetUUID(id)
			t.Fatalf("Match = %s|%s, want a refusal", co.SetCode, co.Number)
		}
	})
}
