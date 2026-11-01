package tcgplayer

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"

	"github.com/mtgban/go-tcgplayer"
)

// Index prices Magic singles from the partner API's price guide, the
// low and market numbers rather than any one seller's listing.
type Index struct {
	logCallback    mtgban.LogCallbackFunc
	inventoryDate  time.Time
	affiliate      string
	maxConcurrency int

	inventory mtgban.InventoryRecord

	backend *matcher.Backend
	client  *tcgplayer.Client

	// printings maps a product id and the subtype of its price rows to
	// the printing that product sells in that finish.
	printings map[string]map[string]string

	// links ties the catalog products the datastore names no card for to
	// the card each one sells; see LoadCatalogLinks.
	links SKUMap
}

var availableIndexNames = []string{
	"TCG Low", "TCG Market", "TCG Mid", "TCG Direct Low",
}

func (tcg *Index) printf(format string, a ...any) {
	if tcg.logCallback != nil {
		tcg.logCallback("[TCGIndex] "+format, a...)
	}
}

// NewScraperIndex returns an index scraper authenticated with a partner API
// key pair.
func NewScraperIndex(b *matcher.Backend, publicID, privateID string) (*Index, error) {
	client, err := tcgplayer.NewClient(publicID, privateID)
	if err != nil {
		return nil, err
	}

	tcg := Index{}
	tcg.backend = b
	tcg.inventory = mtgban.InventoryRecord{}
	tcg.client = client
	tcg.maxConcurrency = defaultConcurrency
	return &tcg, nil
}

func (tcg *Index) processEntry(ctx context.Context, channel chan<- responseChan, reqs []string) error {
	var ids []int
	for _, req := range reqs {
		id, err := strconv.Atoi(req)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}

	results, err := tcg.client.GetMarketPricesByProducts(ctx, ids)
	if err != nil {
		return err
	}

	for _, result := range results {
		// Skip empty entries
		if result.LowPrice == 0 && result.MarketPrice == 0 && result.MidPrice == 0 && result.DirectLowPrice == 0 {
			continue
		}

		cardID, found := tcg.printings[fmt.Sprint(result.ProductID)][result.SubTypeName]
		if !found {
			continue
		}
		co, _ := tcg.backend.GetUUID(cardID)

		// These are sorted as in availableIndexNames
		prices := []float64{
			result.LowPrice, result.MarketPrice, result.MidPrice, getDirectPrice(result.DirectLowPrice),
		}

		for i := range availableIndexNames {
			if prices[i] == 0 {
				continue
			}

			// Certain sets are marked as English on the site despite not being as such
			// Override here, so that links don't point at filters that provide no results
			lang := co.Language
			switch co.SetCode {
			case "STA", "SOA":
				lang = "English"
			}

			isDirect := availableIndexNames[i] == "TCG Direct Low"
			link := GenerateProductURL(result.ProductID, result.SubTypeName, tcg.affiliate, "", lang, isDirect)

			out := responseChan{
				cardID: cardID,
				entry: mtgban.InventoryEntry{
					Conditions: mtgban.NM,
					Price:      prices[i],
					Quantity:   1,
					URL:        link,
					SellerName: availableIndexNames[i],
					Bundle:     isDirect,
					OriginalID: fmt.Sprint(result.ProductID),
				},
			}

			channel <- out
		}
	}

	return nil
}

// crossSetProductIDs reports every TCGplayer product id (regular or etched)
// claimed by cards from more than one set, keyed to the sets that claim it.
// A double-faced card's two faces, or a plain misprint beside the copy it
// doubles, always share a set - a same-named oversized sibling does not, and
// neither does an id MTGJSON has simply copied onto the wrong printing (the
// Dungeon of the Mad Mage report this guards against: AFR's ordinary card
// and OAFR's oversized one both carry 245106, which belongs only to the
// oversized product). Spanning more than one set means the id is wrong on
// at least one of them, and pricing from it would be a guess.
func crossSetProductIDs(b *matcher.Backend) map[string][]string {
	setsByID := map[string]map[string]bool{}
	for _, code := range b.GetAllSets() {
		set, err := b.GetSet(code)
		if err != nil {
			continue
		}
		for _, card := range set.Cards {
			for _, key := range [...]string{"tcgplayerProductId", "tcgplayerEtchedProductId"} {
				id, found := card.Identifiers[key]
				if !found || soldAsCopy(b, id, card.Language) {
					continue
				}
				if setsByID[id] == nil {
					setsByID[id] = map[string]bool{}
				}
				setsByID[id][code] = true
			}
		}
	}

	collisions := map[string][]string{}
	for id, sets := range setsByID {
		if len(sets) <= 1 {
			continue
		}
		for code := range sets {
			collisions[id] = append(collisions[id], code)
		}
		sort.Strings(collisions[id])
	}
	return collisions
}

// soldAsCopy reports whether a printing in language is a copy sold under id,
// a product the datastore files at its original in another language.
func soldAsCopy(b *matcher.Backend, id, language string) bool {
	original, err := b.GetUUID(b.ConvertID(matcher.IDSpaceTCGplayer, id))
	return err == nil && original.Language != language
}

// starTwin returns the star printing that is the other finish of the card's
// product, if there is one: the same language, numbered "<number>★" and with
// no product of its own (FRF 65★ is the foil of 95037).
func starTwin(byNumber map[string]*matcher.CardObject, card *matcher.Card) *matcher.CardObject {
	twin, found := byNumber[card.Number+"★|"+card.Language]
	if !found || twin.Identifiers["tcgplayerProductId"] != "" {
		return nil
	}
	return twin
}

// cardsByNumber indexes a set's cards by number and language.
func cardsByNumber(b *matcher.Backend, set *matcher.Set) map[string]*matcher.CardObject {
	byNumber := map[string]*matcher.CardObject{}
	for _, card := range set.Cards {
		co, found := b.UUIDs[card.UUID]
		if found {
			byNumber[co.Number+"|"+co.Language] = co
		}
	}
	return byNumber
}

// productPrintings maps each product id, and the subtype its price rows
// carry, to the printing the datastore says that product sells in that
// finish: "Normal" for nonfoil, "Foil" for foil, and for etched on an etched
// product. A product claimed by more than one card keeps the first, the same
// way requesting each id once always did.
func productPrintings(b *matcher.Backend, collisions map[string][]string, links SKUMap) map[string]map[string]string {
	printings := map[string]map[string]string{}
	add := func(id, subtype, cardID string) {
		if id == "" || cardID == "" || collisions[id] != nil {
			return
		}
		if printings[id] == nil {
			printings[id] = map[string]string{}
		}
		if _, found := printings[id][subtype]; !found {
			printings[id][subtype] = cardID
		}
	}

	for _, code := range b.GetAllSets() {
		set, err := b.GetSet(code)
		if err != nil {
			continue
		}
		byNumber := cardsByNumber(b, set)
		for _, card := range set.Cards {
			co, found := b.UUIDs[card.UUID]
			if !found {
				continue
			}
			id := co.Identifiers["tcgplayerProductId"]
			if id == "" {
				id = linkedProductID(links, co.Identifiers["mtgjsonId"])
			}
			etchedID := co.Identifiers["tcgplayerEtchedProductId"]
			if soldAsCopy(b, id, co.Language) || soldAsCopy(b, etchedID, co.Language) {
				continue
			}
			if etchedID == "" {
				etchedID = id
			}
			add(id, "Normal", co.FoilUUIDs[matcher.FinishNonfoil])
			add(id, "Foil", co.FoilUUIDs[matcher.FinishFoil])
			add(etchedID, "Foil", co.FoilUUIDs[matcher.FinishEtched])
		}

		for _, base := range byNumber {
			co := starTwin(byNumber, &base.Card)
			if co == nil {
				continue
			}
			id := base.Identifiers["tcgplayerProductId"]
			add(id, "Normal", co.FoilUUIDs[matcher.FinishNonfoil])
			add(id, "Foil", co.FoilUUIDs[matcher.FinishFoil])
		}
	}

	// A two-sided token sheet's combined entity is in no set's card list
	// (see matcher/magic/tokenpairs.go), one object per finish.
	for uuid, co := range b.UUIDs {
		if co.Identifiers["derivedTokenPair"] != "true" {
			continue
		}
		subtype := "Normal"
		if co.Foil || co.Etched {
			subtype = "Foil"
		}
		add(co.Identifiers["tcgplayerProductId"], subtype, uuid)
	}
	return printings
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (tcg *Index) Load(ctx context.Context) error {
	collisions := crossSetProductIDs(tcg.backend)
	for id, sets := range collisions {
		tcg.printf("skipping id %s, claimed by more than one set: %v", id, sets)
	}
	tcg.printings = productPrintings(tcg.backend, collisions, tcg.links)

	ids := slices.Collect(maps.Keys(tcg.printings))

	consume := func(result responseChan) {
		// Relaxed because sometimes we get duplicates due to how the ids
		// get buffered, but there is really no harm
		err := tcg.inventory.AddRelaxed(result.cardID, &result.entry)
		if err != nil {
			tcg.printf("%s", err.Error())
		}
	}
	mtgban.WorkerPool(ctx, tcg.maxConcurrency, slices.Collect(slices.Chunk(ids, tcgplayer.MaxIDsInRequest)),
		func(ctx context.Context, ids []string, channel chan<- responseChan) error {
			return tcg.processEntry(ctx, channel, ids)
		},
		consume,
		tcg.printf,
	)
	err := ctx.Err()
	if err != nil {
		return err
	}

	tcg.inventoryDate = time.Now()

	return nil
}

// Inventory returns what Load collected. See mtgban.Seller.
func (tcg *Index) Inventory() mtgban.InventoryRecord {
	return tcg.inventory
}

// MarketNames names the sub-sellers this market splits into. See
// mtgban.Market.
func (tcg *Index) MarketNames() []string {
	return availableIndexNames
}

// InfoForScraper describes one of the sub-scrapers named above.
func (tcg *Index) InfoForScraper(name string) mtgban.ScraperInfo {
	info := tcg.Info()
	info.Name = name
	info.Shorthand = name2shorthand[name]
	return info
}

// Info describes this scraper. See mtgban.Scraper.
func (tcg *Index) Info() (info mtgban.ScraperInfo) {
	info.Name = "TCG Player Index"
	info.Shorthand = "TCGIndex"
	info.InventoryTimestamp = &tcg.inventoryDate
	info.MetadataOnly = true
	info.NoQuantityInventory = true
	info.Family = "TCG"
	info.Game = tcg.backend.Game
	return
}
