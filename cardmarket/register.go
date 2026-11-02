package cardmarket

import (
	"errors"
	"fmt"

	cm "github.com/mtgban/go-cardmarket"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
)

// Secret names the environment variables bantool has always read these
// credentials from.
const (
	// SecretAppToken and SecretAppSecret authenticate the Market and Sealed
	// scrapers, which query Cardmarket's own API rather than reading a
	// published download the way Index does.
	SecretAppToken  = "MKM_APP_TOKEN"
	SecretAppSecret = "MKM_APP_SECRET"

	// SecretBanKey authenticates the mtgban price snapshot Market's Load
	// reads to pre-filter which cards are worth a live call. It is
	// optional: only cardmarket_market reads it, into BanPriceKey.
	SecretBanKey = "BAN_API_KEY"
)

// Resource names the typed inputs the constructors below read out of
// mtgban.Options.
const (
	// ResourceCatalog names the published id-map catalog Index and Market
	// resolve from before falling back to name/number matching.
	ResourceCatalog = "cardmarket.catalog"

	// ResourceBridge names the Cardmarket-id to TCGplayer-id bridge; see
	// BridgeUse.
	ResourceBridge = "cardmarket.bridge"

	// ResourceTargetProduct names the optional single sealed product filter.
	ResourceTargetProduct = "cardmarket.target_product"
)

// WithCatalog hands Index or Market the published id-map catalog it
// resolves from before falling back to name/number matching.
func WithCatalog(catalog *cm.Catalog) mtgban.Option {
	return mtgban.WithResource(ResourceCatalog, catalog)
}

// WithBridge hands a Cardmarket scraper the Cardmarket-id to TCGplayer-id
// bridge cardtrader's blueprints supply - the one source linking the two
// marketplaces. See BridgeUse for which games need one.
func WithBridge(bridge map[int]int) mtgban.Option {
	return mtgban.WithResource(ResourceBridge, bridge)
}

// WithTargetProduct limits the Sealed scraper to one product name or UUID.
func WithTargetProduct(product string) mtgban.Option {
	return mtgban.WithResource(ResourceTargetProduct, product)
}

// BridgeUse says how much a game's Index and Market scraper leans on the
// TCGplayer bridge, for bantool to decide whether the bridge - a cardtrader
// crawl - is worth building, and whether a failure to build it is fatal.
type BridgeUse int

const (
	// BridgeUnused means the catalog names every printing of this game on
	// its own; a bridge given anyway changes nothing.
	BridgeUnused BridgeUse = iota
	// BridgeHelps means the catalog names most of the shelf by itself; the
	// bridge settles what a collector number cannot, and its absence only
	// costs those printings.
	BridgeHelps
	// BridgeRequired means same-name products abound and the catalog alone
	// cannot tell them apart; without a bridge the scraper cannot run.
	BridgeRequired
)

// BridgeUseOf reports how much a game's Index and Market scrapers lean on
// the TCGplayer bridge; see gameResolvers.
func BridgeUseOf(game matcher.Game) BridgeUse {
	switch game {
	case matcher.GamePokemon, matcher.GameYuGiOh, matcher.GameFleshAndBlood:
		return BridgeRequired
	case matcher.GameOnePiece, matcher.GameRiftbound, matcher.GameLorcana, matcher.GameGundam:
		return BridgeHelps
	default:
		return BridgeUnused
	}
}

// SealedBridgeUseOf is BridgeUseOf for the Sealed scraper, whose need is a
// different one: it reads the bridge only to name products by their
// TCGplayer id where the datastore's own Cardmarket ids run out, which a
// constructor cannot see in advance for any game but Magic, whose sealed
// map never falls back to names (see Sealed.Load).
func SealedBridgeUseOf(game matcher.Game) BridgeUse {
	if game == matcher.GameMagic {
		return BridgeUnused
	}
	return BridgeRequired
}

// cardmarketGames are the games Cardmarket's Index, Market and Sealed
// scrapers price - the same eight mkmGames names.
var cardmarketGames = []matcher.Game{
	matcher.GameMagic, matcher.GameLorcana, matcher.GameRiftbound,
	matcher.GameOnePiece, matcher.GamePokemon, matcher.GameYuGiOh,
	matcher.GameFleshAndBlood, matcher.GameGundam,
}

func init() {
	mtgban.Register("cardmarket", cardmarketGames, buildIndex)
	mtgban.Register("cardmarket_market", cardmarketGames, buildMarket)
	mtgban.Register("cardmarket_sealed", cardmarketGames, buildSealed)
}

// buildIndex is cardmarket's Constructor. It reads no credential: Index
// prices from a published catalog and the public price guide, and makes no
// authenticated call; see TestIndexMakesNoAuthenticatedCall.
func buildIndex(b *matcher.Backend, opts mtgban.Options) (mtgban.Scraper, error) {
	game := b.Game
	scraper, err := NewScraperIndex(b)
	if err != nil {
		return nil, err
	}

	catalog, err := mtgban.Resource[*cm.Catalog](opts, ResourceCatalog)
	if err != nil {
		return nil, err
	}
	if catalog == nil {
		return nil, errors.New("cardmarket needs WithCatalog")
	}
	scraper.catalog = catalog

	bridge, err := mtgban.Resource[map[int]int](opts, ResourceBridge)
	if err != nil {
		return nil, err
	}
	if len(bridge) == 0 && BridgeUseOf(game) == BridgeRequired {
		return nil, fmt.Errorf("cardmarket needs WithBridge for %s", game)
	}
	scraper.tcgBridge = bridge

	scraper.logCallback = opts.LogCallback
	scraper.affiliate = opts.Affiliate
	scraper.targetEdition = opts.TargetEdition
	if opts.MaxConcurrency != 0 {
		scraper.maxConcurrency = opts.MaxConcurrency
	}
	return scraper, nil
}

// buildMarket is cardmarket_market's Constructor.
func buildMarket(b *matcher.Backend, opts mtgban.Options) (mtgban.Scraper, error) {
	game := b.Game
	appToken, err := opts.Secret(SecretAppToken)
	if err != nil {
		return nil, err
	}
	appSecret, err := opts.Secret(SecretAppSecret)
	if err != nil {
		return nil, err
	}
	scraper, err := NewScraperMarket(b, appToken, appSecret)
	if err != nil {
		return nil, err
	}

	catalog, err := mtgban.Resource[*cm.Catalog](opts, ResourceCatalog)
	if err != nil {
		return nil, err
	}
	if catalog == nil {
		return nil, errors.New("cardmarket_market needs WithCatalog")
	}
	scraper.catalog = catalog

	bridge, err := mtgban.Resource[map[int]int](opts, ResourceBridge)
	if err != nil {
		return nil, err
	}
	if len(bridge) == 0 && BridgeUseOf(game) == BridgeRequired {
		return nil, fmt.Errorf("cardmarket_market needs WithBridge for %s", game)
	}
	scraper.tcgBridge = bridge

	scraper.logCallback = opts.LogCallback
	scraper.affiliate = opts.Affiliate
	scraper.targetEdition = opts.TargetEdition
	// Market prices without it, so a caller that holds no BAN key still
	// gets a working scraper.
	scraper.banPriceKey, _ = opts.Secret(SecretBanKey)
	return scraper, nil
}

// buildSealed is cardmarket_sealed's Constructor; the bridge it needs is
// SealedBridgeUseOf's question, not BridgeUseOf's.
func buildSealed(b *matcher.Backend, opts mtgban.Options) (mtgban.Scraper, error) {
	game := b.Game
	appToken, err := opts.Secret(SecretAppToken)
	if err != nil {
		return nil, err
	}
	appSecret, err := opts.Secret(SecretAppSecret)
	if err != nil {
		return nil, err
	}
	scraper, err := NewScraperSealed(b, appToken, appSecret)
	if err != nil {
		return nil, err
	}

	bridge, err := mtgban.Resource[map[int]int](opts, ResourceBridge)
	if err != nil {
		return nil, err
	}
	if len(bridge) == 0 && SealedBridgeUseOf(game) == BridgeRequired {
		return nil, fmt.Errorf("cardmarket_sealed needs WithBridge for %s", game)
	}
	scraper.tcgBridge = bridge

	scraper.logCallback = opts.LogCallback
	scraper.affiliate = opts.Affiliate
	scraper.targetEdition = opts.TargetEdition
	targetProduct, err := mtgban.Resource[string](opts, ResourceTargetProduct)
	if err != nil {
		return nil, err
	}
	scraper.targetProduct = targetProduct
	if opts.MaxConcurrency != 0 {
		scraper.maxConcurrency = opts.MaxConcurrency
	}
	return scraper, nil
}
