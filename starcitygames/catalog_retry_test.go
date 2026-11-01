package starcitygames

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	_ "github.com/mtgban/go-mtgban/matcher/fleshandblood"
	"github.com/mtgban/go-mtgban/mtgban"
)

// TestLoadCatalogRetryStartsClean drives the singles scraper through a catalog
// export that breaks partway and is read again from the top. The pieces are
// each covered on their own - the client's replay, the bucket decision,
// reset() itself - but nothing covered the scraper wired to the replay, so a
// stream callback that dropped the reset left the abandoned pass's records in
// place and every one of them arrived a second time. The quantities survive
// that (a record identical to itself is refused rather than doubled), so it is
// what the run says that gives it away: the replay must have nothing to report
// about the records it re-reads.
//
// The export carries the Armory Deck pair, so the run that follows the break
// has to end up with one card holding the pair's copies once, and with the
// listing on record as a pair exactly once.
func TestLoadCatalogRetryStartsClean(t *testing.T) {
	b := withGameDatastore(t, "fleshandblood", "FLESHANDBLOOD_PATH")

	product := func(sku string, nm, sp int) CatalogProduct {
		return CatalogProduct{
			SKU: sku, Name: "Anka, Drag Under", Game: "Flesh and Blood",
			Set: "Armory Deck: Gravy Bones", ProductType: ProductTypeSingles,
			CollectorNumber: "014", Finish: "Non-foil", FinishGroup: "Non-foil",
			URL: "/anka-drag-under-" + sku + "/",
			Variants: []CatalogVariant{
				{SKU: sku + "1", Condition: "Near Mint", Qty: nm, Price: "0.49", SellListPrice: "0.10"},
				{SKU: sku + "2", Condition: "Played", Qty: sp, Price: "0.39", SellListPrice: "0.05"},
			},
		}
	}
	marked, err := json.Marshal(product("SGL-FAB-AGB-014_CC-ENN", 7, 2))
	if err != nil {
		t.Fatalf("marshalling the marked record: %v", err)
	}
	plain, err := json.Marshal(product("SGL-FAB-AGB-014-ENN", 11, 5))
	if err != nil {
		t.Fatalf("marshalling the plain record: %v", err)
	}
	catalog := "[" + string(marked) + "," + string(plain) + "]"
	// Whole first record, then the connection drops inside the second one:
	// the abandoned pass has priced a listing and put its pair on record.
	broken := "[" + string(marked) + "," + string(plain)[:20]

	var served int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served++
		w.Header().Set("Content-Type", "application/json")
		if served == 1 {
			_, _ = w.Write([]byte(broken))
			w.(http.Flusher).Flush()
			if hijacker, ok := w.(http.Hijacker); ok {
				conn, _, hijackErr := hijacker.Hijack()
				if hijackErr == nil {
					_ = conn.Close()
				}
			}
			return
		}
		_, _ = w.Write([]byte(catalog))
	}))
	defer srv.Close()

	// The set index is fetched before the catalog and only decorates the
	// buylist links; answer it with an empty index rather than reaching for
	// the live one.
	sets := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"hits":[]}`))
	}))
	defer sets.Close()

	scg, err := NewScraper(b, "")
	if err != nil {
		t.Fatal(err)
	}
	client := *scg.client
	client.catalogURL = srv.URL
	client.setsURL = sets.URL
	scg.client = &client

	var reported []string
	scg.logCallback = func(format string, a ...any) {
		reported = append(reported, fmt.Sprintf(format, a...))
	}

	if err := scg.loadCatalog(context.Background()); err != nil {
		t.Fatalf("loadCatalog: %v", err)
	}
	if served != 2 {
		t.Errorf("served %d catalogs, want the broken one and the whole one", served)
	}

	// The replay says it is replaying and how much it read, and nothing
	// else: a record it re-reads is a record it has no memory of, so none
	// of them is reported as the duplicate of itself it would otherwise be.
	want := []string{
		"[SCG] Catalog stream broke after 1 products, downloading it again",
		"[SCG] Processed 2 products total",
	}
	if !slices.Equal(reported, want) {
		t.Errorf("the run said %q, want %q", reported, want)
	}

	if len(scg.inventory) != 1 {
		t.Fatalf("got %d priced cards, want the pair's one: %v", len(scg.inventory), reported)
	}
	for _, entries := range scg.inventory {
		wantQty := map[mtgban.Condition]int{mtgban.NM: 18, mtgban.SP: 7}
		if len(entries) != 2*len(wantQty) {
			t.Fatalf("got %d entries, want one per grade of each record: %v", len(entries), entries)
		}
		gotQty := map[mtgban.Condition]int{}
		for _, entry := range entries {
			gotQty[entry.Conditions] += entry.Quantity
		}
		for grade, qty := range wantQty {
			if gotQty[grade] != qty {
				t.Errorf("%s holds %d copies, want %d: the abandoned pass was counted again",
					grade, gotQty[grade], qty)
			}
		}
	}
	for _, entries := range scg.buylist {
		if len(entries) != 2 {
			t.Errorf("got %d buylist entries, want one per grade: %v", len(entries), entries)
		}
	}

	// One listing was seen as a pair, and the retry left no second copy of
	// that behind: the fold state goes back with the records it describes.
	if len(scg.buckets) != 1 {
		t.Errorf("the run ended with %v on record, want the one pair", scg.buckets)
	}
	if _, ok := scg.buckets["SGL-FAB-AGB-014-ENN"]; !ok {
		t.Errorf("the pair is not on record: %v", scg.buckets)
	}
}
