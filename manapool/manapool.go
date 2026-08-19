// Package manapool scrapes Mana Pool, for singles and sealed product.
package manapool

import (
	"cmp"
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/matcher/magic"
	"github.com/mtgban/go-mtgban/mtgban"
)

// Manapool prices Mana Pool's catalog.
type Manapool struct {
	logCallback mtgban.LogCallbackFunc
	partner     string

	backend *matcher.Backend

	inventoryDate time.Time
	inventory     mtgban.InventoryRecord
}

// NewScraper returns a scraper pricing Mana Pool's singles, both what they
// sell and what they buy, matching against b.
func NewScraper(b *matcher.Backend) *Manapool {
	mp := Manapool{backend: b}
	mp.inventory = mtgban.InventoryRecord{}
	return &mp
}

func (mp *Manapool) printf(format string, a ...any) {
	if mp.logCallback != nil {
		mp.logCallback("[MP] "+format, a...)
	}
}

// isUnindexed reports whether the backend was never meant to know this card,
// so that failing to match its id is expected and not worth reporting. Whole
// editions are dropped when the datastore is built - oversize, minigames,
// front cards, playtest - and a sheet of tokens the datastore carries no set
// for is dropped the same way, which is what the edition answers for.
func isUnindexed(b *matcher.Backend, card Product) bool {
	_, err := b.GetSet(card.SetCode)
	return err != nil
}

// isTokenSheet reports whether the printing a listing landed on sits in a set
// of tokens, the only place a two-sided listing is a pairing of two faces.
func isTokenSheet(b *matcher.Backend, co *matcher.CardObject) bool {
	set, err := b.GetSet(co.SetCode)
	return err == nil && set.Type == "token"
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (mp *Manapool) Load(ctx context.Context) error {
	pricelist, err := GetPriceList(ctx)
	if err != nil {
		return err
	}

	mp.printf("Found %d prices", len(pricelist))
	mp.price(pricelist)
	mp.inventoryDate = time.Now()
	return nil
}

// price records every row of the list the store answers with.
func (mp *Manapool) price(pricelist []Product) {
	for _, card := range pricelist {
		foil := card.FinishID == "FO"
		etched := card.FinishID == "EF"

		cardID, err := mp.backend.MatchID(card.ScryfallID, foil, etched)
		if err != nil {
			if !isUnindexed(mp.backend, card) {
				mp.printf("%v %s for %s [%s]", err, card.ScryfallID, card.Name, card.SetCode)
			}
			continue
		}
		// A two-sided token sheet prints one physical card for a pairing
		// matcher/magic may already carry a combined entity for. mtgjson
		// already models some of these natively (one single entity of its
		// own, its own name already "X // Y") - the plain resolution above
		// is already correct for those, told apart by its own Name already
		// containing " // ". For the rest, Mana Pool's own scryfall_id
		// names Scryfall's single representative face, not a combined one,
		// so the plain resolution above lands on that one face alone -
		// resolve through the combined entity instead, by that same id
		// plus the listing's own name (see magic.MatchTokenPairing, shared
		// with cardkingdom's own version of this same problem), and refuse
		// rather than keep the single-face result when no combined entity
		// is on file: mtgjson's own tokenProducts feed simply has no
		// record of every pairing a vendor sells (see matcher/magic's
		// own datastore pairing data), and a listing whose own
		// name says two faces is worth more than a single-face guess would
		// silently be wrong for is worth refusing instead.
		co, err := mp.backend.GetUUID(cardID)
		if err != nil {
			continue
		}
		if strings.Contains(card.Name, " // ") && !strings.Contains(co.Name, " // ") &&
			isTokenSheet(mp.backend, co) {
			cardID = ""
			// MatchTokenPairing answers either with a bare derived-entity
			// uuid (no usable TCGplayer id at all) or a raw TCGplayer
			// product id (an ordinary derived pairing) - MatchID resolves
			// either shape to the real uuid record() needs, the same way
			// the plain path above already did.
			if id := magic.MatchTokenPairing(mp.backend, card.ScryfallID, card.Name, foil); id != "" {
				cardID, _ = mp.backend.MatchID(id, foil, etched)
			}
			if cardID == "" {
				continue
			}
		}
		mp.record(card, cardID)
	}
}

// record validates and prices one already-resolved row, the tail end of
// price() shared by both the plain scryfall-id path and the token-pairing
// one above.
func (mp *Manapool) record(card Product, cardID string) {
	// Validate language
	co, err := mp.backend.GetUUID(cardID)
	if err != nil {
		return
	}
	// The table names English by the empty tag, and Portuguese without the
	// "(Brazil)" the datastore adds, so the printing's language holds it.
	lang, found := matcher.LanguageCode2LanguageTag[strings.ToLower(card.LanguageID)]
	if !found || !strings.Contains(co.Language, cmp.Or(lang, "English")) {
		return
	}

	// Build URL
	u, err := url.Parse(card.URL)
	if err != nil {
		mp.printf("%v", err)
		return
	}
	v := url.Values{}
	if mp.partner != "" {
		v.Set("ref", mp.partner)
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
	grade, err := mtgban.ParseCondition(card.ConditionID)
	if err != nil {
		mp.printf("unsupported %s condition", card.ConditionID)
		return
	}

	// AnonTCG: store the RAW item LowPrice (cents → dollars). The 4.2%
	// Manapool buyer fee is a buyer-facing acquisition cost, not part of the
	// card's value — valuation (sealed EV / exports) uses the listed price,
	// not the fee-inclusive cost. The fee rate is preserved in CustomFields
	// so card-view display can reconstruct "what a buyer pays"
	// (price * 1.042). Upstream multiplies the fee in here.
	price := float64(card.LowPrice) / 100.0

	// Got there! The product id is what Mana Pool's cart optimizer takes
	out := &mtgban.InventoryEntry{
		Conditions: grade,
		Price:      price,
		Available:  card.AvailableQuantity,
		URL:        link,
		InstanceID: card.ProductID,
		CustomFields: map[string]string{
			"buyerFeeRate": "0.042",
		},
	}
	mp.addCheapest(cardID, out)
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
	info.Game = mp.backend.Game
	return
}

// addCheapest records one price per printing and grade: the lowest one, with
// its product's link and id, and the copies every product holds as its
// Available. The list carries a row per product, and a printing the store
// files under more than one product - the same token from several decks, a
// card in two of its own product lines - arrives once per product, each with
// its own low price. The site shows one row for the grade, and a buyer pays
// the lower of them. Available can so count more copies than the product
// InstanceID names holds.
func (mp *Manapool) addCheapest(cardID string, entry *mtgban.InventoryEntry) {
	err := mp.inventory.AddUnique(cardID, entry)
	if !errors.Is(err, mtgban.ErrDuplicateEntry) {
		if err != nil {
			mp.printf("%v", err)
		}
		return
	}
	entries := mp.inventory[cardID]
	for i := range entries {
		if entries[i].Conditions != entry.Conditions {
			continue
		}
		entries[i].Available += entry.Available
		if entry.Price < entries[i].Price {
			entries[i].Price = entry.Price
			entries[i].URL = entry.URL
			entries[i].InstanceID = entry.InstanceID
		}
	}
}
