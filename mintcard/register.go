package mintcard

import (
	"errors"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
	"github.com/mtgban/go-mtgban/tcgplayer"

	api "github.com/mtgban/go-tcgplayer"
)

func init() {
	mtgban.Register("mintcard", []matcher.Game{matcher.GameMagic}, newScraper)
}

func newScraper(b *matcher.Backend, opts mtgban.Options) (mtgban.Scraper, error) {
	// The catalog names the sku each listing carries, the same catalog the
	// tcgplayer scrapers price from
	catalog, err := mtgban.Resource[*api.CatalogDump](opts, tcgplayer.ResourceCatalog)
	if err != nil {
		return nil, err
	}
	if catalog == nil {
		return nil, errors.New("mintcard needs tcgplayer.WithCatalog")
	}

	scraper := NewScraper(b)
	scraper.catalog = catalog
	scraper.logCallback = opts.LogCallback
	scraper.partner = opts.Affiliate
	return scraper, nil
}
