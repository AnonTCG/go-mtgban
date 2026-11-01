// Package abugames scrapes ABU Games, for both singles and sealed product.
package abugames

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
)

const (
	defaultConcurrency = 4
)

// ABUGames prices ABU Games' singles, both what they sell and what they buy.
type ABUGames struct {
	logCallback    mtgban.LogCallbackFunc
	inventoryDate  time.Time
	buylistDate    time.Time
	maxConcurrency int

	client  *ABUClient
	backend *matcher.Backend

	inventory mtgban.InventoryRecord
	buylist   mtgban.BuylistRecord
}

// NewScraper returns a singles scraper. ABU needs no credentials for prices.
func NewScraper(b *matcher.Backend) *ABUGames {
	abu := ABUGames{}
	abu.inventory = mtgban.InventoryRecord{}
	abu.buylist = mtgban.BuylistRecord{}
	abu.client = NewABUClient()
	abu.backend = b
	abu.maxConcurrency = defaultConcurrency
	return &abu
}

type resultChan struct {
	theCard    matcher.InputCard
	cardID     string
	invEntry   *mtgban.InventoryEntry
	buyEntry   *mtgban.BuylistEntry
	tradeEntry *mtgban.BuylistEntry
}

func (abu *ABUGames) printf(format string, a ...any) {
	if abu.logCallback != nil {
		abu.logCallback("[ABU] "+format, a...)
	}
}

func (abu *ABUGames) processEntry(ctx context.Context, filter string, channel chan<- resultChan, page int) error {
	product, err := abu.client.GetProduct(ctx, filter, page)
	if err != nil {
		return err
	}

	for _, group := range product.Grouped.ProductID.Groups {
		// hasMintGrade4Retail feeds abuRetailCondition; buylist ignores it.
		var hasMintGrade4Retail bool
		for _, doc := range group.Doclist.Docs {
			if doc.Condition == "MINT" && !isSlab(&doc) && (doc.SellQuantity > 0 || doc.SubSellQuantity > 0) && doc.SellPrice > 0 {
				hasMintGrade4Retail = true
			}
		}

		if len(group.Doclist.Docs) == 0 {
			continue
		}

		theCard, err := preprocess(abu.backend, &group.Doclist.Docs[0])
		if err != nil {
			continue
		}

		cardID, err := matchCard(abu.backend, &group.Doclist.Docs[0], theCard)
		if errors.Is(err, matcher.ErrUnsupported) {
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

			var alias *matcher.AliasingError
			if errors.As(err, &alias) {
				probes := alias.Probe()
				for _, probe := range probes {
					card, _ := abu.backend.GetUUID(probe)
					abu.printf("- %s", card)
				}
			}
			continue
		}

		co, err := abu.backend.GetUUID(cardID)
		if err != nil {
			continue
		}

		for _, doc := range group.Doclist.Docs {
			if doc.Condition == "SP" && !isSlab(&doc) {
				// There is nothing available on the website under this condition
				continue
			}

			// Sanity check, a bunch of cards are market as foil when they
			// actually don't have a foil printing, just skip them
			if foilOfUnfoiled(doc.DisplayTitle, co) {
				continue
			}

			// Older sets tend to be rougher, so grade stricter
			// 2003 is picked as the modern frame introduction
			var lowerGrade bool
			date, err := abu.backend.CardReleaseDate(cardID)
			if err == nil && date.Year() <= 2003 {
				lowerGrade = true
			}

			var invEntry *mtgban.InventoryEntry
			var buyEntry *mtgban.BuylistEntry
			var tradeEntry *mtgban.BuylistEntry

			// For URL generation searchQuery needs to be in plaintext, not URL-encoded
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

			if isSlab(&doc) {
				if doc.SellQuantity <= 0 || doc.SellPrice <= 0 {
					continue
				}
				cond, text := slabCondition(&doc, lowerGrade, theCard.Foil)
				if cond == "" {
					abu.printf("unsupported %q condition on %s", text, doc.ID)
					continue
				}

				u.Path = "/magic-the-gathering/singles"
				v.Set("magic_features", `[["Graded"]]`)
				u.RawQuery = v.Encode()

				channel <- resultChan{
					theCard: *theCard,
					cardID:  cardID,
					invEntry: &mtgban.InventoryEntry{
						Conditions: cond,
						Price:      doc.SellPrice,
						Quantity:   doc.SellQuantity,
						URL:        u.String() + searchQuery,
						OriginalID: group.GroupValue,
						InstanceID: doc.ID,
						SellerName: availableMarketNames[2],
					},
				}
				continue
			}

			if doc.SellQuantity > 0 && doc.SellPrice > 0 {
				cond, err := abuRetailCondition(doc.Condition, hasMintGrade4Retail, lowerGrade, theCard.Foil)
				if err != nil {
					abu.printf("unsupported %s condition", doc.Condition)
					continue
				}

				u.Path = "/magic-the-gathering/singles"

				invEntry = &mtgban.InventoryEntry{
					Conditions: cond,
					Price:      doc.SellPrice,
					Quantity:   doc.SellQuantity,
					URL:        u.String() + searchQuery,
					OriginalID: group.GroupValue,
					InstanceID: doc.ID,
					SellerName: availableMarketNames[0],
				}

				if strings.Contains(doc.CompleteDescription, "picture of the actual card") {
					invEntry.SellerName = availableMarketNames[1]
				}
			}

			if doc.BuyQuantity > 0 && doc.BuyPrice > 0 {
				// MINT is skipped: it cannot be mapped correctly.
				var cond mtgban.Condition
				if doc.Condition != "MINT" {
					grade, err := abuBuylistCondition(doc.Condition, theCard.Foil)
					if err != nil {
						abu.printf("unsupported %s condition", doc.Condition)
						continue
					}
					cond = grade
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
						OriginalID: group.GroupValue,
						InstanceID: doc.ID,
						VendorName: availableTraderNames[0],
					}

					tradeEntry = &mtgban.BuylistEntry{
						Conditions: cond,
						BuyPrice:   doc.TradePrice,
						Quantity:   doc.BuyQuantity,
						PriceRatio: priceRatioTrade,
						URL:        u.String() + searchQuery,
						OriginalID: group.GroupValue,
						InstanceID: doc.ID,
						VendorName: availableTraderNames[1],
					}

					// ABU has no SP grade for foils and no MP one for
					// non-foils, so pay the grade below there too.
					var gap mtgban.Condition
					if cond == mtgban.MP && theCard.Foil {
						gap = mtgban.SP
					} else if cond == mtgban.HP && !theCard.Foil {
						gap = mtgban.MP
					}
					if gap != "" {
						gapBuy, gapTrade := *buyEntry, *tradeEntry
						gapBuy.Conditions = gap
						gapTrade.Conditions = gap
						channel <- resultChan{
							theCard:    *theCard,
							cardID:     cardID,
							buyEntry:   &gapBuy,
							tradeEntry: &gapTrade,
						}
					}
				}
			}

			if invEntry != nil || buyEntry != nil {
				channel <- resultChan{
					theCard:    *theCard,
					cardID:     cardID,
					invEntry:   invEntry,
					buyEntry:   buyEntry,
					tradeEntry: tradeEntry,
				}
			}
		}
	}

	return nil
}

// abuRetailCondition maps ABU's condition to our grade for a sale listing.
// A group with a MINT copy on sale is a four-step scale, NM read as SP;
// otherwise PLD and HP read a grade up unless lowerGrade or foil is set.
func abuRetailCondition(condition string, hasMintGrade4Retail, lowerGrade, foil bool) (mtgban.Condition, error) {
	switch condition {
	case "MINT":
		return mtgban.NM, nil
	case "NM":
		if hasMintGrade4Retail {
			return mtgban.SP, nil
		}
		return mtgban.NM, nil
	case "PLD":
		if !lowerGrade && !hasMintGrade4Retail && !foil {
			return mtgban.SP, nil
		}
		return mtgban.MP, nil
	case "HP":
		if !lowerGrade && !hasMintGrade4Retail && !foil {
			return mtgban.MP, nil
		}
		return mtgban.HP, nil
	default:
		return "", fmt.Errorf("unsupported condition %q", condition)
	}
}

// abuBuylistCondition maps ABU's condition to our grade for a buy listing.
// MINT is not a case here: it is skipped before this is called.
func abuBuylistCondition(condition string, foil bool) (mtgban.Condition, error) {
	switch condition {
	case "NM":
		return mtgban.NM, nil
	case "PLD":
		// Stricter grading for foils
		if !foil {
			return mtgban.SP, nil
		}
		return mtgban.MP, nil
	case "HP":
		return mtgban.HP, nil
	default:
		return "", fmt.Errorf("unsupported condition %q", condition)
	}
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (abu *ABUGames) Load(ctx context.Context) error {
	filter := singlesFilter()
	count, err := abu.client.GetTotalItems(ctx, filter)
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
			abu.printf("Processing page %d/%d", page/maxEntryPerRequest, count/maxEntryPerRequest)
			err := abu.processEntry(ctx, filter, results, page)
			if err != nil {
				abu.printf("%v", err)
			}
			return nil
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
func (abu *ABUGames) Inventory() mtgban.InventoryRecord {
	return abu.inventory
}

// Buylist returns what Load collected. See mtgban.Vendor.
func (abu *ABUGames) Buylist() mtgban.BuylistRecord {
	return abu.buylist
}

var availableMarketNames = []string{
	"ABU Games",
	"ABU Games Scans",
	"ABU Games Graded",
}

var availableTraderNames = []string{
	"ABU Games",
	"ABU Games (credit)",
}

var name2shorthand = map[string]string{
	"ABU Games":          "ABUGames",
	"ABU Games Scans":    "ABUScans",
	"ABU Games Graded":   "ABUGraded",
	"ABU Games (credit)": "ABUCredit",
}

// MarketNames names the sub-sellers this market splits into. See
// mtgban.Market.
func (abu *ABUGames) MarketNames() []string {
	return availableMarketNames
}

// TraderNames names the sub-vendors this trader splits into. See
// mtgban.Trader.
func (abu *ABUGames) TraderNames() []string {
	return availableTraderNames
}

// InfoForScraper describes one of the sub-scrapers named above.
func (abu *ABUGames) InfoForScraper(name string) mtgban.ScraperInfo {
	info := abu.Info()
	info.Name = name
	info.Shorthand = name2shorthand[name]
	if info.Shorthand == "ABUCredit" {
		info.CreditMultiplier = 1
	}
	return info
}

// Info describes this scraper. See mtgban.Scraper.
func (abu *ABUGames) Info() (info mtgban.ScraperInfo) {
	info.Name = "ABU Games"
	info.Shorthand = "ABU"
	info.InventoryTimestamp = &abu.inventoryDate
	info.BuylistTimestamp = &abu.buylistDate
	info.Game = matcher.GameMagic
	return
}
