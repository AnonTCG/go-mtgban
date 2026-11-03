// Package cardkingdom scrapes Card Kingdom, covering singles, sealed
// product and their separately graded listings.
package cardkingdom

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mtgban/go-cardkingdom"
	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
)

// Cardkingdom prices Card Kingdom's singles, both what they sell and what they
// buy.
type Cardkingdom struct {
	logCallback mtgban.LogCallbackFunc
	partner     string
	preserveOOS bool

	localPath     string
	inventoryDate time.Time
	buylistDate   time.Time

	backend *matcher.Backend

	inventory mtgban.InventoryRecord
	buylist   mtgban.BuylistRecord
}

// NewScraperLocal returns a singles scraper reading the feed from a file
// instead of the network.
func NewScraperLocal(b *matcher.Backend, localPath string) *Cardkingdom {
	ck := Cardkingdom{}
	ck.inventory = mtgban.InventoryRecord{}
	ck.buylist = mtgban.BuylistRecord{}
	ck.localPath = localPath
	ck.backend = b
	return &ck
}

// NewScraper returns a singles scraper reading Card Kingdom's published feed.
func NewScraper(b *matcher.Backend) *Cardkingdom {
	ck := Cardkingdom{}
	ck.inventory = mtgban.InventoryRecord{}
	ck.buylist = mtgban.BuylistRecord{}
	ck.backend = b
	return &ck
}

func (ck *Cardkingdom) printf(format string, a ...any) {
	if ck.logCallback != nil {
		ck.logCallback("[CK] "+format, a...)
	}
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (ck *Cardkingdom) Load(ctx context.Context) error {
	link := ck.localPath
	if link == "" {
		link = cardkingdom.PricelistURL
	}
	pricelist, err := cardkingdom.Pricelist(ctx, mtgban.NewHTTPClient(), link)
	if err != nil {
		return err
	}

	ck.printf("Found %d prices", len(pricelist))

	start := time.Now()

	for _, card := range pricelist {
		skipErrors := card.Edition == "Mystery Booster/The List"

		theCard, err := Preprocess(ck.backend, card)
		if err != nil {
			if !errors.Is(err, matcher.ErrUnsupported) {
				ck.printf("%v", err)
			}
			continue
		}

		cardID, err := ck.backend.Match(theCard)
		if errors.Is(err, matcher.ErrUnsupported) {
			id, rerr := matchPrereleaseSKU(ck.backend, card.SKU, card.ScryfallID, theCard.Foil)
			if rerr == nil {
				cardID, err = id, nil
			}
			if err != nil {
				continue
			}
		} else if err != nil {
			ogErr := err
			cardID, err = ck.backend.MatchID(card.ScryfallID, theCard.Foil, strings.Contains(card.Variation, "Etched"))
			if err != nil {
				if skipErrors || unindexedTokenSheet(ck.backend, card.SKU) {
					continue
				}
				ck.printf("%v", ogErr)
				ck.printf("%q", theCard)
				ck.printf("%q", card)

				var alias *matcher.AliasingError
				if errors.As(err, &alias) {
					probes := alias.Probe()
					for _, probe := range probes {
						card, _ := ck.backend.GetUUID(probe)
						ck.printf("- %s", card)
					}
				}
				continue
			}
		}

		u, _ := url.Parse("https://www.cardkingdom.com/")

		u.Path = card.URL
		if ck.partner != "" {
			q := u.Query()
			q.Set("partner", ck.partner)
			q.Set("utm_source", ck.partner)
			q.Set("utm_medium", "affiliate")
			q.Set("utm_campaign", ck.partner)
			u.RawQuery = q.Encode()
		}
		link := u.String()

		retailPrices := []float64{
			card.ConditionValues.NMPrice, card.ConditionValues.EXPrice, card.ConditionValues.VGPrice, card.ConditionValues.GPrice,
		}
		// For newly added cards the nmPrice may not be initialized, so we the
		// root price value that is known to be valid (and skip card if data is
		// missing, since it gets used in the buylist step).
		if retailPrices[0] == 0 {
			retailPrices[0] = card.PriceRetail
		}
		if retailPrices[0] == 0 {
			continue
		}

		if card.QtyRetail > 0 && card.PriceRetail > 0 {
			qtys := []int{
				card.ConditionValues.NMQty, card.ConditionValues.EXQty, card.ConditionValues.VGQty, card.ConditionValues.GQty,
			}

			for i, cond := range mtgban.DefaultGradeTags {
				if qtys[i] == 0 {
					continue
				}

				out := &mtgban.InventoryEntry{
					Conditions: cond,
					Price:      retailPrices[i],
					Quantity:   qtys[i],
					URL:        link,
					OriginalID: strconv.Itoa(card.ID),
					InstanceID: card.SKU,
				}
				if cond != mtgban.NM {
					out.CustomFields = map[string]string{
						"RetailPrice": fmt.Sprint(card.PriceRetail),
					}
				}
				err = ck.inventory.AddUnique(cardID, out)
			}
		} else if ck.preserveOOS {
			// Only save URL information
			out := &mtgban.InventoryEntry{
				URL: link,
			}
			err = ck.inventory.AddUnique(cardID, out)
		}
		if err != nil && !skipErrors {
			ck.printf("%v", err)
		}

		u, _ = url.Parse("https://www.cardkingdom.com/purchasing/mtg_singles")
		if card.QtyBuying > 0 {
			q := u.Query()
			q.Set("filter[search]", "mtg_advanced")

			// Include as much detail as possible in the name
			cardName := card.Name
			if card.Variation != "" {
				cardName = fmt.Sprintf("%s (%s)", card.Name, card.Variation)
			}
			q.Set("filter[name]", strings.ReplaceAll(cardName, "\"", ""))

			// Always show both non-foil and foil cards, the filtering
			// on the website accurate enough (ie for Prerelease)
			q.Set("filter[singles]", "1")
			q.Set("filter[foils]", "1")

			// Edition is derived from the url itself, not the edition field
			urlPaths := strings.Split(card.URL, "/")
			if len(urlPaths) > 2 {
				q.Set("filter[edition]", urlPaths[1])
			}
			if ck.partner != "" {
				q.Set("partner", ck.partner)
				q.Set("utm_source", ck.partner)
				q.Set("utm_medium", "affiliate")
				q.Set("utm_campaign", ck.partner)
			}
			u.RawQuery = q.Encode()

			for i, grade := range mtgban.DefaultGradeTags {
				// Apply the same downgrade present in the retail prices since
				// there are a class of cards where CK downgrades price differently,
				// as if they were nonfoil
				buyPrice := card.PriceBuy * retailPrices[i] / retailPrices[0]

				var priceRatio float64

				if card.PriceRetail > 0 {
					priceRatio = card.PriceBuy / card.PriceRetail * 100
				}

				out := &mtgban.BuylistEntry{
					Conditions: grade,
					BuyPrice:   buyPrice,
					Quantity:   card.QtyBuying,
					PriceRatio: priceRatio,
					URL:        u.String(),
					OriginalID: strconv.Itoa(card.ID),
					InstanceID: card.SKU,
					VendorName: availableTraderNames[0],
				}
				// Add the line entry as needed by the csv import
				if grade == mtgban.NM {
					out.CustomFields = map[string]string{
						"CKTitle":   cardName,
						"CKEdition": card.Edition,
						"CKFoil":    strconv.FormatBool(card.IsFoil),
						"CKSKU":     card.SKU,
						"CKID":      strconv.Itoa(card.ID),
					}
				}
				err = ck.buylist.Add(cardID, out)
				if err != nil && !skipErrors {
					ck.printf("%v", err)
				}
			}
		} else if ck.preserveOOS {
			for i, grade := range mtgban.DefaultGradeTags {
				buyPrice := card.PriceBuy * retailPrices[i] / retailPrices[0]

				// The price CK lists while not buying, and the product it
				// lists it for
				out := &mtgban.BuylistEntry{
					BuyPrice:   buyPrice,
					Conditions: grade,
					VendorName: availableTraderNames[1],
					OriginalID: strconv.Itoa(card.ID),
					InstanceID: card.SKU,
				}
				err = ck.buylist.Add(cardID, out)
				if err != nil && !skipErrors {
					ck.printf("%v", err)
				}
			}
		}
	}

	ck.printf("Took %v", time.Since(start))

	ck.inventoryDate = time.Now()
	ck.buylistDate = time.Now()

	return nil
}

// Inventory returns what Load collected. See mtgban.Seller.
func (ck *Cardkingdom) Inventory() mtgban.InventoryRecord {
	return ck.inventory
}

// Buylist returns what Load collected. See mtgban.Vendor.
func (ck *Cardkingdom) Buylist() mtgban.BuylistRecord {
	return ck.buylist
}

var availableTraderNames = []string{
	"Card Kingdom",
	"Card Kingdom (last known)",
}

var name2shorthand = map[string]string{
	"Card Kingdom":              "CK",
	"Card Kingdom (last known)": "CKBLLast",
}

// TraderNames names the sub-vendors this trader splits into. See
// mtgban.Trader.
func (ck *Cardkingdom) TraderNames() []string {
	if !ck.preserveOOS {
		return availableTraderNames[:1]
	}
	return availableTraderNames
}

// InfoForScraper describes one of the sub-scrapers named above.
func (ck *Cardkingdom) InfoForScraper(name string) mtgban.ScraperInfo {
	info := ck.Info()
	info.Name = name
	info.Shorthand = name2shorthand[name]
	return info
}

// Info describes this scraper. See mtgban.Scraper.
func (ck *Cardkingdom) Info() (info mtgban.ScraperInfo) {
	info.Name = "Card Kingdom"
	info.Shorthand = "CK"
	info.InventoryTimestamp = &ck.inventoryDate
	info.BuylistTimestamp = &ck.buylistDate
	info.CreditMultiplier = 1.3
	info.Game = ck.backend.Game
	return
}
