package tcgplayer

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/mtgban/go-tcgplayer"
)

func TestLoadCatalogRefusesAnEmptyDump(t *testing.T) {
	_, err := LoadCatalog(strings.NewReader(`{"category": {"categoryId": 86}, "products": []}`))
	if err == nil {
		t.Error("LoadCatalog() of a dump with no products error = nil, want it refused")
	}
}

func TestCatalogProducts(t *testing.T) {
	dump := &tcgplayer.CatalogDump{Products: []tcgplayer.Product{
		{ProductID: 1, ProductType: "Cards"},
		{ProductID: 2, ProductType: "Sealed Products"},
		{ProductID: 3, ProductType: "Booster Box"},
		// Listed once per type it is filed under
		{ProductID: 3, ProductType: "Sealed Products"},
	}}
	for _, tt := range []struct {
		types []string
		want  []int
	}{
		{[]string{"Cards"}, []int{1}},
		{[]string{"Sealed Products", "Booster Box"}, []int{2, 3}},
	} {
		products, err := catalogProducts(dump, tt.types)
		if err != nil {
			t.Errorf("catalogProducts(%q) error = %v", tt.types, err)
		}
		var got []int
		for _, product := range products {
			got = append(got, product.ProductID)
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("catalogProducts(%q) = %v, want %v", tt.types, got, tt.want)
		}
	}
}

func TestCatalogProductsRefusesNoneOfTheTypes(t *testing.T) {
	// A dump written without product types
	dump := &tcgplayer.CatalogDump{Products: []tcgplayer.Product{{ProductID: 1}}}
	products, err := catalogProducts(dump, []string{"Cards"})
	if err == nil {
		t.Errorf("catalogProducts() = %d products and no error, want the dump refused", len(products))
	}
}

// gameCatalogDump is the Gundam category's dump of gameProducts.
func gameCatalogDump() *tcgplayer.CatalogDump {
	return &tcgplayer.CatalogDump{
		Category:  tcgplayer.Category{CategoryID: tcgplayer.CategoryGundam, Name: "Gundam Card Game"},
		Printings: []tcgplayer.Printing{{PrintingID: 168, Name: "Normal"}, {PrintingID: 169, Name: "Holofoil"}},
		Languages: []tcgplayer.Language{{LanguageID: 1, Name: "English"}, {LanguageID: 7, Name: "Japanese"}},
		Products:  gameProducts,
	}
}

// servePricing stands in for the pricing API, and fails the test on any
// other call: the catalog is all the scrapers read the products from.
// Every sku lists at 1 with a market price of 2; every product is priced
// at 3 in the finish subtypes names.
func servePricing(t *testing.T, subtypes map[string]string) *tcgplayer.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"access_token": "t", "token_type": "bearer", "expires_in": 86400}`)
	})
	mux.HandleFunc("/pricing/sku/{ids}", func(w http.ResponseWriter, r *http.Request) {
		var rows []string
		for _, id := range strings.Split(r.PathValue("ids"), ",") {
			rows = append(rows, fmt.Sprintf(`{"skuId": %s, "lowPrice": 1, "lowestListingPrice": 1, "marketPrice": 2}`, id))
		}
		fmt.Fprintf(w, `{"success": true, "errors": [], "results": [%s]}`, strings.Join(rows, ","))
	})
	mux.HandleFunc("/pricing/product/{ids}", func(w http.ResponseWriter, r *http.Request) {
		var rows []string
		for _, id := range strings.Split(r.PathValue("ids"), ",") {
			rows = append(rows, fmt.Sprintf(`{"productId": %s, "lowPrice": 3, "subTypeName": %q}`, id, subtypes[id]))
		}
		fmt.Fprintf(w, `{"success": true, "errors": [], "results": [%s]}`, strings.Join(rows, ","))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL.Path)
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	token, sku, product := tcgplayer.TokenURL, tcgplayer.PricingSKUURL, tcgplayer.PricingProductURL
	t.Cleanup(func() { tcgplayer.TokenURL, tcgplayer.PricingSKUURL, tcgplayer.PricingProductURL = token, sku, product })
	tcgplayer.TokenURL = srv.URL + "/token"
	tcgplayer.PricingSKUURL = srv.URL + "/pricing/sku"
	tcgplayer.PricingProductURL = srv.URL + "/pricing/product"

	client, err := tcgplayer.NewClient("k", "k")
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestGameLoadPricesTheCatalog(t *testing.T) {
	for _, tt := range []struct {
		desc   string
		sealed bool
		want   []string
	}{
		{"singles", false, []string{
			"Gundam (MA Form) ST01-002: TCGplayer NM 1.00",
			"Gundam (MA Form) ST01-002: TCGplayer SP 1.00",
			"Gundam (MA Form) ST01-002: TCGplayer Trends NM 2.00",
			"Gundam (MA Form) ST01-002: TCGplayer Trends SP 2.00",
			"Gundam ST01-001 foil: TCGplayer HP 1.00",
			"Gundam ST01-001 foil: TCGplayer MP 1.00",
			"Gundam ST01-001 foil: TCGplayer NM 1.00",
			"Gundam ST01-001 foil: TCGplayer PO 1.00",
			"Gundam ST01-001 foil: TCGplayer SP 1.00",
			"Gundam ST01-001 foil: TCGplayer Trends HP 2.00",
			"Gundam ST01-001 foil: TCGplayer Trends MP 2.00",
			"Gundam ST01-001 foil: TCGplayer Trends NM 2.00",
			"Gundam ST01-001 foil: TCGplayer Trends PO 2.00",
			"Gundam ST01-001 foil: TCGplayer Trends SP 2.00",
		}},
		{"sealed", true, []string{
			"Gundam Card Game Edition Beta Box: TCGplayer NM 1.00",
		}},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			tcg, _ := gameScraper(t, tt.sealed)
			if tt.sealed {
				tcg.productTypes = tcgplayer.SealedProductTypes(tcg.category)
			}
			tcg.client = servePricing(t, nil)
			tcg.catalog = gameCatalogDump()
			err := tcg.Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for key, entries := range tcg.Inventory() {
				for _, entry := range entries {
					entry.URL = ""
					got = append(got, describe(tcg.backend, genericChan{key: key, entry: entry}))
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestGameIndexLoadPricesTheCatalog(t *testing.T) {
	tcg, _ := gameScraper(t, false)
	index, err := NewScraperGameIndex(tcg.backend, "public", "private")
	if err != nil {
		t.Fatal(err)
	}
	index.client = servePricing(t, map[string]string{"616528": "Holofoil", "616532": "Normal"})
	index.catalog = gameCatalogDump()
	err = index.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for key, entries := range index.Inventory() {
		for _, entry := range entries {
			entry.URL = ""
			got = append(got, describe(index.backend, genericChan{key: key, entry: entry}))
		}
	}
	slices.Sort(got)
	want := []string{
		"Gundam (MA Form) ST01-002: TCG Low NM 3.00",
		"Gundam ST01-001 foil: TCG Low NM 3.00",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if !slices.Equal(index.printings, []string{"Normal", "Holofoil"}) {
		t.Errorf("printings %q, want the catalog's", index.printings)
	}
}

func TestGameLoadRefusesAnotherCategory(t *testing.T) {
	tcg, _ := gameScraper(t, false)
	tcg.client = servePricing(t, nil)
	tcg.catalog = gameCatalogDump()
	tcg.catalog.Category.CategoryID = tcgplayer.CategoryMagic
	err := tcg.Load(context.Background())
	if err == nil {
		t.Error("Load() over Magic's catalog error = nil, want it refused")
	}
}
