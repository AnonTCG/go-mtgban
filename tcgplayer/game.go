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

// TCGGame is the retail scraper for every game's TCGplayer category: it
// prices the skus the category's catalog dump lists, each resolved to a card
// by its product id, finish and language (see resolver). tcgGames below maps
// each game to the category it is served from.
type TCGGame struct {
	logCallback    mtgban.LogCallbackFunc
	inventoryDate  time.Time
	buylistDate    time.Time
	affiliate      string
	maxConcurrency int

	inventory mtgban.InventoryRecord
	buylist   mtgban.BuylistRecord

	// catalog lists the products priced, and names the category and its
	// printings.
	catalog *tcgplayer.CatalogDump

	printings     map[int]string
	languageNames map[int]string

	// resolve ties each single's sku to the printing it sells.
	resolve *resolver
	english int

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
// TCGplayer category carrying it. Supporting one more game is one entry
// here, provided the matcher has a datastore for it and the datastore
// publish writes its catalog dump.
var tcgGames = map[matcher.Game]int{
	matcher.GameMagic:         tcgplayer.CategoryMagic,
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
	tcg.buylist = mtgban.BuylistRecord{}
	tcg.client = client
	tcg.maxConcurrency = defaultConcurrency

	tcg.category = category
	tcg.productTypes = tcgplayer.SinglesProductTypes(category)

	tcg.printings = map[int]string{}
	tcg.languageNames = map[int]string{}

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

func (tcg *TCGGame) priceSKUs(ctx context.Context, channel chan<- genericChan, skus []resolvedSKU) error {
	ids := make([]int, len(skus))
	bySKU := map[int]resolvedSKU{}
	for i, sku := range skus {
		ids[i] = sku.SKUID
		bySKU[sku.SKUID] = sku
	}
	results, err := tcg.client.GetMarketPricesBySKUs(ctx, ids)
	if err != nil {
		return err
	}
	for _, result := range results {
		sku, found := bySKU[result.SKUID]
		if !found {
			continue
		}
		for _, out := range tcg.skuEntries(result, sku) {
			channel <- out
		}
	}
	return nil
}

// resolvedSKU is a sku to price and the card it resolved to.
type resolvedSKU struct {
	tcgplayer.SKU
	cardID string
}

// skusToPrice resolves the skus the products are priced by, unopened in
// the sealed mode and in a grade it reads otherwise, before any is asked
// for. Only the failures that say something about the datastore are
// reported one by one; the rest are counted.
func (tcg *TCGGame) skusToPrice(products []tcgplayer.Product) []resolvedSKU {
	var out []resolvedSKU
	skipped := map[string]int{}
	for i := range products {
		product := &products[i]
		if !tcg.sealed && isUnsupportedProduct(product) {
			continue
		}
		for _, sku := range product.Skus {
			if tcg.sealed {
				if sku.ConditionID != SKUConditionUnopened || sku.LanguageID != tcg.english {
					continue
				}
				// The product id is the sealed entry's whole identity
				uuids := tcg.sealedMap[sku.ProductID]
				if len(uuids) != 1 {
					skipped["product no sealed entry owns"]++
					continue
				}
				out = append(out, resolvedSKU{sku, uuids[0]})
				continue
			}
			_, found := SKUConditionMap[sku.ConditionID]
			if !found {
				continue
			}
			printing, found := tcg.printings[sku.PrintingID]
			if !found {
				skipped["printing the catalog does not name"]++
				continue
			}
			cardID, err := tcg.resolve.sku(sku, printing)
			switch {
			case err == nil:
				out = append(out, resolvedSKU{sku, cardID})
			case errors.Is(err, errUnownedProduct):
				skipped["product no card owns"]++
			case errors.Is(err, errRefusedProduct):
				skipped["product id claimed by more than one set"]++
			case errors.Is(err, matcher.ErrCardWrongFinish):
				skipped["finish the printing is not sold in"]++
			case errors.Is(err, matcher.ErrCardUnknownID):
				skipped["language the printing has no copy in"]++
			case errors.Is(err, matcher.ErrUnsupported):
				skipped["printing the matcher does not support"]++
			default:
				tcg.reportSKU(err, product, sku, printing)
			}
		}
	}
	for reason, n := range skipped {
		tcg.printf("Skipped %d skus of a %s", n, reason)
	}
	return out
}

// reportSKU says which product a sku failed to resolve for: a sku id alone
// says nothing about it.
func (tcg *TCGGame) reportSKU(err error, product *tcgplayer.Product, sku tcgplayer.SKU, printing string) {
	tcg.printf("%v for %q %s (product %d)", err, product.Name, printing, sku.ProductID)
	var alias *matcher.AliasingError
	if errors.As(err, &alias) {
		probes := alias.Probe()
		tcg.printf("%d %s got ids: %s", sku.ProductID, product.Name, probes)
		for _, probe := range probes {
			co, _ := tcg.backend.GetUUID(probe)
			tcg.printf("%s: %s", probe, co)
		}
	}
}

// skuEntries are the inventory entries a sku makes: its lowest listing, and
// for singles its Direct listing and market price, each where TCGplayer
// quotes one.
func (tcg *TCGGame) skuEntries(result tcgplayer.SKUPriceSet, sku resolvedSKU) []genericChan {
	if tcg.sealed {
		if result.LowestListingPrice == 0 {
			return nil
		}
		return []genericChan{{
			key: sku.cardID,
			entry: mtgban.InventoryEntry{
				Conditions: mtgban.NM,
				Price:      result.LowestListingPrice,
				Quantity:   1,
				URL:        GenerateProductURL(sku.ProductID, "", tcg.affiliate, "", "", false),
				SellerName: gameListingName,
				OriginalID: fmt.Sprint(sku.ProductID),
				InstanceID: fmt.Sprint(sku.SKUID),
			},
		}}
	}

	condition := SKUConditionMap[sku.ConditionID]
	printing := tcg.printings[sku.PrintingID]
	// The link filters the listings to the sku's language, as its price
	language := tcg.languageNames[sku.LanguageID]

	entry := func(name string, price float64, isDirect bool) genericChan {
		return genericChan{
			key: sku.cardID,
			entry: mtgban.InventoryEntry{
				Conditions: condition,
				Price:      price,
				Quantity:   1,
				URL:        GenerateProductURL(sku.ProductID, printing, tcg.affiliate, condition, language, isDirect),
				SellerName: name,
				Bundle:     isDirect,
				OriginalID: fmt.Sprint(sku.ProductID),
				InstanceID: fmt.Sprint(sku.SKUID),
			},
		}
	}

	// The lowest listing stays at $0, so the sku is still found through
	// it; Direct and the market price only where TCGplayer quotes one.
	out := []genericChan{entry(gameListingName, result.LowestListingPrice, false)}
	direct := getDirectPrice(result.DirectLowPrice)
	if tcg.direct() && direct > 0 {
		out = append(out, entry(gameDirectName, direct, true))
		net := DirectPriceAfterFees(direct)
		if net > 0 {
			last := &out[len(out)-1]
			last.bl = &mtgban.BuylistEntry{
				Conditions: condition,
				BuyPrice:   net,
				URL:        last.entry.URL,
				VendorName: gameDirectNetName,
				OriginalID: last.entry.OriginalID,
				InstanceID: last.entry.InstanceID,
			}
		}
	}
	if result.MarketPrice > 0 {
		out = append(out, entry(trendsName, result.MarketPrice, false))
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
	for _, language := range tcg.catalog.Languages {
		tcg.languageNames[language.LanguageID] = language.Name
	}

	products, err := catalogProducts(tcg.catalog, tcg.productTypes)
	if err != nil {
		return err
	}
	tcg.printf("Found %d products", len(products))

	tcg.english, err = catalogEnglish(tcg.catalog)
	if err != nil {
		return err
	}
	if tcg.sealed {
		tcg.sealedMap = tcg.backend.BuildSealedProductMap("tcgplayerProductId")
		tcg.printf("Loaded %d sealed products", len(tcg.sealedMap))
	} else {
		tcg.resolve, err = newResolver(tcg.backend, tcg.catalog)
		if err != nil {
			return err
		}
		for id, sets := range tcg.resolve.refused {
			tcg.printf("skipping id %s, claimed by more than one set: %v", id, sets)
		}
	}
	skus := tcg.skusToPrice(products)
	tcg.printf("Pricing %d skus", len(skus))

	err = loadPages(ctx, tcg.maxConcurrency, len(skus),
		func(ctx context.Context, offset int, channel chan<- genericChan) error {
			return tcg.priceSKUs(ctx, channel, skus[offset:min(offset+tcgplayer.MaxIDsInRequest, len(skus))])
		},
		func(result genericChan) {
			err := tcg.inventory.Add(result.key, &result.entry)
			if err != nil {
				tcg.printf("%s", err.Error())
			}
			if result.bl != nil {
				err := tcg.buylist.Add(result.key, result.bl)
				if err != nil {
					tcg.printf("%s", err.Error())
				}
			}
		},
		tcg.printf,
	)
	if err != nil {
		return err
	}

	tcg.inventoryDate = time.Now()
	tcg.buylistDate = time.Now()

	return nil
}

// Inventory returns what Load collected. See mtgban.Seller.
func (tcg *TCGGame) Inventory() mtgban.InventoryRecord {
	return tcg.inventory
}

// Buylist returns what Load collected: the Direct net of fees, for a
// category TCGplayer sells through Direct. See mtgban.Vendor.
func (tcg *TCGGame) Buylist() mtgban.BuylistRecord {
	return tcg.buylist
}

// direct reports whether the category sells through TCGplayer Direct, as
// its catalog dump says.
func (tcg *TCGGame) direct() bool {
	return tcg.catalog != nil && tcg.catalog.Category.IsDirect
}

// TCGGame's sub-scrapers: the lowest listing of a sku, its Direct listing
// and what that nets the seller after fees, and the market price TCGplayer
// quotes for it.
const (
	gameListingName   = "TCGplayer"
	gameDirectName    = "TCG Direct"
	gameDirectNetName = "TCG Direct (net)"
)

// MarketNames names the sub-sellers this market splits into, the lowest
// listing alone for sealed product. See mtgban.Market.
func (tcg *TCGGame) MarketNames() []string {
	if tcg.sealed {
		return []string{gameListingName}
	}
	if tcg.direct() {
		return []string{gameListingName, gameDirectName, trendsName}
	}
	return []string{gameListingName, trendsName}
}

// TraderNames names the sub-vendors this trader splits into: Direct's net,
// for singles of a category sold through Direct. See mtgban.Trader.
func (tcg *TCGGame) TraderNames() []string {
	if tcg.sealed || !tcg.direct() {
		return nil
	}
	return []string{gameDirectNetName}
}

// InfoForScraper describes one of the sub-scrapers named above. The lowest
// listing keeps the scraper's own name and shorthand.
func (tcg *TCGGame) InfoForScraper(name string) mtgban.ScraperInfo {
	info := tcg.Info()
	if name != gameListingName {
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
	info.BuylistTimestamp = &tcg.buylistDate
	info.NoQuantityInventory = true
	info.Game = tcg.backend.Game
	if tcg.sealed {
		info.Shorthand = "TCGSealed"
		info.SealedMode = true
	}
	return
}
