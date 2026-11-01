package magic

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mtgban/go-mtgban/matcher"
)

// BuyABoxInExpansionSetsDate is when buy-a-box promos began appearing in the
// expansion set rather than in a promos set of their own.
var BuyABoxInExpansionSetsDate = time.Date(2018, time.April, 1, 0, 0, 0, 0, time.UTC)

// PromosForEverybodyYay is when assorted promos began appearing in the
// expansion set itself.
var PromosForEverybodyYay = time.Date(2019, time.October, 1, 0, 0, 0, 0, time.UTC)

// Sheet is one pool a booster draws from, as mtgjson publishes it.
type Sheet struct {
	AllowDuplicates bool           `json:"allowDuplicates"`
	BalanceColors   bool           `json:"balanceColors"`
	Cards           map[string]int `json:"cards"`
	Fixed           bool           `json:"fixed"`
	Foil            bool           `json:"foil"`
	TotalWeight     int            `json:"totalWeight"`
}

// Booster describes how a product's boosters are built, as mtgjson publishes
// it.
type Booster struct {
	Boosters []struct {
		Contents map[string]int `json:"contents"`
		Weight   int            `json:"weight"`
	} `json:"boosters"`
	BoostersTotalWeight int              `json:"boostersTotalWeight"`
	Sheets              map[string]Sheet `json:"sheets"`
	Name                string           `json:"name"`
}

// SealedContent is one component of a sealed product, as mtgjson publishes it.
type SealedContent struct {
	Code   string `json:"code"`
	Count  int    `json:"count"`
	Etched bool   `json:"etched"`
	Foil   bool   `json:"foil"`
	Name   string `json:"name"`
	Set    string `json:"set"`
	UUID   string `json:"uuid"`

	// For variable_config
	Chance int `json:"chance"`
	Weight int `json:"weight"`

	// This recursive definition is used for "variable" mode in which one
	// possible configuration is chosen at random
	Configs []map[string][]SealedContent `json:"configs"`
}

// DeckCard is one card of a preconstructed deck, as mtgjson publishes it.
type DeckCard struct {
	Count    int    `json:"count"`
	IsEtched bool   `json:"isEtched"`
	IsFoil   bool   `json:"isFoil"`
	UUID     string `json:"uuid"`
}

// SealedProduct is a sealed item and what opening it can produce, as mtgjson
// publishes it.
type SealedProduct struct {
	Category    string                     `json:"category"`
	Contents    map[string][]SealedContent `json:"contents"`
	Identifiers map[string]string          `json:"identifiers"`
	Language    string                     `json:"language"`
	Name        string                     `json:"name"`
	SetCode     string                     `json:"setCode"`
	CardCount   int                        `json:"cardCount"`
	ReleaseDate string                     `json:"releaseDate"`
	Subtype     string                     `json:"subtype"`
	UUID        string                     `json:"uuid"`
}

// Set is an edition and everything printed in it, as mtgjson publishes it.
type Set struct {
	BaseSetSize   int    `json:"baseSetSize"`
	Code          string `json:"code"`
	Cards         []Card `json:"cards"`
	IsFoilOnly    bool   `json:"isFoilOnly"`
	IsNonFoilOnly bool   `json:"isNonFoilOnly"`
	IsOnlineOnly  bool   `json:"isOnlineOnly"`
	KeyruneCode   string `json:"keyruneCode"`
	Name          string `json:"name"`
	ParentCode    string `json:"parentCode"`
	ReleaseDate   string `json:"releaseDate"`
	TokenSetCode  string `json:"tokenSetCode"`
	Tokens        []Card `json:"tokens"`
	Type          string `json:"type"`

	// List of rarities present in the set
	Rarities []string
	// List of card colors present in the set
	Colors []string
	// Precomputed ReleaseDate value
	ReleaseDateTime time.Time

	Booster       map[string]Booster `json:"booster"`
	SealedProduct []SealedProduct    `json:"sealedProduct"`
	Decks         []struct {
		Code               string     `json:"code"`
		Commander          []DeckCard `json:"commander"`
		MainBoard          []DeckCard `json:"mainBoard"`
		DisplayCommander   []DeckCard `json:"displayCommander"`
		Planes             []DeckCard `json:"planes"`
		Schemes            []DeckCard `json:"schemes"`
		SideBoard          []DeckCard `json:"sideBoard"`
		Tokens             []DeckCard `json:"tokens"`
		Name               string     `json:"name"`
		SealedProductUUIDs []string   `json:"sealedProductUuids"`
	} `json:"decks"`
}

// Card is one printing as mtgjson publishes it, before the loader turns it
// into the matcher's own Card.
type Card struct {
	Artist              string              `json:"artist"`
	AttractionLights    []int               `json:"attractionLights"`
	BorderColor         string              `json:"borderColor"`
	Colors              []string            `json:"colors"`
	ColorIdentity       []string            `json:"colorIdentity"`
	FaceName            string              `json:"faceName"`
	FaceFlavorName      string              `json:"faceFlavorName"`
	FacePrintedName     string              `json:"facePrintedName"`
	Finishes            []string            `json:"finishes"`
	FlavorName          string              `json:"flavorName"`
	FlavorText          string              `json:"flavorText"`
	FrameEffects        []string            `json:"frameEffects"`
	FrameVersion        string              `json:"frameVersion"`
	HasContentWarning   bool                `json:"hasContentWarning"`
	Identifiers         map[string]string   `json:"identifiers"`
	IsAlternative       bool                `json:"isAlternative"`
	IsGameChanger       bool                `json:"isGameChanger"`
	IsFullArt           bool                `json:"isFullArt"`
	IsFunny             bool                `json:"isFunny"`
	IsOnlineOnly        bool                `json:"isOnlineOnly"`
	IsOversized         bool                `json:"isOversized"`
	IsPromo             bool                `json:"isPromo"`
	IsReserved          bool                `json:"isReserved"`
	Language            string              `json:"language"`
	Layout              string              `json:"layout"`
	Name                string              `json:"name"`
	Number              string              `json:"number"`
	OriginalReleaseDate string              `json:"originalReleaseDate"`
	PrintedName         string              `json:"printedName"`
	PrintedType         string              `json:"printedType"`
	Printings           []string            `json:"printings"`
	PromoTypes          []string            `json:"promoTypes"`
	Rarity              string              `json:"rarity"`
	SetCode             string              `json:"setCode"`
	SourceProducts      map[string][]string `json:"sourceProducts"`
	Side                string              `json:"side"`
	Subsets             []string            `json:"subsets"`
	Types               []string            `json:"types"`
	Subtypes            []string            `json:"subtypes"`
	Supertypes          []string            `json:"supertypes"`
	UUID                string              `json:"uuid"`
	Legalities          map[string]string   `json:"legalities"`
	Variations          []string            `json:"variations"`
	Watermark           string              `json:"watermark"`

	ForeignData []struct {
		Name           string              `json:"name"`
		Language       string              `json:"language"`
		Identifiers    map[string]string   `json:"identifiers"`
		Type           string              `json:"type"`
		UUID           string              `json:"uuid"`
		SourceProducts map[string][]string `json:"sourceProducts"`
	} `json:"foreignData"`

	// TokenProducts names, for a token this printing shares a physical card
	// with, every vendor product that sells the pair - a two-sided token
	// sheet prints two card faces on one object, and mtgjson does not mint
	// a combined printing of its own for most of them the way it does for
	// layout "double_faced_token". See tokenpairs.go.
	TokenProducts []TokenProduct `json:"tokenProducts"`

	PlainNumber string

	// A list of URLs containing the image of the card
	// At a minimum "full" and "thumbnail" versions should be provided
	Images map[string]string
}

// TokenProduct is one vendor product selling a physical card that prints two
// token faces. Its TokenParts names the two mtgjson printings it carries.
type TokenProduct struct {
	Identifiers map[string]string `json:"identifiers"`
	TokenParts  []TokenPart       `json:"tokenParts"`
}

// TokenPart is one face of a TokenProduct. A face mtgjson has not
// individually catalogued as its own card carries FaceID/FaceName instead
// of a UUID - there is no printing to point a derived entity at.
type TokenPart struct {
	UUID      string `json:"uuid"`
	FaceID    string `json:"faceId"`
	FaceName  string `json:"faceName"`
	TokenType string `json:"tokenType"`
}

// Card implements the Stringer interface
func (c Card) String() string {
	if c.Number == "" {
		return fmt.Sprintf("[%s] %s", c.SetCode, c.Name)
	}
	return fmt.Sprintf("%s|%s|%s", c.Name, c.SetCode, c.Number)
}

// HasFinish reports whether the printing was sold in this finish.
func (c *Card) HasFinish(fi string) bool {
	return slices.Contains(c.Finishes, fi)
}

// HasFrameEffect reports whether the printing carries this frame effect.
func (c *Card) HasFrameEffect(fe string) bool {
	return slices.Contains(c.FrameEffects, fe)
}

// HasPromoType reports whether the printing carries this promo type.
func (c *Card) HasPromoType(pt string) bool {
	return slices.Contains(c.PromoTypes, pt)
}

// The frame effects, promo types and border colors mtgjson spells out, which
// the rules match against.
const (
	FrameEffectExtendedArt = "extendedart"
	FrameEffectInverted    = "inverted"
	FrameEffectShowcase    = "showcase"
	FrameEffectShattered   = "shatteredglass"

	PromoTypeArenaLeague = "arenaleague"

	// The loader says these as promo types too, so one vocabulary answers.
	PromoTypeBorderless  = BorderColorBorderless
	PromoTypeExtendedArt = FrameEffectExtendedArt
	PromoTypeShowcase    = FrameEffectShowcase
	PromoTypeRetroFrame  = "retroframe"

	PromoTypeBeginnerBox       = "beginnerbox"
	PromoTypeBoosterfun        = "boosterfun"
	PromoTypeBundle            = "bundle"
	PromoTypeBuyABox           = "buyabox"
	PromoTypeChocoboTrackFoil  = "chocobotrackfoil"
	PromoTypeConcept           = "concept"
	PromoTypeConfettiFoil      = "confettifoil"
	PromoTypeCosmicFoil        = "cosmicfoil"
	PromoTypeDazzleFoil        = "dazzlefoil"
	PromoTypeDoubleExposure    = "doubleexposure"
	PromoTypeDoubleRainbow     = "doublerainbow"
	PromoTypeDracula           = "draculaseries"
	PromoTypeDraftWeekend      = "draftweekend"
	PromoTypeDragonScaleFoil   = "dragonscalefoil"
	PromoTypeEmbossed          = "embossed"
	PromoTypeFNM               = "fnm"
	PromoTypeFirstPlaceFoil    = "firstplacefoil"
	PromoTypeFacetFoil         = "facetfoil"
	PromoTypeFractureFoil      = "fracturefoil"
	PromoTypeGalaxyFoil        = "galaxyfoil"
	PromoTypeGameDay           = "gameday"
	PromoTypeGilded            = "gilded"
	PromoTypeGlossy            = "glossy"
	PromoTypeGodzilla          = "godzillaseries"
	PromoTypeHaloFoil          = "halofoil"
	PromoTypeHeadliner         = "headliner"
	PromoTypeIntroPack         = "intropack"
	PromoTypeInvisibleInk      = "invisibleink"
	PromoTypeJapanShowcase     = "japanshowcase"
	PromoTypeJudgeGift         = "judgegift"
	PromoTypeManaFoil          = "manafoil"
	PromoTypeNeonInk           = "neonink"
	PromoTypeNeonInkBlue       = "neoninkblue"
	PromoTypeNeonInkGreen      = "neoninkgreen"
	PromoTypeNeonInkPink       = "neoninkpink"
	PromoTypeNeonInkYellow     = "neoninkyellow"
	PromoTypeOilSlick          = "oilslick"
	PromoTypePlayPromo         = "playpromo"
	PromoTypePlayerRewards     = "playerrewards"
	PromoTypePoster            = "poster"
	PromoTypePrerelease        = "prerelease"
	PromoTypePromoPack         = "promopack"
	PromoTypeRainbowFoil       = "rainbowfoil"
	PromoTypeRaisedFoil        = "raisedfoil"
	PromoTypeRelease           = "release"
	PromoTypeResale            = "resale"
	PromoTypeRippleFoil        = "ripplefoil"
	PromoTypeSChineseAltArt    = "schinesealtart"
	PromoTypeScroll            = "scroll"
	PromoTypeSilverScroll      = "silverscroll"
	PromoTypeSLDBonus          = "sldbonus"
	PromoTypeSerialized        = "serialized"
	PromoTypeSetExtension      = "setextension"
	PromoTypeSilverFoil        = "silverfoil"
	PromoTypeSingularityFoil   = "singularityfoil"
	PromoTypeStandardShowdown  = "standardshowdown"
	PromoTypeStarterCollection = "startercollection"
	PromoTypeStarterDeck       = "starterdeck"
	PromoTypeStepAndCompleat   = "stepandcompleat"
	PromoTypeStoreChampionship = "storechampionship"
	PromoTypeSurgeFoil         = "surgefoil"
	PromoTypeTextured          = "textured"
	PromoTypeThickDisplay      = "thick"
	PromoTypeToken             = "token"
	PromoTypeTourney           = "tourney"
	PromoTypeWPN               = "wizardsplaynetwork"

	BorderColorBorderless = "borderless"

	LanguageJapanese  = "Japanese"
	LanguagePhyrexian = "Phyrexian"

	SuffixSpecial = "★"
	SuffixVariant = "†"
	SuffixPhi     = "Φ"
)

// NewPrereleaseDate is when any card in a set could be a prerelease promo,
// rather than only the chosen few.
var NewPrereleaseDate = time.Date(2014, time.September, 1, 0, 0, 0, 0, time.UTC)

// BuyABoxNotUniqueDate is when buy-a-box promos stopped being unique to that
// promotion, so the tag alone no longer picks one printing.
var BuyABoxNotUniqueDate = time.Date(2020, time.September, 1, 0, 0, 0, 0, time.UTC)

// SeparateFinishCollectorNumberDate is when a finish began carrying its own
// collector number, so etched, gilded and thick stopped sharing one.
var SeparateFinishCollectorNumberDate = time.Date(2022, time.February, 1, 0, 0, 0, 0, time.UTC)

// AllPrintings is the top-level structure of the MTGJSON AllPrintings file.
type AllPrintings struct {
	Data map[string]*Set `json:"data"`
	Meta struct {
		Date    string `json:"date"`
		Version string `json:"version"`
	} `json:"meta"`
}

// Load reads an AllPrintings JSON file from r and returns the
// parsed structure or an error.
func Load(r io.Reader) (*matcher.Backend, error) {
	var payload AllPrintings
	err := json.NewDecoder(r).Decode(&payload)
	if err != nil {
		return nil, err
	}
	if len(payload.Data) == 0 {
		return nil, errors.New("empty AllPrintings file")
	}
	return payload.newBackend(), nil
}

// fileTokensUnderTokenSet moves every set's tokens into the set their
// tokenSetCode names, creating it when missing. A set whose tokens already
// live under its own code has nowhere to move them to. Nothing is left
// behind: the edition a storefront writes for a token reaches the sheet
// either way, since FilterPrintings answers the parent set's name too.
func fileTokensUnderTokenSet(sets map[string]*Set) {
	codes := make([]string, 0, len(sets))
	for code := range sets {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	for _, code := range codes {
		set := sets[code]
		if len(set.Tokens) == 0 {
			continue
		}
		tokenCode := set.TokenSetCode
		if tokenCode == "" || tokenCode == set.Code {
			continue
		}

		for i := range set.Tokens {
			set.Tokens[i].SetCode = tokenCode
		}

		tokenSet, found := sets[tokenCode]
		if !found {
			tokenSet = &Set{
				Code:         tokenCode,
				Name:         set.Name + " Tokens",
				ParentCode:   set.Code,
				Type:         "token",
				TokenSetCode: tokenCode,
				KeyruneCode:  set.KeyruneCode,
				ReleaseDate:  set.ReleaseDate,
			}
			sets[tokenCode] = tokenSet
		}
		tokenSet.Tokens = append(tokenSet.Tokens, set.Tokens...)
		set.Tokens = nil
	}
}

func skipSet(set *Set) bool {
	// Skip unsupported sets
	switch set.Code {
	case "PRED", // a single foreign card
		"PSAL", "PS11", "PHUK", // salvat05, salvat11, hachette
		"OLGC", "OLEP", "OVNT", "O90P": // oversize
		return true
	}
	// Skip online sets
	if set.IsOnlineOnly ||
		strings.HasSuffix(set.Name, "Art Series") ||
		strings.HasSuffix(set.Name, "Minigames") ||
		strings.HasSuffix(set.Name, "Front Cards") ||
		strings.Contains(set.Name, "Heroes of the Realm") {
		return true
	}
	// In case there is nothing interesting in the set
	if len(set.Cards)+len(set.Tokens)+len(set.SealedProduct) == 0 {
		return true
	}
	return false
}

func generateUUIDsMap(sets map[string]*Set) (map[string]*matcher.CardObject, []string, []string, map[string][]string, map[string][]string) {
	uuids := map[string]*matcher.CardObject{}
	for _, set := range sets {
		for _, card := range set.Cards {
			generateCardUUIDs(card, uuids, set.Name)
		}
		for _, product := range set.SealedProduct {
			generateSealedUUIDs(product, uuids, set.Name)
		}
	}
	fillinSealedContents(sets, uuids)

	// Separate all the uuids generated
	var allUUIDs []string
	var allSealedUUIDs []string
	for uuid, co := range uuids {
		if co.Sealed {
			allSealedUUIDs = append(allSealedUUIDs, uuid)
			continue
		}
		allUUIDs = append(allUUIDs, uuid)
	}

	// Keep slices sorted for more reproducible results
	sort.Strings(allUUIDs)
	sort.Strings(allSealedUUIDs)

	// Bucket every uuid by its set, mirroring the singles/sealed split.
	// Built from the sorted slices so each bucket stays sorted too.
	setUUIDs := map[string][]string{}
	for _, uuid := range allUUIDs {
		code := uuids[uuid].SetCode
		setUUIDs[code] = append(setUUIDs[code], uuid)
	}
	setSealedUUIDs := map[string][]string{}
	for _, uuid := range allSealedUUIDs {
		code := uuids[uuid].SetCode
		setSealedUUIDs[code] = append(setSealedUUIDs[code], uuid)
	}

	return uuids, allUUIDs, allSealedUUIDs, setUUIDs, setSealedUUIDs
}

// Append "_f" and "_e" to uuids, unless etched is the only printing.
// If it's not etched, append "_f", unless foil is the only printing.
// Leave uuids unchanged, if there is a single printing of any kind.
func generateCardUUIDs(card Card, uuids map[string]*matcher.CardObject, edition string) {
	// The co value below is reused and tweaked between insertions, so each
	// map entry gets its own copy (save's parameter) to point at
	save := func(uuid string, co matcher.CardObject) {
		uuids[uuid] = &co
	}
	// Register the uuid each finish resolves to, mirroring the suffix rules
	// applied below, so output() can pull it instead of re-deriving the suffix.
	finishUUIDs := map[string]string{}
	switch {
	case card.HasFinish(matcher.FinishEtched):
		if card.HasFinish(matcher.FinishNonfoil) {
			finishUUIDs[matcher.FinishNonfoil] = card.UUID
		}
		if card.HasFinish(matcher.FinishFoil) {
			if card.HasFinish(matcher.FinishNonfoil) {
				finishUUIDs[matcher.FinishFoil] = card.UUID + suffixFoil
			} else {
				finishUUIDs[matcher.FinishFoil] = card.UUID
			}
		}
		if card.HasFinish(matcher.FinishNonfoil) || card.HasFinish(matcher.FinishFoil) {
			finishUUIDs[matcher.FinishEtched] = card.UUID + suffixEtched
		} else {
			finishUUIDs[matcher.FinishEtched] = card.UUID
		}
	case card.HasFinish(matcher.FinishFoil):
		if card.HasFinish(matcher.FinishNonfoil) {
			finishUUIDs[matcher.FinishNonfoil] = card.UUID
			finishUUIDs[matcher.FinishFoil] = card.UUID + suffixFoil
		} else {
			finishUUIDs[matcher.FinishFoil] = card.UUID
		}
	default:
		finishUUIDs[matcher.FinishNonfoil] = card.UUID
	}

	// Shared card object. Every uuid saved below names the finish it carries;
	// the plain one is the finish each branch starts from, and the branches
	// that move on to a foil or an etched uuid rename it as they go.
	base := toMtgCard(card)
	base.FoilUUIDs = finishUUIDs
	base.Finish = matcher.FinishNonfoil
	co := matcher.CardObject{
		Card:    base,
		Edition: edition,
	}

	if card.HasFinish(matcher.FinishEtched) {
		uuid := card.UUID

		// Etched + Nonfoil [+ Foil]
		if card.HasFinish(matcher.FinishNonfoil) {
			save(uuid, co)
		}

		// Etched + Foil
		if card.HasFinish(matcher.FinishFoil) {
			// Set the main property
			co.Foil = true
			co.Finish = matcher.FinishFoil
			// Make sure "_f" is appended if a different version exists
			if card.HasFinish(matcher.FinishNonfoil) {
				uuid = card.UUID + suffixFoil
				co.UUID = uuid
			}
			save(uuid, co)
		}

		// Etched
		// Set the main properties
		co.Foil = false
		co.Etched = true
		co.Finish = matcher.FinishEtched
		// If there are alternative finishes, always append the suffix
		if card.HasFinish(matcher.FinishNonfoil) || card.HasFinish(matcher.FinishFoil) {
			uuid = card.UUID + suffixEtched
			co.UUID = uuid
		}
		save(uuid, co)
	} else if card.HasFinish(matcher.FinishFoil) {
		uuid := card.UUID

		// Foil [+ Nonfoil]
		if card.HasFinish(matcher.FinishNonfoil) {
			save(uuid, co)

			// Update the uuid for the *next* finish type
			uuid = card.UUID + suffixFoil
			co.UUID = uuid
		}

		co.Foil = true
		co.Finish = matcher.FinishFoil
		save(uuid, co)
	} else {
		// Single printing, use as-is
		save(card.UUID, co)
	}
}

// Generate product URL using TCGplayer
func generateSealedImageURL(card Card, version string) string {
	tcgID, found := card.Identifiers["tcgplayerProductId"]
	if !found {
		return ""
	}
	if version == "small" {
		// This size is the default "small" format
		tcgID = "fit-in/146x204/" + tcgID
	}
	return "https://product-images.tcgplayer.com/" + tcgID + ".jpg"
}

func generateSealedUUIDs(product SealedProduct, uuids map[string]*matcher.CardObject, edition string) {
	card := Card{
		UUID:        product.UUID,
		Name:        product.Name,
		SetCode:     product.SetCode,
		Identifiers: product.Identifiers,
		Rarity:      "product",
		Layout:      product.Category,
		Side:        product.Subtype,
		// Will be filled later
		SourceProducts: map[string][]string{},
		Images:         map[string]string{},
	}

	// SLD, SLC and SLU aggregate products released over years under one set code,
	// so preserve their product date. Other sets derive it from their set date.
	if product.SetCode == "SLD" || product.SetCode == "SLC" || product.SetCode == "SLU" {
		card.OriginalReleaseDate = product.ReleaseDate
	}

	card.Images["full"] = generateSealedImageURL(card, "normal")
	card.Images["thumbnail"] = generateSealedImageURL(card, "small")
	card.Images["crop"] = generateSealedImageURL(card, "normal")

	isEtched := strings.Contains(product.Name, "Etched")
	isFoil := isEtched || sealedHoldsOnlyFoils(product.Name)

	uuids[product.UUID] = &matcher.CardObject{
		Card:    toMtgCard(card),
		Sealed:  true,
		Edition: edition,
		Foil:    isFoil,
		Etched:  isEtched,
	}
}

// sealedHoldsOnlyFoils reports whether every card a sealed product holds is
// foil, which the product's own name usually says and sometimes does not.
//
// Where a whole product line is foil the line is named here rather than each
// of its products: every From the Vault, every Scene Box, every SDCC
// planeswalker set. The rest are named
// one by one in productsWithOnlyFoils, because nothing about the name says it.
func sealedHoldsOnlyFoils(name string) bool {
	switch {
	case strings.Contains(name, "Foil") && !strings.Contains(name, "Non"):
		return true
	case strings.Contains(name, "Premium"):
		return true
	case strings.Contains(name, "VIP Edition"):
		return true
	case strings.Contains(name, "Commander Deck") && strings.Contains(name, "Collector"):
		return true
	case strings.Contains(name, "From the Vault"):
		return true
	case strings.Contains(name, "Scene Box"):
		return true
	case strings.Contains(name, "SDCC") && strings.Contains(name, "Planeswalker"):
		return true
	case slices.Contains(productsWithOnlyFoils, name):
		return true
	}
	return false
}

func sortPrintings(sets map[string]*Set, printings []string) {
	sort.Slice(printings, func(i, j int) bool {
		setI := sets[printings[i]]
		setJ := sets[printings[j]]

		if setI.ReleaseDateTime.Equal(setJ.ReleaseDateTime) {
			return setI.Name < setJ.Name
		}

		return setI.ReleaseDateTime.After(setJ.ReleaseDateTime)
	})
}

// Generate image URL using Scryfall - we assume that every card has such id
func generateImageURL(card Card, version string) string {
	id, found := card.Identifiers["scryfallId"]
	if !found {
		return ""
	}

	altID, found := card.Identifiers["originalScryfallId"]
	if found {
		id = altID
	}

	return fmt.Sprintf("https://cards.scryfall.io/%s/front/%c/%c/%s.jpg", version, id[0], id[1], id)
}

// Make sure Printings array is filled, and make token properties uniform
func adjustTokens(sets map[string]*Set) {
	printings := make(map[string][]string)

	// Adjust input data, making sure layout is set
	for _, set := range sets {
		for i := range set.Tokens {
			// Art series are not carried, so keep their layout naming them
			if set.Tokens[i].Layout == "art_series" {
				continue
			}
			// Reset various token types to correct properties
			if slices.Contains(set.Tokens[i].Types, "Card") ||
				slices.Contains(set.Tokens[i].Types, "Dungeon") ||
				slices.Contains(set.Tokens[i].Types, "Emblem") ||
				slices.Contains(set.Tokens[i].Types, "Token") {
				set.Tokens[i].Layout = "token"
				set.Tokens[i].Rarity = "token"

				if set.TokenSetCode != "" {
					set.Tokens[i].Identifiers["tokenSetCode"] = set.TokenSetCode
				}
			}
		}
	}

	// Load up all the printings found among tokens
	for _, set := range sets {
		for i := range set.Tokens {
			if set.Tokens[i].Layout != "token" {
				continue
			}
			if slices.Contains(printings[set.Tokens[i].Name], set.Code) {
				continue
			}
			printings[set.Tokens[i].Name] = append(printings[set.Tokens[i].Name], set.Code)
		}
	}

	// Assign printings to tokens
	// Sorting will happen later
	for _, set := range sets {
		for i := range set.Tokens {
			if set.Tokens[i].Layout != "token" {
				continue
			}
			set.Tokens[i].Printings = printings[set.Tokens[i].Name]
		}
	}
}

// numberDecorations are the marks a collector number is decorated with, and
// plainNumberTail the letters that stand behind one to name its printing.
const plainNumberTail = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

var numberDecorations = SuffixSpecial + SuffixVariant + SuffixPhi + strings.ToLower(SuffixPhi) + "*"

// plainNumber is the collector number without the decoration that tells one
// printing of it from another, and without whatever the decoration carries
// behind it: "265†a" is the a of five Tamiyo's Journals and the plain number
// of every one of them is 265. Trimming from the right would reach only the
// marks standing last, so a dozen numbers spelled "139★s" or "265†a" would keep
// theirs and answer to no search for the number they print. Numeric numbers
// also lose catalog padding, so "071" and "71" share the same plain number.
func plainNumber(number string) string {
	if i := strings.IndexAny(number, numberDecorations); i >= 0 {
		number = number[:i]
	}
	// The letters a number ends in name the printing rather than number it:
	// the s of a prerelease, the p of a promo pack, the jpn of a Japanese
	// copy. They go the way the marks do, so the two printings of one card
	// agree on the number they print - 139s and 139★s are both the 139 of
	// Tenth Edition. A number that is letters alone is a number.
	plain := strings.TrimRight(number, plainNumberTail)
	if plain == "" {
		return number
	}
	plain = strings.TrimLeft(plain, "0")
	if plain == "" {
		return "0"
	}
	return plain
}

func (ap *AllPrintings) newBackend() *matcher.Backend {
	canonicalNames := map[string]string{}
	sealedNames := map[string]string{}
	sealedProducts := map[string]*SealedProduct{}
	alternates := map[string]matcher.AlternateProps{}
	commanderKeywordMap := map[string]string{}
	// Card and token names are collected as sets: the only question ever
	// asked of them is membership, and scanning a slice of them would cost
	// more than every other step of the index build put together, with the
	// catalogue past thirty thousand names.
	cardNames := map[string]bool{}
	tokenNames := map[string]bool{}
	// Every name the real cards answer to, so a token face repeating one
	// never steals its lookup. Filled by the same pass as cardNames, which
	// runs before any token is read.
	cardOwnedNames := map[string]bool{}
	// The oracle ids each name answers for among the cards that are not
	// playtest cards, so a playtest card sharing a name can be told apart.
	cardOracles := map[string][]string{}
	var tokens []string
	var allSets []string

	for code, set := range ap.Data {
		// Filter out unneeded data
		if skipSet(set) {
			delete(ap.Data, code)
			continue
		}

		// Load all possible card names
		for _, card := range set.Cards {
			cardNames[card.Name] = true
			if !slices.Contains(card.PromoTypes, "playtest") {
				key := matcher.Normalize(card.Name)
				oracle := card.Identifiers["scryfallOracleId"]
				if !slices.Contains(cardOracles[key], oracle) {
					cardOracles[key] = append(cardOracles[key], oracle)
				}
			}
			for _, name := range []string{
				card.Name, card.FaceName, card.FlavorName,
				card.FaceFlavorName, card.PrintedName, card.FacePrintedName,
			} {
				if name != "" {
					cardOwnedNames[name] = true
				}
			}
		}

		// Save the names of sealed products for later sorting, and index them
		// by uuid so that the source check below is a lookup rather than a
		// scan of every product in every set
		for i, product := range set.SealedProduct {
			sealedNames[product.UUID] = product.Name
			sealedProducts[product.UUID] = &set.SealedProduct[i]
		}
	}

	fileTokensUnderTokenSet(ap.Data)

	// Load token names (that don't have the same name of a real card):
	// this needs every card name loaded first, or the clash check would
	// depend on the iteration order of the sets
	for _, set := range ap.Data {
		for _, token := range set.Tokens {
			if !tokenNames[token.Name] && !cardNames[token.Name] {
				tokenNames[token.Name] = true
				tokens = append(tokens, token.Name)
			}
		}
	}

	adjustTokens(ap.Data)

	// Precompute ReleaseDateTime for all sets to avoid repeated time.Parse calls
	for _, set := range ap.Data {
		set.ReleaseDateTime, _ = time.Parse("2006-01-02", set.ReleaseDate)
	}

	for code, set := range ap.Data {
		var filteredCards []Card
		var rarities, colors []string

		allSets = append(allSets, code)

		allCards := set.Cards
		tokensStart := len(allCards)

		// A multi-face card's non-'a' face carries its own multiverseId,
		// dropped along with the rest of that face below; index it here
		// so it can be filed under the 'a' face that survives.
		sideBMultiverseIDs := map[string]string{}
		for _, c := range set.Cards {
			if c.Side != "" && c.Side != "a" {
				if id := c.Identifiers["multiverseId"]; id != "" {
					sideBMultiverseIDs[c.Number] = id
				}
			}
		}

		// Append tokens to the list of considered cards. A token named the
		// same way as a real card is carried as "<name> Token", the shape
		// the hand-renamed clashes use, so the plain name keeps
		// answering with the card. Art series stay out, as their sets do.
		for _, token := range set.Tokens {
			if token.Layout == "art_series" {
				continue
			}
			if cardNames[token.Name] {
				token.Name += " Token"
			}
			allCards = append(allCards, token)
		}

		switch set.Code {
		// Remove reference to an online-only set
		case "PMIC":
			set.ParentCode = ""
		}

		for cardIndex, card := range allCards {
			fromTokens := cardIndex >= tokensStart

			// Skip anything non-paper
			if card.IsOnlineOnly {
				continue
			}

			// Only a card printed in the set's language takes it, and its image
			language, found := forcedLanguages[set.Code]
			if found {
				for _, foreignData := range card.ForeignData {
					if foreignData.Language == language {
						card.Language = language
						// mtgjsonId stays mtgjson's card, which TCGplayer skus and the
						// id index key on: see todo/foreign-black-border-languages.md
						card.Identifiers["originalScryfallId"] = foreignData.Identifiers["scryfallId"]
					}
				}
			}

			// A playtest card named like a different card is carried as
			// "<name> Playtest", so the plain name keeps answering for it
			if !fromTokens && slices.Contains(card.PromoTypes, "playtest") &&
				slices.ContainsFunc(cardOracles[matcher.Normalize(card.Name)], func(oracle string) bool {
					return oracle != card.Identifiers["scryfallOracleId"]
				}) {
				card.Name += " Playtest"
				if card.FaceName != "" {
					card.FaceName += " Playtest"
				}
			}

			card.Images = map[string]string{}
			card.Images["full"] = generateImageURL(card, "normal")
			card.Images["thumbnail"] = generateImageURL(card, "small")
			card.Images["crop"] = generateImageURL(card, "art_crop")

			// Custom modifications or skips
			switch set.Code {
			// Missing variant tags
			case "PALP":
				card.FlavorText = missingPALPtags[card.Number]
			case "PELP":
				card.FlavorText = missingPELPtags[card.Number]
			// Remove frame effects and borders where they don't belong
			case "STA", "PLST":
				card.PromoTypes = nil
				card.FrameEffects = nil
				card.BorderColor = "black"

			// Promo-only sets
			case "PPC1", "PMIC":
				card.IsPromo = true

			// Missing promo type for this series
			case "DFT":
				num, _ := strconv.Atoi(card.Number)
				if num >= 333 && num <= 346 || num >= 532 && num <= 545 {
					card.PromoTypes = append(card.PromoTypes, "ruderiders")
				}

			case "SLD":
				switch card.Number {
				// One of the tokens is a DFC but burns a card number, skip it
				case "28":
					continue
				// Source is "technically correct" but it gets too messy to track
				case "589":
					card.Finishes = []string{"nonfoil", "etched"}

				// A series of bonus cards that are not tagged as such
				case "59":
					card.IsPromo = true
				case "721":
					card.PromoTypes = append(card.PromoTypes, "convention")
				case "797":
					card.PromoTypes = append(card.PromoTypes, "convention")
				case "8001":
					card.PromoTypes = append(card.PromoTypes, PromoTypeTourney)
					card.IsPromo = true

				default:
					num, _ := strconv.Atoi(card.Number)
					// Override the frame type for the Braindead drops
					if (num >= 821 && num <= 824) ||
						(num >= 1652 && num <= 1666) ||
						(num >= 2514 && num <= 2523) ||
						(num >= 7105 && num <= 7108) {
						card.FrameVersion = "2015"
					}
				}

			// Clashing printing
			case "TBTH":
				if card.Name == "Unquenchable Fury" {
					card.Name += " Token"
				}

			case "SLX":
				num, _ := strconv.Atoi(card.Number)
				// These cards have been distributed by stores and not found in products
				if num >= 24 && num <= 30 {
					card.PromoTypes = append(card.PromoTypes, PromoTypeWPN)
				}

			case "CMB1", "CMB2", "MB2":
				// Only a face of this one is named like a card, Bind (INV)
				if card.Name == "Bind // Liberate" {
					card.Name = "Bind // Liberate Playtest"
					card.FaceName = "Bind Playtest"
				}

			case "TMC":
				set.Name = "Teenage Mutant Ninja Turtles Commander"

			case "UNK":
				// PSSC's "Math is for Blockers (Plane)" once the prefilter
				// drops its suffix
				if card.Name == "Math is for Blockers" {
					card.Name += " Playtest"
				}
			}

			// A shop names the border and these frames the way it names any
			// promo type, so file them among them
			if card.BorderColor == BorderColorBorderless {
				card.PromoTypes = append(card.PromoTypes, PromoTypeBorderless)
			}
			if card.HasFrameEffect(FrameEffectExtendedArt) {
				card.PromoTypes = append(card.PromoTypes, PromoTypeExtendedArt)
			}
			if card.HasFrameEffect(FrameEffectShowcase) {
				card.PromoTypes = append(card.PromoTypes, PromoTypeShowcase)
			}
			// Every token-like card (emblem, dungeon, helper card) is a token by now
			if card.Layout == "token" {
				card.PromoTypes = append(card.PromoTypes, PromoTypeToken)
			}
			// The 1997 frame is also just what every card printed before
			// 8th Edition already wears, so only file it as a promo type
			// where it is a deliberate modern treatment, not the frame
			// its era printed by default. A long-running promo set dates
			// each card on its own (a 2023 Media Insert in a 1995 set).
			// The List, Unfinity's included, reprints a card in the frame
			// it first wore, so there it is never the treatment.
			releaseDate := set.ReleaseDateTime
			if card.OriginalReleaseDate != "" {
				releaseDate, _ = time.Parse("2006-01-02", card.OriginalReleaseDate)
			}
			theList := set.Code == "PLST" || set.Code == "ULST"
			if card.FrameVersion == "1997" && releaseDate.After(PromosForEverybodyYay) && !theList {
				card.PromoTypes = append(card.PromoTypes, PromoTypeRetroFrame)
			}

			// Rename DFCs into a single name
			// All names need to be redacted
			dfcSameName := IsDFCSameName(card.Name)
			if dfcSameName {
				card.Name = strings.Split(card.Name, " // ")[0]
				card.FlavorName = strings.Split(card.FlavorName, " // ")[0]
				card.FaceName = strings.Split(card.FaceName, " // ")[0]
				card.FaceFlavorName = strings.Split(card.FaceFlavorName, " // ")[0]
				card.PrintedName = strings.Split(card.PrintedName, " // ")[0]
				card.FacePrintedName = strings.Split(card.FacePrintedName, " // ")[0]
				card.Identifiers["isDFCSameName"] = "true"
			}

			for i, name := range []string{
				card.FaceName, card.FlavorName, card.FaceFlavorName, card.PrintedName, card.FacePrintedName,
			} {
				// Skip empty entries
				if name == "" {
					continue
				}
				// A token's face may repeat the name a real card or its
				// reskin goes by (the Day // Night helper repeats the
				// Apocalypse card's face), and registering it would steal
				// the alternate lookup from the card. Ask the cards
				// themselves rather than the set the token came from: the
				// clash is between the two names, not between two sets.
				if fromTokens && cardOwnedNames[name] {
					continue
				}
				// Skip FaceName entries that could be aliased
				// ie 'Start' could be Start//Finish and Start//Fire
				switch name {
				case "Bind",
					"Fire",
					"Smelt",
					"Start":
					continue
				}
				// Skip faces of DFCs with same names that aren't reskin version of other cars
				if dfcSameName && card.FlavorName == "" {
					continue
				}

				// If the name is unique, keep track of the numbers so that they
				// can be decoupled later for reprints of the main card.
				// If the name is not unique, we might overwrite data and lose
				// track of the main version
				props := matcher.AlternateProps{
					OriginalName:   card.Name,
					OriginalNumber: card.Number,
					IsFlavor:       i > 0,
				}
				existing, found := alternates[name]
				if found {
					if existing.OriginalName <= props.OriginalName {
						existing.OriginalNumber = ""
						alternates[name] = existing
						continue
					}
					props.OriginalNumber = ""
				}
				alternates[name] = props
			}

			// MTGJSON v5 contains duplicated card info for each face, and we do
			// not need that level of detail, so just skip any extra side.
			if card.Side != "" && card.Side != "a" {
				continue
			}
			if id, found := sideBMultiverseIDs[card.Number]; found && card.Identifiers["multiverseId"] != id {
				card.Identifiers["multiverseIdBSide"] = id
			}

			// Filter out unneeded printings
			var printings []string
			for i := range card.Printings {
				subset, found := ap.Data[card.Printings[i]]
				// If not found it means the set was already deleted above
				if !found || skipSet(subset) {
					continue
				}
				printings = append(printings, card.Printings[i])
			}
			// Sort printings by most recent sets first; duplicate() keeps
			// this order when it adds its sets later
			sortPrintings(ap.Data, printings)

			card.Printings = printings

			// Filter out unneeded sources and sort them alphabetically
			for finish, sources := range card.SourceProducts {
				var filtered []string
				for _, source := range sources {
					if isBaseSealed(ap.Data, sealedProducts, source, card.UUID, finish) {
						filtered = append(filtered, source)
					}
				}
				sort.Slice(filtered, func(i, j int) bool {
					return sealedNames[filtered[i]] < sealedNames[filtered[j]]
				})
				card.SourceProducts[finish] = filtered
			}

			// Upstream cannot represent the foils of the Secret Lair Countdown
			// drops: every card in them has only a chance to come in traditional
			// foil, and a fractional chance to come in halo foil (which is a
			// separate number entirely), so the products can only ever list the
			// non-foil printings without distorting pull probabilities.
			//
			// Synthesize the missing foil sources from the non-foil ones. This has
			// to happen *after* the filter above, which is finish-strict and would
			// otherwise discard everything set here.
			if set.Code == "SLC" {
				if card.SourceProducts == nil {
					card.SourceProducts = map[string][]string{}
				}

				// Never overwrite a source upstream already got right: the
				// foil-only bonus cards (#27 and #2023) are declared as foil in
				// their product and would be orphaned by a blind assignment.
				if len(card.SourceProducts["foil"]) == 0 {
					num, _ := strconv.Atoi(card.Number)
					switch {
					// Traditional foils share the product of their non-foil printing
					case (num >= 1 && num <= 26) || (num >= 1993 && num <= 2022):
						card.SourceProducts["foil"] = card.SourceProducts["nonfoil"]
					// Halo foils have no non-foil printing of their own; they come
					// from the same drop as the base cards (allCards[0] is #1, and
					// its sources are already filtered by the time we get here).
					case num >= 28 && num <= 53:
						card.SourceProducts["foil"] = allCards[0].SourceProducts["nonfoil"]
					}
				}
			}

			// Custom properties for tokens
			if card.IsOversized {
				card.Rarity = "oversize"
			}

			// Save the original uuid
			card.Identifiers["mtgjsonId"] = card.UUID

			// The lit bulbs are what tells one attraction printing from
			// its siblings, so they are filed with the identifiers
			if len(card.AttractionLights) > 0 {
				card.Identifiers[attractionLightsID] = attractionTag(card.AttractionLights)
			}

			// Save the collector number stripped of its ★/†/Φ marks and of
			// the letters that name a printing rather than number it
			card.PlainNumber = Rules{}.PlainNumber(card.Number)

			// Now assign the card to the list of cards to be saved
			filteredCards = append(filteredCards, card)

			alternativeID, found := card.Identifiers["tcgplayerAlternativeFoilProductId"]
			if found {
				// Change properties of the current card
				filteredCards[len(filteredCards)-1].Finishes = []string{"nonfoil"}
				filteredCards[len(filteredCards)-1].Variations = []string{card.UUID + suffixFoil}

				// Create new card
				card.Variations = []string{card.UUID}
				card.UUID += suffixFoil
				card.Number += SuffixSpecial
				card.Finishes = []string{"foil"}

				// Clone the map and replace it, overriding the id
				newIdentifiers := map[string]string{}
				maps.Copy(newIdentifiers, card.Identifiers)

				card.Identifiers = newIdentifiers
				card.Identifiers["tcgplayerProductId"] = alternativeID
				// Signal that the TCG SKUs from MTGJSON need to be refreshed
				card.Identifiers["needsNewTCGSKUs"] = "true"

				// Append the new card
				filteredCards = append(filteredCards, card)
			}

			// Add possible rarities and colors
			if !slices.Contains(rarities, card.Rarity) {
				rarities = append(rarities, card.Rarity)
			}
			for _, color := range card.Colors {
				if !slices.Contains(colors, mtgColorNameMap[color]) {
					colors = append(colors, mtgColorNameMap[color])
				}
			}
			if len(card.Colors) == 0 && !slices.Contains(colors, "colorless") {
				colors = append(colors, "colorless")
			}
			if len(card.Colors) > 1 && !slices.Contains(colors, "multicolor") {
				colors = append(colors, "multicolor")
			}

		}

		// Replace the original array with the filtered one
		set.Cards = filteredCards

		// Assign the rarities and colors present in the set
		matcher.SortByOrder(rarities, mtgRarities)
		set.Rarities = rarities
		matcher.SortByOrder(colors, mtgColors)
		set.Colors = colors

		// Adjust the setBaseSize to take into account the cards with
		// the same name in the same set (also make sure that it is
		// correctly initialized)
		if set.ReleaseDateTime.After(PromosForEverybodyYay) {
			for _, card := range set.Cards {
				if card.HasPromoType(PromoTypeBoosterfun) {
					// Usually boosterfun cards have real numbers
					cn, err := strconv.Atoi(card.Number)
					if err == nil {
						set.BaseSetSize = cn - 1
					}
					break
				}
			}
		}

		// Retrieve the best describing word for a commander set and save it for later reuse
		if strings.HasSuffix(set.Name, "Commander") && !strings.Contains(set.Name, "Display") {
			keyword := longestWordInEditionName(strings.TrimSuffix(set.Name, "Commander"))
			commanderKeywordMap[keyword] = set.Name
		}

		for _, product := range set.SealedProduct {
			if product.Identifiers == nil {
				product.Identifiers = map[string]string{}
			}
			product.Identifiers["mtgjsonId"] = product.UUID
		}
	}

	// A token mtgjson sells under its own product only through
	// tokenProducts takes that product as its id, before any is filed
	adoptOwnTokenProducts(ap.Data)

	duplicate(ap.Data, "Legends Italian", "LEG", "ITA", "1995-09-01")
	duplicate(ap.Data, "The Dark Italian", "DRK", "ITA", "1995-08-01")
	duplicate(ap.Data, "Alternate Fourth Edition", "4ED", "ALT", "1995-04-01")
	allSets = append(allSets, "LEGITA", "DRKITA", "4EDALT")

	if ap.Data["SLD"] != nil {
		sldDupes := duplicateCards(ap.Data, "SLD", "JPN", sldJPNLangDupes)
		ap.Data["SLD"].Cards = append(ap.Data["SLD"].Cards, sldDupes...)
	}

	if ap.Data["PURL"] != nil {
		purlDupes := duplicateCards(ap.Data, "PURL", "JPN", []string{"1"})
		ap.Data["PURL"].Cards = append(ap.Data["PURL"].Cards, purlDupes...)
	}

	// Generate the unique identifiers for singles and products
	uuids, allUUIDs, allSealedUUIDs, setUUIDs, setSealedUUIDs := generateUUIDsMap(ap.Data)

	// Remove promo tags that apply to a single finish only
	filterInvalidPromoTypes(ap.Data, uuids)

	// Add all names and associated uuids to the global names and hashes arrays
	hashes := map[string][]string{}
	var names, fullNames, lowerNames []string
	var sealed, fullSealed, lowerSealed []string
	var promoTypes []string
	externalIDs := map[matcher.IDSpace]map[string]string{
		matcher.IDSpaceMTGJSON:    {},
		matcher.IDSpaceScryfall:   {},
		matcher.IDSpaceTCGplayer:  {},
		matcher.IDSpaceMultiverse: {},
	}
	for _, uuid := range append(allUUIDs, allSealedUUIDs...) {
		card := uuids[uuid]

		// The identifiers are shared by every finish sibling, so each id
		// files at the base sibling - the unsuffixed uuid every branch of
		// generateCardUUIDs starts from - except the etched product id,
		// which names the etched printing specifically.
		baseUUID := card.UUID
		for _, finish := range []string{matcher.FinishNonfoil, matcher.FinishFoil, matcher.FinishEtched} {
			id, found := card.FoilUUIDs[finish]
			if found {
				baseUUID = id
				break
			}
		}
		etchedUUID, found := card.FoilUUIDs[matcher.FinishEtched]
		if !found {
			etchedUUID = baseUUID
		}

		for _, filing := range []struct {
			tag    string
			space  matcher.IDSpace
			target string
		}{
			{"mtgjsonId", matcher.IDSpaceMTGJSON, baseUUID},
			{"scryfallId", matcher.IDSpaceScryfall, baseUUID},
			{"tcgplayerProductId", matcher.IDSpaceTCGplayer, baseUUID},
			{"tcgplayerEtchedProductId", matcher.IDSpaceTCGplayer, etchedUUID},
			{"multiverseId", matcher.IDSpaceMultiverse, baseUUID},
			{"multiverseIdBSide", matcher.IDSpaceMultiverse, baseUUID},
		} {
			id, found := card.Identifiers[filing.tag]
			if !found {
				continue
			}
			// A language copy files only the ids mtgjson gave it: TCGplayer
			// sells it under the English products, and the other ids it
			// cloned from the original its mtgjsonId names are the original's
			original, found := uuids[card.Identifiers["mtgjsonId"]]
			if found && card.Language != original.Language &&
				(filing.space == matcher.IDSpaceTCGplayer || original.Identifiers[filing.tag] == id) {
				continue
			}
			// Skip if already loaded
			_, found = externalIDs[filing.space][id]
			if found {
				continue
			}
			externalIDs[filing.space][id] = filing.target
		}

		// Add to the ever growing list of promo types
		for _, promoType := range card.PromoTypes {
			if !slices.Contains(promoTypes, promoType) {
				promoTypes = append(promoTypes, promoType)
			}
		}

		namesToAdd := []string{card.Name}
		if card.Identifiers["isDFCSameName"] == "true" {
			namesToAdd = append(namesToAdd, card.Name+" // "+card.Name)
			if card.FlavorName != "" && !slices.Contains(namesToAdd, card.FlavorName+" // "+card.FlavorName) {
				namesToAdd = append(namesToAdd, card.FlavorName+" // "+card.FlavorName)
			}
			if card.PrintedName != "" && !slices.Contains(namesToAdd, card.PrintedName+" // "+card.PrintedName) {
				namesToAdd = append(namesToAdd, card.PrintedName+" // "+card.PrintedName)
			}
		} else {
			for _, name := range []string{
				card.FaceName, card.FlavorName, card.FaceFlavorName, card.PrintedName, card.FacePrintedName,
			} {
				if name == "" {
					continue
				}
				namesToAdd = append(namesToAdd, name)
			}
		}

		for _, nameToAdd := range namesToAdd {
			norm := matcher.Normalize(nameToAdd)
			_, found := hashes[norm]
			if !found {
				if card.Sealed {
					sealed = append(sealed, norm)
					fullSealed = append(fullSealed, card.Name)
					lowerSealed = append(lowerSealed, strings.ToLower(card.Name))
				} else {
					names = append(names, norm)
					fullNames = append(fullNames, nameToAdd)
					lowerNames = append(lowerNames, strings.ToLower(nameToAdd))
				}
			}
			if slices.Contains(hashes[norm], uuid) {
				continue
			}
			hashes[norm] = append(hashes[norm], uuid)
		}

		// Due to several cards having the same name of a token we hardcode
		// this value to tell them apart in the future -- checks and names
		// are still using the official Scryfall name (without the extra Token)
		norm := matcher.Normalize(card.Name)
		if card.Layout == "token" && !strings.Contains(card.Name, "Token") {
			norm += "token"
		}

		canonicalNames[norm] = card.Name
	}

	sort.Strings(promoTypes)
	sort.Strings(allSets)

	sort.Strings(names)
	sort.Strings(fullNames)
	sort.Strings(lowerNames)
	sort.Strings(sealed)
	sort.Strings(fullSealed)
	sort.Strings(lowerSealed)

	// Convert local sets to matcher sets
	mSets := make(map[string]*matcher.Set, len(ap.Data))
	for k, v := range ap.Data {
		mSets[k] = toMtgSet(v)
	}

	// Mint the combined entity for every two-sided token sheet whose
	// pairing data resolves safely, now that every id and uuid above is
	// filed - an id a real printing already claims is refused rather than
	// shadowed. Nothing here touches hashes or canonicalNames, and no
	// derived uuid is added to allUUIDs: a derived entity reaches Match
	// only by its own TCGplayer product id, never by name, and a consumer
	// that wants every one of them (a future pricing pass, say) filters
	// uuids for Identifiers["derivedTokenPair"] == "true" rather than
	// reading a second index kept only for that - one fact, one place.
	// See tokenpairs.go.
	derivedCards := deriveTokenPairs(ap.Data, uuids, externalIDs[matcher.IDSpaceTCGplayer])

	for _, card := range derivedCards {
		edition := card.SetCode
		if set, found := mSets[card.SetCode]; found {
			edition = set.Name
		}
		generateCardUUIDs(card, uuids, edition)
	}

	var b matcher.Backend

	b.Hashes = hashes
	b.AllSets = allSets
	b.AllUUIDs = allUUIDs
	b.AllSealedUUIDs = allSealedUUIDs
	b.SetUUIDs = setUUIDs
	b.SetSealedUUIDs = setSealedUUIDs

	b.AllNames = names
	b.AllCanonicalNames = fullNames
	b.AllLowerNames = lowerNames

	b.AllSealed = sealed
	b.AllCanonicalSealed = fullSealed
	b.AllLowerSealed = lowerSealed

	b.Sets = mSets
	b.IndexSets()
	b.Rarities = mtgRarities
	b.Colors = mtgColors
	b.CanonicalNames = canonicalNames
	b.Tokens = tokens
	b.UUIDs = uuids
	b.ExternalIdentifiers = externalIDs
	b.AlternateProps = alternates
	b.AllPromoTypes = promoTypes
	// Declare only the types this datastore actually carries, so the list
	// describes the data rather than everything Magic has ever printed.
	b.PromoTypeLabels = map[string]string{}
	for _, promoType := range promoTypes {
		if label := promoTypeLabels[promoType]; label != "" {
			b.PromoTypeLabels[promoType] = label
		}
	}

	b.CommanderKeywordMap = commanderKeywordMap
	b.SLDDeckNames = fillinSLDdecks(ap.Data["SLD"])

	tokenPairs := buildTokenPairIndices(&b)
	b.TokenPairIndex = tokenPairs.byFace
	b.TokenPairIDByUUIDs = tokenPairs.byUUIDPair
	b.TokenPairIDByBothNames = tokenPairs.byBothNames

	b.IndexRarities()
	b.SetRules(Rules{})
	b.SortSealed()

	return &b
}

var mtgRarities = []string{
	"oversize", "special", "mythic", "rare", "uncommon", "common", "token",
}

var mtgColors = []string{
	"white", "blue", "black", "red", "green", "colorless", "multicolor",
}

func fillinSLDdecks(set *Set) []string {
	// A datastore cut down for a test may not carry the set at all
	if set == nil {
		return nil
	}

	var output []string
	for _, product := range set.SealedProduct {
		if strings.HasPrefix(product.Name, "Secret Lair Commander") {
			name := strings.TrimPrefix(product.Name, "Secret Lair Commander Deck ")
			if !slices.Contains(output, name) {
				output = append(output, name)
			}
		}
	}
	return output
}

// Add a map of which kind of products sealed contains
func fillinSealedContents(sets map[string]*Set, uuids map[string]*matcher.CardObject) {
	result := map[string][]string{}
	tmp := map[string][]string{}

	// Figure out which sealed products contain a given sealed item
	for _, set := range sets {
		for _, product := range set.SealedProduct {
			dedup := map[string]int{}
			list := sealedWithinSealed(product)
			for _, item := range list {
				dedup[item]++
			}
			for uuid := range dedup {
				tmp[product.UUID] = append(tmp[product.UUID], uuid)
			}
		}
	}

	// Reverse to be compatible with SourceProducts model (child->parent map)
	for _, list := range tmp {
		for _, item := range list {
			for key, sublist := range tmp {
				// Add if item is in the sublist, and the key was not already added
				if slices.Contains(sublist, item) && !slices.Contains(result[item], key) {
					result[item] = append(result[item], key)
				}
			}
		}
	}

	// Write back the result
	for uuid, co := range uuids {
		if !co.Sealed {
			continue
		}

		res, found := result[uuid]
		if !found {
			continue
		}

		sort.Slice(res, func(i, j int) bool {
			return uuids[res[i]].Name < uuids[res[j]].Name
		})

		uuids[uuid].SourceProducts["sealed"] = res
	}
}

// Remove promo tags that apply to a single finish only
func filterInvalidPromoTypes(sets map[string]*Set, uuids map[string]*matcher.CardObject) {
	for uuid, card := range uuids {
		if !card.Foil && !card.Etched && !card.Sealed {
			for _, promoType := range []string{
				PromoTypeDoubleExposure,
				PromoTypeGalaxyFoil,
				PromoTypeSilverFoil,
				PromoTypeRainbowFoil,
				PromoTypeRippleFoil,
				PromoTypeSurgeFoil,
			} {
				if card.HasPromoType(promoType) {
					// Filter
					var filtered []string
					for _, pt := range card.PromoTypes {
						if pt != promoType {
							filtered = append(filtered, pt)
						}
					}

					// Update UUID map (through the shared pointer)
					card.PromoTypes = filtered

					// Also update data in the original slice
					for i, c := range sets[card.SetCode].Cards {
						if c.UUID != uuid {
							continue
						}
						sets[card.SetCode].Cards[i].PromoTypes = filtered
					}
				}
			}
		}
	}
}

// Return a list of sealed products contained by the input product
// Decks and Packs and Card cannot contain other sealed product, so they are ignored here
func sealedWithinSealed(product SealedProduct) []string {
	var list []string

	for key, contents := range product.Contents {
		for _, content := range contents {
			switch key {
			case "sealed":
				list = append(list, content.UUID)

			case "variable":
				for _, config := range content.Configs {
					for _, sealed := range config["sealed"] {
						list = append(list, sealed.UUID)
					}
				}
			}
		}
	}

	return list
}

// Check whether the sealed product directly contains the given card with
// the requested finish. "Directly" means via a card/deck/pack entry at the
// top level (or inside a variable config) - not reachable only through a
// nested sealed sub-product.
func isBaseSealed(sets map[string]*Set, products map[string]*SealedProduct, productUUID, cardUUID, finish string) bool {
	product, found := products[productUUID]
	if !found {
		return false
	}
	return contentsContainCard(sets, product.Contents, cardUUID, finish)
}

func contentsContainCard(sets map[string]*Set, contents map[string][]SealedContent, cardUUID, finish string) bool {
	wantFoil := finish == "foil"
	wantEtched := finish == "etched"

	for key, items := range contents {
		for _, item := range items {
			switch key {
			case "card":
				if item.UUID == cardUUID {
					if finish == "" {
						return true
					}
					if wantEtched && item.Etched {
						return true
					}
					if wantFoil && item.Foil {
						return true
					}
					if !wantFoil && !wantEtched && !item.Foil && !item.Etched {
						return true
					}
				}
			case "deck":
				if set, ok := sets[strings.ToUpper(item.Set)]; ok {
					for _, d := range set.Decks {
						if d.Name != item.Name {
							continue
						}
						for _, list := range [][]DeckCard{
							d.MainBoard, d.SideBoard,
							d.Commander, d.DisplayCommander,
							d.Planes, d.Schemes, d.Tokens,
						} {
							for _, dc := range list {
								if dc.UUID != cardUUID {
									continue
								}
								if finish == "" {
									return true
								}
								if wantEtched && dc.IsEtched {
									return true
								}
								if wantFoil && dc.IsFoil {
									return true
								}
								if !wantFoil && !wantEtched && !dc.IsFoil && !dc.IsEtched {
									return true
								}
							}
						}
					}
				}
			case "pack":
				if set, ok := sets[strings.ToUpper(item.Set)]; ok {
					if booster, ok := set.Booster[item.Code]; ok {
						for _, sheet := range booster.Sheets {
							if _, ok := sheet.Cards[cardUUID]; !ok {
								continue
							}
							if finish == "" {
								return true
							}
							if (wantFoil || wantEtched) && sheet.Foil {
								return true
							}
							if !wantFoil && !wantEtched && !sheet.Foil {
								return true
							}
						}
					}
				}
			case "variable":
				for _, config := range item.Configs {
					if contentsContainCard(sets, config, cardUUID, finish) {
						return true
					}
				}
				// "sealed": intentionally not handled - nested products don't count as direct.
			}
		}
	}
	return false
}

var langs = map[string]string{
	"JPN": "Japanese",
	"ITA": "Italian",
	"ALT": "English",
}

// Duplicate an entire set of cards, using a custom code and a different language
func duplicate(sets map[string]*Set, name, code, tag, date string) {
	if sets[code] == nil {
		return
	}

	// Copy base set information
	dup := *sets[code]

	// Update with new info
	dup.Name = name
	dup.Code = code + tag
	dup.ParentCode = code
	dup.ReleaseDate = date
	dup.ReleaseDateTime, _ = time.Parse("2006-01-02", date)

	// Target slice for later use
	var numbers []string

	// Add duplicated set (with no cards) to the root, for sortPrintings
	sets[dup.Code] = &dup

	// Rework printings information
	for i := range sets[code].Cards {
		// Skip misprints from main sets
		if strings.HasSuffix(sets[code].Cards[i].Number, SuffixVariant) {
			continue
		}

		// A same-name card met earlier already gave every card of this name
		// the printings below
		if slices.Contains(sets[code].Cards[i].Printings, dup.Code) {
			numbers = append(numbers, sets[code].Cards[i].Number)
			continue
		}

		// Update printings for the original set, keeping them sorted by
		// date like every other card's: the site lists them in this order
		printings := append(sets[code].Cards[i].Printings, dup.Code)
		sortPrintings(sets, printings)
		sets[code].Cards[i].Printings = printings

		// Loop through all other sets mentioned
		for _, setCode := range printings {
			// Skip the set being added, there might be cards containing
			// the set code being processed due to variants
			if setCode == dup.Code {
				continue
			}
			_, found := sets[setCode]
			if !found {
				continue
			}
			if skipSet(sets[setCode]) {
				continue
			}

			for j := range sets[setCode].Cards {
				// Name match, can't break after the first because there could
				// be other variants
				if sets[setCode].Cards[j].Name == sets[code].Cards[i].Name {
					sets[setCode].Cards[j].Printings = printings
				}
			}
		}

		numbers = append(numbers, sets[code].Cards[i].Number)
	}

	// Duplicate cards
	dup.Cards = duplicateCards(sets, code, tag, numbers)

	// dup is a shallow copy, so everything not replaced above still points at
	// the source set's slices. Sealed products, tokens and decks belong to the
	// printing that actually shipped them; shared, their uuids are counted
	// under both sets and the edition recorded for one flips between runs.
	dup.SealedProduct = nil
	dup.Tokens = nil
	dup.Decks = nil
	dup.Booster = nil

	// Cardmarket sells a whole-set copy as an expansion of its own, with its
	// own products (Legends Italian), so it keeps none of the original's
	for i := range dup.Cards {
		altIdentifiers := map[string]string{}
		for k, v := range dup.Cards[i].Identifiers {
			switch k {
			case "mcmId", "mcmEtchedId":
				continue
			}
			altIdentifiers[k] = v
		}
		dup.Cards[i].Identifiers = altIdentifiers
	}
}

// mintedSuffix is what a printing minted here from an mtgjson one, such as an
// alternative foil, adds to that printing's uuid; an mtgjson printing adds none.
func mintedSuffix(card Card) string {
	id := card.Identifiers["mtgjsonId"]
	if id == "" || !strings.HasPrefix(card.UUID, id) {
		return ""
	}
	return card.UUID[len(id):]
}

// Duplicate certain cards within the same set according to the language tag
func duplicateCards(sets map[string]*Set, code, tag string, numbers []string) []Card {
	var duplicates []Card
	copyOf := map[string]string{}

	if sets[code] == nil {
		return nil
	}

	for i := range sets[code].Cards {
		// Skip unneeded; a listed number copies its starred sibling too
		if !slices.Contains(numbers, strings.TrimSuffix(sets[code].Cards[i].Number, SuffixSpecial)) {
			continue
		}

		mainUUID := sets[code].Cards[i].UUID

		// Update with new info
		dupeCard := sets[code].Cards[i]
		dupeCard.UUID = mainUUID + "_" + strings.ToLower(tag)
		dupeCard.Language = langs[tag]
		dupeCard.Identifiers = maps.Clone(dupeCard.Identifiers)
		// TCGplayer sells a copy in another language under its original's
		// products, and one in the original's language under its own
		if dupeCard.Language == sets[code].Cards[i].Language {
			delete(dupeCard.Identifiers, "tcgplayerProductId")
			delete(dupeCard.Identifiers, "tcgplayerEtchedProductId")
		}

		// A whole-set copy is told apart by its own code, one filed next to
		// its original only by the tag behind its number
		_, found := sets[code+tag]
		if found {
			dupeCard.SetCode = code + tag
		} else {
			dupeCard.Number += strings.ToLower(tag)
		}

		// Retrieve Printed data if available
		for j, foreignData := range sets[code].Cards[i].ForeignData {
			if foreignData.Language != dupeCard.Language {
				continue
			}
			dupeCard.PrintedName = foreignData.Name
			dupeCard.PrintedType = foreignData.Type
			// mtgjson's ids for this printing replace the original's
			if id, found := foreignData.Identifiers["scryfallId"]; found {
				dupeCard.Identifiers["scryfallId"] = id
				dupeCard.Identifiers["originalScryfallId"] = id
			}
			if id, found := foreignData.Identifiers["multiverseId"]; found {
				dupeCard.Identifiers["multiverseId"] = id
			}
			// mtgjson names this printing itself, and knows which products hold it
			dupeCard.UUID = foreignData.UUID + mintedSuffix(sets[code].Cards[i])
			if foreignData.SourceProducts != nil {
				dupeCard.SourceProducts = foreignData.SourceProducts
			}
			// The original's entry names its copy. A printing minted here shares
			// its foreignData with the one it came from, so it gets its own first.
			if foreignData.UUID != dupeCard.UUID {
				sets[code].Cards[i].ForeignData = slices.Clone(sets[code].Cards[i].ForeignData)
				sets[code].Cards[i].ForeignData[j].UUID = dupeCard.UUID
			}
		}

		// Update images
		dupeCard.Images = map[string]string{}
		dupeCard.Images["full"] = generateImageURL(dupeCard, "normal")
		dupeCard.Images["thumbnail"] = generateImageURL(dupeCard, "small")
		dupeCard.Images["crop"] = generateImageURL(dupeCard, "art_crop")

		copyOf[mainUUID] = dupeCard.UUID
		duplicates = append(duplicates, dupeCard)
	}

	// A copy's variations are its original's in the copy's language, which
	// is how a finish twin split off the original is found from the copy
	for i := range duplicates {
		duplicates[i].Variations = slices.Clone(duplicates[i].Variations)
		for j, variation := range duplicates[i].Variations {
			if id, found := copyOf[variation]; found {
				duplicates[i].Variations[j] = id
			}
		}
	}

	return duplicates
}

const (
	suffixFoil   = "_f"
	suffixEtched = "_e"
)

// colorNames spells MTGJSON's color letters by name ("W" is "white"), the
// way every game's cards carry their colors.
func colorNames(letters []string) []string {
	if len(letters) == 0 {
		return letters
	}
	names := make([]string, 0, len(letters))
	for _, letter := range letters {
		names = append(names, mtgColorNameMap[letter])
	}
	return names
}

// toMtgCard converts a local Card to matcher.Card via direct field copy.
func toMtgCard(c Card) matcher.Card {
	mc := matcher.Card{
		Artist:              c.Artist,
		BorderColor:         c.BorderColor,
		Colors:              colorNames(c.Colors),
		ColorIdentity:       colorNames(c.ColorIdentity),
		FaceName:            c.FaceName,
		FaceFlavorName:      c.FaceFlavorName,
		FacePrintedName:     c.FacePrintedName,
		Finishes:            c.Finishes,
		FlavorName:          c.FlavorName,
		FlavorText:          c.FlavorText,
		FrameEffects:        c.FrameEffects,
		FrameVersion:        c.FrameVersion,
		HasContentWarning:   c.HasContentWarning,
		Identifiers:         c.Identifiers,
		IsAlternative:       c.IsAlternative,
		IsGameChanger:       c.IsGameChanger,
		IsFullArt:           c.IsFullArt,
		IsFunny:             c.IsFunny,
		IsOnlineOnly:        c.IsOnlineOnly,
		IsOversized:         c.IsOversized,
		IsPromo:             c.IsPromo,
		IsReserved:          c.IsReserved,
		Language:            c.Language,
		Layout:              c.Layout,
		Name:                c.Name,
		Number:              c.Number,
		PlainNumber:         c.PlainNumber,
		OriginalReleaseDate: c.OriginalReleaseDate,
		PrintedName:         c.PrintedName,
		PrintedType:         c.PrintedType,
		Printings:           c.Printings,
		PromoTypes:          c.PromoTypes,
		Rarity:              c.Rarity,
		SetCode:             c.SetCode,
		SourceProducts:      c.SourceProducts,
		Side:                c.Side,
		Subsets:             c.Subsets,
		Types:               c.Types,
		Subtypes:            c.Subtypes,
		Supertypes:          c.Supertypes,
		UUID:                c.UUID,
		Legalities:          c.Legalities,
		Variations:          c.Variations,
		Watermark:           c.Watermark,
		Images:              c.Images,
	}
	// A card with no foreign printings keeps the nil rather than an empty
	// slice standing in for one, so the set-level cards and the uuid index,
	// both converted here, describe the same card the same way.
	if c.ForeignData != nil {
		mc.ForeignData = make([]struct {
			Name        string            `json:"name"`
			Language    string            `json:"language"`
			Identifiers map[string]string `json:"identifiers"`
			Type        string            `json:"type"`
			UUID        string            `json:"uuid"`
		}, len(c.ForeignData))
		for i, fd := range c.ForeignData {
			mc.ForeignData[i].Name = fd.Name
			mc.ForeignData[i].Language = fd.Language
			mc.ForeignData[i].Identifiers = fd.Identifiers
			mc.ForeignData[i].Type = fd.Type
			mc.ForeignData[i].UUID = fd.UUID
		}
	}
	return mc
}

// toMtgBooster converts a local Booster. Its sheets are named by this
// package, so the map is rebuilt rather than converted whole; the weighted
// booster list is an anonymous struct both sides spell identically, and
// carries over as it is.
func toMtgBooster(b Booster) matcher.Booster {
	mb := matcher.Booster{
		Boosters:            b.Boosters,
		BoostersTotalWeight: b.BoostersTotalWeight,
		Name:                b.Name,
	}
	if b.Sheets != nil {
		mb.Sheets = make(map[string]matcher.Sheet, len(b.Sheets))
		for name, sheet := range b.Sheets {
			mb.Sheets[name] = matcher.Sheet(sheet)
		}
	}
	return mb
}

// toMtgSealedContents converts what opening a product can produce. A
// component describes further components under "variable" mode, so this
// recurses rather than running as a loop at its one call site.
func toMtgSealedContents(contents map[string][]SealedContent) map[string][]matcher.SealedContent {
	if contents == nil {
		return nil
	}
	out := make(map[string][]matcher.SealedContent, len(contents))
	for key, list := range contents {
		converted := make([]matcher.SealedContent, len(list))
		for i, content := range list {
			converted[i] = matcher.SealedContent{
				Code:   content.Code,
				Count:  content.Count,
				Foil:   content.Foil,
				Name:   content.Name,
				Set:    content.Set,
				UUID:   content.UUID,
				Chance: content.Chance,
				Weight: content.Weight,
			}
			for _, config := range content.Configs {
				converted[i].Configs = append(converted[i].Configs, toMtgSealedContents(config))
			}
		}
		out[key] = converted
	}
	return out
}

// toMtgSealedProduct converts a local SealedProduct.
func toMtgSealedProduct(p SealedProduct) matcher.SealedProduct {
	return matcher.SealedProduct{
		Category:    p.Category,
		Contents:    toMtgSealedContents(p.Contents),
		Identifiers: p.Identifiers,
		Language:    p.Language,
		Name:        p.Name,
		SetCode:     p.SetCode,
		CardCount:   p.CardCount,
		ReleaseDate: p.ReleaseDate,
		Subtype:     p.Subtype,
		UUID:        p.UUID,
	}
}

// toMtgDeckCards converts one of a preconstructed deck's card lists.
func toMtgDeckCards(cards []DeckCard) []matcher.DeckCard {
	if cards == nil {
		return nil
	}
	out := make([]matcher.DeckCard, len(cards))
	for i, card := range cards {
		out[i] = matcher.DeckCard(card)
	}
	return out
}

// toMtgSet converts a local Set to *matcher.Set field by field.
//
// The types the two packages share are identical but named apart. Bridging
// them with json.Marshal and json.Unmarshal would read the whole catalogue a
// second time, since a set carries its cards, tokens, sealed products and
// decks: 7.04s of an 11.80s index build, more than reading the 659MB file
// costs in the first place.
func toMtgSet(s *Set) *matcher.Set {
	if s == nil {
		return nil
	}
	ms := &matcher.Set{
		BaseSetSize:     s.BaseSetSize,
		Code:            s.Code,
		IsFoilOnly:      s.IsFoilOnly,
		IsNonFoilOnly:   s.IsNonFoilOnly,
		IsOnlineOnly:    s.IsOnlineOnly,
		KeyruneCode:     s.KeyruneCode,
		Name:            s.Name,
		ParentCode:      s.ParentCode,
		ReleaseDate:     s.ReleaseDate,
		TokenSetCode:    s.TokenSetCode,
		Type:            s.Type,
		Rarities:        s.Rarities,
		Colors:          s.Colors,
		ReleaseDateTime: s.ReleaseDateTime,
	}
	if s.Cards != nil {
		ms.Cards = make([]matcher.Card, len(s.Cards))
		for i, card := range s.Cards {
			ms.Cards[i] = toMtgCard(card)
		}
	}
	if s.Tokens != nil {
		ms.Tokens = make([]matcher.Card, len(s.Tokens))
		for i, token := range s.Tokens {
			ms.Tokens[i] = toMtgCard(token)
		}
	}
	if s.Booster != nil {
		ms.Booster = make(map[string]matcher.Booster, len(s.Booster))
		for name, booster := range s.Booster {
			ms.Booster[name] = toMtgBooster(booster)
		}
	}
	if s.SealedProduct != nil {
		ms.SealedProduct = make([]matcher.SealedProduct, len(s.SealedProduct))
		for i, product := range s.SealedProduct {
			ms.SealedProduct[i] = toMtgSealedProduct(product)
		}
	}
	// The deck list's element is an anonymous struct, so the destination is
	// grown into rather than respelled here: naming that type would copy a
	// definition that has to stay identical to the matcher's own.
	ms.Decks = slices.Grow(ms.Decks, len(s.Decks))[:len(s.Decks)]
	for i, deck := range s.Decks {
		ms.Decks[i].Code = deck.Code
		ms.Decks[i].Name = deck.Name
		ms.Decks[i].SealedProductUUIDs = deck.SealedProductUUIDs
		ms.Decks[i].Commander = toMtgDeckCards(deck.Commander)
		ms.Decks[i].DisplayCommander = toMtgDeckCards(deck.DisplayCommander)
		ms.Decks[i].MainBoard = toMtgDeckCards(deck.MainBoard)
		ms.Decks[i].Planes = toMtgDeckCards(deck.Planes)
		ms.Decks[i].Schemes = toMtgDeckCards(deck.Schemes)
		ms.Decks[i].SideBoard = toMtgDeckCards(deck.SideBoard)
		ms.Decks[i].Tokens = toMtgDeckCards(deck.Tokens)
	}
	return ms
}

// longestWordInEditionName finds the longest keyword in an edition name, ignoring punctuation.
func longestWordInEditionName(str string) string {
	fields := strings.Fields(str)
	longest := ""
	for _, field := range fields {
		field = strings.TrimRight(field, ":'")
		if len(field) > len(longest) {
			longest = field
		}
	}
	return longest
}
