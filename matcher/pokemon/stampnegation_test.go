package pokemon

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestWithoutNegatedStamp pins the wordings a denial is stripped from, and
// that a genuine stamp demand is left alone.
func TestWithoutNegatedStamp(t *testing.T) {
	tests := []struct {
		variation string
		want      string
	}{
		{"SM199 No Detective Pikachu Stamp", "SM199"},
		{"No Detective Pikachu Stamp", ""},
		{"Non-Stamped 133/132", "133/132"},
		{"Unstamped", ""},
		{"Not Stamped", ""},
		{"Detective Pikachu Stamp", "Detective Pikachu Stamp"},
		{"Cosmos Holo", "Cosmos Holo"},
	}
	for _, tt := range tests {
		t.Run(tt.variation, func(t *testing.T) {
			got := withoutNegatedStamp(tt.variation)
			if got != tt.want {
				t.Errorf("withoutNegatedStamp(%q) = %q, want %q", tt.variation, got, tt.want)
			}
		})
	}
}

// TestNonStampedListings pins the SM Promos shapes Cool Stuff Inc sells
// both ways: the SM Detective Pikachu stamp promos it lists plain (the
// stamped copy is a different, more expensive product) and a Mega
// Evolution illustration rare that carries no stamped printing at all. Each
// case is a listing exactly as pokemonListing hands it to Match, gathered
// from a replay of today's Cool Stuff Inc capture.
func TestNonStampedListings(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	tests := []struct {
		desc                     string
		name, edition, variation string
		wantUUID                 string
	}{
		{
			desc: "a denial naming the stamp it lacks",
			name: "Psyduck (Non-Stamped) - SM199", edition: "Promo", variation: "SM199 No Detective Pikachu Stamp",
			wantUUID: "sm199_188321_holofoil",
		},
		{
			desc: "the denial alone, no catalogue number ahead of it",
			name: "Snubbull (Non-Stamped) - SM200", edition: "Promo", variation: "No Detective Pikachu Stamp",
			wantUUID: "sm200_188322_holofoil",
		},
		{
			desc: "\"Non-Stamped\" in the name, nothing in the variation",
			name: "Detective Pikachu (Non-Stamped) - SM190", edition: "SM Promos",
			wantUUID: "sm190_189805_holofoil",
		},
		{
			desc: "the same shape, an illustration rare rather than a promo",
			name: "Bulbasaur (Non-Stamped) - 133/132", edition: "Mega Evolution",
			wantUUID: "133-132_654472_holofoil",
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			in := &matcher.InputCard{Name: tt.name, Edition: tt.edition, Variation: tt.variation}
			id, err := b.Match(in)
			if err != nil {
				t.Fatalf("Match(%+v) = %v", tt, err)
			}
			if id != tt.wantUUID {
				t.Errorf("Match(%+v) = %s, want %s", tt, id, tt.wantUUID)
			}
			co, err := b.GetUUID(id)
			if err != nil {
				t.Fatal(err)
			}
			for _, promoType := range co.PromoTypes {
				if promoType == "stamped" {
					t.Errorf("landed on the stamped printing %s", id)
				}
			}
		})
	}

	// The positive control: a wording that genuinely names the stamp still
	// has to reach it, on the very card the denial above must miss.
	t.Run("a genuine stamp demand still lands on the stamped copy", func(t *testing.T) {
		in := &matcher.InputCard{Name: "Psyduck (Detective Pikachu Stamp) - SM199", Edition: "Promo"}
		const want = "sm199_206825_holofoil"
		id, err := b.Match(in)
		if err != nil {
			t.Fatalf("Match(%+v) = %v", in, err)
		}
		if id != want {
			t.Errorf("Match(%+v) = %s, want %s", in, id, want)
		}
	})
}
