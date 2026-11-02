package matcher

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// The errors Match and its helpers return. A caller can tell an input it
// could not place from a datastore that was never loaded, and ErrAliasing
// carries the candidates it could not choose between.
var (
	ErrDatastoreEmpty     = errors.New("datastore is empty")
	ErrCardUnknownID      = errors.New("unknown id")
	ErrCardDoesNotExist   = errors.New("unknown card name")
	ErrCardNotInEdition   = errors.New("unknown edition")
	ErrCardWrongVariant   = errors.New("unknown variant")
	ErrCardMissingVariant = errors.New("missing necessary variant")
	ErrCardWrongFinish    = errors.New("unknown finish")

	// ErrCardUnnamedFinish is a finish name the game cannot place at all, as
	// opposed to one it placed onto a finish the printing is not sold in. It is
	// not evidence of anything - a vendor spelling nobody has taught the game
	// yet reads the same as a typo - so Match falls through to the wording.
	ErrCardUnnamedFinish = errors.New("unrecognized finish")

	ErrUnsupported = errors.New("unsupported")
	ErrAliasing    = NewAliasingError()
)

// AliasingError reports that a listing matched several printings and names
// them, so a caller can report the ambiguity rather than guess.
type AliasingError struct {
	Message string
	Dupes   []string
}

// NewAliasingError builds an aliasing error over the candidate uuids.
func NewAliasingError(duplicates ...string) *AliasingError {
	return &AliasingError{
		Message: "aliasing detected",
		Dupes:   duplicates,
	}
}

func (err *AliasingError) Error() string {
	return err.Message
}

// Probe returns the uuids that could not be told apart.
func (err *AliasingError) Probe() []string {
	return err.Dupes
}

// Regexp for SplitVariants, an optional space and a parenthesis
var re = regexp.MustCompile(` ?\(`)

// SplitVariants returns an array of strings from the parentheses-defined fields
// commonly used to distinguish some cards across editions.
func SplitVariants(str string) []string {
	fields := re.Split(str, -1)
	for i := range fields {
		pos := strings.Index(fields[i], ")")
		if pos > 0 {
			fields[i] = fields[i][:pos]
		}
	}
	if len(fields) > 1 {
		if strings.HasPrefix(str, "Erase (Not the Urza's Legacy One)") ||
			strings.HasPrefix(str, "Hazmat Suit (Used") ||
			strings.HasPrefix(str, "B.O.B.") ||
			(Contains(str, "B.F.M.") && strings.Contains(str, "Big Furry Monster")) {
			fields[0] = fmt.Sprintf("%s (%s)", fields[0], fields[1])
			fields = append(fields[:1], fields[2:]...)
		}
		if strings.HasPrefix(str, "Dwight, Assistant (to the)") {
			fields[0] = fmt.Sprintf("%s (%s) King", fields[0], fields[1])
			fields = append(fields[:1], fields[2:]...)
		}
	}

	// This might have been lost in the split if it was after the ()
	if strings.Contains(strings.ToLower(str), "token") &&
		!strings.Contains(strings.ToLower(fields[0]), "token") {
		fields[0] += " Token"
	}

	return fields
}

var months = []string{
	"january",
	"february",
	"march",
	"april",
	"may",
	"june",
	"july",
	"august",
	"september",
	"october",
	"november",
	"december",
}

// SaysWord reports whether the wording carries the word as a word of its
// own, under simple case folding: not run into a letter, a digit, or one
// of the joiners, the ascii characters that bind to a word in the caller's
// vocabulary ("+" where "C+" is a rarity of its own, so "C" is not said by
// "C+"). A rune that folds to an ascii letter, such as the Kelvin sign,
// counts as a letter. An empty word is said by nothing.
func SaysWord(wording, word, joiners string) bool {
	if word == "" {
		return false
	}
	for i := range wording {
		n, ok := foldedPrefix(wording[i:], word)
		if !ok {
			continue
		}
		before, _ := utf8.DecodeLastRuneInString(wording[:i])
		after, _ := utf8.DecodeRuneInString(wording[i+n:])
		if !wordRune(before, joiners) && !wordRune(after, joiners) {
			return true
		}
	}
	return false
}

func wordRune(r rune, joiners string) bool {
	if r < utf8.RuneSelf {
		return 'A' <= r && r <= 'Z' || 'a' <= r && r <= 'z' || '0' <= r && r <= '9' || strings.ContainsRune(joiners, r)
	}
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		if f < utf8.RuneSelf {
			return true
		}
	}
	return false
}

// foldedPrefix reports whether s opens with word under simple case folding,
// and how many bytes of s that prefix spans.
func foldedPrefix(s, word string) (int, bool) {
	n := 0
	for _, w := range word {
		r, size := utf8.DecodeRuneInString(s[n:])
		if size == 0 || !sameFold(r, w) {
			return 0, false
		}
		n += size
	}
	return n, true
}

func sameFold(r, w rune) bool {
	for f := r; ; {
		if f == w {
			return true
		}
		f = unicode.SimpleFold(f)
		if f == r {
			return false
		}
	}
}

// ExtractNumber returns the _first_ collector number below 1993 found in
// the given string, lowercased, or an empty string if there is none.
//
// The number may carry a single character as prefix, or up to two as
// suffix (one letter and one special character), but not both. The rules
// for what is read and what is dropped:
//   - extra letters are ignored while locating the number, but kept in the
//     result
//   - leading # characters, zeroes and parenthesis are stripped away
//   - numbers starting with M are ignored, as they could be confused with
//     core set names
//   - a month name appearing anywhere as a single word returns an empty
//     string, so a date or a day is never read as a number
//   - for a rational number, only the numerator is considered
func ExtractNumber(str string) string {
	return extractNumber(str, 1993)
}

// ExtractNumberAny returns the first number in the input whatever its length,
// where ExtractNumber caps how many digits it will accept.
func ExtractNumberAny(str string) string {
	return extractNumber(str, math.MaxInt32)
}

func extractNumber(str string, threshold int) string {
	fields := strings.Fields(str)
	for _, field := range fields {
		for _, month := range months {
			if Equals(field, month) {
				return ""
			}
		}
	}

	fields = strings.Fields(str)
	for _, field := range fields {
		field = strings.Replace(field, "(", "", -1)
		field = strings.Replace(field, ")", "", -1)
		field = strings.Replace(field, "#", "", -1)

		if strings.Contains(field, "/") && strings.Count(field, "/") == 1 {
			subfields := strings.Split(field, "/")
			field = strings.TrimSpace(subfields[0])
		}

		// Skip any ordinal number that would be caught up in the check below
		ordinal := strings.ToLower(field)
		if strings.HasSuffix(ordinal, "th") ||
			strings.HasSuffix(ordinal, "st") ||
			strings.HasSuffix(ordinal, "nd") ||
			strings.HasSuffix(ordinal, "rd") {
			continue
		}

		num := strings.TrimLeft(field, "0")
		val, err := strconv.Atoi(num)
		if err == nil && val < threshold {
			return num
		}
		if len(num) > 1 {
			if !unicode.IsDigit(rune(num[len(num)-1])) {
				trimmed := num

				// Remove any suffix
				index := -1
				for i, r := range num {
					if !unicode.IsDigit(r) {
						index = i
						break
					}
				}
				if index > 0 {
					trimmed = num[:index]
				}

				// Try converting to an integer number
				val, err = strconv.Atoi(trimmed)
				if err == nil && val < threshold {
					return strings.ToLower(num)
				}
			}
			if !unicode.IsDigit(rune(num[0])) && num[0] != 'M' {
				val, err = strconv.Atoi(strings.TrimLeft(num[1:], "0"))
				if err == nil && val < threshold {
					return strings.ToLower(num)
				}
			}
			// Check for the PLST numbering system
			if strings.Contains(field, "-") {
				subfields := strings.Split(field, "-")
				if len(subfields) == 2 {
					parsed := ExtractNumber(subfields[1])
					if parsed != "" {
						return subfields[0] + "-" + strings.TrimLeft(subfields[1], "0")
					}
				}
			}
		}
	}
	return ""
}

var reNumerical = regexp.MustCompile(`\d+`)

// ExtractNumberValue returns the first run of digits in the input with leading
// zeroes stripped, so that it survives strconv.Atoi.
func ExtractNumberValue(str string) string {
	return strings.TrimLeft(reNumerical.FindString(str), "0")
}

// ExtractYear returns as string with _first_ year after 1993 found in a
// given string, or an empty string if nothing is found.  It takes care
// of some special characters like parenthesis (ignored) and abbreviations
// (so '06 becomes 2006).
func ExtractYear(str string) string {
	fields := strings.FieldsSeq(str)
	for field := range fields {
		// Drop characters that could interfere with the numeric part
		field = strings.Replace(field, "(", "", -1)
		field = strings.Replace(field, ")", "", -1)
		field = strings.Replace(field, ":", "", -1)

		// Handle abbreviations, checking if year is before or after 2000
		if strings.Contains(field, "'") || strings.HasPrefix(field, "M") {
			probe := "'"
			if strings.HasPrefix(field, "M") {
				probe = "M"
			}
			yearIndex := strings.Index(field, probe)
			yearStr := field[yearIndex+1:]

			// If it not a number maybe it's a common apostrophe,
			// keep iterating over the other fields
			val, err := strconv.Atoi(strings.TrimLeft(yearStr, "0"))
			if err == nil {
				field = strings.Replace(field, probe, "20", 1)
				if val >= 93 {
					field = strings.Replace(field, "20", "19", 1)
				}
			}
		}

		num := strings.TrimLeft(field, "0")
		val, err := strconv.Atoi(num)
		if err == nil && val >= 1993 {
			return num
		}
	}
	return ""
}

// Cut splits the input string in two segments, stripping any whitespace
// before or after the cut, if present.
func Cut(in, tag string) []string {
	splits := strings.SplitN(in, tag, 2)
	if len(splits) > 1 {
		splits[0] = strings.TrimSpace(splits[0])
		splits[1] = strings.TrimSpace(tag + splits[1])
	}
	return splits
}

// ParsePrice reads a price written for people, with a dollar sign and
// thousands separators, as a float. Only decimal notation is a price:
// ParseFloat's NaN, Inf and hexadecimal spellings are refused, as is a
// number too large to be finite, and a refusal always answers zero.
func ParsePrice(priceStr string) (float64, error) {
	priceStr = strings.Replace(priceStr, "$", "", 1)
	priceStr = strings.Replace(priceStr, ",", "", -1)
	priceStr = strings.TrimSpace(priceStr)
	notDecimal := strings.ContainsFunc(priceStr, func(r rune) bool {
		return !strings.ContainsRune("0123456789.+-eE", r)
	})
	if notDecimal {
		return 0, fmt.Errorf("%q is not a decimal price", priceStr)
	}
	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		return 0, err
	}
	return price, nil
}

// Wrapper for the deprecated strings.Title
// abc -> Abc
// ABC -> Abc
// ordinalSuffix matches an English ordinal's letters where a title-caser has
// capitalised them. The digit in front is what makes them wrong: nothing else
// about "St" or "Th" says it should be lower case.
var ordinalSuffix = regexp.MustCompile(`([0-9])(St|Nd|Rd|Th)\b`)

// smallWords are the words an English title leaves in lower case when they
// fall inside the phrase: "Legacy of the Duelist", "Back to Duel". They are
// only small in that position - a phrase opening with one still opens with a
// capital, so "the sacred cards" stays "The Sacred Cards".
var smallWords = map[string]bool{
	"a":    true,
	"an":   true,
	"and":  true,
	"as":   true,
	"at":   true,
	"but":  true,
	"by":   true,
	"for":  true,
	"in":   true,
	"nor":  true,
	"of":   true,
	"on":   true,
	"or":   true,
	"the":  true,
	"to":   true,
	"with": true,
}

// opensPhrase reports whether a word ends on a mark that hands what follows
// a phrase of its own, where a small word is no longer inside one and opens
// its phrase capitalised: "Tag Force: The World", "Duel of Destiny - The
// Movie".
func opensPhrase(word string) bool {
	last, _ := utf8.DecodeLastRuneInString(word)
	switch last {
	case ':', ';', '.', '!', '?', '-', '\u2013', '\u2014':
		return true
	}
	return false
}

// Title capitalizes a phrase the way a title is written, replacing the
// deprecated strings.Title: abc -> Abc, and ABC -> Abc. It also puts back
// down what a plain title-caser gets wrong, the letter after a digit in an
// ordinal and the small words sitting inside a phrase rather than opening
// one.
func Title(str string) string {
	titled := cases.Title(language.English).String(str)
	// A title-caser capitalises the letter after a digit, so "1st place"
	// comes back "1St Place" and "10th anniversary" "10Th Anniversary".
	// Only an ordinal is put back down: "3d text" keeps its "3D Text",
	// where the capital is the one wanted.
	titled = ordinalSuffix.ReplaceAllStringFunc(titled, strings.ToLower)

	// A title-caser capitalises every word, including the ones a title
	// leaves down: "duel of destiny" comes back "Duel Of Destiny". The
	// first word is never one of them, whatever it is, and neither is the
	// word a mark hands a new phrase to.
	words := strings.Split(titled, " ")
	for i := 1; i < len(words); i++ {
		if opensPhrase(words[i-1]) || !smallWords[strings.ToLower(words[i])] {
			continue
		}
		words[i] = strings.ToLower(words[i])
	}
	return strings.Join(words, " ")
}

// CardReleaseDate returns the date the card's set was released.
func (b *Backend) CardReleaseDate(cardID string) (time.Time, error) {
	co, err := b.GetUUID(cardID)
	if err != nil {
		return time.Time{}, err
	}
	releaseDate := co.OriginalReleaseDate

	if releaseDate == "" {
		set, err := b.GetSet(co.SetCode)
		if err != nil {
			return time.Time{}, err
		}
		return set.ReleaseDateTime, nil
	}

	return time.Parse("2006-01-02", releaseDate)
}

// promoHeadings are the headings storefronts file promotional printings
// under without saying which set issued them. They name no set in any game:
// the heading spans every promotional printing a game has, while a set
// carrying that name holds only the products a catalog groups there.
//
// Built on first use rather than at init: Normalize memoizes through a map
// this package sets up in its own init, and a package-level initializer
// would reach it before that runs.
var promoHeadings = sync.OnceValue(func() map[string]bool {
	out := map[string]bool{}
	for _, heading := range []string{
		"Promo", "Promos", "Promo Cards",
		"Promotional", "Promotionals", "Promotional Cards",
	} {
		out[Normalize(heading)] = true
	}
	return out
})

// IsPromoHeading reports whether an edition is one of the headings a
// storefront files promotional printings under rather than a set name.
//
// A game reading one has to decide what to do with it, and the answers
// differ: Lorcana clears it, so the heading cannot narrow to the one set
// wearing its name and drop the promos upstream files in the set they
// reprint; Riftbound keeps it as a gate on the promo-only names while
// refusing to let it choose a set. What they share is the question, which
// is why only the question lives here.
func IsPromoHeading(edition string) bool {
	return promoHeadings()[Normalize(edition)]
}

// ordinalLetters are the letters a printing can be spelled with behind its
// ordinal, which name the printing rather than number it.
const ordinalLetters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// ordinalRun is the run of digits a number ends with once those letters are
// gone.
var ordinalRun = regexp.MustCompile(`[0-9]+$`)

// PlainOrdinal reduces a collector number to the ordinal it carries, without
// the padding or the codes written either side of it: OP01-007 is card 7,
// and so is WTR007 and YS13-ENV07. The whole number stays on Card.Number and
// is what names a printing; this is the shorthand a person types and a
// storefront publishes, and the ordinal is what the two spellings agree on.
// A number carrying no ordinal - the DON!! cards, a leader numbered by a
// word - has none to answer with and yields nothing, rather than an ordinal
// it never printed or a word that is not one. The games numbering their
// cards behind a set code answer PlainNumber with it.
func PlainOrdinal(number string) string {
	run := ordinalRun.FindString(strings.TrimRight(number, ordinalLetters))
	if run == "" {
		return ""
	}
	return CanonicalTail(run)
}

// CanonicalTail strips the leading zeros off a bare number, an all-zero run
// staying "0", so that "007" and "7" read as one number.
func CanonicalTail(number string) string {
	trimmed := strings.TrimLeft(number, "0")
	if trimmed == "" && number != "" {
		return "0"
	}
	return trimmed
}
