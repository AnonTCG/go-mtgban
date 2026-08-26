// Command bantool runs the scrapers and writes what they return, either to
// disk or to a cloud bucket, one file per scraper and price kind.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/scizorman/go-ndjson"

	_ "github.com/joho/godotenv/autoload"
	"github.com/mtgban/go-mtgban/abugames"
	"github.com/mtgban/go-mtgban/arcanafrisia"
	"github.com/mtgban/go-mtgban/cardkingdom"
	"github.com/mtgban/go-mtgban/cardmarket"
	"github.com/mtgban/go-mtgban/cardtrader"
	"github.com/mtgban/go-mtgban/coolstuffinc"
	"github.com/mtgban/go-mtgban/hareruya"
	"github.com/mtgban/go-mtgban/magiccorner"
	"github.com/mtgban/go-mtgban/manapool"
	"github.com/mtgban/go-mtgban/merlion"
	"github.com/mtgban/go-mtgban/miniaturemarket"
	"github.com/mtgban/go-mtgban/mintcard"
	"github.com/mtgban/go-mtgban/mtgmatcher"
	"github.com/mtgban/go-mtgban/mtgseattle"
	"github.com/mtgban/go-mtgban/sealedev"
	"github.com/mtgban/go-mtgban/starcitygames"
	"github.com/mtgban/go-mtgban/strikezone"
	"github.com/mtgban/go-mtgban/tcgplayer"
	"github.com/mtgban/go-mtgban/trollandtoad"
	"github.com/mtgban/go-mtgban/vegassingles"

	"github.com/mtgban/go-mtgban/mtgban"
	"github.com/mtgban/simplecloud"

	_ "github.com/mtgban/go-mtgban/mtgmatcher/games"
)

// GlobalLogCallback is handed to every scraper, so one run logs in one place.
var GlobalLogCallback mtgban.LogCallbackFunc = log.Printf

// MaxConcurrency overrides each scraper's own default when set, so a whole run
// can be slowed down at once.
var MaxConcurrency int

// Commit is the revision this binary was built from, read out of the build
// info rather than stamped at link time.
var Commit = func() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
	}
	return ""
}()

type scraperOption struct {
	Enabled    bool
	OnlySeller bool
	OnlyVendor bool
	Init       func() (mtgban.Scraper, error)
}

// cardtraderBridge maps every Cardmarket product id to the TCGplayer id of
// the same product, read off cardtrader's blueprints - the one source
// linking the two marketplaces' ids. The blueprints cover singles and
// sealed alike, so the same bridge serves both cardmarket scrapers; they
// receive it as plain data, and the composition of the two vendors happens
// here and nowhere else.
func cardtraderBridge(gameID int) (map[int]int, error) {
	ctTokenBearer := os.Getenv("CARDTRADER_TOKEN_BEARER")
	if ctTokenBearer == "" {
		return nil, errors.New("missing CARDTRADER_TOKEN_BEARER env var")
	}
	client := cardtrader.NewCTAuthClient(ctTokenBearer)

	blueprints, _, err := cardtrader.BlueprintsForGame(context.Background(), client, gameID, "", log.Printf)
	if err != nil {
		return nil, err
	}

	bridge := map[int]int{}
	for _, bp := range blueprints {
		if bp.TCGplayerID == 0 {
			continue
		}
		for _, mkmID := range bp.CardMarketIDs {
			bridge[mkmID] = bp.TCGplayerID
		}
	}
	log.Printf("bridge: %d cardmarket ids linked to a tcgplayer id", len(bridge))
	return bridge, nil
}

func init() {
	MaxConcurrency, _ = strconv.Atoi(os.Getenv("MAX_CONCURRENCY"))

	log.Println("Workers running with", MaxConcurrency, "parallel threads")
}

var options = map[string]*scraperOption{
	"abugames": {
		Init: func() (mtgban.Scraper, error) {
			scraper := abugames.NewScraper()
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"abugames_sealed": {
		Init: func() (mtgban.Scraper, error) {
			scraper := abugames.NewScraperSealed()
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"arcanafrisia": {
		Init: func() (mtgban.Scraper, error) {
			scraper := arcanafrisia.NewScraper()
			scraper.LogCallback = GlobalLogCallback
			return scraper, nil
		},
	},
	"cardkingdom": {
		Init: func() (mtgban.Scraper, error) {
			scraper := cardkingdom.NewScraper()
			scraper.LogCallback = GlobalLogCallback
			scraper.Partner = os.Getenv("CK_PARTNER")
			scraper.PreserveOOS = true
			return scraper, nil
		},
	},
	"cardkingdom_sealed": {
		Init: func() (mtgban.Scraper, error) {
			scraper := cardkingdom.NewScraperSealed()
			scraper.LogCallback = GlobalLogCallback
			scraper.Partner = os.Getenv("CK_PARTNER")
			scraper.PreserveOOS = true
			return scraper, nil
		},
	},
	"cardkingdom_graded": {
		Init: func() (mtgban.Scraper, error) {
			scraper, err := cardkingdom.NewScraperGraded()
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Partner = os.Getenv("CK_PARTNER")
			return scraper, nil
		},
	},
	"cardmarket": {
		Init: func() (mtgban.Scraper, error) {
			mkmAppToken := os.Getenv("MKM_APP_TOKEN")
			mkmAppSecret := os.Getenv("MKM_APP_SECRET")
			if mkmAppToken == "" || mkmAppSecret == "" {
				return nil, errors.New("missing MKM_APP_TOKEN or MKM_APP_SECRET env vars")
			}

			scraper, err := cardmarket.NewScraperIndex(cardmarket.GameMagic, mkmAppToken, mkmAppSecret)
			if err != nil {
				return nil, err
			}
			scraper.Affiliate = "mtgban"
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("MKM_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardmarket_sealed": {
		Init: func() (mtgban.Scraper, error) {
			mkmAppToken := os.Getenv("MKM_APP_TOKEN")
			mkmAppSecret := os.Getenv("MKM_APP_SECRET")
			if mkmAppToken == "" || mkmAppSecret == "" {
				return nil, errors.New("missing MKM_APP_TOKEN or MKM_APP_SECRET env vars")
			}

			scraper, err := cardmarket.NewScraperSealed(cardmarket.GameMagic, mkmAppToken, mkmAppSecret)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("MKM_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardtrader": {
		Init: func() (mtgban.Scraper, error) {
			ctTokenBearer := os.Getenv("CARDTRADER_TOKEN_BEARER")
			if ctTokenBearer == "" {
				return nil, errors.New("missing CARDTRADER_TOKEN_BEARER env var")
			}

			scraper, err := cardtrader.NewScraperMarket(cardtrader.GameMagic, ctTokenBearer)
			if err != nil {
				return nil, err
			}
			scraper.ShareCode = os.Getenv("CT_PARTNER")
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardtrader_sealed": {
		Init: func() (mtgban.Scraper, error) {
			ctTokenBearer := os.Getenv("CARDTRADER_TOKEN_BEARER")
			if ctTokenBearer == "" {
				return nil, errors.New("missing CARDTRADER_TOKEN_BEARER env var")
			}

			scraper, err := cardtrader.NewScraperSealed(cardtrader.GameMagic, ctTokenBearer)
			if err != nil {
				return nil, err
			}
			scraper.ShareCode = os.Getenv("CT_PARTNER")
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"coolstuffinc": {
		Init: func() (mtgban.Scraper, error) {
			scraper := coolstuffinc.NewScraper(coolstuffinc.GameMagic)
			scraper.LogCallback = GlobalLogCallback
			scraper.Partner = os.Getenv("CSI_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"coolstuffinc_sealed": {
		Init: func() (mtgban.Scraper, error) {
			scraper := coolstuffinc.NewScraperSealed(coolstuffinc.GameMagic)
			scraper.LogCallback = GlobalLogCallback
			scraper.Partner = os.Getenv("CSI_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"hareruya": {
		Init: func() (mtgban.Scraper, error) {
			scraper := hareruya.NewScraper()
			scraper.LogCallback = GlobalLogCallback
			return scraper, nil
		},
	},
	"hareruya_sealed": {
		Init: func() (mtgban.Scraper, error) {
			scraper := hareruya.NewScraperSealed()
			scraper.LogCallback = GlobalLogCallback
			return scraper, nil
		},
	},
	"magiccorner": {
		Init: func() (mtgban.Scraper, error) {
			scraper, err := magiccorner.NewScraper()
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"manapool": {
		Init: func() (mtgban.Scraper, error) {
			scraper := manapool.NewScraper()
			scraper.Partner = os.Getenv("MP_PARTNER")
			scraper.LogCallback = GlobalLogCallback
			return scraper, nil
		},
	},
	"manapool_index": {
		Init: func() (mtgban.Scraper, error) {
			scraper := manapool.NewScraperIndex()
			scraper.Partner = os.Getenv("MP_PARTNER")
			scraper.LogCallback = GlobalLogCallback
			return scraper, nil
		},
	},
	"manapool_sealed": {
		Init: func() (mtgban.Scraper, error) {
			scraper := manapool.NewScraperSealed()
			scraper.Partner = os.Getenv("MP_PARTNER")
			scraper.LogCallback = GlobalLogCallback
			return scraper, nil
		},
	},
	"miniaturemarket_sealed_lorcana": {
		Init: func() (mtgban.Scraper, error) {
			scraper := miniaturemarket.NewScraperSealed(miniaturemarket.GameLorcana)
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("MM_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"miniaturemarket_sealed_riftbound": {
		Init: func() (mtgban.Scraper, error) {
			scraper := miniaturemarket.NewScraperSealed(miniaturemarket.GameRiftbound)
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("MM_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"miniaturemarket_sealed_onepiece": {
		Init: func() (mtgban.Scraper, error) {
			scraper := miniaturemarket.NewScraperSealed(miniaturemarket.GameOnePiece)
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("MM_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"miniaturemarket_sealed_fleshandblood": {
		Init: func() (mtgban.Scraper, error) {
			scraper := miniaturemarket.NewScraperSealed(miniaturemarket.GameFleshAndBlood)
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("MM_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"miniaturemarket_sealed": {
		Init: func() (mtgban.Scraper, error) {
			scraper := miniaturemarket.NewScraperSealed(miniaturemarket.GameMagic)
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"mintcard": {
		Init: func() (mtgban.Scraper, error) {
			tcgSKUPath := os.Getenv("MTGJSON_TCGSKU_PATH")
			if tcgSKUPath == "" {
				return nil, errors.New("missing MTGJSON_TCGSKU_PATH env var")
			}

			scraper := mintcard.NewScraper()
			scraper.LogCallback = GlobalLogCallback
			scraper.Partner = os.Getenv("MINT_PARTNER")

			start := time.Now()
			skuBucket, err := initializeBucket(tcgSKUPath, os.Getenv("B2_KEY_ID_DATASTORE"), os.Getenv("B2_APP_KEY_DATASTORE"))
			if err != nil {
				return nil, err
			}
			skuReader, err := simplecloud.InitReader(context.Background(), skuBucket, tcgSKUPath)
			if err != nil {
				return nil, err
			}
			defer skuReader.Close()
			skus, err := tcgplayer.LoadTCGSKUs(skuReader)
			if err != nil {
				return nil, err
			}
			scraper.SKUsData = skus
			log.Println("loading skus took:", time.Since(start))

			return scraper, nil
		},
	},
	"mtgseattle": {
		OnlySeller: true,
		Init: func() (mtgban.Scraper, error) {
			scraper := mtgseattle.NewScraper()
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"sealed_ev": {
		Init: func() (mtgban.Scraper, error) {
			banKey := os.Getenv("BAN_API_KEY")
			if banKey == "" {
				return nil, errors.New("missing BAN_API_KEY env var")
			}
			scraper := sealedev.NewScraper(banKey)
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			scraper.BuylistAffiliate = os.Getenv("CK_PARTNER")
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	// AnonTCG: the SCG scrapers run on the sellyourcards sell-list API
	// (SCG_BEARER scraped from the public app.js at workflow time), not
	// upstream's catalog API — we have no catalog x-api-key, and the
	// sell-list path is what production has been running. SCG_BUYLIST_ONLY
	// skips the retail side, which needs the GUID we don't have.
	"starcitygames": {
		Init: func() (mtgban.Scraper, error) {
			scgGUID := os.Getenv("SCG_GUID")
			scgBearer := os.Getenv("SCG_BEARER")
			if scgBearer == "" {
				return nil, errors.New("missing SCG_BEARER env var")
			}
			if scgGUID == "" && os.Getenv("SCG_BUYLIST_ONLY") != "true" {
				return nil, errors.New("missing SCG_GUID env var")
			}

			scraper := starcitygames.NewScraper(starcitygames.GameMagic, scgGUID, scgBearer)
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("SCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"starcitygames_sealed": {
		Init: func() (mtgban.Scraper, error) {
			scgGUID := os.Getenv("SCG_GUID")
			scgBearer := os.Getenv("SCG_BEARER")
			if scgBearer == "" {
				return nil, errors.New("missing SCG_BEARER env var")
			}
			if scgGUID == "" && os.Getenv("SCG_BUYLIST_ONLY") != "true" {
				return nil, errors.New("missing SCG_GUID env var")
			}

			scraper := starcitygames.NewScraperSealed(scgGUID, scgBearer)
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("SCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"strikezone": {
		Init: func() (mtgban.Scraper, error) {
			scraper := strikezone.NewScraper(strikezone.GameMagic)
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_index": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}

			scraper, err := tcgplayer.NewScraperIndex(tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}

			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_market": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			tcgSKUPath := os.Getenv("MTGJSON_TCGSKU_PATH")
			if tcgPublicID == "" || tcgPrivateID == "" || tcgSKUPath == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY or MTGJSON_TCGSKU_PATH env vars")
			}

			scraper, err := tcgplayer.NewScraperMarket(tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}

			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}

			start := time.Now()
			skuBucket, err := initializeBucket(tcgSKUPath, os.Getenv("B2_KEY_ID_DATASTORE"), os.Getenv("B2_APP_KEY_DATASTORE"))
			if err != nil {
				return nil, err
			}
			skuReader, err := simplecloud.InitReader(context.Background(), skuBucket, tcgSKUPath)
			if err != nil {
				return nil, err
			}
			defer skuReader.Close()
			skus, err := tcgplayer.LoadTCGSKUs(skuReader)
			if err != nil {
				return nil, err
			}
			scraper.SKUsData = skus
			log.Println("loading skus took:", time.Since(start))

			return scraper, nil
		},
	},
	"tcg_sealed": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			tcgSKUPath := os.Getenv("MTGJSON_TCGSKU_PATH")
			if tcgPublicID == "" || tcgPrivateID == "" || tcgSKUPath == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY or MTGJSON_TCGSKU_PATH env vars")
			}

			scraper, err := tcgplayer.NewScraperSealed(tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}

			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}

			start := time.Now()
			skuBucket, err := initializeBucket(tcgSKUPath, os.Getenv("B2_KEY_ID_DATASTORE"), os.Getenv("B2_APP_KEY_DATASTORE"))
			if err != nil {
				return nil, err
			}
			skuReader, err := simplecloud.InitReader(context.Background(), skuBucket, tcgSKUPath)
			if err != nil {
				return nil, err
			}
			defer skuReader.Close()
			skus, err := tcgplayer.LoadTCGSKUs(skuReader)
			if err != nil {
				return nil, err
			}
			scraper.SKUsData = skus
			log.Println("loading skus took:", time.Since(start))

			return scraper, nil
		},
	},
	"tcg_syplist": {
		Init: func() (mtgban.Scraper, error) {
			tcgAuth := os.Getenv("TCGPLAYER_AUTH")
			tcgSKUPath := os.Getenv("MTGJSON_TCGSKU_PATH")
			if tcgAuth == "" || tcgSKUPath == "" {
				return nil, errors.New("missing TCGPLAYER_AUTH or MTGJSON_TCGSKU_PATH env var")
			}
			scraper := tcgplayer.NewScraperSYP(tcgAuth)
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")

			start := time.Now()
			skuBucket, err := initializeBucket(tcgSKUPath, os.Getenv("B2_KEY_ID_DATASTORE"), os.Getenv("B2_APP_KEY_DATASTORE"))
			if err != nil {
				return nil, err
			}
			skuReader, err := simplecloud.InitReader(context.Background(), skuBucket, tcgSKUPath)
			if err != nil {
				return nil, err
			}
			defer skuReader.Close()
			skus, err := tcgplayer.LoadTCGSKUs(skuReader)
			if err != nil {
				return nil, err
			}
			scraper.SKUsData = skus
			log.Println("loading skus took:", time.Since(start))

			return scraper, nil
		},
	},
	"trollandtoad": {
		Init: func() (mtgban.Scraper, error) {
			scraper := trollandtoad.NewScraper()
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"vegassingles": {
		Init: func() (mtgban.Scraper, error) {
			scraper := vegassingles.NewScraper(vegassingles.GameMagic)
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},

	"cardmarket_lorcana": {
		Init: func() (mtgban.Scraper, error) {
			mkmAppToken := os.Getenv("MKM_APP_TOKEN")
			mkmAppSecret := os.Getenv("MKM_APP_SECRET")
			if mkmAppToken == "" || mkmAppSecret == "" {
				return nil, errors.New("missing MKM_APP_TOKEN or MKM_APP_SECRET env vars")
			}

			scraper, err := cardmarket.NewScraperIndex(cardmarket.GameLorcana, mkmAppToken, mkmAppSecret)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("MKM_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardtrader_lorcana": {
		Init: func() (mtgban.Scraper, error) {
			ctTokenBearer := os.Getenv("CARDTRADER_TOKEN_BEARER")
			if ctTokenBearer == "" {
				return nil, errors.New("missing CARDTRADER_TOKEN_BEARER env var")
			}

			scraper, err := cardtrader.NewScraperMarket(cardtrader.GameLorcana, ctTokenBearer)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.ShareCode = os.Getenv("CT_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardmarket_sealed_lorcana": {
		Init: func() (mtgban.Scraper, error) {
			mkmAppToken := os.Getenv("MKM_APP_TOKEN")
			mkmAppSecret := os.Getenv("MKM_APP_SECRET")
			if mkmAppToken == "" || mkmAppSecret == "" {
				return nil, errors.New("missing MKM_APP_TOKEN or MKM_APP_SECRET env vars")
			}

			scraper, err := cardmarket.NewScraperSealed(cardmarket.GameLorcana, mkmAppToken, mkmAppSecret)
			if err != nil {
				return nil, err
			}
			scraper.TCGBridge, err = cardtraderBridge(cardtrader.GameLorcana)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("MKM_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardtrader_sealed_lorcana": {
		Init: func() (mtgban.Scraper, error) {
			ctTokenBearer := os.Getenv("CARDTRADER_TOKEN_BEARER")
			if ctTokenBearer == "" {
				return nil, errors.New("missing CARDTRADER_TOKEN_BEARER env var")
			}

			scraper, err := cardtrader.NewScraperSealed(cardtrader.GameLorcana, ctTokenBearer)
			if err != nil {
				return nil, err
			}
			scraper.ShareCode = os.Getenv("CT_PARTNER")
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"coolstuffinc_sealed_lorcana": {
		Init: func() (mtgban.Scraper, error) {
			scraper := coolstuffinc.NewScraperSealed(coolstuffinc.GameLorcana)
			scraper.LogCallback = GlobalLogCallback
			scraper.Partner = os.Getenv("CSI_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"coolstuffinc_lorcana": {
		Init: func() (mtgban.Scraper, error) {
			scraper := coolstuffinc.NewScraper(coolstuffinc.GameLorcana)
			scraper.LogCallback = GlobalLogCallback
			scraper.Partner = os.Getenv("CSI_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"strikezone_lorcana": {
		Init: func() (mtgban.Scraper, error) {
			scraper := strikezone.NewScraper(strikezone.GameLorcana)
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"strikezone_pokemon": {
		Init: func() (mtgban.Scraper, error) {
			scraper := strikezone.NewScraper(strikezone.GamePokemon)
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"strikezone_fleshandblood": {
		Init: func() (mtgban.Scraper, error) {
			scraper := strikezone.NewScraper(strikezone.GameFleshAndBlood)
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_index_lorcana": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}
			scraper, err := tcgplayer.NewScraperGameIndex(mtgban.GameLorcana, tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_market_lorcana": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}
			scraper, err := tcgplayer.NewScraperGame(mtgban.GameLorcana, tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_sealed_lorcana": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}
			scraper, err := tcgplayer.NewScraperGameSealed(mtgban.GameLorcana, tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},

	"cardmarket_riftbound": {
		Init: func() (mtgban.Scraper, error) {
			mkmAppToken := os.Getenv("MKM_APP_TOKEN")
			mkmAppSecret := os.Getenv("MKM_APP_SECRET")
			if mkmAppToken == "" || mkmAppSecret == "" {
				return nil, errors.New("missing MKM_APP_TOKEN or MKM_APP_SECRET env vars")
			}

			scraper, err := cardmarket.NewScraperIndex(cardmarket.GameRiftbound, mkmAppToken, mkmAppSecret)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("MKM_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardmarket_onepiece": {
		Init: func() (mtgban.Scraper, error) {
			mkmAppToken := os.Getenv("MKM_APP_TOKEN")
			mkmAppSecret := os.Getenv("MKM_APP_SECRET")
			if mkmAppToken == "" || mkmAppSecret == "" {
				return nil, errors.New("missing MKM_APP_TOKEN or MKM_APP_SECRET env vars")
			}

			scraper, err := cardmarket.NewScraperIndex(cardmarket.GameOnePiece, mkmAppToken, mkmAppSecret)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("MKM_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardmarket_yugioh": {
		Init: func() (mtgban.Scraper, error) {
			mkmAppToken := os.Getenv("MKM_APP_TOKEN")
			mkmAppSecret := os.Getenv("MKM_APP_SECRET")
			if mkmAppToken == "" || mkmAppSecret == "" {
				return nil, errors.New("missing MKM_APP_TOKEN or MKM_APP_SECRET env vars")
			}

			scraper, err := cardmarket.NewScraperIndex(cardmarket.GameYuGiOh, mkmAppToken, mkmAppSecret)
			if err != nil {
				return nil, err
			}
			scraper.TCGBridge, err = cardtraderBridge(cardtrader.GameYuGiOh)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("MKM_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardmarket_fleshandblood": {
		Init: func() (mtgban.Scraper, error) {
			mkmAppToken := os.Getenv("MKM_APP_TOKEN")
			mkmAppSecret := os.Getenv("MKM_APP_SECRET")
			if mkmAppToken == "" || mkmAppSecret == "" {
				return nil, errors.New("missing MKM_APP_TOKEN or MKM_APP_SECRET env vars")
			}

			scraper, err := cardmarket.NewScraperIndex(cardmarket.GameFleshAndBlood, mkmAppToken, mkmAppSecret)
			if err != nil {
				return nil, err
			}
			scraper.TCGBridge, err = cardtraderBridge(cardtrader.GameFleshAndBlood)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("MKM_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardmarket_pokemon": {
		Init: func() (mtgban.Scraper, error) {
			mkmAppToken := os.Getenv("MKM_APP_TOKEN")
			mkmAppSecret := os.Getenv("MKM_APP_SECRET")
			if mkmAppToken == "" || mkmAppSecret == "" {
				return nil, errors.New("missing MKM_APP_TOKEN or MKM_APP_SECRET env vars")
			}

			scraper, err := cardmarket.NewScraperIndex(cardmarket.GamePokemon, mkmAppToken, mkmAppSecret)
			if err != nil {
				return nil, err
			}
			scraper.TCGBridge, err = cardtraderBridge(cardtrader.GamePokemon)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("MKM_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardtrader_riftbound": {
		Init: func() (mtgban.Scraper, error) {
			ctTokenBearer := os.Getenv("CARDTRADER_TOKEN_BEARER")
			if ctTokenBearer == "" {
				return nil, errors.New("missing CARDTRADER_TOKEN_BEARER env var")
			}

			scraper, err := cardtrader.NewScraperMarket(cardtrader.GameRiftbound, ctTokenBearer)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.ShareCode = os.Getenv("CT_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardtrader_onepiece": {
		Init: func() (mtgban.Scraper, error) {
			ctTokenBearer := os.Getenv("CARDTRADER_TOKEN_BEARER")
			if ctTokenBearer == "" {
				return nil, errors.New("missing CARDTRADER_TOKEN_BEARER env var")
			}

			scraper, err := cardtrader.NewScraperMarket(cardtrader.GameOnePiece, ctTokenBearer)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.ShareCode = os.Getenv("CT_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardtrader_yugioh": {
		Init: func() (mtgban.Scraper, error) {
			ctTokenBearer := os.Getenv("CARDTRADER_TOKEN_BEARER")
			if ctTokenBearer == "" {
				return nil, errors.New("missing CARDTRADER_TOKEN_BEARER env var")
			}

			scraper, err := cardtrader.NewScraperMarket(cardtrader.GameYuGiOh, ctTokenBearer)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.ShareCode = os.Getenv("CT_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardtrader_fleshandblood": {
		Init: func() (mtgban.Scraper, error) {
			ctTokenBearer := os.Getenv("CARDTRADER_TOKEN_BEARER")
			if ctTokenBearer == "" {
				return nil, errors.New("missing CARDTRADER_TOKEN_BEARER env var")
			}

			scraper, err := cardtrader.NewScraperMarket(cardtrader.GameFleshAndBlood, ctTokenBearer)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.ShareCode = os.Getenv("CT_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardtrader_pokemon": {
		Init: func() (mtgban.Scraper, error) {
			ctTokenBearer := os.Getenv("CARDTRADER_TOKEN_BEARER")
			if ctTokenBearer == "" {
				return nil, errors.New("missing CARDTRADER_TOKEN_BEARER env var")
			}

			scraper, err := cardtrader.NewScraperMarket(cardtrader.GamePokemon, ctTokenBearer)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.ShareCode = os.Getenv("CT_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardmarket_sealed_riftbound": {
		Init: func() (mtgban.Scraper, error) {
			mkmAppToken := os.Getenv("MKM_APP_TOKEN")
			mkmAppSecret := os.Getenv("MKM_APP_SECRET")
			if mkmAppToken == "" || mkmAppSecret == "" {
				return nil, errors.New("missing MKM_APP_TOKEN or MKM_APP_SECRET env vars")
			}

			scraper, err := cardmarket.NewScraperSealed(cardmarket.GameRiftbound, mkmAppToken, mkmAppSecret)
			if err != nil {
				return nil, err
			}
			scraper.TCGBridge, err = cardtraderBridge(cardtrader.GameRiftbound)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("MKM_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardmarket_sealed_onepiece": {
		Init: func() (mtgban.Scraper, error) {
			mkmAppToken := os.Getenv("MKM_APP_TOKEN")
			mkmAppSecret := os.Getenv("MKM_APP_SECRET")
			if mkmAppToken == "" || mkmAppSecret == "" {
				return nil, errors.New("missing MKM_APP_TOKEN or MKM_APP_SECRET env vars")
			}

			scraper, err := cardmarket.NewScraperSealed(cardmarket.GameOnePiece, mkmAppToken, mkmAppSecret)
			if err != nil {
				return nil, err
			}
			scraper.TCGBridge, err = cardtraderBridge(cardtrader.GameOnePiece)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("MKM_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardmarket_sealed_yugioh": {
		Init: func() (mtgban.Scraper, error) {
			mkmAppToken := os.Getenv("MKM_APP_TOKEN")
			mkmAppSecret := os.Getenv("MKM_APP_SECRET")
			if mkmAppToken == "" || mkmAppSecret == "" {
				return nil, errors.New("missing MKM_APP_TOKEN or MKM_APP_SECRET env vars")
			}

			scraper, err := cardmarket.NewScraperSealed(cardmarket.GameYuGiOh, mkmAppToken, mkmAppSecret)
			if err != nil {
				return nil, err
			}
			scraper.TCGBridge, err = cardtraderBridge(cardtrader.GameYuGiOh)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("MKM_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardmarket_sealed_fleshandblood": {
		Init: func() (mtgban.Scraper, error) {
			mkmAppToken := os.Getenv("MKM_APP_TOKEN")
			mkmAppSecret := os.Getenv("MKM_APP_SECRET")
			if mkmAppToken == "" || mkmAppSecret == "" {
				return nil, errors.New("missing MKM_APP_TOKEN or MKM_APP_SECRET env vars")
			}

			scraper, err := cardmarket.NewScraperSealed(cardmarket.GameFleshAndBlood, mkmAppToken, mkmAppSecret)
			if err != nil {
				return nil, err
			}
			scraper.TCGBridge, err = cardtraderBridge(cardtrader.GameFleshAndBlood)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("MKM_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardmarket_sealed_pokemon": {
		Init: func() (mtgban.Scraper, error) {
			mkmAppToken := os.Getenv("MKM_APP_TOKEN")
			mkmAppSecret := os.Getenv("MKM_APP_SECRET")
			if mkmAppToken == "" || mkmAppSecret == "" {
				return nil, errors.New("missing MKM_APP_TOKEN or MKM_APP_SECRET env vars")
			}

			scraper, err := cardmarket.NewScraperSealed(cardmarket.GamePokemon, mkmAppToken, mkmAppSecret)
			if err != nil {
				return nil, err
			}
			scraper.TCGBridge, err = cardtraderBridge(cardtrader.GamePokemon)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("MKM_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardtrader_sealed_riftbound": {
		Init: func() (mtgban.Scraper, error) {
			ctTokenBearer := os.Getenv("CARDTRADER_TOKEN_BEARER")
			if ctTokenBearer == "" {
				return nil, errors.New("missing CARDTRADER_TOKEN_BEARER env var")
			}

			scraper, err := cardtrader.NewScraperSealed(cardtrader.GameRiftbound, ctTokenBearer)
			if err != nil {
				return nil, err
			}
			scraper.ShareCode = os.Getenv("CT_PARTNER")
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardtrader_sealed_onepiece": {
		Init: func() (mtgban.Scraper, error) {
			ctTokenBearer := os.Getenv("CARDTRADER_TOKEN_BEARER")
			if ctTokenBearer == "" {
				return nil, errors.New("missing CARDTRADER_TOKEN_BEARER env var")
			}

			scraper, err := cardtrader.NewScraperSealed(cardtrader.GameOnePiece, ctTokenBearer)
			if err != nil {
				return nil, err
			}
			scraper.ShareCode = os.Getenv("CT_PARTNER")
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardtrader_sealed_yugioh": {
		Init: func() (mtgban.Scraper, error) {
			ctTokenBearer := os.Getenv("CARDTRADER_TOKEN_BEARER")
			if ctTokenBearer == "" {
				return nil, errors.New("missing CARDTRADER_TOKEN_BEARER env var")
			}

			scraper, err := cardtrader.NewScraperSealed(cardtrader.GameYuGiOh, ctTokenBearer)
			if err != nil {
				return nil, err
			}
			scraper.ShareCode = os.Getenv("CT_PARTNER")
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardtrader_sealed_fleshandblood": {
		Init: func() (mtgban.Scraper, error) {
			ctTokenBearer := os.Getenv("CARDTRADER_TOKEN_BEARER")
			if ctTokenBearer == "" {
				return nil, errors.New("missing CARDTRADER_TOKEN_BEARER env var")
			}

			scraper, err := cardtrader.NewScraperSealed(cardtrader.GameFleshAndBlood, ctTokenBearer)
			if err != nil {
				return nil, err
			}
			scraper.ShareCode = os.Getenv("CT_PARTNER")
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"cardtrader_sealed_pokemon": {
		Init: func() (mtgban.Scraper, error) {
			ctTokenBearer := os.Getenv("CARDTRADER_TOKEN_BEARER")
			if ctTokenBearer == "" {
				return nil, errors.New("missing CARDTRADER_TOKEN_BEARER env var")
			}

			scraper, err := cardtrader.NewScraperSealed(cardtrader.GamePokemon, ctTokenBearer)
			if err != nil {
				return nil, err
			}
			scraper.ShareCode = os.Getenv("CT_PARTNER")
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"coolstuffinc_sealed_riftbound": {
		Init: func() (mtgban.Scraper, error) {
			scraper := coolstuffinc.NewScraperSealed(coolstuffinc.GameRiftbound)
			scraper.LogCallback = GlobalLogCallback
			scraper.Partner = os.Getenv("CSI_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"coolstuffinc_onepiece": {
		Init: func() (mtgban.Scraper, error) {
			scraper := coolstuffinc.NewScraper(coolstuffinc.GameOnePiece)
			scraper.LogCallback = GlobalLogCallback
			scraper.Partner = os.Getenv("CSI_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"coolstuffinc_pokemon": {
		Init: func() (mtgban.Scraper, error) {
			scraper := coolstuffinc.NewScraper(coolstuffinc.GamePokemon)
			scraper.LogCallback = GlobalLogCallback
			scraper.Partner = os.Getenv("CSI_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"coolstuffinc_yugioh": {
		Init: func() (mtgban.Scraper, error) {
			scraper := coolstuffinc.NewScraper(coolstuffinc.GameYuGiOh)
			scraper.LogCallback = GlobalLogCallback
			scraper.Partner = os.Getenv("CSI_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"coolstuffinc_sealed_onepiece": {
		Init: func() (mtgban.Scraper, error) {
			scraper := coolstuffinc.NewScraperSealed(coolstuffinc.GameOnePiece)
			scraper.LogCallback = GlobalLogCallback
			scraper.Partner = os.Getenv("CSI_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"coolstuffinc_sealed_pokemon": {
		Init: func() (mtgban.Scraper, error) {
			scraper := coolstuffinc.NewScraperSealed(coolstuffinc.GamePokemon)
			scraper.LogCallback = GlobalLogCallback
			scraper.Partner = os.Getenv("CSI_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"coolstuffinc_sealed_yugioh": {
		Init: func() (mtgban.Scraper, error) {
			scraper := coolstuffinc.NewScraperSealed(coolstuffinc.GameYuGiOh)
			scraper.LogCallback = GlobalLogCallback
			scraper.Partner = os.Getenv("CSI_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"coolstuffinc_riftbound": {
		Init: func() (mtgban.Scraper, error) {
			scraper := coolstuffinc.NewScraper(coolstuffinc.GameRiftbound)
			scraper.LogCallback = GlobalLogCallback
			scraper.Partner = os.Getenv("CSI_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"merlion_riftbound": {
		Init: func() (mtgban.Scraper, error) {
			scraper := merlion.NewScraper()
			scraper.LogCallback = GlobalLogCallback
			return scraper, nil
		},
	},
	"tcg_index_riftbound": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}
			scraper, err := tcgplayer.NewScraperGameIndex(mtgban.GameRiftbound, tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_index_onepiece": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}
			scraper, err := tcgplayer.NewScraperGameIndex(mtgban.GameOnePiece, tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_index_yugioh": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}
			scraper, err := tcgplayer.NewScraperGameIndex(mtgban.GameYuGiOh, tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_index_fleshandblood": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}
			scraper, err := tcgplayer.NewScraperGameIndex(mtgban.GameFleshAndBlood, tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_index_pokemon": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}
			scraper, err := tcgplayer.NewScraperGameIndex(mtgban.GamePokemon, tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_market_riftbound": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}
			scraper, err := tcgplayer.NewScraperGame(mtgban.GameRiftbound, tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"vegassingles_riftbound": {
		Init: func() (mtgban.Scraper, error) {
			scraper := vegassingles.NewScraper(vegassingles.GameRiftbound)
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_market_onepiece": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}
			scraper, err := tcgplayer.NewScraperGame(mtgban.GameOnePiece, tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"vegassingles_onepiece": {
		Init: func() (mtgban.Scraper, error) {
			scraper := vegassingles.NewScraper(vegassingles.GameOnePiece)
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_market_yugioh": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}
			scraper, err := tcgplayer.NewScraperGame(mtgban.GameYuGiOh, tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_market_fleshandblood": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}
			scraper, err := tcgplayer.NewScraperGame(mtgban.GameFleshAndBlood, tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_market_pokemon": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}
			scraper, err := tcgplayer.NewScraperGame(mtgban.GamePokemon, tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"vegassingles_pokemon": {
		Init: func() (mtgban.Scraper, error) {
			scraper := vegassingles.NewScraper(vegassingles.GamePokemon)
			scraper.LogCallback = GlobalLogCallback
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_sealed_riftbound": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}
			scraper, err := tcgplayer.NewScraperGameSealed(mtgban.GameRiftbound, tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_sealed_onepiece": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}
			scraper, err := tcgplayer.NewScraperGameSealed(mtgban.GameOnePiece, tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_sealed_yugioh": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}
			scraper, err := tcgplayer.NewScraperGameSealed(mtgban.GameYuGiOh, tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_sealed_fleshandblood": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}
			scraper, err := tcgplayer.NewScraperGameSealed(mtgban.GameFleshAndBlood, tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
	"tcg_sealed_pokemon": {
		Init: func() (mtgban.Scraper, error) {
			tcgPublicID := os.Getenv("TCGPLAYER_PUBLIC_KEY")
			tcgPrivateID := os.Getenv("TCGPLAYER_PRIVATE_KEY")
			if tcgPublicID == "" || tcgPrivateID == "" {
				return nil, errors.New("missing TCGPLAYER_PUBLIC_KEY or TCGPLAYER_PRIVATE_KEY env vars")
			}
			scraper, err := tcgplayer.NewScraperGameSealed(mtgban.GamePokemon, tcgPublicID, tcgPrivateID)
			if err != nil {
				return nil, err
			}
			scraper.LogCallback = GlobalLogCallback
			scraper.Affiliate = os.Getenv("TCG_PARTNER")
			if MaxConcurrency != 0 {
				scraper.MaxConcurrency = MaxConcurrency
			}
			return scraper, nil
		},
	},
}

type inventoryElement struct {
	UUID string
	mtgban.InventoryEntry
}

type buylistElement struct {
	UUID string
	mtgban.BuylistEntry
}

func writeSellerToNDJSON(seller mtgban.Seller, w io.Writer) error {
	inventory := seller.Inventory()

	var inventoryFlat []inventoryElement
	for uuid, entries := range inventory {
		for _, entry := range entries {
			inventoryFlat = append(inventoryFlat, inventoryElement{
				UUID:           uuid,
				InventoryEntry: entry,
			})
		}
	}

	output, err := ndjson.Marshal(inventoryFlat)
	if err != nil {
		return err
	}

	_, err = w.Write(output)
	return err
}

func writeVendorToNDJSON(vendor mtgban.Vendor, w io.Writer) error {
	buylist := vendor.Buylist()

	var buylistFlat []buylistElement
	for uuid, entries := range buylist {
		for _, entry := range entries {
			buylistFlat = append(buylistFlat, buylistElement{
				UUID:         uuid,
				BuylistEntry: entry,
			})
		}
	}

	output, err := ndjson.Marshal(buylistFlat)
	if err != nil {
		return err
	}

	_, err = w.Write(output)
	return err
}

func dumpSeller(dataBucket simplecloud.Writer, seller mtgban.Seller, outputPath, format string) (err error) {
	if len(seller.Inventory()) == 0 {
		return fmt.Errorf("seller %s has no data", seller.Info().Shorthand)
	}

	target := fmt.Sprintf("%s/retail/%s.%s", outputPath, seller.Info().Shorthand, format)
	log.Println("Writing", target)

	writer, err := simplecloud.InitWriter(context.Background(), dataBucket, target)
	if err != nil {
		return err
	}
	// Close is where a buffered cloud writer commits the upload, so it
	// reports whether anything was durably written at all
	defer func() {
		cerr := writer.Close()
		if err == nil {
			err = cerr
		}
	}()

	switch strings.Split(format, ".")[0] {
	case "json":
		err = mtgban.WriteSellerToJSON(seller, writer)
	case "csv":
		err = mtgban.WriteInventoryToCSV(seller.Inventory(), writer)
	case "ndjson":
		err = writeSellerToNDJSON(seller, writer)
	default:
		err = errors.New("invalid format")
	}

	return err
}

func dumpVendor(dataBucket simplecloud.Writer, vendor mtgban.Vendor, outputPath, format string) (err error) {
	if len(vendor.Buylist()) == 0 {
		return fmt.Errorf("vendor %s has no data", vendor.Info().Shorthand)
	}

	target := fmt.Sprintf("%s/buylist/%s.%s", outputPath, vendor.Info().Shorthand, format)
	log.Println("Writing", target)

	writer, err := simplecloud.InitWriter(context.Background(), dataBucket, target)
	if err != nil {
		return err
	}
	// Close is where a buffered cloud writer commits the upload, so it
	// reports whether anything was durably written at all
	defer func() {
		cerr := writer.Close()
		if err == nil {
			err = cerr
		}
	}()

	switch strings.Split(format, ".")[0] {
	case "json":
		err = mtgban.WriteVendorToJSON(vendor, writer)
	case "csv":
		err = mtgban.WriteBuylistToCSV(vendor.Buylist(), vendor.Info().CreditMultiplier, writer)
	case "ndjson":
		err = writeVendorToNDJSON(vendor, writer)
	default:
		err = errors.New("invalid format")
	}

	return err
}

// countResults sums the distinct cards each unfolded half holds, so a run
// can say what it found before writing any of it.
func countResults(sellers []mtgban.Seller, vendors []mtgban.Vendor) (int, int) {
	var retail, buylist int
	for _, seller := range sellers {
		retail += len(seller.Inventory())
	}
	for _, vendor := range vendors {
		buylist += len(vendor.Buylist())
	}
	return retail, buylist
}

// reportSuspectPricings says which cards a scraper priced on both sides at
// prices too close to be the same printing. A shop's buy price sits well under
// what it asks, so a pairing that does not is usually two products meeting on
// one id - and nothing in the run log says so otherwise, because both halves
// resolved perfectly well on their own.
func reportSuspectPricings(sellers []mtgban.Seller, vendors []mtgban.Vendor) {
	for _, seller := range sellers {
		for _, vendor := range vendors {
			if seller.Info().Shorthand != vendor.Info().Shorthand {
				continue
			}

			suspects := mtgban.SuspectPricings(seller.Inventory(), vendor.Buylist(), mtgban.SuspectRatioThreshold)
			if len(suspects) == 0 {
				continue
			}

			log.Printf("[%s] %d cards are bought at %.0f%% or more of their asking price",
				seller.Info().Shorthand, len(suspects), mtgban.SuspectRatioThreshold)
			for _, suspect := range suspects {
				card, _ := mtgmatcher.GetUUID(suspect.CardID)
				log.Printf("[%s] - %.0f%% buy $%.2f ask $%.2f %s %s",
					seller.Info().Shorthand, suspect.Ratio, suspect.BuyPrice, suspect.Price,
					suspect.Conditions, card)
			}
		}
	}
}

func dump(dataBucket simplecloud.Writer, sellers []mtgban.Seller, vendors []mtgban.Vendor, outputPath, format string, meta bool) []error {
	log.Println("Writing results to", outputPath)

	var sellerErrs []error
	for _, seller := range sellers {
		err := dumpSeller(dataBucket, seller, outputPath, format)
		if err != nil {
			log.Println(err)
			sellerErrs = append(sellerErrs, err)
			continue
		}

		if meta && format != "json" {
			sellerMeta := mtgban.NewSellerFromInventory(nil, seller.Info())
			err := dumpSeller(dataBucket, sellerMeta, outputPath, "json")
			if err != nil {
				sellerErrs = append(sellerErrs, err)
				continue
			}
		}
	}

	var vendorErrs []error
	for _, vendor := range vendors {
		err := dumpVendor(dataBucket, vendor, outputPath, format)
		if err != nil {
			log.Println(err)
			vendorErrs = append(vendorErrs, err)
			continue
		}

		if meta && format != "json" {
			vendorMeta := mtgban.NewVendorFromBuylist(nil, vendor.Info())
			err := dumpVendor(dataBucket, vendorMeta, outputPath, "json")
			if err != nil {
				vendorErrs = append(vendorErrs, err)
				continue
			}
		}
	}

	return append(sellerErrs, vendorErrs...)
}

// HTTPBucket reads a datastore over plain HTTP, for the files served rather
// than kept in a cloud bucket. It implements only the reading half.
type HTTPBucket struct {
	Client *http.Client
	URL    *url.URL
}

// NewHTTPBucket returns a bucket rooted at the given URL.
func NewHTTPBucket(client *http.Client, path string) (*HTTPBucket, error) {
	u, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	return &HTTPBucket{
		Client: client,
		URL:    u,
	}, nil
}

// NewReader opens the object at path for reading.
func (h *HTTPBucket) NewReader(ctx context.Context, path string) (io.ReadCloser, error) {
	u := new(url.URL)
	*u = *h.URL
	if h.URL.User != nil {
		u.User = new(url.Userinfo)
		*u.User = *h.URL.User
	}

	u.Path = path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// NewWriter always fails: nothing can be written back over plain HTTP.
func (h *HTTPBucket) NewWriter(ctx context.Context, path string) (io.WriteCloser, error) {
	return nil, errors.New("an http bucket cannot be written to")
}

func initializeBucket(outputPath string, env ...string) (simplecloud.ReadWriter, error) {
	u, err := url.Parse(outputPath)
	if err != nil {
		return nil, err
	}

	var bucket simplecloud.ReadWriter

	switch u.Scheme {
	case "":
		_, err := os.Stat(u.Path)
		if os.IsNotExist(err) {
			return nil, errors.New("path does not exist")
		}
		bucket = &simplecloud.FileBucket{}
	case "http", "https":
		bucket, err = NewHTTPBucket(cleanhttp.DefaultClient(), outputPath)
		if err != nil {
			return nil, err
		}
	case "gs":
		if len(env) < 1 {
			return nil, errors.New("missing required environment variable")
		}
		serviceAcc := env[0]

		bucket, err = simplecloud.NewGCSClient(context.Background(), serviceAcc, u.Host)
		if err != nil {
			return nil, err
		}
	case "b2":
		if len(env) < 2 {
			return nil, errors.New("missing required environment variables")
		}
		accessKey := env[0]
		secretKey := env[1]

		b2Bucket, err := simplecloud.NewB2Client(context.Background(), accessKey, secretKey, u.Host)
		if err != nil {
			return nil, err
		}
		b2Bucket.ConcurrentDownloads = 20
		bucket = b2Bucket
	case "s3":
		accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
		secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
		endpoint := os.Getenv("AWS_ENDPOINT")

		bucket, err = simplecloud.NewS3Client(context.Background(), accessKey, secretKey, u.Host, endpoint, "")
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported path scheme %s", u.Scheme)
	}

	return bucket, nil
}

func run() int {
	start := time.Now()

	for key, val := range options {
		label := key
		if label != "" {
			label = strings.ToUpper(label[:1]) + label[1:]
		}
		flag.BoolVar(&val.Enabled, key, false, "Enable "+label)
	}

	datastoreOpt := flag.String("datastore", "", "Path to AllPrintings file")
	outputPathOpt := flag.String("output-path", "", "Path where to dump results")

	scrapersOpt := flag.String("scrapers", "", "Comma-separated list of scrapers to enable")
	sellersOpt := flag.String("sellers", "", "Comma-separated list of sellers to enable")
	vendorsOpt := flag.String("vendors", "", "Comma-separated list of vendors to enable")

	fileFormatOpt := flag.String("format", "json", "File format of the output files (json/csv/ndjson)")
	metaOpt := flag.Bool("meta", false, "When format is not json, output a second file for scraper metadata")

	signOpt := flag.String("sign", "", "Sign input")
	versionOpt := flag.Bool("v", false, "Print version information")
	flag.Parse()

	log.Println("bantool version", Commit)
	if *versionOpt {
		return 0
	}

	if *signOpt != "" {
		sig, err := signAPI(*signOpt)
		if err != nil {
			log.Println(err)
			return 1
		}
		fmt.Fprintln(os.Stdout, *signOpt+"?sig="+sig)
		return 0
	}

	switch strings.Split(*fileFormatOpt, ".")[0] {
	case "json", "csv", "ndjson":
	default:
		log.Println("Invalid -format option, see -h for supported values")
		return 1
	}

	if *outputPathOpt == "" {
		log.Println("Missing output-path argument")
		return 1
	}

	dataBucket, err := initializeBucket(*outputPathOpt, os.Getenv("B2_KEY_ID"), os.Getenv("B2_APP_KEY"))
	if err != nil {
		log.Println("cannot initilize buckets:", err)
		return 1
	}

	if *datastoreOpt == "" {
		log.Println("Missing datatore argument")
		return 1
	}

	if os.Getenv("B2_KEY_ID_DATASTORE") == "" {
		os.Setenv("B2_KEY_ID_DATASTORE", os.Getenv("B2_KEY_ID"))
	}
	if os.Getenv("B2_APP_KEY_DATASTORE") == "" {
		os.Setenv("B2_APP_KEY_DATASTORE", os.Getenv("B2_APP_KEY"))
	}

	datastoreBucket, err := initializeBucket(*datastoreOpt, os.Getenv("B2_KEY_ID_DATASTORE"), os.Getenv("B2_APP_KEY_DATASTORE"))
	if err != nil {
		log.Println(err)
		return 1
	}

	// Enable Scrapers or Sellers/Vendors
	scraps := strings.SplitSeq(*scrapersOpt, ",")
	for name := range scraps {
		if options[name] != nil {
			options[name].Enabled = true
		}
	}
	if *sellersOpt != "" {
		sells := strings.SplitSeq(*sellersOpt, ",")
		for name := range sells {
			if options[name] == nil {
				log.Println("Seller", name, "not found")
				return 1
			}
			options[name].Enabled = true
			options[name].OnlySeller = true
			options[name].OnlyVendor = false
		}
	}
	if *vendorsOpt != "" {
		vends := strings.SplitSeq(*vendorsOpt, ",")
		for name := range vends {
			if options[name] == nil {
				log.Println("Vendor", name, "not found")
				return 1
			}
			options[name].Enabled = true
			options[name].OnlySeller = false
			options[name].OnlyVendor = true
		}
	}

	datastoreReader, err := simplecloud.InitReader(context.Background(), datastoreBucket, *datastoreOpt)
	if err != nil {
		log.Println(err)
		return 1
	}
	defer datastoreReader.Close()

	now := time.Now()
	err = mtgmatcher.LoadDatastore(datastoreReader)
	if err != nil {
		log.Println(err)
		return 1
	}
	log.Println("loading datastore took:", time.Since(now))

	var scrapers []mtgban.Scraper

	// Initialize the enabled scrapers
	for _, opt := range options {
		if !opt.Enabled {
			continue
		}

		scraper, err := opt.Init()
		if err != nil {
			log.Println(err)
			return 1
		}

		// Check if any sub data source needs to be disabled
		config, ok := scraper.(mtgban.ScraperConfig)
		if ok {
			config.SetConfig(mtgban.ScraperOptions{
				DisableRetail:  opt.OnlyVendor,
				DisableBuylist: opt.OnlySeller,
			})
		}

		scrapers = append(scrapers, scraper)
	}

	if len(scrapers) == 0 {
		log.Println("No scraper configured, run with -h for a list of commands")
		return 1
	}
	countSellers, countVendors := mtgban.CountScrapers(scrapers)
	log.Println("Configured with", countSellers, "sellers and", countVendors, "vendors")

	now = time.Now()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var nonFatalErrors []error

	// Load the data
	for _, scraper := range scrapers {
		err := scraper.Load(ctx)
		if err != nil {
			log.Println(err)
			nonFatalErrors = append(nonFatalErrors, err)
		}
	}

	log.Println("loading scraper data took:", time.Since(now))

	sellers, vendors := mtgban.UnfoldScrapers(scrapers)
	retailResults, buylistResults := countResults(sellers, vendors)
	log.Println("Found", retailResults, "retail results and", buylistResults, "buylist results")

	reportSuspectPricings(sellers, vendors)
	if retailResults == 0 && buylistResults == 0 {
		log.Println("No retail or buylist data retrieved")
		return 1
	}

	now = time.Now()
	// Dump the results
	dumpErrors := dump(dataBucket, sellers, vendors, *outputPathOpt, *fileFormatOpt, *metaOpt)
	nonFatalErrors = append(nonFatalErrors, dumpErrors...)

	log.Println("uploading data took:", time.Since(now))

	log.Println("Completed in", time.Since(start))

	// Check for non-fatal errors and exit accordingly
	if nonFatalErrors != nil {
		log.Println("There were non-fatal errors:")
		for _, err := range nonFatalErrors {
			log.Println("-", err)
		}
		return 2
	}

	return 0
}

func main() {
	os.Exit(run())
}

func signAPI(link string) (string, error) {
	u, err := url.Parse(link)
	if err != nil {
		return "", err
	}

	v := url.Values{}
	v.Set("API", path.Base(u.Path))
	v.Set("APImode", "load")

	expires := time.Now().Add(1 * time.Minute)
	v.Set("Expires", fmt.Sprintf("%d", expires.Unix()))

	path := u.Scheme + "://" + u.Host
	if !strings.Contains(u.Host, "localhost") {
		path = "http://www.mtgban.com"
	}

	data := fmt.Sprintf("GET%d%s%s", expires.Unix(), path, v.Encode())
	key := os.Getenv("BAN_SECRET")
	if key == "" {
		return "", errors.New("missing BAN_SECRET")
	}

	// signHMACSHA1Base64
	h := hmac.New(sha1.New, []byte(key))
	h.Write([]byte(data))
	sig := base64.StdEncoding.EncodeToString(h.Sum(nil))

	v.Set("Signature", sig)
	str := base64.StdEncoding.EncodeToString([]byte(v.Encode()))

	return str, nil
}
