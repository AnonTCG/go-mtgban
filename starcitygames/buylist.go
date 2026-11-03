package starcitygames

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
)

const (
	// The set index is served by the same public MeiliSearch instance the
	// sell-your-cards storefront queries, with the search-only key embedded in
	// its frontend.
	scgSetsURL       = "https://search.starcitygames.com/indexes/sets_v2/search"
	scgSetsSearchKey = "f0b723e5fddfe6edbdad6fecc814511511a62b1722c6c30cff42be5bca4ed834"

	sellYourCardsHost = "https://sellyourcards.starcitygames.com"

	// The buy offers themselves, every grade of every single, from the index
	// the same storefront searches.
	scgSellListURL = "https://search.starcitygames.com/indexes/sell_list_products_v2/search"
)

// scgMinBuyPrice is the least a published offer can be worth: the index
// carries sub-cent figures that no buylist pays out.
const scgMinBuyPrice = 0.01

const (
	// sellListCap is the most hits the index serves one filter, offset
	// included, so a game is crawled in slices that each fit under it.
	sellListCap  = 45000
	sellListPage = 1000

	sellListConcurrency = 4
)

// sellListFacets are the attributes a game's products are sliced by, the
// next one only where a slice of the one before still holds sellListCap.
var sellListFacets = []string{"finish", "rarity"}

// sellListGrades maps the index's grades to ours. SCG grades no MP.
var sellListGrades = map[string]mtgban.Condition{
	"NM": mtgban.NM,
	"PL": mtgban.SP,
	"HP": mtgban.HP,
}

type sellListVariant struct {
	SKU          string  `json:"sku"`
	Status       string  `json:"status"`
	VariantValue string  `json:"variant_value"`
	BuyPrice     float64 `json:"buy_price"`
}

// sellListProduct is a product as the index describes it: its offers, and the
// wording a product the catalog export does not list is resolved from. The
// finish and rarity are numbers, or an empty string where a product has none.
type sellListProduct struct {
	Name            string            `json:"name"`
	Language        string            `json:"language"`
	Finish          any               `json:"finish"`
	Rarity          any               `json:"rarity"`
	CollectorNumber string            `json:"collector_number"`
	SetName         string            `json:"set_name"`
	Variants        []sellListVariant `json:"variants"`
}

type sellListResponse struct {
	Hits               []sellListProduct         `json:"hits"`
	FacetDistribution  map[string]map[string]int `json:"facetDistribution"`
	EstimatedTotalHits int                       `json:"estimatedTotalHits"`
}

type sellListSlice struct {
	filter string
	count  int
}

type sellListPageQuery struct {
	filter string
	offset int
}

func (scg *SCGClient) searchSellList(ctx context.Context, body map[string]any) (*sellListResponse, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, scg.sellListURL, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+scgSetsSearchKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := scg.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sell list search failed: HTTP %d", resp.StatusCode)
	}

	var out sellListResponse
	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// sellListSlices splits filter by the facets from depth on until every slice
// holds fewer than sellListCap products, the only size the index pages fully.
// The facet counts leave out a product with an empty, null or missing
// value, so those are a slice of their own.
func (scg *SCGClient) sellListSlices(ctx context.Context, filter string, depth int) ([]sellListSlice, error) {
	if depth == len(sellListFacets) {
		return nil, fmt.Errorf("sell list slice %q still holds %d or more products", filter, sellListCap)
	}
	facet := sellListFacets[depth]
	resp, err := scg.searchSellList(ctx, map[string]any{
		"q": "", "limit": 0, "filter": filter, "facets": []string{facet},
	})
	if err != nil {
		return nil, err
	}
	noValue := fmt.Sprintf("%s AND (%s IS EMPTY OR %s IS NULL OR %s NOT EXISTS)", filter, facet, facet, facet)
	empty, err := scg.searchSellList(ctx, map[string]any{
		"q": "", "limit": 0, "filter": noValue,
	})
	if err != nil {
		return nil, err
	}

	counts := map[string]int{}
	for value, count := range resp.FacetDistribution[facet] {
		counts[fmt.Sprintf("%s AND %s = %s", filter, facet, value)] = count
	}
	if empty.EstimatedTotalHits > 0 {
		counts[noValue] = empty.EstimatedTotalHits
	}

	var out []sellListSlice
	for sub, count := range counts {
		if count < sellListCap {
			out = append(out, sellListSlice{sub, count})
			continue
		}
		deeper, err := scg.sellListSlices(ctx, sub, depth+1)
		if err != nil {
			return nil, err
		}
		out = append(out, deeper...)
	}
	return out, nil
}

// sellListOffers returns what SCG pays for each single of a game it is
// buying, by product sku and grade, and the index's own description of each
// of those products. The catalog export carries a Played or Heavily Played
// variant for only some of them, and does not list some at all; the
// sell-your-cards index carries every grade, with the status that says
// whether a price is a bid.
func (scg *SCGClient) sellListOffers(ctx context.Context, game int) (map[string]map[mtgban.Condition]sellListVariant, map[string]sellListProduct, error) {
	filter := fmt.Sprintf("game_id = %d AND product_type_id = 1 AND primary_status IN [buying_at_cost, hotlist]", game)
	sliced, err := scg.sellListSlices(ctx, filter, 0)
	if err != nil {
		return nil, nil, err
	}

	var pages []sellListPageQuery
	var want int
	for _, slice := range sliced {
		want += slice.count
		for offset := 0; offset < slice.count; offset += sellListPage {
			pages = append(pages, sellListPageQuery{slice.filter, offset})
		}
	}

	var mu sync.Mutex
	var errs []error
	var got int
	out := map[string]map[mtgban.Condition]sellListVariant{}
	products := map[string]sellListProduct{}
	mtgban.WorkerPool(ctx, sellListConcurrency, pages,
		func(ctx context.Context, page sellListPageQuery, results chan<- sellListProduct) error {
			resp, err := scg.searchSellList(ctx, map[string]any{
				"q": "", "limit": sellListPage, "offset": page.offset, "filter": page.filter,
				"attributesToRetrieve": []string{
					"name", "language", "finish", "rarity", "collector_number", "set_name",
					"variants.sku", "variants.status", "variants.variant_value", "variants.buy_price",
				},
			})
			if err != nil {
				return err
			}
			for _, hit := range resp.Hits {
				results <- hit
			}
			return nil
		},
		func(hit sellListProduct) {
			got++
			for _, v := range hit.Variants {
				grade, found := sellListGrades[v.VariantValue]
				if !found || v.SKU == "" || v.BuyPrice < scgMinBuyPrice {
					continue
				}
				if v.Status != "buying_at_cost" && v.Status != "hotlist" {
					continue
				}
				// A variant sku is its product's with one grade digit after.
				product := v.SKU[:len(v.SKU)-1]
				if out[product] == nil {
					out[product] = map[mtgban.Condition]sellListVariant{}
				}
				out[product][grade] = v
				products[product] = hit
			}
		},
		func(format string, a ...any) {
			mu.Lock()
			defer mu.Unlock()
			errs = append(errs, fmt.Errorf(format, a...))
		},
	)
	if len(errs) > 0 {
		return nil, nil, errs[0]
	}
	if got != want {
		return nil, nil, fmt.Errorf("sell list returned %d of the %d products its facets count", got, want)
	}

	// The storefront buys nothing of a product it makes no NM bid on, and
	// says "We are not currently buying this product" whatever its played
	// grades are priced at.
	for product, grades := range out {
		if _, found := grades[mtgban.NM]; !found {
			delete(out, product)
			delete(products, product)
		}
	}
	return out, products, nil
}

// scgLanguageCodes maps the catalog language names to the codes the
// sell-your-cards bookmark links expect.
var scgLanguageCodes = map[string]string{
	"English":               "en",
	"German":                "de",
	"Spanish":               "es",
	"French":                "fr",
	"Italian":               "it",
	"Japanese":              "ja",
	"Korean":                "ko",
	"Portuguese":            "pt",
	"Russian":               "ru",
	"Chinese - Simplified":  "zs",
	"Chinese - Traditional": "zt",
}

type scgSet struct {
	Name        string `json:"name"`
	SetID       int    `json:"set_id"`
	GameID      int    `json:"game_id"`
	WizardsCode string `json:"wizards_code"`
}

// SetIDs returns the set name -> set_id map for a game, as used by the
// sell-your-cards bookmark links.
func (scg *SCGClient) SetIDs(ctx context.Context, game int) (map[string]int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, scg.setsURL,
		strings.NewReader(`{"q":"","limit":2000}`))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+scgSetsSearchKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := scg.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var page struct {
		Hits []scgSet `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, err
	}

	// Key by both the set name (matched fuzzily against a single's catalog set)
	// and the Wizards code (an exact fallback for the set code carried in a
	// SKU). A short code never fuzzy-matches a multi-word set name, so the two
	// keyspaces don't interfere.
	out := map[string]int{}
	for _, s := range page.Hits {
		if s.GameID != game {
			continue
		}
		if s.Name != "" {
			out[s.Name] = s.SetID
		}
		if s.WizardsCode != "" {
			out[s.WizardsCode] = s.SetID
		}
	}
	return out, nil
}

// setIDsForProduct resolves the sell-your-cards set ids for a catalog product:
// by the catalog set name (fuzzily), and when that is empty or unknown -- as it
// is for sealed and a few uncategorized singles -- by the set code carried in
// the SKU (SGL-<brand>-<set>-...), tried raw then normalized.
func setIDsForProduct(b *matcher.Backend, setIDs map[string]int, setName, sku string) []int {
	if ids := matchSetIDs(setIDs, setName); len(ids) > 0 {
		return ids
	}
	fields := strings.Split(sku, "-")
	if len(fields) > 2 {
		if id, ok := setIDs[fields[2]]; ok {
			return []int{id}
		}
		if id, ok := setIDs[fixupSetCode(b, fields[2])]; ok {
			return []int{id}
		}
	}
	return nil
}

func setWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// matchSetIDs resolves a catalog set name to sell-your-cards set ids. An exact
// name wins outright; otherwise every set sharing the longest leading-word
// prefix with the query is returned, so a name the index doesn't carry verbatim
// still lands on the closest set group -- "Marvel Super Heroes Commander" pulls
// in the Marvel Super Heroes sets, "Promo: General" falls back to "Promo".
func matchSetIDs(setIDs map[string]int, query string) []int {
	query = strings.TrimSuffix(query, " (Foil)")
	if id, ok := setIDs[query]; ok {
		return []int{id}
	}

	qwords := setWords(query)
	if len(qwords) == 0 {
		return nil
	}

	best := 0
	byLen := map[int][]int{}
	for name, id := range setIDs {
		n := leadingWordMatch(qwords, setWords(name))
		if n == 0 {
			continue
		}
		byLen[n] = append(byLen[n], id)
		if n > best {
			best = n
		}
	}
	return byLen[best]
}

func leadingWordMatch(a, b []string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

func gamePath(game int) string {
	switch game {
	case GameFleshAndBlood:
		return "fab"
	case GameLorcana:
		return "lorcana"
	case GameRiftbound:
		return "riftbound"
	}
	return "mtg"
}

// SCGBuylistURL builds the sell-your-cards bookmark link that lists a product
// for sale to SCG, filtered to the given set ids (as opposed to the retail page
// SCGProductURL points to). The segment order mirrors the storefront's own URL
// builder, and the link excludes bulk so it shows the same offers the scraper
// publishes. Returns "" if no set ids are given.
func SCGBuylistURL(game int, name, language string, setIDs []int) string {
	if len(setIDs) == 0 {
		return ""
	}

	ids := make([]string, len(setIDs))
	for i, id := range setIDs {
		ids[i] = strconv.Itoa(id)
	}

	nameSeg := "0"
	if name != "" {
		nameSeg = url.PathEscape(name)
	}
	langSeg := ","
	if code := scgLanguageCodes[language]; code != "" {
		langSeg = code
	}

	segments := []string{
		gamePath(game),
		"bookmark",
		nameSeg,                  // cardName
		"1",                      // cardNameExactMatch
		"1",                      // filterOutBulkProducts
		"0",                      // filterOnlyHotlist
		"0",                      // exportAsCSV
		strings.Join(ids, "%2C"), // set_ids
		langSeg,                  // languages
		",",                      // rarities
		"0",                      // cardPurchasePriceMin
		"999999.99",              // cardPurchasePriceMax
		",",                      // finishes
		"setReleaseDateAsc",      // sort
	}
	return sellYourCardsHost + "/" + strings.Join(segments, "/")
}
