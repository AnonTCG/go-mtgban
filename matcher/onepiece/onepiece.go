// Package onepiece loads a One Piece Card Game datastore.
//
// The datastore is built by github.com/mtgban/datastore-gen's cmd/onepiece
// from the TCGplayer catalog dump for category 68, annotated with
// punk-records' mirror of the official Bandai card list. Identity is the
// catalog's: every entry is one priced printing of an English single
// product, so every uuid is priced by construction. Most products sell in
// a single finish, but the few sold both plain and foil carry one entry
// per finish, the foil one's id suffixed "_foil". Alternate arts,
// parallels and event printings share their base card's collector number
// and are told apart by the variant label the builder distills from the
// product name.
package onepiece

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/mtgban/go-mtgban/matcher"
)

// Datastore is the cmd/onepiece output: sets keyed by code, one card entry
// per priced finish, and the sealed products.
type Datastore struct {
	Game string `json:"game"`
	Sets map[string]struct {
		Name        string `json:"name"`
		ReleaseDate string `json:"releaseDate"`

		// Type is "promo" on the sets that hand their cards out rather
		// than sell them in packs, and empty on every other.
		Type string `json:"type,omitempty"`
	} `json:"sets"`
	Cards  []DatastoreCard   `json:"cards"`
	Sealed []DatastoreSealed `json:"sealed"`

	// Properties orders the values a set lists, one list per property named
	// in the singular: rarities rarest first, colours in the game's own order.
	Properties map[string][]string `json:"properties"`
}

// DatastoreCard is one printing as the datastore publishes it.
type DatastoreCard struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Number  string   `json:"number"`
	SetCode string   `json:"setCode"`
	Rarity  string   `json:"rarity"`
	Colors  []string `json:"colors"`
	Type    string   `json:"type"`

	// Variant is the label distilled from the product name's qualifiers:
	// empty for the base printing, "Alternate Art", "Parallel", "Manga",
	// "SP" or an event name for the others.
	Variant string `json:"variant,omitempty"`

	// PromoTypes are the labels the variant distils to, one entry each.
	// A datastore says them as words or as slugs depending on when it was
	// built, and promoTypeWords reads either.
	PromoTypes []string `json:"promoTypes,omitempty"`

	// Watermark is the mark saying which copy of a number this is where
	// nothing else does: the DON!! character, the deck, the instalment, or
	// the season a promotion ran in. It is part of a printing's wording,
	// not a promotion, so it is written back onto the label rather than
	// declared as a tag.
	Watermark string `json:"watermark,omitempty"`

	// OriginalReleaseDate is the day a promotion ran, where the set that
	// holds it says another. "One Piece Promotion Cards" is dated
	// 2022-09-30 and holds everything handed out since.
	OriginalReleaseDate string `json:"originalReleaseDate,omitempty"`

	// Language is the language a printing is printed in, where the catalog
	// calls it out. Everything else is English.
	Language string `json:"language,omitempty"`

	// Finish is the TCGplayer printing this entry prices, "Normal" or
	// "Foil". Entries sharing everything but the finish are the same
	// product sold both ways.
	Finish string `json:"finish"`

	Image         string `json:"image"`
	ExternalLinks struct {
		TcgPlayerID int `json:"tcgPlayerId"`

		// BandaiID is the official card list's _pN printing id, annotated
		// where the builder could align the two sources unambiguously.
		BandaiID string `json:"bandaiId,omitempty"`

		// CardmarketID is the Cardmarket product a printing no TCGplayer
		// product sells is priced by, published for the hand-carried ones.
		CardmarketID int `json:"cardmarketId,omitempty"`
	} `json:"externalLinks"`
}

// DatastoreSealed is one sealed product as the datastore publishes it.
type DatastoreSealed struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	SetCode       string `json:"setCode"`
	ReleaseDate   string `json:"releaseDate"`
	Image         string `json:"image"`
	ExternalLinks struct {
		TcgPlayerID int `json:"tcgPlayerId"`
	} `json:"externalLinks"`
}

// Load reads a One Piece datastore from r and returns a Backend for it, or
// an error when r holds something else. The datastore names its game at
// the root, and every card carries the identity fields the backend is built
// from.
//
// It reads the {"meta":...,"data":...} envelope the builders publish, where
// "data" holds the payload; a bare document reads as empty and is refused.
func Load(r io.Reader) (*matcher.Backend, error) {
	var envelope struct {
		Data Datastore `json:"data"`
	}
	if err := json.NewDecoder(r).Decode(&envelope); err != nil {
		return nil, err
	}
	payload := envelope.Data
	if payload.Game != "onepiece" || len(payload.Sets) == 0 || len(payload.Cards) == 0 {
		return nil, errors.New("not a One Piece datastore")
	}
	for _, card := range payload.Cards {
		// No number is a card the catalog files with none, not a wrong file.
		if card.ID == "" || card.Name == "" || card.Finish == "" {
			return nil, errors.New("not a One Piece datastore")
		}
	}
	return payload.newBackend(), nil
}

// setIsPromotional reports whether a set hands out promotional printings.
// The datastore types the sets it knows to be promotional; where it says
// nothing the name carries it. The promo set names itself "One Piece
// Promotion Cards", and the pre-release sets hand out stamped copies ahead
// of a release, which are promos by every other name. Matching on the name
// rather than the code keeps the Premium Booster sets out, whose codes
// begin "PRB" but which are an ordinary product.
func setIsPromotional(set *matcher.Set) bool {
	if set == nil {
		return false
	}
	if set.Type == "promo" {
		return true
	}
	lower := strings.ToLower(set.Name)
	return strings.Contains(lower, "promotion cards") || strings.Contains(lower, "pre-release")
}

// qualifiedName spells a printing the way TCGplayer names the product, the
// character name followed by the qualifier that tells it from its siblings
// ("Nami (Premium Card Collection -Best Selection Vol. 6-)"). Every One Piece
// card is named after a character, so the bare name reaches a hundred
// printings and the qualifier is the only thing that says which one; the
// catalog spells it out, so it is worth being able to search for. Empty for
// a base printing, which the bare name already describes.
func qualifiedName(card *DatastoreCard) string {
	if card.Variant == "" {
		return ""
	}
	return card.Name + " (" + card.Variant + ")"
}

func (payload *Datastore) newBackend() *matcher.Backend {
	b := matcher.NewBackend(matcher.IDSpaceTCGplayer, matcher.IDSpaceCardmarket)

	for code, set := range payload.Sets {
		b.AllSets = append(b.AllSets, code)
		releaseDateTime, _ := time.Parse("2006-01-02", set.ReleaseDate)
		b.Sets[code] = &matcher.Set{
			Name:            set.Name,
			Code:            code,
			ReleaseDate:     set.ReleaseDate,
			ReleaseDateTime: releaseDateTime,
			Type:            set.Type,
		}
	}

	printingsByName := map[string][]string{}
	for _, card := range payload.Cards {
		n := matcher.Normalize(card.Name)
		if !slices.Contains(printingsByName[n], card.SetCode) {
			printingsByName[n] = append(printingsByName[n], card.SetCode)
		}
	}

	for _, card := range payload.Cards {
		b.AddCanonicalName(card.Name)
		qualified := qualifiedName(&card)
		if qualified == "" {
			continue
		}
		for _, promoType := range card.PromoTypes {
			b.AddPromoType(matcher.PromoTypeSlug(promoType), promoTypeSpelling(promoType))
		}
		b.AddName(qualified)
	}

	// Group sibling entries back into their product: a dual-printing
	// product's Normal and Foil entries are the same card twice, and the
	// matcher wants it once, with FoilUUIDs naming the uuid each finish
	// prices.
	for _, group := range matcher.GroupProducts(payload.Cards, (*DatastoreCard).productKey) {
		printings := map[string]*DatastoreCard{}
		uuids := map[string]string{}
		for _, entry := range group {
			finish := matcher.FinishSlug(entry.Finish)
			printings[finish] = entry
			uuids[finish] = entry.ID
		}
		// The set-level card and the product id follow the plain printing
		// where one is sold, and the foil where it is not.
		card, found := matcher.DefaultPrinting(printings, false)
		if !found {
			card, found = matcher.DefaultPrinting(printings, true)
		}
		if !found {
			card = group[0]
		}
		if b.Sets[card.SetCode] == nil {
			continue
		}

		var promoTypes []string
		for _, promoType := range card.PromoTypes {
			promoTypes = append(promoTypes, matcher.PromoTypeSlug(promoType))
		}
		// The mark rides with the tags on the card, though it is never
		// declared as one. It is not a promotion - nothing promoted a
		// DON!! card for picturing Nami - so it has no place in the
		// vocabulary a query is written against; but every DON!! card is
		// named "DON!! Card" with no number, so what it pictures is the
		// only thing a listing can name to tell one from another, and
		// every rule that asks what a printing answers with reads this
		// list. The other games keep undeclared tokens on the card for
		// the same reason.
		if card.Watermark != "" {
			promoTypes = append(promoTypes, matcher.PromoTypeSlug(card.Watermark))
		}
		// A rarity the catalog also writes in a product name rides with
		// them for the same reason: a listing naming "TR" is naming that
		// printing and not the plain one beside it at the same number. The
		// datastore publishes the label where the catalog's name carried
		// one, and the rarity says it for the Treasure Rares whose did not.
		if quoted := quotedRarity(card.Rarity); quoted != "" {
			slug := matcher.PromoTypeSlug(quoted)
			if !slices.Contains(promoTypes, slug) {
				promoTypes = append(promoTypes, slug)
			}
		}
		// So does the date, where the datastore publishes one the set does
		// not state: "Treasure Cup August 2025" stands beside a plain
		// "Treasure Cup" at another number, and the date is the whole of
		// what a listing has to tell them apart by.
		if when := promoDate(card.OriginalReleaseDate); when != "" {
			promoTypes = append(promoTypes, matcher.PromoTypeSlug(when))
		}

		finishes, foilUUIDs := matcher.SoldFinishes(uuids)

		convertedCard := matcher.Card{
			UUID:     card.ID,
			Name:     card.Name,
			SetCode:  card.SetCode,
			Finishes: finishes,
			Number:   card.Number,
			Images: map[string]string{
				"full":      card.Image,
				"thumbnail": card.Image,
			},
			Language:            cmp.Or(card.Language, "English"),
			Colors:              matcher.ColorNames(card.Colors),
			Rarity:              b.AddRarity(card.Rarity),
			Types:               []string{card.Type},
			PromoTypes:          promoTypes,
			IsOversized:         slices.Contains(promoTypes, "oversized"),
			Watermark:           card.Watermark,
			OriginalReleaseDate: card.OriginalReleaseDate,
			IsPromo:             setIsPromotional(b.Sets[card.SetCode]),
			Printings:           printingsByName[matcher.Normalize(card.Name)],

			PlainNumber: Rules{}.PlainNumber(card.Number),
		}
		convertedCard.FoilUUIDs = foilUUIDs

		// Each identifier is guarded on its own. The product id and the
		// upstream id are separate facts about a printing, and gathering
		// them under one guard lost the second whenever the first was
		// missing: a card upstream names and TCGplayer does not sell
		// carried no identifier at all, though its own id was in hand.
		identifiers := map[string]string{}
		if card.ExternalLinks.TcgPlayerID != 0 {
			pid := fmt.Sprint(card.ExternalLinks.TcgPlayerID)
			identifiers["tcgplayerProductId"] = pid
			// The product id names the product, not one of its finishes, so
			// it points at the plain entry where that exists and at the foil
			// one when the card is only sold foil. MatchID re-resolves the
			// finish from the caller's own flag either way.
			b.ExternalIdentifiers[matcher.IDSpaceTCGplayer][pid] = card.ID
		}
		if id := card.ExternalLinks.BandaiID; id != "" {
			identifiers["bandaiId"] = id
		}
		if card.ExternalLinks.CardmarketID != 0 {
			mcmID := fmt.Sprint(card.ExternalLinks.CardmarketID)
			identifiers["mcmId"] = mcmID
			b.ExternalIdentifiers[matcher.IDSpaceCardmarket][mcmID] = card.ID
		}
		// A printing with none keeps the nil map it had, so nothing is
		// stamped with an empty string for want of a value.
		if len(identifiers) > 0 {
			convertedCard.Identifiers = identifiers
		}

		b.Sets[card.SetCode].Cards = append(b.Sets[card.SetCode].Cards, convertedCard)

		for _, entry := range group {
			b.AddPrinting(&convertedCard, entry.ID, matcher.FinishSlug(entry.Finish), card.Name, qualifiedName(card))
		}
	}

	b.Rarities = matcher.RarityNames(payload.Properties["rarity"])
	b.Colors = matcher.ColorNames(payload.Properties["color"])
	for _, set := range b.Sets {
		set.Rarities = matcher.RaritiesOf(set.Cards, b.Rarities)
		set.Colors = matcher.ColorsOf(set.Cards, b.Colors)
	}

	// Sealed products live in the sealed namespace throughout; AddSealed
	// is what files them there.
	for _, product := range payload.Sealed {
		b.AddSealed(product.ID, product.Name, product.SetCode, product.Image, product.ExternalLinks.TcgPlayerID)
	}
	b.Complete(Rules{})

	return b
}

// productKey names the product an entry is a printing of, read off what the
// entry publishes: the product id the catalog stamps on every printing it
// sells. An entry the builder mints carries none - it is minted one printing
// at a time, from an upstream record nothing else is minted from - so it is a
// product of one printing and stands for itself.
//
// Nothing here takes an id apart. The tail an id ends in is the builder's to
// spell, and a loader that reads one stops folding the day the spelling
// changes - which is exactly what "_holo" becoming "_holofoil" did here.
func (card *DatastoreCard) productKey() string {
	if card.ExternalLinks.TcgPlayerID != 0 {
		return fmt.Sprint(card.ExternalLinks.TcgPlayerID)
	}
	return card.ID
}
