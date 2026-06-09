package tcgplayer

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/mtgban/go-mtgban/mtgban"
	"github.com/mtgban/go-mtgban/mtgmatcher"

	"github.com/mtgban/go-tcgplayer"
)

type TCGPlayerMarket struct {
	LogCallback    mtgban.LogCallbackFunc
	inventoryDate  time.Time
	buylistDate    time.Time
	Affiliate      string
	MaxConcurrency int
	SKUsData       SKUMap

	inventory mtgban.InventoryRecord
	buylist   mtgban.BuylistRecord

	client *tcgplayer.Client
}

type marketChan struct {
	UUID      string
	Condition string
	Printing  string
	Finish    string
	ProductId int
	SkuId     int
	Language  string
}

type responseChan struct {
	cardId string
	entry  mtgban.InventoryEntry
	bl     *mtgban.BuylistEntry
}

var availableMarketNames = []string{
	"TCG Player", "TCG Direct",
}

var name2shorthand = map[string]string{
	"TCG Low":          "TCGLow",
	"TCG Market":       "TCGMarket",
	"TCG Mid":          "TCGMid",
	"TCG Direct Low":   "TCGDirectLow",
	"TCG Player":       "TCGPlayer",
	"TCG Direct":       "TCGDirect",
	"TCG Direct (net)": "TCGDirectNet",
	"TCGplayer":        "TCGPlayer",
	"TCGplayer Direct": "TCGDirect",
}

var skuConditions = map[string]string{
	"NEAR MINT":         "NM",
	"LIGHTLY PLAYED":    "SP",
	"MODERATELY PLAYED": "MP",
	"HEAVILY PLAYED":    "HP",
	"DAMAGED":           "PO",
}

func (tcg *TCGPlayerMarket) printf(format string, a ...interface{}) {
	if tcg.LogCallback != nil {
		tcg.LogCallback("[TCGMkt] "+format, a...)
	}
}

func NewScraperMarket(publicId, privateId string) (*TCGPlayerMarket, error) {
	client, err := tcgplayer.NewClient(publicId, privateId)
	if err != nil {
		return nil, err
	}

	tcg := TCGPlayerMarket{}
	tcg.inventory = mtgban.InventoryRecord{}
	tcg.buylist = mtgban.BuylistRecord{}
	tcg.client = client
	tcg.MaxConcurrency = defaultConcurrency
	return &tcg, nil
}

func (tcg *TCGPlayerMarket) processEntry(ctx context.Context, channel chan<- responseChan, reqs []marketChan) error {
	ids := make([]int, len(reqs))
	for i := range reqs {
		ids[i] = reqs[i].SkuId
	}

	// Retrieve a list of skus with their prices
	results, err := tcg.client.GetMarketPricesBySKUs(ctx, ids)
	if err != nil {
		return err
	}

	for _, result := range results {
		var req marketChan
		for _, req = range reqs {
			if result.SkuId == req.SkuId {
				break
			}
		}

		isFoil := req.Printing == "FOIL"
		isEtched := req.Finish == "ETCHED"
		cardId, err := mtgmatcher.MatchId(req.UUID, isFoil, isEtched)
		if err != nil {
			tcg.printf("%s - (tcgId:%d / uuid:%s)", err.Error(), req.ProductId, req.UUID)
			continue
		}

		// Skip impossible entries, such as listing mistakes that list a foil
		// price for a foil-only card
		co, _ := mtgmatcher.GetUUID(cardId)
		if !co.Etched &&
			((co.Foil && req.Printing != "FOIL") ||
				(!co.Foil && req.Printing != "NON FOIL")) {
			continue
		}

		cond, found := skuConditions[req.Condition]
		if !found {
			tcg.printf("unknown condition %s for %d", req.Condition, req.SkuId)
			continue
		}

		// Sorted as in availableMarketNames.
		// TCGPlayer carries the bare item LowPrice — NOT LowestListingPrice,
		// which is the delivered price (item + shipping) and inflates every
		// downstream valuation (sealed EV, exports, "lowest price") by the
		// shipping component, badly so for sub-$5 cards. Shipping is preserved
		// separately in CustomFields below so a delivered price stays
		// reconstructable. TCGDirect uses the item-level DirectLowPrice.
		prices := []float64{
			result.LowPrice, getDirectPrice(result.DirectLowPrice),
		}
		printing := "Normal"
		if req.Printing == "FOIL" {
			printing = "Foil"
		}
		for i := range availableMarketNames {
			isDirect := i == 1
			link := GenerateProductURL(req.ProductId, printing, tcg.Affiliate, cond, req.Language, isDirect)

			out := responseChan{
				cardId: cardId,
				entry: mtgban.InventoryEntry{
					Conditions: cond,
					Price:      prices[i],
					Quantity:   1,
					URL:        link,
					SellerName: availableMarketNames[i],
					Bundle:     isDirect,
					OriginalId: fmt.Sprint(req.ProductId),
					InstanceId: fmt.Sprint(result.SkuId),
				},
			}

			// Preserve shipping for the TCGPlayer (marketplace) entry so the
			// delivered price (item + shipping) stays reconstructable downstream
			// even though Price is now the bare item LowPrice. Direct has its
			// own fulfillment model, so this only applies to the marketplace row.
			if !isDirect {
				out.entry.CustomFields = map[string]string{
					"lowestShipping":     fmt.Sprintf("%.2f", result.LowestShipping),
					"lowestListingPrice": fmt.Sprintf("%.2f", result.LowestListingPrice),
				}
			}

			if isDirect {
				price := DirectPriceAfterFees(prices[i])
				if price > 0 {
					out.bl = &mtgban.BuylistEntry{
						Conditions: cond,
						BuyPrice:   price,
						URL:        link,
						VendorName: "TCG Direct (net)",
						OriginalId: fmt.Sprint(req.ProductId),
						InstanceId: fmt.Sprint(result.SkuId),
					}
				}
			}

			channel <- out
		}
	}

	return nil
}

func (tcg *TCGPlayerMarket) Load(ctx context.Context) error {
	skusMap := tcg.SKUsData
	if skusMap == nil {
		return errors.New("sku map not loaded")
	}
	tcg.printf("Found skus for %d entries", len(skusMap))

	start := time.Now()

	pages := make(chan marketChan)
	channel := make(chan responseChan)
	var wg sync.WaitGroup

	for i := 0; i < tcg.MaxConcurrency; i++ {
		wg.Add(1)
		go func() {
			buffer := make([]marketChan, 0, tcgplayer.MaxIdsInRequest)

			for page := range pages {
				// Add our data to the buffer
				buffer = append(buffer, page)

				// When buffer is full, process its contents and empty it
				if len(buffer) == cap(buffer) {
					err := tcg.processEntry(ctx, channel, buffer)
					if err != nil {
						tcg.printf("%s", err.Error())
					}
					buffer = buffer[:0]
				}
			}
			// Process any spillover
			if len(buffer) != 0 {
				err := tcg.processEntry(ctx, channel, buffer)
				if err != nil {
					tcg.printf("%s", err.Error())
				}
			}
			wg.Done()
		}()
	}

	go func() {
		sets := mtgmatcher.GetAllSets()
		total := len(sets) - 1
		i := 1

		var idsFound []int
		for _, code := range sets {
			set, _ := mtgmatcher.GetSet(code)

			switch set.Code {
			case "4EDALT":
				continue
			}

			tcg.printf("Scraping %s (%d/%d)", set.Name, i, total)
			i++

			for _, card := range set.Cards {
				uuid := card.Identifiers["mtgjsonId"]
				skus, found := skusMap[uuid]
				if !found {
					continue
				}

				_, found = card.Identifiers["needsNewTCGSKUs"]
				if found {
					tcgId := card.Identifiers["tcgplayerProductId"]
					id, err := strconv.Atoi(tcgId)
					if err != nil {
						continue
					}

					altSkus, err := tcg.client.ListProductSKUs(ctx, id)
					if err != nil {
						tcg.printf("Error retrieving alternative SKUs: %s", err.Error())
						continue
					}

					skus = skus[:0]
					for _, sku := range altSkus {
						lang, found := map[int]string{
							1:  "ENGLISH",
							2:  "CHINESE SIMPLIFIED",
							3:  "CHINESE TRADITIONAL",
							4:  "FRENCH",
							5:  "GERMAN",
							6:  "ITALIAN",
							7:  "JAPANESE",
							8:  "KOREAN",
							9:  "PORTUGUESE BRAZIL",
							10: "RUSSIAN",
							11: "SPANISH",
						}[sku.LanguageId]
						if !found {
							continue
						}

						// Check for language early because we cannot have
						// duplicated sku ids, while the card may very well do
						if !mtgmatcher.Equals(lang, card.Language) {
							continue
						}

						printing := "NORMAL"
						if sku.PrintingId == 2 {
							printing = "FOIL"
						}

						cond, found := map[int]string{
							1: "NEAR MINT",
							2: "LIGHTLY PLAYED",
							3: "MODERATELY PLAYED",
							4: "HEAVILY PLAYED",
							5: "DAMAGED",
						}[sku.ConditionId]
						if !found {
							continue
						}

						skus = append(skus, TCGSku{
							Condition: cond,
							Language:  lang,
							Printing:  printing,
							ProductId: id,
							SkuId:     sku.SkuId,
						})
					}
				}

				hasNonfoil := card.HasFinish(mtgmatcher.FinishNonfoil)
				hasFoil := card.HasFinish(mtgmatcher.FinishFoil)
				hasEtched := card.HasFinish(mtgmatcher.FinishEtched)

				for _, sku := range skus {
					// Skip sealed products
					if sku.Condition == "UNOPENED" {
						continue
					}
					// Skip non-main languages
					if !mtgmatcher.Equals(sku.Language, card.Language) {
						// These two sets contain English sku, skip them
						switch set.Code {
						case "LEGITA", "DRKITA":
							continue
						}
						// Otherwise many Japanese and special cards are listed as English, skip anything else
						if sku.Language != "ENGLISH" {
							continue
						}
					}
					// Extra validation for incorrect data
					if !hasNonfoil && sku.Printing == "NON FOIL" {
						continue
					}
					if !hasFoil && !hasEtched && (sku.Printing == "FOIL" || sku.Finish == "ETCHED") {
						continue
					}
					if !hasEtched && sku.Finish == "ETCHED" {
						continue
					}
					// Make sure the right id is parsed
					// Check for tcgplayerProductId due to non-English cards from duplicated sets
					if sku.Finish != "ETCHED" && card.Identifiers["tcgplayerProductId"] != "" && fmt.Sprint(sku.ProductId) != card.Identifiers["tcgplayerProductId"] {
						continue
					}
					// Skip dupes
					if slices.Contains(idsFound, sku.SkuId) {
						continue
					}
					idsFound = append(idsFound, sku.SkuId)

					pages <- marketChan{
						UUID:      card.UUID,
						Condition: sku.Condition,
						Printing:  sku.Printing,
						Finish:    sku.Finish,
						ProductId: sku.ProductId,
						SkuId:     sku.SkuId,
						Language:  sku.Language,
					}
				}
			}
		}
		close(pages)

		wg.Wait()
		close(channel)
	}()

	for result := range channel {
		err := tcg.inventory.AddStrict(result.cardId, &result.entry)
		if err != nil {
			tcg.printf("%s", err.Error())
		}
		if result.bl != nil {
			err := tcg.buylist.Add(result.cardId, result.bl)
			if err != nil {
				tcg.printf("%s", err.Error())
			}
		}
	}
	tcg.inventoryDate = time.Now()
	tcg.buylistDate = time.Now()

	tcg.printf("Took %v", time.Since(start))

	return nil
}

func (tcg *TCGPlayerMarket) Inventory() mtgban.InventoryRecord {
	return tcg.inventory
}

func (tcg *TCGPlayerMarket) Buylist() mtgban.BuylistRecord {
	return tcg.buylist
}

func (tcg *TCGPlayerMarket) MarketNames() []string {
	return availableMarketNames
}

func (tcg *TCGPlayerMarket) TraderNames() []string {
	return []string{"TCG Direct (net)"}
}

func (tcg *TCGPlayerMarket) InfoForScraper(name string) mtgban.ScraperInfo {
	info := tcg.Info()
	info.Name = name
	info.Shorthand = name2shorthand[name]
	return info
}

func (tcg *TCGPlayerMarket) Info() (info mtgban.ScraperInfo) {
	info.Name = "TCG Player Market"
	info.Shorthand = "TCGMkt"
	info.InventoryTimestamp = &tcg.inventoryDate
	info.BuylistTimestamp = &tcg.buylistDate
	info.NoQuantityInventory = true
	return
}
