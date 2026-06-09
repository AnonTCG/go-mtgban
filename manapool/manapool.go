package manapool

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/mtgban/go-mtgban/mtgban"
	"github.com/mtgban/go-mtgban/mtgmatcher"
)

type Manapool struct {
	LogCallback mtgban.LogCallbackFunc
	Partner     string

	inventoryDate time.Time
	inventory     mtgban.InventoryRecord
}

func NewScraper() *Manapool {
	mp := Manapool{}
	mp.inventory = mtgban.InventoryRecord{}
	return &mp
}

func (mp *Manapool) printf(format string, a ...interface{}) {
	if mp.LogCallback != nil {
		mp.LogCallback("[MP] "+format, a...)
	}
}

func (mp *Manapool) Load(ctx context.Context) error {
	pricelist, err := GetPriceList(ctx)
	if err != nil {
		return err
	}

	mp.printf("Found %d prices", len(pricelist))

	for _, card := range pricelist {
		cardId, err := mtgmatcher.MatchId(card.ScryfallID, card.FinishID == "FO", card.FinishID == "EF")
		if err != nil {
			// Skip errors for unsupported cards (tokens, art cards, front cards)
			if !mtgmatcher.IsToken(card.Name) &&
				!strings.HasPrefix(card.SetCode, "T") &&
				!strings.HasPrefix(card.SetCode, "A") &&
				!strings.HasPrefix(card.SetCode, "F") {
				mp.printf("%v %s for %s [%s]", err, card.ScryfallID, card.Name, card.SetCode)
			}
			continue
		}

		// Validate language
		co, err := mtgmatcher.GetUUID(cardId)
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
		err = mp.inventory.AddUnique(cardId, out)
	}

	mp.inventoryDate = time.Now()

	return nil
}

func (mp *Manapool) Inventory() mtgban.InventoryRecord {
	return mp.inventory
}

func (mp *Manapool) Info() (info mtgban.ScraperInfo) {
	info.Name = "Manapool"
	info.Shorthand = "MP"
	info.InventoryTimestamp = &mp.inventoryDate
	info.NoQuantityInventory = true
	return
}
