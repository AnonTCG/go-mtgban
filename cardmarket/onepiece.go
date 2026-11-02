package cardmarket

import (
	"cmp"
	"fmt"
	"strings"

	cm "github.com/mtgban/go-cardmarket"

	"github.com/mtgban/go-mtgban/matcher"
)

// onePieceShelves spells the shelves Cardmarket names apart from the set of
// ours they sell.
var onePieceShelves = map[string]string{
	"Reprints":   "Revision Pack Cards",
	"Demo Decks": "One Piece Demo Deck Cards",
}

// onePieceShelfLabels names the event the products of a promo shelf were
// handed out at, which the datastore labels the printing with.
var onePieceShelfLabels = map[string]string{
	"Store Tournament Promos": "Tournament Pack",
	"Winner Cards":            "Winner Pack",
}

// onePieceDons spells, by product id, the DON!! cards whose product name
// reads apart from the datastore's label.
var onePieceDons = map[int]struct{ edition, label string }{
	768199: {"", "One Piece Film RED Promo"},
	873734: {"", "Nico Robin"},
	873735: {"", "Nico Robin Gold"},
	873730: {"", "Heroines Special Set"},
	873731: {"", "Heroines Special Set Gold"},
	873738: {"", "Boa Hancock"},
	873739: {"", "Boa Hancock Gold"},
	877822: {"One Piece Promotion Cards", "Japanese Version 3rd Anniversary Set"},
	882449: {"One Piece Promotion Cards", "Japanese Version 3rd Anniversary Set"},
	906844: {"One Piece Promotion Cards", "Flame-Flame Fruit Coliseum"},
	906864: {"One Piece Promotion Cards", "English Version 3rd Anniversary Set Ace, Luffy, Sabo"},
}

// onePieceNonEnglish holds the products Cardmarket shelves among the English
// cards that were printed in another language alone: the Première Édition
// starter leaders, the French 1st Anniversary Pack, the French-exclusive
// Treasure Rares, the Japanese DON!! cards listed as the V.2 of an
// English one, and the Japanese and Chinese event DON!! cards (Kumamoto,
// Saikyo Jump, the Chinese anniversaries) the datastore carries no row of,
// and the promos CardTrader files under Asian Promos, Chinese Exclusives or
// French Promos with no TCGplayer id. Their wording reaches the English
// printing.
var onePieceNonEnglish = map[int]bool{
	818082: true, 818083: true, 818084: true, 818085: true, 818086: true, 818087: true,
	818088: true, 818089: true, 818090: true, 818091: true, 818092: true, 818093: true,
	878040: true, 878041: true, 878042: true, 878043: true, 878044: true, 878045: true, 878046: true,
	814438: true, 817152: true, 830578: true, 843043: true, 859007: true, 868221: true, 880301: true,
	874451: true, 883576: true, 883577: true, 883578: true, 884708: true, 901320: true, 911358: true,
	878056: true, 878057: true, 878058: true, 878059: true, 878060: true, 878061: true,
	878062: true, 878063: true, 878064: true, 878065: true,
	850409: true, 858758: true, 867220: true, 870125: true, 808800: true, 866419: true,
	916712: true, 792758: true, 845697: true, 845698: true, 847934: true,
	818112: true, 818114: true, 707719: true, 707720: true, 722776: true, 722779: true,
	818077: true, 818110: true, 866414: true, 916701: true, 916705: true, 916707: true,
	916744: true, 912173: true,
}

// onePieceEventLinks names, by Cardmarket product id, the TCGplayer product
// of the event promos CardTrader links with an id of 0: the datastore
// labels each with its event, and neither the wording nor the bridge says
// which one a product sells. The bridge answers first.
var onePieceEventLinks = map[int]int{
	776930: 552131, 780212: 552138, 787474: 580043, 787493: 580052, 787496: 580053,
	792749: 552131, 794716: 584365, 794717: 583773, 794718: 584366,
	806194: 607976, 806198: 607978, 806200: 607979,
	825209: 641163,
	837221: 681246, 837872: 646747, 840669: 646734,
	840672: 646733, 840673: 646731, 840688: 646720, 840710: 646727,
	845293: 539552, 845294: 525694, 845295: 539549, 845297: 539553,
	845298: 539557, 845299: 539555, 845300: 539550, 845301: 539548,
	845302: 539554, 845303: 539551, 850914: 658338, 850915: 658340,
	854473: 656608, 854474: 656609,
	858533: 668173, 858534: 668174, 858535: 668175, 858536: 668177,
	858537: 668179, 858538: 668181, 858539: 668182,
	858759: 659595, 858766: 661690,
	858767: 661692, 858768: 661693, 858769: 661694, 858773: 661702,
	866566: 671734, 866567: 671741, 866568: 671735, 867182: 672373,
	867183: 672374, 867185: 672376, 867187: 672378, 869926: 669279,
	869927: 669293, 869928: 669296, 874225: 657221, 882469: 675742,
	882471: 684110, 882472: 683981,
	896412: 708457,
	901276: 721093, 901282: 710742, 901283: 710741, 901319: 717459, 901342: 712840,
	901343: 712842, 901344: 712844, 901345: 712848, 901346: 712846,
	901347: 712851, 904362: 721297, 904363: 710222, 904370: 710237,
	904371: 710276, 904372: 710277, 904374: 721296, 904376: 710212,
	904378: 710213, 904380: 710214,
	904382: 710215, 904384: 710216, 904386: 710218, 904387: 710254,
	904388: 710219, 904390: 710221, 904398: 709561, 904401: 710245, 904415: 710728,
	904418: 710729, 904421: 710730, 904424: 710731, 912181: 710271,
	912182: 710270, 914111: 722631, 914113: 722640, 914116: 722634,
	914117: 722637,
}

// tcgLink answers the TCGplayer id a One Piece product is known by: the
// bridge's, then onePieceEventLinks'.
func (r *resolver) tcgLink(productID int) (int, bool) {
	tcgID, found := r.tcgBridge[productID]
	if found {
		return tcgID, true
	}
	tcgID, found = onePieceEventLinks[productID]
	return tcgID, found
}

// onePieceByID answers a One Piece product from the TCGplayer id tcgLink
// gives it, naming its printing outright. An empty id under a nil error leaves the
// product to its wording.
func (r *resolver) onePieceByID(product *cm.Product) (string, error) {
	tcgID, found := r.tcgLink(product.IDProduct)
	if !found {
		return "", nil
	}
	id, err := r.backend.MatchID(fmt.Sprint(tcgID), r.foilVersions[product.IDProduct])
	if err != nil || offCode(r.backend, product, id) {
		return "", nil
	}
	if r.offShelf(product, id) || r.notPreErrata(product, id) {
		return "", errNoPrinting
	}
	return id, nil
}

// onePieceTrophy reports whether a product is a serialized trophy card: an
// event prize, no card of the game, priced only where an id links it.
func onePieceTrophy(product *cm.Product) bool {
	return strings.HasSuffix(versionTail.ReplaceAllString(product.Name, ""), "Trophy Card")
}

// onePieceDeckReprint reports whether a starter deck's card is a booster
// card reprinted under its own code, which no catalog lists apart: the
// name lands on the booster printing another shelf prices already. A
// product linked to a TCGplayer product the datastore carries names a
// printing of its own, whose absence is a refusal; a link to a TCGplayer
// product the datastore does not carry (deleted, or not minted yet) names
// nothing.
func (r *resolver) onePieceDeckReprint(product *cm.Product) bool {
	if !strings.HasPrefix(product.ExpansionName, "Starter Deck:") {
		return false
	}
	tcgID, linked := r.tcgLink(product.IDProduct)
	return !linked || r.backend.ConvertID(matcher.IDSpaceTCGplayer, fmt.Sprint(tcgID)) == ""
}

// offCode reports whether the product's name carries a collector number
// other than the printing's. The bridge's links are another marketplace's,
// and one pointing at the neighbouring product names the wrong card.
func offCode(b *matcher.Backend, product *cm.Product, cardID string) bool {
	fields := nameCode.FindStringSubmatch(versionTail.ReplaceAllString(product.Name, ""))
	if fields == nil {
		return false
	}
	co, err := b.GetUUID(cardID)
	return err == nil && !strings.EqualFold(co.Number, fields[1])
}

// claimByID fills claimed with the printings the walk's products are
// answered with by id, before any product is named by its wording.
func (r *resolver) claimByID(byExpansion map[int][]int, products map[int]cm.CatalogProduct, items []cm.Expansion) {
	r.claimed = map[string]bool{}
	r.foilSold = map[string]bool{}
	r.foilVersions = foilVersions(r.tcgLink, byExpansion, products, items)
	for _, exp := range items {
		for _, id := range byExpansion[exp.IDExpansion] {
			product := &cm.Product{IDProduct: id, Name: products[id].Name, ExpansionName: exp.Name}
			cardID, err := r.onePieceByID(product)
			if err != nil || cardID == "" {
				continue
			}
			r.claimed[cardID] = true
			if r.foilVersions[id] {
				r.foilSold[cardID] = true
			}
			foilID, err := r.backend.MatchID(cardID, true)
			if err == nil {
				r.claimed[foilID] = true
			}
		}
	}
}

// foilVersions names the products link gives the same TCGplayer product as
// a lower version on their shelf. CardTrader keeps one blueprint
// for both finishes of a The Best DON!!, where Cardmarket sells the foil as
// the card's V.2.
func foilVersions(link func(int) (int, bool), byExpansion map[int][]int, products map[int]cm.CatalogProduct, items []cm.Expansion) map[int]bool {
	foils := map[int]bool{}
	for _, exp := range items {
		lowest := map[int]int{}
		for _, id := range byExpansion[exp.IDExpansion] {
			tcgID, found := link(id)
			if !found {
				continue
			}
			low, seen := lowest[tcgID]
			if !seen || products[id].Version < low {
				lowest[tcgID] = products[id].Version
			}
		}
		for _, id := range byExpansion[exp.IDExpansion] {
			tcgID, found := link(id)
			if found && products[id].Version > lowest[tcgID] {
				foils[id] = true
			}
		}
	}
	return foils
}

// giveWay refuses a One Piece product whose wording landed on a printing
// another product is answered with by id. Cardmarket sells the treasure
// rares, winner copies and event stamps the datastore does not carry beside
// the printing they reprint, and the wording reaches that printing for all
// of them, publishing a second price for it.
func (r *resolver) giveWay(results []resolved) {
	for i, res := range results {
		if res.err != nil || !r.claimed[res.cardID] {
			continue
		}
		id, err := r.onePieceByID(res.product)
		if err == nil && id != "" {
			continue
		}
		results[i] = resolved{product: res.product, err: errTwin}
	}
}

// resolveOnePiece names a One Piece product's printing: the one the
// datastore records under the product's id, the pre-errata printings no
// TCGplayer product sells; else the one onePieceByID links; else the one its
// name, number and version index reach, held to the shelf that sells it.
func resolveOnePiece(r *resolver, product *cm.Product) (string, string, bool, error) {
	if onePieceNonEnglish[product.IDProduct] {
		return "", "", false, errForeign
	}
	cardID := r.backend.ConvertID(matcher.IDSpaceCardmarket, fmt.Sprint(product.IDProduct))
	if cardID != "" {
		return r.withFoil(cardID)
	}
	id, err := r.onePieceByID(product)
	if err != nil {
		return "", "", false, err
	}
	if id != "" {
		return r.withFoil(id)
	}
	if onePieceTrophy(product) {
		return "", "", false, nil
	}

	fields := strings.SplitN(product.Name, " (V.", 2)
	cardName := fields[0]
	// Alias the shelf before picking the number: onePieceNumber only trusts
	// the name's own code on a shelf naming a set of ours, and Reprints/Demo
	// Decks only do that once aliased.
	edition := cmp.Or(onePieceShelves[product.ExpansionName], product.ExpansionName)
	number := onePieceNumber(r.backend, cardName, product.Number, edition)
	// The V-index Cardmarket synthesizes for same-number siblings tells a
	// base art from its variants (V.1 the base, the rest its alternates).
	// The foil probes are inert here: both flags resolve to the same
	// printing.
	if len(fields) > 1 {
		number = strings.TrimSpace(number + " V." + strings.TrimSuffix(fields[1], ")"))
	}
	// Only the rarity tells an oversized card from the set's own at its
	// number; a card with no oversized printing is then unsupported.
	if product.Rarity == "Oversized" {
		number = strings.TrimSpace(number + " Oversized")
	}
	// Store Tournament Promos and Winner Cards drop the event their own
	// shelf names; the datastore labels the printing with it.
	if label, found := onePieceShelfLabels[product.ExpansionName]; found {
		number = strings.TrimSpace(number + " " + label)
	}
	// These DON!! products spell apart from the datastore's own label; carry
	// each by id instead of by its product wording.
	if don, found := onePieceDons[product.IDProduct]; found {
		cardName, edition, number = "DON!!", cmp.Or(don.edition, edition), don.label
	}

	// A miss is judged beside the product's siblings first - a version of a
	// card another version already priced is a twin, not a report - so the
	// walk says what is left to say; see twinsAmong.
	cardID, cardIDFoil, err := r.matchNumbered(cardName, edition, number, false)
	if err != nil || cardID == "" {
		return "", "", false, err
	}

	// One Piece is the catalog that files one card onto shelf after shelf;
	// see offShelf.
	if r.offShelf(product, cardID) {
		if r.onePieceDeckReprint(product) {
			return "", "", false, errTwin
		}
		return "", "", false, errNoPrinting
	}
	// A pre-errata printing the datastore records under a sibling product
	// is that sibling's, whatever the name reaches.
	if r.ownedElsewhere(product, cardID) {
		return "", "", false, errNoPrinting
	}
	// A product on the pre-errata shelf lands on a pre-errata printing or
	// not at all; see notPreErrata.
	if r.notPreErrata(product, cardID) {
		return "", "", false, errNoPrinting
	}
	return cardID, cardIDFoil, false, nil
}
