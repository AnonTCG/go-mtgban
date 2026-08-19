// Package tcgplayer scrapes TCGplayer through both their partner API and
// their storefront: market and index pricing, sealed product, per-seller
// inventory, and the SKU catalog the other scrapers resolve against.
package tcgplayer

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/mtgban/go-mtgban/mtgban"
	"github.com/mtgban/go-mtgban/mtgmatcher"

	"github.com/mtgban/go-tcgplayer"
)

// Market prices singles from TCGplayer's partner API, splitting the
// result into the sub-sellers their pricing endpoint reports and the buylist
// they publish alongside it.
type Market struct {
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
	ProductID int
	SkuID     int
	Language  string
}

type responseChan struct {
	cardID string
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

func (tcg *Market) printf(format string, a ...any) {
	if tcg.LogCallback != nil {
		tcg.LogCallback("[TCGMkt] "+format, a...)
	}
}

// NewScraperMarket returns a market scraper authenticated with a partner API
// key pair.
func NewScraperMarket(publicID, privateID string) (*Market, error) {
	client, err := tcgplayer.NewClient(publicID, privateID)
	if err != nil {
		return nil, err
	}

	tcg := Market{}
	tcg.inventory = mtgban.InventoryRecord{}
	tcg.buylist = mtgban.BuylistRecord{}
	tcg.client = client
	tcg.MaxConcurrency = defaultConcurrency
	return &tcg, nil
}

func (tcg *Market) processEntry(ctx context.Context, channel chan<- responseChan, reqs []marketChan) error {
	ids := make([]int, len(reqs))
	for i := range reqs {
		ids[i] = reqs[i].SkuID
	}

	// Retrieve a list of skus with their prices
	results, err := tcg.client.GetMarketPricesBySKUs(ctx, ids)
	if err != nil {
		return err
	}

	for _, result := range results {
		var req marketChan
		for _, req = range reqs {
			if result.SKUID == req.SkuID {
				break
			}
		}

		isFoil := req.Printing == "FOIL"
		isEtched := req.Finish == "ETCHED"
		cardID, err := mtgmatcher.MatchID(req.UUID, isFoil, isEtched)
		if err != nil {
			tcg.printf("%s - (tcgId:%d / uuid:%s)", err.Error(), req.ProductID, req.UUID)
			continue
		}

		// Skip impossible entries, such as listing mistakes that list a foil
		// price for a foil-only card
		co, _ := mtgmatcher.GetUUID(cardID)
		if !co.Etched &&
			((co.Foil && req.Printing != "FOIL") ||
				(!co.Foil && req.Printing != "NON FOIL")) {
			continue
		}

		cond, found := skuConditions[req.Condition]
		if !found {
			tcg.printf("unknown condition %s for %d", req.Condition, req.SkuID)
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
			link := GenerateProductURL(req.ProductID, printing, tcg.Affiliate, cond, req.Language, isDirect)

			out := responseChan{
				cardID: cardID,
				entry: mtgban.InventoryEntry{
					Conditions: cond,
					Price:      prices[i],
					Quantity:   1,
					URL:        link,
					SellerName: availableMarketNames[i],
					Bundle:     isDirect,
					OriginalID: fmt.Sprint(req.ProductID),
					InstanceID: fmt.Sprint(result.SKUID),
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
						OriginalID: fmt.Sprint(req.ProductID),
						InstanceID: fmt.Sprint(result.SKUID),
					}
				}
			}

			channel <- out
		}
	}

	return nil
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (tcg *Market) Load(ctx context.Context) error {
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
		wg.Go(func() {
			buffer := make([]marketChan, 0, tcgplayer.MaxIDsInRequest)

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
		})
	}

	go func() {
		sets := mtgmatcher.GetAllSets()
		total := len(sets) - 1
		i := 1

		idsFound := map[int]struct{}{}
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
					tcgID := card.Identifiers["tcgplayerProductId"]
					id, err := strconv.Atoi(tcgID)
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
						}[sku.LanguageID]
						if !found {
							continue
						}

						// Check for language early because we cannot have
						// duplicated sku ids, while the card may very well do
						if !mtgmatcher.Equals(lang, card.Language) {
							continue
						}

						// Must be "NON FOIL" (not "NORMAL") to match LoadTCGSKUs and the
						// downstream printing checks (the processEntry "impossible entry"
						// guard rejects any nonfoil row whose Printing != "NON FOIL").
						// A refetched nonfoil SKU tagged "NORMAL" was silently dropped —
						// latent bug, first exercised by the MSC nonfoil override.
						printing := "NON FOIL"
						if sku.PrintingID == 2 {
							printing = "FOIL"
						}

						cond, found := map[int]string{
							1: "NEAR MINT",
							2: "LIGHTLY PLAYED",
							3: "MODERATELY PLAYED",
							4: "HEAVILY PLAYED",
							5: "DAMAGED",
						}[sku.ConditionID]
						if !found {
							continue
						}

						skus = append(skus, TCGSku{
							Condition: cond,
							Language:  lang,
							Printing:  printing,
							ProductID: id,
							SkuID:     sku.SKUID,
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
					if sku.Finish != "ETCHED" && card.Identifiers["tcgplayerProductId"] != "" && fmt.Sprint(sku.ProductID) != card.Identifiers["tcgplayerProductId"] {
						continue
					}
					// Skip dupes
					_, found := idsFound[sku.SkuID]
					if found {
						continue
					}
					idsFound[sku.SkuID] = struct{}{}

					pages <- marketChan{
						UUID:      card.UUID,
						Condition: sku.Condition,
						Printing:  sku.Printing,
						Finish:    sku.Finish,
						ProductID: sku.ProductID,
						SkuID:     sku.SkuID,
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
		err := tcg.inventory.AddStrict(result.cardID, &result.entry)
		if err != nil {
			tcg.printf("%s", err.Error())
		}
		if result.bl != nil {
			err := tcg.buylist.Add(result.cardID, result.bl)
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

// Inventory returns what Load collected. See mtgban.Seller.
func (tcg *Market) Inventory() mtgban.InventoryRecord {
	return tcg.inventory
}

// Buylist returns what Load collected. See mtgban.Vendor.
func (tcg *Market) Buylist() mtgban.BuylistRecord {
	return tcg.buylist
}

// MarketNames names the sub-sellers this market splits into. See
// mtgban.Market.
func (tcg *Market) MarketNames() []string {
	return availableMarketNames
}

// TraderNames names the sub-vendors this trader splits into. See
// mtgban.Trader.
func (tcg *Market) TraderNames() []string {
	return []string{"TCG Direct (net)"}
}

// InfoForScraper describes one of the sub-scrapers named above.
func (tcg *Market) InfoForScraper(name string) mtgban.ScraperInfo {
	info := tcg.Info()
	info.Name = name
	info.Shorthand = name2shorthand[name]
	return info
}

// Info describes this scraper. See mtgban.Scraper.
func (tcg *Market) Info() (info mtgban.ScraperInfo) {
	info.Name = "TCG Player Market"
	info.Shorthand = "TCGMkt"
	info.InventoryTimestamp = &tcg.inventoryDate
	info.BuylistTimestamp = &tcg.buylistDate
	info.NoQuantityInventory = true
	return
}
