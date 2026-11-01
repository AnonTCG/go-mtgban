package tcgplayer

import (
	"errors"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"

	"github.com/mtgban/go-tcgplayer"
)

// The environment variable names bantool has always read these secrets
// under.
const (
	SecretPublicKey  = "TCGPLAYER_PUBLIC_KEY"
	SecretPrivateKey = "TCGPLAYER_PRIVATE_KEY"
	SecretAuth       = "TCGPLAYER_AUTH"
)

// The typed resources tcg_index, tcg_market, tcg_sealed and tcg_syplist
// read out of their Options.
const (
	ResourceSYPCatalog = "tcgplayer.sypcatalog"
	ResourceCatalog    = "tcgplayer.catalog"
)

// WithSYPCatalog hands the SYP scraper the catalog LoadSYPCatalog returns,
// which is the step between a sku id the list names and the product and
// finish the datastore knows it by.
func WithSYPCatalog(catalog SYPCatalog) mtgban.Option {
	return mtgban.WithResource(ResourceSYPCatalog, catalog)
}

// WithCatalog hands the index, market and sealed scrapers the catalog dump
// LoadCatalog returns: the products they price, with their skus, and the
// category's printings and languages.
func WithCatalog(catalog *tcgplayer.CatalogDump) mtgban.Option {
	return mtgban.WithResource(ResourceCatalog, catalog)
}

// gameCatalog is the catalog a scraper built for key prices.
func gameCatalog(opts mtgban.Options, key string) (*tcgplayer.CatalogDump, error) {
	catalog, err := mtgban.Resource[*tcgplayer.CatalogDump](opts, ResourceCatalog)
	if err != nil {
		return nil, err
	}
	if catalog == nil {
		return nil, errors.New(key + " needs WithCatalog")
	}
	return catalog, nil
}

func tcgCredentials(opts mtgban.Options) (publicID, privateID string, err error) {
	publicID, err = opts.Secret(SecretPublicKey)
	if err != nil {
		return "", "", err
	}
	privateID, err = opts.Secret(SecretPrivateKey)
	if err != nil {
		return "", "", err
	}
	return publicID, privateID, nil
}

// newTCGIndexScraper builds tcg_index: TCGGameIndex over the catalog dump.
func newTCGIndexScraper(b *matcher.Backend, opts mtgban.Options) (mtgban.Scraper, error) {
	publicID, privateID, err := tcgCredentials(opts)
	if err != nil {
		return nil, err
	}
	catalog, err := gameCatalog(opts, "tcg_index")
	if err != nil {
		return nil, err
	}

	scraper, err := NewScraperGameIndex(b, publicID, privateID)
	if err != nil {
		return nil, err
	}
	scraper.catalog = catalog
	scraper.logCallback = opts.LogCallback
	scraper.affiliate = opts.Affiliate
	if opts.MaxConcurrency != 0 {
		scraper.maxConcurrency = opts.MaxConcurrency
	}
	return scraper, nil
}

// newTCGMarketScraper builds tcg_market: TCGGame over the catalog dump.
func newTCGMarketScraper(b *matcher.Backend, opts mtgban.Options) (mtgban.Scraper, error) {
	return newTCGGameScraper(b, opts, "tcg_market", false)
}

// newTCGSealedScraper builds tcg_sealed: TCGGame over the catalog dump,
// resolving sealed products through the datastore's sealed product map.
func newTCGSealedScraper(b *matcher.Backend, opts mtgban.Options) (mtgban.Scraper, error) {
	return newTCGGameScraper(b, opts, "tcg_sealed", true)
}

// newTCGGameScraper builds key's TCGGame, in the sealed mode or for singles.
func newTCGGameScraper(b *matcher.Backend, opts mtgban.Options, key string, sealed bool) (mtgban.Scraper, error) {
	publicID, privateID, err := tcgCredentials(opts)
	if err != nil {
		return nil, err
	}
	catalog, err := gameCatalog(opts, key)
	if err != nil {
		return nil, err
	}

	var scraper *TCGGame
	if sealed {
		scraper, err = NewScraperGameSealed(b, publicID, privateID)
	} else {
		scraper, err = NewScraperGame(b, publicID, privateID)
	}
	if err != nil {
		return nil, err
	}
	scraper.catalog = catalog
	scraper.logCallback = opts.LogCallback
	scraper.affiliate = opts.Affiliate
	if opts.MaxConcurrency != 0 {
		scraper.maxConcurrency = opts.MaxConcurrency
	}
	return scraper, nil
}

// newTCGSYPListScraper builds tcg_syplist, served the same way for every
// game it covers: the list names a sku and the catalog resolves it to a
// product and finish the datastore knows.
func newTCGSYPListScraper(b *matcher.Backend, opts mtgban.Options) (mtgban.Scraper, error) {
	authTicket, err := opts.Secret(SecretAuth)
	if err != nil {
		return nil, err
	}
	catalog, err := mtgban.Resource[SYPCatalog](opts, ResourceSYPCatalog)
	if err != nil {
		return nil, err
	}
	if catalog == nil {
		return nil, errors.New("tcg_syplist needs WithSYPCatalog")
	}

	scraper, err := NewScraperSYP(b, authTicket)
	if err != nil {
		return nil, err
	}
	scraper.catalog = catalog
	scraper.logCallback = opts.LogCallback
	scraper.affiliate = opts.Affiliate
	return scraper, nil
}

func init() {
	mtgban.Register("tcg_index", matcher.AllGames, newTCGIndexScraper)
	mtgban.Register("tcg_market", matcher.AllGames, newTCGMarketScraper)
	mtgban.Register("tcg_sealed", matcher.AllGames, newTCGSealedScraper)
	mtgban.Register("tcg_syplist", []matcher.Game{matcher.GameMagic, matcher.GamePokemon}, newTCGSYPListScraper)
}
