package onepiece

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestNamedFinish pins which wordings say a finish. The word for the plain
// printing contains the word for the other, so the order they are asked in is
// the whole of it.
func TestNamedFinish(t *testing.T) {
	for _, tt := range []struct {
		desc, variation, want string
		named                 bool
	}{
		{"the plain printing spelled with a dash", "OP10-109 Non-Foil", matcher.FinishNonfoil, true},
		{"and spelled without one", "OP01-013 Nonfoil | Revision Pack", matcher.FinishNonfoil, true},
		{"the other one", "OP01-016 Foil Reprint", matcher.FinishFoil, true},
		{"a wording naming neither", "OP12-037 Beginners Deck Party", "", false},
		{"nor does a bare number", "OP10-109", "", false},
		{"a treatment that merely ends in the word is still the word", "OP06-023 Jolly Roger Foil", matcher.FinishFoil, true},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			in := matcher.InputCard{Variation: tt.variation}
			got, named := namedFinish(&in)
			if got != tt.want || named != tt.named {
				t.Errorf("namedFinish(%q) = %q, %v, want %q, %v", tt.variation, got, named, tt.want, tt.named)
			}
		})
	}
}

// TestFinishNamedTiebreak pins that the wording is read only when it says
// something and only when it settles the tie outright. Cardtrader sends no
// finish property for the game, so the flag beside the wording is the zero
// value on every listing, and a wording that says nothing must leave the tie
// where it was rather than hand it to the plain printing.
func TestFinishNamedTiebreak(t *testing.T) {
	b := gametest.Backend(t, matcher.GameOnePiece)

	for _, tt := range []struct {
		desc, name, variation, edition string
		wantSet                        string
	}{
		{
			"the wording says which of the two printings at this number it is",
			"Basil Hawkins", "OP10-109 Non-Foil", "Learn Together Deck Set", "PRB-02",
		},
		{
			"the same number saying nothing stays refused",
			"Basil Hawkins", "OP10-109", "Learn Together Deck Set", "",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			in := matcher.InputCard{Name: tt.name, Variation: tt.variation, Edition: tt.edition}
			id, err := b.Match(&in)
			if tt.wantSet == "" {
				if err == nil {
					co, _ := b.GetUUID(id)
					t.Errorf("Match(%q) resolved to %s|%s, want a refusal", tt.variation, co.SetCode, co.Number)
				}
				return
			}
			if err != nil {
				t.Fatalf("Match(%q) = %v", tt.variation, err)
			}
			co, err := b.GetUUID(id)
			if err != nil {
				t.Fatal(err)
			}
			if co.SetCode != tt.wantSet {
				t.Errorf("Match(%q) = %s|%s, want set %s", tt.variation, co.SetCode, co.Number, tt.wantSet)
			}
		})
	}
}
