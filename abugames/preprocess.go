package abugames

import (
	"errors"
	"strings"

	"github.com/mtgban/go-mtgban/mtgmatcher"
)

var cardTable = map[string]string{
	// Typos
	"Bogart Brute":                        "Boggart Brute",
	"Deathgazer Cockatrice":               "Deathgaze Cockatrice",
	"Elminate":                            "Eliminate",
	"Fireblade Artist Ravnica Allegiance": "Fireblade Artist",
	"Mindblade Rendor":                    "Mindblade Render",
	"Neglected Hierloom":                  "Neglected Heirloom",
	"Rathi Berserker":                     "Aerathi Berserker",
	"Smelt and Herd and Saw":              "Smelt // Herd // Saw",
	"Soulmemder":                          "Soulmender",
	"Svagthos, the Restless Tomb":         "Svogthos, the Restless Tomb",
	"Trial and Error":                     "Trial // Error",
	"Simic Signat":                        "Simic Signet",
	"Specmen 73":                          "Specimen 73",
	"Zilortha, Strength Incarnated":       "Zilortha, Strength Incarnate",
	"Makindi Siderunner":                  "Makindi Sliderunner",
	"Anti-Venom, Horrifying Hero":         "Anti-Venom, Horrifying Healer",

	"The Emperor of Palamecia /The Lord Master of Hell": "The Emperor of Palamecia",

	// Funny cards
	"No Name":                         "_____",
	"Absolute Longest Card Name Ever": mtgmatcher.LongestCardEver,
}

var promoTags = []string{
	"Alternate Art Duelist",
	"Arena",
	"Book",
	"Buy-a-Box",
	"Convention",
	"Draft Weekend",
	"FNM",
	"Into Pack",
	"Judge",
	"Launch",
	"OPEN HOUSE FULL ART",
	"Open House",
	"Planeswalker Weekend",
	"Prerelease",
	"Preview",
	"Promo",
	"Release",
	"SDCC",
	"Store Championship",
	"TopDeck Magazine",
}

func preprocess(card *ABUCard) (*mtgmatcher.InputCard, error) {
	lang := ""
	if len(card.Language) > 0 {
		lang = card.Language[0]
	}

	// Non-Singles magic cards
	switch card.Layout {
	case "Scheme", "Plane", "Phenomenon":
		return nil, errors.New("non-single card")
	}

	// Non-existing cards
	switch card.DisplayTitle {
	case "Silent Submersible (Promo Pack)",
		"Silent Submersible (Promo Pack) - FOIL",
		"Hymn to Tourach (B - Mark Justice - 1996)",
		"Skyclave Shade (Extended Art)",
		"Mountain (6th Edition 343 - Mark Le Pine - 1999)":
		return nil, errors.New("untracked card")
	}
	switch card.Id {
	case "1604919", "1604921", "1604922", // Living Twister
		"1604802", "1604801", "1604799": // Commence the Endgame
		return nil, errors.New("duplicated card")
	}

	isFoil := strings.Contains(strings.ToLower(card.DisplayTitle), " foil") ||
		strings.Contains(strings.ToLower(card.DisplayTitle), " - fol") // SS3 Pyroblast

	edition := card.Edition
	title := card.DisplayTitle
	if title == "" {
		title = card.SimpleTitle
	}

	// Split by -, rebuild the cardname in a standardized way
	variation := ""
	vars := strings.Split(title, " - ")
	cardName := vars[0]
	if len(vars) > 1 {
		if vars[len(vars)-1] == edition {
			vars = vars[:len(vars)-1]
		}

		variation = strings.Join(vars[1:], " ")

		// Fix some untagged prerelease cards
		// Nahiri's Wrath, Tendershoot Dryad
		if strings.Contains(variation, edition+" FOIL") {
			variation = strings.Replace(variation, edition+" FOIL", "Prerelease", 1)
		}
	}

	// Split by ()
	vars = mtgmatcher.SplitVariants(cardName)
	cardName = vars[0]
	if len(vars) > 1 {
		oldVariation := variation
		variation = strings.Join(vars[1:], " ")
		if oldVariation != "" {
			variation += " " + oldVariation
		}
	}

	// Separate flavor names
	if strings.Contains(cardName, " | ") {
		cardName = strings.Split(cardName, " | ")[0]
	}

	// Cleanup variation as necessary
	if variation != "" {
		variation = strings.Replace(variation, "FOIL", "", -1)

		variation = strings.Replace(variation, "(", "", -1)
		variation = strings.Replace(variation, ")", "", -1)

		variation = strings.Replace(variation, "Not Tournament Legal", "", 1)

		variation = strings.TrimSpace(variation)

		isPromo := false
		for _, tag := range promoTags {
			if strings.Contains(variation, tag) {
				isPromo = true
				break
			}
		}
		if isPromo {
			// Handle promo cards appearing in multiple editions
			// like Sorcerous Spyglass
			if strings.Contains(variation, "Promo Pack") || strings.Contains(variation, "Prerelease") {
				variation += " " + edition
			}
			// Reset edition, and trust mtgmatcher to find it by its variation
			edition = "Promo"
		}
	}

	switch edition {
	case "":
		if mtgmatcher.IsBasicLand(cardName) {
			card.Edition = "GK2"
		} else {
			return nil, errors.New("missing edition")
		}
	case "Promo":
		switch cardName {
		case "Skirk Marauder":
			edition = "Arena League 2003"
		case "Damnation(Secret Lair":
			cardName = "Damnation"
			edition = "SLD"
		case "Elvish Aberration":
			if variation == "FNM" {
				variation = "Arena"
			}
		case "Elvish Lyrist":
			if variation == "FNM" {
				variation = "JSS Foil"
			}
		case "Island":
			if variation == "Arena 1999 No Symbol Promo" {
				variation = "Arena 1999 misprint"
			}
		case "Stocking Tiger":
			if variation == "Target" {
				variation = "misprint"
			}
		case "Psychatog":
			if variation == "FNM" {
				variation = "Textless"
			}
		case "Sol Ring":
			if variation == "Commander" {
				variation = "MagicFest 2019"
			}
		case "Mountain":
			if variation == "APAC a Phillippines" {
				variation = "APAC a Philippines"
			}
		case "Beast of Burden":
			if variation == "Prerelease No Expansion Symbol FOIL" {
				variation = "Prerelease misprint"
			}
		case "Mechagodzilla, Battle Fortress / Hangarback Walker":
			if variation == "Welcome Back" {
				cardName = "Hangarback Walker"
				edition = "PLG20"
			}
		case "Serra Angel":
			if strings.Contains(variation, "25th Anniversary") {
				edition = "PDOM"
			}
		case "Hidetsugu, Devouring Chaos":
			edition = "NEO"
		case "Rafiq of the Many":
			edition = "SHA"
			variation = "250"
		case "Swiftfoot Boots":
			if strings.Contains(variation, "Launch") {
				edition = "PW22"
				variation = "4"
			}
		case "Brood Sliver":
			edition = "SLD"
		case "Lavinia, Azorius Renegade":
			edition = "PRNA"
			variation = "189"
		}
		if strings.Contains(variation, "Secret") || strings.Contains(variation, "Lair") {
			num := mtgmatcher.ExtractNumber(variation)
			if num != "" {
				variation = num
			} else if strings.Contains(variation, "Seb McKinnon") {
				variation = "119"
			}
			edition = "Secret Lair Drop"
		} else if mtgmatcher.IsBasicLand(cardName) && strings.Contains(variation, "Full-Text") {
			edition = "SLD"
			variation = strings.TrimPrefix(variation, "Full-Text ")
		} else if strings.Contains(variation, "Play Promo") {
			variation = strings.Replace(variation, "FNM", "", 1)
		} else if card.Layout == "Planar" {
			edition = "Planechase Promos"
		} else if variation == "Preview" {
			if len(mtgmatcher.MatchInSet(cardName, "MGB")) > 0 {
				edition = "MGB"
			}
		}
	case "Secret Lair", "Secret Lair Drop":
		edition = "Secret Lair Drop"
		if len(mtgmatcher.MatchInSetNumber(cardName, "SLC", card.Number)) > 0 {
			edition = "SLC"
			variation = card.Number
		} else {
			// Check if variation contains a number as it's usually more
			// accurate. If not, check the card.Number property (yolo).
			// ExtractNumberAny (not the year-capped ExtractNumber) so a Secret
			// Lair collector number >= 1993 (e.g. 7010) is recognized and not
			// clobbered by a stale card.Number, while still ignoring dates and
			// ordinals.
			num := mtgmatcher.ExtractNumberAny(variation)
			if num == "" && card.Number != "" {
				variation += " " + card.Number
			}
		}
	case "Anthologies":
		if cardName == "Mountain" {
			if variation == "A" {
				variation = "B"
			} else if variation == "B" {
				variation = "A"
			}
		}
	case "World Championship":
		if cardName == "City of Brass" {
			if variation == "Leon Lindback 1996" {
				variation = "Sideboard Leon Lindback 1996"
			}
		}
	case "Ikoria: Lair of Behemoths":
		if strings.Contains(cardName, " / ") {
			cardName = card.SimpleTitle
			variation = "Godzilla"
		}
	case "Oath of the Gatewatch":
		if cardName == "Captain's Claws" && variation == "Goldnight Castigator Shadow FOIL" {
			variation = "misprint"
		}
	case "Core Set 2020 / M20":
		if cardName == "Corpse Knight" && variation == "2/3" {
			variation = "misprint"
		}
	case "Mystery Booster":
		if cardName == "Trial and Error" {
			// Hack to prevent aliasing wiith the real "Trial // Error"
			cardName = "Trial and Error "
		}
	case "Summer Magic / Edgar":
		if mtgmatcher.IsBasicLand(cardName) {
			return nil, errors.New("unsupported")
		}
	case "Streets of New Capenna Commander":
		if strings.Contains(cardName, "Spellbinding Soprano") && strings.Contains(cardName, "The List") {
			cardName = "Spellbinding Soprano"
			variation = "Promo Pack"
		}
	case "Fourth Edition Foreign Black Border":
		variation = strings.TrimSuffix(variation, " Japanese")
		variation = strings.TrimSuffix(variation, " BB")
		variations := strings.Fields(variation)
		if len(variations) > 1 && len(variations[0]) == 1 {
			variation = strings.Join(variations[1:], " ")
		}
	case "Fallen Empires":
		variations := strings.Fields(variation)
		if len(variations) > 1 && len(variations[0]) == 1 {
			variation = strings.Join(variations[1:], " ")
		}
	case "Modern Horizons 2 Timeshifts":
		edition = "MH2"
	// This set has some non standard way to tag foil, but luckily variations are all on different CNs
	case "Edge of Eternities: Stellar Sights":
		variation = card.Number
	case "Aetherdrift":
		if variation == "Borderless" {
			variation += " " + card.Number
		}
	case "Final Fantasy Commander":
		variation += " " + card.Number
	}

	// Either Promo or "European Land Program"
	if strings.Contains(variation, "Scandanavia") {
		variation = strings.Replace(variation, "Scandanavia", "Scandinavia", 1)
	} else if strings.Contains(variation, "Phillippines") {
		variation = strings.Replace(variation, "Phillippines", "Philippines", 1)
	}

	// Use collector number data when the variation carries has none, unless for a couple of editions
	if card.Number != "" && mtgmatcher.ExtractNumberAny(variation) == "" {
		switch edition {
		case "Unfinity", "Promo":
		default:
			if variation != "" {
				variation += " "
			}
			variation += card.Number
		}
	}
	// Restore canonical name for split cards
	if strings.Contains(cardName, " / ") {
		cardName = strings.Replace(cardName, " / ", " // ", -1)
	}
	if strings.Contains(cardName, " // ") {
		cardName = strings.Split(cardName, " // ")[0]
		cardName = strings.TrimSpace(cardName)
	}

	name, found := cardTable[cardName]
	if found {
		cardName = name
	}

	// Stash the language information (filtered earlier)
	if lang != "" && lang != "English" {
		if variation != "" {
			variation += " "
		}
		variation += lang
	}

	return &mtgmatcher.InputCard{
		Name:      cardName,
		Variation: variation,
		Edition:   edition,
		Foil:      isFoil,
		Language:  lang,
	}, nil
}
