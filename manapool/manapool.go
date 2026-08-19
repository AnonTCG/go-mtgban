// Package manapool scrapes Mana Pool, for singles and sealed product.
package manapool

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/mtgban/go-mtgban/mtgban"
	"github.com/mtgban/go-mtgban/mtgmatcher"
)

// Manapool prices Mana Pool's catalog.
type Manapool struct {
	LogCallback mtgban.LogCallbackFunc
	Partner     string

	inventoryDate time.Time
	inventory     mtgban.InventoryRecord
}

// NewScraper returns a scraper pricing Mana Pool's singles, both what they
// sell and what they buy.
func NewScraper() *Manapool {
	mp := Manapool{}
	mp.inventory = mtgban.InventoryRecord{}
	return &mp
}

func (mp *Manapool) printf(format string, a ...any) {
	if mp.LogCallback != nil {
		mp.LogCallback("[MP] "+format, a...)
	}
}

// isUnindexed reports whether the backend was never meant to know this card,
// so that failing to match its id is expected and not worth reporting. Whole
// editions are dropped when the datastore is built (oversize, minigames,
// front cards, playtest, token-only sets), and tokens are never indexed even
// in the editions that are kept.
func isUnindexed(card Product) bool {
	_, err := mtgmatcher.GetSet(card.SetCode)
	if err != nil || mtgmatcher.IsToken(card.Name) {
		return true
	}

	// Double-faced tokens are named after both of their faces
	faces := strings.Split(card.Name, " // ")
	if len(faces) < 2 {
		return false
	}
	for _, face := range faces {
		if !mtgmatcher.IsToken(face) {
			return false
		}
	}
	return true
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (mp *Manapool) Load(ctx context.Context) error {
	pricelist, err := GetPriceList(ctx)
	if err != nil {
		return err
	}

	mp.printf("Found %d prices", len(pricelist))

	for _, card := range pricelist {
		cardID, err := mtgmatcher.MatchID(card.ScryfallID, card.FinishID == "FO", card.FinishID == "EF")
		if err != nil {
			if !isUnindexed(card) {
				mp.printf("%v %s for %s [%s]", err, card.ScryfallID, card.Name, card.SetCode)
			}
			continue
		}

		// Validate language
		co, err := mtgmatcher.GetUUID(cardID)
		if err != nil {
			continue
		}
		if mtgmatcher.LanguageTag2LanguageCode[co.Language] != strings.ToLower(card.LanguageID) {
			continue
		}

		// Build URL
		u, err := url.Parse(card.URL)
		if err != nil {
			mp.printf("%v", err)
			continue
		}
		v := url.Values{}
		if mp.Partner != "" {
			v.Set("ref", mp.Partner)
		}
		v.Set("conditions", card.ConditionID)
		switch card.FinishID {
		case "EF":
			v.Set("finish", "etched")
		case "FO":
			v.Set("finish", "foil")
		case "NF":
			v.Set("finish", "nonfoil")
		}
		u.RawQuery = v.Encode()
		link := u.String()

		// Match conditions
		conds := card.ConditionID
		switch card.ConditionID {
		case "NM", "MP", "HP":
		case "LP":
			conds = "SP"
		case "DMG":
			conds = "PO"
		default:
			mp.printf("Unknown %s condition for %s (%s)", conds, card.Name, card.SetCode)
			continue
		}

		// Store the RAW item LowPrice (cents → dollars). The 4.2% Manapool
		// buyer fee is a buyer-facing acquisition cost, not part of the card's
		// value — valuation (sealed EV / exports) uses the listed price, not the
		// fee-inclusive cost. The fee rate is preserved in CustomFields so card-
		// view display can reconstruct "what a buyer pays" (price * 1.042).
		price := float64(card.LowPrice) / 100.0

		// Got there!
		out := &mtgban.InventoryEntry{
			Conditions: conds,
			Price:      price,
			URL:        link,
			CustomFields: map[string]string{
				"buyerFeeRate": "0.042",
			},
		}
		err = mp.inventory.AddUnique(cardID, out)
		if err != nil {
			mp.printf("%v", err)
		}
	}

	mp.inventoryDate = time.Now()

	return nil
}

// Inventory returns what Load collected. See mtgban.Seller.
func (mp *Manapool) Inventory() mtgban.InventoryRecord {
	return mp.inventory
}

// Info describes this scraper. See mtgban.Scraper.
func (mp *Manapool) Info() (info mtgban.ScraperInfo) {
	info.Name = "Mana Pool"
	info.Shorthand = "MP"
	info.InventoryTimestamp = &mp.inventoryDate
	info.NoQuantityInventory = true
	return
}
