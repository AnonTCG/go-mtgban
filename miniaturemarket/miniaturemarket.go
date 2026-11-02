// Package miniaturemarket scrapes Miniature Market, which stocks sealed
// product only.
package miniaturemarket

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
)

// Miniaturemarket prices Miniature Market's sealed product; they carry no
// singles.
type Miniaturemarket struct {
	logCallback    mtgban.LogCallbackFunc
	maxConcurrency int
	affiliate      string

	inventoryDate time.Time
	inventory     mtgban.InventoryRecord
	productMap    map[string]string
	backend       *matcher.Backend
	widget        string
	client        *http.Client
}

// mmGames is what NewScraperSealed is built through, and a game named
// nowhere here is not one Miniature Market is read for. The value is the CMS
// navigation id behind the game's category page, read off the category pages,
// which serves the paginated product listing the scraper walks; unlike the
// other storefronts there is no separate shelf name to expose, since the
// widget id is the only thing a request ever carries.
var mmGames = map[matcher.Game]string{
	matcher.GameMagic:         "be53d253d6bc3258a8160556dda3e9b2",
	matcher.GameLorcana:       "4e0223a87610176ef0d24ef6d2dcde3a",
	matcher.GameRiftbound:     "019be122ca9779e5af00a663d064f775",
	matcher.GameOnePiece:      "f7ac67a9aa8d255282de7d11391e1b69",
	matcher.GameFleshAndBlood: "619205da514e83f869515c782a328d3c",
	matcher.GameGundam:        "019be1227c9b730eb41abadcdd09015a",
}

// NewScraperSealed returns a sealed scraper for the datastore's game.
func NewScraperSealed(b *matcher.Backend) (*Miniaturemarket, error) {
	game := b.Game
	widget, ok := mmGames[game]
	if !ok {
		return nil, fmt.Errorf("unsupported game %q", game)
	}
	mm := Miniaturemarket{}
	mm.inventory = mtgban.InventoryRecord{}
	mm.maxConcurrency = defaultConcurrency
	mm.productMap = map[string]string{}
	mm.backend = b
	mm.client = mtgban.NewHTTPClient()
	mm.widget = widget
	return &mm, nil
}

const defaultConcurrency = 6

// The decorations miniaturemarket writes into a One Piece product name: a
// storefront prefix, availability and pack-count parentheticals, and the
// set code in brackets where the canonical names spell no code at all.
var (
	// gundamDecorations are the shelf tags this storefront hangs off a
	// Gundam product: a pack count and the banner it was new under.
	gundamDecorations = regexp.MustCompile(`\s*\((?:New Arrival|Preorder|\d+)\)`)

	// gundamSetCode is the bracketed code the storefront writes mid-name.
	// The trailing letter is the Premium Collections' - "[PC01A]" - which
	// One Piece's own pattern does not allow for.
	gundamSetCode = regexp.MustCompile(`\s*\[([A-Z]+)(\d+)[A-Z]?\]`)

	onePieceDecorations = regexp.MustCompile(`\s*\((?:Preorder|\d+ Packs?)\)`)
	onePieceSetCode     = regexp.MustCompile(`\s*\[([A-Z]+)-?(\d+)\]`)
)

// The decorations miniaturemarket writes into a Flesh and Blood product
// name: a storefront prefix, and a trailing pack count the canon does not
// spell. "(Preorder)" and "(New Arrival)" are left for the resolve retry,
// which already sees past a trailing parenthetical - and which the Silver
// Age decks depend on keeping, since theirs names the hero's class.
var fabPackCount = regexp.MustCompile(`\s*\(\d+\)`)

// fabDeckSet matches the storefront's name for a chapter's full deck
// display, which the canon calls a display rather than a set of its count.
var fabDeckSet = regexp.MustCompile(`\s+Deck - Set of \d+`)

// sealedName rewrites a storefront listing toward the shape its game's
// canonical names use, where the two differ by more than the trailing
// decoration the resolve retry already sees past.
//
// One Piece names arrive as "One Piece TCG: BLUE Kuzan [ST-33] - Starter
// Deck (Preorder)" while the canon says "Starter Deck 33: BLUE Kuzan": the
// prefix and parentheticals only decorate, and the bracket code either
// carries the deck number the canon leads with, or restates a set the rest
// of the name already spells.
func sealedName(game matcher.Game, name string) string {
	if game == matcher.GameFleshAndBlood {
		name = strings.TrimPrefix(name, "Flesh & Blood TCG: ")
		name = fabPackCount.ReplaceAllString(name, "")
		// The canon spells an unlimited printing as a bracketed edition at
		// the end; the storefront abbreviates it in the middle of the name.
		unlimited := strings.Contains(name, " Unlimited Ed ")
		name = strings.Replace(name, " Unlimited Ed ", " ", 1)
		// The canon runs a set name straight into what it is sold as, and
		// separates an Armory deck from its hero with a colon; the
		// storefront spells a dash for both. The Silver Age decks are the
		// exception it cannot be applied to blindly - their dash is the
		// canon's own.
		name = strings.Replace(name, " - Booster ", " Booster ", 1)
		name = strings.Replace(name, "Armory Deck - ", "Armory Deck: ", 1)
		// A display of every deck in a chapter is sold as a set of its
		// count, and named for what it is.
		name = fabDeckSet.ReplaceAllString(name, " Deck Display")
		if unlimited {
			name += " [Unlimited Edition]"
		}
		return strings.TrimSpace(name)
	}
	if game == matcher.GameGundam {
		name = strings.TrimPrefix(name, "GUNDAM Card Game: ")
		name = gundamDecorations.ReplaceAllString(name, "")

		// The canon names a starter deck for its number and the set it is,
		// where the storefront brackets the code in the middle and says what
		// it is at the end - the same shape One Piece writes below.
		match := gundamSetCode.FindStringSubmatch(name)
		name = gundamSetCode.ReplaceAllString(name, "")
		if match != nil && match[1] == "ST" {
			name = strings.Replace(name, " - Starter Deck", "", 1)
			return strings.TrimSpace("Starter Deck " + match[2] + ": " + strings.TrimSpace(name))
		}
		// A Premium Card Collection is named for the series arc it collects.
		// The canon prefixes that arc with the series it belongs to and ends
		// on the bracketed code; the storefront spells no series at all, and
		// writes the code mid-name where every other product carries it.
		if match != nil && match[1] == "PC" {
			name = strings.Replace(name, " - ", " - Mobile Suit Gundam ", 1)
			return strings.TrimSpace(name + match[0])
		}
		// Everything else runs the set name straight into what it is sold
		// as, where the storefront dashes the two apart.
		name = strings.Replace(name, " - ", " ", 1)
		return strings.TrimSpace(name)
	}

	if game != matcher.GameOnePiece {
		return name
	}

	name = strings.TrimPrefix(name, "One Piece TCG: ")
	name = onePieceDecorations.ReplaceAllString(name, "")

	match := onePieceSetCode.FindStringSubmatch(name)
	if match != nil && match[1] == "ST" {
		name = onePieceSetCode.ReplaceAllString(name, "")
		name = strings.Replace(name, " - Starter Deck", "", 1)
		name = "Starter Deck " + match[2] + ": " + strings.TrimSpace(name)
	} else {
		name = onePieceSetCode.ReplaceAllString(name, "")
	}

	return strings.TrimSpace(name)
}

// sealedWords splits a product name the way a comparison of two spellings of
// it needs: lower case, and the punctuation a storefront and the canon differ
// over dropped rather than glued to a word.
var sealedPunct = strings.NewReplacer(",", " ", ":", " ", "-", " ", "(", " ", ")", " ", "[", " ", "]", " ")

func sealedWords(name string) []string {
	return strings.Fields(strings.ToLower(sealedPunct.Replace(name)))
}

// resolveByNamedCard answers a listing whose canonical name says more than
// the storefront did, when every word it adds belongs to a card.
//
// A deck is named for the hero it plays, and the canon spells that hero out
// where a storefront prints the first word of the name: "Silver Age Chapter 3
// Deck - Blaze" against "... - Blaze Firemind". Nothing about the product
// differs, only how much of the hero got typed.
//
// What keeps this from reading a case as the box it holds - "Booster Box"
// says everything "Booster Box Case" does - is that the words it forgives
// must be a card the datastore carries. "Firemind" is one and "Case" is not,
// which is the same line the matcher draws when it decides that a bracket
// naming a card is a qualifier and one naming a count is identity.
//
// A tie says nothing: two products the storefront's words fit equally are two
// products it did not choose between, and neither is the answer. The
// aliasing error names the first two found.
func resolveByNamedCard(b *matcher.Backend, listed string) (string, error) {
	vendor := sealedWords(listed)
	if len(vendor) == 0 {
		return "", matcher.ErrUnsupported
	}

	var found string
	var foundName string
	for _, uuid := range b.GetSealedUUIDs() {
		co, err := b.GetUUID(uuid)
		if err != nil {
			continue
		}
		extras, ok := extraWords(sealedWords(co.Name), vendor)
		if !ok || len(extras) == 0 || !extrasNameACard(b, extras, sealedWords(co.Name)) {
			continue
		}
		if found != "" && foundName != co.Name {
			return "", matcher.NewAliasingError(found, uuid)
		}
		found, foundName = uuid, co.Name
	}
	if found == "" {
		return "", matcher.ErrUnsupported
	}
	return found, nil
}

// extraWords returns the words a candidate holds beyond the ones the vendor
// said, and whether the vendor said nothing the candidate does not.
func extraWords(candidate, vendor []string) ([]string, bool) {
	counts := map[string]int{}
	for _, word := range candidate {
		counts[word]++
	}
	for _, word := range vendor {
		if counts[word] == 0 {
			return nil, false
		}
		counts[word]--
	}
	var extras []string
	for _, word := range candidate {
		if counts[word] > 0 {
			counts[word]--
			extras = append(extras, word)
		}
	}
	return extras, true
}

// extrasNameACard reports whether the words a candidate adds are part of a
// card the candidate itself names - which is what tells a hero's epithet from
// the word that makes a case a case.
//
// The card has to be spelled inside the candidate, every word of it, not
// merely exist somewhere in the datastore. Searching by substring alone would
// answer "case" with Staircase and forgive the word that distinguishes a case
// from the box in it.
func extrasNameACard(b *matcher.Backend, extras, candidate []string) bool {
	uuids, err := b.SearchContains(strings.Join(extras, " "))
	if err != nil {
		return false
	}
	inCandidate := map[string]bool{}
	for _, word := range candidate {
		inCandidate[word] = true
	}
	for _, uuid := range uuids {
		co, err := b.GetUUID(uuid)
		if err != nil {
			continue
		}
		card := sealedWords(co.Name)
		if len(card) == 0 {
			continue
		}
		spelled := true
		for _, word := range card {
			if !inCandidate[word] {
				spelled = false
				break
			}
		}
		if !spelled {
			continue
		}
		if _, covered := extraWords(card, extras); covered {
			return true
		}
	}
	return false
}

// resolveListing names the sealed product a storefront listing prices, or
// says why none was found. Every listing this scraper leaves behind leaves it
// here, which is why the reason is returned rather than swallowed: otherwise
// a run that resolved nothing at all looks exactly like a run with nothing to
// resolve.
//
// Magic routes through the id the datastore records; the other games' data
// carries no miniaturemarket ids, so the product is resolved by its listed
// name, English only, unique or nothing. A failing name retries without its
// trailing decoration ("(New Arrival)"), which resolution rightly refuses to
// see past on its own.
func (mm *Miniaturemarket) resolveListing(id, listed string) (string, string) {
	if uuid, found := mm.productMap[id]; found {
		return uuid, ""
	}
	if mm.backend.Game == matcher.GameMagic {
		return "", "no datastore id"
	}
	name := strings.TrimSpace(sealedName(mm.backend.Game, listed))
	if name == "" {
		return "", "unnamed listing"
	}
	if matcher.SealedIsLanguageVariant(name) {
		return "", "language variant"
	}
	// A trailing parenthetical is the storefront's decoration on some
	// products and the product's own on others - "(Preorder)" against
	// "(Chaos Assassin)" - so a name that fails is asked both ways rather
	// than guessed at, by the resolver and then by the fallback.
	trimmed := name
	if idx := strings.LastIndexByte(name, '('); idx > 0 {
		trimmed = strings.TrimSpace(name[:idx])
	}
	spellings := []string{name}
	if trimmed != name {
		spellings = append(spellings, trimmed)
	}

	uuid, err := mm.backend.ResolveSealed(name)
	if err == nil {
		return uuid, ""
	}
	// The resolver's refusal on the name as listed is the one worth
	// reporting: every attempt after this one is a guess at what else the
	// listing might have meant, and saying that a guess failed tells a
	// reader nothing about the name that did.
	refusal := err
	for _, spelling := range spellings[1:] {
		if uuid, err = mm.backend.ResolveSealed(spelling); err == nil {
			return uuid, ""
		}
	}
	for _, spelling := range spellings {
		if uuid, err = resolveByNamedCard(mm.backend, spelling); err == nil {
			return uuid, ""
		}
	}
	return "", refusal.Error()
}

func (mm *Miniaturemarket) mainURL() string {
	return "https://www.miniaturemarket.com/widgets/cms/navigation/" + mm.widget + "?filter-inStock=1&no-aggregations=1&order=name-asc&p=1"
}

type respChan struct {
	cardID   string
	invEntry *mtgban.InventoryEntry

	// drop names why a listing was not priced and repeats the name it was
	// listed under. A record carrying one prices nothing; it is there so
	// the run can say what it left behind instead of dropping it in
	// silence, which is what a run pricing none of a game looked like.
	drop string
	name string
}

func (mm *Miniaturemarket) printf(format string, a ...any) {
	if mm.logCallback != nil {
		mm.logCallback("[MMSealed] "+format, a...)
	}
}

func (mm *Miniaturemarket) processPage(ctx context.Context, channel chan<- respChan, page int) error {
	u, err := url.Parse(mm.mainURL())
	if err != nil {
		return err
	}
	v := u.Query()
	v.Set("p", fmt.Sprint(page))
	u.RawQuery = v.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return err
	}
	resp, err := mm.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		mm.printf("newDoc - %s", err.Error())
		return err
	}

	doc.Find(`div[class="product-info"]`).Each(func(i int, s *goquery.Selection) {
		listed := strings.TrimSpace(s.Find(`a.product-name`).Text())
		id, _ := s.Find(`input[name="product-id"]`).Attr("value")
		uuid, drop := mm.resolveListing(id, listed)
		if drop != "" {
			channel <- respChan{drop: drop, name: listed}
			return
		}

		link, _ := s.Find(`a.product-name`).Attr("href")
		if mm.affiliate != "" {
			link += "?utm_source=" + mm.affiliate + "&utm_medium=feed&utm_campaign=mtg_singles"
		}

		priceStr := s.Find(`.product-price`).Text()
		price, err := matcher.ParsePrice(priceStr)
		if err != nil {
			channel <- respChan{drop: "unparseable price", name: listed}
			return
		}

		channel <- respChan{
			cardID: uuid,
			invEntry: &mtgban.InventoryEntry{
				Price: price,
				URL:   link,
			},
		}
	})

	return nil
}

// NumberOfPages returns how many pages the widget paginates into, read off
// the last link in its pagination.
func (mm *Miniaturemarket) NumberOfPages(ctx context.Context) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mm.mainURL(), http.NoBody)
	if err != nil {
		return 0, err
	}
	resp, err := mm.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		mm.printf("newDoc - %s", err.Error())
		return 0, err
	}

	// A catalog that fits one page renders no pagination at all
	href, _ := doc.Find("a.page-link").Last().Attr("href")
	if href == "" {
		return 1, nil
	}
	u, err := url.Parse(href)
	if err != nil {
		return 0, err
	}
	num := u.Query().Get("p")
	if num == "" {
		return 1, nil
	}
	return strconv.Atoi(num)
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (mm *Miniaturemarket) Load(ctx context.Context) error {
	for _, uuid := range mm.backend.GetSealedUUIDs() {
		co, err := mm.backend.GetUUID(uuid)
		if err != nil || co.Identifiers["miniaturemarketId"] == "" {
			continue
		}
		mm.productMap[co.Identifiers["miniaturemarketId"]] = uuid
	}
	mm.printf("Loaded %d sealed products", len(mm.productMap))
	if mm.backend.Game != matcher.GameMagic {
		mm.printf("Resolving %s products by name", mm.backend.Game)
	}

	totalPages, err := mm.NumberOfPages(ctx)
	if err != nil {
		return err
	}
	mm.printf("Parsing %d pages", totalPages)

	// Pages are numbered from one. Walking from zero would fetch the first
	// page twice - the widget answers p=0 with it - and stop one short,
	// leaving the last page of every multi-page catalog unread.
	pageNums := make([]int, totalPages)
	for i := range pageNums {
		pageNums[i] = i + 1
	}

	// The consumer runs on one goroutine, so the tally needs no locking.
	var listed, priced int
	dropped := map[string]int{}
	mtgban.WorkerPool(ctx, mm.maxConcurrency, pageNums,
		func(ctx context.Context, page int, results chan<- respChan) error {
			return mm.processPage(ctx, results, page)
		},
		func(record respChan) {
			listed++
			if record.drop != "" {
				dropped[record.drop]++
				// The names a resolver turned down are the whole reason a
				// run prices a fraction of a catalog, so say which name and
				// which refusal, the way the other sealed scrapers do. The
				// products carrying no id of ours are the ordinary case for
				// Magic and are counted rather than listed.
				if record.drop != "no datastore id" {
					mm.printf("%q: %s", record.name, record.drop)
				}
				return
			}
			priced++
			err := mm.inventory.AddRelaxed(record.cardID, record.invEntry)
			if err != nil {
				mm.printf("%v", err)
			}
		},
		mm.printf,
	)
	mm.printf("Priced %d of %d listings", priced, listed)
	for _, reason := range slices.Sorted(maps.Keys(dropped)) {
		mm.printf("Dropped %d listings: %s", dropped[reason], reason)
	}

	mm.inventoryDate = time.Now()

	return nil
}

// Inventory returns what Load collected. See mtgban.Seller.
func (mm *Miniaturemarket) Inventory() mtgban.InventoryRecord {
	return mm.inventory
}

// Info describes this scraper. See mtgban.Scraper.
func (mm *Miniaturemarket) Info() (info mtgban.ScraperInfo) {
	info.Name = "Miniature Market"
	info.Shorthand = "MMSealed"
	info.InventoryTimestamp = &mm.inventoryDate
	info.SealedMode = true
	info.NoQuantityInventory = true
	info.Game = mm.backend.Game
	return
}
