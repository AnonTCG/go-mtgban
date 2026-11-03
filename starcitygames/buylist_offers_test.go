package starcitygames

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/mtgban"

	_ "github.com/mtgban/go-mtgban/matcher/fleshandblood"
)

// sellListServer answers the index the way MeiliSearch does: facet counts for
// a query with no limit, and each filter's products a page at a time.
func sellListServer(t *testing.T, facets map[string]map[string]map[string]int, hits map[string][]map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Limit  int      `json:"limit"`
			Offset int      `json:"offset"`
			Filter string   `json:"filter"`
			Facets []string `json:"facets"`
		}
		err := json.NewDecoder(r.Body).Decode(&body)
		if err != nil {
			t.Errorf("decoding a search: %v", err)
		}
		out := map[string]any{"hits": []any{}}
		if body.Limit == 0 {
			dist := map[string]map[string]int{}
			for _, facet := range body.Facets {
				dist[facet] = facets[body.Filter][facet]
			}
			out["facetDistribution"] = dist
			out["estimatedTotalHits"] = len(hits[body.Filter])
		} else {
			page := hits[body.Filter]
			end := min(body.Offset+body.Limit, len(page))
			if body.Offset < end {
				out["hits"] = page[body.Offset:end]
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
}

func sellListHit(sku string, variants ...[3]any) map[string]any {
	var vs []map[string]any
	for i, v := range variants {
		vs = append(vs, map[string]any{
			"sku": fmt.Sprintf("%s%d", sku, i+1), "variant_value": v[0], "status": v[1], "buy_price": v[2],
		})
	}
	return map[string]any{"variants": vs}
}

// TestSellListOffers covers the crawl: a slice the index cannot page whole is
// split again, a product with no finish is still read, only a variant's own
// bid counts, a product with no NM bid is not bought at all, and a crawl that
// comes back short is refused rather than published as the whole buylist.
func TestSellListOffers(t *testing.T) {
	const base = "game_id = 2 AND product_type_id = 1 AND primary_status IN [buying_at_cost, hotlist]"
	facets := map[string]map[string]map[string]int{
		base:                     {"finish": {"1": sellListCap, "2": 2}},
		base + " AND finish = 1": {"rarity": {"3": 1, "4": 1}},
	}
	hits := map[string][]map[string]any{
		base + " AND finish = 2": {
			sellListHit("SGL-FAB-A-ENN",
				[3]any{"NM", "buying_at_cost", 4}, [3]any{"PL", "buying_at_cost", 2.5}, [3]any{"HP", "buying_at_cost", 1}),
			// The Tournament Pack cold foil Cut to the Chase (Red) on
			// 2026-11-02, which the storefront does not buy.
			sellListHit("SGL-FAB-E-ENN",
				[3]any{"PL", "buying_at_cost", 25}, [3]any{"HP", "buying_at_cost", 12}),
		},
		base + " AND finish = 1 AND rarity = 3": {sellListHit("SGL-FAB-B-ENN",
			[3]any{"NM", "hotlist", 10}, [3]any{"PL", "buying_in_bulk", 0.05}, [3]any{"HP", "buying_at_cost", 0.005})},
		base + " AND finish = 1 AND rarity = 4": {sellListHit("SGL-FAB-C-ENN",
			[3]any{"NM", "buying_at_cost", 3}, [3]any{"PL", "not_buying", 2}, [3]any{"HP", "do_not_show", 1})},
		base + " AND (finish IS EMPTY OR finish IS NULL OR finish NOT EXISTS)": {sellListHit("SGL-FAB-D-ENN",
			[3]any{"NM", "buying_at_cost", 7})},
	}
	srv := sellListServer(t, facets, hits)
	defer srv.Close()

	scg := NewSCGClient("")
	scg.sellListURL = srv.URL
	got, products, err := scg.sellListOffers(context.Background(), GameFleshAndBlood)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]map[mtgban.Condition]float64{
		"SGL-FAB-A-ENN": {mtgban.NM: 4, mtgban.SP: 2.5, mtgban.HP: 1},
		"SGL-FAB-B-ENN": {mtgban.NM: 10},
		"SGL-FAB-C-ENN": {mtgban.NM: 3},
		"SGL-FAB-D-ENN": {mtgban.NM: 7},
	}
	if len(got) != len(want) {
		t.Errorf("got %d products, want %d: %v", len(got), len(want), got)
	}
	for sku, grades := range want {
		prices := map[mtgban.Condition]float64{}
		for grade, offer := range got[sku] {
			prices[grade] = offer.BuyPrice
		}
		if !maps.Equal(prices, grades) {
			t.Errorf("%s: got %v, want %v", sku, prices, grades)
		}
	}
	// The products feed the singles only the sell list carries, so one not
	// bought leaves them too.
	for sku := range want {
		if _, found := products[sku]; !found {
			t.Errorf("%s is missing from the sell list's products", sku)
		}
	}
	if _, found := products["SGL-FAB-E-ENN"]; found {
		t.Error("SGL-FAB-E-ENN, with no NM bid, is still among the sell list's products")
	}
	if got["SGL-FAB-A-ENN"][mtgban.SP].SKU != "SGL-FAB-A-ENN2" {
		t.Errorf("the PL offer carries sku %q, want its own variant's", got["SGL-FAB-A-ENN"][mtgban.SP].SKU)
	}

	delete(hits, base+" AND finish = 1 AND rarity = 4")
	_, _, err = scg.sellListOffers(context.Background(), GameFleshAndBlood)
	if err == nil || !strings.Contains(err.Error(), "4 of the 5") {
		t.Errorf("a crawl missing a product was not refused: %v", err)
	}
}

// TestBuylistFromOffers covers what processProduct files from the index: every
// grade SCG bids on, whether or not the catalog lists that variant, with HP's
// price at MP too, since SCG grades no MP. The listing is the one the catalog
// carried on 2026-11-01 with only Near Mint and Heavily Played variants.
func TestBuylistFromOffers(t *testing.T) {
	b := withGameDatastore(t, "fleshandblood", "FLESHANDBLOOD_PATH")

	const sku = "SGL-FAB-MON1-108-ENC"
	product := CatalogProduct{
		SKU: sku, Name: "Gallantry Gold", Game: "Flesh and Blood",
		Set: "Monarch (1st Edition)", ProductType: ProductTypeSingles,
		CollectorNumber: "108", Finish: "Cold Foil", FinishGroup: "Alt Foil",
		Language: "English", Rarity: "Common", URL: "/gallantry-gold-sgl-fab-mon1-108-enc/",
		Variants: []CatalogVariant{
			{SKU: sku + "1", Condition: "Near Mint", Qty: 1, Price: "2.00"},
			{SKU: sku + "3", Condition: "Heavily Played", Qty: 1, Price: "1.00"},
		},
	}
	offer := func(digit string, price float64) sellListVariant {
		return sellListVariant{SKU: sku + digit, BuyPrice: price}
	}

	for _, tt := range []struct {
		desc   string
		offers map[mtgban.Condition]sellListVariant
		want   map[mtgban.Condition]float64
	}{
		{"every grade the index bids on, MP at HP's price",
			map[mtgban.Condition]sellListVariant{mtgban.NM: offer("1", 0.5), mtgban.SP: offer("2", 0.3), mtgban.HP: offer("3", 0.1)},
			map[mtgban.Condition]float64{mtgban.NM: 0.5, mtgban.SP: 0.3, mtgban.MP: 0.1, mtgban.HP: 0.1}},
		{"no MP without an HP bid",
			map[mtgban.Condition]sellListVariant{mtgban.NM: offer("1", 0.5), mtgban.SP: offer("2", 0.3)},
			map[mtgban.Condition]float64{mtgban.NM: 0.5, mtgban.SP: 0.3}},
		{"nothing where the index has no bid", nil, map[mtgban.Condition]float64{}},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			scg, err := NewScraper(b, "")
			if err != nil {
				t.Fatal(err)
			}
			scg.logCallback = func(format string, a ...any) { t.Logf(format, a...) }
			scg.offers = map[string]map[mtgban.Condition]sellListVariant{sku: tt.offers}
			scg.processProduct(product)

			got := map[mtgban.Condition]float64{}
			for _, entries := range scg.buylist {
				for _, entry := range entries {
					got[entry.Conditions] = entry.BuyPrice
					if entry.Conditions == mtgban.SP && entry.InstanceID != sku+"2" {
						t.Errorf("SP entry has instance %q, want the PL variant's sku", entry.InstanceID)
					}
					if entry.Conditions == mtgban.NM && entry.PriceRatio != 25 {
						t.Errorf("NM price ratio = %v, want 25", entry.PriceRatio)
					}
					if entry.Conditions == mtgban.SP && entry.PriceRatio != 0 {
						t.Errorf("SP price ratio = %v with no Played retail price", entry.PriceRatio)
					}
				}
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
			if len(scg.inventory) != 1 {
				t.Errorf("retail has %d cards, want the listing's one", len(scg.inventory))
			}
		})
	}
}
