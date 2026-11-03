package manapool

import (
	"context"
	"net/url"
	"strconv"
	"time"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
)

// Sealed prices Mana Pool's sealed product.
type Sealed struct {
	logCallback mtgban.LogCallbackFunc
	partner     string

	backend *matcher.Backend

	inventoryDate time.Time
	inventory     mtgban.InventoryRecord
}

// NewScraperSealed returns a sealed scraper matching against b.
func NewScraperSealed(b *matcher.Backend) *Sealed {
	mp := Sealed{backend: b}
	mp.inventory = mtgban.InventoryRecord{}
	return &mp
}

func (mp *Sealed) printf(format string, a ...any) {
	if mp.logCallback != nil {
		mp.logCallback("[MPSealed] "+format, a...)
	}
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (mp *Sealed) Load(ctx context.Context) error {
	pricelist, err := GetSealedList(ctx)
	if err != nil {
		return err
	}

	mp.printf("Found %d prices", len(pricelist))

	var foundProduct int

	sets := mp.backend.GetAllSets()
	for _, code := range sets {
		set, _ := mp.backend.GetSet(code)

		// Skip products without Sealed or Booster information
		switch set.Code {
		case "FBB", "4BB", "DRKITA", "LEGITA", "RIN", "4EDALT", "BCHR":
			continue
		}

		for _, product := range set.SealedProduct {
			tcgIDStr, found := product.Identifiers["tcgplayerProductId"]
			if !found {
				continue
			}

			tcgID, err := strconv.Atoi(tcgIDStr)
			if err != nil {
				continue
			}

			for _, sealed := range pricelist {
				if tcgID != sealed.TcgplayerProductID {
					continue
				}

				foundProduct++

				// Build URL
				u, err := url.Parse(sealed.URL)
				if err != nil {
					mp.printf("%v", err)
					continue
				}
				v := url.Values{}
				if mp.partner != "" {
					v.Set("ref", mp.partner)
				}
				u.RawQuery = v.Encode()

				out := &mtgban.InventoryEntry{
					Price:     float64(sealed.LowPrice) / 100.0,
					Available: sealed.AvailableQuantity,
					URL:       u.String(),
				}
				err = mp.inventory.AddUnique(product.UUID, out)
				if err != nil {
					mp.printf("%v", err)
				}
			}
		}
	}

	perc := float64(foundProduct) * 100 / float64(len(pricelist))
	mp.printf("Found %d products over %d items (%.02f%%)", foundProduct, len(pricelist), perc)

	mp.inventoryDate = time.Now()

	return nil
}

// Inventory returns what Load collected. See mtgban.Seller.
func (mp *Sealed) Inventory() mtgban.InventoryRecord {
	return mp.inventory
}

// Info describes this scraper. See mtgban.Scraper.
func (mp *Sealed) Info() (info mtgban.ScraperInfo) {
	info.Name = "Manapool"
	info.Shorthand = "MPSealed"
	info.InventoryTimestamp = &mp.inventoryDate
	info.SealedMode = true
	info.NoQuantityInventory = true
	info.Game = mp.backend.Game
	return
}
