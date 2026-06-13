package cardtrader

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mtgban/go-mtgban/mtgban"
	"github.com/mtgban/go-mtgban/mtgmatcher"
)

type CardtraderSealed struct {
	LogCallback    mtgban.LogCallbackFunc
	MaxConcurrency int
	ShareCode      string

	// Only retrieve data from a single edition
	TargetEdition string

	exchangeRate float64
	client       *CTAuthClient

	inventoryDate time.Time
	inventory     mtgban.InventoryRecord
	gameId        int
}

func NewScraperSealed(token string) (*CardtraderSealed, error) {
	ct := CardtraderSealed{}
	ct.inventory = mtgban.InventoryRecord{}
	// API is strongly rated limited, hardcode a lower amount
	ct.MaxConcurrency = 2
	ct.client = NewCTAuthClient(token)
	ct.gameId = GameIdMagic
	return &ct, nil
}

func (ct *CardtraderSealed) printf(format string, a ...interface{}) {
	if ct.LogCallback != nil {
		ct.LogCallback("[CTSealed] "+format, a...)
	}
}

func (ct *CardtraderSealed) processEntry(ctx context.Context, channel chan<- resultChan, expansionId int, expansionName string, productMap map[int][]string) error {
	allProducts, err := ct.client.ProductsForExpansion(ctx, expansionId)
	if err != nil {
		return err
	}

	for _, products := range allProducts {
		for _, product := range products {
			uuids, found := productMap[product.BlueprintId]
			if !found {
				continue
			}

			if product.Properties.MTGLanguage != "en" {
				continue
			}

			uuid := uuids[0]
			if product.Properties.MTGFoil && len(uuids) > 1 {
				uuid = uuids[1]
			}

			switch {
			case product.Quantity < 1,
				product.OnVacation,
				product.Bundle,
				product.Properties.Altered:
				continue
			case mtgmatcher.Contains(product.Description, "ita"),
				mtgmatcher.Contains(product.Description, "empty booster"),
				mtgmatcher.Contains(product.Description, "empty box"),
				mtgmatcher.Contains(product.Description, "opened"),
				mtgmatcher.Contains(product.Description, "only cards"),
				mtgmatcher.Contains(product.Description, "missing"),
				mtgmatcher.Contains(product.Description, "deck box only"):
				continue
			}

			link := "https://www.cardtrader.com/cards/" + fmt.Sprint(product.BlueprintId)
			if ct.ShareCode != "" {
				link += "?share_code=" + ct.ShareCode
			}

			price := float64(product.Price.Cents) / 100
			if product.Price.Currency != "USD" {
				price *= ct.exchangeRate
			}

			// Assign a seller name as required by Market
			sellerName := availableMarketNames[0]
			if product.User.SealedZero {
				sellerName = availableMarketNames[1]
				if strings.Contains(strings.ToLower(product.User.Name), "day ready") {
					sellerName = availableMarketNames[2]
				}
			}

			channel <- resultChan{
				cardId: uuid,
				invEntry: &mtgban.InventoryEntry{
					Conditions: "NM",
					Price:      price,
					Quantity:   product.Quantity,
					URL:        link,
					SellerName: sellerName,
					Bundle:     product.User.SealedZero,
					OriginalId: fmt.Sprint(product.BlueprintId),
					InstanceId: fmt.Sprint(product.Id),
					CustomFields: map[string]string{
						"SubSellerName": product.User.Name,
						"SubSellerGeo":  product.User.CountryCode,
					},
				},
			}
		}
	}

	return nil
}

func (ct *CardtraderSealed) Load(ctx context.Context) error {
	rate, err := mtgban.GetExchangeRate(ctx, "EUR")
	if err != nil {
		return err
	}
	ct.exchangeRate = rate

	productMap := mtgmatcher.BuildSealedProductMap("cardtraderId")
	ct.printf("Loaded %d sealed products", len(productMap))

	expansionsRaw, err := ct.client.Expansions(ctx)
	if err != nil {
		return err
	}
	ct.printf("Retrieved %d global sets", len(expansionsRaw))

	if ct.TargetEdition != "" {
		ct.printf("-> only targeting edition %s", ct.TargetEdition)
	}

	var blueprintsRaw []Blueprint
	for _, exp := range expansionsRaw {
		if exp.GameId != ct.gameId {
			continue
		}
		if ct.TargetEdition != "" && exp.Name != ct.TargetEdition && exp.Code != strings.ToLower(ct.TargetEdition) {
			continue
		}

		bp, err := ct.client.Blueprints(ctx, exp.Id)
		if err != nil {
			ct.printf("skipping %d %s due to %s", exp.Id, exp.Name, err.Error())
			continue
		}
		blueprintsRaw = append(blueprintsRaw, bp...)
	}
	ct.printf("Found %d blueprints", len(blueprintsRaw))

	_, expansions := FormatBlueprints(blueprintsRaw, expansionsRaw, true)
	ct.printf("Parsing %d expansions", len(expansions))

	type expItem struct {
		id   int
		name string
	}
	expItems := make([]expItem, 0, len(expansions))
	for id, name := range expansions {
		expItems = append(expItems, expItem{id, name})
	}

	mtgban.WorkerPool(ctx, ct.MaxConcurrency, expItems,
		func(ctx context.Context, item expItem, results chan<- resultChan) error {
			ct.printf("Processing %s [%d]", item.name, item.id)
			return ct.processEntry(ctx, results, item.id, item.name, productMap)
		},
		func(result resultChan) {
			// Only keep one offer per condition
			skip := false
			entries := ct.inventory[result.cardId]
			for _, entry := range entries {
				if entry.Conditions == result.invEntry.Conditions && entry.Bundle == result.invEntry.Bundle {
					skip = true
					break
				}
			}
			if skip {
				return
			}

			err := ct.inventory.Add(result.cardId, result.invEntry)
			if err != nil {
				ct.printf("%s", err.Error())
			}
		},
		ct.printf,
	)

	ct.inventoryDate = time.Now()

	return nil
}

func (ct *CardtraderSealed) Inventory() mtgban.InventoryRecord {
	return ct.inventory
}

func (tcg *CardtraderSealed) MarketNames() []string {
	return availableMarketNames
}

var name2shorthandSealed = map[string]string{
	"Card Trader":      "CTSealed",
	"Card Trader Zero": "CT0Sealed",
	"Card Trader 1DR":  "CT1DRSealed",
}

func (ct *CardtraderSealed) InfoForScraper(name string) mtgban.ScraperInfo {
	info := ct.Info()
	info.Name = name
	info.Shorthand = name2shorthandSealed[name]
	return info
}

func (ct *CardtraderSealed) Info() (info mtgban.ScraperInfo) {
	info.Name = "Card Trader Sealed"
	info.Shorthand = "CTSealedWrapper"
	info.InventoryTimestamp = &ct.inventoryDate
	info.CountryFlag = "EU"
	info.SealedMode = true
	return
}
