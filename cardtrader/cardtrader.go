// Package cardtrader scrapes Card Trader, both the marketplace and sealed
// product, across every game they carry.
package cardtrader

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
)

const (
	defaultConcurrency = 8
)

// Market prices singles from Card Trader, splitting the result into
// the storefronts they sell under: the marketplace itself, Zero, and 1DR.
type Market struct {
	logCallback    mtgban.LogCallbackFunc
	inventoryDate  time.Time
	maxConcurrency int
	shareCode      string

	// Only retrieve data from a single edition
	targetEdition string

	exchangeRates map[string]float64
	client        *CTAuthClient
	backend       *matcher.Backend

	inventory mtgban.InventoryRecord

	blueprints map[int]*Blueprint

	// Flesh and Blood only: fabNumbers of backend and fabListedIDs of
	// blueprints.
	fabSiblings map[string][]*matcher.CardObject
	fabListed   map[string]bool

	gameID int
}

var availableMarketNames = []string{
	"Card Trader", "Card Trader Zero", "Card Trader 1DR",
}

var name2shorthand = map[string]string{
	"Card Trader":      "CT",
	"Card Trader Zero": "CT0",
	"Card Trader 1DR":  "CT1DR",
}

// NewScraperMarket returns a market scraper for the datastore's game,
// authenticated with a full API token.
func NewScraperMarket(b *matcher.Backend, token string) (*Market, error) {
	game := b.Game
	id, found := ctGames[game]
	if !found {
		return nil, fmt.Errorf("unsupported game %q", game)
	}
	ct := Market{}
	ct.inventory = mtgban.InventoryRecord{}
	ct.maxConcurrency = defaultConcurrency
	ct.client = NewCTAuthClient(token)
	ct.backend = b
	ct.gameID = id
	if id == GameFleshAndBlood {
		ct.fabSiblings = fabNumbers(b)
	}
	return &ct, nil
}

func (ct *Market) printf(format string, a ...any) {
	if ct.logCallback != nil {
		ct.logCallback("[CT] "+format, a...)
	}
}

type resultChan struct {
	cardID   string
	invEntry *mtgban.InventoryEntry
}

// ctCondition maps Card Trader's condition to our grade. A listing naming
// none is taken as near mint, and "Played", Card Trader's grade between
// Moderately and Heavily Played, folds into HP.
func ctCondition(s string) (mtgban.Condition, error) {
	switch s {
	case "":
		return mtgban.NM, nil
	case "Played":
		return mtgban.HP, nil
	default:
		return mtgban.ParseCondition(s)
	}
}

var langMap = map[string]string{
	"en":    "",
	"fr":    "French",
	"de":    "German",
	"es":    "Spanish",
	"it":    "Italian",
	"jp":    "Japanese",
	"kr":    "Korean",
	"pt":    "Portuguese",
	"ru":    "Russian",
	"zh-cn": "Chinese",
	"zh-tw": "Chinese",
}

// listingID resolves a listing on the printing id its blueprint names. A
// named finish reaches the sibling the foil flag cannot: the flag has one bit
// and lands on the product's foil default, where the name says which of its
// treatments the listing prices. A name the product is sold in no printing of
// falls back to the print run the listing states, then to the flag, which
// answers with the default rather than nothing.
func listingID(b *matcher.Backend, gameID int, id string, card *matcher.InputCard, product Product) string {
	var cardID string
	if card.Finish != "" {
		cardID, _ = b.MatchIDFinish(id, card.Finish)
		run := listingRunFinish(gameID, product)
		if cardID == "" && run != "" {
			cardID, _ = b.MatchIDFinish(id, run)
		}
	}
	if cardID == "" {
		cardID, _ = b.MatchID(id, card.Foil)
	}
	return cardID
}

// listingRunFinish names the holo printing in the print run a Pokemon listing
// states, for a product whose version never says holo: the flag fallback takes
// the first foil printing, which can sit in the other run.
func listingRunFinish(gameID int, product Product) string {
	if gameID != GamePokemon {
		return ""
	}
	if product.Properties.FirstEdition {
		return matcher.PrintingFinish(matcher.Run1stEdition, "Holofoil")
	}
	return matcher.PrintingFinish(matcher.RunUnlimited, "Holofoil")
}

// tcgIDOverrides corrects a blueprint's own TCGplayerID where it names a
// sibling card, or supplies one Card Trader sends none for at all.
var tcgIDOverrides = map[int]int{
	// Flesh and Blood: the id names the other half of a printed pair.
	334549: 633634, // Saltwater Swell (Yellow), Lost Treasure | Cold Foil
	334550: 633633, // Saltwater Swell (Blue), Lost Treasure | Cold Foil
	295093: 557937, // Take Flight (Red), Armory Deck: Boltyn
	295089: 557943, // Engulfing Light (Red), Armory Deck: Boltyn
	// Flesh and Blood: the id names the hero's Rainbow Foil promo, HER137.
	331877: 638041, // Puffin, Hightail, SEA001
	// Pokemon: same shape, a Poké Ball Pattern reverse holo pair.
	343417: 642431, // Rufflet, 077/086
	343418: 642432, // Braviary, 078/086
	357942: 685999, // Judge, Professor Program Stamp | 167/182
	// One Piece DON!! promos: no id sent at all.
	391163: 692132, // Bandai Fest | Elbaph
	374929: 711010, // 3rd Anniversary Set | English Version
	406330: 680489, // 3rd Anniversary Set Japan
	290971: 544805, // Pop Art (3D Text)
	406254: 715681, // Double Pack Set Vol. 12 Shanks, labeled Gol.D.Roger
	406255: 715682, // Double Pack Set Vol. 12 Rocks.D.Xebec
	294212: 600706, // Garp and Luffy, Saikyo Jump Promo, 2nd Anniversary Tournament
	358981: 710745, // EB-03, Heroines Special Set
	// One Piece: the id names another event's print of the card.
	326797: 712033, // O-Nami, Dash Pack 2025, sent the Illustration Box's 623070
	// Riftbound: the T1 2025 Worlds Signature Edition Bundle, T1S002.
	400778: 716016, // Xin Zhao, Vigilant, 002 Showcase
	400783: 716018, // Xin Zhao, Vigilant, 002s Showcase, Signed Serialized
	// Riftbound Radiance: the id names the neighbouring or the plain card.
	415816: 723434, // The Exile, 149, sent Blade of the Exile's 722279
	417516: 723429, // Seraphine, Inspiring, 065a Alternate Art, sent the plain 723430
	// Pokemon: none sent either.
	285780: 658742, // Professor Turo's Scenario, Professor Program Stamp | 171/182
	340672: 211449, // Palkia & Dialga LEGEND, Jumbo Oversized
	242584: 250252, // Mewtwo V-UNION, Jumbo Oversized
	332883: 545663, // Lugia ex, Jumbo Oversized, TCG Classic deck
	171625: 252517, // Charizard, Gold Metal, Celebrations
	171624: 252516, // Pikachu, Gold Metal, Celebrations
	314342: 616824, // Greninja ex, Metal Card, Twilight Masquerade
	228430: 131869, // Arcanine BREAK, Jumbo Oversized, id names no product
	341092: 478425, // Pikachu VMAX, Jumbo Oversized | 286
	323065: 131864, // Snorlax GX, Jumbo Oversized | 069
	// Yu-Gi-Oh: the qualifier copy named the YCS one, which has none.
	341992: 679562, // Blue-Eyes White Dragon, 25YC-ENP01 World Championship Qualifier
	346315: 637082, // Blue-Eyes White Dragon, 25YC-ENP01 YCS Stamp
	346313: 637080, // Effect Veiler, 25YC-EN002 YCS Stamp
	// Lorcana: none sent for the one errata reprint TCGplayer sells.
	311912: 597095, // Bucky - Squirrel Squeak Tutor, Errata Version
	// Pokemon promos: none sent, and the shelf names no set.
	287980: 562025, // Pecharunt, Blister Exclusives | 149
	337180: 645415, // Wellspring Mask Ogerpon ex, Premium Collection | TWM 040
	337178: 645458, // Hearthflame Mask Ogerpon ex, Premium Collection | TWM 040
	374802: 678169, // Bulbasaur, Pokémon Day Promo | 001/132
	131770: 244347, // Rapidash, Pokemon Center Stamped | 51
	111628: 164284, // Tepig, Pokémon League | 15/114
	128302: 123196, // Gyarados, Holo Promo | 19/100
	114420: 145864, // Latias, Stamp Holo Promo | 009/020
	323068: 247362, // Duraludon, Gamestop Promo | 028
	341065: 594468, // Magneton, Pokemon Center Stamped | 159
	124918: 165634, // Blastoise, Cosmos Holo | ©2013 Promo 16/101
	351718: 658354, // Yveltal, EB Games Promo | 088/132
	// One blueprint for both finishes: the Non-Holo is the pick, as on the
	// siblings of this wording that do send an id.
	296379: 565488, // Morpeko, Non-Holo | Cosmos Holo | PAR 121
	296385: 565453, // Counter Catcher, Non-Holo / Cosmos Holo | PAR 160
	296386: 565266, // Earthen Vessel, Non-Holo / Cosmos Holo | PAR 163
	397591: 713261, // Nidorina, MEP 101 | Pokemon Center Stamp
	118520: 153288, // Electabuzz, Toys R Us Promo | 41/108
	354634: 666533, // Suicune, GameStop Promo | 026/132
	358323: 664006, // Flygon, Non-Holo | 053/167
	312388: 515290, // Toxtricity, Non-Holo Prerelase Kit
	399831: 244348, // Ho-Oh, Pokemon Center Stamped | 52
	128303: 125079, // Tyranitar, Holo Promo | 30/100
	399560: 706199, // Zarude, MEP 088 | Pokemon Center Stamp
	391487: 694626, // Ho-Oh, EB Games Promo | 010/086
	392243: 694625, // Ho-Oh, GameStop Promo | 010/086
	364566: 680957, // Eevee, GYM Stamped | 143/191
	357871: 680953, // Reshiram ex, WHT 020
	337188: 639952, // Team Rocket's Giovanni, Holo | DRI 174
	299930: 607999, // Basic Lightning Energy, 004 | Pikachu 7
	299914: 557054, // Jacq, 175 | Armarouge 49
	396495: 716740, // Pikachu, MEP 093 | Winner Stamp
	120070: 136645, // Clefairy, 20th Anniversary Holo Promo 50/83
	// One Piece promos: none sent, and a blank or unlabelled version.
	358719: 668182, // Gol.D.Roger P-107, Store Tournament 2025 Vol. 4
	400812: 719661, // DON!! Flame-Flame Fruit Coliseum, Championships Promo
	337323: 646721, // Kid & Killer EB01-003P2, Offline Regional 25-26 Finalist
	337325: 646726, // Trafalgar Law OP07-047P2, Offline Regional 25-26 Finalist
	337316: 646734, // Tony Tony.Chopper P-089, Offline Regional 25-26 Participant
	337328: 646733, // Chopper P-089P2, Offline Regional 25-26 Finalist
	337329: 646720, // Kid & Killer EB01-003, Offline Regional 25-26 Champion
	337331: 646727, // Trafalgar Law OP07-047, Offline Regional 25-26 Champion
	337334: 646731, // Chopper P-089, Offline Regional 25-26 Champion
	345022: 646743, // Shanks, 25-26 Offline Regionals Top 8
	345023: 646742, // Benn.Beckman, 25-26 Regionals Top 16
	345024: 646741, // Yasopp, 25-26 Regionals Top 16
	// Riftbound tokens: none sent.
	367443: 678186, // Mech // Buff, T01
	408192: 713239, // Shadow Clone // Tentacle, Full Art
	// Yu-Gi-Oh: arts of one number, told apart by the artwork.
	81844: 123620, // Blue-Eyes White Dragon, LDK2-ENK01 Version 4
	81905: 123525, // Blue-Eyes White Dragon, LDK2-ENK01 Version 2
	81937: 123621, // Blue-Eyes White Dragon, LDK2-ENK01 Version 1
	81519: 79484,  // Cyber Dragon, SDCR-EN003 White
	81520: 79485,  // Cyber Dragon, SDCR-EN003 Black
	// Yu-Gi-Oh: a video game promo on a shelf naming no set.
	69769: 22817,  // Riryoku, TSC-002
	75164: 22817,  // Riryoku, TSC-002
	81033: 22941,  // Dark Magician, DDS-002
	72219: 186749, // Meteor B. Dragon, SBLS-EN013
	// Yu-Gi-Oh: the id names the European print, After the Struggle (LOD-EN086).
	63798: 21725, // After Genocide, LOD-086
	// Yu-Gi-Oh: Legendary Collection Kaiba 2025 reprint arts.
	353822: 658908, // Blue-Eyes White Dragon, LCKC-EN001
	353823: 658907, // Blue-Eyes White Dragon, LCKC-EN001
	353824: 658909, // Blue-Eyes White Dragon, LCKC-EN001
	353893: 658910, // Blue-Eyes White Dragon, LCKC-EN001
	353939: 658920, // Crush Card Virus, LCKC-EN046
	353940: 658921, // Crush Card Virus, LCKC-EN046 Alternate Art
	// Flesh and Blood: the TCGplayer id sent is the Cardmarket one.
	281847: 580614, // Cintari Saber (Left) (Golden), FAB184
	281849: 580615, // Cintari Saber (Right) (Golden), FAB185
	// Flesh and Blood: Spectral Shield's plain print, not its token.
	289168: 552843, // Spectral Shield, MST158-A
	// Gundam: the ST11-ST14 release event promos, filed under GD06.
	412578: 717612, // Acguy, ST11-002
	412579: 717613, // Char Aznable, ST11-011
	412580: 717615, // Unicorn Gundam 02 Banshee, ST12-009
	412581: 717617, // Ple-Twelve, ST12-012
	// Gundam: Store Tournament and Winner Pack 06 promos, none sent.
	417447: 721540, // Victory Gundam, GD04-011 Participation
	417448: 721541, // GD04-107 Participation
	417449: 721542, // GD05-016 Participation
	417450: 721543, // GD05-019 Participation
	417451: 721544, // GD05-069 Participation
	417452: 721545, // Quess Paraya, GD05-094 Participation
	417453: 721546, // GD05-111 Participation
	417454: 721547, // T-029 Participation
	417438: 721548, // GD04-011 Winner Pack 06
	417440: 721549, // GD04-107 Winner Pack 06
	417441: 721550, // Murasame, GD05-016 Winner Pack 06
	417442: 721551, // GD05-019 Winner Pack 06
	417443: 721552, // GD05-069 Winner Pack 06
	417444: 721553, // GD05-094 Winner Pack 06
	417445: 721554, // GD05-111 Winner Pack 06
	417446: 721555, // T-029 Winner Pack 06
	// Magic: the Scryfall id names the card before it on the shelf.
	155490: 237359, // Nicol Bolas, God-Pharaoh, PHOU 140p
}

// TCGplayerProductID answers the TCGplayer id a blueprint's listings resolve
// against: its own TCGplayerID, or tcgIDOverrides' correction where Card
// Trader's is wrong or missing.
func (bp *Blueprint) TCGplayerProductID() int {
	if id, found := tcgIDOverrides[bp.ID]; found {
		return id
	}
	return bp.TCGplayerID
}

func (ct *Market) processProducts(channel chan<- resultChan, bpID int, products []Product) {
	blueprint, found := ct.blueprints[bpID]
	if !found {
		return
	}
	// A blueprint whose own game_id disagrees with the scraper's is a
	// vendor catalog error, not a card this game ever sees: Card Trader
	// files a couple of Pokemon blueprints under a Magic expansion.
	if blueprint.GameID != ct.gameID {
		return
	}

	var blueprintCard *matcher.InputCard
	if ct.gameID == GameMagic {
		var err error
		blueprintCard, err = Preprocess(ct.backend, blueprint)
		if err != nil {
			return
		}
	}

	// Listings of one blueprint mostly fail alike, so each distinct refusal
	// is logged once.
	refusals := map[string]bool{}
	for _, product := range products {
		switch {
		case product.Quantity < 1,
			product.OnVacation,
			product.Properties.Altered,
			product.Graded:
			continue
		case matcher.Contains(product.Description, "ita"),
			matcher.Contains(product.Description, "mix"):
			continue
		}

		cond := product.Properties.Condition
		if product.Properties.Signed ||
			matcher.Contains(product.Description, "signed") ||
			matcher.Contains(product.Description, "inked") ||
			matcher.Contains(product.Description, "stamp") ||
			matcher.Contains(product.Description, "poor") ||
			matcher.Contains(product.Description, "water") {
			cond = "Poor"
		}

		conditions, err := ctCondition(cond)
		if err != nil {
			ct.printf("unsupported %s condition", cond)
			continue
		}

		// Build the per-game input card; the match and error handling below are
		// shared. Magic copies the blueprint-derived card (applying the
		// product language), Lorcana builds one from the product's number.
		var theCard *matcher.InputCard
		switch ct.gameID {
		case GameMagic:
			// Match rewrites the card it is given, so each listing is
			// matched on its own copy of the blueprint's
			card := *blueprintCard
			theCard = &card
			lang := product.Properties.MTGLanguage
			if lang != "" {
				lang, found = langMap[strings.ToLower(lang)]
				if !found {
					ct.printf("unsupported '%s' language", product.Properties.MTGLanguage)
					ct.printf("%s %+v", theCard, product)
					continue
				}
				theCard.Language = lang
			}
		case GameLorcana, GameRiftbound, GameOnePiece, GameYuGiOh, GameFleshAndBlood,
			GamePokemon, GameGundam, GamePalworld:
			if gameLanguage(ct.gameID, product) != "en" {
				continue
			}
			if unsupportedBlueprint(ct.gameID, blueprint) {
				continue
			}
			// A listing copies the collector number when it is created and
			// never refreshes it, so a blueprint corrected later leaves its
			// older listings quoting a number that now belongs to a
			// different card. The blueprint is the authoritative one.
			number := blueprint.Properties.Number
			if number == "" {
				number = product.Properties.Number
			}
			if ct.gameID == GamePalworld {
				number = palworldNumber(ct.backend, blueprint, number)
			}
			theCard = &matcher.InputCard{
				Name:      gameName(ct.backend, ct.gameID, blueprint),
				Edition:   gameEdition(ct.backend, ct.gameID, blueprint),
				Variation: gameVariation(ct.gameID, blueprint, number),
				// A listing is one printing in one finish, and the games
				// whose properties name it say which; the flag beside it
				// keeps carrying the plain foilness like everywhere else.
				Finish: gameFinish(ct.gameID, blueprint, product),
				Foil:   gameFoil(ct.gameID, product),
			}
		}

		// The blueprint carries the TCGplayer product id, which names the
		// printing outright instead of inferring it. It is worth trying
		// first: for Lorcana it resolves cards the name and number cannot,
		// and for the Riftbound promos it lands on the promo printing where
		// the edition alone leaves the base one. A blueprint no TCGplayer
		// product sells can still name the Cardmarket one a printing was
		// minted from. Magic keeps its own preprocessing, and a blueprint
		// with neither id falls through.
		var cardID string
		if ct.gameID != GameMagic {
			var ids []string
			if tcgID := blueprint.TCGplayerProductID(); tcgID != 0 {
				ids = append(ids, fmt.Sprint(tcgID))
			}
			for _, mkmID := range blueprint.CardMarketIDs {
				uuid := ct.backend.ConvertID(matcher.IDSpaceCardmarket, fmt.Sprint(mkmID))
				if uuid != "" {
					ids = append(ids, uuid)
				}
			}
			for _, id := range ids {
				cardID = listingID(ct.backend, ct.gameID, id, theCard, product)
				if cardID != "" {
					break
				}
			}
		}
		// A foil claim on a Lorcana card sold in no foil names a printing
		// we do not carry: its Panorama is priced from its own blueprint.
		if ct.gameID == GameLorcana && theCard.Foil && cardID != "" {
			co, err := ct.backend.GetUUID(cardID)
			if err == nil && !co.Foil {
				continue
			}
		}

		byName := cardID == ""
		if byName {
			var err error
			cardID, err = ct.backend.Match(theCard)
			if errors.Is(err, matcher.ErrUnsupported) {
				continue
			} else if err != nil {
				// Preprocess already tried the full token-pairing gauntlet;
				// what's left is mtgjson never publishing a combined uuid.
				// Checked against blueprint.Name, which Match never
				// mutates (see the commit for why theCard's own copy is
				// not safe to read here).
				if isTwoSidedTokenBlueprint(blueprint, blueprint.Name) {
					continue
				}
				// A Flesh and Blood face with no printing of its own has
				// nothing to land on (see the commit). Skipped when the
				// blueprint's own name already spells both faces: that
				// case is uniquely identifiable, so any refusal is a real
				// bug elsewhere rather than this shape's dead end.
				if ct.gameID == GameFleshAndBlood && !strings.Contains(blueprint.Name, " // ") {
					number := blueprint.Properties.Number
					if number == "" {
						number = product.Properties.Number
					}
					if fabDoubleSidedFace(ct.backend, fabNumber(blueprint, number), gameFoil(ct.gameID, product)) {
						continue
					}
				}
				if refusals[err.Error()] {
					continue
				}
				refusals[err.Error()] = true
				ct.printf("%v", err)
				ct.printf("%q", theCard)
				ct.printf("%d %+v", bpID, blueprint)

				var alias *matcher.AliasingError
				if errors.As(err, &alias) {
					for _, probe := range alias.Probe() {
						co, _ := ct.backend.GetUUID(probe)
						ct.printf("- %s", co)
					}
				}
				continue
			}
		}
		// On a promotional shelf an answer with no promotional label is the
		// number speaking alone; a Yu-Gi-Oh misprint is asked after an id
		// too, since its id can name the genuine card.
		if (byName || ct.gameID == GameYuGiOh) && promoShelfNeedsLabel(ct.backend, ct.gameID, blueprint) {
			co, err := ct.backend.GetUUID(cardID)
			if err != nil || !shelfLabelled(ct.gameID, co) {
				continue
			}
		}
		if ygoNoRow[bpID] {
			co, err := ct.backend.GetUUID(cardID)
			if err != nil || !matcher.Equals(co.Number, ygoBlueprintNumbers[bpID]) {
				continue
			}
		}
		// A Flesh and Blood listing flagged in a treatment a sibling at the
		// number is sold in is that sibling, not this printing: a Regular
		// copy of Volzar's Cold Foil token is its Normal face. It lands on
		// that sibling when no blueprint lists it.
		if ct.gameID == GameFleshAndBlood {
			cardID = fabListingID(ct.backend, ct.fabSiblings, ct.fabListed, cardID, blueprint.TCGplayerProductID(), product)
			if cardID == "" {
				continue
			}
		}

		// A starter deck's exclusive foil is its card's Holofoil, and only
		// the version says a blueprint is one.
		if ct.gameID == GameLorcana && lorcanaStarterFoils[blueprint.Version] {
			holo, err := ct.backend.MatchIDFinish(cardID, "Holofoil")
			if err != nil {
				continue
			}
			cardID = holo
		}

		// Magic only: the other games carry the finish on the input already.
		if ct.gameID == GameMagic && product.Properties.MTGFoil {
			cardID = foilPrintingID(ct.backend, cardID)
			// A derived token pairing refuses rather than substitutes the
			// nonfoil id when the pairing was never sold in foil - the
			// one case foilPrintingID can return "" for.
			if cardID == "" {
				continue
			}
		}

		qty := product.Quantity
		if product.Bundle {
			qty *= 4
		}

		link := "https://www.cardtrader.com/cards/" + fmt.Sprint(product.BlueprintID)
		if ct.shareCode != "" {
			link += "?share_code=" + ct.shareCode
		}

		price, err := priceToUSD(product.Price.Cents, product.Price.Currency, ct.exchangeRates)
		if err != nil {
			ct.printf("%v for blueprint %d", err, product.BlueprintID)
			continue
		}

		// Assign a seller name as required by Market
		sellerName := availableMarketNames[0]
		if product.User.SinglesZero {
			sellerName = availableMarketNames[1]
			if strings.Contains(strings.ToLower(product.User.Name), "day ready") {
				sellerName = availableMarketNames[2]
			}
		} else {
			// Skip non-professional users with excessive shipping options due to geo
			if product.User.UserType == "normal" {
				switch product.User.CountryCode {
				case "GB", "ES", "CH":
					continue
				}
			}
		}

		channel <- resultChan{
			cardID: cardID,
			invEntry: &mtgban.InventoryEntry{
				Conditions:   conditions,
				Price:        price,
				Quantity:     qty,
				URL:          link,
				SellerName:   sellerName,
				Bundle:       product.User.SinglesZero,
				OriginalID:   fmt.Sprint(product.BlueprintID),
				InstanceID:   fmt.Sprint(product.ID),
				CustomFields: listingFields(ct.gameID, product),
			},
		}
	}
}

// listingFields are the CustomFields a listing's entry keeps: its seller, its
// price as Card Trader quotes it, and its listingProperties, for a cart to
// find the listings of the card priced at or under this one.
func listingFields(gameID int, product Product) map[string]string {
	return map[string]string{
		"SubSellerName": product.User.Name,
		"SubSellerGeo":  product.User.CountryCode,
		"PriceCents":    strconv.Itoa(product.Price.Cents),
		"PriceCurrency": product.Price.Currency,
		"Properties":    listingProperties(gameID, product).Encode(),
	}
}

// listingProperties are the properties_hash keys and values that tell one
// game's listings of a blueprint apart beside the condition: the language,
// and the finish where the game names it, a false flag as an empty value.
// Gundam and Palworld name no finish: the blueprint is the printing.
func listingProperties(gameID int, product Product) url.Values {
	props := product.Properties
	values := url.Values{}
	switch gameID {
	case GameMagic:
		values.Set("mtg_language", props.MTGLanguage)
		setFlag(values, "mtg_foil", props.MTGFoil)
	case GameLorcana:
		values.Set("lorcana_language", props.LorcanaLanguage)
		setFlag(values, "lorcana_foil", props.LorcanaFoil)
	case GameRiftbound:
		values.Set("riftbound_language", props.RiftboundLanguage)
		setFlag(values, "riftbound_foil", props.RiftboundFoil)
	case GameOnePiece:
		values.Set("onepiece_language", props.OnePieceLanguage)
		setFlag(values, "onepiece_foil", props.OnePieceFoil)
	case GameYuGiOh:
		values.Set("yugioh_language", props.YuGiOhLanguage)
		setFlag(values, "first_edition", props.FirstEdition)
	case GameFleshAndBlood:
		values.Set("fab_language", props.FabLanguage)
		values.Set("fab_foil_new", props.FabFoilNew)
		setFlag(values, "first_edition", props.FirstEdition)
	case GamePokemon:
		values.Set("pokemon_language", props.PokemonLanguage)
		setFlag(values, "pokemon_reverse", props.PokemonReverse)
		setFlag(values, "first_edition", props.FirstEdition)
	case GameGundam:
		values.Set("gundam_language", props.GundamLanguage)
	case GamePalworld:
		values.Set("palworld_language", props.PalworldLanguage)
	}
	return values
}

// setFlag sets a listingProperties flag, "true" when on and empty when off.
func setFlag(values url.Values, key string, on bool) {
	values.Set(key, "")
	if on {
		values.Set(key, "true")
	}
}

func (ct *Market) processExpansion(ctx context.Context, channel chan<- resultChan, expansionID int) error {
	allProducts, err := ct.client.ProductsForExpansion(ctx, expansionID)
	if err != nil {
		return err
	}

	for id, products := range allProducts {
		ct.processProducts(channel, id, products)
	}

	return nil
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (ct *Market) Load(ctx context.Context) error {
	rates, err := mtgban.GetExchangeRates(ctx)
	if err != nil {
		return err
	}
	ct.exchangeRates = rates

	if ct.targetEdition != "" {
		ct.printf("-> only targeting edition %s", ct.targetEdition)
	}
	blueprintsRaw, expansionsRaw, err := BlueprintsForGameID(ctx, ct.client, ct.gameID, ct.targetEdition, ct.printf)
	if err != nil {
		return err
	}
	ct.printf("Retrieved %d global sets", len(expansionsRaw))
	ct.printf("Found %d blueprints", len(blueprintsRaw))

	blueprints, expansions := FormatBlueprints(blueprintsRaw, expansionsRaw, false)
	ct.blueprints = blueprints
	if ct.gameID == GameFleshAndBlood {
		ct.fabListed = fabListedIDs(blueprints)
	}
	ct.printf("Parsing %d expansions with %d blueprints", len(expansions), len(blueprints))

	type expItem struct {
		id   int
		name string
	}
	expItems := make([]expItem, 0, len(expansions))
	for id, name := range expansions {
		expItems = append(expItems, expItem{id, name})
	}

	mtgban.WorkerPool(ctx, ct.maxConcurrency, expItems,
		func(ctx context.Context, item expItem, results chan<- resultChan) error {
			ct.printf("Processing %s [%d]", item.name, item.id)
			return ct.processExpansion(ctx, results, item.id)
		},
		func(result resultChan) {
			addCheapestOffer(ct.inventory, result, ct.printf)
		},
		ct.printf,
	)

	ct.inventoryDate = time.Now()

	return nil
}

// addCheapestOffer keeps the cheapest offer per condition and storefront,
// holding every offer's copies as its Available. Several blueprints can land
// on one card and arrive in any order, so an equal price goes to the
// blueprint id that sorts first. Singles and sealed both fold through it.
func addCheapestOffer(inventory mtgban.InventoryRecord, result resultChan, printf func(string, ...any)) {
	offer := result.invEntry
	offer.Available = offer.Quantity

	entries := inventory[result.cardID]
	for i := range entries {
		kept := entries[i]
		if kept.Conditions != offer.Conditions || kept.SellerName != offer.SellerName {
			continue
		}
		cheaper := offer.Price < kept.Price || (offer.Price == kept.Price && offer.OriginalID < kept.OriginalID)
		if !cheaper {
			entries[i].Available += offer.Quantity
			return
		}
		// Re-added rather than edited, so the entries stay sorted by price.
		offer.Available += kept.Available
		inventory[result.cardID] = slices.Delete(entries, i, i+1)
		break
	}

	err := inventory.Add(result.cardID, offer)
	if err != nil {
		printf("%s", err.Error())
	}
}

// Inventory returns what Load collected. See mtgban.Seller.
func (ct *Market) Inventory() mtgban.InventoryRecord {
	return ct.inventory
}

// MarketNames names the sub-sellers this market splits into. See
// mtgban.Market.
func (ct *Market) MarketNames() []string {
	return availableMarketNames
}

// InfoForScraper describes one of the sub-scrapers named above.
func (ct *Market) InfoForScraper(name string) mtgban.ScraperInfo {
	info := ct.Info()
	info.Name = name
	info.Shorthand = name2shorthand[name]
	return info
}

// Info describes this scraper. See mtgban.Scraper.
func (ct *Market) Info() (info mtgban.ScraperInfo) {
	info.Name = "Card Trader"
	info.Shorthand = "CTMarket"
	info.InventoryTimestamp = &ct.inventoryDate
	info.CountryFlag = "EU"
	info.Family = "CT"
	info.Game = ct.backend.Game
	return
}
