package cardmarket

import (
	"fmt"
	"strings"

	cm "github.com/mtgban/go-cardmarket"

	"github.com/mtgban/go-mtgban/matcher"
)

// gundamRarities maps Cardmarket's spelling of a Gundam rarity onto the
// catalog's. The catalog marks a parallel by suffixing the rarity, where
// Cardmarket spells the rarity out and spaces the suffix off.
var gundamRarities = map[string]string{
	"Legendary Rare":    "Legend Rare",
	"Legendary Rare +":  "LR+",
	"Legendary Rare ++": "LR++",
	"Rare +":            "R+",
	"Special Rare":      "SP",
	"Uncommon +":        "U+",
	"Common +":          "C+",
	"Common ++":         "C++",
}

// gundamRarity spells a Cardmarket Gundam rarity the catalog's way.
func gundamRarity(rarity string) string {
	rarity = strings.TrimSpace(rarity)
	if spelled, found := gundamRarities[rarity]; found {
		return spelled
	}
	return rarity
}

// gundamLabels names, by product id, the promo print a product sells where
// Cardmarket names it like the card itself: the Premium Bandai products
// selling a Premium Card Collection 02 print, which CardTrader's blueprints
// identify, and the second Common of GD02-029's promo shelf, the one print
// of that card no other product prices.
var gundamLabels = map[int]string{
	908743: "Premium Card Collection 02", // Sword Strike Gundam, GD01-073
	908744: "Premium Card Collection 02", // A Healthy Curiosity, GD03-101
	908745: "Premium Card Collection 02", // GN Armor Type-E, GD04-063
	908746: "Premium Card Collection 02", // Darkness Finger, GD05-110
	908747: "Premium Card Collection 02", // Widespread Annihilation, GD05-114
	908748: "Premium Card Collection 02", // Char's Zaku II, ST03-006

	914392: "Store Tournament Participation Pack", // Gundam AGE-1 Normal, GD02-029
}

// gundamUnmade lists Cardmarket products that name no printing at all: a
// duplicate carrying a code of another card, or a prize that is no card.
var gundamUnmade = map[int]bool{
	908636: true, // a second Haman Karn tournament promo, coded GD02-092
	908740: true, // Trophy Card Gundam (World Championship 25-26), V.1
	908741: true, // V.2
	908742: true, // V.3
}

// gundamNoRow lists the products of a printing the datastore does not carry,
// which the promo shelf pin would otherwise land on a sibling's row.
var gundamNoRow = map[int]bool{
	908683: true, // Char's Zaku II, ST03-006, a Special Tournament Promo
	908594: true, // GD01-013 V.2, a fifth print no catalog names
}

// gundamPromoSet is the one set the datastore files every promotional
// printing in, which Cardmarket splits over shelves of its own.
const gundamPromoSet = "Gundam Promotional Cards"

// gundamPromoShelves are the Cardmarket shelves holding promotional prints.
// Named as they are, the matcher reaches past them to the card's main-set
// printing, which the card's own product already prices.
var gundamPromoShelves = map[string]bool{
	"Unnumbered Promos":         true,
	"Special Tournament Promos": true,
	"Winner Cards":              true,
	"Premium Bandai Products":   true,
	"Judge Promos":              true,
	"Promos":                    true,
}

// gundamNumber answers the collector number of a product. Cardmarket's own
// number field can hold another card's code, where the code the name ends
// with is always the card's.
func gundamNumber(cardName, number string) string {
	fields := nameCode.FindStringSubmatch(cardName)
	if fields != nil && strings.Contains(number, "-") {
		return fields[1]
	}
	return number
}

// resolveGundam names a Gundam product's printing: the one the datastore
// records under the product's id; else the one the bridge links, where the
// code in the product's name agrees with it; else the one its name, number
// and rarity reach.
func resolveGundam(r *resolver, product *cm.Product) (string, string, bool, error) {
	if gundamUnmade[product.IDProduct] {
		return "", "", false, nil
	}
	if gundamNoRow[product.IDProduct] {
		return "", "", false, errNoPrinting
	}
	cardID := r.backend.ConvertID(matcher.IDSpaceCardmarket, fmt.Sprint(product.IDProduct))
	if cardID != "" {
		return r.withFoil(cardID)
	}
	// A Gundam product writes the card's own number into its name, which
	// says more than a name the two catalogs spell apart ("Tallgeese Ⅲ", a
	// token without "Token").
	id, found := r.bridged(product)
	if found && !offCode(r.backend, product, id) {
		if r.offShelf(product, id) {
			return "", "", false, errNoPrinting
		}
		return r.withFoil(id)
	}

	fields := strings.SplitN(product.Name, " (V.", 2)
	cardName := fields[0]
	number := gundamNumber(cardName, product.Number)
	edition := product.ExpansionName
	if gundamPromoShelves[edition] {
		edition = gundamPromoSet
	}
	if len(fields) > 1 {
		// The rarity after the index is read from the product's own.
		version, _, _ := strings.Cut(strings.TrimSuffix(fields[1], ")"), " - ")
		number = strings.TrimSpace(number + " V." + version)
	}
	// Only the rarity tells an oversized card from the set's own at its
	// number; a card with no oversized printing is then unsupported.
	if product.Rarity == "Oversized" {
		number = strings.TrimSpace(number + " Oversized")
	}
	// A parallel shares its base card's number, and only the rarity,
	// spelled the catalog's way, tells the two apart.
	number = strings.TrimSpace(number + " " + gundamRarity(product.Rarity))
	if label, found := gundamLabels[product.IDProduct]; found {
		number = strings.TrimSpace(number + " " + label)
	}

	cardID, cardIDFoil, err := r.matchNumbered(cardName, edition, number, true)
	if err != nil || cardID == "" {
		return "", "", false, err
	}
	return cardID, cardIDFoil, false, nil
}
