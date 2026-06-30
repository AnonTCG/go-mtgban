package starcitygames

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/mtgban/go-mtgban/mtgban"
	"github.com/mtgban/go-mtgban/mtgmatcher"
)

type StarcitygamesSealed struct {
	LogCallback    mtgban.LogCallbackFunc
	inventoryDate  time.Time
	buylistDate    time.Time
	MaxConcurrency int

	Affiliate string

	DisableRetail  bool
	DisableBuylist bool

	inventory mtgban.InventoryRecord
	buylist   mtgban.BuylistRecord

	productMap map[string]string
	client     *SCGClient
	game       int
}

func NewScraperSealed(guid, bearer string) *StarcitygamesSealed {
	scg := StarcitygamesSealed{}
	scg.inventory = mtgban.InventoryRecord{}
	scg.buylist = mtgban.BuylistRecord{}
	scg.client = NewSCGClient(guid, bearer)
	scg.client.SealedMode = true
	scg.MaxConcurrency = defaultConcurrency
	scg.game = GameMagic
	return &scg
}

func (scg *StarcitygamesSealed) printf(format string, a ...interface{}) {
	if scg.LogCallback != nil {
		scg.LogCallback("[SCGSealed] "+format, a...)
	}
}

func buildProductMap() map[string]string {
	out := map[string]string{}
	for _, uuid := range mtgmatcher.GetSealedUUIDs() {
		co, err := mtgmatcher.GetUUID(uuid)
		if err != nil {
			continue
		}
		scgId := co.Identifiers["scgId"]
		out[scgId] = uuid
	}
	return out
}

func (scg *StarcitygamesSealed) processPage(ctx context.Context, channel chan<- responseChan, page int) error {
	results, err := scg.client.GetPage(ctx, scg.game, page)
	if err != nil {
		return err
	}

	for _, result := range results {
		if len(result.Document.ProductType) == 0 {
			return errors.New("malformed product_type")
		}
		if result.Document.ProductType[0] == "Singles" {
			scg.printf("Skipping product_type %s", result.Document.ProductType[0])
			continue
		}

		if len(result.Document.ItemDisplayName) == 0 {
			return errors.New("malformed item_display_name")
		}
		if len(result.Document.UniqueID) == 0 {
			return errors.New("malformed unique_id")
		}

		if len(result.Document.URLDetail) == 0 {
			return errors.New("malformed url_detail")
		}
		urlPath := result.Document.URLDetail[0]

		if !strings.Contains(urlPath, "-mtg-") {
			continue
		}

		for _, attribute := range result.Document.HawkChildAttributes {
			if len(attribute.VariantSKU) == 0 {
				return errors.New("malformed sku")
			}
			sku := attribute.VariantSKU[0]

			uuid, found := scg.productMap[sku]
			if !found {
				continue
			}

			if len(attribute.Price) == 0 {
				return errors.New("malformed price")
			}
			if len(attribute.Qty) == 0 {
				return errors.New("malformed qty")
			}
			priceStr := attribute.Price[0]
			qty := attribute.Qty[0]

			price, err := mtgmatcher.ParsePrice(priceStr)
			if err != nil {
				co, _ := mtgmatcher.GetUUID(uuid)
				scg.printf("invalid price for %s: %s", co, err.Error())
				continue
			}

			if qty == 0 || price == 0 {
				continue
			}

			link := SCGProductURL(result.Document.URLDetail, attribute.VariantSKU, scg.Affiliate)

			out := responseChan{
				cardId: uuid,
				invEntry: &mtgban.InventoryEntry{
					Price:      price,
					Quantity:   qty,
					OriginalId: sku,
					URL:        link,
				},
			}
			channel <- out
		}
	}

	return nil
}

func (scg *StarcitygamesSealed) scrape(ctx context.Context) error {
	scg.productMap = buildProductMap()

	totalPages, err := scg.client.NumberOfPages(ctx, scg.game)
	if err != nil {
		return err
	}
	scg.printf("Found %d pages", totalPages)

	pageNums := make([]int, totalPages)
	for i := range pageNums {
		pageNums[i] = i + 1
	}

	mtgban.WorkerPool(ctx, scg.MaxConcurrency, pageNums,
		func(ctx context.Context, page int, results chan<- responseChan) error {
			scg.printf("Processing page %d", page)
			return scg.processPage(ctx, results, page)
		},
		func(record responseChan) {
			err := scg.inventory.Add(record.cardId, record.invEntry)
			if err != nil && !record.ignoreErr {
				scg.printf("%s", err.Error())
			}
		},
		scg.printf,
	)

	scg.inventoryDate = time.Now()

	return nil
}

func (scg *StarcitygamesSealed) processBLPage(ctx context.Context, channel chan<- responseChan, page int) error {
	search, err := scg.client.SearchAll(ctx, scg.game, page, buylistRequestLimit, 0)
	if err != nil {
		return err
	}

	var gamePath string
	switch {
	case false:
		gamePath = "lorcana"
	case true:
		gamePath = "mtg"
	default:
		panic("unsupported game")
	}

	for _, hit := range search.Hits {
		link, _ := url.JoinPath(
			buylistBookmark,
			gamePath,
			"bookmark",
			url.QueryEscape(hit.Name),
			",/0/0/0", // various faucets (hot list, rarity, bulk etc)
			fmt.Sprint(hit.SetID),
			",",           // unclear
			hit.Language,  // language ofc<D-x>
			"0/999999.99", // min/max price range
			",",           // finish
			"default",
		)

		for _, result := range hit.Variants {
			uuid, found := scg.productMap[result.Sku]
			if !found {
				continue
			}

			var priceRatio, sellPrice float64
			price := result.BuyPrice

			invCards := scg.inventory[uuid]
			for _, invCard := range invCards {
				sellPrice = invCard.Price
				break
			}
			if sellPrice > 0 {
				priceRatio = price / sellPrice * 100
			}

			channel <- responseChan{
				cardId: uuid,
				buyEntry: &mtgban.BuylistEntry{
					BuyPrice:   price,
					PriceRatio: priceRatio,
					URL:        link,
				},
			}
		}
	}
	return nil
}

func (scg *StarcitygamesSealed) parseBL(ctx context.Context) error {
	scg.productMap = buildProductMap()

	search, err := scg.client.SearchAll(ctx, scg.game, 0, 1, 0)
	if err != nil {
		return err
	}
	scg.printf("Parsing %d products", search.EstimatedTotalHits)

	totalBLPages := search.EstimatedTotalHits/buylistRequestLimit + 1
	blPageNums := make([]int, totalBLPages)
	for i := range blPageNums {
		blPageNums[i] = i
	}

	mtgban.WorkerPool(ctx, scg.MaxConcurrency, blPageNums,
		func(ctx context.Context, page int, results chan<- responseChan) error {
			scg.printf("Processing page %d", page)
			return scg.processBLPage(ctx, results, page)
		},
		func(record responseChan) {
			err := scg.buylist.Add(record.cardId, record.buyEntry)
			if err != nil {
				scg.printf("%s", err.Error())
			}
		},
		scg.printf,
	)

	scg.buylistDate = time.Now()

	return nil
}

func (scg *StarcitygamesSealed) SetConfig(opt mtgban.ScraperOptions) {
	scg.DisableRetail = opt.DisableRetail
	scg.DisableBuylist = opt.DisableBuylist
}

func (scg *StarcitygamesSealed) Load(ctx context.Context) error {
	var errs []error

	if !scg.DisableRetail {
		err := scg.scrape(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("inventory load failed: %w", err))
		}
	}

	if !scg.DisableBuylist {
		err := scg.parseBL(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("buylist load failed: %w", err))
		}
	}

	return errors.Join(errs...)
}

func (scg *StarcitygamesSealed) Inventory() mtgban.InventoryRecord {
	return scg.inventory
}

func (scg *StarcitygamesSealed) Buylist() mtgban.BuylistRecord {
	return scg.buylist
}

func (scg *StarcitygamesSealed) Info() (info mtgban.ScraperInfo) {
	info.Name = "Star City Games"
	info.Shorthand = "SCGSealed"
	info.InventoryTimestamp = &scg.inventoryDate
	info.BuylistTimestamp = &scg.buylistDate
	info.SealedMode = true
	return
}
