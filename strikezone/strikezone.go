// Package strikezone scrapes Strike Zone.
package strikezone

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
)

// The games this scraper covers, as the storefront names them.
const (
	GameMagic         = "Magic_the_Gathering"
	GameLorcana       = "Lorcana"
	GamePokemon       = "Pokemon_Tcg"
	GameYuGiOh        = "Yu_Gi_Oh"
	GameFleshAndBlood = "Flesh_and_Blood"
)

// szGames is what NewScraper is built through: it names the storefront
// category a game is filed under, and a game named nowhere here is not one
// Strike Zone is read for.
var szGames = map[matcher.Game]string{
	matcher.GameMagic:         GameMagic,
	matcher.GameLorcana:       GameLorcana,
	matcher.GamePokemon:       GamePokemon,
	matcher.GameYuGiOh:        GameYuGiOh,
	matcher.GameFleshAndBlood: GameFleshAndBlood,
}

// lorcanaNameTable resolves a bare name this storefront lists without its
// subtitle: Ursula's own row is the epithet-less "Ursula", which only the
// First Chapter's "Ursula - Power Hungry" answers.
var lorcanaNameTable = map[string]string{
	"Ursula": "Ursula - Power Hungry",
}

// lorcanaListing turns the storefront's own naming into the matcher's: a
// trailing " - Enchanted" carries the number that already disambiguates it,
// so dropping the suffix is enough, and a name lorcanaNameTable covers is
// resolved to the one the catalog files.
func lorcanaListing(cardName string) string {
	if name, found := strings.CutSuffix(cardName, " - Enchanted"); found {
		cardName = name
	}
	if name, found := lorcanaNameTable[cardName]; found {
		cardName = name
	}
	return cardName
}

const (
	defaultConcurrency = 8

	szInventoryURL = "http://shop.strikezoneonline.com/Category/%s_Singles.html"
	szBuylistURL   = "http://shop.strikezoneonline.com/BuyList/%s.html"

	modeRetail  = "retail"
	modeBuylist = "buylist"
)

// The pages the crawl must not enter: the re-sorted copies of every category,
// which would double its rows, and each game's sealed hubs, whose products
// are not singles. Gift_Sets and Collector_Tins hang off both the Pokemon
// and the Yu-Gi-Oh tree under one name.
var skipSuffixes = []string{
	"_ByTable.html",
	"_ByRarity.html",
	"_ByNumber.html",
	"Games.html",
	"Magic_Booster_Boxes.html",
	"Fat_Packs.html",
	"Gift_Sets_and_Secret_Lairs.html",
	"Preconstructed_Decks.html",
	"Pokemon_Booster_Boxes.html",
	"Pokemon_Booster_Packs.html",
	"Yugioh_Booster_Boxes.html",
	"Yu_Gi_Oh_Booster_Packs.html",
	"Collector_Tins.html",
	"Gift_Sets.html",
}

// Strikezone prices Strike Zone's singles, both what they sell and what they
// buy.
type Strikezone struct {
	logCallback    mtgban.LogCallbackFunc
	inventoryDate  time.Time
	buylistDate    time.Time
	maxConcurrency int

	inventory mtgban.InventoryRecord
	buylist   mtgban.BuylistRecord

	disableRetail  bool
	disableBuylist bool

	backend *matcher.Backend
	shelf   string
	client  *http.Client
}

// NewScraper returns a scraper for the datastore's game.
func NewScraper(b *matcher.Backend) (*Strikezone, error) {
	game := b.Game
	shelf, ok := szGames[game]
	if !ok {
		return nil, fmt.Errorf("unsupported game %q", game)
	}
	sz := Strikezone{}
	sz.inventory = mtgban.InventoryRecord{}
	sz.buylist = mtgban.BuylistRecord{}
	sz.maxConcurrency = defaultConcurrency
	sz.backend = b
	sz.shelf = shelf
	sz.client = mtgban.NewHTTPClient()
	return &sz, nil
}

func (sz *Strikezone) printf(format string, a ...any) {
	if sz.logCallback != nil {
		sz.logCallback("[SZ] "+format, a...)
	}
}

type respChan struct {
	cardID string
	inv    *mtgban.InventoryEntry
	bl     *mtgban.BuylistEntry
}

func (sz *Strikezone) getDoc(ctx context.Context, link string) (*goquery.Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := sz.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", link, resp.Status)
	}
	return goquery.NewDocumentFromReader(resp.Body)
}

func absoluteURL(base, href string) string {
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if ref.IsAbs() {
		return ref.String()
	}
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	return b.ResolveReference(ref).String()
}

func (sz *Strikezone) keepLink(link, mode string) bool {
	basePath := "/Category/"
	if mode == modeBuylist {
		basePath = "/BuyList/"
	}
	if !strings.Contains(link, basePath) {
		return false
	}
	for _, suffix := range skipSuffixes {
		if strings.HasSuffix(link, suffix) {
			return false
		}
	}
	return true
}

func (sz *Strikezone) childLinks(doc *goquery.Document, pageURL, mode string) []string {
	var links []string
	seen := map[string]struct{}{}
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if !ok {
			return
		}
		abs := absoluteURL(pageURL, href)
		if abs == "" || !sz.keepLink(abs, mode) {
			return
		}
		if u, err := url.Parse(abs); err != nil || u.Host != "shop.strikezoneonline.com" {
			return
		}
		if _, dup := seen[abs]; dup {
			return
		}
		seen[abs] = struct{}{}
		links = append(links, abs)
	})
	return links
}

// magicFoilSiblings scans a Magic shelf page for card names carrying both a
// Normal and a Foil row, the shape a genuine second listing leaves behind.
func magicFoilSiblings(doc *goquery.Document, tableRowName string) map[string]bool {
	seen := map[string][2]bool{}
	doc.Find(tableRowName).Each(func(_ int, el *goquery.Selection) {
		name := strings.TrimSpace(el.Find("td:nth-child(1)").Text())
		if name == "" || name == "Name" {
			return
		}
		notes := strings.TrimSpace(el.Find("td:nth-child(4)").Text())
		pair := seen[name]
		if strings.Contains(strings.ToLower(notes), "foil") {
			pair[1] = true
		} else {
			pair[0] = true
		}
		seen[name] = pair
	})
	both := map[string]bool{}
	for name, pair := range seen {
		if pair[0] && pair[1] {
			both[name] = true
		}
	}
	return both
}

// szCondition reads the grade Strike Zone's Details wording opens with, one
// of detailConditions, through the shared word table.
func szCondition(details string) (mtgban.Condition, error) {
	for _, cond := range detailConditions {
		if strings.HasPrefix(details, cond) {
			return mtgban.ParseCondition(cond)
		}
	}
	return "", fmt.Errorf("unsupported %s condition", details)
}

func (sz *Strikezone) processRow(mode string, channel chan<- respChan, el *goquery.Selection, edition string, foilSiblings map[string]bool) error {
	var cardName, pathURL, notes, cond, qty, price string

	cardName = strings.TrimSpace(el.Find("td:nth-child(1)").Text())
	if cardName == "" || cardName == "Name" {
		// No error as empty page may not have anything to process
		return nil
	}

	pathURL, _ = el.Find("a").Attr("href")

	// The columns and card construction differ per game; the match and error
	// handling below are shared.
	var theCard *matcher.InputCard
	switch sz.backend.Game {
	case matcher.GameMagic:
		if mode == modeRetail {
			notes = strings.TrimSpace(el.Find("td:nth-child(4)").Text())
			cond = strings.TrimSpace(el.Find("td:nth-child(5)").Text())
			qty = strings.TrimSpace(el.Find("td:nth-child(6)").Text())
			price = strings.TrimSpace(el.Find("td:nth-child(7)").Text())
		} else if mode == modeBuylist {
			notes = strings.TrimSpace(el.Find("td:nth-child(4)").Text())
			cond = notes
			qty = strings.TrimSpace(el.Find("td:nth-child(5)").Text())
			price = strings.TrimSpace(el.Find("td:nth-child(6)").Text())
		}

		c, err := preprocess(sz.backend, cardName, edition, notes)
		if err != nil {
			return nil
		}
		theCard = c
	case matcher.GameLorcana:
		notes = strings.TrimSpace(el.Find("td:nth-child(2)").Text())
		cond = strings.TrimSpace(el.Find("td:nth-child(4)").Text())
		qty = strings.TrimSpace(el.Find("td:nth-child(5)").Text())
		price = strings.TrimSpace(el.Find("td:nth-child(6)").Text())

		foil := strings.Contains(strings.ToLower(cond), "foil")
		theCard = &matcher.InputCard{Name: lorcanaListing(cardName), Edition: edition, Variation: notes, Foil: foil}
	case matcher.GamePokemon, matcher.GameYuGiOh, matcher.GameFleshAndBlood:
		number := strings.TrimSpace(el.Find("td:nth-child(2)").Text())
		cond = strings.TrimSpace(el.Find("td:nth-child(4)").Text())
		qty = strings.TrimSpace(el.Find("td:nth-child(5)").Text())
		price = strings.TrimSpace(el.Find("td:nth-child(6)").Text())

		c, err := preprocessDetails(sz.backend.Game, cardName, edition, number, cond)
		if err != nil {
			return nil
		}
		theCard = c
	default:
		return nil
	}

	cardID, err := sz.backend.Match(theCard)
	var alias *matcher.AliasingError
	if errors.As(err, &alias) {
		id := resolveAliasing(sz.backend, aliasedListing{name: cardName, shelf: edition, card: theCard}, alias.Probe())
		if id != "" {
			cardID, err = id, nil
		}
	}
	if errors.Is(err, matcher.ErrUnsupported) {
		return nil
	} else if err != nil {
		// Skip errors from these sets, there is not enough information
		switch edition {
		case "Secret Lair", "The List", "Mystery Booster":
			return nil
		}
		sz.printf("%q", theCard)
		sz.printf("%s|%s|%s", cardName, edition, notes)

		if errors.As(err, &alias) {
			for _, probe := range alias.Probe() {
				card, _ := sz.backend.GetUUID(probe)
				sz.printf("- %s", card)
			}
		}
		return err
	}

	if sz.backend.Game == matcher.GameMagic {
		co, coErr := sz.backend.GetUUID(cardID)
		if coErr == nil && (namesAbsentTreatment(theCard.Variation, co) ||
			wearsUnnamedTextured(sz.backend, theCard.Variation, co) ||
			!finishPrinted(sz.backend, co, theCard.Foil, theCard.Variation, foilSiblings[cardName])) {
			return nil
		}
	}

	cardPrice, err := matcher.ParsePrice(price)
	if err != nil || cardPrice <= 0 {
		return err
	}

	quantity, err := strconv.Atoi(qty)
	if err != nil || quantity <= 0 {
		return err
	}

	grade, err := szCondition(cond)
	if err != nil {
		return err
	}

	if mode == modeRetail {
		channel <- respChan{
			cardID: cardID,
			inv: &mtgban.InventoryEntry{
				Conditions: grade,
				Price:      cardPrice,
				Quantity:   quantity,
				URL:        "http://shop.strikezoneonline.com" + pathURL,
				InstanceID: cartCode(el, "Add"),
			},
		}
	} else if mode == modeBuylist {
		var sellPrice, priceRatio float64

		invCards := sz.inventory[cardID]
		for _, invCard := range invCards {
			if invCard.Conditions == mtgban.NM {
				sellPrice = invCard.Price
				break
			}
		}

		if sellPrice > 0 {
			priceRatio = cardPrice / sellPrice * 100
		}

		// Some buy pages return wrong results if they have a comma
		cardName = url.QueryEscape(strings.Replace(cardName, ",", "", -1))
		link := "http://shop.strikezoneonline.com/TUser?MC=CUSTS&MF=B&BUID=637&ST=D&M=B&CMD=Search&T=" + cardName

		channel <- respChan{
			cardID: cardID,
			bl: &mtgban.BuylistEntry{
				Conditions: grade,
				BuyPrice:   cardPrice,
				Quantity:   quantity,
				PriceRatio: priceRatio,
				URL:        link,
				InstanceID: cartCode(el, "Buy"),
			},
		}
	}
	return nil
}

// shelfName reads the shelf off a category page's heading, which carries
// stray whitespace and the prefix and suffix each listing mode adds.
func shelfName(h1 string) string {
	shelf := strings.Join(strings.Fields(h1), " ")
	shelf = strings.TrimSuffix(shelf, " Buy Lists")
	shelf = strings.TrimPrefix(shelf, "Lorcana Singles ")
	return strings.TrimPrefix(shelf, "Singles ")
}

func (sz *Strikezone) parseRows(doc *goquery.Document, pageURL, mode string, channel chan<- respChan) {
	edition := shelfName(doc.Find("h1").First().Text())

	sz.printf("Parsing %s", edition)

	// Only the Magic categories render the denser rtti table; every
	// other game lists retail and buylist alike in the generic one.
	tableRowName := "table.rtti tr"
	if mode == modeBuylist || sz.backend.Game != matcher.GameMagic {
		tableRowName = "table.ItemTable tr"
	}

	var foilSiblings map[string]bool
	if sz.backend.Game == matcher.GameMagic {
		foilSiblings = magicFoilSiblings(doc, tableRowName)
	}

	doc.Find(tableRowName).Each(func(_ int, el *goquery.Selection) {
		err := sz.processRow(mode, channel, el, edition, foilSiblings)
		if err != nil {
			cardName := strings.TrimSpace(el.Find("td:nth-child(1)").Text())
			sz.printf("cannot process %s %s (%s): %s", mode, cardName, edition, err.Error())
			sz.printf("-> %s", pageURL)
		}
	})
}

// visitedPages is the set of category pages the crawl has reached, shared
// by the pool's workers.
type visitedPages struct {
	mu    sync.Mutex
	pages map[string]bool
}

func newVisitedPages(pages ...string) *visitedPages {
	v := &visitedPages{pages: map[string]bool{}}
	for _, page := range pages {
		v.pages[page] = true
	}
	return v
}

// first reports whether this is the first visit to the page, and records it.
func (v *visitedPages) first(page string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.pages[page] {
		return false
	}
	v.pages[page] = true
	return true
}

// processPage fetches one category page, emits its rows, then walks any
// deeper category links the same way mtgseattle recurses into subcategories.
func (sz *Strikezone) processPage(ctx context.Context, channel chan<- respChan, pageURL, mode string, visited *visitedPages) error {
	if !visited.first(pageURL) {
		return nil
	}

	doc, err := sz.getDoc(ctx, pageURL)
	if err != nil {
		return err
	}

	sz.parseRows(doc, pageURL, mode, channel)

	for _, child := range sz.childLinks(doc, pageURL, mode) {
		if err := sz.processPage(ctx, channel, child, mode, visited); err != nil {
			sz.printf("%v", err)
		}
	}
	return nil
}

func (sz *Strikezone) scrape(ctx context.Context, mode string) error {
	var link string
	if mode == modeRetail {
		link = fmt.Sprintf(szInventoryURL, sz.shelf)
		// The storefront files the Flesh and Blood singles under a bare
		// name no other game shares, instead of its own prefixed one.
		if sz.backend.Game == matcher.GameFleshAndBlood {
			link = "http://shop.strikezoneonline.com/Category/Singles.html"
		}
	} else if mode == modeBuylist {
		link = fmt.Sprintf(szBuylistURL, sz.shelf)
	}
	sz.printf("Visiting %s", link)

	doc, err := sz.getDoc(ctx, link)
	if err != nil {
		return err
	}

	links := sz.childLinks(doc, link, mode)
	sz.printf("Found %d categories", len(links))

	consume := func(resp respChan) {
		if resp.inv != nil {
			err := sz.inventory.Add(resp.cardID, resp.inv)
			if err != nil {
				sz.printf("%v", err)
			}
		}
		if resp.bl != nil {
			err := sz.buylist.Add(resp.cardID, resp.bl)
			if err != nil {
				sz.printf("%v", err)
			}
		}
	}

	// Hub rows first (the entry URL was scraped for rows as well as links).
	hubCh := make(chan respChan)
	var hubWG sync.WaitGroup
	hubWG.Add(1)
	go func() {
		defer hubWG.Done()
		for resp := range hubCh {
			consume(resp)
		}
	}()
	sz.parseRows(doc, link, mode, hubCh)
	close(hubCh)
	hubWG.Wait()

	// The hub is already loaded; keep workers from fetching it again when a
	// child page links back.
	visited := newVisitedPages(link)

	mtgban.WorkerPool(ctx, sz.maxConcurrency, links,
		func(ctx context.Context, page string, results chan<- respChan) error {
			return sz.processPage(ctx, results, page, mode, visited)
		},
		consume,
		sz.printf,
	)

	if mode == modeRetail {
		sz.inventoryDate = time.Now()
	} else if mode == modeBuylist {
		sz.buylistDate = time.Now()
	}

	return nil
}

// SetConfig applies options after the scraper was built. See
// mtgban.ScraperConfig.
func (sz *Strikezone) SetConfig(opt mtgban.ScraperOptions) {
	sz.disableRetail = opt.DisableRetail
	sz.disableBuylist = opt.DisableBuylist
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (sz *Strikezone) Load(ctx context.Context) error {
	var errs []error

	if !sz.disableRetail {
		err := sz.scrape(ctx, modeRetail)
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %w", mtgban.ErrInventoryLoad, err))
		}
	}

	if !sz.disableBuylist {
		err := sz.scrape(ctx, modeBuylist)
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %w", mtgban.ErrBuylistLoad, err))
		}
	}

	return errors.Join(errs...)
}

// Inventory returns what Load collected. See mtgban.Seller.
func (sz *Strikezone) Inventory() mtgban.InventoryRecord {
	return sz.inventory
}

// Buylist returns what Load collected. See mtgban.Vendor.
func (sz *Strikezone) Buylist() mtgban.BuylistRecord {
	return sz.buylist
}

// Info describes this scraper. See mtgban.Scraper.
func (sz *Strikezone) Info() (info mtgban.ScraperInfo) {
	info.Name = "Strike Zone"
	info.Shorthand = "SZ"
	info.InventoryTimestamp = &sz.inventoryDate
	info.BuylistTimestamp = &sz.buylistDate
	info.Game = sz.backend.Game
	return
}
