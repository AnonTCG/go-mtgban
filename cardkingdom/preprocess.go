package cardkingdom

import (
	"errors"
	"slices"
	"strings"

	"github.com/mtgban/go-cardkingdom"
	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/matcher/magic"
	"github.com/mtgban/go-mtgban/mtgban"
)

// This table contains all SKUs that contain incorrect codes or codes that could
// be mistaken for edition codes (thus misdirecting the matcher) or that contain
// incorrect numbers. Sometimes both.
var skuFixupTable = map[string]string{
	// Some of the lands from the first Arena set
	"PAL96-001": "PARL-001",
	"PAL96-003": "PARL-003",
	"PAL96-004": "PARL-004",

	// Lightning Bolt
	"F19-001": "PF19-001",

	// Path of Ancestry
	"PF21-001": "PLG21-C3",

	// Yellow Hidetsugu
	"PNEO-432": "NEO-432",

	// Random WCD cards
	"WC97-JS097":    "WC97-JS242",
	"WC97-PM037":    "WC97-PM037B",
	"WC98-343":      "WC98-BR343",
	"WC98-344":      "WC98-BR344",
	"WC98-345":      "WC98-BR345",
	"WC98-346":      "WC98-BR346",
	"WC98-RB330":    "WC98-RB330SB",
	"WC01-AB078":    "WC01-AB078SB",
	"WC02-SHH266":   "WC02-SHH266SB",
	"WC02-CR057SBA": "WC02-CR057SB",
	"WC02-SSH335":   "WC02-SHH335",
	"WC02-RL336":    "WC02-RL336A",
	"WC02-RL336A":   "WC02-RL336B",
	"WC02-CR337":    "WC02-CR337A",
	"WC02-CR337A":   "WC02-CR337B",
	"WC02-RL337":    "WC02-RL337A",
	"WC02-RL337A":   "WC02-RL337B",
	"WC03-WE062":    "WC03-WE062SB",

	// Planeshift Altx Art
	"PPLS-074": "PLS-074★",
	"PPLS-107": "PLS-107★",
	"PPLS-133": "PLS-133★",

	// Warhammer 40,000 surge-foil tokens: the set catalogues the foil
	// printing of these two under a ★-suffixed duplicate of the same
	// number, never the bare one CK's own sku names
	"SFT40K-015": "SFT40K-015★",
	"SFT40K-016": "SFT40K-016★",

	// The German misprint of Drudge Skeletons is filed under 4ED 107†
	"3ED-107-DE": "4ED-107†",

	// Duplicated ULST cards
	"FMUST-147A": "ULST-55",
	"FMUST-147F": "ULST-56",
	"FMUST-113A": "ULST-38",
	"FMUST-113C": "ULST-37",

	// Wrong PLST codes
	"MF19-001":  "MPF19-1",
	"MZNR-091":  "MKHC-91",
	"MTAFR-001": "PLST-TAFR-1",

	// Naya Sojourners
	"PM10-028": "DCI-29",
	// Mitotic Slime
	"PM11-185": "DCI-53",

	// Duel Decks Beast Token
	"TDDD-001": "TDDD-T1",
	"TDDD-002": "TDDD-T2",
	"TDDD-003": "TDDD-T3",

	// M20 Promo Pack lands
	"PRM-001P": "PPP1-1",
	"PRM-002P": "PPP1-2",
	"PRM-003P": "PPP1-3",
	"PRM-004P": "PPP1-4",
	"PRM-005P": "PPP1-5",

	// Crucible of Words promo
	"PWOR19-001": "PWOR-2019",

	// Flusterstorm BaB
	"MH1-255P":  "MH1-255",
	"PMH3-0496": "MH3-496",

	// Glimpse, the Unthinkable
	"MB2-0355": "MB2-594",

	// Spider-man Play Promos
	"PPSPM-0002B":  "PW25-10",
	"FPPSPM-0002B": "PW25-10",
	"PPSPM-0005":   "PW25-12",
	"FPPSPM-0005":  "PW25-12",
	"PPSPM-0003B":  "PW25-13",
	"FPPSPM-0003B": "PW25-13",

	// Some Avatar Eternal cards got merged in foil/nonfoil,
	// but they actually have different numbers
	"TLE-0210": "TLE-265",
	"TLE-0211": "TLE-266",
	"TLE-0212": "TLE-267",
	"TLE-0214": "TLE-268",
	"TLE-0215": "TLE-269",
	"TLE-0217": "TLE-270",
	"TLE-0218": "TLE-273",
	"TLE-0219": "TLE-274",
	"TLE-0220": "TLE-275",
	"TLE-0221": "TLE-276",
	"TLE-0234": "TLE-277",
	"TLE-0235": "TLE-278",
	"TLE-0236": "TLE-279",
	"TLE-0238": "TLE-280",
	"TLE-0239": "TLE-281",
	"TLE-0240": "TLE-282",
	"TLE-0241": "TLE-283",
	"TLE-0244": "TLE-285",
	"TLE-0245": "TLE-286",
	"TLE-0246": "TLE-287",
	"TLE-0247": "TLE-288",

	// Maximum effort
	"SLD-IFIYW-01":   "SLD-IFIYW-1",
	"SLD-IFIYW-02":   "SLD-IFIYW-2",
	"SLD-IFIYW-03":   "SLD-IFIYW-3",
	"SLD-IFIYW-04":   "SLD-IFIYW-4",
	"SLD-IFIYW-05":   "SLD-IFIYW-5",
	"CFSLD-IFIYW-06": "SLD-IFIYW-6",
	"CFSLD-IFIYW-07": "SLD-IFIYW-7",
	"CFSLD-IFIYW-08": "SLD-IFIYW-8",
	"CFSLD-IFIYW-09": "SLD-IFIYW-9",
	"CFSLD-IFIYW-10": "SLD-IFIYW-10",
}

// This table contains the names CK misspells, keyed by the sku the listing
// carries once skuFixupTable has had its say, and mapped to the name the set
// files the printing under. A misspelling is the vendor's own, so it is keyed
// literally rather than reached for by resemblance.
var nameFixupTable = map[string]string{
	// Edgar, Moonlit Sovereign
	"FRA-0257": "Edgar, Moonlit Sovereign",
}

// List of tags that need to be preserved in one way or another
var preserveTags = []string{
	"Display",
	"Etched",
	"Japanese",
	"JPN",
}

func setCodeExists(b *matcher.Backend, code string) bool {
	_, err := b.GetSet(code)
	return err == nil
}

// The wrappings a sku puts on the code of the set a token was filed with,
// the treatment first and the token itself last
var tokenSetPrefixes = []string{"F", "T", "FT", "SF", "RF", "CF"}

// unindexedTokenSheet reports whether a sku names a sheet of tokens no set in
// the datastore stands for, as the Jumpstart theme cards do: the set the
// sheet came with is carried, the sheet itself never was, so no row of it can
// match and none is worth reporting. The first prefix that leaves a set code
// decides, and only one ending in the token marker makes a sheet: FTRK is a
// foil of the TRK set, a card like any other.
func unindexedTokenSheet(b *matcher.Backend, sku string) bool {
	fields := strings.Split(sku, "-")
	if len(fields) < 2 || setCodeExists(b, fields[0]) {
		return false
	}
	for _, prefix := range tokenSetPrefixes {
		trimmed := strings.TrimPrefix(fields[0], prefix)
		if trimmed != fields[0] && setCodeExists(b, trimmed) {
			return strings.HasSuffix(prefix, "T")
		}
	}
	return false
}

// resolveEmblem answers the datastore's spelling and number for an emblem row,
// and empty strings for anything else or for a planeswalker the named set does
// not pin to exactly one emblem.
func resolveEmblem(b *matcher.Backend, edition, cardName, cardVariation string) (string, string) {
	face := cardName
	for _, separator := range []string{" // ", " - "} {
		before, _, found := strings.Cut(face, separator)
		if found {
			face = before
		}
	}

	hint := cardVariation
	inner, found := strings.CutPrefix(face, "Emblem (")
	if found {
		hint = strings.TrimSuffix(inner, ")")
	} else if face != "Emblem" {
		return "", ""
	}
	if hint == "" {
		return "", ""
	}

	set, err := b.GetSet(edition)
	if err != nil {
		return "", ""
	}

	var name, number string
	var matches int
	for _, token := range set.Tokens {
		if !strings.HasSuffix(token.Name, " Emblem") ||
			!strings.Contains(strings.ToLower(token.Name), strings.ToLower(hint)) {
			continue
		}
		name, number = token.Name, token.Number
		matches++
	}
	if matches != 1 {
		return "", ""
	}
	return name, number
}

// Preprocess turns a feed entry into the card description the matcher takes,
// reporting an error for the entries that are not cards.
func Preprocess(b *matcher.Backend, card cardkingdom.Product) (*matcher.InputCard, error) {
	foilVariant := strings.Contains(card.Variation, "Foil") && !strings.Contains(card.Variation, "Non")
	isFoil := card.IsFoil || foilVariant
	isEtched := strings.Contains(card.Variation, "Etched")

	// Retrieve setCode and number
	sku := card.SKU
	fields := strings.Split(sku, "-")
	if len(fields) < 2 {
		return nil, errors.New("unsupported SKU format")
	}
	setCode := fields[0]

	// Strip the initial F from set codes that do not exist
	if isFoil && strings.HasPrefix(sku, "F") && setCodeExists(b, setCode[1:]) {
		sku = sku[1:]
	}
	// Same for Etched and E
	if isEtched && strings.HasPrefix(sku, "E") && setCodeExists(b, setCode[1:]) {
		sku = sku[1:]
	}
	// ccccombo (EF is for emblem foils)
	if isFoil && isEtched && strings.HasPrefix(sku, "FE") && setCodeExists(b, setCode[2:]) {
		sku = sku[2:]
	}

	// Custom replacements
	fixup, found := skuFixupTable[sku]
	if found {
		sku = fixup
	}

	fixedName, found := nameFixupTable[sku]
	if found {
		card.Name = fixedName
	}

	// Update the fields if needed
	fields = strings.Split(sku, "-")
	setCode = fields[0]

	number := strings.Join(fields[1:], "")
	number = strings.TrimLeft(number, "0")
	number = strings.TrimRight(number, "JP")
	number = strings.TrimRight(number, "IT")

	edition := setCode
	variation := strings.ToLower(number)

	// CK titles a punch card after the set it came in, "Hour of Devastation
	// Punch Card", while the datastore files it as Punchcard on that set's
	// token sheet, so the title reaches no printing on its own. The sku's
	// own number is CK's index rather than the card's ("001X" against the
	// sheet's 18), and the scryfall id the row publishes cannot stand in
	// for either: Lorwyn Eclipsed's punch card carries the Treefolk
	// token's id, two more carry one the datastore does not know and three
	// carry none at all. Asking the sheet for the name it files is the
	// only anchor that answers for all of them, and a sheet holding no
	// punch card refuses the row quietly - there is no printing for it to
	// reach, and nothing the log can add.
	if strings.HasSuffix(card.Name, " Punch Card") {
		if !sheetHolds(b, setCode, punchcardName) {
			return nil, matcher.ErrUnsupported
		}
		card.Name = punchcardName
	}

	// Validate if setCode exists, if not preserve info from the card
	if !setCodeExists(b, setCode) {
		if (len(setCode) > 3 && setCodeExists(b, setCode[len(setCode)-3:])) ||
			(len(setCode) > 4 && setCodeExists(b, setCode[len(setCode)-4:])) {
			edition = card.Edition
			variation += " " + card.Variation
		}
	}

	switch card.Edition {
	case "World Championships":
		if strings.HasPrefix(variation, "sr") {
			variation = strings.Replace(variation, "sr", "shr", 1)
		}
	case "Deckmaster",
		"Collectors Ed",
		"Collectors Ed Intl":
		variation = card.Variation
	case "Promo Pack":
		variation = card.Variation
		edition = card.Edition
		// Until the stamped set exists, reach the sku's own printing by number
		// so the missing promo tag skips it, rather than another set's pack.
		if !setCodeExists(b, "P"+setCode) && setCodeExists(b, setCode) {
			edition = setCode
			variation = number + " Promo Pack"
		}
	case "Promotional":
		variation = card.Variation
		switch {
		case strings.Contains(variation, "APAC"),
			strings.Contains(variation, "German Misprint"),
			strings.Contains(variation, "Euro"):
			variation = number
		case strings.Contains(variation, "Arena"),
			strings.Contains(variation, "Game Day"),
			strings.Contains(variation, "Gameday"):
			edition = card.Edition
		case strings.Contains(variation, "Symbol"):
			maybeNum := setCode + "-" + strings.TrimLeft(number, "0")
			if len(b.MatchInSetNumber(card.Name, "PLST", maybeNum)) == 1 {
				edition = "PLST"
				variation = maybeNum
			}
		case strings.Contains(variation, "Ugin's Fate"):
			edition = "UGIN"
		case strings.Contains(setCode, "DFT") && strings.Contains(card.Name, "Raceway"):
			edition = "DFT"
			variation += " Bundle"
		case variation == "Commander's Bundle Promo":
			edition = strings.TrimPrefix(setCode, "P")
		case setCode == "PLG21" && strings.Contains(variation, "Japanese"):
			// The datastore numbers the Japanese Orbs J1-J3, CK 001-003
			variation = "J" + number
		}
	case "Mystery Booster/The List":
		edition = card.Edition
		switch setCode {
		case "CMB1":
			variation = card.Variation
		// Code modified from original SKU
		case "ULST":
			edition = setCode
			variation = number
		default:
			variation = setCode[1:] + "-" + strings.TrimLeft(number, "0")
			// A trailing P marks CK's promo copy, which PLST files under the
			// promo set's own code wherever it holds a row for it
			if strings.HasSuffix(sku, "P") && len(b.MatchInSetNumber(card.Name, "PLST", "P"+variation)) == 1 {
				variation = "P" + variation
			}
		}
	case "Streets of New Capenna Variants":
		if card.Name == "Gala Greeters" {
			variation = card.Variation
		}
	case "Ultimate Box Topper":
		edition = "PUMA"
	case "Avatar: The Last Airbender Eternal-Legal":
		// Look up the sku again, and restore the original one if foil
		_, found := skuFixupTable[strings.TrimPrefix(card.SKU, "F")]
		if found && isFoil {
			fields = strings.Split(card.SKU, "-")
			variation = strings.TrimLeft(fields[1], "0")
		}
	case "Secret Lair":
		// Restore the correct hyphen set for this given drop
		if strings.Contains(number, "IFIYW") {
			variation = strings.Join(fields[1:], "-")
		}
	}

	// CK writes an emblem as the bare word plus the planeswalker, either
	// parenthesized in the name or left in the variation, while the
	// datastore spells the whole planeswalker name into the token name
	if name, number := resolveEmblem(b, edition, card.Name, card.Variation); name != "" {
		card.Name = name
		variation = number
	}

	// Preserve any remaining tag
	for _, tag := range preserveTags {
		if strings.Contains(card.Variation, tag) && !strings.Contains(variation, tag) {
			variation += " " + tag
		}
	}

	// Unwrap a "TF" (textured foil) prefix before reading the "T" token-sheet
	// check below, or a split card's textured foil misreads as a token sheet.
	tokenSetCode := setCode
	trimmed := strings.TrimPrefix(setCode, "TF")
	if trimmed != setCode {
		set, err := b.GetSet(trimmed)
		if err == nil && set.Type != "token" {
			tokenSetCode = trimmed
		}
	}

	isTwoSidedToken := (strings.Contains(card.Name, " // ") || strings.Contains(card.Name, " - ")) &&
		(strings.Contains(card.Name, "Token") || strings.HasPrefix(tokenSetCode, "T") || strings.HasPrefix(tokenSetCode, "FT"))

	// A two-sided token sheet prints one physical card for a pairing
	// matcher/magic may already carry a combined entity for - resolve
	// that precisely, by id, before falling back to the one-face collapse
	// below. See magic.MatchTokenPairing (shared with every other vendor
	// package resolving its own two-sided token listings the same way).
	if isTwoSidedToken {
		if id := magic.MatchTokenPairing(b, card.ScryfallID, card.Name, isFoil); id != "" {
			return &matcher.InputCard{ID: id, Foil: isFoil}, nil
		}

		// CK never publishes a scryfallId for a "Mystery Booster/The
		// List" listing that bundles two independently-numbered
		// token-sheet entries into one retail sku (no vendor ever
		// sells that exact ad-hoc pairing as one product, so no id
		// exists to publish) - anchor the first face by its own sku
		// set/number instead. setCode/number are still the raw sku's
		// own fields here (the switch above only rewrote variation),
		// so setCode[1:] is the same real token-filing set the
		// "Mystery Booster/The List" case's own default arm already
		// derives.
		if card.ScryfallID == "" && card.Edition == "Mystery Booster/The List" &&
			strings.HasPrefix(setCode, "MT") && setCodeExists(b, setCode[1:]) {
			if id := magic.MatchTokenPairingBySetNumber(b, setCode[1:], number, card.Name, isFoil); id != "" {
				return &matcher.InputCard{ID: id, Foil: isFoil}, nil
			}
		}
	}

	// Drop one side of dfc tokens, without doubling the suffix when the
	// kept face already carries it, and leaving alone the split cards a
	// T-prefixed set code sweeps in: the set carries those under both faces
	if isTwoSidedToken &&
		len(b.MatchInSetNumber(card.Name, setCode, number)) == 0 {
		if strings.Contains(card.Name, " // ") {
			card.Name = strings.Split(card.Name, " // ")[0]
		} else {
			card.Name = strings.Split(card.Name, " - ")[0]
		}
		if !strings.HasSuffix(card.Name, "Token") {
			card.Name += " Token"
		}
	}
	// Tokens are filed under their own set code when the datastore carries
	// one, while a code it does not carry names the set the tokens are
	// filed with once its treatment and token wrappings are stripped
	if (strings.Contains(card.Name, "Token") || strings.Contains(card.Name, "Bounty")) &&
		!setCodeExists(b, setCode) {
		for _, prefix := range tokenSetPrefixes {
			trimmed := strings.TrimPrefix(setCode, prefix)
			if trimmed != setCode && setCodeExists(b, trimmed) {
				edition = trimmed
				break
			}
		}
	}

	// The treatment a token sku wraps its set code in promises a finish the
	// sheet was never sold in, and stripping the wrapping to reach the sheet
	// drops that promise: the row would land on the plain printing and be
	// priced as it, right beside the plain row that belongs there
	printing := tokenPrinting(b, edition, number)
	if isFoil && printing != nil &&
		!printing.HasFinish("foil") && !printing.HasFinish("etched") {
		return nil, matcher.ErrUnsupported
	}

	return &matcher.InputCard{
		Name:      card.Name,
		Edition:   edition,
		Variation: variation,
		Foil:      isFoil,
	}, nil
}

// matchPrereleaseSKU rescues a PREL sku Match refused: CK titles it
// Prerelease, but the id it publishes may name a Release promo instead.
func matchPrereleaseSKU(b *matcher.Backend, sku, scryfallID string, foil bool) (string, error) {
	if !strings.HasPrefix(sku, "PREL-") {
		return "", matcher.ErrUnsupported
	}
	id, err := b.MatchID(scryfallID, foil, false)
	if err != nil {
		return "", err
	}
	co, err := b.GetUUID(id)
	if err != nil || !co.HasPromoType(magic.PromoTypeRelease) {
		return "", matcher.ErrUnsupported
	}
	return id, nil
}

// punchcardName is the name the datastore files a punch card under, one word
// where every vendor selling one writes two.
const punchcardName = "Punchcard"

// sheetHolds reports whether the set files a printing under exactly this
// name.
func sheetHolds(b *matcher.Backend, code, name string) bool {
	set, err := b.GetSet(code)
	if err != nil {
		return false
	}
	for _, printing := range set.Cards {
		if printing.Name == name {
			return true
		}
	}
	return false
}

// tokenPrinting answers the printing a token sheet files at a number, asking
// the sheet rather than the name because a token only carries the Token
// suffix when a real card answers to the same name.
func tokenPrinting(b *matcher.Backend, code, number string) *matcher.Card {
	set, err := b.GetSet(code)
	if err != nil || set.Type != "token" {
		return nil
	}
	for i, printing := range set.Cards {
		if printing.Number == number {
			return &set.Cards[i]
		}
	}
	return nil
}

func preprocessGraded(title string) (*matcher.InputCard, error) {
	if strings.Contains(title, "Multiverse Mystery Slab") {
		return nil, matcher.ErrUnsupported
	}

	// A graded title is "Name (edition and grade) #id", and the edition may
	// carry a parenthetical of its own - "(TMNT Foil (Showcase) CGC
	// Pristine 10)". The split hands those back as a third field, and
	// refusing them dropped 41 listings of the roughly 950 this storefront
	// grades, every one of them silently.
	vars := matcher.SplitVariants(title)
	if len(vars) != 2 && len(vars) != 3 {
		return nil, errors.New("unsupported format")
	}

	cardName := vars[0]
	edition := strings.Replace(vars[1], "- ", "", -1)
	variant := ""
	if len(vars) == 3 {
		// The inner parenthetical is the treatment, which is what the
		// score would otherwise have been cut off to leave. The grade goes
		// missing with it, and is read off the whole title separately.
		variant = vars[2]
	}

	// Remove serialized number tags
	if strings.Contains(cardName, "/") {
		fields := strings.Fields(cardName)
		for i := range fields {
			if strings.Contains(fields[i], "/") {
				fields[i] = ""
			}
		}
		cardName = strings.Join(fields, " ")
		cardName = strings.Replace(cardName, "  ", " ", -1)
	}

	for _, score := range supportedScores {
		before, after, found := strings.Cut(edition, score)
		if !found {
			continue
		}
		edition = strings.TrimSpace(before)
		if rest := strings.TrimSpace(after); rest != "" {
			if variant != "" {
				variant += " "
			}
			variant += rest
		}
		break
	}

	isFoil := strings.Contains(variant, "Foil") || strings.Contains(edition, "Foil")
	edition = strings.TrimSuffix(edition, " Foil")
	variant = strings.TrimSuffix(variant, " Foil")

	// "X Eternal-Legal" is CK's own name for the datastore's "X Eternal"
	// (SPE, HOC, TLE); Replace, not TrimSuffix, since a tag can still
	// trail it ("The Hobbit Eternal-Legal Borderless Foil").
	edition = strings.Replace(edition, " Eternal-Legal", " Eternal", 1)

	// Hack to remove 9.5-style scores
	variant = strings.Replace(variant, ".", "", -1)
	num := matcher.ExtractNumber(variant)
	if num != "" {
		variant = strings.Replace(variant, num, "", -1)
	}
	// Pristine is a CGC grade tier, not a printing detail.
	variant = strings.Replace(variant, "Pristine", "", -1)
	variant = strings.TrimSpace(variant)

	if renamed, found := gradedEditions[edition]; found {
		edition = renamed
	}

	if strings.Contains(edition, "Final Fantasy") {
		if variant != "" {
			variant += " "
		}
		variant += edition
		if strings.HasPrefix(edition, "Final Fantasy Through the Ages") {
			edition = "FCA"
		}
	}

	// Move tags to the appropriate field to help edition matching
	for _, tag := range []string{
		"Borderless", "Extended Art", "Serialized", "Textured", "Japan Showcase", "Raised", "Halo",
		"Breaking News Showcase", "Breaking New", "Showcase Magnified", "Godzilla Series", "Etched",
		"Showcase", // needs to be last
	} {
		if strings.HasSuffix(edition, tag) {
			edition = strings.TrimSuffix(edition, " "+tag)
			moved := tag
			// "Breaking New" is this storefront's own typo for Breaking News.
			switch tag {
			case "Breaking New":
				edition, moved = "Breaking News", ""
			case "Breaking News Showcase":
				edition, moved = "Breaking News", "Showcase"
			}
			if moved != "" {
				if variant != "" {
					variant += " "
				}
				variant += moved
			}
		}
	}

	return &matcher.InputCard{
		Name:      cardName,
		Edition:   edition,
		Variation: variant,
		Foil:      isFoil,
	}, nil
}

// matchGraded retries an unknown "Secret Lair" edition against the
// storefront's own Secret Lair Countdown shelf before giving up, and settles
// a tie that only a claim-only promo type causes (see unclaimedPrinting).
func matchGraded(b *matcher.Backend, theCard *matcher.InputCard) (string, error) {
	cardID, err := b.Match(theCard)
	if errors.Is(err, matcher.ErrCardNotInEdition) && theCard.Edition == "Secret Lair" {
		retry := *theCard
		retry.Edition = "Secret Lair Countdown"
		id, rerr := b.Match(&retry)
		if rerr == nil {
			return id, nil
		}
	}

	var alias *matcher.AliasingError
	if errors.As(err, &alias) {
		id := unclaimedPrinting(b, theCard, alias.Probe())
		if id != "" {
			return id, nil
		}
	}
	return cardID, err
}

// gradedClaimOnly lists the promo types a graded title always spells out
// when the slab is one, with the words it spells them with.
var gradedClaimOnly = []struct {
	promoType string
	words     []string
}{
	{magic.PromoTypeJapanShowcase, []string{"japan", "jp"}},
	{magic.PromoTypeHeadliner, []string{"signature", "headliner"}},
}

// unclaimedPrinting answers the one candidate left once the printings whose
// promo type the title never claims are set aside, and empty when that does
// not leave exactly one.
func unclaimedPrinting(b *matcher.Backend, theCard *matcher.InputCard, candidates []string) string {
	text := strings.ToLower(theCard.Edition + " " + theCard.Variation)

	var left []string
	for _, id := range candidates {
		co, err := b.GetUUID(id)
		if err != nil {
			return ""
		}
		claimed := true
		for _, rule := range gradedClaimOnly {
			spelled := slices.ContainsFunc(rule.words, func(word string) bool {
				return strings.Contains(text, word)
			})
			if co.HasPromoType(rule.promoType) && !spelled {
				claimed = false
			}
		}
		if claimed {
			left = append(left, id)
		}
	}
	if len(left) != 1 {
		return ""
	}
	return left[0]
}

// gradedEditions spells an edition this storefront abbreviates the way the
// catalog writes it out.
//
// The abbreviation alone is not the whole of it. "TMNT" on its own reaches
// the Teenage Mutant Ninja Turtles set, but "TMNT Source Material Cards"
// reaches nothing and the card falls back to the set it was first printed
// in - Plague of Vermin to Shadowmoor, Waves of Aggression to Eventide,
// each an ordinary card standing in for a Universes Beyond reprint. Neither
// half fixes it by itself: spelling the name out still leaves a trailing
// "Cards" the catalog does not have, and dropping "Cards" still leaves the
// abbreviation. So the edition is named outright, which is how every other
// spelling this storefront uses is handled.
var gradedEditions = map[string]string{
	"TMNT Source Material Cards":         "Teenage Mutant Ninja Turtles Source Material",
	"Avatar Suki of the Kyoshi Warriors": "Avatar: The Last Airbender Eternal",
	"Promotional RPTQ Promo":             "Pro Tour Promos",
}

// gradedScryfallIDs names the printing of a slab whose title leaves several
// candidates, keyed by the storefront's product id.
var gradedScryfallIDs = map[string]string{
	"312820": "2e97dadd-0849-4c18-9523-4775d09fca9a", // SNC 279
	"314659": "07eb0e22-1e19-46e0-913e-94ccbf858bb9", // M10 249
	"315977": "013612b4-fed9-4112-8018-9267ae608fd7", // MOM 287
	"315996": "c464b81a-3d91-4d07-bc9d-6756780417e3", // BRO 284
	"316001": "ef099f25-2b0b-4028-8490-fbc859e35f6c", // MH3 343
	"317660": "5549b73a-9abe-4d63-a673-aa08e9f6a678", // CED 290
	"318592": "f0ca4b9f-4ee6-4ad8-a95f-326ada9de3cd", // ZEN 246
	"321125": "ed878350-ebc3-419b-9dd9-384e44906569", // EOS 35
	"321128": "010abd40-3f9c-44bc-8570-d539750c3cd7", // EOS 14
	"321786": "6f9c1419-aa54-4dd3-8d7d-4e2f60e7f581", // SLD 115
	"321922": "651f1740-1a79-42c5-bc0b-33a79a00621e", // SLD 687
	"323183": "f76d6ecc-a774-44d5-8c16-e2ba4bf7d283", // SLD 1514
	"323411": "59bdb4a0-0f2d-4018-ad74-3970a5cd71ab", // SPM 237
	"323459": "c163a697-01ee-4a55-814f-b0ba79be3a7f", // ACR 105
	"323559": "040ed422-663a-4b6c-bdcd-09092f1c9004", // PIP 320
	"324425": "feef6ea3-67f3-42d8-bf19-02ba9151c2a3", // SLD 1150
	"324725": "b2d1219f-fadb-4427-8627-e5aef3589600", // SLD 1591
	"325111": "68df89dc-3909-4051-adc1-a86589d0e99d", // INV 345
	"326239": "5d93b770-dc46-46ad-aefe-282dac8cc246", // SLD 7010
	"326989": "29da3c7f-f886-4be0-bce5-d2707c5fc926", // SLD 1146
	"327025": "6e169f68-d336-429d-bdd0-f034a53391a3", // SLD 1170
	"330063": "0f0cdff4-c81e-4e53-8721-748eb71d9e81", // SLD 1145
	"331275": "e1e88b41-7ae5-40fc-8947-5f5aa03388be", // ODY 344
	"331402": "16ffb489-3c9c-43e2-b407-aae1ba55035a", // SLD 7034
	"334364": "d7741b03-c16a-426c-8847-622b5fc1236f", // PWCS 2023-4
	"334652": "81690ce7-0651-40e4-9769-baebe10e2cbf", // FIN 551a
	"334653": "ac3db85f-8cc3-4869-a555-882b62a1726b", // FIN 551b
	"334693": "81690ce7-0651-40e4-9769-baebe10e2cbf", // FIN 551a
	"335186": "5d393449-bdbe-4638-ba0b-7ec83f81b51e", // MAR 41
}

var supportedScores = []string{
	"PSA", "BGS", "CGC",
}

// parseGradedCondition reads the grade off a graded listing's title, and
// alongside it the grader-plus-score text a caller can name a refusal by
// ("PSA 11"), empty when the title names no known grader at all.
func parseGradedCondition(title string) (mtgban.Condition, string) {
	var grade string
	var score string
	for _, score = range supportedScores {
		_, after, found := strings.Cut(title, score+" ")
		if !found {
			continue
		}
		grade = after
		break
	}

	if grade == "" {
		return "", ""
	}

	grade = strings.Split(grade, ")")[0]
	grade = strings.Split(grade, ".")[0]
	grade = strings.TrimSuffix(grade, " Quad ++")
	grade = strings.TrimSuffix(grade, " Quad++")

	slab := score + " " + grade

	// A grade the table does not cover has no condition, and must not fall
	// through to the zero value: an empty condition is silently promoted to
	// NM when the entry is added
	condition, found := mtgban.SlabCondition(score, grade)
	if !found {
		return "", slab
	}

	return condition, slab
}
