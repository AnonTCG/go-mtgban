// Package toamagic scrapes Tales of Adventure.
package toamagic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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

	inventoryURL = "https://www.toamagic.com/catalog/magic_singles/8?layout=false"
)

// TOAMagic prices Tales of Adventure's stock.
type TOAMagic struct {
	logCallback    mtgban.LogCallbackFunc
	maxConcurrency int

	backend *matcher.Backend

	inventoryDate time.Time
	inventory     mtgban.InventoryRecord

	client *http.Client
}

// NewScraper returns a scraper matching against b.
func NewScraper(b *matcher.Backend) *TOAMagic {
	toa := TOAMagic{backend: b}
	toa.inventory = mtgban.InventoryRecord{}
	toa.maxConcurrency = defaultConcurrency
	toa.client = mtgban.NewHTTPClient()
	return &toa
}

type responseChan struct {
	cardID   string
	invEntry *mtgban.InventoryEntry
}

func (toa *TOAMagic) printf(format string, a ...any) {
	if toa.logCallback != nil {
		toa.logCallback("[TOA] "+format, a...)
	}
}

// offer is one in-stock variant of a product.
type offer struct {
	conditions mtgban.Condition
	price      float64
	qty        int
}

// inventoryOffers reads every in-stock variant of the product holding meta
// from its detail rows; the grid block shows only the most expensive one.
func (toa *TOAMagic) inventoryOffers(meta *goquery.Selection) []offer {
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

		cond := strings.TrimPrefix(fields[0], "Website Exclusive ")
		if cond == "Graded" {
			return
		}
		grade, err := mtgban.ParseCondition(cond)
		if err != nil {
			toa.printf("unsupported %s condition", cond)
			return
		}

		offers = append(offers, offer{conditions: grade, price: price, qty: qty})
	})
	return offers
}

func (toa *TOAMagic) processProduct(ctx context.Context, channel chan<- responseChan, productPath string) error {
	link := "https://www.toamagic.com" + productPath + "?layout=false&filter_by_stock=in-stock"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := toa.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return err
	}

	edition := doc.Find(`h1[class="page-title"]`).Text()
	edition = strings.TrimSuffix(edition, " Art Variants")

	// Inventory mode needs to might need to discover additional elements
	if strings.HasSuffix(edition, "Block") ||
		strings.HasSuffix(edition, "Sets") ||
		strings.HasSuffix(edition, "Editions") ||
		strings.HasSuffix(edition, "Edition") ||
		strings.HasSuffix(edition, "Decks") ||
		strings.HasSuffix(edition, "From the Vault") ||
		strings.HasSuffix(edition, "Collector Booster Era") ||
		strings.HasSuffix(edition, "Promos") {
		var links []string
		doc.Find(`a[class="clearfix"]`).Each(func(_ int, s *goquery.Selection) {
			link, _ := s.Attr("href")
			links = append(links, link)
		})

		var failed int
		for _, link := range links {
			err := toa.processProduct(ctx, channel, link)
			if err != nil {
				failed++
				toa.printf("%s", err.Error())
			}
		}
		if failed > 0 {
			return fmt.Errorf("%d of %d subcategories of %q failed", failed, len(links), edition)
		}
		return nil
	}

	xpath := `ul[class="products"] li[class="product"] div[class="inner"] div[class="meta"]`
	doc.Find(xpath).Each(func(_ int, s *goquery.Selection) {
		link, _ := s.Find("a").Attr("href")

		title := strings.TrimSpace(s.Find("h4").Text())
		fields := strings.Split(title, " - ")
		variant := ""
		cardName := fields[0]
		if len(fields) > 1 {
			variant = strings.Join(fields[1:], " ")
		}

		offers := toa.inventoryOffers(s)
		if len(offers) == 0 {
			return
		}

		theCard, err := preprocess(cardName, edition, variant)
		if err != nil {
			return
		}

		cardID, err := toa.backend.Match(theCard)
		if errors.Is(err, matcher.ErrUnsupported) {
			return
		} else if err != nil {
			// Skip reporting an error for known failures (invalid variant number)
			switch edition {
			case "Homelands",
				"Fallen Empires":
				return
			case "Magic 2010 M10",
				"Mirage",
				"Portal",
				"Portal Second Age",
				"Tempest":
				if magic.IsBasicLand(cardName) {
					return
				}
			}

			toa.printf("%v", err)
			toa.printf("%q", theCard)
			toa.printf("%s ~ %s ~ %s", cardName, edition, variant)

			var alias *matcher.AliasingError
			if errors.As(err, &alias) {
				probes := alias.Probe()
				for _, probe := range probes {
					card, _ := toa.backend.GetUUID(probe)
					toa.printf("- %s", card)
				}
			}
			return
		}

		for _, o := range offers {
			out := responseChan{
				cardID: cardID,
				invEntry: &mtgban.InventoryEntry{
					Price:      o.price,
					Conditions: o.conditions,
					Quantity:   o.qty,
					URL:        "https://www.toamagic.com" + link,
				},
			}
			channel <- out
		}
	})

	// Search for the next page, if not found we processed them all
	next, found := doc.Find(`a[class="next_page"]`).Attr("href")
	if !found {
		return nil
	}

	return toa.processProduct(ctx, channel, next)
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (toa *TOAMagic) Load(ctx context.Context) error {
	link := inventoryURL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := toa.client.Do(req)
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
	xpath := `ul[class="parent-category list small-12 column eat-both across-3  "] li`
	doc.Find(xpath).Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Find("a").Attr("href")
		links = append(links, href)
		title := strings.TrimSpace(s.Find(`div[class="name"]`).Text())
		titles = append(titles, title)
	})

	toa.printf("Found %d categories", len(links))

	type item struct {
		link  string
		title string
	}
	items := make([]item, len(links))
	for i := range links {
		items[i] = item{links[i], titles[i]}
	}

	mtgban.WorkerPool(ctx, toa.maxConcurrency, items,
		func(ctx context.Context, it item, results chan<- responseChan) error {
			toa.printf("Processing %s", it.title)
			return toa.processProduct(ctx, results, it.link)
		},
		func(record responseChan) {
			if record.invEntry != nil {
				err := toa.inventory.AddRelaxed(record.cardID, record.invEntry)
				if err != nil {
					toa.printf("%s", err.Error())
				}
			}
		},
		toa.printf,
	)

	toa.inventoryDate = time.Now()

	return nil
}

// Inventory returns what Load collected. See mtgban.Seller.
func (toa *TOAMagic) Inventory() mtgban.InventoryRecord {
	return toa.inventory
}

// Info describes this scraper. See mtgban.Scraper.
func (toa *TOAMagic) Info() (info mtgban.ScraperInfo) {
	info.Name = "Tales of Adventure"
	info.Shorthand = "TOA"
	info.InventoryTimestamp = &toa.inventoryDate
	info.Game = toa.backend.Game
	return
}
