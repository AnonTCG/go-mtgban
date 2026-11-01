package cardmarket

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	cm "github.com/mtgban/go-cardmarket"

	"github.com/mtgban/go-mtgban/matcher"
)

// pokemonForeignExpansions names the Cardmarket Pokemon expansions that are
// Japanese or other non-English catalogs wearing an English set's name, so
// GetSetByName resolves them onto the English set: "Mystery of the Fossils"
// is Japan's Fossil, "Pokémon Jungle" trims down to Jungle, and "XY Promos"
// is the very name the datastore gives the English promos Cardmarket sells
// as "XY Black Star Promos"; "BW Promos" is likewise Japan's BW-P, the
// English shelf being "BW Black Star Promos". Without the gate their singles
// price English printings from another market's stock - 115 wrong landings
// when measured.
// The list is Cardmarket's own naming, which is why it lives here and not
// in the matcher: other storefronts mean the English set by some of these
// spellings.
//
// What proves each entry is the CardTrader bridge: none of the expansions'
// products link to a TCGplayer id in the English Pokemon catalog, while a
// genuinely English expansion's products overwhelmingly do. The bridge
// route stays open for them and answers nothing for the same reason.
//
// The value says whether a product naming a collector number may pass. The
// datastore's "XY Promos" set itself carries twelve Japanese promos under
// their ##/XY-P numbers - Mega Tokyo's Pikachu, the poncho Pikachus - and
// Cardmarket's numbered products land exactly on them; only its unnumbered
// products land on English printings. Everywhere else the wrong landings
// carry numbers too, so nothing passes.
var pokemonForeignExpansions = map[string]bool{
	"Advent of Arceus":                       false,
	"BW Promos":                              false,
	"Great Detective Pikachu":                false,
	"Magma Gang VS Aqua Gang: Double Crisis": false,
	"Mystery of the Fossils":                 false,
	"Pikachu Legendary Celebration":          false,
	"Pokémon Jungle":                         false,
	"Scarlet & Violet Battle Academy":        false,
	"The Glory of Team Rocket":               false,
	"XY Promos":                              true,
	// A shelf of Japanese vending, CD and magazine promos, the "_____'s
	// Pikachu" and "Imakuni?" kind, with no number on any of them.
	"Unnumbered Promos": false,
}

// pokemonCrossedLinks lists the products whose CardTrader link names another
// card's TCGplayer product, or the one basic energy of many printings; they
// resolve by their wording instead.
var pokemonCrossedLinks = map[int]bool{
	864380: true, // Lunatone, linked to Latios
	883858: true, // Gardevoir ex, linked to Scream Tail
	883865: true, // Scream Tail, linked to Gardevoir ex
	651753: true, // Metal Energy, linked to the HGSS basic
}

// pokemonStampLinks names, by Cardmarket product id, the TCGplayer product
// of the stamped, league, Countdown Calendar and promo copies Cardmarket
// sells under the plain card's name and number, or under a deck slot's
// number, which neither the bridge nor the wording tells from the plain
// card.
var pokemonStampLinks = map[int]int{
	280585: 488813, 295325: 131300, 295411: 131301, 295426: 131303,
	319255: 131861, 371657: 158422, 567076: 145860, 722693: 635612,
	722697: 600060, 722699: 681598, 749041: 527882, 749909: 545659,
	762531: 586555, 800144: 599656, 800837: 620552, 804338: 604492,
	819347: 628291, 833946: 634333, 841258: 664140, 841273: 663020,
	858716: 610125, 858724: 616155, 858734: 654647, 862171: 658501,
	882856: 232876, 901195: 181559,
	359292: 168219, 359295: 168220, 359322: 168222, 359325: 168221,
	550116: 219066, 550326: 219188, 764550: 658742, 833950: 219067,
	833952: 219065, 833969: 219099, 834011: 219165, 855028: 696170,
	855691: 702301, 868033: 724661, 868432: 681248, 882196: 153104, 882197: 153094,
	882198: 153106, 882199: 153108, 882200: 153083, 882201: 153089,
	882202: 153092, 882203: 153095, 882205: 153084, 882207: 153085,
	882210: 153096, 882214: 153105, 882215: 153107, 882216: 153110,
	882217: 153087, 882218: 153091, 882219: 153086, 882220: 153093,
	882221: 153088, 882222: 153090, 882223: 153097, 882224: 153102,
	882225: 153109, 894193: 704471, 916454: 714875, 916463: 714846,
	916469: 714870, 917602: 723261,
}

// pokemonHoloLinks names, by Cardmarket product id, the TCGplayer product
// whose Holofoil printing a product sells. TCGplayer files the holo and the
// plain card of a McDonald's Collection 25th Anniversary card or a Prize
// Pack Series card under one product, and Cardmarket sells them as two,
// which the plain card's product holds. The rows go once datastore-gen
// publishes these products' cardmarketId.
var pokemonHoloLinks = map[int]int{
	538783: 232316, 538793: 232317, 538803: 232318, 538813: 232319,
	538823: 232320, 538833: 232321, 538843: 232322, 538853: 232323,
	538863: 232324, 538873: 232325, 538883: 232326, 538893: 232327,
	538903: 232328, 538913: 232329, 538923: 232341, 538933: 232330,
	538943: 232331, 538953: 232332, 538963: 232333, 538973: 232334,
	538983: 232335, 538993: 232336, 539003: 232337, 539013: 232338,
	539023: 232339, 782313: 515640, 782315: 515639, 782317: 515644,
	782319: 515641, 782321: 515643, 782323: 515638, 782325: 515635,
	782327: 515642, 835690: 489332, 835691: 515539, 835692: 515463,
	835693: 515464, 835694: 515349, 835697: 515470, 835698: 515474,
	835699: 515476, 835703: 489354, 835704: 489322, 835705: 515480,
	835707: 515582, 835708: 515583, 835710: 515452, 835712: 515458,
	835783: 489333, 835784: 489332, 835787: 476104, 835788: 489334,
	835789: 489335, 835790: 476106, 835791: 489355, 835792: 489347,
	835793: 476011, 835794: 476117, 835795: 489356, 835796: 489348,
	835797: 476051, 835798: 476003, 835799: 476079, 835800: 489354,
	835801: 476025, 835802: 489322, 835803: 489323, 835804: 475994,
	835805: 476100, 835806: 489336, 835807: 489350, 835808: 476110,
	835809: 476134, 835810: 476115, 835811: 476123, 835812: 476124,
	835813: 476135, 835814: 489337, 835815: 476038, 835816: 489328,
	835817: 476086, 835818: 476131, 835819: 476095, 835820: 476132,
	835821: 476082, 835822: 476104, 835824: 476106, 835825: 476001,
	835826: 476113, 835827: 476011, 835828: 476117, 835829: 476003,
	835830: 476002, 835831: 476079, 835832: 476025, 835833: 476085,
	835834: 476091, 835835: 476038, 835836: 476098, 835837: 476100,
	835838: 476103, 835839: 476107, 835840: 476110, 835841: 476134,
	835842: 476115, 835843: 476122, 835844: 476135, 835845: 476076,
	835846: 476123, 835847: 476124, 835848: 476137, 894237: 703865,
	894239: 703866, 894241: 703867, 894243: 703868, 894245: 703869,
	894248: 703870, 894250: 703871,
}

// pokemonByID answers a Pokemon product the datastore records by its
// Cardmarket id or pokemonStampLinks and pokemonHoloLinks name; an empty id
// leaves the product to the bridge and its wording. A table row the
// datastore cannot answer is an error rather than a fall through to the
// plain card the row exists to keep the product off.
func (r *resolver) pokemonByID(product *cm.Product) (string, error) {
	uuid := r.backend.ConvertID(matcher.IDSpaceCardmarket, strconv.Itoa(product.IDProduct))
	if uuid != "" {
		return uuid, nil
	}
	tcgID, found := pokemonStampLinks[product.IDProduct]
	if found {
		return r.backend.MatchID(strconv.Itoa(tcgID), false)
	}
	tcgID, found = pokemonHoloLinks[product.IDProduct]
	if !found {
		return "", nil
	}
	return r.backend.MatchIDFinish(strconv.Itoa(tcgID), "Holofoil")
}

// pokemonForeign reports whether an expansion is one of those catalogs.
func pokemonForeign(expansion string) bool {
	_, found := pokemonForeignExpansions[expansion]
	return found
}

// pokemonForeignDenied reports whether a Pokemon product from a foreign
// expansion must not reach the name path.
func pokemonForeignDenied(expansion, number string) bool {
	numberedOK, found := pokemonForeignExpansions[expansion]
	if !found {
		return false
	}
	return !(numberedOK && number != "")
}

// pokemonEnergyTypes are the nine kinds of basic energy. The catalog names
// one either way round - "Grass Energy" and "Basic Grass Energy" are both on
// the shelves - and the datastore carries both spellings too.
var pokemonEnergyTypes = []string{
	"Grass", "Fire", "Water", "Lightning", "Psychic",
	"Fighting", "Darkness", "Metal", "Fairy",
}

// pokemonBasicEnergy reports whether a name is a basic energy's. The whole
// name is read rather than a substring of it: "Rainbow Energy", "Jet Energy"
// and "Luminous Energy" are special energies that a shelf sells on their own
// account, and "Superior Energy Retrieval" is not an energy at all.
func pokemonBasicEnergy(name string) bool {
	name = strings.TrimSpace(name)
	if len(name) > len("Basic ") && strings.EqualFold(name[:len("Basic ")], "Basic ") {
		name = name[len("Basic "):]
	}
	for _, kind := range pokemonEnergyTypes {
		if strings.EqualFold(name, kind+" Energy") {
			return true
		}
	}
	return false
}

// pokemonExpansion is what a Cardmarket expansion name means in the
// catalog's terms: the sets its products may be printings of, tried in
// order, and the programme prefix the promo numbers are written without.
type pokemonExpansion struct {
	sets   []string
	prefix string
}

// pokemonExpansions maps the Cardmarket Pokemon expansions the matcher
// resolves to no set onto the sets their bridged products land on. Each
// entry was read off the cardtrader bridge: the products of the expansion
// that carry a TCGplayer id land in the set named, and the name path is
// held to the same set.
var pokemonExpansions = map[string]pokemonExpansion{
	"Scarlet & Violet":          {sets: []string{"SV01: Scarlet & Violet Base Set"}},
	"Sword & Shield":            {sets: []string{"SWSH01: Sword & Shield Base Set"}},
	"Sun & Moon":                {sets: []string{"SM Base Set"}},
	"151":                       {sets: []string{"SV: Scarlet & Violet 151"}},
	"XY Black Star Promos":      {sets: []string{"XY Promos"}, prefix: "XY"},
	"SM Black Star Promos":      {sets: []string{"SM Promos"}, prefix: "SM"},
	"SWSH Black Star Promos":    {sets: []string{"SWSH: Sword & Shield Promo Cards"}, prefix: "SWSH"},
	"SV Black Star Promos":      {sets: []string{"SV: Scarlet & Violet Promo Cards"}},
	"MEP Black Star Promos":     {sets: []string{"ME: Mega Evolution Promo"}},
	"BW Black Star Promos":      {sets: []string{"Black and White Promos"}, prefix: "BW"},
	"DP Black Star Promos":      {sets: []string{"Diamond and Pearl Promos"}, prefix: "DP"},
	"Southeast Asia Promos":     {sets: []string{"Southeast Asia Exclusives"}},
	"Professor Program":         {sets: []string{"Professor Program Promos"}},
	"Best of Game Cards Promos": {sets: []string{"Best of Promos"}},
	"Promos":                    {sets: []string{"League & Championship Cards", pokemonJumbo}},
	"XY Trainer Kit":            {sets: []string{"XY Trainer Kit: Sylveon & Noivern"}},
	"Futsal Promos":             {sets: []string{"Miscellaneous Cards & Products"}},
}

// pokemonJumbo is the set the datastore files the oversized cards in.
const pokemonJumbo = "Jumbo Cards"

// pokemonJumboHeld lists the oversized products whose jumbo row another
// product prices by its name, in an expansion of its own.
var pokemonJumboHeld = map[int]bool{
	362859: true, // Base Set Pikachu, held by the Power Magazine Pikachu
}

// pokemonAdditionals are the sets an "Additionals" expansion draws on: the
// stamped, cosmos, deck and blister printings the base set's own products
// are not. The base set is left out on purpose - the products here that
// are its own printings are its pattern reprints, which the name cannot
// tell from the plain card.
var pokemonAdditionals = []string{"Miscellaneous Cards & Products", "Deck Exclusives", "Blister Exclusives", pokemonJumbo}

var (
	pokemonPrizePack = "Play! Pokémon Prize Pack Series "
	pokemonWorlds    = regexp.MustCompile(`^WCD (\d{4})$`)
	pokemonBracket   = regexp.MustCompile(`\s*\[[^\]]*\]\s*$`)
)

// pokemonEditions answers the sets a Cardmarket expansion may hold and the
// number prefix its promos need.
func pokemonEditions(b *matcher.Backend, expansion string) ([]string, string) {
	if alias, found := pokemonExpansions[expansion]; found {
		return alias.sets, alias.prefix
	}
	if strings.HasPrefix(expansion, pokemonPrizePack) {
		return []string{"Prize Pack Series Cards"}, ""
	}
	if m := pokemonWorlds.FindStringSubmatch(expansion); m != nil {
		return []string{m[1] + " World Championship Decks"}, ""
	}
	if base, found := strings.CutSuffix(expansion, ": Additionals"); found {
		if _, err := b.GetSetByName(base); err == nil {
			return pokemonAdditionals, ""
		}
	}
	return []string{expansion}, ""
}

// pokemonName reads the card's name off a product name: Cardmarket tells
// same-name products apart by their attacks in brackets ("Tapu Koko [Flying
// Flip | Electric Ball]"), writes an energy symbol inside a name as its
// letter in brackets ("Magnetic [M] Energy", "Unit Energy [LPM]") where the
// catalog spells one letter out and keeps several as they are, and pads
// some names with a trailing space.
func pokemonName(name string) string {
	name = pokemonBracket.ReplaceAllString(name, "")
	name = pokemonSymbol.ReplaceAllStringFunc(name, func(symbol string) string {
		letters := strings.Trim(symbol, "[]")
		if word, found := pokemonEnergyLetters[letters]; found {
			return word
		}
		return letters
	})
	return strings.Join(strings.Fields(name), " ")
}

var pokemonSymbol = regexp.MustCompile(`\[[A-Z]+\]`)

var pokemonEnergyLetters = map[string]string{
	"G": "Grass", "R": "Fire", "W": "Water", "L": "Lightning", "P": "Psychic",
	"F": "Fighting", "D": "Darkness", "M": "Metal", "Y": "Fairy", "N": "Dragon", "C": "Colorless",
}

// pokemonNonCards are the products a Pokemon shelf sells that are not cards.
// A booster box's VSTAR marker is a counter, printed on card stock and
// shelved beside the singles, and no catalog has a row for one.
var pokemonNonCards = []string{
	"Code Card",
	"VSTAR Marker",
}

// pokemonCodeCard reports whether a product is something other than a card -
// a code for the online game, or one of the markers a set ships with.
func pokemonCodeCard(name string) bool {
	for _, kind := range pokemonNonCards {
		if strings.Contains(name, kind) {
			return true
		}
	}
	return false
}

// twinsAmong refuses the by-name results whose printing a product the game
// calls the same in the same expansion already holds. Cardmarket sells a
// stamped, cosmos or deck variant as a second product with the same name and
// number and nothing else to tell it by, an unnumbered product beside a
// numbered one of the same name, and a print run as a version index; the
// name reaches the plain printing for all of them. An id-resolved sibling
// holds its printing outright; among by-name siblings the first holds it
// and the rest give way. A printing held by a product of another name is
// left to the inventory to refuse out loud, since that is a disagreement
// worth reading.
func twinsAmong(results []resolved, same func(a, b *cm.Product) bool, face func(product *cm.Product, cardID string) bool) {
	held := map[string][]int{}
	var holders []int
	for i, r := range results {
		if r.err == nil && r.cardID != "" && !r.byName {
			held[r.cardID] = append(held[r.cardID], i)
			holders = append(holders, i)
		}
	}
	for i, r := range results {
		if r.err == nil && r.cardID != "" && r.byName {
			twin := false
			for _, j := range held[r.cardID] {
				if same(results[j].product, r.product) || (face != nil && face(r.product, r.cardID)) {
					twin = true
					break
				}
			}
			if twin {
				results[i].cardID, results[i].cardIDFoil, results[i].err = "", "", errTwin
				continue
			}
			held[r.cardID] = append(held[r.cardID], i)
			holders = append(holders, i)
		}
	}
	// A miss beside a same-named product that holds a printing is the
	// same twin: the legacy unnumbered product of a card the expansion
	// sells numbered too, or the version of a promo the name alone cannot
	// tell from the versions already priced.
	for i, r := range results {
		if r.err == nil || errors.Is(r.err, errForeign) {
			continue
		}
		for _, j := range holders {
			if same(results[j].product, r.product) || (face != nil && face(r.product, results[j].cardID)) {
				results[i].err = errTwin
				break
			}
		}
	}
}

// pokemonSameProduct reports whether two products are named alike: the same
// card name once the shelf's own decorations are off it - the "(Theme
// Deck)" parenthetical, the "Basic" an energy is sometimes written with -
// and the same number, or none, once the letter a shelf hangs off it and
// the zeros it pads with are off.
func pokemonSameProduct(a, b *cm.Product) bool {
	if pokemonPlainName(a.Name) != pokemonPlainName(b.Name) {
		return false
	}
	return a.Number == "" || b.Number == "" || pokemonPlainNumber(a.Number) == pokemonPlainNumber(b.Number)
}

var pokemonParenthetical = regexp.MustCompile(`\s*\([^)]*\)`)

func pokemonPlainName(name string) string {
	name = pokemonParenthetical.ReplaceAllString(pokemonName(name), "")
	name = strings.TrimPrefix(name, "Basic ")
	return matcher.Normalize(name)
}

// pokemonPlainNumber is the number two products are compared on: the
// programme a promo's number is written with comes off first, and what is
// left is reduced the way the Pokemon rules reduce a plain number - the
// letters behind it and the zeros before it. The reduction is spelled here
// rather than asked of the matcher because the comparison runs on products
// before any backend is installed, in the tests that pin the twins.
func pokemonPlainNumber(number string) string {
	if m := pokemonPromoNumber.FindStringSubmatch(strings.TrimSpace(number)); m != nil {
		number = m[2]
	}
	number = strings.TrimRight(strings.TrimSpace(number), "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	if trimmed := strings.TrimLeft(number, "0"); trimmed != "" {
		number = trimmed
	}
	return number
}

// pokemonPromoCodes are the programmes a promo's number is written with
// when the card is sold on another shelf ("Flareon V SWSH 149" in a prize
// pack series): the number names the promo set outright.
var pokemonPromoCodes = map[string]string{
	"SWSH": "SWSH Black Star Promos",
	"SVP":  "SV Black Star Promos",
	"SM":   "SM Black Star Promos",
	"XY":   "XY Black Star Promos",
	"MEP":  "MEP Black Star Promos",
	"BW":   "BW Black Star Promos",
	"DP":   "DP Black Star Promos",
}

var pokemonPromoNumber = regexp.MustCompile(`^([A-Z]+) (\S+)$`)

// pokemonLettered matches a collector number with a letter hung off it,
// which is how the alternate-art promos are numbered after the card they
// reprint: "92a", and "XY198a" where the programme's prefix is written too.
var pokemonLettered = regexp.MustCompile(`^[A-Za-z]*\d+[a-z]$`)

// pokemonLetteredSets are the two sets those numbers belong to. Cardmarket
// shelves such a product under the set it reprints - "Field Blower 125a" on
// Guardians Rising - while the datastore files it with the programme that
// handed it out, and the two programmes number their cards the same way.
var pokemonLetteredSets = []string{"Alternate Art Promos", "League & Championship Cards"}

// pokemonFinishCell reports the (isFirstEd, isReverseHolo) Cardmarket query
// cell a Pokemon finish name projects onto. Cardmarket exposes only these
// two article-level flags for the game; holo-ness on its own has no flag at
// all, so several of the matcher's own finish names collapse onto the same
// cell - Normal, Holofoil, Unlimited and Unlimited Holofoil all read
// (false, false), confirmed live: Cardmarket sells an EX-era holo rare's
// plain and holofoil printings as one product, told apart only by which
// physical card is in hand, never a flag on the listing. The matcher's own
// vocabulary carries no finish crossing both axes at once, matching what
// was found live: zero real products carry both a 1st Edition and a Reverse
// Holofoil finish, so (true, true) is never produced here.
func pokemonFinishCell(finish string) (isFirstEd, isReverseHolo bool) {
	switch matcher.FinishSlug(finish) {
	case "reverseholofoil":
		return false, true
	case "1stedition", "1steditionholofoil":
		return true, false
	}
	return false, false
}

// pokemonFinishTarget is one Cardmarket query Market will make for a
// Pokemon product: the uuid to file the result under, and the flags to ask
// for and verify.
type pokemonFinishTarget struct {
	cardID        string
	isFirstEd     bool
	isReverseHolo bool
}

// pokemonFinishPlan groups cardID's own finish siblings (matcher.
// FinishSiblings, which already carries the game's whole vocabulary and
// handles finish twins) by the Cardmarket cell each projects onto (see
// pokemonFinishCell), keeping one anchor uuid per occupied cell - cardID
// itself for its own cell, otherwise the first FinishSiblings returns for
// that cell, which is already ordered base finish first, then the game's
// own vocabulary alphabetically. Any other sibling sharing that cell is a
// real, distinct printing this scraper cannot separately price:
// Cardmarket's own flags cannot tell it from the anchor, so querying it
// again would file the same listings under two uuids instead of one -
// confirmed live: 349 real products (EX-era holo rares) carry both a
// Normal and a Holofoil sibling as one Cardmarket product.
func pokemonFinishPlan(b *matcher.Backend, cardID string) []pokemonFinishTarget {
	type cell struct{ isFirstEd, isReverseHolo bool }
	anchors := map[cell]int{}
	var targets []pokemonFinishTarget
	for _, sibling := range b.FinishSiblings(cardID) {
		co, err := b.GetUUID(sibling)
		if err != nil {
			continue
		}
		isFirstEd, isReverseHolo := pokemonFinishCell(co.Finish)
		c := cell{isFirstEd, isReverseHolo}
		i, found := anchors[c]
		if found {
			if sibling == cardID {
				targets[i].cardID = cardID
			}
			continue
		}
		anchors[c] = len(targets)
		targets = append(targets, pokemonFinishTarget{cardID: sibling, isFirstEd: isFirstEd, isReverseHolo: isReverseHolo})
	}
	return targets
}

// pokemonStamped answers the stamped printing of a plain card that
// Cardmarket's own rarity, "Promo", says a product sells: the one row of the
// same name, number and set total in another set that carries a promo type.
// Rows spanning more than one TCGplayer product, or with a row of no
// TCGplayer product among them, name nothing, and the product stays with the
// plain card's other siblings.
func (r *resolver) pokemonStamped(plain *matcher.CardObject) string {
	if plain.IsPromo || len(plain.PromoTypes) > 0 || plain.Rarity == matcher.RarityName("Promo") {
		return ""
	}
	uuids, _ := r.backend.SearchEquals(plain.Name)
	var found, tcgID string
	for _, uuid := range uuids {
		co, err := r.backend.GetUUID(uuid)
		if err != nil || co.SetCode == plain.SetCode || co.Name != plain.Name {
			continue
		}
		if strings.TrimLeft(co.Number, "0") != strings.TrimLeft(plain.Number, "0") ||
			strings.TrimLeft(co.SetTotal, "0") != strings.TrimLeft(plain.SetTotal, "0") {
			continue
		}
		if len(co.PromoTypes) == 0 {
			continue
		}
		id := co.Identifiers["tcgplayerProductId"]
		if id == "" || (found != "" && id != tcgID) {
			return ""
		}
		if found == "" || co.Finish != matcher.FinishSlug(pokemonReverseHolo) {
			found, tcgID = uuid, id
		}
	}
	return found
}

// bridgeHolds reports whether the bridge links a product other than the one
// given to the TCGplayer product of cardID, which that product then prices.
func (r *resolver) bridgeHolds(cardID string, product int) bool {
	co, err := r.backend.GetUUID(cardID)
	if err != nil {
		return false
	}
	tcgID, err := strconv.Atoi(co.Identifiers["tcgplayerProductId"])
	if err != nil {
		return false
	}
	for id, linked := range r.tcgBridge {
		if linked == tcgID && id != product {
			return true
		}
	}
	return false
}
