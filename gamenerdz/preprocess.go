package gamenerdz

import (
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// preprocess turns a storefront product into the matcher's input, in the
// grammar its game's display names follow. The finish comes from the
// selectedFinish field, which names the printing in each game's own words;
// the display name restates it as a tail on the set, and for Pokemon
// pokemonStatedPrinting reads that tail where the field disagrees. Each
// game's resolver calls its own; this is the one entry the tests share.
func preprocess(b *matcher.Backend, product GNProduct, game matcher.Game) (*matcher.InputCard, error) {
	switch game {
	case matcher.GameLorcana:
		return preprocessLorcana(product)
	case matcher.GamePokemon:
		return preprocessPokemon(b, product)
	case matcher.GameOnePiece:
		return preprocessOnePiece(product)
	}
	return preprocessMagic(b, product)
}

// magicCode is the set-and-number tag Magic display names carry, like
// "(TLE-265)" in "Aang, Air Nomad (0265) (TLE-265) - ...": always the last
// such tag, after any bare display number the name also shows. The number
// may wear the marks the catalog numbers a variant with ("40K-86★",
// "FCA-063†"), the code an ampersand ("AFR&-53A"), and a List tag spells
// the origin set into the number ("LIST-C16-177", "LIST-229/350").
var magicCode = regexp.MustCompile(`\((?:[0-9A-Z]{2,6}, )?([0-9A-Z&]{2,6})-([0-9A-Za-z★†φ/-]*)\)`)

// skuFamily is the sku a storefront product shares with its own other
// finish: the whole of it less the segment that marks the finish.
// MTG-RNA-249-F-6NUARTU9FH and MTG-RNA-249-6NUARTU9FH are one product sold
// two ways, so either can be read for what the other's body says.
func skuFamily(product GNProduct) string {
	for _, variant := range product.RetailVariants {
		if variant.SKU == "" {
			continue
		}
		fields := strings.Split(variant.SKU, "-")
		family := make([]string, 0, len(fields))
		for i, field := range fields {
			if i > 0 && i < len(fields)-1 && (field == "F" || field == "EF") {
				continue
			}
			family = append(family, field)
		}
		return strings.Join(family, "-")
	}
	return ""
}

// skuMarker is the finish segment a product's sku carries, "F" or "EF", and
// empty when it carries none. A product with no sku at all has none either.
func skuMarker(product GNProduct) string {
	for _, variant := range product.RetailVariants {
		if variant.SKU == "" {
			continue
		}
		fields := strings.Split(variant.SKU, "-")
		for i := 1; i < len(fields)-1; i++ {
			if fields[i] == "F" || fields[i] == "EF" {
				return fields[i]
			}
		}
		return ""
	}
	return ""
}

// skuNumber is the digits of the collector number a product's sku carries:
// every segment between the set code and the ten-character random tail, so
// 249 in MTG-RNA-249-F-6NUARTU9FH and 1652 in the List's
// MTG-PLST-C16-52-AK6D7ICGZV. A sku with nothing there, like
// MTG-CHR-PGEXSBNC6R, says nothing.
func skuNumber(product GNProduct) string {
	for _, variant := range product.RetailVariants {
		fields := strings.Split(variant.SKU, "-")
		if len(fields) > 2 {
			end := 2
			for end < len(fields) && len(fields[end]) != 10 {
				end++
			}
			return numberDigits(strings.Join(fields[2:end], ""))
		}
	}
	return ""
}

// doubtfulFoil reports a product whose selectedFinish is the only thing
// saying foil: its sku carries no finish segment and its name no Foil tail.
// The field is wrong on some of these - a ninth edition land sold plain is
// flagged foil and lands its star twin - and right on the rest, where the
// product is a second listing of its foil sibling at the same price or the
// card was only ever printed foil. Its price tells which, once the sibling's
// is known.
func doubtfulFoil(product GNProduct) bool {
	if !strings.EqualFold(product.SelectedFinish, "foil") || nameSaysFoil(product.DisplayName) {
		return false
	}
	return skuMarker(product) == "" && skuFamily(product) != ""
}

// bodyNamesOwnSet reports whether a product's body names the set its own
// display name does. A product with no set tag to read has nothing to
// disagree with and counts as its own witness.
func bodyNamesOwnSet(product GNProduct) bool {
	tags := magicCode.FindAllStringSubmatch(product.DisplayName, -1)
	if len(tags) == 0 {
		return true
	}
	return strings.EqualFold(tags[len(tags)-1][1], string(product.ProductData.Set))
}

// magicBareNumber is the four digit collector number a few display names
// bracket before their code, "Eddie Brock (0233) (Borderless) (SPM-224)".
var magicBareNumber = regexp.MustCompile(`\((0\d{3})\)`)

// magicBodyTag is the set a display name's tag leads with when the tag names
// the body: "(SPE, MAR-005)" is a Marvel card whose body is Spider-Man
// Eternal's.
var magicBodyTag = regexp.MustCompile(`\(([0-9A-Z]{2,6}), [0-9A-Z&]{2,6}-`)

// foreignBody reports whether a product's body is a real set that neither the
// display's code, its leading body tag nor a List origin names, while the code
// names a known set: the body then belongs to another product. A body on one
// of the storefront's own shelves never qualifies.
func foreignBody(b *matcher.Backend, product GNProduct) bool {
	tags := magicCode.FindAllStringSubmatch(product.DisplayName, -1)
	if len(tags) == 0 || bodyNamesOwnSet(product) {
		return false
	}
	body := strings.ToLower(string(product.ProductData.Set))
	lead := magicBodyTag.FindStringSubmatch(product.DisplayName)
	if lead != nil && strings.EqualFold(lead[1], body) {
		return false
	}
	// A List tag spells the origin set into its number, "PLST-JMP-18".
	if strings.HasPrefix(strings.ToLower(tags[len(tags)-1][2]), body+"-") {
		return false
	}
	_, err := b.GetSet(body)
	if err != nil {
		return false
	}
	_, err = b.GetSet(tags[len(tags)-1][1])
	return err == nil
}

// magicOrigin is the bare set-code tag a List display name puts before the
// number when the number alone would not say which printing the card is
// reprinted from: "Ancestral Mask (MMQ) (LIST-229/350)".
var magicOrigin = regexp.MustCompile(`\(([0-9A-Z]{2,6})\) \(LIST-`)

// A Magic display name reads
//
//	Aang's Shelter - Teferi's Protection (Borderless) (TLE-007) - Avatar...
//
// The name stops at the first parenthesis: the collector number pins the
// printing, so variant wording like (Borderless) only restates it.
func preprocessMagic(b *matcher.Backend, product GNProduct) (*matcher.InputCard, error) {
	// The matcher refuses the Spanish Salvat reprint sets. The shelf the
	// name ends on is read, not the body, which can name Salvat's set under
	// a name that is another card's.
	tail := magicTailSet(product.DisplayName)
	if strings.HasPrefix(tail, "Salvat") {
		return nil, matcher.ErrUnsupported
	}
	// A front card is the art card on the face of a Jumpstart pack, not
	// a card; its name is a prefix of a real one and would land on it.
	if strings.HasSuffix(tail, "Front Cards") {
		return nil, matcher.ErrUnsupported
	}
	number, err := magicNumber(product)
	if err != nil {
		return nil, err
	}

	cardName := magicName(b, product.DisplayName)
	if respelled, found := magicRespellings[cardName]; found {
		cardName = respelled
	}
	cardName = strings.ReplaceAll(cardName, " / ", " // ")
	// The bare number a display writes ahead of its code, "(0233)", is the
	// printing's own, and the code's can name another card's of the same set.
	bare := magicBareNumber.FindStringSubmatch(product.DisplayName)
	if bare != nil {
		own := strings.TrimLeft(bare[1], "0")
		if own != strings.TrimLeft(number, "0") && len(b.MatchWithNumber(cardName, strings.ToUpper(string(product.ProductData.Set)), own)) > 0 {
			number = own
		}
	}
	fixed, found := magicNumberTypos[magicTypo{strings.ToLower(string(product.ProductData.Set)), cardName, number}]
	if found {
		number = fixed
	}

	edition := string(product.ProductData.Set)
	if edition == "" {
		edition = product.ProductData.SetName
	}
	shelfSet := magicShelves[product.ProductData.SetName]
	foil := strings.EqualFold(product.SelectedFinish, "foil") || nameSaysFoil(product.DisplayName)
	switch {
	// The promo pack shelves are coded with a second P in front of the
	// set the catalog files the packs' promos under ("ppeoe" for PEOE),
	// and the newer ones name no set by either name field. The code says
	// which set, and the number's own letter, when it has none, is the
	// pack's. The shelf also holds the set's own booster-fun printings,
	// numbered past the pack's range, so the plain set answers for a
	// number the promo set turns down.
	case !namesASet(b, edition) && strings.HasPrefix(edition, "pp") && namesASet(b, edition[1:]):
		promo := edition[1:]
		packed := number
		if !strings.ContainsAny(number, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			packed += "p"
		}
		edition, number = promo, packed
		_, err := b.Match(&matcher.InputCard{Name: cardName, Edition: promo, Variation: packed, Foil: foil})
		if err != nil && namesASet(b, promo[1:]) {
			edition, number = promo[1:], strings.TrimSuffix(packed, "p")
		}
	// The shelf's name is the catalog's own name for a set the code does
	// not say: the Timeshifts of a Modern Horizons set are coded with the
	// set's code and named as the set of their own the catalog files them
	// in, and the playtest cards of a year are coded with the first year's
	// code. A name that names no set leaves the code to answer, and a code
	// that names none either leaves it to the set the display name ends
	// on ("Duress (FNM-010) - Friday Night Magic 2005 Foil"), which is
	// where the year of a yearly shelf is written.
	case namesASet(b, product.ProductData.SetName):
		edition = product.ProductData.SetName
	case !namesASet(b, edition) && namesASet(b, magicTailSet(product.DisplayName)):
		edition = magicTailSet(product.DisplayName)
	case !namesASet(b, edition) && namesASet(b, shelfSet):
		edition = shelfSet
	// A yearly shelf is coded once for every year ("fnm" is the catalog's
	// code for the first Friday Night Magic year) and the year the product
	// is from is written only where the name ends.
	case sameShelfAnotherYear(b, edition, magicTailSet(product.DisplayName)):
		edition = magicTailSet(product.DisplayName)
	}

	deck := sldCommanderDeck(b, product.ProductData.SetName)
	if deck != "" {
		edition, number = secretLairDeckPrinting(b, deck, product.ProductData.SetName, cardName)
	}

	card := &matcher.InputCard{
		Name:      cardName,
		Edition:   edition,
		Variation: number,
		Foil:      foil,
	}

	// The play shelf holds Play Booster promo packs of older sets, numbered
	// as in the set they came from. Naming the stamp reaches the pack; a
	// card with no pack keeps the reading it had.
	if strings.EqualFold(string(product.ProductData.Set), playShelf) && !namesASet(b, edition) {
		probe := *card
		probe.Variation = strings.TrimSpace(number + " " + packStamp)
		_, err := b.Match(&probe)
		if err == nil {
			card.Variation = probe.Variation
		}
	}

	// The storefront files every prerelease stamp under one pseudo-set,
	// whatever set the card was printed for. That code names no set the
	// catalog has, so the edition selects nothing and the number answers
	// alone - with the ordinary printing, the one the stamp is not. The
	// number is the card's own, so naming the stamp is enough to reach the
	// prerelease printing without ever knowing which set it belongs to.
	// The catalog answering is what says the shelf was right: a handful of
	// ordinary cards sit on it by mistake, and no stamp to reach is what
	// tells them apart, so they keep the reading they already had. The
	// shelf is told by its code: a product on it may name the promo set
	// outright, which narrows the stamp to that set.
	if strings.EqualFold(string(product.ProductData.Set), preShelf) {
		stamped := strings.TrimSpace(number + " " + preStamp)
		probe := matcher.InputCard{
			Name:      cardName,
			Edition:   edition,
			Variation: stamped,
			Foil:      card.Foil,
		}
		_, err := b.Match(&probe)
		if err == nil {
			card.Variation = stamped
		}
		return card, nil
	}

	// A set's promos get a shelf code of the storefront's own - "ppthb" for
	// Theros Beyond Death Promos - which again names no set the catalog
	// has, so the edition selects nothing and the number answers with the
	// ordinary printing. The shelf's name is the catalog's own name for
	// that set, and is what says where to look. Most of these numbers
	// already carry the letter the catalog files a promo pack under, and
	// answer as they are; a number without it is still a promo pack, since
	// the prereleases this shelf would otherwise hold sit on their own. So
	// the plain reading is asked for first and the pack only when the
	// catalog turns it down, which is what keeps a promo that is neither
	// off a promo pack's printing.
	shelf := product.ProductData.CatalogSet
	if !namesASet(b, edition) && namesASet(b, shelf) {
		for _, variation := range []string{number, strings.TrimSpace(number + " " + packStamp)} {
			probe := matcher.InputCard{
				Name:      cardName,
				Edition:   shelf,
				Variation: variation,
				Foil:      card.Foil,
			}
			_, err := b.Match(&probe)
			if err != nil {
				continue
			}
			card.Edition = shelf
			card.Variation = variation
			break
		}
	}

	// The etched printing is a finish of its own, which the matcher reads off
	// the word in the variation. Asking for it can only improve on the reading
	// already reached: a card whose one finish is etched answers by number
	// alone and does not move, and one with no etched printing at all keeps
	// what it had. No product that says etched reaches the shelves returning
	// above this, so asking once here covers them all.
	if saysEtched(product) {
		probe := *card
		probe.Variation = strings.TrimSpace(card.Variation + " " + etchedStamp)
		_, err := b.Match(&probe)
		if err == nil {
			card.Variation = probe.Variation
		}
	}

	// The miscellaneous shelves number their products themselves ("UMP-003"
	// for a Costco bundle promo the catalog numbers 2025-13), so the number
	// says nothing to the catalog and the wording in the name's other
	// parentheses is what tells the printing. A number the set answers for
	// is kept; only one it turns down gives way to the wording.
	if !namesASet(b, string(product.ProductData.Set)) {
		probe := *card
		_, err := b.Match(&probe)
		if err != nil {
			wording := magicWording(product.DisplayName)
			var tries []matcher.InputCard
			for _, edition := range []string{card.Edition, product.ProductData.SetName, shelf} {
				if namesASet(b, edition) {
					tries = append(tries, matcher.InputCard{Name: cardName, Edition: edition, Variation: wording, Foil: card.Foil})
				}
			}
			// Failing that the card is looked for by name in the sets the
			// shelf holds its promos in.
			for _, edition := range magicPromoShelves[strings.ToLower(string(product.ProductData.Set))] {
				tries = append(tries, matcher.InputCard{Name: cardName, Edition: edition, Foil: card.Foil})
			}
			for _, try := range tries {
				_, err := b.Match(&try)
				if err == nil {
					card.Edition = try.Edition
					card.Variation = try.Variation
					break
				}
			}
		}
	}

	return card, nil
}

// sameShelfAnotherYear reports whether two editions name the same yearly
// shelf in different years: the same set name once a trailing year is off
// each, both naming a set of ours.
func sameShelfAnotherYear(b *matcher.Backend, code, tail string) bool {
	if tail == "" || !namesASet(b, code) || !namesASet(b, tail) {
		return false
	}
	coded, err := b.GetSetByName(code)
	if err != nil {
		return false
	}
	named, err := b.GetSetByName(tail)
	if err != nil || coded.Code == named.Code {
		return false
	}
	codedYear := magicYearTail.FindStringSubmatch(coded.Name)
	namedYear := magicYearTail.FindStringSubmatch(named.Name)
	return codedYear != nil && namedYear != nil && codedYear[1] == namedYear[1]
}

var magicYearTail = regexp.MustCompile(`^(.*\S)\s+(?:19|20)\d\d$`)

// magicTailSet reads the set a display name ends on, past the last dash,
// less the finish and grade the buylist tails it with.
func magicTailSet(displayName string) string {
	idx := strings.LastIndex(displayName, " - ")
	if idx == -1 {
		return ""
	}
	tail := magicRarityTail.ReplaceAllString(displayName[idx+3:], "")
	return strings.TrimSpace(magicColonNote.ReplaceAllString(tail, ""))
}

// magicColonNote is the bracketed note a few shelves hang after a colon,
// "Friday Night Magic 2005: (tombstone)". A bracket that belongs to the set's
// name, "The List (Unfinity Foil Edition)", has no colon before it.
var magicColonNote = regexp.MustCompile(`:\s*\([^)]*\)$`)

// magicWording joins what a display name brackets besides its code tag,
// which is the variant the storefront is describing.
func magicWording(displayName string) string {
	var words []string
	for _, tag := range bracketed.FindAllString(displayName, -1) {
		tag = strings.TrimSpace(tag)
		if magicCode.MatchString(tag) {
			continue
		}
		words = append(words, strings.Trim(tag, "()"))
	}
	return strings.Join(words, " ")
}

// etchedStamp is the word the catalog tells an etched printing by, and
// etchedSKU the segment this storefront marks one with.
const (
	etchedStamp = "Etched"
	etchedSKU   = "-EF-"
)

// saysEtched reports whether a product is sold as the etched printing. Two of
// the three ways this storefront spells it can be trusted: the segment the sku
// carries and the wording the display brackets beside the number. The shelf
// tail cannot - it reads "Etched Foil" on ordinary foils too, and every
// product that claims it there alone is a second listing of a foil sibling at
// the same price.
func saysEtched(product GNProduct) bool {
	if strings.EqualFold(product.SelectedFinish, "etched") {
		return true
	}
	if strings.Contains(strings.ToLower(magicWording(product.DisplayName)), strings.ToLower(etchedStamp)) {
		return true
	}
	for _, variant := range product.RetailVariants {
		if strings.Contains(variant.SKU, etchedSKU) {
			return true
		}
	}
	return false
}

// magicShelves pairs the shelves this storefront names its own way with the
// catalog's name for the set.
var magicShelves = map[string]string{
	"The List Reprints": "The List",
}

// magicPromoShelves pairs the codes of the shelves that number their promos
// with the storefront's own index with the sets those promos sit in.
var magicPromoShelves = map[string][]string{
	"mfp":   {"MagicFest 2025"},
	"ump":   {"MagicFest 2025", "Wizards Play Network 2026", "Media and Collaboration Promos", "URL/Convention Promos"},
	"media": {"URL/Convention Promos", "Media and Collaboration Promos"},
	"play":  {"Spotlight Series"},
}

// magicRespellings pairs the names this storefront misspells with the
// catalog's own, each read off the TCGplayer id the retail feed carries
// beside the misspelt name, or off the set the buylist files it in. "Dwight,
// Assistant" is the flavor name a Secret Lair gave Baral, cut short.
var magicRespellings = map[string]string{
	"Airbender Lesson":       "Airbending Lesson",
	"Beetle-Headed Mechants": "Beetle-Headed Merchants",
	"Broodguard Ellite":      "Broodguard Elite",
	"Captain Guard":          "Capital Guard",
	"Charging Strikeknight":  "Charging Strifeknight",
	"Cursecloth Wrapping":    "Cursecloth Wrappings",
	"Dollhouse of Horros":    "Dollhouse of Horrors",
	"Dwight, Assistant":      "Baral, Chief of Compliance",
	"Flitting Guerilla":      "Flitting Guerrilla",
	"Frosteliff Siege":       "Frostcliff Siege",
	"Glided Kids":            "Glider Kids",
	"Kefka, Court Magem":     "Kefka, Court Mage",
	"Nine Live":              "Nine Lives",
	"Passeneger Ferry":       "Passenger Ferry",
	"Perigree Beckoner":      "Perigee Beckoner",
	"Thousand-Year Elixer":   "Thousand-Year Elixir",
	"Village Messenger Treatments // Moonrise Intruder": "Village Messenger // Moonrise Intruder",
	"Volatile Arsonist / Dire-Strain Anaarchist":        "Volatile Arsonist // Dire-Strain Anarchist",
}

// sldCommanderDeck returns the Secret Lair Commander deck a shelf is named
// after, or "" for any other shelf.
func sldCommanderDeck(b *matcher.Backend, shelf string) string {
	for _, name := range b.SLDDeckNames {
		if matcher.Contains(shelf, name) {
			return name
		}
	}
	return ""
}

// secretLairDeckPrinting returns the edition and number to look a card of a
// Secret Lair Commander deck's shelf up by. The deck's cards are List
// reprints numbered by the deck's own index, which names no card of the
// List, so the printing the deck lists for the card is what places it. A
// card the deck does not list, or lists as more than one printing, is
// placed by the shelf's name alone.
func secretLairDeckPrinting(b *matcher.Backend, deckName, shelf, cardName string) (string, string) {
	set, found := b.Sets["SLD"]
	if found {
		for _, deck := range set.Decks {
			if matcher.Equals(deck.Name, deckName) {
				cards := slices.Concat(deck.Commander, deck.DisplayCommander, deck.MainBoard)
				return deckPrinting(b, cards, shelf, cardName)
			}
		}
	}
	return "Secret Lair Commander: " + shelf, ""
}

// deckPrinting is the one printing of the card a deck's list holds, or the
// shelf's own name when it holds none or several.
func deckPrinting(b *matcher.Backend, cards []matcher.DeckCard, shelf, cardName string) (string, string) {
	var listed *matcher.CardObject
	for _, entry := range cards {
		co, err := b.GetUUID(entry.UUID)
		if err != nil || !matcher.Equals(co.Name, cardName) {
			continue
		}
		if listed != nil && listed.UUID != co.UUID {
			return "Secret Lair Commander: " + shelf, ""
		}
		listed = co
	}
	if listed == nil {
		return "Secret Lair Commander: " + shelf, ""
	}
	return listed.SetCode, listed.Number
}

// magicTypo names a product whose display number is wrong: its set code in
// lower case, the card's name as the display writes it, and the number it
// prints.
type magicTypo struct{ set, name, number string }

// magicNumberTypos pairs those display numbers with the catalog's own, each
// read off the number and TCGplayer id the retail feed carries for the same
// product. The buylist feed carries neither, so its display number is all it
// has to go on.
var magicNumberTypos = map[magicTypo]string{
	{"dft", "Webstrike Elite", "455"}:    "544",
	{"fin", "Traveling Chocobo", "511B"}: "551b",
	{"fin", "Traveling Chocobo", "511D"}: "551d",
	{"fmb1", "Knowledge Pool", "103"}:    "MBS-111",
	{"inr", "Pack Guardian", "316"}:      "416",
	{"spg", "Galvanic Blast", "010"}:     "100",
	{"wpn", "Orb of Dragonkind", "001"}:  "J1",
	{"wpn", "Orb of Dragonkind", "002"}:  "J2",
	{"wpn", "Orb of Dragonkind", "003"}:  "J3",
}

// magicName reads the card's name off a display name. The name stops at the
// first parenthesis where the name carries a tag. A name with no tag runs
// to the dash that opens the set, less the rarity and number a few shelves
// write after a colon ("Ascendant Packleader: Rare #383 - Innistrad: Crimson
// Vow"); and a flavor name written before the card's own ("Battra, Terror
// of the City - Dirge Bat") keeps only the card's own.
func magicName(b *matcher.Backend, displayName string) string {
	name := displayName
	if idx := strings.Index(name, " ("); idx != -1 {
		name = name[:idx]
		// A card can carry a parenthesis of its own, "Erase (Not the Urza's
		// Legacy One)", so the longest head the catalog knows is the name.
		for at := idx + 2; ; {
			next := strings.Index(displayName[at:], " (")
			if next == -1 {
				break
			}
			at += next
			_, err := b.SearchEquals(displayName[:at])
			if err == nil {
				name = displayName[:at]
			}
			at += 2
		}
	} else if idx := strings.Index(name, " - "); idx != -1 {
		name = name[:idx]
	}
	name = strings.TrimSpace(magicRarityTail.ReplaceAllString(name, ""))
	// The tail is the card's own name when the catalog knows it and not
	// the head: a flavor name comes first, the card's own after the dash.
	if idx := strings.Index(name, " - "); idx != -1 {
		head, tail := name[:idx], name[idx+3:]
		if _, err := b.SearchEquals(tail); err == nil {
			if _, err := b.SearchEquals(head); err != nil {
				return tail
			}
		}
	}
	return name
}

// magicRarityTail matches the rarity and number a few shelves write after
// the name, and the finish and grade the buylist tails a name with.
var magicRarityTail = regexp.MustCompile(`(: (?:Common|Uncommon|Rare|Mythic)(?: Rare)? #.*|\s+(?:Etched )?Foil(?: \(\w+\))?|\s+#\d+\S*)$`)

// magicNumber reads a product's collector number, from the tag in its
// display name where the name carries one and from the product body where
// it does not. A List tag names the reprinted printing by origin and number
// ("C16-177"), or by number alone, with or without the set size ("229/350",
// "285"), with the origin in a tag of its own before it, and without either
// the number alone is left to the name. The retail body carries the number
// as printed; the buylist body only its digits, which is still the number for
// every card the set numbers plainly.
func magicNumber(product GNProduct) (string, error) {
	matches := magicCode.FindAllStringSubmatch(product.DisplayName, -1)
	if matches != nil {
		last := matches[len(matches)-1]
		code, number := last[1], last[2]
		if code == "LIST" {
			number, _, _ = strings.Cut(number, "/")
			origin := magicOrigin.FindStringSubmatch(product.DisplayName)
			if origin != nil && !strings.Contains(number, "-") {
				number = origin[1] + "-" + strings.TrimLeft(number, "0")
			}
		}
		if number != "" {
			return number, nil
		}
	}
	if number := string(product.ProductData.Number); number != "" {
		return number, nil
	}
	if number := string(product.ProductData.NumberDigits); number != "" && number != "0" {
		return number, nil
	}
	return "", errors.New("no collector number in display name or product body")
}

// namesASet reports whether the catalog knows an edition by this name, which
// is how a shelf the storefront invented is told from one it shares.
func namesASet(b *matcher.Backend, edition string) bool {
	_, err := b.GetSetByName(edition)
	return err == nil
}

// The set code the storefront shelves every prerelease stamp under, and the
// word the catalog tells those printings apart by.
const (
	preShelf  = "pre"
	playShelf = "play"
	preStamp  = "Prerelease"
	packStamp = "Promo Pack"
)

// bracketed is anything this storefront brackets into a display name: the
// set-and-number tag, the variant wording, the grade it tails a played copy
// with. What is left is the shelf the product is sold from, which ends in
// the finish.
var bracketed = regexp.MustCompile(`\s*\([^)]*\)`)

// nameSaysFoil reads the finish off the name a product is sold under, which
// the finish field beside it sometimes contradicts. Only this direction is
// worth reading: a name spelling the finish is a statement, while a name
// that does not is silence - Aether Revolt's Ajani, Valiant Protector is
// printed in foil only and its name never says so - so the name adds a
// finish the field left off and never takes one away.
//
// Reading the name whole would not do: a set can be called a foil edition
// without the product being one, so only the finish the shelf ends in
// counts.
func nameSaysFoil(displayName string) bool {
	shelf := strings.TrimSpace(bracketed.ReplaceAllString(displayName, ""))
	return strings.HasSuffix(shelf, "Foil")
}

// lorcanaNumber is the collector group Lorcana display names carry: the
// printing's own number over the set size the matcher does not need, like
// "(17/204)". A promo number's own side of that fraction is not always
// plain digits over digits - "(17/P3)" is the promo set's own number,
// "(54//204)" doubles the slash, and a shelf that publishes no set size at
// all just writes the number alone, "(4)".
var lorcanaNumber = regexp.MustCompile(`\((\d+[a-z]?)(?:/+[A-Z]?\d+)?\)`)

// lorcanaRespellings pairs the names this storefront misspells with the
// catalog's own: a rarity added to a song's name, and a subtitle left off,
// kept to the shelf it was read on where other sets print the bare name.
var lorcanaRespellings = []struct{ name, shelf, to string }{
	{"Into the Unknown - Enchanted", "", "Into the Unknown"},
	{"Mulan", "The First Chapter", "Mulan - Imperial Soldier"},
}

// A Lorcana display name reads
//
//	4*Town - Hottest Band of the Year (17/204) - Attack of the Vine
//
// The dash is part of the card's own name-and-subtitle, so the name only
// stops at the number's parenthesis - the last one before the shelf, since
// a promo card's own wording sometimes brackets a qualifier of its own
// ahead of the number ("Minnie Mouse - Pirate Lookout (Disney Cruise
// Promo) (17/P3) - Disney Lorcana Promo Cards").
func preprocessLorcana(product GNProduct) (*matcher.InputCard, error) {
	head := product.DisplayName
	idx := strings.LastIndex(head, " - ")
	if idx != -1 {
		head = head[:idx]
	}
	locs := lorcanaNumber.FindAllStringSubmatchIndex(head, -1)
	if locs == nil {
		return nil, errors.New("no collector number in display name")
	}
	loc := locs[len(locs)-1]

	finish := product.SelectedFinish
	if strings.EqualFold(finish, "Normal") {
		finish = ""
	}

	name := strings.TrimSpace(product.DisplayName[:loc[0]])
	for _, respelling := range lorcanaRespellings {
		if respelling.name == name && (respelling.shelf == "" || respelling.shelf == product.ProductData.SetName) {
			name = respelling.to
		}
	}

	return &matcher.InputCard{
		Name:      name,
		Edition:   product.ProductData.SetName,
		Variation: product.DisplayName[loc[2]:loc[3]],
		Finish:    finish,
		Foil:      finish != "",
	}, nil
}

// pokemonNumber is a collector number as this game's display names write
// them: "63", "65/130", "SWSH197", the lettered numbers the alphabet sets
// carry - Unown is "Z/115" and the Alph Lithographs are "ONE/123" - and the
// promo numbers written over their series, "381/SM-P".
var pokemonNumber = regexp.MustCompile(`^(?:[A-Z]{0,5}[0-9]+[a-zA-Z]?(?:/[0-9]+)?|[A-Z]+/[0-9]+|[0-9]+/[A-Z]+-P)$`)

// pokemonSizedNumber is the half of those written over the set's size,
// "65/130", "Z/115", "?/115" or "RC10/RC25". Only a card's collector number is written that way,
// so where one appears it is the number and the name ends in front of it -
// which is what tells "Alakazam E4 38/111" from "Dragonite 149/165 (Cosmos
// Holo) 149", the one carrying its own suffix and the other the catalog's
// number repeated.
var pokemonSizedNumber = regexp.MustCompile(`^(?:[A-Z]{0,5}[0-9]+[a-zA-Z]?|[A-Z]+|[!?])/[A-Z]*[0-9]+$`)

// pokemonGenders spells the symbols this storefront prints on a face where
// the catalog writes the letter: Nidoran and its Gym-era owners are filed as
// "Nidoran F" and "Nidoran M".
var pokemonGenders = strings.NewReplacer("♀", "F", "♂", "M")

// pokemonRespellings pairs the names this storefront misspells in a display
// name with the catalog's own, each read off the body the retail feed carries
// beside the name: the body names the card as TCGplayer sells it and carries
// that product id, and both agree with the catalog wherever the display name
// does not. The buylist feed carries neither, and sells the same products
// under the same wrong names, so the correction is a table rather than a
// second reading of the body.
var pokemonRespellings = map[string]string{
	"Arver's Toedscool":                        "Arven's Toedscool",
	"Blend Energy GrassFirePsychicDarkness":    "Blend Energy GFPD",
	"Blend Energy WaterLightningFightingMetal": "Blend Energy WLFM",
	"Defiant Band":                             "Defiance Band",
	"Deoxys (Normal Forme)":                    "Deoxys",
	"Electro Generator":                        "Electric Generator",
	"Feebass":                                  "Feebas",
	"Jamming Net Team Flare Hyper Gear":        "Jamming Net",
	"Marnie's Marpeko":                         "Marnie's Morpeko",
	"ナッシー[Exeggutor]":                          "Exeggutor",
	"Oinkalogne ex":                            "Oinkologne ex",
	"Target Whistle Team Flare Gear":           "Target Whistle",
	"Team Rocket's Nidorand":                   "Team Rocket's Nidoran M",
	"Unit Energy FightingDarknessFairy":        "Unit Energy FDY",
	"Unit Energy GrassFireWater":               "Unit Energy GRW",
	"Unit Energy LightningPsychicMetal":        "Unit Energy LPM",
}

// pokemonEnergyLetters are the letters the catalog writes an energy type as
// in a special energy's name ("Bubbly W Energy"), where this storefront
// spells the type out. The catalog does not letter every one ("Magnetic Metal
// Energy"), so the spelled name is kept wherever the catalog knows it.
var pokemonEnergyLetters = map[string]string{
	"Grass": "G", "Fire": "R", "Water": "W", "Lightning": "L", "Psychic": "P",
	"Fighting": "F", "Darkness": "D", "Metal": "M", "Fairy": "Y", "Dragon": "N",
	"Colorless": "C",
}

var pokemonTypedEnergy = regexp.MustCompile(`^(\w+) (Grass|Fire|Water|Lightning|Psychic|Fighting|Darkness|Metal|Fairy|Dragon|Colorless) Energy$`)

// pokemonGreek spells out the Greek letters this storefront prints in a name
// where the catalog writes Alpha, Beta, Gamma and Delta.
var pokemonGreek = strings.NewReplacer(" α", " Alpha", " β", " Beta", " γ", " Gamma", " δ", " Delta", "δ ", "Delta ")

// pokemonLabels spells the labels this storefront gives a promo the way the
// catalog words them. Only these exact labels are read: a bracket can also
// name the set a card was printed in, "[Rebel Clash]".
var pokemonLabels = strings.NewReplacer(
	"(GameStop Promo)", "(GameStop Exclusive)",
	"(Store Promo)", "(Store Exclusive)",
	"(Cosmo Foil)", "(Cosmos Holo)",
	"(Cosmo Holo)", "(Cosmos Holo)",
	"(Pokemon Day Stamped)", "(Pokemon Day)",
	"(25th Anniversary Stamp)", "(25th Anniversary)",
	"[Winner]", "(Winner)",
)

// pokemonCatalogName is the name the catalog files a card under where the
// storefront's spelling of an energy type or a Greek letter is one it does
// not know.
func pokemonCatalogName(b *matcher.Backend, name string) string {
	_, err := b.SearchEquals(name)
	if err == nil {
		return name
	}
	candidates := []string{pokemonGreek.Replace(name)}
	typed := pokemonTypedEnergy.FindStringSubmatch(name)
	if typed != nil {
		candidates = append(candidates, typed[1]+" "+pokemonEnergyLetters[typed[2]]+" Energy")
	}
	for _, candidate := range candidates {
		_, err := b.SearchEquals(candidate)
		if candidate != name && err == nil {
			return candidate
		}
	}
	return name
}

// pokemonStatedFinish is the finish a display name ends its shelf on, "Neo
// Destiny 1st Edition Holofoil", read by the catalog's own finish names and
// zero when the shelf ends on none. The grade after it is not part of it.
func pokemonStatedFinish(displayName string) matcher.Finish {
	tail := displayName[strings.LastIndex(displayName, " - ")+1:]
	tail = strings.TrimSpace(gradeTag.ReplaceAllString(tail, ""))

	var stated matcher.Finish
	for _, finish := range matcher.Finishes {
		if len(finish.TCGplayer) > len(stated.TCGplayer) && strings.HasSuffix(tail, " "+finish.TCGplayer) {
			stated = finish
		}
	}
	return stated
}

// pokemonStatedPrinting answers the printing of the finish the display name
// states, where the storefront's selectedFinish names another: it says Normal
// on a Reverse Holofoil and Unlimited Holofoil on a 1st Edition Holofoil,
// while the name and the sku both state the finish. A finish the landed card
// was not printed in leaves the landing alone, except for a print run: the
// storefront lists a 1st Edition beside the holofoil of a card that never had
// a plain one, which is no printing at all, and the answer is empty.
func pokemonStatedPrinting(b *matcher.Backend, cardID string, product GNProduct) string {
	stated := pokemonStatedFinish(product.DisplayName)
	if stated.Slug == "" {
		return cardID
	}
	sibling, err := b.MatchIDFinish(cardID, stated.TCGplayer)
	if err == nil {
		return sibling
	}
	if stated.Run != "" && errors.Is(err, matcher.ErrCardWrongFinish) {
		return ""
	}
	return cardID
}

// A Pokemon display name reads
//
//	Abra 65/130 - Base Set 2 Reverse Holofoil
//	Galarian Zapdos - 082/203 (Cosmos Holo) 82 - Miscellaneous Cards  Products
//
// The shelf stands behind the last dash, the number just before it, and the
// name before that. A card whose own name ends in something number-shaped -
// "Alakazam E4", "Darkness Cube 01", "Team Magma Technical Machine 01" - is
// why the number is read from the end rather than the start: reading the
// first took the card's own suffix for its collector number and asked for a
// card nobody prints. The finish travels in its own field for this game,
// where the matcher tells Holofoil from Reverse Holofoil by wording.
func preprocessPokemon(b *matcher.Backend, product GNProduct) (*matcher.InputCard, error) {
	product.DisplayName = pokemonLabels.Replace(product.DisplayName)
	shelf := strings.LastIndex(product.DisplayName, " - ")
	if shelf < 0 {
		return pokemonShelfless(b, product)
	}
	head := product.DisplayName[:shelf]

	// A name of its own carrying a dash puts the number after it, so what
	// stands before the first dash is the name entire and everything after
	// it describes the printing.
	name, rest, dashed := strings.Cut(head, " - ")
	if !dashed {
		name, rest = head, ""
	}

	var number string
	if !dashed {
		// The printed number is the one written over the set's size, and
		// it opens the description wherever it appears: what follows is
		// the catalog's own number and the wording. Where none is written
		// over a size, the last number-shaped field is the collector
		// number and anything earlier belongs to the name.
		fields := strings.Fields(head)
		at := -1
		for i, field := range fields {
			if pokemonSizedNumber.MatchString(field) {
				at = i
				break
			}
		}
		if at < 0 {
			for i := len(fields) - 1; i >= 0; i-- {
				if pokemonNumber.MatchString(fields[i]) {
					at = i
					break
				}
			}
		}
		if at < 0 {
			return pokemonUnnumbered(b, product, head), nil
		}
		number = fields[at]
		name = strings.Join(fields[:at], " ")
		rest = strings.Join(fields[at+1:], " ")
	}
	// The printed number opens the description where the name carried a
	// dash, the catalog's own following it; the printed one is the one that
	// names a printing.
	if dashed {
		for _, field := range strings.Fields(rest) {
			if pokemonNumber.MatchString(field) {
				number = field
				break
			}
		}
		if number == "" {
			return pokemonUnnumbered(b, product, head), nil
		}
	}

	// A qualifier hung behind a bracket in the name belongs with the
	// number: "Professor's Research [Professor Elm] (Poke Ball Pattern)".
	variation := number
	moved := pokemonNameQualifier.FindStringSubmatch(name)
	if moved != nil {
		name, variation = moved[1], number+" "+moved[2]
	}
	card := pokemonCard(b, product, name, variation)

	// A printing's second axis rides just behind the number, bracketed -
	// "Snorlax - 051 (Pokemon Center Exclusive)". The same set prints the
	// same number both ways, so the number alone cannot tell them apart,
	// and this storefront sells the two at $15.31 and $251.59. Wording the
	// catalog does not know costs nothing, since a variation it cannot
	// place falls back on the number it was read from.
	qualifier := pokemonQualifier.FindStringSubmatch(strings.TrimPrefix(strings.TrimSpace(rest), number))
	if qualifier != nil {
		card.Variation = strings.TrimSpace(variation + " " + qualifier[1])
	}

	return card, nil
}

// pokemonNameQualifier splits a name ending in a bracket and then a
// parenthesis into the two.
var pokemonNameQualifier = regexp.MustCompile(`^(.*\])\s+(\([^)]*\))$`)

// pokemonPromoCode is the number a Scarlet & Violet promo's display name
// carries, "(SVP-007)", and pokemonSKUNumber the one its sku does, as in
// "PKM-SS&VPC-002-H-VLDF24NPY4".
var (
	pokemonPromoCode = regexp.MustCompile(`\(SVP-(\d+)\)`)
	pokemonSKUNumber = regexp.MustCompile(`^PKM-[^-]+-([^-]+)-`)
)

// pokemonShelfless reads a display name with no shelf after a dash, which is
// how this storefront writes the Scarlet & Violet promos: "Hawlucha (SVP-007)
// (Scarlet & Violet Base Set) Holofoil". The name stops at its first bracket,
// and the number is the one the name codes where it does and the sku's where
// it does not; the set is the product body's own.
func pokemonShelfless(b *matcher.Backend, product GNProduct) (*matcher.InputCard, error) {
	name, _, _ := strings.Cut(product.DisplayName, " (")
	name, _, _ = strings.Cut(name, " [")

	var number string
	code := pokemonPromoCode.FindStringSubmatch(product.DisplayName)
	if code != nil {
		number = code[1]
	}
	if number == "" && len(product.RetailVariants) > 0 {
		sku := pokemonSKUNumber.FindStringSubmatch(product.RetailVariants[0].SKU)
		if sku != nil {
			number = sku[1]
		}
	}
	if number == "" {
		return nil, errors.New("no collector number in display name")
	}
	return pokemonCard(b, product, name, number), nil
}

// pokemonCard is the matcher's input for a name and a variation read off a
// display name, with the finish and the spelling the catalog uses.
func pokemonCard(b *matcher.Backend, product GNProduct, name, variation string) *matcher.InputCard {
	finish := product.SelectedFinish
	if strings.EqualFold(finish, "Normal") {
		finish = ""
	}

	cardName := strings.TrimSuffix(strings.TrimSpace(name), " -")
	if respelled, found := pokemonRespellings[cardName]; found {
		cardName = respelled
	}
	cardName = pokemonCatalogName(b, pokemonGenders.Replace(cardName))

	return &matcher.InputCard{
		Name:      cardName,
		Edition:   product.ProductData.SetName,
		Variation: variation,
		Finish:    finish,
	}
}

// pokemonBracket is one bracketed group of a display name, and
// pokemonLetteredNumber the number Aquapolis writes in one, "(74a)".
var (
	pokemonBracket        = regexp.MustCompile(`\s*[(\[]([^)\]]*)[)\]]`)
	pokemonLetteredNumber = regexp.MustCompile(`^\d+[a-z]$`)
)

// pokemonUnnumbered reads a head that carries no number field: the basic
// energies, which the catalog files with an empty number, and the cards that
// write their lettered number in a bracket. Every other bracket is wording
// that names the printing, "(2019 Unnumbered)".
func pokemonUnnumbered(b *matcher.Backend, product GNProduct, head string) *matcher.InputCard {
	var number string
	var wording []string
	for _, group := range pokemonBracket.FindAllStringSubmatch(head, -1) {
		if number == "" && pokemonLetteredNumber.MatchString(group[1]) {
			number = group[1]
			continue
		}
		wording = append(wording, group[1])
	}
	name := strings.Join(strings.Fields(pokemonBracket.ReplaceAllString(head, " ")), " ")
	if name == "Pikachu PW" && len(wording) == 1 {
		world, found := pokemonWorldNumbers[wording[0]]
		if found {
			return pokemonCard(b, product, "Pikachu", world)
		}
	}
	variation := strings.TrimSpace(number + " " + strings.Join(wording, " "))
	return pokemonCard(b, product, name, variation)
}

// pokemonWorldNumbers are the numbers the catalog gives the Pikachu World
// Collection promos, which this storefront names by language alone:
// "Pikachu (French) PW". Each is read off the TCGplayer id the retail feed
// carries beside the listing.
var pokemonWorldNumbers = map[string]string{
	"English":    "PW1",
	"Italian":    "PW2",
	"Korean":     "PW3",
	"Spanish":    "PW4",
	"Japanese":   "PW5",
	"German":     "PW6",
	"French":     "PW7",
	"Polish":     "PW8",
	"Portuguese": "PW9",
}

// pokemonBasicEnergy is a basic energy's name, which the catalog files with an
// empty number where a few sets print none.
var pokemonBasicEnergy = regexp.MustCompile(`^(?:Grass|Fire|Water|Lightning|Psychic|Fighting|Darkness|Metal|Fairy) Energy$`)

// retryPokemon asks the catalog again for a card it turned down, in the two
// readings the first one cannot take. A basic energy is asked for without the
// number the storefront gives it. And where the name wrote a dash ahead of the
// number, the size beside it belongs to the set the card was printed in, not
// the shelf it is sold from ("Happiny - 52/124 [Diamond & Pearl] 52 - Burger
// King Promos"), so the number alone is asked for; that answer is taken only
// from the shelf's own set, at that number, since a printing elsewhere would
// be a different card.
func retryPokemon(b *matcher.Backend, product GNProduct, card *matcher.InputCard) (string, bool) {
	if pokemonBasicEnergy.MatchString(card.Name) && card.Variation != "" {
		probe := *card
		probe.Variation = ""
		id, err := b.Match(&probe)
		if err == nil && landedWithoutNumber(b, id) {
			return id, true
		}
	}

	shelf := strings.LastIndex(product.DisplayName, " - ")
	if shelf < 0 || !strings.Contains(product.DisplayName[:shelf], " - ") {
		return "", false
	}
	fields := strings.Fields(card.Variation)
	if len(fields) == 0 || !pokemonSizedNumber.MatchString(fields[0]) {
		return "", false
	}
	numerator, _, _ := strings.Cut(fields[0], "/")
	fields[0] = numerator
	probe := *card
	probe.Variation = strings.Join(fields, " ")
	id, err := b.Match(&probe)
	if err != nil {
		return "", false
	}
	set, err := b.GetSetByName(card.Edition)
	if err != nil {
		return "", false
	}
	landed, err := b.GetUUID(id)
	if err != nil || landed.SetCode != set.Code || b.PlainNumber(landed.Number) != numerator {
		return "", false
	}
	return id, true
}

// landedWithoutNumber reports whether a printing is one the catalog files with
// no collector number, which is what a numberless probe asks for.
func landedWithoutNumber(b *matcher.Backend, id string) bool {
	landed, err := b.GetUUID(id)
	return err == nil && landed.Number == ""
}

// pokemonQualifier is the bracketed wording a display name hangs behind the
// number, before the dash that opens the set.
var pokemonQualifier = regexp.MustCompile(`^\s*((?:\([^)]*\)\s*)+)`)

// onePieceCode is the card code One Piece display names carry, like
// "(OP12-042)" or "(ST25-001)", which names the printing on its own. A few
// listings pad the code with a trailing space before the parenthesis
// ("(P-101 )"), and the Extra Booster dash packs write it bare off the
// name's own dash instead of wrapping it at all ("Koala - OP05-006").
var onePieceCode = regexp.MustCompile(`\(?\b([A-Z]+\d*-\d+[a-z]?)\s*\)?`)

// A One Piece display name reads
//
//	Ama no Murakumo Sword (Jolly Roger Foil) (OP06-056) Premium Booster...
//
// Everything before the code stays in the name: the matcher reads variant
// wording like (Jolly Roger Foil) or (Reprint) from it to pick the printing
// the storefront is describing. The Premium Booster listings write the code
// twice, once more ahead of that wording - "Baby 5 (OP04-032) (Jolly Roger
// Foil) (OP04-032)" - so the name runs to the last code and sheds the code's
// earlier copy, keeping the wording between them.
// squareDecorations rewrites the brackets this storefront hangs a finishing
// place in - "(Online Regional 2024 Vol. 2) [Winner]" - as the parentheses it
// writes every other qualifier in. The matcher splits a parenthetical off the
// name and reads it as the printing being named; a bracket it leaves in the
// name, where the place is lost and the listing lands on the printing that
// was awarded none. That is the whole of the difference between a $558 card
// and a $4 one at the same number.
var squareDecorations = strings.NewReplacer("[", "(", "]", ")")

func preprocessOnePiece(product GNProduct) (*matcher.InputCard, error) {
	if strings.HasPrefix(product.DisplayName, "DON!! Card") {
		return preprocessOnePieceDon(product), nil
	}

	locs := onePieceCode.FindAllStringSubmatchIndex(product.DisplayName, -1)
	if locs == nil {
		return preprocessOnePieceLeader(product)
	}
	loc := locs[len(locs)-1]

	code := product.DisplayName[loc[2]:loc[3]]
	cardName := strings.TrimSuffix(strings.TrimSpace(product.DisplayName[:loc[0]]), " -")
	cardName = strings.ReplaceAll(cardName, "("+code+")", "")
	// Bracketed wording trailing the code joins the name too - the dash
	// packs write theirs there instead of before it, "Koala - OP05-006
	// (Dash Pack)".
	for _, wording := range bracketed.FindAllString(product.DisplayName[loc[1]:], -1) {
		cardName += " " + wording
	}
	cardName = squareDecorations.Replace(cardName)
	for _, relabel := range onePieceRelabels {
		if relabel.code == code && relabel.shelf == product.ProductData.SetName {
			cardName = strings.Replace(cardName, relabel.from, relabel.to, 1)
		}
	}

	return &matcher.InputCard{
		Name:      strings.Join(strings.Fields(cardName), " "),
		Edition:   product.ProductData.SetName,
		Variation: code,
		Foil:      strings.EqualFold(product.SelectedFinish, "foil"),
	}, nil
}

// onePieceRelabels are the listings whose label names another printing than
// the catalog's for it, each read off the retail TCGplayer id of the product.
// On Emperors in the New World the storefront's "(Parallel)" label names the
// manga printing, sold beside separate "(Alternate Art)" products. Shanks of
// Premium Booster Vol. 2 is the SP, listed under the alternate art's label.
var onePieceRelabels = []struct{ code, shelf, from, to string }{
	{"OP09-004", "Emperors in the New World", "(Parallel)", "(Manga)"},
	{"OP09-093", "Emperors in the New World", "(Parallel)", "(Manga)"},
	{"OP09-051", "Emperors in the New World", "(Paralle)", "(Manga)"},
	{"OP09-119", "Emperors in the New World", "(Parallel)", "(Manga)"},
	{"OP06-007", "Premium Booster -The Best- Vol. 2", "(Alternate Art)", "(SP)"},
}

// onePieceCodeless reports a listing whose name carries no card code and is
// not a DON!! card.
func onePieceCodeless(displayName string) bool {
	return !strings.HasPrefix(displayName, "DON!! Card") && !onePieceCode.MatchString(displayName)
}

// landedOnLeader reports whether a printing is one of the promo leaders the
// catalog numbers "LEADER", which is all a listing with no code can name.
func landedOnLeader(b *matcher.Backend, id string) bool {
	landed, err := b.GetUUID(id)
	return err == nil && landed.Number == "LEADER"
}

// preprocessOnePieceLeader answers a promo leader that carries no card code,
// "Monkey.D.Luffy (Sealed Battle 2024 Vol. 2)". The catalog numbers these
// "LEADER" and tells them apart by their label alone, so the head up to its
// last bracket stays whole as the name, the way a DON!! listing's does.
// resolveOnePiece keeps the answer only where it lands on such a leader.
func preprocessOnePieceLeader(product GNProduct) (*matcher.InputCard, error) {
	end := strings.LastIndex(product.DisplayName, ")")
	if end < 0 {
		return nil, errors.New("no card code in display name")
	}
	name := squareDecorations.Replace(product.DisplayName[:end+1])
	return &matcher.InputCard{
		Name:    strings.Join(strings.Fields(name), " "),
		Edition: product.ProductData.SetName,
		Foil:    strings.EqualFold(product.SelectedFinish, "foil"),
	}, nil
}

// onePieceDonWording is the parenthetical wording a DON!! listing's head
// carries between "DON!! Card" and the shelf: the character, the border,
// the pack it came double-packed in.
var onePieceDonWording = regexp.MustCompile(`\(([^)]*)\)`)

// preprocessOnePieceDon answers a DON!! listing the way the catalog files
// it, mirroring coolstuffinc's own onePieceDonName: every DON!! card is
// named "DON!! Card" and told apart by promo type wording bracketed between
// the name and the shelf's double space. Two listings carry that wording as
// part of the card's own name after a "//" instead ("DON!! Card // Green
// Compass"), which the catalog names literally and this keeps whole.
func preprocessOnePieceDon(product GNProduct) *matcher.InputCard {
	head, _, _ := strings.Cut(product.DisplayName, "  ")
	head = squareDecorations.Replace(head)

	if strings.Contains(head, " // ") {
		return &matcher.InputCard{
			Name:    strings.TrimSpace(head),
			Edition: product.ProductData.SetName,
			Foil:    strings.EqualFold(product.SelectedFinish, "foil"),
		}
	}

	var words []string
	for _, m := range onePieceDonWording.FindAllStringSubmatch(head, -1) {
		words = append(words, m[1])
	}

	return &matcher.InputCard{
		Name:      "DON!! Card",
		Edition:   product.ProductData.SetName,
		Variation: strings.Join(words, " "),
		Foil:      strings.EqualFold(product.SelectedFinish, "foil"),
	}
}
