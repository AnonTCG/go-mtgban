// Package cardgamecorner scrapes Card Game Corner.
package cardgamecorner

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"sync/atomic"
	"time"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/matcher/magic"
	"github.com/mtgban/go-mtgban/mtgban"
)

const (
	defaultConcurrency = 8
)

// Cardgamecorner prices Card Game Corner's singles of one game: what they
// sell in every game, and what they buy in Magic.
type Cardgamecorner struct {
	verboseLog     bool
	logCallback    mtgban.LogCallbackFunc
	inventoryDate  time.Time
	buylistDate    time.Time
	maxConcurrency int

	disableRetail  bool
	disableBuylist bool

	exchangeRate float64

	// bridge maps a Cardmarket product id to the TCGplayer id of the same
	// product; nil where bantool gave none.
	bridge map[int]int

	inventory mtgban.InventoryRecord
	buylist   mtgban.BuylistRecord
	client    *Client
	backend   *matcher.Backend
}

// NewScraper returns a scraper against the datastore, failing if the
// datastore's game is not one the store sells.
func NewScraper(b *matcher.Backend) (*Cardgamecorner, error) {
	game, ok := cgcGames[b.Game]
	if !ok {
		return nil, fmt.Errorf("unsupported game %q", b.Game)
	}
	cgc := Cardgamecorner{}
	cgc.inventory = mtgban.InventoryRecord{}
	cgc.buylist = mtgban.BuylistRecord{}
	cgc.client = NewClient(game)
	cgc.backend = b
	cgc.maxConcurrency = defaultConcurrency
	return &cgc, nil
}

type resultChan struct {
	cardID   string
	invEntry *mtgban.InventoryEntry
	buyEntry *mtgban.BuylistEntry
}

func (cgc *Cardgamecorner) printf(format string, a ...any) {
	if cgc.logCallback != nil {
		cgc.logCallback("[CGC] "+format, a...)
	}
}

// blindAttraction reports whether the printings an aliasing error names are
// all attractions. Their printings are told apart by which of their lights
// are lit and by nothing else, and this store publishes a print-run index
// instead - a number that names no light, and that no other seller repeats
// - so a listing of one names them all equally. There is nothing to choose
// between, and nothing worth reporting.
func blindAttraction(b *matcher.Backend, probes []string) bool {
	for _, probe := range probes {
		co, err := b.GetUUID(probe)
		if err != nil || magic.AttractionLights(&co.Card) == "" {
			return false
		}
	}
	return len(probes) > 0
}

// wantedLanguage reports whether a copy in the given language is read. Only
// English is, with a few shelves where another language is the one the
// datastore carries. Yu-Gi-Oh files its North American English copies as
// "US".
func wantedLanguage(game matcher.Game, language, edition string) bool {
	switch language {
	case "EN":
		return true
	case "US":
		return game == matcher.GameYuGiOh
	case "JP":
		return edition == "War of the Spark: Japanese Alternate-Art Planeswalkers"
	case "IT":
		switch edition {
		case "Revised EU FBB", "Rinascimento", "L'Oscurità", "Leggende":
			return true
		}
	}
	return false
}

func (cgc *Cardgamecorner) processEntry(channel chan<- resultChan, cards []listing) {
	// Keep track of the processed ids, and don't add duplicates
	duplicate := map[int]bool{}
	// A preprocess failure is the shelf's, so it is logged once per edition
	failedEditions := map[string]bool{}

	match, found := gameMatchers[cgc.backend.Game]
	if !found {
		match = matchWording
	}

	for _, l := range cards {
		edition, card := l.edition, l.card

		for i, v := range card.Variants {
			// Skip duplicate cards
			if duplicate[v.ID] {
				if cgc.verboseLog {
					cgc.printf("Skipping duplicate card: %s (%s %s)", card.Name, card.Edition, v.Foil)
				}
				continue
			}

			if !wantedLanguage(cgc.backend.Game, v.Language, edition.Name) {
				continue
			}

			if v.Quantity < 1 {
				continue
			}

			grade, err := mtgban.ParseCondition(v.Condition)
			if err != nil {
				cgc.printf("unsupported %s condition", v.Condition)
				continue
			}

			theCard, err := preprocess(cgc.backend, &card, i)
			if errors.Is(err, matcher.ErrUnsupported) {
				continue
			} else if err != nil {
				if !failedEditions[card.Edition] {
					cgc.printf("%s: %v", card.Edition, err)
					failedEditions[card.Edition] = true
				}
				continue
			}

			cardID, err := match(cgc, &card, i, theCard)
			if errors.Is(err, matcher.ErrUnsupported) {
				continue
			} else if err != nil {
				var alias *matcher.AliasingError
				aliased := errors.As(err, &alias)
				if aliased && blindAttraction(cgc.backend, alias.Probe()) {
					continue
				}

				cgc.printf("%v", err)
				cgc.printf("%q", theCard)
				cgc.printf("%q", card)

				if aliased {
					for _, probe := range alias.Probe() {
						card, _ := cgc.backend.GetUUID(probe)
						cgc.printf("- %s", card)
					}
				}
				continue
			}

			if cgc.backend.Game == matcher.GameMagic && v.Foil == "Foil" {
				cardID = foilStarTwin(cgc.backend, cardID)
			}

			channel <- resultChan{
				cardID: cardID,
				invEntry: &mtgban.InventoryEntry{
					Conditions: grade,
					Price:      v.Price * cgc.exchangeRate,
					Quantity:   v.Quantity,
					URL:        "https://www.cardgamecorner.com" + card.URL,
					OriginalID: fmt.Sprint(card.ID),
					InstanceID: fmt.Sprint(v.ID),
				},
			}

			duplicate[v.ID] = true
		}
	}
}

// listing is a card as one edition lists it.
type listing struct {
	edition Edition
	card    Card
}

// ownersBySKU picks the listing each SKU's stock is read against. Listings
// sharing a SKU are one card listed twice, and the catch-all Promo edition,
// or a copy missing the English name or the image, is the worse one to read
// it against.
func ownersBySKU(listed []listing, editions []Edition) map[string]listing {
	order := map[int]int{}
	for i, edition := range editions {
		order[edition.ID] = i
	}
	rank := func(l *listing) (promo, incomplete int) {
		if l.edition.ID == promoEditionID {
			promo++
		}
		if l.card.Name == l.card.OrigName {
			incomplete++
		}
		if l.card.Extra == "/noimage.png" {
			incomplete++
		}
		return promo, incomplete
	}
	slices.SortFunc(listed, func(a, b listing) int {
		promoA, incompleteA := rank(&a)
		promoB, incompleteB := rank(&b)
		return cmp.Or(
			cmp.Compare(promoA, promoB),
			cmp.Compare(incompleteA, incompleteB),
			cmp.Compare(order[a.edition.ID], order[b.edition.ID]),
			cmp.Compare(a.card.ID, b.card.ID),
		)
	})

	owners := map[string]listing{}
	for _, l := range listed {
		_, found := owners[l.card.SKU]
		if !found {
			owners[l.card.SKU] = l
		}
	}
	return owners
}

// Scrape returns an array of Entry, containing pricing and card information.
// Every edition is listed before any stock is read, because the stock is
// asked for by SKU alone, and a SKU can be listed more than once.
func (cgc *Cardgamecorner) scrape(ctx context.Context) error {
	editionList, err := cgc.client.GetEditionList(ctx, cgc.backend.Game == matcher.GameMagic)
	if err != nil {
		return err
	}

	var listed []listing
	var failedEditions atomic.Int32
	mtgban.WorkerPool(ctx, cgc.maxConcurrency, editionList,
		func(ctx context.Context, edition Edition, results chan<- listing) error {
			cards, err := cgc.client.GetInventoryForEdition(ctx, edition)
			if err != nil {
				failedEditions.Add(1)
				return err
			}
			if cgc.verboseLog {
				cgc.printf("Processing id %d - %s (%d cards)", edition.ID, edition.Name, len(cards))
			}
			var noSKU int
			for _, card := range cards {
				// A SKU is all the stock is asked for by, so a card without
				// one has none to read
				if card.SKU == "" {
					noSKU++
					continue
				}
				results <- listing{edition: edition, card: card}
			}
			if noSKU > 0 {
				cgc.printf("%s: %d cards listed without a SKU", edition.Name, noSKU)
			}
			return nil
		},
		func(l listing) {
			listed = append(listed, l)
		},
		cgc.printf,
	)
	if failedEditions.Load() > 0 {
		return fmt.Errorf("%d of %d editions failed to list", failedEditions.Load(), len(editionList))
	}
	if len(listed) == 0 {
		return errors.New("no edition listed any card")
	}

	owners := ownersBySKU(listed, editionList)
	skus := slices.Sorted(maps.Keys(owners))

	batches := slices.Collect(slices.Chunk(skus, variantsBatchSize))
	var failedBatches atomic.Int32
	mtgban.WorkerPool(ctx, cgc.maxConcurrency, batches,
		func(ctx context.Context, batch []string, results chan<- resultChan) error {
			variants, err := cgc.client.GetVariants(ctx, batch)
			if err != nil {
				failedBatches.Add(1)
				return err
			}
			bySKU := map[string][]Variant{}
			for _, v := range variants {
				bySKU[v.SKU] = append(bySKU[v.SKU], v)
			}
			cards := make([]listing, 0, len(batch))
			for _, sku := range batch {
				owner := owners[sku]
				owner.card.Variants = bySKU[sku]
				cards = append(cards, owner)
			}
			cgc.processEntry(results, cards)
			return nil
		},
		func(result resultChan) {
			err := cgc.inventory.AddRelaxed(result.cardID, result.invEntry)
			if err != nil {
				cgc.printf("%s", err.Error())
			}
		},
		cgc.printf,
	)

	// A batch holds cards from every edition, so a partial inventory would
	// read as a whole one, and so would a cancelled run's
	if failedBatches.Load() > 0 {
		return fmt.Errorf("%d of %d stock batches failed", failedBatches.Load(), len(batches))
	}
	err = ctx.Err()
	if err != nil {
		return err
	}
	// Every listed card has a copy in stock, so a stock read naming none
	// failed without saying so
	if len(cgc.inventory) == 0 {
		return fmt.Errorf("no stock read for %d listed cards", len(skus))
	}

	cgc.inventoryDate = time.Now()

	return nil
}

// SetConfig applies options after the scraper was built. See
// mtgban.ScraperConfig.
func (cgc *Cardgamecorner) SetConfig(opt mtgban.ScraperOptions) {
	cgc.disableRetail = opt.DisableRetail
	cgc.disableBuylist = opt.DisableBuylist
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (cgc *Cardgamecorner) Load(ctx context.Context) error {
	var errs []error

	// Both sides price in euro, so the rate has to be in place before either
	// runs: fetched from the retail path alone it stayed zero whenever retail
	// was disabled, and every buy price came out at zero with it
	rate, err := mtgban.GetExchangeRate(ctx, "EUR")
	if err != nil {
		return err
	}
	cgc.exchangeRate = rate

	if !cgc.disableRetail {
		err := cgc.scrape(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %w", mtgban.ErrInventoryLoad, err))
		}
	}

	// Only Magic's buylist is read
	if !cgc.disableBuylist && cgc.backend.Game == matcher.GameMagic {
		err := cgc.scrapeBL(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %w", mtgban.ErrBuylistLoad, err))
		}
	}

	return errors.Join(errs...)
}

// Inventory returns what Load collected. See mtgban.Seller.
func (cgc *Cardgamecorner) Inventory() mtgban.InventoryRecord {
	return cgc.inventory
}

// Buylist returns what Load collected. See mtgban.Vendor.
func (cgc *Cardgamecorner) Buylist() mtgban.BuylistRecord {
	return cgc.buylist
}

func (cgc *Cardgamecorner) parseBL(ctx context.Context, channel chan<- resultChan, edition Expansion) error {
	i := 1
	totals := 0
	for {
		cgc.printf("Querying %s page %d", edition.Name, i)
		result, err := cgc.client.GetBuylistForEdition(ctx, edition.ID, i)
		if err != nil {
			return err
		}

		for _, product := range result.Products {
			// Product is not being bought
			if product.SerialNumber == 99999 {
				continue
			}

			cardName := product.ModelEn
			edition := product.Category
			price := product.MinAcquisto
			qty := min(product.Quantity, 4)

			if price == 0 {
				continue
			}

			theCard, err := preprocessBL(cgc.backend, cardName, edition, product.ID, product.SerialNumber)
			if err != nil {
				continue
			}

			cardID, err := cgc.backend.Match(theCard)
			if errors.Is(err, matcher.ErrUnsupported) {
				continue
			} else if err != nil {
				var alias *matcher.AliasingError
				aliased := errors.As(err, &alias)
				if aliased && blindAttraction(cgc.backend, alias.Probe()) {
					continue
				}

				cgc.printf("%v", err)
				cgc.printf("%q", theCard)

				if aliased {
					for _, probe := range alias.Probe() {
						card, _ := cgc.backend.GetUUID(probe)
						cgc.printf("- %s", card)
					}
				}
				continue
			}

			link := fmt.Sprintf("https://www.cardgamecorner.com/it/buylist?q=%s&game=magic", url.QueryEscape(product.ModelEn))

			gradeMap := map[mtgban.Condition]float64{
				mtgban.NM: 1, mtgban.SP: 0.77, mtgban.MP: 0, mtgban.HP: 0.36,
			}
			for _, grade := range mtgban.DefaultGradeTags {
				factor := gradeMap[grade]
				if factor == 0 {
					continue
				}

				var quantity int
				if grade == mtgban.NM {
					quantity = qty
				}

				channel <- resultChan{
					cardID: cardID,
					buyEntry: &mtgban.BuylistEntry{
						Quantity:   quantity,
						Conditions: grade,
						BuyPrice:   price * cgc.exchangeRate * factor,
						URL:        link,
						OriginalID: product.ID,
					},
				}
			}
		}

		i++
		totals += len(result.Products)
		if totals >= result.Total {
			break
		}

		// The exit above is a fixpoint: a successful page carrying no products
		// leaves totals where it was while Total stays positive, so the same
		// request repeats forever
		if len(result.Products) == 0 {
			return fmt.Errorf("%s: page %d was empty after %d of %d products", edition.Name, i, totals, result.Total)
		}
	}

	return nil
}

func (cgc *Cardgamecorner) scrapeBL(ctx context.Context) error {
	editions, err := cgc.client.GetBuylistEditions(ctx)
	if err != nil {
		return err
	}
	cgc.printf("Found %d editions", len(editions))

	var failedEditions atomic.Int32
	mtgban.WorkerPool(ctx, cgc.maxConcurrency, editions,
		func(ctx context.Context, edition Expansion, results chan<- resultChan) error {
			err := cgc.parseBL(ctx, results, edition)
			if err != nil {
				failedEditions.Add(1)
			}
			return err
		},
		func(record resultChan) {
			err := cgc.buylist.AddRelaxed(record.cardID, record.buyEntry)
			if err != nil {
				cgc.printf("%s", err.Error())
			}
		},
		cgc.printf,
	)

	// An edition that failed is indistinguishable from one with nothing to
	// buy once its entries are missing, so a partial buylist must not pass
	if failedEditions.Load() > 0 {
		return fmt.Errorf("%d of %d editions failed to read", failedEditions.Load(), len(editions))
	}
	err = ctx.Err()
	if err != nil {
		return err
	}
	if len(cgc.buylist) == 0 {
		return fmt.Errorf("no buylist entry read in %d editions", len(editions))
	}

	cgc.buylistDate = time.Now()

	return nil
}

// Info describes this scraper. See mtgban.Scraper.
func (cgc *Cardgamecorner) Info() (info mtgban.ScraperInfo) {
	info.Name = "Card Game Corner"
	info.Shorthand = "CGC"
	info.CountryFlag = "EU"
	info.InventoryTimestamp = &cgc.inventoryDate
	info.BuylistTimestamp = &cgc.buylistDate
	info.Game = cgc.backend.Game
	return
}
