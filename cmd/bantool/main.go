// Command bantool runs the scrapers and writes what they return, either to
// disk or to a cloud bucket, one file per scraper and price kind.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-cleanhttp"

	_ "github.com/joho/godotenv/autoload"

	cm "github.com/mtgban/go-cardmarket"
	tcgapi "github.com/mtgban/go-tcgplayer"

	"github.com/mtgban/go-mtgban/cardgamecorner"
	"github.com/mtgban/go-mtgban/cardmarket"
	"github.com/mtgban/go-mtgban/cardtrader"
	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
	"github.com/mtgban/go-mtgban/tcgplayer"
	"github.com/mtgban/simplecloud"

	_ "github.com/mtgban/go-mtgban/abugames"
	_ "github.com/mtgban/go-mtgban/arcanafrisia"
	_ "github.com/mtgban/go-mtgban/cardkingdom"
	_ "github.com/mtgban/go-mtgban/coolstuffinc"
	_ "github.com/mtgban/go-mtgban/gamenerdz"
	_ "github.com/mtgban/go-mtgban/hareruya"
	_ "github.com/mtgban/go-mtgban/manaleak"
	_ "github.com/mtgban/go-mtgban/manapool"
	_ "github.com/mtgban/go-mtgban/matcher/games"
	_ "github.com/mtgban/go-mtgban/merlion"
	_ "github.com/mtgban/go-mtgban/miniaturemarket"
	_ "github.com/mtgban/go-mtgban/mintcard"
	_ "github.com/mtgban/go-mtgban/mtgseattle"
	_ "github.com/mtgban/go-mtgban/sealedev"
	_ "github.com/mtgban/go-mtgban/starcitygames"
	_ "github.com/mtgban/go-mtgban/strikezone"
	_ "github.com/mtgban/go-mtgban/vegassingles"
)

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
}

// resolveGame validates -game against the option table, naming the games
// actually registered when it does not match. An empty name is the "no
// -game given" case, since flag.String defaults it to "".
func resolveGame(options map[matcher.Game]map[string]*scraperOption, name string) (matcher.Game, error) {
	game := matcher.Game(name)
	if _, ok := options[game]; ok {
		return game, nil
	}

	games := make([]string, 0, len(options))
	for g := range options {
		games = append(games, string(g))
	}
	slices.Sort(games)

	if name == "" {
		return "", fmt.Errorf("no -game given (registered: %s)", strings.Join(games, ", "))
	}
	return "", fmt.Errorf("unknown game %q (registered: %s)", name, strings.Join(games, ", "))
}

// enableStore turns on the named store within one game's scrapers, or
// reports why it could not. -store, -sellers and -vendors all take bare
// registry keys within the sub-map -game selects, so an
// unknown name is caught here rather than at mtgban.NewScraper.
func enableStore(scrapers map[string]*scraperOption, game matcher.Game, name string) (*scraperOption, error) {
	opt, ok := scrapers[name]
	if ok {
		opt.Enabled = true
		return opt, nil
	}

	names := make([]string, 0, len(scrapers))
	for n := range scrapers {
		names = append(names, n)
	}
	slices.Sort(names)
	return nil, fmt.Errorf("store %q is not registered for %s (registered: %s)",
		name, game, strings.Join(names, ", "))
}

// enableStores applies -store, -sellers and -vendors to one game's
// scrapers. At least one of the three must name something, or a run has
// nothing to do.
func enableStores(scrapers map[string]*scraperOption, game matcher.Game, store, sellers, vendors string) error {
	if store == "" && sellers == "" && vendors == "" {
		return errors.New("no store given, run with -h for a list of commands")
	}

	if store != "" {
		for name := range strings.SplitSeq(store, ",") {
			_, err := enableStore(scrapers, game, name)
			if err != nil {
				return err
			}
		}
	}
	// Clearing the other half here would overwrite what the entry itself
	// says about the store rather than meet it; both are left standing, and
	// mtgban.NewScraper refuses the pair that cannot hold.
	if sellers != "" {
		for name := range strings.SplitSeq(sellers, ",") {
			opt, err := enableStore(scrapers, game, name)
			if err != nil {
				return err
			}
			opt.OnlySeller = true
		}
	}
	if vendors != "" {
		for name := range strings.SplitSeq(vendors, ",") {
			opt, err := enableStore(scrapers, game, name)
			if err != nil {
				return err
			}
			opt.OnlyVendor = true
		}
	}
	return nil
}

// cardtraderBridge maps every Cardmarket product id to the TCGplayer id of
// the same product, read off cardtrader's blueprints - the one source
// linking the two marketplaces' ids - with the corrections that package
// keeps for the ids it sends wrong or not at all. The blueprints cover singles and
// sealed alike, so the same bridge serves both cardmarket scrapers; they
// receive it as plain data, and the composition of the two vendors happens
// here and nowhere else.
func cardtraderBridge(game matcher.Game) (map[int]int, error) {
	ctTokenBearer := os.Getenv("CARDTRADER_TOKEN_BEARER")
	if ctTokenBearer == "" {
		return nil, errors.New("missing CARDTRADER_TOKEN_BEARER env var")
	}
	client := cardtrader.NewCTAuthClient(ctTokenBearer)

	blueprints, _, err := cardtrader.BlueprintsForGame(context.Background(), client, game, "", log.Printf)
	if err != nil {
		return nil, err
	}

	bridge := map[int]int{}
	for _, bp := range blueprints {
		tcgID := bp.TCGplayerProductID()
		if tcgID == 0 {
			continue
		}
		for _, mkmID := range bp.CardMarketIDs {
			bridge[mkmID] = tcgID
		}
	}
	log.Printf("bridge: %d cardmarket ids linked to a tcgplayer id", len(bridge))
	return bridge, nil
}

// targets lists every registered scraper for every game, keyed by game then
// name. Everything mtgban.Registered(game) names starts out enabled for both
// halves; three targets across the whole registry do not answer for one of
// them, and are marked here rather than left for a run to find out the hard
// way.
func targets() map[matcher.Game]map[string]*scraperOption {
	all := make(map[matcher.Game]map[string]*scraperOption)
	for _, game := range matcher.AllGames {
		scrapers := make(map[string]*scraperOption)
		for _, name := range mtgban.Registered(game) {
			scrapers[name] = &scraperOption{}
		}
		all[game] = scrapers
	}

	// The store keeps no Magic singles shelf - not one variant in 960 sampled
	// had stock - so only the half it does answer for is asked here.
	all[matcher.GameMagic]["vegassingles"].OnlyVendor = true
	all[matcher.GameMagic]["mtgseattle"].OnlySeller = true
	all[matcher.GameYuGiOh]["coolstuffinc_sealed"].OnlySeller = true
	all[matcher.GameLorcana]["coolstuffinc_sealed"].OnlySeller = true

	return all
}

var options = targets()

// mkmCatalog reads Cardmarket's own published id-map catalog, whatever the
// game and whichever of cardmarket's scrapers asks for it, as go-cardmarket's
// mkmcatalog builds it.
func mkmCatalog() (*cm.Catalog, error) {
	path := os.Getenv("CARDMARKET_CATALOG_PATH")
	if path == "" {
		return nil, errors.New("missing CARDMARKET_CATALOG_PATH env var")
	}
	reader, err := openPath(path, os.Getenv("B2_KEY_ID_DATASTORE"), os.Getenv("B2_APP_KEY_DATASTORE"))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return cm.LoadCatalog(reader)
}

// tcgSYPCatalog reads the catalog tcg_syplist resolves its skus against: the
// list names a sku and the datastore names products, and the catalog is the
// step between the two - the same file the datastore generators build from.
func tcgSYPCatalog() (tcgplayer.SYPCatalog, error) {
	path := os.Getenv("TCGPLAYER_CATALOG_PATH")
	if path == "" {
		return nil, errors.New("missing TCGPLAYER_CATALOG_PATH env var")
	}
	reader, err := openPath(path, os.Getenv("B2_KEY_ID_DATASTORE"), os.Getenv("B2_APP_KEY_DATASTORE"))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return tcgplayer.LoadSYPCatalog(reader)
}

// tcgCatalog reads the catalog dump tcg_index, tcg_market and tcg_sealed
// price from, and mintcard names its listings' skus by: the products the
// category sells and their skus.
func tcgCatalog() (*tcgapi.CatalogDump, error) {
	path := os.Getenv("TCGPLAYER_CATALOG_PATH")
	if path == "" {
		return nil, errors.New("missing TCGPLAYER_CATALOG_PATH env var")
	}
	start := time.Now()
	reader, err := openPath(path, os.Getenv("B2_KEY_ID_DATASTORE"), os.Getenv("B2_APP_KEY_DATASTORE"))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	catalog, err := tcgplayer.LoadCatalog(reader)
	if err != nil {
		return nil, err
	}
	log.Println("loading catalog took:", time.Since(start))
	return catalog, nil
}

// scraperResources loads the catalogs, sku lists and bridge a key needs
// beyond its own secrets and flags, and wraps each as the typed
// mtgban.Option its package exports. Loading stays bantool's job; the
// constructor a key registered receives loaded data only.
func scraperResources(b *matcher.Backend, key string) ([]mtgban.Option, error) {
	game := b.Game
	var opts []mtgban.Option

	switch key {
	case "cardmarket", "cardmarket_market":
		catalog, err := mkmCatalog()
		if err != nil {
			return nil, err
		}
		opts = append(opts, cardmarket.WithCatalog(catalog))

		switch cardmarket.BridgeUseOf(game) {
		case cardmarket.BridgeRequired:
			bridge, err := cardtraderBridge(game)
			if err != nil {
				return nil, err
			}
			opts = append(opts, cardmarket.WithBridge(bridge))
		case cardmarket.BridgeHelps:
			// The catalog names most of the shelf by itself; the bridge only
			// settles what a collector number cannot, so a cardtrader that
			// will not answer costs those printings and nothing else, and
			// the run goes ahead saying so rather than failing and pricing
			// nothing.
			bridge, err := cardtraderBridge(game)
			if err != nil {
				log.Printf("bridge unavailable, naming what the catalog can on its own: %v", err)
			} else {
				opts = append(opts, cardmarket.WithBridge(bridge))
			}
		}

	case "cardmarket_sealed":
		if cardmarket.SealedBridgeUseOf(game) == cardmarket.BridgeUnused {
			break
		}
		bridge, err := cardtraderBridge(game)
		if err != nil {
			return nil, err
		}
		opts = append(opts, cardmarket.WithBridge(bridge))

	case "cardgamecorner":
		if game != matcher.GameOnePiece && game != matcher.GamePokemon {
			break
		}
		// The bridge only settles what the wording cannot, so a cardtrader
		// that will not answer costs those printings and nothing else.
		bridge, err := cardtraderBridge(game)
		if err != nil {
			log.Printf("bridge unavailable, naming what the wording can on its own: %v", err)
		} else {
			opts = append(opts, cardgamecorner.WithBridge(bridge))
		}

	case "tcg_index", "tcg_market", "tcg_sealed", "mintcard":
		catalog, err := tcgCatalog()
		if err != nil {
			return nil, err
		}
		opts = append(opts, tcgplayer.WithCatalog(catalog))

	case "tcg_syplist":
		catalog, err := tcgSYPCatalog()
		if err != nil {
			return nil, err
		}
		opts = append(opts, tcgplayer.WithSYPCatalog(catalog))
	}

	return opts, nil
}

// envAuthenticator answers a scraper's named secret from the environment
// variable spelled the same way. Reading the environment is bantool's job,
// not the scraper packages': they name the secrets they need and this is
// the one place that knows where the names are looked up. An unset or
// empty variable is a missing secret.
type envAuthenticator struct{}

// Secret implements mtgban.Authenticator.
func (envAuthenticator) Secret(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("%w: %s env var", mtgban.ErrMissingSecret, name)
	}
	return value, nil
}

// scraperOptions turns the flags and environment bantool reads into the
// options the key's own registered constructor understands: always the log
// callback and, where set, a concurrency cap; one half if the target asked
// for it; AFFILIATE when the caller set it (CI passes one partner per
// target); and any catalog, sku list or bridge scraperResources loads.
// Secrets are not read here either: envAuthenticator goes along as an
// option and answers each constructor's own Secret* names directly.
func scraperOptions(b *matcher.Backend, key string, opt *scraperOption, maxConcurrency int) ([]mtgban.Option, error) {
	opts := []mtgban.Option{
		mtgban.WithLogCallback(log.Printf),
		mtgban.WithAuthenticator(envAuthenticator{}),
	}
	if maxConcurrency != 0 {
		opts = append(opts, mtgban.WithMaxConcurrency(maxConcurrency))
	}
	if opt.OnlySeller {
		opts = append(opts, mtgban.WithRetailOnly())
	}
	if opt.OnlyVendor {
		opts = append(opts, mtgban.WithBuylistOnly())
	}

	if v := os.Getenv("AFFILIATE"); v != "" {
		opts = append(opts, mtgban.WithAffiliate(v))
	}

	resources, err := scraperResources(b, key)
	if err != nil {
		return nil, err
	}
	return append(opts, resources...), nil
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
	enc := json.NewEncoder(w)
	for uuid, entries := range inventory {
		for _, entry := range entries {
			if err := enc.Encode(inventoryElement{
				UUID:           uuid,
				InventoryEntry: entry,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeVendorToNDJSON(vendor mtgban.Vendor, w io.Writer) error {
	buylist := vendor.Buylist()
	enc := json.NewEncoder(w)
	for uuid, entries := range buylist {
		for _, entry := range entries {
			if err := enc.Encode(buylistElement{
				UUID:         uuid,
				BuylistEntry: entry,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// abortWrite discards an unfinished dump rather than publishing it, as
// simplecloud does internally: Abort where the writer can, and Close, which
// commits whatever was written, where it cannot.
func abortWrite(w io.WriteCloser) error {
	aborter, ok := w.(simplecloud.Aborter)
	if ok {
		return aborter.Abort()
	}
	return w.Close()
}

func dumpSeller(backend *matcher.Backend, dataBucket simplecloud.Writer, seller mtgban.Seller, outputPath, format string) (err error) {
	if len(seller.Inventory()) == 0 {
		return fmt.Errorf("seller %s has no data", seller.Info().Shorthand)
	}

	target := fmt.Sprintf("%s/retail/%s.%s", outputPath, seller.Info().Shorthand, format)
	log.Println("Writing", target)

	writer, err := simplecloud.InitWriter(context.Background(), dataBucket, target)
	if err != nil {
		return err
	}
	// On the cloud backends Close is what publishes, so only a clean encode
	// may reach it: an error or a panic aborts, keeping the last good dump
	// there; a local file is truncated on open and removed on abort
	var complete bool
	defer func() {
		if !complete {
			err = errors.Join(err, abortWrite(writer))
			return
		}
		err = writer.Close()
	}()

	switch strings.Split(format, ".")[0] {
	case "json":
		err = mtgban.WriteSellerToJSON(seller, writer)
	case "csv":
		err = mtgban.WriteInventoryToCSV(backend, seller.Inventory(), writer)
	case "ndjson":
		err = writeSellerToNDJSON(seller, writer)
	default:
		err = errors.New("invalid format")
	}

	complete = err == nil
	return err
}

func dumpVendor(backend *matcher.Backend, dataBucket simplecloud.Writer, vendor mtgban.Vendor, outputPath, format string) (err error) {
	if len(vendor.Buylist()) == 0 {
		return fmt.Errorf("vendor %s has no data", vendor.Info().Shorthand)
	}

	target := fmt.Sprintf("%s/buylist/%s.%s", outputPath, vendor.Info().Shorthand, format)
	log.Println("Writing", target)

	writer, err := simplecloud.InitWriter(context.Background(), dataBucket, target)
	if err != nil {
		return err
	}
	// On the cloud backends Close is what publishes, so only a clean encode
	// may reach it: an error or a panic aborts, keeping the last good dump
	// there; a local file is truncated on open and removed on abort
	var complete bool
	defer func() {
		if !complete {
			err = errors.Join(err, abortWrite(writer))
			return
		}
		err = writer.Close()
	}()

	switch strings.Split(format, ".")[0] {
	case "json":
		err = mtgban.WriteVendorToJSON(vendor, writer)
	case "csv":
		err = mtgban.WriteBuylistToCSV(backend, vendor.Buylist(), vendor.Info().CreditMultiplier, writer)
	case "ndjson":
		err = writeVendorToNDJSON(vendor, writer)
	default:
		err = errors.New("invalid format")
	}

	complete = err == nil
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

// reportResultCounts logs how many distinct cards each seller and vendor the
// run asked for holds, so a change in the run totals can be traced to the
// scraper behind it. A half that came back empty or failed to load was
// dropped when unfolding, and logs a zero; a half the run did not ask for,
// by its option, logs nothing.
func reportResultCounts(scrapers []mtgban.Scraper, opts []*scraperOption, sellers []mtgban.Seller, vendors []mtgban.Vendor) {
	retail := map[string]int{}
	for _, seller := range sellers {
		retail[seller.Info().Shorthand] = len(seller.Inventory())
	}
	buylist := map[string]int{}
	for _, vendor := range vendors {
		buylist[vendor.Info().Shorthand] = len(vendor.Buylist())
	}
	for i, scraper := range scrapers {
		if opts[i].OnlyVendor {
			continue
		}
		names, _ := halfShorthands(scraper)
		for _, name := range names {
			log.Printf("[%s] %d retail results", name, retail[name])
		}
	}
	for i, scraper := range scrapers {
		if opts[i].OnlySeller {
			continue
		}
		_, names := halfShorthands(scraper)
		for _, name := range names {
			log.Printf("[%s] %d buylist results", name, buylist[name])
		}
	}
}

// halfShorthands names the sellers and vendors a scraper unfolds into, the
// enabled sub-scrapers of a Market or Trader included, whatever they hold.
func halfShorthands(scraper mtgban.Scraper) ([]string, []string) {
	var sellers, vendors []string
	market, isMarket := scraper.(mtgban.Market)
	if isMarket {
		for _, name := range market.MarketNames() {
			sellers = append(sellers, market.InfoForScraper(name).Shorthand)
		}
	}
	_, isSeller := scraper.(mtgban.Seller)
	if isSeller && !isMarket {
		sellers = append(sellers, scraper.Info().Shorthand)
	}
	trader, isTrader := scraper.(mtgban.Trader)
	if isTrader {
		for _, name := range trader.TraderNames() {
			vendors = append(vendors, trader.InfoForScraper(name).Shorthand)
		}
	}
	_, isVendor := scraper.(mtgban.Vendor)
	if isVendor && !isTrader {
		vendors = append(vendors, scraper.Info().Shorthand)
	}
	return sellers, vendors
}

// reportSuspectPricings says which cards a scraper priced on both sides at
// prices too close to be the same printing. A shop's buy price sits well under
// what it asks, so a pairing that does not is usually two products meeting on
// one id - and nothing in the run log says so otherwise, because both halves
// resolved perfectly well on their own.
func reportSuspectPricings(backend *matcher.Backend, sellers []mtgban.Seller, vendors []mtgban.Vendor) {
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
				card, _ := backend.GetUUID(suspect.CardID)
				log.Printf("[%s] - %.0f%% buy $%.2f ask $%.2f %s %s",
					seller.Info().Shorthand, suspect.Ratio, suspect.BuyPrice, suspect.Price,
					suspect.Conditions, card)
			}
		}
	}
}

// reportCollapsedPricings says which cards a vendor buys at more than one
// price at one grade. A shop pays one price for one card, so a second is not
// a better offer but a second product: the feed named two printings and the
// match folded them onto one id. The run log says nothing about it either -
// both listings resolved, to the same card - and the pair of listing links
// is where the wording that tells them apart is published.
func reportCollapsedPricings(backend *matcher.Backend, vendors []mtgban.Vendor) {
	for _, vendor := range vendors {
		collapsed := mtgban.CollapsedPricings(vendor.Buylist(), mtgban.CollapsedRatioThreshold)
		if len(collapsed) == 0 {
			continue
		}

		log.Printf("[%s] %d cards are bought at several prices at one grade",
			vendor.Info().Shorthand, len(collapsed))
		for _, entry := range collapsed {
			card, _ := backend.GetUUID(entry.CardID)
			log.Printf("[%s] - %.1fx %d prices, $%.2f against $%.2f %s %s",
				vendor.Info().Shorthand, entry.Ratio, entry.Count,
				entry.High, entry.Low, entry.Conditions, card)
			log.Printf("[%s]   %s", vendor.Info().Shorthand, entry.HighURL)
			log.Printf("[%s]   %s", vendor.Info().Shorthand, entry.LowURL)
		}
	}
}

// load loads every scraper and answers the sellers and vendors that loaded.
// One whose Load failed may hold part of its data, which must not replace the
// last complete dump, so its error is reported and nothing of it is kept,
// except the half that loaded when the error names the other half alone.
func load(ctx context.Context, scrapers []mtgban.Scraper) ([]mtgban.Seller, []mtgban.Vendor, []error) {
	var sellers []mtgban.Seller
	var vendors []mtgban.Vendor
	var errs []error
	for _, scraper := range scrapers {
		err := scraper.Load(ctx)
		retailFailed := errors.Is(err, mtgban.ErrInventoryLoad)
		buylistFailed := errors.Is(err, mtgban.ErrBuylistLoad)
		if err != nil {
			log.Println(err)
			if retailFailed == buylistFailed {
				errs = append(errs, fmt.Errorf("%s not dumped: %w", scraper.Info().Shorthand, err))
				continue
			}
			errs = append(errs, fmt.Errorf("%s: %w", scraper.Info().Shorthand, err))
		}
		s, v := mtgban.UnfoldScrapers([]mtgban.Scraper{scraper})
		if !retailFailed {
			sellers = append(sellers, s...)
		}
		if !buylistFailed {
			vendors = append(vendors, v...)
		}
	}
	return sellers, vendors, errs
}

func dump(backend *matcher.Backend, dataBucket simplecloud.Writer, sellers []mtgban.Seller, vendors []mtgban.Vendor, outputPath, format string) []error {
	log.Println("Writing results to", outputPath)

	var sellerErrs []error
	for _, seller := range sellers {
		err := dumpSeller(backend, dataBucket, seller, outputPath, format)
		if err != nil {
			log.Println(err)
			sellerErrs = append(sellerErrs, err)
		}
	}

	var vendorErrs []error
	for _, vendor := range vendors {
		err := dumpVendor(backend, dataBucket, vendor, outputPath, format)
		if err != nil {
			log.Println(err)
			vendorErrs = append(vendorErrs, err)
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

// concurrentDownloads is how many ranged parts B2 is asked for at once.
const concurrentDownloads = 20

// openPath reads the object at path, resolving the scheme the same way
// initializeBucket does and taking the same trailing environment values:
// one names a GCS service account, two are a B2 key pair. Reading needs no
// bucket of its own, so a path that is only ever read goes through here
// rather than being built into one first.
func openPath(path string, env ...string) (io.ReadCloser, error) {
	opts := []simplecloud.OpenOption{
		simplecloud.WithHTTPClient(cleanhttp.DefaultClient()),
		simplecloud.WithConcurrentDownloads(concurrentDownloads),
		simplecloud.WithS3Credentials(os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY")),
		simplecloud.WithS3Endpoint(os.Getenv("AWS_ENDPOINT")),
	}
	if len(env) > 0 {
		opts = append(opts, simplecloud.WithGCSServiceAccount(env[0]))
	}
	if len(env) > 1 {
		opts = append(opts, simplecloud.WithB2Credentials(env[0], env[1]))
	}
	return simplecloud.Open(context.Background(), path, opts...)
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
		b2Bucket.ConcurrentDownloads = concurrentDownloads
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

	maxConcurrency, _ := strconv.Atoi(os.Getenv("MAX_CONCURRENCY"))
	log.Println("Workers running with", maxConcurrency, "parallel threads")

	gameOpt := flag.String("game", "", "Game to run (required unless -sign or -v)")
	storeOpt := flag.String("store", "", "Comma-separated list of stores to enable, within -game")

	datastoreOpt := flag.String("datastore", "", "Path to AllPrintings file")
	outputPathOpt := flag.String("output-path", "", "Path where to dump results")

	sellersOpt := flag.String("sellers", "", "Comma-separated list of sellers to enable, within -game")
	vendorsOpt := flag.String("vendors", "", "Comma-separated list of vendors to enable, within -game")

	fileFormatOpt := flag.String("format", "json", "File format of the output files (json/csv/ndjson)")

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

	game, err := resolveGame(options, *gameOpt)
	if err != nil {
		log.Println(err)
		return 1
	}
	err = enableStores(options[game], game, *storeOpt, *sellersOpt, *vendorsOpt)
	if err != nil {
		log.Println(err)
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

	datastoreReader, err := simplecloud.InitReader(context.Background(), datastoreBucket, *datastoreOpt)
	if err != nil {
		log.Println(err)
		return 1
	}
	defer datastoreReader.Close()

	now := time.Now()
	backend, err := matcher.Open(game, datastoreReader)
	if err != nil {
		log.Println(err)
		return 1
	}
	log.Printf("loading datastore took: %v (%s)", time.Since(now), game)

	var scrapers []mtgban.Scraper
	var scraperOpts []*scraperOption

	// Initialize the enabled scrapers
	for key, opt := range options[game] {
		if !opt.Enabled {
			continue
		}

		opts, err := scraperOptions(backend, key, opt, maxConcurrency)
		if err != nil {
			log.Println(err)
			return 1
		}

		scraper, err := mtgban.NewScraper(backend, key, opts...)
		if err != nil {
			log.Println(err)
			return 1
		}

		scrapers = append(scrapers, scraper)
		scraperOpts = append(scraperOpts, opt)
	}

	countSellers, countVendors := mtgban.CountScrapers(scrapers)
	log.Println("Configured with", countSellers, "sellers and", countVendors, "vendors")

	now = time.Now()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	sellers, vendors, nonFatalErrors := load(ctx, scrapers)

	log.Println("loading scraper data took:", time.Since(now))

	retailResults, buylistResults := countResults(sellers, vendors)
	log.Println("Found", retailResults, "retail results and", buylistResults, "buylist results")
	reportResultCounts(scrapers, scraperOpts, sellers, vendors)

	reportSuspectPricings(backend, sellers, vendors)
	reportCollapsedPricings(backend, vendors)
	if retailResults == 0 && buylistResults == 0 {
		log.Println("No retail or buylist data retrieved")
		return 1
	}

	now = time.Now()
	// Dump the results
	dumpErrors := dump(backend, dataBucket, sellers, vendors, *outputPathOpt, *fileFormatOpt)
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
