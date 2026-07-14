package mtgmatcher

import (
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type cardFilterCallback func(inCard *InputCard, card *Card) bool

type promoTypeElement struct {
	// Name of the promo type to validate
	PromoType string

	// Validity date of the check, skipped if set is published before this
	ValidDate time.Time

	// Tag function
	TagFunc func(inCard *InputCard) bool

	// Simple tags to check, if TagFunc is not set
	Tags []string

	// Whether certain promos are not tagged, and are selected as wildcards
	CanBeWild bool
}

var promoTypeElements = []promoTypeElement{
	{
		PromoType: PromoTypePrerelease,
		Tags:      []string{"Prerelease", "Preview"},
	},
	{
		PromoType: PromoTypePlayPromo,
		Tags:      []string{"Play Promo"},
	},
	{
		PromoType: PromoTypePromoPack,
		TagFunc: func(inCard *InputCard) bool {
			return inCard.isPromoPack()
		},
	},
	{
		PromoType: PromoTypeSChineseAltArt,
		TagFunc: func(inCard *InputCard) bool {
			return inCard.isChineseAltArt()
		},
	},
	{
		PromoType: PromoTypeBuyABox,
		// After ZNR buy-a-box is also present in main set
		ValidDate: BuyABoxNotUniqueDate,
		TagFunc: func(inCard *InputCard) bool {
			return inCard.isBaB() || inCard.isRelease()
		},
		CanBeWild: true,
	},
	{
		PromoType: PromoTypeBundle,
		Tags:      []string{"Bundle"},
		CanBeWild: true,
	},
	{
		PromoType: PromoTypeGilded,
		Tags:      []string{"Gilded"},
	},
	{
		PromoType: PromoTypeTextured,
		Tags:      []string{"Textured"},
	},
	{
		PromoType: PromoTypeGalaxyFoil,
		TagFunc: func(inCard *InputCard) bool {
			// A lot of providers don't tag SLD cards as Galaxy, but just foil
			// (same for RainbowFoil), so this check essentially makes the test
			// pass, and let filtering continue elsewhere
			if inCard.isSecretLair() &&
				hasPrinting(inCard.Name, "promo_type", PromoTypeGalaxyFoil, "SLD") {
				// The only card which *also* has RainbowFoil, so the check would fail for Galaxy
				if inCard.Name == "Command Tower" {
					return inCard.Contains("1496")
				}
				return inCard.Foil || inCard.Contains("Galaxy")
			}
			return inCard.Contains("Galaxy")
		},
	},
	{
		PromoType: PromoTypeSurgeFoil,
		TagFunc: func(inCard *InputCard) bool {
			return inCard.isSurgeFoil()
		},
	},
	{
		PromoType: PromoTypeStepAndCompleat,
		Tags:      []string{"Compleat"},
	},
	{
		PromoType: PromoTypeConcept,
		TagFunc: func(inCard *InputCard) bool {
			if inCard.Contains("Concept") {
				return true
			}
			if inCard.isBorderless() && hasPrinting(inCard.Name, "promo_type", PromoTypeConcept) {
				return true
			}
			return false
		},
	},
	{
		PromoType: PromoTypeOilSlick,
		TagFunc: func(inCard *InputCard) bool {
			return inCard.isOilSlick()
		},
	},
	{
		PromoType: PromoTypeHaloFoil,
		Tags:      []string{"Halo"},
	},
	{
		PromoType: PromoTypeThickDisplay,
		ValidDate: SeparateFinishCollectorNumberDate,
		Tags:      []string{"Display", "Thick"},
	},
	{
		PromoType: PromoTypeSerialized,
		TagFunc: func(inCard *InputCard) bool {
			return inCard.isSerialized()
		},
	},
	{
		PromoType: PromoTypeConfettiFoil,
		Tags:      []string{"Confetti"},
	},
	{
		PromoType: PromoTypeEmbossed,
		Tags:      []string{"Ampersand", "Emblem", "Embossed"},
	},
	{
		PromoType: PromoTypeScroll,
		Tags:      []string{"Scroll", "Showcase Silver Foil"},
	},
	{
		PromoType: PromoTypePoster,
		Tags:      []string{"Poster", "Hand Drawn"},
	},
	{
		PromoType: PromoTypeInvisibleInk,
		Tags:      []string{"Invisible"},
	},
	{
		PromoType: PromoTypeRippleFoil,
		Tags:      []string{"Ripple"},
	},
	{
		PromoType: PromoTypeRaisedFoil,
		Tags:      []string{"Raised"},
		// Needed due to oilslick cards from ONE sometimes being referred to as raised
		ValidDate: time.Date(2024, time.April, 1, 0, 0, 0, 0, time.UTC),
	},
	{
		PromoType: PromoTypeFractureFoil,
		Tags:      []string{"Fracture", "Fractal"},
	},
	{
		PromoType: PromoTypeManaFoil,
		Tags:      []string{"Mana Foil"},
	},
}

var simpleFilterCallbacks = map[string]cardFilterCallback{
	"ARN": lightDarkManaCost,

	"FEM":  femVariantInArtist,
	"ALL":  variantInArtistOrFlavor,
	"HML":  variantInArtistOrFlavor,
	"PALP": variantInArtistOrFlavor,
	"PELP": variantInArtistOrFlavor,
	"CM2":  variantInArtistOrFlavor,

	"PMPS": variantInWatermark,
	"GK1":  variantInWatermark,
	"GK2":  variantInWatermark,

	"PLS": altArtCheck,

	"BBD": foilCheck,
	"CN2": foilCheck,
	"8ED": foilCheck,
	"FRF": foilCheck,
	"9ED": foilCheck,
	"ONS": foilCheck,
	"7ED": foilCheck,
	"10E": foilCheck,
	"UNH": foilCheck,
	"40K": foilCheck,

	"DKM": deckmastersVariant,
	"UST": singleLetterVariant,

	"POR": portalDemoGame,

	"2XM": launchPromoInSet,
	"2X2": launchPromoInSet,
	"CMM": launchPromoInSet,

	"CLB": variantBeforePlainCard,

	"KLD": starterDeckCheck,
	"AER": starterDeckCheck,

	"DD2": japaneseCheck,
	"STA": japaneseCheck,
	"WAR": japaneseCheck,

	"GRN": guildgateVariant,
	"RNA": guildgateVariant,

	"UNF": attractionVariant,

	"BOT": shatteredCheck,

	"MAT":  retroCheck,
	"MH2":  retroCheck,
	"P30H": retroCheck,
	"30A":  retroCheck,
	"PW23": retroCheck,
	"RVR":  retroCheck,
	"MH1":  retroCheck, // Due to Flusterstorm in MH3

	"BRO": babOrBuyaboxRetroCheck,

	"THB": foilMisprint,
	"STX": foilMisprint,
	"SHM": foilMisprint,

	"PAL99": nodateMisprint,
	"PULG":  nodateMisprint,
	"HHO":   nodateMisprint,
	"PTOR":  laquatusMisprint,

	"PTC":  wcdNumberCompare,
	"WC97": wcdNumberCompare,
	"WC98": wcdNumberCompare,
	"WC99": wcdNumberCompare,
	"WC00": wcdNumberCompare,
	"WC01": wcdNumberCompare,
	"WC02": wcdNumberCompare,
	"WC03": wcdNumberCompare,
	"WC04": wcdNumberCompare,

	"PPTK": lubuPrereleaseVariant,

	"ALA": showcaseCheck,

	"IKO": reskinGodzillaCheck,

	"PLTR": lotrTripleFiltering,

	"BFZ": fullartCheckForBasicLands,
	"ZEN": fullartCheckForBasicLands,

	// This is needed only for sets with multiple printings of the same card
	"KHM": phyrexianCheck,
	"NEO": phyrexianCheck,
	"SNC": phyrexianCheck,
	"DMU": phyrexianCheck,
	"ONE": phyrexianCheck,
	"FDN": phyrexianCheck,

	"TSR": releaseRetroCheck,
	"CLU": releaseRetroCheck,
	"INR": releaseRetroCheck,

	"PHOU": draftweekendCheck,
	"PXLN": draftweekendCheck,
	"PUST": draftweekendCheck,
	"PRIX": draftweekendCheck,
	"PDOM": draftweekendCheck,
	"PM19": draftweekendCheck,
	"PGRN": draftweekendCheck,
	"PRNA": draftweekendCheck,

	"G14": judgeLandCheck,
	"P23": judgeLandCheck,
}

var complexFilterCallbacks = map[string][]cardFilterCallback{
	"JMP": {launchPromoInSet, phyrexianCheck},
	"J22": {launchPromoInSet, animeCheck},
	"BRR": {schematicCheck},
	"DMR": {launchPromoInSet, releaseRetroCheck},
	"VOW": {wpnCheck, reskinDraculaCheck},
	"SLD": {sldVariant, etchedCheck, thickDisplayCheck, phyrexianCheck, reskinRenameCheck},
	"CMR": {variantInCommanderDeck, etchedCheck, thickDisplayCheck},
	"M3C": {foilCheckM3C, thickDisplayCheck},
	"FIC": {foilCheck},

	"PWAR": {japaneseCheck, draftweekendCheck},

	"LTR": {lotrTripleFiltering, serialCheck},
	"MH3": {retroCheck, serialCheck},
	// These two checks need to be separate in case two cards have the same number
	// but are originally from two different editions
	"PLST": {listNumberCompare, listEditionCheck},
}

func judgeLandCheck(inCard *InputCard, card *Card) bool {
	if (inCard.Contains("14") && !strings.HasSuffix(card.Number, SuffixSpecial)) ||
		inCard.Contains("23") && strings.HasSuffix(card.Number, SuffixSpecial) {
		return true
	} else if !inCard.Contains("14") && strings.HasSuffix(card.Number, SuffixSpecial) ||
		!inCard.Contains("23") && !strings.HasSuffix(card.Number, SuffixSpecial) {
		return true
	}
	return false
}

func listNumberCompare(inCard *InputCard, card *Card) bool {
	number := ExtractNumber(inCard.Variation)

	// If a number is found, check that it's matching the card number
	if number != "" {
		// Compare the number portion of the field only
		cardNumbers := strings.Split(card.Number, "-")
		listNumbers := strings.Split(number, "-")
		cardNumber := cardNumbers[len(cardNumbers)-1]
		listNumber := listNumbers[len(listNumbers)-1]
		if cardNumber == listNumber {
			return false
		}

		// Skip anything else, the number needs to be correct,
		// unless there is actually an edition name (ie Masters 25)
		// that will be processed later
		maybeEdition := inCard.Variation
		maybeEdition = strings.Replace(maybeEdition, "Non-Foil", "", 1)
		maybeEdition = strings.Replace(maybeEdition, "Foil", "", 1)
		maybeEdition = strings.TrimLeft(maybeEdition, " -")
		_, err := GetSetByName(maybeEdition)
		if err != nil {
			return true
		}
	}

	return false
}

var allPlayerRewardsSet = []string{
	"P03", "P04", "P05", "P06", "P07", "P08", "P09", "P10", "P11",
}

var onlineCodes = map[string]string{
	"ME1": "Masters Edition I",
	"ME2": "Masters Edition II",
	"ME3": "Masters Edition III",
	"ME4": "Masters Edition IV",
	"TD0": "Magic Online Theme Decks",
	"TD2": "Duel Decks: Mirrodin Pure vs. New Phyrexia",
	"TPR": "Tempest Remastered",
}

func listEditionCheck(inCard *InputCard, card *Card) bool {
	var setName string

	code := strings.Split(card.Number, "-")[0]
	set, err := GetSet(code)
	if err == nil {
		setName = set.Name
	} else {
		setName = onlineCodes[code]
		if setName == "" {
			return true
		}
	}

	// The few promo sets will have the same number, so filter out all input card that might
	// resemble a promo, unless correctly tagged
	if !strings.HasSuffix(setName, "Promos") && (inCard.Contains("P"+code) || inCard.Contains("Promos")) {
		return true
	}

	switch inCard.Name {
	case "Phantom Centaur",
		"Arcane Teachings":
		return misprintCheck(inCard, card)
	// Cards with same numeric part need special treatment because the chunk below trips the later check
	case "Laboratory Maniac",
		"Bad Moon":
		if !inCard.Contains(code) && !inCard.Contains(setName) && EditionTable[inCard.Variation] != setName {
			return true
		}
	default:
		switch {
		case inCard.Contains("Player Rewards"):
			if slices.Contains(allPlayerRewardsSet, code) {
				return false
			}
		case inCard.Contains("Game Day"):
			ids, _ := SearchEquals(card.Name)
			for _, id := range ids {
				co := defaultBackend.UUIDs[id]
				if co.SetCode == code && co.HasPromoType(PromoTypeGameDay) {
					return false
				}
			}
		}

		if strings.Contains(inCard.Variation, "vs.") {
			setName = strings.TrimPrefix(setName, "Duel Decks: ")
		}

		if !inCard.Contains(code) && !inCard.Contains(setName) && EditionTable[inCard.Variation] != setName {
			// This chunk is needed in case there was a plain number already
			// processed in the previous step
			number := ExtractNumber(inCard.Variation)

			cardNumbers := strings.Split(card.Number, "-")
			listNumbers := strings.Split(number, "-")
			cardNumber := cardNumbers[len(cardNumbers)-1]
			listNumber := listNumbers[len(listNumbers)-1]
			// All promos have the same number, so trust the filtering above
			if cardNumber == listNumber && !strings.HasSuffix(setName, "Promos") {
				return false
			}

			return true
		}
	}

	return false
}

func phyrexianCheck(inCard *InputCard, card *Card) bool {
	if inCard.isPhyrexian() && card.Language != LanguagePhyrexian {
		return true
	} else if !inCard.isPhyrexian() && card.Language == LanguagePhyrexian {
		return true
	}
	return false
}

// Handle full vs nonfull art basic land
func fullartCheckForBasicLands(inCard *InputCard, card *Card) bool {
	if inCard.isBasicFullArt() && !card.IsFullArt {
		return true
	} else if inCard.isBasicNonFullArt() && card.IsFullArt {
		return true
	}
	return false
}

func lotrTripleFiltering(inCard *InputCard, card *Card) bool {
	switch card.Name {
	case "Delighted Halfling",
		"Lobelia Sackville-Baggins",
		"Frodo Baggins",
		"Bilbo, Retired Burglar",
		"Gandalf, Friend of the Shire",
		"Wizard's Rockets":
		num := ExtractNumber(inCard.Variation)
		if num != "" && (Contains(inCard.Edition, "Prerelease") || Contains(inCard.Edition, "Promo")) {
			return card.SetCode != "PLTR"
		}
		if inCard.Contains("Stamp") {
			if !inCard.Contains("No") && card.SetCode != "PLTR" {
				return true
			} else if inCard.Contains("No") && card.SetCode == "PLTR" {
				return true
			}
		}
	case "Saruman of Many Colors":
		if inCard.Contains("Store Champ") && !card.HasPromoType(PromoTypeStoreChampionship) {
			return true
		} else if !inCard.Contains("Store Champ") && card.HasPromoType(PromoTypeStoreChampionship) {
			return true
		}
	}
	return false
}

func lightDarkManaCost(inCard *InputCard, card *Card) bool {
	if inCard.isARNLightMana() && !strings.HasSuffix(card.Number, SuffixVariant) {
		return true
	} else if (inCard.isARNDarkMana() || inCard.Variation == "") && strings.HasSuffix(card.Number, SuffixVariant) {
		return true
	}
	return false
}

func femVariantInArtist(inCard *InputCard, card *Card) bool {
	// Since the check is field by field Foglio may alias Phil or Kaja
	if strings.Contains(inCard.Variation, "Foglio") {
		inCard.Variation = strings.Replace(inCard.Variation, "Phil Foglio", "PhilFoglio", 1)
		inCard.Variation = strings.Replace(inCard.Variation, "Kaja Foglio", "KajaFoglio", 1)
	}
	return variantInArtistOrFlavor(inCard, card)
}

func variantInArtistOrFlavor(inCard *InputCard, card *Card) bool {
	// Skip the check if this tag is empty, so that users can notice
	// there is an aliasing problem
	if inCard.Variation == "" {
		return true
	}
	variant := inCard.Variation
	variant = strings.Replace(variant, "Illust.", "", 1)

	fields := strings.Fields(variant)
	found := false

	// Keep flavor text author only
	flavor := card.FlavorText
	if strings.Contains(flavor, "—") {
		fields := strings.Split(flavor, "—")
		flavor = fields[len(fields)-1]
	}

	// Check field by field, it's usually enough for just two elements
	for _, field := range fields {
		// Skip short text like 'jr.' since they are often missing
		// Skip Land too for High and Low lands alias
		// Skip Sass due to the fact that 's' are ignored
		if len(field) < 4 || strings.HasPrefix(field, "Land") || field == "Sass" {
			continue
		}
		if Contains(flavor, field) || Contains(card.Artist, field) {
			found = true
			break
		}
	}

	if !found {
		// If not found double check if variation contains the same number suffix
		numberSuffix := inCard.possibleNumberSuffix()
		if numberSuffix == "" || (numberSuffix != "" && !strings.HasSuffix(card.Number, numberSuffix)) {
			return true
		}
	}

	return false
}

// Check watermark when variation has no number information
func variantInWatermark(inCard *InputCard, card *Card) bool {
	// Skip the check if this tag is empty, so that users can notice there is an aliasing problem
	if inCard.Variation == "" {
		return true
	}
	if !Contains(inCard.Variation, card.Watermark) {
		return true
	}
	return false
}

// Foil-only-booster cards, non-special version has both foil and non-foil
func altArtCheck(inCard *InputCard, card *Card) bool {
	if inCard.isGenericAltArt() && !strings.HasSuffix(card.Number, SuffixSpecial) {
		return true
	} else if !inCard.isGenericAltArt() && strings.HasSuffix(card.Number, SuffixSpecial) {
		return true
	}
	return false
}

// Foil-only-booster cards, non-special version only have non-foil
// (only works if card has no other duplicates within the same edition)
func foilCheck(inCard *InputCard, card *Card) bool {
	if inCard.Foil && card.HasFinish(FinishNonfoil) {
		return true
	} else if !inCard.Foil && card.HasFinish(FinishFoil) {
		return true
	}
	return false
}

// This is a terrible hack because the number check is disabled
// (Modern Horizons THREE Commander contains the same number of this card)
// and there are several variants of this card (Satya), so we cannot
// enable the etched check for *all* of them
func foilCheckM3C(inCard *InputCard, card *Card) bool {
	if card.Number == "3" || card.Number == "23" {
		return etchedCheck(inCard, card)
	}
	if inCard.Foil && card.HasFinish(FinishNonfoil) {
		return true
	} else if !inCard.Foil && card.HasFinish(FinishFoil) {
		return true
	}
	return false
}

func etchedCheck(inCard *InputCard, card *Card) bool {
	if inCard.isEtched() && !card.HasFinish(FinishEtched) {
		return true
		// Some thick display cards are not marked as etched
	} else if !inCard.isEtched() && !inCard.isThickDisplay() && card.HasFinish(FinishEtched) {
		return true
	}
	return false
}

func thickDisplayCheck(inCard *InputCard, card *Card) bool {
	if inCard.isThickDisplay() && !card.HasPromoType(PromoTypeThickDisplay) {
		return true
	} else if !inCard.isThickDisplay() && card.HasPromoType(PromoTypeThickDisplay) {
		return true
	}
	return false
}

// Single letter variants
func singleLetterVariant(inCard *InputCard, card *Card) bool {
	numberSuffix := inCard.possibleNumberSuffix()
	if len(card.Variations) > 0 && numberSuffix == "" {
		numberSuffix = "a"
	}
	if numberSuffix != "" && !strings.HasSuffix(card.Number, numberSuffix) {
		return true
	}
	return false
}

func deckmastersVariant(inCard *InputCard, card *Card) bool {
	numberSuffix := inCard.possibleNumberSuffix()
	switch card.Name {
	case "Incinerate", "Icy Manipulator":
		inCard.Foil = inCard.Foil || inCard.Contains("Promo")
		return foilCheck(inCard, card)
	default:
		// Pick the first of the two if not specified
		if len(card.Variations) > 0 && numberSuffix == "" {
			numberSuffix = "a"
		}
		// Reset for lands
		if inCard.isBasicLand() {
			numberSuffix = ""
		}
	}

	if numberSuffix != "" && !strings.HasSuffix(card.Number, numberSuffix) {
		return true
	}
	return false
}

// Variants related to flavor text presence
func portalDemoGame(inCard *InputCard, card *Card) bool {
	if inCard.isPortalAlt() && !strings.HasSuffix(card.Number, SuffixVariant) && !strings.HasSuffix(card.Number, "d") {
		return true
	} else if !inCard.isPortalAlt() && (strings.HasSuffix(card.Number, SuffixVariant) || strings.HasSuffix(card.Number, "d")) {
		return true
	}
	return false
}

// Launch promos within the set itself
func launchPromoInSet(inCard *InputCard, card *Card) bool {
	anyAlternative := card.IsAlternative ||
		card.BorderColor == BorderColorBorderless ||
		card.HasFrameEffect(FrameEffectExtendedArt)
	if (inCard.isRelease() || inCard.isBaB()) && !anyAlternative {
		return true
	} else if !(inCard.isRelease() || inCard.isBaB()) && anyAlternative && !card.HasPromoType(PromoTypeBoosterfun) {
		return true
	}
	return false
}

// Identical cards
func variantInCommanderDeck(inCard *InputCard, card *Card) bool {
	// Filter only cards that may have the flag set
	hasAlternate := card.IsAlternative
	for _, id := range card.Variations {
		alt := defaultBackend.UUIDs[id]
		if alt.IsAlternative {
			hasAlternate = true
			break
		}
	}
	// Only check when cards do have alts, as some vendors use the
	// Variation field for unnecessary info for unrelated cards
	// Skip EA because it does not need this deduplication
	if !inCard.isExtendedArt() && !inCard.isEtched() && hasAlternate {
		if inCard.Variation == "" && card.IsAlternative {
			return true
		} else if inCard.Variation != "" && !card.IsAlternative {
			return true
		}
	}
	return false
}

// EA cards from commander decks appear before the normal prints, beyondBaseSet needs help
func variantBeforePlainCard(inCard *InputCard, card *Card) bool {
	cn, _ := strconv.Atoi(card.Number)
	if cn > 607 && cn < 930 {
		return extendedartCheck(inCard, card)
	}
	return false
}

// Intro/Starter deck
func starterDeckCheck(inCard *InputCard, card *Card) bool {
	isStarter := Contains(inCard.Variation, "Starter") || Contains(inCard.Variation, "Intro")
	if !isStarter && (card.HasPromoType(PromoTypeStarterDeck) || card.IsAlternative) {
		return true
	} else if isStarter && !card.HasPromoType(PromoTypeStarterDeck) && !card.IsAlternative {
		return true
	}
	return false
}

// Japanese Planeswalkers
func japaneseCheck(inCard *InputCard, card *Card) bool {
	if (inCard.isJPN() || inCard.isGenericAltArt()) && card.Language != LanguageJapanese {
		return true
	} else if !inCard.isJPN() && !inCard.isGenericAltArt() && card.Language == LanguageJapanese {
		return true
	}
	return false
}

// Pick one of the printings in case they are not specified
func guildgateVariant(inCard *InputCard, card *Card) bool {
	if strings.Contains(card.Name, "Guildgate") && inCard.Variation == "" {
		cn, _ := strconv.Atoi(card.Number)
		if cn%2 == 0 {
			return true
		}
	}
	return false
}

// Due to the WPN lands
func wpnCheck(inCard *InputCard, card *Card) bool {
	if inCard.isWPNGateway() && !card.HasPromoType(PromoTypeWPN) {
		return true
	} else if !inCard.isWPNGateway() && card.HasPromoType(PromoTypeWPN) {
		return true
	}
	return false
}

// Handle the different Attractions
func attractionVariant(inCard *InputCard, card *Card) bool {
	if card.AttractionLights != nil && (strings.Contains(inCard.Variation, "/") || strings.Contains(inCard.Variation, "-")) {
		lights := make([]string, 0, len(card.AttractionLights))
		for _, light := range card.AttractionLights {
			lights = append(lights, strconv.Itoa(light))
		}
		tag := strings.Join(lights, "/")
		variation := strings.Replace(inCard.Variation, " ", "", -1)
		variation = strings.Replace(variation, "-", "/", -1)
		if variation != tag {
			return true
		}
	}
	switch card.Name {
	case "Space Beleren",
		"Comet, Stellar Pup":
		if inCard.isBorderless() && !inCard.isGalaxyFoil() {
			if card.HasPromoType(PromoTypeGalaxyFoil) {
				return true
			}
		} else if inCard.isGalaxyFoil() && !inCard.isBorderless() {
			if card.BorderColor == BorderColorBorderless {
				return true
			}
		}
	default:
		if !inCard.isBorderless() && !inCard.isGalaxyFoil() &&
			slices.Contains(card.Types, "Land") &&
			card.BorderColor == BorderColorBorderless &&
			card.HasPromoType(PromoTypeGalaxyFoil) {
			return true
		}
	}
	return false
}

func shatteredCheck(inCard *InputCard, card *Card) bool {
	isShattered := inCard.Contains("Shattered") || inCard.Contains("Borderless")
	if isShattered && !card.HasFrameEffect(FrameEffectShattered) {
		return true
	} else if !isShattered && card.HasFrameEffect(FrameEffectShattered) {
		return true
	}
	return false
}

// This check skips serialized cards as their collector numbers would not match
func schematicCheck(inCard *InputCard, card *Card) bool {
	cn, err := strconv.Atoi(card.Number)
	if err != nil {
		return false
	}
	isSchematic := inCard.Contains("Schematic") || inCard.Contains("Blueprint")
	if isSchematic && cn < 64 {
		return true
	} else if !isSchematic && cn >= 64 {
		return true
	}
	return false
}

func animeCheck(inCard *InputCard, card *Card) bool {
	switch card.Name {
	case "Valorous Stance",
		"Dragon Fodder",
		"Stitcher's Supplier",
		"Tragic Slip",
		"Thermo-Alchemist":
		cn, _ := strconv.Atoi(card.Number)
		isAnime := inCard.Contains("Anime")
		if isAnime && (cn < 52 || cn > 97) {
			return true
		} else if !isAnime && cn >= 52 && cn <= 97 {
			return true
		}
	}
	return false
}

func serialCheck(inCard *InputCard, card *Card) bool {
	if inCard.isSerialized() && !card.HasPromoType(PromoTypeSerialized) {
		return true
	} else if !inCard.isSerialized() && card.HasPromoType(PromoTypeSerialized) {
		return true
	}
	return false
}

func retroCheckInternal(isRetro bool, cardFrameVersion string) bool {
	if isRetro && cardFrameVersion != "1997" {
		return true
	} else if !isRetro && cardFrameVersion == "1997" {
		return true
	}
	return false
}

func retroCheck(inCard *InputCard, card *Card) bool {
	return retroCheckInternal(inCard.isRetro() || inCard.beyondBaseSet, card.FrameVersion)
}

// This edition has retro-only promotional cards, but most
// providers only tag the promo type, instead of the frame
func babOrBuyaboxRetroCheck(inCard *InputCard, card *Card) bool {
	return retroCheckInternal(inCard.isBundle() || inCard.isBaB(), card.FrameVersion)
}

func releaseRetroCheck(inCard *InputCard, card *Card) bool {
	return retroCheckInternal(inCard.isRetro() || inCard.isRelease(), card.FrameVersion)
}

// Foil cards which exist *only* as misprints
func foilMisprint(inCard *InputCard, card *Card) bool {
	if !inCard.Foil {
		return strings.HasSuffix(card.Number, SuffixSpecial)
	}

	// Get number in case there is no EA information available
	maybeNumber := ExtractNumber(inCard.Variation)

	switch card.Name {
	case "Temple of Abandon":
		if inCard.isExtendedArt() || strings.HasPrefix(maybeNumber, "347") {
			return !strings.HasSuffix(card.Number, SuffixSpecial)
		}
	case "Strict Proctor":
		if !inCard.isExtendedArt() || strings.HasPrefix(maybeNumber, "33") {
			return !strings.HasSuffix(card.Number, SuffixSpecial)
		}
	case "Reflecting Pool":
		return !strings.HasSuffix(card.Number, SuffixSpecial)
	}
	return strings.HasSuffix(card.Number, SuffixSpecial)
}

func nodateMisprint(inCard *InputCard, card *Card) bool {
	switch card.Name {
	case "Beast of Burden",
		"Island",
		"Stocking Tiger":
		if inCard.Contains("Misprint") ||
			inCard.Contains("No Expansion Symbol") ||
			inCard.Contains("No Date") ||
			inCard.Contains("No Stamp") ||
			inCard.Contains("No Symbol") {
			return !strings.HasSuffix(card.Number, SuffixVariant)
		}
	}
	return strings.HasSuffix(card.Number, SuffixVariant)
}

func laquatusMisprint(inCard *InputCard, card *Card) bool {
	switch card.Name {
	case "Laquatus's Champion":
		if Contains(inCard.Variation, "dark") {
			return card.Number != "67†a"
		}
		if Contains(inCard.Variation, "misprint") {
			return card.Number != "67†"
		}
		return card.Number != "67"
	}
	return false
}

func sldVariant(inCard *InputCard, card *Card) bool {
	switch card.Name {
	case "Geralf's Messenger":
		return retroCheckInternal(card.Number == "887", card.FrameVersion)
	case "Demonlord Belzenlok",
		"Griselbrand",
		"Liliana's Contract",
		"Kothophed, Soul Hoarder",
		"Razaketh, the Foulblooded":
		num, _ := strconv.Atoi(ExtractNumberValue(card.Number))
		if num < 200 {
			result := strings.HasSuffix(card.Number, SuffixSpecial)
			if inCard.isEtched() {
				result = !result
			}
			return result
		}
	case "Plague Sliver",
		"Shadowborn Apostle",
		"Toxin Sliver",
		"Virulent Sliver":
		result := strings.HasSuffix(card.Number, SuffixPhiLow)
		if inCard.isStepAndCompleat() {
			result = !result
		}
		return result
	case "Blasphemous Act":
		if card.Number == "322" {
			return foilCheck(inCard, card)
		}
		result := strings.HasSuffix(card.Number, SuffixSpecial)
		if inCard.Foil {
			result = !result
		}
		return result
	case "Okaun, Eye of Chaos",
		"Zndrsplt, Eye of Wisdom":
		result := strings.HasSuffix(card.Number, SuffixSpecial)
		if inCard.isThickDisplay() {
			result = !result
		}
		return result
	case "Command Tower", "Food",
		"Mogis, God of Slaughter",
		"Lava Dart", "Monastery Swiftspear", "Soul-Scar Mage", "Underworld Breach", "Mishra's Bauble",
		"Chain Lightning", "Dragon's Rage Channeler", "Lava Spike", "Rift Bolt", "Skewer the Critics":
		if !inCard.Contains(ExtractNumberValue(card.Number)) {
			return true
		}
	}

	return foilCheck(inCard, card)
}

func wcdNumberCompare(inCard *InputCard, card *Card) bool {
	prefix, sideboard := inCard.worldChampPrefix()
	wcdNum := extractWCDNumber(inCard.Variation, prefix, sideboard)

	// If a wcdNum is found, check that it's matching the card number
	if wcdNum != "" {
		if wcdNum == card.Number {
			return false
		}
		// Skip anything else, the number needs to be correct
		return true
	}

	// Else rebuild the number manually using prefix, sideboard, and num as hints
	if prefix != "" {
		// Copy this field so we can discard portions that have
		// already been used for deduplication
		cn := card.Number
		if sideboard && !strings.HasSuffix(cn, "sb") {
			return true
		} else if !sideboard && strings.HasSuffix(cn, "sb") {
			return true
		}
		cn = strings.Replace(cn, "sb", "", 1)

		// ML and MLP conflict with HasPrefix, so strip away
		// the numeric part and do a straight equal
		idx := strings.IndexFunc(cn, func(c rune) bool {
			return unicode.IsDigit(c)
		})
		if idx < 1 || prefix != cn[:idx] {
			return true
		}
		cn = strings.Replace(cn, prefix, "", 1)

		num := ExtractNumber(inCard.Variation)
		if num != "" {
			cnn := cn
			// Strip last character if it's a letter
			if unicode.IsLetter(rune(cn[len(cn)-1])) {
				cnn = cn[:len(cn)-1]
			}
			// Try both simple number and original collector number
			if num != cnn && num != cn {
				return true
			}
			cn = strings.Replace(cn, num, "", 1)
		}

		if len(cn) > 0 && unicode.IsLetter(rune(cn[len(cn)-1])) {
			suffix := inCard.possibleNumberSuffix()
			if suffix != "" && !strings.HasSuffix(cn, suffix) {
				return true
			}
		}
	}
	return false
}

func lubuPrereleaseVariant(inCard *InputCard, card *Card) bool {
	if (strings.Contains(inCard.Variation, "April") || strings.Contains(inCard.Variation, "4/29")) && card.OriginalReleaseDate != "1999-04-29" {
		return true
	} else if (strings.Contains(inCard.Variation, "July") || strings.Contains(inCard.Variation, "7/4")) && card.OriginalReleaseDate != "1999-07-04" {
		return true
	}
	return false
}

func borderlessCheck(inCard *InputCard, card *Card) bool {
	if inCard.isBorderless() && card.BorderColor != BorderColorBorderless {
		return true
	} else if !inCard.isBorderless() && card.BorderColor == BorderColorBorderless && !card.HasFrameEffect(FrameEffectShowcase) {
		return true
	}
	return false
}

func showcaseCheck(inCard *InputCard, card *Card) bool {
	if inCard.isShowcase() && !card.HasFrameEffect(FrameEffectShowcase) {
		return true
	} else if !inCard.isShowcase() && card.HasFrameEffect(FrameEffectShowcase) {
		return true
	}
	return false
}

func extendedartCheck(inCard *InputCard, card *Card) bool {
	if inCard.isExtendedArt() && !card.HasFrameEffect(FrameEffectExtendedArt) {
		return true
		// BaB are allowed to have extendedart
	} else if !inCard.isExtendedArt() && card.HasFrameEffect(FrameEffectExtendedArt) && !card.HasPromoType(PromoTypeBuyABox) {
		return true
	}
	return false
}

// IKO-Style cards with different names
func reskinGodzillaCheck(inCard *InputCard, card *Card) bool {
	// Also some providers do not tag Japanese-only Godzilla cards as such
	if inCard.isReskin() && !card.HasPromoType(PromoTypeGodzilla) {
		return true
	} else if !inCard.isReskin() && !inCard.beyondBaseSet && card.HasPromoType(PromoTypeGodzilla) {
		return true
	}
	return false
}

func reskinDraculaCheck(inCard *InputCard, card *Card) bool {
	if inCard.isReskin() && !card.HasPromoType(PromoTypeDracula) {
		return true
	} else if !inCard.isReskin() && !inCard.beyondBaseSet && card.HasPromoType(PromoTypeDracula) {
		return true
	}
	return false
}

// In case there is no number information and the card may known with other names
func reskinRenameCheck(inCard *InputCard, card *Card) bool {
	if ExtractNumber(inCard.Variation) != "" || card.FlavorName == "" {
		return false
	}
	if inCard.isReskin() && !Contains(inCard.originalName, card.FlavorName) {
		return true
	} else if !inCard.isReskin() && Contains(inCard.originalName, card.FlavorName) {
		return true
	}
	return false
}

func misprintCheck(inCard *InputCard, card *Card) bool {
	// These cards are allowed to have the star at the end
	if (inCard.isBasicLand() && inCard.isJudge()) || inCard.isPrerelease() {
		return false
	}

	hasSuffix := strings.HasSuffix(card.Number, SuffixVariant) || strings.HasSuffix(card.Number, SuffixSpecial)
	if inCard.Contains("Misprint") && !hasSuffix {
		return true
	} else if !inCard.Contains("Misprint") && hasSuffix {
		return true
	}
	return false
}

func draftweekendCheck(inCard *InputCard, card *Card) bool {
	releaseOrDraft := inCard.Contains("Draft Weekend") || (inCard.Contains("Release") && !inCard.isPrerelease())
	if releaseOrDraft && !card.HasPromoType(PromoTypeDraftWeekend) {
		return true
	} else if !releaseOrDraft && card.HasPromoType(PromoTypeDraftWeekend) {
		return true
	}
	return false
}

type numberFilterCallback func(inCard *InputCard) []string

var numberFilterCallbacks = map[string]numberFilterCallback{
	// Some editions duplicate foil and nonfoil in the same set
	"7ED": duplicateEveryFoil,
	"8ED": duplicateEveryFoil,
	"9ED": duplicateEveryFoil,

	// These editions duplicate foil and nonfoil for some cards only
	"10E": duplicateSomeFoil,
	"UNH": duplicateSomeFoil,
	"FRF": duplicateSomeFoil,
	"ONS": duplicateSomeFoil,
	"THB": duplicateSomeFoil,
	"STX": duplicateSomeFoil,
	"SHM": duplicateSomeFoil,
	"DKM": duplicateSomeFoil,

	// Intro lands from these sets when non-fullart always have this
	"ZEN": duplicateBasicLands,
	"BFZ": duplicateBasicLands,
	"OGW": duplicateBasicLands,

	// JPN planeswalkers
	"WAR":  duplicateJPNPlaneswalkers,
	"PWAR": duplicateJPNPlaneswalkers,
	"DD2":  duplicateJPNPlaneswalkers,

	// 40K could have numbers reported alongside the surge tag
	"40K": duplicateSomeFoil,

	// This is a mess
	"SLD": duplicateSLD,
}

func duplicateEveryFoil(inCard *InputCard) []string {
	if inCard.Foil {
		return []string{SuffixSpecial}
	}
	return nil
}

func duplicateSomeFoil(inCard *InputCard) []string {
	if inCard.Foil {
		return []string{SuffixSpecial, ""}
	}
	return nil
}

func duplicateBasicLands(inCard *InputCard) []string {
	if inCard.isBasicNonFullArt() {
		return []string{"a"}
	}
	return nil
}

func duplicateJPNPlaneswalkers(inCard *InputCard) []string {
	if inCard.isJPN() {
		return []string{SuffixSpecial, "s" + SuffixSpecial}
	}
	return nil
}

func duplicateSLD(inCard *InputCard) []string {
	if inCard.isStepAndCompleat() {
		return []string{SuffixPhiLow, ""}
	}

	if inCard.isEtched() || inCard.isThickDisplay() || inCard.Foil {
		return []string{SuffixSpecial, ""}
	}

	return nil
}
