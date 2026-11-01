package tcgplayer

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"

	"github.com/mtgban/go-tcgplayer"
)

// TCGGameIndex is the market-price index counterpart of TCGGame, pricing the
// same catalog dump's products by product id and resolving them the same
// way.
type TCGGameIndex struct {
	logCallback    mtgban.LogCallbackFunc
	inventoryDate  time.Time
	affiliate      string
	maxConcurrency int

	inventory mtgban.InventoryRecord

	// catalog lists the products priced, and names the category and its
	// printings.
	catalog *tcgplayer.CatalogDump

	// printings are the names of the finishes the category sells.
	printings []string

	category     int
	categoryName string

	productTypes []string

	backend *matcher.Backend
	client  *tcgplayer.Client
}

func (tcg *TCGGameIndex) printf(format string, a ...any) {
	if tcg.logCallback != nil {
		tag := "[TCG](" + tcg.categoryName + ") "
		if !slices.Equal(tcg.productTypes, tcgplayer.SinglesProductTypes(tcg.category)) {
			tag += "{" + strings.Join(tcg.productTypes, ",") + "} "
		}
		tcg.logCallback(tag+format, a...)
	}
}

// NewScraperGameIndex returns an index scraper for one game, authenticated
// with a partner API key pair.
func NewScraperGameIndex(b *matcher.Backend, publicID, privateID string) (*TCGGameIndex, error) {
	game := b.Game
	category, found := tcgGames[game]
	if !found {
		return nil, fmt.Errorf("unsupported game %q", game)
	}

	client, err := tcgplayer.NewClient(publicID, privateID)
	if err != nil {
		return nil, err
	}

	tcg := TCGGameIndex{}
	tcg.backend = b
	tcg.inventory = mtgban.InventoryRecord{}
	tcg.client = client
	tcg.maxConcurrency = defaultConcurrency

	tcg.category = category
	tcg.productTypes = tcgplayer.SinglesProductTypes(category)

	return &tcg, nil
}

func (tcg *TCGGameIndex) processPage(ctx context.Context, channel chan<- genericChan, products []tcgplayer.Product) error {
	productMap := map[int]tcgplayer.Product{}
	ids := make([]int, len(products))
	for i, product := range products {
		ids[i] = product.ProductID
		productMap[product.ProductID] = product
	}

	results, err := tcg.client.GetMarketPricesByProducts(ctx, ids)
	if err != nil {
		return err
	}

	for _, result := range results {
		product, found := productMap[result.ProductID]
		if !found {
			continue
		}
		for _, out := range tcg.productEntries(result, product) {
			channel <- out
		}
	}

	return nil
}

// productEntries are the inventory entries a product's price row makes, one
// per price it quotes, or none where the row names nothing the scraper
// prices.
func (tcg *TCGGameIndex) productEntries(result tcgplayer.ProductPriceSet, product tcgplayer.Product) []genericChan {
	if result.LowPrice == 0 && result.MarketPrice == 0 && result.MidPrice == 0 && result.DirectLowPrice == 0 {
		return nil
	}
	if isUnsupportedProduct(&product) {
		return nil
	}

	cardName := product.Name
	// See TCGGame.processPage: the product id and the finish identify
	// the price row.
	theCard := &matcher.InputCard{
		ID:     fmt.Sprint(result.ProductID),
		Finish: result.SubTypeName,
		Foil:   result.SubTypeName != "Normal",
	}
	cardID, err := tcg.backend.Match(theCard)
	if errors.Is(err, matcher.ErrUnsupported) {
		return nil
	} else if err != nil {
		// A row quoting a market price and nothing else is a pricing-side
		// relic: the catalog has split or retired the printing its subtype
		// names, and the surviving printing is priced by its own row
		// alongside this one. Refusing it is right - the stale number can
		// be many times the real price - but complaining every run is
		// noise, so only a finish the printing lacks, or one the category
		// no longer sells, is let through quietly. Any other failure on
		// such a row still speaks up.
		marketOnly := result.LowPrice == 0 && result.MidPrice == 0 && result.DirectLowPrice == 0
		retired := !slices.Contains(tcg.printings, result.SubTypeName)
		if marketOnly && (retired || errors.Is(err, matcher.ErrCardWrongFinish)) {
			return nil
		}

		// Name the card, not just the price row: a product id alone
		// says nothing about which product failed to match.
		tcg.printf("%v for %q %s (product %d)", err, cardName, result.SubTypeName, result.ProductID)
		tcg.printf("%+v", result)

		var alias *matcher.AliasingError
		if errors.As(err, &alias) {
			probes := alias.Probe()
			tcg.printf("%d %s got ids: %s", product.ProductID, cardName, probes)
			for _, probe := range probes {
				co, _ := tcg.backend.GetUUID(probe)
				tcg.printf("%s: %s", probe, co)
			}
		}
		return nil
	}

	var entries []genericChan
	var directLow float64
	if tcg.direct() {
		directLow = getDirectPrice(result.DirectLowPrice)
	}
	prices := []float64{
		result.LowPrice, result.MarketPrice, result.MidPrice, directLow,
	}

	for i := range prices {
		if prices[i] == 0 {
			continue
		}

		isDirect := availableIndexNames[i] == "TCG Direct Low"
		link := GenerateProductURL(result.ProductID, result.SubTypeName, tcg.affiliate, "", "", isDirect)

		entries = append(entries, genericChan{
			key: cardID,
			entry: mtgban.InventoryEntry{
				Price:      prices[i],
				Quantity:   1,
				URL:        link,
				SellerName: availableIndexNames[i],
				Bundle:     isDirect,
				OriginalID: fmt.Sprint(result.ProductID),
			},
		})
	}

	return entries
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (tcg *TCGGameIndex) Load(ctx context.Context) error {
	err := checkCatalog(tcg.catalog, tcg.category)
	if err != nil {
		return err
	}
	tcg.categoryName = tcg.catalog.Category.Name

	for _, printing := range tcg.catalog.Printings {
		tcg.printings = append(tcg.printings, printing.Name)
	}

	products, err := catalogProducts(tcg.catalog, tcg.productTypes)
	if err != nil {
		return err
	}
	tcg.printf("Found %d products", len(products))

	err = loadPages(ctx, tcg.maxConcurrency, len(products),
		func(ctx context.Context, page int, channel chan<- genericChan) error {
			return tcg.processPage(ctx, channel, catalogPage(products, page))
		},
		func(result genericChan) {
			err := tcg.inventory.Add(result.key, &result.entry)
			if err != nil {
				tcg.printf("%s", err.Error())
			}
		},
		tcg.printf,
	)
	if err != nil {
		return err
	}

	tcg.inventoryDate = time.Now()

	return nil
}

// Inventory returns what Load collected. See mtgban.Seller.
func (tcg *TCGGameIndex) Inventory() mtgban.InventoryRecord {
	return tcg.inventory
}

// MarketNames names the sub-sellers this market splits into, Direct's low
// for a category TCGplayer sells through Direct. See mtgban.Market.
func (tcg *TCGGameIndex) MarketNames() []string {
	if tcg.direct() {
		return availableIndexNames
	}
	return availableIndexNames[:len(availableIndexNames)-1]
}

// direct reports whether the category sells through TCGplayer Direct, as
// its catalog dump says.
func (tcg *TCGGameIndex) direct() bool {
	return tcg.catalog != nil && tcg.catalog.Category.IsDirect
}

// InfoForScraper describes one of the sub-scrapers named above.
func (tcg *TCGGameIndex) InfoForScraper(name string) mtgban.ScraperInfo {
	info := tcg.Info()
	info.Name = name
	info.Shorthand = name2shorthand[name]
	return info
}

// Info describes this scraper. See mtgban.Scraper.
func (tcg *TCGGameIndex) Info() (info mtgban.ScraperInfo) {
	info.Name = "TCG Player Index"
	info.Shorthand = "TCGIndex"
	info.InventoryTimestamp = &tcg.inventoryDate
	info.MetadataOnly = true
	info.NoQuantityInventory = true
	info.Game = tcg.backend.Game
	info.Family = "TCG"
	return
}
