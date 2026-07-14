package mtgmatcher

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var ErrDatastoreEmpty = errors.New("datastore is empty")
var ErrCardUnknownId = errors.New("unknown id")
var ErrCardDoesNotExist = errors.New("unknown card name")
var ErrCardNotInEdition = errors.New("unknown edition")
var ErrCardWrongVariant = errors.New("unknown variant")
var ErrCardMissingVariant = errors.New("missing necessary variant")
var ErrUnsupported = errors.New("unsupported")
var ErrAliasing = NewAliasingError()

type AliasingError struct {
	Message string
	Dupes   []string
}

func NewAliasingError(duplicates ...string) *AliasingError {
	return &AliasingError{
		Message: "aliasing detected",
		Dupes:   duplicates,
	}
}

func (err *AliasingError) Error() string {
	return err.Message
}

func (err *AliasingError) Probe() []string {
	return err.Dupes
}

const LongestCardEver = "Our Market Research Shows That Players Like Really Long Card Names So We Made this Card to Have the Absolute Longest Card Name Ever Elemental"
const NightmareCard = "The Ultimate Nightmare of Wizards of the Coast® Customer Service"

// Date since any card could be Prerelease Promo
var NewPrereleaseDate = time.Date(2014, time.September, 1, 0, 0, 0, 0, time.UTC)

// Date since BuyABox cards are found in the expansion set instead of Promos
var BuyABoxInExpansionSetsDate = time.Date(2018, time.April, 1, 0, 0, 0, 0, time.UTC)

// Date in which random promos can be in the expansion set
var PromosForEverybodyYay = time.Date(2019, time.October, 1, 0, 0, 0, 0, time.UTC)

// Date since BuyABox cards are not unique any more
var BuyABoxNotUniqueDate = time.Date(2020, time.September, 1, 0, 0, 0, 0, time.UTC)

// Date since different finishes (etched, gilded, thick) get separate collector numbers
var SeparateFinishCollectorNumberDate = time.Date(2022, time.February, 1, 0, 0, 0, 0, time.UTC)

// Guilds found in GRN
var GRNGuilds = []string{"Boros", "Dimir", "Golgari", "Izzet", "Selesnya"}

// Guilds found in ARN
var ARNGuilds = []string{"Azorius", "Gruul", "Orzhov", "Rakdos", "Simic"}

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

// ExtractNumber returns as lower case string with the _first_ collector number
// below 1993 found in a given string, or an empty string if none could be found.
// The input string may have a single character as prefix or up to two characters
// as suffix (one letter and one special character), but not both.
// Any extra letters will be ignored while determining the number portion, but
// preserved in the ouput, and returned as lowercase.
// Any leading # characters, zeroes or parenthesis are stripped away.
// Numbers starting with M are ignored because they could be confused
// with core set names.
// If a month name is detected anywhere as a single word in the input string,
// an empty string is returned, to prevent confusing a number with a date or day.
// If a rational number is provided, only the numerator part is considered.
func ExtractNumber(str string) string {
	return extractNumber(str, 1993)
}

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

		// Skip tags that could be confused with set codes
		// unless it ends with "a" (ie 30A)
		_, err := GetSet(field)
		if err == nil && !strings.HasSuffix(field, "a") {
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

// Obtain the numerical value of the input so that it can be used for strconv.Atoi()
func ExtractNumberValue(str string) string {
	return strings.TrimLeft(reNumerical.FindString(str), "0")
}

// Specialized version of ExtractNumber, suited for parsing WCD numbers
func extractWCDNumber(str, prefix string, sideboard bool) string {
	fields := strings.Fields(str)
	for _, field := range fields {
		field = strings.Replace(field, "(", "", -1)
		field = strings.Replace(field, ")", "", -1)

		if !strings.HasPrefix(field, prefix) {
			continue
		}

		num := strings.TrimPrefix(field, prefix)
		if sideboard {
			num = strings.Replace(num, "sb", "", 1)
		}
		num = strings.TrimLeft(num, "0")
		if unicode.IsLetter(rune(num[len(num)-1])) {
			num = num[:len(num)-1]
		}
		val, err := strconv.Atoi(num)
		if err == nil && val < 1993 {
			// Special way to discard any leading zeros without rebuilding manually
			field = strings.Replace(field, prefix+"00", prefix, 1)
			return strings.Replace(field, prefix+"0", prefix, 1)
		}
	}

	return ""
}

// ExtractYear returns as string with _first_ year after 1993 found in a
// given string, or an empty string if nothing is found.  It takes care
// of some special characters like parenthesis (ignored) and abbreviations
// (so '06 becomes 2006).
func ExtractYear(str string) string {
	fields := strings.Fields(str)
	for _, field := range fields {
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

// Strip input string of dollar sign and commas, convert it to a normal float
func ParsePrice(priceStr string) (float64, error) {
	priceStr = strings.Replace(priceStr, "$", "", 1)
	priceStr = strings.Replace(priceStr, ",", "", -1)
	priceStr = strings.TrimSpace(priceStr)
	return strconv.ParseFloat(priceStr, 64)
}

// Wrapper for the deprecated strings.Title
// abc -> Abc
// ABC -> Abc
func Title(str string) string {
	return cases.Title(language.English).String(str)
}

// Find the keyword in an edition name, ignoring punctuation
func longestWordInEditionName(str string) string {
	fields := strings.Fields(str)
	longest := ""
	for _, field := range fields {
		field = strings.TrimRight(field, ":'")
		if len(field) > len(longest) {
			longest = field
		}
	}
	return longest
}

// Greatest common divisor (GCD) via Euclidean algorithm
func GCD(a, b int) int {
	for b != 0 {
		t := b
		b = a % b
		a = t
	}
	return a
}

// Find Least Common Multiple (LCM) via GCD
func LCM(a, b int) int {
	return a * b / GCD(a, b)
}

// Retrieve the card release date
func CardReleaseDate(cardId string) (time.Time, error) {
	co, err := GetUUID(cardId)
	if err != nil {
		return time.Time{}, err
	}
	releaseDate := co.OriginalReleaseDate

	if releaseDate == "" {
		set, err := GetSet(co.SetCode)
		if err != nil {
			return time.Time{}, err
		}
		return set.ReleaseDateTime, nil
	}

	return time.Parse("2006-01-02", releaseDate)
}

// Check if a dual-faced card has the same for both faces
func IsDFCSameName(name string) bool {
	if !strings.Contains(name, " // ") {
		return false
	}
	left := name[:len(name)/2-2]
	right := name[len(name)/2+2:]
	return left == right
}

// Check if the string could pass as a uuid
func maybeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	return true
}
