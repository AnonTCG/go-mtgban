package cardkingdom

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/RomainMichau/cloudscraper_go/cloudscraper"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
)

const (
	gradedURL = "https://www.cardkingdom.com/mtg/graded-magic"
)

// Graded prices the cards Card Kingdom lists with a professional
// grade, which they sell apart from their ungraded stock.
type Graded struct {
	logCallback mtgban.LogCallbackFunc
	partner     string

	inventoryDate time.Time
	inventory     mtgban.InventoryRecord

	client  *cloudscraper.CloudScrapper
	backend *matcher.Backend
}

// NewScraperGraded returns a graded scraper.
func NewScraperGraded(b *matcher.Backend) (*Graded, error) {
	client, err := cloudscraper.Init(false, false)
	if err != nil {
		return nil, err
	}

	ck := Graded{}
	ck.inventory = mtgban.InventoryRecord{}
	ck.client = client
	ck.backend = b

	return &ck, nil
}

func (ck *Graded) printf(format string, a ...any) {
	if ck.logCallback != nil {
		ck.logCallback("[CKGraded] "+format, a...)
	}
}

func (ck *Graded) totalPages() (string, int, error) {
	// name_asc, not price_desc: a graded title carries the cert number, so
	// it sorts uniquely, while price ties have no stable order across page
	// requests and repeat or skip listings at page boundaries.
	cookieMap := map[string]string{
		"Cookie": "limit=100; sortBy=name_asc; viewType=listShowCart listShowDetails;",
	}
	res, err := ck.client.Get(gradedURL, cookieMap, http.MethodGet)
	if err != nil {
		return "", 0, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(res.Body))
	if err != nil {
		return "", 0, err
	}

	lastPageText := strings.TrimSpace(doc.Find("a[aria-label='Display Final Results Page']").Text())
	pages, err := strconv.Atoi(lastPageText)
	if err != nil {
		return "", 0, fmt.Errorf("could not find final page link: %w", err)
	}

	var session string
	for _, c := range res.Cookies {
		if c.Name == "laravel_session" {
			session = c.Value
		}
	}
	if session == "" {
		return "", 0, errors.New("could not find session cookie")
	}

	return session, pages, nil
}

func (ck *Graded) scrapePage(session string, page int) error {
	cookieMap := map[string]string{
		"Cookie": "limit=100; sortBy=name_asc; viewType=listShowCart listShowDetails; laravel_session=" + session + ";",
	}
	res, err := ck.client.Get(gradedURL+"?page="+fmt.Sprint(page), cookieMap, http.MethodGet)
	if err != nil {
		return err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(res.Body))
	if err != nil {
		return err
	}

	u, _ := url.Parse("https://www.cardkingdom.com/")
	if ck.partner != "" {
		q := u.Query()
		q.Set("partner", ck.partner)
		q.Set("utm_source", ck.partner)
		q.Set("utm_medium", "affiliate")
		q.Set("utm_campaign", ck.partner)
		u.RawQuery = q.Encode()
	}

	doc.Find(".productListWrapper.gradedMagic").Each(func(i int, s *goquery.Selection) {
		if s.Find("form.addToCartForm").HasClass("noInventory") {
			return
		}

		title := strings.TrimSpace(s.Find(".productTitle a").Text())

		// Product ID from hidden input
		id, _ := s.Find("input.product_id").Attr("value")

		theCard, err := preprocessGraded(title)
		if err != nil {
			if !errors.Is(err, matcher.ErrUnsupported) {
				ck.printf("%s: %v", title, err)
			}
			return
		}
		if scryfallID, found := gradedScryfallIDs[id]; found {
			theCard.ID = scryfallID
		}

		cardID, err := matchGraded(ck.backend, theCard)
		if errors.Is(err, matcher.ErrUnsupported) {
			return
		} else if err != nil {
			// A slab title naming several printings and no collector number
			// has nothing left to tell them apart.
			var alias *matcher.AliasingError
			aliased := errors.As(err, &alias)
			if aliased && matcher.ExtractNumber(theCard.Variation) == "" {
				return
			}
			ck.printf("%v", err)
			ck.printf("%q", theCard)
			ck.printf("%q", title)
			if aliased {
				for _, probe := range alias.Probe() {
					card, _ := ck.backend.GetUUID(probe)
					ck.printf("- %s", card)
				}
			}
			return
		}

		// Price
		priceStr := strings.TrimSpace(s.Find(".itemPrice").First().Text())
		price, err := matcher.ParsePrice(priceStr)
		if err != nil {
			ck.printf("%s", err.Error())
			return
		}

		// URL (relative href)
		linkPath, _ := s.Find(".productTitle a").Attr("href")
		u.Path = linkPath
		link := u.String()

		conditions, slab := parseGradedCondition(title)
		if conditions == "" {
			text := slab
			if text == "" {
				text = title
			}
			ck.printf("unsupported %s condition", text)
			return
		}

		out := &mtgban.InventoryEntry{
			Conditions: conditions,
			Price:      price,
			URL:        link,
			OriginalID: id,
		}
		err = ck.inventory.Add(cardID, out)
		if err != nil {
			ck.printf("page %d: %s", page, err.Error())
			return
		}
	})

	return nil
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (ck *Graded) Load(ctx context.Context) error {
	session, pages, err := ck.totalPages()
	if err != nil {
		return err
	}

	ck.printf("Found %d pages", pages)

	start := time.Now()

	for i := 1; i <= pages; i++ {
		ck.printf("Scraping page %d", i)
		err := ck.scrapePage(session, i)
		if err != nil {
			ck.printf("%s", err.Error())
		}
	}

	ck.printf("Took %v", time.Since(start))
	ck.inventoryDate = time.Now()

	return nil
}

// Inventory returns what Load collected. See mtgban.Seller.
func (ck *Graded) Inventory() mtgban.InventoryRecord {
	return ck.inventory
}

// Info describes this scraper. See mtgban.Scraper.
func (ck *Graded) Info() (info mtgban.ScraperInfo) {
	info.Name = "Card Kingdom Graded"
	info.Shorthand = "CKGraded"
	info.InventoryTimestamp = &ck.inventoryDate
	info.Game = ck.backend.Game
	return
}
