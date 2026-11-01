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

// TCGGame is the retail scraper for any single-game TCGplayer category
// (Lorcana, Riftbound, ...): it prices the skus the category's catalog dump
// lists, each resolved to a card by its product id and finish; Magic has its
// own SKU-driven scrapers. tcgGames below maps each game to the category it
// is served from.
type TCGGame struct {
	logCallback    mtgban.LogCallbackFunc
	inventoryDate  time.Time
	affiliate      string
	maxConcurrency int

	inventory mtgban.InventoryRecord

	// catalog lists the products priced, and names the category and its
	// printings.
	catalog *tcgplayer.CatalogDump

	printings map[int]string

	category     int
	categoryName string

	productTypes []string

	// sealed selects the sealed mode: products are resolved through the
	// sealed product map by their product id instead of the matcher, and
	// only Unopened skus are priced.
	sealed    bool
	sealedMap map[int][]string

	backend *matcher.Backend
	client  *tcgplayer.Client
}

func (tcg *TCGGame) printf(format string, a ...any) {
	if tcg.logCallback != nil {
		tag := "[TCG](" + tcg.categoryName + ") "
		if !slices.Equal(tcg.productTypes, tcgplayer.SinglesProductTypes(tcg.category)) {
			tag += "{" + strings.Join(tcg.productTypes, ",") + "} "
		}
		tcg.logCallback(tag+format, a...)
	}
}

// tcgGames maps every game TCGGame and TCGGameIndex can be built for to the
// TCGplayer category carrying it. Magic is deliberately absent: it is
// identified by SKU and has its own scrapers. Supporting one more game is one
// entry here, provided the matcher has a datastore for it.
var tcgGames = map[matcher.Game]int{
	matcher.GameLorcana:       tcgplayer.CategoryLorcana,
	matcher.GameRiftbound:     tcgplayer.CategoryRiftbound,
	matcher.GameOnePiece:      tcgplayer.CategoryOnePiece,
	matcher.GameYuGiOh:        tcgplayer.CategoryYuGiOh,
	matcher.GameFleshAndBlood: tcgplayer.CategoryFleshAndBlood,
	matcher.GamePokemon:       tcgplayer.CategoryPokemon,
	matcher.GameGundam:        tcgplayer.CategoryGundam,
	matcher.GamePalworld:      tcgplayer.CategoryPalworld,
}

// NewScraperGame returns a singles scraper for one game, authenticated with a
// partner API key pair.
func NewScraperGame(b *matcher.Backend, publicID, privateID string) (*TCGGame, error) {
	game := b.Game
	category, found := tcgGames[game]
	if !found {
		return nil, fmt.Errorf("unsupported game %q", game)
	}

	client, err := tcgplayer.NewClient(publicID, privateID)
	if err != nil {
		return nil, err
	}

	tcg := TCGGame{}
	tcg.backend = b
	tcg.inventory = mtgban.InventoryRecord{}
	tcg.client = client
	tcg.maxConcurrency = defaultConcurrency

	tcg.category = category
	tcg.productTypes = tcgplayer.SinglesProductTypes(category)

	tcg.printings = map[int]string{}

	return &tcg, nil
}

// NewScraperGameSealed prices a game's sealed products: everything the
// category files outside the singles type, so a product type TCGplayer
// adds later is picked up rather than silently skipped. Products resolve
// through the sealed product map by their product id, the identity the
// datastore stamps on every sealed entry.
func NewScraperGameSealed(b *matcher.Backend, publicID, privateID string) (*TCGGame, error) {
	tcg, err := NewScraperGame(b, publicID, privateID)
	if err != nil {
		return nil, err
	}
	tcg.sealed = true
	tcg.productTypes = tcgplayer.SealedProductTypes(tcg.category)
	return tcg, nil
}

func (tcg *TCGGame) processPage(ctx context.Context, channel chan<- genericChan, products []tcgplayer.Product) error {
	productMap, skuMap, skuIDs := tcg.skusToPrice(products)

	for i := 0; i < len(skuIDs); i += tcgplayer.MaxIDsInRequest {
		start := i
		end := min(i+tcgplayer.MaxIDsInRequest, len(skuIDs))

		results, err := tcg.client.GetMarketPricesBySKUs(ctx, skuIDs[start:end])
		if err != nil {
			return err
		}

		for _, result := range results {
			sku := skuMap[result.SKUID]
			product, found := productMap[sku.ProductID]
			if !found {
				continue
			}
			for _, out := range tcg.skuEntries(result, sku, product) {
				channel <- out
			}
		}
	}

	return nil
}

// skusToPrice keeps the skus a page's products are priced by: English ones,
// unopened in the sealed mode and in a grade it reads otherwise.
func (tcg *TCGGame) skusToPrice(products []tcgplayer.Product) (map[int]tcgplayer.Product, map[int]tcgplayer.SKU, []int) {
	productMap := map[int]tcgplayer.Product{}
	skuMap := map[int]tcgplayer.SKU{}
	var skuIDs []int
	for _, product := range products {
		productMap[product.ProductID] = product

		for _, sku := range product.Skus {
			if tcg.sealed {
				if sku.ConditionID != SKUConditionUnopened {
					continue
				}
			} else {
				_, found := SKUConditionMap[sku.ConditionID]
				if !found {
					continue
				}
			}
			// Only English
			if sku.LanguageID != 1 {
				continue
			}

			skuIDs = append(skuIDs, sku.SKUID)
			skuMap[sku.SKUID] = sku
		}
	}

	return productMap, skuMap, skuIDs
}

// skuEntries are the inventory entries a sku makes: its lowest listing, and
// for singles its market price, each where TCGplayer quotes one. None where
// the sku names nothing the scraper prices.
func (tcg *TCGGame) skuEntries(result tcgplayer.SKUPriceSet, sku tcgplayer.SKU, product tcgplayer.Product) []genericChan {
	if tcg.sealed {
		if result.LowestListingPrice == 0 {
			return nil
		}
		// The product id is the sealed entry's whole identity;
		// anything the map does not name is a product the
		// datastore does not carry
		uuids := tcg.sealedMap[sku.ProductID]
		if len(uuids) != 1 {
			return nil
		}
		return []genericChan{{
			key: uuids[0],
			entry: mtgban.InventoryEntry{
				Conditions: mtgban.NM,
				Price:      result.LowestListingPrice,
				Quantity:   1,
				URL:        GenerateProductURL(sku.ProductID, "", tcg.affiliate, "", "", false),
				SellerName: gameMarketNames[0],
				OriginalID: fmt.Sprint(sku.ProductID),
				InstanceID: fmt.Sprint(sku.SKUID),
			},
		}}
	}
	if result.LowestListingPrice == 0 && result.MarketPrice == 0 {
		return nil
	}
	if isUnsupportedProduct(&product) {
		return nil
	}

	cardName := product.Name
	// A sku is a printing in one finish, and every game datastore
	// stamps the product id on the printing it names: the id and
	// the finish identify the sku, and a product the datastore does
	// not carry is reported rather than guessed from its wording.
	printing := tcg.printings[sku.PrintingID]
	theCard := &matcher.InputCard{
		ID:     fmt.Sprint(sku.ProductID),
		Finish: printing,
		Foil:   printing != "Normal",
	}
	cardID, err := tcg.backend.Match(theCard)
	if errors.Is(err, matcher.ErrUnsupported) {
		return nil
	} else if err != nil {
		// Name the card, not just the price row: a sku id alone
		// says nothing about which product failed to match.
		tcg.printf("%v for %q %s (product %d)", err, cardName, printing, sku.ProductID)
		tcg.printf("%+v", result)

		var alias *matcher.AliasingError
		if errors.As(err, &alias) {
			probes := alias.Probe()
			tcg.printf("%d %s got ids: %s", sku.ProductID, cardName, probes)
			for _, probe := range probes {
				co, _ := tcg.backend.GetUUID(probe)
				tcg.printf("%s: %s", probe, co)
			}
		}
		return nil
	}

	condition := SKUConditionMap[sku.ConditionID]

	link := GenerateProductURL(sku.ProductID, printing, tcg.affiliate, condition, "", false)

	// Sorted as in gameMarketNames
	prices := []float64{result.LowestListingPrice, result.MarketPrice}
	var out []genericChan
	for i, price := range prices {
		if price == 0 {
			continue
		}
		out = append(out, genericChan{
			key: cardID,
			entry: mtgban.InventoryEntry{
				Conditions: condition,
				Price:      price,
				Quantity:   1,
				URL:        link,
				SellerName: gameMarketNames[i],
				OriginalID: fmt.Sprint(sku.ProductID),
				InstanceID: fmt.Sprint(sku.SKUID),
			},
		})
	}
	return out
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (tcg *TCGGame) Load(ctx context.Context) error {
	err := checkCatalog(tcg.catalog, tcg.category)
	if err != nil {
		return err
	}
	tcg.categoryName = tcg.catalog.Category.Name

	tcg.printf("Found %d printings for category %d", len(tcg.catalog.Printings), tcg.category)
	for _, printing := range tcg.catalog.Printings {
		tcg.printf("%d - %s", printing.PrintingID, printing.Name)
		tcg.printings[printing.PrintingID] = printing.Name
	}

	products, err := catalogProducts(tcg.catalog, tcg.productTypes)
	if err != nil {
		return err
	}
	tcg.printf("Found %d products", len(products))

	if tcg.sealed {
		tcg.sealedMap = tcg.backend.BuildSealedProductMap("tcgplayerProductId")
		tcg.printf("Loaded %d sealed products", len(tcg.sealedMap))
	}

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
func (tcg *TCGGame) Inventory() mtgban.InventoryRecord {
	return tcg.inventory
}

// gameMarketNames are TCGGame's sub-sellers: the lowest listing of a sku,
// and the market price TCGplayer quotes for it.
var gameMarketNames = []string{
	"TCGplayer", trendsName,
}

// MarketNames names the sub-sellers this market splits into, the lowest
// listing alone for sealed product. See mtgban.Market.
func (tcg *TCGGame) MarketNames() []string {
	if tcg.sealed {
		return gameMarketNames[:1]
	}
	return gameMarketNames
}

// InfoForScraper describes one of the sub-scrapers named above. The lowest
// listing keeps the scraper's own name and shorthand.
func (tcg *TCGGame) InfoForScraper(name string) mtgban.ScraperInfo {
	info := tcg.Info()
	if name != gameMarketNames[0] {
		info.Name = name
		info.Shorthand = name2shorthand[name]
	}
	info.MetadataOnly = name == trendsName
	return info
}

// Info describes this scraper. See mtgban.Scraper.
func (tcg *TCGGame) Info() (info mtgban.ScraperInfo) {
	info.Name = "TCGplayer"
	info.Shorthand = "TCGPlayer"
	info.InventoryTimestamp = &tcg.inventoryDate
	info.NoQuantityInventory = true
	info.Game = tcg.backend.Game
	if tcg.sealed {
		info.Shorthand = "TCGSealed"
		info.SealedMode = true
	}
	return
}
