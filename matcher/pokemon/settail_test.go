package pokemon

import (
	"maps"
	"slices"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestSetNamedByTail pins the era-prefixed spellings resolving, and pins the
// rules answering the same whether Load precomputed their index or not: the
// index is a shortcut, never the reason an answer exists.
func TestSetNamedByTail(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)
	loaded, ok := NewRules(b), Rules{}

	for _, tt := range []struct {
		edition, want string
	}{
		{"SWSH Darkness Ablaze", "SWSH03: Darkness Ablaze"},
		{"XY Steam Siege", "XY - Steam Siege"},
		{"Nothing Names This Set", ""},
	} {
		t.Run(tt.edition, func(t *testing.T) {
			got := loaded.setNamedByTail(b, tt.edition)
			if got != tt.want {
				t.Errorf("loaded rules: setNamedByTail(%q) = %q, want %q", tt.edition, got, tt.want)
			}
			if bare := ok.setNamedByTail(b, tt.edition); bare != got {
				t.Errorf("bare rules answered %q where the loaded ones answered %q", bare, got)
			}
		})
	}

	if NewRules(b).setsByTail == nil {
		t.Error("NewRules left the index unbuilt")
	}
}

// TestSetIndexesMatchBareRules pins the yearly-run and subset indexes the
// same way: every set's shelf figures and subsets come out the same whether
// Load precomputed them or not.
func TestSetIndexesMatchBareRules(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)
	loaded, bare := NewRules(b), Rules{}
	if loaded.runs == nil || loaded.subsets == nil {
		t.Fatal("NewRules left an index unbuilt")
	}

	for code := range b.Sets {
		sets := []matcher.CandidateSet{{Code: code}}
		if !maps.Equal(loaded.shelfFigures(b, sets), bare.shelfFigures(b, sets)) {
			t.Errorf("%s: loaded and bare rules read different shelf figures", code)
		}
		codes := []string{code}
		if !slices.Equal(loaded.subsetsOf(b, codes), bare.subsetsOf(b, codes)) {
			t.Errorf("%s: loaded and bare rules list different subsets", code)
		}
	}
}
