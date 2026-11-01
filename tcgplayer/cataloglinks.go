package tcgplayer

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"

	"github.com/mtgban/go-tcgplayer"
)

// LoadCatalogLinks reads Magic's catalog dump and returns the products the
// datastore names no card for, each filed under the one card it sells, in
// the shape of the sku catalog LoadTCGSKUs returns.
//
// The datastore takes its product ids from Scryfall, which can leave a
// printing TCGplayer shelves in a group of its own without one, as a set's
// promo pack cards sold from "Promo Pack: <set>". A product is tied to a
// card only when nothing is in doubt: no card claims the product,
// the card has no product of its own, the two share an English name and a
// collector number, the product's group is named for the card's set (whole,
// or after a "Promo Pack: "-style prefix), and each is the other's only
// candidate.
func LoadCatalogLinks(b *matcher.Backend, reader io.Reader) (SKUMap, error) {
	var dump tcgplayer.CatalogDump
	err := json.NewDecoder(reader).Decode(&dump)
	if err != nil {
		return nil, err
	}
	if len(dump.Products) == 0 {
		return nil, errors.New("empty catalog dump")
	}
	return catalogLinks(b, &dump), nil
}

// productIDKeys are the identifiers that give a card a TCGplayer product.
var productIDKeys = [...]string{"tcgplayerProductId", "tcgplayerEtchedProductId", "tcgplayerAlternativeFoilProductId"}

// catalogLinkKey is what a product and a card must agree on to be linked.
func catalogLinkKey(setName, name, number string) string {
	return matcher.Normalize(setName) + "|" + matcher.Normalize(name) + "|" + strings.TrimLeft(number, "0")
}

func catalogLinks(b *matcher.Backend, dump *tcgplayer.CatalogDump) SKUMap {
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
		uuid := hits[0].Identifiers["mtgjsonId"]
		products[uuid] = append(products[uuid], product)
	}

	printings := map[int]string{}
	for _, printing := range dump.Printings {
		printings[printing.PrintingID] = printing.Name
	}
	conditions := map[int]string{}
	for _, condition := range dump.Conditions {
		conditions[condition.ConditionID] = strings.ToUpper(condition.Name)
	}

	links := SKUMap{}
	for uuid, list := range products {
		if len(list) != 1 {
			continue
		}
		for _, sku := range list[0].Skus {
			if sku.LanguageID != skuLanguageEnglish {
				continue
			}
			printing := "NON FOIL"
			if printings[sku.PrintingID] == "Foil" {
				printing = "FOIL"
			}
			links[uuid] = append(links[uuid], TCGSku{
				Condition: conditions[sku.ConditionID],
				Language:  "ENGLISH",
				Printing:  printing,
				ProductID: sku.ProductID,
				SkuID:     sku.SKUID,
			})
		}
		if len(links[uuid]) == 0 {
			delete(links, uuid)
		}
	}
	return links
}

// linkedProductID is the product links tie to the card with this mtgjson
// uuid, or "" when they tie none.
func linkedProductID(links SKUMap, uuid string) string {
	skus := links[uuid]
	if len(skus) == 0 {
		return ""
	}
	return strconv.Itoa(skus[0].ProductID)
}

// WithLinks returns the sku catalog with links added for the cards it has
// no skus for. Neither map is changed.
func (skus SKUMap) WithLinks(links SKUMap) SKUMap {
	if len(links) == 0 {
		return skus
	}
	merged := make(SKUMap, len(skus)+len(links))
	for uuid, list := range skus {
		merged[uuid] = list
	}
	for uuid, list := range links {
		_, found := merged[uuid]
		if !found {
			merged[uuid] = list
		}
	}
	return merged
}
