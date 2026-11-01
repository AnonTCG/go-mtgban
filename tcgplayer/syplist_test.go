package tcgplayer

import (
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
	"github.com/mtgban/go-tcgplayer"
)

// TestNewScraperSYPGames pins which games the list is read for and the
// category each one names. The list is served per category, so a wrong
// number here reads someone else's SYP list rather than failing.
func TestNewScraperSYPGames(t *testing.T) {
	for _, tt := range []struct {
		game     matcher.Game
		category int
		wantErr  bool
	}{
		// Magic is the zero value of the game field, so it is also what an
		// unset game asks for - that is the one name this map must hold.
		{game: matcher.GameMagic, category: tcgplayer.CategoryMagic},
		{game: matcher.GamePokemon, category: tcgplayer.CategoryPokemon},
		// A game the list is not read for is refused rather than fetching a
		// category whose rows nothing can resolve.
		{game: matcher.GameLorcana, wantErr: true},
		{game: matcher.GameOnePiece, wantErr: true},
	} {
		name := string(tt.game)
		if tt.game == matcher.GameMagic {
			name = "Magic"
		}
		t.Run(name, func(t *testing.T) {
			b := &matcher.Backend{Game: tt.game}
			scraper, err := NewScraperSYP(b, "auth")
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewScraperSYP(%q) was accepted, want refused", tt.game)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if scraper.category != tt.category {
				t.Errorf("category is %d, want %d", scraper.category, tt.category)
			}
			if got := scraper.Info().Game; got != tt.game {
				t.Errorf("Info().Game is %q, want %q", got, tt.game)
			}
		})
	}
}

// TestInfoNamesTheGame pins that every TCGplayer scraper reports the game of
// the datastore it was built on.
func TestInfoNamesTheGame(t *testing.T) {
	b := &matcher.Backend{Game: matcher.GameMagic}
	market, err := NewScraperGame(b, "public", "private")
	if err != nil {
		t.Fatal(err)
	}
	index, err := NewScraperGameIndex(b, "public", "private")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := NewScraperGameSealed(b, "public", "private")
	if err != nil {
		t.Fatal(err)
	}
	seller := NewScraperForSellerIDs(b, []string{"seller"}, false)
	for name, info := range map[string]mtgban.ScraperInfo{
		"market": market.Info(),
		"index":  index.Info(),
		"sealed": sealed.Info(),
		"seller": seller.Info(),
	} {
		if info.Game != matcher.GameMagic {
			t.Errorf("%s Info().Game is %q, want %q", name, info.Game, matcher.GameMagic)
		}
	}
}

// TestSYPCatalogFromDump pins the step between the list and the datastore:
// a sku id names a product, and the printing it is sold in comes with it.
// Only the English Near Mint skus are kept, which is all Direct hosts and a
// small part of what a dump carries.
func TestSYPCatalogFromDump(t *testing.T) {
	const dump = `{
	  "category": {"categoryId": 1},
	  "printings": [
	    {"printingId": 1, "name": "Normal"},
	    {"printingId": 2, "name": "Foil"}
	  ],
	  "products": [
	    {"productId": 100, "name": "Counterspell", "skus": [
	      {"skuId": 11, "productId": 100, "languageId": 1, "printingId": 1, "conditionId": 1},
	      {"skuId": 12, "productId": 100, "languageId": 1, "printingId": 2, "conditionId": 1},
	      {"skuId": 13, "productId": 100, "languageId": 1, "printingId": 1, "conditionId": 3},
	      {"skuId": 14, "productId": 100, "languageId": 2, "printingId": 1, "conditionId": 1}
	    ]},
	    {"productId": 200, "name": "Kudo, King Among Bears (Foil Etched)", "skus": [
	      {"skuId": 21, "productId": 200, "languageId": 1, "printingId": 2, "conditionId": 1}
	    ]}
	  ]
	}`

	catalog, err := LoadSYPCatalog(strings.NewReader(dump))
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		desc      string
		skuID     int
		wantFound bool
		productID int
		finish    string
	}{
		{"a plain near mint sku names its product", 11, true, 100, "Normal"},
		{"the foil sibling is a sku of its own", 12, true, 100, "Foil"},
		{"etched is read off the title, which is the only place it is said", 21, true, 200, matcher.FinishEtched},
		{"a played grade is not hosted", 13, false, 0, ""},
		{"nor is another language", 14, false, 0, ""},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			sku, found := catalog[tt.skuID]
			if found != tt.wantFound {
				t.Fatalf("sku %d found = %v, want %v", tt.skuID, found, tt.wantFound)
			}
			if !found {
				return
			}
			if sku.ProductID != tt.productID || sku.Finish != tt.finish {
				t.Errorf("sku %d -> product %d finish %q, want product %d finish %q",
					tt.skuID, sku.ProductID, sku.Finish, tt.productID, tt.finish)
			}
		})
	}
}

// TestLoadSYPCatalogRefusesEmpty pins that a dump naming nothing is an error
// rather than a run that prices nothing and reports success.
func TestLoadSYPCatalogRefusesEmpty(t *testing.T) {
	if _, err := LoadSYPCatalog(strings.NewReader(`{"products": []}`)); err == nil {
		t.Error("an empty dump was accepted")
	}
	if _, err := LoadSYPCatalog(strings.NewReader(
		`{"printings":[{"printingId":1,"name":"Normal"}],"products":[{"productId":1,"name":"x","skus":[{"skuId":1,"productId":1,"languageId":2,"conditionId":1,"printingId":1}]}]}`,
	)); err == nil {
		t.Error("a dump with no English near mint sku was accepted")
	}
}
