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

	"github.com/mtgban/go-mtgban/mtgmatcher"
)

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
	Code  string `json:"code"`
	Count int    `json:"count"`
	Foil  bool   `json:"foil"`
	Name  string `json:"name"`
	Set   string `json:"set"`
	UUID  string `json:"uuid"`

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
		Name        string            `json:"name"`
		Language    string            `json:"language"`
		Identifiers map[string]string `json:"identifiers"`
		Type        string            `json:"type"`
	} `json:"foreignData"`

	OriginalNumber string

	// A list of URLs containing the image of the card
	// At a minimum "full" and "thumbnail" versions should be provided
	Images map[string]string
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

	PromoTypeArenaLeague       = "arenaleague"
	PromoTypeBoosterfun        = "boosterfun"
	PromoTypeBundle            = "bundle"
	PromoTypeBuyABox           = "buyabox"
	PromoTypeChocoboTrackFoil  = "chocobotrackfoil"
	PromoTypeConcept           = "concept"
	PromoTypeConfettiFoil      = "confettifoil"
	PromoTypeCosmicFoil        = "cosmicfoil"
	PromoTypeDoubleExposure    = "doubleexposure"
	PromoTypeDoubleRainbow     = "doublerainbow"
	PromoTypeDracula           = "draculaseries"
	PromoTypeDraftWeekend      = "draftweekend"
	PromoTypeDragonScaleFoil   = "dragonscalefoil"
	PromoTypeEmbossed          = "embossed"
	PromoTypeFNM               = "fnm"
	PromoTypeFirstPlaceFoil    = "firstplacefoil"
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
	PromoTypeRippleFoil        = "ripplefoil"
	PromoTypeSChineseAltArt    = "schinesealtart"
	PromoTypeScroll            = "scroll"
	PromoTypeSerialized        = "serialized"
	PromoTypeSilverFoil        = "silverfoil"
	PromoTypeStarterDeck       = "starterdeck"
	PromoTypeStepAndCompleat   = "stepandcompleat"
	PromoTypeStoreChampionship = "storechampionship"
	PromoTypeSurgeFoil         = "surgefoil"
	PromoTypeTextured          = "textured"
	PromoTypeThickDisplay      = "thick"
	PromoTypeWPN               = "wizardsplaynetwork"

	BorderColorBorderless = "borderless"

	LanguageJapanese  = "Japanese"
	LanguagePhyrexian = "Phyrexian"

	SuffixSpecial = "★"
	SuffixVariant = "†"
	SuffixPhiLow  = "φ"
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
func Load(r io.Reader) (*mtgmatcher.Backend, error) {
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

func okForTokens(set *Set) bool {
	return slices.Contains(setAllowedForTokens, set.Code) ||
		strings.Contains(set.Name, "Duel Deck")
}

func skipSet(set *Set) bool {
	// Skip unsupported sets
	switch set.Code {
	case "PRED", // a single foreign card
		"PSAL", "PS11", "PHUK", // salvat05, salvat11, hachette
		"OAFR", "OCLB", // oversized dungeons
		"UNK", "PUNK", // not on sale anywhere
		"OLGC", "OLEP", "OVNT", "O90P": // oversize
		return true
	}
	// Skip online sets, and any token-based sets
	if set.IsOnlineOnly ||
		(set.Type == "token" && !okForTokens(set)) ||
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

func generateUUIDsMap(sets map[string]*Set) (map[string]*mtgmatcher.CardObject, []string, []string, map[string][]string, map[string][]string) {
	uuids := map[string]*mtgmatcher.CardObject{}
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
func generateCardUUIDs(card Card, uuids map[string]*mtgmatcher.CardObject, edition string) {
	// The co value below is reused and tweaked between insertions, so each
	// map entry gets its own copy (save's parameter) to point at
	save := func(uuid string, co mtgmatcher.CardObject) {
		uuids[uuid] = &co
	}
	// Register the uuid each finish resolves to, mirroring the suffix rules
	// applied below, so output() can pull it instead of re-deriving the suffix.
	finishUUIDs := map[string]string{}
	switch {
	case card.HasFinish(mtgmatcher.FinishEtched):
		if card.HasFinish(mtgmatcher.FinishNonfoil) {
			finishUUIDs[mtgmatcher.FinishNonfoil] = card.UUID
		}
		if card.HasFinish(mtgmatcher.FinishFoil) {
			if card.HasFinish(mtgmatcher.FinishNonfoil) {
				finishUUIDs[mtgmatcher.FinishFoil] = card.UUID + suffixFoil
			} else {
				finishUUIDs[mtgmatcher.FinishFoil] = card.UUID
			}
		}
		if card.HasFinish(mtgmatcher.FinishNonfoil) || card.HasFinish(mtgmatcher.FinishFoil) {
			finishUUIDs[mtgmatcher.FinishEtched] = card.UUID + suffixEtched
		} else {
			finishUUIDs[mtgmatcher.FinishEtched] = card.UUID
		}
	case card.HasFinish(mtgmatcher.FinishFoil):
		if card.HasFinish(mtgmatcher.FinishNonfoil) {
			finishUUIDs[mtgmatcher.FinishNonfoil] = card.UUID
			finishUUIDs[mtgmatcher.FinishFoil] = card.UUID + suffixFoil
		} else {
			finishUUIDs[mtgmatcher.FinishFoil] = card.UUID
		}
	default:
		finishUUIDs[mtgmatcher.FinishNonfoil] = card.UUID
	}

	// Shared card object. Every uuid saved below names the finish it carries;
	// the plain one is the finish each branch starts from, and the branches
	// that move on to a foil or an etched uuid rename it as they go.
	base := toMtgCard(card)
	base.FoilUUIDs = finishUUIDs
	base.Finish = mtgmatcher.FinishNonfoil
	co := mtgmatcher.CardObject{
		Card:    base,
		Edition: edition,
	}

	if card.HasFinish(mtgmatcher.FinishEtched) {
		uuid := card.UUID

		// Etched + Nonfoil [+ Foil]
		if card.HasFinish(mtgmatcher.FinishNonfoil) {
			// Save the card object
			save(uuid, co)
		}

		// Etched + Foil
		if card.HasFinish(mtgmatcher.FinishFoil) {
			// Set the main property
			co.Foil = true
			co.Finish = mtgmatcher.FinishFoil
			// Make sure "_f" is appended if a different version exists
			if card.HasFinish(mtgmatcher.FinishNonfoil) {
				uuid = card.UUID + suffixFoil
				co.UUID = uuid
			}
			// Save the card object
			save(uuid, co)
		}

		// Etched
		// Set the main properties
		co.Foil = false
		co.Etched = true
		co.Finish = mtgmatcher.FinishEtched
		// If there are alternative finishes, always append the suffix
		if card.HasFinish(mtgmatcher.FinishNonfoil) || card.HasFinish(mtgmatcher.FinishFoil) {
			uuid = card.UUID + suffixEtched
			co.UUID = uuid
		}
		// Save the card object
		save(uuid, co)
	} else if card.HasFinish(mtgmatcher.FinishFoil) {
		uuid := card.UUID

		// Foil [+ Nonfoil]
		if card.HasFinish(mtgmatcher.FinishNonfoil) {
			// Save the card object
			save(uuid, co)

			// Update the uuid for the *next* finish type
			uuid = card.UUID + suffixFoil
			co.UUID = uuid
		}

		// Foil
		co.Foil = true
		co.Finish = mtgmatcher.FinishFoil
		// Save the card object
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

func generateSealedUUIDs(product SealedProduct, uuids map[string]*mtgmatcher.CardObject, edition string) {
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

	// Preserve ReleaseDate information only for SLD, the other sets
	// will derive it from the set date itself
	if product.SetCode == "SLD" {
		card.OriginalReleaseDate = product.ReleaseDate
	}

	card.Images["full"] = generateSealedImageURL(card, "normal")
	card.Images["thumbnail"] = generateSealedImageURL(card, "small")
	card.Images["crop"] = generateSealedImageURL(card, "normal")

	isEtched := strings.Contains(product.Name, "Etched")
	isFoil := isEtched || sealedHoldsOnlyFoils(product.Name)

	uuids[product.UUID] = &mtgmatcher.CardObject{
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

	// Adjust input data, filtering out unneeded sets, and making sure layout is set
	for code, set := range sets {
		// Remove undesired tokens
		if !okForTokens(set) {
			sets[code].Tokens = nil
			continue
		}

		for i := range set.Tokens {
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

func (ap *AllPrintings) newBackend() *mtgmatcher.Backend {
	canonicalNames := map[string]string{}
	sealedNames := map[string]string{}
	sealedProducts := map[string]*SealedProduct{}
	alternates := map[string]mtgmatcher.AlternateProps{}
	commanderKeywordMap := map[string]string{}
	var allCardNames []string
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
			if !slices.Contains(allCardNames, card.Name) {
				allCardNames = append(allCardNames, card.Name)
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

	// Load token names (that don't have the same name of a real card):
	// this needs every card name loaded first, or the clash check would
	// depend on the iteration order of the sets
	for _, set := range ap.Data {
		for _, token := range set.Tokens {
			if !slices.Contains(tokens, token.Name) && !slices.Contains(allCardNames, token.Name) {
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

		// Append tokens to the list of considered cards
		// if they are not named in the same way of a real card
		for _, token := range set.Tokens {
			if !slices.Contains(allCardNames, token.Name) {
				allCards = append(allCards, token)
			}
		}

		switch set.Code {
		// Remove reference to an online-only set
		case "PMIC":
			set.ParentCode = ""
		}

		for _, card := range allCards {
			// Skip anything non-paper
			if card.IsOnlineOnly {
				continue
			}

			card.Images = map[string]string{}
			card.Images["full"] = generateImageURL(card, "normal")
			card.Images["thumbnail"] = generateImageURL(card, "small")
			card.Images["crop"] = generateImageURL(card, "art_crop")

			// Custom modifications or skips
			switch set.Code {
			// Override non-English Language
			case "FBB":
				card.Language = "Italian"
			case "4BB":
				card.Language = "Japanese"
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
					card.PromoTypes = append(card.PromoTypes, "tourney")
					card.IsPromo = true

				// The Shapeshift token with clashing name
				// SDL is the only set enabled for this case
				case "1906", "1907", "1908", "1909":
					card.Name += " Token"

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
					card.PromoTypes = append(card.PromoTypes, "wizardsplaynetwork")
				}

			case "CMB1", "CMB2":
				// Rename cards that have names clashing with real cards
				switch card.Name {
				case "Pick Your Poison",
					"Red Herring":
					card.Name += " Playtest"
				// This could mess up Bind (INV)
				case "Bind // Liberate":
					card.Name = "Bind // Liberate Playtest"
					card.FaceName = "Bind Playtest"
				}

			case "TMC":
				set.Name = "Teenage Mutant Ninja Turtles Commander"
			}

			// Make sure this property is correctly initialized
			if strings.HasSuffix(card.Number, "p") && !slices.Contains(card.PromoTypes, PromoTypePromoPack) {
				card.PromoTypes = append(card.PromoTypes, PromoTypePromoPack)
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
				props := mtgmatcher.AlternateProps{
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
			// Sort printings by most recent sets first
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

			// AnonTCG: correct MTGJSON's mis-published TCGplayer productIds
			// (MSC surge-foil/base swap, HOB #239/#275 shared ids) before the
			// alt-foil split below reads the identifiers. No-op for every
			// other card. See tcgplayer_id_override.go.
			applyTCGIDOverride(card.Identifiers, card.UUID)

			// Save the original uuid
			card.Identifiers["mtgjsonId"] = card.UUID

			// Save the collector number stripped of its ★/†/φ decorations
			card.OriginalNumber = strings.TrimRight(card.Number, SuffixSpecial+SuffixVariant+SuffixPhiLow+"*")

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
		sort.Slice(rarities, func(i, j int) bool {
			return mtgRarityMap[rarities[i]] > mtgRarityMap[rarities[j]]
		})
		set.Rarities = rarities
		sort.Slice(colors, func(i, j int) bool {
			return mtgColorMap[colors[i]] > mtgColorMap[colors[j]]
		})
		set.Colors = colors

		// Adjust the setBaseSize to take into account the cards with
		// the same name in the same set (also make sure that it is
		// correctly initialized)
		if set.ReleaseDateTime.After(mtgmatcher.PromosForEverybodyYay) {
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

	duplicate(ap.Data, "Legends Italian", "LEG", "ITA", "1995-09-01")
	duplicate(ap.Data, "The Dark Italian", "DRK", "ITA", "1995-08-01")
	duplicate(ap.Data, "Alternate Fourth Edition", "4ED", "ALT", "1995-04-01")
	allSets = append(allSets, "LEGITA", "DRKITA", "4EDALT")

	sldDupes := duplicateCards(ap.Data, "SLD", "JPN", sldJPNLangDupes)
	ap.Data["SLD"].Cards = append(ap.Data["SLD"].Cards, sldDupes...)

	purlDupes := duplicateCards(ap.Data, "PURL", "JPN", []string{"1"})
	ap.Data["PURL"].Cards = append(ap.Data["PURL"].Cards, purlDupes...)

	// Generate the unique identifiers for singles and products
	uuids, allUUIDs, allSealedUUIDs, setUUIDs, setSealedUUIDs := generateUUIDsMap(ap.Data)

	// Remove promo tags that apply to a single finish only
	filterInvalidPromoTypes(ap.Data, uuids)

	// Add all names and associated uuids to the global names and hashes arrays
	hashes := map[string][]string{}
	var names, fullNames, lowerNames []string
	var sealed, fullSealed, lowerSealed []string
	var promoTypes []string
	externalIDs := map[string]string{}
	for _, uuid := range append(allUUIDs, allSealedUUIDs...) {
		card := uuids[uuid]

		// Load up the any external id
		for _, tag := range []string{
			"mtgjsonId",
			"scryfallId",
			"tcgplayerProductId",
			"tcgplayerEtchedProductId",
		} {
			id, found := card.Identifiers[tag]
			if !found {
				continue
			}
			// Skip if already loaded
			_, found = externalIDs[id]
			if found {
				continue
			}
			externalIDs[id] = card.UUID
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
			norm := mtgmatcher.Normalize(nameToAdd)
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
		norm := mtgmatcher.Normalize(card.Name)
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

	// Convert local sets to mtgmatcher sets
	mSets := make(map[string]*mtgmatcher.Set, len(ap.Data))
	for k, v := range ap.Data {
		mSets[k] = toMtgSet(v)
	}

	var b mtgmatcher.Backend

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

	b.SetRules(Rules{})

	return &b
}

var mtgRarityMap = map[string]int{
	"token":    1,
	"common":   2,
	"uncommon": 3,
	"rare":     4,
	"mythic":   5,
	"special":  6,
	"oversize": 7,
}

var mtgColorMap = map[string]int{
	"white":      7,
	"blue":       6,
	"black":      5,
	"red":        4,
	"green":      3,
	"colorless":  2,
	"multicolor": 1,
}

func fillinSLDdecks(set *Set) []string {
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
func fillinSealedContents(sets map[string]*Set, uuids map[string]*mtgmatcher.CardObject) {
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
func filterInvalidPromoTypes(sets map[string]*Set, uuids map[string]*mtgmatcher.CardObject) {
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

// Check whether the sealed product directly contains the given card. "Directly"
// means via a card/deck/pack entry at the top level (or inside a variable
// config) — not reachable only through a nested sealed sub-product. Finish is
// not checked here; we trust MTGJSON's per-finish SourceProducts bucketing.
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
					if wantFoil || wantEtched {
						if item.Foil {
							return true
						}
					} else if !item.Foil {
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
				// "sealed": intentionally not handled — nested products don't count as direct.
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

	// Rework printings information
	for i := range sets[code].Cards {
		// Skip misprints from main sets
		if strings.HasSuffix(sets[code].Cards[i].Number, SuffixVariant) {
			continue
		}

		// Update printings for the original set
		printings := append(sets[code].Cards[i].Printings, dup.Code)
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

	// Add duplicated set (with no cards) to the root
	sets[dup.Code] = &dup

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

	// Remove store references to avoid duplicates
	for i := range dup.Cards {
		altIdentifiers := map[string]string{}
		for k, v := range dup.Cards[i].Identifiers {
			switch k {
			case "tcgplayerProductId", "tcgplayerEtchedProductId", "mcmId", "mcmEtchedId":
				continue
			}
			altIdentifiers[k] = v
		}
		dup.Cards[i].Identifiers = altIdentifiers
	}
}

// Duplicate certain cards within the same set according to the language tag
func duplicateCards(sets map[string]*Set, code, tag string, numbers []string) []Card {
	var duplicates []Card

	for i := range sets[code].Cards {
		// Skip unneeded
		if !slices.Contains(numbers, sets[code].Cards[i].Number) {
			continue
		}

		mainUUID := sets[code].Cards[i].UUID

		// Update with new info
		dupeCard := sets[code].Cards[i]
		dupeCard.UUID = mainUUID + "_" + strings.ToLower(tag)
		dupeCard.Language = langs[tag]
		dupeCard.Number += strings.ToLower(tag)

		// Set a new code and edition name if we're duplicating a whole set
		_, found := sets[code+tag]
		if found {
			dupeCard.SetCode = code + tag
		}

		// Retrieve Printed data if available
		for _, foreignData := range sets[code].Cards[i].ForeignData {
			if foreignData.Language != dupeCard.Language {
				continue
			}
			dupeCard.PrintedName = foreignData.Name
			dupeCard.PrintedType = foreignData.Type
			dupeCard.Identifiers["originalScryfallId"] = foreignData.Identifiers["scryfallId"]
		}

		// Update images
		dupeCard.Images = map[string]string{}
		dupeCard.Images["full"] = generateImageURL(dupeCard, "normal")
		dupeCard.Images["thumbnail"] = generateImageURL(dupeCard, "small")
		dupeCard.Images["crop"] = generateImageURL(dupeCard, "art_crop")

		duplicates = append(duplicates, dupeCard)
	}

	return duplicates
}

const (
	suffixFoil   = "_f"
	suffixEtched = "_e"
)

// toMtgCard converts a local Card to mtgmatcher.Card via direct field copy.
func toMtgCard(c Card) mtgmatcher.Card {
	mc := mtgmatcher.Card{
		Artist:              c.Artist,
		AttractionLights:    c.AttractionLights,
		BorderColor:         c.BorderColor,
		Colors:              c.Colors,
		ColorIdentity:       c.ColorIdentity,
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
		OriginalNumber:      c.OriginalNumber,
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
	mc.ForeignData = make([]struct {
		Name        string            `json:"name"`
		Language    string            `json:"language"`
		Identifiers map[string]string `json:"identifiers"`
		Type        string            `json:"type"`
	}, len(c.ForeignData))
	for i, fd := range c.ForeignData {
		mc.ForeignData[i].Name = fd.Name
		mc.ForeignData[i].Language = fd.Language
		mc.ForeignData[i].Identifiers = fd.Identifiers
		mc.ForeignData[i].Type = fd.Type
	}
	return mc
}

// toMtgSet converts a local Set to *mtgmatcher.Set via JSON round-trip.
func toMtgSet(s *Set) *mtgmatcher.Set {
	if s == nil {
		return nil
	}
	data, _ := json.Marshal(s)
	ms := &mtgmatcher.Set{}
	_ = json.Unmarshal(data, ms)
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
