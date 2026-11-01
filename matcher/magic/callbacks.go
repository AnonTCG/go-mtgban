package magic

import (
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/mtgban/go-mtgban/matcher"
)

type cardFilterCallback func(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool

type promoTypeElement struct {
	// Name of the promo type to validate
	PromoType string

	// Validity date of the check, skipped if set is published before this
	ValidDate time.Time

	// Tag function
	TagFunc func(b *matcher.Backend, inCard *matcher.InputCard) bool

	// Simple tags to check, if TagFunc is not set
	Tags []string

	// Whether certain promos are not tagged, and are selected as wildcards
	CanBeWild bool

	// Printings the denial stands aside for. A listing silent about the
	// treatment keeps them, though one that names it still narrows to
	// them: a showcase printing is borderless without the border being
	// what a shop calls it.
	SilentKeeps func(card *matcher.Card) bool

	// Whether the tag is only ever read as a claim and never as a denial:
	// such an element narrows to the treatment it is told about and stands
	// aside when the listing says nothing.
	//
	// Set this only where a shop can be expected to stay silent about the
	// treatment on a card that has it. Where a shop that stocks one sells
	// the plain printing beside it and spells out which is which, silence
	// is evidence, and forgiving it costs the spelled-out listing its own
	// printing to the listing that named nothing.
	OnlyWhenNamed bool
}

// carriedBy reports whether the printing wears the treatment this element
// names.
func (element promoTypeElement) carriedBy(card *matcher.Card) bool {
	return card.HasPromoType(element.PromoType)
}

// claimedBy reports whether the listing names the treatment this element
// stands for.
func (element promoTypeElement) claimedBy(b *matcher.Backend, inCard *matcher.InputCard) bool {
	if element.TagFunc != nil {
		return element.TagFunc(b, inCard)
	}
	return slices.ContainsFunc(element.Tags, inCard.Contains)
}

// vetoes reports whether the element rules this printing out, reading the
// claim and the silence the one way. It is what the stages that run after
// the filters answering a silence ask, where an element the promo pass
// carries claim-only still has its denial to state.
func (element promoTypeElement) vetoes(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if element.claimedBy(b, inCard) {
		return !element.carriedBy(card)
	}
	return element.carriedBy(card) && (element.SilentKeeps == nil || !element.SilentKeeps(card))
}

// The treatments the catalog files outside the promo types: the border on
// the printing, the frame beside it. A shop names them the way it names
// every other treatment it sells a card under, so the promo pass reads the
// claim with the rest, and the stage that runs once the filters answering a
// silence have spoken reads the denial.
var (
	borderlessTreatment = promoTypeElement{
		OnlyWhenNamed: true,
		PromoType:     PromoTypeBorderless,
		TagFunc: func(b *matcher.Backend, inCard *matcher.InputCard) bool {
			return isBorderless(inCard)
		},
		// A showcase printing is borderless without the border being what a
		// shop calls it.
		SilentKeeps: func(card *matcher.Card) bool {
			return card.HasFrameEffect(FrameEffectShowcase)
		},
	}

	// Some old full art promos wear the extended art frame in a sense modern
	// Extended Art is not, so the claim is read only from the date the two
	// stopped meaning different things.
	extendedArtTreatment = promoTypeElement{
		OnlyWhenNamed: true,
		PromoType:     PromoTypeExtendedArt,
		ValidDate:     PromosForEverybodyYay,
		TagFunc: func(b *matcher.Backend, inCard *matcher.InputCard) bool {
			return isExtendedArt(inCard)
		},
		SilentKeeps: func(card *matcher.Card) bool {
			return card.HasPromoType(PromoTypeBuyABox)
		},
	}

	// The showcase frame states itself both ways with nothing to stand
	// aside for.
	showcaseTreatment = promoTypeElement{
		OnlyWhenNamed: true,
		PromoType:     PromoTypeShowcase,
		TagFunc: func(b *matcher.Backend, inCard *matcher.InputCard) bool {
			return isShowcase(inCard)
		},
	}
)

var setExtensionTreatment = promoTypeElement{
	PromoType:     PromoTypeSetExtension,
	Tags:          []string{"Set Extension"},
	OnlyWhenNamed: true,
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
		TagFunc: func(b *matcher.Backend, inCard *matcher.InputCard) bool {
			return IsPromoPack(inCard)
		},
	},
	{
		PromoType: PromoTypeSChineseAltArt,
		TagFunc: func(b *matcher.Backend, inCard *matcher.InputCard) bool {
			return isChineseAltArt(inCard)
		},
	},
	{
		PromoType: PromoTypeBuyABox,
		// After ZNR buy-a-box is also present in main set
		ValidDate: BuyABoxNotUniqueDate,
		TagFunc: func(b *matcher.Backend, inCard *matcher.InputCard) bool {
			return isBaB(inCard) || isRelease(inCard)
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
		TagFunc: func(b *matcher.Backend, inCard *matcher.InputCard) bool {
			// A lot of providers don't tag SLD cards as Galaxy, but just foil
			// (same for RainbowFoil), so this check essentially makes the test
			// pass, and let filtering continue elsewhere
			if isSecretLair(inCard) &&
				b.HasPrinting(inCard.Name, "promo_type", PromoTypeGalaxyFoil, "SLD") {
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
		TagFunc: func(b *matcher.Backend, inCard *matcher.InputCard) bool {
			return isSurgeFoil(inCard)
		},
	},
	{
		PromoType: PromoTypeStepAndCompleat,
		Tags:      []string{"Compleat"},
	},
	{
		PromoType: PromoTypeConcept,
		Tags:      []string{"Concept"},
	},
	{
		PromoType: PromoTypeOilSlick,
		TagFunc: func(b *matcher.Backend, inCard *matcher.InputCard) bool {
			return isOilSlick(inCard)
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
		TagFunc: func(b *matcher.Backend, inCard *matcher.InputCard) bool {
			return isSerialized(inCard)
		},
	},
	{
		PromoType: PromoTypeDoubleRainbow,
		Tags:      []string{"Double Rainbow"},
	},
	{
		PromoType: PromoTypeConfettiFoil,
		Tags:      []string{"Confetti"},
	},
	{
		// Secret Lair sells its dazzle foils as the Pool Party edition
		PromoType: PromoTypeDazzleFoil,
		Tags:      []string{"Dazzle", "Pool Party"},
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
		PromoType:     PromoTypeSilverScroll,
		Tags:          []string{"Silver Scroll"},
		OnlyWhenNamed: true,
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
		PromoType: PromoTypeFacetFoil,
		Tags:      []string{"Facet"},
	},
	{
		PromoType: PromoTypeFractureFoil,
		// The only tag that is part of a set name, so the name is not a claim
		TagFunc: func(b *matcher.Backend, inCard *matcher.InputCard) bool {
			setName := matcher.Normalize("Reality Fracture")
			for _, field := range []string{inCard.Variation, inCard.Edition} {
				text := strings.ReplaceAll(matcher.Normalize(field), setName, "")
				if matcher.Contains(text, "Fracture") || matcher.Contains(text, "Fractal") {
					return true
				}
			}
			return false
		},
	},
	{
		PromoType: PromoTypeCosmicFoil,
		Tags:      []string{"Cosmic"},
	},
	{
		PromoType: PromoTypeSingularityFoil,
		Tags:      []string{"Singularity"},
	},
	{
		PromoType: PromoTypeManaFoil,
		Tags:      []string{"Mana Foil"},
	},
	{
		PromoType: PromoTypeFirstPlaceFoil,
		Tags:      []string{"First Place Foil"},
	},
	{
		PromoType: PromoTypeDragonScaleFoil,
		Tags:      []string{"Dragon Scale"},
	},
	{
		PromoType: PromoTypeChocoboTrackFoil,
		Tags:      []string{"Chocobo Track"},
	},
	{
		// The four TMT headliners are the only ones a storefront spells
		// out, and it calls them by the signature printed on them
		PromoType:     PromoTypeHeadliner,
		Tags:          []string{"Headliner", "Gold Stamped"},
		OnlyWhenNamed: true,
	},
	{
		PromoType:     PromoTypeNeonInk,
		Tags:          []string{"Neon Ink"},
		OnlyWhenNamed: true,
	},
	{
		PromoType:     PromoTypeNeonInkBlue,
		Tags:          []string{"Neon Ink Blue"},
		OnlyWhenNamed: true,
	},
	{
		PromoType:     PromoTypeNeonInkGreen,
		Tags:          []string{"Neon Ink Green"},
		OnlyWhenNamed: true,
	},
	{
		PromoType:     PromoTypeNeonInkPink,
		Tags:          []string{"Neon Ink Pink"},
		OnlyWhenNamed: true,
	},
	{
		PromoType:     PromoTypeNeonInkYellow,
		Tags:          []string{"Neon Ink Yellow"},
		OnlyWhenNamed: true,
	},
	{
		// Claim-only: providers routinely say only "Showcase" and stay
		// silent about the Japan-exclusive variant.
		PromoType:     PromoTypeJapanShowcase,
		Tags:          []string{"Japan Showcase"},
		OnlyWhenNamed: true,
	},
	{
		// Claim-only, and "Rainbow" alone: "Double Rainbow Foil" also
		// contains it, so exclude that or every doublerainbow candidate
		// gets vetoed too.
		PromoType: PromoTypeRainbowFoil,
		TagFunc: func(b *matcher.Backend, inCard *matcher.InputCard) bool {
			return inCard.Contains("Rainbow") && !inCard.Contains("Double Rainbow")
		},
		OnlyWhenNamed: true,
	},
	{
		PromoType:     PromoTypeBeginnerBox,
		Tags:          []string{"Beginner Box"},
		OnlyWhenNamed: true,
	},
	{
		PromoType:     PromoTypeStarterCollection,
		Tags:          []string{"Starter Collection"},
		OnlyWhenNamed: true,
	},
	setExtensionTreatment,
	borderlessTreatment,
	extendedArtTreatment,
	showcaseTreatment,
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
	"SOA": japaneseCheck,
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

	"ALA": showcaseTreatment.vetoes,

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
	"FDN": {phyrexianCheck, setExtensionTwin},
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

func judgeLandCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if (inCard.Contains("14") && !strings.HasSuffix(card.Number, SuffixSpecial)) ||
		inCard.Contains("23") && strings.HasSuffix(card.Number, SuffixSpecial) {
		return true
	} else if !inCard.Contains("14") && strings.HasSuffix(card.Number, SuffixSpecial) ||
		!inCard.Contains("23") && !strings.HasSuffix(card.Number, SuffixSpecial) {
		return true
	}
	return false
}

func listNumberCompare(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	number := matcher.ExtractNumber(inCard.Variation)

	// If a number is found, check that it's matching the card number
	if number != "" {
		// Compare the number portion of the field only
		cardNumbers := strings.Split(card.Number, "-")
		listNumbers := strings.Split(number, "-")
		cardNumber := cardNumbers[len(cardNumbers)-1]
		listNumber := listNumbers[len(listNumbers)-1]
		// A misprint's listing leaves its number's dagger off, and
		// misprintCheck picks between the copies
		if inCard.Contains("Misprint") {
			cardNumber = strings.TrimSuffix(cardNumber, SuffixVariant)
		}
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
		_, err := b.GetSetByName(maybeEdition)
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

// namesSourceSet reports whether the variation names the set a PLST
// candidate was reprinted from, up to a colon subtitle and the
// singular/plural "Duel Deck(s)" split.
func namesSourceSet(variation, setName string) bool {
	v := strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(variation, ".", ""))), " ")
	if strings.HasPrefix(v, "duel deck:") || strings.HasPrefix(v, "duel deck ") {
		v = "duel decks" + v[len("duel deck"):]
	}
	if v == "" {
		return false
	}
	n := strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(setName, ".", ""))), " ")
	if v == n {
		return true
	}
	pre, _, found := strings.Cut(n, ":")
	return found && pre == v
}

// namesOtherListSet reports whether the listing names, by its code, the set
// of another List printing of the card.
func namesOtherListSet(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	fields := strings.Fields(inCard.Variation)
	for _, other := range b.MatchInSet(inCard.Name, "PLST") {
		code, _, found := strings.Cut(other.Number, "-")
		if found && other.Number != card.Number && slices.Contains(fields, code) {
			return true
		}
	}
	return false
}

func listEditionCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	var setName string

	cardNumbers := strings.Split(card.Number, "-")
	code := cardNumbers[0]
	cardNumber := cardNumbers[len(cardNumbers)-1]
	set, err := b.GetSet(code)
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

	// A listing that explicitly asks for the alternate-art printing can't
	// be answered by the List's reprint when the origin set tells the two
	// apart by a starred number (Planeshift's Ertai, the Corrupted): the
	// List carries one entry for the card, not one per printing, so it
	// cannot honor the distinction and must stand aside for the origin
	// set's own starred candidate, which a filter of its own (PLS's
	// altArtCheck) already narrows to correctly.
	if isGenericAltArt(inCard) && len(b.MatchInSetNumber(inCard.Name, code, cardNumber+SuffixSpecial)) != 0 {
		return true
	}

	switch inCard.Name {
	case "Phantom Centaur",
		"Arcane Teachings",
		"Grizzly Fate":
		return misprintCheck(b, inCard, card)
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
			ids, _ := b.SearchEquals(card.Name)
			for _, id := range ids {
				co, cerr := b.GetUUID(id)
				if cerr == nil && co.SetCode == code && co.HasPromoType(PromoTypeGameDay) {
					return false
				}
			}
		}

		if strings.Contains(inCard.Variation, "vs.") {
			setName = strings.TrimPrefix(setName, "Duel Decks: ")
		}

		if !inCard.Contains(code) && !inCard.Contains(setName) &&
			!namesSourceSet(inCard.Variation, setName) &&
			EditionTable[inCard.Variation] != setName {
			// This chunk is needed in case there was a plain number already
			// processed in the previous step
			number := matcher.ExtractNumber(inCard.Variation)

			cardNumbers := strings.Split(card.Number, "-")
			listNumbers := strings.Split(number, "-")
			cardNumber := cardNumbers[len(cardNumbers)-1]
			listNumber := listNumbers[len(listNumbers)-1]
			// All promos have the same number, so trust the filtering above,
			// unless the listing names the other List printing sharing it
			if cardNumber == listNumber && !strings.HasSuffix(setName, "Promos") &&
				!namesOtherListSet(b, inCard, card) {
				return false
			}

			return true
		}
	}

	return false
}

// setExtensionTwin reports the extension printing of a card whose listing
// does not call for it. Foundations repeats cards of the set under new
// numbers, and a shop that does not say so means the original.
func setExtensionTwin(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	return setExtensionTreatment.carriedBy(card) && !setExtensionTreatment.claimedBy(b, inCard)
}

func phyrexianCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if isPhyrexian(inCard) && card.Language != LanguagePhyrexian {
		return true
	} else if !isPhyrexian(inCard) && card.Language == LanguagePhyrexian {
		return true
	}
	return false
}

// Handle full vs nonfull art basic land
func fullartCheckForBasicLands(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if isBasicFullArt(inCard) && !card.IsFullArt {
		return true
	} else if isBasicNonFullArt(inCard) && card.IsFullArt {
		return true
	}
	return false
}

func lotrTripleFiltering(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	switch card.Name {
	case "Delighted Halfling",
		"Lobelia Sackville-Baggins",
		"Frodo Baggins",
		"Bilbo, Retired Burglar",
		"Gandalf, Friend of the Shire",
		"Wizard's Rockets":
		num := matcher.ExtractNumber(inCard.Variation)
		if num != "" && (matcher.Contains(inCard.Edition, "Prerelease") || matcher.Contains(inCard.Edition, "Promo")) {
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

func lightDarkManaCost(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if isARNLightMana(inCard) && !strings.HasSuffix(card.Number, SuffixVariant) {
		return true
	} else if (isARNDarkMana(inCard) || inCard.Variation == "") && strings.HasSuffix(card.Number, SuffixVariant) {
		return true
	}
	return false
}

func femVariantInArtist(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	// Since the check is field by field Foglio may alias Phil or Kaja
	if strings.Contains(inCard.Variation, "Foglio") {
		inCard.Variation = strings.Replace(inCard.Variation, "Phil Foglio", "PhilFoglio", 1)
		inCard.Variation = strings.Replace(inCard.Variation, "Kaja Foglio", "KajaFoglio", 1)
	}
	return variantInArtistOrFlavor(b, inCard, card)
}

func variantInArtistOrFlavor(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
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
		if matcher.Contains(flavor, field) || matcher.Contains(card.Artist, field) {
			found = true
			break
		}
	}

	if !found {
		// If not found double check if variation contains the same number suffix
		numberSuffix := possibleNumberSuffix(inCard)
		if numberSuffix == "" || (numberSuffix != "" && !strings.HasSuffix(card.Number, numberSuffix)) {
			return true
		}
	}

	return false
}

// Check watermark when variation has no number information
func variantInWatermark(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	// Skip the check if this tag is empty, so that users can notice there is an aliasing problem
	if inCard.Variation == "" {
		return true
	}
	if !matcher.Contains(inCard.Variation, card.Watermark) {
		return true
	}
	return false
}

// Foil-only-booster cards, non-special version has both foil and non-foil
func altArtCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if isGenericAltArt(inCard) && !strings.HasSuffix(card.Number, SuffixSpecial) {
		return true
	} else if !isGenericAltArt(inCard) && strings.HasSuffix(card.Number, SuffixSpecial) {
		return true
	}
	return false
}

// Foil-only-booster cards, non-special version only have non-foil
// (only works if card has no other duplicates within the same edition)
func foilCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if inCard.Foil && card.HasFinish(matcher.FinishNonfoil) {
		return true
	} else if !inCard.Foil && card.HasFinish(matcher.FinishFoil) {
		return true
	}
	return false
}

// This is a terrible hack because the number check is disabled
// (Modern Horizons THREE Commander contains the same number of this card)
// and there are several variants of this card (Satya), so we cannot
// enable the etched check for *all* of them
func foilCheckM3C(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if card.Number == "3" || card.Number == "23" {
		return etchedCheck(b, inCard, card)
	}
	if inCard.Foil && card.HasFinish(matcher.FinishNonfoil) {
		return true
	} else if !inCard.Foil && card.HasFinish(matcher.FinishFoil) {
		return true
	}
	return false
}

func etchedCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if inCard.IsEtched() && !card.HasFinish(matcher.FinishEtched) {
		return true
		// Some thick display cards are not marked as etched
	} else if !inCard.IsEtched() && !isThickDisplay(inCard) && card.HasFinish(matcher.FinishEtched) {
		return true
	}
	return false
}

func thickDisplayCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if isThickDisplay(inCard) && !card.HasPromoType(PromoTypeThickDisplay) {
		return true
	} else if !isThickDisplay(inCard) && card.HasPromoType(PromoTypeThickDisplay) {
		return true
	}
	return false
}

// Single letter variants
func singleLetterVariant(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	numberSuffix := possibleNumberSuffix(inCard)
	if len(card.Variations) > 0 && numberSuffix == "" {
		numberSuffix = "a"
	}
	if numberSuffix != "" && !strings.HasSuffix(card.Number, numberSuffix) {
		return true
	}
	return false
}

func deckmastersVariant(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	numberSuffix := possibleNumberSuffix(inCard)
	switch card.Name {
	case "Incinerate", "Icy Manipulator":
		inCard.Foil = inCard.Foil || inCard.Contains("Promo")
		return foilCheck(b, inCard, card)
	default:
		// Pick the first of the two if not specified
		if len(card.Variations) > 0 && numberSuffix == "" {
			numberSuffix = "a"
		}
		// Reset for lands
		if isExactBasicLand(inCard.Name) {
			numberSuffix = ""
		}
	}

	if numberSuffix != "" && !strings.HasSuffix(card.Number, numberSuffix) {
		return true
	}
	return false
}

// Variants related to flavor text presence
func portalDemoGame(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if isPortalAlt(inCard) && !strings.HasSuffix(card.Number, SuffixVariant) && !strings.HasSuffix(card.Number, "d") {
		return true
	} else if !isPortalAlt(inCard) && (strings.HasSuffix(card.Number, SuffixVariant) || strings.HasSuffix(card.Number, "d")) {
		return true
	}
	return false
}

// Launch promos within the set itself
func launchPromoInSet(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	anyAlternative := card.IsAlternative ||
		card.BorderColor == BorderColorBorderless ||
		card.HasFrameEffect(FrameEffectExtendedArt)
	if (isRelease(inCard) || isBaB(inCard)) && !anyAlternative {
		return true
	} else if !(isRelease(inCard) || isBaB(inCard)) && anyAlternative && !card.HasPromoType(PromoTypeBoosterfun) {
		return true
	}
	return false
}

// Identical cards
func variantInCommanderDeck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	// Filter only cards that may have the flag set
	hasAlternate := card.IsAlternative
	for _, id := range card.Variations {
		alt, aerr := b.GetUUID(id)
		if aerr == nil && alt.IsAlternative {
			hasAlternate = true
			break
		}
	}
	// Only check when cards do have alts, as some vendors use the
	// Variation field for unnecessary info for unrelated cards
	// Skip EA because it does not need this deduplication
	if !isExtendedArt(inCard) && !inCard.IsEtched() && hasAlternate {
		if inCard.Variation == "" && card.IsAlternative {
			return true
		} else if inCard.Variation != "" && !card.IsAlternative {
			return true
		}
	}
	return false
}

// EA cards from commander decks appear before the normal prints, BeyondBaseSet needs help
func variantBeforePlainCard(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	cn, _ := strconv.Atoi(card.Number)
	if cn > 607 && cn < 930 {
		return extendedArtTreatment.vetoes(b, inCard, card)
	}
	return false
}

// Intro/Starter deck
func starterDeckCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	isStarter := matcher.Contains(inCard.Variation, "Starter") || matcher.Contains(inCard.Variation, "Intro")
	if !isStarter && (card.HasPromoType(PromoTypeStarterDeck) || card.IsAlternative) {
		return true
	} else if isStarter && !card.HasPromoType(PromoTypeStarterDeck) && !card.IsAlternative {
		return true
	}
	return false
}

// Japanese Planeswalkers
func japaneseCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if (IsJPN(inCard) || isGenericAltArt(inCard)) && card.Language != LanguageJapanese {
		return true
	} else if !IsJPN(inCard) && !isGenericAltArt(inCard) && card.Language == LanguageJapanese {
		return true
	}
	return false
}

// Pick one of the printings in case they are not specified
func guildgateVariant(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if strings.Contains(card.Name, "Guildgate") && inCard.Variation == "" {
		cn, _ := strconv.Atoi(card.Number)
		if cn%2 == 0 {
			return true
		}
	}
	return false
}

// Due to the WPN lands
func wpnCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if isWPNGateway(inCard) && !card.HasPromoType(PromoTypeWPN) {
		return true
	} else if !isWPNGateway(inCard) && card.HasPromoType(PromoTypeWPN) {
		return true
	}
	return false
}

// Handle the different Attractions
func attractionVariant(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	tag := AttractionLights(card)
	if tag != "" && (strings.Contains(inCard.Variation, "/") || strings.Contains(inCard.Variation, "-")) {
		variation := strings.Replace(inCard.Variation, " ", "", -1)
		variation = strings.Replace(variation, "-", "/", -1)
		if variation != tag {
			return true
		}
	}
	switch card.Name {
	case "Space Beleren",
		"Comet, Stellar Pup":
		if isBorderless(inCard) && !isGalaxyFoil(inCard) {
			if card.HasPromoType(PromoTypeGalaxyFoil) {
				return true
			}
		} else if isGalaxyFoil(inCard) && !isBorderless(inCard) {
			if card.BorderColor == BorderColorBorderless {
				return true
			}
		}
	default:
		if !isBorderless(inCard) && !isGalaxyFoil(inCard) &&
			slices.Contains(card.Types, "Land") &&
			card.BorderColor == BorderColorBorderless &&
			card.HasPromoType(PromoTypeGalaxyFoil) {
			return true
		}
	}
	return false
}

func shatteredCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	isShattered := inCard.Contains("Shattered") || inCard.Contains("Borderless")
	if isShattered && !card.HasFrameEffect(FrameEffectShattered) {
		return true
	} else if !isShattered && card.HasFrameEffect(FrameEffectShattered) {
		return true
	}
	return false
}

// This check skips serialized cards as their collector numbers would not match
func schematicCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
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

func animeCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
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

func serialCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if isSerialized(inCard) && !card.HasPromoType(PromoTypeSerialized) {
		return true
	} else if !isSerialized(inCard) && card.HasPromoType(PromoTypeSerialized) {
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

func retroCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	return retroCheckInternal(isRetro(inCard) || inCard.BeyondBaseSet, card.FrameVersion)
}

// This edition has retro-only promotional cards, but most
// providers only tag the promo type, instead of the frame
func babOrBuyaboxRetroCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	return retroCheckInternal(isBundle(inCard) || isBaB(inCard), card.FrameVersion)
}

func releaseRetroCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	return retroCheckInternal(isRetro(inCard) || isRelease(inCard), card.FrameVersion)
}

// Foil cards which exist *only* as misprints
func foilMisprint(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if !inCard.Foil {
		return strings.HasSuffix(card.Number, SuffixSpecial)
	}

	// Get number in case there is no EA information available
	maybeNumber := matcher.ExtractNumber(inCard.Variation)

	switch card.Name {
	case "Temple of Abandon":
		if isExtendedArt(inCard) || strings.HasPrefix(maybeNumber, "347") {
			return !strings.HasSuffix(card.Number, SuffixSpecial)
		}
	case "Strict Proctor":
		if !isExtendedArt(inCard) || strings.HasPrefix(maybeNumber, "33") {
			return !strings.HasSuffix(card.Number, SuffixSpecial)
		}
	case "Reflecting Pool":
		return !strings.HasSuffix(card.Number, SuffixSpecial)
	}
	return strings.HasSuffix(card.Number, SuffixSpecial)
}

func nodateMisprint(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
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

func laquatusMisprint(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	switch card.Name {
	case "Laquatus's Champion":
		if matcher.Contains(inCard.Variation, "dark") {
			return card.Number != "67†a"
		}
		if matcher.Contains(inCard.Variation, "misprint") {
			return card.Number != "67†"
		}
		return card.Number != "67"
	}
	return false
}

func sldVariant(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	switch card.Name {
	case "Geralf's Messenger":
		return retroCheckInternal(card.Number == "887", card.FrameVersion)
	case "Demonlord Belzenlok",
		"Griselbrand",
		"Liliana's Contract",
		"Kothophed, Soul Hoarder",
		"Razaketh, the Foulblooded":
		num, _ := strconv.Atoi(matcher.ExtractNumberValue(card.Number))
		if num < 200 {
			result := strings.HasSuffix(card.Number, SuffixSpecial)
			if inCard.IsEtched() {
				result = !result
			}
			return result
		}
	case "Plague Sliver",
		"Shadowborn Apostle",
		"Toxin Sliver",
		"Virulent Sliver":
		result := strings.HasSuffix(card.Number, SuffixPhi)
		if isStepAndCompleat(inCard) {
			result = !result
		}
		return result
	case "Blasphemous Act":
		if card.Number == "322" {
			return foilCheck(b, inCard, card)
		}
		result := strings.HasSuffix(card.Number, SuffixSpecial)
		if inCard.Foil {
			result = !result
		}
		return result
	case "Okaun, Eye of Chaos",
		"Zndrsplt, Eye of Wisdom":
		result := strings.HasSuffix(card.Number, SuffixSpecial)
		if isThickDisplay(inCard) {
			result = !result
		}
		return result
	case "Command Tower", "Food",
		"Mogis, God of Slaughter",
		"Lava Dart", "Monastery Swiftspear", "Soul-Scar Mage", "Underworld Breach", "Mishra's Bauble",
		"Chain Lightning", "Dragon's Rage Channeler", "Lava Spike", "Rift Bolt", "Skewer the Critics":
		if !inCard.Contains(matcher.ExtractNumberValue(card.Number)) {
			return true
		}
	}

	return foilCheck(b, inCard, card)
}

func wcdNumberCompare(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	prefix, sideboard := worldChampPrefix(inCard)
	wcdNum := ExtractWCDNumber(inCard.Variation, prefix, sideboard)

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
		// ExtractNumber reads the string it is given and nothing else, so
		// the set the deck's card came from is dropped here instead.
		variation := dropSetCodes(b, inCard.Variation)
		// A listing naming only the player says nothing either way about
		// the sideboard, and a deck can hold the card there alone - Leon
		// Lindback's City of Brass is ll112sb and nothing else. Let the
		// player decide it, and leave the rest to aliasing.
		named := matcher.ExtractNumber(variation) != ""
		if sideboard && !strings.HasSuffix(cn, "sb") {
			return true
		} else if !sideboard && named && strings.HasSuffix(cn, "sb") {
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

		num := matcher.ExtractNumber(variation)
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
			suffix := possibleNumberSuffix(inCard)
			if suffix != "" && !strings.HasSuffix(cn, suffix) {
				return true
			}
		}
	}
	return false
}

func lubuPrereleaseVariant(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if (strings.Contains(inCard.Variation, "April") || strings.Contains(inCard.Variation, "4/29")) && card.OriginalReleaseDate != "1999-04-29" {
		return true
	} else if (strings.Contains(inCard.Variation, "July") || strings.Contains(inCard.Variation, "7/4")) && card.OriginalReleaseDate != "1999-07-04" {
		return true
	}
	return false
}

// IKO-Style cards with different names
func reskinGodzillaCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	// Also some providers do not tag Japanese-only Godzilla cards as such
	if isReskin(inCard) && !card.HasPromoType(PromoTypeGodzilla) {
		return true
	} else if !isReskin(inCard) && !inCard.BeyondBaseSet && card.HasPromoType(PromoTypeGodzilla) {
		return true
	}
	return false
}

func reskinDraculaCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if isReskin(inCard) && !card.HasPromoType(PromoTypeDracula) {
		return true
	} else if !isReskin(inCard) && !inCard.BeyondBaseSet && card.HasPromoType(PromoTypeDracula) {
		return true
	}
	return false
}

// In case there is no number information and the card may known with other names
func reskinRenameCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	if matcher.ExtractNumber(inCard.Variation) != "" || card.FlavorName == "" {
		return false
	}
	if isReskin(inCard) && !matcher.Contains(inCard.OriginalName, card.FlavorName) {
		return true
	} else if !isReskin(inCard) && matcher.Contains(inCard.OriginalName, card.FlavorName) {
		return true
	}
	return false
}

func misprintCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	// These cards are allowed to have the star at the end
	if (isExactBasicLand(inCard.Name) && isJudge(inCard)) || IsPrerelease(inCard) {
		return false
	}

	// The star is not always a misprint's: where a set filed the foil as a
	// card of its own, the twin's number wears one too, and dropping it
	// takes the foil printing of an ordinary card
	hasSuffix := strings.HasSuffix(card.Number, SuffixVariant) || strings.HasSuffix(card.Number, SuffixSpecial)
	if inCard.Contains("Misprint") && !hasSuffix {
		return true
	} else if !inCard.Contains("Misprint") && hasSuffix && !b.IsFinishTwin(card.UUID) {
		return true
	}
	return false
}

func draftweekendCheck(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) bool {
	releaseOrDraft := inCard.Contains("Draft Weekend") || (inCard.Contains("Release") && !IsPrerelease(inCard))
	if releaseOrDraft && !card.HasPromoType(PromoTypeDraftWeekend) {
		return true
	} else if !releaseOrDraft && card.HasPromoType(PromoTypeDraftWeekend) {
		return true
	}
	return false
}

type numberFilterCallback func(inCard *matcher.InputCard) []string

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

func duplicateEveryFoil(inCard *matcher.InputCard) []string {
	if inCard.Foil {
		return []string{SuffixSpecial}
	}
	return nil
}

func duplicateSomeFoil(inCard *matcher.InputCard) []string {
	if inCard.Foil {
		return []string{SuffixSpecial, ""}
	}
	return nil
}

func duplicateBasicLands(inCard *matcher.InputCard) []string {
	if isBasicNonFullArt(inCard) {
		return []string{"a"}
	}
	return nil
}

func duplicateJPNPlaneswalkers(inCard *matcher.InputCard) []string {
	if IsJPN(inCard) {
		return []string{SuffixSpecial, "s" + SuffixSpecial}
	}
	return nil
}

func duplicateSLD(inCard *matcher.InputCard) []string {
	if isStepAndCompleat(inCard) {
		return []string{SuffixPhi, ""}
	}

	if inCard.IsEtched() || isThickDisplay(inCard) || inCard.Foil {
		return []string{SuffixSpecial, ""}
	}

	return nil
}
