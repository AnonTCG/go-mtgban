// Package coolstuffinc scrapes Cool Stuff Inc, for singles and sealed
// product.
package coolstuffinc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/matcher/magic"
	"github.com/mtgban/go-mtgban/mtgban"
)

const (
	defaultConcurrency = 8

	// The feed names no buylist quantity; this is the most CSI buys of one.
	maxBuylistQuantity = 20

	csiInventoryURL = "https://www.coolstuffinc.com/sq/?s="
)

// The games this scraper covers, as the storefront names them.
const (
	GameMagic             = "mtg"
	GameLorcana           = "lorcana"
	GameRiftbound         = "riftbound"
	GameYuGiOh            = "yugioh"
	GameDragonBallSuper   = "dbs"
	GameOnePiece          = "optcg"
	GameStarWarsUnlimited = "swu"
	GamePokemon           = "pokemon"
	GameGundam            = "gundam"
	GamePalworld          = "palworld"
)

// csiGames is what the two constructors are built through: it names the
// shelf a game is searched under, and a game named nowhere here is not one
// Cool Stuff Inc is read for. The shelves themselves stay public - Search,
// GetBuylist and LoadBuylistEditions each ask for one by name.
var csiGames = map[matcher.Game]string{
	matcher.GameMagic:     GameMagic,
	matcher.GameLorcana:   GameLorcana,
	matcher.GameRiftbound: GameRiftbound,
	matcher.GameYuGiOh:    GameYuGiOh,
	matcher.GameOnePiece:  GameOnePiece,
	matcher.GamePokemon:   GamePokemon,
	matcher.GameGundam:    GameGundam,
	matcher.GamePalworld:  GamePalworld,
}

var deductions = []float64{1, 1, 0.75}

var availableMarketNames = []string{
	"Cool Stuff Inc", "Cool Stuff Inc (unique)", "Cool Stuff Inc Graded",
}

var name2shorthand = map[string]string{
	"Cool Stuff Inc":          "CSI",
	"Cool Stuff Inc (unique)": "CSIUnique",
	"Cool Stuff Inc Graded":   "CSIGraded",
}

// Coolstuffinc prices Cool Stuff Inc's singles, both what they sell and what
// they buy.
type Coolstuffinc struct {
	logCallback mtgban.LogCallbackFunc
	partner     string

	// If set to true scrape will include all entries without a nonfoil NM price
	// but will be almost twice as slow
	includeOOS bool

	inventoryDate  time.Time
	buylistDate    time.Time
	maxConcurrency int

	targetEdition string

	disableRetail  bool
	disableBuylist bool

	inventory mtgban.InventoryRecord
	buylist   mtgban.BuylistRecord

	client  *http.Client
	shelf   string
	backend *matcher.Backend

	// skuIndex is skuNumbers of the backend, for a Yu-Gi-Oh scraper.
	skuIndex map[string][]string

	// nonfoilPIDs holds the buylist's products that have a nonfoil row,
	// which tells Magic's placeholder foil rows apart; see buylistMagic.
	nonfoilPIDs map[string]bool
}

// nameParenthetical matches a qualifier a buylist name carries in brackets,
// like "(Parallel)" or "(Alternate Art)".
var nameParenthetical = regexp.MustCompile(`\(([^)]+)\)`)

// nameQualifiers answers the wording a One Piece buylist name carries behind
// the card's own, which is where that feed spends the qualifier telling one
// printing from another - the note beside it describes the artwork instead
// ("Hand Blocking Sun", "Leg Gun"), and the matcher reads neither the name
// nor a description. A number-shaped bracket is left behind: the feed repeats
// the collector number there and it says nothing the number field has not.
func nameQualifiers(name string) string {
	return strings.Join(nameQualifierList(name), " ")
}

// nameQualifierList answers each qualifier nameQualifiers joins, one per
// bracket.
func nameQualifierList(name string) []string {
	var words []string
	for _, match := range nameParenthetical.FindAllStringSubmatch(name, -1) {
		qualifier := strings.TrimSpace(match[1])
		if qualifier == "" || buylistNumberWord.MatchString(qualifier) {
			continue
		}
		words = append(words, qualifier)
	}
	return words
}

// csiSignedBy matches the autograph credit a name carries, "Signed by X" or
// "Signed FOIL  by X", which only the artist's copy has.
var csiSignedBy = regexp.MustCompile(`(?i)\bsigned\s+(?:foil\s+)?by\b`)

// csiSignedFoil matches the credit that puts FOIL after Signed, where the
// other listings put it before.
var csiSignedFoil = regexp.MustCompile(`(?i)signed\s+foil\s+by`)

// csiAmbiguousNote matches the note a listing carries when it says it is one
// of several printings ("Can be Normal or 25th Anniversary Stamp"): no single
// identity exists for it, so a refusal of one is the vendor's own ambiguity.
var csiAmbiguousNote = regexp.MustCompile(`(?i)^\s*can be\b`)

// csiAmbiguousRefusal reports a refusal that is the note's own ambiguity:
// the printings it names alias, or its wording names no single variant. A
// name or edition the catalog lacks is refused whatever the note says.
func csiAmbiguousRefusal(notes string, err error) bool {
	if !csiAmbiguousNote.MatchString(notes) {
		return false
	}
	var alias *matcher.AliasingError
	return errors.As(err, &alias) || errors.Is(err, matcher.ErrCardWrongVariant)
}

// buylistNumberWord matches the number-shaped words of a buylist note.
var buylistNumberWord = regexp.MustCompile(`(?i)^[A-Z]{0,4}\d+[a-z]?(?:/[A-Z]{0,4}\d+)?[,.]?$`)

var buylistReprintNote = regexp.MustCompile(`(?i)\breprints?\b`)

// jpArtWordingRe matches "Japanese Art"/"Artwork"/"Art Style", which
// matcher's language filter otherwise reads as a request for a
// Japanese-language card. "Japanese Letters in Art" is a Yu-Gi-Oh artwork
// note and is read by yugiohArtNotes before it gets here.
var jpArtWordingRe = regexp.MustCompile(`(?i)Japanese Art(?:work|\s+Style)?`)

// jpArtWording rewrites the wording jpArtWordingRe matches to "JP Art", the
// catalog's own name for the treatment.
func jpArtWording(s string) string {
	return jpArtWordingRe.ReplaceAllString(s, "JP Art")
}

// buylistVariation builds everything the buylist knows about a printing
// beyond its name. The feed spends a free-text note on the qualifier that
// the sell listing spends its variation on - "Love Ball Foil", "Detective
// Pikachu Stamped" - and reading only the number field asked the two sides
// of one product different questions.
//
// The note's own numbers stay behind. They name other products the buyer
// might have meant ("Can be Pikachu 2, 19, 41, or 45"), while the collector
// number arrives in its own field and again in the product name. A note
// that says a printing reprints another stays behind whole, because every
// word after the number is then the name of the set being reprinted rather
// than of this one.
func buylistVariation(product CSIPriceEntry) string {
	if buylistReprintNote.MatchString(product.Notes) {
		return product.Number
	}
	words := []string{product.Number}
	for word := range strings.FieldsSeq(product.Notes) {
		if buylistNumberWord.MatchString(word) {
			continue
		}
		words = append(words, word)
	}
	return strings.TrimSpace(strings.Join(words, " "))
}

// NewScraper returns a singles scraper for the datastore's game.
func NewScraper(b *matcher.Backend) (*Coolstuffinc, error) {
	game := b.Game
	shelf, ok := csiGames[game]
	if !ok {
		return nil, fmt.Errorf("unsupported game %q", game)
	}
	csi := Coolstuffinc{}
	csi.inventory = mtgban.InventoryRecord{}
	csi.buylist = mtgban.BuylistRecord{}
	csi.client = newCSIHTTPClient()
	csi.maxConcurrency = defaultConcurrency
	csi.shelf = shelf
	csi.backend = b
	if game == matcher.GameYuGiOh {
		csi.skuIndex = skuNumbers(b)
	}
	return &csi, nil
}

type responseChan struct {
	cardID   string
	invEntry *mtgban.InventoryEntry
	relaxed  bool
}

func (csi *Coolstuffinc) printf(format string, a ...any) {
	if csi.logCallback != nil {
		csi.logCallback("[CSI] "+format, a...)
	}
}

// saleTail is what the price column writes in front of a discounted price.
// The space is a non-breaking one, as the page spells it.
const saleTail = "Was\u00a0"

// offerCondition reads the condition out of one offer row's text. The row
// spells the quantity, the condition, whatever promotions it is running and
// the price, and only the condition is wanted.
//
// The promotions are tails on the condition, and a row may run several at
// once: the bundle flag sits in the condition column and the sale's "Was" in
// the price one, so cutting a single tail would leave the other row's flag
// glued to the condition and drop the whole offer as unparseable. Cutting
// until nothing more comes off reads them in whatever order the page lays
// them out, and a row running none is unchanged by the first pass.
func offerCondition(fullRow, qtyStr, bundleStr string) string {
	// The count is a prefix, not a set of characters to eat: trimming it as
	// a cutset eats the leading digit of a condition that opens with one,
	// turning "1st Edition" into "st Edition".
	conditions := strings.TrimPrefix(fullRow, qtyStr)
	conditions = strings.TrimPrefix(conditions, "+")
	conditions = strings.TrimLeft(conditions, " ")
	conditions = strings.Split(conditions, "$")[0]
	for {
		trimmed := strings.TrimSuffix(conditions, bundleStr)
		trimmed = strings.TrimSuffix(trimmed, saleTail)
		if trimmed == conditions {
			return conditions
		}
		conditions = trimmed
	}
}

// firstEditionShelf reads the print run a Pokemon shelf names in its own
// title. The storefront files the first-edition run of a set as a shelf
// beside the set - "1st Edition Fossil" next to "Fossil" - where the catalog
// files the run as a finish of the set itself, so the shelf name has to
// become one. Left as it is, every listing on those shelves matches the
// unlimited printing and is published at a fraction of its price, and nothing
// says so: the match succeeds, it just answers with the other run.
func firstEditionShelf(edition string) (string, []string) {
	trimmed := strings.TrimPrefix(edition, "1st Edition ")
	if trimmed == edition {
		return edition, nil
	}
	// Base Set is the one shelf whose run the catalog files as a set of its
	// own rather than a finish of the set beside it: the first-edition and
	// shadowless printings are "Base Set (Shadowless)", and "Base Set" holds
	// only the shadowed unlimited run. Naming the shelf's own set asks for a
	// run that set has one card of, so every other row would fall through
	// to the unlimited printing and be published at the first edition's
	// price.
	if set, found := runShelfEditions[trimmed]; found {
		trimmed = set
	}
	return trimmed, conditionRuns["1st Edition"]
}

// runShelfEditions name the set a print-run shelf sells where the catalog
// files the run apart from the shelf's own set. Fossil, Jungle, Team Rocket,
// the Gyms and the Neos all carry theirs as a finish and are left alone.
var runShelfEditions = map[string]string{
	"Base Set": "Base Set (Shadowless)",
}

// conditionRuns are the print runs this storefront names where a condition
// would go. The run is a finish in the game's own vocabulary rather than a
// state of the card, and the spelling has to be exact: asking for the plain
// run of a card printed in holo answers with the holo of the other run - the
// unlimited Lapras where the first-edition one was listed, at a fraction of
// its price - so both spellings are tried and the answer is checked for
// carrying the run before it is taken.
var conditionRuns = map[string][]string{
	"1st Edition": {"1st Edition Holofoil", "1st Edition"},
}

// conditionRun reads the run a row names in its condition column, giving the
// finishes it may be sold as, widest first.
func conditionRun(conditions string) []string {
	for marker, finishes := range conditionRuns {
		if strings.Contains(conditions, marker) {
			return finishes
		}
	}
	return nil
}

// matchRun resolves a listing sold as one of a card's print runs, refusing
// an answer that does not carry it rather than pricing the run as the
// ordinary printing.
func matchRun(b *matcher.Backend, inCard *matcher.InputCard, finishes []string) (string, error) {
	var err error
	for _, finish := range finishes {
		probe := *inCard
		probe.Finish = finish
		var cardID string
		cardID, err = b.Match(&probe)
		if err != nil {
			continue
		}
		co, uerr := b.GetUUID(cardID)
		if uerr != nil {
			continue
		}
		if co.Finish == matcher.Normalize(finish) {
			return cardID, nil
		}
	}
	if err == nil {
		err = matcher.ErrUnsupported
	}
	return "", err
}

// skippedConditions name a copy the catalog has no row for, so there is
// nothing to price it as and nothing to report either. Asian names no
// language in particular, and the only Evolving Wilds sold under it - the
// one listing that carries the wording - is a name with no set, number or
// language beside it, against seventy-seven printings that are all English.
var skippedConditions = []string{"Asian"}

// isSkippedCondition reports whether the wording names one of those.
func isSkippedCondition(conditions string) bool {
	for _, skipped := range skippedConditions {
		if strings.Contains(conditions, skipped) {
			return true
		}
	}
	return false
}

// conditionPrintings are the printings this storefront sells as an offer of
// their own, naming them where a condition would go. The run is a real
// printing rather than a state of the card, so the wording moves to the
// variation and lets the matcher pick it, and the offer is priced as the
// stock it is.
var conditionPrintings = map[string]string{
	"PRE-ERRATA":     "Pre-Errata",
	"Pokemon Center": "Pokemon Center",
}

// conditionPrinting reads the printing a row names in its condition column,
// or "" where the wording names a condition.
func conditionPrinting(conditions string) string {
	for marker, variation := range conditionPrintings {
		if strings.Contains(conditions, marker) {
			return variation
		}
	}
	return ""
}

// gradedMarkers are the wordings a row carries when the copy is not being
// sold at a condition tier: a slab named for the service that graded it, the
// one-off the storefront files as unique, or the printing a single copy is
// when it is sold on another printing's product - the shadowless Venusaur
// offered on the unlimited Venusaur's page, at three times what the copy
// beside it costs. None of those is a condition and none of their prices is
// the card's, so such a row is published as its own seller rather than
// beside the ordinary copies.
var gradedMarkers = []string{"BGS", "PSA", "CGC", "TAG", "Non-Foil", "Unique", "Shadowless", "No Set Symbol"}

// isGraded reports whether the condition wording names one of those rather
// than a condition. A wording it does not know is refused by the condition
// parser, which says so, so a new grading service costs a line here.
func isGraded(conditions string) bool {
	for _, marker := range gradedMarkers {
		if strings.Contains(conditions, marker) {
			return true
		}
	}
	return false
}

// reSlab reads a slab's grader and score off the condition wording
// ("PSA 10  PSA 10 ").
var reSlab = regexp.MustCompile(`^(PSA|BGS|CGC) (\d+(?:\.\d+)?)\b`)

// slabCondition grades a slab by its score, and reports false for any other
// wording, or a score the grade table does not hold.
func slabCondition(conditions string) (mtgban.Condition, bool) {
	match := reSlab.FindStringSubmatch(strings.TrimPrefix(conditions, "Foil "))
	if match == nil {
		return "", false
	}
	return mtgban.SlabCondition(match[1], match[2])
}

// bundleRe matches the wording of the bundle promotion, whatever count it
// gives away: "Buy 1 get 3 free!" sells four copies for the listed price.
var bundleRe = regexp.MustCompile(`^Buy 1 get (\d+) free!$`)

// bundledCopies reads how many copies the listed price buys: one, unless the
// row runs the bundle promotion, whose price covers the bought copy and the
// free ones together. The wording carries the count, so every promotion the
// condition parser cuts is also one the price is divided by, whatever count
// it spells.
//
// The price is the only thing the promotion changes. The count beside it is
// the same card-qty column every row on the page carries, promotion or not,
// capped at "20+" the way a stock figure is, so it goes out as the stock it
// reads as rather than divided to match the price.
func bundledCopies(bundleStr string) int {
	match := bundleRe.FindStringSubmatch(bundleStr)
	if match == nil {
		return 1
	}
	free, err := strconv.Atoi(match[1])
	if err != nil {
		return 1
	}
	return 1 + free
}

// sellListing is what a search result row states about one offer.
type sellListing struct {
	pid, name, edition, notes, rarity, image string
	foil                                     bool

	// printing is the printing the offer's condition names, and run the
	// finishes it says the copy is sold in.
	printing string
	run      []string
	graded   bool
	slabbed  bool
}

// retailResolvers names the printing an offer of each game is. A skipped
// offer answers matcher.ErrUnsupported; any other error comes back with the
// card that was matched, for the log.
var retailResolvers = map[matcher.Game]func(csi *Coolstuffinc, l sellListing) (*matcher.InputCard, string, error){
	matcher.GameMagic:     retailMagic,
	matcher.GameYuGiOh:    retailYugioh,
	matcher.GamePokemon:   retailPokemon,
	matcher.GameOnePiece:  retailOnePiece,
	matcher.GameGundam:    retailGundam,
	matcher.GamePalworld:  retailPalworld,
	matcher.GameRiftbound: retailRiftbound,
	matcher.GameLorcana:   retailLorcana,
}

// buylistResolvers names the printing a buylist row of each game is, the way
// retailResolvers does for an offer.
var buylistResolvers = map[matcher.Game]func(csi *Coolstuffinc, p CSIPriceEntry) (*matcher.InputCard, string, error){
	matcher.GameMagic:     buylistMagic,
	matcher.GameYuGiOh:    buylistYugioh,
	matcher.GamePokemon:   buylistPokemon,
	matcher.GameOnePiece:  buylistOnePiece,
	matcher.GameGundam:    buylistGundam,
	matcher.GamePalworld:  buylistPalworld,
	matcher.GameRiftbound: buylistRiftbound,
	matcher.GameLorcana:   buylistLorcana,
}

// match matches an offer's card with the printing its condition names, in
// the run of finishes it names, if any.
func (l sellListing) match(b *matcher.Backend, theCard *matcher.InputCard) (string, error) {
	if l.printing != "" {
		theCard.Variation = strings.TrimSpace(theCard.Variation + " " + l.printing)
	}
	if l.run != nil {
		return matchRun(b, theCard, l.run)
	}
	return b.Match(theCard)
}

func (csi *Coolstuffinc) processSearch(ctx context.Context, results chan<- responseChan, itemName string, rarities []string) error {
	skipOOS := !csi.includeOOS
	switch itemName {
	case "Alpha", "Beta", "Unlimited Edition":
		skipOOS = false
	}
	result, err := Search(ctx, csi.client, csi.shelf, itemName, skipOOS, rarities)
	if err != nil {
		return err
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(result.Data))
	if err != nil {
		return fmt.Errorf("page 1: %w", err)
	}

	// An empty next link is the last page, and the only thing that says
	// so: a search whose rows divide evenly into pages ends on a full one
	// and asking past the end answers that page over again.
	next := result.NextLink

	// A shelf that ends on its first page is already read; one that does
	// not is read again from the top at the larger page size.
	if next != "" {
		wide, wideNext := widenSearchPage(ctx, csi.client, next)
		if wide != nil {
			doc = wide
			next = wideNext
		}
	}

	for page := 1; ; page++ {
		doc.Find(searchRowSelector).Each(func(i int, s *goquery.Selection) {
			// The storefront escapes its names twice, so the decode the
			// parser already did leaves the entity still written out:
			// "Fiendish Engine &#937;" is the Omega the datastore spells.
			cardName := html.UnescapeString(s.Find(`span[itemprop="name"]`).Text())

			pid, _ := s.Find(`span[class="rating-display "]`).Attr("data-pid")
			edition := itemName
			notes := s.Find(`div[class="large-8 medium-12 small- 12 product-notes"]`).Text()
			notes = strings.TrimPrefix(notes, "Notes: ")

			// The storefront prints the rarity in the row under the
			// breadcrumb, and for Yu-Gi-Oh it is the only thing that
			// tells apart the printings a set files at one number: the
			// Battle Packs sell the same card as Common and as Mosaic
			// Rare, both numbered alike.
			rarity := strings.TrimSpace(s.Find(`div[class="breadcrumb-trail"]`).Parent().Parent().Next().Text())

			imgURL, _ := s.Find(`a[class="productLink"]`).Find("img").Attr("data-src")
			if imgURL == "" {
				imgURL, _ = s.Find(`a[class="productLink"]`).Find("img").Attr("src")
				if imgURL == "" {
					csi.printf("img not found %s %s", cardName, edition)
				}
			}

			s.Find(`div[itemprop="offers"]`).Each(func(i int, se *goquery.Selection) {
				var relaxed bool
				var graded bool
				fullRow := strings.TrimSpace(se.Text())

				switch {
				case strings.Contains(fullRow, "Out of Stock"),
					strings.Contains(fullRow, "not currently available"):
					return
				}

				qtyStr := se.Find(`span[class="card-qty"]`).Text()
				qtyStr = strings.TrimSpace(strings.TrimSuffix(qtyStr, "+"))
				// If preorder has no quantity,, set max allowed
				if qtyStr == "" && strings.Contains(notes, "Preorder") {
					qtyStr = "20"
				}
				// An offer that only says Preorder is not on sale yet
				if qtyStr == "" && strings.Contains(fullRow, "Preorder") {
					return
				}

				qty, err := strconv.Atoi(qtyStr)
				if err != nil {
					csi.printf("%s", fullRow)
					csi.printf("%s %s %v", cardName, edition, err)
					return
				}

				bundleStr := se.Find(`div[class="b1-gx-free"]`).Text()
				bundleCopies := bundledCopies(bundleStr)

				conditions := offerCondition(fullRow, qtyStr, bundleStr)

				isFoil := strings.HasPrefix(conditions, "Foil")

				slabGrade, slabbed := slabCondition(conditions)
				// Read ahead of the graded rule, which drops the wording: a
				// slab names its printing too ("PSA 9 Pokemon Center").
				printing := conditionPrinting(conditions)
				if isGraded(conditions) {
					conditions = "Near Mint"
					graded = true
				}

				if printing != "" {
					conditions = "Near Mint"
				}

				runFinishes := conditionRun(conditions)
				if runFinishes != nil {
					conditions = "Near Mint"
				}

				if isSkippedCondition(conditions) {
					return
				}

				// Sometimes etched cards have a Near Mint and Near Mint Foil condition
				// for the same card
				if strings.Contains(cardName, "Foil-etched") {
					relaxed = true
				}

				grade, err := mtgban.ParseCondition(strings.TrimPrefix(conditions, "Foil "))
				if err != nil {
					csi.printf("unsupported %s condition", conditions)
					return
				}
				if csiSignedBy.MatchString(cardName) {
					grade = mtgban.HP
				}

				priceStr := se.Find(`b[itemprop="price"]`).Text()
				price, err := strconv.ParseFloat(priceStr, 64)
				if err != nil {
					csi.printf("%v", err)
					return
				}
				if bundleCopies > 1 {
					price /= float64(bundleCopies)
				}

				if price == 0.0 || qty == 0 {
					return
				}

				link := "https://www.coolstuffinc.com/p/" + pid
				if csi.partner != "" {
					link += "?utm_referrer=" + csi.partner
				}

				resolve, found := retailResolvers[csi.backend.Game]
				if !found {
					return
				}
				theCard, cardID, err := resolve(csi, sellListing{
					pid:      pid,
					name:     cardName,
					edition:  edition,
					notes:    notes,
					rarity:   rarity,
					image:    imgURL,
					foil:     isFoil,
					printing: printing,
					run:      runFinishes,
					graded:   graded,
					slabbed:  slabbed,
				})
				if errors.Is(err, matcher.ErrUnsupported) {
					return
				} else if err != nil {
					switch {
					// Ignore expected misses
					case magic.IsBasicLand(theCard.Name),
						notes == "" && strings.Contains(edition, "The List"),
						strings.Contains(notes, "Preorder"),
						csiAmbiguousRefusal(notes, err):
					default:
						csi.printf("%v", err)
						csi.printf("%v", theCard)
						csi.printf("'%s' '%s' '%s'", cardName, edition, notes)
						csi.printf("- %s", link)

						var alias *matcher.AliasingError
						if errors.As(err, &alias) {
							for _, probe := range alias.Probe() {
								card, _ := csi.backend.GetUUID(probe)
								csi.printf("- %s", card)
							}
						}
					}
					return
				}

				out := responseChan{
					cardID: cardID,
					invEntry: &mtgban.InventoryEntry{
						Conditions: grade,
						Price:      price,
						Quantity:   qty,
						URL:        link,
						OriginalID: pid,
						InstanceID: offerRowID(se),
						SellerName: availableMarketNames[0],
					},
					relaxed: relaxed || graded,
				}

				if slabbed {
					out.invEntry.Conditions = slabGrade
					out.invEntry.SellerName = availableMarketNames[2]
				} else if graded {
					out.invEntry.SellerName = availableMarketNames[1]
				}
				results <- out
			})
		})

		if next == "" {
			break
		}

		doc, err = fetchSearchPage(ctx, csi.client, next)
		if err != nil {
			return fmt.Errorf("page %d: %w", page+1, err)
		}
		next = searchNextLink(doc)
	}

	return nil
}

func (csi *Coolstuffinc) scrape(ctx context.Context) error {
	link := csiInventoryURL + csi.shelf
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := csi.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return err
	}

	var itemNames []string
	var rarities []string
	doc.Find(`fieldset`).Each(func(i int, s *goquery.Selection) {
		title := s.Find(`h2[class="mb10"] b`).Text()
		switch title {
		case "Item Set":
			s.Find(`div[class="toggleTable"]`).Find("li").Each(func(j int, se *goquery.Selection) {
				itemName, _ := se.Find(`input[type="checkbox"]`).Attr("value")
				switch {
				case strings.Contains(itemName, "Bulk"),
					strings.Contains(itemName, "Random Lots"),
					strings.Contains(itemName, "Relic Token"),
					itemName == "Magic":
					return
				}

				itemNames = append(itemNames, itemName)
			})
		case "Rarity":
			// Reading the tiers off the page the editions come from
			// spares a fixed list, which would ask every game for
			// Magic's tiers.
			rarities = singlesRarities(s)
		}
	})
	// Sort for predictable results
	sort.Strings(itemNames)

	csi.printf("Found %d items over %d rarity tiers", len(itemNames), len(rarities))

	start := time.Now()

	if csi.targetEdition != "" {
		filtered := itemNames[:0]
		for _, item := range itemNames {
			if item == csi.targetEdition {
				filtered = append(filtered, item)
			}
		}
		itemNames = filtered
	}

	// The search answers a product on every shelf it is filed on, and a
	// Heroines Edition card sits on two, so the same offer arrives twice
	// with one url, one condition and one quantity. The second is the
	// first again, not a second seller, and adding it would only be
	// refused as the duplicate it is.
	seen := map[string]bool{}
	mtgban.WorkerPool(ctx, csi.maxConcurrency, itemNames,
		func(ctx context.Context, itemName string, results chan<- responseChan) error {
			csi.printf("Processing %s", itemName)
			return csi.processSearch(ctx, results, itemName, rarities)
		},
		func(record responseChan) {
			if offerSeen(seen, record) {
				return
			}
			var err error
			if record.relaxed {
				err = csi.inventory.AddRelaxed(record.cardID, record.invEntry)
			} else {
				err = csi.inventory.Add(record.cardID, record.invEntry)
			}
			// The search lists a card once per shelf it sits on, so the
			// same listing arrives twice; the second is the same entry.
			if err != nil && !strings.Contains(err.Error(), "same url, and qty") {
				csi.printf("%s", err.Error())
			}
		},
		csi.printf,
	)

	csi.printf("This operation took %v", time.Since(start))

	csi.inventoryDate = time.Now()

	return nil
}

// offerSeen reports whether this very offer was already collected - the
// same printing at the same condition from the same seller behind one url -
// and records it otherwise. The url and the condition alone do not tell
// offers apart: a product row lists its foil, its graded copies and its
// first edition as further offers of the same url, and every one of them
// is filed at NM.
func offerSeen(seen map[string]bool, record responseChan) bool {
	entry := record.invEntry
	key := strings.Join([]string{entry.URL, record.cardID, string(entry.Conditions), entry.SellerName}, "\x00")
	if seen[key] {
		return true
	}
	seen[key] = true
	return false
}

func (csi *Coolstuffinc) parseBL(ctx context.Context) error {
	edition2id, err := LoadBuylistEditions(ctx, csi.client, csi.shelf)
	if err != nil {
		return err
	}
	csi.printf("Loaded %d editions", len(edition2id))

	products, err := GetBuylist(ctx, csi.client, csi.shelf)
	if err != nil {
		return err
	}
	csi.printf("Found %d products", len(products))

	csi.nonfoilPIDs = map[string]bool{}
	for _, product := range products {
		if product.IsFoil == 0 {
			csi.nonfoilPIDs[product.PID] = true
		}
	}

	resolve, found := buylistResolvers[csi.backend.Game]
	if !found {
		return fmt.Errorf("unsupported game %q", csi.backend.Game)
	}

	for _, product := range products {
		if product.RarityName == "Box" {
			continue
		}

		// Filter by set if needed
		if csi.targetEdition != "" && product.ItemSet != csi.targetEdition {
			continue
		}

		// Build link early to help debug
		u, _ := url.Parse(csiBuylistLink)
		v := url.Values{}
		v.Set("s", csi.shelf)
		v.Set("a", "1")
		v.Set("name", product.Name)
		v.Set("f[]", fmt.Sprint(product.IsFoil))

		id, found := edition2id[product.ItemSet]
		if found {
			v.Set("is[]", id)
		}
		u.RawQuery = v.Encode()
		link := u.String()

		theCard, cardID, err := resolve(csi, product)
		if errors.Is(err, matcher.ErrUnsupported) {
			continue
		} else if err != nil {
			if csiAmbiguousRefusal(product.Notes, err) {
				continue
			}
			csi.printf("error: %v", err)
			csi.printf("original: %q", product)
			csi.printf("preprocessed: %q", theCard)

			var alias *matcher.AliasingError
			if errors.As(err, &alias) {
				for _, probe := range alias.Probe() {
					co, _ := csi.backend.GetUUID(probe)
					csi.printf("- %s", co)
				}
			}
			continue
		}

		buyPrice, err := matcher.ParsePrice(product.Price)
		if err != nil {
			csi.printf("%s error: %s", product.Name, err.Error())
			continue
		}

		var priceRatio, sellPrice float64

		invCards := csi.inventory[cardID]
		for _, invCard := range invCards {
			sellPrice = invCard.Price
			break
		}
		if sellPrice > 0 {
			priceRatio = buyPrice / sellPrice * 100
		}

		for i, deduction := range deductions {
			buyEntry := mtgban.BuylistEntry{
				Conditions: mtgban.DefaultGradeTags[i],
				BuyPrice:   buyPrice * deduction,
				PriceRatio: priceRatio,
				Quantity:   maxBuylistQuantity,
				URL:        link,
				InstanceID: product.PPQID,
				CustomFields: map[string]string{
					"originalProduct": fmt.Sprintf("%+v", product),
				},
			}

			err := csi.buylist.Add(cardID, &buyEntry)
			if errors.Is(err, mtgban.ErrDuplicateEntry) {
				// The buylist names a token once per deck it came in,
				// every one at one price for the one printing.
				continue
			}
			if err != nil {
				csi.printf("%s", err.Error())
				continue
			}
		}
	}

	csi.buylistDate = time.Now()

	return nil
}

// magicPhantomFoilTwin reports whether a Magic buylist row is CSI's flat-
// priced isFoil=1 placeholder for a printing that has no real foil finish.
func magicPhantomFoilTwin(co *matcher.CardObject, isFoil int, hasNonfoilRow bool) bool {
	return isFoil == 1 && hasNonfoilRow && !co.Foil && !co.Etched
}

// SetConfig applies options after the scraper was built. See
// mtgban.ScraperConfig.
func (csi *Coolstuffinc) SetConfig(opt mtgban.ScraperOptions) {
	csi.disableRetail = opt.DisableRetail
	csi.disableBuylist = opt.DisableBuylist
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (csi *Coolstuffinc) Load(ctx context.Context) error {
	var errs []error

	if !csi.disableRetail {
		err := csi.scrape(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %w", mtgban.ErrInventoryLoad, err))
		}
	}

	if !csi.disableBuylist {
		err := csi.parseBL(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: %w", mtgban.ErrBuylistLoad, err))
		}
	}

	return errors.Join(errs...)
}

// Inventory returns what Load collected. See mtgban.Seller.
func (csi *Coolstuffinc) Inventory() mtgban.InventoryRecord {
	return csi.inventory
}

// Buylist returns what Load collected. See mtgban.Vendor.
func (csi *Coolstuffinc) Buylist() mtgban.BuylistRecord {
	return csi.buylist
}

// MarketNames names the sub-sellers this market splits into. See
// mtgban.Market.
func (csi *Coolstuffinc) MarketNames() []string {
	return availableMarketNames
}

// InfoForScraper describes one of the sub-scrapers named above.
func (csi *Coolstuffinc) InfoForScraper(name string) mtgban.ScraperInfo {
	info := csi.Info()
	info.Name = name
	info.Shorthand = name2shorthand[name]
	return info
}

// Info describes this scraper. See mtgban.Scraper.
func (csi *Coolstuffinc) Info() (info mtgban.ScraperInfo) {
	info.Name = "Cool Stuff Inc"
	info.Shorthand = "CSI"
	info.InventoryTimestamp = &csi.inventoryDate
	info.BuylistTimestamp = &csi.buylistDate
	info.CreditMultiplier = 1.25
	info.Game = csi.backend.Game
	return
}

// singlesRarities answers the rarity tiers a singles search should ask for,
// read from the Rarity fieldset of the storefront's advanced search.
func singlesRarities(fieldset *goquery.Selection) []string {
	var rarities []string
	fieldset.Find("li").Each(func(_ int, s *goquery.Selection) {
		rarity, found := s.Find(`input[type="checkbox"]`).Attr("value")
		if !found || rarity == "" {
			return
		}
		if sealedTiers[strings.ToLower(strings.TrimSpace(s.Text()))] {
			return
		}

		rarities = append(rarities, rarity)
	})
	return rarities
}

// sealedTiers names the rarity tiers the storefront files sealed products
// under, spelled as the search page prints them. They are the tiers a
// singles search leaves out; a tier missing from here only lets sealed
// through to be refused later, where a card tier missing from the search
// loses the card outright.
var sealedTiers = map[string]bool{
	"box":  true,
	"pack": true,
}

// csiTreatments spells the patterned holos the catalog's way. Cool Stuff Inc
// calls them foils - "Master Ball Foil" - where the catalog files the pattern
// as what it is, and the difference is not cosmetic: "foil" is a finish the
// matcher already reads, so asking for a Master Ball Alomomola by that name
// answers with the set's reverse holo rather than missing.
var csiTreatments = strings.NewReplacer(
	"Master Ball Foil", "Master Ball Pattern",
	"Poke Ball Foil", "Poke Ball Pattern",
	"Shatterfoil", "Cracked Ice Holo",
	"Cosmo Holo", "Cosmos Holo",
	"Energy Foil", "Energy Symbol Pattern",
)

// catalogTreatment answers the catalog's wording for a treatment the
// storefront spells its own way, and leaves everything else as it stands.
func catalogTreatment(variation string) string {
	return csiTreatments.Replace(variation)
}

// csiRarities spells the storefront's Yu-Gi-Oh rarity names the way the
// catalog does. Only the foil tiers disagree: the storefront drops the
// "Rare" the catalog keeps, writes Starfoil as two words, and drops the
// possessive s from Collector's. Everything else - Common, Rare, Mosaic
// Rare - it already spells alike, so a name absent from this table passes
// through as it stands.
// csiUnknownPrintings names the listings this storefront sells under a
// printing the catalog does not carry, keyed by the name and the shelf it
// sits on.
//
// Each is a rarity the set prints and the card at that number does not.
// Gladiator Beast Octavius exists in Gladiator's Assault as a Secret Rare
// and nothing else, and the Exodia of Limited Pack World Championship 2025
// is one of that set's two Emblazoned rarities and never a plain Secret
// Rare or Ultra Rare - EN000 is the only one of its 21 numbers without the
// plain tiers, which is what says the card is Emblazoned-only rather than
// half-published.
// Each is priced at a quarter against the printing it lands on, $13.00,
// $500.00 and $45.00, which is the price of a card that is not that card.
//
// They are listed one at a time because no rule separates them from a
// decorated rarity a storefront spells shorter: "Secret Rare" for the
// catalog's "Prismatic Secret Rare" is the same shape and is correct.
// Refusing a rarity the card does not carry drops 55 Yu-Gi-Oh listings to
// catch these, and around 35 of those are right.
var csiUnknownPrintings = map[string]bool{
	"Exodia the Forbidden One (Secret Rare)|Limited Pack World Championship 2025": true,
	"Exodia the Forbidden One (Ultra Rare)|Limited Pack World Championship 2025":  true,
	"Gladiator Beast Octavius (Super Rare)|Gladiators Assault":                    true,
}

// unknownPrinting reports a listing that names a printing the catalog does
// not carry, which is a listing worth dropping rather than matching: the
// nearest printing to it is a different card at a different price.
func unknownPrinting(name, edition string) bool {
	return csiUnknownPrintings[name+"|"+edition]
}

// csiColors spells a Duelist League colour the way the catalog files it, for
// the ones this storefront names differently. A league prints one number in
// several colours and nothing else tells them apart, so the word is the whole
// identification.
//
// The spelling has to be swapped rather than added to. "Light Blue" says the
// word blue, which names the blue printing outright, so a listing carrying
// both words names two printings and ties instead of answering one - and
// the storefront's own word says nothing else worth keeping.
var csiColors = strings.NewReplacer("(Light Blue)", "(Silver)")

// catalogColor spells the colour a Yu-Gi-Oh listing names the way the catalog
// files it.
func catalogColor(name string) string {
	return csiColors.Replace(name)
}

// csiSpellings corrects the Yu-Gi-Oh names this storefront misspells. Each
// key is a name no set in the game has and each value is the card it means,
// one slip of the fingers away: a doubled letter, a dropped one, a swapped
// pair. Left as typed they look up nothing at all and the listing goes
// unpriced.
//
// One pair is not a slip: the storefront writes the Greek letter Konami
// prints on the card where the catalog writes the word for it. The catalog
// is what has to be asked, so it is corrected here alongside the typos
// rather than argued with.
//
// The pairs are spelled out rather than found by nearest match. A catalog
// tells its numbered siblings apart by a single character - "Armed Dragon
// LV3" against LV5, "Harpie Lady 1" against 2 - and 255 pairs of names
// inside one set are a single edit apart for that reason, so a reader that
// corrects by distance can answer a card the datastore is merely missing
// with its neighbour. A table cannot.
var csiSpellings = map[string]string{
	"Belial - Marqis of Darkness":   "Belial - Marquis of Darkness",
	"Compulsory Evactuation Device": "Compulsory Evacuation Device",
	"Doube-Edged Sword Technique":   "Double-Edged Sword Technique",
	"Fearl Imp":                     "Feral Imp",
	"Fiendish Engine Ω":             "Fiendish Engine Omega",
	"Homumculus the Alchemic Being": "Homunculus the Alchemic Being",
	"Miracle Jurrassic Egg":         "Miracle Jurassic Egg",
	"Perfect Synch - A-Un":          "Perfect Sync - A-Un",
	"Rush Recklessely":              "Rush Recklessly",
	"Sealing Ceremony of Mokuten":   "Sealing Ceremony of Mokuton",
	"Sealing Cermony of Raiton":     "Sealing Ceremony of Raiton",
}

// catalogSpelling spells a Yu-Gi-Oh name the way the catalog does, where this
// storefront has typed it wrong.
//
// A set that files one card under several printings gets the printing hung
// behind the name in brackets, and the name in front of it is typed the same
// wrong way as on the listings that carry no bracket. The correction reaches
// the head of the line for that reason, leaving the bracket to the matcher,
// which reads it into the variation either way. No name in the catalog begins
// with any key in the table, so a head that matches one is the typo and never
// the opening of a longer card name.
func catalogSpelling(name string) string {
	if spelled, found := csiSpellings[name]; found {
		return spelled
	}
	head, printing, bracketed := strings.Cut(name, " (")
	if !bracketed {
		return name
	}
	spelled, found := csiSpellings[head]
	if !found {
		return name
	}
	return spelled + " (" + printing
}

var csiRarities = map[string]string{
	"Star Foil":      "Starfoil Rare",
	"Shatterfoil":    "Shatterfoil Rare",
	"Collector Rare": "Collector's Rare",
}

// csiPrintRuns names both runs a Yu-Gi-Oh edition was printed in, keyed by
// the edition the storefront publishes. It sells both under that one name
// and tells them apart the only way anybody can, by the copyright date
// printed on the card, which it writes into the note. Nothing downstream can
// pick between them - the later run reissues the original's numbers - which
// is why matcher's Yu-Gi-Oh edition aliases leave these sets out on
// purpose, and why naming only one of the two would file the other under it.
//
// The earlier run is spelled out even where the storefront's own name
// already reaches it, so the table states the whole answer rather than
// leaning on a name that happens to match. Two of them do not match: the
// catalog keeps a "The" the storefront drops, and numbers Retro Pack without
// the 1 it sells the pack as. Both were settled by the buylist's own
// collector numbers, which write the earlier run plain ("LOB-001") and the
// later one with the language ("LOB-EN015").
var csiPrintRuns = map[string][2]string{
	"Dark Crisis":                      {"Dark Crisis", "Dark Crisis (25th Anniversary Edition)"},
	"Invasion of Chaos":                {"Invasion of Chaos", "Invasion of Chaos (25th Anniversary Edition)"},
	"Legend of Blue Eyes White Dragon": {"The Legend of Blue Eyes White Dragon", "Legend of Blue Eyes White Dragon (25th Anniversary Edition)"},
	"Light of Destruction":             {"Light of Destruction", "Light of Destruction (2020 Date Reprint)"},
	"Metal Raiders":                    {"Metal Raiders", "Metal Raiders (25th Anniversary Edition)"},
	"Pharaohs Servant":                 {"Pharaoh's Servant", "Pharaoh's Servant (25th Anniversary Edition)"},
	"Retro Pack 1":                     {"Retro Pack", "Retro Pack (2020 Date Reprint)"},
	"Retro Pack 2":                     {"Retro Pack 2", "Retro Pack 2 (2020 Date Reprint)"},
	"Spell Ruler":                      {"Spell Ruler", "Spell Ruler (25th Anniversary Edition)"},
}

// printRunEdition answers the edition the note's copyright date names. An
// edition sold in one run, or a note that says no date, is left as it is.
func printRunEdition(edition, notes string) string {
	runs, found := csiPrintRuns[edition]
	if !found || !strings.Contains(notes, "Copyright") {
		return edition
	}
	if strings.Contains(notes, "2020") {
		return runs[1]
	}
	return runs[0]
}

// catalogRarity answers the catalog's name for a rarity the storefront
// prints, so the rarity tier can read it.
func catalogRarity(rarity string) string {
	if spelled, found := csiRarities[rarity]; found {
		return spelled
	}
	return rarity
}

// reOfferRow reads an offer's Add to Cart button, atc[<product>][<row>].
var reOfferRow = regexp.MustCompile(`^atc\[\d+\]\[(\d+)\]$`)

// offerRowID is the id CSI's store cart adds an offer row by, one per
// condition and finish of a product.
func offerRowID(offer *goquery.Selection) string {
	m := reOfferRow.FindStringSubmatch(offer.Find("[data-atc]").AttrOr("data-atc", ""))
	if m == nil {
		return ""
	}
	return m[1]
}

// imageStem reads the file name off a product image, whatever the
// storefront pictures in.
var imageStem = regexp.MustCompile(`(?i)/([^/]+)\.(?:jpe?g|png|webp|avif)$`)

// skuNumbers indexes a datastore's collector numbers by the spelling an image
// sku gives them, upper-cased with the dash left out, each in the order the
// datastore lists its cards.
func skuNumbers(b *matcher.Backend) map[string][]string {
	index := map[string][]string{}
	for _, uuid := range b.GetUUIDs() {
		co, err := b.GetUUID(uuid)
		if err != nil {
			continue
		}
		key := strings.ToUpper(strings.ReplaceAll(co.Number, "-", ""))
		if !slices.Contains(index[key], co.Number) {
			index[key] = append(index[key], co.Number)
		}
	}
	return index
}
