package gundam

import (
	"regexp"
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
)

// The rarity test answers as the pattern it replaced did: the rarity as a
// word of its own, case-insensitively, with "+" counted as part of a word.
func TestSaysRarityMatchesPattern(t *testing.T) {
	for _, tc := range []struct{ variation, rarity string }{
		{"C+ parallel", "C+"}, {"C++ parallel", "C+"}, {"rare C", "C"}, {"RC", "C"},
		{"Foil (SR)", "SR"}, {"Foil (sr)", "SR"}, {"SR+", "SR"}, {"", "C"}, {"C", ""},
		{"Über Rare", "Rare"}, {"Kind", "kind"}, {"kind", "Kind"}, {"pre-R", "R"},
		{"R", "R"}, {"RR", "R"}, {"R R", "R"}, {"x R+ y", "R+"}, {"R+", "R"},
		{"\u017f0", "0"}, {"\u212aInd", "ind"}, {"ind\u017f", "ind"}, {"\u212a", "k"},
	} {
		want := false
		if tc.rarity != "" {
			pattern := regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9+])` + regexp.QuoteMeta(tc.rarity) + `(?:$|[^A-Za-z0-9+])`)
			want = pattern.MatchString(tc.variation)
		}
		if got := matcher.SaysWord(tc.variation, tc.rarity, "+"); got != want {
			t.Errorf("SaysWord(%q, %q, \"+\") = %v, pattern gives %v", tc.variation, tc.rarity, got, want)
		}
	}
}
