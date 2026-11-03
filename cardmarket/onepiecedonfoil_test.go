package cardmarket

import (
	"errors"
	"testing"

	cm "github.com/mtgban/go-cardmarket"

	"github.com/mtgban/go-mtgban/matcher"
)

// onePieceDonDatastore is the published datastore's three Zoro DON!! cards
// of The Best, copied verbatim: TCGplayer's one product in both finishes,
// and the gold one of its own.
const onePieceDonDatastore = `{"data": {
 "game": "onepiece",
 "sets": {"PRB-01": {"name": "Premium Booster -The Best-", "releaseDate": "2024-11-08"}},
 "cards": [
  {"color": "", "externalLinks": {"tcgPlayerId": 587192}, "finish": "Foil", "id": "587192_foil", "name": "DON!! Card", "promoTypes": ["gold"], "rarity": "DON!!", "setCode": "PRB-01", "type": "DON!!", "variant": "Zoro Gold", "watermark": "zoro"},
  {"color": "", "externalLinks": {"tcgPlayerId": 593830}, "finish": "Normal", "id": "593830", "name": "DON!! Card", "rarity": "DON!!", "setCode": "PRB-01", "type": "DON!!", "variant": "Zoro", "watermark": "zoro"},
  {"color": "", "externalLinks": {"tcgPlayerId": 593830}, "finish": "Foil", "id": "593830_foil", "name": "DON!! Card", "rarity": "DON!!", "setCode": "PRB-01", "type": "DON!!", "variant": "Zoro", "watermark": "zoro"}
 ]
}}`

// TestOnePieceDonFoilVersion pins that the V.2 of a The Best DON!! is its
// foil: CardTrader's one blueprint links the V.1 and V.2 to one TCGplayer
// product. The V.1 then hands no foil of its own to price. The products and
// links are the catalog's and CardTrader's own.
func TestOnePieceDonFoilVersion(t *testing.T) {
	b := datastoreBackend(t, "onepiece", onePieceDonDatastore)

	mkm, err := NewScraperIndex(b)
	if err != nil {
		t.Fatalf("NewScraperIndex(b) = %v", err)
	}
	mkm.tcgBridge = map[int]int{799541: 593830, 799542: 593830, 799543: 587192}

	exp := cm.Expansion{IDExpansion: 5805, Name: "The Best"}
	products := map[int]cm.CatalogProduct{
		799541: {ExpansionID: 5805, Name: "Don!! (PRB Zoro) (V.1)", Rarity: "DON!!", Version: 1},
		799542: {ExpansionID: 5805, Name: "Don!! (PRB Zoro) (V.2)", Rarity: "DON!!", Version: 2},
		799543: {ExpansionID: 5805, Name: "Don!! (PRB Zoro) (V.3)", Rarity: "DON!!", Version: 3},
	}
	mkm.resolver.claimByID(map[int][]int{5805: {799541, 799542, 799543}}, products, []cm.Expansion{exp})

	for id, want := range map[int]string{
		799541: "593830",
		799542: "593830_foil",
		799543: "587192_foil",
	} {
		got := mkm.resolver.resolveMapped(id, products[id], exp)
		if got.err != nil || got.cardID != want {
			t.Errorf("%d %q: resolveMapped = (%q, %v), want %q", id, products[id].Name, got.cardID, got.err, want)
		}
		if got.cardIDFoil == "593830_foil" && id != 799542 {
			t.Errorf("%d %q: cardIDFoil = %q, the V.2 sells it", id, products[id].Name, got.cardIDFoil)
		}
	}
}

// TestOnePieceDonAliasing pins that an event DON!! the datastore does not
// carry, which aliases across its DON!! rows, answers the matcher's own
// error rather than a refusal.
func TestOnePieceDonAliasing(t *testing.T) {
	b := datastoreBackend(t, "onepiece", onePieceDonDatastore)

	mkm, err := NewScraperIndex(b)
	if err != nil {
		t.Fatalf("NewScraperIndex(b) = %v", err)
	}
	exp := cm.Expansion{IDExpansion: 5244, Name: "One Piece Products"}
	product := cm.CatalogProduct{ExpansionID: 5244, Name: "Don!! (Elbaph)", Rarity: "DON!!"}

	got := mkm.resolver.resolveMapped(858544, product, exp)
	var alias *matcher.AliasingError
	if !errors.As(got.err, &alias) || got.cardID != "" {
		t.Errorf("resolveMapped = (%q, %v), want an aliasing error", got.cardID, got.err)
	}
}
