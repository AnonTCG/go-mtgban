package tcgplayer

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-tcgplayer"

	_ "github.com/mtgban/go-mtgban/matcher/gundam"
)

// gameFixture is the published Gundam datastore's rows of 2026-10-02 for
// three Edition Beta singles and the set's booster box.
const gameFixture = `{"data": {"game": "gundam",
	"sets": {"GD01-B": {"name": "Edition Beta", "releaseDate": "2025-02-06"}},
	"cards": [
		{"color": "Blue", "externalLinks": {"tcgPlayerId": 616528}, "finish": "Holofoil", "id": "st01-001_616528_holofoil", "image": "https://tcgplayer-cdn.tcgplayer.com/product/616528_400w.jpg", "name": "Gundam", "number": "ST01-001", "rarity": "Legend Rare", "setCode": "GD01-B", "type": "Unit"},
		{"color": "Blue", "externalLinks": {"tcgPlayerId": 616531}, "finish": "Holofoil", "id": "st01-001_616531_holofoil", "image": "https://tcgplayer-cdn.tcgplayer.com/product/616531_400w.jpg", "name": "Gundam", "number": "ST01-001", "rarity": "LR+", "setCode": "GD01-B", "type": "Unit"},
		{"color": "Blue", "externalLinks": {"tcgPlayerId": 616532}, "finish": "Normal", "id": "st01-002_616532", "image": "https://tcgplayer-cdn.tcgplayer.com/product/616532_400w.jpg", "name": "Gundam (MA Form)", "number": "ST01-002", "rarity": "Common", "setCode": "GD01-B", "type": "Unit"}
	],
	"sealed": [
		{"externalLinks": {"tcgPlayerId": 617436}, "id": "gd01-b-617436", "image": "https://tcgplayer-cdn.tcgplayer.com/product/617436_400w.jpg", "name": "Gundam Card Game Edition Beta Box", "releaseDate": "2025-02-06", "setCode": "GD01-B"}
	]
}}`

// gameProducts are TCGplayer's catalog rows for those products on
// 2026-10-02, but for the last sku of the second: a Japanese copy of its
// near mint sku, which the catalog does not list.
var gameProducts = []tcgplayer.Product{
	{ProductID: 616528, Name: "Gundam", ProductType: "Cards", Skus: []tcgplayer.SKU{
		{SKUID: 8560578, ProductID: 616528, LanguageID: 1, PrintingID: 169, ConditionID: 1},
		{SKUID: 8560579, ProductID: 616528, LanguageID: 1, PrintingID: 169, ConditionID: 2},
		{SKUID: 8560580, ProductID: 616528, LanguageID: 1, PrintingID: 169, ConditionID: 3},
		{SKUID: 8560581, ProductID: 616528, LanguageID: 1, PrintingID: 169, ConditionID: 4},
		{SKUID: 8560582, ProductID: 616528, LanguageID: 1, PrintingID: 169, ConditionID: 5},
	}},
	{ProductID: 616532, Name: "Gundam (MA Form)", ProductType: "Cards", Skus: []tcgplayer.SKU{
		{SKUID: 8560649, ProductID: 616532, LanguageID: 1, PrintingID: 168, ConditionID: 1},
		{SKUID: 8560650, ProductID: 616532, LanguageID: 1, PrintingID: 168, ConditionID: 2},
		{SKUID: 1, ProductID: 616532, LanguageID: 7, PrintingID: 168, ConditionID: 1},
	}},
	{ProductID: 617436, Name: "Gundam Card Game Edition Beta Box", ProductType: "Sealed Products", Skus: []tcgplayer.SKU{
		{SKUID: 8569425, ProductID: 617436, LanguageID: 1, PrintingID: 168, ConditionID: 6},
	}},
}

// gameScraper builds a scraper over gameFixture that has made no request:
// its printings are the category's, as TCGplayer lists them.
func gameScraper(t *testing.T, sealed bool) (*TCGGame, *[]string) {
	t.Helper()
	b, err := matcher.Open(matcher.GameGundam, strings.NewReader(gameFixture))
	if err != nil {
		t.Fatal(err)
	}
	tcg, err := NewScraperGame(b, "public", "private")
	if err != nil {
		t.Fatal(err)
	}
	if sealed {
		tcg.sealed = true
		tcg.sealedMap = b.BuildSealedProductMap("tcgplayerProductId")
	}
	tcg.printings = map[int]string{168: "Normal", 169: "Holofoil"}
	var logs []string
	tcg.logCallback = func(format string, a ...any) {
		logs = append(logs, fmt.Sprintf(format, a...))
	}
	return tcg, &logs
}

// describe names what an entry landed on, and how it is priced.
func describe(b *matcher.Backend, out genericChan) string {
	what := out.key
	co, err := b.GetUUID(out.key)
	if err == nil {
		what = co.Name
		if !co.Sealed {
			what += " " + co.Number
		}
		if co.Foil {
			what += " foil"
		}
	}
	fields := []string{what + ":", out.entry.SellerName, string(out.entry.Conditions), fmt.Sprintf("%.2f", out.entry.Price), out.entry.URL}
	return strings.Join(slices.DeleteFunc(fields, func(field string) bool { return field == "" }), " ")
}

func TestGameSKUsToPrice(t *testing.T) {
	for _, tt := range []struct {
		desc   string
		sealed bool
		want   []int
	}{
		{"singles: every English sku in a grade", false, []int{8560578, 8560579, 8560580, 8560581, 8560582, 8560649, 8560650}},
		{"sealed: the unopened sku alone", true, []int{8569425}},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			tcg, _ := gameScraper(t, tt.sealed)
			productMap, skuMap, skuIDs := tcg.skusToPrice(gameProducts)
			if !slices.Equal(skuIDs, tt.want) {
				t.Errorf("skus %v, want %v", skuIDs, tt.want)
			}
			for _, id := range skuIDs {
				_, found := productMap[skuMap[id].ProductID]
				if !found {
					t.Errorf("sku %d has no product", id)
				}
			}
		})
	}
}

func TestGameSKUEntry(t *testing.T) {
	sku := map[int]tcgplayer.SKU{}
	product := map[int]tcgplayer.Product{}
	for _, p := range gameProducts {
		product[p.ProductID] = p
		for _, s := range p.Skus {
			sku[s.SKUID] = s
		}
	}
	// A normal sku of a product printed in holofoil alone
	sku[2] = tcgplayer.SKU{SKUID: 2, ProductID: 616528, LanguageID: 1, PrintingID: 168, ConditionID: 1}

	for _, tt := range []struct {
		desc   string
		sealed bool
		direct bool
		result tcgplayer.SKUPriceSet
		want   []string
		net    []string
		logged bool
	}{
		{
			desc:   "a holofoil single at near mint",
			result: tcgplayer.SKUPriceSet{SKUID: 8560578, LowestListingPrice: 29, MarketPrice: 48.27},
			want: []string{
				"Gundam ST01-001 foil: TCGplayer NM 29.00 https://www.tcgplayer.com/product/616528?Condition=Near+Mint&Language=all&Printing=Holofoil&direct=false",
				"Gundam ST01-001 foil: TCGplayer Trends NM 48.27 https://www.tcgplayer.com/product/616528?Condition=Near+Mint&Language=all&Printing=Holofoil&direct=false",
			},
		},
		{
			desc:   "lightly played reads as SP",
			result: tcgplayer.SKUPriceSet{SKUID: 8560579, LowestListingPrice: 33.98, MarketPrice: 33.74},
			want: []string{
				"Gundam ST01-001 foil: TCGplayer SP 33.98 https://www.tcgplayer.com/product/616528?Condition=Lightly+Played&Language=all&Printing=Holofoil&direct=false",
				"Gundam ST01-001 foil: TCGplayer Trends SP 33.74 https://www.tcgplayer.com/product/616528?Condition=Lightly+Played&Language=all&Printing=Holofoil&direct=false",
			},
		},
		{
			desc:   "a normal single",
			result: tcgplayer.SKUPriceSet{SKUID: 8560649, LowestListingPrice: 2.15, MarketPrice: 0.79},
			want: []string{
				"Gundam (MA Form) ST01-002: TCGplayer NM 2.15 https://www.tcgplayer.com/product/616532?Condition=Near+Mint&Language=all&Printing=Normal&direct=false",
				"Gundam (MA Form) ST01-002: TCGplayer Trends NM 0.79 https://www.tcgplayer.com/product/616532?Condition=Near+Mint&Language=all&Printing=Normal&direct=false",
			},
		},
		{
			desc:   "a grade with no listing and no sales keeps its sku at $0",
			result: tcgplayer.SKUPriceSet{SKUID: 8560580},
			want:   []string{"Gundam ST01-001 foil: TCGplayer MP 0.00 https://www.tcgplayer.com/product/616528?Condition=Moderately+Played&Language=all&Printing=Holofoil&direct=false"},
		},
		{
			desc:   "a grade with sales but no listing",
			result: tcgplayer.SKUPriceSet{SKUID: 8560580, MarketPrice: 21.5},
			want: []string{
				"Gundam ST01-001 foil: TCGplayer MP 0.00 https://www.tcgplayer.com/product/616528?Condition=Moderately+Played&Language=all&Printing=Holofoil&direct=false",
				"Gundam ST01-001 foil: TCGplayer Trends MP 21.50 https://www.tcgplayer.com/product/616528?Condition=Moderately+Played&Language=all&Printing=Holofoil&direct=false",
			},
		},
		{
			desc:   "a Direct listing, and what it nets",
			direct: true,
			result: tcgplayer.SKUPriceSet{SKUID: 8560578, LowestListingPrice: 29, DirectLowPrice: 30, MarketPrice: 48.27},
			want: []string{
				"Gundam ST01-001 foil: TCGplayer NM 29.00 https://www.tcgplayer.com/product/616528?Condition=Near+Mint&Language=all&Printing=Holofoil&direct=false",
				"Gundam ST01-001 foil: TCG Direct NM 30.00 https://www.tcgplayer.com/product/616528?Condition=Near+Mint&Language=all&Printing=Holofoil&direct=true",
				"Gundam ST01-001 foil: TCGplayer Trends NM 48.27 https://www.tcgplayer.com/product/616528?Condition=Near+Mint&Language=all&Printing=Holofoil&direct=false",
			},
			net: []string{"TCG Direct (net) NM 25.45"},
		},
		{
			desc:   "Direct is floored, and under $2.50 it nets half",
			direct: true,
			result: tcgplayer.SKUPriceSet{SKUID: 8560649, LowestListingPrice: 0.25, DirectLowPrice: 0.1},
			want: []string{
				"Gundam (MA Form) ST01-002: TCGplayer NM 0.25 https://www.tcgplayer.com/product/616532?Condition=Near+Mint&Language=all&Printing=Normal&direct=false",
				"Gundam (MA Form) ST01-002: TCG Direct NM 0.40 https://www.tcgplayer.com/product/616532?Condition=Near+Mint&Language=all&Printing=Normal&direct=true",
			},
			net: []string{"TCG Direct (net) NM 0.20"},
		},
		{
			desc:   "no Direct where the category has none",
			result: tcgplayer.SKUPriceSet{SKUID: 8560578, LowestListingPrice: 29, DirectLowPrice: 30},
			want:   []string{"Gundam ST01-001 foil: TCGplayer NM 29.00 https://www.tcgplayer.com/product/616528?Condition=Near+Mint&Language=all&Printing=Holofoil&direct=false"},
		},
		{
			desc:   "a row nobody prices is dropped quietly when no card owns it",
			result: tcgplayer.SKUPriceSet{SKUID: 2},
		},
		{
			desc:   "a finish the printing is not sold in is reported",
			result: tcgplayer.SKUPriceSet{SKUID: 2, LowestListingPrice: 10},
			logged: true,
		},
		{
			desc:   "the booster box",
			sealed: true,
			result: tcgplayer.SKUPriceSet{SKUID: 8569425, LowestListingPrice: 4749.99, MarketPrice: 2842.9},
			want:   []string{"Gundam Card Game Edition Beta Box: TCGplayer NM 4749.99 https://www.tcgplayer.com/product/617436?Language=all&direct=false"},
		},
		{
			desc:   "a single is no sealed product",
			sealed: true,
			result: tcgplayer.SKUPriceSet{SKUID: 8560578, LowestListingPrice: 29},
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			tcg, logs := gameScraper(t, tt.sealed)
			tcg.catalog = &tcgplayer.CatalogDump{Category: tcgplayer.Category{IsDirect: tt.direct}}
			s := sku[tt.result.SKUID]
			var got, net []string
			for _, out := range tcg.skuEntries(tt.result, s, product[s.ProductID]) {
				if out.entry.Bundle != (out.entry.SellerName == "TCG Direct") {
					t.Errorf("%s bundled: %v", out.entry.SellerName, out.entry.Bundle)
				}
				got = append(got, describe(tcg.backend, out))
				if out.bl != nil {
					net = append(net, fmt.Sprintf("%s %s %.2f", out.bl.VendorName, out.bl.Conditions, out.bl.BuyPrice))
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
			if !slices.Equal(net, tt.net) {
				t.Errorf("buylist %q, want %q", net, tt.net)
			}
			if tt.logged != (len(*logs) > 0) {
				t.Errorf("logged %q, want a report: %v", *logs, tt.logged)
			}
			if tt.logged && !strings.Contains((*logs)[0], "(product 616528)") {
				t.Errorf("report %q does not name the product", (*logs)[0])
			}
		})
	}
}

// TestGameMarketNames pins the shorthands the sub-scrapers dump under: the
// lowest listing keeps the scraper's own, which sealed product has alone,
// and Direct is there for a category sold through it.
func TestGameMarketNames(t *testing.T) {
	for _, tt := range []struct {
		sealed   bool
		direct   bool
		want     []string
		metadata []string
		vendors  []string
	}{
		{false, false, []string{"TCGPlayer", "TCGTrends"}, []string{"TCGTrends"}, nil},
		{false, true, []string{"TCGPlayer", "TCGDirect", "TCGTrends"}, []string{"TCGTrends"}, []string{"TCGDirectNet"}},
		{true, false, []string{"TCGSealed"}, nil, nil},
		{true, true, []string{"TCGSealed"}, nil, nil},
	} {
		tcg, _ := gameScraper(t, tt.sealed)
		tcg.catalog = &tcgplayer.CatalogDump{Category: tcgplayer.Category{IsDirect: tt.direct}}
		var got, metadata, vendors []string
		for _, name := range tcg.MarketNames() {
			info := tcg.InfoForScraper(name)
			got = append(got, info.Shorthand)
			if info.MetadataOnly {
				metadata = append(metadata, info.Shorthand)
			}
		}
		for _, name := range tcg.TraderNames() {
			vendors = append(vendors, tcg.InfoForScraper(name).Shorthand)
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("sealed %v direct %v: shorthands %v, want %v", tt.sealed, tt.direct, got, tt.want)
		}
		if !slices.Equal(metadata, tt.metadata) {
			t.Errorf("sealed %v direct %v: metadata-only shorthands %v, want %v", tt.sealed, tt.direct, metadata, tt.metadata)
		}
		if !slices.Equal(vendors, tt.vendors) {
			t.Errorf("sealed %v direct %v: vendor shorthands %v, want %v", tt.sealed, tt.direct, vendors, tt.vendors)
		}
	}
}

func TestGameIndexProductEntries(t *testing.T) {
	b, err := matcher.Open(matcher.GameGundam, strings.NewReader(gameFixture))
	if err != nil {
		t.Fatal(err)
	}
	tcg, err := NewScraperGameIndex(b, "public", "private")
	if err != nil {
		t.Fatal(err)
	}
	tcg.printings = []string{"Normal", "Holofoil"}
	var logs []string
	tcg.logCallback = func(format string, a ...any) {
		logs = append(logs, fmt.Sprintf(format, a...))
	}
	product := map[int]tcgplayer.Product{}
	for _, p := range gameProducts {
		product[p.ProductID] = p
	}

	for _, tt := range []struct {
		desc   string
		direct bool
		result tcgplayer.ProductPriceSet
		want   []string
		logged bool
	}{
		{
			desc:   "one entry per price quoted",
			result: tcgplayer.ProductPriceSet{ProductID: 616528, LowPrice: 29, MarketPrice: 48.27, MidPrice: 54.74, HighPrice: 199, SubTypeName: "Holofoil"},
			want: []string{
				"Gundam ST01-001 foil: TCG Low 29.00 https://www.tcgplayer.com/product/616528?Language=all&Printing=Holofoil&direct=false",
				"Gundam ST01-001 foil: TCG Market 48.27 https://www.tcgplayer.com/product/616528?Language=all&Printing=Holofoil&direct=false",
				"Gundam ST01-001 foil: TCG Mid 54.74 https://www.tcgplayer.com/product/616528?Language=all&Printing=Holofoil&direct=false",
			},
		},
		{
			desc:   "a finish nobody prices",
			result: tcgplayer.ProductPriceSet{ProductID: 616528, SubTypeName: "Normal"},
		},
		{
			desc:   "no direct price where the category has no Direct",
			result: tcgplayer.ProductPriceSet{ProductID: 616532, LowPrice: 0.66, DirectLowPrice: 0.7, SubTypeName: "Normal"},
			want: []string{
				"Gundam (MA Form) ST01-002: TCG Low 0.66 https://www.tcgplayer.com/product/616532?Language=all&Printing=Normal&direct=false",
			},
		},
		{
			desc:   "Direct's low is floored",
			direct: true,
			result: tcgplayer.ProductPriceSet{ProductID: 616532, LowPrice: 0.25, DirectLowPrice: 0.1, SubTypeName: "Normal"},
			want: []string{
				"Gundam (MA Form) ST01-002: TCG Low 0.25 https://www.tcgplayer.com/product/616532?Language=all&Printing=Normal&direct=false",
				"Gundam (MA Form) ST01-002: TCG Direct Low 0.40 https://www.tcgplayer.com/product/616532?Language=all&Printing=Normal&direct=true",
			},
		},
		{
			desc:   "the direct price links to direct listings",
			direct: true,
			result: tcgplayer.ProductPriceSet{ProductID: 616532, LowPrice: 0.66, MarketPrice: 0.79, MidPrice: 0.99, DirectLowPrice: 0.7, SubTypeName: "Normal"},
			want: []string{
				"Gundam (MA Form) ST01-002: TCG Low 0.66 https://www.tcgplayer.com/product/616532?Language=all&Printing=Normal&direct=false",
				"Gundam (MA Form) ST01-002: TCG Market 0.79 https://www.tcgplayer.com/product/616532?Language=all&Printing=Normal&direct=false",
				"Gundam (MA Form) ST01-002: TCG Mid 0.99 https://www.tcgplayer.com/product/616532?Language=all&Printing=Normal&direct=false",
				"Gundam (MA Form) ST01-002: TCG Direct Low 0.70 https://www.tcgplayer.com/product/616532?Language=all&Printing=Normal&direct=true",
			},
		},
		{
			desc:   "a market price alone on a finish the printing lacks is let through quietly",
			result: tcgplayer.ProductPriceSet{ProductID: 616532, MarketPrice: 0.5, SubTypeName: "Holofoil"},
		},
		{
			desc:   "a market price alone on a finish the category no longer sells is let through quietly",
			result: tcgplayer.ProductPriceSet{ProductID: 616532, MarketPrice: 0.5, SubTypeName: "1st Edition - Ultimate"},
		},
		{
			desc:   "a market price alone on a finish the category sells is priced",
			result: tcgplayer.ProductPriceSet{ProductID: 616532, MarketPrice: 0.79, SubTypeName: "Normal"},
			want: []string{
				"Gundam (MA Form) ST01-002: TCG Market 0.79 https://www.tcgplayer.com/product/616532?Language=all&Printing=Normal&direct=false",
			},
		},
		{
			desc:   "any other price on that finish is reported",
			result: tcgplayer.ProductPriceSet{ProductID: 616532, LowPrice: 0.4, MarketPrice: 0.5, SubTypeName: "Holofoil"},
			logged: true,
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			logs = nil
			tcg.catalog = &tcgplayer.CatalogDump{Category: tcgplayer.Category{IsDirect: tt.direct}}
			var got []string
			for _, out := range tcg.productEntries(tt.result, product[tt.result.ProductID]) {
				if out.entry.Bundle != (out.entry.SellerName == "TCG Direct Low") {
					t.Errorf("%s bundled: %v", out.entry.SellerName, out.entry.Bundle)
				}
				got = append(got, describe(b, out))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
			if tt.logged != (len(logs) > 0) {
				t.Errorf("logged %q, want a report: %v", logs, tt.logged)
			}
		})
	}
}
