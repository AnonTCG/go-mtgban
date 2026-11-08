package cardgamecorner

import (
	"regexp"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// numberedSKUPrefixes open the SKUs of the shelves whose version tag counts
// the store's own index, where the number the listing states names the
// printing: the Lord of the Rings Holiday Release and the Secret Lair
// Promos.
var numberedSKUPrefixes = []string{"LTRH", "SLP"}

// skuNumberRe matches the three digits a SKU ends with, ahead of the version
// the store appends to a card it lists more than once.
var skuNumberRe = regexp.MustCompile(`([0-9]{3})(?:V[0-9]+)?$`)

// skuDigits returns the three digits the card's SKU ends with, which spell
// its collector number where the number field is wrong or empty.
func skuDigits(sku string) string {
	m := skuNumberRe.FindStringSubmatch(sku)
	if m == nil {
		return ""
	}
	return m[1]
}

// matchBySKUNumber answers a listing again with the number its SKU ends in
// instead of the one the store's number field gives. The answer is accepted
// only when the printing's number ends in those digits.
func matchBySKUNumber(b *matcher.Backend, card *Card, index int) (string, bool) {
	digits := skuDigits(card.SKU)
	if digits == "" || trailingNumber(card.Number) == trailingNumber(digits) {
		return "", false
	}

	probe := *card
	probe.Number = digits
	input, err := preprocess(b, &probe, index)
	if err != nil {
		return "", false
	}
	cardID, err := b.Match(input)
	if err != nil {
		return "", false
	}
	co, err := b.GetUUID(cardID)
	if err != nil || !strings.HasSuffix(co.Number, digits) {
		return "", false
	}
	return cardID, true
}
