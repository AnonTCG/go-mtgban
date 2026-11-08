package coolstuffinc

import (
	"errors"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/matcher/magic"
)

const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var numFixes = map[string]string{
	"GolgariSignetCM2":                "CM2191",
	"GolgariSignetCM2v2":              "CM2192",
	"Aura_Shards":                     "PLSTCMD-182",
	"aurashardslist2":                 "PLSTINV-233",
	"NissaWhoShakestheWorld518v2":     "SLD518",
	"SorcerousSpyglassv2":             "PXLN248p",
	"253486Signed_gold":               "WC97JK1",
	"one420eleshnornmotherofmachines": "ONE420",
	"295044":                          "SLD51",
	"wild0042":                        "SLP42",
	"living0030":                      "SLP30",
	"fatalpushsld884":                 "SLD884",
	"Sol381656":                       "SLD1512",
	"Jetmir381652":                    "SLD1509",
	"SLD318a":                         "SLD318",
	"SLD050fea":                       "SLD50",
	"ThoughtseizeSLDJPNFEa":           "SLD1117jpn",
	"TDM0300a":                        "TDM300",
	"LTR0425":                         "LTR425",
	"LTR0400PrereleaseNoStamp":        "LTR400",
	"LeylineoftheVoidv2":              "PM20107p",
	"386443":                          "POTJ149p",
	"Ugin001":                         "M211",
	"SLDDPDispute":                    "SLDIFIYW-1",
	"SLDDPBolt":                       "SLDIFIYW-2",
	"SLDDPThrill":                     "SLDIFIYW-3",
	"SLDDPGreaves":                    "SLDIFIYW-4",
	"SLDDPSolRing":                    "SLDIFIYW-5",
	"SLDDPDisputeCF":                  "SLDIFIYW-6",
	"SLDDPBoltCF":                     "SLDIFIYW-7",
	"SLDDPThrillCF":                   "SLDIFIYW-8",
	"SLDDPGreavesCF":                  "SLDIFIYW-9",
	"SLDDPSolRingCF":                  "SLDIFIYW-10",
	"SLDMikuSwanSong":                 "SLD1591",
	"414937":                          "FIN385",
	"414881":                          "FIN398",
	"414952":                          "FIN382",
	"381147":                          "PMKM187p",
	"AssassinsTrophyv2":               "PGRN152p",
	"BlastZonev2":                     "PWAR244p",
	"BreedingPoolv2":                  "PRNA246p",
	"OvergrownTombv2":                 "PGRN253p",
	"291465":                          "PGRN258p",
	"WateryGravev2":                   "PGRN259p",
	"BloodCryptv2":                    "PRNA245p",
	"GodlessShrinev2":                 "PRNA248p",
	"HallowedFountainv2":              "PRNA251p",
	"SteamVentsv2":                    "PGRN257p",
	"StompingGroundv2":                "PRNA259p",
	"SLD1129a":                        "SLD1129",
	"SLD1113a":                        "SLD1113",
	"SLD1125a":                        "SLD1125",
	"SLD1995a":                        "SLD1995",
	"fdn0720b":                        "FDN720",
	"SeaGateOracle043a":               "KHC43",
	"40K169a":                         "40K169",
	"ONE272a":                         "ONE272",
	"ONE273a":                         "ONE273",
	"ONE274a":                         "ONE274",
	"ONE275a":                         "ONE275",
	"ONE276a":                         "ONE276",
	"SLDMikuScryingSheets":            "SLD1606",
	"SLDMikuYouthValk":                "SLD1588",
	"SLD1117a":                        "SLD1117",
	"SLD1117fea":                      "SLD1117",
	"TMT0228_02":                      "TMT228",
	"Island_78_EvT":                   "DDF78",
	"Island_79_EvT":                   "DDF79",
	"SLDMikuGiadaJPN":                 "SLD1586",
	"SLDMikuYouthValkJPN":             "SLD1588",
	"SLD1112JPN":                      "SLD1112",
	"396167":                          "THB009",
	"mid265u":                         "PMID265p",
	"446707":                          "INR71",
	"StrategicBetrayal0422":           "TDM422",
	// The images of Tellah 416385 and 416386 are swapped on CSI's side,
	// and so are those of the two Copy Land products.
	"FIN0510": "FIN349",
	"FIN0349": "FIN510",
	"M3C0047": "M3C99",
	"M3C0099": "M3C47",
}

// shelfNumFixes holds the image stems that name a printing only on one shelf:
// CSI reuses these generic ones on the shelves of every other reprint.
var shelfNumFixes = map[string]map[string]string{
	"Commander Anthology Volume II": {
		"TempleoftheFalse_God271": "CM2271",
		"TempleoftheFalseGod":     "CM2272",
		"SolemnSimulacrum":        "CM2218",
		"SolemnSimulacrum__219v2": "CM2219",
	},
}

var variantTable = map[string]string{
	"Jeff A Menges":                                    "Jeff A. Menges",
	"Jeff a Menges":                                    "Jeff A. Menges",
	"San Diego Comic-Con Promo M15":                    "SDCC 2014",
	"San Diego Comic-Con Promo M14":                    "SDCC 2013",
	"EURO Land White Cliffs of Dover Ben Thompson art": "EURO White Cliffs of Dover",
	"EURO Land Danish Island Ben Thompson art":         "EURO Land Danish Island",
	"Eighth Edition Prerelease Promo":                  "Release Promo",
	"Release 27 Promo":                                 "Release",
	"2/2 Power and Toughness":                          "misprint",
	"Big Furry Monster Left Side":                      "28",
	"Big Furry Monster Right Side":                     "29",
}

// anthologyDecks maps the deck a Duel Decks: Anthology basic names in its
// notes to the deck's own set.
var anthologyDecks = map[string]string{
	"Garruk vs Liliana": "GVL",
	"Elves vs Goblins":  "EVG",
	"Goblins vs Elves":  "EVG",
	"Jace vs Chandra":   "JVC",
	"Divine vs Demonic": "DVD",
}

// basicSibling holds the suffix of the printing a basic's image number names
// when the set also files a lettered sibling under the same digits: the
// unsuffixed number in Zendikar, and "a" in Battle for Zendikar, whose
// unsuffixed number is the full art CSI sells as its own product.
var basicSibling = map[string]string{
	"Zendikar":            "",
	"Battle for Zendikar": "a",
}

// finishSplitNote reads the two numbers a note offers for one product whose
// printings differ only by finish: "Card number can be 210 or 265", or
// "Nonfoil is number 0765, Foil is 0720". A range or a longer list names
// more than one printing per finish and is not read.
var finishSplitNote = regexp.MustCompile(`(?i)(?:card number can be 0*(\d+) or|nonfoil is number 0*(\d+), foil is) 0*(\d+)(?:\s*$|\s+-\s)`)

// finishSplitCard answers the one printing, in the row's own set, among the
// numbers the note offers that carries the listing's finish. A foreign
// language row is left to the ordinary path, which carries its language.
func finishSplitCard(b *matcher.Backend, cardName, setCode, notes, language string, isFoil bool) *matcher.InputCard {
	m := finishSplitNote.FindStringSubmatch(notes)
	if m == nil || language != "" {
		return nil
	}
	first, second := m[1]+m[2], m[3]
	ids, err := b.SearchEquals(cardName)
	if err != nil {
		return nil
	}
	var found *matcher.InputCard
	for _, id := range ids {
		co, err := b.GetUUID(id)
		if err != nil || (co.Number != first && co.Number != second) || co.Foil != isFoil || co.Etched {
			continue
		}
		if setCode != "" && co.SetCode != setCode {
			continue
		}
		if found != nil {
			return nil
		}
		found = &matcher.InputCard{Name: cardName, Edition: co.SetCode, Variation: co.Number, Foil: isFoil}
	}
	return found
}

// shelfFinishSplit answers finishSplitCard for a sale listing, scoped to the
// shelf's own set where its name resolves to one that holds the card; a shelf
// filing another set's printings does not. A listing naming a language is
// never split.
func shelfFinishSplit(b *matcher.Backend, cardName, edition, notes string, isFoil bool) *matcher.InputCard {
	if !finishSplitNote.MatchString(notes) {
		return nil
	}
	if buylistLanguage(cardName, notes) != "" {
		return nil
	}
	shelf, err := b.GetSetByName(edition)
	if err == nil {
		split := finishSplitCard(b, cardName, shelf.Code, notes, "", isFoil)
		if split != nil {
			return split
		}
	}
	return finishSplitCard(b, cardName, "", notes, "", isFoil)
}

var nameTable = map[string]string{
	"Yennet, Cryptic Sovereign":              "Yennett, Cryptic Sovereign",
	"Invasion of Moag // Bloomweaver Dryads": "Invasion of Moag // Bloomwielder Dryads",
	"Bene Supremo":                           "Greater Good",
	"Ambitious Farmhand // Seasoned Cather":  "Ambitious Farmhand // Seasoned Cathar",
	"Maalfield Twins":                        "Maalfeld Twins",
	"Environmental Studies":                  "Environmental Sciences",
	"Proficient Pryodancer":                  "Proficient Pyrodancer",
	"Leotau Grizalho":                        "Grizzled Leotau",
	"____ ____ ____ Trespasser":              "_____ _____ _____ Trespasser",
	"Garravoraz":                             "Vorstclaw",
	"Gepanzerter Wasserwanderer":             "Plated Seastrider",
	"Camminatore di Phyrexia":                "Phyrexian Walker",
	"Odric, Lunarch Marshall":                "Odric, Lunarch Marshal",
	"Alesha, Whos Smiles at Death":           "Alesha, Who Smiles at Death",
	"Pertified Hamlet":                       "Petrified Hamlet",
	"Zuri, Warrior of Wakana":                "Zuri, Warrior of Wakanda",
	"Rin and Seri, Inseperable":              "Rin and Seri, Inseparable",
	"Shadowheart, Dark Justicar":             "Shadowheart, Dark Justiciar",
	"Sakashima the Imposter":                 "Sakashima the Impostor",

	"Doric, Nature's Warden // Doric, Owlbear Avenger": "Doric, Nature's Warden",
}

// buylistLanguage reads a foreign-language marker out of a buylist row's
// name or notes, Japan Showcase and Ghostfire treatments included: MTGJSON
// files those as ordinary English rows, so the language check below refuses
// them the same as any other print with no localized row on file.
func buylistLanguage(name, notes string) string {
	switch {
	case strings.Contains(name, "Japanese") || strings.Contains(notes, "Japanese"):
		return "Japanese"
	case strings.Contains(name, "- Spanish") || strings.Contains(notes, "Spanish"):
		return "Spanish"
	}
	return ""
}

// preprocess answers the card a retail listing names. foil is the finish of
// the offer row, which the listing's own wording does not carry.
func preprocess(b *matcher.Backend, cardName, edition, notes, imgURL string, foil bool) (*matcher.InputCard, error) {
	imgName := strings.TrimSuffix(path.Base(imgURL), filepath.Ext(imgURL))
	fixup, curated := numFixes[imgName]
	if !curated {
		fixup, curated = shelfNumFixes[edition][imgName]
	}
	if curated {
		imgName = fixup
	}

	variant := cleanVariant(notes)
	vars, found := variantTable[variant]
	if found {
		variant = vars
	}

	cardName = csiSignedFoil.ReplaceAllString(cardName, "FOIL Signed by")
	if strings.Contains(cardName, "Signed") && strings.Contains(cardName, "by") {
		cuts := matcher.Cut(cardName, "Signed")
		cardName = cuts[0]
	}

	variants := matcher.SplitVariants(cardName)
	if len(variants) > 1 {
		cardName = variants[0]
		if variant != "" {
			variant += " "
		}
		variant += strings.Join(variants[1:], " ")
	}

	isFoil := false
	if strings.Contains(cardName, "FOIL") {
		cardName = strings.Replace(cardName, " FOIL", "", 1)
		isFoil = true
	}

	if strings.HasSuffix(cardName, "Promo") {
		cuts := matcher.Cut(cardName, "Promo")
		cardName = cuts[0]
	}
	cardName = strings.TrimSpace(cardName)

	fixup, found = nameTable[cardName]
	if found {
		cardName = fixup
	}

	split := shelfFinishSplit(b, cardName, edition, notes, isFoil || foil)
	if split != nil {
		return split, nil
	}

	cardName, edition, variant, err := magicShelfFixups(cardName, edition, variant)
	if err != nil {
		return nil, err
	}

	// Skip tokens with the same names as cards
	if isEmblemListing(b, cardName, variant) {
		return nil, matcher.ErrUnsupported
	}

	if edition == "Duel Decks: Anthology" {
		code, found := anthologyDecks[variant]
		if found {
			edition = code
		}
	}
	input := basicLandListing(b, cardName, edition, variant, isFoil, imgName)
	if input != nil {
		return input, nil
	}

	// A "ds" in front of the set code marks a double-sided card's image.
	rest, found := strings.CutPrefix(imgName, "ds")
	if found && !hasSetPrefix(b, imgName) && hasSetPrefix(b, rest) {
		imgName = rest
	}

	// The promo pack's images are named for the set they were printed for or
	// for the product, so a set and number read out of one is the base card's,
	// unless numFixes names the printing.
	imgName = imageNumberStem(b, cardName, edition, variant, imgName)
	if len(imgName) > 4 && (curated || edition != "Universal Promo Pack") {
		for i := range 2 {
			maybeSet := strings.ToUpper(imgName[:i+3])
			maybeNum := strings.TrimLeft(imgName[i+3:], "_0")
			if edition == "Mystery Booster Reprints" {
				listNum := maybeSet + "-" + maybeNum
				if len(b.MatchInSetNumber(cardName, "PLST", listNum)) == 1 {
					return imageCard(cardName, "PLST", listNum, variant, isFoil), nil
				}
				// The image is the original printing's, not The List's.
				continue
			}
			if imagePrinting(b, cardName, maybeSet, maybeNum, variant) {
				if edition == "Prerelease Promo" {
					input = prereleaseImageCard(b, cardName, maybeSet, maybeNum, variant, isFoil)
					if input != nil {
						return input, nil
					}
				}
				return imageCard(cardName, maybeSet, maybeNum, variant, isFoil), nil
			}
		}
		input = imageNumberAfterName(b, cardName, edition, variant, imgName, isFoil)
		if input != nil {
			return input, nil
		}
		input = imageLetterAfterName(b, cardName, edition, variant, imgName, isFoil)
		if input != nil {
			return input, nil
		}
		// A letter can stand between the set code and the number, marking
		// the treatment: "TMCS0032" is the surge foil of TMC 32, where
		// "TMC0093" is the pixel art one filed at its own number. Neither
		// length above reads it - three characters leave "S0032", which is
		// no number, and four leave "TMCS", which is no set - so the two
		// listings both answer with 93 and the cheaper of them is priced as
		// the dearer. The letters are dropped and the digits behind them
		// asked for, which the set and the number together still have to
		// agree on.
		maybeSet := strings.ToUpper(imgName[:3])
		maybeNum := strings.TrimLeft(imgName[3:], "_0")
		trimmed := strings.TrimLeft(maybeNum, letters)
		if trimmed != maybeNum {
			maybeNum = strings.TrimLeft(trimmed, "_0")
			if imagePrinting(b, cardName, maybeSet, maybeNum, variant) {
				return imageCard(cardName, maybeSet, maybeNum, variant, isFoil), nil
			}
		}
	}

	var language string
	switch edition {
	// The shelf mixes every foreign black-bordered print run under one
	// name; the language written in the note or the bracket is the only
	// thing that says which one. Only Italian (FBB) and Japanese (4BB)
	// have a row in the datastore - the rest were never captured as their
	// own set, so a German/French/Spanish/Chinese/Korean listing has
	// nothing to resolve against.
	case "Black Bordered (foreign)":
		switch {
		case strings.Contains(variant, "Italian"):
			edition = "FBB"
			language = "Italian"
		case strings.Contains(variant, "Japanese"):
			edition = "4BB"
			language = "Japanese"
		default:
			return nil, matcher.ErrUnsupported
		}

	case "Promo":
		// Black Lotus - Ultra Pro Puzzle - Eight of 9
		if strings.Contains(cardName, "Ultra Pro Puzzle") {
			return nil, matcher.ErrUnsupported
		}

		switch variant {
		case "Junior Super Series Promo",
			"Junior Super Series Promo Carl Critchlow art":
			edition = "PSUS"
			variant = ""
		default:
			possibleEd, possibleVar := card2promo(cardName, variant)
			if variant != possibleVar {
				variant = possibleVar
			}
			if possibleEd != "" {
				edition = possibleEd
			}
		}

	case "Prerelease Promo":
		variant = strings.Replace(variant, "Core 21", "Core Set 2021", 1)
		if variant == "Ixalan Prerelease Promo" {
			variant = "Prerelease Ixalan"
		}

	case "Universal Promo Pack":
		m := promoPackSymbol.FindStringSubmatch(variant)
		if strings.HasPrefix(imgName, "UPP") && len(imgName) > 6 {
			maybeSet := strings.ToUpper(imgName[3:6])
			if maybeSet != "" {
				variant = maybeSet
			}
		} else if m != nil {
			set, err := b.GetSetByName(m[1])
			if err == nil {
				variant = set.Code
			}
		}

	case "Deckmasters":
		variant = strings.TrimSpace(strings.Split(variant, "Deckmaster")[0])

	case "Final Fantasy Variants":
		// The two-sided cards with no number in their image are the
		// borderless ones; the note names only the colour behind the art.
		if finalFantasyBackground.MatchString(variant) {
			variant += " Borderless"
		}

	case "Unfinity":
		variant = strings.Replace(variant, ",", "/", -1)

	case "Conspiracy: Take the Crown":
		if cardName == "Kaya, Ghost Assassin" && variant == "Alternate Art Foil" {
			variant = "222"
		}
	}

	input = &matcher.InputCard{
		Name:      cardName,
		Variation: variant,
		Edition:   edition,
		Foil:      isFoil,
		Language:  language,
	}
	if edition == "Mystery Booster Reprints" {
		input = listReprint(b, input)
	}
	return input, nil
}

// listReprint moves a Mystery Booster Reprints listing that landed on the
// printing The List copied from onto The List's own row, which is the one
// the shelf sells. The List files each copy as "<set>-<number>".
func listReprint(b *matcher.Backend, input *matcher.InputCard) *matcher.InputCard {
	probe := *input
	id, err := b.Match(&probe)
	if err != nil {
		return input
	}
	co, err := b.GetUUID(id)
	if err != nil || co.SetCode == "PLST" || co.SetCode == "ULST" {
		return input
	}
	listNum := co.SetCode + "-" + co.Number
	if len(b.MatchInSetNumber(co.Name, "PLST", listNum)) != 1 {
		return input
	}
	return &matcher.InputCard{Name: co.Name, Variation: listNum, Edition: "PLST", Foil: input.Foil}
}

// prereleaseImageCard answers the stamped printing a prerelease listing
// sells when its image names the set's booster printing: the image is the
// card's own art, so the "P<set>" number-"s" row is asked for instead. It
// answers nothing where the image already names a prerelease printing or
// the set has no such stamped row.
func prereleaseImageCard(b *matcher.Backend, cardName, set, number, variant string, isFoil bool) *matcher.InputCard {
	for _, card := range b.MatchInSetNumber(cardName, set, number) {
		if slices.Contains(card.PromoTypes, magic.PromoTypePrerelease) {
			return nil
		}
	}
	if !imagePrinting(b, cardName, "P"+set, number+"s", variant) {
		return nil
	}
	return imageCard(cardName, "P"+set, number+"s", variant, isFoil)
}

// imagePrinting reports whether a product image's set and number name exactly
// one printing of the card, whose front face a listing may name alone. A
// variants shelf reuses the base card's image for its extended art copy, so
// when the listing asks for extended art, the image is turned down if it
// names a printing without it where the set holds one.
func imagePrinting(b *matcher.Backend, cardName, setCode, number, variant string) bool {
	cards := b.MatchInSetNumber(cardName, setCode, number)
	set, err := b.GetSet(setCode)
	if err != nil {
		return false
	}
	if len(cards) == 0 {
		for _, card := range set.Cards {
			if card.Number == number && (matcher.Equals(card.FaceName, cardName) || matcher.Equals(card.Name, cardName)) {
				cards = append(cards, card)
			}
		}
	}
	if len(cards) != 1 {
		return false
	}
	// The dagger copy shares the plain number's image, so a note the
	// matcher reads as that copy decides between the two.
	for _, card := range set.Cards {
		if variant != "" && card.Number == number+"†" && (matcher.Equals(card.FaceName, cardName) || matcher.Equals(card.Name, cardName)) && noteLandsOn(b, cardName, setCode, variant, card.Number) {
			return false
		}
	}
	if !matcher.Contains(variant, "Extended Art") || isExtendedArt(cards[0]) {
		return true
	}
	for _, card := range set.Cards {
		if card.Name == cardName && isExtendedArt(card) {
			return false
		}
	}
	return true
}

// noteLandsOn reports whether the listing's note, asked in this set, lands on
// the given number.
func noteLandsOn(b *matcher.Backend, cardName, setCode, variant, number string) bool {
	cardID, err := b.Match(&matcher.InputCard{Name: cardName, Edition: setCode, Variation: variant})
	if err != nil {
		return false
	}
	co, err := b.GetUUID(cardID)
	if err != nil {
		return false
	}
	return co.SetCode == setCode && co.Number == number
}

// isExtendedArt reports an extended art printing, by promo type or by frame.
func isExtendedArt(card matcher.Card) bool {
	return slices.Contains(card.PromoTypes, "extendedart") || slices.Contains(card.FrameEffects, "extendedart")
}

// imageCard asks for the printing a product image names, in the finish and
// language the listing's own wording gives it: the image is the same for the
// etched and the plain copy, and for the Japanese one.
func imageCard(cardName, set, number, variant string, isFoil bool) *matcher.InputCard {
	for _, tag := range [...]string{"etched", "Japanese"} {
		if matcher.Contains(variant, tag) {
			number += " " + tag
		}
	}
	return &matcher.InputCard{Name: cardName, Variation: number, Edition: set, Foil: isFoil}
}

// hasSetPrefix reports whether an image stem opens with a set code.
func hasSetPrefix(b *matcher.Backend, stem string) bool {
	for _, n := range [...]int{3, 4} {
		if len(stem) <= n {
			continue
		}
		_, err := b.GetSet(strings.ToUpper(stem[:n]))
		if err == nil {
			return true
		}
	}
	return false
}

// stemLetters keeps the letters and digits of a name, as CSI spells it into
// an image file name.
func stemLetters(name string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, strings.ToLower(name))
}

// imageNumberStem drops the card's own name from the end of an image stem
// ("znr389roileruption"), leaving the set and number the plain stems carry.
// The prerelease shelf keeps only the stems that carry the "s" of the stamped
// printing, since the bare number there is the intro pack's, and a variant
// the matcher already places by its own table is left to it.
func imageNumberStem(b *matcher.Backend, cardName, edition, variant, imgName string) string {
	name := stemLetters(cardName)
	low := strings.ToLower(imgName)
	if name == "" || len(low) <= len(name) || !strings.HasSuffix(low, name) {
		return imgName
	}
	stem := imgName[:len(imgName)-len(name)]
	if edition == "Prerelease Promo" && !strings.HasSuffix(strings.ToLower(stem), "s") {
		return imgName
	}
	set, err := b.GetSetByName(edition)
	if err == nil {
		_, tabled := magic.VariantsTable[set.Name][cardName][strings.ToLower(variant)]
		if tabled {
			return imgName
		}
	}
	return stem
}

// imageNumberAfterName reads a stem that writes the card's name before the
// number ("borosguildgate244") as that number in the shelf's own set.
func imageNumberAfterName(b *matcher.Backend, cardName, edition, variant, imgName string, isFoil bool) *matcher.InputCard {
	name := stemLetters(cardName)
	low := strings.ToLower(imgName)
	if name == "" || !strings.HasPrefix(low, name) {
		return nil
	}
	num := strings.TrimLeft(low[len(name):], "_0")
	if num == "" || leadingDigits(num) != num {
		return nil
	}
	set, err := b.GetSetByName(edition)
	if err != nil || !imagePrinting(b, cardName, set.Code, num, variant) {
		return nil
	}
	return imageCard(cardName, set.Code, num, variant, isFoil)
}

// imageLetterAfterName reads a stem that is the card's name and one letter
// ("targetminotaurb") as the lettered number of that name in the shelf's own
// set, when exactly one printing of the card carries that letter.
func imageLetterAfterName(b *matcher.Backend, cardName, edition, variant, imgName string, isFoil bool) *matcher.InputCard {
	name := stemLetters(cardName)
	low := strings.ToLower(imgName)
	if name == "" || len(low) != len(name)+1 || !strings.HasPrefix(low, name) {
		return nil
	}
	letter := low[len(name):]
	if letter < "b" || letter > "z" {
		return nil
	}
	set, err := b.GetSetByName(edition)
	if err != nil {
		return nil
	}
	var numbers []string
	for _, card := range set.Cards {
		if card.Number == leadingDigits(card.Number)+letter && (matcher.Equals(card.FaceName, cardName) || matcher.Equals(card.Name, cardName)) {
			numbers = append(numbers, card.Number)
		}
	}
	if len(numbers) != 1 {
		return nil
	}
	return imageCard(cardName, set.Code, numbers[0], variant, isFoil)
}

// basicLandLetter matches a shelf-lettered basic land ("Island A",
// "Snow-Covered Forest C") or a plain one ("Mountain"): CSI sells every art
// of a basic under the same card name, telling the printings apart only by
// a trailing letter or by the product image.
var basicLandLetter = regexp.MustCompile(`^((?:Snow-Covered )?(?:Plains|Island|Swamp|Mountain|Forest|Wastes))(?:\s+([A-Z]))?$`)

// basicLandListing resolves a lettered or letterless basic land listing to
// the printing its image names, deferring to the ordinary pipeline first
// and only stepping in where that already fails. Battle Royale is the
// exception: CSI letters its arts in collector number order, and the
// matcher's own letters follow another, so there the position of the letter
// decides and the matcher's reading of it is never used.
func basicLandListing(b *matcher.Backend, cardName, edition, variant string, isFoil bool, imgName string) *matcher.InputCard {
	m := basicLandLetter.FindStringSubmatch(cardName)
	if m == nil {
		return nil
	}
	base, letter := m[1], m[2]
	ownLetters := edition == "Battle Royale"

	if !ownLetters {
		already := &matcher.InputCard{Name: cardName, Edition: edition, Variation: variant, Foil: isFoil}
		_, err := b.Match(already)
		if err == nil {
			return nil
		}
	}

	return basicLandPrinting(b, base, letter, edition, variant, isFoil, imgName)
}

// basicLandPrinting finds the collector number of a basic among the
// candidates of its edition, from the number its image names or, failing
// that, from the position its letter holds among the plain numbers of a
// single set. Candidates are gathered under both foil states to require the
// same single number either way.
func basicLandPrinting(b *matcher.Backend, base, letter, edition, variant string, isFoil bool, imgName string) *matcher.InputCard {
	// A hash names no number or full art, whatever its characters spell.
	if imageHash.MatchString(strings.ToLower(strings.TrimSuffix(path.Base(imgName), filepath.Ext(imgName)))) {
		imgName = ""
	}
	var match string
	for _, foil := range [...]bool{false, true} {
		probe := &matcher.InputCard{Name: base, Edition: edition, Variation: variant, Foil: foil}
		_, err := b.Match(probe)
		var alias *matcher.AliasingError
		if !errors.As(err, &alias) && variant != "" {
			// The notes can rule every printing out; the edition alone then
			// has to name the candidates.
			probe = &matcher.InputCard{Name: base, Edition: edition, Foil: foil}
			_, err = b.Match(probe)
		}
		if !errors.As(err, &alias) {
			return nil
		}
		setCodes := map[string]bool{}
		setCode := ""
		var nums, regular, fullArt []string
		for _, id := range alias.Probe() {
			co, err := b.GetUUID(id)
			if err != nil {
				continue
			}
			setCode = co.SetCode
			setCodes[co.SetCode] = true
			number := strings.TrimSuffix(co.Number, magic.SuffixSpecial)
			nums = append(nums, number)
			if !co.IsFullArt {
				regular = append(regular, number)
			} else if !slices.Contains(fullArt, number) {
				fullArt = append(fullArt, number)
			}
		}

		digits := basicLandStemNumber(imgName, setCode, base)
		// Digits that name no candidate are a picture index or a hash.
		named := false
		for _, n := range nums {
			trimmed := strings.TrimLeft(n, "0")
			if trimmed == digits || strings.TrimRight(trimmed, letters) == digits {
				named = true
			}
		}
		if !named {
			digits = ""
		}
		if digits == "" && letter == "" && strings.HasSuffix(strings.ToLower(imgName), "fa") && len(fullArt) == 1 {
			digits = strings.TrimLeft(fullArt[0], "0")
		}
		if digits == "" && len(setCodes) == 1 {
			// CSI sells a full-art basic as its own unlettered product, so
			// the letters only count the other arts when the set has any.
			ordinals := regular
			if len(ordinals) == 0 {
				ordinals = nums
			}
			digits = basicLandOrdinal(ordinals, letter)
		}
		if digits == "" {
			return nil
		}
		found, matches, lettered := "", 0, false
		for _, n := range nums {
			trimmed := strings.TrimLeft(n, "0")
			if trimmed == digits {
				found = n
				matches++
				continue
			}
			// A letter-suffixed sibling on the same digits ("255a" next to
			// "255") means the stem alone can't tell the printings apart.
			bare := strings.TrimRight(trimmed, letters)
			if bare != trimmed && bare == digits {
				lettered = true
			}
		}
		suffix, sided := basicSibling[edition]
		if lettered && sided && slices.Contains(nums, digits+suffix) {
			found, matches, lettered = digits+suffix, 1, false
		}
		if matches != 1 || lettered || (match != "" && found != match) {
			return nil
		}
		match = found
	}
	input := imageCard(base, edition, match, variant, isFoil)
	if matcher.Contains(variant, "Surge Foil") {
		input.Variation += " Surge Foil"
	}
	return input
}

// imageHash matches an image file named by a hash rather than a set and
// number.
var imageHash = regexp.MustCompile(`^[0-9a-f]{32}$`)

// basicLandOrdinal reads a basic's letter as its position among the plain
// collector numbers of a set ("B" is the second one). Arts are lettered in
// number order, but a number with a suffix names an art the letters do not
// count, so it answers nothing. basicLandPrinting trims the star beforehand.
func basicLandOrdinal(nums []string, letter string) string {
	if letter == "" {
		return ""
	}
	var plain []int
	for _, n := range nums {
		v, err := strconv.Atoi(n)
		if err != nil {
			return ""
		}
		if !slices.Contains(plain, v) {
			plain = append(plain, v)
		}
	}
	slices.Sort(plain)
	idx := int(letter[0] - 'A')
	if idx >= len(plain) {
		return ""
	}
	return strconv.Itoa(plain[idx])
}

// basicLandStemNumber pulls a collector number out of a basic land image
// filename stem ("M13234", "forest1", or "265" alone), trimming a set code
// or the basic's name off the front and refusing anything left over that
// isn't itself a number.
func basicLandStemNumber(imgName, setCode, base string) string {
	stem := strings.ToLower(strings.TrimSuffix(path.Base(imgName), filepath.Ext(imgName)))
	fields := strings.Fields(strings.ToLower(base))
	word := fields[len(fields)-1]
	switch {
	case setCode != "" && strings.HasPrefix(stem, strings.ToLower(setCode)):
		stem = stem[len(setCode):]
	case strings.HasPrefix(stem, word):
		stem = stem[len(word):]
	case leadingDigits(stem) != "":
		// left as-is
	default:
		return ""
	}
	digits := leadingDigits(stem)
	return strings.TrimLeft(digits, "0")
}

func leadingDigits(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

func cleanVariant(variant string) string {
	if strings.Contains(variant, "Picture") {
		variant = strings.Replace(variant, "Picture 1", "", 1)
		variant = strings.Replace(variant, "Picture 2", "", 1)
		variant = strings.Replace(variant, "Picture 3", "", 1)
		variant = strings.Replace(variant, "Picture 4", "", 1)
	}
	if strings.Contains(variant, "Artist") {
		variant = strings.Replace(variant, "Artist ", "", 1)
	}
	variant = strings.Replace(variant, "(", "", -1)
	variant = strings.Replace(variant, ")", "", -1)
	variant = strings.Replace(variant, ",", "", -1)
	variant = strings.Replace(variant, ".", "", -1)
	variant = strings.Replace(variant, "- ", " ", -1)
	variant = strings.Replace(variant, "  ", " ", -1)
	variant = strings.Replace(variant, "\r\n", " ", -1)
	variant = strings.Replace(variant, "\n", " ", -1)
	variant = strings.Replace(variant, "  ", " ", -1)
	return strings.TrimSpace(variant)
}

var preserveTags = []string{
	"etched",
	"retro frame",
	"step-and-compleat",
	"serialized",
}

func card2promo(cardName, variant string) (string, string) {
	var edition string
	ogVariant := variant

	switch {
	case strings.Contains(variant, "30th Anniversary") && !strings.Contains(variant, "History Promos"):
		if strings.Contains(variant, "Japanese 30th Anniversary Celebration Tokyo Promo") {
			return "P30T", ""
		}
		return "P30A", ""
	case strings.Contains(variant, "Cowboy Bebop"):
		return "PCBB", ""
	case strings.Contains(variant, "PWCS"):
		return "PWCS", ""
	case strings.Contains(variant, "Magic Spotlight"), strings.Contains(variant, "Spotlight Series"):
		return "PSPL", ""
	case strings.Contains(variant, "Manga Promo") && !strings.Contains(variant, "Japanese"):
		return "PMEI", ""
	case strings.Contains(variant, "Eternal Weekend"):
		return "PEWK", ""
	case strings.Contains(variant, "2025 MagicCon"):
		return "PF25", ""
	case strings.Contains(variant, "Friday Night Magic Promo"):
		variant = "FNM"
	case strings.Contains(variant, "Japanese Summer Vacation"):
		return "PSVC", ""
	case strings.Contains(variant, "Japan Standard Cup 2025 Promo"):
		return "PJSC", ""
	case strings.Contains(variant, "MKM Standard Showdown"):
		return "MKM Standard Showdown", ""
	}

	switch cardName {
	case "Demonic Tutor":
		if variant == "Daarken Judge Rewards Promo" {
			variant = "Judge 2008"
		} else if variant == "Anna Steinbauer Judge Promo" {
			variant = "Judge 2020"
		}
	case "Vampiric Tutor":
		if variant == "Judge Rewards Promo Old Border" {
			variant = "Judge 2000"
		} else if variant == "Judge Rewards Promo New Border" {
			variant = "Judge 2018"
		}
	case "Wasteland":
		if variant == "Judge Rewards Promo Carl Critchlow art" {
			variant = "Judge 2010"
		} else if variant == "Judge Rewards Promo Steve Belledin art" {
			variant = "Judge 2015"
		}
	case "Vindicate":
		if variant == "Judge Rewards Promo Mark Zug art" {
			variant = "Judge 2007"
		} else if variant == "Judge Rewards Promo Karla Ortiz art" {
			variant = "Judge 2013"
		}
	case "Fling":
		// Only one of the two is present
		if variant == "Gateway Promo Wizards Play Network Daren Bader art" {
			variant = "DCI"
		}
	case "Sylvan Ranger":
		if variant == "Judge Rewards Promo Mark Zug art" {
			variant = "WPN"
		}
	case "Goblin Warchief":
		if ogVariant == "Friday Night Magic Promo Old Border" {
			return "F06", "5"
		} else if ogVariant == "Friday Night Magic Promo New Border" {
			return "F16", "5"
		}
	case "Cabal Therapy":
		if strings.HasPrefix(variant, "Gold-bordered") {
			variant = "2003"
		}
	case "Rishadan Port":
		if strings.HasPrefix(variant, "Gold-bordered") {
			variant = "2000"
		}
	case "Hangarback Walker":
		edition = "Love your LGS"
		variant = "2"
	case "Chord of Calling":
		edition = "Double Masters"
		variant = "Release"
	case "Wrath of God":
		if variant == "Player Rewards Promo textless" {
			edition = "P07"
			variant = "1"
		} else {
			edition = "Double Masters"
			variant = "Release"
		}
	case "Conjurer's Closet":
		edition = "PW21"
		variant = "6"
	case "Cryptic Command":
		if variant == "Qualifier Promo" {
			edition = "PPRO"
			variant = "2020-1"
		}
	case "Dauntless Dourbark":
		edition = "DCI"
		variant = "12"
	case "Eye of Ugin":
		edition = "J20"
		variant = "10"
	case "Serra Avatar":
		if variant == "Junior Super Series Promo Dermot Power art" {
			edition = "PSUS"
			variant = "2"
		}
	case "Steward of Valeron":
		edition = "PURL"
		variant = "1"
	case "Llanowar Elves":
		switch ogVariant {
		case "Friday Night Magic Promo":
			edition = "FNM"
			variant = "11"
		case "Open House Promo":
			edition = "PDOM"
			variant = "168"
		case "Retro Frame Tin Promo":
			edition = "PDMU"
			variant = "Retro Frame"
		}
	case "Masked Vandal":
		edition = "KHM"
		variant = "405"
	case "Reliquary Tower":
		if variant == "Textless Commander Promo" {
			edition = "PF23"
			variant = "3"
		}
	case "Gideon, Ally of Zendikar":
		if variant == "2016 San Diego Comic Con Zombie Promo BFZ" {
			edition = "PS16"
			variant = "29"
		} else if variant == "Regional Championship Qualifiers 2022" {
			edition = "PRCQ"
			variant = "1"
		}
	case "Snapcaster Mage":
		if variant == "2016 Regional PTQ Promo" {
			edition = "PPRO"
		} else if variant == "Regional Championship Qualifier 2023" {
			edition = "PRCQ"
		}
	case "Avacyn's Pilgrim":
		if variant == "2025 Festival Promo" {
			edition = "PF25"
		}
	case "Counterspell":
		if variant == "Festival # 0001" {
			edition = "PF24"
		}

	case "Aven Mindcensor", "Dig Through Time", "Goblin Guide", "Scavenging Ooze":
		if variant == "Love Your Local Game Store Promo" {
			return "PLG21", ""
		}
	case "Bolas's Citadel":
		if variant == "Draft Weekend Promo" {
			edition = "PWAR"
			variant = "79"
		} else if variant == "Love Your Local Game Store Promo" {
			edition = "PLG21"
			variant = "3"
		}
	case "Nicol Bolas",
		"Earthquake",
		"Serra Angel":
		if variant == "Japanese Magic x Duel Masters Promo" {
			return "PMDA", ""
		}
	case "Sol Ring":
		if variant == "Commander Promo" {
			edition = "PF19"
		} else if matcher.Equals(variant, "Love Your Local Game Store promo") {
			edition = "PLG22"
			variant = "1"
		}
	case "Arcane Signet":
		if variant == "Festival Magic Con" {
			return "P30M", "1F"
		}
		if strings.Contains(variant, "Magic 30") {
			if matcher.Contains(variant, "etched") {
				return "P30M", "etched"
			}
			return "P30M", ""
		}
	case "Sakura-Tribe Elder":
		if variant == "Textless Victor Adame Minguez art" {
			edition = "PLG24"
		}
	case "Dragon's Hoard":
		if variant == "Tarkir: Dragonstorm Magic Academy Promo" {
			return "PW25", "18"
		}
	case "Lightning Bolt":
		switch {
		case variant == "MagicFest Promo textless":
			return "PF19", "1"
		case strings.Contains(variant, "TMNT Standard Showdown"):
			return "PW26", "5"
		}
	case "Highly Illogical":
		if strings.Contains(variant, "Play Promo") {
			return "PW26", "17"
		}
	case "Wood Elves":
		if strings.Contains(variant, "Standard Showdown") {
			return "PW26", "16"
		}
	case "Tragic Trajectory":
		if strings.Contains(variant, "Standard Showdown") {
			return "PW26", "22"
		}
	case "Gandalf, Friend of the Shire":
		if strings.Contains(variant, "Play Promo") {
			return "PF23", "1"
		}
	case "Crystal Ball":
		if strings.Contains(variant, "Commander Night") {
			return "PW26", "21"
		}
	case "Swords to Plowshares":
		if strings.Contains(variant, "Final Fantasy Event Promo") {
			return "PF25", "12"
		}
	case "Cloud, Midgar Mercenary":
		switch variant {
		case "0001 FFVII Commander Deck Game Edition Promo":
			return "PMEI", "2025-21"
		case "English Language Pro Tour Final Fantasy Promo":
			return "PPRO", "2025-1"
		case "Japanese Language Magic Spotlight: Final Fantasy Promo":
			return "PSPL", "4"
		}
	case "Ultimate Green Goblin":
		return "PW25", "12"
	case "J. Jonah Jameson":
		return "PF25", "17"
	case "Katara, the Fearless":
		return "PURL", "2025-3"
	case "Unbreakable Formation":
		if strings.Contains(variant, "TMNT Promo") {
			return "PURL", "2025-5"
		}
	case "Mental Misstep":
		if strings.Contains(variant, "Phyrexian") {
			return "PMEI", "2023-1"
		}
	case "Lotus Bloom":
		if strings.Contains(variant, "Timeshifted") {
			return "TSR", "411"
		}
	case "Command Tower":
		if variant == "Marvel Super Heroes Event Promo" {
			return "PMEI", "2026-13"
		}
	case "Portable Hole":
		return "AFR", "398"
	}
	return edition, variant
}

// buylistNumberFixes corrects a Number field the vendor got wrong on a
// specific product; its own Image sku disagrees with Number on 268 other
// products for an unrelated reason, so this is a keyed table, not a rule.
var buylistNumberFixes = map[string]string{
	"343896": "675",      // Lightning Bolt (Hadoken): SLD x Street Fighter
	"306846": "315",      // Horizon Stone: Commander Legends extended art
	"391205": "244",      // Ratonhnhake:ton (Foil-Etched): Assassin's Creed
	"409117": "123",      // Stormscale Scion: Tarkir: Dragonstorm
	"325589": "368",      // Demonic Bargain: Crimson Vow extended art
	"299005": "356",      // Demonic Embrace: Core Set 2021 extended art
	"325967": "384",      // Avabruck Caretaker: Crimson Vow extended art
	"326111": "DDD-48",   // Bad Moon: The List, Garruk vs. Liliana
	"326909": "PDKA-127", // Strangleroot Geist: Game Day promo
	"411004": "315",      // Purging Stormbrood: Tarkir: Dragonstorm showcase
	"318838": "PBFZ-50",  // Stasis Snare: Game Day promo
	"289318": "C19-249",  // Graypelt Refuge: Commander 2019
	"296103": "280",      // Huntmaster Liger: Ikoria showcase
}

// buylistImageNumber retries a card whose Number field named no printing
// with the set and number its own Image sku carries, when that names
// exactly one printing. The candidate is confirmed through Match itself,
// language included - though Match clamps a mismatched foil request
// rather than reject it, so a landed candidate is not proof of finish.
func buylistImageNumber(b *matcher.Backend, cardName, variant string, isFoil bool, language, image string) *matcher.InputCard {
	fixup, found := numFixes[image]
	if found {
		image = fixup
	}
	stem := strings.ToUpper(image)
	for _, n := range [...]int{3, 4} {
		if len(stem) <= n {
			continue
		}
		setCode := stem[:n]
		num := strings.TrimLeft(stem[n:], "_0")
		if num == "" {
			continue
		}
		nums := []string{num}
		bare := strings.TrimRight(num, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
		if bare != num && bare != "" {
			nums = append(nums, bare)
		}
		for _, num := range nums {
			if len(b.MatchInSetNumber(cardName, setCode, num)) != 1 {
				continue
			}
			candidate := imageCard(cardName, setCode, num, variant, isFoil)
			candidate.Language = language
			_, err := b.Match(candidate)
			if err == nil {
				return candidate
			}
		}
	}
	return nil
}

// PreprocessBuylist is Preprocess for the buylist feed, which describes a card
// differently from the sale catalog.
func PreprocessBuylist(b *matcher.Backend, card CSIPriceEntry) (*matcher.InputCard, error) {
	if fix, ok := buylistNumberFixes[card.PID]; ok {
		card.Number = fix
	}
	// A two-sided token sheet prints one physical card for a pairing the
	// ordinary per-card pipeline below was never built for: a name like
	// "Soldier (Token) // Beast (Token)" carries no external id to route
	// through the way ManaPool's scryfall_id does, so it needs its own
	// anchor before that pipeline mangles it, and refuses outright rather
	// than fall into whatever single-face guess the rest of this function
	// would otherwise make of one half of the name.
	if strings.Contains(card.Name, "(Token)") && strings.Contains(card.Name, " // ") {
		return preprocessTokenPairBuylist(b, card)
	}

	language := buylistLanguage(card.Name, card.Notes)

	num := strings.TrimLeft(card.Number, "0")
	cleanVar := cleanVariant(card.Notes)
	edition := card.ItemSet
	isFoil := card.IsFoil == 1
	cardName := card.Name

	if matcher.Contains(cardName, "signed by") {
		return nil, matcher.ErrUnsupported
	}

	variant := num
	if variant == "" {
		variant = cleanVar
	}

	fixName := matcher.SplitVariants(cardName)
	var altVariant string
	if len(fixName) > 1 {
		altVariant = strings.Join(fixName[1:], " ")
		if variant != "" {
			variant += " "
		}
		variant += altVariant
	}
	cardName = fixName[0]

	fixup, found := nameTable[cardName]
	if found {
		cardName = fixup
	}

	vars, found := variantTable[variant]
	if found {
		variant = vars
		cleanVar = vars
	}
	vars, found = variantTable[cleanVar]
	if found {
		variant = vars
		cleanVar = vars
	}

	split := finishSplitCard(b, cardName, card.Code, card.Notes, language, isFoil)
	if split != nil {
		return split, nil
	}

	cardName, edition, variant, err := magicShelfFixups(cardName, edition, variant)
	if err != nil {
		return nil, err
	}

	// Skip tokens with the same names as cards
	if isEmblemListing(b, cardName, variant) {
		return nil, matcher.ErrUnsupported
	}

	switch edition {
	case "Zendikar", "Battle for Zendikar", "Oath of the Gatewatch":
		// Strip the extra letter from the name
		if magic.IsBasicLand(cardName) {
			cardName = strings.Fields(cardName)[0]
			if num != "" && cleanVar != "" {
				variant = strings.TrimSpace(variant + " " + cleanVar)
			}
		}
	case "Unstable":
		variant = cleanVar
	case "Prerelease Promo":
		// Without a stamp the card is the booster printing its image names.
		if matcher.Contains(card.Notes, "Non-Stamped") {
			retry := buylistImageNumber(b, cardName, "", isFoil, language, card.Image)
			if retry != nil {
				return retry, nil
			}
		}
	case "Mystery Booster Reprints":
		// Keep the cards that strictly need extra information
		switch cardName {
		case "Aerial Responder",
			"Command Tower",
			"Laboratory Maniac",
			"Everythingamajig",
			"Ineffable Blessing":
			variant = cleanVar
		case "Forest":
			if variant == "291" {
				variant = "292"
			}
		}
	case "Secret Lair":
		variant = cleanVar

		if num != "" {
			variant = num + " " + cleanVar
			// CSI's notes often just repeat the collector number followed by the
			// product/deck name ("2406 - Goblin Storm Commander Deck"), which is
			// noise that doubles the number and trips the variant/token filters.
			// When the notes lead with the collector number, trust it alone.
			if strings.HasPrefix(cleanVar, num) {
				variant = num
			}
		}
	case "Deckmasters":
		variant = strings.TrimSpace(strings.Split(variant, "Deckmaster")[0])
	case "Duel Decks: Anthology":
		if num != "" {
			variant = num + " " + cleanVar
		}
	case "Conspiracy: Take the Crown":
		if cardName == "Kaya, Ghost Assassin" && strings.Contains(variant, "Alternate") {
			variant = "222"
		}
	case "D&D Ampersand":
		edition = "PAFR"
		variant = "Ampersand"
	case "Promo":
		variant = cleanVar
		if cleanVar == "" {
			// No notes: the number alone is the promo pack's, and the
			// image names the set it belongs to.
			stem := imageNumberStem(b, cardName, edition, num, card.Image)
			retry := buylistImageNumber(b, cardName, num, isFoil, language, stem)
			if retry != nil {
				return retry, nil
			}
			variant = num
		}
		switch variant {
		case "Ravnica Weekend Promo":
			edition = variant
			variant = num
		case "Stained Glass Art", "Secret Lair Bonus Cards":
			edition = "SLD"
			variant = num
		case "Japan Planeswalker Series Summer 2025 Promo":
			edition = "PWCS"
			variant = ""
		case "Junior Super Series Promo",
			"Junior Super Series Promo Carl Critchlow art":
			edition = "PSUS"
			variant = ""
		default:
			possibleEd, possibleVar := card2promo(cardName, variant)
			if variant != possibleVar {
				variant = possibleVar
			}
			if possibleEd != "" {
				edition = possibleEd
			}
		}
	case "Unfinity":
		if strings.Contains(variant, ",") {
			variant = strings.Replace(altVariant, ",", "/", -1)
		}
	}

	// Add previously removed/ignored tags, reading the tag word itself out
	// of altVariant too (Secret Lair spells "Foil-Etched" in the name's own
	// parenthetical), without folding all of altVariant into variant.
	tagSource := strings.ToLower(cleanVar + " " + altVariant)
	for _, tag := range preserveTags {
		if strings.Contains(tagSource, tag) && !strings.Contains(strings.ToLower(variant), tag) {
			if variant != "" {
				variant += " "
			}
			variant += tag
		}
	}

	// Added last, so Secret Lair's own case above doesn't discard it:
	// IsJPN reads the word from the variation, not from Language.
	if language == "Japanese" && !strings.Contains(strings.ToLower(variant), "japanese") {
		if variant != "" {
			variant += " "
		}
		variant += "Japanese"
	}

	final := &matcher.InputCard{
		Language:  language,
		Name:      cardName,
		Variation: variant,
		Edition:   edition,
		Foil:      isFoil,
	}

	// A bad Number retries against the Image sku, except when Oversize
	// failed: the retry rebuilds Variation from the image and drops it.
	probe := *final
	_, err = b.Match(&probe)
	if err != nil && !(errors.Is(err, matcher.ErrUnsupported) && matcher.Contains(final.Variation, "Oversize")) {
		// The image names a printing of the basic, whatever its letter.
		retryName := cardName
		m := basicLandLetter.FindStringSubmatch(cardName)
		if m != nil {
			retryName = m[1]
		}
		retry := buylistImageNumber(b, retryName, final.Variation, isFoil, language, card.Image)
		if retry != nil {
			return retry, nil
		}
		if m != nil {
			basic := basicLandPrinting(b, m[1], m[2], edition, final.Variation, isFoil, card.Image)
			if basic != nil {
				return basic, nil
			}
		}
		// The set code and number CSI files the product under, when that
		// names exactly one printing of the card.
		if card.Code != "" && num != "" && len(b.MatchInSetNumber(cardName, card.Code, num)) == 1 {
			candidate := imageCard(cardName, card.Code, num, final.Variation, isFoil)
			candidate.Language = language
			probe := *candidate
			_, matchErr := b.Match(&probe)
			if matchErr == nil {
				return candidate, nil
			}
		}
	}
	return final, nil
}

// preprocessTokenPairBuylist resolves a two-sided token buylist row to the
// combined entity matcher/magic derives for it, anchored by the row's
// own set code and collector number - CSI publishes both (Code and Number)
// alongside the storefront's edition wording, unlike ManaPool's own single
// scryfall_id shape. A row this precise with no combined entity on file
// (no mtgjson tokenProducts record at all - not every real pairing has
// one) is refused rather than left for the rest of PreprocessBuylist to
// mistake for a single face.
func preprocessTokenPairBuylist(b *matcher.Backend, card CSIPriceEntry) (*matcher.InputCard, error) {
	isFoil := card.IsFoil == 1

	tokenSet := magic.EditionTokenSetCode(b, card.ItemSet)
	if tokenSet == "" && card.Code != "" {
		tokenSet = magic.SetTokenSetCode(b, card.Code)
	}
	if tokenSet == "" {
		tokenSet = card.Code
	}
	if tokenSet != "" {
		for _, number := range csiTokenPairNumbers(card.Number) {
			if uuid := magic.MatchNativeTokenPair(b, tokenSet, number, card.Name); uuid != "" {
				if id, err := b.MatchID(uuid, isFoil); err == nil {
					return &matcher.InputCard{ID: id}, nil
				}
			}
			if tcgID := magic.MatchTokenPairingBySetNumber(b, tokenSet, number, card.Name, isFoil); tcgID != "" {
				if id, err := b.MatchID(tcgID, isFoil); err == nil {
					return &matcher.InputCard{ID: id}, nil
				}
			}
		}
	}

	if pairID := magic.MatchTokenPairingByNamesAndEdition(b, card.Name, card.ItemSet, isFoil); pairID != "" {
		if id, err := b.MatchID(pairID, isFoil); err == nil {
			return &matcher.InputCard{ID: id}, nil
		}
	}

	return nil, matcher.ErrUnsupported
}

// csiTokenPairNumbers returns the collector numbers worth anchoring a two-
// sided token's first face against, in the order CSI's own Number field
// gives them: a compound "020/021" one per face, trusting which half names
// which face at face value since which one is first is not fixed across
// editions; a bare "003" (a boxed set's own singles-shaped numbering) tried
// as-is.
func csiTokenPairNumbers(number string) []string {
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

// isEmblemListing reports whether the "Emblem" in a listing's notes is a
// planeswalker emblem filed under a card's name. It is not when the word is
// part of what the card is called: the Jurassic World "Emblem Variant"
// treatment, or a printing's own flavor name.
func isEmblemListing(b *matcher.Backend, cardName, variant string) bool {
	if !strings.Contains(variant, "Emblem") || b.IsToken(cardName) {
		return false
	}
	if strings.Contains(variant, "Emblem Variant") {
		return false
	}
	ids, err := b.SearchEquals(cardName)
	if err != nil {
		return true
	}
	for _, id := range ids {
		co, err := b.GetUUID(id)
		if err != nil {
			continue
		}
		if co.FlavorName != "" && strings.Contains(variant, co.FlavorName) {
			return false
		}
	}
	return true
}

// magicShelfFixups reads the shapes this storefront gives a few classes of
// listing before the matcher sees them: an emblem named "Emblem" with the
// planeswalker in the notes, where the catalog names it after the
// planeswalker; the duel decks headed in the singular; the blank cards a
// few sets ship, which are not cards; a note saying either of two numbers
// may be received, of which the first is the one asked for; and the T the
// storefront hangs on a token's number.
func magicShelfFixups(cardName, edition, variant string) (string, string, string, error) {
	if cardName == "Emblem" && variant != "" {
		if strings.Contains(variant, "Token") || strings.Contains(variant, "//") {
			return "", "", "", matcher.ErrUnsupported
		}
		// The buylist writes the token number ahead of the planeswalker
		// ("7 The Capitoline Triad", "8/8 Tamiyo, the Moon Sage").
		cardName, variant = emblemNumber.ReplaceAllString(variant, "")+" Emblem", ""
	}
	if strings.HasPrefix(cardName, "Blank Card") {
		return "", "", "", matcher.ErrUnsupported
	}
	if strings.HasPrefix(edition, "Duel Deck:") {
		edition = "Duel Decks:" + strings.TrimPrefix(edition, "Duel Deck:")
	}
	if m := eitherNumber.FindStringSubmatch(variant); m != nil {
		variant = strings.TrimLeft(m[1], "0")
	}
	if m := tokenNumber.FindStringSubmatch(variant); m != nil {
		variant = m[1] + " Token"
	}
	return cardName, edition, variant, nil
}

var (
	eitherNumber = regexp.MustCompile(`(?i)either card number (\d+) or \d+`)
	tokenNumber  = regexp.MustCompile(`^(\d+)T Token$`)
	emblemNumber = regexp.MustCompile(`^\d+(?:/\d+)?[A-Za-z]? `)
)

// finalFantasyBackground matches the note of a Final Fantasy borderless
// variant: "XVI in Gray Background", or its misspelling "Blackground".
var finalFantasyBackground = regexp.MustCompile(`(?i) in .* Bl?ackground`)

// promoPackSymbol reads the set a Universal Promo Pack listing names in its
// note, "<Set Name> - Silver Planeswalker Symbol" once cleaned.
var promoPackSymbol = regexp.MustCompile(`^(.+) Silver Planeswalker Symbol$`)

// retailMagic reads an offer's wording, and keeps it only where the printing
// was sold in the finish the offer names.
func retailMagic(csi *Coolstuffinc, l sellListing) (*matcher.InputCard, string, error) {
	theCard, err := preprocess(csi.backend, l.name, l.edition, l.notes, l.image, l.foil)
	if err != nil {
		return nil, "", matcher.ErrUnsupported
	}
	// preprocess() might return something that derived foil status
	// from one of the fields (cardName in particular)
	theCard.Foil = theCard.Foil || l.foil
	cardID, err := l.match(csi.backend, theCard)
	if err != nil {
		return theCard, "", err
	}
	if strings.Contains(l.name, "Foil-etched") {
		co, err := csi.backend.GetUUID(cardID)
		if err != nil || !co.Etched {
			return theCard, "", matcher.ErrUnsupported
		}
	}
	if l.foil {
		co, err := csi.backend.GetUUID(cardID)
		if err != nil || (!co.Etched && !co.Foil) {
			return theCard, "", matcher.ErrUnsupported
		}
	}
	return theCard, cardID, nil
}

// buylistMagic reads a buylist row's wording, and drops the placeholder foil
// row some products carry beside their nonfoil one.
func buylistMagic(csi *Coolstuffinc, p CSIPriceEntry) (*matcher.InputCard, string, error) {
	theCard, err := PreprocessBuylist(csi.backend, p)
	if err != nil {
		return nil, "", matcher.ErrUnsupported
	}
	cardID, err := csi.backend.Match(theCard)
	if err != nil {
		return theCard, "", err
	}
	co, cerr := csi.backend.GetUUID(cardID)
	if cerr == nil && magicPhantomFoilTwin(co, p.IsFoil, csi.nonfoilPIDs[p.PID]) {
		return theCard, "", matcher.ErrUnsupported
	}
	return theCard, cardID, nil
}
