package coolstuffinc

import (
	"errors"
	"path"
	"path/filepath"
	"strings"

	"github.com/mtgban/go-mtgban/mtgmatcher"
)

var numFixes = map[string]string{
	"GolgariSignetCM2":                "CM2191",
	"GolgariSignetCM2v2":              "CM2192",
	"TempleoftheFalse_God271":         "CM2271",
	"TempleoftheFalseGod":             "CM2272",
	"SolemnSimulacrum":                "CM2218",
	"SolemnSimulacrum__219v2":         "CM2219",
	"Aura_Shards":                     "PLSTCMD-182",
	"aurashardslist2":                 "PLSTINV-233",
	"NissaWhoShakestheWorld518v2":     "SLD518",
	"SorcerousSpyglassv2":             "PXLN248p",
	"253486Signed_gold":               "WC97JK1",
	"one420eleshnornmotherofmachines": "ONE420",
	"295044":                          "SLD51",
	"wild0042":                        "SLD42",
	"Sol381656":                       "SLD1512",
	"TDM0300a":                        "TDM300",
	"LTR0425":                         "LTR425",
	"LeylineoftheVoidv2":              "PM20107p",
	"386443":                          "POTJ149p",
	"Ugin001":                         "M211",
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
}

func preprocess(cardName, edition, variant, imgURL string) (*mtgmatcher.InputCard, error) {
	imgName := strings.TrimSuffix(path.Base(imgURL), filepath.Ext(imgURL))
	fixup, found := numFixes[imgName]
	if found {
		imgName = fixup
	}

	variant = cleanVariant(variant)
	vars, found := variantTable[variant]
	if found {
		variant = vars
	}

	if strings.Contains(cardName, "Signed") && strings.Contains(cardName, "by") {
		cuts := mtgmatcher.Cut(cardName, "Signed")
		cardName = cuts[0]
	}

	variants := mtgmatcher.SplitVariants(cardName)
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
		cuts := mtgmatcher.Cut(cardName, "Promo")
		cardName = cuts[0]
	}
	cardName = strings.TrimSpace(cardName)

	fixup, found = nameTable[cardName]
	if found {
		cardName = fixup
	}

	// Skip tokens with the same names as cards
	if strings.Contains(variant, "Emblem") && !mtgmatcher.IsToken(cardName) {
		return nil, mtgmatcher.ErrUnsupported
	}

	if len(imgName) > 4 {
		for i := 0; i < 2; i++ {
			maybeSet := strings.ToUpper(imgName[:i+3])
			maybeNum := strings.TrimLeft(imgName[i+3:len(imgName)], "_0")
			if len(mtgmatcher.MatchInSetNumber(cardName, maybeSet, maybeNum)) == 1 {
				return &mtgmatcher.InputCard{
					Name:      cardName,
					Variation: maybeNum,
					Edition:   maybeSet,
					Foil:      isFoil,
				}, nil
			}
		}
	}

	switch edition {
	case "Promo":
		// Black Lotus - Ultra Pro Puzzle - Eight of 9
		if strings.Contains(cardName, "Ultra Pro Puzzle") {
			return nil, mtgmatcher.ErrUnsupported
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
		if strings.HasPrefix(imgName, "UPP") && len(imgName) > 6 {
			maybeSet := strings.ToUpper(imgName[3:6])
			if maybeSet != "" {
				variant = maybeSet
			}
		}

	case "Deckmasters":
		variant = strings.TrimSpace(strings.Split(variant, "Deckmaster")[0])

	case "Unfinity":
		variant = strings.Replace(variant, ",", "/", -1)

	case "Conspiracy: Take the Crown":
		if cardName == "Kaya, Ghost Assassin" && variant == "Alternate Art Foil" {
			variant = "222"
		}
	}

	return &mtgmatcher.InputCard{
		Name:      cardName,
		Variation: variant,
		Edition:   edition,
		Foil:      isFoil,
	}, nil
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
	variant = strings.Replace(variant, "- ", "", -1)
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
	case strings.Contains(variant, "Magic Spotlight"):
		return "PSPL", ""
	case strings.Contains(variant, "Friday Night Magic Promo"):
		variant = "FNM"
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
		}
	case "Sakura-Tribe Elder":
		if variant == "Textless Victor Adame Minguez art" {
			edition = "PLG24"
		}
	case "Ephemerate":
		if variant == "Japanese Summer Vacation 2022 Promo" {
			edition = "PSVC"
		}
	case "Dragon's Hoard":
		if variant == "Tarkir: Dragonstorm Magic Academy Promo" {
			return "PW25", "1p"
		}
	case "Lightning Bolt":
		switch variant {
		case "MagicFest Promo textless":
			return "PF19", "1"
		case "Future Sight Frame 2025 MagicCon Atlanta Promo":
			return "PF25", "13"
		}
	case "Ugin, the Spirit Dragon":
		if variant == "Retro Frame 2025 MagicCon Las Vegas Promo" {
			return "PF25", "6"
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
	case "Portable Hole":
		return "AFR", "398"
	}
	return edition, variant
}

func PreprocessBuylist(card CSIPriceEntry) (*mtgmatcher.InputCard, error) {
	num := strings.TrimLeft(card.Number, "0")
	cleanVar := cleanVariant(card.Notes)
	edition := card.ItemSet
	isFoil := card.IsFoil == 1
	cardName := card.Name

	if mtgmatcher.Contains(cardName, "signed by") {
		return nil, mtgmatcher.ErrUnsupported
	}

	variant := num
	if variant == "" {
		variant = cleanVar
	}

	fixName := mtgmatcher.SplitVariants(cardName)
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

	// Skip tokens with the same names as cards
	if strings.Contains(variant, "Emblem") && !mtgmatcher.IsToken(cardName) {
		return nil, mtgmatcher.ErrUnsupported
	}

	switch edition {
	case "Coldsnap Theme Deck":
		if mtgmatcher.IsBasicLand(cardName) {
			return nil, mtgmatcher.ErrUnsupported
		}
	case "Zendikar", "Battle for Zendikar", "Oath of the Gatewatch":
		// Strip the extra letter from the name
		if mtgmatcher.IsBasicLand(cardName) {
			cardName = strings.Fields(cardName)[0]
		}
	case "Unstable":
		variant = cleanVar
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
		switch variant {
		case "Ravnica Weekend Promo":
			edition = variant
			variant = num
		case "Stained Glass Art":
			edition = "SLD"
			variant = num
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

	// Add previously removed/ignored tags
	for _, tag := range preserveTags {
		if strings.Contains(strings.ToLower(cleanVar), tag) && !strings.Contains(strings.ToLower(variant), tag) {
			if variant != "" {
				variant += " "
			}
			variant += tag
		}
	}

	return &mtgmatcher.InputCard{
		Name:      cardName,
		Variation: variant,
		Edition:   edition,
		Foil:      isFoil,
	}, nil
}

func Preprocess(card CSICard) (*mtgmatcher.InputCard, error) {
	cardName := card.Name
	variant := card.Variation
	edition := card.Edition

	if mtgmatcher.Contains(cardName, "Signed by") {
		return nil, errors.New("not singles")
	}

	fields := mtgmatcher.SplitVariants(cardName)
	cardName = fields[0]
	if len(fields) > 1 {
		if variant != "" {
			variant += " "
		}
		variant += strings.Join(fields[1:], " ")
	}

	switch edition {
	case "Online Arena":
		return nil, errors.New("not supported")
	case "Black Bordered (foreign)":
		switch variant {
		case "German", "French", "Spanish", "Chinese", "Korean":
			return nil, errors.New("not supported")
		case "Italian":
			edition = "FBB"
		case "Japanese":
			edition = "4BB"
		}
	case "Ikoria: Lair of Behemoths: Variants":
		if variant == "Japanese" {
			switch cardName {
			case "Dirge Bat", "Mysterious Egg", "Crystalline Giant":
				variant += " Godzilla"
			default:
				return nil, errors.New("not supported")
			}
		}
	case "Prerelease Promo":
		switch cardName {
		case "On Serra's Wings":
			return nil, errors.New("does not exist")
		}
	case "Portal 3 Kingdoms":
		if variant == "Japanese" || variant == "Chinese" {
			return nil, errors.New("not english")
		}
	case "Mystical Archive", "Double Masters: Variants":
		variant = strings.Replace(variant, "Showcase Frame", "", 1)
	case "Dominaria United: Variants":
		if variant == "Stained Glass Frame" {
			variant = "Showcase"
		}
	}

	return &mtgmatcher.InputCard{
		Id:        card.ScryfallId,
		Name:      cardName,
		Variation: variant,
		Edition:   edition,
		Foil:      card.IsFoil,
	}, nil
}
