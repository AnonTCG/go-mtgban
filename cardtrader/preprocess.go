package cardtrader

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/matcher/magic"
)

// reprintedPrereleaseShelves are the prerelease shelves Card Trader also files
// the set's reprint slots under, though no prerelease copy of those was ever
// printed. Only the promo set named here says which cards the shelf really
// holds.
var reprintedPrereleaseShelves = map[string]string{
	"Modern Horizons 2 Prerelease":    "PMH2",
	"Modern Horizons 3 Prerelease":    "PMH3",
	"March of the Machine Prerelease": "PMOM",
}

// tcgplayerIDWins lists the blueprints whose scryfall id Card Trader copied
// from a sibling blueprint of the same name and number, leaving the TCGplayer
// id as the only one naming the printing the blueprint sells.
var tcgplayerIDWins = map[int]bool{
	62502:  true, // Bombardment, carries the "No PW Symbol" blueprint's id
	155482: true, // Illuminate History, carries the Prerelease blueprint's id
	416464: true, // Aerid Konstrari, Reality Fracture Promos, carries the base FRA 121's id
}

// namedID picks between the two ids a blueprint carries, preferring the
// scryfall one as before and only weighing the blueprint's own wording where
// the vendor's ids contradict each other.
//
// An id is the surest thing a storefront publishes, but the two are not
// checked against each other on the way in and a shelf can be filed a card
// off: Card Trader's eight Strixhaven promos each carry the scryfall id of
// the card before them, so "Exponential Growth" priced as Ecological
// Appreciation. It runs the other way too - two blueprints carry a
// TCGplayer id belonging to another card - so neither space can simply win.
//
// Where the ids name two different cards the blueprint's own name is the
// third thing said about it, and the id it agrees with is the one to keep.
// Where they name two printings of one card, the collector number says which
// one, and failing that the English printing beats a foreign one. Where the
// wording settles nothing, which is what a pair of tokens sold under one
// blueprint looks like, the order stands and nothing changes.
func namedID(b *matcher.Backend, bp *Blueprint, scryfallID, tcgplayerID string) string {
	if scryfallID == "" {
		return tcgplayerID
	}
	if tcgplayerID == "" || scryfallID == tcgplayerID {
		return scryfallID
	}
	if tcgplayerIDWins[bp.ID] {
		return tcgplayerID
	}

	scryfallCard, err := b.GetUUID(scryfallID)
	if err != nil {
		return scryfallID
	}
	tcgplayerCard, err := b.GetUUID(tcgplayerID)
	if err != nil {
		return scryfallID
	}
	if matcher.Equals(scryfallCard.Name, tcgplayerCard.Name) {
		if preferTCGplayerPrinting(scryfallCard, tcgplayerCard, bp.Properties.Number) {
			return tcgplayerID
		}
		return scryfallID
	}

	namesScryfall := idNamesCard(scryfallCard, bp.Name)
	namesTCGplayer := idNamesCard(tcgplayerCard, bp.Name)
	if namesTCGplayer && !namesScryfall {
		return tcgplayerID
	}
	return scryfallID
}

// preferTCGplayerPrinting reports whether, of two printings of one card, the
// TCGplayer id's is the one a blueprint numbered number sells: the printing
// carrying that collector number, with a World Championship player prefix
// ("js440") set aside if neither does, and the English printing if the
// numbers settle nothing. A tie keeps the scryfall printing.
func preferTCGplayerPrinting(scryfallCard, tcgplayerCard *matcher.CardObject, number string) bool {
	number = strings.TrimLeft(number, "0")
	for _, stripPrefix := range []bool{false, true} {
		scryfallMatch := cardNumberIs(scryfallCard, number, stripPrefix)
		tcgplayerMatch := cardNumberIs(tcgplayerCard, number, stripPrefix)
		if scryfallMatch != tcgplayerMatch {
			return tcgplayerMatch
		}
	}
	return scryfallCard.Language != "English" && tcgplayerCard.Language == "English"
}

// cardNumberIs reports whether a printing carries the collector number,
// ignoring case and, when stripPrefix is set, any letters leading it.
func cardNumberIs(co *matcher.CardObject, number string, stripPrefix bool) bool {
	cardNumber := co.Number
	if stripPrefix {
		cardNumber = strings.TrimLeftFunc(cardNumber, unicode.IsLetter)
	}
	return number != "" && strings.EqualFold(cardNumber, number)
}

// idNamesCard reports whether a card is the one a blueprint's wording names.
// A storefront writes the front of a two-faced card where the catalog spells
// both halves, so the front alone stands for the whole.
func idNamesCard(co *matcher.CardObject, cardName string) bool {
	if matcher.Equals(co.Name, cardName) || matcher.Equals(co.FaceName, cardName) {
		return true
	}
	front, _, split := strings.Cut(co.Name, " // ")
	return split && matcher.Equals(front, cardName)
}

// hasShelfPromo reports whether the card has a prerelease or promo pack
// printing in its own set or that set's promo set, which is what a promo
// shelf would be selling instead of the plain card.
func hasShelfPromo(b *matcher.Backend, co *matcher.CardObject) bool {
	for _, code := range []string{co.SetCode, "P" + co.SetCode} {
		// HasPrinting widens to every printing when the set is unknown.
		if b.Sets[code] == nil {
			continue
		}
		if b.HasPrinting(co.Name, "promo_type", magic.PromoTypePrerelease, code) || magic.HasPromoPackPrinting(b, co.Name, code) {
			return true
		}
	}
	return false
}

// plstNumber returns the unique PLST collector number for cardName that
// ends in "-"+number, the shape every reprint on The List carries, or ""
// when no printing does or more than one does.
func plstNumber(b *matcher.Backend, cardName, number string) string {
	found := ""
	for _, card := range b.MatchInSet(cardName, "PLST") {
		if strings.HasSuffix(card.Number, "-"+number) {
			if found != "" {
				return ""
			}
			found = card.Number
		}
	}
	return found
}

// tokenPairNumberRes are the real shapes measured against Card Trader's
// own collector_number for a two-sided token blueprint, tried in order:
// a leading T/F/CT marker then both numbers ("T 05/20", "F 1/3",
// "CT 01/01"), both numbers then a shared total and marker ("024-027/031
// T"), and each face's own number-total-T marker separately ("05-014T /
// 03-014T"). None of these are collector numbers standing alone the way
// Card Kingdom's or Star City Games's skus embed one - Card Trader
// composites both faces' own numbers into the one field it publishes,
// which is exactly why a plain single-number anchor (the shape every
// other vendor's fallback already handles) never applied here.
var tokenPairNumberRes = []*regexp.Regexp{
	regexp.MustCompile(`^(?:T|F|CT)?\s*0*(\d+)\s*/\s*0*(\d+)$`),
	regexp.MustCompile(`^0*(\d+)-0*(\d+)/\d+\s*T$`),
	regexp.MustCompile(`^0*(\d+)-\d+T\s*/\s*0*(\d+)-\d+T$`),
}

// tokenPairNumbers parses a two-sided token blueprint's own composite
// collector_number into each face's own number. Returns ok=false for a
// shape none of tokenPairNumberRes recognizes rather than guess at one -
// refuse, don't parse a shape never measured against the real catalog.
func tokenPairNumbers(number string) (n1, n2 string, ok bool) {
	number = strings.TrimSpace(number)
	for _, re := range tokenPairNumberRes {
		if m := re.FindStringSubmatch(number); m != nil {
			return m[1], m[2], true
		}
	}
	return "", "", false
}

// ctPairSeparatorRe matches the "//" between a two-sided pairing's faces,
// tolerating Card Trader's own inconsistent spacing around it ("Hero //
// Hero", but also "Thopter //Dog" on blueprint 276925).
var ctPairSeparatorRe = regexp.MustCompile(`\S\s*//\s*\S`)

// isTwoSidedTokenBlueprint reports whether a blueprint names a two-sided
// Magic token pairing the way Card Trader's own Tokens category does.
func isTwoSidedTokenBlueprint(bp *Blueprint, cardName string) bool {
	return bp.CategoryID == CategoryMagicTokens && ctPairSeparatorRe.MatchString(cardName)
}

// Preprocess turns a blueprint into the card description the matcher takes,
// reporting an error for the blueprints that are not cards.
func Preprocess(b *matcher.Backend, bp *Blueprint) (*matcher.InputCard, error) {
	cardName := bp.Name
	edition := bp.Expansion.Name
	if code, found := reprintedPrereleaseShelves[edition]; found &&
		!b.HasPrinting(cardName, "promo_type", magic.PromoTypePrerelease, code) {
		return nil, errors.New("pass")
	}
	number := strings.TrimLeft(bp.Properties.Number, "0")
	variant := ""

	// A two-sided token sheet's product ("Beast // Plant") is exactly the
	// shape namedID below silently mis-resolves: whichever single face one
	// of the blueprint's own ids happens to name, rather than the combined
	// pairing the name actually describes - the same "silently prices the
	// whole two-sided product as if it were just the one face" risk
	// documented in cardkingdom's and starcitygames's own versions of this
	// check (see magic.MatchTokenPairing's doc comment, shared by all
	// three). Card Trader differs from both in one way worth knowing: its
	// own TCGplayerID is frequently the *pairing's own* product id
	// directly, not one face's - magic.VerifyTokenPairingFinish checks
	// that case explicitly rather than relying on namedID's fallthrough to
	// stumble onto it by accident, which it otherwise would for a
	// scryfallID-less blueprint (measured: true for most, not all).
	// Foil is always false here: a blueprint has no finish of its own,
	// multiple products of different finishes share one, so it is
	// resolved per-product in cardtrader.go instead (see foilPrintingID's
	// own derived-pairing handling).
	//
	// Gated to CategoryMagicTokens rather than every " // " blueprint:
	// checked against the live catalog (122,480 blueprints, 6,077 named
	// "X // Y") that this isn't narrower than it looks. Every blueprint
	// whose own identifiers/name actually resolve through
	// magic.MatchTokenPairing or magic.VerifyTokenPairingFinish -
	// dungeon-card pairings included ("Dungeon of the Mad Mage // Lost
	// Mine of Phandelver") - was filed under CategoryMagicTokens; nothing
	// in CategoryMagicSingles/Oversized/Sleeves/Albums/etc. resolved.
	// CategoryMagicSingles' own 2,148 " // " blueprints are ordinary
	// split/transform/DFC cards whose real name is "X // Y" ("Turn //
	// Burn"), already served correctly by namedID below; the merchandise
	// categories' hits quote a card's name as product flavor text, not
	// cards at all. Re-check this if Card Trader ever starts selling a
	// two-sided pairing shape outside Tokens.
	if isTwoSidedTokenBlueprint(bp, cardName) {
		if tcgID := magic.MatchTokenPairing(b, bp.ScryfallID, cardName, false); tcgID != "" {
			if id, err := b.MatchID(tcgID, false); err == nil {
				return &matcher.InputCard{
					ID: id, Name: cardName, Edition: edition, Variation: bp.Version,
				}, nil
			}
		}
		if verified := magic.VerifyTokenPairingFinish(b, fmt.Sprintf("%d", bp.TCGplayerProductID()), false); verified != "" {
			if id, err := b.MatchID(verified, false); err == nil {
				return &matcher.InputCard{
					ID: id, Name: cardName, Edition: edition, Variation: bp.Version,
				}, nil
			}
		}

		// Neither id resolved. Card Trader's own collector_number for a
		// two-sided token blueprint is not one face's number - it
		// composites both (see tokenPairNumbers) - so where it parses,
		// each face can be anchored by identity the same way Card
		// Kingdom's and Star City Games's sku-embedded numbers already
		// are: try the real printing mtgjson already files under one
		// combined name (magic.MatchNativeTokenPair), then the sku-
		// anchored derived-pairing fallback (magic.MatchTokenPairingBySetNumber,
		// the same mechanism both other vendors' own number-anchored
		// paths use), trying each parsed number in turn since which face
		// the first number names is not fixed across the shapes measured.
		if n1, n2, ok := tokenPairNumbers(bp.Properties.Number); ok {
			if tokenSet := magic.EditionTokenSetCode(b, edition); tokenSet != "" {
				for _, number := range []string{n1, n2} {
					if uuid := magic.MatchNativeTokenPair(b, tokenSet, number, cardName); uuid != "" {
						if id, err := b.MatchID(uuid, false); err == nil {
							return &matcher.InputCard{
								ID: id, Name: cardName, Edition: edition, Variation: bp.Version,
							}, nil
						}
					}
					if tcgID := magic.MatchTokenPairingBySetNumber(b, tokenSet, number, cardName, false); tcgID != "" {
						if id, err := b.MatchID(tcgID, false); err == nil {
							return &matcher.InputCard{
								ID: id, Name: cardName, Edition: edition, Variation: bp.Version,
							}, nil
						}
					}
				}
			}
		}

		// Still nothing - both faces' own names alone, guarded by
		// requiring the blueprint's own claimed edition to independently
		// agree with the match - see magic.MatchTokenPairingByNamesAndEdition's
		// own doc comment for why the guard is not optional (measured:
		// 11.6% of name-only matches against Card Trader's real catalog
		// would otherwise be silently wrong). Tried last: a real number
		// anchor above is strictly the safer bar when one parses.
		if tcgID := magic.MatchTokenPairingByNamesAndEdition(b, cardName, edition, false); tcgID != "" {
			if id, err := b.MatchID(tcgID, false); err == nil {
				return &matcher.InputCard{
					ID: id, Name: cardName, Edition: edition, Variation: bp.Version,
				}, nil
			}
		}
	}

	// Some, but not all, have a proper id we can reuse right away, and the
	// blueprint says which space each one lives in
	scryfallID := b.ConvertID(matcher.IDSpaceScryfall, bp.ScryfallID)
	tcgplayerID := b.ConvertID(matcher.IDSpaceTCGplayer, fmt.Sprintf("%d", bp.TCGplayerProductID()))
	id := namedID(b, bp, scryfallID, tcgplayerID)
	if id != "" {
		idEdition, idVariation := edition, bp.Version

		// Override the shelf wording once the id names a real promo, or both
		// vendor ids name the same plain card, so Match's promo-tag check on
		// the answer doesn't veto it.
		co, err := b.GetUUID(id)
		idsAgree := scryfallID != "" && scryfallID == tcgplayerID && err == nil
		if err == nil && (len(co.PromoTypes) > 0 || idsAgree) {
			probe := matcher.InputCard{Edition: idEdition, Variation: idVariation}
			vetoed := (magic.IsPrerelease(&probe) && !co.HasPromoType(magic.PromoTypePrerelease)) ||
				(magic.IsPromoPack(&probe) && !co.HasPromoType(magic.PromoTypePromoPack))
			// Agreeing ids override only where the card has no shelf promo,
			// asked last since it costs up to four lookups.
			if vetoed && (len(co.PromoTypes) > 0 || !hasShelfPromo(b, co)) {
				idEdition = co.Edition
				idVariation = ""
				if strings.Contains(bp.Version, "Etched") {
					idVariation = "Etched"
				}
			}
		}

		return &matcher.InputCard{
			ID: id,
			// Not needed, but helps debugging
			Name:    cardName,
			Edition: idEdition,
			// Needed to detect etched finish
			Variation: idVariation,
		}, nil
	}

	switch edition {
	case "Alliances", "Fallen Empires", "Homelands",
		"Guilds of Ravnica",
		"Ravnica Allegiance",
		"Kaldheim",
		"Asia Pacific Land Program",
		"European Land Program",
		"Commander's Arsenal",
		"Commander Anthology Volume II",
		"Unglued",
		"Mystical Archive: Japanese alternate-art",
		"Chronicles",
		"Chronicles Japanese",
		"Rinascimento",
		"Antiquities":
		variant = number
	case "Commander Legends: Commander Decks":
		edition = "Commander Legends"
		variant = number
	// Every blueprint here with no resolving id is a reprint Scryfall
	// deleted as never printed.
	case "Welcome Deck 2024":
		return nil, errors.New("pass")
	case "Arabian Nights":
		if strings.HasSuffix(number, "b") {
			variant = "light"
		} else if strings.Contains(number, "a") {
			variant = "dark"
		}
	case "Champions of Kamigawa":
		if cardName == "Brothers Yamazaki" {
			if bp.Version == "1" {
				variant = "160a"
			} else if bp.Version == "2" {
				variant = "160b"
			}
		}
	case "Buy a Box ", "Buy a Box",
		"Armada Comics",
		"Prerelease Promos":
		variant = edition
		if cardName == "Voldaren Estate" {
			variant = "Dracula"
		}
	case "Factory Misprints":
		variant = edition
		switch cardName {
		case "Laquatus's Champion":
			variant = "prerelease misprint"
		case "Island":
			edition = "PAL99"
		}
	case "Judge Gift Cards",
		"Wizards Play Network",
		"Arena League Promos",
		"Friday Night Magic",
		"Player Rewards Promos":
		ed, found := id2edition[bp.ID]
		if found {
			edition = ed
		}
		if bp.ID == 29063 {
			variant = "1"
		} else if bp.ID == 29053 {
			variant = "11"
		}
	case "Champs and States":
		if cardName == "Crucible of Worlds" {
			edition = "World Championship Promos"
		}
	case "Core Set 2021":
		if cardName == "Teferi, Master of Time" {
			variant = number
		}
	case "DCI Promos":
		switch cardName {
		case "Cryptic Command":
			edition = "PPRO"
		case "Flooded Strand":
			edition = "PNAT"
		}
	case "Grand Prix Promos":
		if cardName == "Wilt-Leaf Cavaliers" {
			edition = "DCI"
		}
	case "Store Championships":
		if bp.Version == "Japan Standard Cup" {
			edition = bp.Version
			variant = number
		}
	case "The List", "The List - Mystery Booster Commander Edition":
		edition = "The List"
		switch cardName {
		case "Everythingamajig", "Ineffable Blessing":
			variant = strings.Fields(bp.Version)[0]
		default:
			if len(b.MatchInSetNumber(cardName, "PLST", number)) > 0 {
				variant = number
			} else {
				n := plstNumber(b, cardName, number)
				if n != "" {
					variant = n
				}
			}
		}
	case "Mystery Booster: Convention Edition Playtest Cards":
		variant = bp.Version
	case "Modern Horizons 2",
		"Modern Horizons 1: Timeshifted":
		variant = number
		if strings.HasSuffix(variant, "e") {
			variant = strings.TrimSuffix(variant, "e")
			variant += " Etched"
		}
	case "Commander: The Lord of the Rings - Tales of Middle-earth Collectors",
		"The Lord of the Rings: Tales of Middle-earth Holiday Release":
		variant = strings.Replace(number, "s", "z", 1)
	case "Simplified Chinese Alternate Art Cards":
		switch cardName {
		case "Drudge Skeletons":
			if bp.Version != "" {
				edition = bp.Version
			}
			variant = number
			if !strings.HasSuffix(variant, "s") {
				variant += "s"
			}
		}
	default:
		if strings.HasPrefix(edition, "Secret Lair Commander Deck") {
			// An id that got this far resolved nowhere: find the List printing
			// by name and number, else refuse, since the number alone would
			// guess at another card. Only an id-less card may be an SLD print.
			hasID := bp.ScryfallID != "" || bp.TCGplayerProductID() != 0
			n := plstNumber(b, cardName, number)
			if !hasID && len(b.MatchInSetNumber(cardName, "SLD", number)) > 0 {
				edition = "Secret Lair Drop"
				variant = number
			} else if n != "" {
				edition = "The List"
				variant = n
			} else if hasID {
				return nil, fmt.Errorf("unknown printing %q for %q", edition, cardName)
			} else {
				variant = number
			}
		} else if strings.HasPrefix(edition, "Secret Lair") {
			variant = number
		} else if strings.HasSuffix(edition, "Collectors") {
			variant = number
		} else if strings.HasPrefix(edition, "WCD") ||
			strings.HasPrefix(edition, "Pro Tour 1996") {
			variant = number
			if strings.HasPrefix(variant, "sr") {
				variant = strings.Replace(variant, "sr", "shr", 1)
			}

			switch bp.ID {
			case 25481: // Scrabbling Claws
				variant = "jn237sb"
			case 25491: // Chrome Mox
				variant = "mb152"
			case 25501: // Seething Song
				variant = "ap104sb"
			case 32184: // Aura of Silence
				variant = "bh7bsb"
			case 35075: // Shatter
				variant = "gb219sb"
			}
		} else if strings.Contains(edition, "Japanese") {
			variant = "Japanese"
			if strings.Contains(edition, "Promo") {
				variant += " Prerelease"
			}
		} else if strings.HasSuffix(edition, "Promos") {
			variant = number

			switch edition {
			case "Media Inserts Promos":
				edition = "Media and Collaboration Promos"
			case "Guilds of Ravnica Promos":
				switch cardName {
				case "Attendant of Vraska",
					"Kraul Raider",
					"Precision Bolt",
					"Ral's Dispersal",
					"Ral's Staticaster",
					"Ral, Caller of Storms",
					"Vraska's Stoneglare",
					"Vraska, Regal Gorgon":
					edition = "Guilds of Ravnica"
				}
			// Starting from RNA, the collector numbers do not have the
			// right suffix any more, we need to special case a lot of cards
			case "Ravnica Allegiance Promos":
				switch cardName {
				case "Light Up the Stage",
					"Growth Spiral",
					"Mortify",
					"Rakdos Firewheeler",
					"Simic Ascendancy":
					variant = number
				case "Dovin, Architect of Law",
					"Elite Arrester",
					"Dovin's Dismissal",
					"Dovin's Automaton",
					"Domri, City Smasher",
					"Ragefire",
					"Charging War Boar",
					"Domri's Nodorog":
					edition = "Ravnica Allegiance"
				default:
					variant = number + "s"
				}
			case "War of the Spark Promos":
				switch cardName {
				case "Augur of Bolas",
					"Liliana's Triumph",
					"Paradise Druid",
					"Dovin's Veto":
					variant = number
				case "Bolas's Citadel",
					"Karn's Bastion":
					variant = number
					if bp.ID == 56810 || bp.ID == 56746 {
						variant = "Prerelease"
					} else if bp.ID == 60193 || bp.ID == 105989 {
						variant = "Promo Pack"
					}
				case "Feather, the Redeemed":
					variant = "Promo Pack"
					if bp.ID == 56782 {
						variant = "Prerelease"
					}
				case "Desperate Lunge",
					"Gideon's Battle Cry",
					"Gideon's Company",
					"Gideon, the Oathsworn",
					"Guildpact Informant",
					"Jace's Projection",
					"Jace's Ruse",
					"Jace, Arcane Strategist",
					"Orzhov Guildgate",
					"Simic Guildgate",
					"Tezzeret, Master of the Bridge":
					edition = "War of the Spark"
				default:
					variant = "Prerelease"
				}
			// This set acts mostly as a catch-all for anything prior :(
			case "Core Set 2020 Promos":
				version := matcher.ExtractNumber(strings.Replace(bp.Slug, "-", " ", -1))
				if magic.IsBasicLand(cardName) {
					edition = "M20 Promo Packs"
				} else if cardName == "Chandra's Regulator" {
					if version == "1" {
						variant = number
					} else if version == "2" {
						variant = "Promo Pack"
					} else if version == "3" {
						variant = "Prerelease"
					}
				} else if version == "1" {
					variant = "Promo Pack"
				} else if version == "2" {
					variant = "Prerelease"
				} else {
					if magic.HasPromoPackPrinting(b, cardName) {
						variant = "Promo Pack"
						if cardName == "Sorcerous Spyglass" {
							edition = "PXLN"
						}
					} else {
						edition = "Core Set 2020"
					}
				}
			case "D&D: Adventures in the Forgotten Realms Promos":
				variant = "Promo Pack"
				edition = "PAFR"
			default:
				// Expansion.Code is always blank; MTGJSON names promo sets
				// "<Set> Promos", the wording Card Trader shelves them under.
				set, err := b.GetSetByName(bp.Expansion.Name)
				if err != nil {
					return nil, err
				}
				setDate, err := time.Parse("2006-01-02", set.ReleaseDate)
				if err != nil {
					return nil, err
				}

				if setDate.After(magic.PromosForEverybodyYay) {
					notPromoPack := false
					num, convErr := strconv.Atoi(number)
					parentSet, setErr := b.GetSet(set.ParentCode)
					if convErr == nil && setErr == nil {
						notPromoPack = num > parentSet.BaseSetSize
					}

					switch {
					case magic.HasPromoPackPrinting(b, cardName, set.Code) && !notPromoPack:
						variant = "Promo Pack"
					case notPromoPack && bp.ScryfallID == "" && bp.TCGplayerProductID() == 0:
						// Only guess the base edition when the vendor gave
						// no id to distrust; an unresolved id stays refused.
						edition = strings.TrimSuffix(edition, " Promos")
					default:
						return nil, fmt.Errorf("unknown edition %q for %q", edition, cardName)
					}
				} else {
					switch edition {
					case "Hour of Devastation Promos":
						if cardName == "Nicol Bolas, God-Pharaoh" && number == "140" {
							variant = "Prerelase"
						}
					case "Dominaria Promos":
						if cardName == "Steel Leaf Champion" && bp.ID == 1833 {
							variant = "182"
						}
					}
				}
			}
		}
	}

	if magic.IsBasicLand(cardName) {
		variant = number
		switch edition {
		case "International Edition",
			"Introductory Two-Player Set",
			"Collectors’ Edition":
			return nil, errors.New("pass")
		// Some basic land foil are mapped to the Promos
		case "Guilds of Ravnica Promos",
			"Ravnica Allegiance Promos":
			edition = strings.TrimSuffix(edition, " Promos")
			if strings.HasPrefix(variant, "A") {
				edition = "GRN Ravnica Weekend"
			} else if strings.HasPrefix(variant, "B") {
				edition = "RNA Ravnica Weekend"
			}
		// Some lands have years set
		case "Arena League Promos":
			variant = matcher.ExtractYear(strings.Replace(bp.Slug, "-", " ", -1))

			switch variant {
			case "2001":
				switch cardName {
				case "Forest":
					variant = "2001 1"
				case "Mountain", "Swamp":
					variant = "2000"
				}
			case "2002":
				switch cardName {
				case "Forest":
					variant = "2001 11"
				case "Mountain", "Swamp":
					variant = "2001"
				}
			}
		case "Magic Premiere Shop":
			if number == "" {
				number = fmt.Sprint(bp.ID)
			}
			variant = pmpsTable[number]
		}
	}

	if strings.Contains(edition, "Prerelease") {
		edition = strings.Replace(edition, "Prerelease", "Promos", 1)
		variant = "Prerelease"

		switch cardName {
		case "Lu Bu, Master-at-Arms":
			edition = "Prerelease Events"
			if number == "6" {
				variant = "April"
			} else if number == "8" {
				variant = "July"
			}
		case "Chord of Calling", "Wrath of God":
			edition = "Double Masters"
			variant = number
		case "Magic Missile":
			edition = "ARF"
			variant = "401"
		}
	} else if strings.HasSuffix(edition, "Theme Deck") {
		edition = strings.TrimSuffix(edition, " Theme Deck")
	}

	if strings.Contains(bp.Version, "Etched") && !strings.Contains(variant, "Etched") {
		if variant != "" {
			variant += " "
		}
		variant += "Etched"
	}

	// Serialized uses a different suffix than scryfall
	if strings.Contains(bp.Version, "Serialized") {
		variant = "Serialized"
	}

	// Make sure the token tag is always present
	if bp.CategoryID == CategoryMagicTokens && !strings.Contains(cardName, "Token") {
		cardName += " Token"
		if variant == "" {
			variant = strings.TrimPrefix(number, "T")
		}
	}
	if bp.CategoryID == CategoryMagicOversized {
		switch {
		// Preserve the Display Commander tag
		case strings.Contains(bp.Version, "Display"):
			variant += " Display"
		// Make sure the oversize tag is always present
		case !strings.Contains(edition, "Oversize"):
			edition += " Oversize"
		}
	}

	return &matcher.InputCard{
		Name:      cardName,
		Edition:   edition,
		Variation: variant,
	}, nil
}
