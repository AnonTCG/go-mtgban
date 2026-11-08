package cardgamecorner

import (
	"cmp"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/matcher/magic"
)

var cardTable = map[string]string{
	"Fire (Fire/Ice) ":       "Fire // Ice",
	"Fire (Fire/Ice)":        "Fire // Ice",
	"Fire/Ice":               "Fire // Ice",
	"Kingsbaile Skirmisher":  "Kinsbaile Skirmisher",
	"Rough (Rough/Tumble)":   "Rough",
	"Time Shrike":            "Tine Shrike",
	"Treasure Find":          "Treasured Find",
	"Wax/Wane":               "Wax // Wane",
	"Ramirez Di Pietro":      "Ramirez DePietro",
	"Sir Shandlar di Eberyn": "Sir Shandlar of Eberyn",
	"Rohgahh di Kher":        "Rohgahh of Kher Keep",
	"El-Ajjaj":               "El-Hajjâj",
	"Immagina Fantasma":      "Phantasmal Image",
	"Distese di Llanowar":    "Llanowar Wastes",
	"Tramutante Puntatrice":  "Homing Sliver",
	"Tramutante Possente":    "Might Sliver",

	"Valentin, Dean of the Vein // Lisette, Dean of the": "Valentin, Dean of the Vein",
	"Mourning Patrol // Mourning Apparition":             "Mourning Patrol",
	"Growing Rites of Itlimoc / Itlimoc, Cradle of the":  "Growing Rites of Itlimoc",

	"Insegnamenti Mistici": "Mystical Teachings",
	"Massa Chimerica ":     "Chimeric Mass",
	"Condannare":           "Condemn",
	"Novelle":              "Tidings",
	"Torpore":              "Stupor",
	"Tattica del Cenn":     "Cenn's Tactician",
	"Aeronaut":             "Helionaut",

	"Lullamage's Familiar":    "Lullmage's Familiar",
	"Sword of Heart and Home": "Sword of Hearth and Home",
	"Allosaurus Sheperd":      "Allosaurus Shepherd",
	"Brazen Bucaneers":        "Brazen Buccaneers",

	"Bedhead Bestie":                    "Bedhead Beastie",
	"Duggan, Private Detectiva":         "Duggan, Private Detective",
	"The Cyper-Controller":              "The Cyber-Controller",
	"Lotho, Corrupt Shirrif":            "Lotho, Corrupt Shirriff",
	"Spellscorn Covern // Take It Back": "Spellscorn Coven // Take It Back",
	"Fear, Fires, Foes!":                "Fear, Fire, Foes!",
	"Cut Shot":                          "Cut Short",
	"Ambitions's Cost":                  "Ambition's Cost",
	"Inscriptiom of Abundance":          "Inscription of Abundance",
	"Ral, Caller of Storm":              "Ral, Caller of Storms",
}

var editionTable = map[string]string{
	"Apocalisse":                 "Apocalypse",
	"Ascesa Oscura":              "Dark Ascension",
	"Ascesa degli Eldrazi":       "Rise of the Eldrazi",
	"Assalto":                    "Onslaught",
	"Aurora":                     "Morningtide",
	"Battaglia per Zendikar":     "Battle for Zendikar",
	"Campioni di Kamigawa":       "Champions of Kamigawa",
	"Caos Dimensionale":          "Planar Chaos",
	"Cavalcavento":               "Weatherlight",
	"Cicatrici di Mirrodin":      "Scars of Mirrodin",
	"Commander Arsenal":          "Commander's Arsenal",
	"Congiunzione":               "Planeshift",
	"Decima Edizione":            "Tenth Edition",
	"Destino di Urza":            "Urza's Destiny",
	"Discordia":                  "Dissension",
	"Draghi di Tarkir":           "Dragons of Tarkir",
	"Era Glaciale":               "Ice Age",
	"Eredità di Urza":            "Urza's Legacy",
	"Esodo":                      "Exodus",
	"Figli degli Dei":            "Born of the Gods",
	"Flagello":                   "Scourge",
	"Fortezza":                   "Stronghold",
	"Frammenti di Alara":         "Shards of Alara",
	"Giuramento dei Guardiani":   "Oath of the Gatewatch",
	"I Khan di Tarkir":           "Khans of Tarkir",
	"Il Patto delle Gilde":       "Guildpact",
	"Invasione":                  "Invasion",
	"Irruzione":                  "Gatecrash",
	"L'Era della Rovina":         "Hour of Devastation",
	"L'Oscurità":                 "The Dark Italian",
	"La Guerra della Scintilla":  "War of the Spark",
	"Labirinto del Drago":        "Dragon's Maze",
	"Landa Tenebrosa":            "Shadowmoor",
	"Leggende":                   "Legends Italian",
	"Legioni":                    "Legions",
	"Liberatori di Kamigawa":     "Saviors of Kamigawa",
	"Luna Spettrale":             "Eldritch Moon",
	"Maschere di Mercadia":       "Mercadian Masques",
	"Mirrodin Assediato":         "Mirrodin Besieged",
	"Nona Edizione":              "Ninth Edition",
	"Nuova Phyrexia":             "New Phyrexia",
	"Odissea":                    "Odyssey",
	"Ombre su Innistrad":         "Shadows over Innistrad",
	"Ondata Glaciale":            "Coldsnap",
	"Orizzonti di Modern":        "Modern Horizons",
	"Ottava Edizione":            "Eighth Edition",
	"Profezia":                   "Prophecy",
	"Quarta Edizione":            "Fourth Edition",
	"Quinta Alba":                "Fifth Dawn",
	"Quinta Edizione":            "Fifth Edition",
	"Ravnica: Città delle Gilde": "Ravnica: City of Guilds",
	"Revised EU FBB":             "Foreign Black Border",
	"Revised EU FWB":             "Foreign White Border",
	"Riforgiare il Destino":      "Fate Reforged",
	"Rinascita di Alara":         "Alara Reborn",
	"Ritorno a Ravnica":          "Return to Ravnica",
	"Ritorno di Avacyn":          "Avacyn Restored",
	"Rivali di Ixalan":           "Rivals of Ixalan",
	"Rivolta dell'Etere":         "Aether Revolt",
	"Saga di Urza":               "Urza's Saga",
	"Sentenza":                   "Judgment",
	"Sesta Edizione":             "Classic Sixth Edition",
	"Settima Edizione":           "Seventh Edition",
	"Spirale Temporale":          "Time Spiral",
	"Tempesta":                   "Tempest",
	"Theros: Oltre la Morte":     "Theros Beyond Death",
	"Tormento":                   "Torment",
	"Traditori di Kamigawa":      "Betrayers of Kamigawa",
	"Trono di Eldraine":          "Throne of Eldraine",
	"Vespro":                     "Eventide",
	"Viaggio Verso Nyx":          "Journey into Nyx",
	"Visione Futura":             "Future Sight",
	"Visioni":                    "Visions",

	"Duel Deck: Ajani Vs Bolas":        "Duel Decks: Ajani vs. Nicol Bolas",
	"Duel Deck: Cavalieri vs Draghi":   "Duel Decks: Knights vs. Dragons",
	"Duel Deck: Elfi Vs Goblin":        "Duel Decks: Elves vs. Goblins",
	"Duel Deck: Elspeth Vs Tezzereth":  "Duel Decks: Elspeth vs. Tezzeret",
	"Duel Decks: Cavalieri vs. Draghi": "Duel Decks: Knights vs. Dragons",

	"Commander Legends: Battle for Baldur's Gate: Promos": "Battle for Baldur's Gate Promos",

	"Eight Edition":  "Eighth Edition",
	"Fifth Ediiton":  "Fifth Edition",
	"Fifth Editon":   "Fifth Edition",
	"Journey to Nyx": "Journey into Nyx",
}

func preprocess(b *matcher.Backend, card *Card, index int) (*matcher.InputCard, error) {
	if b.Game != matcher.GameMagic {
		return preprocessGame(b, card, index)
	}
	cardName := card.Name
	edition := card.Edition

	if strings.Contains(card.Extra, "pokemon") {
		return nil, matcher.ErrUnsupported
	}
	if strings.Contains(cardName, "Art Series") {
		return nil, matcher.ErrUnsupported
	}

	// Grab the image url and keep only the image name
	extra := strings.TrimSuffix(path.Base(card.Extra), path.Ext(card.Extra))

	// Circle of Protection: Red in Revised EU FWB???
	if card.Variants[index].ID == 223958 ||
		// Excruciator RAV duplicate card
		card.Variants[index].ID == 108840 {
		return nil, matcher.ErrUnsupported
	}

	isFoil := card.Variants[index].Foil == "Foil"

	// Some cards do not have an English name field
	if cardName == "" {
		cardName = card.OrigName
	}

	// A name holding an apostrophe sometimes arrives quoted the way a
	// database would quote it - wrapped in single quotes with the inner
	// apostrophe doubled. The matcher recovers the card anyway, but every
	// lookup the scraper itself runs on the name comes up empty.
	cardName = unquote(cardName)

	cn, found := cardTable[cardName]
	if found {
		cardName = cn
	}

	variation := ""
	variants := matcher.SplitVariants(cardName)
	if len(variants) > 1 {
		variation = variants[1]
	}
	cardName = variants[0]
	if variation == "" {
		variants = matcher.SplitVariants(card.OrigName)
		if len(variants) > 1 {
			variation = variants[1]
		}
	}

	// Run the lookup to catch any variant
	cn, found = cardTable[cardName]
	if found {
		cardName = cn
	}

	// The English name field is sometimes still the Italian one, or the
	// store's own misspelling. The image beside it is named after the card
	// in English, so read the name from there - but only once the published
	// one turns out to name no card at all. The store also serves the wrong
	// image now and then, and a name that does exist is the better witness.
	if !namesACard(b, cardName) {
		slugName, slugVariation := imageName(extra, card.Edition)
		if namesACard(b, slugName) {
			cardName = slugName
			if variation == "" {
				variation = slugVariation
			}
		}
	}

	cardName, edition, variation = internalPreprocess(b, cardName, edition, variation, extra)

	var id string
	switch edition {
	case "Core 2021: Extras",
		"Ikoria: Lair of Behemoths: Extras",
		"Zendikar Rising: Extras":
		id = extra
	case "Commander Anthology 2018":
		variation = card.URL
	}

	lutName, found := editionTable[edition]
	if found {
		edition = lutName
	}

	variation = basicSKUNumber(cardName, edition, variation, extra, card.SKU)

	if card.Number != "" && slices.ContainsFunc(numberedSKUPrefixes, func(prefix string) bool {
		return strings.HasPrefix(card.SKU, prefix)
	}) {
		variation = card.Number
	}

	if id == "" {
		id = imageProductID(b, card.Edition, card.Extra, cardName, edition, variation, isFoil)
	}

	return &matcher.InputCard{
		ID:        id,
		Name:      cardName,
		Variation: variation,
		Edition:   edition,
		Foil:      isFoil,
	}, nil
}

// mcmImageIDRe matches the Cardmarket product id an image name carries as its
// trailing number, on Magic's Extras and Promos shelves and every One Piece
// one, where the extension is sometimes doubled as ".jpg.png".
var mcmImageIDRe = regexp.MustCompile(`[-_/](\d{5,7})(?:-\d+)?(?:\.jpe?g)?\.(?:jpe?g|png)$`)

// scryfallImageRe matches the Scryfall id an image name ends with: the whole
// uuid on older shelves, its first eight digits after the card's slug on newer
// ones.
var scryfallImageRe = regexp.MustCompile(`(?:^|_)([0-9a-f]{8}(?:-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})?)$`)

// scryfallImageID returns the Scryfall id, or its leading digits, an image
// path ends with.
func scryfallImageID(imagePath string) string {
	base := strings.TrimSuffix(path.Base(imagePath), path.Ext(imagePath))
	m := scryfallImageRe.FindStringSubmatch(base)
	if m == nil {
		return ""
	}
	return m[1]
}

// imageProductID resolves an image's Cardmarket product id or Scryfall id to
// a uuid. It is accepted only when it names the listing's card, under the
// wording's own language, and in the wording's set unless the wording failed or
// the shelf names no set - this store also reuses one image across unrelated
// products.
func imageProductID(b *matcher.Backend, rawEdition, imagePath, cardName, edition, variation string, isFoil bool) string {
	// The wording of these shelves names no set, so it lands on whichever
	// printing of the card it finds first and the image is the better witness.
	imageFirst := strings.HasPrefix(rawEdition, "Secret Lair Commander Deck")
	language := "English"
	switch rawEdition {
	case warSparkJapaneseEdition:
		language = "Japanese"
	// Most images here are named after the English printing, not the
	// Italian-only one on sale, so only an Italian hit is the listing's.
	case "Rinascimento", "Revised EU FBB", "L'Oscurità", "Leggende":
		language = "Italian"
	case "Promo", "DCI Promo", "Judge Rewards Promos":
		imageFirst = true
	}
	if cardName == "" {
		return ""
	}

	m := mcmImageIDRe.FindStringSubmatch(imagePath)
	scryfall := scryfallImageID(imagePath)
	if m == nil && scryfall == "" {
		return ""
	}

	ids, err := b.SearchContains(cardName)
	if err != nil {
		return ""
	}
	var hit *matcher.CardObject
	for _, candidate := range ids {
		co, err := b.GetUUID(candidate)
		if err != nil {
			continue
		}
		byMCM := m != nil && co.Identifiers["mcmId"] == m[1]
		byScryfall := scryfall != "" && strings.HasPrefix(co.Identifiers["scryfallId"], scryfall)
		if !byMCM && !byScryfall {
			continue
		}
		// SearchContains is a substring search: Plains also finds
		// Snow-Covered Plains, whose image a Plains listing may carry.
		if !namesFrontFace(co.Name, cardName) {
			continue
		}
		// The store sells Phyrexian printings on the English shelf, so
		// those hits are accepted too. This check, not the twin rule
		// below, keeps a Japanese twin out: PlainNumber drops "jpn".
		if co.Language != language && co.Language != magic.LanguagePhyrexian {
			continue
		}
		if hit == nil {
			hit = co
			continue
		}
		if co.SetCode != hit.SetCode || co.PlainNumber != hit.PlainNumber {
			// The id names more than a foil/star twin under this card
			// name - not trustworthy enough to pick one over the wording.
			return ""
		}
	}
	if hit == nil {
		return ""
	}

	// A snapshot ahead of Match, which is free to mutate the card it is
	// handed: the promo-tag check below has to see what the wording itself
	// claimed, not what Match rewrote it to while landing.
	claimed := matcher.InputCard{Variation: variation, Edition: edition}

	wording := &matcher.InputCard{Name: cardName, Edition: edition, Variation: variation, Foil: isFoil}
	wordingID, wordingErr := b.Match(wording)
	sameSet := false
	if wordingErr == nil {
		co, err := b.GetUUID(wordingID)
		if err == nil {
			sameSet = inSetOrPromo(b, co.SetCode, hit.SetCode)
		}
	} else {
		var alias *matcher.AliasingError
		if errors.As(wordingErr, &alias) {
			for _, probe := range alias.Probe() {
				co, err := b.GetUUID(probe)
				if err == nil && inSetOrPromo(b, co.SetCode, hit.SetCode) {
					sameSet = true
					break
				}
			}
		}
	}
	// Where the wording failed, the image's id is the only witness left. A
	// wording refused as unsupported is a verdict, not a failure.
	failed := wordingErr != nil && !errors.Is(wordingErr, matcher.ErrUnsupported)
	if !sameSet && !failed && !imageFirst {
		return ""
	}

	// A serialized print is only the one on sale where the wording says so;
	// otherwise the image names its plain twin, the same number without the
	// serial suffix.
	if hit.HasPromoType(magic.PromoTypeSerialized) && !matcher.Contains(variation+" "+edition, "serial") {
		var twin *matcher.CardObject
		for _, candidate := range ids {
			co, err := b.GetUUID(candidate)
			if err != nil || co.HasPromoType(magic.PromoTypeSerialized) {
				continue
			}
			if co.SetCode != hit.SetCode || co.PlainNumber != hit.PlainNumber || co.Language != hit.Language || !namesFrontFace(co.Name, cardName) {
				continue
			}
			twin = co
			break
		}
		if twin == nil {
			return ""
		}
		hit = twin
	}

	// Match applies this same promo-tag check once ID is set and drops the
	// listing outright on a mismatch, so failing here defers to the
	// wording instead of turning an already-landed row into a silent loss.
	if (magic.Rules{}).IsUnsupported(b, &claimed, hit, matcher.StageAnswer) {
		return ""
	}

	resolved, err := b.MatchID(hit.UUID, isFoil)
	if err != nil {
		return ""
	}
	return resolved
}

// inSetOrPromo reports whether the image's set is the wording's own, or the
// promo set decorating it: the wording sends a promo shelf to the base set.
// The other way round is not accepted, as the store shows one base image on a
// promo listing that the wording had placed correctly.
func inSetOrPromo(b *matcher.Backend, wordingSet, imageSet string) bool {
	if wordingSet == imageSet {
		return true
	}
	set, err := b.GetSet(imageSet)
	return err == nil && set.Type == "promo" && set.ParentCode == wordingSet
}

// namesFrontFace reports whether name and cardName share a front face. The
// cut at " /" reads both the store's "A / B" and the datastore's "A // B".
func namesFrontFace(name, cardName string) bool {
	front, _, _ := strings.Cut(name, " /")
	cardFront, _, _ := strings.Cut(cardName, " /")
	return matcher.Equals(front, cardFront)
}

func internalPreprocess(b *matcher.Backend, cardName, edition, variation, extra string) (string, string, string) {
	switch edition {
	case "Unlimited":
		cardName = matcher.Cut(cardName, "Unlimited")[0]
	case "Arabian Nights":
		if variation == "V.1" {
			variation = "dark"
		} else if variation == "V.2" {
			variation = "light"
		}
	case "Antiquities":
		if variation != "" {
			variation = extra
		}
	case warSparkJapaneseEdition:
		if variation == "Version 2" {
			variation = "Japanese Prerelease"
			edition = "War of the Spark Promos"
		} else {
			variation = "Japanese"
			edition = "War of the Spark"
		}
	case "Core 2020: Extras":
		edition = "PM20"
		if cardName == "Chandra's Regulator" {
			if variation == "1" {
				variation = "131"
			} else if variation == "V.2" {
				variation = "Promo Pack"
			} else if variation == "V.3" {
				variation = "Prerelease"
			}
		} else {
			switch variation {
			case "V.1":
				variation = "Promo Pack"
			case "V.2":
				variation = "Prerelease"
			default:
				if magic.HasPromoPackPrinting(b, cardName) {
					variation = "Promo Pack 2020"
					edition = "Promos"
					if cardName == "Sorcerous Spyglass" {
						edition = "PXLN"
					}
				}
			}
		}
	case "Throne of Eldraine: Promos":
		switch variation {
		case "V.1":
			variation = "Prerelease"
		case "V.2":
			variation = "Promo Pack"
		default:
			if magic.HasPromoPackPrinting(b, cardName) {
				variation = "Promo Pack ELD"
				edition = "Promos"
			}
		}
	case "Kaldheim: Extras":
		if cardName == "Vorinclex, Monstrous Raider" {
			if variation == "V.1" {
				variation = "Showcase"
			} else if variation == "V.2" {
				variation = "Phyrexian"
			}
		} else {
			if variation == "V.1" {
				variation = "Borderless"
			} else if variation == "V.2" {
				variation = "Showcase"
			}
		}
	case "Modern Horizons 2: Extras":
		// Note: order of these printing checks matters
		if magic.HasExtendedArtPrinting(b, cardName) {
			switch variation {
			case "V.1":
				variation = "Retro Frame"
			case "V.2":
				variation = "Retro Frame Foil Etched"
			case "V.3":
				variation = "Extended Art"
			}
		} else if magic.HasBorderlessPrinting(b, cardName) {
			switch variation {
			case "V.1":
				variation = "Borderless"
			case "V.2":
				variation = "Retro Frame"
				if magic.HasShowcasePrinting(b, cardName) {
					variation = "Showcase"
				}
			case "V.3":
				variation = "Retro Frame Foil Etched"
			}
		} else if magic.HasShowcasePrinting(b, cardName) {
			switch variation {
			case "V.1":
				variation = "Showcase"
			case "V.2":
				variation = "Retro Frame"
			case "V.3":
				variation = "Retro Frame Foil Etched"
			}
		} else if magic.HasRetroFramePrinting(b, cardName) {
			switch variation {
			case "V.1":
				variation = "Retro Frame"
			case "V.2":
				variation = "Retro Frame Foil Etched"
			}
		}
	// Use the tag information if available
	case "Promo":
		switch variation {
		case "War of the Spark: Extras", "Version1", "Version 2", "V.2":
			// Naming a prerelease for a card that never had one names a
			// printing that does not exist, and vetoes the image's hit.
			variation = ""
			if b.HasPrinting(cardName, "promo_type", magic.PromoTypePrerelease) {
				variation = "Prerelease"
			}
		case "Core 2020: Extras", "Version 1", "V.1":
			// Promo packs only start with Core Set 2020. For everything
			// older this tag means the plain set promo, and naming a
			// promo pack there names a printing that does not exist,
			// leaving the set promo and the prerelease to alias.
			variation = ""
			if hasPromoPack(b, cardName) {
				variation = "Promo Pack"
			}
		case "Judge Gift Program", "Judge Promo":
			switch cardName {
			case "Demonic Tutor":
				variation = "Judge 2020"
			case "Vampiric Tutor":
				variation = "Judge 2018"
			case "Vindicate",
				"Wasteland":
				if len(extra) > 5 {
					variation = extra[1:5]
					edition = "Judge"
				}
			}
		default:
			switch cardName {
			case "Sword of Dungeons & Dragons":
				edition = "H17"
			case "Ugin, the Spirit Dragon":
				// This shelf never carries the "(V.N)" suffix the store's
				// own DCI Promo shelf puts on the exact same listing, so
				// the matcher is left with a bare name shared by half a
				// dozen printings and lands on the Secret Lair Promo pair
				// every time. The image name still carries the version
				// the wording drops.
				if slugName, slugVersion := imageName(extra, edition); slugName != "" {
					if slugVersion == "V.2" {
						edition, variation = "MagicFest 2025", "6"
					} else {
						edition, variation = "Ugin's Fate", "1"
					}
				}
			}
		}
	// Use the number from extra if present, or keep the current version
	case "Unstable":
		if strings.HasPrefix(variation, "Version") && strings.HasPrefix(extra, "UST") {
			variation = strings.TrimLeft(extra[3:], "0")
		}
	case "Unsanctioned":
		if len(extra) > 3 {
			variation = strings.TrimLeft(extra[3:], "0")
		}
	// Handle wastes
	case "Oath of the Gatewatch":
		if cardName == "Wastes" {
			variation = extra
		}
	// DDA correct edition is hidden in the extra
	case "Duel Decks Anthology":
		switch {
		case strings.HasPrefix(extra, "DDA0"):
			edition = "Duel Decks Anthology: Elves vs. Goblins"
		case strings.HasPrefix(extra, "DDA1"):
			edition = "Duel Decks Anthology: Jace vs. Chandra"
		case strings.HasPrefix(extra, "DDA2"):
			edition = "Duel Decks Anthology: Divine vs. Demonic"
		case strings.HasPrefix(extra, "DDA3"):
			edition = "Duel Decks Anthology: Garruk vs. Liliana"
		}
		variation = strings.TrimLeft(extra[4:], "0")
	case "Commander Legends: Extras":
		if b.HasEtchedPrinting(cardName, "CMR") {
			variation = "etched"
		} else if magic.HasExtendedArtPrinting(b, cardName, "CMR") {
			variation = "extended art"
		}
	case "Secret Lair Drop Series":
		if strings.HasPrefix(extra, "SLD") {
			variation = strings.TrimLeft(extra[3:], "0")
		}
	default:
		// All the prerelease/promopack versions >= THB
		if base, colon, promo := promoSetBase(b, edition); promo {
			switch variation {
			case "V.1":
				variation = "Prerelease"
			case "V.2":
				// A card without its own promo-pack printing on this shelf
				// was never bundled that way; V.2 there names the bundle
				// promo instead, same as V.3 always does.
				variation = "Promo Pack"
				if !hasPromoPack(b, cardName) {
					variation = "Bundle"
					edition = base
				}
			case "V.3":
				variation = "Bundle"
				edition = base
			default:
				// Where the store spelled the colon this has stamped over
				// whatever the listing said for as long as it has existed.
				// The spelling without one is new here, and only takes a
				// listing that said nothing: the version tags the older
				// sets carry are read from the image name further down, and
				// a stamp would take them before they get there.
				if (colon || variation == "") && magic.HasPromoPackPrinting(b, cardName) {
					variation = "Promo Pack"
				}
			}
		} else if strings.HasSuffix(edition, firstPlaceSuffix) {
			// The store sells the box toppers as their own edition but
			// says nothing about the treatment, and its index counts the
			// flavours in collector-number order: the bordered one first,
			// the borderless one second. Say the number instead, so the
			// topper stops aliasing with the base printing.
			base := strings.TrimSuffix(edition, firstPlaceSuffix)
			numbers := firstPlaceNumbers(b, cardName, base)
			switch {
			case len(numbers) == 1:
				variation = strconv.Itoa(numbers[0])
			case len(numbers) == 2 && !magic.IsBasicLand(cardName):
				switch variation {
				case "V.1":
					variation = strconv.Itoa(numbers[0])
				case "V.2":
					variation = strconv.Itoa(numbers[1])
				}
			}
		}
	}

	// The store now writes "(Version N)" into the name of the older sets'
	// multi-art commons. It says that the card has more than one art, never
	// which one, and yet it counts as a variation and skips everything
	// below - taking with it the image name those editions are keyed on in
	// the variants table, the one thing that does say which. Drop the tag
	// there and let the image name answer.
	if genericVersionRe.MatchString(variation) && namesTheArt(edition, extra) {
		variation = ""
	}

	if variation == "" {
		switch {
		// Work around missing tags until (if) they add them
		case edition == "Promo":
			if strings.HasPrefix(extra, "p2019ELD") {
				internalNumber := strings.TrimLeft(extra[8:], "0")
				num, err := strconv.Atoi(internalNumber)
				if err == nil {
					if num < 69 {
						variation = "Prerelease"
					} else {
						variation = "Promo Pack"
					}
				}
			} else {
				switch cardName {
				case "Flusterstorm":
					edition = "Modern Horizons"
				case "Gideon Blackblade":
					edition = "SLD"
				case "Angelic Guardian":
					edition = "G18"
				case "Wrath of God":
					edition = "2XM"
					variation = "383"
				}
			}
		// These editions contain numbers that can be used safely
		case hasNumberedImage(edition):
			variation = extra
			internalNumber := imageNumber(extra)
			if internalNumber != "" {
				variation = internalNumber
			}
		// Same for this one, except the specifier is elsewhere
		case edition == "Core 2020: Extras":
			variation = "Promo Pack"
		// Full-art Zendikar lands
		case edition == "Zendikar":
			if magic.IsBasicLand(cardName) {
				s := strings.Fields(cardName)
				if len(s) > 1 {
					cardName = s[0]
					variation = s[1] + "a"
				}
			}
		default:
			// Try using the number, except the set code can randomly be
			// 2 to 4 characters.
			if magic.IsBasicLand(cardName) {
				number := basicImageNumber(extra)
				if number != "" {
					variation = number
				}
			}
		}
		if needsImageLookup(edition) {
			variation = extra
		}
	}

	return cardName, edition, variation
}

// basicImageNumber returns the collector number a basic land's image name or
// SKU spells after its set code, which is 2 to 4 characters long and holds a
// letter, so a Cardmarket id names none. A digit in the fourth place opens
// the number of a shorter code, so no 4-character code is read there.
func basicImageNumber(extra string) string {
	drops := []int{2, 3, 4}
	if len(extra) > 3 && extra[3] >= '0' && extra[3] <= '9' {
		drops = drops[:2]
	}
	for _, lengthToDrop := range drops {
		if len(extra) > lengthToDrop && strings.ContainsFunc(extra[:lengthToDrop], unicode.IsLetter) {
			number := strings.TrimLeft(extra[lengthToDrop:], "0")
			val, err := strconv.Atoi(number)
			if err == nil && val < 400 {
				return number
			}
		}
	}
	return ""
}

// storeOrderBasics lists the editions whose basic land SKUs count in the
// store's own order, not in collector order, so a SKU number is not trusted.
var storeOrderBasics = []string{"Collector's Edition", "Foreign Black Border"}

// basicSKUNumber answers the number a basic land is placed by. The image of a
// land is sometimes another SKU's picture, so where the SKU spells a number of
// its own it replaces the one the image gave.
func basicSKUNumber(cardName, edition, variation, extra, sku string) string {
	if !magic.IsBasicLand(cardName) || variation == "" || variation != basicImageNumber(extra) {
		return variation
	}
	if slices.Contains(storeOrderBasics, edition) {
		return variation
	}
	return cmp.Or(basicImageNumber(sku), variation)
}

// promoBasicRuns lists the promo runs whose basic land SKUs, lower-cased, end
// in the land's index in the run: the set holding the run and how it spells
// the number. The Arena League years left out count their lands another way.
var promoBasicRuns = map[string]struct{ set, numberFormat string }{
	"p2018prwk":   {"PRWK", "A%02d"},
	"p2019prwk":   {"PRW2", "B%02d"},
	"p2017gift":   {"G17", "%d"},
	"p2018m19std": {"PSS3", "%d"},
	"p2001ar":     {"PAL01", "%d"},
	"p2002ar":     {"PAL02", "%d"},
	"p2004ar":     {"PAL04", "%d"},
	"p2005ar":     {"PAL05", "%d"},
	"p2006ar":     {"PAL06", "%d"},
}

// promoBasicPlacement answers the set and number a promo run's SKU spells.
func promoBasicPlacement(sku string) (set, number string) {
	sku = strings.ToLower(sku)
	for prefix, run := range promoBasicRuns {
		index, found := strings.CutPrefix(sku, prefix)
		if !found {
			continue
		}
		n, err := strconv.Atoi(index)
		if err != nil {
			return "", ""
		}
		return run.set, fmt.Sprintf(run.numberFormat, n)
	}
	return "", ""
}

// matchBasicByImage retries a basic land the wording could not place with the
// number its SKU spells after the set code, then with the one its image names,
// then with the set and number a promo run's SKU spells. The store's version
// tag counts its own index, which is not collector order, so a number is
// trusted only where the landed card confirms it. The store's Number field is
// not read: several SKUs of one land share it. input is a copy taken before
// the first Match, which mutates it.
func matchBasicByImage(b *matcher.Backend, card *Card, input matcher.InputCard) (string, bool) {
	imageNumber := basicImageNumber(strings.TrimSuffix(path.Base(card.Extra), path.Ext(card.Extra)))
	skuNumber := basicImageNumber(card.SKU)
	if slices.Contains(storeOrderBasics, input.Edition) {
		skuNumber = ""
	}
	var attempts []matcher.InputCard
	for _, number := range []string{skuNumber, imageNumber} {
		attempt := input
		attempt.Variation = number
		attempts = append(attempts, attempt)
	}
	set, number := promoBasicPlacement(card.SKU)
	promo := input
	promo.Edition = set
	promo.Variation = number
	attempts = append(attempts, promo)

	for _, attempt := range attempts {
		number := attempt.Variation
		if number == "" {
			continue
		}
		cardID, found := matchNumbered(b, &attempt, number)
		if found {
			return cardID, true
		}
	}
	return "", false
}

// matchNumbered matches the card and answers it only where the printing it
// lands on carries the number: its own, its plain one, or the tail of a
// The List number, which the set it was printed in prefixes.
func matchNumbered(b *matcher.Backend, input *matcher.InputCard, number string) (string, bool) {
	cardID, err := b.Match(input)
	if err != nil {
		return "", false
	}
	co, err := b.GetUUID(cardID)
	if err != nil {
		return "", false
	}
	if co.Number == number || co.PlainNumber == number || strings.HasSuffix(co.Number, "-"+number) {
		return cardID, true
	}
	return "", false
}

// foilStarTwin answers a foil listing that landed on a printing sold only
// nonfoil with the foil-only printing the set numbers with a star beside it,
// which the matcher drops while the listing names no treatment.
func foilStarTwin(b *matcher.Backend, cardID string) string {
	co, err := b.GetUUID(cardID)
	if err != nil || co.Foil || co.Etched {
		return cardID
	}
	twinID, err := b.MatchID(cardID, true)
	if err != nil {
		return cardID
	}
	twin, err := b.GetUUID(twinID)
	if err != nil || !twin.Foil || twin.SetCode != co.SetCode || twin.Number != co.Number+"★" {
		return cardID
	}
	return twinID
}

// matchByListedNumber retries a card the wording could not place with the
// number the listing states, where the store's version tag counts its own
// index and the number names the printing. input is a copy taken before the
// first Match, which mutates it.
func matchByListedNumber(b *matcher.Backend, card *Card, input matcher.InputCard) (string, bool) {
	number := strings.TrimLeft(card.Number, "0")
	if number == "" {
		return "", false
	}
	input.Variation = number
	return matchNumbered(b, &input, number)
}

// genericVersionRe matches the store's bare "Version N" tag, which names no
// printing on its own.
var genericVersionRe = regexp.MustCompile(`(?i)^version\s*[0-9]+$`)

// promoSetBase answers the set a promo edition decorates, and whether the
// edition names one at all. The store spells the same thing both ways - with
// the colon its own set list uses, and without it - so the colon cannot be
// what decides.
//
// Without it, the words in front of "Promos" have to name an expansion the
// promos could decorate. Two things they must not name. An edition that names
// an event has no expansion behind it at all ("Game Day Promos", "Judge
// Rewards Promos"), and the branch below would stamp a promo pack on every
// card that has one anywhere. And an edition naming a promo set is already
// where its cards live ("San Diego Comic-Con 2013 Promos"), so decorating it
// again sends them somewhere else.
//
// The name is compared exactly rather than through Normalize, which drops a
// standalone "s": "Store Championship" would otherwise reach the promo set
// spelled "Store Championships" and take the Game Day cards with it.
func promoSetBase(b *matcher.Backend, edition string) (base string, colon bool, ok bool) {
	if base, found := strings.CutSuffix(edition, ": Promos"); found {
		return base, true, true
	}
	base, found := strings.CutSuffix(edition, " Promos")
	if !found {
		return "", false, false
	}
	set, err := b.GetSetByName(base)
	if err != nil || !strings.EqualFold(set.Name, base) || set.Type == "promo" {
		return "", false, false
	}
	return base, false, true
}

// hasPromoPack reports whether the card was ever printed in a promo pack. It
// walks the card's own printings because the name-keyed lookup answers false
// for a double-faced card the store names by its front face alone.
func hasPromoPack(b *matcher.Backend, cardName string) bool {
	if magic.HasPromoPackPrinting(b, cardName) {
		return true
	}
	printings, err := b.Printings4Card(cardName)
	if err != nil {
		return false
	}
	for _, code := range printings {
		for _, card := range b.MatchInSet(cardName, code) {
			if card.HasPromoType(magic.PromoTypePromoPack) {
				return true
			}
		}
	}
	return false
}

// needsImageLookup reports whether an edition's reprints are told apart in
// the variants table by the store's own image name rather than by any
// wording it publishes.
func needsImageLookup(edition string) bool {
	switch edition {
	case "Antiquities", "Fallen Empires", "Chronicles",
		"Alliances", "Renaissance", "Rinascimento", "Homelands":
		return true
	}
	return false
}

// hasNumberedImage reports whether an edition names its images after the set
// code and the collector number, the pair that tells its reprints apart.
func hasNumberedImage(edition string) bool {
	switch edition {
	case "Throne of Eldraine: Extras", "Theros Beyond Death: Extras",
		"Guilds of Ravnica", "Ravnica Allegiance", "Unglued", "Battlebond":
		return true
	}
	return false
}

// imageNumber returns the collector number an image name spells out, or the
// empty string when the store named that image some other way.
func imageNumber(extra string) string {
	if len(extra) <= 3 {
		return ""
	}
	number := strings.TrimLeft(extra[3:], "0")
	_, err := strconv.Atoi(number)
	if err != nil {
		return ""
	}
	return number
}

// namesACard reports whether the datastore can place the name at all. The
// printings index answers for nearly every card; the few known only by a
// flavor name reach their printing through the matcher alone, so ask it
// second rather than call those names unknown.
func namesACard(b *matcher.Backend, cardName string) bool {
	if cardName == "" {
		return false
	}
	_, err := b.Printings4Card(cardName)
	if err == nil {
		return true
	}
	_, err = b.Match(&matcher.InputCard{Name: cardName})
	return !errors.Is(err, matcher.ErrCardDoesNotExist)
}

var (
	// imageIDRe matches the product id the store hangs off the end of an
	// image name, in either the decimal or the hexadecimal shape it uses.
	imageIDRe = regexp.MustCompile(`[-_][0-9a-f]{6,}$`)
	// imageVersionRe matches the art index some image names carry.
	imageVersionRe = regexp.MustCompile(`-v([0-9]+)$`)
	// slugSeparatorRe matches everything a slug spells as a separator.
	slugSeparatorRe = regexp.MustCompile(`[^a-z0-9]+`)
)

// imageName reads the card name an image is named after, along with the art
// index the name carries when it has one. It returns an empty name when the
// store named that image some other way.
func imageName(extra, edition string) (string, string) {
	slug := extra
	id := imageIDRe.FindString(slug)
	if id == "" || !strings.ContainsAny(id, "0123456789") {
		return "", ""
	}
	slug = strings.TrimSuffix(slug, id)
	slug = strings.TrimPrefix(slug, slugify(edition)+"-")

	variation := ""
	version := imageVersionRe.FindStringSubmatch(slug)
	if version != nil {
		variation = "V." + version[1]
		slug = strings.TrimSuffix(slug, version[0])
	}
	return strings.ReplaceAll(slug, "-", " "), variation
}

// slugify spells a name the way the store spells it in an image name.
func slugify(name string) string {
	return strings.Trim(slugSeparatorRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

// unquote returns the name a quoted field means, undoing the wrapping single
// quotes and the doubled apostrophe inside them.
func unquote(cardName string) string {
	if !strings.HasPrefix(cardName, "'") || !strings.HasSuffix(cardName, "'") {
		return cardName
	}
	return strings.ReplaceAll(strings.Trim(cardName, "'"), "''", "'")
}

// firstPlaceSuffix is how the store names the box topper edition that sits
// beside a set.
const firstPlaceSuffix = ": First-Place"

// warSparkJapaneseEdition is the one shelf whose listings are Japanese
// rather than English.
const warSparkJapaneseEdition = "War of the Spark: Japanese Alternate-Art Planeswalkers"

// firstPlaceNumbers returns the collector numbers of the card's first-place
// foil printings in the set the edition names, lowest first. It returns
// nothing when any of them is numbered in a way the order cannot be read
// from.
func firstPlaceNumbers(b *matcher.Backend, cardName, edition string) []int {
	set, err := b.GetSetByName(edition)
	if err != nil {
		return nil
	}
	var numbers []int
	for _, card := range b.MatchInSet(cardName, set.Code) {
		if !card.HasPromoType(magic.PromoTypeFirstPlaceFoil) {
			continue
		}
		number, err := strconv.Atoi(card.Number)
		if err != nil {
			return nil
		}
		numbers = append(numbers, number)
	}
	slices.Sort(numbers)
	return numbers
}

// namesTheArt reports whether the store's image name, rather than any wording
// it publishes, is what says which art a listing is selling.
func namesTheArt(edition, extra string) bool {
	if needsImageLookup(edition) {
		return true
	}
	return hasNumberedImage(edition) && imageNumber(extra) != ""
}

// preprocessBL builds the card a buylist listing prices. The number is the
// one the listing states outright, which is the only thing telling the
// treatments of one card apart: the store publishes the same name for the
// plain printing and every borderless or showcase sibling beside it, and
// prices them separately.
func preprocessBL(b *matcher.Backend, cardName, edition, extra string, number int) (*matcher.InputCard, error) {
	variant := ""
	if strings.Contains(edition, "(") {
		vars := matcher.SplitVariants(edition)
		edition = vars[0]
		if len(vars) > 1 {
			variant = vars[1]
		}
	}
	// The store also writes the version on the name, "X (V.2)".
	vars := matcher.SplitVariants(cardName)
	if variant == "" && len(vars) > 1 {
		cardName, variant = vars[0], vars[1]
	}

	cardName, edition, variant = internalPreprocess(b, cardName, edition, variant, extra)

	cn, found := cardTable[cardName]
	if found {
		cardName = cn
	}

	lutName, found := editionTable[edition]
	if found {
		edition = lutName
	}

	// Retail settles a numbered DCI Promo by its image; the buylist has
	// none, and the bare number matches in several sets. Secret Lair Promo
	// holds the ones that read as another set's copy of the same number.
	if edition == "DCI Promo" && number > 0 && len(b.MatchInSetNumber(cardName, "SLP", strconv.Itoa(number))) > 0 {
		edition = "Secret Lair Promo"
	}

	// The wording the store hangs off the edition still speaks first: it
	// names the printing where the number is the store's own rather than
	// the catalog's, and a number saying the same thing costs nothing.
	if number > 0 {
		variant = strings.TrimSpace(variant + " " + strconv.Itoa(number))
	}

	return &matcher.InputCard{
		Name:      cardName,
		Edition:   edition,
		Variation: variant,
	}, nil
}
