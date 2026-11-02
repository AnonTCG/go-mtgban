package pokemon

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestEraHeadingAliases pins the storefront headings that name a set the
// general readings cannot reach: one spells the era behind the word "Promos"
// rather than in front of it, one heads a set by the print run, and one names
// a base set that is not the one whose name it ends with.
func TestEraHeadingAliases(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	for _, tt := range []struct {
		desc, name, edition, number, wantSet, wantNumber string
	}{
		{
			desc: "the era written behind the word reaches the promo set",
			name: "Grookey", edition: "Promos Sword and Shield", number: "SWSH001",
			wantSet: "SWSD", wantNumber: "SWSH001",
		},
		{
			desc: "and the other era does too",
			name: "Snorlax GX", edition: "Promos Sun and Moon", number: "SM05",
			wantSet: "SMP", wantNumber: "SM05",
		},
		{
			desc: "the era written in front still reaches it, as it always has",
			name: "Grookey", edition: "SWSH Black Star Promos", number: "SWSH001",
			wantSet: "SWSD", wantNumber: "SWSH001",
		},
		{
			desc: "a set headed by its print run is still the set",
			name: "Charizard", edition: "Base Set Unlimited", number: "4",
			wantSet: "BS", wantNumber: "004",
		},
		{
			desc: "Mega Evolution's base set is not the one from 1999",
			name: "Mega Venusaur ex", edition: "Mega Evolution Base Set", number: "003/132",
			wantSet: "MEG", wantNumber: "003",
		},
		{
			desc: "and the 1999 one is still itself",
			name: "Charizard", edition: "Base Set", number: "4",
			wantSet: "BS", wantNumber: "004",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			in := matcher.InputCard{Name: tt.name, Edition: tt.edition, Variation: tt.number}
			id, err := b.Match(&in)
			if err != nil {
				t.Fatalf("Match(%q [%s] %s) = %v", tt.name, tt.edition, tt.number, err)
			}
			co, err := b.GetUUID(id)
			if err != nil {
				t.Fatalf("GetUUID(%s) = %v", id, err)
			}
			if co.SetCode != tt.wantSet || co.Number != tt.wantNumber {
				t.Errorf("Match(%q [%s] %s) = %s|%s, want %s|%s",
					tt.name, tt.edition, tt.number, co.SetCode, co.Number, tt.wantSet, tt.wantNumber)
			}
		})
	}
}
