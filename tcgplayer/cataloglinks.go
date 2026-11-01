package tcgplayer

import (
	"strconv"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"

	"github.com/mtgban/go-tcgplayer"
)

// productIDKeys are the identifiers that give a card a TCGplayer product.
var productIDKeys = [...]string{"tcgplayerProductId", "tcgplayerEtchedProductId", "tcgplayerAlternativeFoilProductId"}

// catalogLinkKey is what a product and a card must agree on to be linked.
func catalogLinkKey(setName, name, number string) string {
	return matcher.Normalize(setName) + "|" + matcher.Normalize(name) + "|" + strings.TrimLeft(number, "0")
}

// catalogLinks ties each catalog product the datastore names no card for to
// the one card it sells, by product id.
//
// The datastore takes Magic's product ids from Scryfall, which can leave a
// printing TCGplayer shelves in a group of its own without one, as a set's
// promo pack cards sold from "Promo Pack: <set>". A product is tied to a
// card only when nothing is in doubt: no card claims the product, the card
// has no product of its own, the two share an English name and a collector
// number, the product's group is named for the card's set (whole, or after
// a "Promo Pack: "-style prefix), and each is the other's only candidate.
func catalogLinks(b *matcher.Backend, dump *tcgplayer.CatalogDump) map[int]string {
	claimed := map[string]bool{}
	unlinked := map[string][]matcher.Card{}
	for _, code := range b.GetAllSets() {
		set, err := b.GetSet(code)
		if err != nil {
			continue
		}
		for _, card := range set.Cards {
			owned := false
			for _, key := range productIDKeys {
				id := card.Identifiers[key]
				if id != "" {
					claimed[id] = true
					owned = true
				}
			}
			if owned || card.Language != "English" {
				continue
			}
			key := catalogLinkKey(set.Name, card.Name, card.Number)
			unlinked[key] = append(unlinked[key], card)
		}
	}

	groups := map[int]string{}
	for _, group := range dump.Groups {
		groups[group.GroupID] = group.Name
	}

	products := map[string][]tcgplayer.Product{}
	for _, product := range dump.Products {
		number := product.Extended("Number")
		if number == "" || claimed[strconv.Itoa(product.ProductID)] {
			continue
		}
		setNames := []string{groups[product.GroupID]}
		_, after, found := strings.Cut(setNames[0], ": ")
		if found {
			setNames = append(setNames, after)
		}
		var hits []matcher.Card
		for _, setName := range setNames {
			hits = append(hits, unlinked[catalogLinkKey(setName, product.Name, number)]...)
		}
		if len(hits) != 1 {
			continue
		}
		products[hits[0].UUID] = append(products[hits[0].UUID], product)
	}

	links := map[int]string{}
	for uuid, list := range products {
		if len(list) == 1 {
			links[list[0].ProductID] = uuid
		}
	}
	return links
}
