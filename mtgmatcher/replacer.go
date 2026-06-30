package mtgmatcher

import (
	"strings"
)

var replacerStrings = []string{
	// Remove a very common field, sometimes added with no reason
	// Needs the dashes to work with will-o'-the-wisp, which is why
	// it needs to be before removing the dash step
	" the ", "",
	"-the-", "",
	// Hopefully "of/from the" is specific enough
	"of the", "of the",
	"from the", "from the",

	// I can't even
	"jeong, the", "jeongthe",

	// Wrong escaping or conversion
	"&quot;", "",

	// Quotes and commas and whatnot
	"''", "",
	"“", "",
	"”", "",
	"\"", "",
	"'", "",
	"-", "",
	"—", "",
	"’", "",
	",", "",
	"®", "",
	":", "",
	"꞉", "",
	"：", "",
	"~", "",
	"(", "",
	")", "",
	".", "",
	"!", "",
	"?", "",
	"+", "",
	"…", "",

	// UNF blanks
	"___________", "_____",
	"__________", "_____",
	"_________", "_____",
	"________", "_____",
	"_______", "_____",
	"______", "_____",

	// Separators
	"goblin // soldier", "goblin // soldier",
	"/", "",
	"|", "",
	"trial and error", "trial and error",
	"welcome to", "welcometo",
	" and ", "",
	" to ", "",
	" & ", "",
	"reverse the polarity", "reverse the polarity",
	"glimpse, the unthinkable", "glimpse, the unthinkable",

	// Accented characters
	"â", "a",
	"á", "a",
	"à", "a",
	"ä", "a",
	"ā", "a",
	"é", "e",
	"í", "i",
	"ï", "i",
	"ö", "o",
	"ō", "o",
	"ó", "o",
	"ú", "u",
	"û", "u",
	"ü", "u",
	"ñ", "n",

	// Ancient ligature
	"æ", "ae",

	// Also plurals, just preserve 'blossom' that aliases 'lotus bloom'
	// and 'asp' for 'tangle asp'/'tanglesap', and ogress...
	// 'vs' is a key for determining duel decks
	// Any accented s need to be removed as well to behave like a normal s
	"asp", "asp",
	"lossom", "lossom",
	"ogress", "ogress",
	"slash", "slash",
	"squash", "squash",
	"sword", "sword",
	"kess", "kess",
	"kediss", "kediss",
	"vs", "vs",
	"š", "",
	"s", "",

	// Spaces are overrated, except when not
	"waste land", "waste land",
	" ", "",
}

var replacer = strings.NewReplacer(replacerStrings...)

// Normalize uses the rules defined in Replacer to replace uncommon elements of
// card names, dropping all the spaces and producing a lowercase string.
func Normalize(str string) string {
	str = strings.TrimSpace(str)
	str = strings.ToLower(str)
	str = replacer.Replace(str)
	return str
}

// Compare strings after both are Normalize-d.
func Equals(str1, str2 string) bool {
	return Normalize(str1) == Normalize(str2)
}

// Check if str1 contains str2 after both are Normalize-d.
func Contains(str1, str2 string) bool {
	return strings.Contains(Normalize(str1), Normalize(str2))
}

// Check if str2 is the prefix of str1 after both are Normalize-d.
func HasPrefix(str1, str2 string) bool {
	return strings.HasPrefix(Normalize(str1), Normalize(str2))
}

// Check if str2 is the suffix of str1 after both are Normalize-d.
func HasSuffix(str1, str2 string) bool {
	return strings.HasSuffix(Normalize(str1), Normalize(str2))
}
