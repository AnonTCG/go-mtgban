// Package starcitygames scrapes Star City Games, for singles and sealed
// product, across every game they carry.
//
// See README.md for the id-collision defect class this package spends
// most of its resolution logic defending against, and the method for
// finding and fixing the next one.
package starcitygames

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
)

// Starcitygames prices SCG's singles, both what they sell and what they buy.
type Starcitygames struct {
	logCallback   mtgban.LogCallbackFunc
	inventoryDate time.Time
	buylistDate   time.Time

	affiliate string

	targetEdition string

	inventory mtgban.InventoryRecord
	buylist   mtgban.BuylistRecord

	// buckets holds the listings a second stock record has already been seen
	// for, so the pair is filed as one listing in either stream order.
	buckets map[string]struct{}

	setIDs  map[string]int
	offers  map[string]map[mtgban.Condition]sellListVariant
	client  *SCGClient
	backend *matcher.Backend
	gameID  int
}

// NewScraper returns a singles scraper for one game, using the given API key.
func NewScraper(b *matcher.Backend, apiKey string) (*Starcitygames, error) {
	game := b.Game
	gameID, ok := scgGames[game]
	if !ok {
		return nil, fmt.Errorf("unsupported game %q", game)
	}
	scg := Starcitygames{}
	scg.reset()
	scg.client = NewSCGClient(apiKey)
	scg.backend = b
	scg.gameID = gameID
	return &scg, nil
}

// reset drops everything a run has collected, for a stream that broke and has
// to be read again from the top. What a listing's records have already been
// seen goes with the records themselves: they are all coming again.
func (scg *Starcitygames) reset() {
	scg.inventory = mtgban.InventoryRecord{}
	scg.buylist = mtgban.BuylistRecord{}
	scg.buckets = map[string]struct{}{}
}

func (scg *Starcitygames) printf(format string, a ...any) {
	if scg.logCallback != nil {
		scg.logCallback("[SCG] "+format, a...)
	}
}

func (scg *Starcitygames) processProduct(p CatalogProduct) {
	// A single malformed product must never abort the whole catalog stream;
	// recover, log, and skip it.
	defer func() {
		if r := recover(); r != nil {
			scg.printf("recovered from panic on %q (sku=%s): %v", p.Name, p.SKU, r)
		}
	}()

	// This scraper handles singles only; sealed has its own scraper, and
	// the catalog also carries supplies and bulk lots that are not cards
	// at all. product_type says which is which, so nothing else has to
	// infer it from the name or the sku.
	if p.ProductType != ProductTypeSingles {
		return
	}
	if !strings.HasPrefix(p.SKU, "SGL-") {
		return
	}
	if gameFromCatalog(p.Game) != scg.gameID {
		return
	}
	if scg.targetEdition != "" && scg.targetEdition != p.Set {
		return
	}

	cardID, err := resolveProduct(scg.backend, scg.gameID, p)
	if err != nil {
		if errors.Is(err, matcher.ErrUnsupported) {
			return
		}
		// Skip tokens and similar
		if strings.Contains(p.Name, "Token") || strings.HasPrefix(p.Name, "{") {
			return
		}
		scg.printf("%v for %q [%s %s #%s] sku=%s scryfall=%s", err, p.Name, p.Set, p.Finish, p.CollectorNumber, p.SKU, p.ScryfallID)

		var alias *matcher.AliasingError
		if errors.As(err, &alias) {
			for _, probe := range alias.Probe() {
				co, _ := scg.backend.GetUUID(probe)
				scg.printf("- %s", co)
			}
		}
		return
	}

	link := SCGProductURL(p.URL, "", scg.affiliate)

	// The buylist link points at the sell-your-cards page for this printing,
	// not the retail page; fall back to retail if the set can't be matched.
	buyURL := link
	ids := setIDsForProduct(scg.backend, scg.setIDs, p.Set, p.SKU)
	if len(ids) > 0 {
		buyURL = SCGBuylistURL(scg.gameID, p.Name, p.Language, ids)
	}

	customFields := map[string]string{
		"SCGName":     p.Name,
		"SCGEdition":  p.Set,
		"SCGLanguage": p.Language,
		"SCGFinish":   p.Finish,
		"scgNumber":   p.CollectorNumber,
		"scgSKU":      p.SKU,
	}

	ignore := strings.Contains(p.Set, "World Championship") || strings.Contains(p.Name, "Token")

	// A second stock bucket of a listing the catalog also carries plainly.
	// Star City Games splits some Armory Deck singles across two product
	// records - "SGL-FAB-AGB-014-ENN" beside "SGL-FAB-AGB-014_CC-ENN" - which
	// agree on the card, the set, the number, the treatment and the price of
	// every grade they share, and differ only in how many copies sit in each.
	// They are one listing's stock written twice over, so the second record
	// is filed as an offer of its own, under its own sku, rather than
	// reported as the duplicate it would otherwise look like.
	//
	// Which of the two comes down the stream first is not the catalog's to
	// promise, and it does not hold: most Armory Deck: Pleiades listings lead
	// with the marked record. So it is the one that arrives second that is
	// filed that way, whether or not it wears the marker - the first is the
	// one there is nothing yet to stand beside.
	second := scg.secondStock(p.SKU)

	retailPrices := map[mtgban.Condition]float64{}
	for _, v := range p.Variants {
		condition, err := catalogCondition(v.Condition)
		if err != nil {
			scg.printf("unsupported %s condition", v.Condition)
			continue
		}

		retailPrice, _ := matcher.ParsePrice(v.Price)
		retailPrices[condition] = retailPrice

		if retailPrice > 0 && v.Qty > 0 {
			entry := &mtgban.InventoryEntry{
				Price:      retailPrice,
				Conditions: condition,
				Quantity:   v.Qty,
				OriginalID: p.SKU,
				InstanceID: v.SKU,
				URL:        SCGProductURL(p.URL, v.SKU, scg.affiliate),
			}
			if condition == mtgban.NM {
				entry.CustomFields = customFields
			}
			if err := scg.addInventoryStock(second, cardID, entry); err != nil && !ignore {
				scg.printf("%s", err.Error())
				scg.printf("-> %s", link)
			}
		}

	}

	for _, condition := range []mtgban.Condition{mtgban.NM, mtgban.SP, mtgban.HP} {
		offer, found := scg.offers[p.SKU][condition]
		if !found {
			continue
		}

		var priceRatio float64
		if retailPrices[condition] > 0 {
			priceRatio = offer.BuyPrice / retailPrices[condition] * 100
		}

		var blFields map[string]string
		if condition == mtgban.NM {
			blFields = customFields
		}

		entry := &mtgban.BuylistEntry{
			Conditions:   condition,
			BuyPrice:     offer.BuyPrice,
			PriceRatio:   priceRatio,
			URL:          buyURL,
			OriginalID:   p.SKU,
			InstanceID:   offer.SKU,
			CustomFields: blFields,
		}
		err := scg.addBuylistStock(second, cardID, entry)
		if err != nil && !ignore {
			scg.printf("%s", err.Error())
		}

		// SCG grades no MP, so pay HP's price there too.
		if condition == mtgban.HP {
			mp := *entry
			mp.Conditions = mtgban.MP
			err := scg.addBuylistStock(second, cardID, &mp)
			if err != nil && !ignore {
				scg.printf("%s", err.Error())
			}
		}
	}
}

// loadCatalog streams the catalog export and fills the inventory from it,
// and the buylist from the offers already read for each product it names.
func (scg *Starcitygames) loadCatalog(ctx context.Context) error {
	setIDs, err := scg.client.SetIDs(ctx, scg.gameID)
	if err != nil {
		scg.printf("could not load set ids for buylist links: %v", err)
	}
	scg.setIDs = setIDs

	count, ofGame := 0, 0
	err = scg.client.StreamCatalog(ctx, func() {
		scg.printf("Catalog stream broke after %d products, downloading it again", count)
		scg.reset()
		count, ofGame = 0, 0
	}, func(p CatalogProduct) error {
		if gameFromCatalog(p.Game) == scg.gameID {
			ofGame++
		}
		scg.processProduct(p)
		count++
		if count%5000 == 0 {
			scg.printf("Processed %d products", count)
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Every game SCG is registered for has singles in the export, so none
	// at all means the catalog spells the game in a way gameFromCatalog
	// does not map, and the run would publish an empty dump.
	if ofGame == 0 {
		return fmt.Errorf("none of %d catalog products maps to %s", count, scg.backend.Game)
	}
	scg.printf("Processed %d products total", count)

	now := time.Now()
	scg.inventoryDate = now
	scg.buylistDate = now
	return nil
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (scg *Starcitygames) Load(ctx context.Context) error {
	var buylistErr error
	offers, err := scg.client.sellListOffers(ctx, scg.gameID)
	if err != nil {
		buylistErr = fmt.Errorf("%w: %w", mtgban.ErrBuylistLoad, err)
	}
	scg.offers = offers

	err = scg.loadCatalog(ctx)
	if err != nil {
		return fmt.Errorf("catalog load failed: %w", err)
	}
	return buylistErr
}

// secondStock reports whether this sku is the second stock bucket of a
// listing already seen this run: by the marker it wears, or by its plain
// twin arriving after the marked record registered the pair. Which of the
// two the catalog streams first is not its to promise, and it does not
// hold, so whichever arrives second is the one filed beside the first.
func (scg *Starcitygames) secondStock(sku string) bool {
	key := bucketKey(sku)
	if secondBucket(sku) {
		scg.buckets[key] = struct{}{}
		return true
	}
	_, paired := scg.buckets[key]
	return paired
}

// addInventoryStock files a variant's retail record with the strictness the
// bucket decision picked: a second bucket is filed beside the first rather
// than reading as the duplicate it would otherwise be, and a primary record is
// unique - one card sells for one price at one grade, so a second one here is
// the sweep's own signature, two catalog products folding onto one uuid.
func (scg *Starcitygames) addInventoryStock(second bool, cardID string, entry *mtgban.InventoryEntry) error {
	if second {
		return scg.inventory.Add(cardID, entry)
	}
	return scg.inventory.AddUnique(cardID, entry)
}

// addBuylistStock is addInventoryStock for the buylist side of the record.
func (scg *Starcitygames) addBuylistStock(second bool, cardID string, entry *mtgban.BuylistEntry) error {
	if second {
		return scg.buylist.AddRelaxed(cardID, entry)
	}
	return scg.buylist.AddUnique(cardID, entry)
}

// Inventory returns what Load collected. See mtgban.Seller.
func (scg *Starcitygames) Inventory() mtgban.InventoryRecord {
	return scg.inventory
}

// Buylist returns what Load collected. See mtgban.Vendor.
func (scg *Starcitygames) Buylist() mtgban.BuylistRecord {
	return scg.buylist
}

// Info describes this scraper. See mtgban.Scraper.
func (scg *Starcitygames) Info() (info mtgban.ScraperInfo) {
	info.Name = "Star City Games"
	info.Shorthand = "SCG"
	info.InventoryTimestamp = &scg.inventoryDate
	info.BuylistTimestamp = &scg.buylistDate
	info.CreditMultiplier = 1.3
	info.Game = scg.backend.Game
	return
}
