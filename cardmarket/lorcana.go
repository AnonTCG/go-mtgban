package cardmarket

import (
	"cmp"
	"fmt"
	"strings"

	cm "github.com/mtgban/go-cardmarket"

	"github.com/mtgban/go-mtgban/matcher"
)

// lorcanaRenames gives the name the datastore knows a Cardmarket Lorcana
// product by, where Cardmarket sells the printing under another title.
var lorcanaRenames = map[int]string{
	804164: "Moana - Adventurer of Land and Sea", // "Vaiana", Moana's European title
}

// lorcanaNonEnglish holds the products Cardmarket shelves among the English
// cards that were printed in another language alone: the Chinese Exclusive
// Snow White and Stitch promos.
var lorcanaNonEnglish = map[int]bool{
	835454: true, 895480: true,
}

// resolveLorcana names a Lorcana product's printing: the one the datastore
// records under the product's id, a card Cardmarket names its own way; else
// the one the bridge links, where the name still agrees with it; else the
// one its name and number reach.
func resolveLorcana(r *resolver, product *cm.Product) (string, string, bool, error) {
	if lorcanaFaces[product.IDProduct] {
		return "", "", false, nil
	}
	if lorcanaNonEnglish[product.IDProduct] {
		return "", "", false, errForeign
	}
	cardID := r.backend.ConvertID(matcher.IDSpaceCardmarket, fmt.Sprint(product.IDProduct))
	if cardID != "" {
		return r.withFoil(cardID)
	}
	id, found := r.bridged(product)
	if found && bridgeNamesCard(r.backend, product, id) {
		if r.offShelf(product, id) {
			return "", "", false, errNoPrinting
		}
		return r.withFoil(id)
	}

	fields := strings.SplitN(product.Name, " (V.", 2)
	cardName := cmp.Or(lorcanaRenames[product.IDProduct], fields[0])
	number := product.Number
	// The V-index is only a last-resort tiebreak among same-named Special
	// printings a bare number cannot separate - a promo Ravensburger
	// reprints wave to wave, each wave its own locally-numbered pool - so
	// it is otherwise inert.
	if len(fields) > 1 {
		number = strings.TrimSpace(number + " V." + strings.TrimSuffix(fields[1], ")"))
	}
	// Only the rarity tells an oversized card from the set's own at its
	// number; a card with no oversized printing is then unsupported.
	if product.Rarity == "Oversized" {
		number = strings.TrimSpace(number + " Oversized")
	}

	cardID, cardIDFoil, err := r.matchNumbered(cardName, product.ExpansionName, number, true)
	if err != nil || cardID == "" {
		return "", "", false, err
	}

	// A starter deck's exclusive foil is a V.N at its card's own number,
	// filed by the datastore as that card's Holofoil.
	if len(fields) > 1 && strings.TrimSuffix(fields[1], ")") != "1" {
		holo, holoErr := r.backend.MatchIDFinish(cardID, "Holofoil")
		if holoErr == nil && holo != cardID && holo != cardIDFoil {
			cardID, cardIDFoil = holo, holo
		}
	}
	// Misprints and pre-errata prints the datastore has no printing of
	// apart from the card's own.
	if r.ownedElsewhere(product, cardID) {
		return "", "", false, nil
	}
	return cardID, cardIDFoil, false, nil
}
