package pokemon

import (
	"errors"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestNameTailsTheCatalogWrites pins the spellings the catalog itself puts
// behind a dash, which a storefront copying the product name carries along:
// a number written with a dash for its slash, a bracketed set behind the
// number, the two halves' numbers of a LEGEND pair printed as one card, and
// a number behind a dash with no space after it. The datastore names none of
// them into the card.
func TestNameTailsTheCatalogWrites(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	for _, tt := range []struct {
		desc string
		in   matcher.InputCard
		want string
	}{
		{"a dash for the slash", matcher.InputCard{
			Name: "Morpeko - 072-167", Edition: "Prize Pack Series Cards"}, "072-167_619142"},
		{"a bracketed set behind the number", matcher.InputCard{
			Name: "Pichu - 45/106 [Platinum]", Variation: "045/100", Edition: "Burger King Promos"}, "045-100_188126_reverseholofoil"},
		{"and behind a number the catalog's own field contradicts", matcher.InputCard{
			Name: "Piplup - 93/130 [Diamond and Pearl]", Variation: "093/100", Edition: "Burger King Promos"}, "093-100_235080_reverseholofoil"},
		{"a LEGEND pair's two numbers", matcher.InputCard{
			Name: "Darkrai & Cresselia Legend - 99/102 & 100/102", Variation: "Single Oversized Promo", Edition: "Jumbo Cards"}, "099-102-100-102_211448_holofoil"},
		{"joined with a plus", matcher.InputCard{
			Name: "Palkia & Dialga Legends - 101/102 + 102/102", Variation: "Single Oversized Promo", Edition: "Jumbo Cards"}, "101-102-102-102_211449_holofoil"},
		{"read as one number in the wording too, not as the halves", matcher.InputCard{
			Name: "Darkrai & Cresselia Legend", Variation: "099/102 & 100/102"}, "099-102-100-102_211448_holofoil"},
		{"a dash with no space after it", matcher.InputCard{
			Name: "Ninetales -199/197", Edition: "SV03: Obsidian Flames"}, "199-197_509945_holofoil"},
		{"and a lone number behind it", matcher.InputCard{
			Name: "Rayquaza EX -BW47", Edition: "Black and White Promos"}, "bw47_88646_holofoil"},
		{"a name carrying a bracket of its own stays whole", matcher.InputCard{
			Name: "Greninja V-UNION [Set of 4]", Edition: "SWSH Promos"}, "248896_holofoil"},
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

// TestLegendPairNeverAnswersAHalf pins what a pair's number reaches: it
// names the one card printed with both halves' numbers, and the storefronts
// shelve that card under Triumphant, where the edition admits the halves
// alone. A listing saying oversized reaches the jumbo past the edition, and
// one that does not is refused.
func TestLegendPairNeverAnswersAHalf(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	in := matcher.InputCard{
		Name: "Darkrai & Cresselia Legend - 99/102 & 100/102", Variation: "Single Oversized Promo", Edition: "Triumphant"}
	if got, err := b.Match(&in); err != nil || got != "099-102-100-102_211448_holofoil" {
		t.Errorf("Match(%+v) = %q, %v, want the jumbo", in, got, err)
	}

	in = matcher.InputCard{Name: "Darkrai & Cresselia Legend - 99/102 & 100/102", Edition: "Triumphant"}
	if got, err := b.Match(&in); !errors.Is(err, matcher.ErrCardWrongVariant) {
		t.Errorf("Match(%+v) = %q, %v, want %v", in, got, err, matcher.ErrCardWrongVariant)
	}
}
