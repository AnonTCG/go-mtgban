package tcgplayer

import (
	"strconv"
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-tcgplayer"
)

// unlinkedCards returns English printings the datastore gives no product,
// each the only one of its set, name and number, sold nonfoil: what a
// catalog product can be tied to. Read off the datastore rather than named,
// since upstream links any one of them sooner or later.
func unlinkedCards(b *matcher.Backend, n int) []matcher.Card {
	count := map[string]int{}
	var all []matcher.Card
	for _, code := range b.GetAllSets() {
		set, _ := b.GetSet(code)
		for _, card := range set.Cards {
			if ownsProduct(card) || card.Language != "English" {
				continue
			}
			key := catalogLinkKey(set.Name, card.Name, card.Number)
			count[key]++
			if card.HasFinish(matcher.FinishNonfoil) && card.HasFinish(matcher.FinishFoil) {
				all = append(all, card)
			}
		}
	}
	var cards []matcher.Card
	for _, card := range all {
		set, _ := b.GetSet(card.SetCode)
		if count[catalogLinkKey(set.Name, card.Name, card.Number)] == 1 {
			cards = append(cards, card)
		}
		if len(cards) == n {
			break
		}
	}
	return cards
}

func ownsProduct(card matcher.Card) bool {
	for _, key := range productIDKeys {
		if card.Identifiers[key] != "" {
			return true
		}
	}
	return false
}

// linkedCard returns an English printing the datastore gives a product under
// key and under none of the other identifiers.
func linkedCard(b *matcher.Backend, key string) matcher.Card {
	for _, code := range b.GetAllSets() {
		set, _ := b.GetSet(code)
		for _, card := range set.Cards {
			if card.Language != "English" || card.Identifiers[key] == "" {
				continue
			}
			only := true
			for _, other := range productIDKeys {
				if other != key && card.Identifiers[other] != "" {
					only = false
				}
			}
			if only {
				return card
			}
		}
	}
	return matcher.Card{}
}

func catalogProduct(id, group int, name, number string) tcgplayer.Product {
	return tcgplayer.Product{
		ProductID:    id,
		GroupID:      group,
		Name:         name,
		ExtendedData: []tcgplayer.ExtendedField{{Name: "Number", Value: number}},
		Skus: []tcgplayer.SKU{
			{SKUID: id*10 + 1, ProductID: id, LanguageID: 1, PrintingID: 1, ConditionID: 1},
			{SKUID: id*10 + 2, ProductID: id, LanguageID: 1, PrintingID: 2, ConditionID: 2},
			{SKUID: id*10 + 3, ProductID: id, LanguageID: 7, PrintingID: 1, ConditionID: 1},
		},
	}
}

// TestCatalogLinks pins when a product the datastore names no card for is
// tied to one: only when the product is nobody's, the card has no product,
// the product's group is named for the card's set, and each is the other's
// only candidate.
func TestCatalogLinks(t *testing.T) {
	b := realDatastore(t)
	cards := unlinkedCards(b, 4)
	if len(cards) < 4 {
		t.Skip("the datastore gives a product to nearly every printing")
	}
	priced := linkedCard(b, "tcgplayerProductId")
	etched := linkedCard(b, "tcgplayerEtchedProductId")
	if etched.Name == "" {
		t.Skip("the datastore has no printing sold etched only")
	}
	claimedID, _ := strconv.Atoi(priced.Identifiers["tcgplayerProductId"])

	setName := func(card matcher.Card) string {
		set, _ := b.GetSet(card.SetCode)
		return set.Name
	}
	dump := tcgplayer.CatalogDump{
		Conditions: []tcgplayer.Condition{{ConditionID: 1, Name: "Near Mint"}, {ConditionID: 2, Name: "Lightly Played"}},
		Printings:  []tcgplayer.Printing{{PrintingID: 1, Name: "Normal"}, {PrintingID: 2, Name: "Foil"}},
		Groups: []tcgplayer.Group{
			{GroupID: 1, Name: "Promo Pack: " + setName(cards[0])},
			{GroupID: 2, Name: setName(cards[1])},
			{GroupID: 3, Name: setName(cards[2])},
			{GroupID: 4, Name: "Not " + setName(cards[3])},
			{GroupID: 5, Name: setName(priced)},
			{GroupID: 6, Name: setName(etched)},
		},
		Products: []tcgplayer.Product{
			// A group named for the set after a prefix
			catalogProduct(900000001, 1, cards[0].Name, cards[0].Number),
			// The set's own group, the number zero-padded
			catalogProduct(900000002, 2, cards[1].Name, "000"+cards[1].Number),
			// Two products for one card tie neither
			catalogProduct(900000003, 3, cards[2].Name, cards[2].Number),
			catalogProduct(900000004, 3, cards[2].Name, cards[2].Number),
			// A group that is not the set's
			catalogProduct(900000005, 4, cards[3].Name, cards[3].Number),
			// A card with a product of its own takes no other
			catalogProduct(900000006, 5, priced.Name, priced.Number),
			// Nor does one sold etched only
			catalogProduct(900000007, 6, etched.Name, etched.Number),
			// A product a card claims is not up for linking
			catalogProduct(claimedID, 1, cards[0].Name, cards[0].Number),
		},
	}

	links := catalogLinks(b, &dump)

	want := map[string]int{
		cards[0].Identifiers["mtgjsonId"]: 900000001,
		cards[1].Identifiers["mtgjsonId"]: 900000002,
	}
	if len(links) != len(want) {
		t.Errorf("got %d links, want %d: %v", len(links), len(want), links)
	}
	for uuid, id := range want {
		skus := links[uuid]
		if len(skus) != 2 {
			t.Fatalf("%s: got %d skus, want the 2 English ones: %v", uuid, len(skus), skus)
		}
		for _, sku := range skus {
			if sku.ProductID != id || sku.Language != "ENGLISH" {
				t.Errorf("%s: got %+v, want product %d in English", uuid, sku, id)
			}
		}
		if skus[0].Condition != "NEAR MINT" || skus[0].Printing != "NON FOIL" ||
			skus[1].Condition != "LIGHTLY PLAYED" || skus[1].Printing != "FOIL" {
			t.Errorf("%s: skus read as %+v", uuid, skus)
		}
	}

	// The index prices the linked product's rows as the card's own
	co, _ := b.GetUUID(cards[0].Identifiers["mtgjsonId"])
	printings := productPrintings(b, nil, links)
	for subtype, finish := range map[string]string{"Normal": matcher.FinishNonfoil, "Foil": matcher.FinishFoil} {
		got := printings["900000001"][subtype]
		if got != co.FoilUUIDs[finish] {
			t.Errorf("product 900000001 %s prices %q, want %q", subtype, got, co.FoilUUIDs[finish])
		}
	}
}

// TestSKUMapWithLinks pins that links only fill cards the sku catalog has
// nothing for, and that neither map is changed.
func TestSKUMapWithLinks(t *testing.T) {
	skus := SKUMap{"a": {{SkuID: 1}}}
	links := SKUMap{"a": {{SkuID: 2}}, "b": {{SkuID: 3}}}

	merged := skus.WithLinks(links)

	if merged["a"][0].SkuID != 1 || merged["b"][0].SkuID != 3 || len(merged) != 2 {
		t.Errorf("merged is %v", merged)
	}
	if len(skus) != 1 || len(links) != 2 {
		t.Errorf("inputs changed: %v %v", skus, links)
	}
}

// TestLoadCatalogLinksRefusesEmpty pins that a dump naming nothing is an
// error rather than a run that links nothing and reports success.
func TestLoadCatalogLinksRefusesEmpty(t *testing.T) {
	_, err := LoadCatalogLinks(&matcher.Backend{}, strings.NewReader(`{"products": []}`))
	if err == nil {
		t.Error("an empty dump was accepted")
	}
}
