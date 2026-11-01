package cardgamecorner

import "regexp"

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
