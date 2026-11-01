package tcgplayer

import (
	"errors"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
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
	ResourceSKUs         = "tcgplayer.skus"
	ResourceSYPCatalog   = "tcgplayer.sypcatalog"
	ResourceCatalogLinks = "tcgplayer.cataloglinks"
)

// WithSKUs hands the Magic market and sealed scrapers the sku catalog
// LoadTCGSKUs returns, which is what resolves a card to the skus the
// partner API prices. The other games identify a sku by name and number
// instead and read no sku catalog.
func WithSKUs(skus SKUMap) mtgban.Option {
	return mtgban.WithResource(ResourceSKUs, skus)
}

// WithSYPCatalog hands the SYP scraper the catalog LoadSYPCatalog returns,
// which is the step between a sku id the list names and the product and
// finish the datastore knows it by.
func WithSYPCatalog(catalog SYPCatalog) mtgban.Option {
	return mtgban.WithResource(ResourceSYPCatalog, catalog)
}

// WithCatalogLinks hands the Magic index scraper the links LoadCatalogLinks
// returns, so it prices the products the datastore names no card for. The
// sku-driven scrapers take the same links merged into WithSKUs instead.
func WithCatalogLinks(links SKUMap) mtgban.Option {
	return mtgban.WithResource(ResourceCatalogLinks, links)
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

// newTCGIndexScraper builds tcg_index: Magic is priced by its own sku-driven
// scraper, every other game by the shared TCGGameIndex.
func newTCGIndexScraper(b *matcher.Backend, opts mtgban.Options) (mtgban.Scraper, error) {
	publicID, privateID, err := tcgCredentials(opts)
	if err != nil {
		return nil, err
	}

	game := b.Game

	if game == matcher.GameMagic {
		links, err := mtgban.Resource[SKUMap](opts, ResourceCatalogLinks)
		if err != nil {
			return nil, err
		}

		scraper, err := NewScraperIndex(b, publicID, privateID)
		if err != nil {
			return nil, err
		}
		scraper.links = links
		scraper.logCallback = opts.LogCallback
		scraper.affiliate = opts.Affiliate
		if opts.MaxConcurrency != 0 {
			scraper.maxConcurrency = opts.MaxConcurrency
		}
		return scraper, nil
	}

	scraper, err := NewScraperGameIndex(b, publicID, privateID)
	if err != nil {
		return nil, err
	}
	scraper.logCallback = opts.LogCallback
	scraper.affiliate = opts.Affiliate
	if opts.MaxConcurrency != 0 {
		scraper.maxConcurrency = opts.MaxConcurrency
	}
	return scraper, nil
}

// newTCGMarketScraper builds tcg_market: Magic's Market scraper needs the
// sku catalog to run, every other game's TCGGame resolves by name and
// number alone.
func newTCGMarketScraper(b *matcher.Backend, opts mtgban.Options) (mtgban.Scraper, error) {
	publicID, privateID, err := tcgCredentials(opts)
	if err != nil {
		return nil, err
	}

	game := b.Game

	if game == matcher.GameMagic {
		skus, err := mtgban.Resource[SKUMap](opts, ResourceSKUs)
		if err != nil {
			return nil, err
		}
		if skus == nil {
			return nil, errors.New("tcg_market needs WithSKUs on Magic")
		}

		scraper, err := NewScraperMarket(b, publicID, privateID)
		if err != nil {
			return nil, err
		}
		scraper.skusData = skus
		scraper.logCallback = opts.LogCallback
		scraper.affiliate = opts.Affiliate
		if opts.MaxConcurrency != 0 {
			scraper.maxConcurrency = opts.MaxConcurrency
		}
		return scraper, nil
	}

	scraper, err := NewScraperGame(b, publicID, privateID)
	if err != nil {
		return nil, err
	}
	scraper.logCallback = opts.LogCallback
	scraper.affiliate = opts.Affiliate
	if opts.MaxConcurrency != 0 {
		scraper.maxConcurrency = opts.MaxConcurrency
	}
	return scraper, nil
}

// newTCGSealedScraper builds tcg_sealed: Magic's Sealed scraper needs the
// sku catalog the same way its Market sibling does, every other game's
// TCGGame resolves sealed products through the datastore's sealed product
// map instead.
func newTCGSealedScraper(b *matcher.Backend, opts mtgban.Options) (mtgban.Scraper, error) {
	publicID, privateID, err := tcgCredentials(opts)
	if err != nil {
		return nil, err
	}

	game := b.Game

	if game == matcher.GameMagic {
		skus, err := mtgban.Resource[SKUMap](opts, ResourceSKUs)
		if err != nil {
			return nil, err
		}
		if skus == nil {
			return nil, errors.New("tcg_sealed needs WithSKUs on Magic")
		}

		scraper, err := NewScraperSealed(b, publicID, privateID)
		if err != nil {
			return nil, err
		}
		scraper.skusData = skus
		scraper.logCallback = opts.LogCallback
		scraper.affiliate = opts.Affiliate
		if opts.MaxConcurrency != 0 {
			scraper.maxConcurrency = opts.MaxConcurrency
		}
		return scraper, nil
	}

	scraper, err := NewScraperGameSealed(b, publicID, privateID)
	if err != nil {
		return nil, err
	}
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
