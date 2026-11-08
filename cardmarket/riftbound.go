package cardmarket

import (
	"fmt"
	"strings"

	cm "github.com/mtgban/go-cardmarket"

	"github.com/mtgban/go-mtgban/matcher"
)

// bridgeNamesCard reports whether a bridged printing is the card the
// product names, allowing for the datastore's own shortened or misspelt
// wording. It is what keeps a bridge link to a fused-token product (whose
// TCGplayer id names an unrelated dual-faced card) from landing.
func bridgeNamesCard(b *matcher.Backend, product *cm.Product, cardID string) bool {
	co, err := b.GetUUID(cardID)
	if err != nil {
		return false
	}
	name, _, _ := strings.Cut(product.Name, " (V.")
	if matcher.CloseName(co.Name, name) {
		return true
	}
	// A promo's name carries what sets it apart, "(JP Exclusive)" and the like.
	bare, decorated := strings.CutSuffix(co.Name, ")")
	if i := strings.LastIndex(bare, " ("); decorated && i > 0 {
		return matcher.CloseName(bare[:i], name)
	}
	return false
}

// riftboundChineseShelf is the code of Project K Promos, Riftbound's Chinese
// release: its FND numbers follow no English set's, and CardTrader marks its
// items Chinese Exclusive.
const riftboundChineseShelf = "PROK"

// riftboundNonEnglish holds the products Cardmarket shelves among the English
// cards that were printed in another language alone: the Korean Ahri,
// Nine-Tailed Fox showcases (303a), and the Chinese Exclusive Kennen and
// Seraphine showcases.
var riftboundNonEnglish = map[int]bool{
	907588: true,
	916878: true,
	916943: true,
	917605: true,
	917606: true,
}

// riftboundFoilLinks names the TCGplayer product of the Vendetta: Promos
// runes (R01-R06, V.2): the shelf sells as its own product the foil rune that
// TCGplayer sells as the Foil of Vendetta's rune.
var riftboundFoilLinks = map[int]int{
	917119: 706028, // Fury Rune
	917120: 706010, // Calm Rune
	917123: 706067, // Mind Rune
	917140: 706006, // Body Rune
	917141: 706011, // Chaos Rune
	917151: 706078, // Order Rune
}

// riftboundShelves spells a Cardmarket shelf of promotional prints as the set
// the datastore files them in and the bundle it labels them with; a starred
// number is the bundle's serial-numbered print.
var riftboundShelves = map[string]struct{ edition, label, starLabel string }{
	"T1 2025 Worlds Champion Collection": {
		edition:   "Riftbound Promotional Cards",
		label:     "T1 Worlds Champion Signature Edition Bundle",
		starLabel: "T1 Worlds Champion Signature Edition Bundle Serial Numbered",
	},
}

// riftboundUnpricedShelves are the shelves whose printings no catalog row
// carries: Plated Legends' metal cards name no set, so their wording reaches
// the base sets' own printings, which other products already price.
var riftboundUnpricedShelves = map[string]bool{
	"Plated Legends - Metal Cards": true,
}

// otherSetPromo reports whether a product on a "<Set>: Promos" shelf landed
// on a printing promoted for another set of the datastore. The matcher folds
// every such shelf into one "Promos" heading, so a set's promo runes reach the
// earlier set's printings at the same numbers: Radiance's R01b-R06b land on
// Vendetta's, which the datastore carries and Vendetta: Promos already prices.
// A shelf naming its own set keeps its printings.
func (r *resolver) otherSetPromo(product *cm.Product, cardID string) bool {
	set, promos := strings.CutSuffix(product.ExpansionName, ": Promos")
	if r.gameID != cm.GameRiftbound || !promos {
		return false
	}
	co, err := r.backend.GetUUID(cardID)
	if err != nil {
		return false
	}
	for _, promoType := range co.PromoTypes {
		_, namesSet := r.backend.NormalizedSets[matcher.Normalize(promoType)]
		if namesSet && !matcher.Equals(promoType, set) {
			return true
		}
	}
	return false
}

// resolveRiftbound names a Riftbound product's printing: the one the
// datastore records under the product's id; else, for a Vendetta promo rune,
// the foil TCGplayer sells it as; else the one the bridge links, where the
// name still agrees with it; else the one its name and number reach, on the
// shelf riftboundShelves files it under.
func resolveRiftbound(r *resolver, product *cm.Product) (string, string, bool, error) {
	if riftboundNonEnglish[product.IDProduct] {
		return "", "", false, errForeign
	}
	cardID := r.backend.ConvertID(matcher.IDSpaceCardmarket, fmt.Sprint(product.IDProduct))
	if cardID != "" {
		return r.withFoil(cardID)
	}
	// The foil is the whole product: the nonfoil of the same TCGplayer
	// product is priced by the set's own rune.
	foilLink, linked := riftboundFoilLinks[product.IDProduct]
	if linked {
		id, err := r.backend.MatchIDFinish(fmt.Sprint(foilLink), "foil")
		if err != nil {
			return "", "", false, err
		}
		return id, id, false, nil
	}
	id, found := r.bridged(product)
	if found && bridgeNamesCard(r.backend, product, id) {
		if r.offShelf(product, id) {
			return "", "", false, errNoPrinting
		}
		return r.withFoil(id)
	}
	if riftboundUnpricedShelves[product.ExpansionName] {
		return "", "", false, nil
	}

	fields := strings.SplitN(product.Name, " (V.", 2)
	cardName := fields[0]
	number := product.Number
	edition := product.ExpansionName
	if len(fields) > 1 {
		number = strings.TrimSpace(number + " V." + strings.TrimSuffix(fields[1], ")"))
	}
	// Only the rarity tells an oversized card from the set's own at its
	// number; a card with no oversized printing is then unsupported.
	if product.Rarity == "Oversized" {
		number = strings.TrimSpace(number + " Oversized")
	}
	if shelf, found := riftboundShelves[product.ExpansionName]; found {
		label := shelf.label
		if strings.HasSuffix(product.Number, "*") {
			label = shelf.starLabel
		}
		edition, number = shelf.edition, strings.TrimSpace(number+" "+label)
	}

	cardID, cardIDFoil, err := r.matchNumbered(cardName, edition, number, true)
	if err != nil || cardID == "" {
		return "", "", false, err
	}
	if r.otherSetPromo(product, cardID) {
		return "", "", false, errNoPrinting
	}
	return cardID, cardIDFoil, false, nil
}
