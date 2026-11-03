package cardkingdom

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mtgban/go-cardkingdom"
	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
)

const (
	ckBuylistLink = "https://www.cardkingdom.com/purchasing/mtg_sealed"
)

// Sealed prices Card Kingdom's sealed product.
type Sealed struct {
	logCallback mtgban.LogCallbackFunc
	partner     string
	preserveOOS bool

	inventoryDate time.Time
	buylistDate   time.Time

	inventory mtgban.InventoryRecord
	buylist   mtgban.BuylistRecord

	backend *matcher.Backend
}

// NewScraperSealed returns a sealed scraper.
func NewScraperSealed(b *matcher.Backend) *Sealed {
	ck := Sealed{}
	ck.inventory = mtgban.InventoryRecord{}
	ck.buylist = mtgban.BuylistRecord{}
	ck.backend = b
	return &ck
}

func (ck *Sealed) printf(format string, a ...any) {
	if ck.logCallback != nil {
		ck.logCallback("[CKSealed] "+format, a...)
	}
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (ck *Sealed) Load(ctx context.Context) error {
	pricelist, err := cardkingdom.SealedPricelist(ctx, mtgban.NewHTTPClient())
	if err != nil {
		return err
	}

	foundProduct := 0

	sets := ck.backend.GetAllSets()
	for _, code := range sets {
		set, _ := ck.backend.GetSet(code)

		// Skip products without Sealed or Booster information
		switch set.Code {
		case "FBB", "4BB", "DRKITA", "LEGITA", "RIN", "4EDALT", "BCHR":
			continue
		}

		for _, product := range set.SealedProduct {
			ckIDStr, found := product.Identifiers["cardKingdomId"]
			if !found {
				continue
			}

			ckID, err := strconv.Atoi(ckIDStr)
			if err != nil {
				continue
			}

			for _, sealed := range pricelist {
				if ckID != sealed.ID {
					continue
				}

				foundProduct++

				u, _ := url.Parse("https://www.cardkingdom.com/")

				// Rebuild the URL to have the same format as non-sealed
				basename := strings.TrimPrefix(sealed.URL, "mtg-sealed/")

				// Slugify the edition
				edition := strings.ToLower(sealed.Edition)
				edition = strings.Replace(edition, "'", "", -1)
				edition = strings.Replace(edition, ":", "", -1)
				edition = strings.Replace(edition, ".", "", -1)
				edition = strings.Replace(edition, ",", "", -1)
				edition = strings.Replace(edition, " -", "", -1)
				edition = strings.Replace(edition, "&", "and", -1)
				edition = strings.Replace(edition, " ", "-", -1)

				u.Path = "mtg" + "/" + edition + "/" + basename

				if ck.partner != "" {
					q := u.Query()
					q.Set("partner", ck.partner)
					q.Set("utm_source", ck.partner)
					q.Set("utm_medium", "affiliate")
					q.Set("utm_campaign", ck.partner)
					u.RawQuery = q.Encode()
				}
				link := u.String()

				if sealed.QtyRetail > 0 && sealed.PriceRetail > 0 {
					out := &mtgban.InventoryEntry{
						Conditions: mtgban.NM,
						Price:      sealed.PriceRetail,
						Quantity:   sealed.QtyRetail,
						URL:        link,
					}
					err = ck.inventory.Add(product.UUID, out)
					if err != nil {
						ck.printf("%v", err)
					}
				} else if ck.preserveOOS {
					// Only save URL information
					out := &mtgban.InventoryEntry{
						URL: link,
					}
					err = ck.inventory.AddUnique(product.UUID, out)
					if err != nil {
						ck.printf("%v", err)
					}
				}

				if sealed.QtyBuying > 0 && sealed.PriceBuy > 0 {
					var priceRatio float64

					if sealed.PriceRetail > 0 {
						priceRatio = sealed.PriceBuy / sealed.PriceRetail * 100
					}

					u, _ = url.Parse(ckBuylistLink)
					q := u.Query()
					q.Set("filter[sort]", "price_desc")
					q.Set("search", "mtg_advanced")
					q.Set("filter[name]", sealed.Name)
					q.Set("filter[edition]", "")
					q.Set("filter[subtype]", "all")
					if ck.partner != "" {
						q.Set("partner", ck.partner)
						q.Set("utm_source", ck.partner)
						q.Set("utm_medium", "affiliate")
						q.Set("utm_campaign", ck.partner)
					}
					u.RawQuery = q.Encode()

					out := &mtgban.BuylistEntry{
						BuyPrice:   sealed.PriceBuy,
						Quantity:   sealed.QtyBuying,
						PriceRatio: priceRatio,
						URL:        u.String(),
					}
					err = ck.buylist.Add(product.UUID, out)
					if err != nil {
						ck.printf("%v", err)
					}
				}
			}
		}
	}

	perc := float64(foundProduct) * 100 / float64(len(pricelist))
	ck.printf("Found %d products over %d items (%.02f%%)", foundProduct, len(pricelist), perc)

	ck.inventoryDate = time.Now()
	ck.buylistDate = time.Now()

	return nil
}

// Inventory returns what Load collected. See mtgban.Seller.
func (ck *Sealed) Inventory() mtgban.InventoryRecord {
	return ck.inventory
}

// Buylist returns what Load collected. See mtgban.Vendor.
func (ck *Sealed) Buylist() mtgban.BuylistRecord {
	return ck.buylist
}

// Info describes this scraper. See mtgban.Scraper.
func (ck *Sealed) Info() (info mtgban.ScraperInfo) {
	info.Name = "Card Kingdom"
	info.Shorthand = "CKSealed"
	info.InventoryTimestamp = &ck.inventoryDate
	info.BuylistTimestamp = &ck.buylistDate
	info.SealedMode = true
	info.CreditMultiplier = 1.3
	info.Game = ck.backend.Game
	return
}
