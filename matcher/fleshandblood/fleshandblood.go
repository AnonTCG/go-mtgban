// Package fleshandblood loads a Flesh and Blood datastore.
//
// The datastore is built by github.com/mtgban/datastore-gen's
// cmd/fleshandblood from the TCGplayer catalog dump for category 62,
// annotated with the official Legend Story Studios card identifiers where
// the builder could align the two sources. Identity is the catalog's: every
// English single product is one card, and the print run and foil treatment
// combinations it sold in (1st Edition or Unlimited Edition or neither,
// crossed with Normal, Rainbow Foil or Cold Foil) are priced separately, so
// each entry is one printing of one product. The plain Normal entry keeps
// the bare id; the others suffix it with the edition and treatment slugs
// (_1e, _unlrainbow, _cold); sibling entries share their product's
// tcgPlayerId. Extended arts, marvels and the other alternate printings
// share their base card's collector number and are told apart by the
// variant label the builder distills from the product name.
package fleshandblood

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

// Datastore is the cmd/fleshandblood output: sets keyed by code, one card
// entry per priced printing, and the sealed products.
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
	ID      string `json:"id"`
	Name    string `json:"name"`
	Number  string `json:"number"`
	SetCode string `json:"setCode"`
	Rarity  string `json:"rarity"`

	// Pitches is the colour a printing pitches for - "Red", "Yellow",
	// "Blue" - which the catalog carries as the pitch value printed on the
	// card. A card that pitches for nothing carries none: a hero, an
	// equipment, a token.
	Pitches []string `json:"pitches,omitempty"`

	// Finish is the TCGplayer printing this entry prices, drawn from the
	// closed vocabulary of "1st Edition" or "Unlimited Edition" (or
	// neither) crossed with "Normal", "Rainbow Foil" or "Cold Foil".
	// Entries sharing everything but the finish are the same product sold
	// several ways.
	Finish string `json:"finish"`

	// Variant is the label distilled from the product name's qualifiers:
	// empty for the base printing, "Extended Art", "Marvel", "Golden" or
	// the like for the alternate printings that share its number.
	Variant string `json:"variant,omitempty"`

	// PromoTypes is the same residue as the labels it is made of, which
	// the joined Variant cannot be read back into: "Cold Foil Extended
	// Art" is two labels, and a query naming either has to reach the
	// printing.
	PromoTypes []string `json:"promoTypes,omitempty"`

	// Artist is the credit the catalog never carries at all, joined from
	// every face's own name where the printing has more than one - it is
	// the only field that tells two products sharing a number and a
	// stripped-down name apart, where two Aurora (Marvel) rows are one
	// card's Extended Art and the other its Alternate Art and nothing
	// else in this struct says which is which.
	Artist string `json:"artist,omitempty"`

	// Watermark is the mark saying which copy of a number this is where
	// nothing else does: the artwork letter that is all there is between
	// three printings of Lightning Flow at OMN203. It is no promotion -
	// nothing promoted a card for being the second drawing of it - so it
	// rides with the tags on the card without being declared as one.
	Watermark string `json:"watermark,omitempty"`

	// Language is what the printing is printed in, where that is not
	// English: the Japanese and Korean exclusives TCGplayer sells in no
	// English sku. Empty means English, which is what every other entry is.
	Language string `json:"language,omitempty"`

	Image         string `json:"image"`
	ExternalLinks struct {
		TcgPlayerID int `json:"tcgPlayerId"`

		// FabID is the official Legend Story Studios card identifier,
		// annotated where the builder could align the two sources.
		FabID string `json:"fabId,omitempty"`
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

// Load reads a Flesh and Blood datastore from r and returns a Backend for
// it, or an error when r holds something else. The datastore
// names its game at the root, and every card carries the identity fields
// the backend is built from. The collector number is not among them: the
// set art cards carry none, and a card the catalog sells under no number
// still has to be sold.
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
	if payload.Game != "fleshandblood" || len(payload.Sets) == 0 || len(payload.Cards) == 0 {
		return nil, errors.New("not a Flesh and Blood datastore")
	}
	for _, card := range payload.Cards {
		if card.ID == "" || card.Name == "" || card.Finish == "" {
			return nil, errors.New("not a Flesh and Blood datastore")
		}
	}
	return payload.newBackend(), nil
}

// restatesNumber reports whether a label only respells the printing's own
// collector number, which as a tag describes nothing the number does not
// already say. Both sides lose their separators and their leading zeros
// first: the catalog writes "HER0156" where the label says "HER156" and
// "MST158-A" where it says "158-A". A label naming a different printing's
// number stays - "FAB442" on PEN331 is a promo pointing at the printing it
// reprints, which is worth reading.
func restatesNumber(variant, number string) bool {
	label := foldNumber(variant)
	full := foldNumber(number)
	if label == "" || full == "" {
		return false
	}
	if label == full || label == strings.TrimLeft(full, "abcdefghijklmnopqrstuvwxyz") {
		return true
	}
	// The base number of a suffixed printing ("LSS003" on LSS003-CF): what
	// follows the label has to open a suffix rather than carry on the digits,
	// so "HER1" is not read as a respelling of HER156.
	rest := strings.TrimPrefix(full, label)
	if rest == full || rest == "" {
		return false
	}
	return rest[0] < '0' || rest[0] > '9'
}

// foldNumber reduces a collector number to the letters and digits that carry
// it, dropping the zeros each digit run is padded with.
func foldNumber(number string) string {
	var out strings.Builder
	zeros := true
	for _, r := range strings.ToLower(number) {
		switch {
		case r >= 'a' && r <= 'z':
			out.WriteRune(r)
			zeros = true
		case r >= '0' && r <= '9':
			if r == '0' && zeros {
				continue
			}
			zeros = false
			out.WriteRune(r)
		}
	}
	return out.String()
}

// setTypePromo is what the builder types a set that hands its cards out.
const setTypePromo = "promo"

// quotedRarities are the rarities this catalog writes in a product name as
// well as in the rarity field. Marvel is the one: "Enigma, New Moon
// (Marvel)" is filed at rarity Marvel, and every other rarity is only ever
// the field.
var quotedRarities = map[string]string{"Marvel": "Marvel"}

// promoTypeSlugs is a printing's promo types as the tokens a query can carry,
// which is what a card stores: a search splits its words apart before a
// filter sees them, so a tag only survives the trip as one. A printing that
// publishes none has none; its variant is the prose they were distilled out
// of, and reading it back would put on what the builder took off.
func promoTypeSlugs(card *DatastoreCard) []string {
	labels := card.PromoTypes
	if len(labels) == 0 {
		return nil
	}
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		out = append(out, matcher.PromoTypeSlug(label))
	}
	return out
}

// describingPromoTypes keeps the labels worth declaring as tags: the ones
// the printing's own finish or number does not already say. The card keeps
// the full list either way, which the matcher still reads to tell sibling
// printings apart; only the declaration is filtered, the same terms
// Riftbound carries its number-restating labels on.
func describingPromoTypes(card *DatastoreCard) []string {
	var out []string
	for _, promoType := range card.PromoTypes {
		if describingVariant(promoType, card.Finish, card.Number) == "" {
			continue
		}
		out = append(out, promoType)
	}
	return out
}

// describingVariant drops the labels a printing's finish already says - the
// catalog names a Cold Foil promo "... (Cold Foil)" and prices it as a Cold
// Foil, and a 1st Edition rainbow arrives labelled "Rainbow" - since as a
// tag they describe nothing the printing does not already spell. The label
// is weighed against the finish's run and treatment, as Finishes spells them,
// with the trailing "Foil" and "Edition" optional rather than by containment, so the one-letter labels an
// art variant carries ("C" beside a Cold Foil) are not read as an
// abbreviation of one. The card keeps the label either way: FilterCards
// tiers the candidates by it, and only the declaration is filtered.
func describingVariant(variant, finish, number string) string {
	if restatesNumber(variant, number) {
		return ""
	}
	label := matcher.PromoTypeSlug(variant)
	if label == "" {
		return ""
	}
	sold, _ := matcher.FinishOf(matcher.FinishSlug(finish))
	treatment, _ := matcher.FinishOf(sold.Treatment)
	for _, axis := range []string{sold.Run, treatment.TCGplayer} {
		if axis != "" && bareAxis(label) == bareAxis(matcher.PromoTypeSlug(axis)) {
			return ""
		}
	}
	return variant
}

// bareAxis spells a print run or a treatment without the word a label may
// leave off it: "1st" for 1st Edition, "cold" for Cold Foil.
func bareAxis(name string) string {
	return strings.TrimSuffix(strings.TrimSuffix(name, "foil"), "edition")
}

// qualifiedName spells a printing the way TCGplayer names the product, the
// card name followed by the label that tells it from the printings sharing
// its number ("Enigma, Ledger of Ancestry (Marvel)"). Empty for a printing
// the catalog qualifies with nothing, or with nothing its finish has not
// already said.
//
// Empty too for a spelling the datastore already carries as a name of its
// own: the pitch color belongs to a Flesh and Blood name ("Dig In (Red)")
// and a few products file it as a label beside the plain name instead, so
// indexing the label's spelling would pour one product's printings into the
// bucket the whole name answers with.
func qualifiedName(card *DatastoreCard, printingsByName map[string][]string) string {
	variant := describingVariant(card.Variant, card.Finish, card.Number)
	if variant == "" {
		return ""
	}
	qualified := card.Name + " (" + variant + ")"
	if printingsByName[matcher.Normalize(qualified)] != nil {
		return ""
	}
	return qualified
}

func (payload *Datastore) newBackend() *matcher.Backend {
	b := matcher.NewBackend(matcher.IDSpaceTCGplayer)

	for code, set := range payload.Sets {
		b.AllSets = append(b.AllSets, code)
		releaseDateTime, _ := time.Parse("2006-01-02", set.ReleaseDate)
		b.Sets[code] = &matcher.Set{
			Name:            set.Name,
			Code:            code,
			Type:            set.Type,
			ReleaseDate:     set.ReleaseDate,
			ReleaseDateTime: releaseDateTime,
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
		// Labelled by the slug, not by what the datastore spelled: the words
		// are on their way out of it, and a token reaching its label the same
		// way either side of that lets the two halves land in any order.
		for _, promoType := range describingPromoTypes(&card) {
			slug := matcher.PromoTypeSlug(promoType)
			b.AddPromoType(slug, promoTypeLabel(slug))
		}
		if qualified := qualifiedName(&card, printingsByName); qualified != "" {
			b.AddName(qualified)
		}
	}

	// Group sibling entries back into their product: a product priced in
	// several printings is the same card several times, and the matcher
	// wants it once, with FoilUUIDs naming the uuid each printing prices.
	// Which entries are one product is read off the identifiers they
	// publish, never off the shape of their ids.
	for _, group := range matcher.GroupProducts(payload.Cards, (*DatastoreCard).productKey) {
		printings := map[string]*DatastoreCard{}
		uuids := map[string]string{}
		for _, entry := range group {
			finish := matcher.FinishSlug(entry.Finish)
			printings[finish] = entry
			uuids[finish] = entry.ID
		}

		// The printings the bare nonfoil and foil flags answer with.
		nonfoil, _ := matcher.DefaultPrinting(printings, false)
		foil, _ := matcher.DefaultPrinting(printings, true)

		// The product id and the set-level card follow the nonfoil default
		// where one exists, exactly as riftbound points a product at its
		// plain printing; MatchID re-resolves the finish from the caller's
		// own flag either way.
		card := nonfoil
		if card == nil {
			card = foil
		}
		if card == nil {
			card = group[0]
		}
		if b.Sets[card.SetCode] == nil {
			continue
		}

		promoTypes := promoTypeSlugs(card)
		// The mark rides with them without being declared: it is a fact a
		// listing names, and for an artwork letter it is the only one.
		if card.Watermark != "" {
			promoTypes = append(promoTypes, matcher.PromoTypeSlug(card.Watermark))
		}
		// So does a rarity the catalog also writes in a product name. The
		// builder drops such a label because the rarity field says it, and
		// a listing saying "Enigma, New Moon (Marvel)" is naming that
		// printing and not the plain one beside it at the same number.
		if quoted := quotedRarities[card.Rarity]; quoted != "" {
			promoTypes = append(promoTypes, matcher.PromoTypeSlug(quoted))
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
			Language:   cmp.Or(card.Language, "English"),
			Rarity:     b.AddRarity(card.Rarity),
			Colors:     matcher.ColorNames(card.Pitches),
			PromoTypes: promoTypes,
			Watermark:  card.Watermark,
			IsPromo:    payload.Sets[card.SetCode].Type == setTypePromo,
			Printings:  printingsByName[matcher.Normalize(card.Name)],
			Artist:     card.Artist,

			IsOversized: slices.Contains(promoTypes, "oversized"),

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
			b.ExternalIdentifiers[matcher.IDSpaceTCGplayer][pid] = card.ID
		}
		if id := card.ExternalLinks.FabID; id != "" {
			identifiers["fabId"] = id
		}
		// A printing with neither keeps the nil map it had, so nothing is
		// stamped with an empty string for want of a value.
		if len(identifiers) > 0 {
			convertedCard.Identifiers = identifiers
		}

		b.Sets[card.SetCode].Cards = append(b.Sets[card.SetCode].Cards, convertedCard)

		qualified := qualifiedName(card, printingsByName)
		for _, entry := range group {
			b.AddPrinting(&convertedCard, entry.ID, matcher.FinishSlug(entry.Finish), card.Name, qualified)
		}
	}

	b.Rarities = matcher.RarityNames(payload.Properties["rarity"])
	b.Colors = matcher.ColorNames(payload.Properties["pitch"])
	for _, set := range b.Sets {
		set.Rarities = matcher.RaritiesOf(set.Cards, b.Rarities)
		set.Colors = matcher.ColorsOf(set.Cards, b.Colors)
	}

	// Sealed products live in the sealed namespace throughout; AddSealed
	// is what files them there.
	for _, product := range payload.Sealed {
		b.AddSealed(product.ID, product.Name, product.SetCode, product.Image, product.ExternalLinks.TcgPlayerID)
	}
	b.Complete(newRules(b))

	return b
}

// productKey names the product an entry is a printing of, read off what the
// entry publishes. A product the catalog sells stamps its product id on every
// printing of it; an entry minted for a card the catalog sells no product for
// names the Legend Story Studios card its printings were minted together
// from. An entry naming neither is a product of one printing, and stands for
// itself.
//
// Nothing here takes an id apart. The tail an id ends in is the builder's to
// spell, and a loader that reads one stops folding the day the spelling
// changes - which is exactly what "_holo" becoming "_holofoil" did here.
func (card *DatastoreCard) productKey() string {
	if card.ExternalLinks.TcgPlayerID != 0 {
		return fmt.Sprint(card.ExternalLinks.TcgPlayerID)
	}
	if id := card.ExternalLinks.FabID; id != "" {
		return id
	}
	return card.ID
}
