package tcgplayer

import (
	"errors"
	"strings"

	"github.com/mtgban/go-mtgban/mtgmatcher"
	"github.com/mtgban/go-tcgplayer"
)

var tokenIds = map[int]string{
	78614:  "MPR",
	78617:  "MPR",
	78618:  "MPR",
	78621:  "MPR",
	78632:  "MPR",
	78622:  "PR2",
	78631:  "PR2",
	78636:  "PR2",
	78615:  "P03",
	78623:  "P03",
	78630:  "P03",
	78613:  "P04",
	78633:  "P04",
	78417:  "L12",
	78444:  "L13",
	78526:  "L13",
	82612:  "L13",
	108434: "L14",
	108436: "L13",
	108437: "L14",
}

var cardIds = map[int]string{
	// Serra Angel
	284951: "P30A",
	284923: "P30H",
	284921: "P30H",
	// Shivan Dragon
	515925: "P30T",
	284937: "P30H",
	284939: "P30H",
}

func Preprocess(product *tcgplayer.Product, editions map[int]string) (*mtgmatcher.InputCard, error) {
	cardName, variant := GetProductNameAndVariant(product)

	number := GetProductNumber(product)
	edition := editions[product.GroupId]

	// Unsupported cards depending on their variant
	switch cardName {
	case "Bruna, Light of Alabaster":
		if variant == "Commander 2018" {
			return nil, errors.New("does not exist")
		}
	case "Glissa, the Traitor":
		if variant == "Mirrodin Besieged" {
			return nil, errors.New("untracked")
		}
	case "Elvish Vanguard":
		if strings.Contains(variant, "Spanish") ||
			strings.Contains(variant, "French") ||
			strings.Contains(variant, "Italian") {
			return nil, errors.New("non english")
		}
	case "Tamiyo's Journal":
		if edition == "Shadows over Innistrad" {
			_, found := mtgmatcher.VariantsTable[edition][variant]
			if !found {
				return nil, errors.New("non english")
			}
		}
	case "Tarmogoyf":
		if variant == "JP Exclusive" {
			variant = ""
		}
	case "Scrounging Deathclaw":
		cardName = "Tarmogoyf"
	default:
		if strings.Contains(variant, "JP Amazon Exclusive") ||
			strings.Contains(variant, "SEA Exclusive") ||
			strings.Contains(variant, "JP WonderGOO Exclusive") ||
			strings.Contains(variant, "JP Hareruya Exclusive") {
			return nil, errors.New("unofficial")
		} else if isToken(product) && strings.Contains(product.CleanName, "Double") {
			return nil, errors.New("duplicate")
		} else if strings.Contains(edition, "Tales of Middle-earth") && strings.HasSuffix(cardName, "Scene") {
			return nil, errors.New("unsupported")
		}
	}

	ogVariant := variant
	switch edition {
	case "Portal":
		switch cardName {
		case "Blaze":
			switch variant {
			case "CS Alternate Art":
				variant = "118†s"
			case "Starter Deck CS Alternate Art":
				variant = "118s"
			case "Flavor Text":
				variant = "118"
			default:
				variant = "118†"
			}
		case "Warrior's Charge",
			"Raging Goblin",
			"Anaconda",
			"Elite Cat Warrior",
			"Monstrous Growth":
			if variant == "Flavor Text" {
				variant = ""
			} else if variant == "" {
				variant = "Reminder Text"
			}
		case "Hand of Death":
			// Variants are fine as is
		case "Armored Pegasus",
			"Bull Hippo",
			"Cloud Pirates",
			"Feral Shadow",
			"Snapping Drake",
			"Storm Crow":
			if variant == "Reminder Text" {
				edition = "Portal Demo Game"
			}
		}
	case "Alliances",
		"Portal Second Age":
		if variant == "" {
			variant = number
			// Missing everything
			if cardName == "Awesome Presence" {
				variant = "arms spread"
			}
		}
	case "Fallen Empires":
		if variant == "" {
			fixup, found := map[string]string{
				"Armor Thrull":           "33a",
				"Basal Thrull":           "34a",
				"Elven Fortress":         "65a",
				"Elvish Hunter":          "67a",
				"Elvish Scout":           "68a",
				"Farrel's Zealot":        "3a",
				"Goblin Chirurgeon":      "54a",
				"High Tide":              "18a",
				"Homarid":                "19a",
				"Icatian Infantry":       "7a",
				"Icatian Moneychanger":   "10a",
				"Merseine":               "23a",
				"Orcish Veteran":         "62a",
				"Order of Leitbur":       "16c",
				"Order of the Ebon Hand": "42a",
				"Spore Cloud":            "72a",
				"Thorn Thallid":          "80a",
				"Tidal Flats":            "27a",
				"Vodalian Mage":          "30a",
			}[cardName]
			if found {
				variant = fixup
			}
		}
	case "Homelands":
		num := number
		fixup, found := map[string]string{
			"Abbey Matron":    "2",
			"Aliban's Tower":  "61",
			"Ambush Party":    "63",
			"Anaba Bodyguard": "66",
			"Anaba Shaman":    "67",
			"Carapace":        "84",
			"Cemetery Gate":   "44",
			"Reef Pirates":    "",
		}[cardName]
		if found {
			num = fixup
		}
		if variant == "" {
			variant = num + "a"
		} else if variant == "Version 2" {
			variant = num + "b"
		}
	case "Media Promos":
		switch cardName {
		case "Nalathni Dragon":
			if variant == "Redemption Program" {
				edition = variant
			}
		case "Duress":
			if variant == "IDW Comics 2014" {
				edition = "PIDW"
			}
		case "Stocking Tiger":
			variant = "misprint"
		case "Sword of Dungeons & Dragons":
			edition = "H17"
		case "Llanowar Elves":
			edition = "PDMU"
		case "Sleight of Hand", "Lay Down Arms", "Cut Down":
			edition = "PLG24"
		case "Anti-Venom, Horrifying Healer", "Spectacular Spider-Man",
			"Colonel John Jameson", "Iron Spider, Stark Upgrade":
			edition = "LMAR"
		case "Sokka, Bold Boomeranger":
			edition = "PF25"
		default:
			if strings.Contains(variant, "SDCC") {
				edition = "SDCC"
			}
		}
	case "Play Promos":
		switch cardName {
		case "Spider-Ham, Peter Porker", "Carnage, Crimson Chaos", "Mary Jane Watson":
			edition = "PW25"
		case "Spectacular Spider-Man":
			edition = "PSPL"
		}
	case "Prerelease Cards":
		switch cardName {
		case "Lu Bu, Master-at-Arms":
			if variant == "Japan 4/29/99" {
				variant = "April"
			} else if variant == "Singapore 7/4/99" {
				variant = "July"
			}
		case "Beast of Burden":
			if variant == "No Date" {
				variant = "misprint"
			}
		}
	case "Junior Series Promos":
		if variant == "Japan Junior Tournament" {
			edition = "PJJT"
		} else {
			switch cardName {
			case "Sakura-Tribe Elder":
				switch variant {
				case "Super Series":
					edition = "PSUS"
				case "Junior Super Series":
					edition = "PJSE"
				}
			case "Royal Assassin":
			default:
				if len(mtgmatcher.MatchInSet(cardName, "PSUS")) == 1 {
					edition = "PSUS"
				}
			}
		}
	case "Judge Promos":
		switch cardName {
		case "Demonic Tutor":
			if variant == "" {
				variant = "2008"
			} else if variant == "J20" {
				variant = "2020"
			}
		case "Vampiric Tutor":
			if variant == "" {
				variant = "2000"
			} else if variant == "J18" {
				variant = "2018"
			}
		case "Wasteland":
			if variant == "" {
				variant = "2010"
			}
		}
	case "Unique and Miscellaneous Promos":
		for _, ext := range product.ExtendedData {
			if ext.Name == "OracleText" && strings.Contains(ext.Value, "Heroes of the Realm") {
				edition = "Heroes of the Realm"
				break
			}
		}

		switch cardName {
		case "Fiendish Duo":
			edition = "PKHM"
		case "Arasta of the Endless Web":
			edition = "THB"
			variant = "352"
		case "Archmage Emeritus":
			edition = "STX"
			variant = "377"
		case "Yusri, Fortune's Flame":
			edition = "MH2"
			variant = "492"
		case "Sigarda's Summons":
			edition = "VOW"
			variant = "404"
		case "Serra Angel":
			if variant == "" {
				edition = "PWOS"
			} else if variant == "25th Anniversary Exposition" {
				edition = "PDOM"
			}
		case "Moraug, Fury of Akoum", "Ox of Agonas", "Angrath, the Flame-Chained", "Tahngarth, First Mate":
			if strings.Contains(variant, "JP Exclusive") {
				return nil, errors.New("non-english")
			}
		case "Yuriko, the Tiger's Shadow":
			if variant == "" {
				edition = "PL22"
			}
		case "Relentless Rats":
			if strings.Contains(variant, "Italian") {
				return nil, errors.New("non-english")
			}
		case "Lotus Petal":
			edition = "P30M"
		case "Gala Greeters":
			if variant == "English" {
				edition = "SNC"
				variant = "450"
			}
		case "Tishana's Tidebinder", "Brotherhood's End":
			if variant == "JP Exclusive" {
				edition = "pjsc"
				variant = ""
			}
		case "Ultimecia, Temporal Threat", "Lightning, Security Sergeant",
			"Cloud, Planet's Champion", "Sephiroth, Planet's Heir", "Kaalia of the Vast":
			if variant == "Costco Bundle" {
				edition = "PMEI"
			}
		case "Peter Parker":
			edition = "PMEI"
		case "J. Jonah Jameson":
			edition = "PF25"
		case "Iroh, Grand Lotus":
			edition = "PMEI"
		case "Lightning Bolt":
			edition = "PW26"
		default:
			if variant == "JP Exclusive Summer Vacation" && len(mtgmatcher.MatchInSet(cardName, "PL21")) == 0 {
				edition = "PSVC"
			} else if isToken(product) && strings.Contains(variant, "JP") && strings.Contains(variant, "Exclusive") {
				edition = "WDMU"
			}
		}
	case "Pro Tour Promos":
		switch cardName {
		case "Char":
			edition = "P15A"
		case "Gideon, Ally of Zendikar", "Selfless Spirit", "Thraben Inspector":
			edition = "PRCQ"
		case "Thing in the Ice":
			edition = "PR23"
		case "Snapcaster Mage":
			if variant == "Regional Championship Qualifiers 2023" {
				edition = variant
			}
		case "Sauron, the Dark Lord":
			edition = "LTR"
			variant = "301"
		case "Gandalf the White":
			edition = "LTR"
			variant = "299"
		default:
			for _, code := range []string{"PR23", "SLP"} {
				cards := mtgmatcher.MatchInSet(cardName, code)
				if len(cards) > 0 {
					edition = cards[0].SetCode
					variant = cards[0].Number
					break
				}
			}
		}
	case "Battle for Zendikar",
		"Oath of the Gatewatch":
		if mtgmatcher.IsBasicLand(cardName) {
			if cardName == "Wastes" && variant == "" {
				variant = "183a"
			} else if !strings.HasSuffix(product.CleanName, "Full Art") {
				variant += "a"
			}
		}
	case "Standard Showdown Promos":
		switch variant {
		case "Rebecca Guay":
			edition = "PSS2"
		case "Alayna Danner", "Alayna Danner 2018":
			edition = "PSS3"
		case "Samuele Bandini", "Piotr Dura", "Alayna Danner 2024", "Lucas Graciano", "Alexander Forssberg":
			edition = "PSS4"
		case "Year of the Dragon 2024":
			edition = "PL24"
		case "Year of the Snake 2025":
			edition = "PL25"
		default:
			edition = "PSS1"

			for _, tag := range []string{
				"PCBB", "PSS5",
			} {
				if len(mtgmatcher.MatchInSet(cardName, tag)) > 0 {
					edition = tag
				}
			}
		}
	case "World Championship Decks":
		// Typo
		variant = strings.Replace(variant, "SD", "SB", 1)

		// These do not exist
		if (cardName == "Red Elemental Blast" && variant == "1996 George Baxter 4ED") ||
			(cardName == "Island" && variant == "2002 Raphael Levy 7ED 337") {
			return nil, errors.New("does not exist")
		}
		// Try parsing the rightmost portion of the data, by looking up
		// any recurring tag available in the VariantsTable
		for _, code := range []string{"FEM", "4ED", "TMP"} {
			if strings.Contains(variant, code) {
				fields := mtgmatcher.Cut(variant, code)
				// Clean up by removing the set code and the option sideboard tag
				vars := strings.TrimPrefix(fields[1], code)
				vars = strings.TrimSpace(vars)
				vars = strings.TrimPrefix(vars, "SB")
				vars = strings.TrimSpace(vars)
				vars = strings.ToLower(vars)

				if vars != "" {
					set, _ := mtgmatcher.GetSet(code)

					tag := mtgmatcher.VariantsTable[set.Name][cardName][vars]
					if tag != "" {
						variant += " " + tag
					}
				}
				break
			}
		}
	case "Launch Party & Release Event Promos":
		if mtgmatcher.IsBasicLand(cardName) {
			edition = "Ravnica Weekend"
		}
		switch cardName {
		case "Shivan Dragon":
			if variant == "Moscow 2005" {
				edition = "P9ED"
			}
		}
	case "Renaissance":
		if strings.Contains(variant, "French") || strings.Contains(variant, "German") {
			return nil, errors.New("non english")
		}
		if strings.HasSuffix(variant, "Italian") {
			edition = "Rinascimento"
			variant = strings.TrimSuffix(variant, " Italian")
			variants := strings.Split(variant, "\" ")
			if len(variants) > 1 {
				variant = variants[1]
			}
		}
	case "WMCQ Promo Cards":
		switch cardName {
		case "Arcbound Ravager":
			edition = "PPRO"
		}
	case "Kaladesh":
		if variant == "17/264" || variant == "Reprint" {
			variant = "Intro Pack"
		}
	case "Secret Lair Drop Series":
		variant = number
		switch cardName {
		case "Plains // Battlefield Forge":
			cardName = "Battlefield Forge"
			variant = "669"
		case "Hadoken":
			cardName = "Lightning Bolt"
			variant = "675"
		case "Zndrsplt, Eye of Wisdom", "Okaun, Eye of Chaos":
			variant = ogVariant
		case "Counterspell":
			if ogVariant == "SL PLAYTEST" {
				variant = "SCTLR"
			} else if ogVariant == "0002" {
				edition = "PURL"
				variant = "9"
			}
		default:
			// Preserve all etched/galaxy/rainbow foil properties
			if strings.Contains(ogVariant, "Foil") {
				variant += " " + ogVariant
			}
		}
	case "AFR Ampersand Promos":
		variant = number + "a"
	case "WPN & Gateway Promos":
		switch cardName {
		case "Mind Stone":
			edition = "DCI"
			if variant == "2021" {
				edition = "PW21"
			}
		case "Orb of Dragonkind":
			variant = "J" + number
		}
	case "Game Day & Store Championship Promos":
		// Using Contains since reskinned cards might have the original card name there
		if strings.Contains(variant, "Winner") || strings.Contains(variant, "Top 8") {
			return nil, errors.New("untracked")
		}
	case "Special Occasion":
		if len(mtgmatcher.MatchInSet(cardName, "PCEL")) == 1 {
			edition = "PCEL"
		} else if len(mtgmatcher.MatchInSet(cardName, "HHO")) == 1 {
			edition = "HHO"
		} else {
			return nil, errors.New("untracked")
		}
	case "Unfinity":
		// Skip attractions, number is incorrect
		if !strings.Contains(variant, "-") {
			variant = number
		}
	case "Murders at Karlov Manor":
		num := number

		if num != "" && variant != "a" && variant != "b" {
			variant = num
		}
	case "The List Reprints":
		variant = number + " " + variant
		if variant == "50 Full Art" {
			variant = "Game Day"
		}
	case "Kaldheim":
		if cardName == "Harald, King of Skemfar" && variant == "JP Exclusive" {
			edition = "PMEI"
			variant = ""
		}
	case "MagicFest Cards":
		switch cardName {
		case "Lightning Bolt":
			edition = "PF19"
			if variant == "Future Sight" {
				edition = "PF25"
			}
		case "Arcane Signet":
			vars, found := mtgmatcher.VariantsTable["30th Anniversary Misc Promos"]["Arcane Signet"][strings.ToLower(variant)]
			if found {
				variant = vars
				edition = "P30M"
			}
		case "Counterspell":
			edition = "PF24"
			if variant == "Retro Frame" {
				edition = "PF26"
			}
		case "The First Sliver", "Serra the Benevolent", "Ponder",
			"Ugin, the Spirit Dragon", "Sliver Hive",
			"The Ur-Dragon", "Scourge of Valkas":
			cards := mtgmatcher.MatchInSet(cardName, "PF25")
			if len(cards) > 0 && (number == "3" || number == "2" || number == "1") {
				edition = "PF25"
				variant = cards[0].Number
			}
		}
	case "Fourth Edition",
		"Revised Edition",
		"Mirage",
		"Tempest",
		"Planechase 2012",
		"Anthologies",
		"Deckmasters Garfield vs Finkel",
		"Duel Decks: Anthology",
		"International Edition",
		"Collector's Edition",
		"Duel Decks: Jace vs. Chandra",
		"Revised Edition (Foreign Black Border)",
		"Fourth Edition (Foreign Black Border)",
		"Unstable",
		"Shadows over Innistrad",
		"War of the Spark",
		"Modern Horizons",
		"30th Anniversary Promos",
		"Universes Beyond: Warhammer 40,000",
		"The Brothers' War: Retro Frame Artifacts",
		"Mystery Booster Cards",
		"Planeshift",
		"": // cosmetic
		// Variants are fine as is
	default:
		num := number

		if num != "" && mtgmatcher.ExtractYear(variant) == "" {
			variant = num
		}
	}

	// Override any complex cases
	ed, found := cardIds[product.ProductId]
	if found {
		edition = ed
	}

	if isToken(product) && edition != "Unfinity" {
		// Strip pw/tou numbers that could be misinterpreted as numbers
		if strings.Contains(variant, "/") {
			variant = ""
		}
		// If number is available, use it as it's usually accurate
		num := number
		if num != "" {
			variant = num
		}
		// Decouple
		ed, found := tokenIds[product.ProductId]
		if found {
			edition = ed
		}

		if edition == "L13" {
			if product.ProductId == 82612 {
				variant = "4"
			} else if product.ProductId == 78444 {
				variant = "1"
			}
		}

		// Skip double-faced token cards by checking if there are
		// multiple collector numbers reported
		// We cannot trust the name exclusively since it can get corrected
		rawNum := RawProductNumber(product)
		if strings.Contains(rawNum, "//") {
			return nil, errors.New("duplicate")
		}
	}

	// Special tags and finishes
	for _, tag := range []string{"misprint", "serial", "etched", "retro frame"} {
		if strings.Contains(strings.ToLower(ogVariant), tag) && !strings.Contains(strings.ToLower(variant), tag) {
			if variant != "" {
				variant += " "
			}
			variant += tag
		}
	}
	// Need to erase any previously added numbers number that may confuse the assignment
	if strings.Contains(strings.ToLower(ogVariant), "display commander") {
		variant = ogVariant
	}

	isFoil := strings.Contains(ogVariant, "Foil")

	card := mtgmatcher.InputCard{
		Name:      cardName,
		Edition:   edition,
		Variation: variant,
		Foil:      isFoil,
	}

	return &card, nil
}
