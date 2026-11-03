// Package manaleak scrapes Manaleak.
package manaleak

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
)

const (
	defaultConcurrency = 8
)

// Manaleak prices Manaleak's Magic singles, both what they sell and what
// they buy. The storefront quotes pounds; every price is converted to
// dollars at the day's rate.
type Manaleak struct {
	logCallback    mtgban.LogCallbackFunc
	maxConcurrency int

	backend *matcher.Backend

	disableRetail  bool
	disableBuylist bool

	client *MLClient
	rate   float64

	inventoryDate time.Time
	buylistDate   time.Time
	inventory     mtgban.InventoryRecord
	buylist       mtgban.BuylistRecord

	// listedNM is the listing's price of each card in stock, which is its
	// top grade's whatever grades the page holds; the buylist's ratio
	// reads it.
	listedNM map[string]float64

	// The retail rows whose product page did and did not hold a grade in
	// stock.
	graded, gradeless int
}

// NewScraper returns a scraper matching against b.
func NewScraper(b *matcher.Backend) *Manaleak {
	ml := Manaleak{backend: b}
	ml.inventory = mtgban.InventoryRecord{}
	ml.buylist = mtgban.BuylistRecord{}
	ml.listedNM = map[string]float64{}
	ml.client = NewMLClient()
	ml.maxConcurrency = defaultConcurrency
	return &ml
}

// SetConfig applies options after the scraper was built. See
// mtgban.ScraperConfig.
func (ml *Manaleak) SetConfig(opt mtgban.ScraperOptions) {
	ml.disableRetail = opt.DisableRetail
	ml.disableBuylist = opt.DisableBuylist
}

func (ml *Manaleak) printf(format string, a ...any) {
	if ml.logCallback != nil {
		ml.logCallback("[ML] "+format, a...)
	}
}

// nameTypos holds the names the storefront misspells, keyed by the spelling
// it lists.
var nameTypos = map[string]string{
	"Dread Sanctuary": "Dread Statuary",
}

// staleTCGIDs maps a TCGplayer product id the storefront still lists, and
// TCGplayer has deleted, to the live product of the same card.
var staleTCGIDs = map[string]string{
	"507429": "510091", // Fists of Flame, Commander Masters 223
}

// promoShelf is the storefront's catch-all category for promos.
const promoShelf = "Promos"

// siblingShelves are the storefront's own shelves of a set's promos (box
// promos, launch and release events, prerelease, game days), which its
// product pages file under the set's own name. The datastore holds those as
// the set's Promos sibling. Where a shelf holds one kind of promo, the value
// is the word naming it, which spells out a trailing " - Promo" qualifier.
var siblingShelves = map[string]string{
	"MBP":  "",
	"MLP":  "Release",
	"PTC":  "Prerelease",
	"MGDC": "",
	"REP":  "",
}

// promoShelfWords maps the letters a Promos-shelf product code opens with to
// the word the matcher reads the program by, for the shelves whose index is
// the storefront's own rather than a collector number. The shelves not
// listed either index by collector number or file several programs together.
var promoShelfWords = map[string]string{
	"FNMP":  "FNM",
	"MPRP":  "Player Rewards",
	"ARENA": "Arena League",
	"MGDC":  "Game Day",
	"CLASH": "Clash Pack",
	"GPX":   "Grand Prix",
	"JR":    "Judge Gift",
	"PRO":   "Pro Tour",
}

// promoSetType is the type MTGJSON gives the sets of promos, and starterSetType
// the one it gives a clash pack, whose card is a promo printing when the pack
// is filed under a main set.
const (
	promoSetType   = "promo"
	starterSetType = "starter"
)

// qualifierSpellings lists the storefront's spelling of a promo qualifier
// and the one the matcher reads.
var qualifierSpellings = [][2]string{
	{" - Box Promo", " - Buy-a-Box Promo"},
}

// collectorDigits reads the number out of a collector number, past its
// leading zeros and before any suffix.
var collectorDigits = regexp.MustCompile(`[1-9]\d*`)

// match resolves the card a row lists. The newer sets carry their TCGplayer
// product id and the older ones their multiverse id, each converted through
// its own id space; either answers by itself unless it needs the name to
// confirm it. The name and set only speak for a row whose id the datastore
// does not know.
func (ml *Manaleak) match(product MLProduct) (string, error) {
	// Tokens are out of scope, and a product page names one by its set only.
	tokenName, isToken := strings.CutSuffix(product.Name, " Token")
	if product.FromPage && isToken && ml.backend.IsToken(tokenName) {
		return "", matcher.ErrUnsupported
	}

	cardName := product.Name
	foil := false
	etched := false
	switch {
	case strings.HasSuffix(cardName, " - Etched Foil"), strings.HasSuffix(cardName, " - Foil Etched"):
		cardName = strings.TrimSuffix(cardName, " - Etched Foil")
		cardName = strings.TrimSuffix(cardName, " - Foil Etched")
		etched = true
	case strings.HasSuffix(cardName, " - Foil"):
		cardName = strings.TrimSuffix(cardName, " - Foil")
		foil = true
	}
	cardName = cmp.Or(nameTypos[cardName], cardName)
	for _, spelling := range qualifierSpellings {
		cardName = strings.Replace(cardName, spelling[0], spelling[1], 1)
	}
	foil = foil || product.Foil

	for _, guess := range product.idGuesses() {
		inputID := guess.id
		if guess.space == matcher.IDSpaceTCGplayer {
			inputID = cmp.Or(staleTCGIDs[inputID], inputID)
		}
		cardID, err := ml.backend.MatchID(ml.backend.ConvertID(guess.space, inputID), foil, etched)
		if err == nil && (!guess.confirm || ml.nameAgrees(cardID, cardName)) {
			return cardID, nil
		}
	}

	in := matcher.InputCard{Name: cardName, Edition: product.SetName, Foil: foil}
	kind, sibling := siblingShelves[product.Shelf]
	switch {
	case product.FromPage && product.SetName == promoShelf:
		cardID, err := ml.backend.Match(&in)
		return ml.onPromoShelf(product, cardName, cardID, err, foil, etched)
	case product.FromPage && sibling:
		if kind != "" {
			for _, tail := range []string{" Promo", " Promo Foil"} {
				stem, found := strings.CutSuffix(in.Name, " -"+tail)
				if found {
					in.Name = stem + " - " + kind + tail
				}
			}
		}
		return ml.inSibling(in, etched)
	}
	return ml.backend.Match(&in)
}

// inSibling settles a wording match for a row on one of a set's own promo
// shelves. The printing counts only when it sits in a promo set filed under
// the set the category names and holds the row's finish: a card the Promos
// sibling does not hold lands on a main-set or another event's printing,
// which is not what the row sells, so the row is left unpriced. A category
// naming no set, such as "Magic 2015 Core Set", accepts any promo set. A card
// the category's sets do not file may still have a single promo printing.
func (ml *Manaleak) inSibling(in matcher.InputCard, etched bool) (string, error) {
	parent := ""
	main, err := ml.backend.GetSetByName(in.Edition)
	if err == nil {
		parent = main.Code
	}

	var refused error
	for _, edition := range []string{in.Edition + " Promos", in.Edition} {
		try := in
		try.Edition = edition
		cardID, err := ml.backend.Match(&try)
		if err != nil {
			if refused == nil && !errors.Is(err, matcher.ErrUnsupported) &&
				!errors.As(err, new(*matcher.AliasingError)) {
				refused = err
			}
			continue
		}
		co, err := ml.backend.GetUUID(cardID)
		if err != nil || co.Foil != in.Foil || co.Etched != etched {
			continue
		}
		set, err := ml.backend.GetSet(co.SetCode)
		if err == nil && set.Type == promoSetType && (parent == "" || set.ParentCode == parent) {
			return cardID, nil
		}
	}
	cardID, err := ml.onlyPromo(in.Name, in.Foil, etched)
	if err == nil {
		return cardID, nil
	}
	if refused != nil {
		return "", refused
	}
	return "", matcher.ErrUnsupported
}

// onPromoShelf settles a wording match on the Promos shelf, which files the
// promos of every set under one name. A printing counts when it holds the
// collector number the product code ends in, in the row's finish. Where the
// index is the storefront's own, the shelf's program word may name the
// printing. Otherwise the row counts only if the card has a single promo
// printing, and is left unpriced if not.
func (ml *Manaleak) onPromoShelf(product MLProduct, cardName, cardID string, err error, foil, etched bool) (string, error) {
	word, byProgram := promoShelfWords[product.Shelf]

	var candidates []string
	var aliasing *matcher.AliasingError
	switch {
	case err == nil:
		candidates = []string{cardID}
	case errors.As(err, &aliasing):
		candidates = aliasing.Probe()
	case !byProgram:
		return "", err
	}

	// A program shelf's index is the storefront's own, and names no printing
	number := product.Number
	if byProgram {
		number = ""
	}
	var found []string
	for _, id := range candidates {
		co, uuidErr := ml.backend.GetUUID(id)
		if uuidErr == nil && co.Foil == foil && co.Etched == etched &&
			number != "" && collectorDigits.FindString(co.Number) == number {
			found = append(found, id)
		}
	}
	if len(found) == 1 {
		return found[0], nil
	}

	if byProgram {
		in := matcher.InputCard{Name: cardName, Variation: word, Foil: foil}
		cardID, err = ml.backend.Match(&in)
		if err == nil {
			co, uuidErr := ml.backend.GetUUID(cardID)
			if uuidErr == nil && co.Foil == foil && co.Etched == etched {
				return cardID, nil
			}
		}
	}
	return ml.onlyPromo(cardName, foil, etched)
}

// onlyPromo answers the one promo or clash pack printing a card has in a
// finish, whatever the wording of the shelf. A card with several is left
// unpriced, since nothing says which one the row sells.
func (ml *Manaleak) onlyPromo(cardName string, foil, etched bool) (string, error) {
	name, _, _ := strings.Cut(cardName, " - ")
	ids, err := ml.backend.SearchEquals(name)
	if err != nil {
		return "", matcher.ErrUnsupported
	}

	var found []string
	for _, id := range ids {
		co, err := ml.backend.GetUUID(id)
		if err != nil || co.Foil != foil || co.Etched != etched {
			continue
		}
		set, err := ml.backend.GetSet(co.SetCode)
		if err == nil && (set.Type == promoSetType ||
			set.Type == starterSetType && set.ParentCode != "") {
			found = append(found, id)
		}
	}
	if len(found) != 1 {
		return "", matcher.ErrUnsupported
	}
	return found[0], nil
}

// nameAgrees reports whether cardID is one of the printings named cardName,
// used to confirm an AmbiguousID guess before trusting it. The search reads
// a double-faced card by either face; the printing's own name is asked
// first, since a derived token pair is kept out of the search.
func (ml *Manaleak) nameAgrees(cardID, cardName string) bool {
	if cardName == "" {
		return false
	}
	co, err := ml.backend.GetUUID(cardID)
	if err == nil && matcher.Equals(co.Name, cardName) {
		return true
	}
	ids, err := ml.backend.SearchEquals(cardName)
	if err != nil {
		return false
	}
	return slices.Contains(ids, cardID)
}

// wanted reports whether a row is priced and, on the retail side, in stock.
func wanted(mode string, product MLProduct) bool {
	return product.Price != 0 && (mode != modeRetail || !product.OutOfStock)
}

func (ml *Manaleak) processProduct(mode string, product MLProduct) {
	if !wanted(mode, product) {
		return
	}
	// The brand page lists the sealed product and the repacks beside the
	// singles; only a row wearing a card image id, or whose product page
	// names a card, is one.
	if len(product.idGuesses()) == 0 && !product.FromPage {
		return
	}

	cardID, err := ml.match(product)
	if errors.Is(err, matcher.ErrUnsupported) {
		return
	} else if err != nil {
		ml.printf("%v", err)
		ml.printf("%s: %q [%s]", mode, product.Name, product.SetName)
		return
	}

	if mode == modeRetail {
		ml.addGrades(cardID, product)
	} else {
		var priceRatio float64
		sellPrice := ml.listedNM[cardID]
		if sellPrice > 0 {
			priceRatio = product.Price * ml.rate / sellPrice * 100
		}

		err = ml.buylist.Add(cardID, &mtgban.BuylistEntry{
			Conditions: mtgban.NM,
			BuyPrice:   product.Price * ml.rate,
			PriceRatio: priceRatio,
			URL:        product.URL,
			InstanceID: product.ProductID,
		})
	}
	if err != nil && !errors.Is(err, mtgban.ErrDuplicateEntry) {
		ml.printf("%s", err.Error())
	}
}

// addGrades lists a row once per grade its product page holds in stock,
// each at its own price and quantity.
func (ml *Manaleak) addGrades(cardID string, product MLProduct) {
	// The cheapest listing, whichever product page answers first
	price := product.Price * ml.rate
	listed, found := ml.listedNM[cardID]
	if !found || price < listed {
		ml.listedNM[cardID] = price
	}
	if len(product.Grades) == 0 {
		ml.gradeless++
		return
	}
	ml.graded++
	for _, grade := range product.Grades {
		cond, err := mtgban.ParseCondition(grade.Name)
		if err != nil {
			ml.printf("unsupported %s condition", grade.Name)
			continue
		}
		err = ml.inventory.Add(cardID, &mtgban.InventoryEntry{
			Conditions: cond,
			Price:      grade.Price * ml.rate,
			Quantity:   grade.Quantity,
			URL:        product.URL,
			InstanceID: product.ProductID,
		})
		if err != nil && !errors.Is(err, mtgban.ErrDuplicateEntry) {
			ml.printf("%s", err.Error())
		}
	}
}

const (
	modeRetail  = "retail"
	modeBuylist = "buylist"
)

func (ml *Manaleak) scrape(ctx context.Context, mode string) error {
	base := inventoryURL
	if mode == modeBuylist {
		base = buylistURL
	}

	// A page that fails past the client's retries loses its rows, and a
	// store missing them would publish as whole.
	var failed atomic.Int32

	// A row whose image names no card waits for its product page, and so
	// does every retail row, whose grades only that page lists.
	var paged []MLProduct
	take := func(products []MLProduct) {
		for _, product := range products {
			if wanted(mode, product) && product.ProductID != "" &&
				(mode == modeRetail || len(product.idGuesses()) == 0) {
				paged = append(paged, product)
				continue
			}
			ml.processProduct(mode, product)
		}
	}

	// The first page carries the listing's own total, which sizes the
	// fan-out over the rest.
	products, total, err := ml.client.GetPage(ctx, base, 1)
	if err != nil {
		return err
	}
	take(products)

	pages, err := pageCount(total, len(products))
	if err != nil {
		return fmt.Errorf("%s: %w", mode, err)
	}
	ml.printf("%s: %d products over %d pages", mode, total, pages)

	pageNums := make([]int, 0, pages)
	for page := 2; page <= pages; page++ {
		pageNums = append(pageNums, page)
	}

	mtgban.WorkerPool(ctx, ml.maxConcurrency, pageNums,
		func(ctx context.Context, page int, results chan<- []MLProduct) error {
			products, _, err := ml.client.GetPage(ctx, base, page)
			if err != nil {
				failed.Add(1)
				return fmt.Errorf("page %d: %s", page, err.Error())
			}
			results <- products
			return nil
		},
		take,
		ml.printf,
	)

	ml.printf("%s: %d product pages", mode, len(paged))
	mtgban.WorkerPool(ctx, ml.maxConcurrency, paged,
		func(ctx context.Context, product MLProduct, results chan<- MLProduct) error {
			product, err := ml.client.GetProduct(ctx, product)
			if errors.Is(err, errNotFound) {
				// Removed since the listing was read
				return nil
			}
			if err != nil {
				failed.Add(1)
				return fmt.Errorf("product %s: %s", product.ProductID, err.Error())
			}
			results <- product
			return nil
		},
		func(product MLProduct) {
			ml.processProduct(mode, product)
		},
		ml.printf,
	)

	err = ctx.Err()
	if err != nil {
		return err
	}
	n := failed.Load()
	if n > 0 {
		return fmt.Errorf("%s: %d of %d listing and product pages failed", mode, n, pages+len(paged))
	}
	if mode == modeRetail {
		ml.printf("%s: %d rows with a grade in stock, %d without", mode, ml.graded, ml.gradeless)
		// The listing calls these in stock, so none graded means the
		// condition select moved
		if ml.graded == 0 && ml.gradeless > 0 {
			return fmt.Errorf("%s: no product page named a grade in stock", mode)
		}
	}
	return nil
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (ml *Manaleak) Load(ctx context.Context) error {
	rate, err := mtgban.GetExchangeRate(ctx, "GBP")
	if err != nil {
		return err
	}
	ml.rate = rate

	var errs []error

	if !ml.disableRetail {
		err := ml.scrape(ctx, modeRetail)
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %w", mtgban.ErrInventoryLoad, err))
		} else {
			ml.inventoryDate = time.Now()
		}
	}

	if !ml.disableBuylist {
		err := ml.scrape(ctx, modeBuylist)
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %w", mtgban.ErrBuylistLoad, err))
		} else {
			ml.buylistDate = time.Now()
		}
	}

	return errors.Join(errs...)
}

// Inventory returns what Load collected. See mtgban.Seller.
func (ml *Manaleak) Inventory() mtgban.InventoryRecord {
	return ml.inventory
}

// Buylist returns what Load collected. See mtgban.Vendor.
func (ml *Manaleak) Buylist() mtgban.BuylistRecord {
	return ml.buylist
}

// Info describes this scraper. See mtgban.Scraper.
func (ml *Manaleak) Info() (info mtgban.ScraperInfo) {
	info.Name = "Manaleak"
	info.Shorthand = "ML"
	info.Game = ml.backend.Game
	info.CountryFlag = "GB"
	info.InventoryTimestamp = &ml.inventoryDate
	info.BuylistTimestamp = &ml.buylistDate
	return
}

// pageCount sizes the fan-out off the listing's own count of what it
// paginates. A count smaller than the first page is a count that was not
// read - the "Showing … of N" line moved or went missing - and sizing the
// run on it would walk one page and report it as the whole store.
func pageCount(total, onFirstPage int) (int, error) {
	if total < onFirstPage {
		return 0, fmt.Errorf("the listing counts %d products and its first page holds %d", total, onFirstPage)
	}
	return (total + pageLimit - 1) / pageLimit, nil
}
