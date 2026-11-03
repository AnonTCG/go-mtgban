package gamenerdz

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
)

// The LP copies of the two cards in untitledVariantDatastore, each the
// product of its own the storefront lists a grade as.
const (
	playedHancockProduct = `{
 "product_id": 183401,
 "display_name": "Boa Hancock (SP) (OP01-078) Kingdoms of Intrigue Foil (LP)",
 "price": 963.21,
 "selectedFinish": "Foil",
 "product_data": {"rarity": "Super Rare", "set": "Kingdoms of Intrigue", "setName": "Kingdoms of Intrigue"},
 "store_pass_variant_info": [{"id": 181039, "offer_price": 602.01, "selected_finish": "Foil"}]
}`
	playedLuffyProduct = `{
 "product_id": 358458,
 "display_name": "Monkey.D.Luffy (Welcome Pack 2026 Vol.1) (OP10-111) One Piece Promotion Cards (LP)",
 "price": 29.22,
 "selectedFinish": "Normal",
 "product_data": {"rarity": "Rare", "set": "One Piece Promotion Cards", "setName": "One Piece Promotion Cards"},
 "store_pass_variant_info": [{"id": 314672, "offer_price": 12.26, "selected_finish": "Normal"}]
}`
)

// TestDropUnbought pins that a card bought only at a played grade is dropped,
// and one bought near mint keeps its played offers too.
func TestDropUnbought(t *testing.T) {
	b, err := matcher.Open("onepiece", strings.NewReader(untitledVariantDatastore))
	if err != nil {
		t.Fatal(err)
	}
	gn, err := NewScraper(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{playedHancockProduct, noTitleProduct, playedLuffyProduct} {
		var product GNProduct
		err := json.Unmarshal([]byte(raw), &product)
		if err != nil {
			t.Fatal(err)
		}
		err = gn.processProduct(modeBuylist, product)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(gn.Buylist()["op01-078_516555_foil"]) != 1 {
		t.Fatalf("the LP Hancock was not recorded: %v", gn.Buylist())
	}

	gn.dropUnbought()

	if _, found := gn.Buylist()["op01-078_516555_foil"]; found {
		t.Error("Hancock, bought only at LP, is still in the buylist")
	}
	entries := gn.Buylist()["op10-111_693122"]
	if len(entries) != 2 || entries[0].Conditions != mtgban.NM || entries[1].Conditions != mtgban.SP {
		t.Errorf("Luffy has %v; want an NM and an SP offer", entries)
	}
}
