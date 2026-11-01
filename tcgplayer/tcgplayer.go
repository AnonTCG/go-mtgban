// Package tcgplayer scrapes TCGplayer through both their partner API and
// their storefront: market and index pricing, sealed product, per-seller
// inventory and the SYP list, every game's products read from its catalog
// dump.
package tcgplayer

import (
	"github.com/mtgban/go-mtgban/mtgban"
)

type responseChan struct {
	cardID string
	entry  mtgban.InventoryEntry
}

// trendsName is the sub-seller holding TCGplayer's market price, an index of
// past sales rather than a listing anyone sells at.
const trendsName = "TCGplayer Trends"

var name2shorthand = map[string]string{
	"TCG Low":          "TCGLow",
	"TCG Market":       "TCGMarket",
	"TCG Mid":          "TCGMid",
	"TCG Direct Low":   "TCGDirectLow",
	"TCG Direct":       "TCGDirect",
	"TCG Direct (net)": "TCGDirectNet",
	"TCGplayer":        "TCGPlayer",
	"TCGplayer Direct": "TCGDirect",
	"TCGplayer Trends": "TCGTrends",
}
