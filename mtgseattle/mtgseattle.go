// Package mtgseattle scrapes MTGSeattle.
package mtgseattle

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/matcher/magic"
	"github.com/mtgban/go-mtgban/mtgban"

	"github.com/PuerkitoBio/goquery"
	"github.com/hashicorp/go-retryablehttp"
)

const (
	// The site throttles concurrent traffic (see NewScraper's client
	// setup), so this stays low rather than at the other scrapers' 8.
	defaultConcurrency = 2

	baseURL      = "https://www.mtgseattle.com"
	inventoryURL = baseURL + "/catalog/magic_singles/8"
	buylistURL   = baseURL + "/buylist"

	modeInventory = "inventory"
	modeBuylist   = "buylist"
)

// MTGSeattle prices MTGSeattle's singles, both what they sell and what they
// buy.
type MTGSeattle struct {
	logCallback    mtgban.LogCallbackFunc
	maxConcurrency int

	backend *matcher.Backend

	inventoryDate time.Time
	buylistDate   time.Time

	inventory mtgban.InventoryRecord
	buylist   mtgban.BuylistRecord

	disableRetail  bool
	disableBuylist bool

	client *http.Client
}

// NewScraper returns a scraper matching against b.
func NewScraper(b *matcher.Backend) *MTGSeattle {
	ms := MTGSeattle{backend: b}
	ms.inventory = mtgban.InventoryRecord{}
	ms.buylist = mtgban.BuylistRecord{}
	ms.maxConcurrency = defaultConcurrency
	// The site appears to throttle rather than reject outright, so
	// retry slower and longer instead of giving up in seconds.
	ms.client = mtgban.NewHTTPClient(
		mtgban.WithHTTPBackoff(retryablehttp.RateLimitLinearJitterBackoff),
		mtgban.WithHTTPRetryWait(2*time.Second, 10*time.Second),
		mtgban.WithHTTPRetries(20),
		mtgban.WithHTTPErrorHandler(retryErrorHandler),
	)
	return &ms
}

// retryErrorHandler reports the last HTTP status once retries are
// exhausted; retryablehttp's own message omits it.
func retryErrorHandler(resp *http.Response, err error, numTries int) (*http.Response, error) {
	status := "no response"
	if resp != nil {
		status = resp.Status
		resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("giving up after %d attempt(s), last status %s: %w", numTries, status, err)
	}
	return nil, fmt.Errorf("giving up after %d attempt(s), last status %s", numTries, status)
}

type responseChan struct {
	cardID   string
	invEntry *mtgban.InventoryEntry
	buyEntry *mtgban.BuylistEntry
}

func (ms *MTGSeattle) printf(format string, a ...any) {
	if ms.logCallback != nil {
		ms.logCallback("[MS] "+format, a...)
	}
}

// buildProductURL sets layout=false on product, merging it into any
// query string the href already carries instead of appending a second "?".
// inStock lists only products in stock; a container's subcategories show
// either way.
func buildProductURL(product string, inStock bool) (string, error) {
	u, err := url.Parse(baseURL + product)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("layout", "false")
	if inStock {
		q.Set("filter_by_stock", "in-stock")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// offer is one in-stock variant of a product.
type offer struct {
	conditions mtgban.Condition
	price      float64
	qty        int
}

// inventoryOffers reads every in-stock variant of the product holding meta
// from its detail rows; the grid block shows only the most expensive one.
func (ms *MTGSeattle) inventoryOffers(meta *goquery.Selection) []offer {
	var offers []offer
	rows := meta.Closest(`div[class="inner"]`).Find(`div[class="variants"] div[class="variant-row row"]`)
	rows.Each(func(_ int, row *goquery.Selection) {
		condLang := row.Find(`span[class="variant-short-info variant-description"]`).Text()
		fields := strings.Split(condLang, ", ")
		if len(fields) > 1 && fields[1] != "English" {
			return
		}

		qtyStr := strings.TrimSpace(row.Find(`span[class="variant-short-info variant-qty"]`).Text())
		qtyStr = strings.TrimPrefix(qtyStr, "Limit ")
		qtyStr = strings.TrimSuffix(qtyStr, " In Stock")
		qty, err := strconv.Atoi(qtyStr)
		if err != nil {
			return
		}

		price, _ := matcher.ParsePrice(row.Find(`span[class="regular price"]`).Text())
		if price == 0 {
			return
		}

		cond := fields[0]
		if cond == "Graded" {
			return
		}
		grade, err := mtgban.ParseCondition(cond)
		if err != nil {
			ms.printf("unsupported %s condition", cond)
			return
		}

		// Adjust price for their discount when bought from the website
		offers = append(offers, offer{conditions: grade, price: price * 0.95, qty: qty})
	})
	return offers
}

// buylistOffers reads the NM English offer of a buylist product, if any.
func buylistOffers(meta *goquery.Selection) []offer {
	container := `span[class="variant-main-info small-12 medium-5 large-5 column eat-both"]`
	// Early exit to avoid catching sealed and similar
	condLang := meta.Find(container + ` span[class="variant-short-info variant-description"]`).Text()
	if condLang != "NM-Mint, English" {
		return nil
	}

	qtyStr := meta.Find(container + ` span[class="variant-short-info variant-qty"]`).Text()
	qtyStr = strings.TrimPrefix(qtyStr, "Limit ")
	qtyStr = strings.TrimSuffix(qtyStr, " In Stock")
	qty, err := strconv.Atoi(qtyStr)
	if err != nil {
		return nil
	}

	price, _ := matcher.ParsePrice(meta.Find(`div[class="product-price"] span[class="regular price"]`).Text())
	if price == 0 {
		return nil
	}
	return []offer{{conditions: mtgban.NM, price: price, qty: qty}}
}

func (ms *MTGSeattle) processProduct(ctx context.Context, channel chan<- responseChan, product, mode string) error {
	link, err := buildProductURL(product, mode == modeInventory)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := ms.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return err
	}

	edition := doc.Find(`h1[class="page-title"]`).Text()

	// Inventory mode needs to might need to discover additional elements
	if mode == modeInventory &&
		(strings.HasSuffix(edition, "Block") ||
			strings.HasSuffix(edition, "Sets") ||
			strings.HasSuffix(edition, "Editions") ||
			strings.HasSuffix(edition, "Edition") ||
			strings.HasSuffix(edition, "Decks") ||
			strings.HasSuffix(edition, "From the Vault") ||
			strings.HasSuffix(edition, "Promos")) {
		var links []string
		doc.Find(`a[class="clearfix"]`).Each(func(_ int, s *goquery.Selection) {
			link, _ := s.Attr("href")
			links = append(links, link)
		})

		var failed int
		for _, link := range links {
			err := ms.processProduct(ctx, channel, link, mode)
			if err != nil {
				failed++
				ms.printf("%s", err.Error())
			}
		}
		if failed > 0 {
			return fmt.Errorf("%d of %d subcategories of %q failed", failed, len(links), edition)
		}
		return nil
	}

	xpath := `ul[class="products"] li[class="product"] div[class="inner"] div[class="meta"]`
	if mode == modeBuylist {
		xpath = `ul[class="products"] li[class="product"] div[class="inner"] div[class="meta credit"]`
	}
	doc.Find(xpath).Each(func(_ int, s *goquery.Selection) {
		link, _ := s.Find("a").Attr("href")

		title := strings.TrimSpace(s.Find("h4").Text())
		fields := strings.Split(title, " - ")
		variant := ""
		cardName := fields[0]
		if len(fields) > 1 {
			variant = strings.Join(fields[1:], " ")
		}
		if strings.HasSuffix(cardName, "- Foil") {
			cardName = strings.TrimSuffix(cardName, "- Foil")
			variant = "Foil"
		}

		var offers []offer
		if mode == modeInventory {
			offers = ms.inventoryOffers(s)
		} else if mode == modeBuylist {
			offers = buylistOffers(s)
		}
		if len(offers) == 0 {
			return
		}

		theCard, err := preprocess(ms.backend, cardName, edition, variant)
		if err != nil {
			return
		}

		cardID, err := ms.backend.Match(theCard)
		if errors.Is(err, matcher.ErrUnsupported) {
			return
		} else if err != nil {
			// Skip reporting an error for known failures (invalid variant number)
			if magic.IsBasicLand(cardName) {
				switch edition {
				case "5th Edition",
					"Collectors Edition",
					"Ice Age",
					"International Collectors Edition",
					"Mirage",
					"Portal 1",
					"Portal Second Age",
					"Summer Magic (Edgar)",
					"Tempest":
					return
				}
			}
			switch edition {
			case "Homelands":
				return
			}

			ms.printf("%v", err)
			ms.printf("%q", theCard)
			ms.printf("%s ~ %s ~ %s", cardName, edition, variant)

			var alias *matcher.AliasingError
			if errors.As(err, &alias) {
				probes := alias.Probe()
				for _, probe := range probes {
					card, _ := ms.backend.GetUUID(probe)
					ms.printf("- %s", card)
				}
			}
			return
		}

		// Sanity check, a bunch of EA cards are market as foil when they
		// actually don't have a foil printing, just skip them
		if strings.Contains(title, "Foil - Extended Art") {
			co, err := ms.backend.GetUUID(cardID)
			if err != nil || !co.Foil {
				return
			}
		}

		if mode == modeInventory {
			for _, o := range offers {
				out := responseChan{
					cardID: cardID,
					invEntry: &mtgban.InventoryEntry{
						Price:      o.price,
						Conditions: o.conditions,
						Quantity:   o.qty,
						URL:        baseURL + link,
					},
				}
				channel <- out
			}
		} else if mode == modeBuylist {
			price, qty := offers[0].price, offers[0].qty
			var priceRatio, sellPrice float64

			invCards := ms.inventory[cardID]
			for _, invCard := range invCards {
				sellPrice = invCard.Price
				break
			}
			if sellPrice > 0 {
				priceRatio = price / sellPrice * 100
			}

			gradeMap := grading(ms.backend, cardID, price)
			for _, grade := range mtgban.DefaultGradeTags {
				var quantity int
				if grade == mtgban.NM {
					quantity = qty
				}

				factor := gradeMap[grade]
				out := responseChan{
					cardID: cardID,
					buyEntry: &mtgban.BuylistEntry{
						Conditions: grade,
						BuyPrice:   price * factor,
						PriceRatio: priceRatio,
						Quantity:   quantity,
						URL:        baseURL + link,
					},
				}
				channel <- out
			}
		}
	})

	// Search for the next page, if not found we processed them all
	next, found := doc.Find(`a[class="next_page"]`).Attr("href")
	if !found {
		return nil
	}

	return ms.processProduct(ctx, channel, next, mode)
}

func (ms *MTGSeattle) scrape(ctx context.Context, mode string) error {
	link := inventoryURL
	if mode == modeBuylist {
		link = buylistURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := ms.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return err
	}

	var links []string
	var titles []string
	xpath := `div[class="content inner clearfix"] ul[class="parent-category list small-12 columns eat-both fancy-row across-1  "] li a[class="clearfix"]`
	if mode == modeBuylist {
		xpath = `div[class="hidden-buylist-tree"] ul[id="category_tree"] li[class="depth_1"] ul[class="category_tree"] li a`
	}
	doc.Find(xpath).Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		links = append(links, href)
		titles = append(titles, strings.TrimSpace(s.Text()))
	})

	ms.printf("Found %d categories", len(links))

	type item struct {
		link  string
		title string
	}
	items := make([]item, len(links))
	for i := range links {
		items[i] = item{links[i], titles[i]}
	}

	mtgban.WorkerPool(ctx, ms.maxConcurrency, items,
		func(ctx context.Context, it item, results chan<- responseChan) error {
			ms.printf("Processing %s", it.title)
			return ms.processProduct(ctx, results, it.link, mode)
		},
		func(record responseChan) {
			var err error
			if record.invEntry != nil {
				err = ms.inventory.Add(record.cardID, record.invEntry)
			} else if record.buyEntry != nil {
				err = ms.buylist.Add(record.cardID, record.buyEntry)
			}
			if err != nil {
				ms.printf("%s", err.Error())
			}
		},
		ms.printf,
	)

	if mode == modeInventory {
		ms.inventoryDate = time.Now()
	} else if mode == modeBuylist {
		ms.buylistDate = time.Now()
	}

	return nil
}

// SetConfig applies options after the scraper was built. See
// mtgban.ScraperConfig.
func (ms *MTGSeattle) SetConfig(opt mtgban.ScraperOptions) {
	ms.disableRetail = opt.DisableRetail
	ms.disableBuylist = opt.DisableBuylist
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (ms *MTGSeattle) Load(ctx context.Context) error {
	var errs []error

	if !ms.disableRetail {
		err := ms.scrape(ctx, modeInventory)
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %w", mtgban.ErrInventoryLoad, err))
		}
	}

	if !ms.disableBuylist {
		err := ms.scrape(ctx, modeBuylist)
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %w", mtgban.ErrBuylistLoad, err))
		}
	}

	return errors.Join(errs...)
}

// Inventory returns what Load collected. See mtgban.Seller.
func (ms *MTGSeattle) Inventory() mtgban.InventoryRecord {
	return ms.inventory
}

// Buylist returns what Load collected. See mtgban.Vendor.
func (ms *MTGSeattle) Buylist() mtgban.BuylistRecord {
	return ms.buylist
}

func grading(b *matcher.Backend, cardID string, price float64) map[mtgban.Condition]float64 {
	co, err := b.GetUUID(cardID)
	if err != nil {
		return nil
	}

	if co.Foil {
		if price >= 50 {
			return map[mtgban.Condition]float64{
				mtgban.NM: 1, mtgban.SP: 0.8, mtgban.MP: 0.6, mtgban.HP: 0.4,
			}
		}
		if price >= 5 {
			return map[mtgban.Condition]float64{
				mtgban.NM: 1, mtgban.SP: 0.75, mtgban.MP: 0.5, mtgban.HP: 0.3,
			}
		}
		return map[mtgban.Condition]float64{
			mtgban.NM: 1, mtgban.SP: 0.7, mtgban.MP: 0.4, mtgban.HP: 0.25,
		}
	}

	switch co.SetCode {
	case "LEA", "LEB", "2ED":
		if price >= 50 {
			return map[mtgban.Condition]float64{
				mtgban.NM: 1, mtgban.SP: 0.8, mtgban.MP: 0.6, mtgban.HP: 0.4,
			}
		}
		if price >= 5 {
			return map[mtgban.Condition]float64{
				mtgban.NM: 1, mtgban.SP: 0.75, mtgban.MP: 0.55, mtgban.HP: 0.35,
			}
		}
		return map[mtgban.Condition]float64{
			mtgban.NM: 1, mtgban.SP: 0.7, mtgban.MP: 0.5, mtgban.HP: 0.3,
		}
	}

	if price >= 50 {
		return map[mtgban.Condition]float64{
			mtgban.NM: 1, mtgban.SP: 0.85, mtgban.MP: 0.75, mtgban.HP: 0.65,
		}
	}
	if price >= 5 {
		return map[mtgban.Condition]float64{
			mtgban.NM: 1, mtgban.SP: 0.80, mtgban.MP: 0.7, mtgban.HP: 0.6,
		}
	}
	return map[mtgban.Condition]float64{
		mtgban.NM: 1, mtgban.SP: 0.75, mtgban.MP: 0.6, mtgban.HP: 0.5,
	}
}

// Info describes this scraper. See mtgban.Scraper.
func (ms *MTGSeattle) Info() (info mtgban.ScraperInfo) {
	info.Name = "MTGSeattle"
	info.Shorthand = "MS"
	info.InventoryTimestamp = &ms.inventoryDate
	info.BuylistTimestamp = &ms.buylistDate
	info.CreditMultiplier = 1.33
	info.Game = ms.backend.Game
	return
}
