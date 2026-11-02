package lorcana

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// The three name lists feed searchFunc, which adds a matching entry's whole
// hash bucket: a repeated entry returns every printing of that name once per
// repeat. Each list holds distinct values of its own kind.
func TestNameListsAreDistinct(t *testing.T) {
	b := gametest.Backend(t, matcher.GameLorcana)
	for _, tt := range []struct {
		name string
		list []string
	}{
		{"AllNames", b.AllNames},
		{"AllCanonicalNames", b.AllCanonicalNames},
		{"AllLowerNames", b.AllLowerNames},
	} {
		seen := map[string]bool{}
		for _, entry := range tt.list {
			if seen[entry] {
				t.Errorf("%s repeats %q", tt.name, entry)
			}
			seen[entry] = true
		}
	}
}
