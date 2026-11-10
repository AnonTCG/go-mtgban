package starcitygames

import (
	"errors"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
)

// AnonTCG: the SCG scrapers run on the sellyourcards sell-list API, not
// upstream's catalog API — we have no catalog x-api-key (SCG_API_KEY), and
// the sell-list path is what production has been running. The bearer is
// scraped from the public app.js at workflow time; the GUID is only needed
// by the retail side, so a buylist-only run (bantool -vendors) may omit it.
const (
	SecretGUID   = "SCG_GUID"
	SecretBearer = "SCG_BEARER"
)

// Only Magic is registered: the other games upstream serves need its
// catalog scraper.
var starcitygamesGames = []matcher.Game{matcher.GameMagic}

func scgCredentials(opts mtgban.Options) (guid, bearer string, err error) {
	bearer, err = opts.Secret(SecretBearer)
	if err != nil {
		return "", "", err
	}
	guid, err = opts.Secret(SecretGUID)
	if err != nil {
		if !opts.DisableRetail || !errors.Is(err, mtgban.ErrMissingSecret) {
			return "", "", err
		}
		guid = ""
	}
	return guid, bearer, nil
}

func init() {
	mtgban.Register("starcitygames", starcitygamesGames,
		func(b *matcher.Backend, opts mtgban.Options) (mtgban.Scraper, error) {
			guid, bearer, err := scgCredentials(opts)
			if err != nil {
				return nil, err
			}
			scraper := NewScraper(b, GameMagic, guid, bearer)
			scraper.LogCallback = opts.LogCallback
			scraper.Affiliate = opts.Affiliate
			scraper.TargetEdition = opts.TargetEdition
			if opts.MaxConcurrency != 0 {
				scraper.MaxConcurrency = opts.MaxConcurrency
			}
			return scraper, nil
		})
	mtgban.Register("starcitygames_sealed", starcitygamesGames,
		func(b *matcher.Backend, opts mtgban.Options) (mtgban.Scraper, error) {
			guid, bearer, err := scgCredentials(opts)
			if err != nil {
				return nil, err
			}
			scraper := NewScraperSealed(b, guid, bearer)
			scraper.LogCallback = opts.LogCallback
			scraper.Affiliate = opts.Affiliate
			if opts.MaxConcurrency != 0 {
				scraper.MaxConcurrency = opts.MaxConcurrency
			}
			return scraper, nil
		})
}
