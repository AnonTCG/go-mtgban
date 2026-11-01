package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
)

// TestCardmarketNeedsItsBridge pins that a Cardmarket target Cardmarket cannot
// identify on its own refuses to be built without the CardTrader bridge, all
// three scrapers alike - sealed, singles, and the live-listing market.
//
// The scraper can price by name where the bridge is missing, and that is the
// weaker answer, not the same one: the bridge settles a product by an id both
// catalogs carry, while a name reaches about half of what each datastore holds
// and reaches it on a spelling. A run that quietly delivered the weaker answer
// would be a run nobody was told about, so the failure is loud instead -
// scraperResources returning an error ends the run.
func TestCardmarketNeedsItsBridge(t *testing.T) {
	t.Setenv("CARDTRADER_TOKEN_BEARER", "")
	// The singles and market scrapers read their catalog before they ask for
	// the bridge, so the catalog has to be there for the bridge to be what
	// is missing.
	catalog := filepath.Join(t.TempDir(), "catalog.json")
	err := os.WriteFile(catalog, []byte(`{"data":{"products":{"1":{"expansionId":1,"name":"Blue-Eyes White Dragon"}}}}`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CARDMARKET_CATALOG_PATH", catalog)

	_, err = scraperResources(&matcher.Backend{Game: matcher.GamePokemon}, "cardmarket_sealed")
	if err == nil || !strings.Contains(err.Error(), "CARDTRADER_TOKEN_BEARER") {
		t.Errorf("sealed was built without a bridge: %v", err)
	}
	_, err = scraperResources(&matcher.Backend{Game: matcher.GameYuGiOh}, "cardmarket")
	if err == nil || !strings.Contains(err.Error(), "CARDTRADER_TOKEN_BEARER") {
		t.Errorf("singles was built without a bridge: %v", err)
	}
	_, err = scraperResources(&matcher.Backend{Game: matcher.GameYuGiOh}, "cardmarket_market")
	if err == nil || !strings.Contains(err.Error(), "CARDTRADER_TOKEN_BEARER") {
		t.Errorf("market was built without a bridge: %v", err)
	}
}
