// Package cardmarket scrapes Cardmarket, both the price-guide index and
// sealed product, across every game they carry.
package cardmarket

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	cm "github.com/mtgban/go-cardmarket"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
)

const (
	defaultConcurrency = 8
)

type responseChan struct {
	ogID   int
	cardID string
	entry  mtgban.InventoryEntry
	// byName marks a price whose printing was named rather than looked
	// up by id, which is a guess however well guarded; see namedLast.
	byName bool
	// owned marks a price for a printing the datastore files under this
	// very product, which namedLast prefers among products pricing it in
	// as many columns.
	owned bool
	// product is what was priced, for the collector to tell a twin of a
	// product already priced from a disagreement worth reporting.
	product *cm.Product
	// tally carries an edition's walked and refused counts in place of a
	// price, one record per edition, so the pool's single collector can
	// count the run without the workers sharing anything.
	tally   bool
	walked  int
	refused int
	// foreign is how many of the refused were products of a catalog we do
	// not carry, which the run reports as its own figure: they are shelves
	// nobody can act on, where the rest of the refusals are work.
	foreign int
	// unpriced lists the products that resolved but had no guide row yet,
	// which are no refusal: the guide trails the product list by a day.
	unpriced []int
}

// namedLast holds back the prices whose printing was named until every
// price looked up by an id is in.
//
// The catalogs keep one expansion per old set where the datastore keeps a
// printing per print run, so the set guard puts every run's product on the
// base set's printing, and a run the bridge already priced is a run a name
// can reach too. AddUnique keeps whichever price arrives first, so without
// the wait the winner is whichever expansion the pool happened to walk
// first - and half the time that hands a verified printing over to a guess
// about a different one. Waiting decides it instead: the guess is offered
// only where nothing verified prices the printing in as many columns.
//
// A printing is priced by one product, in every column. The product pricing
// it in the most columns holds it, the first to reach it among equals, and
// any other gives way, so the Low and Trend shelves name the same product
// for a card and never mix the prices of two.
type namedLast struct {
	add     func(responseChan)
	results []responseChan
	// held is the product pricing each printing so far, keyed by uuid.
	held map[string]holder
	// twin says whether two products are the same card sold twice, for
	// the games whose shelves do that.
	twin func(a, b *cm.Product) bool
	// face says whether a product names one face of the fused printing
	// it is beside, the other way a shelf sells one card twice
	face  func(product *cm.Product, cardID string) bool
	twins int
	// clash hears of a price that gave way to another product that is
	// neither its twin nor its face; nil only counts it.
	clash   func(result responseChan, held int)
	clashes int
	// The run's tally, summed from the editions' records; the collector
	// runs on one goroutine, so plain counts are all this takes.
	walked  int
	refused int
	foreign int
	// unpriced gathers the editions' products the guide does not price yet.
	unpriced []int
}

// collect takes one result, holding it back or counting it into the run's
// tally. Every price waits for flush, the named ones and the rest alike:
// the pool walks the expansions in whatever order they finish, and which of
// two products reaching one printing arrived first would otherwise be
// decided by that, so a promo sold on two shelves took one shelf's price
// this run and the other's next. flush puts them in the catalog's order.
func (n *namedLast) collect(result responseChan) {
	if result.tally {
		n.walked += result.walked
		n.refused += result.refused
		n.foreign += result.foreign
		n.unpriced = append(n.unpriced, result.unpriced...)
		return
	}
	n.results = append(n.results, result)
}

// holder is the product a printing is priced by.
type holder struct {
	ogID    int
	product *cm.Product
}

// hold records the product pricing a printing, and reports whether the
// price is that product's. A twin or a face of the holder gives way
// silently, being the same card sold again; any other product is a second
// guess at the printing, and clash hears of it.
func (n *namedLast) hold(result responseChan) bool {
	if n.held == nil {
		n.held = map[string]holder{}
	}
	held, found := n.held[result.cardID]
	if !found {
		n.held[result.cardID] = holder{ogID: result.ogID, product: result.product}
		return true
	}
	if held.ogID == result.ogID && held.product == result.product {
		return true
	}
	if held.product != nil && result.product != nil &&
		((n.twin != nil && n.twin(held.product, result.product)) || (n.face != nil && n.face(result.product, result.cardID))) {
		n.twins++
		return false
	}
	n.clashes++
	if n.clash != nil {
		n.clash(result, held.ogID)
	}
	return false
}

// flush adds everything held back: the prices of the product pricing a
// printing in the most columns first, then of its own product, then those
// looked up by id, then the named ones, each in the order of the catalog -
// expansion, then product - and reports how many named prices went in and
// how many gave way to a twin already priced.
func (n *namedLast) flush() (added, twins int) {
	// A product with nothing in a column would empty it, and a foil-only
	// product's plain Low is its foil one, so the fuller product holds.
	type priced struct {
		ogID   int
		cardID string
	}
	columns := map[priced]int{}
	for _, result := range n.results {
		columns[priced{result.ogID, result.cardID}]++
	}
	sort.SliceStable(n.results, func(i, j int) bool {
		a, b := n.results[i], n.results[j]
		ca, cb := columns[priced{a.ogID, a.cardID}], columns[priced{b.ogID, b.cardID}]
		if ca != cb {
			return ca > cb
		}
		if a.owned != b.owned {
			return a.owned
		}
		if a.byName != b.byName {
			return !a.byName
		}
		return productBefore(a.product, b.product)
	})
	before := n.twins
	for i := range n.results {
		if !n.hold(n.results[i]) {
			continue
		}
		n.add(n.results[i])
		if n.results[i].byName {
			added++
		}
	}
	return added, n.twins - before
}

// productBefore orders two products the way the catalog files them, by
// expansion and then by product id, with a result carrying no product last.
func productBefore(a, b *cm.Product) bool {
	if a == nil || b == nil {
		return a != nil && b == nil
	}
	if a.Expansion.IDExpansion != b.Expansion.IDExpansion {
		return a.Expansion.IDExpansion < b.Expansion.IDExpansion
	}
	if a.ExpansionName != b.ExpansionName {
		return a.ExpansionName < b.ExpansionName
	}
	return a.IDProduct < b.IDProduct
}

// Index prices singles from Cardmarket's price guide, the low and
// trend numbers rather than any one seller's listing.
type Index struct {
	resolver

	logCallback    mtgban.LogCallbackFunc
	inventoryDate  time.Time
	affiliate      string
	maxConcurrency int
	exchangeRate   float64

	inventory mtgban.InventoryRecord

	// priceGuide holds one game's published prices, indexed by the product
	// id they belong to: a run asks for one product's prices tens of
	// thousands of times, once per product in the catalog.
	priceGuide map[int]cm.PriceGuide
}

var availableIndexNames = []string{
	"MKM Low", "MKM Trend",
}

var name2shorthand = map[string]string{
	"MKM Low":   "MKMLow",
	"MKM Trend": "MKMTrend",
}

// mkmGames is the only way into these scrapers: a game names its Cardmarket
// id here or it is not one this package is read for.
var mkmGames = map[matcher.Game]cm.Game{
	matcher.GameMagic:         cm.GameMagic,
	matcher.GameLorcana:       cm.GameLorcana,
	matcher.GameRiftbound:     cm.GameRiftbound,
	matcher.GameOnePiece:      cm.GameOnePiece,
	matcher.GameYuGiOh:        cm.GameYuGiOh,
	matcher.GameFleshAndBlood: cm.GameFleshAndBlood,
	matcher.GamePokemon:       cm.GamePokemon,
	matcher.GameGundam:        cm.GameGundam,
}

// defaultArticleFilter is the filter a price is read through: played or
// better, from a seller with a record, neither signed nor altered, and in
// English. Anything looser prices a card off a listing nobody would buy.
// It is a value, so a caller wanting another language copies it and sets
// Language.
var defaultArticleFilter = cm.ArticleQuery{
	MinCondition: cm.ConditionGood,
	MinUserScore: cm.UserScoreGood,
	Language:     cm.LanguageEnglish,
	Signed:       cm.None,
	Altered:      cm.None,
}

// onlyIf narrows a link to the listings carrying a flag the article carries,
// and leaves the listings alone where it does not. A false flag is not the
// same as "show me the ones without it": that is cm.None, and saying so is a
// decision rather than a translation of an article's own boolean.
func onlyIf(flag bool) cm.Filter {
	if flag {
		return cm.Only
	}
	return cm.Any
}

func (mkm *Index) printf(format string, a ...any) {
	if mkm.logCallback != nil {
		mkm.logCallback("[MKMIndex] "+format, a...)
	}
}

// NewScraperIndex returns an index scraper matching against b. It prices
// from the published catalog and the public price guide, so it needs no
// credential.
func NewScraperIndex(b *matcher.Backend) (*Index, error) {
	game := b.Game
	id, found := mkmGames[game]
	if !found {
		return nil, fmt.Errorf("unsupported game %q", game)
	}
	mkm := Index{}
	mkm.inventory = mtgban.InventoryRecord{}
	mkm.maxConcurrency = defaultConcurrency
	mkm.gameID = id
	mkm.resolver.backend = b
	mkm.resolver.printf = mkm.printf
	return &mkm, nil
}

// versionTail matches the parenthetical Cardmarket tells same-name products
// apart with, which names its own version index and the rarity beside it.
var versionTail = regexp.MustCompile(` \(V\.\d+.*\)$`)

// rarityTail captures the rarity out of that same parenthetical, which is
// the only place Cardmarket writes it.
var rarityTail = regexp.MustCompile(` \(V\.\d+ - ([^)]+)\)$`)

// numberTail matches the digits a collector number ends on, which is the part
// two catalogs numbering the same card agree about.
var numberTail = regexp.MustCompile(`\d+[A-Za-z]?$`)

// nameCode matches the collector number Cardmarket writes at the end of a One
// Piece product name, beside the one it writes in the number field.
var nameCode = regexp.MustCompile(`\(([A-Za-z]+[0-9]*-[0-9]+[a-zA-Z]*)\)$`)

// onePieceNumber picks the collector number a One Piece product is asked
// with, out of the two Cardmarket writes for it.
//
// The number field is sometimes another card's ("Sanji (OP01-013)" filed
// under ST13-016, "Corrida Coliseum (OP04-096)" under OP04-092) and
// sometimes a typo of the name's own code with a digit doubled ("ST13-0151",
// "P-0611"). The code inside the name is the card's, so it answers for the
// product when the two disagree.
//
// Only on a shelf naming a set of ours, though. Cardmarket sells its promos
// in buckets no set of ours answers for - "Judge Promos", "Winner Cards",
// "Premium Bandai Products" - and with nothing to hold it to a set the
// matcher reaches past the edition and lands on the ordinary booster
// printing of that number, which is a mass-printed card wearing a promo's
// price. A refusal says less but claims nothing.
func onePieceNumber(b *matcher.Backend, name, number, expansion string) string {
	fields := nameCode.FindStringSubmatch(name)
	if fields == nil || strings.EqualFold(fields[1], number) {
		return number
	}
	if _, err := b.GetSetByName(expansion); err != nil {
		return number
	}
	return fields[1]
}

// shelvedSets names, for each set of ours, the one expansion of this catalog
// that sells it. A set no expansion names is absent, and so is an expansion
// naming no set.
func shelvedSets(b *matcher.Backend, list []cm.Expansion) map[string]string {
	shelved := make(map[string]string, len(list))
	for _, exp := range list {
		set, err := b.GetSetByName(cmp.Or(onePieceShelves[exp.Name], exp.Name))
		if err != nil {
			continue
		}
		shelved[set.Code] = exp.Name
	}
	return shelved
}

// numberPrefix returns the letters a collector number opens on, the set code
// aside: "EN005" yields "EN", "DCR-005" yields "", "SGX1-END19" yields "END".
func numberPrefix(number string) string {
	if _, tail, dashed := strings.Cut(number, "-"); dashed {
		number = tail
	}
	return strings.ToUpper(number[:len(number)-len(strings.TrimLeft(number, letters))])
}

// letters spells the alphabet a collector number's prefix is drawn from.
const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// otherPrintRun reports whether a product's collector number names a print
// run other than the one the answer's number belongs to.
//
// Cardmarket sells the North American print of an old Yu-Gi-Oh set ("005"),
// the European ("EN005") and the Asian ("A005") as three products of one
// expansion, and files the special-edition promos beside them ("SP1"), where
// the datastore carries the single row the set is numbered by ("DCR-005").
// The matcher reads none of those prefixed numbers - its own numbering opens
// on digits or on a set code - so it drops them and answers the name alone,
// which puts every run on that one row. The prefix has to be the answer's
// own, the region infix the datastore writes and Cardmarket omits aside
// ("D19" is "SGX1-END19", "EN005" is not "DCR-005").
func otherPrintRun(number, full string) bool {
	prefix := numberPrefix(number)
	return prefix != "" && !strings.HasSuffix(numberPrefix(full), prefix)
}

// foreignShelves are the tails Cardmarket appends to a set's name when it
// shelves that set's non-English printings apart from the English ones. They
// are separate catalogs of the same cards, and the datastore carries only the
// English ones, so a price from one of these shelves would land on a printing
// it is not. The three-letter codes are the European language prints of the
// oldest Yu-Gi-Oh sets: "Starter Deck: Kaiba (DDK)" numbers its cards F036
// and C039.
var foreignShelves = []string{
	"(Japanese)", "(Korean)", "(PMT)",
	"(LDB)", "(LDD)", "(LDC)", "(LDI)", "(SDF)", "(SDP)", "(MDM)", "(SDH)", "(SDM)",
	"(BIJ)", "(DDJ)", "(DIJ)", "(MIJ)", "(BIK)", "(DDK)", "(DIK)", "(MIK)",
	"(BIP)", "(DDP)", "(DIP)", "(MIP)", "(BIY)", "(DDY)", "(DIY)", "(MIY)",
}

// foreignShelf reports whether an expansion name wears one of those tails.
func foreignShelf(name string) bool {
	for _, tail := range foreignShelves {
		if strings.HasSuffix(name, tail) {
			return true
		}
	}
	return false
}

// fabForeignPrograms are the Flesh and Blood expansions LSS printed in no
// English edition: the DE/ES/FR/IT Black Label History Packs and the JA
// Archive packs.
var fabForeignPrograms = map[string]bool{
	"1HP-BL": true, "2HP-BL": true, "RAP": true, "MAP": true, "GAP": true,
}

// foreignExpansion reports whether an expansion is a non-English catalog the
// datastore does not carry, which the walks drop before resolving.
func foreignExpansion(gameID cm.Game, exp cm.Expansion) bool {
	switch gameID {
	case cm.GameOnePiece, cm.GameYuGiOh:
		return strings.HasSuffix(exp.SetCode, "-JP") || foreignShelf(exp.Name)
	case cm.GameFleshAndBlood:
		return fabForeignPrograms[exp.SetCode]
	case cm.GameRiftbound:
		return exp.SetCode == riftboundChineseShelf
	}
	return false
}

// errNoGuide marks a resolved product the price guide has no row for yet.
var errNoGuide = errors.New("not in the price guide")

// emitPrices lands a product's guide prices on the printings resolved for
// it, the plain columns on one and the foil columns on the other, in the
// games that split them.
func (mkm *Index) emitPrices(channel chan<- responseChan, product *cm.Product, cardID, cardIDFoil string, byName bool) error {
	// Look for the price presence
	guide, found := mkm.priceGuide[product.IDProduct]
	if !found {
		return errNoGuide
	}

	// Sorted as availableIndexNames
	prices := []float64{guide.LowPrice, guide.TrendPrice}
	foilLow, foilTrend := guide.SecondPrinting(mkm.gameID)
	foilprices := []float64{foilLow, foilTrend}

	co, err := mkm.backend.GetUUID(cardID)
	if err != nil {
		return err
	}

	// A catalog that gives each treatment its own product prices one
	// printing per product, and the product's own columns are that
	// printing's whatever its finish - there is no second column for them
	// to be in. Every other catalog keeps a second printing beside the
	// first and splits the two across the columns: the foil beside the
	// plain card, or in Pokemon's guide the reverse holo beside whatever
	// the card's own printing is, holo or plain or a print run. The finish
	// the loader stored says which side of that split the printing is on;
	// the foil flag cannot, a Pokemon holo being a foil to the flag and a
	// printing of its own to the guide - read by the flag, the holos would
	// be priced from the reverse's columns and the reverses from nothing.
	perTreatment := mkm.gameID == cm.GameFleshAndBlood || mkm.gameID == cm.GameOnePiece
	// Yu-Gi-Oh's second pair is a lone trend-foil that has stopped moving
	// and is not the 1st Edition's price, so only the product's own is read.
	onePair := perTreatment || mkm.gameID == cm.GameYuGiOh
	// A product that never sold a plain copy and lists its cheapest foil as
	// its Low sells foils only, and that Low is the foil's, unless the
	// printing has no foil of its own for that Low to belong to.
	if mkm.gameID == cm.GameMagic && cardIDFoil != "" && cardIDFoil != cardID &&
		guide.TrendPrice == 0 && guide.AvgSellPrice == 0 && guide.AvgDay30 == 0 && guide.LowPrice == foilLow {
		prices[0] = 0
	}
	second := co.Finish != matcher.FinishNonfoil
	if mkm.gameID == cm.GamePokemon {
		// Reverse holo is the guide's only split: it has no 1st-Edition
		// column, unlike Market's per-listing query (see
		// pokemonFinishPlan), so a 1st Edition printing reads as "not
		// the second column" here regardless of print run. The
		// print-run axis is handled below instead, by filing the first
		// pair under every uuid on this product's "not reverse holo"
		// side.
		second = co.Finish == matcher.FinishSlug(pokemonReverseHolo)
	}

	// A printing on the first side takes the first pair and hands the
	// second pair to the printing beside it; one on the second side is
	// priced by the second pair alone.
	if onePair || !second {
		link := cm.BuildURL(mkm.gameID, product.IDProduct, cm.URLOption{
			Signed:    cm.None,
			Altered:   cm.None,
			Language:  cm.LanguageEnglish,
			Affiliate: mkm.affiliate,
		})

		// The first pair's target(s): cardID alone for every other game,
		// but for Pokemon the guide's low/trend blend every listing of the
		// product that is not specifically Reverse Holofoil - Unlimited,
		// 1st Edition, and both their Holofoil crossings all read as one
		// number here, because the guide has no column of its own for the
		// print-run axis at all. Filing the same blended number under
		// every one of those uuids beats filing it under only one and
		// leaving the rest unpriced, even though it cannot separate
		// what a live listing would: Market's per-listing query tells
		// 1st Edition from Unlimited (see pokemonFinishPlan) by asking
		// Cardmarket per finish, but the price guide never breaks the
		// two out, so there is no better number to give either
		// one here.
		targets := []string{cardID}
		if mkm.gameID == cm.GamePokemon {
			for _, target := range pokemonFinishPlan(mkm.backend, cardID) {
				if target.isReverseHolo || target.cardID == cardID {
					continue
				}
				targets = append(targets, target.cardID)
			}
		}

		for _, id := range targets {
			for i := range availableIndexNames {
				if prices[i] == 0 {
					continue
				}

				out := responseChan{
					ogID:    product.IDProduct,
					product: product,
					cardID:  id,
					byName:  byName,
					owned:   mkm.owns(product, id),
					entry: mtgban.InventoryEntry{
						Conditions: mtgban.NM,
						Price:      prices[i] * mkm.exchangeRate,
						URL:        link,
						SellerName: availableIndexNames[i],
						OriginalID: fmt.Sprint(product.IDProduct),
					},
				}

				channel <- out
			}
		}

		if !onePair && (foilprices[0] != 0 || foilprices[1] != 0) {
			link := cm.BuildURL(mkm.gameID, product.IDProduct, cm.URLOption{
				Foil:      cm.Only,
				Signed:    cm.None,
				Altered:   cm.None,
				Language:  cm.LanguageEnglish,
				Affiliate: mkm.affiliate,
			})

			// An empty foil id means the card has no foil printing (Match
			// errored on the foil probe), so residual foil prices in the
			// guide have nothing to attach to
			if cardIDFoil != "" && cardID != cardIDFoil {
				for i := range availableIndexNames {
					if foilprices[i] == 0 {
						continue
					}
					out := responseChan{
						ogID:    product.IDProduct,
						product: product,
						cardID:  cardIDFoil,
						byName:  byName,
						owned:   mkm.owns(product, cardIDFoil),
						entry: mtgban.InventoryEntry{
							Conditions: mtgban.NM,
							Price:      foilprices[i] * mkm.exchangeRate,
							URL:        link,
							SellerName: availableIndexNames[i],
							OriginalID: fmt.Sprint(product.IDProduct),
						},
					}

					channel <- out
				}
			}
		}
	} else {
		link := cm.BuildURL(mkm.gameID, product.IDProduct, cm.URLOption{
			Foil:      cm.Only,
			Signed:    cm.None,
			Altered:   cm.None,
			Language:  cm.LanguageEnglish,
			Affiliate: mkm.affiliate,
		})

		for i := range availableIndexNames {
			if foilprices[i] == 0 {
				continue
			}
			out := responseChan{
				ogID:    product.IDProduct,
				product: product,
				cardID:  cardID,
				byName:  byName,
				owned:   mkm.owns(product, cardID),
				entry: mtgban.InventoryEntry{
					Conditions: mtgban.NM,
					Price:      foilprices[i] * mkm.exchangeRate,
					URL:        link,
					SellerName: availableIndexNames[i],
					OriginalID: fmt.Sprint(product.IDProduct),
				},
			}

			channel <- out
		}
	}

	return nil
}

// owns reports whether the datastore files cardID's printing under this
// product's id, the strongest word there is on which product prices it:
// another printing's foil column can reach it too, through a foil sibling
// picked by finish alone.
func (mkm *Index) owns(product *cm.Product, cardID string) bool {
	co, err := mkm.backend.GetUUID(cardID)
	if err != nil {
		return false
	}
	return co.Identifiers["mcmId"] == fmt.Sprint(product.IDProduct)
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (mkm *Index) Load(ctx context.Context) error {
	err := mkm.checkCatalog()
	if err != nil {
		return err
	}

	rate, err := mtgban.GetExchangeRate(ctx, "EUR")
	if err != nil {
		return err
	}
	mkm.exchangeRate = rate

	priceGuide, err := cm.DownloadPriceGuide(ctx, mkm.gameID)
	if err != nil {
		return err
	}
	// An empty guide is a download that went wrong, not a day with no
	// prices, and walking the catalog against it reports every product as
	// merely not priced yet.
	if len(priceGuide) == 0 {
		return errors.New("empty price guide")
	}
	mkm.priceGuide = make(map[int]cm.PriceGuide, len(priceGuide))
	for _, entry := range priceGuide {
		mkm.priceGuide[entry.IDProduct] = entry
	}

	mkm.printf("Obtained today's price guide with %d prices", len(priceGuide))

	return mkm.walkCatalog(ctx)
}

// collectPrices runs worker over every expansion and files what it produces
// into the inventory, prices whose printing was named last. It is where the
// wait namedLast describes is actually taken: the pool hands its results to
// the collector rather than to the inventory, so a named price cannot win a
// printing merely by being walked first.
func (mkm *Index) collectPrices(ctx context.Context, items []cm.Expansion, worker func(context.Context, cm.Expansion, chan<- responseChan) error) (walked, refused, foreign int) {
	// The bridge is keyed by the Cardmarket id and valued by the TCGplayer
	// one, and a cardtrader blueprint names every Cardmarket product it
	// sells as, so nothing stops two products from resolving to one
	// printing. namedLast keeps the one pricing it in the most columns,
	// catalog order among equals, so the inventory sees one product per
	// printing and one price per column.
	collector := namedLast{
		add: func(result responseChan) {
			err := mkm.inventory.AddUnique(result.cardID, &result.entry)
			if err != nil {
				mkm.printf("%d - %s", result.ogID, err.Error())
			}
		},
		twin: sameProduct(mkm.gameID),
		face: faceOf(mkm.backend, mkm.gameID),
	}
	// A second product on a printing is rare enough in these games to be
	// worth a line each. Elsewhere it is counted: Magic's are mostly V.N,
	// Extras and reprint shelves the datastore has no printing for.
	switch mkm.gameID {
	case cm.GameYuGiOh, cm.GameFleshAndBlood, cm.GamePokemon:
		collector.clash = func(result responseChan, held int) {
			mkm.printf("%d - gives way to %d on %s", result.ogID, held, result.cardID)
		}
	}

	mtgban.WorkerPool(ctx, mkm.maxConcurrency, items, worker, collector.collect, mkm.printf)

	added, _ := collector.flush()
	mkm.printf("Adding %d prices whose printing was named", added)
	if collector.twins > 0 {
		mkm.printf("%d prices gave way to a product of the same name already priced", collector.twins)
	}
	if collector.clashes > 0 {
		mkm.printf("%d prices gave way to another product already pricing the printing", collector.clashes)
	}
	if len(collector.unpriced) > 0 {
		slices.Sort(collector.unpriced)
		mkm.printf("%d product(s) not in the price guide yet: %v", len(collector.unpriced), collector.unpriced)
	}
	return collector.walked, collector.refused, collector.foreign
}

// Inventory returns what Load collected. See mtgban.Seller.
func (mkm *Index) Inventory() mtgban.InventoryRecord {
	return mkm.inventory
}

// MarketNames names the sub-sellers this market splits into. See
// mtgban.Market.
func (mkm *Index) MarketNames() []string {
	return availableIndexNames
}

// InfoForScraper describes one of the sub-scrapers named above.
func (mkm *Index) InfoForScraper(name string) mtgban.ScraperInfo {
	info := mkm.Info()
	info.Name = name
	info.Shorthand = name2shorthand[name]
	return info
}

// Info describes this scraper. See mtgban.Scraper.
func (mkm *Index) Info() (info mtgban.ScraperInfo) {
	info.Name = "Card Market Index"
	info.Shorthand = "MKMIndex"
	info.CountryFlag = "EU"
	info.InventoryTimestamp = &mkm.inventoryDate
	info.MetadataOnly = true
	info.Family = "MKM"
	info.Game = mkm.backend.Game
	return
}
