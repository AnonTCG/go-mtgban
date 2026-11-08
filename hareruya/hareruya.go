// Package hareruya scrapes Hareruya, for singles and sealed product.
package hareruya

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/matcher/magic"
	"github.com/mtgban/go-mtgban/mtgban"

	"github.com/PuerkitoBio/goquery"
)

const (
	defaultConcurrency = 8

	modeInventory = "inventory"
	modeBuylist   = "buylist"

	buylistURL = "https://www.hareruyamtg.com/ja/purchase/search?"
)

// reAltered finds the note a storefront grade carries for a copy someone
// signed or inked, or that ink, water or a chipped case has damaged, in
// English or Japanese ("MP Signed", "BGS8 サイン9点", "Poor Water").
var reAltered = regexp.MustCompile(`Singed|Signed|Ink|Water|サイン|欠け`)

// reSlab reads a graded slab's label, grader and score ("BGS8.5").
var reSlab = regexp.MustCompile(`^(PSA|BGS|CGC)\s*(\S+)$`)

// Hareruya prices Hareruya's singles.
type Hareruya struct {
	logCallback    mtgban.LogCallbackFunc
	maxConcurrency int

	backend *matcher.Backend

	exchangeRate float64

	inventory     mtgban.InventoryRecord
	inventoryDate time.Time
	disableRetail bool

	buylist        mtgban.BuylistRecord
	buylistDate    time.Time
	disableBuylist bool

	targetEdition string

	client *http.Client
}

// NewScraper returns a singles scraper matching against b.
func NewScraper(b *matcher.Backend) *Hareruya {
	ha := Hareruya{backend: b}
	ha.inventory = mtgban.InventoryRecord{}
	ha.buylist = mtgban.BuylistRecord{}
	ha.maxConcurrency = defaultConcurrency
	ha.client = mtgban.NewHTTPClient()
	return &ha
}

type responseChan struct {
	cardID   string
	invEntry *mtgban.InventoryEntry
	buyEntry *mtgban.BuylistEntry

	// duplicate marks a listing the catalog does not tell apart from another
	// this storefront sells, and which is kept only where that one is absent.
	duplicate bool
}

func (ha *Hareruya) printf(format string, a ...any) {
	if ha.logCallback != nil {
		ha.logCallback("[HA] "+format, a...)
	}
}

func (ha *Hareruya) processBuylistSet(ctx context.Context, channel chan<- responseChan, cardSet string) error {
	var i int
	// The search orders by price with nothing to break a tie, so a listing
	// priced the same as the ones around a page boundary is answered on both
	// pages. Remember what the set has already given, and read each once.
	seen := map[string]bool{}
	for {
		i++
		results, canExit, err := ha.processBuylistPage(ctx, channel, cardSet, i, seen)
		if err != nil {
			return err
		}

		if len(results) == 0 {
			return nil
		}

		ha.printf("cardSet %s page %d found %d results", cardSet, i, len(results))

		for _, result := range results {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case channel <- result:
				// done
			}
		}

		if canExit {
			return nil
		}
	}
}

func (ha *Hareruya) processBuylistPage(ctx context.Context, channel chan<- responseChan, cardSet string, page int, seen map[string]bool) ([]responseChan, bool, error) {
	var canExit bool

	v := url.Values{}
	v.Set("sort", "price")
	v.Set("order", "DESC")
	v.Set("cardId", "")
	v.Set("product", "")
	v.Set("category", "")
	v.Set("cardset", cardSet)
	v.Set("colorsType", "0")
	v.Add("rarity[]", "1")
	v.Add("rarity[]", "2")
	v.Add("rarity[]", "3")
	v.Add("rarity[]", "4")
	v.Set("cardtypesType", "0")
	v.Set("subtype", "")
	v.Set("format", "")
	v.Set("illustrator", "")
	v.Add("foilFlg[]", "0")
	v.Add("foilFlg[]", "1")
	v.Set("language[]", "2") // English only
	v.Set("page", fmt.Sprint(page))

	link := buylistURL + v.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, http.NoBody)
	if err != nil {
		return nil, canExit, err
	}
	resp, err := ha.client.Do(req)
	if err != nil {
		return nil, canExit, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, canExit, err
	}

	var results []responseChan
	doc.Find(`.itemList`).EachWithBreak(func(i int, s *goquery.Selection) bool {
		link, _ := s.Find(`div.itemData a`).Attr("href")
		link = strings.TrimSpace(link)
		id := strings.Split(path.Base(link), "?")[0]

		title := s.Find(`div.itemData a`).Text()
		title = strings.TrimSpace(title)

		priceStr := s.Find(`p.itemDetail__price`).Text()
		priceStr = strings.TrimPrefix(priceStr, "¥ ")
		price, err := matcher.ParsePrice(priceStr)
		if err != nil {
			ha.printf("page %d entry %d: %s", page, i, err.Error())
			return false
		}
		price *= ha.exchangeRate

		// Since we're sorting by price, as soon as we found an item that is not
		// being bought we can assume there are no more items in this set
		stock := s.Find(`span.itemUserAct__number__title`).Text()
		if stock != "個数" {
			canExit = true
			return false
		}

		// After the check above, never before it: a repeat that returned
		// early would swallow the signal that ends the set.
		if seen[id] {
			return true
		}
		seen[id] = true

		theCard, err := preprocess(ha.backend, title)
		if err != nil {
			return true
		}

		cardID, err := ha.backend.Match(theCard)
		if errors.Is(err, matcher.ErrUnsupported) {
			return true
		} else if err != nil {
			if magic.IsBasicLand(theCard.Name) {
				return true
			}
			ha.printf("%v in cardSet %s at page %d", err, cardSet, page)
			ha.printf("%q", theCard)
			ha.printf("%s", title)

			var alias *matcher.AliasingError
			if errors.As(err, &alias) {
				probes := alias.Probe()
				for _, probe := range probes {
					card, _ := ha.backend.GetUUID(probe)
					ha.printf("- %s", card)
				}
			}
			return true
		}

		// The finish a listing states is read from the group before the card
		// name, and where it states none the plain card is meant. A printing
		// sold only in foil has no such card, and answering with the foil
		// prices it off a copy that was never made. A treatment stated in
		// that group is a foil by another name and says so itself.
		co, cerr := ha.backend.GetUUID(cardID)
		if cerr == nil && co.Foil && !theCard.Foil &&
			!reThick.MatchString(strings.TrimPrefix(title, "【EN】")) {
			return true
		}
		// The converse: a foil listing on a printing with no foil is priced
		// off the plain copy beside it.
		if cerr == nil && theCard.Foil && !co.Foil && !co.Etched {
			return true
		}

		var priceRatio, sellPrice float64

		invCards := ha.inventory[cardID]
		for _, invCard := range invCards {
			sellPrice = invCard.Price
			break
		}
		if sellPrice > 0 {
			priceRatio = price / sellPrice * 100
		}

		duplicate := strings.Contains(title, setBooster) || strings.Contains(title, secretLairDeck)
		class := cartClass(s)

		deductions := []float64{1, 0.8, 0.5}
		for i, deduction := range deductions {
			out := responseChan{
				cardID:    cardID,
				duplicate: duplicate,
				buyEntry: &mtgban.BuylistEntry{
					Conditions: mtgban.DefaultGradeTags[i],
					BuyPrice:   price * deduction,
					PriceRatio: priceRatio,
					URL:        "https://www.hareruyamtg.com" + link,
					OriginalID: id,
					InstanceID: class,
				},
			}

			results = append(results, out)
		}

		// Continue to next element
		return true
	})

	return results, canExit, nil
}

// colorFilters narrow a set's search to each color it files listings under:
// white, blue, black, red, green, and colorless. A multicolored card is
// filed under each of its colors, and a card of no color (a conspiracy, a
// double token) under none.
var colorFilters = []string{"&fq.color=1", "&fq.color=2", "&fq.color=3", "&fq.color=4", "&fq.color=5", "&fq.color=6"}

// foilFilters narrow a set's search to each foil flag, which every listing
// carries, a card of no color included.
var foilFilters = []string{"&fq.foil_flg=0", "&fq.foil_flg=1"}

// processSet reads a set whole. A set holding more listings than the search
// pages through is read again once per color, then once per foil flag for
// what the colors left out, passing over the listings already sent. The
// listings still unread are counted in the log.
func (ha *Hareruya) processSet(ctx context.Context, channel chan<- responseChan, cardSet string) error {
	sent := map[string]bool{}
	err := ha.walkSet(ctx, channel, cardSet, "", sent)
	var capErr *SearchCapError
	if !errors.As(err, &capErr) {
		return err
	}
	numFound := capErr.NumFound

	ha.printf("Set %s is past the search cap, reading it by color", cardSet)
	err = ha.walkFilters(ctx, channel, cardSet, colorFilters, sent)
	if err != nil {
		return err
	}
	if len(sent) < numFound {
		ha.printf("Set %s: %d listings past the cap are still unread after the colors, reading it by foil flag", cardSet, numFound-len(sent))
		err = ha.walkFilters(ctx, channel, cardSet, foilFilters, sent)
		if err != nil {
			return err
		}
	}
	if len(sent) < numFound {
		ha.printf("Set %s: %d of its %d listings stay past the cap", cardSet, numFound-len(sent), numFound)
	}
	return nil
}

// walkFilters walks a set once per filter. A narrowed search past the cap
// keeps what it served, and what it could not is left to the caller's count.
func (ha *Hareruya) walkFilters(ctx context.Context, channel chan<- responseChan, cardSet string, filters []string, sent map[string]bool) error {
	for _, filter := range filters {
		err := ha.walkSet(ctx, channel, cardSet, filter, sent)
		var capErr *SearchCapError
		if err != nil && !errors.As(err, &capErr) {
			return err
		}
	}
	return nil
}

// unsent returns the products not yet sent, and records them as sent.
func unsent(products []Product, sent map[string]bool) []Product {
	var fresh []Product
	for _, product := range products {
		key := product.Product + "/" + product.ProductClass
		if sent[key] {
			continue
		}
		sent[key] = true
		fresh = append(fresh, product)
	}
	return fresh
}

// walkSet sends every page of a set, narrowed by a search filter unless it
// is empty, leaving out the listings already in sent.
func (ha *Hareruya) walkSet(ctx context.Context, channel chan<- responseChan, cardSet, filter string, sent map[string]bool) error {
	var i int
	for {
		i++
		listings, err := SearchCardSetFiltered(ctx, ha.client, cardSet, filter, i)
		if err != nil {
			return err
		}

		if len(listings) == 0 {
			return nil
		}

		products := unsent(listings, sent)
		if len(products) == 0 {
			continue
		}

		lazyData, err := ha.getLazy(ctx, products, 0)
		if err != nil {
			return err
		}

		if len(lazyData) != len(products) {
			return fmt.Errorf("products %d vs lazyData %d", len(products), len(lazyData))
		}

		for _, product := range products {
			theCard, err := Preprocess(ha.backend, product)
			if err != nil {
				continue
			}

			cardID, err := ha.backend.Match(theCard)
			if errors.Is(err, matcher.ErrUnsupported) {
				continue
			} else if err != nil {
				// Skip errors from lands, "misc" promos, tokens, emblem
				// tokens (the shelf's own "紋章(<planeswalker>)" wording,
				// not the bare word real cards carry too), and the insert
				// cards a set ships instructions or a helper on (補助カード).
				if magic.IsBasicLand(theCard.Name) ||
					strings.Contains(theCard.Edition, "The List") || // lots at set 280
					strings.Contains(theCard.Edition, "Mystery Booster") || // lots at set 280
					strings.Contains(product.ProductName, "Token") ||
					strings.Contains(product.ProductName, "トークン") ||
					strings.Contains(product.ProductName, "エンブレム") ||
					strings.Contains(product.ProductName, "紋章(") ||
					strings.Contains(product.ProductName, "紋章（") ||
					strings.Contains(product.ProductName, "補助カード") {
					continue
				}
				ha.printf("%v at set %s (page %d)", err, cardSet, i)
				ha.printf("%q", theCard)
				ha.printf("%v", product)

				var alias *matcher.AliasingError
				if errors.As(err, &alias) {
					probes := alias.Probe()
					for _, probe := range probes {
						card, _ := ha.backend.GetUUID(probe)
						ha.printf("- %s", card)
					}
				}
				continue
			}

			for _, row := range listingsFor(product, lazyData) {
				cond := row.Condition
				price := row.Price * ha.exchangeRate
				qty := row.Quantity

				link := "https://www.hareruyamtg.com/en/products/detail/" + product.Product + "?lang=EN&class=" + row.Class
				out := responseChan{
					cardID: cardID,
					invEntry: &mtgban.InventoryEntry{
						Price:      price,
						Conditions: cond,
						Quantity:   qty,
						URL:        link,
						OriginalID: product.Product,
						InstanceID: row.Class,
						SellerName: availableMarketNames[0],
					},
				}
				if row.Graded {
					out.invEntry.SellerName = availableMarketNames[1]
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				case channel <- out:
					// done
				}
			}
		}
	}
}

// listingsFor gathers the storefront's rows for one lot of a product. The
// lazy endpoint answers a block per lot, and a product sold in several
// conditions has several lots, so pairing on the product alone gives every
// lot of it every other lot's rows.
func listingsFor(product Product, lazyData []LazyResult) []Row {
	// A block leaves its lot unnamed where the product has one lot, and also
	// for the plain lot beside a slab's; it answers only a lot no block names.
	var own, unnamed []Row
	var named bool
	for _, lazy := range lazyData {
		if lazy.ProductID != product.Product {
			continue
		}
		switch lazy.ProductClass {
		case product.ProductClass:
			named = true
			own = append(own, lazy.Rows...)
		case "":
			unnamed = append(unnamed, lazy.Rows...)
		}
	}
	rows := unnamed
	if named {
		rows = own
	}
	// A lot's own row whose cart button is missing adds the lot itself.
	for i := range rows {
		if rows[i].Main && rows[i].Class == "" {
			rows[i].Class = product.ProductClass
		}
	}
	return rows
}

// Row is one line of the storefront's table, before it becomes a listing.
type Row struct {
	Quantity  int
	Condition mtgban.Condition
	Price     float64
	// Graded marks a copy in a grading service's slab.
	Graded bool
	// Class is the product class the row's cart button adds: each
	// condition of a lot has its own.
	Class string
	// Main marks the lot's own row, rather than an extra condition row.
	Main bool
}

// cartClass is the product class a listing's own cart button adds, the one
// beside its main row rather than an extra condition row's, for the store
// and the buylist alike.
func cartClass(item *goquery.Selection) string {
	return item.Find(".itemUserAct__cart [data-productclass]").AttrOr("data-productclass", "")
}

// haCondition maps Hareruya's condition text to our grade, and reports
// whether it names a graded slab, which grades by its score.
func haCondition(s string) (mtgban.Condition, bool, error) {
	match := reSlab.FindStringSubmatch(s)
	if match == nil {
		grade, err := mtgban.ParseCondition(s)
		return grade, false, err
	}
	grade, found := mtgban.SlabCondition(match[1], match[2])
	if !found {
		return "", true, fmt.Errorf("%w: %q", mtgban.ErrInvalidCondition, s)
	}
	return grade, true, nil
}

// LazyResult carries a page of rows and the error that ended the walk, so a
// caller sees both what arrived and why it stopped.
type LazyResult struct {
	ProductID string
	// ProductClass names which lot of the product the block holds. The
	// storefront spells it only where a product has more than one, so an
	// empty one means the product's only lot.
	ProductClass string
	ProductName  string
	Rows         []Row
}

func (ha *Hareruya) getLazy(ctx context.Context, products []Product, attempt int) ([]LazyResult, error) {
	attempt++
	link := "https://www.hareruyamtg.com/en/products/search/unisearch/lazy"

	v := url.Values{}
	for i, product := range products {
		prefix := "docs[" + fmt.Sprint(i) + "]"
		v.Set(prefix+"[product]", product.Product)
		v.Set(prefix+"[product_name]", product.ProductName)
		v.Set(prefix+"[product_name_en]", product.ProductNameEN)
		v.Set(prefix+"[card_name]", product.CardName)
		v.Set(prefix+"[language]", product.Language)
		v.Set(prefix+"[price]", product.Price)
		v.Set(prefix+"[image_url]", product.ImageURL)
		v.Set(prefix+"[foil_flg]", product.FoilFlag)
		v.Set(prefix+"[stock]", product.Stock)
		v.Set(prefix+"[weekly_sales]", product.WeeklySales)
		v.Set(prefix+"[product_class]", product.ProductClass)
		v.Set(prefix+"[card_condition]", product.CardCondition)
		v.Set(prefix+"[sale_flg]", product.SaleFlag)
		v.Set(prefix+"[high_price_code]", product.HighPriceCode)
	}
	v.Set("css", "itemList")
	payload := v.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, link, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := ha.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// The system stops returning results after querying ~60 consecutive results
	// So sleep for 5 minutes, and try again for 5 times
	if resp.StatusCode == 403 {
		if attempt > 5 {
			return nil, errors.New("timeout exceeded")
		}
		ha.printf("lazy request returned forbidden, trying again in a bit (%d)", attempt)
		resp.Body.Close()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Minute):
		}
		return ha.getLazy(ctx, products, attempt)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	return ha.lazyResults(doc), nil
}

// lazyResults reads the lazy endpoint's answer, a block per lot.
func (ha *Hareruya) lazyResults(doc *goquery.Document) []LazyResult {
	var out []LazyResult

	doc.Find(".itemList").Each(func(i int, s *goquery.Selection) {
		name := strings.TrimSpace(s.Find(".itemDataWrapper .itemData a.itemName").Text())
		link, _ := s.Find(".itemDataWrapper .itemData a.itemName").Attr("href")
		id, query, _ := strings.Cut(path.Base(strings.TrimSpace(link)), "?")
		values, _ := url.ParseQuery(query)

		var result LazyResult
		result.ProductID = id
		result.ProductClass = values.Get("class")
		result.ProductName = name

		// A block with early exits rather than a loop: each guard below leaves
		// the listing unpriced and carries on with the rest of the page.
		for {
			priceStr := strings.TrimSpace(s.Find(".itemDataWrapper .itemDetail__price").Text())
			price, err := matcher.ParsePrice(strings.TrimPrefix(priceStr, "¥ "))
			if err != nil {
				break
			}

			line := strings.TrimSpace(s.Find(".itemDataWrapper .itemDetail__stock").Text())
			fields := strings.Split(strings.Trim(line, "【】"), ":")
			if len(fields) != 2 {
				break
			}

			condition := strings.TrimRight(strings.TrimSuffix(fields[0], " Stock"), "-+")
			if reAltered.MatchString(condition) {
				break
			}

			qty, err := strconv.Atoi(fields[1])
			if err != nil || qty == 0 {
				break
			}

			grade, graded, err := haCondition(condition)
			if err != nil {
				ha.printf("unsupported %s condition", condition)
				break
			}

			result.Rows = append(result.Rows, Row{
				Price:     price,
				Condition: grade,
				Quantity:  qty,
				Graded:    graded,
				Class:     cartClass(s),
				Main:      true,
			})
			//lint:ignore SA4004 the single iteration is the point
			break
		}

		s.Find(".row.not-first.ng-star-inserted").Each(func(j int, se *goquery.Selection) {
			qtyStr := strings.TrimSpace(se.Find(`div[class="col-xs-2"]`).Text())
			qty, err := strconv.Atoi(qtyStr)
			if err != nil || qty == 0 {
				return
			}

			priceStr := strings.TrimSpace(se.Find(".col-xs-4").Text())
			price, err := matcher.ParsePrice(strings.TrimPrefix(priceStr, "¥ "))
			if err != nil {
				return
			}

			condition := strings.TrimSpace(se.Find("strong").Text())
			if reAltered.MatchString(condition) {
				return
			}

			grade, graded, err := haCondition(condition)
			if err != nil {
				ha.printf("unsupported %s condition", condition)
				return
			}

			result.Rows = append(result.Rows, Row{
				Price:     price,
				Condition: grade,
				Quantity:  qty,
				Graded:    graded,
				Class:     se.Find(".addCart[data-productclass]").AttrOr("data-productclass", ""),
			})
		})

		out = append(out, result)
	})

	return out
}

func (ha *Hareruya) getCardSets(ctx context.Context) ([]string, error) {
	link := "https://www.hareruyamtg.com/en/products/search"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := ha.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var results []string
	doc.Find("select#front_product_search_cardset option").Each(func(i int, s *goquery.Selection) {
		value, _ := s.Attr("value")
		if value == "" {
			return
		}
		results = append(results, value)
	})

	// Sort to keep results predictable and find min/max
	slices.SortFunc(results, func(a, b string) int {
		if len(a) > len(b) {
			return 1
		}
		if len(a) < len(b) {
			return -1
		}
		return strings.Compare(a, b)
	})

	return results, nil
}

func (ha *Hareruya) scrape(ctx context.Context, mode string) error {
	rate, err := mtgban.GetExchangeRate(ctx, "JPY")
	if err != nil {
		return err
	}
	ha.exchangeRate = rate
	ha.printf("Received JPY rate of %f", rate)

	cardSets, err := ha.getCardSets(ctx)
	if err != nil {
		return err
	}
	ha.printf("Found %d card sets", len(cardSets))

	// Pre-filter items if a target edition is set
	items := cardSets
	if ha.targetEdition != "" {
		items = nil
		for _, cs := range cardSets {
			if cs == ha.targetEdition {
				items = append(items, cs)
			}
		}
	}

	var consume func(responseChan)
	if mode == modeInventory {
		consume = func(record responseChan) {
			err := ha.inventory.Add(record.cardID, record.invEntry)
			if err != nil {
				ha.printf("%s", err.Error())
			}
		}
	} else if mode == modeBuylist {
		// The listing holding an id whose copy the catalog does not tell apart,
		// keyed by that id. A duplicate is kept while it is the only thing sold
		// under the id, and gives way as soon as the listing it duplicates
		// arrives - which is either side of it, since the pages are read at
		// once. Its own remaining grades are told from that listing by the id
		// the storefront gives them.
		heldBy := map[string]string{}

		consume = func(record responseChan) {
			held, isHeld := heldBy[record.cardID]
			switch {
			case record.duplicate:
				if isHeld && held != record.buyEntry.OriginalID {
					return
				}
				if !isHeld {
					if _, found := ha.buylist[record.cardID]; found {
						return
					}
					heldBy[record.cardID] = record.buyEntry.OriginalID
				}
			case isHeld:
				delete(ha.buylist, record.cardID)
				delete(heldBy, record.cardID)
			}

			co, _ := ha.backend.GetUUID(record.cardID)
			// This store tracks the two different EU/US printings as separate entries
			var err error
			if co.SetCode == "MPS" {
				err = ha.buylist.AddRelaxed(record.cardID, record.buyEntry)
			} else {
				err = ha.buylist.Add(record.cardID, record.buyEntry)
			}
			if err != nil {
				ha.printf("%s", err.Error())
			}
		}
	}

	mtgban.WorkerPool(ctx, ha.maxConcurrency, items,
		func(ctx context.Context, cardSet string, results chan<- responseChan) error {
			ha.printf("Processing card set %s", cardSet)
			if mode == modeInventory {
				return ha.processSet(ctx, results, cardSet)
			} else if mode == modeBuylist {
				return ha.processBuylistSet(ctx, results, cardSet)
			}
			return nil
		},
		consume,
		ha.printf,
	)

	if mode == modeInventory {
		ha.inventoryDate = time.Now()
	} else if mode == modeBuylist {
		ha.buylistDate = time.Now()
	}

	return nil
}

// SetConfig applies options after the scraper was built. See
// mtgban.ScraperConfig.
func (ha *Hareruya) SetConfig(opt mtgban.ScraperOptions) {
	ha.disableRetail = opt.DisableRetail
	ha.disableBuylist = opt.DisableBuylist
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (ha *Hareruya) Load(ctx context.Context) error {
	var errs []error

	if !ha.disableRetail {
		err := ha.scrape(ctx, modeInventory)
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %w", mtgban.ErrInventoryLoad, err))
		}
	}

	if !ha.disableBuylist {
		err := ha.scrape(ctx, modeBuylist)
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %w", mtgban.ErrBuylistLoad, err))
		}
	}

	return errors.Join(errs...)
}

// Inventory returns what Load collected. See mtgban.Seller.
func (ha *Hareruya) Inventory() mtgban.InventoryRecord {
	return ha.inventory
}

// Buylist returns what Load collected. See mtgban.Vendor.
func (ha *Hareruya) Buylist() mtgban.BuylistRecord {
	return ha.buylist
}

// availableMarketNames are the sub-sellers: the storefront's copies, and
// those it sells in a grading service's slab.
var availableMarketNames = []string{
	"Hareruya",
	"Hareruya Graded",
}

var name2shorthand = map[string]string{
	"Hareruya":        "HA",
	"Hareruya Graded": "HAGraded",
}

// MarketNames names the sub-sellers this market splits into. See
// mtgban.Market.
func (ha *Hareruya) MarketNames() []string {
	return availableMarketNames
}

// InfoForScraper describes one of the sub-scrapers named above.
func (ha *Hareruya) InfoForScraper(name string) mtgban.ScraperInfo {
	info := ha.Info()
	info.Name = name
	info.Shorthand = name2shorthand[name]
	return info
}

// Info describes this scraper. See mtgban.Scraper.
func (ha *Hareruya) Info() (info mtgban.ScraperInfo) {
	info.Name = "Hareruya"
	info.Shorthand = "HA"
	info.CountryFlag = "JP"
	info.InventoryTimestamp = &ha.inventoryDate
	info.BuylistTimestamp = &ha.buylistDate
	info.Game = ha.backend.Game
	return
}
