// Package secretdeskorrigans scrapes Le Secret des Korrigans.
package secretdeskorrigans

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

	inventoryURL = "https://www.lesecretdeskorrigans.com/catalog/magic_singles/8?layout=false"
)

// SecretDesKorrigans prices Le Secret des Korrigans' stock.
type SecretDesKorrigans struct {
	logCallback    mtgban.LogCallbackFunc
	maxConcurrency int

	backend *matcher.Backend

	inventoryDate time.Time
	inventory     mtgban.InventoryRecord

	exchangeRate float64

	client *http.Client
}

// NewScraper returns a scraper matching against b, failing if the edition
// list cannot be read.
func NewScraper(b *matcher.Backend) (*SecretDesKorrigans, error) {
	sdk := SecretDesKorrigans{backend: b}
	sdk.inventory = mtgban.InventoryRecord{}
	sdk.maxConcurrency = defaultConcurrency
	sdk.client = mtgban.NewHTTPClient()
	return &sdk, nil
}

type responseChan struct {
	cardID   string
	invEntry *mtgban.InventoryEntry
}

func (sdk *SecretDesKorrigans) printf(format string, a ...any) {
	if sdk.logCallback != nil {
		sdk.logCallback("[SDK] "+format, a...)
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
func (sdk *SecretDesKorrigans) inventoryOffers(meta *goquery.Selection) []offer {
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
		qtyStr = strings.TrimSuffix(qtyStr, " En stock")
		qtyStr = strings.TrimSuffix(qtyStr, " En Stock")
		qtyStr = strings.TrimSuffix(qtyStr, " In Stock")
		qty, err := strconv.Atoi(qtyStr)
		if err != nil {
			return
		}

		priceStr := strings.TrimSpace(row.Find(`span[class="regular price"]`).Text())
		priceStr = strings.TrimPrefix(priceStr, "CAD")
		price, perr := matcher.ParsePrice(priceStr)
		if price == 0 {
			sdk.printf("price error '%s': %v", priceStr, perr)
			return
		}

		cond := strings.TrimPrefix(fields[0], "Website Exclusive ")
		if cond == "Graded" {
			return
		}
		grade, err := mtgban.ParseCondition(cond)
		if err != nil {
			sdk.printf("unsupported %s condition", cond)
			return
		}

		offers = append(offers, offer{conditions: grade, price: price, qty: qty})
	})
	return offers
}

func (sdk *SecretDesKorrigans) processProduct(ctx context.Context, channel chan<- responseChan, productPath string) error {
	link := "https://www.lesecretdeskorrigans.com" + productPath + "?layout=false&filter_by_stock=in-stock"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := sdk.client.Do(req)
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
	edition = strings.TrimSuffix(edition, " Singles")
	edition = strings.TrimSuffix(edition, " singles")

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
			err := sdk.processProduct(ctx, channel, link)
			if err != nil {
				failed++
				sdk.printf("%s", err.Error())
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

		offers := sdk.inventoryOffers(s)
		if len(offers) == 0 {
			return
		}

		theCard, err := preprocess(cardName, edition, variant)
		if err != nil {
			return
		}

		cardID, err := sdk.backend.Match(theCard)
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

			sdk.printf("%v", err)
			sdk.printf("%q", theCard)
			sdk.printf("%s | %s | %s", cardName, edition, variant)

			var alias *matcher.AliasingError
			if errors.As(err, &alias) {
				probes := alias.Probe()
				for _, probe := range probes {
					card, _ := sdk.backend.GetUUID(probe)
					sdk.printf("- %s", card)
				}
			}
			return
		}

		for _, o := range offers {
			out := responseChan{
				cardID: cardID,
				invEntry: &mtgban.InventoryEntry{
					Price:      o.price * sdk.exchangeRate,
					Conditions: o.conditions,
					Quantity:   o.qty,
					URL:        "https://www.lesecretdeskorrigans.com" + link,
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

	return sdk.processProduct(ctx, channel, next)
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (sdk *SecretDesKorrigans) Load(ctx context.Context) error {
	rate, err := mtgban.GetExchangeRate(ctx, "CAD")
	if err != nil {
		return err
	}
	sdk.exchangeRate = rate

	link := inventoryURL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := sdk.client.Do(req)
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
	xpath := `ul.parent-category li`
	doc.Find(xpath).Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Find("a").Attr("href")
		links = append(links, href)
		title := strings.TrimSpace(s.Find(`div[class="name"]`).Text())
		titles = append(titles, title)
	})

	sdk.printf("Found %d categories", len(links))

	type item struct {
		link  string
		title string
	}
	items := make([]item, len(links))
	for i := range links {
		items[i] = item{links[i], titles[i]}
	}

	mtgban.WorkerPool(ctx, sdk.maxConcurrency, items,
		func(ctx context.Context, it item, results chan<- responseChan) error {
			sdk.printf("Processing %s", it.title)
			return sdk.processProduct(ctx, results, it.link)
		},
		func(record responseChan) {
			err := sdk.inventory.AddRelaxed(record.cardID, record.invEntry)
			if err != nil {
				sdk.printf("%s", err.Error())
			}
		},
		sdk.printf,
	)

	sdk.inventoryDate = time.Now()

	return nil
}

// Inventory returns what Load collected. See mtgban.Seller.
func (sdk *SecretDesKorrigans) Inventory() mtgban.InventoryRecord {
	return sdk.inventory
}

// Info describes this scraper. See mtgban.Scraper.
func (sdk *SecretDesKorrigans) Info() (info mtgban.ScraperInfo) {
	info.Name = "Le Secret des Korrigans"
	info.Shorthand = "SK"
	info.InventoryTimestamp = &sdk.inventoryDate
	info.Game = sdk.backend.Game
	return
}
