package mtgmatcher

import (
	"slices"
	"strconv"
	"strings"
)

// Remove any unrelated edition from the input array.
func filterPrintings(inCard *InputCard, editions []string) (printings []string) {
	maybeYear := ExtractYear(inCard.Variation)
	if maybeYear == "" {
		maybeYear = ExtractYear(inCard.Edition)
	}

	for _, setCode := range editions {
		set, found := defaultBackend.Sets[setCode]
		if !found {
			continue
		}

		setDate := set.ReleaseDateTime

		switch {
		// If the edition matches, use it as is
		// except for two "catch all" sometimes overlapping sets
		case Equals(inCard.Edition, set.Name) && !inCard.isMysteryList() && !inCard.isSecretLair():
			// pass-through

		case inCard.isPrerelease():
			switch set.Name {
			// Sets that could be marked as prerelease, but they aren't really
			case "M15 Prerelease Challenge",
				"Open the Helvault":
			// Sets that have prerelease cards mixed in
			case "Innistrad: Double Feature",
				"March of the Machine Commander",
				"The Lord of the Rings: Tales of Middle-earth":
				skip := true
				foundCards := MatchInSet(inCard.Name, setCode)
				for _, card := range foundCards {
					if card.HasPromoType(PromoTypePrerelease) {
						skip = false
						break
					}
				}
				if skip {
					continue
				}
			case "Duels of the Planeswalkers 2012 Promos",
				"Grand Prix Promos",
				"Pro Tour Promos",
				"World Championship Promos":
				continue
			default:
				if strings.HasPrefix(set.Name, "30th Anniversary") {
					continue
				}
				if !strings.HasSuffix(set.Name, "Promos") {
					continue
				}
			}

		case inCard.isPromoPack():
			switch set.Name {
			case "Dragon's Maze Promos", // due to Plains
				"Grand Prix Promos":
				continue
			case "M20 Promo Packs":
			default:
				switch {
				case strings.HasPrefix(set.Name, "30th Anniversary"):
					continue
				case strings.HasSuffix(set.Name, "Promos"):
				case setDate.After(PromosForEverybodyYay) && (set.Type == "expansion" || set.Type == "core"):
					skip := true
					foundCards := MatchInSet(inCard.Name, setCode)
					for _, card := range foundCards {
						if card.HasPromoType(PromoTypePromoPack) || card.HasPromoType(PromoTypePlayPromo) {
							skip = false
							break
						}
					}
					if skip {
						continue
					}
				default:
					continue
				}
			}

		case inCard.isRelease():
			skip := true
			foundCards := MatchInSet(inCard.Name, setCode)
			for _, card := range foundCards {
				if card.HasPromoType(PromoTypeRelease) ||
					card.HasPromoType(PromoTypeDraftWeekend) ||
					card.HasPromoType(PromoTypeWPN) {
					skip = false
					break
				}
			}
			if skip {
				continue
			}

		case inCard.isBaB():
			skip := true
			foundCards := MatchInSet(inCard.Name, setCode)
			for _, card := range foundCards {
				if card.HasPromoType(PromoTypeBuyABox) {
					skip = false
					break
				}
			}
			if skip {
				continue
			}

		case inCard.isBundle():
			skip := true
			foundCards := MatchInSet(inCard.Name, setCode)
			for _, card := range foundCards {
				if card.HasPromoType(PromoTypeBundle) {
					skip = false
					break
				}
			}
			if skip {
				continue
			}

		case inCard.isFNM():
			switch {
			case strings.HasPrefix(set.Name, "Friday Night Magic "+maybeYear):
			case set.Name == "Magic × Duel Masters Promos":
			case strings.HasSuffix(set.Name, "Promos"):
				skip := true
				foundCards := MatchInSet(inCard.Name, setCode)
				for _, card := range foundCards {
					if card.HasPromoType(PromoTypeFNM) {
						inCard.Variation = "FNM Promo"
						skip = false
						break
					}
				}
				if skip {
					continue
				}
			default:
				continue
			}

		case inCard.isJudge():
			switch {
			case strings.HasPrefix(set.Name, "Judge Gift Cards "+maybeYear):
			default:
				continue
			}

		case inCard.isArena():
			maybeYear = inCard.arenaYear(maybeYear)
			switch {
			case set.Name == "DCI Legend Membership":
			case strings.HasPrefix(set.Name, "Arena League "+maybeYear):
			default:
				continue
			}

		// This needs to be above any possible printing type below
		// Both kinds need to be checked in the same place as there is
		// a lot of overlap in the product and naming across stores
		case inCard.isMysteryList() || inCard.isSecretLair():
			noSymbol := inCard.Contains("No") && inCard.Contains("Symbol")
			switch set.Code {
			case "CMB1":
				if noSymbol || strings.Contains(inCard.Variation, "V.2") {
					continue
				}
			case "CMB2":
				if !noSymbol && !strings.Contains(inCard.Variation, "V.2") {
					continue
				}
			case "MB2":
				if !inCard.Contains("Mystery Booster 2") {
					continue
				}
			case "PLST":
				// Check if there is an exact match in plain SLD
				num := ExtractNumber(inCard.Variation)
				if len(MatchInSetNumber(inCard.Name, "SLD", num)) != 0 {
					// If there is a match, make sure there are no other cards in PLST with the same number
					shouldNotContinue := false
					cardsWithSameName := MatchInSet(inCard.Name, "PLST")
					for _, altCard := range cardsWithSameName {
						var altNum string
						altNums := strings.Split(altCard.Number, "-")
						if len(altNums) > 1 {
							altNum = altNums[1]
						}
						if altNum == num {
							shouldNotContinue = true
							break
						}
					}
					if !shouldNotContinue {
						continue
					}
				}
				if inCard.isSecretLair() {
					skip := true
					for _, name := range defaultBackend.SLDDeckNames {
						if Contains(inCard.Edition, name) || Contains(inCard.Variation, name) {
							skip = false
						}
					}
					if skip {
						continue
					}
				}
			case "ULST":
			case "SLX", "SLU", "SLC", "SLP":
				// If these have no strict matches AND are not properly tagged, skip them
				if len(MatchInSetNumber(inCard.Name, set.Code, ExtractNumber(inCard.Variation))) == 0 && !inCard.hasSecretLairTag(set.Code) {
					continue
				}
			case "SLD":
				skip := false

				// Iterate on all possible combinations of tags, and skip if a
				// condition is unmet
				for _, code := range []string{"SLU", "SLX", "SLC", "SLP"} {
					// The only card with the same name within and without
					if code == "SLX" && inCard.Name == "Themberchaud" {
						continue
					}
					if len(MatchInSet(inCard.Name, code)) > 0 && inCard.hasSecretLairTag(code) {
						skip = true
						break
					}
				}

				// No PLST in SLD
				if inCard.isMysteryList() {
					skip = true
				}

				// Check that the card reported is not coming from a SLD Deck
				// or if it does, make sure it is actually from SLD
				if len(MatchInSetNumber(inCard.Name, "SLD", ExtractNumber(inCard.Variation))) == 0 && len(MatchInSet(inCard.Name, "PLST")) > 0 {
					for _, name := range defaultBackend.SLDDeckNames {
						deckNameInCard := Contains(inCard.Edition, name) || Contains(inCard.Variation, name)
						if deckNameInCard {
							skip = true
							break
						}
					}
				}

				if skip {
					continue
				}
			default:
				continue
			}

		// Some providers use "Textless" for MF cards
		case inCard.isRewards() && !inCard.isMagicFest():
			maybeYear = inCard.playerRewardsYear(maybeYear)
			switch {
			case strings.HasPrefix(set.Name, "Magic Player Rewards "+maybeYear):
			default:
				continue
			}

		case inCard.isWPNGateway():
			switch set.Name {
			case "DCI Promos":
			case "Innistrad: Crimson Vow",
				"The Lost Caverns of Ixalan":
				skip := true
				foundCards := MatchInSet(inCard.Name, set.Code)
				for _, card := range foundCards {
					if card.HasPromoType(PromoTypeWPN) {
						skip = false
						break
					}
				}
				if skip {
					continue
				}
			default:
				switch {
				case strings.HasPrefix(set.Name, "Wizards Play Network "+maybeYear):
				case strings.HasPrefix(set.Name, "Love Your LGS "+maybeYear):
				default:
					continue
				}
			}

		case inCard.isIDWMagazineBook():
			// No Media cards in these sets
			switch set.Code {
			case "P30A", "P30H", "P30M":
				continue
			case "PGPX", "PWOR", "PPRO", "PWCS":
				continue
			}

			switch {
			case !inCard.isJPN() && (set.Name == "IDW Comics Inserts" || set.Name == "HarperPrism Book Promos"):
			case !inCard.isJPN() && strings.HasPrefix(set.Name, "Duels of the Planeswalkers "+maybeYear):
			default:
				switch set.Code {
				case "PURL", "JP1", "DLGM":
				case "PDOM":
					// This set contains both FNM and Media cards
					skip := false
					foundCards := MatchInSet(inCard.Name, set.Code)
					for _, card := range foundCards {
						if card.HasPromoType(PromoTypeFNM) {
							skip = true
							break
						}
					}
					if skip {
						continue
					}
				case "P9ED":
					if inCard.isJPN() {
						continue
					}
				case "PMEI":
					// This is the only card present in IDW and Media Inserts
					// so make sure it is properly tagged
					if inCard.Name == "Duress" && !inCard.isJPN() {
						continue
					}
					// This could be mixed in P9ED Russian
					if inCard.Name == "Shivan Dragon" && !inCard.isJPN() {
						continue
					}
				default:
					if !strings.HasSuffix(set.Name, "Promos") {
						continue
					}
				}
			}

		case inCard.isResale():
			switch set.Code {
			case "DCI", "P30A", "P30H", "P30M":
				continue
			case "PDOM":
				// This might conflict with Llanowar Elves
				continue
			case "PMEI", "PLTC":
				// These sets may actually contain Resale cards
			default:
				if !strings.HasSuffix(set.Name, "Promos") {
					continue
				}
			}

		case inCard.Contains("Hero") && inCard.Contains("Path"):
			switch set.Name {
			case "Born of the Gods Hero's Path",
				"Journey into Nyx Hero's Path",
				"Journey into Nyx Promos",
				"Theros Hero's Path",
				"Defat a God",
				"Face the Hydra",
				"Battle the Horde":
			default:
				continue
			}

		case inCard.Contains("Convention"):
			switch set.Name {
			case "URL/Convention Promos":
			default:
				if strings.HasPrefix(set.Name, "30th Anniversary") {
					continue
				}
				if !strings.HasSuffix(set.Name, "Promos") {
					continue
				}
			}

		case inCard.Contains("Premier Play") || inCard.Contains("Regional Championship Qualifiers"):
			switch {
			case set.Name == "Pro Tour Promos":
				// Special case for a card that could be in PR23
				if inCard.Name == "Snapcaster Mage" && !inCard.Contains("Pro Tour") {
					continue
				}
			case strings.HasPrefix(set.Name, "Regional Championship Qualifiers "+maybeYear):
			default:
				continue
			}

		case inCard.Contains("Game Day") || inCard.Contains("Store Championship"):
			switch set.Code {
			case "SCH":
			case "LTR":
			case "PEWK":
			default:
				skip := true
				switch {
				case strings.HasSuffix(set.Name, "Promos"):
					foundCards := MatchInSet(inCard.Name, set.Code)
					for _, card := range foundCards {
						if card.HasPromoType(PromoTypeStoreChampionship) ||
							card.HasPromoType(PromoTypeGameDay) {
							skip = false
							break
						}
					}
				case strings.HasPrefix(set.Name, "Wizards Play Network "+maybeYear):
					skip = false
				}
				if skip {
					continue
				}
			}

		case inCard.isWorldChamp():
			switch {
			case (maybeYear == "1996" || maybeYear == "") && set.Name == "Pro Tour Collector Set":
			case maybeYear != "" && strings.HasPrefix(set.Name, "World Championship Decks "+maybeYear):
			case maybeYear == "" && strings.HasPrefix(set.Name, "World Championship Decks"):
				skip := true
				num, _ := parseWorldChampPrefix(inCard.Variation)
				foundCards := MatchInSet(inCard.Name, set.Code)
				if num == "" || len(foundCards) == 1 {
					skip = false
				} else {
					for _, card := range foundCards {
						if card.Number == num {
							skip = false
							break
						}
					}
				}
				if skip {
					continue
				}
			default:
				continue
			}

		case inCard.isMagicFest():
			// Some providers use GP2018 instead of MF2019
			if maybeYear == "2018" && inCard.isBasicLand() {
				maybeYear = "2019"
			}
			switch {
			case strings.HasPrefix(set.Name, "MagicFest "+maybeYear):
				if len(MatchInSet(inCard.Name, "SLP")) > 0 && !inCard.Contains("Fest") {
					continue
				}
			case set.Code == "PLG21":
			case set.Code == "PEWK":
			case set.Code == "SLP":
				// If the 'Secret' tag is missing, confirm that this could not be found in other
				// MagicFest sets
				if (len(MatchInSet(inCard.Name, "PF19")) > 0 ||
					len(MatchInSet(inCard.Name, "PF25")) > 0) && !inCard.Contains("Secret") {
					continue
				}
			default:
				continue
			}

		case inCard.isSDCC():
			switch {
			case strings.HasPrefix(set.Name, "San Diego Comic-Con "+maybeYear):
			default:
				continue
			}

		case inCard.isDuelsOfThePW():
			switch {
			case strings.HasPrefix(set.Name, "Duels of the Planeswalkers"):
			default:
				continue
			}

		// DDA with number or deck variant specified in the variation
		case inCard.isDuelDecksAnthology():
			switch {
			case strings.HasPrefix(set.Name, "Duel Decks Anthology"):
				found := false
				// Look if the variation or the edition contain part of set name
				for _, location := range []string{inCard.Variation, inCard.Edition} {
					if !strings.Contains(strings.ToLower(location), "vs") {
						continue
					}
					fields := strings.Fields(location)
					for _, field := range fields {
						// Skip elements that are too short to be representative
						if len(field) < 4 {
							continue
						}
						if Contains(set.Name, field) {
							found = true
							break
						}
					}
				}
				// Do number check only if well known elements are missing
				wellKnownTags := inCard.Contains("Divine") || inCard.Contains("Garruk") ||
					inCard.Contains("Chandra") || inCard.Contains("Goblins")
				if !found && !wellKnownTags {
					num := ExtractNumber(inCard.Variation)
					if num != "" {
						foundCards := MatchInSet(inCard.Name, setCode)
						for _, card := range foundCards {
							if card.Number == num {
								found = true
								break
							}
						}
					}
				}
				if wellKnownTags && !found {
					continue
				}
			default:
				continue
			}

		case inCard.isDuelDecks():
			variant := inCard.duelDecksVariant()
			switch {
			case strings.HasPrefix(set.Name, "Duel Decks") &&
				!strings.Contains(set.Name, "Anthology"):
				if !Contains(set.Name, variant) {
					continue
				}
			default:
				continue
			}

		case strings.Contains(inCard.Variation, "Champs") ||
			strings.Contains(inCard.Variation, "States"):
			switch set.Name {
			case "Champs and States":
			case "Grand Prix Promos":
				continue
			default:
				if strings.HasPrefix(set.Name, "30th Anniversary") {
					continue
				}
				if !strings.HasSuffix(set.Name, "Promos") {
					continue
				}
			}

		case inCard.isPremiereShop():
			if maybeYear == "" {
				guilds := append(GRNGuilds, ARNGuilds...)
				for _, guild := range guilds {
					if strings.Contains(inCard.Variation, guild) {
						maybeYear = "2005"
						break
					}
				}
				if maybeYear == "" && strings.HasSuffix(inCard.Variation, "Cycle") {
					maybeYear = map[string]string{
						"Time Spiral Cycle":       "2006",
						"Lorwyn Cycle":            "2007",
						"Shards of Alara Cycle":   "2008",
						"Zendikar Cycle":          "2009",
						"Scars of Mirrodin Cycle": "2010",
						"Innistrad Cycle":         "2011",
					}[inCard.Variation]
				}
			}
			switch {
			case strings.HasPrefix(set.Name, "Magic Premiere Shop "+maybeYear):
			default:
				continue
			}

		case inCard.isBasicLand() && strings.Contains(inCard.Variation, "APAC"):
			if set.Name != "Asia Pacific Land Program" {
				continue
			}

		case inCard.isBasicLand() && Contains(inCard.Variation, "EURO"):
			if set.Name != "European Land Program" {
				continue
			}

		case strings.Contains(inCard.Edition, "Core Set") ||
			strings.Contains(inCard.Edition, "Core 20") ||
			strings.Contains(inCard.Edition, "Magic 20"):
			switch {
			case !inCard.isGenericPromo() && strings.HasSuffix(set.Name, "Promos"):
				continue
			case strings.HasPrefix(set.Name, "Core Set "+maybeYear):
			case strings.HasPrefix(set.Name, "Magic "+maybeYear):
			default:
				continue
			}

		case inCard.Contains("Grand Prix"):
			switch {
			case strings.HasPrefix(set.Name, "MagicFest "+maybeYear):
			default:
				if set.Name != "Grand Prix Promos" {
					continue
				}
			}

		case inCard.Contains("Lunar New Year") || inCard.Contains("Year of the "):
			switch set.Code {
			case "PLNY":
			default:
				if !strings.HasPrefix(set.Name, "Year of the ") {
					continue
				}
			}

		case inCard.isThickDisplay():
			switch set.Code {
			// The sets with thick display cards separate from the main commander set
			case "OC21", "OAFC", "OMIC", "OVOC":
			// SLD may contain DFC with thick display
			case "SLD":
			default:
				// Skip any set before this date if not from the sets above
				if setDate.Before(SeparateFinishCollectorNumberDate) {
					continue
				}
			}

		case inCard.Contains("Bring-A-Friend") ||
			inCard.Contains("Love Your LGS") ||
			inCard.Contains("Love Your Local Game Store") ||
			inCard.Contains("Welcome Back") ||
			inCard.Contains("Open House") ||
			inCard.Contains("LGS Promo"):
			switch {
			case strings.HasPrefix(set.Name, "Love Your LGS "+maybeYear):
			case strings.HasPrefix(set.Name, "Wizards Play Network "+maybeYear):
			case set.Name == "Grand Prix Promos":
				continue
			default:
				if strings.HasPrefix(set.Name, "30th Anniversary") ||
					strings.HasPrefix(set.Name, "Duels of the Planeswalkers") {
					continue
				}
				if !strings.HasSuffix(set.Name, "Promos") {
					continue
				}
			}

		case inCard.Contains("30th"):
			switch set.Code {
			case "P30A", "P30H", "P30M":
			case "P30T":
				if inCard.isRetro() || !inCard.isJPN() {
					continue
				}
			default:
				continue
			}

		case inCard.Contains("Planeswalker") && inCard.Contains("Promos"):
			switch set.Code {
			case "PWCS":
			default:
				if !strings.HasSuffix(set.Name, "Promos") {
					continue
				}
			}

		// For all the promos with "Extended Art" which refer to the full art promo
		case inCard.isExtendedArt() && !inCard.Contains("Game Day"):
			if setDate.Before(PromosForEverybodyYay) {
				switch set.Code {
				case "DCI":
				default:
					continue
				}
			}

		case inCard.Contains("Eternal Weekend"):
			switch set.Code {
			case "PEWK":
			default:
				continue
			}

		// Last resort, if this is set on the input card, and there were
		// no better descriptors earlier, try looking at the set type
		case inCard.promoWildcard:
			switch set.Type {
			case "promo":
				// Skip common promos, they are usually correctly listed
				if strings.HasPrefix(set.Name, "Judge Gift") ||
					strings.HasPrefix(set.Name, "30th Anniversary") {
					continue
				}
				skip := false
				foundCards := MatchInSet(inCard.Name, setCode)
				// It is required to set a proper tag to parse non-English
				// cards or well-known promos
				for _, card := range foundCards {
					if card.Language == LanguageJapanese {
						skip = true
						break
					}
				}

				if skip {
					continue
				}
			case "starter":
				if !strings.HasSuffix(set.Name, "Clash Pack") {
					continue
				}
			case "expansion", "core", "masters", "draft_innovation":
				skip := true
				foundCards := MatchInSet(inCard.Name, setCode)
				for _, card := range foundCards {
					// Skip boosterfun because they are inherently non-promo
					if card.IsPromo && !card.HasPromoType(PromoTypeBoosterfun) {
						skip = false
						break
					}
				}
				if skip {
					continue
				}
			case "box":
				skip := true
				switch setCode {
				// Only keep the planeswalkers from SLD for this category
				case "SLD":
					foundCards := MatchInSet(inCard.Name, setCode)
					for _, card := range foundCards {
						if slices.Contains(card.Types, "Planeswalker") {
							skip = false
							break
						}
					}
				}
				if skip {
					continue
				}
			case "funny":
				if set.Name != "HasCon 2017" {
					continue
				}
			default:
				continue
			}

		// Tokens need correct set names or special handling earlier
		case (strings.HasSuffix(inCard.Name, "Token") &&
			defaultBackend.UUIDs[defaultBackend.Hashes[Normalize(inCard.Name)][0]].Layout == "token") ||
			(!strings.HasSuffix(inCard.Name, "Token") &&
				defaultBackend.UUIDs[defaultBackend.Hashes[Normalize(inCard.Name)][0]].Layout == "token"):
			if !Equals(inCard.Edition, set.Name) {
				continue
			}
		}

		printings = append(printings, setCode)
	}

	return
}

// Deduplicate cards with the same name.
func filterCards(inCard *InputCard, cardSet map[string][]Card) (outCards []Card) {
	for setCode, inCards := range cardSet {
		set := defaultBackend.Sets[setCode]

		for _, card := range inCards {
			// Super lucky case, we were expecting the card
			num, found := VariantsTable[set.Name][card.Name][strings.ToLower(inCard.Variation)]
			if found {
				if num == card.Number {
					outCards = append(outCards, card)
				}

				// If a variant is expected we assume that all cases are covered
				continue
			}

			checkNum := true
			// Lucky case, variation is just the collector number
			num = ExtractNumber(inCard.Variation)
			// Special case for SLD, finally breaking the check against years
			if num == "" && card.SetCode == "SLD" {
				num = ExtractNumberAny(inCard.Variation)
			}
			if inCard.shouldIgnoreNumber(set.Name, num) {
				checkNum = false
				logger.Println("Skipping number check")
			}
			if checkNum && num != "" {
				// The empty string will allow to test the number without any
				// additional prefix first
				possibleSuffixes := []string{""}
				variation := strings.Replace(inCard.Variation, "-", "", 1)
				fields := strings.Fields(strings.ToLower(variation))
				possibleSuffixes = append(possibleSuffixes, fields...)

				// Check if edition-specific numbers need special suffixes
				numFilterFunc, found := numberFilterCallbacks[set.Code]
				if found {
					overrides := numFilterFunc(inCard)
					if overrides != nil {
						possibleSuffixes = overrides
					}
				} else if card.Identifiers["tcgplayerAlternativeFoilProductId"] != "" {
					// Special case when we deal with duplicated entries
					if inCard.Foil || inCard.isFoil() {
						possibleSuffixes = []string{SuffixSpecial}
					} else {
						possibleSuffixes = []string{""}
					}
				}

				// Add any possible extra suffixes if we know what we're dealing with
				switch {
				case inCard.isPrerelease():
					possibleSuffixes = append(possibleSuffixes, "s")
				case inCard.isPromoPack():
					possibleSuffixes = append(possibleSuffixes, "p")
				case inCard.isChineseAltArt():
					possibleSuffixes = append(possibleSuffixes, "s", SuffixSpecial+"s", SuffixVariant+"s")
				case inCard.isSerialized():
					possibleSuffixes = append(possibleSuffixes, "z")
				case inCard.isJudge() || inCard.isResale():
					possibleSuffixes = append(possibleSuffixes, SuffixSpecial)
				case inCard.isJPN():
					possibleSuffixes = append(possibleSuffixes, "jpn")
				case inCard.Edition == "Alternate Fourth Edition":
					possibleSuffixes = append(possibleSuffixes, "alt")
				}

				for _, numSuffix := range possibleSuffixes {
					// The self test is already expressed by the empty string
					// This avoids an odd case of testing 1.1 = 11
					if num == numSuffix {
						continue
					}
					number := strings.ToLower(num)
					if numSuffix != "" && !strings.HasSuffix(number, numSuffix) {
						number += numSuffix
					}

					if number == strings.ToLower(card.Number) {
						logger.Println("Found match with card number", card.Number)
						outCards = append(outCards, card)

						// Card was found, skip any other suffix
						break
					}
				}

				// If a variant is a number we expect that this information is
				// reliable, so skip anything else
				continue
			}

			// The last-ditch effort from above - when this is set, only check
			// the non-promo sets as some promos can be mixed next to the
			// normal cards - in this way, promo sets can process as normal and
			// deduplicate all the various prerelease and promo packs
			switch set.Type {
			case "expansion", "core", "masters", "draft_innovation":
				if inCard.promoWildcard &&
					!card.HasPromoType(PromoTypeBoosterfun) &&
					!card.HasPromoType(PromoTypePromoPack) &&
					!card.HasPromoType(PromoTypeStarterDeck) &&
					!card.HasPromoType(PromoTypeIntroPack) &&
					!strings.HasSuffix(card.Number, SuffixSpecial) &&
					!card.IsPromo {
					continue
				}
			}

			outCards = append(outCards, card)
		}
	}

	// Sort through the array of promo types
	if len(outCards) > 1 {
		var filteredOutCards []Card
		for _, card := range outCards {
			set, found := defaultBackend.Sets[card.SetCode]
			if !found {
				continue
			}
			setDate := set.ReleaseDateTime

			var shouldContinue bool
			for _, promoElement := range promoTypeElements {
				if setDate.Before(promoElement.ValidDate) {
					continue
				}
				if promoElement.CanBeWild && inCard.promoWildcard {
					continue
				}

				var tagPresent bool
				if promoElement.TagFunc != nil {
					tagPresent = promoElement.TagFunc(inCard)
				} else {
					for _, tag := range promoElement.Tags {
						if inCard.Contains(tag) {
							tagPresent = true
							break
						}
					}
				}

				if tagPresent && !card.HasPromoType(promoElement.PromoType) {
					shouldContinue = true
					break
				} else if !tagPresent && card.HasPromoType(promoElement.PromoType) {
					shouldContinue = true
					break
				}
			}
			if shouldContinue {
				continue
			}

			if inCard.beyondBaseSet {
				// Filter out any card that is located in the base set only
				num, err := strconv.Atoi(card.Number)
				if err == nil && num < set.BaseSetSize {
					continue
				}
			}
			filteredOutCards = append(filteredOutCards, card)
		}

		// Don't throw away what was found if filtering checks is too aggressive
		if len(filteredOutCards) > 0 {
			outCards = filteredOutCards
		}
	}

	// Sort through any custom per-edition filter(s)
	if len(outCards) > 1 {
		var filteredOutCards []Card
		for _, card := range outCards {
			cardFilterFunc, foundSimple := simpleFilterCallbacks[card.SetCode]
			cardFilterFuncs, foundComplex := complexFilterCallbacks[card.SetCode]
			if foundSimple {
				if cardFilterFunc(inCard, &card) {
					continue
				}
			} else if foundComplex {
				shouldContinue := false
				for _, fn := range cardFilterFuncs {
					if fn(inCard, &card) {
						shouldContinue = true
						break
					}
				}
				if shouldContinue {
					continue
				}
			} else {
				if misprintCheck(inCard, &card) {
					continue
				}
			}

			filteredOutCards = append(filteredOutCards, card)
		}

		// Don't throw away what was found if filtering checks is too aggressive
		if len(filteredOutCards) > 0 {
			outCards = filteredOutCards
		}
	}

	if len(outCards) > 1 {
		logger.Println("Filtering status after main loop")
		for _, card := range outCards {
			logger.Println(card.SetCode, card.Name, card.Number)
		}
	}

	// Check if there are multiple printings for Prerelease and Promo Pack cards
	// Sometimes these contain the ParentCode or the parent edition name in the field
	if len(outCards) > 1 && (inCard.isPrerelease() || inCard.isPromoPack()) {
		allSameEdition := true
		for _, card := range outCards {
			if card.Name != outCards[0].Name || !strings.HasPrefix(card.SetCode, "P") {
				allSameEdition = false
				break
			}
		}

		if allSameEdition {
			logger.Println("allSameEdition pass needed")
			var filteredOutCards []Card
			for _, card := range outCards {
				set := defaultBackend.Sets[card.SetCode]
				// The year is necessary to decouple PM20 and PM21 cards
				year := ExtractYear(set.Name)
				// Check if the parent set code is present in the variation or edition
				if strings.Contains(inCard.Variation, set.ParentCode) ||
					strings.Contains(inCard.Edition, set.ParentCode) ||
					(year != "" && inCard.Contains(year)) {
					filteredOutCards = append(filteredOutCards, card)
				} else {
					for probe, number := range multiPromosTable[set.Name][card.Name] {
						if inCard.Contains(probe) && ExtractNumberValue(card.Number) == number {
							filteredOutCards = append(filteredOutCards, card)
						}
					}
				}
			}

			// Don't throw away what was found if filtering checks is too aggressive
			if len(filteredOutCards) > 0 {
				outCards = filteredOutCards
			}
		}
	}

	if len(outCards) > 1 && ExtractNumber(inCard.Variation) == "" {
		// Separate finishes have different collector numbers after this date
		if len(outCards) > 1 {
			var filteredOutCards []Card
			for _, card := range outCards {
				set, found := defaultBackend.Sets[card.SetCode]
				if !found {
					continue
				}
				setDate := set.ReleaseDateTime
				if setDate.After(SeparateFinishCollectorNumberDate) && etchedCheck(inCard, &card) {
					continue
				}
				filteredOutCards = append(filteredOutCards, card)
			}

			// Don't throw away what was found if filtering checks are too aggressive
			if len(filteredOutCards) > 0 {
				outCards = filteredOutCards
			}
		}

		// If above filters were not enough, check the boder, but dont skip card if it's showcase
		// ExtendedArt is fine as there cannot be a borderless one
		if len(outCards) > 1 {
			var filteredOutCards []Card
			for _, card := range outCards {
				if borderlessCheck(inCard, &card) {
					continue
				}
				filteredOutCards = append(filteredOutCards, card)
			}

			// Don't throw away what was found if filtering checks are too aggressive
			if len(filteredOutCards) > 0 {
				outCards = filteredOutCards
			}
		}

		// If above filters were not enough, check the frameEffects
		// Extended Art
		if len(outCards) > 1 {
			var filteredOutCards []Card
			for _, card := range outCards {
				// This needs date check because some old full art promos are marked
				// as extended art, in a different way of what modern Extended Art is
				set, found := defaultBackend.Sets[card.SetCode]
				if !found {
					continue
				}
				setDate := set.ReleaseDateTime
				if setDate.After(PromosForEverybodyYay) && extendedartCheck(inCard, &card) {
					continue
				}
				filteredOutCards = append(filteredOutCards, card)
			}

			// Don't throw away what was found if filtering checks are too aggressive
			if len(filteredOutCards) > 0 {
				outCards = filteredOutCards
			}
		}

		// Showcase
		if len(outCards) > 1 {
			var filteredOutCards []Card
			for _, card := range outCards {
				if showcaseCheck(inCard, &card) {
					continue
				}
				filteredOutCards = append(filteredOutCards, card)
			}

			// Don't throw away what was found if filtering checks are too aggressive
			if len(filteredOutCards) > 0 {
				outCards = filteredOutCards
			}
		}
	}

	return
}
