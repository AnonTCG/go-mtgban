package hareruya

import (
	"errors"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/matcher/magic"
)

var reParens = regexp.MustCompile(`\(([^)]+)\)`)
var reBrackets = regexp.MustCompile(`\[([^\]]+)\]`)
var reSquares = regexp.MustCompile(`■([^■]+)■`)
var reJapanese = regexp.MustCompile(`[\p{Hiragana}\p{Katakana}\p{Han}]`)

var reCardName = regexp.MustCompile(`《([^》]+)》`)
var reThick = regexp.MustCompile(`【([^】]+)】`)

// rePLSTCode matches a "The List" reprint's own set-and-number after a slash.
var rePLSTCode = regexp.MustCompile(`^[A-Z0-9]{2,5}-\d+$`)

// reTrailingNumber reads a collector number the English product line states
// alone, right after the bracketed set tag.
var reTrailingNumber = regexp.MustCompile(`\]\((\d+)\)`)

// reLeadingNumber reads a collector number the English product line opens
// with, ahead of any finish group.
var reLeadingNumber = regexp.MustCompile(`^(?:【[^】]*】)*\((\d+)\)`)

// reSDCC matches the convention tag, which the storefront writes against the
// year it names, while the catalog reads the year as a word of its own.
var reSDCC = regexp.MustCompile(`SDCC ?`)

// spaceSDCC puts the year apart from the convention tag.
func spaceSDCC(s string) string {
	return reSDCC.ReplaceAllString(s, "SDCC ")
}

// reYearEdition matches the year a MagicFest promo states as "YYYY年版".
var reYearEdition = regexp.MustCompile(`^(\d{4})年版$`)

// dashSuffix reads the tag the storefront appends to a set code. "-RT" names
// the timeshifted reprints, which are a set of their own and numbered as one.
// Every other tag describes the frame or the booster a card came out of,
// which the collector number in the title already pins down, so they are
// dropped as they always were.
//
// The tempting generalisation is "-P" onto the set's promo set. It was tried
// and measured: those listings carry the base set's collector number, and the
// promo sets number differently, so it lost 51 rows and moved 151 more while
// gaining none.
func dashSuffix(base, suffix string) string {
	if suffix == "RT" && base == "MH1" {
		return "H1R"
	}
	return base
}

// setBooster and secretLairDeck are the two wordings this storefront uses
// for a copy the catalog does not tell apart from the one beside it: which
// booster a card came out of, and whether it was sold in a Secret Lair
// deck. Either is the same printing, so the listing is kept only where the
// one it duplicates is absent.
const setBooster = "ドラフト・セットブースター版"
const secretLairDeck = "SLD構築済み"

// storeStamped is the wording for a store championship prize printed with
// the winning shop's name.
const storeStamped = "店舗名印字入り"

// prerelease is the one promo a modern set numbers among its own cards, so
// the wording naming it does not send the listing to the set's promo line.
// The title is read for it further down either way.
const prerelease = "プレリリース"

// judgeRewards is the wording every judge tag translates into, spelled once
// so the tables and the rule cannot drift apart.
const judgeRewards = "Judge Reward"

// promoLineTreatments names the promo type a treatment group stands for in a
// numbered "-P" retail title: the three the promo line numbers as the set
// itself does, so the number alone cannot tell the two printings apart.
var promoLineTreatments = map[string]string{
	"プロモパック":       magic.PromoTypePromoPack,
	"Bundleプロモ":    magic.PromoTypeBundle,
	"Bundle Promo": magic.PromoTypeBundle,
	"リセールプロモ":      magic.PromoTypeResale,
}

// treatedPromoPrinting finds the printing on the set's promo line that a
// numbered "-P" title names by its treatment, when the line holds exactly
// one of that card at the title's number. The set holds the card at the
// same number, which is where the number alone would send the listing.
func treatedPromoPrinting(b *matcher.Backend, title, cardName, edition, number string, foil bool) (setCode, promoNumber string) {
	if number == "" {
		return "", ""
	}
	var promoType string
	_, after, _ := strings.Cut(title, "》")
	for _, group := range reParens.FindAllStringSubmatch(after, -1) {
		promoType = promoLineTreatments[group[1]]
		if promoType != "" {
			break
		}
	}
	if promoType == "" {
		return "", ""
	}

	finish := "nonfoil"
	if foil {
		finish = "foil"
	}
	front, _, _ := strings.Cut(cardName, "+")
	var found []matcher.Card
	for _, card := range b.MatchInSet(front, "P"+edition) {
		if card.HasPromoType(promoType) && card.HasFinish(finish) &&
			strings.TrimRight(card.PlainNumber, "ps") == number {
			found = append(found, card)
		}
	}
	if len(found) != 1 {
		return "", ""
	}
	return found[0].SetCode, found[0].Number
}

// powerToughness matches the pair a title states in place of a collector
// number for the cards a set prints several times at several sizes.
var powerToughness = regexp.MustCompile(`^\d+/\d+$`)

// splitParens separates the parenthesised groups of a title into the collector
// number, which comes before the card name, and the series, which comes after
// it. A trailing group only counts as a series when it spells a set name
// outright: the matcher's lookup also honors set codes and its own aliases,
// and the storefront's promo qualifiers land on those by accident.
func splitParens(b *matcher.Backend, title string) (number, series, treatment string) {
	end := strings.Index(title, "》")
	for _, loc := range reParens.FindAllStringSubmatchIndex(title, -1) {
		group := title[loc[2]:loc[3]]
		if end >= 0 && loc[0] > end {
			set, err := b.GetSetByName(group)
			if err == nil && matcher.Normalize(set.Name) == matcher.Normalize(group) {
				series = group
				continue
			}
			// The Spotlight Series shelf is a set of its own that
			// GetSetByName does not know by this name.
			if group == "スポットライトシリーズプロモ" {
				series = "Spotlight Series"
				continue
			}
		}
		if number == "" {
			// A slash separates two spellings of one promo code, and the
			// first is the one the catalog files - except where both sides
			// are numbers, which is a power and toughness rather than a
			// code, and the only thing telling one variant of an Unstable
			// card from another, and except where the second side spells a
			// The List reprint's own number, which is the one the catalog
			// files under.
			number = group
			if !powerToughness.MatchString(group) {
				before, after, found := strings.Cut(group, "/")
				if found && rePLSTCode.MatchString(after) {
					code, num, _ := strings.Cut(after, "-")
					number = code + "-" + strings.TrimLeft(num, "0")
				} else {
					// The padding is a collector number's, and a power is
					// not one: stripping a leading zero off "0/4" leaves
					// "/4", which names nothing and drops the listing. Nor
					// is the cost line "0.0.2" that names an Unstable card.
					number = before
					if !strings.Contains(before, ".") {
						number = strings.TrimLeft(before, "0")
					}
				}
			}
			continue
		}

		// Past the number the title states the treatment, and only the
		// wordings the table answers are read as one: the rest of what a
		// group holds there is the card's colour.
		if treatment == "" && end >= 0 && loc[0] > end && group != prerelease {
			if _, found := editionTable[group]; found {
				treatment = group
			}
		}
	}
	return number, series, treatment
}

// announcedTreatment returns the catalog's wording for the treatment a title
// announces in the group the plain finish otherwise occupies, or "" when that
// group names none.
func announcedTreatment(title string) string {
	matches := reThick.FindStringSubmatch(title)
	if len(matches) > 1 {
		return treatmentTable[matches[1]]
	}
	return ""
}

// prereleaseOnPromoLine reports whether a prerelease card is filed on the
// set's promo line rather than among the set's own cards.
//
// Every set since Murders at Karlov Manor numbers its prerelease cards
// among its own, and the number the title carries then names the printing
// in the set itself. A set before it files them on the promo line under
// the base card's number with an "s" behind it, so a title carrying the
// base number names the plain card in the set - and The Lord of the Rings
// does both, holding a prerelease-tagged borderless at 402 beside the promo
// line's date-stamped 402s, which is the one a prerelease listing sells.
// The promo line is asked first: where it holds a prerelease printing of
// the card, that is where the listing belongs, whatever the set holds at
// the number.
func prereleaseOnPromoLine(b *matcher.Backend, cardName, edition, number string) bool {
	if promoLineHolds(b, cardName, edition, "prerelease") {
		return true
	}
	return number == "" || len(b.MatchInSetNumber(cardName, edition, number)) != 1
}

// promoLineHolds reports whether the set's promo line files a printing of
// the card with the promo type.
func promoLineHolds(b *matcher.Backend, cardName, edition, promoType string) bool {
	// The storefront joins the two halves of a split card with a plus.
	front, _, _ := strings.Cut(cardName, "+")
	for _, card := range b.MatchInSet(front, "P"+edition) {
		if card.HasPromoType(promoType) {
			return true
		}
	}
	return false
}

// promoPackFiled reports whether the set, or a set filed under it such as
// its promo line or PPP1 under M20, holds a promo pack printing of the card.
func promoPackFiled(b *matcher.Backend, cardName, setCode string) bool {
	front, _, _ := strings.Cut(cardName, "+")
	for _, set := range b.Sets {
		if set.Code != setCode && set.ParentCode != setCode {
			continue
		}
		if magic.HasPromoPackPrinting(b, front, set.Code) {
			return true
		}
	}
	return false
}

// basicArtSets names the set code of the shelves that number a basic land's
// art variants with a letter.
var basicArtSets = map[string]string{
	"DKM":          "DKM",
	"IE":           "CEI",
	"CE":           "CED",
	"Summer Magic": "SUM",
}

// reArtLetter reads the letter a basic land's title states for its art, in
// parentheses before the set tag, bare before the artist, or after the tag.
var reArtLetter = regexp.MustCompile(`》\(?([A-C])[)（]|\]([A-C])(?:\s|$)`)

// basicArtNumber returns the collector number of the art variant a title
// names by letter: the letter's place among the set's printings of the
// basic land, in number order. It returns "" for a title with no letter.
func basicArtNumber(b *matcher.Backend, setCode, cardName, title string) string {
	m := reArtLetter.FindStringSubmatch(title)
	if m == nil {
		return ""
	}
	letter := m[1] + m[2]

	var numbers []int
	for _, card := range b.MatchInSet(cardName, setCode) {
		n, err := strconv.Atoi(card.Number)
		if err == nil {
			numbers = append(numbers, n)
		}
	}
	slices.Sort(numbers)
	numbers = slices.Compact(numbers)

	i := int(letter[0] - 'A')
	if i >= len(numbers) {
		return ""
	}
	return strconv.Itoa(numbers[i])
}

// Preprocess turns a storefront product into the card description the matcher
// takes, reporting an error for what is not a card.
func Preprocess(b *matcher.Backend, product Product) (*matcher.InputCard, error) {
	// The art cards a set booster carries are filed in art series sets the
	// datastore does not carry, so a row of one has no printing to reach
	if strings.Contains(product.ProductNameEN, "【Art Card】") ||
		strings.Contains(product.ProductName, "【アート・カード】") ||
		strings.Contains(product.ProductNameEN, "Wyvern back") ||
		strings.Contains(product.ProductNameEN, "Orversized") ||
		strings.Contains(product.ProductNameEN, "Oversized") ||
		strings.Contains(product.ProductNameEN, "Error Card") ||
		strings.Contains(product.ProductNameEN, "Error card") ||
		strings.Contains(product.ProductNameEN, "H19") ||
		strings.Contains(product.ProductNameEN, "Test Print") ||
		strings.Contains(product.ProductName, "Ultra Pro Puzzle") ||
		strings.Contains(strings.ToLower(product.CardName), "test print") {
		return nil, matcher.ErrUnsupported
	}

	// A two-sided token sheet's own card_name ("Cat Token/Soldier Token")
	// is not one the ordinary pipeline below was ever built to read - it
	// would search for one card literally named that. Unlike the "/" this
	// same field can carry for an ordinary card's dual Japanese/English
	// spelling (see the buylist's own preprocess, which picks a language
	// rather than splits a pairing), a token listing's two names are both
	// English and both real face names, which is what the "Token" gate
	// below tells apart.
	if strings.Contains(product.CardName, "Token") && strings.Contains(product.CardName, "/") {
		return preprocessTokenPair(b, product)
	}

	cardName := product.CardName
	fixup, found := cardTable[cardName]
	if found {
		cardName = fixup
	}

	foil := product.FoilFlag == "1"
	var edition string
	var variant string
	var promoLine bool

	// Usually there is more information the JPN product line, but sometimes
	// we need to look at the English version too
	match := reBrackets.FindStringSubmatch(product.ProductName)
	if len(match) > 1 {
		edition = match[1]
		// Use the English information if present
		if reJapanese.MatchString(edition) {
			match = reBrackets.FindStringSubmatch(product.ProductNameEN)
			if len(match) > 1 {
				edition = match[1]
			}
		}
		if base, suffix, found := strings.Cut(edition, "-"); found {
			promoLine = suffix == "P"
			edition = dashSuffix(base, suffix)
		}
	}

	// Variant is always found in the English line
	match = reSquares.FindStringSubmatch(product.ProductNameEN)
	if len(match) > 1 {
		variant = match[1]
	}

	// The finish group is read off the Japanese line, which spells every
	// treatment one way where the English line spells several.
	treatment := announcedTreatment(product.ProductName)
	if treatment != "" {
		if variant != "" {
			variant += " "
		}
		variant += treatment
	}

	// The number is only found in the JPN line, which may name the series too
	number, series, _ := splitParens(b, product.ProductName)
	if series != "" {
		// A set that holds the card is where the listing is; the group is
		// then a variation of it, like Amateur Auteur's plane.
		if len(b.MatchInSet(cardName, edition)) == 0 {
			edition = series
		} else {
			variant = strings.TrimSpace(variant + " " + series)
		}
	}
	// Fires on any retail listing splitParens found no number for; the
	// English line states it right after the bracketed set tag, as Guild Kit
	// basics do, or ahead of the name, as the Unsanctioned foil basics do.
	if number == "" {
		m := reLeadingNumber.FindStringSubmatch(product.ProductNameEN)
		if m == nil {
			m = reTrailingNumber.FindStringSubmatch(product.ProductNameEN)
		}
		if m != nil {
			number = m[1]
		}
	}
	// A MagicFest basic states no number of its own, only the year it was
	// handed out, and that is the only thing separating one year's printing
	// from the next.
	if edition == "MagicFest" {
		m := reYearEdition.FindStringSubmatch(number)
		if m != nil {
			number = m[1]
		}
	}
	if number != "" {
		if variant != "" {
			variant += " "
		}
		variant += number
	}

	fixup, found = editionTable[edition]
	if found {
		edition = fixup
	}
	// The promo line a -P edition names holds printings the set itself does
	// not, and the retail line states which by the same wording the buylist
	// does. The group the title puts in the number's place is that wording
	// rather than a number, and only the ones the table answers count: the
	// rest of what a group holds there says nothing about a promo line.
	//
	// The wording is read in the catalog's words only here. Translating every
	// variation the table has an answer for reaches listings whose edition is
	// no promo line at all, and moves them off printings they had right.
	promoWording, isPromoWording := editionTable[number]
	switch {
	case promoLine && series == "" && isPromoWording &&
		!strings.ContainsFunc(number, unicode.IsDigit):
		edition += "-P"
		variant = strings.Replace(variant, number, promoWording, 1)

	// A title that gives the promo line and then says nothing at all - no
	// number of the set's, no treatment - has named the printing as exactly
	// as it is going to. The set it draws from is not where it is, and the
	// table below says which line is.
	case promoLine && series == "" && number == "":
		edition += "-P"
	}

	switch edition {
	case "4ED":
		// Announced in the English name here, where the Japanese line
		// announces it in the group the finish otherwise occupies.
		if strings.Contains(product.ProductNameEN, "【Alternate】") {
			edition = "4EDALT"
		}
	case "Ampersand PROMOS":
		// The retail side never sees the Japanese marker: the English name
		// overrides it before this runs.
		if number != "" {
			edition = "PAFR"
			variant = number + "a"
		}
	case "Judge Foil":
		if magic.IsBasicLand(cardName) && strings.Contains(product.ProductNameEN, "Jacinto") {
			edition = "P23"
			variant = ""
		}
	case "IE", "CE":
		cardName = strings.TrimPrefix(cardName, "【International Edition】")
		cardName = strings.TrimPrefix(cardName, "【Collector's Edition】")

		variants := matcher.SplitVariants(product.ProductNameEN)
		if len(variants) > 1 {
			variant = variants[1]
		}
	case "SLD", "SLD Commander Deck":
		if edition == "SLD Commander Deck" || strings.Contains(product.ProductNameEN, "SLD Commander Deck") ||
			strings.Contains(product.ProductName, "[PWシンボル付き再版]") {
			edition = "PLST"
		}
	default:
		if strings.Contains(edition, "P Stamped_") || strings.Contains(variant, "Promo Stamped") {
			edition = "Promo Pack"
		} else if strings.Contains(product.ProductName, prerelease) && prereleaseOnPromoLine(b, cardName, edition, number) {
			edition += " Prerelease"
		} else if number != "" && promoLineHolds(b, cardName, edition, "prerelease") &&
			len(b.MatchInSetNumber(cardName, edition, number)) == 1 {
			// The set holds this number once, so the frame word beside it
			// only pulls in the prerelease copy filed on the promo line.
			variant = strings.TrimPrefix(variant, "Borderless ")
		}

		variant = strings.Replace(variant, "RetroF ", "Retro Frame ", 1)
		cardName = strings.TrimPrefix(cardName, "【Gold Frame】")
	}

	setCode, found := basicArtSets[edition]
	if found && magic.IsBasicLand(cardName) {
		number := basicArtNumber(b, setCode, cardName, product.ProductName)
		if number != "" {
			variant = number
		}
	}

	if strings.Contains(product.ProductName, "シリアル入り") {
		variant += " Serialized"
	}
	if strings.Contains(product.ProductNameEN, "Normal Border") {
		variant += " non-full art"
	}
	variant = strings.Replace(variant, "FT無し", "No Flavor Text", 1)
	// The tag names the set the card was drawn from, not the convention
	// promo set that holds it.
	if strings.Contains(product.ProductName, "SDCC") {
		edition = ""
	}
	if promoLine && series == "" {
		promoSet, promoNumber := treatedPromoPrinting(b, product.ProductName, cardName, edition, number, foil)
		if promoSet != "" {
			edition, variant = promoSet, promoNumber
		}
	}
	variant = strings.TrimSpace(spaceSDCC(variant))
	override, found := promoMap[edition][cardName][variant]
	if found {
		edition = override.Edition
		variant = override.Variant
	}
	if isDeckEdition(edition) {
		variant = withPlayer(variant, product.ProductName)
		number := deckArtNumber(b, edition, cardName, product.ImageURL)
		if number != "" {
			variant = number
		}
	}

	language := ""
	if product.Language == "1" {
		language = "Japanese"
	}

	if strings.Contains(product.ProductNameEN, "【No Emblem】") {
		variant += " No Symbol"
	}

	return &matcher.InputCard{
		Name:      cardName,
		Variation: variant,
		Edition:   edition,
		Foil:      foil,
		Language:  language,
	}, nil
}

// preprocessTokenPair resolves a two-sided token listing to the combined
// entity matcher/magic derives for it, anchored by the set code and
// collector numbers the storefront's own product_name already carries -
// "(005/006)《Cat+Soldier Token》[C18]" names Cat as C18's own token #5 and
// Soldier as its #6, confirmed against real listings to agree with
// mtgjson's own token numbering exactly. Unlike Cardmarket's own catalog
// ordinal, this is a real anchor; unlike Cool Stuff Inc's own feed, the set
// code sits beside the number in the same field rather than in one of its
// own.
func preprocessTokenPair(b *matcher.Backend, product Product) (*matcher.InputCard, error) {
	faces := strings.SplitN(product.CardName, "/", 2)
	if len(faces) != 2 {
		return nil, matcher.ErrUnsupported
	}
	listingName := faces[0] + " // " + faces[1]
	foil := product.FoilFlag == "1"

	setCode := ""
	if m := reBrackets.FindStringSubmatch(product.ProductName); len(m) > 1 {
		setCode = m[1]
		// As the ordinary path above does: the Japanese line's own
		// bracket group is occasionally non-Latin, and the English one
		// carries the real code where it is.
		if reJapanese.MatchString(setCode) {
			if m := reBrackets.FindStringSubmatch(product.ProductNameEN); len(m) > 1 {
				setCode = m[1]
			}
		}
		if base, suffix, found := strings.Cut(setCode, "-"); found {
			setCode = dashSuffix(base, suffix)
		}
	}
	if setCode != "" {
		if tokenSet := magic.SetTokenSetCode(b, setCode); tokenSet != "" {
			setCode = tokenSet
		}
		number, _, _ := splitParens(b, product.ProductName)
		for _, n := range hareruyaTokenPairNumbers(number) {
			if uuid := magic.MatchNativeTokenPair(b, setCode, n, listingName); uuid != "" {
				if id, err := b.MatchID(uuid, foil); err == nil {
					return &matcher.InputCard{ID: id}, nil
				}
			}
			if tcgID := magic.MatchTokenPairingBySetNumber(b, setCode, n, listingName, foil); tcgID != "" {
				if id, err := b.MatchID(tcgID, foil); err == nil {
					return &matcher.InputCard{ID: id}, nil
				}
			}
		}
	}

	return nil, matcher.ErrUnsupported
}

// hareruyaTokenPairNumbers splits the storefront's own compound number
// ("005/006") into the two faces' own numbers, trying each in turn since
// which half names the listing's first face is not fixed across sheets. A
// bare number is tried as-is.
func hareruyaTokenPairNumbers(number string) []string {
	before, after, found := strings.Cut(number, "/")
	if !found {
		if n := strings.TrimLeft(strings.TrimSpace(number), "0"); n != "" {
			return []string{n}
		}
		return nil
	}
	var out []string
	for _, n := range []string{before, after} {
		if n := strings.TrimLeft(strings.TrimSpace(n), "0"); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// withPlayer appends to the variant the player the deck belonged to, who
// closes the title past the last Japanese field and takes as many words as
// their name needs.
func withPlayer(variant, title string) string {
	fields := strings.Fields(title)
	i := len(fields) - 1
	for i >= 0 && !reJapanese.MatchString(fields[i]) {
		i--
	}
	if i+1 >= len(fields) {
		return variant
	}

	player := strings.Join(fields[i+1:], " ")
	if variant != "" {
		variant += " "
	}
	return variant + player
}

// reDeckArt reads the initials and the number a deck card's image file names,
// as in "pp0378.jpg", "ll0038b.jpg" or "wc00-jf0139a.jpg".
var reDeckArt = regexp.MustCompile(`^(?:wc\d\d-)?([a-z]+)0*(\d+)([a-z]*)$`)

// deckArtNumber returns the collector number of the deck card whose art the
// product image names, which tells apart the copies of a card one player's
// deck holds more than once. The catalog extends the initials the image
// carries ("sh" is "shr"). It returns "" unless one printing fits.
func deckArtNumber(b *matcher.Backend, edition, cardName, imageURL string) string {
	stem, _, _ := strings.Cut(path.Base(imageURL), ".")
	m := reDeckArt.FindStringSubmatch(strings.ToLower(stem))
	if m == nil {
		return ""
	}
	digits, art := m[2], m[3]
	// The 2001 images letter the first art "a" where the catalog leaves
	// the number bare, and the second "a" in its place.
	if edition == "WC01" && len(art) == 1 {
		if art == "a" {
			art = ""
		} else {
			art = string(rune(art[0] - 1))
		}
	}

	setCode := edition
	if edition == "PT96" {
		setCode = "PTC"
	}
	found := ""
	for _, card := range b.MatchInSet(cardName, setCode) {
		initials, tail := splitDeckNumber(card.Number)
		if tail != digits+art || !strings.HasPrefix(initials, m[1]) {
			continue
		}
		if found != "" {
			return ""
		}
		found = card.Number
	}
	return found
}

// splitDeckNumber cuts a deck card's collector number after its initials.
func splitDeckNumber(number string) (string, string) {
	i := strings.IndexFunc(number, unicode.IsDigit)
	if i < 0 {
		return number, ""
	}
	return number[:i], number[i:]
}

// isDeckEdition reports whether an edition is one of the player-deck
// shelves, where the player is what tells a card from its other copies.
func isDeckEdition(edition string) bool {
	return strings.Contains(edition, "WC9") || strings.Contains(edition, "WC0") || edition == "PT96"
}

// process titles like
// 【EN】【Foil】(168)《武器製造/Weapons Manufacturing》[EOE] 赤R
// 【EN】【Foil】(086)■プレリリース■《虚空間渡り/Weftwalking》[EOE] 青R
func preprocess(b *matcher.Backend, title string) (*matcher.InputCard, error) {
	// Test prints and the trading card game this storefront also carries
	// are not Magic printings.
	if strings.Contains(title, "Ultra Pro Puzzle") ||
		strings.Contains(title, "テストプリント") ||
		strings.Contains(title, "■FFTCG■") {
		return nil, matcher.ErrUnsupported
	}

	// A store championship prize is printed with the winning shop's name on
	// it. The catalog holds the prize card once, without the name, so the
	// stamped copy has no printing of its own and answering with the plain
	// one prices that card off the stamp.
	if strings.Contains(title, storeStamped) {
		return nil, matcher.ErrUnsupported
	}

	// A misprint is not a printing of its own, and answering with the card it
	// is a misprint of prices that card off the error.
	if strings.Contains(title, "シンボル無し") {
		return nil, matcher.ErrUnsupported
	}

	var cardName string
	var edition string
	var variant string
	var foil bool

	title = strings.TrimPrefix(title, "【EN】")
	title = strings.Replace(title, "(Bottom)", "", -1)
	title = strings.Replace(title, "(Big Furry Monster)", "", -1)
	title = spaceSDCC(title)
	title = strings.Replace(title, "No Emblem", "No Symbol", -1)

	// /Weapons Manufacturing
	matches := reCardName.FindStringSubmatch(title)
	if len(matches) > 1 {
		cardName = matches[1]
	}

	if strings.Contains(cardName, "/") {
		fields := strings.Split(cardName, "/")
		cardName = ""
		for _, field := range fields {
			if reJapanese.MatchString(field) {
				continue
			}
			cardName = field
		}
	}
	if cardName == "" {
		return nil, errors.New("invalid title format")
	}
	if reJapanese.MatchString(cardName) {
		return nil, matcher.ErrUnsupported
	}

	// [EOE]
	var promoLine bool
	matches = reBrackets.FindStringSubmatch(title)
	if len(matches) > 1 {
		edition = matches[1]
		if base, suffix, found := strings.Cut(edition, "-"); found {
			promoLine = suffix == "P"
			edition = dashSuffix(base, suffix)
		}
	}

	// (168) and (Junior Super Series)
	number, series, promoWording := splitParens(b, title)
	if series != "" {
		edition = series
	}

	// A -P edition is the set's promos rather than the set itself, and where
	// the promo is filed away from that set - the Arena League foils, the
	// game day and release printings - naming the set pins the listing to
	// one that does not hold it, before the treatment beside it is read.
	//
	// The number says which of the two this is. A modern set files its own
	// prerelease promo among its cards and the title gives that card's
	// number, so the set is right and keeping the suffix would lose it. The
	// older promos have no number of the set's to give, and the group the
	// title puts there instead is the treatment - a word, with no digit in
	// it, which is what tells the two apart.
	// A title that states the treatment past the number has said which of
	// the two it is outright, and needs no reading of the number at all.
	if promoLine && series == "" &&
		(promoWording != "" || !strings.ContainsFunc(number, unicode.IsDigit)) {
		edition += "-P"
	}

	// ■プレリリース■
	matches = reSquares.FindStringSubmatch(title)
	if len(matches) > 1 {
		variant = matches[1]
		variant = strings.TrimSpace(variant)
	}

	//【Foil】/【エッチング・Foil】
	matches = reThick.FindStringSubmatch(title)
	if len(matches) > 1 {
		foil = strings.Contains(matches[1], "Foil")
	}
	// The treatment is announced where the plain finish would be, and
	// carries the same set tag and number as the printing it is a
	// treatment of, so nothing else in the title tells them apart.
	treatment := announcedTreatment(title)
	if treatment != "" {
		if variant != "" {
			variant += " "
		}
		variant += treatment
	}

	if number != "" {
		if variant != "" {
			variant += " "
		}
		variant += number
	}

	// The treatment follows the number rather than replacing it: the promo
	// line files more than one printing under the set's own number, and the
	// number is what picks between them.
	if promoWording != "" {
		if variant != "" {
			variant += " "
		}
		variant += editionTable[promoWording]
	}

	fixup, found := editionTable[edition]
	if found {
		edition = fixup
	}
	fixup, found = editionTable[variant]
	if found {
		// A value naming a set is the table saying which set the printing
		// is in, and only the edition can carry that. Left in the variant
		// it still reaches the printing where the edition is already a
		// promo line of its own, which is how the game day textless cards
		// resolve. A -P edition is not that: it names one set's promos and
		// pins the listing among them, and the Champs textless Imperious
		// Perfect is filed in PCMP rather than with Lorwyn's.
		_, setErr := b.GetSet(fixup)
		if promoLine && setErr == nil {
			edition, variant = fixup, ""
		} else {
			variant = fixup
		}
	}

	fixup, found = cardTable[cardName]
	if found {
		cardName = fixup
	}

	// A stamped copy of a card the set files no promo pack printing of has
	// no printing of its own to land on.
	set, setErr := b.GetSet(edition)
	if strings.Contains(variant, "プロモスタンプ付") && setErr == nil &&
		!promoPackFiled(b, cardName, set.Code) {
		return nil, matcher.ErrUnsupported
	}

	if strings.Contains(edition, "Pスタンプ_") ||
		strings.Contains(edition, "P Stamped_") ||
		strings.Contains(variant, "Promo Stamped") ||
		strings.Contains(variant, "プロモスタンプ付") {
		edition = "Promo Pack"
	}

	//variant = strings.Replace(variant, "RetroF ", "Retro Frame ", 1)
	//cardName = strings.TrimPrefix(cardName, "【Gold Frame】")

	override, found := promoMap[edition][cardName][variant]
	if found {
		edition = override.Edition
		variant = override.Variant
	}

	// A misprint the table does not place has no printing in the catalog,
	// and answering with the card it misprints prices that card off the error.
	if edition == "Misprint" {
		return nil, matcher.ErrUnsupported
	}

	if isDeckEdition(edition) {
		variant = withPlayer(variant, title)
	} else if strings.Contains(variant, "P30H") {
		edition = variant
	} else if strings.Contains(title, "プレリリース") {
		variant += " Prerelease"
		if prereleaseOnPromoLine(b, cardName, edition, number) {
			edition += " Prerelease"
		}
	} else if strings.Contains(title, "シリアル入り") {
		variant += " Serialized"
	} else if edition == "4ED" && variant == "Alternate" {
		edition = "4EDALT"
	} else if strings.Contains(title, "アンパサンド") && number != "" {
		// The ampersand card reprints another at its own number, and the
		// promo set is where the two are kept apart: every one of them is
		// the base number with an "a" behind it, and they are the only
		// numbers in that set spelled that way. Without a number there is
		// nothing to suffix, and the promo set numbers the same card three
		// ways, so a listing that gives none is left where it was.
		edition = "PAFR"
		variant = number + "a"
	} else if strings.Contains(variant, doubleRainbow) && magic.HasSerializedPrinting(b, cardName, edition) {
		// The marker travels with whatever else the title says about the
		// printing - its number, and the frame it is printed in - so the
		// variant is only ever equal to it on a title that says nothing
		// else, which the serialized listings do not.
		variant += " Serialized"
	}

	if strings.Contains(title, "(FNM)") ||
		strings.Contains(title, "(CardZ") ||
		strings.Contains(title, "SDCC") {
		// Wipes "[6ED-P]" tags which confuse the matcher
		edition = ""
	}

	// The same for a judge reward, which is a set of its own that the
	// storefront files under the set the card was first printed in: the
	// "-P" tag saying so is dropped with every other frame tag, so the base
	// set stands and deletes the promo printing before the wording naming
	// it is read. Gaea's Cradle answered with the $751 Urza's Saga land
	// where the listing is the $2660 judge foil, and it reaches the judge
	// set on its own once the edition stops contradicting it.
	//
	// The tag alone is not enough to go on - mapping "-P" onto a set's own
	// promo set was measured and lost 51 rows - and this asks for the
	// wording instead, which names the set rather than merely denying the
	// one the tag came from.
	if strings.Contains(variant, judgeRewards) {
		edition = ""
	}

	return &matcher.InputCard{
		Name:      cardName,
		Variation: strings.TrimSpace(variant),
		Edition:   edition,
		Foil:      foil,
	}, nil
}

var cardTable = map[string]string{
	// The Secret Lair flavor name the title puts ahead of the card's own
	"Chancla relámpagos":                 "Lightning Greaves",
	"Chicken ? la King":                  "Chicken à la King",
	"Adorable | KittenAdorable | Kitten": "Adorable Kitten",
	"Tyrannosaurs Rex":                   "Tyrannosaurus Rex",
}

// doubleRainbow is the treatment every serialized printing is sold in, and
// some printings that are not: a set that holds no serialized printing of
// the card sells it unnumbered, as Secret Lair's rainbow foils are.
const doubleRainbow = "Double Rainbow Foil"

// treatmentTable names the treatments the storefront announces in the group
// the plain finish otherwise occupies, in the words the catalog labels them
// with. Only the marker says which printing a listing is: the treated
// printing shares its set tag and collector number with the plain one, so a
// marker read as a bare "Foil" prices the treatment as the plain card - and
// these are the printings a shop pays the most for.
//
// Every entry was read off the storefront's own buylist: each marker below
// appears there, and each names a promo type the catalog carries.
var treatmentTable = map[string]string{
	"Pool Party・Foil": "Pool Party",
	"S&C・Foil":        "Step-and-Compleat Foil",
	"エッチング・Foil":      "Etched Foil",
	"オイルスリック・Foil":    "Oil Slick",
	"ギャラクシー・Foil":     "Galaxy Foil",
	"コンフェッティ・Foil":    "Confetti Foil",
	"サージ・Foil":        "Surge Foil",
	"ダブルレインボウ・Foil":   doubleRainbow,
	"テクスチャー・Foil":     "Textured Foil",
	"ドラゴンスケイル・Foil":   "Dragonscale Foil",
	"ネオンインク・Foil":     "Neon Ink",
	"ハロー・Foil":        "Halo Foil",
	"ファーストプレイス・Foil":  "First Place Foil",
	"リップル・Foil":       "Ripple Foil",
	"レイズド・Foil":       "Raised Foil",
	"不可視インク":          "Invisible Ink",
	"銀幕・Foil":         "Silver Foil",

	// Not a finish: the alternate printing is announced in the same group.
	"アルターネイト版": "Alternate",
}

var editionTable = map[string]string{
	"2007年版ジャッジ褒賞":        "2007 Judge Rewards",
	"2010年版ジャッジ褒賞":        "2010 Judge Rewards",
	"2013年版ジャッジ褒賞":        "2013 Judge Rewards",
	"2015年版ジャッジ褒賞":        "2015 Judge Rewards",
	"2018年版ジャッジ褒賞":        "2018 Judge Rewards",
	"2020年版":              "2020 Edition",
	"30周年記念":              "30th Anniversary",
	"APACランド":             "Asia Pacific Land Program",
	"BOOKプロモ":             "Book Promo",
	"BOXプロモ":              "Buy a Box",
	"CardZプロモ":            "CardZ Promo",
	"CSP構築済み":             "CST",
	"DCIマーク":              "DCI Promo",
	"Etched Foil 30周年プロモ": "P30M etched frame",
	"GPプロモ":               "Grand Prix Promos",
	"Guru Lnad":           "Guru Land",
	"Marvel Legend Promo": "LMAR",
	"MCQプロモ":              "MCQ Promo",
	"Nationalプロモ":         "National Promos",
	"PWシンボル付き再版":          "Mystery Booster/The List",
	"RPTQプロモ":             "RPTQ Promos",
	"URL入りイベントプロモ":        "PURL",
	"WMCQプロモ":             "WMCQ Promo",
	"その他プロモ":              "Other Promos",
	"アリーナ":                "Arena",
	"アルターネイト版":            "Alternate",
	"アンパサンド":              "Ampersand",
	"アンパサンド・カード":          "Ampersand Promo",
	"イラスト違い":              "S-Chinese alt art",
	"ウギンの運命":              "Ugin's Fate",
	"エッチング・Foil":          "Etched Foil",
	"エラーカード":              "Misprint",
	"エンブレムあり":             "With Symbol",
	"エントリーセット":            "Intro Pack Promo",
	"エンブレムなし":             "No Symbol",
	"ゲートウェイ":              "Gateway",
	"ギフトボックス":             "Gift Box",
	"ストアチャンピオンシップ":        "Store Championship",
	"ゲームデー":               "Game Day",
	"コマンドフェスト":            "Command Fest",
	"サージ・Foil":            "Surge Foil",
	"ショーダウン":              "Showdown",
	"ジャッジ褒賞":              "Judge Rewards",
	"基本セット系プロモ":           "Promo",
	"発売記念":                "Release",
	"ダブルレインボウ・Foil":       doubleRainbow,
	"テキストボックスレス ゲームデー":    "PCMP",
	"テキストレス Magic Fest":   "Textless Magic Fest",
	"テキストレス 褒賞プログラム":      "Textless Player Rewards",
	"テキストレス":              "Textless",
	"テストプリント":             "Test Print",
	"ヒストリープロモ":            " 30th Anniversary History",
	"ファイレクシア語 その他プロモ":     "Phyrexian Other Promos",
	"ファイレクシア語 ジャッジ褒賞":     "Phyrexian Judge Reward",
	"フルアート 1":             "Full Art 1",
	"フルアート 2":             "Full Art 2",
	"フルアート コマンドフェスト":      "Fullart CommandFest",
	"プレリリース":              "Prerelease",
	"プロツアープロモ":            "Pro Tour Promos",
	"ボーダーレス Premier Play": "Borderless Premier Play",
	"ボーダーレス その他イベント記念":    "Borderless Other Event Commemoration",
	"ボーダーレス その他イベント記念系":   "Borderless Other Event",
	"ボーダーレス マーベル・レジェンドプロモ": "Borderless Marvel Legends Promo",
	"ボーダーレス 褒賞プロモ":         "Borderless Player Rewards",
	"ボーダーレス":               "Borderless",
	"ボーダーレスショーダウン":         "Borderless Showdown",
	"マジックリーグ":              "Year of the Tiger 2022",
	"メディア系プロモ":             "Media Promo",
	"リセールプロモ":              "Resale Promo",
	"リリースプロモ":              "Release Promo",
	"旧正月プロモ":               "Lunar New Year",
	"午年プロモ":                "Year of the Horse 2026",
	"卯年プロモ":                "Year of the Rabbit 2023",
	"大判カード":                "Oversize",
	"対戦キット":                "Clash Pack",
	"巳年プロモ":                "Year of the Snake 2025",
	"拡張アート MagicConプロモ":    "Extended Art MagicCon Promo",
	"拡張アート その他プロモ":         "Extended Art Other Promos",
	"拡張アート":                "Extended Art",
	"新枠 2008年版ジャッジ褒賞":      "Mordern Frame 2008 Judge Rewards",
	"旧枠 2000年版ジャッジ褒賞":      "Retro Frame 2000 Judge Rewards",
	"旧枠 ジャッジ褒賞":            "Retro Frame Judge Rewards",
	"旧枠 その他プロモ":            "Retro Frame Other Promos",
	"旧枠 ヒストリープロモ":          "Retro Frame 30th Anniversary History",
	"旧枠 褒賞プログラム":           "Old Frame Rewards Program",
	"旧枠":                   "Retro Frame",
	"絵違いVer.":              "Alternate Art",
	"褒賞プログラム":              "Rewards Program",
	"辰年プロモ":                "Year of the Dragon 2024",

	"S&C・Foil":               "Step-and-Compleat Foil",
	"Secret Lair Showdown":   "SLP",
	"Retro Frame Promos":     "PLG21",
	"30th Promo":             "P30A",
	"POS Reward Promo":       "PW24",
	"CMA":                    "CM1",
	"WMC":                    "World Magic Cup Qualifiers",
	"JapanStandardCup Promo": "Japan Standard Cup",

	"DvD": "Duel Decks: Divine vs. Demonic",
	"EVG": "Duel Decks: Elves vs. Goblins",
	"EvG": "Duel Decks: Elves vs. Goblins",
	"GvL": "Duel Decks: Garruk vs. Liliana",
	"JvC": "Duel Decks: Jace vs. Chandra",

	"FNM": "Friday Night Magic",

	"Old Arena": "PARL",
	"ONS Arena": "PAL03",
	"MRD Arena": "PAL04",
	"CHK Arena": "PAL05",
	"RAV Arena": "PAL06",
}

var promoMap = map[string]map[string]map[string]struct {
	Edition string
	Variant string
}{
	// The Standard Showdown packs of 2016 are a set of their own holding
	// nothing but that block's five battle lands, and the storefront files
	// them under the set they were drawn from, saying only that they are its
	// promos. Nothing else in the title tells them from that set's own card.
	"BFZ-P": {
		"Canopy Vista":     {"": {Edition: "PSS1", Variant: "234"}},
		"Cinder Glade":     {"": {Edition: "PSS1", Variant: "235"}},
		"Prairie Stream":   {"": {Edition: "PSS1", Variant: "241"}},
		"Smoldering Marsh": {"": {Edition: "PSS1", Variant: "247"}},
		"Sunken Hollow":    {"": {Edition: "PSS1", Variant: "249"}},
	},
	"Other event promo": {
		"Jedit Ojanen": {
			"Textless マジックリーグ": {
				Edition: "PL22",
				Variant: "2",
			},
		},
		"Swords to Plowshares": {
			"Borderless その他イベント記念系": {
				Edition: "PF25",
				Variant: "12",
			},
		},
	},
	"Other Event Promo": {
		"Arcbound Ravager": {"卯年プロモ": {Edition: "PL23", Variant: "6"}},
		"Ephemerate": {
			"夏休み": {
				Edition: "PSVC",
				Variant: "1",
			},
		},
		"Swiftfoot Boots": {
			"卯年プロモ": {
				Edition: "PL23",
				Variant: "4",
			},
		},
	},
	"Other Event anniversary": {
		"Sol Ring": {
			"旧枠プロモ": {
				Edition: "PFDN",
				Variant: "1",
			},
		},
		"Vengevine": {
			"WMCQプロモ": {
				Edition: "WMC",
				Variant: "2013",
			},
		},
		"Sakura-Tribe Elder": {
			"E06": {
				Edition: "PJSE",
				Variant: "1E06",
			},
		},
		"Soltari Priest": {
			"E07": {
				Edition: "PJSE",
				Variant: "1E07",
			},
		},
		"Glorious Anthem": {
			"U08": {
				Edition: "PJAS",
				Variant: "1U08",
			},
		},
		"Steward of Valeron": {
			"URL入りイベントプロモ": {
				Edition: "PURL",
				Variant: "1",
			},
		},
		"Cryptic Command": {
			"MCQプロモ": {
				Edition: "PPRO",
				Variant: "2020-1",
			},
		},
		"Reya Dawnbringer": {
			"": {
				Edition: "P10E",
				Variant: "35",
			},
		},
		"Earl of Squirrel": {
			"": {
				Edition: "PUST",
				Variant: "108",
			},
		},
		"Fyndhorn Elves": {
			"Textless コマンドフェスト": {
				Edition: "PF26",
				Variant: "2",
			},
		},
		"Gandalf, Friend of the Shire": {
			"コマンドフェスト": {
				Edition: "PF23",
				Variant: "1",
			},
		},
	},
	"Other Promos": {
		"Plains":   {"Illus.Mark Poole": {Edition: "G17", Variant: "1"}},
		"Island":   {"Illus.Mark Poole": {Edition: "G17", Variant: "2"}},
		"Mountain": {"Illus.Mark Poole": {Edition: "G17", Variant: "4"}},
		"Serra the Benevolent": {
			"Retro Frame その他プロモ": {
				Edition: "PF25",
				Variant: "1",
			},
		},
		"Ugin, the Spirit Dragon": {
			"Retro Frame その他プロモ": {
				Edition: "PF25",
				Variant: "6",
			},
		},
		"Ponder": {
			"その他プロモ": {
				Edition: "PF25",
				Variant: "2",
			},
		},
		"Avacyn's Pilgrim": {
			"テキストレス Festival": {
				Edition: "PF25",
				Variant: "1F",
			},
		},
		"Sliver Hive": {
			"Retro Frame その他プロモ": {
				Edition: "PF25",
				Variant: "7",
			},
		},
		"Sakura-Tribe Elder": {
			"": {
				Edition: "PLG24",
				Variant: "1",
			},
			"Textless": {
				Edition: "PLG24",
				Variant: "1",
			},
		},
		"Mutavault": {
			"PCMP": {
				Edition: "PCMP",
				Variant: "12",
			},
		},
		"Counterspell": {
			"テキストレス MagicConプロモ": {
				Edition: "PF24",
				Variant: "1",
			},
		},
		"Electrolyze": {
			"PCMP": {
				Edition: "PCMP",
				Variant: "1",
			},
		},
		"Mind Stone": {
			"2021年版プロモ": {
				Edition: "PW21",
				Variant: "5",
			},
		},
	},
	"PB・Draft Promos": {
		"Arcane Signet": {
			"Retro Frame PBドラフトプロモ": {
				Edition: "P30M",
				Variant: "1P",
			},
		},
		"Commander's Sphere": {
			"PBドラフトプロモ": {
				Edition: "PW24",
				Variant: "8",
			},
		},
		"Chaos Warp": {
			"PBドラフトプロモ": {
				Edition: "PW24",
				Variant: "7",
			},
		},
	},
	// The 2025 and 2026 promo shelves: each card's Wizards Play Network
	// or Standard Showdown printing, filed by year, the Spotlight Series
	// set, and the Final Fantasy Standard Showdown set.
	"Showdown Promo": {
		"Go for the Throat": {"Borderless スタンダード・ショーダウン": {Edition: "PCBB", Variant: "3"}},
		"Plains":            {"Full-Art Illus.Lucas Graciano": {Edition: "PSS4", Variant: "1"}},
		"Island":            {"Full-Art Illus.Piotr Dura": {Edition: "PSS4", Variant: "2"}},
		"Swamp":             {"Full-Art Illus.Alexander Forssberg": {Edition: "PSS4", Variant: "3"}},
		"Mountain":          {"Full-Art 2024年版 Illus.Alayna Danner": {Edition: "PSS4", Variant: "4"}, "2018年版 Illus.Alayna Danner": {Edition: "PSS3", Variant: "4"}},
		"Wood Elves": {
			"Extended Art スタンダード・ショーダウン": {
				Edition: "PW26",
				Variant: "16",
			},
		},
		"Squall, SeeD Mercenary": {
			"Borderless スタンダード・ショーダウン": {
				Edition: "PSS5",
				Variant: "2",
			},
		},
		"Ultima": {
			"Borderless スタンダード・ショーダウン": {
				Edition: "PSS5",
				Variant: "1",
			},
		},
		"Carnage, Crimson Chaos": {
			"スタンダード・ショーダウン": {
				Edition: "PW25",
				Variant: "13",
			},
		},
		"Unlucky Cabbage Merchant": {
			"スタンダード・ショーダウン": {
				Edition: "PW25",
				Variant: "15",
			},
		},
		"Lightning Bolt": {
			"Borderless スタンダード・ショーダウン": {
				Edition: "PW26",
				Variant: "5",
			},
		},
		"Into the Flood Maw": {
			"Retro Frame スタンダード・ショーダウン": {
				Edition: "PW26",
				Variant: "8",
			},
		},
		"Dark Deed": {
			"スタンダード・ショーダウン": {
				Edition: "PW26",
				Variant: "12",
			},
		},
		"Nowhere to Run": {
			"Retro Frame スタンダード・ショーダウン": {
				Edition: "PW26",
				Variant: "1",
			},
		},
	},
	"Commander Event Promo": {
		"Gran-Gran":                {"コマンダーイベントプロモ": {Edition: "PW25", Variant: "14"}},
		"Spider-Ham, Peter Porker": {"コマンダーイベントプロモ": {Edition: "PW25", Variant: "10"}},
		"Echo, Perceptive Prodigy": {
			"コマンダーイベントプロモ": {
				Edition: "PW26",
				Variant: "11",
			},
		},
		"Mister Fantastic, Reed Richards": {
			"コマンダーイベントプロモ": {
				Edition: "PW26",
				Variant: "10",
			},
		},
		"Farhaven Elf": {
			"Retro Frame コマンダーイベントプロモ": {
				Edition: "PW26",
				Variant: "2",
			},
		},
		"Access Tunnel": {
			"Retro Frame コマンダーイベントプロモ": {
				Edition: "PW26",
				Variant: "9",
			},
		},
		"Command Tower": {
			"Full-Art コマンダーイベントプロモ": {
				Edition: "PW25",
				Variant: "17",
			},
		},
	},
	"Magic Presents Promo": {
		"Ultimate Green Goblin": {"マジック・プレゼンツプロモ": {Edition: "PW25", Variant: "12"}},
		"Hellcat, Undying Vigilante": {
			"マジック・プレゼンツプロモ": {
				Edition: "PW26",
				Variant: "13",
			},
		},
		"An Unexpected Party": {
			"Extended Art マジック・プレゼンツプロモ": {
				Edition: "PW26",
				Variant: "14",
			},
		},
	},
	"LRW-P": {
		"Imperious Perfect": {
			"Game Day": {
				Edition: "PCMP",
				Variant: "9",
			},
		},
	},
	"UNF-P Prerelease": {
		"Water Gun Balloon Game": {
			"Prerelease": {
				Edition: "UNF",
				Variant: "538",
			},
		},
	},
	"M14-P": {
		"Scavenging Ooze": {
			"Promo": {
				Edition: "PDP14",
				Variant: "3",
			},
		},
	},
	// The Pool Party drop reprints cards under their earlier sets' numbers
	// in the title, and the datastore files it under its own; the dazzle
	// foil is the marked product, the plain foil and nonfoil the other.
	"SLD": {
		"Deadly Dispute": {
			"2XM-080":            {Edition: "SLD", Variant: "IFIYW-1"},
			"Pool Party 2XM-080": {Edition: "SLD", Variant: "IFIYW-6"},
		},
		"Thrill of Possibility": {
			"J22-615":            {Edition: "SLD", Variant: "IFIYW-3"},
			"Pool Party J22-615": {Edition: "SLD", Variant: "IFIYW-8"},
		},
		"Lightning Greaves": {
			"NCC-382":            {Edition: "SLD", Variant: "IFIYW-4"},
			"Pool Party NCC-382": {Edition: "SLD", Variant: "IFIYW-9"},
		},
		"Sol Ring": {
			"SCD-288":            {Edition: "SLD", Variant: "IFIYW-5"},
			"Pool Party SCD-288": {Edition: "SLD", Variant: "IFIYW-10"},
		},
		"Lightning Bolt": {
			"GN2-042":            {Edition: "SLD", Variant: "IFIYW-2"},
			"Pool Party GN2-042": {Edition: "SLD", Variant: "IFIYW-7"},
		},
	},
	"IKO": {
		// The first Godzilla print, before the name was changed
		"Void Beckoner": {
			"First edition 373a": {
				Edition: "IKO",
				Variant: "373",
			},
		},
	},
	"Standard Showdown Promo": {
		"Monstrous Rage": {
			"Retro Frame Standard Showdown": {
				Edition: "PW25",
				Variant: "9",
			},
		},
	},
	"Showdown": {
		"Go for the Throat": {
			"Borderless ショーダウン": {
				Edition: "PCBB",
				Variant: "3",
			},
		},
	},
	// The buylist names the promo shelf by its shooting-star mark, and
	// the wording beside the card by the program that handed it out.
	"流星マーク": {
		"Cloud, Midgar Mercenary": {"統率者デッキプロモ": {Edition: "PMEI", Variant: "2025-21"}},
		"Unstoppable Slasher": {
			"ジャパンスタンダードカッププロモ": {Edition: "PJSC", Variant: "2026-2"},
		},
		"Sheltered by Ghosts": {
			"ジャパンスタンダードカッププロモ": {Edition: "PJSC", Variant: "2026-1"},
		},
		"Swords to Plowshares": {
			"Borderless Other Event": {Edition: "PF25", Variant: "12"},
			"ボーダーレス メディア系プロモ":        {Edition: "PMEI", Variant: "2026-4"},
		},
		"Katara, the Fearless": {
			"Extended Art MagicCon Promo": {Edition: "PURL", Variant: "2025-3"},
		},
		"Pyroblast": {
			"": {Edition: "PW23", Variant: "8"},
		},
		"Reliquary Tower": {
			"Fullart CommandFest": {Edition: "PF23", Variant: "3"},
		},
		"Zombie Master": {
			"Borderless Player Rewards": {Edition: "PW24", Variant: "3"},
		},
		"Lord of Atlantis": {
			"Borderless Player Rewards": {Edition: "PW24", Variant: "2"},
		},
		"Serra Angel": {
			"Borderless Player Rewards": {Edition: "PW24", Variant: "1"},
		},
		"Ultima": {
			"ボーダーレス スタンダード・ショーダウン": {Edition: "PSS5", Variant: "1"},
		},
		"Squall, SeeD Mercenary": {
			"ボーダーレス スタンダード・ショーダウン": {Edition: "PSS5", Variant: "2"},
		},
		"Ossification": {
			"ボーダーレス スタンダード・ショーダウン": {Edition: "PCBB", Variant: "1"},
		},
		"Ultimate Green Goblin": {
			"マジック・プレゼンツプロモ": {Edition: "PW25", Variant: "12"},
		},
		"Gran-Gran": {
			"コマンダーイベントプロモ": {Edition: "PW25", Variant: "14"},
		},
		"Courier of Comestibles": {
			"ボーダーレス コマンダーイベントプロモ": {Edition: "PW26", Variant: "6"},
		},
		"Spicy Oatmeal Pizza": {
			"ボーダーレス マジック・プレゼンツプロモ": {Edition: "PW26", Variant: "7"},
		},
		"An Unexpected Party": {
			"拡張アート マジック・プレゼンツプロモ": {Edition: "PW26", Variant: "14"},
		},
		"Bilbo, Luckwearer": {
			"拡張アート コマンダーイベントプロモ": {Edition: "PW26", Variant: "15"},
		},
		"Zack Fair": {
			"ボーダーレス その他プロモ": {Edition: "PMEI", Variant: "2026-3"},
		},
		// The buylist reads a double-faced card's own name off the first
		// face only.
		"Peter Parker": {
			"Extended Art Other Promos": {Edition: "PMEI", Variant: "2025-22"},
		},
		"Into the Flood Maw": {
			"旧枠 スタンダード・ショーダウン": {Edition: "PW26", Variant: "8"},
		},
		"Dark Deed": {
			"スタンダード・ショーダウン": {Edition: "PW26", Variant: "12"},
		},
		"Wood Elves": {
			"拡張アート スタンダード・ショーダウン": {Edition: "PW26", Variant: "16"},
		},
		"Echo, Perceptive Prodigy": {
			"コマンダーイベントプロモ": {Edition: "PW26", Variant: "11"},
		},
		"Command Tower": {
			"フルアート コマンダーイベントプロモ": {Edition: "PW25", Variant: "17"},
			"ボーダーレス その他プロモ":      {Edition: "PMEI", Variant: "2026-13"},
			"旧枠 メディア系プロモ":        {Edition: "PMEI", Variant: "2026-1"},
		},
		"Yuna, Grand Summoner": {
			"ボーダーレス コマンダーボックスリーグプロモ": {Edition: "PW25", Variant: "16"},
		},
		"Sliver Hive": {
			"Retro Frame Other Promos": {Edition: "PF25", Variant: "7"},
		},
		"Ugin, the Spirit Dragon": {
			"Retro Frame Other Promos": {Edition: "PF25", Variant: "6"},
		},
		"Ponder": {
			"Other Promos": {Edition: "PF25", Variant: "2"},
		},
		"Ephemerate": {
			"夏休み": {Edition: "PSVC", Variant: "1"},
		},
		"Arcane Signet": {
			"30th Anniversary":   {Edition: "P30M", Variant: "1F"},
			"Etched Foil 30周年記念": {Edition: "P30M", Variant: "1F★"},
		},
		"Bolas's Citadel": {
			"旧枠プロモ": {Edition: "PLG21", Variant: "3"},
		},
		"Loki, God of Mischief": {
			"その他イベント記念系": {Edition: "PMEI", Variant: "2026-14"},
		},
		"Counterspell": {
			"旧枠 MagicConプロモ": {Edition: "PF26", Variant: "5"},
			"Full Art 2":     {Edition: "PURL", Variant: "2"},
		},
		"Lightning Bolt": {
			"MagicConプロモ": {Edition: "PF25", Variant: "13"},
		},
		"Lotus Petal": {
			"P30M etched frame": {Edition: "P30M", Variant: "2"},
		},
		"Tifa Lockhart": {
			"Borderless Premier Play": {Edition: "PF25", Variant: "9"},
		},
		"Gandalf, Friend of the Shire": {
			"Play Promo": {Edition: "PF23", Variant: "1"},
		},
		"Behold the Sinister Six!": {
			"Extended Art Other Promos": {Edition: "PURL", Variant: "2025-4"},
		},
	},
	"DCI Promo": {
		"Goblin Warchief": {
			"2006年度版FNM": {Edition: "F06", Variant: "5"},
		},
	},
	"新枠プロモ": {
		"Kor Skyfisher": {
			"PURL": {Edition: "PURL", Variant: "23"},
		},
	},
	"NEM": {
		"Rhox": {
			"S00プロモ": {Edition: "S00", Variant: "43"},
		},
	},
	// The oldest prerelease promos of a set are filed as its release
	// promos, the only promo printing the catalog holds of each.
	"LRW-P Prerelease": {
		"Shriekmaw": {"Prerelease": {Edition: "PLRW", Variant: "139★"}},
	},
	"PLC-P Prerelease": {
		"Hedge Troll": {"Prerelease": {Edition: "PPLC", Variant: "151★"}},
	},
	"FUT-P Prerelease": {
		"Storm Entity": {"Prerelease": {Edition: "PFUT", Variant: "122★"}},
	},
	"BOK-P Prerelease": {
		"Budoka Pupil": {"Prerelease": {Edition: "PBOK", Variant: "122★"}},
	},
	"9ED-P Prerelease": {
		"Force of Nature": {"Prerelease": {Edition: "P9ED", Variant: "242★"}},
	},
	"Commander Play": {
		"Palladium Myr": {
			"Retro Frame Commander Play": {
				Edition: "PW25",
				Variant: "6",
			},
		},
	},
	"Premier Play": {
		"Tifa Lockhart": {
			"Borderless Premier Play": {
				Edition: "PF25",
				Variant: "9",
			},
		},
	},
	"Magic Academy Promo": {
		"Mary Jane Watson": {"マジック・アカデミープロモ": {Edition: "PW25", Variant: "11"}},
	},
	"6ED": {
		"Balduvian Horde": {"世界選手権99プロモ": {Edition: "PWOR", Variant: "1"}},
	},
	"DOM": {
		"Zahid, Djinn of the Lamp": {"Draft Weekend": {Edition: "PDOM", Variant: "76"}},
	},
	"GRN": {
		"Firemind's Research": {"Draft Weekend": {Edition: "PGRN", Variant: "171"}},
	},
	"XLN": {
		"Deeproot Champion": {"Convention": {Edition: "PXLN", Variant: "185"}},
	},
	"CHK": {
		"Brothers Yamazaki": {"160": {Edition: "CHK", Variant: "160b"}},
	},
	"Magic Academy": {
		"Trinket Mage": {
			"Retro Frame Magic Academy": {
				Edition: "PW25",
				Variant: "8",
			},
		},
	},
	"MagicCon Promo": {
		"Lightning Bolt": {
			"MagicConプロモ": {
				Edition: "PF25",
				Variant: "13",
			},
		},
		"Sokka, Bold Boomeranger": {
			"Extended Art MagicConプロモ": {
				Edition: "PURL",
				Variant: "2025-4",
			},
		},
		"J. Jonah Jameson": {
			"Extended Art MagicConプロモ": {
				Edition: "PSPM",
				Variant: "3a",
			},
		},
		"Counterspell": {
			"Textless MagicConプロモ": {
				Edition: "PF24",
				Variant: "1",
			},
			"Full-Art 2":              {Edition: "PURL", Variant: "2"},
			"Retro Frame MagicConプロモ": {Edition: "PF26", Variant: "5"},
		},
	},
	"MagicFest": {
		"Lightning Bolt": {
			"": {
				Edition: "PF19",
				Variant: "1",
			},
			// This printing carries no promo type to disambiguate on.
			"Textless": {
				Edition: "PF19",
				Variant: "1",
			},
		},
	},
	"Judge Foil": {
		"Demonic Tutor": {
			"2020年版": {
				Edition: "J20",
				Variant: "4",
			},
			"2008年版ジャッジ褒賞": {
				Edition: "G08",
				Variant: "3",
			},
		},
		"Vindicate": {
			"2007年版ジャッジ褒賞": {
				Edition: "G07",
				Variant: "4",
			},
			"2013年版ジャッジ褒賞": {
				Edition: "G13",
				Variant: "7",
			},
		},
		"Vampiric Tutor": {
			"2000Ver. 2000年版ジャッジ褒賞": {
				Edition: "G00",
				Variant: "2",
			},
			"2018Ver. 2018年版ジャッジ褒賞": {
				Edition: "J18",
				Variant: "2",
			},
		},
		"Wasteland": {
			"2010Ver. 2010年版ジャッジ褒賞": {
				Edition: "G10",
				Variant: "8",
			},
			"2015Ver. 2015年版ジャッジ褒賞": {
				Edition: "J15",
				Variant: "8",
			},
		},
	},
	"UST-P": {
		"Earl of Squirrel": {"": {Edition: "PUST", Variant: "108"}},
	},
	"M15-P": {
		"Aeronaut Tinkerer": {"PURL": {Edition: "PURL", Variant: "8"}},
	},
	"ARENA": {
		"Plains":   {"ICE版アリーナFoilランド": {Edition: "PAL01", Variant: "7"}},
		"Island":   {"ICE版アリーナFoilランド": {Edition: "PAL01", Variant: "3"}},
		"Swamp":    {"ICE版アリーナFoilランド": {Edition: "PAL01", Variant: "9"}},
		"Mountain": {"ICE版アリーナFoilランド": {Edition: "PAL01", Variant: "5"}},
		"Forest":   {"ICE版アリーナFoilランド": {Edition: "PAL01", Variant: "1"}},
	},
	"Arena Foil Land": {
		"Mountain": {"MMQ版": {Edition: "PAL00", Variant: "11"}},
		"Forest":   {"ベータ版": {Edition: "PAL01", Variant: "11"}},
	},
	"USG": {
		"Plains": {"アリーナFoilランド": {Edition: "PAL99", Variant: "7"}},
	},
	"Game Day Promos": {
		"Niv-Mizzet, the Firemind": {"ゲームデー": {Edition: "PCMP", Variant: "2"}},
		"Urza's Factory": {
			"ゲームデー": {
				Edition: "PCMP",
				Variant: "5",
			},
		},
		"Bramblewood Paragon": {
			"ゲームデー": {
				Edition: "PCMP",
				Variant: "11",
			},
		},
		"Mutavault": {
			"ゲームデー": {
				Edition: "PCMP",
				Variant: "12",
			},
		},
		"Doran, the Siege Tower": {
			"ゲームデー": {
				Edition: "PCMP",
				Variant: "10",
			},
		},
		"Serra Avenger": {
			"ゲームデー": {
				Edition: "PCMP",
				Variant: "6",
			},
		},
	},
	"P30A": {
		"Arcane Signet": {
			"30周年プロモ": {
				Edition: "P30M",
				Variant: "1F",
			},
			"30周年記念": {
				Edition: "P30M",
				Variant: "1F",
			},
		},
	},
	"SLP": {
		"Lightning Bolt": {
			"": {
				Edition: "SLP",
				Variant: "37",
			},
		},
	},
	"": {
		"Mirrored Depths": {
			"その他プロモ": {
				Edition: "DCI",
				Variant: "44",
			},
		},
		"Celestine Reef": {
			"その他プロモ": {
				Edition: "DCI",
				Variant: "42",
			},
		},
	},
	"Misprint": {
		"Laquatus's Champion": {
			"印刷ミス": {
				Edition: "PTOR",
				Variant: "67†a",
			},
			"": {
				Edition: "PTOR",
				Variant: "67†",
			},
		},
	},
	"PRM": {
		"Rampant Growth": {
			"Etched Foil 1": {
				Edition: "PW23",
				Variant: "9",
			},
		},
	},
	"The List": {
		"Lightning Bolt":    {"Textless Magic Fest": {Edition: "PLST", Variant: "PF19-1"}},
		"Imperious Perfect": {"ゲームデー": {Edition: "PLST", Variant: "PCMP-9"}},
		"Abrade":            {"ゲームデー": {Edition: "PLST", Variant: "HOU-83"}},
		"Stasis Snare":      {"BFZ-P": {Edition: "PLST", Variant: "PBFZ-50"}},
		"Negate": {
			"褒賞プログラム": {Edition: "PLST", Variant: "P09-8"},
		},
		"Burst Lightning": {
			"Textless 褒賞プログラム": {Edition: "PLST", Variant: "P10-8"},
		},
		"Mortify": {
			"Textless 褒賞プログラム": {Edition: "PLST", Variant: "P07-3"},
		},
		"Harmonize": {
			"Textless 褒賞プログラム": {Edition: "PLST", Variant: "P08-5"},
		},
	},
	"Mystery Booster/The List": {
		"Lightning Bolt": {
			"Textless Magic Fest": {
				Edition: "PLST",
				Variant: "PF19-1",
			},
		},
	},
	"Japan Junior Tournament": {
		"Serra Avatar": {
			"": {
				Edition: "PSUS",
				Variant: "2",
			},
		},
	},
}
