package cardtrader

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/matcher/magic"
	"github.com/mtgban/go-mtgban/mtgban"
)

// priceToUSD converts a CardTrader price to dollars, through a rate table
// fetched once per run by mtgban.GetExchangeRates. A currency the feed does
// not quote is reported rather than being silently multiplied by the wrong
// rate.
//
// The whole table is held rather than a chosen few rates. The quoting currency
// belongs to the authenticating app and not to the listing - the same
// expansion comes back in dollars to one token and euros to another, and an
// app's setting can change between runs - so there is no list of currencies to
// be right about in advance, and being wrong about it costs an expansion at a
// time: one request fetches a whole expansion and the response quotes all of
// it in the one currency.
func priceToUSD(cents int, currency string, rates map[string]float64) (float64, error) {
	price := float64(cents) / 100
	if currency == "USD" {
		return price, nil
	}
	rate, found := rates[strings.ToLower(currency)]
	if !found {
		return 0, fmt.Errorf("unsupported currency %q", currency)
	}
	return price * rate, nil
}

// ExportStock returns your own listings as an InventoryRecord, using the
// Simple API token rather than the full one.
func (ct *CTAuthClient) ExportStock(ctx context.Context, b *matcher.Backend, blueprints map[int]*Blueprint) (mtgban.InventoryRecord, error) {
	products, err := ct.ProductsExport(ctx)
	if err != nil {
		return nil, err
	}

	rates, err := mtgban.GetExchangeRates(ctx)
	if err != nil {
		return nil, err
	}

	inventory := mtgban.InventoryRecord{}
	for _, product := range products {
		blueprint, found := blueprints[product.BlueprintID]
		if !found {
			continue
		}

		theCard, err := Preprocess(b, blueprint)
		if err != nil {
			continue
		}
		theCard.Foil = product.Properties.MTGFoil

		cardID, err := b.Match(theCard)
		if err != nil {
			continue
		}

		price, err := priceToUSD(product.PriceCents, product.PriceCurrency, rates)
		if err != nil {
			continue
		}

		quantity := product.Quantity

		condition, err := ctCondition(product.Properties.Condition)
		if err != nil {
			continue
		}

		inventory.AddRelaxed(cardID, &mtgban.InventoryEntry{
			Price:      price,
			Quantity:   quantity,
			Conditions: condition,
			SellerName: "mtgban",
			OriginalID: fmt.Sprint(product.BlueprintID),
			InstanceID: fmt.Sprint(product.ID),
		})
	}

	return inventory, nil
}

// listingPrice reads whichever of the three price fields a listing carries.
// The endpoints disagree on which one they fill - an export quotes
// price_cents, the marketplace quotes price, and an order quotes buyer_price -
// and each carries the currency it was quoted in, so the two travel together.
func listingPrice(product Product) (int, string) {
	if product.PriceCents != 0 {
		return product.PriceCents, product.PriceCurrency
	}
	if product.Price.Cents != 0 {
		return product.Price.Cents, product.Price.Currency
	}
	return product.BuyerPrice.Cents, product.BuyerPrice.Currency
}

// ConvertProducts turns listings into an InventoryRecord, resolving each
// through the blueprint it was sold against and pricing it in dollars by the
// rate table that mtgban.GetExchangeRates fills. A listing quoted in a
// currency the table does not cover is left out rather than priced by the
// wrong rate.
func ConvertProducts(b *matcher.Backend, blueprints map[int]*Blueprint, products []Product, rates map[string]float64) mtgban.InventoryRecord {
	inventory := mtgban.InventoryRecord{}
	for _, product := range products {
		bp, found := blueprints[product.BlueprintID]
		if !found {
			continue
		}
		theCard, err := Preprocess(b, bp)
		if err != nil {
			continue
		}
		theCard.Foil = product.Properties.MTGFoil

		cardID, err := b.Match(theCard)
		if err != nil {
			continue
		}

		cents, currency := listingPrice(product)
		price, err := priceToUSD(cents, currency, rates)
		if err != nil {
			continue
		}

		quantity := product.Quantity

		conds, err := ctCondition(product.Properties.Condition)
		if err != nil {
			continue
		}

		var customFields map[string]string
		if product.Description != "" || product.UserDataField != "" || product.Tag != "" {
			customFields = map[string]string{}
			if product.Description != "" {
				customFields["description"] = product.Description
			}
			if product.UserDataField != "" {
				customFields["user_data_field"] = product.UserDataField
			}
			if product.Tag != "" {
				customFields["tag"] = product.Tag
			}
		}

		inventory.AddRelaxed(cardID, &mtgban.InventoryEntry{
			Price:        price,
			Quantity:     quantity,
			Conditions:   conds,
			SellerName:   "mtgban",
			OriginalID:   fmt.Sprint(product.BlueprintID),
			InstanceID:   fmt.Sprint(product.ID),
			CustomFields: customFields,
		})
	}

	return inventory
}

// BlueprintsForGame fetches every blueprint of a game, one expansion at a
// time, skipping the expansions that fail to fetch rather than aborting
// the lot. A non-empty targetEdition narrows the fetch to that expansion
// by name or code; logf, when given, reports the skips. The expansions
// are returned too, since callers key edition names off them.
func BlueprintsForGame(ctx context.Context, client *CTAuthClient, game matcher.Game, targetEdition string, logf func(string, ...any)) ([]Blueprint, []Expansion, error) {
	gameID, found := ctGames[game]
	if !found {
		return nil, nil, fmt.Errorf("unsupported game %q", game)
	}
	return BlueprintsForGameID(ctx, client, gameID, targetEdition, logf)
}

// BlueprintsForGameID is the same fetch asked by Card Trader's own catalog
// number, which reaches the games above that mtgban models no Game for at
// all - Digimon, Vanguard, Union Arena and the rest of this catalog.
//
// It is also what the scrapers here call, because they already hold the id:
// asking by the typed game would make a run read one field where everything
// else reads the other, and the Game a scraper never set is Magic rather than
// nothing - so a disagreement would fetch Magic's blueprints and process them
// as some other game without saying a word.
func BlueprintsForGameID(ctx context.Context, client *CTAuthClient, gameID int, targetEdition string, logf func(string, ...any)) ([]Blueprint, []Expansion, error) {
	expansions, err := client.Expansions(ctx)
	if err != nil {
		return nil, nil, err
	}

	var blueprints []Blueprint
	for _, exp := range expansions {
		if exp.GameID != gameID {
			continue
		}
		// The OCG expansions are whole separate Japanese catalogs (bach-jp
		// beside bach) whose prices must not land on the TCG printings the
		// datastore carries.
		if gameID == GameYuGiOh && (strings.HasSuffix(exp.Code, "-jp") || strings.Contains(exp.Name, "OCG")) {
			continue
		}
		if targetEdition != "" && exp.Name != targetEdition && exp.Code != strings.ToLower(targetEdition) {
			continue
		}
		bp, err := client.Blueprints(ctx, exp.ID)
		if err != nil {
			if logf != nil {
				logf("skipping %d %s due to %s", exp.ID, exp.Name, err.Error())
			}
			continue
		}
		blueprints = append(blueprints, bp...)
	}
	return blueprints, expansions, nil
}

// gameLanguage and gameFoil read a listing's language and foil flag for the
// given game: cardtrader keys the properties per game, the Magic fields
// decoding empty for every other one.
func gameLanguage(gameID int, product Product) string {
	switch gameID {
	case GameMagic:
		return product.Properties.MTGLanguage
	case GameLorcana:
		return product.Properties.LorcanaLanguage
	case GameRiftbound:
		return product.Properties.RiftboundLanguage
	case GameOnePiece:
		return product.Properties.OnePieceLanguage
	case GameYuGiOh:
		return product.Properties.YuGiOhLanguage
	case GameFleshAndBlood:
		return product.Properties.FabLanguage
	case GamePokemon:
		return product.Properties.PokemonLanguage
	case GameGundam:
		return product.Properties.GundamLanguage
	case GamePalworld:
		return product.Properties.PalworldLanguage
	}
	return ""
}

// collectorNumberRe matches the collector number shapes One Piece's matcher
// reads out of a variation before it falls back to the first digit-leading
// word. A number of this shape wins that read whatever else the variation
// carries.
//
// The Greek letters belong in the tail because they are how this storefront
// numbers the corrected runs ("OP01-002β"), and those are the numbers whose
// Version cannot be dropped: several corrected printings share one base
// number, and the Version is the only thing that says which of them a
// listing is.
// parallelTail matches a Gundam collector number and the letters Card Trader
// hangs off it for a parallel run, keeping the number the datastore spells.
var parallelTail = regexp.MustCompile(`^([A-Za-z]+[0-9]*-[0-9]+)[a-zA-Z]+$`)

var collectorNumberRe = regexp.MustCompile(`^[A-Za-z]+[0-9]*-[0-9]+[a-zA-Z\x{03b1}\x{03b2}]*$`)

// opBlueprintNumbers are the collector numbers Card Trader gets wrong on a
// handful of ROUND1 promo blueprints, keyed by the blueprint since the
// number it wrote is itself a real card's number, just not this one's.
var opBlueprintNumbers = map[int]string{
	399332: "OP09-068",
	399334: "ST21-011",
	399335: "OP11-056",
	399336: "ST29-005",
}

// opNumber spells a One Piece blueprint's collector number the way the card
// wears it, where Card Trader wrote its own instead.
func opNumber(bp *Blueprint, number string) string {
	if spelled, found := opBlueprintNumbers[bp.ID]; found {
		return spelled
	}
	return number
}

// gameVariation spells the printing a blueprint names. One Piece, Riftbound
// and Yu-Gi-Oh all file several printings under one collector number, so the
// number alone aliases them; the blueprint's Version carries the very wording
// the datastore labels them with ("OP16 Release Event", "Summoner Skirmish |
// Champion", and for Yu-Gi-Oh the rarity itself), which is what tells them
// apart.
//
// The Version rides behind a number, and only then: its wording is full of
// years and volume numbers, and with nothing ahead of them one of those would
// answer as the collector number and select nothing at all.
func gameVariation(gameID int, bp *Blueprint, number string) string {
	if gameID == GameFleshAndBlood {
		number = fabNumber(bp, number)
		if fabPuzzleRe.MatchString(bp.Name) {
			return strings.TrimSpace(number + " " + fabPuzzlePiece(bp.Version))
		}
	}
	if gameID == GameYuGiOh {
		number = ygoNumber(bp, number)
	}
	if gameID == GamePokemon && bp.Expansion.Name == pkmLeagueShelf && !pkmCollectorNumberRe.MatchString(number) {
		return bp.Version
	}
	if gameID == GameOnePiece {
		number = opNumber(bp, number)
		// A leader or DON!! blueprint's number is a placeholder rather than
		// a real collector number, so the Version answers in its place.
		if bp.Version != "" && !collectorNumberRe.MatchString(number) &&
			(!strings.ContainsAny(number, "0123456789") || bp.Name == "DON!!") {
			return bp.Version
		}
	}
	if gameID == GameGundam {
		number = gundamNumber(bp, number)
	}
	// Only the category tells a Lorcana oversized card from the set's own at
	// its number, and a card with no oversized printing is then unsupported.
	if gameID == GameLorcana && bp.CategoryID == CategoryLorcanaOversized {
		return strings.TrimSpace(number + " Oversized")
	}
	if bp.Version == "" || number == "" {
		return number
	}
	switch gameID {
	case GameOnePiece:
		// A number that still fails collectorNumberRe here carries a digit
		// but not in a shape the matcher can read - "OP07-047P2" - so the
		// Version is dropped rather than guessed onto it.
		if !collectorNumberRe.MatchString(number) {
			return number
		}
	case GameRiftbound:
		// Riftbound numbers are bare ("202", "058c", "T05"), always readable.
	case GameYuGiOh:
		// Yu-Gi-Oh's Version is the rarity, which is the game's whole
		// identity beside the number, and the matcher narrows on nothing
		// else. The one wording to leave behind is the literal "Token": it
		// names the game's token products, and the shared token guard reads
		// that word out of the variation and refuses the listing outright.
		if bp.Version == "Token" {
			return number
		}
		// A handful of blueprints carry the copyright line printed at a
		// physical card's bottom instead of a rarity: Harpie Lady MRD-008
		// is filed twice, once original-artwork and once new-art, sharing
		// a name, number and rarity, and "©1996" is the only word Card
		// Trader ever writes for either of them. Spelling out the artwork
		// it names lets the matcher's own wording tiers pick the printing
		// the rarity cannot.
		if wording, found := ygoVersionWording[bp.Version]; found {
			return number + " " + wording
		}
	case GameFleshAndBlood:
		// A blueprint's version crosses the treatment with the wording
		// that picks between same-numbered printings, "Extended Art |
		// Rainbow Foil". The treatment half is already spoken for: the
		// listing states it as its own finish, off FabFoilNew, and saying
		// it twice narrows nothing. Only what the finish cannot say is
		// worth passing on.
		number, classic := fabPlainNumber(number)
		version := fabWording(fabVersions.Replace(bp.Version), number)
		if classic && !matcher.Contains(version, fabCCTag) {
			version = strings.TrimSpace(version + " " + fabCCTag)
		}
		if version == "" {
			return number
		}
		return number + " " + version
	case GameGundam:
		// Card Trader letters a parallel printing's collector number where
		// the datastore does not - "GD01-001a", "GD01-001aa", "ST05-010s" -
		// so the number has to lose the tail to name a printing at all.
		// Trimming alone is worse than nothing: it collapses every parallel
		// onto the base card. What separates them again is the Version,
		// which spells the rarity the way the datastore does, "LR+" beside
		// "LR++", and says what a rarity cannot besides - "Token", "Store
		// Tournament | Winner".
		number = parallelTail.ReplaceAllString(number, "$1")
	case GamePokemon:
		// Pokemon's Version carries what the bare number cannot: the real
		// collector number with its set total ("Holo Promo | 013/025" where
		// the number field says "013h"), the treatment that picks between
		// same-numbered printings ("Non-Holo", "Cosmos Holo", "Reverse
		// Holo"), and the label the promo sets tell their reprints apart by.
		// The matcher reads its numbers from the back, so the Version's
		// full number is asked before the decorated blueprint field.
		if spelled, found := pkmBlueprintVersions[bp.ID]; found {
			return number + " " + spelled
		}
		if spelled, found := pkmBlueprintNumbers[bp.ID]; found {
			number = spelled
		}
		return strings.TrimSpace(number + " " + pkmShelfVersion(bp))
	default:
		return number
	}
	return number + " " + bp.Version
}

// fabCCTag is the catalog's word for the Classic Constructed printing, which
// CardTrader spells "CC Label".
const fabCCTag = "CC Tag"

// fabVersions spells a Flesh and Blood version the way the catalog does.
var fabVersions = strings.NewReplacer("CC Label", fabCCTag)

// fabPlainNumber takes the Classic Constructed marker off a collector number
// and says whether it was there. The catalog files that printing at the plain
// number and tells it from its twin by the tag alone, so a number wearing the
// marker names nothing, and the listing settles for whichever other set
// prints the card.
func fabPlainNumber(number string) (string, bool) {
	trimmed, found := strings.CutSuffix(number, "cc")
	if !found || trimmed == "" {
		return number, false
	}
	// Only a number wears the marker; a name ending in those letters does not.
	last := trimmed[len(trimmed)-1]
	if last < '0' || last > '9' {
		return number, false
	}
	return trimmed, true
}

// fabTreatments are the values a Flesh and Blood listing states as its own
// finish. A version naming one of them adds nothing the finish has not
// already said.
var fabTreatments = map[string]bool{
	"rainbow foil": true,
	"cold foil":    true,
	"gold foil":    true,
	"regular":      true,
	"normal":       true,
}

// fabWording keeps the half of a blueprint's version that names a printing
// rather than a finish. A version restating the collector number - "DYN115"
// against the number DYN115 - names nothing either, and would only stutter.
func fabWording(version, number string) string {
	var kept []string
	for _, part := range strings.Split(version, "|") {
		part = strings.TrimSpace(part)
		if part == "" || fabTreatments[strings.ToLower(part)] || strings.EqualFold(part, number) {
			continue
		}
		kept = append(kept, part)
	}
	return strings.Join(kept, " ")
}

// gameName spells the card a blueprint names, which is not always its Name
// field. Yu-Gi-Oh's token blueprints say "Token" in the Version and leave the
// bare art in the name - "Dual Avatar Spirit", "Generaider" - while the
// catalog files those printings as "Token: Dual Avatar Spirit". The word
// cannot ride in the variation, where the shared token guard reads it and
// refuses the listing outright, so it moves into the name instead. A name
// already saying it is left alone: "Grinder Token" must not become
// "Token: Grinder Token".
// promoShelfNeedsLabel reports whether a blueprint matched by name alone
// has to answer with a promotional printing rather than an ordinary card.
//
// It answers yes for Gundam and for One Piece's promotional shelves
// (opPromoShelves), only because those games' numbers are not unique. Their
// promotional shelves reprint another set's card under that card's own
// number, and Card Trader files them under shelves the datastore names no set
// for - "Premium Accessory and Card Set", "Gundam Championships", "Reprints".
// The edition then narrows nothing and the number answers alone, with the
// ordinary card. A shelf that sells nothing but promotional reprints
// answering with a card carrying no promotional label is a contradiction, and
// publishing it files a promo's price at the ordinary card's identity.
//
// Yu-Gi-Oh asks of a blueprint whose version says misprint or counterfeit,
// which TCGplayer sells none of: the name and number reach the genuine card.
// Its answer must be a misprint printing (shelfLabelled), not any promo.
//
// One Piece asks only the closed list. For Gundam, a shelf that names a set
// is never asked, and so is one that opens with the game's own set code:
// Bandai brands its sets that way and the catalog spells the code into the
// set name too, so "GD-01: Newtype Rising" is a set's shelf even though no
// exact lookup answers for it. The starter decks are why the code has to be
// read rather than the name looked up - the catalog files them as "Starter
// Deck 01: Heroic Beginnings", which "ST-01: Heroic Beginnings" never reaches
// by name. Of Card Trader's 37 Gundam shelves, 4 name a set, 22 carry a code,
// and the 11 left are the promotional ones this asks about.
func promoShelfNeedsLabel(b *matcher.Backend, gameID int, bp *Blueprint) bool {
	if gameID == GameOnePiece {
		return opPromoShelves[bp.Expansion.Name]
	}
	if gameID == GameYuGiOh {
		return matcher.Contains(bp.Version, "misprint") || matcher.Contains(bp.Version, "counterfeit")
	}
	if gameID != GameGundam {
		return false
	}
	if codedShelf.MatchString(bp.Expansion.Name) {
		return false
	}
	// A mapped shelf names a set, so the number no longer answers alone
	// and there is nothing left to refuse.
	_, mapped := gundamShelfSets[bp.Expansion.Name]
	if mapped {
		return false
	}
	set, err := b.GetSetByName(bp.Expansion.Name)
	return err != nil || set == nil
}

// shelfLabelled reports whether an answer carries the label
// promoShelfNeedsLabel asks for. Yu-Gi-Oh needs a misprint promo type
// ("misprint", "unlimitedmisprint", "chaosneosmisprint"); every other game
// any promo type.
func shelfLabelled(gameID int, co *matcher.CardObject) bool {
	if gameID != GameYuGiOh {
		return len(co.PromoTypes) > 0
	}
	return slices.ContainsFunc(co.PromoTypes, func(promoType string) bool {
		return strings.HasSuffix(promoType, "misprint")
	})
}

// opPromoShelves are the One Piece shelves that sell nothing but promotional
// (or, on Pre-Errata Cards, pre-errata) printings of other sets' cards, so an
// answer with no promotional label is the base card answering for a promo the
// datastore does not carry yet.
var opPromoShelves = map[string]bool{
	"Winner Pack":              true,
	"Tournament Pack":          true,
	"Store Tournaments Promos": true,
	"Championships Promo":      true,
	"Pre-Errata Cards":         true,
}

// codedShelf matches a shelf named for the set it sells, opening with that
// set's code: "GD-01: Newtype Rising", "EB-01: Eternal Nexus", and the
// lowercase "St-14: Heavy Dominion" Card Trader writes for one of them. The
// code is captured in two halves, the letters that name the line and the
// number that names the set, for the sealed names that open the same way.
var codedShelf = regexp.MustCompile(`(?i)^([a-z]{1,4})-?([0-9]{1,2})\s*[:-]\s+`)

func gameName(b *matcher.Backend, gameID int, bp *Blueprint) string {
	if gameID == GameYuGiOh && bp.Version == "Token" &&
		!matcher.Contains(bp.Name, "token") {
		return "Token: " + bp.Name
	}
	if gameID == GameFleshAndBlood {
		if spelled, found := fabNames[bp.Name]; found {
			return spelled
		}
		if name := fabPuzzleName(b, bp); name != "" {
			return name
		}
	}
	if gameID == GameYuGiOh {
		if spelled, found := ygoBlueprintNames[bp.ID]; found {
			return spelled
		}
		if spelled, found := ygoNames[bp.Name]; found {
			return spelled
		}
	}
	if gameID == GamePokemon {
		if spelled, found := pkmNames[bp.Name]; found {
			return spelled
		}
		if bp.Expansion.Name == pkmProfessorProgramShelf {
			if energy, found := strings.CutPrefix(bp.Name, "Basic "); found && strings.HasSuffix(energy, " Energy") {
				return energy
			}
		}
	}
	return bp.Name
}

// ygoNames spells the Yu-Gi-Oh names Card Trader misspells. Most lost a run
// of letters somewhere in the feed - "Plant" is "ant", "Recklessly" is
// "Recklely", "Amazoness" is "Amazone" - and one is a card renamed by the
// catalog.
var ygoNames = map[string]string{
	"Cyber Repair ant":            "Cyber Repair Plant",
	"Fusion Recycling ant":        "Fusion Recycling Plant",
	"Rush Recklely":               "Rush Recklessly",
	"Mask of Weakne":              "Mask of Weakness",
	"Amazone Archers":             "Amazoness Archers",
	"Amazone Chain Master":        "Amazoness Chain Master",
	"Amazone Heirloom":            "Amazoness Heirloom",
	"Amazone Sage":                "Amazoness Sage",
	"Amazone Village":             "Amazoness Village",
	"Amazone Swords Woman":        "Amazoness Swords Woman",
	"Timelord Progenitor Vulgate": "Timelord Progenitor Vorpgate",
	"Neymar Jr":                   "Token: NEYMAR JR",
}

// ygoBlueprintNames are the Yu-Gi-Oh names a blueprint's own Name field
// cannot spell, keyed by the blueprint since the printed name differs from
// the one Card Trader gives it: the McDonald's promo prints "Frog The Jam"
// where the blueprint says "Slime Toad", Konami's own rename of the same
// Normal Monster (both share konamiId 68638985), and the two Speed Duel:
// Scars of Battle skill cards print "(Skill Card)" after their name.
var ygoBlueprintNames = map[int]string{
	81560: "Frog The Jam",
	80276: "Double Evolution Pill (Skill Card)",
	80277: "Heavy Metal Raiders (Skill Card)",
}

// ygoInserts are products Card Trader sells as Yu-Gi-Oh singles that are not
// cards.
var ygoInserts = map[string]bool{
	"Rainbow Front Filler Card": true,
}

// ygoForeignShelves are Card Trader Yu-Gi-Oh shelves that sell only German
// or Italian product, even where a seller marks a copy "en": the datastore
// names those rows after TCGplayer's German titles, or has none, so the plain
// name reaches another set's printing.
var ygoForeignShelves = map[string]bool{
	"Advent Calendar 2011":                   true,
	"Advent Calendar 2014":                   true,
	"GX Card Almanac":                        true,
	"Yu-Gi-Oh! The Official Magazine Promos": true,
}

// ygoBlueprintNumbers are the collector numbers Card Trader writes as an
// index of its own where the card wears another set's code, keyed by the
// blueprint since the index names another card of the shelf: the Raging
// Battle tin promos are RGBT-ENPP1 through RGBT-ENPP6 and filed under the
// booster as 001 through 006, the Force of the Breaker special edition's
// Volcanic Rocket is FOTB-ENSP1 filed as 001, the Reshef of Destruction
// promo of Sage's Stone is ROD-EN003 filed as 001sec, and the Stardust
// Accelerator promos are filed under each other's numbers.
var ygoBlueprintNumbers = map[int]string{
	70127: "RGBT-ENPP1",
	70129: "RGBT-ENPP2",
	70122: "RGBT-ENPP3",
	70128: "RGBT-ENPP4",
	70125: "RGBT-ENPP6",
	81236: "FOTB-ENSP1",
	75159: "ROD-EN003",
	85478: "WC09-EN002",
	85476: "WC09-EN003",
	// The duelist packs' Assault Mode Activate is DP09-EN022 filed as 002
	// and Captain Tenacious DP05-EN002 filed as 011; each pack holds the
	// card once.
	79602: "DP09-EN022",
	82918: "DP05-EN002",
	// Three Sneak Preview Series 3 promos filed under Card Trader's own
	// running index (001) on the booster each previewed - Gladiator's
	// Assault, Strike of Neos and Tactical Evolution - instead of their
	// own SP1 number.
	77618: "GLAS-ENSP1",
	82012: "STON-ENSP1",
	83641: "TAEV-ENSP1",
	// Video game promos filed under the shelf's own running count: Knight's
	// Title's two rarities both wear ROD-EN002, Blue-Eyes White Dragon's
	// Power of Chaos and Dark Duel Stories promos, and Duel Monsters GX Tag
	// Force's Red-Eyes Wyvern.
	75158: "ROD-EN002",
	75161: "ROD-EN002",
	73798: "PCK-001",
	81032: "DDS-001",
	78246: "GX06-EN002",
	// The R manga's own promo, Gorz.
	70002: "YR01-EN003",
	// Sengenjin's World Championship Series promo.
	78176: "WCS-EN403",
	// The McDonald's promo (a Normal Monster, not a skill card) and the two
	// Speed Duel: Scars of Battle skill cards.
	81560: "MP1-004",
	80276: "SBSC-ENS05",
	80277: "SBSC-ENS01",
	// The Swedish Shonen Jump copies of Blue-Eyes White Dragon and Red-Eyes
	// B. Dragon, which the English JMP-001 and JMP-002 are not.
	208973: "JMP-EN001",
	208974: "JMP-EN002",
}

// ygoNoRow are the blueprints in ygoBlueprintNumbers whose printing the
// datastore does not carry yet: the matcher reads JMP-EN001 as the English
// JMP-001, so an answer is kept only when it wears the blueprint's own number.
var ygoNoRow = map[int]bool{
	208973: true,
	208974: true,
}

// ygoShelfNumberRe matches the numbers Card Trader writes on the shelves
// that pool several sets, with the set's own index ahead of the card's:
// "5-001" on the Duelist League shelf is DL5-EN001, "1-E002" is DL1-E002,
// and "1-001" on the R comic shelf is YR01-EN001.
var ygoShelfNumberRe = regexp.MustCompile(`^([0-9]+)-(E?)([0-9]{3})$`)

// ygoTokenShelfRe matches the token shelves, numbered by their sheet.
var ygoTokenShelfRe = regexp.MustCompile(`^Token Promos ([0-9]+)$`)

// ygoNumber spells a Yu-Gi-Oh blueprint's collector number the way the
// catalog does, where Card Trader wrote its own index instead.
func ygoNumber(bp *Blueprint, number string) string {
	if spelled, found := ygoBlueprintNumbers[bp.ID]; found {
		return spelled
	}
	if m := ygoTokenShelfRe.FindStringSubmatch(bp.Expansion.Name); m != nil && number != "" {
		return "TKN" + m[1] + "-EN" + number
	}
	m := ygoShelfNumberRe.FindStringSubmatch(number)
	if m == nil {
		return number
	}
	infix := "EN"
	if m[2] != "" {
		infix = m[2]
	}
	switch bp.Expansion.Name {
	case "Duelist League Promos Upperdeck":
		return "DL" + m[1] + "-" + infix + m[3]
	case "R Comic Book Promos":
		return fmt.Sprintf("YR%02s-%s%s", m[1], infix, m[3])
	}
	return number
}

// ygoVersionWording spells the printing a blueprint's copyright-line Version
// names, keyed literally since the line is the catalog's own words for it.
// "©1996" is the only wording Card Trader ever gives Harpie Lady MRD-008's
// original-artwork half; its new-art twin never comes up in the feed.
var ygoVersionWording = map[string]string{
	"©1996": "Original Artwork",
}

// fabNames spells the Flesh and Blood names Card Trader misspells.
var fabNames = map[string]string{
	"Kassai of the Golden Sands": "Kassai of the Golden Sand",
	"In the Palm of Your Hands":  "In the Palm of Your Hand",
}

// fabCodes spells the set codes Card Trader misspells in its collector
// numbers: the Heavy Hitters tokens are "HBY240" through "HBY242".
var fabCodes = map[string]string{
	"HBY": "HVY",
}

// fabBlueprintNumbers are the collector numbers Card Trader got wrong on a
// blueprint, keyed by the blueprint since the number it wrote names another
// card of the set: the Alpha shelf's Nimble Strike is filed a hundred below
// WTR185-187 and its Wrecker Romp a hundred above WTR029-031 (Cardmarket
// files both under the numbers the cards wear), and the Chane blitz deck's
// Bounding Demigon is CHN009 where CHN010 is Piercing Shadow Vise.
var fabBlueprintNumbers = map[int]string{
	215356: "WTR185",
	215357: "WTR186",
	215358: "WTR187",
	215188: "WTR029",
	215189: "WTR030",
	215190: "WTR031",
	158167: "CHN009",
	// The Nuu blitz deck's Fang Strike and Slither carry the parent set's
	// numbers instead of the blitz deck's own.
	289007: "NUU028",
	289009: "NUU029",
	// Azalea, Ace in the Hole // Azalea: blank, and the listings' ARC038
	// names the back face of twelve other pairings.
	215671: "ARC039//ARC038",
}

// fabNumber spells a Flesh and Blood blueprint's collector number the way the
// card wears it.
func fabNumber(bp *Blueprint, number string) string {
	if spelled, found := fabBlueprintNumbers[bp.ID]; found {
		return spelled
	}
	for code, spelled := range fabCodes {
		if strings.HasPrefix(number, code) {
			return spelled + strings.TrimPrefix(number, code)
		}
	}
	return number
}

// fabDoubleSidedFace reports whether a Flesh and Blood collector number
// names no printing of its own, existing in the backend only as a face of
// at least two double-sided pairings and never standalone. See the commit
// for why a number that also carries a printing of its own (Spectral
// Shield's MST158-A/-B) is excluded. A standalone printing only counts when
// it is sold in the listing's own finish: a foil-only one is not what a
// regular listing can be, which leaves it a face of one of the pairings.
func fabDoubleSidedFace(b *matcher.Backend, number string, foil bool) bool {
	if number == "" || strings.Contains(number, "//") {
		return false
	}
	pairs := map[string]bool{}
	for _, uuid := range b.GetUUIDs() {
		co, err := b.GetUUID(uuid)
		if err != nil {
			continue
		}
		front, back, split := strings.Cut(co.Number, "//")
		if !split {
			rest, found := strings.CutPrefix(co.Number, number)
			if found && (rest == "" || !unicode.IsDigit(rune(rest[0]))) && co.Foil == foil {
				return false
			}
			continue
		}
		if front == number || back == number {
			pairs[co.Number] = true
		}
	}
	return len(pairs) >= 2
}

// fabPuzzleRe matches the name Card Trader gives a piece of a puzzle art
// card, quoting the face the piece is cut from.
var fabPuzzleRe = regexp.MustCompile(`^"(.+)" Macro Puzzle Card$`)

// fabPuzzleFaces spells the faces Card Trader names its own way: the map on
// the back of the Treasure Island puzzle is "Chart the High Seas" to it and
// "High Seas Map" to the datastore.
var fabPuzzleFaces = map[string]string{
	"Chart the High Seas":           "High Seas Map",
	"Kano, Dracai of Aether":        "Kano, Draci of Aether",
	"Taipanis, Dracai of Judgement": "Taipanis Draca of Judgement",
}

// fabArtCardSuffixes are the tails the datastore hangs on a double-sided art
// card's pair of faces.
var fabArtCardSuffixes = []string{" Double Sided Art Card", " Doubled Sided Art Card"}

// fabPuzzleName answers the datastore's name for a puzzle piece blueprint,
// empty for any other blueprint or a face no art card carries. Card Trader
// sells each face of a double-sided puzzle card as a product of its own,
// "\"Uzuri, Switchblade\" Macro Puzzle Card" and "\"Emperor, Dracai of Aesir\"
// Macro Puzzle Card", where the datastore keeps one printing per piece named
// by both faces; the face finds the printing, and the piece is the version's
// to say.
func fabPuzzleName(b *matcher.Backend, bp *Blueprint) string {
	face := fabPuzzleFace(bp.Name)
	if face == "" {
		return ""
	}
	uuids, err := b.SearchContains(face)
	if err != nil {
		return ""
	}
	var names []string
	for _, uuid := range uuids {
		co, err := b.GetUUID(uuid)
		if err != nil || !strings.Contains(co.Name, "//") {
			continue
		}
		if !strings.Contains(co.Name, "Art Card") && !strings.Contains(co.Number, "PZL") {
			continue
		}
		name := co.Name
		for _, suffix := range fabArtCardSuffixes {
			name = strings.TrimSuffix(name, suffix)
		}
		for part := range strings.SplitSeq(name, "//") {
			if matcher.Normalize(part) == matcher.Normalize(face) && !slices.Contains(names, co.Name) {
				names = append(names, co.Name)
			}
		}
	}
	if len(names) != 1 {
		return ""
	}
	return names[0]
}

// fabPuzzleFace answers the face a puzzle piece blueprint quotes, spelled
// the way the datastore spells it, empty for any other blueprint.
func fabPuzzleFace(name string) string {
	m := fabPuzzleRe.FindStringSubmatch(name)
	if m == nil {
		return ""
	}
	if spelled, found := fabPuzzleFaces[m[1]]; found {
		return spelled
	}
	return m[1]
}

// fabPuzzlePiece spells the piece a puzzle blueprint's version names the way
// the datastore labels it: the middle piece is "Center" to Card Trader and
// "Middle Center" or "Center" to the datastore, and the longer spelling
// describes both.
func fabPuzzlePiece(version string) string {
	if strings.EqualFold(version, "Center") {
		return "Middle Center"
	}
	return version
}

func gameFoil(gameID int, product Product) bool {
	switch gameID {
	case GameMagic:
		return product.Properties.MTGFoil
	case GameLorcana:
		return product.Properties.LorcanaFoil
	case GameRiftbound:
		return product.Properties.RiftboundFoil
	case GameOnePiece:
		return product.Properties.OnePieceFoil
	case GameFleshAndBlood:
		// Every treatment the listing names is a foil, and the print run
		// beside it is not one, so the treatment alone answers the flag.
		// Yu-Gi-Oh deliberately has no arm: the rarity is its treatment,
		// so every listing reads nonfoil through the default below.
		return fabTreatment(product) != "Normal"
	case GamePokemon:
		// Only the reverse holo is a foil treatment the flag can stand for;
		// a first-edition listing is whatever its rarity makes it, and the
		// named finish above is what actually resolves either one. This
		// answers the fallback that matches on text when the blueprint
		// carries no TCGplayer id.
		return product.Properties.PokemonReverse
	}
	return false
}

// lorcanaStarterFoils are the versions Card Trader gives a Lorcana starter
// deck's exclusive foil: its own words on the set's shelf, and "Rainbow Foil"
// on the promo shelves it files some of them on instead.
var lorcanaStarterFoils = map[string]bool{
	"Starter Deck Exclusive Foil": true,
	"Rainbow Foil":                true,
}

// unlimitedShelf matches the Card Trader shelves that sell the unlimited run
// of a set, which it names apart from the first one ("Monarch - Unlimited"
// beside "Monarch - First").
var unlimitedShelf = regexp.MustCompile(`(?i)\bUnlimited\b`)

// fabTreatment names a Flesh and Blood listing's treatment as the datastore
// spells it: the plain values Card Trader writes for no treatment are its
// Normal, and the stringly-false is what old listings carry.
func fabTreatment(product Product) string {
	switch product.Properties.FabFoilNew {
	case "", "Regular", "false":
		return "Normal"
	}
	return product.Properties.FabFoilNew
}

// fabListingID names the printing a Flesh and Blood listing lands on, or ""
// where it lands nowhere. The listing keeps the printing it resolved to when
// its treatment, whatever its print run, is the one that printing is sold in.
// A treatment no printing at that number is sold in names nothing, so the
// blueprint's product stands: the printing named by tcgID, or one carrying no
// promo type. A promo printing reached by name alone keeps the strict rule.
// Where a sibling at the number is sold in the treatment, the listing belongs
// to it and is skipped, unless it is the only one and no blueprint carries its
// product id: nothing else lists it, so the listing lands there.
// siblings is the fabNumbers index of the same backend and listed the
// fabListedIDs of its blueprints.
func fabListingID(b *matcher.Backend, siblings map[string][]*matcher.CardObject, listed map[string]bool, cardID string, tcgID int, product Product) string {
	co, err := b.GetUUID(cardID)
	if err != nil {
		return ""
	}
	landed, found := matcher.FinishOf(matcher.FinishSlug(co.Finish))
	if !found {
		return ""
	}
	want := matcher.FinishSlug(fabTreatment(product))
	if landed.Treatment == want {
		return cardID
	}
	ownProduct := tcgID != 0 && co.Identifiers["tcgplayerProductId"] == fmt.Sprint(tcgID)
	if len(co.PromoTypes) > 0 && !ownProduct {
		return ""
	}
	others := fabTreatmentAtNumber(siblings, co, want)
	switch len(others) {
	case 0:
		return cardID
	case 1:
		id := others[0].Identifiers["tcgplayerProductId"]
		if id != "" && !listed[id] {
			return others[0].UUID
		}
	}
	return ""
}

// fabListedIDs collects the TCGplayer product ids the blueprints carry.
func fabListedIDs(blueprints map[int]*Blueprint) map[string]bool {
	listed := map[string]bool{}
	for _, bp := range blueprints {
		id := bp.TCGplayerProductID()
		if id != 0 {
			listed[fmt.Sprint(id)] = true
		}
	}
	return listed
}

// fabNumbers indexes every printing of b by each part of its lowercased
// collector number, a Normal "ROS009//ROS021" under both. It is built once
// and only read.
func fabNumbers(b *matcher.Backend) map[string][]*matcher.CardObject {
	index := map[string][]*matcher.CardObject{}
	for _, uuid := range b.GetUUIDs() {
		co, err := b.GetUUID(uuid)
		if err != nil {
			continue
		}
		for _, part := range strings.Split(strings.ToLower(co.Number), "//") {
			index[part] = append(index[part], co)
		}
	}
	return index
}

// fabTreatmentAtNumber lists the printings at the collector number of co, in
// any set, sold in the treatment want and carrying every promo type co
// carries. Without one the seller's treatment names no printing, and the
// blueprint's own product is the only candidate.
func fabTreatmentAtNumber(siblings map[string][]*matcher.CardObject, co *matcher.CardObject, want string) []*matcher.CardObject {
	var found []*matcher.CardObject
	for _, part := range strings.Split(strings.ToLower(co.Number), "//") {
		for _, other := range siblings[part] {
			f, ok := matcher.FinishOf(matcher.FinishSlug(other.Finish))
			if ok && f.Treatment == want && carriesPromoTypes(other, co.PromoTypes) && !slices.Contains(found, other) {
				found = append(found, other)
			}
		}
	}
	return found
}

// carriesPromoTypes reports whether co carries every one of promoTypes.
func carriesPromoTypes(co *matcher.CardObject, promoTypes []string) bool {
	for _, pt := range promoTypes {
		if !slices.Contains(co.PromoTypes, pt) {
			return false
		}
	}
	return true
}

// gameFinish names the finish a listing prices, for the games whose
// properties name one rather than flagging it. Flesh and Blood is the only
// one so far: its treatment is a string, and the name reaches the printing's
// own Cold Foil sibling, which the boolean cannot - it has one bit for a
// game selling three treatments. A Normal listing is left to the flag.
func gameFinish(gameID int, bp *Blueprint, product Product) string {
	switch gameID {
	case GameFleshAndBlood:
		// The treatment and the print run cross, and the datastore gives
		// each crossing its own printing, so a first-edition listing has
		// to name both. Only the run is worth naming on its own: the
		// unlimited one is what a product answers with by default.
		treatment := fabTreatment(product)
		if product.Properties.FirstEdition {
			return matcher.PrintingFinish("1st Edition", treatment)
		}
		if treatment == "Normal" {
			// The flag only says "not first", which names a printing
			// wherever the unlimited run exists and names nothing where
			// it does not - and Card Trader shelves the runs apart, so
			// the shelf is what can say the run outright.
			if bp != nil && unlimitedShelf.MatchString(bp.Expansion.Name) {
				return matcher.PrintingFinish("Unlimited Edition", treatment)
			}
			return ""
		}
		return treatment
	case GameYuGiOh:
		// The rarity is Yu-Gi-Oh's treatment and the print run is its
		// finish, so the run is the whole of what a listing names. Only
		// the first edition is worth naming: a product answers a lowered
		// flag with its unlimited printing already.
		if product.Properties.FirstEdition {
			return "1st Edition"
		}
	case GamePokemon:
		// Both flags name a printing the foilness cannot reach. The reverse
		// holo sits beside a holo rare's own printing, which is foil too, so
		// the flag lands on the wrong one of the pair; the first-edition run
		// sits beside the unlimited one, which the flag cannot tell it from
		// at all.
		if product.Properties.PokemonReverse {
			return "Reverse Holofoil"
		}
		// The listing flags never say a card is a holo, the blueprint's
		// version does, and a product sold in Normal and Holofoil answers
		// a listing naming neither with its Normal printing.
		treatment := ""
		if bp != nil && pkmHoloVersion.MatchString(bp.Version) && !pkmNotHoloVersion.MatchString(bp.Version) {
			treatment = "Holofoil"
		}
		if product.Properties.FirstEdition {
			return matcher.PrintingFinish("1st Edition", treatment)
		}
		return treatment
	}
	return ""
}

// pkmHoloVersion matches a Pokemon version naming a holo treatment: "Holo
// Rare", "Holo Promo", "Cosmos Holo", "Sheen Holo". pkmNotHoloVersion matches
// the ones that name a different treatment with the same word.
var (
	pkmHoloVersion    = regexp.MustCompile(`(?i)\bholo\b`)
	pkmNotHoloVersion = regexp.MustCompile(`(?i)\bnon[- ]?holo\b|\breverse\b`)
)

// foilPrintingID adopts the foil printing's id for a foil listing, which
// CardTrader files under the plain printing's. A printing that already
// carries a finish keeps its own id: an etched listing raises the same foil
// flag, and the flag has one bit for a card sold in two premium finishes, so
// answering it would walk the etched printing back onto the foil one. The
// resolved card's own name is asked, since the vendor's spelling of it may
// not be one the datastore sells in foil.
func foilPrintingID(b *matcher.Backend, cardID string) string {
	co, err := b.GetUUID(cardID)
	if err != nil || co.Foil || co.Etched {
		return cardID
	}

	// A derived token pairing's own combined name is deliberately excluded
	// from every name index (see matcher/magic/tokenpairs.go's own doc
	// comment on the type), so HasFoilPrinting below always answers false
	// for one regardless of whether the physical product was actually
	// ever sold in foil - silently keeping the nonfoil id, and nonfoil
	// price, for a listing Card Trader itself flagged as foil. Resolved
	// the same way the rest of this feature resolves a finish: by the
	// pairing's own tcgplayerProductId plus the foil flag, not by name,
	// and refusing (returning "") rather than guessing wrong when the
	// pairing was never sold in that finish at all.
	if co.Identifiers["derivedTokenPair"] == "true" {
		verified := magic.VerifyTokenPairingFinish(b, co.Identifiers["tcgplayerProductId"], true)
		if verified == "" {
			return ""
		}
		foilID, err := b.MatchID(verified, true)
		if err != nil {
			return ""
		}
		return foilID
	}

	if !b.HasFoilPrinting(co.Name) {
		return cardID
	}
	foilID, err := b.MatchID(cardID, true)
	if err != nil {
		return cardID
	}
	return foilID
}

// FormatBlueprints indexes blueprints by id and expansions by name, keeping
// either the sealed products or the singles.
func FormatBlueprints(blueprints []Blueprint, inExpansions []Expansion, sealed bool) (map[int]*Blueprint, map[int]string) {
	// Create a map to be able to retrieve edition name in the blueprint
	formatted := map[int]*Blueprint{}
	expansions := map[int]string{}
	for i := range blueprints {
		// Sealed is selected by exclusion, so a category cardtrader adds
		// later lands on the sealed side rather than silently vanishing;
		// accessories arrive with it and are dropped downstream by the
		// product-map resolution, which only names real sealed products.
		// A token and an oversized card are singles, whichever game
		// prints them: the sealed side would otherwise be asked about a
		// card by a name it shares with the set it came in.
		singles := false
		switch blueprints[i].CategoryID {
		case CategoryMagicSingles, CategoryMagicTokens, CategoryMagicOversized,
			CategoryLorcanaSingles, CategoryLorcanaOversized, CategoryLorcanaTokens,
			CategoryRiftboundSingles, CategoryRiftboundOversized,
			CategoryOnePieceSingles, CategoryOnePieceOversized, CategoryOnePieceDon,
			CategoryYuGiOhSingles, CategoryYuGiOhOversized,
			CategoryFleshAndBloodSingles, CategoryFleshAndBloodArtCardTokens,
			CategoryPokemonSingles, CategoryPokemonOversized,
			CategoryGundamSingles, CategoryPalworldSingles:
			singles = true
		}
		if singles == sealed {
			continue
		}

		// Keep track of blueprints as they are more accurate that the
		// information found in product
		formatted[blueprints[i].ID] = &blueprints[i]

		// Load expansions array
		_, found := expansions[blueprints[i].ExpansionID]
		if !found {
			for j := range inExpansions {
				if inExpansions[j].ID == blueprints[i].ExpansionID {
					expansions[blueprints[i].ExpansionID] = inExpansions[j].Name
				}
			}
		}

		// The name is missing from the blueprints endpoint, fill it with data
		// retrieved from the expansions endpoint
		formatted[blueprints[i].ID].Expansion.Name = expansions[blueprints[i].ExpansionID]

		// Move the blueprint properties from the custom structure from blueprints
		// to the place as expected by Preprocess()
		formatted[blueprints[i].ID].Properties = formatted[blueprints[i].ID].FixedProperties
	}

	return formatted, expansions
}

// gundamShelfSets names the Card Trader Gundam shelves that sell one set of
// the catalog under a name the catalog does not use. Card Trader sells the
// deck build box's reprints as "Reprints", and files every card on that
// shelf under the number of the card it reprints, so a shelf naming no set
// narrows nothing and the number aliases onto the original printing.
var gundamShelfSets = map[string]string{
	"Reprints": "SC01",
}

// gundamBlueprintNumbers are the collector numbers Card Trader gets wrong on
// two Reprints-shelf blueprints, keyed by the blueprint since the number it
// wrote is a real card's number from an unrelated set, not an SC01 one:
// Aile Strike Gundam wears GD03's own GD03-072, and Davao wears ST05's own
// Isaribi at ST05-015.
var gundamBlueprintNumbers = map[int]string{
	400427: "ST04-001",
	400433: "ST08-015",
}

// gundamNumber spells a Gundam blueprint's collector number the way the
// card wears it, where Card Trader wrote another set's number instead.
func gundamNumber(bp *Blueprint, number string) string {
	if spelled, found := gundamBlueprintNumbers[bp.ID]; found {
		return spelled
	}
	return number
}

// palworldShelfSets are the sets Card Trader's Palworld shelves hold.
var palworldShelfSets = map[string]string{
	"Dawn of Palpagos":              "BP01",
	"Legends Awaken":                "BP02",
	"Dawn of Palpagos Red・Blue":     "TD01",
	"Dawn of Palpagos Green・Purple": "TD02",
}

// palworldNumber answers a card's own number where Card Trader filed another
// set's in its place: 65 trial-deck blueprints carry EBP01-001SSP. The run is
// the card's on the blueprint's shelf, and a "Special Rare" version picks its
// one parallel; two parallels leave the number as it came, to be refused.
func palworldNumber(b *matcher.Backend, bp *Blueprint, number string) string {
	set, found := palworldShelfSets[bp.Expansion.Name]
	if !found {
		return number
	}
	code, _, _ := strings.Cut(number, "-")
	parallel := strings.Contains(bp.Version, "Special Rare")
	var picked []string
	for _, uuid := range b.Hashes[matcher.Normalize(bp.Name)] {
		co, err := b.GetUUID(uuid)
		if err != nil || co.Sealed || co.SetCode != set || co.Number == "" {
			continue
		}
		// A number from the shelf's own run is the card's, or a slip this
		// cannot correct.
		if strings.HasPrefix(co.Number, code+"-") {
			return number
		}
		if (co.Number != co.PlainNumber) == parallel {
			picked = append(picked, co.Number)
		}
	}
	if len(picked) != 1 {
		return number
	}
	return picked[0]
}

// ygoBlueprintEditions are the sets Card Trader shelves a promo under the
// booster it was released with, keyed by the blueprint: the Raging Battle
// tin promos RGBT-ENPP1 through RGBT-ENPP6 are filed under "Raging Battle",
// which holds Level Retuner at RGBT-EN069 alone, and the Force of the
// Breaker sneak preview promo under the booster.
var ygoBlueprintEditions = map[int]string{
	70127: "Duelist Pack Collection Tin",
	70129: "Duelist Pack Collection Tin",
	70122: "Duelist Pack Collection Tin",
	70128: "Duelist Pack Collection Tin",
	70125: "Duelist Pack Collection Tin",
	81236: "Sneak Preview Series 3",
	77618: "Sneak Preview Series 3",
	82012: "Sneak Preview Series 3",
	83641: "Sneak Preview Series 3",
	75158: "Yu-Gi-Oh! Video Game Promotional Cards",
	75161: "Yu-Gi-Oh! Video Game Promotional Cards",
	73798: "Yu-Gi-Oh! Video Game Promotional Cards",
	81032: "Yu-Gi-Oh! Video Game Promotional Cards",
	78246: "Yu-Gi-Oh! GX Tag Force Promotional Cards",
	70002: "Yu-Gi-Oh! R Manga Promo (YR01)",
	78176: "World Championship Series",
}

// opBlueprintEditions are the One Piece blueprints Card Trader shelves under
// an edition the card's own wording cannot reach otherwise: ST-17's reprints
// of two ST-03 cards carry TCGplayer ids the catalog has since dropped, so
// they need the parent starter spelled out the way their shelf-mates already
// land it; Radical Beam!!'s Version is "Demo Deck", which the matcher does
// not read as wording for OP-DD, and its shelf, "One Piece Promos", names no
// set to fall back on either.
var opBlueprintEditions = map[int]string{
	305520: "Starter Deck 3: The Seven Warlords of The Sea",
	305524: "Starter Deck 3: The Seven Warlords of The Sea",
	377503: "One Piece Demo Deck Cards",
}

// gameEdition names the set a blueprint's shelf sells, which is the shelf's
// own name everywhere but the shelves above.
func gameEdition(b *matcher.Backend, gameID int, bp *Blueprint) string {
	if gameID == GameGundam {
		code, found := gundamShelfSets[bp.Expansion.Name]
		if found {
			set, err := b.GetSet(code)
			if err == nil {
				return set.Name
			}
		}
	}
	if gameID == GameOnePiece {
		if edition, found := opBlueprintEditions[bp.ID]; found {
			return edition
		}
		// DON!! cards on the general promos shelf carry no number the
		// matcher can read (see the placeholder-number branch in
		// gameVariation), so the edition is the only thing left to narrow
		// them - and "One Piece Promos" itself names no set.
		if bp.Name == "DON!!" && bp.Expansion.Name == "One Piece Promos" {
			return "One Piece Promotion Cards"
		}
	}
	if gameID == GameYuGiOh {
		if edition, found := ygoBlueprintEditions[bp.ID]; found {
			return edition
		}
	}
	if gameID == GamePokemon {
		if edition, found := pkmShelfEditions[bp.Expansion.Name]; found && !strings.Contains(bp.Version, "Jumbo") {
			return edition
		}
	}
	return bp.Expansion.Name
}

// pkmShelfEditions are the Pokemon shelves Card Trader heads by the era's
// black star promos where the catalog names the era's promo set. The
// matcher's own alias table leaves these out because other storefronts
// file jumbos under the same words; Card Trader says "Jumbo" in the
// version, and a jumbo keeps the shelf's own name.
var pkmShelfEditions = map[string]string{
	"SV Black Star Promos":   "SV: Scarlet & Violet Promo Cards",
	pkmProfessorProgramShelf: "Professor Program Promos",
}

// pkmProfessorProgramShelf is the shelf Card Trader heads by the promo
// program's own name, where the catalog spells the set "Professor Program
// Promos". Every version on the shelf also leads with "Professor Program
// Stamp", which no catalog row carries as a promo type; pkmShelfVersion
// strips it before the wording reaches the matcher.
const pkmProfessorProgramShelf = "Professor Program"

// pkmHolidayCalendarShelf is the shelf whose version names the stamp Card
// Trader itself prints on these cards ("Holiday Snowflake Stamp") where the
// catalog's own promo label is "Holiday Calendar".
const pkmHolidayCalendarShelf = "Holiday Calendar"

// pkmShelfVersion spells a Pokemon blueprint's version the way the catalog
// does, on the shelves where Card Trader's own wording never matches a row.
func pkmShelfVersion(bp *Blueprint) string {
	version := bp.Version
	switch bp.Expansion.Name {
	case pkmProfessorProgramShelf:
		if rest, found := strings.CutPrefix(version, "Professor Program Stamp"); found {
			version = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), "|"))
		}
	case pkmHolidayCalendarShelf:
		version = strings.Replace(version, "Holiday Snowflake Stamp", "Holiday Calendar", 1)
	}
	return version
}

// pkmBlueprintNumbers are the Pokemon blueprints whose number Card Trader
// writes bare where the catalog wears the shelf's own SWSH prefix.
var pkmBlueprintNumbers = map[int]string{
	127065: "SWSH002",
	127087: "SWSH020",
}

// pkmBlueprintVersions are the Pokemon blueprints whose version carries a
// set total that does not match the printed card - Card Trader wrote a
// different set's total instead of this card's own.
var pkmBlueprintVersions = map[int]string{
	357920: "Destined Rivals Stamp | 031/182",
	357923: "Destined Rivals Stamp | 208/182",
	357913: "Destined Rivals Stamp | 229/182",
	354633: "Cosmos Holo 026/094",
	364952: "Cosmos Holo | 079/086",
	280947: "148/172",
}

// pkmNames spells the Pokemon names Card Trader misspells.
var pkmNames = map[string]string{
	"Psichic Energy": "Psychic Energy",
}

// pkmUnrowedShelves are Card Trader Pokemon shelves the English catalog has
// no row for: Japanese sets, even where a seller marks a copy "en", and
// misprints, which TCGplayer does not sell.
var pkmUnrowedShelves = map[string]bool{
	"Ruler of the Black Flame":                    true,
	"Night Wanderer":                              true,
	"Shiny Treasure ex":                           true,
	"Battle Partners":                             true,
	"Red Collection":                              true,
	"White Collection":                            true,
	"Towering Perfection":                         true,
	"Battle Strength Decks":                       true,
	"Scarlet & Violet Promos":                     true,
	"Lost Link":                                   true,
	"M Audino EX Mega Battle Deck":                true,
	"Mythical & Legendary Dream Shine Collection": true,
	"Shaymin LV. X COLLECTION PACK":               true,
	"Space-Time Creation":                         true,
	"Stellar Miracle":                             true,
	"Stellar Miracle Deck Build Box":              true,
	"Pokémon Misprints":                           true,
}

// pkmInserts are the products Card Trader sells as Pokemon singles that are
// not cards.
var pkmInserts = map[string]bool{
	"VSTAR Marker":      true,
	"Blank Filler Card": true,
}

// gundamInserts are the products Card Trader sells as Gundam singles that are
// not cards.
var gundamInserts = map[string]bool{
	"Damage Counter Punch Card": true,
}

// lorcanaInserts are products Card Trader sells as Lorcana singles that are
// not cards.
var lorcanaInserts = map[string]bool{
	"Discard Filler Card": true,
}

// lorcanaErrataShelf is where Card Trader sells Lorcana's misprints and
// pre-errata runs. TCGplayer sells none apart from the card, so the datastore
// has no printing of them, and their names reach a row another blueprint owns.
const lorcanaErrataShelf = "Errata Cards"

// pkmLeagueShelf is the shelf whose number field carries the card's year
// or its online code rather than a collector number: the league energies
// are unnumbered, and "2006" or "KUF-7XB-05C" names nothing the catalog
// numbers. The version says the year and the treatment, which is what
// the catalog labels them by.
const pkmLeagueShelf = "League Promos"

// pkmCollectorNumberRe matches a Pokemon collector number, with or without
// its set total, as opposed to a year or an online code.
var pkmCollectorNumberRe = regexp.MustCompile(`^[A-Za-z]{0,4}[0-9]{1,3}[a-z]?(?:/[0-9]{1,3})?[a-z]?$`)

// unsupportedBlueprint reports a blueprint no datastore carries a card for.
func unsupportedBlueprint(gameID int, bp *Blueprint) bool {
	switch gameID {
	case GamePokemon:
		return pkmInserts[bp.Name] || pkmUnrowedShelves[bp.Expansion.Name]
	case GameLorcana:
		// The shelf's one printing the datastore carries has a TCGplayer id.
		return lorcanaInserts[bp.Name] || (bp.Expansion.Name == lorcanaErrataShelf && bp.TCGplayerProductID() == 0)
	case GameYuGiOh:
		return ygoInserts[bp.Name] || ygoForeignShelves[bp.Expansion.Name]
	case GameGundam:
		return gundamInserts[bp.Name]
	default:
		return false
	}
}
