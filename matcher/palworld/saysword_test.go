package palworld

import (
	"regexp"
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
)

// The word test answers as the pattern it replaced did: the spelling as a
// word of its own, case-insensitively.
func TestSaysWordMatchesPattern(t *testing.T) {
	for _, tc := range []struct{ variation, word string }{
		{"Prototype", "Prototype"}, {"prototype foil", "Prototype"}, {"Prototypes", "Prototype"},
		{"(Prototype)", "Prototype"}, {"C+", "C"}, {"RC", "C"}, {"", "C"}, {"C", ""},
		{"Über Rare", "Rare"}, {"Kind", "kind"}, {"kind", "Kind"}, {"pre-R", "R"},
		{"R R", "R"}, {"x 1P y", "1P"}, {"01P", "1P"},
		{"\u017f0", "0"}, {"\u212aInd", "ind"}, {"ind\u017f", "ind"}, {"\u212a", "k"}, {"C+", "C"},
	} {
		want := false
		if tc.word != "" {
			pattern := regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])` + regexp.QuoteMeta(tc.word) + `(?:$|[^A-Za-z0-9])`)
			want = pattern.MatchString(tc.variation)
		}
		if got := matcher.SaysWord(tc.variation, tc.word, ""); got != want {
			t.Errorf("SaysWord(%q, %q, \"\") = %v, pattern gives %v", tc.variation, tc.word, got, want)
		}
	}
}
