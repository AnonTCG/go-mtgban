package abugames

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/mtgban/go-mtgban/mtgban"
	"github.com/mtgban/go-mtgban/mtgmatcher"
)

const (
	defaultConcurrency = 4
)

type ABUGames struct {
	LogCallback    mtgban.LogCallbackFunc
	inventoryDate  time.Time
	buylistDate    time.Time
	MaxConcurrency int

	client *ABUClient

	inventory mtgban.InventoryRecord
	buylist   mtgban.BuylistRecord
}

func NewScraper() *ABUGames {
	abu := ABUGames{}
	abu.inventory = mtgban.InventoryRecord{}
	abu.buylist = mtgban.BuylistRecord{}
	abu.client = NewABUClient()
	abu.MaxConcurrency = defaultConcurrency
	return &abu
}

type resultChan struct {
	theCard    mtgmatcher.InputCard
	cardId     string
	invEntry   *mtgban.InventoryEntry
	buyEntry   *mtgban.BuylistEntry
	tradeEntry *mtgban.BuylistEntry
}

func (abu *ABUGames) printf(format string, a ...interface{}) {
	if abu.LogCallback != nil {
		abu.LogCallback("[ABU] "+format, a...)
	}
}

func (abu *ABUGames) processEntry(ctx context.Context, query string, channel chan<- resultChan, page int) error {
	product, err := abu.client.GetProduct(ctx, query, page)
	if err != nil {
		return err
	}

	for _, group := range product.Grouped.ProductId.Groups {
		// When MINT is found, handle conditions as a standard 4-grade
		// Otherwise use the same approach used for starcitygames (stricter grading for foils)
		// This only applies for Retail, the condition is skipped on buylists
		var hasMintGrade4Retail bool
		for _, doc := range group.Doclist.Docs {
			if doc.Condition == "MINT" && (doc.SellQuantity > 0 || doc.SubSellQuantity > 0) && doc.SellPrice > 0 {
				hasMintGrade4Retail = true
			}
		}

		if len(group.Doclist.Docs) == 0 {
			continue
		}

		theCard, err := preprocess(&group.Doclist.Docs[0])
		if err != nil {
			continue
		}

		cardId, err := mtgmatcher.Match(theCard)
		if errors.Is(err, mtgmatcher.ErrUnsupported) {
			continue
		} else if err != nil {
			// There are a bunch of non-existing prerelease cards from mh2
			// and promo pack DFC from lci (among others)
			if strings.Contains(theCard.Variation, "Prerelease") ||
				strings.Contains(theCard.Variation, "The List") ||
				strings.Contains(theCard.Variation, "Mystery Booster") ||
				strings.Contains(theCard.Variation, "Promo Pack") {
				continue
			}
			abu.printf("%v", theCard)
			abu.printf("%v", group.Doclist.Docs[0])
			abu.printf("%v", err)

			var alias *mtgmatcher.AliasingError
			if errors.As(err, &alias) {
				probes := alias.Probe()
				for _, probe := range probes {
					card, _ := mtgmatcher.GetUUID(probe)
					abu.printf("- %s", card)
				}
			}
			continue
		}

		for _, doc := range group.Doclist.Docs {
			if doc.Condition == "SP" {
				// There is nothing available on the website under this condition
				continue
			}

			// Sanity check, a bunch of cards are market as foil when they
			// actually don't have a foil printing, just skip them
			if strings.Contains(doc.DisplayTitle, "FOIL") {
				co, err := mtgmatcher.GetUUID(cardId)
				if err != nil {
					continue
				}
				if !co.Foil {
					continue
				}
			}

			// Older sets tend to be rougher, so grade stricter
			// 2003 is picked as the modern frame introduction
			var lowerGrade bool
			date, err := mtgmatcher.CardReleaseDate(cardId)
			if err == nil && date.Year() <= 2003 {
				lowerGrade = true
			}

			var invEntry *mtgban.InventoryEntry
			var buyEntry *mtgban.BuylistEntry
			var tradeEntry *mtgban.BuylistEntry

			// For URL genration searchQuery needs to be in plaintext, not URL-encoded
			searchQuery := "&search=" + doc.SimpleTitle

			u, err := url.Parse("https://abugames.com")
			if err != nil {
				return err
			}

			v := url.Values{}
			v.Set("magic_edition", "[\""+doc.Edition+"\"]")
			v.Set("card_style", "[\"Normal\"]")
			if theCard.Foil {
				v.Set("card_style", "[\"Foil\"]")
			}
			if len(doc.Language) > 0 {
				v.Set("language", "[\""+doc.Language[0]+"\"]")
			}
			u.RawQuery = v.Encode()

			if doc.SellQuantity > 0 && doc.SellPrice > 0 {
				var cond string
				switch doc.Condition {
				case "MINT":
					cond = "NM"
				case "NM":
					cond = "SP"
					if !hasMintGrade4Retail {
						cond = "NM"
					}
				case "PLD":
					cond = "MP"
					if !lowerGrade && !hasMintGrade4Retail && !theCard.Foil {
						cond = "SP"
					}
				case "HP":
					cond = "HP"
					if !lowerGrade && !hasMintGrade4Retail && !theCard.Foil {
						cond = "MP"
					}
				default:
					abu.printf("Unknown '%s' condition", doc.Condition)
					continue
				}

				u.Path = "/magic-the-gathering/singles"

				invEntry = &mtgban.InventoryEntry{
					Conditions: cond,
					Price:      doc.SellPrice,
					Quantity:   doc.SellQuantity,
					URL:        u.String() + searchQuery,
					OriginalId: group.GroupValue,
					InstanceId: doc.Id,
					SellerName: availableMarketNames[0],
				}

				if strings.Contains(doc.CompleteDescription, "picture of the actual card") {
					invEntry.SellerName = availableMarketNames[1]
				}
			}

			if doc.BuyQuantity > 0 && doc.BuyPrice > 0 {
				var cond string
				switch doc.Condition {
				case "MINT":
					// Skipped since it's impossible to map correctly
				case "NM":
					cond = "NM"
				case "PLD":
					// Stricter grading for foils
					cond = "MP"
					if !theCard.Foil {
						cond = "SP"
					}
				case "HP":
					cond = "HP"
				default:
					abu.printf("Unknown '%s' condition", doc.Condition)
					continue
				}

				if cond != "" {
					// priceRatio is between buy and sell
					var priceRatio float64
					if doc.SellPrice > 0 {
						priceRatio = doc.BuyPrice / doc.SellPrice * 100
					}
					// While priceRatioTrade is between trade and buy
					priceRatioTrade := doc.TradePrice / doc.BuyPrice * 100

					u.Path = "/buylist/magic-the-gathering/singles"

					buyEntry = &mtgban.BuylistEntry{
						Conditions: cond,
						BuyPrice:   doc.BuyPrice,
						Quantity:   doc.BuyQuantity,
						PriceRatio: priceRatio,
						URL:        u.String() + searchQuery,
						OriginalId: group.GroupValue,
						InstanceId: doc.Id,
						VendorName: availableTraderNames[0],
					}

					tradeEntry = &mtgban.BuylistEntry{
						Conditions: cond,
						BuyPrice:   doc.TradePrice,
						Quantity:   doc.BuyQuantity,
						PriceRatio: priceRatioTrade,
						URL:        u.String() + searchQuery,
						OriginalId: group.GroupValue,
						InstanceId: doc.Id,
						VendorName: availableTraderNames[1],
					}
				}
			}

			if invEntry != nil || buyEntry != nil {
				channel <- resultChan{
					theCard:    *theCard,
					cardId:     cardId,
					invEntry:   invEntry,
					buyEntry:   buyEntry,
					tradeEntry: tradeEntry,
				}
			}
		}
	}

	return nil
}

// Scrape returns an array of Entry, containing pricing and card information
func (abu *ABUGames) Load(ctx context.Context) error {
	extraSets := []string{
		`"Alpha"`, `"Beta"`, `"Unlimited"`, `"Arabian Nights"`, `"Antiquities"`, `"Legends"`, `"The Dark"`,
	}
	// Remove all cards with pictures and the editions above
	normalQuery := ` -magic_features:("Actual Picture Card") -magic_edition:("` + strings.Join(extraSets, " OR ") + `")`
	// Enable card with pictures for the editions aboves
	// (the +magic_features, means only report cards with pics, we need both)
	extraQuery := ` magic_features:("Actual Picture Card") +magic_edition:("` + strings.Join(extraSets, " OR ") + `")`

	count, err := abu.client.GetTotalItems(ctx, normalQuery)
	if err != nil {
		return err
	}
	abu.printf("Parsing %d entries", count)

	secondCount, err := abu.client.GetTotalItems(ctx, extraQuery)
	if err != nil {
		return err
	}
	abu.printf("Adding %d entries for pictures", secondCount)

	pageNums := make([]int, 0, count/maxEntryPerRequest+1)
	for i := 0; i < count; i += maxEntryPerRequest {
		pageNums = append(pageNums, i)
	}

	mtgban.WorkerPool(ctx, abu.MaxConcurrency, pageNums,
		func(ctx context.Context, page int, results chan<- resultChan) error {
			abu.printf("Processing page %d/%d", page/maxEntryPerRequest, count/maxEntryPerRequest)
			err := abu.processEntry(ctx, normalQuery, results, page)
			if err != nil {
				abu.printf("%v", err)
			}
			// secondCount will always be less than count so we can hijack
			// the loop and query more in detail when needed
			if page <= secondCount {
				abu.printf("Processing detailed page %d/%d", page/maxEntryPerRequest, secondCount/maxEntryPerRequest)
				err := abu.processEntry(ctx, extraQuery, results, page)
				if err != nil {
					abu.printf("%v", err)
				}
			}
			return nil
		},
		func(result resultChan) {
			if result.invEntry != nil {
				err := abu.inventory.AddRelaxed(result.cardId, result.invEntry)
				if err != nil {
					abu.printf("%s", &result.theCard)
					abu.printf("%s", err.Error())
				}
			}
			if result.buyEntry != nil {
				err := abu.buylist.AddRelaxed(result.cardId, result.buyEntry)
				if err != nil {
					abu.printf("%s", &result.theCard)
					abu.printf("%s", err.Error())
				}
			}
			if result.tradeEntry != nil {
				err := abu.buylist.AddRelaxed(result.cardId, result.tradeEntry)
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

func (abu *ABUGames) Inventory() mtgban.InventoryRecord {
	return abu.inventory
}

func (abu *ABUGames) Buylist() mtgban.BuylistRecord {
	return abu.buylist
}

var availableMarketNames = []string{
	"ABU Games",
	"ABU Games Scans",
}

var availableTraderNames = []string{
	"ABU Games",
	"ABU Games (credit)",
}

var name2shorthand = map[string]string{
	"ABU Games":          "ABUGames",
	"ABU Games Scans":    "ABUScans",
	"ABU Games (credit)": "ABUCredit",
}

func (abu *ABUGames) MarketNames() []string {
	return availableMarketNames
}

func (abu *ABUGames) TraderNames() []string {
	return availableTraderNames
}

func (abu *ABUGames) InfoForScraper(name string) mtgban.ScraperInfo {
	info := abu.Info()
	info.Name = name
	info.Shorthand = name2shorthand[name]
	if info.Shorthand == "ABUCredit" {
		info.CreditMultiplier = 1
	}
	return info
}

func (abu *ABUGames) Info() (info mtgban.ScraperInfo) {
	info.Name = "ABU Games"
	info.Shorthand = "ABU"
	info.InventoryTimestamp = &abu.inventoryDate
	info.BuylistTimestamp = &abu.buylistDate
	return
}
