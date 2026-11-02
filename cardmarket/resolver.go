package cardmarket

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	cm "github.com/mtgban/go-cardmarket"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/matcher/magic"
)

// resolver is the Cardmarket-id -> mtgban-uuid resolution Index and Market
// share: naming the printing(s) a product's fields (name, number, expansion)
// belong to, id-map first and then name/number matching, per game. Neither
// scraper differs in how hard that problem is - each game's shelving quirks,
// twins, foreign catalogs and print runs are the same whichever scraper asks
// - they differ only in what they do with an answer once they have one:
// Index reads it off the published price guide, Market queries the
// product's own live listings.
//
// Embedded anonymously, not held by reference: a scraper's own field access
// (mkm.tcgBridge = ...) and its own calls (mkm.resolveProduct(...)) both
// promote through unchanged, so this extraction changed no call site in
// Index - only where the fields and methods are defined.
type resolver struct {
	// TCGBridge maps a Cardmarket product id to the TCGplayer id of the
	// same single, for the catalogs whose same-name products are told
	// apart only by an exact id: Yu-Gi-Oh and Flesh and Blood carry no
	// collector number, Pokemon sells one card on shelf after shelf, and
	// One Piece files the printings a number cannot tell apart under a
	// version index of its own. bantool builds it from cardtrader's
	// blueprints, the one source linking the two marketplaces; the scraper
	// itself stays vendor-pure and receives it as plain data.
	tcgBridge map[int]int

	// TargetEdition optionally restricts resolution to a single edition.
	targetEdition string

	// Catalog is the published id-map catalog a scraper may resolve from
	// before falling back to name/number matching. bantool loads it from
	// CARDMARKET_CATALOG_PATH; nil is a resolver with no id map at all, which
	// checkCatalog refuses to walk when the game needs one to walk safely.
	catalog *cm.Catalog

	// numbers indexes a set's collector numbers by the card they name,
	// built on first use; see yugiohNumberTaken.
	numbers   map[string]map[string]string
	numbersMu sync.Mutex

	// mcmPrintings indexes Magic's printings by the Cardmarket product id
	// the datastore links them to, built on first use; see magicPrintings.
	mcmPrintings   map[int][]string
	mcmPrintingsMu sync.Mutex

	// foilApart holds, for each expansion of foilApartShelves, the numbers
	// whose foil is a "(V.2)" product of its own, built on first use; see
	// soldFoilApart.
	foilApart   map[int]map[string]bool
	foilApartMu sync.Mutex

	// fabDeckSets indexes a Flesh and Blood set's collector-number prefix
	// onto every set that opens numbers on it, built on first use; see
	// fabShelves and fabDeckSetIndex.
	fabDeckSets   map[string][]*matcher.Set
	fabDeckSetsMu sync.Mutex

	// shelved names, for each set of ours, the expansion of this run that
	// sells it; see offShelf. A scraper's Load fills it once the
	// expansions are known, via shelvedSets.
	shelved map[string]string

	// claimed holds the One Piece printings some product of the run is
	// answered with by id; see claimByID.
	claimed map[string]bool

	// foilVersions holds the One Piece products the bridge answers with
	// the foil of their printing; see foilVersions.
	foilVersions map[int]bool

	// foilSold holds the foil printings those products are answered with.
	foilSold map[string]bool

	// printf logs through the owning scraper's own prefix. Left nil, calls
	// through it are no-ops - a resolver built for a test rather than a
	// live scraper needs none.
	printf func(format string, a ...any)

	gameID cm.Game

	// backend is the datastore Index and Market match against, set by
	// their constructors.
	backend *matcher.Backend
}

func (r *resolver) logf(format string, a ...any) {
	if r.printf != nil {
		r.printf(format, a...)
	}
}

// productLinks names the TCGplayer product of a Cardmarket product whose
// CardTrader blueprint links no TCGplayer id or the wrong one. Cardmarket's
// product ids are global, so one table serves every game.
var productLinks = map[int]int{
	905979: 616641, // Gundam Char's Zaku II, Rare +
	905980: 616640, // Gundam Char's Zaku II, Rare
	915837: 721891, // Lorcana Q3 Sven - Leaping Reindeer
	915838: 721882, // Lorcana Q3 Mr. Incredible - Super Strong
	915839: 721876, // Lorcana Q3 Christopher Robin - Hunny Sage
	915840: 721878, // Lorcana Q3 Violet Parr - Super Resilient
	917633: 726179, // Riftbound Clanging Sentry, Radiance Nexus Night promo numbered 0545
}

// errNoPrinting marks a product no route named a printing of ours for. It
// is what the id-and-name route answers with instead of nothing at all, so a
// refusal is counted and said out loud rather than passing for a success.
var errNoPrinting = errors.New("named no printing of ours")

// errTwin marks a product named and numbered like another of its expansion,
// or like a printing already priced through a different shelf, either way
// leaving the catalog silent on which variant of the card it is.
var errTwin = errors.New("twin of another product")

// errForeign marks a product of a catalog the datastore does not carry.
var errForeign = errors.New("of a catalog we do not carry")

// noPrinting answers the error a product no route named a printing for
// refuses with - which for a Pokemon basic energy, or a Yu-Gi-Oh product no
// set lists, is no error at all.
//
// Every set prints the nine basic energies and reprints them unchanged for
// years, so the catalogs shelve them where no printing of ours is separable:
// by the year they were printed, or on a trainer kit's own shelf, one product
// standing for a card a dozen sets carry. Naming those is the same noise
// Magic's tokens are, and resolveMagic already passes over them for the same
// reason. They are 57 of the 177 lines a Pokemon run would otherwise report.
func (r *resolver) noPrinting(product *cm.Product) error {
	if r.gameID == cm.GamePokemon && pokemonBasicEnergy(pokemonName(product.Name)) {
		return nil
	}
	if r.gameID == cm.GameYuGiOh && yugiohNonCard(product) {
		return nil
	}
	return errNoPrinting
}

// offShelf reports whether a product answered with a printing that another
// expansion of this catalog sells as a product of its own.
//
// Cardmarket files a card on every shelf that ever handed it out, and most
// of those shelves are no set of ours - "Judge Promos", "Unnumbered Promos",
// "Special Tournament Promos", a starter deck whose reprints the datastore
// does not carry. With nothing holding the listing to an edition the matcher
// reaches past it and lands on the ordinary printing of that number, which
// is the very printing the shelf named after that set already prices as a
// product of its own. Two products on one printing means one of them
// publishes a price for a card it does not sell, and which one wins is
// whichever expansion the pool happened to walk first.
//
// Only an unlabelled answer reads that way. The datastore labels every
// printing that is not the ordinary one - the alternate arts, the event
// copies, the deck reprints - and those are what the promo shelves really do
// sell, so a labelled answer is this product's own card however far from its
// shelf the datastore files it. The set has to be one this catalog sells
// elsewhere, too: where no other expansion names it nothing else is pricing
// it, and refusing would drop the only price there is.
func (r *resolver) offShelf(product *cm.Product, cardID string) bool {
	co, err := r.backend.GetUUID(cardID)
	if err != nil || len(co.PromoTypes) > 0 {
		return false
	}
	shelf, found := r.shelved[co.SetCode]
	return found && !strings.EqualFold(shelf, product.ExpansionName)
}

// ownedElsewhere reports whether the datastore records cardID's printing
// under a Cardmarket product other than this one, as the printing or the
// foil that product's own id prices. A Lorcana card's id covers every
// finish, and a finish another product sells apart is not the id's.
func (r *resolver) ownedElsewhere(product *cm.Product, cardID string) bool {
	co, err := r.backend.GetUUID(cardID)
	if err != nil {
		return false
	}
	mcmID := co.Identifiers["mcmId"]
	if mcmID == "" || mcmID == fmt.Sprint(product.IDProduct) {
		return false
	}
	owner := r.backend.ConvertID(matcher.IDSpaceCardmarket, mcmID)
	ownerFoil, _ := r.backend.MatchID(owner, true)
	return owner != "" && (cardID == owner || cardID == ownerFoil)
}

// notPreErrata reports whether a product on One Piece's shelf of pre-errata
// printings answered with a printing the datastore does not tag pre-errata.
// Where it carries none at the number, the name reaches the card the set's
// own shelf sells, which offShelf passes when labelled: 2 V.2s of "Romance
// Dawn (Pre-Errata)" land on Box Toppers that way.
func (r *resolver) notPreErrata(product *cm.Product, cardID string) bool {
	if r.gameID != cm.GameOnePiece || !strings.Contains(strings.ToLower(product.ExpansionName), "errata") {
		return false
	}
	co, err := r.backend.GetUUID(cardID)
	if err != nil {
		return false
	}
	for _, promoType := range co.PromoTypes {
		if strings.Contains(promoType, "errata") {
			return false
		}
	}
	return true
}

// resolveFab names a Flesh and Blood product's printing. The catalog
// carries no collector number to tell its same-name products apart, so a
// product resolves through the TCGplayer id the bridge knows it by first, and
// only falls back on what the catalog says of it.
//
// The id names the card, and the product's own wording names the printing:
// Cardmarket sells each treatment as its own product and each print run as
// its own expansion, and the flag alone lands on the id's default printing.
// The printing the wording names is asked for first, of the id and then of
// the name, since the id's card may carry it under another row; only when
// nothing carries it does the id's own printing stand, the card being agreed
// on and the finish the one disagreement.
func resolveFab(r *resolver, product *cm.Product) (string, string, bool, error) {
	var cardID, loose string
	tcgID, found := r.tcgBridge[product.IDProduct]
	if found {
		cardID, _ = r.backend.MatchID(fmt.Sprint(tcgID), false)
		finish := fabFinish(product.ExpansionName, product.Name)
		if finish != "" && cardID != "" {
			loose = cardID
			cardID, _ = r.backend.MatchIDFinish(fmt.Sprint(tcgID), finish)
		}
	}
	// The bridge speaks through cardtrader's blueprints and knows only a
	// third of the catalog, and what it leaves out is ordinary cards. They
	// can be named without it.
	var byName bool
	if cardID == "" {
		cardID = r.matchFab(product)
		byName = cardID != ""
	}
	if cardID == "" {
		cardID = loose
	}
	if cardID == "" {
		return "", "", false, errNoPrinting
	}
	return cardID, cardID, byName, nil
}

// matchFab resolves a Flesh and Blood product the bridge does not know,
// from what the catalog says of it. The edition has to name a set of ours
// and the answer has to be in it: Cardmarket carries whole catalogs the
// datastores do not, and Match reaches past the edition when nothing in it
// fits, so without both an unknown set's cards land on whichever set happens
// to hold a number like theirs. Pokemon and Yu-Gi-Oh name their products
// through matchPokemon and matchYugioh, and every other game through the
// matcher alone; see gameResolvers.
func (r *resolver) matchFab(product *cm.Product) string {
	fabRenamed(product)

	r.fabDeckSetsMu.Lock()
	if r.fabDeckSets == nil {
		r.fabDeckSets = fabDeckSetIndex(r.backend)
	}
	deckSets := r.fabDeckSets
	r.fabDeckSetsMu.Unlock()

	shelves := fabShelves(r.backend, product, deckSets)
	if len(shelves) == 0 {
		return ""
	}
	// The treatment parenthetical is the printing's, not the name's:
	// fabFinish reads it off the untouched product name below, and a card
	// whose own name ends in a parenthetical ("Sink Below (Red)") keeps
	// it, so the exact name reaches the matcher whole. The art ahead of
	// the treatment stays on the name for the sets that file it as a
	// printing of its own, and comes off for the sets that do not. The raw
	// name stays as the fallback: the sets spelling a pitch color the other
	// one's way ("Rawhide Rumble" at ARR012, "Rawhide Rumble (Red)" at
	// HVY023) file the stripped name under the wrong set, and only the
	// decorated one still finds them.
	names := []string{versionTail.ReplaceAllString(product.Name, "")}
	_, stripped := fabTreatment(names[0])
	if stripped != names[0] {
		names = []string{stripped}
		if plain := fabDropArt(stripped); plain != stripped {
			names = append(names, plain)
		}
		names = append(names, versionTail.ReplaceAllString(product.Name, ""))
	}
	// A double-sided card is filed under both faces and, in the treatments
	// the fused row was never sold in, under the front face alone: Aether
	// Ashwing // Ash is plain, and the cold foil is Aether Ashwing's.
	if front, _, fused := strings.Cut(names[0], " // "); fused && !strings.Contains(product.Number, "/") {
		names = append(names, front)
	}

	// A game selling one card in several print runs needs one of them
	// named, or the runs alias and nothing resolves. A product naming its
	// treatment names the printing it is, and that printing is asked for
	// first wherever the shelves keep it; the card's own printing stands
	// only when no shelf carries the named one, the card being agreed on
	// and the finish the one disagreement.
	finishes := []string{fabFinish(product.ExpansionName, product.Name), ""}

	// The named finish is asked for on every shelf before the plain
	// printing is settled for on any, so a card the fused row was never
	// sold cold in is found under its front face rather than priced as
	// the plain fused card.
	for _, finish := range finishes {
		for _, sh := range shelves {
			set, edition, numberPrefix, printRun := sh.set, sh.edition, sh.numberPrefix, sh.printRun
			// A promo's programme is the prefix our numbering carries
			// and the expansion Cardmarket sells it under, so the number
			// is only whole once the two are put back together; and any
			// set numbering its cards on letters of its own is asked with
			// them, since a fused card answers to its faces' numbers only
			// when they are whole.
			prefix := numberPrefix
			if prefix == "" {
				prefix = fabSetPrefix(set)
			}
			numbers := fabNumbers(prefix, product.Number)

			// A number is asked of every spelling before a looser number
			// is asked of any: the name alone comes last, after every
			// spelling has been tried at the number, so a wording the set
			// files no printing under cannot slip past the number onto
			// the card's plainest printing.
			for _, number := range numbers {
				for _, name := range names {
					id, err := r.backend.Match(&matcher.InputCard{
						Name:      name,
						Edition:   edition,
						Variation: number,
						Finish:    finish,
					})
					if err != nil {
						continue
					}
					co, cerr := r.backend.GetUUID(id)
					if cerr != nil || !strings.EqualFold(co.SetCode, set.Code) {
						continue
					}
					// The run has to hold too, for the same reason the
					// set does: a card the datastore keeps in one run
					// only is answered with that run whichever was asked
					// for, and the other run's expansion sells the very
					// same card.
					if printRun != "" && !strings.HasPrefix(co.Finish, matcher.FinishSlug(printRun)) {
						continue
					}
					// A promo's number is only whole with its programme,
					// and a name is not enough on its own: the same card
					// is handed out by several of them, so an answer that
					// did not come back with the number asked for is
					// another programme's.
					if numberPrefix != "" && !sameFabFace(co.Number, numberPrefix+product.Number) {
						continue
					}
					return id
				}
			}
		}
	}
	return ""
}

// resolveMagic answers a product with the printings its two price columns
// belong to, and whether they were named rather than found by the product's
// id. An empty id under a nil error means the product names nothing this
// datastore carries, which is a skip rather than a failure.
func resolveMagic(r *resolver, product *cm.Product) (string, string, bool, error) {
	// An exact mcmId match ties the product to its printings more
	// reliably than name/number matching, which cannot tell apart
	// products sharing a collector number (e.g. RVR 312 vs 312z,
	// both "312" upstream); preprocess only when no id is known.
	cardID, cardIDFoil := Fallback(r.backend, product)
	if cardID != "" {
		return cardID, cardIDFoil, false, nil
	}

	// A two-sided token sheet's own product name ("Bird Token (W 1/1) //
	// Spirit Token (W 1/1)") is not one Preprocess/Match below was ever
	// built to read. Both faces' names plus the product's edition
	// (magic.MatchTokenPairingByNamesAndEdition, already refusing rather
	// than guessing whenever a name repeats across several same-named
	// tokens in one edition) name the pair, and the product's Number
	// ("T 2/6", "CT 37/2") states its faces' collector numbers, which a
	// pair numbered otherwise is another printing of. Resolved or not,
	// this listing is done here: falling into Preprocess/Match below would
	// only refuse it again, more noisily.
	if strings.Contains(product.Name, "Token") && strings.Contains(product.Name, " // ") {
		if pairID := magic.MatchTokenPairingByNamesAndEdition(r.backend, product.Name, product.ExpansionName, false); pairID != "" {
			cardID, _ = r.backend.MatchID(pairID, false)
		}
		if pairIDFoil := magic.MatchTokenPairingByNamesAndEdition(r.backend, product.Name, product.ExpansionName, true); pairIDFoil != "" {
			cardIDFoil, _ = r.backend.MatchID(pairIDFoil, true)
		}
		if r.otherFaces(product, cardID) || r.otherFaces(product, cardIDFoil) {
			return "", "", true, nil
		}
		return cardID, cardIDFoil, true, nil
	}

	theCard, err := Preprocess(r.backend, product.Name, product.Number, product.ExpansionName)
	if err != nil {
		_, ok := err.(*PreprocessError)
		if ok {
			return "", "", false, err
		}
		return "", "", false, nil
	}

	cardID, err = r.backend.Match(theCard)
	if errors.Is(err, matcher.ErrUnsupported) {
		return "", "", false, nil
	} else if err != nil {
		if r.backend.IsToken(theCard.Name) ||
			theCard.Edition == "Pro Tour Collector Set" ||
			strings.HasPrefix(theCard.Edition, "World Championship Decks") {
			return "", "", false, nil
		}

		r.logf("%v", err)
		r.logf("%q (%d)", theCard, product.IDProduct)
		r.logf("%v | %v | %v ", product.Name, product.ExpansionName, product.Number)

		var alias *matcher.AliasingError
		if errors.As(err, &alias) {
			probes := alias.Probe()
			for _, probe := range probes {
				card, _ := r.backend.GetUUID(probe)
				r.logf("- %s", card)
			}
		}
		return "", "", false, err
	}

	co, cerr := r.backend.GetUUID(cardID)
	switch {
	case cerr != nil:
		cardIDFoil, _ = r.backend.MatchID(cardID, true)
	case co.Etched:
		// No separate guide column for etched; keep it off the plain
		// foil sibling MatchID(cardID, true) would otherwise pick.
		cardIDFoil = cardID
	case foilOnlyShelf(product, co.SetCode):
		// The guide duplicates one price into both columns; redirect
		// both ids onto the foil twin actually sold.
		fid, ferr := r.backend.MatchID(cardID, true)
		if ferr == nil && fid != "" {
			cardID = fid
		}
		cardIDFoil = cardID
	default:
		cardIDFoil, _ = r.backend.MatchID(cardID, true)
	}
	return cardID, cardIDFoil, true, nil
}

// tokenFace reads one face of a token product's Number, a collector number
// that may carry its set's code ("REX02").
var tokenFace = regexp.MustCompile(`^[A-Z]*0*([0-9]+[a-z]?)$`)

// otherFaces reports whether a two-faced token product's Number names face
// numbers other than those of the pair cardID, which a product whose Number
// does not read as two faces never does.
func (r *resolver) otherFaces(product *cm.Product, cardID string) bool {
	_, faces, found := strings.Cut(product.Number, " ")
	if !found {
		faces = product.Number
	}
	front, back, found := strings.Cut(faces, "/")
	if !found {
		return false
	}
	co, err := r.backend.GetUUID(cardID)
	if err != nil {
		return false
	}
	pairFront, pairBack, found := strings.Cut(co.Number, " // ")
	if !found {
		return false
	}
	var got, want []string
	for _, face := range []string{front, back} {
		m := tokenFace.FindStringSubmatch(face)
		if m == nil {
			return false
		}
		got = append(got, m[1])
	}
	for _, face := range []string{pairFront, pairBack} {
		m := tokenFace.FindStringSubmatch(face)
		if m == nil {
			return false
		}
		want = append(want, m[1])
	}
	slices.Sort(got)
	slices.Sort(want)
	return !slices.Equal(got, want)
}

// foilOnlyShelf reports whether product's shelf sells only the foil shown,
// at a number an ordinary nonfoil printing also carries. setCode holds the
// Holiday Release rule to the two sets whose "(V.2)" is the silverfoil of
// its "(V.1)": LTC's box topper and LTR's scroll showcase. Modern Horizons 3
// Extras' "(V.3)" onwards are sold in both finishes or at a number of their
// own. Closed on purpose: widening it would misprice a shelf that
// legitimately sells both finishes.
func foilOnlyShelf(product *cm.Product, setCode string) bool {
	switch product.ExpansionName {
	case "Commander: FINAL FANTASY: Collector's Edition",
		"Commander: Marvel Super Heroes: Collector's Edition",
		"Commander: Teenage Mutant Ninja Turtles: Extras":
		return true
	case "Commander: Modern Horizons 3: Extras":
		return strings.HasSuffix(product.Name, "(V.2)") || !strings.Contains(product.Name, "(V.")
	case "The Lord of the Rings: Tales of Middle-earth Holiday Release":
		return (setCode == "LTC" || setCode == "LTR") && strings.HasSuffix(product.Name, "(V.2)")
	}
	return false
}

// foilApartShelves are the Magic expansions where the foil of a card is a
// "(V.2)" product of its own and the "(V.1)" beside it keeps only a thin
// foil column of mislisted copies. The V.1 gives that column up.
var foilApartShelves = map[string]bool{
	"Commander: Modern Horizons 3: Extras": true,
	"Universes Beyond: Warhammer 40,000":   true,
}

// soldFoilApart reports whether mapped is the "(V.1)" of a card whose foil
// Cardmarket sells as the "(V.2)" product at the same number.
func (r *resolver) soldFoilApart(mapped cm.CatalogProduct, expansion cm.Expansion) bool {
	if !foilApartShelves[expansion.Name] || mapped.Version != 1 || r.catalog == nil {
		return false
	}
	r.foilApartMu.Lock()
	defer r.foilApartMu.Unlock()
	if r.foilApart == nil {
		r.foilApart = map[int]map[string]bool{}
		for _, product := range r.catalog.Data.Products {
			if product.Version != 2 || !foilApartShelves[r.catalog.Data.Expansions[product.ExpansionID].Name] {
				continue
			}
			if r.foilApart[product.ExpansionID] == nil {
				r.foilApart[product.ExpansionID] = map[string]bool{}
			}
			r.foilApart[product.ExpansionID][product.Number] = true
		}
	}
	return r.foilApart[expansion.IDExpansion][mapped.Number]
}

// gameResolvers names the printings a product of each game belongs to; see
// resolveProduct.
var gameResolvers = map[cm.Game]func(r *resolver, product *cm.Product) (string, string, bool, error){
	cm.GameMagic:         resolveMagic,
	cm.GameLorcana:       resolveLorcana,
	cm.GameRiftbound:     resolveRiftbound,
	cm.GameOnePiece:      resolveOnePiece,
	cm.GameGundam:        resolveGundam,
	cm.GameYuGiOh:        resolveYugioh,
	cm.GameFleshAndBlood: resolveFab,
	cm.GamePokemon:       resolvePokemon,
}

// resolveProduct answers a product with the printings its two price columns
// belong to, and whether they were named rather than found by an id, the way
// its game's resolver reads it. An empty id under a nil error means the
// product names nothing this datastore carries, which is a skip rather than
// a failure.
func (r *resolver) resolveProduct(product *cm.Product) (string, string, bool, error) {
	resolve, found := gameResolvers[r.gameID]
	if !found {
		return "", "", false, errors.New("unsupported game")
	}
	cardID, cardIDFoil, byName, err := resolve(r, product)
	if err != nil || cardID == "" {
		return "", "", false, err
	}
	// A foil Cardmarket sells as a product of its own is that product's to
	// price, not the plain version's second column; see claimByID.
	if cardIDFoil != cardID && r.foilSold[cardIDFoil] {
		cardIDFoil = ""
	}
	return cardID, cardIDFoil, byName, nil
}

// withFoil answers a product with a printing and its foil.
func (r *resolver) withFoil(cardID string) (string, string, bool, error) {
	cardIDFoil, _ := r.backend.MatchID(cardID, true)
	return cardID, cardIDFoil, false, nil
}

// bridged names the printing the bridge links a product to, where a row of
// productLinks does not link it instead.
func (r *resolver) bridged(product *cm.Product) (string, bool) {
	tcgID, found := r.tcgBridge[product.IDProduct]
	if crossed, linked := productLinks[product.IDProduct]; linked {
		tcgID, found = crossed, true
	}
	if !found {
		return "", false
	}
	id, err := r.backend.MatchID(fmt.Sprint(tcgID), false)
	return id, err == nil
}

// probeAliasing logs the printings an aliasing error could not choose
// between.
func (r *resolver) probeAliasing(cardName string, err error) {
	var alias *matcher.AliasingError
	if !errors.As(err, &alias) {
		return
	}
	probes := alias.Probe()
	r.logf("%s got ids: %s", cardName, probes)
	for _, probe := range probes {
		co, _ := r.backend.GetUUID(probe)
		r.logf("%s: %s", probe, co)
	}
}

// matchNumbered names the printing a name, edition and number reach, and
// its foil. Both empty under a nil error is a product the matcher does not
// support. A wrong-variant miss may just mean the card has no nonfoil
// printing (Match validates the finish), so the foil is taken for both then;
// where neither finish matches the miss was genuine, and the foil probe's
// error may carry the more informative verdict. report logs the printings
// an aliasing error names.
func (r *resolver) matchNumbered(cardName, edition, number string, report bool) (string, string, error) {
	cardID, err := r.backend.Match(&matcher.InputCard{Name: cardName, Edition: edition, Variation: number})
	if errors.Is(err, matcher.ErrUnsupported) {
		return "", "", nil
	} else if err != nil && !errors.Is(err, matcher.ErrCardWrongVariant) {
		if report {
			r.probeAliasing(cardName, err)
		}
		return "", "", err
	}
	cardIDFoil, errFoil := r.backend.Match(&matcher.InputCard{Name: cardName, Edition: edition, Variation: number, Foil: true})
	if cardID == "" {
		cardID = cardIDFoil
	}
	if cardID == "" {
		if errFoil != nil {
			err = errFoil
		}
		return "", "", err
	}
	return cardID, cardIDFoil, nil
}

// pokemonReverseHolo is the printing Pokemon's guide prices in its second
// pair of columns, as TCGplayer names it.
const pokemonReverseHolo = "Reverse Holofoil"

// resolveUUIDs answers a product from the printings listed for it,
// splitting them by finish the way the guide's columns are split. Ids the
// datastore does not carry are passed over - a double-faced card lists its
// back face too, and the index knows only fronts. Within a finish the
// printing whose number agrees with the product's wins, the way Fallback
// already prefers it; a pick between printings the number cannot settle is
// said out loud, unless only one of them is undecorated, see undecorated.
// Both ids empty means the entry decided nothing.
func (r *resolver) resolveUUIDs(product *cm.Product, uuids []string) (string, string) {
	if r.gameID == cm.GameMagic {
		id := r.numberedPrinting(product, uuids)
		if id != "" {
			uuids = []string{id}
		}
	}
	var plain, foil []string
	var plainMatched, foilMatched bool
	for _, uuid := range uuids {
		co, err := r.backend.GetUUID(uuid)
		if err != nil {
			continue
		}
		// See plausiblePrinting: a Magic WCD or Oversized product can be
		// listed with a printing mtgjson has wrongly linked, the same
		// drift Fallback's own mcmId route guards against.
		if r.gameID == cm.GameMagic && !plausiblePrinting(r.backend, product.ExpansionName, uuid) {
			continue
		}
		// mtgjson also links a few real cards to the emblem or token
		// product of the same name; a Token product prices tokens only.
		if r.gameID == cm.GameMagic && product.Rarity == "Token" && co.Layout != "token" {
			continue
		}
		sameNumber := strings.EqualFold(co.PlainNumber, product.Number)
		if co.Foil || co.Etched {
			if sameNumber && !foilMatched {
				foil = append([]string{uuid}, foil...)
				foilMatched = true
			} else {
				foil = append(foil, uuid)
			}
		} else {
			if sameNumber && !plainMatched {
				plain = append([]string{uuid}, plain...)
				plainMatched = true
			} else {
				plain = append(plain, uuid)
			}
		}
	}

	if len(plain) > 1 && !plainMatched {
		plain, plainMatched = r.undecorated(plain)
	}
	if len(foil) > 1 && !foilMatched {
		foil, foilMatched = r.undecorated(foil)
	}

	if len(plain) > 1 && !plainMatched {
		r.logf("id %d %q lists %d plain printings and the number settles none; keeping %s",
			product.IDProduct, product.Name, len(plain), plain[0])
	}
	if len(foil) > 1 && !foilMatched {
		r.logf("id %d %q lists %d foil printings and the number settles none; keeping %s",
			product.IDProduct, product.Name, len(foil), foil[0])
	}

	var cardID, cardIDFoil string
	switch {
	case len(plain) > 0:
		cardID = plain[0]
		if len(foil) > 0 {
			cardIDFoil = foil[0]
		} else {
			// No foil printing is listed, but the datastore may still
			// carry one, the way resolveProduct probes for it.
			cardIDFoil, _ = r.backend.MatchID(cardID, true)
		}
	case len(foil) > 0:
		// A foil-only product prices through its own columns; both ids
		// point to it, the way Fallback answers a single printing.
		cardID = foil[0]
		cardIDFoil = foil[0]
	}

	// The listing names the printing, not the finish sold; redirect a
	// foil-only shelf's product the way resolveMagic does.
	co, err := r.backend.GetUUID(cardID)
	if err == nil && !co.Etched && foilOnlyShelf(product, co.SetCode) {
		fid, ferr := r.backend.MatchID(cardID, true)
		if ferr == nil && fid != "" {
			cardID = fid
		}
		cardIDFoil = cardID
	}
	return cardID, cardIDFoil
}

// undecorated puts first the one printing of uuids whose number is its own
// PlainNumber, free of every decoration the game strips (a star, a dagger,
// a printing letter, zero padding), and reports whether there was one.
// MTGJSON lists a printing and its decorated variants under one Cardmarket
// id, and the plain printing is the one the product sells. More or fewer
// than one leaves the order alone.
func (r *resolver) undecorated(uuids []string) ([]string, bool) {
	pick := -1
	for i, uuid := range uuids {
		co, err := r.backend.GetUUID(uuid)
		if err != nil || co.Number != co.PlainNumber {
			continue
		}
		if pick >= 0 {
			return uuids, false
		}
		pick = i
	}
	if pick < 0 {
		return uuids, false
	}
	return slices.Concat([]string{uuids[pick]}, uuids[:pick], uuids[pick+1:]), true
}

// numberedPrinting answers the printing of the listed card at the product's
// own number, where none listed is at it and the set carries one: mtgjson
// crosses the ids of two printings of a card now and then, each linked to the
// other's product.
func (r *resolver) numberedPrinting(product *cm.Product, uuids []string) string {
	if product.Number == "" {
		return ""
	}
	var mapped *matcher.CardObject
	for _, uuid := range uuids {
		co, err := r.backend.GetUUID(uuid)
		if err != nil {
			continue
		}
		// Plain here and exact below: Cardmarket numbers ZEN's 230a Plains
		// "230", and 485 such products are mapped right to a lettered twin.
		if strings.EqualFold(co.PlainNumber, product.Number) {
			return ""
		}
		mapped = co
	}
	if mapped == nil {
		return ""
	}
	set, err := r.backend.GetSet(mapped.SetCode)
	if err != nil {
		return ""
	}
	for _, card := range set.Cards {
		if card.Name == mapped.Name && strings.EqualFold(card.Number, product.Number) {
			return card.UUID
		}
	}
	return ""
}

// resolveMapped answers one product of the id map. For Magic,
// versionPrintings answers first, since it corrects the datastore's links,
// then the printings the datastore links to the product's id; what neither
// names is answered from what the catalog says of it, by resolveProduct, so
// a product the datastore does not link yet is matched rather than lost.
func (r *resolver) resolveMapped(id int, mapped cm.CatalogProduct, expansion cm.Expansion) resolved {
	res := r.placeMapped(id, mapped, expansion)
	if res.cardID != "" && r.gameID == cm.GameMagic && r.soldFoilApart(mapped, expansion) {
		res.cardIDFoil = res.cardID
	}
	return res
}

// placeMapped names the printings of one id-mapped product, before the
// shelf's own rules on its foil column.
func (r *resolver) placeMapped(id int, mapped cm.CatalogProduct, expansion cm.Expansion) resolved {
	product := &cm.Product{
		IDProduct:     id,
		Name:          mapped.Name,
		Number:        mapped.Number,
		Rarity:        mapped.Rarity,
		ExpansionName: expansion.Name,
		ExpansionCode: expansion.SetCode,
	}
	product.Expansion.IDExpansion = expansion.IDExpansion

	if r.gameID == cm.GameMagic {
		if unplacedProducts[id] {
			return resolved{product: product}
		}
		cardID, cardIDFoil := versionPrinting(r.backend, product)
		if cardID != "" {
			return resolved{product: product, cardID: cardID, cardIDFoil: cardIDFoil}
		}
		cardID, cardIDFoil = r.resolveUUIDs(product, r.magicPrintings(id))
		if cardID != "" {
			return resolved{product: product, cardID: cardID, cardIDFoil: cardIDFoil}
		}
	}

	cardID, cardIDFoil, byName, err := r.resolveProduct(product)
	return resolved{product: product, cardID: cardID, cardIDFoil: cardIDFoil, byName: byName, err: err}
}

// refusalName is the card a refused product names, as the report files it:
// a Pokemon product carries its attacks and energy symbols in brackets the
// card's name does not, and every other game's product name is the card's.
func (r *resolver) refusalName(name string) string {
	if r.gameID == cm.GamePokemon {
		return pokemonName(name)
	}
	return name
}

// checkCatalog reports whether the id map can be walked. For the games that
// shelve whole foreign catalogs, the map says which shelves those are only
// through the expansion codes: a map written before it carried them cannot
// be walked safely, and the run refuses rather than price the foreign
// printings onto the English ones.
func (r *resolver) checkCatalog() error {
	if r.catalog == nil {
		return errors.New("no id map to price from")
	}
	switch r.gameID {
	case cm.GameOnePiece, cm.GameYuGiOh, cm.GameFleshAndBlood:
	default:
		return nil
	}
	for _, expansion := range r.catalog.Data.Expansions {
		if expansion.Code != "" {
			return nil
		}
	}
	return errors.New("the id map carries no expansion codes to tell the foreign shelves by")
}

// matchPokemon names a Pokemon product's printing from what the catalog
// says of it, held to the sets its expansion may hold. An expansion naming
// no set of ours is a catalog we do not carry, and a miss in one is said so
// rather than reported product by product.
func (r *resolver) matchPokemon(product *cm.Product) (string, error) {
	if pokemonForeignDenied(product.ExpansionName, product.Number) {
		return "", errForeign
	}
	name := pokemonName(product.Name)
	type candidate struct {
		edition, number string
		prefixed, shelf bool
		jumbo           bool
	}
	var candidates []candidate
	editions, prefix := pokemonEditions(r.backend, product.ExpansionName)
	number := product.Number
	if prefix != "" && number != "" {
		number = prefix + number
	}
	for _, edition := range editions {
		candidates = append(candidates, candidate{edition, number, prefix != "", true, false})
	}
	// A prize pack series reprints a card the series before it printed;
	// the series is asked for only when the number alone cannot tell.
	word, onSeries := strings.CutPrefix(product.ExpansionName, pokemonPrizePack)
	series, ordinal := ordinalWords[word]
	if onSeries && ordinal && number != "" {
		for _, edition := range editions {
			candidates = append(candidates, candidate{edition, number + " Series " + strconv.Itoa(series), false, true, false})
		}
	}
	if pokemonLettered.MatchString(number) {
		for _, edition := range pokemonLetteredSets {
			candidates = append(candidates, candidate{edition, number, false, false, false})
		}
	}
	if m := pokemonPromoNumber.FindStringSubmatch(product.Number); m != nil {
		if shelf, found := pokemonPromoCodes[m[1]]; found {
			promo := pokemonExpansions[shelf]
			candidates = append(candidates, candidate{promo.sets[0], promo.prefix + m[2], promo.prefix != "", false, false})
		}
	}
	// Only the rarity tells an oversized card from the set's own at its
	// number, and the datastore files it in the Jumbo Cards set. A shelf
	// of a catalog we do not carry stays foreign.
	if product.Rarity == "Oversized" && !pokemonJumboHeld[product.IDProduct] {
		onShelf := slices.ContainsFunc(candidates, func(c candidate) bool {
			_, err := r.backend.GetSetByName(c.edition)
			return err == nil
		})
		if onShelf {
			candidates = slices.Insert(candidates, 0, candidate{pokemonJumbo, number, prefix != "", false, true})
		}
	}
	carried := false
	for _, c := range candidates {
		set, err := r.backend.GetSetByName(c.edition)
		if err != nil {
			continue
		}
		carried = true
		id, err := r.backend.Match(&matcher.InputCard{Name: name, Edition: c.edition, Variation: c.number})
		if err != nil {
			continue
		}
		co, err := r.backend.GetUUID(id)
		if err != nil || !strings.EqualFold(co.SetCode, set.Code) {
			continue
		}
		if c.prefixed && !strings.EqualFold(co.Number, c.number) {
			continue
		}
		// A jumbo the bridge gives to another product is that one's.
		if c.jumbo && r.bridgeHolds(id, product.IDProduct) {
			continue
		}
		// Cardmarket rates a stamped copy "Promo" and names it like the
		// plain card; a shelf with no base run of its own is a bundle's.
		if c.shelf && set.BaseSetSize > 0 && product.Rarity == "Promo" {
			stamped := r.pokemonStamped(co)
			if stamped != "" && !r.bridgeHolds(stamped, product.IDProduct) {
				return stamped, nil
			}
		}
		return id, nil
	}
	if !carried || pokemonForeign(product.ExpansionName) {
		return "", errForeign
	}
	return "", errNoPrinting
}

// matchYugioh names a Yu-Gi-Oh product's printing from what the catalog
// says of it, held to the sets its expansion may hold. A number written with
// a region prefix names a print run of its own ("EN000" is the European
// print, "A000" the Asian), carried only where the catalog has a set for it;
// an expansion naming no set of ours, or a run we have no set for, is a
// catalog we do not carry rather than a product that named nothing.
func (r *resolver) matchYugioh(product *cm.Product) (string, error) {
	name := yugiohArtToken.ReplaceAllString(versionTail.ReplaceAllString(product.Name, ""), "Art Token: $1")
	var rarity string
	if fields := rarityTail.FindStringSubmatch(product.Name); fields != nil {
		rarity = fields[1]
	}
	// The oversized printing is not in the shelf's set; only the name and
	// the tag reach it. See yugiohOversized.
	if strings.EqualFold(rarity, "Oversized") {
		if id, err := yugiohOversized(r.backend, name); err == nil {
			return id, nil
		}
	}
	region := ""
	if product.Number != "" {
		region = numberPrefix(product.Number)
	}
	tail := numberTail.FindString(product.Number)

	carried := false
	for _, edition := range yugiohEditions(product.ExpansionName) {
		set, err := r.backend.GetSetByName(edition)
		if err != nil {
			continue
		}
		carried = true
		sets := []*matcher.Set{set}
		// The European print is a set of its own where the catalog has
		// one, numbered with the region the product writes.
		if region == "EN" {
			worldwide, err := r.backend.GetSet(set.Code + "-EN")
			if err == nil {
				sets = []*matcher.Set{worldwide}
			}
		}
		for _, set := range sets {
			numbers := []string{product.Number}
			if tail != "" {
				base := strings.TrimSuffix(set.Code, "-EN")
				if region == "" {
					// The catalog writes the region into most sets'
					// numbers ("MP18-EN065") and Cardmarket leaves it
					// out; both spellings are asked.
					numbers = append(numbers, base+"-"+tail, base+"-EN"+tail)
				} else {
					numbers = append(numbers, base+"-"+region+tail)
				}
			}
			// A number the set gives to another card is the storefront's
			// mistake rather than the card's: the oldest sets are sold
			// once more under a numbering shifted by a card or two, and
			// the name is what still says which card it is. Only a
			// number so contradicted is set aside; a number the set
			// merely lacks stays a refusal.
			if tail != "" && r.yugiohNumberTaken(set.Code, numbers[1:], name) {
				numbers = append(numbers, "")
			}
			for _, number := range numbers {
				variation := strings.TrimSpace(number + " " + rarity)
				// A shelf whose printings differ only by a mark the
				// storefront does not name takes it from the version
				// index; see yugiohVersionVariants on what that order is
				// worth. A version the table does not cover is left
				// alone, and aliases.
				if labels := yugiohVersionVariants[strings.TrimSuffix(set.Code, "-EN")]; labels != nil {
					if index := cm.ProductVersion(product); index >= 1 && index <= len(labels) {
						variation = strings.TrimSpace(variation + " " + labels[index-1])
					}
				}
				// The run is a flag on each Cardmarket listing, not a
				// product of its own, so the card's default run answers.
				id, err := r.backend.Match(&matcher.InputCard{
					Name:      name,
					Edition:   set.Name,
					Variation: variation,
				})
				if err != nil {
					continue
				}
				co, err := r.backend.GetUUID(id)
				if err != nil || !strings.EqualFold(co.SetCode, set.Code) {
					continue
				}
				if number != "" && otherPrintRun(product.Number, co.Number) {
					continue
				}
				return id, nil
			}
		}
	}
	if id := r.yugiohTokenNumber(product); id != "" {
		return id, nil
	}
	if carried && region != "" && tail != "" {
		id := r.yugiohInfixed(product, name, rarity, region, tail)
		if id != "" {
			return id, nil
		}
	}
	if !carried {
		id := r.yugiohShelfCode(product, name, rarity, region)
		if id != "" {
			return id, nil
		}
	}
	if !carried || region != "" {
		return "", errForeign
	}
	if strings.HasPrefix(product.ExpansionName, "OTS Tournament Pack") && r.yugiohPastEnd(product.ExpansionName, tail) {
		return "", errForeign
	}
	if r.yugiohEuropean(name, tail) {
		return "", errTwin
	}
	return "", errNoPrinting
}

// yugiohPlainSuffix matches the plain digits a set's own collector number
// ends on, after any "EN" region infix, for comparing against a tail
// Cardmarket writes with no infix of its own.
var yugiohPlainSuffix = regexp.MustCompile(`-(?:EN)?(\d+)$`)

// yugiohPastEnd reports whether an OTS Tournament Pack's number tail runs
// past the last one any set carrying its expansion prints - the Portuguese
// packs' extra numbers. Scoped to OTS so a number a set merely lacks stays a
// refusal elsewhere (Gouki Re-Match in TestMatchYugiohShelves).
func (r *resolver) yugiohPastEnd(expansion, tail string) bool {
	n, err := strconv.Atoi(tail)
	if err != nil {
		return false
	}
	numbers := r.yugiohNumbers()
	var sets int
	for _, edition := range yugiohEditions(expansion) {
		set, err := r.backend.GetSetByName(edition)
		if err != nil {
			continue
		}
		sets++
		for number := range numbers[set.Code] {
			m := yugiohPlainSuffix.FindStringSubmatch(number)
			if m == nil {
				continue
			}
			last, _ := strconv.Atoi(m[1])
			if last >= n {
				return false
			}
		}
	}
	return sets > 0
}

// yugiohInfixed answers the printing numbered like the product, with the
// "EN" region infix the datastore writes and Cardmarket leaves out -
// "RDS-ENSE1" for a product numbered "SE1".
func (r *resolver) yugiohInfixed(product *cm.Product, name, rarity, region, tail string) string {
	var bases []string
	base, _, dashed := strings.Cut(product.Number, "-")
	if dashed {
		bases = append(bases, base)
	} else {
		editions := append([]string{}, yugiohEditions(product.ExpansionName)...)
		for _, edition := range append(editions, product.ExpansionCode) {
			set, err := r.backend.GetSetByName(edition)
			if err == nil {
				bases = append(bases, strings.TrimSuffix(set.Code, "-EN"))
			}
		}
	}
	for _, base := range bases {
		number := base + "-EN" + region + tail
		id, err := r.backend.Match(&matcher.InputCard{
			Name:      name,
			Variation: strings.TrimSpace(number + " " + rarity),
		})
		if err != nil {
			continue
		}
		co, err := r.backend.GetUUID(id)
		if err == nil && strings.EqualFold(co.Number, number) {
			return id
		}
	}
	return ""
}

// yugiohShelfCode answers a product on a shelf that names no set of ours by
// the number the shelf's own code prefixes. The catalog files the video game
// and event promos under catch-all sets (DDS-001 and FMR-001 in VDP), so the
// shelf finds no set while the number still names the printing.
func (r *resolver) yugiohShelfCode(product *cm.Product, name, rarity, region string) string {
	if product.ExpansionCode == "" || product.Number == "" {
		return ""
	}
	numbers := []string{product.ExpansionCode + "-" + product.Number}
	if region == "" {
		numbers = append(numbers, product.ExpansionCode+"-EN"+product.Number)
	}
	for _, number := range numbers {
		id, err := r.backend.Match(&matcher.InputCard{Name: name, Variation: number})
		if err != nil {
			continue
		}
		co, err := r.backend.GetUUID(id)
		if err != nil || !strings.EqualFold(co.Number, number) {
			continue
		}
		// A version names its rarity where the shelf sells the card in two.
		if rarity != "" && co.Rarity != matcher.RarityName(rarity) {
			continue
		}
		return id
	}
	return ""
}

// yugiohEuropean reports whether the card has a European print at this
// number - "MRL-E129" for tail "129" - that a shelf of ours already prices.
// Cardmarket's "Spell Ruler" catalog carries Magic Ruler's own European
// numbers as if they were Spell Ruler's; the row is real, just filed under
// Magic Ruler's shelf instead of the one the product sits on.
func (r *resolver) yugiohEuropean(name, tail string) bool {
	if tail == "" {
		return false
	}
	uuids, err := r.backend.SearchEquals(name)
	if err != nil {
		return false
	}
	for _, uuid := range uuids {
		co, err := r.backend.GetUUID(uuid)
		if err == nil && strings.HasSuffix(strings.ToUpper(co.Number), "-E"+strings.ToUpper(tail)) {
			return true
		}
	}
	return false
}

// reportRefused says what an expansion refused and counts it into the run's
// tally. Every refusal is named, one line each, except in an expansion
// nothing resolved in: that is a catalog we do not carry at all - Cardmarket
// sells whole Japanese programs the datastores have no set for - and the
// count is the whole story, where naming each of its products would be tens
// of thousands of lines saying it again.
//
// A shelf whose every refusal is a foreign one is not worth even that line.
// Cardmarket files the Japanese and other Asian Pokemon catalogs under the
// same game as the English one - 535 of its 774 shelves, 43,603 products -
// and those are programs we do not carry rather than printings we failed to
// find, so a run saying so shelf by shelf is 532 lines that name no work.
// The run's own tally still counts them, and the caller's walk says how
// many in one line at the end.
func (r *resolver) reportRefused(expansion string, total int, refused []string, twins, foreign int) {
	if len(refused) == 0 && twins == 0 {
		return
	}
	count := len(refused) + twins + foreign
	line := fmt.Sprintf("%s: %d of %d products named no printing of ours", expansion, count, total)
	var why []string
	if twins > 0 {
		why = append(why, fmt.Sprintf("%d twins of another product", twins))
	}
	if foreign > 0 {
		why = append(why, fmt.Sprintf("%d of a catalog we do not carry", foreign))
	}
	if len(why) > 0 {
		line += " (" + strings.Join(why, ", ") + ")"
	}
	r.logf("%s", line)
	if len(refused) == total {
		return
	}
	for _, product := range refused {
		r.logf("no printing for %s", product)
	}
}

// disownBridged takes the bridge's answer away from a product it landed on
// a card the same shelf sells, and prices, under another product's name,
// and lets the name answer instead. The bridge speaks through another
// marketplace's links, and a link tied to the neighbouring product lands
// a card on its neighbour's printing: Cardmarket's Herald of Ravages on
// the datastore's Herald of Rebirth, the red Lead with Heart on the
// yellow. A spelling the datastore does not share is not that - no priced
// product of the shelf claims the printing - and the id keeps its say
// over it, the misspelt listing of a card the shelf also sells refused
// included.
func (r *resolver) disownBridged(results []resolved) {
	claimed := map[string]bool{}
	for _, res := range results {
		if res.err != nil || res.cardID == "" {
			continue
		}
		name := fabBaseName(res.product.Name)
		claimed[matcher.Normalize(name)] = true
		claimed[matcher.Normalize(unpitched(name))] = true
	}
	for i, res := range results {
		if res.err != nil || res.cardID == "" || res.byName || fabNamesPrinting(r.backend, res.product, res.cardID) {
			continue
		}
		co, err := r.backend.GetUUID(res.cardID)
		if err != nil || !claimed[matcher.Normalize(fabBaseName(co.Name))] {
			continue
		}
		cardID := r.matchFab(res.product)
		if cardID == "" {
			results[i] = resolved{product: res.product, err: errNoPrinting}
			continue
		}
		results[i] = resolved{product: res.product, cardID: cardID, cardIDFoil: cardID, byName: true}
	}
}

// yugiohNumbers indexes every set's collector numbers by the card each
// names, built once over the whole datastore on first use under a mutex;
// the map itself is never written again, so callers read it lock-free.
func (r *resolver) yugiohNumbers() map[string]map[string]string {
	r.numbersMu.Lock()
	defer r.numbersMu.Unlock()
	if r.numbers == nil {
		r.numbers = map[string]map[string]string{}
		for _, uuid := range r.backend.GetUUIDs() {
			co, err := r.backend.GetUUID(uuid)
			if err != nil {
				continue
			}
			index := r.numbers[co.SetCode]
			if index == nil {
				index = map[string]string{}
				r.numbers[co.SetCode] = index
			}
			index[strings.ToUpper(co.Number)] = matcher.Normalize(co.Name)
		}
	}
	return r.numbers
}

// magicPrintings answers the Magic printings whose mcmId is the product id,
// by mtgjson uuid and sorted, as MTGJSON's own CardmarketIdentifiers lists
// them. The index is built once over the whole datastore on first use under
// a mutex and never written again.
func (r *resolver) magicPrintings(id int) []string {
	r.mcmPrintingsMu.Lock()
	defer r.mcmPrintingsMu.Unlock()
	if r.mcmPrintings == nil {
		r.mcmPrintings = map[int][]string{}
		for _, uuid := range r.backend.GetUUIDs() {
			co, err := r.backend.GetUUID(uuid)
			if err != nil {
				continue
			}
			mcmID, err := strconv.Atoi(co.Identifiers["mcmId"])
			if err != nil {
				continue
			}
			// A printing's foil and etched siblings share its
			// identifiers, mtgjsonId included, so it is listed once.
			printing := cmp.Or(co.Identifiers["mtgjsonId"], uuid)
			if !slices.Contains(r.mcmPrintings[mcmID], printing) {
				r.mcmPrintings[mcmID] = append(r.mcmPrintings[mcmID], printing)
			}
		}
		for _, printings := range r.mcmPrintings {
			slices.Sort(printings)
		}
	}
	return r.mcmPrintings[id]
}

// yugiohNumberTaken reports whether one of the numbers names a card of the
// set other than the one named.
func (r *resolver) yugiohNumberTaken(setCode string, numbers []string, name string) bool {
	index := r.yugiohNumbers()[setCode]
	for _, number := range numbers {
		holder, held := index[strings.ToUpper(number)]
		if held && holder != matcher.Normalize(name) {
			return true
		}
	}
	return false
}
