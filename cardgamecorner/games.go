package cardgamecorner

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// The storefront's own names for the games it sells.
const (
	GameMagic         = "magic"
	GamePokemon       = "pokemon"
	GameYuGiOh        = "yugioh"
	GameOnePiece      = "onepiece"
	GameLorcana       = "lorcana"
	GameRiftbound     = "riftbound"
	GameFleshAndBlood = "fleshandblood"
)

// cgcGames is what NewScraper is built through: it names the storefront's
// game for each of ours, and a game named nowhere here is not one Card Game
// Corner is read for.
var cgcGames = map[matcher.Game]string{
	matcher.GameMagic:         GameMagic,
	matcher.GamePokemon:       GamePokemon,
	matcher.GameYuGiOh:        GameYuGiOh,
	matcher.GameOnePiece:      GameOnePiece,
	matcher.GameLorcana:       GameLorcana,
	matcher.GameRiftbound:     GameRiftbound,
	matcher.GameFleshAndBlood: GameFleshAndBlood,
}

// shelfSets names the set a game files the printings of a store shelf
// under, where the shelf's own name points at no single set. One Piece's
// promo shelves sell event printings, which share their number with the
// booster card they reprint; Pokemon's "Base" is the unlimited Base Set, not
// its Shadowless run, but for its first edition copies; Yu-Gi-Oh's 25th
// anniversary and date reprints share theirs with the original's prints.
var shelfSets = map[matcher.Game]map[string]string{
	matcher.GameOnePiece: {
		"Promos":                     "OP-PR",
		"Unnumbered Promos":          "OP-PR",
		"Premium Bandai Promo":       "OP-PR",
		"Special Tournaments Promos": "OP-PR",
		"Reprints":                   "OP-RP",
	},
	matcher.GamePokemon: {
		"Base":              "BS",
		"30th Celebrations": "30C",
	},
	matcher.GameYuGiOh: {
		"Legend of Blue Eyes White Dragon REPRINT 25th Anniversary Edition": "LOB-EN-23050",
		"Metal Raiders REPRINT 25th Anniversary Edition":                    "MRD-EN-23052",
		"Spell Ruler REPRINT 25th Anniversary Edition":                      "SRL-EN-23053",
		"Pharaoh’s Servant REPRINT 25th Anniversary Edition":                "PSV-EN-23054",
		"Dark Crisis REPRINT 25th Anniversary Edition":                      "DCR-EN-23055",
		"Invasion of Chaos REPRINT 25th Anniversary Edition":                "IOC-EN-23056",
		"Retro Pack 2 Reprint 2025":                                         "RP02-24352",
		"Light of Destruction - 2024 Reprint Unlimited":                     "LODT-23579",
	},
}

// classicSet is the one set the catalog files the three Classic collection
// decks under, which numbers each from 001 and tags it as classicDecks says.
const classicSet = "CL-23323"

// classicDecks is the tag the catalog gives each deck of the Classic
// collection.
var classicDecks = map[string]string{
	"Pokémon Trading Card Game Classic: Blastoise & Suicune Deck": "CLB",
	"Pokémon Trading Card Game Classic: Charizard & Ho-Oh Deck":   "CLC",
	"Pokémon Trading Card Game Classic: Venusaur & Lugia Deck":    "CLV",
}

// promoNumberPrefixes is the code each promo shelf leaves off its numbers.
// A Pokemon Black Star promo is numbered "SWSH011" in the catalog where the
// storefront writes "011"; a Yu-Gi-Oh manga volume is a set of its own,
// "YF03", of which the storefront writes only the volume.
var promoNumberPrefixes = map[string]string{
	"BW Black Star Promos":              "BW",
	"XY Black Star Promos":              "XY",
	"Sun & Moon: Black Star Promos":     "SM",
	"Sword & Shield: Black Star Promos": "SWSH",
	"5D's Manga Promos":                 "YF",
	"GX Manga Promos":                   "YG",
	"ZEXAL Manga Promos":                "YZ",
	"R Comic Book Promos":               "YR",
}

// gameNames holds the card names the store misspells, by game, as the
// datastore spells them, and one Yu-Gi-Oh name it gives in Italian whose
// image carries no English one.
var gameNames = map[matcher.Game]map[string]string{
	matcher.GameYuGiOh: {
		"Abyss Actors' Dress Rehersal":          "Abyss Actors' Dress Rehearsal",
		"Aphophis the Swamp Deity":              "Apophis the Swamp Deity",
		"Battlin‚Äô Boxer Lead Yolk":            "Battlin' Boxer Lead Yoke",
		"Carnot the Eternal Maching":            "Carnot the Eternal Machine",
		"Damage Vaccine O MAX":                  "Damage Vaccine Omega MAX",
		"Exstellarknight Constellar Ptolemy O7": "Exstellarknight Constellar Ptolemy Omega 7",
		"Falchionß":                             "Falchion Beta",
		"Goblin Biker Big Cabonga":              "Goblin Biker Big Gabonga",
		"Gold Pride - Chariot Carrier":          "Gold Pride - Chariot Carrie",
		"K9-ØØ Lupis":                           "K9-00 Lupis",
		"Majespecter Orthus - Nue":              "Majespecter Orthrus - Nue",
		"Maliss Q HEARTS OF CRYPTER":            "Maliss Q Hearts Crypter",
		"Outer Entity Azathoth":                 "Outer Entity Azathot",
		"Raffica di Vento":                      "Gust",
		"Reincarnation Unvealing Mail":          "Reincarnation Unveiling Mail",
		"Sky Striker Ace - Azelea Temperance":   "Sky Striker Ace - Azalea Temperance",
		"Supernatural Regeneration":             "Metaphysical Regeneration",
		"Terrors of the Overrroot":              "Terrors of the Overroot",
		"The Virtuos Vestals":                   "The Virtuous Vestals",
		"Urubonus, the Avatar of the Malice":    "Urubonus, the Avatar of Malice",
		"Xyz Posiedon Splash":                   "Xyz Poseidon Splash",
	},
	matcher.GameLorcana: {
		"Balu - Lively Bear": "Baloo - Fun-Loving Bear",
		"Miss Bianca - International Rescue Aid Society Age": "Miss Bianca - Rescue Aid Society Agent",
	},
	matcher.GamePokemon: {
		"Target Whistle Team Flare Gear": "Target Whistle",
	},
	matcher.GameRiftbound: {
		"lascylla":             "Iascylla",
		"Vi, Piltover ZHorcer": "Vi, Piltover Enforcer",
	},
}

// vendorName returns the name a misspelled one stands for, keeping the
// version the store may have written after it.
func vendorName(game matcher.Game, name string) string {
	head, tail, hasTail := strings.Cut(name, " (")
	fixed, found := gameNames[game][head]
	if !found {
		return name
	}
	if hasTail {
		return fixed + " (" + tail
	}
	return fixed
}

// deltaSpeciesRe matches the words a Delta Species name carries after its
// Pokemon, spelled with a "d" for the δ, which may be cut short and may be
// followed by an attack.
var deltaSpeciesRe = regexp.MustCompile(` d Delta Species?`)

// nidoranSexes spells the sex of a Nidoran the way the catalog does, with a
// letter, where the store writes a word or an HTML entity.
var nidoranSexes = strings.NewReplacer(
	"Nidoran &female;", "Nidoran F",
	"Nidoran female", "Nidoran F",
	"Nidoran ♀", "Nidoran F",
	"Nidoran &male;", "Nidoran M",
	"Nidoran male", "Nidoran M",
	"Nidoran ♂", "Nidoran M",
)

// energyLetters are the type letters the store brackets in the name of an
// energy, which the catalog spells out for some energies and not others.
var energyLetters = strings.NewReplacer(
	"[G]", "Grass", "[R]", "Fire", "[W]", "Water", "[L]", "Lightning",
	"[P]", "Psychic", "[F]", "Fighting", "[D]", "Darkness", "[M]", "Metal",
	"[Y]", "Fairy", "[N]", "Dragon", "[C]", "Colorless",
)

// energyName spells out the type letter of an energy where the catalog
// does ("Wash Water Energy"), and keeps it where the catalog keeps the
// letter ("Bubbly W Energy").
func energyName(b *matcher.Backend, name string) string {
	if !strings.HasSuffix(name, " Energy") {
		return name
	}
	spelled := energyLetters.Replace(name)
	if spelled != name && namesACard(b, spelled) {
		return spelled
	}
	return name
}

// storeEnergyNumberRe matches the number the store gives a basic energy
// the catalog does not number.
var storeEnergyNumberRe = regexp.MustCompile(`^E[0-9]{3}$`)

// pokemonName reads the name of a Pokemon listing, whose wording the store
// spells its own way.
func pokemonName(card *Card) string {
	name := deltaSpeciesRe.ReplaceAllString(strings.TrimSpace(card.Name), "")

	// A "?" stands for the symbol of the sex, which the original name may
	// still carry
	if strings.Contains(name, "Nidoran ?") {
		spelled := nidoranSexes.Replace(card.OrigName)
		for _, sex := range []string{"Nidoran F", "Nidoran M"} {
			if strings.Contains(spelled, sex) {
				name = strings.Replace(name, "Nidoran ?", sex, 1)
			}
		}
	}
	return nidoranSexes.Replace(name)
}

// riftboundVersionRe matches the "(V.2 - Showcase)" a Riftbound name ends in,
// which the number already says: a showcase is "193a" beside the rare's
// "193".
var riftboundVersionRe = regexp.MustCompile(`\s*\(V\.[0-9]+ - [^)]*\)$`)

// oldShelfRe matches the name of a Yu-Gi-Oh shelf holding the first prints
// of an old set, which ends in the set's code.
var oldShelfRe = regexp.MustCompile(`ENG \(([A-Z0-9]+)\)$`)

// oldShelfSet returns the set an old Yu-Gi-Oh shelf holds. The North
// American and the European print of a card are both filed in it, the
// European one as "European <rarity>".
func oldShelfSet(edition string) (string, bool) {
	match := oldShelfRe.FindStringSubmatch(edition)
	if match == nil {
		return "", false
	}
	return match[1], true
}

// knownOldShelf returns the set oldShelfSet reads off a shelf, where the
// datastore knows it.
func knownOldShelf(b *matcher.Backend, edition string) (string, bool) {
	code, found := oldShelfSet(edition)
	if !found {
		return "", false
	}
	_, err := b.GetSet(code)
	return code, err == nil
}

// slugName reads the card an image is named after, where the store gave the
// name in Italian and the image directory holds the English one.
func slugName(b *matcher.Backend, extra string) string {
	slug := strings.ReplaceAll(path.Base(path.Dir(extra)), "-", " ")
	if !namesACard(b, slug) {
		return ""
	}
	return slug
}

// losesFirstEdition reports whether a first edition listing lands on a
// printing that is not one: the matcher falls back to the unlimited print
// where the datastore holds no first edition of the card.
func losesFirstEdition(b *matcher.Backend, inCard *matcher.InputCard) bool {
	run, found := landedRun(b, inCard)
	return found && run != matcher.Run1stEdition
}

// landedRun returns the print run of the printing a listing lands on, and
// false where it lands on none.
func landedRun(b *matcher.Backend, inCard *matcher.InputCard) (string, bool) {
	probe := *inCard
	cardID, err := b.Match(&probe)
	if err != nil {
		return "", false
	}
	co, err := b.GetUUID(cardID)
	if err != nil {
		return "", false
	}
	finish, _ := matcher.FinishOf(co.Finish)
	return finish.Run, true
}

// classicName is the name a Classic deck card goes by in the catalog, which
// calls a basic energy "Basic Fire Energy" where the store writes "Fire
// Energy".
func classicName(b *matcher.Backend, name string) string {
	basic := "Basic " + name
	if strings.HasSuffix(name, " Energy") && namesACard(b, basic) {
		return basic
	}
	return name
}

// pokemonInsertRe matches the products the Pokemon shelves hold that are no
// card: the code cards of a box and the VSTAR marker.
var pokemonInsertRe = regexp.MustCompile(`^((Online|Live) Code Card|VSTAR Marker$)`)

// preprocessGame reads a card of any game but Magic, whose listings name the
// card, its set and its collector number plainly.
func preprocessGame(b *matcher.Backend, card *Card, index int) (*matcher.InputCard, error) {
	// Graded slabs are sold on shelves of their own
	if strings.Contains(card.Edition, "Gradat") {
		return nil, matcher.ErrUnsupported
	}

	if b.Game == matcher.GamePokemon && pokemonInsertRe.MatchString(card.Name) {
		return nil, matcher.ErrUnsupported
	}

	inCard := gameInput(b.Game, card, index)
	if b.Game == matcher.GamePokemon {
		inCard.Name = energyName(b, inCard.Name)
	}
	code, found := shelfSets[b.Game][card.Edition]
	// The Base Set's first edition was printed before its shadow was, so
	// its copies are the Shadowless product's
	if b.Game == matcher.GamePokemon && code == "BS" && card.Variants[index].Print == "First Edition" {
		code = "BSS"
	}
	_, classic := classicDecks[card.Edition]
	if classic {
		code, found = classicSet, true
		inCard.Name = classicName(b, inCard.Name)
	}
	oldShelf := b.Game == matcher.GameYuGiOh && oldShelfRe.MatchString(card.Edition)
	if oldShelf {
		if !namesACard(b, inCard.Name) {
			name := slugName(b, card.Extra)
			if name != "" {
				inCard.Name = name
			}
		}
		if !found {
			code, found = knownOldShelf(b, card.Edition)
		}
	}
	if found {
		set, err := b.GetSet(code)
		if err != nil {
			return nil, err
		}
		inCard.Edition = set.Name
	}

	// A copy of a first edition the datastore lacks, the European ones
	// still, would be priced as the unlimited it lands on
	if oldShelf && inCard.Finish == matcher.Run1stEdition && losesFirstEdition(b, inCard) {
		return nil, matcher.ErrUnsupported
	}

	// A first edition rare its rarity does not call holo, as a Shining
	// card's is, was printed holofoil only, and the plain first edition it
	// asks for lands on the unlimited holofoil
	if b.Game == matcher.GamePokemon && inCard.Finish == matcher.Run1stEdition && losesFirstEdition(b, inCard) {
		probe := *inCard
		probe.Finish = matcher.PrintingFinish(matcher.Run1stEdition, "Holofoil")
		run, found := landedRun(b, &probe)
		if found && run == matcher.Run1stEdition {
			inCard.Finish = probe.Finish
		}
	}
	return inCard, nil
}

// onePiecePromoShelfRe matches the shelf holding the promo printings of a
// set, which names the set's code before its name.
var onePiecePromoShelfRe = regexp.MustCompile(`^[A-Z]{2}[0-9]{2} - (.+): Promo$`)

// unbracketedVersionRe matches a version whose opening bracket the store
// dropped, "... V.8 - Ultra Rare Overframe )".
var unbracketedVersionRe = regexp.MustCompile(`^(.+?) (V\.[0-9]+ - [^()]*?)\s*\)$`)

// gameInput reads a listing's wording the way the game's rules expect it.
func gameInput(game matcher.Game, card *Card, index int) *matcher.InputCard {
	name := card.Name
	number := card.Number
	variant := card.Variants[index]
	foil := variant.Foil == "Foil"
	edition := card.Edition
	var rainbow bool
	switch game {
	case matcher.GameOnePiece:
		// The store spells Cardmarket's "Promos: <set>" shelf as
		// "<code> - <set>: Promo"
		edition = onePiecePromoShelfRe.ReplaceAllString(edition, "Promos: $1")
		// A rarity naming the treatment places the card where its image
		// carries no Cardmarket product id to do it
		if !mcmImageIDRe.MatchString(card.Extra) {
			number = strings.TrimSpace(number + " " + card.Rarity)
		}
	case matcher.GameFleshAndBlood:
		// Every name ends in its finish, which hides the pitch before it
		// from the rules, and is the only place a rainbow foil says so
		name, rainbow = strings.CutSuffix(name, " (Rainbow Foil)")
		// The store cuts a name at 50 characters, leaving " (No" for it
		cut := strings.LastIndex(name, " (")
		if cut >= 0 && len(name)-cut > 2 && strings.HasPrefix(" (Normal)", name[cut:]) {
			name = name[:cut]
		}
		foil = foil || rainbow
		// A double-sided card is numbered "001/002" with its faces in
		// ascending order, which the catalog does not keep, and both
		// faces are in the name
		if strings.Contains(name, " // ") {
			number = ""
		}
	case matcher.GameYuGiOh:
		name = unbracketedVersionRe.ReplaceAllString(name, "$1 ($2)")
		// A manga shelf writes the volume, and on most the card index too
		prefix, found := promoNumberPrefixes[card.Edition]
		if found {
			volume, cardIndex, hasIndex := strings.Cut(number, "-")
			if !hasIndex {
				cardIndex = "001"
			}
			number = fmt.Sprintf("%s%02s-EN%s", prefix, volume, cardIndex)
		}
		// A number holds the card at several rarities, which the name
		// spells only from its second version on
		number = strings.TrimSpace(number + " " + card.Rarity)
		// The European print, an old shelf's other one, numbers its
		// cards on its own
		if oldShelfRe.MatchString(card.Edition) && variant.Language == "EN" {
			number = "European " + card.Rarity
		}
	case matcher.GameRiftbound:
		name = riftboundVersionRe.ReplaceAllString(name, "")
	case matcher.GamePokemon:
		name = pokemonName(card)
		prefix, found := promoNumberPrefixes[card.Edition]
		if found && !strings.HasPrefix(number, prefix) {
			number = prefix + number
		}
		if strings.HasSuffix(name, " Energy") && storeEnergyNumberRe.MatchString(number) {
			number = ""
		}
		deck, found := classicDecks[card.Edition]
		if found {
			number = strings.TrimSpace(number + " " + deck)
		}
	}
	return &matcher.InputCard{
		Name:      vendorName(game, name),
		Edition:   edition,
		Variation: number,
		Foil:      foil,
		Finish:    gameFinish(game, card, variant, rainbow),
	}
}

// gameFinish names the finish a listing prices, for the games whose
// printings the foil flag cannot tell apart: a print run, and Pokemon's two
// holo treatments.
func gameFinish(game matcher.Game, card *Card, variant Variant, rainbow bool) string {
	firstEdition := variant.Print == "First Edition"
	switch game {
	case matcher.GameYuGiOh:
		if firstEdition {
			return "1st Edition"
		}
	case matcher.GamePokemon:
		if variant.Foil == "Reverse Holo" {
			return "Reverse Holofoil"
		}
		var treatment string
		if card.Rarity == "Rare Holo" {
			treatment = "Holofoil"
		}
		if firstEdition {
			return matcher.PrintingFinish("1st Edition", treatment)
		}
		return treatment
	case matcher.GameFleshAndBlood:
		treatment := "Normal"
		if rainbow {
			treatment = "Rainbow Foil"
		}
		switch {
		case firstEdition:
			return matcher.PrintingFinish("1st Edition", treatment)
		case variant.Print == "Unl.":
			return matcher.PrintingFinish("Unlimited Edition", treatment)
		case rainbow:
			return treatment
		}
	}
	return ""
}
