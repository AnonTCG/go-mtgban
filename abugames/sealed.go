package abugames

import (
	"context"
	"net/url"
	"time"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
)

// Sealed prices ABU Games' sealed product.
type Sealed struct {
	logCallback mtgban.LogCallbackFunc

	inventoryDate  time.Time
	buylistDate    time.Time
	maxConcurrency int

	productMap map[string]string
	client     *ABUClient
	backend    *matcher.Backend

	inventory mtgban.InventoryRecord
	buylist   mtgban.BuylistRecord
}

// NewScraperSealed returns a sealed scraper.
func NewScraperSealed(b *matcher.Backend) *Sealed {
	abu := Sealed{}
	abu.inventory = mtgban.InventoryRecord{}
	abu.buylist = mtgban.BuylistRecord{}
	abu.maxConcurrency = defaultConcurrency
	abu.client = NewABUClient()
	abu.backend = b

	abu.productMap = map[string]string{}
	for _, uuid := range b.GetSealedUUIDs() {
		co, err := b.GetUUID(uuid)
		if err != nil {
			continue
		}
		id, found := co.Identifiers["abuId"]
		if !found {
			continue
		}
		abu.productMap[id] = co.UUID
	}
	return &abu
}

func (abu *Sealed) printf(format string, a ...any) {
	if abu.logCallback != nil {
		abu.logCallback("[ABUSealed] "+format, a...)
	}
}

func (abu *Sealed) processEntry(ctx context.Context, channel chan<- resultChan, page int) error {
	response, err := abu.client.GetSealedProduct(ctx, page)
	if err != nil {
		return err
	}

	for _, doc := range response.Response.Docs {
		productID, found := abu.productMap[doc.ID]
		if !found {
			continue
		}

		var invEntry *mtgban.InventoryEntry
		var buyEntry *mtgban.BuylistEntry
		var tradeEntry *mtgban.BuylistEntry

		u, err := url.Parse("https://abugames.com")
		if err != nil {
			return err
		}

		// This works differently than the singles search
		v := url.Values{}
		v.Set("search", doc.DisplayTitle)
		u.RawQuery = v.Encode()

		if doc.SellQuantity > 0 && doc.SellPrice > 0 {
			u.Path = "/magic-the-gathering/packs"

			invEntry = &mtgban.InventoryEntry{
				Price:      doc.SellPrice,
				Quantity:   doc.SellQuantity,
				URL:        u.String(),
				OriginalID: doc.ID,
			}
		}

		if doc.BuyQuantity > 0 && doc.BuyPrice > 0 {
			var priceRatio float64
			if doc.SellPrice > 0 {
				priceRatio = doc.BuyPrice / doc.SellPrice * 100
			}

			u.Path = "/buylist/packs"

			buyEntry = &mtgban.BuylistEntry{
				BuyPrice:   doc.BuyPrice,
				Quantity:   doc.BuyQuantity,
				PriceRatio: priceRatio,
				URL:        u.String(),
				OriginalID: doc.ID,
				VendorName: availableTraderNames[0],
			}

			if doc.SellPrice > 0 {
				priceRatio = doc.TradePrice / doc.SellPrice * 100
			}
			tradeEntry = &mtgban.BuylistEntry{
				BuyPrice:   doc.TradePrice,
				Quantity:   doc.BuyQuantity,
				PriceRatio: priceRatio,
				URL:        u.String(),
				OriginalID: doc.ID,
				VendorName: availableTraderNames[1],
			}
		}

		if invEntry != nil || buyEntry != nil {
			channel <- resultChan{
				cardID:     productID,
				invEntry:   invEntry,
				buyEntry:   buyEntry,
				tradeEntry: tradeEntry,
			}
		}
	}

	return nil
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (abu *Sealed) Load(ctx context.Context) error {
	count, err := abu.client.GetTotalSealedItems(ctx)
	if err != nil {
		return err
	}
	abu.printf("Parsing %d entries", count)

	pageNums := make([]int, 0, count/maxEntryPerRequest+1)
	for i := 0; i < count; i += maxEntryPerRequest {
		pageNums = append(pageNums, i)
	}

	mtgban.WorkerPool(ctx, abu.maxConcurrency, pageNums,
		func(ctx context.Context, page int, results chan<- resultChan) error {
			return abu.processEntry(ctx, results, page)
		},
		func(result resultChan) {
			if result.invEntry != nil {
				err := abu.inventory.AddRelaxed(result.cardID, result.invEntry)
				if err != nil {
					abu.printf("%s", &result.theCard)
					abu.printf("%s", err.Error())
				}
			}
			if result.buyEntry != nil {
				err := abu.buylist.AddRelaxed(result.cardID, result.buyEntry)
				if err != nil {
					abu.printf("%s", &result.theCard)
					abu.printf("%s", err.Error())
				}
			}
			if result.tradeEntry != nil {
				err := abu.buylist.AddRelaxed(result.cardID, result.tradeEntry)
				if err != nil {
					abu.printf("%s", &result.theCard)
					abu.printf("%s", err.Error())
				}
			}
		},
		abu.printf,
	)

	abu.inventoryDate = time.Now()
	abu.buylistDate = time.Now()

	return nil
}

// Inventory returns what Load collected. See mtgban.Seller.
func (abu *Sealed) Inventory() mtgban.InventoryRecord {
	return abu.inventory
}

// Buylist returns what Load collected. See mtgban.Vendor.
func (abu *Sealed) Buylist() mtgban.BuylistRecord {
	return abu.buylist
}

// TraderNames names the sub-vendors this trader splits into. See
// mtgban.Trader.
func (abu *Sealed) TraderNames() []string {
	return availableTraderNames
}

var name2shorthandSealed = map[string]string{
	"ABU Games":          "ABUGamesSealed",
	"ABU Games (credit)": "ABUCreditSealed",
}

// InfoForScraper describes one of the sub-scrapers named above.
func (abu *Sealed) InfoForScraper(name string) mtgban.ScraperInfo {
	info := abu.Info()
	info.Name = name
	info.Shorthand = name2shorthandSealed[name]
	if info.Shorthand == "ABUCreditSealed" {
		info.CreditMultiplier = 1
	}
	return info
}

// Info describes this scraper. See mtgban.Scraper.
func (abu *Sealed) Info() (info mtgban.ScraperInfo) {
	info.Name = "ABU Games"
	info.Shorthand = "ABUSealed"
	info.InventoryTimestamp = &abu.inventoryDate
	info.BuylistTimestamp = &abu.buylistDate
	info.SealedMode = true
	info.Game = abu.backend.Game
	return
}
