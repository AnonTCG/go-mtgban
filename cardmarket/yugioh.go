package cardmarket

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	cm "github.com/mtgban/go-cardmarket"

	"github.com/mtgban/go-mtgban/matcher"
)

// yugiohTokenPrefixes gives the collector number each Token Promos shelf's
// tokens are numbered with. Cardmarket names a token by its art ("Grinder
// Token", "Judge Token") and the datastore by the card it stands for ("Token:
// Grinder Golem"), so only the number names the printing.
var yugiohTokenPrefixes = map[string]string{
	"Token Promos 1": "TKN1-EN",
	"Token Promos 3": "TKN3-EN",
	"Token Promos 4": "TKN4-EN",
}

// yugiohArtToken matches the Legendary Duelists art tokens, which the
// datastore names the other way round: "Art Token: Mai Valentine".
var yugiohArtToken = regexp.MustCompile(`^(.+) Art Token$`)

// yugiohTokenNumber answers the one printing of the product's shelf that
// carries the number the shelf's token prefix gives it, and nothing when
// there is none or the number is held twice.
func (r *resolver) yugiohTokenNumber(product *cm.Product) string {
	prefix := yugiohTokenPrefixes[product.ExpansionName]
	tail := numberTail.FindString(product.Number)
	if prefix == "" || tail == "" {
		return ""
	}
	var found string
	for _, edition := range yugiohEditions(product.ExpansionName) {
		set, err := r.backend.GetSetByName(edition)
		if err != nil {
			continue
		}
		for _, uuid := range r.backend.GetUUIDsInSet(set.Code) {
			co, err := r.backend.GetUUID(uuid)
			if err != nil || !strings.EqualFold(co.Number, prefix+tail) {
				continue
			}
			if found != "" {
				return ""
			}
			found = uuid
		}
	}
	return found
}

// yugiohNonCards names the products no printing of ours exists for, each
// checked against the whole datastore: the tokens and counters handed out
// at events, which no set lists, and the cards a duel is set up with.
var yugiohNonCards = map[string]bool{
	"Duelist Name Card":                    true,
	"Duelist ID Card":                      true,
	"Field Center Card":                    true,
	"QSCR Dummy":                           true,
	"OTS Token":                            true,
	"UDS Judge Token":                      true,
	"UDE Token":                            true,
	"2025 World Championship Token (Red)":  true,
	"2025 World Championship Token (Blue)": true,
}

// yugiohNonCard reports whether a product is one of the yugiohNonCards.
func yugiohNonCard(product *cm.Product) bool {
	return yugiohNonCards[strings.TrimSpace(versionTail.ReplaceAllString(product.Name, ""))]
}

// yugiohExpansions maps the Cardmarket Yu-Gi-Oh expansions the matcher
// resolves to no set onto the sets their bridged products land in, by code
// where the catalog's own name is nothing the storefront would write. Most
// entries were read off the cardtrader bridge: every product of the
// expansion that carries a TCGplayer id lands in the set named. The rest
// carry no bridged product at all and were read off the shelf's own
// Cardmarket expansion code equaling a datastore set code instead. A shelf
// spelled with a language code ("Spell Ruler (SDM)", "Legend of Blue Eyes
// White Dragon (LDD)") is a print in another language and stays out on
// purpose.
var yugiohExpansions = map[string][]string{
	"Legend of Blue Eyes White Dragon":             {"LOB"},
	"2025 Mega-Pack Tin":                           {"MP25"},
	"2020 Tin of Lost Memories Mega Pack":          {"MP20"},
	"Yugi's Legendary Decks":                       {"YGLD"},
	"Shonen Jump Magazine":                         {"JUMP", "JMPS", "JMP"},
	"Lost Art Promos":                              {"LART"},
	"Gold Series 5: Haunted Mine":                  {"GLD5"},
	"Gold Series 2":                                {"GLD2"},
	"Starter Deck: 2009":                           {"5DS2"},
	"Starter Deck: Syrus":                          {"YSDS"},
	"Starter Deck: GX 2006":                        {"YSD"},
	"Structure Deck: The Realm of Light":           {"SDLI"},
	"Yu-Gi-Oh! Championship Prize Cards 2025":      {"25YC"},
	"Yu-Gi-Oh! Championship Series":                {"YCSW"},
	"Premium Collection":                           {"PRC1"},
	"Turbo Pack":                                   {"TU01"},
	"McDonald's Promo Pack 2":                      {"MDP2"},
	"World Championship Celebration Promos":        {"WCJPP"},
	"Booster Pack Tin":                             {"BPT", "BPT-1341"},
	"3D Bonds Beyond Time Movie Pack":              {"YMP1"},
	"Master Collection Vol. 2":                     {"MC2"},
	"The Falsebound Kingdom Promos":                {"TFK"},
	"Dawn of Destiny":                              {"DOD"},
	"Gameboy Worldwide Edition Promos":             {"GBI"},
	"Token Promos 1":                               {"TKN"},
	"Token Promos 2":                               {"TKN2"},
	"Token Promos 4":                               {"TKN", "JPRC"},
	"Promos":                                       {"MISC", "EFC1"},
	"Sneak Preview 2":                              {"SP2", "SP02"},
	"Mattel Action Figure Serie 1":                 {"MF01"},
	"Mattel Action Figure Serie 2":                 {"MF02"},
	"Yu-Gi-Oh! Early Days Collection":              {"EDC1"},
	"Duelist Pack Collection Tin 2011":             {"DPCT-DPC5"},
	"ZEXAL World Duel Carnival Promos":             {"ZDC1"},
	"World Championship 2004":                      {"WC4"},
	"World Championship 2005":                      {"WC5"},
	"5D's Over the Nexus Promotional Cards":        {"WC11"},
	"Yu-Gi-Oh! 5D's Wheelie Breakers Promos":       {"WB01"},
	"Hidden Arsenal: Special Edition":              {"HA04"},
	"Shonen Jump Championship Series":              {"G280", "SJCS"},
	"Speed Duel Starter Decks: Ultimate Predators": {"SS03"},
	"Exclusive Pack 1":                             {"EP1"},
	"Mattel Action Figure Serie 3":                 {"MF03"},
	"5D's Tag Force 5 Promotional Cards":           {"TF05"},
	"Master Collection Vol. 1":                     {"MC1"},
	"Speed Duel: Event Pack":                       {"EVSD"},
	"Legendary Collection":                         {"LC01"},
	"Sneak Preview 1":                              {"SP1"},
	"Yu-Gi-Oh! The Movie":                          {"MOV"},
	"GX Tag Force 3":                               {"GX06"},
	"Nightmare Troubadour":                         {"NTR"},
	"GX Tag Force (PSP)":                           {"GX02"},
	"Duelist Pack 1&2 Special Edition":             {"DPK"},
	"Pharaoh Tour 2005":                            {"PT1"},
	"Pharaoh Tour 2006":                            {"PT02"},
	"Pharaoh Tour 2007":                            {"PT03"},
	"Ultimate Edition Promotional Cards: Series 2": {"UE02"},
	"Fire Fists Special Edition":                   {"FFSE"},
	"Duelist Pack Collection Tin 2010":             {"DPCT-192", "DPCT"},
}

// yugiohShelfPatterns are the expansion families spelled one way per member,
// each family's members landing in the set of the same index: the Duelist
// League participation cards, the Champion and Astral packs, the yearly
// collector's tins and the Mega-Tin packs.
var yugiohShelfPatterns = []struct {
	re   *regexp.Regexp
	code string
}{
	{regexp.MustCompile(`^Duelist League (\d\d)$`), "DL$1"},
	{regexp.MustCompile(`^Champion Pack: Game (\w+)$`), "CP0%d"},
	{regexp.MustCompile(`^Astral Pack (\w+)$`), "AP0%d"},
	{regexp.MustCompile(`^Collector's Tins (\d{4})$`), "$1 Collectors Tin"},
	{regexp.MustCompile(`^(\d{4}) Mega-Tin Mega Pack$`), "$1 Mega-Tins Mega Pack"},
}

var ordinalWords = map[string]int{
	"One": 1, "Two": 2, "Three": 3, "Four": 4, "Five": 5, "Six": 6, "Seven": 7, "Eight": 8, "Nine": 9,
}

// yugiohEditions answers the sets a Cardmarket expansion may hold, by name
// or code, the expansion's own name last.
func yugiohEditions(expansion string) []string {
	if sets, found := yugiohExpansions[expansion]; found {
		return sets
	}
	for _, pattern := range yugiohShelfPatterns {
		m := pattern.re.FindStringSubmatch(expansion)
		if m == nil {
			continue
		}
		if strings.Contains(pattern.code, "%d") {
			if n, found := ordinalWords[m[1]]; found {
				return []string{strings.Replace(pattern.code, "%d", string(rune('0'+n)), 1)}
			}
			continue
		}
		return []string{pattern.re.ReplaceAllString(expansion, pattern.code)}
	}
	return []string{expansion}
}

// yugiohSameProduct reports whether two products are the same card sold
// twice: the same name once the version index is off it. The number is not
// asked, because the oldest sets are sold once more under a numbering
// shifted by a card or two, and two products named alike that reached one
// printing are that printing's however they are numbered.
func yugiohSameProduct(a, b *cm.Product) bool {
	return matcher.Normalize(versionTail.ReplaceAllString(a.Name, "")) == matcher.Normalize(versionTail.ReplaceAllString(b.Name, ""))
}

// yugiohVersionVariants names the printings a shelf sells under one name,
// one number and one rarity, in the order Cardmarket's version index counts
// them. Winner's Pack 2026-2027 hands the same forty cards out through three
// programmes and stamps each with the programme's mark; the datastore keeps
// the three apart by that stamp, and Cardmarket keeps them apart by nothing
// at all - V.1, V.2 and V.3 carry the same rarity, the same reprint count
// and a web address that only repeats the name. Without a label the three
// products alias onto the three rows and none of them prices.
//
// THE ORDER IS A DEFAULT, NOT A FACT. Nothing publishes it, and CardTrader,
// which arbitrates elsewhere, carries no WI26 expansion at all. What the
// published price guide says is that V.1 is the cheapest of the three on all
// 25 cards priced in every version, and that V.3 is the dearest on 21 of
// them - which fits OTS packs being the widely handed-out programme and the
// judge mark the scarce one, and is the whole of the evidence. The prices
// within one card run from 22.50 to 550, so a row found wrong is worth
// correcting here rather than reasoning about: swap two labels and the
// products follow.
//
// Two older shelves use the table with a firmer order. Tournament Pack 1
// numbers its European print "001" where the datastore says "E001", so V.2
// is that print. On a Duelist League shelf V.1 is the Blue print, the
// colour the bridged V.2 to V.4 never reach, and the later colours are the
// ones the CardTrader bridge lands those versions on: all 180 bridged
// products of the three shelves agree with the rows below.
var yugiohVersionVariants = map[string][]string{
	"WI26": {"OTS Stamp", "Regional Qualifier Stamp", "Judge Stamp"},
	"TP1":  {"", "European"},
	"DL09": {"Blue", "Green", "Bronze", "Silver"},
	"DL17": {"Blue", "Green", "Purple", "Red"},
	"DL18": {"Blue", "Green", "Purple", "Red"},
}

// yugiohOversized answers the oversized card a product names, which the
// shelf cannot. An oversized card is not the set's card: the marketplace
// files it under the deck it was handed out with - Machina Fortress under
// Structure Deck: Machina Mayhem, numbered SDMM-EN001 - and the datastore
// files it in the collector or value box it actually came in, which for that
// card is VBX. Set and number therefore disagree by construction, and the
// name and the tag are what is left.
//
// It answers only where exactly one printing of ours carries the name and
// the tag. Twenty-six of the thirty-eight oversized products have no such
// printing at all and stay refused, which is what they are: cards we do not
// carry.
func yugiohOversized(b *matcher.Backend, name string) (string, error) {
	uuids, err := b.SearchEquals(name)
	if err != nil {
		return "", errNoPrinting
	}
	var found string
	for _, uuid := range uuids {
		co, err := b.GetUUID(uuid)
		if err != nil || !slices.Contains(co.PromoTypes, "oversized") {
			continue
		}
		if found != "" {
			return "", errNoPrinting
		}
		found = uuid
	}
	if found == "" {
		return "", errNoPrinting
	}
	return found, nil
}

// yugiohIndexPrints names the print each version index sells on the oldest
// shelves, which Cardmarket splits one product per regional print: "E" the
// European, "" the North American, "A" the Asian English and "EN" the
// worldwide reprint. The catalog writes most of their numbers bare; see
// yugiohPrintNumber.
//
// Seller comments and the numbering give the order: Magic Ruler's first
// version runs to the European 130, its second stops at the North American
// 103. Duelist Pack: Kaiba and Yugi split the same way but number both
// prints alike, which is one row of ours.
var yugiohIndexPrints = map[string][]string{
	"Legend of Blue Eyes White Dragon": {"E", "", "A", "EN"},
	"Metal Raiders":                    {"E", "", "EN"},
	"Magic Ruler":                      {"E", ""},
	"Pharaoh's Servant":                {"E", "", "EN"},
	"Labyrinth of Nightmare":           {"E", "", "EN"},
	"Starter Deck: Yugi":               {"E", "", "A"},
	"Starter Deck: Kaiba":              {"E", "", "A"},
}

// yugiohPrintNumber answers the product with the region prefix its version
// index stands for written onto a bare number, so the European print is not
// taken for the North American row the bridge links it to, and a reprint
// the catalog numbered bare still reaches its own set.
func yugiohPrintNumber(product *cm.Product) (*cm.Product, error) {
	prints := yugiohIndexPrints[product.ExpansionName]
	if prints == nil {
		return product, nil
	}
	index := cm.ProductVersion(product)
	// A product with no index is one the split left behind, with no listings.
	if index < 1 {
		return nil, errTwin
	}
	if index > len(prints) || prints[index-1] == "" || product.Number == "" || numberPrefix(product.Number) != "" {
		return product, nil
	}
	prefixed := *product
	prefixed.Number = prints[index-1] + product.Number
	return &prefixed, nil
}

// yugiohPromoLinks names, by Cardmarket product id, the TCGplayer product of
// the Reshef of Destruction promos, which the datastore files in the video
// game promos as ROD-EN001 and ROD-EN002. Cardmarket numbers them EN1 and 001
// and CardTrader links neither.
var yugiohPromoLinks = map[int]int{
	244606: 23322, 244607: 23323, 363275: 130171, 363276: 130170,
}

// yugiohNoRow lists the products of a printing the datastore does not carry,
// which the bridge or the name would land on a sibling's row.
var yugiohNoRow = map[int]bool{
	// Effect Veiler, 25YC-EN002 World Championship Qualifier: TCGplayer's
	// one product is the YCS Stamp, the row cardtrader.tcgIDOverrides gives it.
	829741: true,
}

// yugiohByID answers a Yu-Gi-Oh product yugiohPromoLinks names; an empty id
// leaves the product to the bridge and its wording.
func (r *resolver) yugiohByID(product *cm.Product) string {
	tcgID, found := yugiohPromoLinks[product.IDProduct]
	if !found {
		return ""
	}
	uuid, _ := r.backend.MatchID(strconv.Itoa(tcgID), false)
	return uuid
}

// yugiohOtherCard reports whether the bridged printing is of a card the
// product's name is not, under any name Konami gave it.
func (r *resolver) yugiohOtherCard(product *cm.Product, cardID string) bool {
	co, err := r.backend.GetUUID(cardID)
	if err != nil {
		return false
	}
	name := versionTail.ReplaceAllString(product.Name, "")
	konamiID := co.Identifiers["konamiId"]
	if konamiID == "" || matcher.Equals(co.Name, name) {
		return false
	}
	uuids, err := r.backend.SearchEquals(name)
	if err != nil {
		return false
	}
	for _, uuid := range uuids {
		named, err := r.backend.GetUUID(uuid)
		if err == nil && named.Identifiers["konamiId"] == konamiID {
			return false
		}
	}
	return true
}

// yugiohOtherNumber reports whether the bridged printing is numbered for
// another printing of the card: a special edition, box topper or European
// print that CardTrader links to the set's base row. Only the digits count,
// and the EN infix a European number ("EN086", "E001") needs; Cardmarket
// writes "SP02" where the datastore writes "ENS02".
func (r *resolver) yugiohOtherNumber(product *cm.Product, cardID string) bool {
	co, err := r.backend.GetUUID(cardID)
	if err != nil {
		return false
	}
	tail, rowTail := numberTail.FindString(product.Number), numberTail.FindString(co.Number)
	if tail == "" || rowTail == "" {
		return false
	}
	if !strings.EqualFold(strings.TrimLeft(tail, "0"), strings.TrimLeft(rowTail, "0")) {
		return true
	}
	return strings.HasPrefix(numberPrefix(product.Number), "E") && !strings.HasPrefix(numberPrefix(co.Number), "EN")
}

// yugiohWorded answers the printing the product's name and rarity reach,
// when the rarity names it and not the bridged printing.
func (r *resolver) yugiohWorded(product *cm.Product, cardID string) string {
	fields := rarityTail.FindStringSubmatch(product.Name)
	if fields == nil {
		return ""
	}
	co, err := r.backend.GetUUID(cardID)
	if err != nil || rarityNames(fields[1], r.backend.RarityLabel(co.Rarity)) {
		return ""
	}
	id, err := r.matchYugioh(product)
	if err != nil || id == "" || id == cardID {
		return ""
	}
	alt, err := r.backend.GetUUID(id)
	if err != nil || !rarityNames(fields[1], r.backend.RarityLabel(alt.Rarity)) {
		return ""
	}
	// Keep the bridged run, which the columns are laid out by.
	run, err := r.backend.MatchIDFinish(id, co.Finish)
	if err == nil {
		id = run
	}
	return id
}

// yugiohShelfSet answers the printing the name reaches in the set the
// product's shelf is named for, when the bridge put the card in a set not
// named after the shelf: CardTrader links the 25th Anniversary Sanga of the
// Thunder to the Worldwide English row, which shares its number. A set named
// after the shelf, a 2020 date reprint or the movie pack's gold edition, is
// filed there on purpose and keeps the bridge.
func (r *resolver) yugiohShelfSet(product *cm.Product, cardID string) string {
	shelf, err := r.backend.GetSetByName(product.ExpansionName)
	if err != nil {
		return ""
	}
	co, err := r.backend.GetUUID(cardID)
	if err != nil || co.SetCode == shelf.Code {
		return ""
	}
	set, err := r.backend.GetSet(co.SetCode)
	if err != nil || strings.HasPrefix(set.Name, shelf.Name) {
		return ""
	}
	id, err := r.matchYugioh(product)
	if err != nil || id == "" {
		return ""
	}
	named, err := r.backend.GetUUID(id)
	if err != nil || named.SetCode != shelf.Code {
		return ""
	}
	return id
}

// rarityNames reports whether every word of the storefront's rarity is a
// word of the datastore's, which may decorate it ("Prismatic Collector's").
func rarityNames(worded, rarity string) bool {
	words := strings.Fields(strings.ToLower(strings.ReplaceAll(rarity, "'", "")))
	for _, word := range strings.Fields(strings.ToLower(strings.ReplaceAll(worded, "'", ""))) {
		if !slices.Contains(words, word) {
			return false
		}
	}
	return true
}

// resolveYugioh names a Yu-Gi-Oh product's printing. The catalog carries no
// collector number to tell its same-name products apart, so a product
// resolves through an id first - the one yugiohByID reads, then the
// TCGplayer id the bridge knows it by - and only falls back on what the
// catalog says of it. Its second column is the first edition's.
func resolveYugioh(r *resolver, product *cm.Product) (string, string, bool, error) {
	if yugiohNoRow[product.IDProduct] {
		return "", "", false, errNoPrinting
	}
	// A product named by id is that printing; its wording reads as a
	// European print or another card.
	cardID := r.yugiohByID(product)
	byID := cardID != ""
	if !byID {
		var err error
		product, err = yugiohPrintNumber(product)
		if err != nil {
			return "", "", false, err
		}
	}
	tcgID, found := r.tcgBridge[product.IDProduct]
	if cardID == "" && found {
		cardID, _ = r.backend.MatchID(fmt.Sprint(tcgID), false)
	}
	// CardTrader's tcg id can name a sibling rarity, another card, another
	// printing's number or another set; distrust it.
	var byName bool
	if cardID != "" && !byID {
		if r.yugiohOtherCard(product, cardID) || r.yugiohOtherNumber(product, cardID) {
			cardID = ""
		} else {
			id := r.yugiohWorded(product, cardID)
			if id == "" {
				id = r.yugiohShelfSet(product, cardID)
			}
			if id != "" {
				cardID, byName = id, true
			}
		}
	}
	// The bridge speaks through cardtrader's blueprints and knows only half
	// of the catalog, and what it leaves out is ordinary cards. They can be
	// named without it.
	if cardID == "" {
		var err error
		cardID, err = r.matchYugioh(product)
		if err != nil {
			if errors.Is(err, errNoPrinting) {
				err = r.noPrinting(product)
			}
			return "", "", false, err
		}
		byName = cardID != ""
	}
	if cardID == "" {
		return "", "", false, r.noPrinting(product)
	}
	// The second printing is the first edition, a print run rather than a
	// foil, so the flag cannot name it. Naming the run reaches it, and errors
	// into an empty id for the products sold in no first edition.
	cardIDFoil, _ := r.backend.MatchIDFinish(cardID, "1st Edition")
	return cardID, cardIDFoil, byName, nil
}
