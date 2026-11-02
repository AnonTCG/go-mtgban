package pokemon

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestShelfFinishGuard pins the plain/holo guard the five CSI era-only
// shelves need: each also carries listings of a Deck Exclusives or Black
// and White Promos twin that editionAliases does not name, selling the
// treatment the resolved set does not. Rows are verbatim Cool Stuff Inc
// wording that a fresh capture landed on the wrong twin before the guard,
// paired with controls proving the same shelf still resolves a wording the
// resolved set actually sells.
func TestShelfFinishGuard(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	for _, tt := range []struct {
		desc                     string
		name, edition, variation string
		wantUUID                 string
	}{
		{"a non-holo N refuses rather than pricing PR-1407's holofoil",
			"N - BW100", "BW Promos",
			"4 *NON-HOLO Version from Rayquaza vs Keldeo Battle Arena Decks*",
			""},
		{"a non-holo Ditto refuses rather than pricing MEW's holofoil",
			"Ditto (Non-Holo) - 132/165", "SV 151", "132/165", ""},
		{"a non-holo Moltres refuses rather than pricing MEW's holofoil",
			"Moltres (Non-Holo) - 146/165", "SV 151", "146/165", ""},
		{"a non-holo Professor's Research refuses rather than pricing SWSH01's holofoil",
			"Professor's Research (Non-Holo) - 178/202", "Sword and Shield",
			"Professor Magnolia", ""},
		{"a holo Eevee refuses rather than pricing SM01's reverse holofoil",
			"Eevee - 101/149 (Holo Promo)", "Sun & Moon",
			"101/149  *HOLO Promo Version from GX Premium Collection*", ""},
		{"the same shelf still reaches the holo Ditto it does sell",
			"Ditto (Holo) - 132/165", "SV 151", "132/165",
			"132-165_516695_holofoil"},
		{"the same shelf still reaches the reverse holo Ditto it does sell",
			"Ditto - 132/165 (Reverse Foil)", "SV 151", "132/165",
			"132-165_516695_reverseholofoil"},
		{"the same shelf still reaches a number it sells plain",
			"Voltorb - 100/165", "SV 151", "100/165", "100-165_516669"},
		{"the same shelf still reaches the holo Professor's Research it does sell",
			"Professor's Research (Holo) - 178/202", "Sword and Shield",
			"Professor Magnolia", "178-202_208508_holofoil"},
		{"a Card Trader Holo Promo wording still reaches a BW promo the shelf sells only nonfoil",
			"Pansage", "BW Black Star Promos", "BW14 Holo Promo | BW14",
			"bw14_87935"},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			in := matcher.InputCard{Name: tt.name, Edition: tt.edition, Variation: tt.variation}
			id, err := b.Match(&in)
			if tt.wantUUID == "" {
				if err == nil {
					t.Errorf("Match(%v) = %s (%v), want an error", in, id, b.UUIDs[id])
				}
				return
			}
			if err != nil {
				t.Fatalf("Match(%v) = %v", in, err)
			}
			if id != tt.wantUUID {
				t.Errorf("Match(%v) = %s (%v), want %s", in, id, b.UUIDs[id], tt.wantUUID)
			}
		})
	}
}
