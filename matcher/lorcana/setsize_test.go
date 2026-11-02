package lorcana

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestBaseSetSize pins that the set's size comes from upstream's own tally
// rather than from a card's number. Read off the first enchanted printing
// the set holds, it would be the card after the base run rather than the
// last one in it, numbered higher still by upstream from set 9 on: every
// set it fired on would come out wrong, and the sets with no enchanted
// printing would come out zero.
func TestBaseSetSize(t *testing.T) {
	b := gametest.Backend(t, matcher.GameLorcana)

	for _, tt := range []struct {
		code string
		want int
	}{
		{"1", 204},
		{"3", 208},
		{"9", 205},
		{"13", 207},
		{"Q1", 31},
		{"Q2", 35},
	} {
		set, found := b.Sets[tt.code]
		if !found {
			t.Errorf("%s is not a set", tt.code)
			continue
		}
		if set.BaseSetSize != tt.want {
			t.Errorf("%s: BaseSetSize is %d, want %d", tt.code, set.BaseSetSize, tt.want)
		}
	}
}
