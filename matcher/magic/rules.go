package magic

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// Rules implements matcher.GameRules for Magic: the Gathering, including
// candidate edition selection and the World Championship ambiguity policy.
type Rules struct{}

// IsUnsupported reports the listings Magic has no printing for: a class of
// product by its wording, a card unsupported in one edition once that
// edition is known, and a promo the resolved printing does not carry yet.
// See matcher.GameRules.
func (Rules) IsUnsupported(b *matcher.Backend, inCard *matcher.InputCard, co *matcher.CardObject, stage matcher.Stage) bool {
	switch stage {
	case matcher.StageWording:
		return isUnsupported(inCard)
	case matcher.StageEdition:
		return unsupportedInEdition(b, inCard)
	case matcher.StageAnswer:
		return missingPromoTag(b, inCard, co)
	}
	return false
}

// unsupportedInEdition reports the named cards unsupported in one edition
// rather than as a class.
func unsupportedInEdition(b *matcher.Backend, inCard *matcher.InputCard) bool {
	// A word saying token in an edition or a variation names a custom set
	// that leaked this far only when no set of tokens answers to the
	// edition: Magic's tokens are carried, each filed under a set of its
	// own named after the one it came with. The check is Magic's, not
	// core's, because other games' tokens are cards like any other -
	// Yu-Gi-Oh files "Token: Kuriboh" under a collector number and sells
	// it by it.
	//
	// The word is looked for with strings rather than Contains, which
	// filters "token" away before the comparison ever sees it.
	editionSaysToken := strings.Contains(strings.ToLower(inCard.Edition), "token")
	variationSaysToken := strings.Contains(strings.ToLower(inCard.Variation), "token")
	if (editionSaysToken || variationSaysToken) && !inCard.Contains("League") {
		if !carriesTokens(b, inCard.Edition) {
			return true
		}
		// The set's sheet being carried admits the edition, but a
		// variation saying token asks for a token by this name; when
		// none is filed the name can only reach the card sharing its
		// bucket, and a token listing must not price the card.
		if variationSaysToken && !editionSaysToken && !namesAToken(b, inCard.Name) {
			return true
		}
	}
	return isSpecificUnsupported(inCard)
}

// namesAToken reports whether a name is a token's: spelled with the word
// itself, or filed under the key that appends it - where every plain-named
// token lives, its bare form left to whatever card normalizes the same way.
func namesAToken(b *matcher.Backend, name string) bool {
	norm := matcher.Normalize(name)
	if strings.HasSuffix(norm, "token") {
		return true
	}
	_, found := b.CanonicalNames[norm+"token"]
	return found
}

// carriesTokens reports whether an edition names a set whose tokens the
// datastore holds, either a sheet of them or the set one was printed beside.
// The word token is asked of the edition alone, so a storefront that names
// the set plainly and says token in the variation is answered the same way.
func carriesTokens(b *matcher.Backend, edition string) bool {
	set, err := b.GetSetByName(edition)
	if err != nil {
		return false
	}
	if set.Type == "token" {
		return true
	}
	_, found := b.Sets[set.TokenSetCode]
	return found
}

// ravnicaWeekend resolves a Ravnica Weekend printing to its edition and
// collector number, by number when the input carries one and by guild name
// through the variant tables otherwise. The wording is the card's own
// edition and variation, or a bare edition and empties on the alias path.
func ravnicaWeekend(name, edition, variation string) (string, string) {
	num := matcher.ExtractNumber(variation)
	if strings.HasPrefix(num, "a") {
		return "GRN Ravnica Weekend", num
	} else if strings.HasPrefix(num, "b") {
		return "RNA Ravnica Weekend", num
	}

	for _, guild := range grnGuilds {
		if matcher.Contains(edition, guild) || matcher.Contains(variation, guild) {
			return "GRN Ravnica Weekend", prwkVariants[name][strings.ToLower(guild)]
		}
	}
	for _, guild := range arnGuilds {
		if matcher.Contains(edition, guild) || matcher.Contains(variation, guild) {
			return "RNA Ravnica Weekend", prw2Variants[name][strings.ToLower(guild)]
		}
	}
	return "", ""
}

// ravnicaGuildKit returns which Guild Kit the wording names, by set name or
// set code, or an empty string if it names none.
func ravnicaGuildKit(b *matcher.Backend, name, edition, variation string) string {
	contains := func(prop string) bool {
		return matcher.Contains(edition, prop) || matcher.Contains(variation, prop)
	}
	if !contains("Guild Kit") {
		return ""
	}

	if contains("Guilds of Ravnica") || contains("GRN") {
		return "GRN Guild Kit"
	}
	if contains("Ravnica Allegiance") || contains("RNA") {
		return "RNA Guild Kit"
	}

	if slices.ContainsFunc(grnGuilds, contains) {
		return "GRN Guild Kit"
	}
	if slices.ContainsFunc(arnGuilds, contains) {
		return "RNA Guild Kit"
	}

	if isExactBasicLand(name) {
		return "Guild Kit"
	}
	if len(b.MatchInSet(name, "GK1")) > 0 {
		return "GRN Guild Kit"
	}
	if len(b.MatchInSet(name, "GK2")) > 0 {
		return "RNA Guild Kit"
	}

	return ""
}

// mediaInsertOriginals are the cards given away with the Destroy All Humans
// manga: a Japanese foil with each original volume, reprinted in English when
// the volume was translated years later. Storefronts file both under one
// edition and tell them apart by the original's own number or by naming the
// language.
//
// Only the original is named here, and only once its English reprint exists -
// while the Japanese printing is the only one, naming it would veto it. The
// reprint's number is deliberately absent: the set's numbers move, and a
// stale one answers every English listing of the card with "unknown variant".
var mediaInsertOriginals = map[string]struct {
	// marker is the number a storefront writes for the original, from the
	// old "Media Promos" numbering. It is compared against the number the
	// variation states rather than searched for inside it, so the volumes
	// numbered #1 apiece say as much as the rest, and a listing leading
	// with a year still reaches the number written after it.
	marker string
	// number is the printing that number names.
	number string
	// idw is the comic insert's number, for the one card that had one.
	idw string
}{
	"Diabolic Edict":   {marker: "31", number: "2019-2"},
	"Shock":            {marker: "32", number: "2019-3"},
	"Duress":           {marker: "34", number: "2019-6", idw: "17"},
	"Voltaic Key":      {marker: "35", number: "2020-1"},
	"Dark Ritual":      {marker: "38", number: "2020-4"},
	"Crop Rotation":    {marker: "42", number: "2020-7"},
	"Counterspell":     {marker: "44", number: "2021-1"},
	"Bone Shredder":    {marker: "45", number: "2021-2"},
	"Disenchant":       {number: "2022-1"},
	"Wild Growth":      {number: "2022-2"},
	"Frantic Search":   {number: "2022-4"},
	"Worn Powerstone":  {number: "2023-2"},
	"Avalanche Riders": {marker: "55", number: "2023-5"},
	"Culling the Weak": {marker: "57", number: "2023-8"},
	"Snuff Out":        {marker: "1", number: "2024-1"},
	"Gush":             {marker: "1", number: "2024-4"},
	"Ancestral Mask":   {marker: "1", number: "2025-2"},
	"Wrath of God":     {marker: "1", number: "2025-3"},
}

// tableEdition spells an edition through the set codes and the alias table:
// a code becomes its set's name, and a name the table knows becomes the one
// the datastore files it under.
func tableEdition(b *matcher.Backend, edition string) string {
	set, found := b.Sets[strings.ToUpper(edition)]
	if found {
		edition = set.Name
	}
	if ed, found := EditionTable[edition]; found {
		edition = ed
	}
	return edition
}

// trimEditionDecorations cuts the decorations storefronts hang on an edition
// name - the game-name prefixes, the text past a dash, the known suffix tags
// - respelling through the alias table after each cut, and reports whether
// anything came off. keepDashes says the wording spells a promo whose dash
// belongs to the name rather than a decoration.
func trimEditionDecorations(edition string, keepDashes bool) (string, bool) {
	trimmed := false
	for _, prefix := range []string{
		"Magic: The Gathering - ",
		"Magic: the Gathering - ",
		"Magic: The Gathering | ",
		"Magic: the Gathering | ",
	} {
		edition = strings.TrimPrefix(edition, prefix)
	}

	// Test for "- " and " -" to avoid catching dashes in the name itself
	if !keepDashes && (strings.Contains(edition, "- ") || strings.Contains(edition, " -")) {
		edition = strings.TrimSpace(strings.Split(edition, "-")[0])
		if ed, found := EditionTable[edition]; found {
			edition = ed
		}
		trimmed = true
	}
	for _, tag := range []string{
		"Box Toppers",
		"(Collector Edition)",
		"Collectors",
		"Collector Booster",
		"Extras",
		"Variants",
		"Etched",
		"Serialized",
		"Surge Foil",
		"Holiday Release",
		"Alternate Foil",
		"Retro Frame",
	} {
		if strings.HasSuffix(edition, tag) {
			edition = strings.TrimSuffix(edition, tag)
			edition = strings.TrimSpace(edition)
			edition = strings.TrimSuffix(edition, ":")
			edition = strings.TrimSuffix(edition, "-")
			edition = strings.TrimSpace(edition)
			if ed, found := EditionTable[edition]; found {
				edition = ed
			}
			trimmed = true
		}
	}
	return edition, trimmed
}

// trimUniversesBeyond cuts the Universes Beyond heading off an edition name,
// respelling what remains through the alias table.
func trimUniversesBeyond(edition string) string {
	edition = strings.TrimPrefix(edition, "Universes Beyond")
	edition = strings.TrimPrefix(edition, "UB")
	edition = strings.TrimLeft(edition, ":- ")
	if ed, found := EditionTable[edition]; found {
		edition = ed
	}
	return edition
}

// boxSetEdition names the box set an edition spells, or trims the
// decorations off one naming none, reporting whether anything came off.
func boxSetEdition(edition string) (string, bool) {
	switch {
	case matcher.Contains(edition, "Mythic Edition"):
		return "Mythic Edition", false
	case strings.Contains(edition, "Invocation"):
		return "Amonkhet Invocations", false
	case strings.Contains(edition, "Inventions"):
		return "Kaladesh Inventions", false
	case strings.Contains(edition, "Expeditions") && !strings.Contains(edition, "Rising"):
		return "Zendikar Expeditions", false
	case strings.Contains(edition, "Expeditions") && strings.Contains(edition, "Rising"):
		return "Zendikar Rising Expeditions", false
	case matcher.Contains(edition, "Timeshift") && matcher.Contains(edition, "Spiral") &&
		!saysMysteryList(edition, ""):
		// The card's name is what picks the timeshifted sheet; the fixup
		// asks it after the aliasing, and the trims stay off either way.
		return edition, false
	}
	// A Prerelease, PromoPack or MB1/List dash separates data, not a
	// decoration; those are processed elsewhere
	keepDashes := matcher.Contains(edition, "Prerelease") || matcher.Contains(edition, "Preview") ||
		matcher.Contains(edition, "Promo Pack") || saysMysteryList(edition, "")
	return trimEditionDecorations(edition, keepDashes)
}

// setFamilyEdition resolves the set families told apart by their wording -
// the Universes Beyond and Commander headings, the Ravnica weekends and
// guild kits, the junior series - returning the edition, the variation, and
// whether the wording asked for the beyond-base-set search. cardEdition is
// the spelling the card still holds while the aliasing continues on
// edition; the alias passes its own pre-trim spelling there.
func setFamilyEdition(b *matcher.Backend, name, edition, cardEdition, variation string) (string, string) {
	contains := func(prop string) bool {
		return matcher.Contains(cardEdition, prop) || matcher.Contains(variation, prop)
	}
	switch {
	case strings.HasPrefix(edition, "Universes Beyond"),
		strings.HasPrefix(edition, "UB:"):
		edition = trimUniversesBeyond(edition)
	case strings.Contains(edition, "Commander") &&
		(!contains("Oversize") || contains("Plane") || contains("Phenomenon")) &&
		!contains("Party"):
		ed := parseCommanderEdition(b, edition, variation)
		if ed != "" {
			edition = ed
		}
	case contains("Ravnica Weekend"):
		ed, vr := ravnicaWeekend(name, cardEdition, variation)
		if ed != "" {
			edition, variation = ed, vr
		}
	case contains("Guild Kit"):
		ed := ravnicaGuildKit(b, name, cardEdition, variation)
		if ed != "" {
			edition = ed
		}
	}
	return edition, variation
}

// AdjustEdition normalizes the edition a storefront published toward a set
// name. See matcher.GameRules.
func (Rules) AdjustEdition(b *matcher.Backend, inCard *matcher.InputCard) {
	edition, trimmed := aliasEdition(b, inCard.Edition)
	variation := inCard.Variation

	set, found := b.Sets[strings.ToUpper(variation)]
	if found && (isJudge(inCard) || isDuelDecks(inCard) || isDuelDecksAnthology(inCard)) {
		edition = set.Name
	}
	ed, found := EditionTable[variation]
	// The Anthologies set has one land with a variant named as an expansion,
	// so what is found should not overwrite the edition in this case
	// As for The List, ignore any further variation
	if found && edition != "Anthologies" && !isMysteryList(inCard) {
		edition = ed

		// If edition was found through the variation tag, drop it
		variation = ""
		// Only keep this information if needed
		if inCard.IsEtched() {
			variation = "Etched"
		}
	}
	// If no other variation, set this flag to do a best effort search
	if trimmed && variation == "" {
		inCard.BeyondBaseSet = true
	}
	inCard.Edition = edition
	inCard.Variation = variation

	// What only the card's own wording can add to the aliasing: the box
	// sets its variation names, and the timeshifted sheet its name picks
	switch {
	case matcher.Contains(variation, "Mythic Edition"):
		edition = "Mythic Edition"
	case (edition == "Hour of Devastation" || edition == "Amonkhet") &&
		strings.Contains(variation, "Invocation"):
		edition = "Amonkhet Invocations"
	case (matcher.Contains(edition, "Timeshift") || matcher.Contains(variation, "Timeshift")) &&
		(matcher.Contains(edition, "Spiral") || matcher.Contains(variation, "Spiral")) &&
		!isMysteryList(inCard):
		if len(b.MatchInSet(inCard.Name, "TSB")) != 0 {
			edition = b.Sets["TSB"].Name
		} else if len(b.MatchInSet(inCard.Name, "TSR")) != 0 {
			edition = b.Sets["TSR"].Name
		}
	}

	// A card whose wording is nothing but the edition has nothing to add
	// to the aliasing; anything more gets a second look at the set
	// families, read over the spelling the aliasing settled on.
	if inCard.Name != "" || variation != "" {
		edition, variation = setFamilyEdition(b, inCard.Name, edition, edition, variation)
	}

	switch {
	case strings.Contains(variation, "APAC Set") || strings.Contains(variation, "Euro Set"):
		num := matcher.ExtractNumber(variation)
		if num != "" {
			variation = strings.Replace(variation, num+" ", "", 1)
		}
	case strings.HasPrefix(variation, "Junior") && strings.Contains(variation, "APAC"),
		strings.HasPrefix(variation, "Junior APAC Series") && strings.Contains(variation, "U"):
		edition = "Junior APAC Series"
	case strings.HasPrefix(variation, "Junior Super Series"),
		strings.HasPrefix(variation, "MSS Foil"),
		strings.HasPrefix(variation, "MSS #J"),
		strings.HasPrefix(variation, "MSS Promo J"),
		strings.HasPrefix(variation, "JSS #J"),
		strings.Contains(variation, "JSS Foil") && !matcher.Contains(variation, "euro"):
		edition = "Junior Super Series"
	case strings.HasPrefix(variation, "Junior Series Europe"),
		strings.HasPrefix(variation, "Junior Series E"),
		strings.HasPrefix(variation, "Junior Series #E"),
		strings.HasPrefix(variation, "Junior Series Promo E"),
		strings.HasPrefix(variation, "Junior Series Promo Foil E"),
		strings.HasPrefix(variation, "ESS Foil E"),
		strings.HasPrefix(variation, "European JrS E"),
		strings.HasPrefix(variation, "European JSS Foil E"):
		edition = "Junior Series Europe"
	case matcher.Contains(variation, "Boosterfun"):
		inCard.BeyondBaseSet = true
	}
	inCard.Edition = edition
	inCard.Variation = variation

	// Special handling since so many providers get this wrong
	switch {
	// Prevent tags from being mixed up, only take care of edition changes
	case isMysteryList(inCard):
		switch inCard.Name {
		case "Rafiq of the Many":
			edition = "Shards of Alara"
			variation = "250"
		default:
			// Decouple wrong SLX cards bundled in PLST, as long as they are not reprinted in PLST
			// In that case we trust the source has been properly tagged and will be decoupled later
			if !isReskin(inCard) && len(b.MatchInSet(inCard.Name, "SLX")) != 0 && len(b.MatchInSet(inCard.Name, "PLST")) == 0 {
				edition = b.Sets["SLX"].Name
			}
		}

	// XLN Treasure Chest
	case isBaB(inCard) && len(b.MatchInSet(inCard.Name, "PXTC")) != 0:
		edition = b.Sets["PXTC"].Name

	// BFZ Standard Series
	case isGenericAltArt(inCard) && len(b.MatchInSet(inCard.Name, "PSS1")) != 0:
		edition = b.Sets["PSS1"].Name

	// Champs and States
	case isGenericExtendedArt(inCard) && len(b.MatchInSet(inCard.Name, "PCMP")) != 0:
		edition = b.Sets["PCMP"].Name

	// Secret Lair {Ultimate,Drop}
	case isSecretLair(inCard):
		// Check if there are also FlavorNames associated to this card
		// It might happen that a non-FlavorName is requested, so check number too
		altProps, found := b.AlternateProps[inCard.Name]
		if found && len(b.MatchInSet(altProps.OriginalName, "SLD")) != 0 {
			var shouldRename bool
			cards := b.MatchInSet(altProps.OriginalName, "SLD")
			num := matcher.ExtractNumber(inCard.Variation)
			for _, card := range cards {
				if card.Number == num || (card.FaceFlavorName != "" && matcher.Contains(inCard.Variation, card.FaceFlavorName)) {
					shouldRename = true
					break
				}
			}

			if shouldRename {
				inCard.Name = altProps.OriginalName
			}
		}

		// This needs to be repeated because this could be skipped if the
		// actual name is used instead
		switch {
		case inCard.Contains("Plains") || inCard.Contains("Battlefield Forge"):
			if inCard.Contains("Unpeeled") || inCard.Contains("669") {
				inCard.Name = "Battlefield Forge"
				inCard.Variation = "669"
			} else if inCard.Contains("Peeled") || inCard.Contains("670") {
				inCard.Name = "Plains"
				inCard.Variation = "670"
			}
		case matcher.Contains(inCard.Name, "Blightsteel Colossus"), matcher.Contains(inCard.Name, "Megatron"), matcher.Contains(inCard.Name, "FAS-BOR7 Horus"),
			inCard.Contains("Blightsteel Colossus"), inCard.Contains("Megatron"), inCard.Contains("FAS-BOR7 Horus"):
			if matcher.Contains(inCard.Name, "Megatron") || inCard.Contains("Megatron") {
				variation = "1079"
			} else if matcher.Contains(inCard.Name, "FAS-BOR7 Horus") || inCard.Contains("FAS-BOR7 Horus") {
				variation = "2223"

			}
		}

	// Untagged Planeshift Alternate Art - these could be solved with the
	// Promo handling, but they are not set as such in scryfall
	case (isGenericPromo(b, inCard) || isGenericAltArt(inCard)) && len(b.MatchInSet(inCard.Name, "PLS")) == 2:
		edition = "PLS"
		variation = "Alternate Art"

	// Rename the official name to the the more commonly used name
	case inCard.Edition == "Commander Legends" && isShowcase(inCard):
		variation = "Etched"

	// Planechase deduplication
	case inCard.Contains("Planechase") && len(b.MatchInSet(inCard.Name, "DCI")) != 0 && (isRelease(inCard) || isDCIPromo(inCard) || isWPNGateway(inCard)):
		edition = b.Sets["DCI"].Name
	case inCard.Equals("Planechase") && len(b.MatchInSet(inCard.Name, "OHOP")) != 0:
		edition = b.Sets["OHOP"].Name
	case inCard.Equals("Planechase 2012") && len(b.MatchInSet(inCard.Name, "OPC2")) != 0:
		edition = b.Sets["OPC2"].Name
	case inCard.Equals("Planechase Anthology") && len(b.MatchInSet(inCard.Name, "OPCA")) != 0:
		edition = b.Sets["OPCA"].Name

	// The first Gift Pack often get folded in the main Core Set 2019 or in the
	// related Promos set, so use a lax way to detected the original expansion
	case ((matcher.Contains(inCard.Edition, "Core") && matcher.Contains(inCard.Edition, "2019")) || isGenericPromo(b, inCard)) && len(b.MatchInSet(inCard.Name, "G18")) == 1:
		edition = b.Sets["G18"].Name

	// Adjust edition for non-English sets, by the language the listing is
	// in, which Match has already read off its wording
	case (inCard.Edition == "Legends" || inCard.Edition == "The Dark") && inCard.Language == "Italian":
		edition += " Italian"
	case inCard.Edition == "Renaissance" && inCard.Language == "Italian" && len(b.MatchInSet(inCard.Name, "RIN")) != 0:
		edition = "Rinascimento"
		// This set has lots of variants, strip away any excess data
		variation = strings.ToLower(inCard.Variation)
		variation = strings.Replace(variation, "italian", "", 1)
		variation = strings.TrimSpace(variation)
	case strings.Contains(inCard.Edition, "Chronicles") && (inCard.Language == "Japanese" || inCard.Contains("FBB")):
		edition = "Chronicles Foreign Black Border"
		// This set has lots of variants, strip away any excess data
		variation = strings.ToLower(inCard.Variation)
		variation = strings.Replace(variation, "japanese", "", 1)
		variation = strings.TrimSpace(variation)
	case inCard.Edition == "Fourth Edition" && matcher.Contains(inCard.Variation, "Japanese"):
		edition = "Fourth Edition Foreign Black Border"
		// Helps with land variants
		variation = strings.ToLower(inCard.Variation)
		variation = strings.Replace(variation, "japanese", "", 1)
		variation = strings.Replace(variation, "bb", "", 1)
		variation = strings.TrimSpace(variation)

	// Separate timeshifted cards
	case inCard.Contains("Modern Horizons") &&
		(inCard.Contains("Retro Frame") || inCard.Contains("Timeshift")) &&
		(len(b.MatchInSet(inCard.Name, "H1R")) != 0 || len(b.MatchInSet(inCard.Name, "H2R")) != 0):
		if len(b.MatchInSet(inCard.Name, "H1R")) != 0 {
			edition = b.Sets["H1R"].Name
		} else if len(b.MatchInSet(inCard.Name, "H2R")) != 0 {
			edition = b.Sets["H2R"].Name
		}

	// Clash pack promos
	case (inCard.Contains("Clash") || isGenericPromo(b, inCard)) && len(b.MatchInSet(inCard.Name, "CP1")) == 1:
		edition = b.Sets["CP1"].Name
	case (inCard.Contains("Clash") || isGenericPromo(b, inCard)) && len(b.MatchInSet(inCard.Name, "CP2")) == 1:
		edition = b.Sets["CP2"].Name
	case (inCard.Contains("Clash") || isGenericPromo(b, inCard)) && len(b.MatchInSet(inCard.Name, "CP3")) == 1:
		edition = b.Sets["CP3"].Name

	// Challenger decks promos
	case (inCard.Contains("Challenger Decks") || isGenericPromo(b, inCard)) && len(b.MatchInSet(inCard.Name, "Q06")) != 0:
		edition = b.Sets["Q06"].Name

	// Open the Helvault oversized cards
	case (inCard.Contains("Oversize") || inCard.Contains("Helvault Promo") || IsPrerelease(inCard)) && len(b.MatchInSet(inCard.Name, "PHEL")) == 1:
		edition = b.Sets["PHEL"].Name
		variation = ""

	// All the oversized commander cards
	case inCard.Contains("Oversize") && !inCard.Contains("Plane") && !inCard.Contains("Phenomenon"):
		for _, tag := range []string{
			"OCM1", "PCMD", "OCMD", "OC13", "OC14", "OC15", "OC16", "OC17", "OC18", "OC19", "OC20",
		} {
			if inCard.Name == "Mayael the Anima" && !inCard.Contains("Arsenal") {
				edition = b.Sets["OC13"].Name
				break
			} else if len(b.MatchInSet(inCard.Name, tag)) == 1 {
				edition = b.Sets[tag].Name
				break
			}
		}

	// Lunar Year Promos
	case (isGenericPromo(b, inCard) || inCard.Contains("Lunar")) && len(b.MatchInSet(inCard.Name, "PL21")) == 1:
		edition = b.Sets["PL21"].Name

	// Love Your LGS 2021, often confused with WPN
	case (isWPNGateway(inCard) || isGenericPromo(b, inCard)) && inCard.Contains("Retro Frame") && len(b.MatchInSet(inCard.Name, "PLG21")) == 1:
		edition = b.Sets["PLG21"].Name

	// WPN 2021
	case inCard.Name != "Mind Stone" && isGenericPromo(b, inCard) && len(b.MatchInSet(inCard.Name, "PW21")) == 1:
		edition = b.Sets["PW21"].Name

	// Unfinity Sticker Sheets
	case inCard.Edition == "Unfinity" && len(b.MatchInSet(inCard.Name, "SUNF")) == 1:
		edition = b.Sets["SUNF"].Name

	// Move Release to Prerelease for Battlebond
	case isRelease(inCard) && strings.Contains(edition, "Battlebond") && len(b.MatchInSet(inCard.Name, "PBBD")) == 1:
		edition = b.Sets["PBBD"].Name

	// Remove edition since the cards are either in ONE or in another set, but single printed
	case inCard.Contains("Phyrexia: All") && inCard.Contains("Concept"):
		switch inCard.Name {
		default:
			edition = "ignored"
		}

	// Decouple P30A from P30H and P30T
	case inCard.Contains("30th Anniversary") && !inCard.Contains("Edition") && !inCard.Contains("Tokyo") && !inCard.Contains("Misc") && len(b.MatchInSet(inCard.Name, "P30H")) > 0:
		maybeEdition := b.Sets["P30H"].Name
		if inCard.Name == "Serra Angel" && (!inCard.Contains("History") || matcher.ExtractYear(inCard.Variation) != "") {
			maybeEdition = b.Sets["P30A"].Name
		}
		edition = maybeEdition

	// Oilslick lands may not have the bundle tag attached to them
	case isExactBasicLand(inCard.Name) && isOilSlick(inCard) && !isBundle(inCard):
		variation += " Bundle"

	// Many providers don't tag these promos correctly
	case isRelease(inCard) && len(b.MatchInSet(inCard.Name, "PBBD")) == 1:
		edition = b.Sets["PBBD"].Name
		variation = "Prerelease"

	// Single card mismatches
	default:
		// A card printed as a Japanese media insert and reprinted in English
		// years later is one shape, not a dozen special cases: the original
		// is named by its own number or by its language, and the reprint by
		// neither. Written here rather than as a case apiece so that a new
		// volume is a line in the table, and so that the names that already
		// have a case of their own for something else do not need merging.
		insert, isMediaInsert := mediaInsertOriginals[inCard.Name]
		if isMediaInsert && isIDWMagazineBook(inCard) {
			edition = "Media and Collaboration Promos"
			switch {
			// Duress alone was also a comic insert, and a listing naming
			// IDW means the comic rather than the manga.
			case insert.idw != "" && inCard.Contains("IDW"):
				edition = "IDW Comics Inserts"
				variation = insert.idw
			case (insert.marker != "" && matcher.ExtractNumber(variation) == insert.marker) ||
				IsJPN(inCard) || inCard.Language == "Japanese":
				variation = insert.number
			default:
				// The reprint is the only English printing under this
				// edition, so nothing more needs saying about it.
				variation = ""
			}
		}

		switch inCard.Name {
		case "Rhox":
			if isGenericAltArt(inCard) || isGenericPromo(b, inCard) {
				edition = "Starter 2000"
			}
		case "Balduvian Horde":
			if isJudge(inCard) || isGenericPromo(b, inCard) || isDCIPromo(inCard) {
				edition = "World Championship Promos"
			}
		case "Disenchant":
			if isArena(inCard) && inCard.Foil {
				edition = "Friday Night Magic 2003"
			}
		case "Nalathni Dragon":
			edition = "Dragon Con"
			variation = ""
		case "Ass Whuppin'",
			"Rukh Egg",
			"Scholar of the Lost Trove":
			if IsPrerelease(inCard) {
				variation = "Release"
				edition = "Release Events"
			}
		case "Reya Dawnbringer":
			if isRelease(inCard) {
				edition = "Tenth Edition Promos"
			}
		case "Ajani Vengeant":
			if isRelease(inCard) {
				variation = "Prerelease"
			}
		case "Tamiyo's Journal":
			if (inCard.Variation == "" || matcher.ExtractNumber(inCard.Variation) == "265") && inCard.Foil {
				variation = "Foil"
			}
		case "Underworld Dreams":
			if isDCIPromo(inCard) || isArena(inCard) || inCard.Contains("2HG") || inCard.Contains("Two-Headed Giant") {
				edition = "Two-Headed Giant Tournament"
			}
		case "Jace Beleren":
			if isDCIPromo(inCard) {
				edition = "Miscellaneous Book Promos"
			}
		case "Serra Angel":
			if isDCIPromo(inCard) || isBaB(inCard) {
				edition = "Wizards of the Coast Online Store"
			}
		case "Incinerate", "Counterspell":
			if isDCIPromo(inCard) || (inCard.Contains("Legend") && (inCard.Contains("Promo") || inCard.Contains("Member"))) {
				edition = "DCI Legend Membership"
			}
		case "Faerie Conclave", "Treetop Village":
			if isWPNGateway(inCard) || inCard.Contains("Summer") {
				edition = "Tenth Edition Promos"
			}
		case "Kamahl, Pit Fighter", "Char":
			if isDCIPromo(inCard) || inCard.Contains("15th Anniversary") || isGenericPromo(b, inCard) {
				edition = "15th Anniversary Cards"
			}
		case "Fling":
			if (isDCIPromo(inCard) || isWPNGateway(inCard)) && matcher.ExtractNumber(inCard.Variation) == "" {
				edition = "DCI Promos"
				if isDCIPromo(inCard) {
					variation = "50"
				} else if isWPNGateway(inCard) {
					variation = "69"
				}
			}
		case "Sylvan Ranger":
			if (isDCIPromo(inCard) || isWPNGateway(inCard)) && matcher.ExtractNumber(inCard.Variation) == "" {
				edition = "DCI Promos"
				if isDCIPromo(inCard) {
					variation = "51"
				} else if isWPNGateway(inCard) {
					variation = "70"
				}
			}
		case "Naya Sojourners":
			if isGenericPromo(b, inCard) {
				edition = "DCI Promos"
			}
		case "Hall of Triumph":
			if isGenericPromo(b, inCard) {
				edition = "Journey into Nyx Promos"
			}
		case "Reliquary Tower":
			if inCard.Contains("League") {
				edition = "Core Set 2019 Promos"
			} else if inCard.Contains("Bring a Friend") {
				edition = "Love Your LGS 2020"
			}
		case "Bolas's Citadel":
			if isGenericPromo(b, inCard) {
				edition = "War of the Spark Promos"
			}
		case "Llanowar Elves":
			if isGenericPromo(b, inCard) && !isWPNGateway(inCard) && !isMagicFest(inCard) {
				edition = "Dominaria Promos"
			}
		case "Evolving Wilds":
			if isGenericPromo(b, inCard) {
				edition = "Rivals of Ixalan Promos"
			}
		case "Teferi, Master of Time":
			num := matcher.ExtractNumber(variation)
			_, err := strconv.Atoi(num)
			if err == nil {
				if IsPrerelease(inCard) {
					variation = num + "s"
				} else if IsPromoPack(inCard) {
					variation = num + "p"
				}
			}
			if num == "" {
				if IsPrerelease(inCard) {
					variation = "75s"
				} else if IsPromoPack(inCard) {
					variation = "75p"
				} else if isBorderless(inCard) {
					variation = "281"
				} else if isShowcase(inCard) {
					variation = "290"
				} else {
					variation = "75"
				}
			}
			if strings.HasSuffix(variation, "s") || strings.HasSuffix(variation, "p") {
				edition = "Core Set 2021 Promos"
			}
		case "Mind Stone":
			switch edition {
			// Skip the check if this card already has the right edition
			case "DCI Promos",
				"Wizards Play Network 2021":
			default:
				if isWPNGateway(inCard) || inCard.Contains("Bring a Friend") {
					edition = "Wizards Play Network 2021"
					if inCard.Contains("Gateway") {
						edition = "DCI Promos"
					}
				}
			}
		case "Runo Stromkirk", "Runo Stromkirk // Krothuss, Lord of the Deep":
			if isShowcase(inCard) || matcher.Contains(inCard.Variation, "Eternal") {
				num := matcher.ExtractNumber(inCard.Variation)
				if num == "" {
					if matcher.Contains(inCard.Variation, "Eternal") {
						variation = "327"
					} else if isShowcase(inCard) {
						variation = "316"
					}
				}
			}
		case "Diabolic Tutor":
			if isIDWMagazineBook(inCard) {
				edition = "Secret Lair Drop"
			}
		case "Magister of Worth":
			if isBaB(inCard) {
				variation = "Launch"
			}
		case "Hangarback Walker":
			if isReskin(inCard) || isGenericPromo(b, inCard) || strings.Contains(inCard.Edition, "LGS") {
				edition = "Love Your LGS 2020"
			}
		// Sometimes these cards are not marked as prerelease because they are showcase
		case "Goro-Goro and Satoru", "Katilda and Lier", "Slimefoot and Squee":
			if isShowcase(inCard) && !IsPrerelease(inCard) {
				variation += " Prerelease"
			}
		// HOC reprints these borderless with no Prerelease tag of its
		// own, so skip the rewrite on that edition.
		case "Delighted Halfling",
			"Lobelia Sackville-Baggins",
			"Frodo Baggins",
			"Bilbo, Retired Burglar",
			"Gandalf, Friend of the Shire",
			"Wizard's Rockets":
			hoc, hocErr := b.GetSet("HOC")
			if isBorderless(inCard) && !IsPrerelease(inCard) &&
				!(hocErr == nil && edition == hoc.Name) {
				variation += " Prerelease"
			}
		case "Arcbound Ravager":
			if inCard.Contains("Qualifiers") || inCard.Contains("WMCQ") {
				edition = "Pro Tour Promos"
			}
		case "Hornet Queen":
			if inCard.Contains("Specimen") {
				variation = strings.Replace(variation, "73", "", -1)
			}
		default:
			// Attempt a best effort match for known promotional tags if card or edition
			// wasn't found in previous steps
			if isGenericPromo(b, inCard) {
				b.Log("Precise matching for promo failed, attempting best effort")
				inCard.PromoWildcard = true
			}
		}
	}
	inCard.Edition = edition
	inCard.Variation = variation

	// Adjust incorrect numbers sometimes used for Etched
	num := matcher.ExtractNumber(inCard.Variation)
	if num != "" && strings.HasSuffix(num, "e") && b.HasEtchedPrinting(inCard.Name, inCard.Edition) {
		fixedNum := strings.TrimSuffix(num, "e")
		variation = strings.Replace(variation, num, fixedNum, -1)
		if !matcher.Contains(variation, "Etched") {
			variation += " Etched"
		}
	}
	inCard.Variation = variation
}

// aliasEdition spells an edition string toward a set name and reports
// whether decorations came off - the one fact of the aliasing the card
// fixup needs back, since a trimmed edition with nothing else to say asks
// for the beyond-base-set search.
func aliasEdition(b *matcher.Backend, edition string) (string, bool) {
	edition = tableEdition(b, edition)
	// the set-family checks read the spelling the box stage has not
	// trimmed yet
	cardEdition := edition

	edition, trimmed := boxSetEdition(edition)
	edition, _ = setFamilyEdition(b, "", edition, cardEdition, "")

	// The name-keyed fixups past this point go quiet on a bare edition,
	// except these; the empty cases shadow the rules below them exactly as
	// their name-keyed counterparts do on the card path.
	switch {
	case saysMysteryList(edition, ""):
	case matcher.Contains(edition, "Secret Lair") || strings.Contains(edition, "SLD"):
	case strings.Contains(edition, "Chronicles") &&
		(matcher.Contains(edition, "Japanese") || matcher.Contains(edition, "FBB")):
		edition = "Chronicles Foreign Black Border"
	case matcher.Contains(edition, "Oversize") && !matcher.Contains(edition, "Plane") &&
		!matcher.Contains(edition, "Phenomenon"):
	case matcher.Contains(edition, "Phyrexia: All") && matcher.Contains(edition, "Concept"):
		edition = "ignored"
	}

	return edition, trimmed
}

// AliasEdition spells an edition string toward a set name using the string
// alone. See matcher.GameRules.
func (Rules) AliasEdition(b *matcher.Backend, edition string) string {
	edition, _ = aliasEdition(b, edition)
	return edition
}

// namesTokenSetParent reports whether an edition names the set a token sheet
// was printed beside. The sheet is filed under its own set - "Hour of
// Devastation Tokens" - but a storefront selling one of its tokens says
// "Hour of Devastation", meaning the set the token came with rather than the
// sheet nobody prints a name on. Both addresses reach the same printings.
//
// A set may have printed more than one sheet - a promotional or a language
// specific one beside the one it calls its own - and then its name alone no
// longer tells them apart. It names one of them in tokenSetCode, so that one
// answers, and the others answer only for the cards the named sheet lacks.
func namesTokenSetParent(b *matcher.Backend, set *matcher.Set, edition string, editions []string) bool {
	if set.Type != "token" || set.ParentCode == "" {
		return false
	}
	parent, found := b.Sets[set.ParentCode]
	if !found || !matcher.Equals(edition, parent.Name) {
		return false
	}
	if parent.TokenSetCode == set.Code {
		return true
	}
	return !slices.Contains(editions, parent.TokenSetCode)
}

// namesOversizedSetParent reports whether an edition names the set an
// all-oversized sibling was printed beside - the same address its
// token-sheet sibling answers to in namesTokenSetParent. A listing that
// also says oversized (in either field, checked by the caller) reaches
// this sibling rather than the token sheet filed under the same name.
func namesOversizedSetParent(b *matcher.Backend, set *matcher.Set, edition string) bool {
	if set.ParentCode == "" {
		return false
	}
	parent, found := b.Sets[set.ParentCode]
	return found && matcher.Equals(edition, parent.Name)
}

// setHoldsOversized reports whether the set holds an oversized printing of
// the card, at the collector number when one is given. The number is compared
// against the printing's own and against the one its decorations were
// stripped from, because a storefront writes 4 for a card the set numbers 4★.
func setHoldsOversized(set *matcher.Set, name, number string) bool {
	for i := range set.Cards {
		card := &set.Cards[i]
		if !card.IsOversized || !matcher.Equals(card.Name, name) {
			continue
		}
		if number == "" || card.Number == number || card.PlainNumber == number {
			return true
		}
	}
	return false
}

// FilterPrintings narrows the sets a card could have come from. See
// matcher.GameRules.
func (Rules) FilterPrintings(b *matcher.Backend, inCard *matcher.InputCard, editions []string) (printings []string) {
	// A playtest card the Unknown Event shares with another set competes
	// only when the listing names the event; what is left of a lone
	// printing is as good as the unfiltered one core would have kept.
	if !inCard.Contains("Unknown Event") && slices.Contains(editions, "UNK") && len(editions) > 1 {
		editions = slices.DeleteFunc(slices.Clone(editions), func(code string) bool { return code == "UNK" })
		if len(editions) == 1 {
			return editions
		}
	}

	maybeYear := matcher.ExtractYear(inCard.Variation)
	if maybeYear == "" {
		maybeYear = matcher.ExtractYear(inCard.Edition)
	}

	// The collector number of an oversized listing is worth using only when
	// a set answers to it: the catalog writes the number of the ordinary
	// printing as often as the oversized one's, and a number nothing carries
	// would narrow every candidate away rather than pick one.
	var oversizedNumber string
	if inCard.Contains("Oversize") {
		number := matcher.ExtractNumber(inCard.Variation)
		for _, setCode := range editions {
			set, found := b.Sets[setCode]
			if found && number != "" && setHoldsOversized(set, inCard.Name, number) {
				oversizedNumber = number
				break
			}
		}
	}

	for _, setCode := range editions {
		set, found := b.Sets[setCode]
		if !found {
			continue
		}

		setDate := set.ReleaseDateTime

		switch {
		// If the edition matches, use it as is
		// except for two "catch all" sometimes overlapping sets
		case matcher.Equals(inCard.Edition, set.Name) && !isMysteryList(inCard) && !isSecretLair(inCard):
			// pass-through

		// The set a token sheet came with names it as surely as the sheet
		// does, and only tokens are filed there, so nothing else answers -
		// unless the listing says oversized, which a token sheet never is;
		// deferring to the oversize case below lets a same-named oversized
		// sibling (a dungeon card's parent set can hold both) win instead of
		// the sheet by default.
		case namesTokenSetParent(b, set, inCard.Edition, editions) && !inCard.Contains("Oversize"):
			// pass-through

		// An edition that says oversize rather than naming a set is the
		// catalog's shelf for them, one group standing for a dozen sets at
		// once, so it narrows to the sets that printed the card oversized
		// and the rest of the input picks among those. Reading only the
		// edition here, not the variation too, is deliberate: a variation
		// saying oversized beside a set we do not carry - the championship
		// prizes - would otherwise be answered with whichever unrelated set
		// printed one oversized, anywhere in the catalog.
		//
		// The one variation-only case let in is a listing whose edition
		// names *this* set's own parent - the same address its token-sheet
		// sibling answers to - since that ties the "oversized" back to a
		// sibling this printing actually has, rather than to the whole
		// shelf.
		case matcher.Contains(inCard.Edition, "Oversize") ||
			(inCard.Contains("Oversize") && namesOversizedSetParent(b, set, inCard.Edition)):
			if !setHoldsOversized(set, inCard.Name, oversizedNumber) {
				continue
			}

		case IsPrerelease(inCard):
			switch set.Name {
			// Sets that could be marked as prerelease, but they aren't really
			case "M15 Prerelease Challenge",
				"Open the Helvault":
			// Sets that have prerelease cards mixed in
			case "Innistrad: Double Feature",
				"March of the Machine Commander",
				"The Lord of the Rings: Tales of Middle-earth":
				prerelease := func(card matcher.Card) bool { return card.HasPromoType(PromoTypePrerelease) }
				if !slices.ContainsFunc(b.MatchInSet(inCard.Name, setCode), prerelease) {
					continue
				}
			case "Duels of the Planeswalkers 2012 Promos",
				"Grand Prix Promos",
				"Pro Tour Promos",
				"World Championship Promos":
				continue
			default:
				if strings.HasPrefix(set.Name, "30th Anniversary") {
					continue
				}
				if !strings.HasSuffix(set.Name, "Promos") {
					continue
				}
			}

		case IsPromoPack(inCard):
			switch set.Name {
			case "Dragon's Maze Promos", // due to Plains
				"Grand Prix Promos":
				continue
			case "M20 Promo Packs":
			default:
				switch {
				case strings.HasPrefix(set.Name, "30th Anniversary"):
					continue
				case strings.HasSuffix(set.Name, "Promos"):
				case setDate.After(PromosForEverybodyYay) && (set.Type == "expansion" || set.Type == "core"):
					promo := func(card matcher.Card) bool {
						return card.HasPromoType(PromoTypePromoPack) || card.HasPromoType(PromoTypePlayPromo)
					}
					if !slices.ContainsFunc(b.MatchInSet(inCard.Name, setCode), promo) {
						continue
					}
				default:
					continue
				}
			}

		case isRelease(inCard):
			release := func(card matcher.Card) bool {
				return card.HasPromoType(PromoTypeRelease) ||
					card.HasPromoType(PromoTypeDraftWeekend) ||
					card.HasPromoType(PromoTypeWPN)
			}
			if !slices.ContainsFunc(b.MatchInSet(inCard.Name, setCode), release) {
				continue
			}

		case isBaB(inCard):
			buyABox := func(card matcher.Card) bool { return card.HasPromoType(PromoTypeBuyABox) }
			if !slices.ContainsFunc(b.MatchInSet(inCard.Name, setCode), buyABox) {
				continue
			}

		case isBundle(inCard):
			bundle := func(card matcher.Card) bool { return card.HasPromoType(PromoTypeBundle) }
			if !slices.ContainsFunc(b.MatchInSet(inCard.Name, setCode), bundle) {
				continue
			}

		case isFNM(inCard):
			switch {
			case strings.HasPrefix(set.Name, "Friday Night Magic "+maybeYear):
			case set.Name == "Magic × Duel Masters Promos":
			case strings.HasSuffix(set.Name, "Promos"):
				fnm := func(card matcher.Card) bool { return card.HasPromoType(PromoTypeFNM) }
				if !slices.ContainsFunc(b.MatchInSet(inCard.Name, setCode), fnm) {
					continue
				}
				inCard.Variation = "FNM Promo"
			default:
				continue
			}

		case isJudge(inCard):
			switch {
			case strings.HasPrefix(set.Name, "Judge Gift Cards "+maybeYear):
			default:
				continue
			}

		case isArena(inCard):
			maybeYear = arenaYear(inCard, maybeYear)
			switch {
			case set.Name == "DCI Legend Membership":
			case strings.HasPrefix(set.Name, "Arena League "+maybeYear):
			default:
				continue
			}

		// This needs to be above any possible printing type below
		// Both kinds need to be checked in the same place as there is
		// a lot of overlap in the product and naming across stores
		case isMysteryList(inCard) || isSecretLair(inCard):
			// Mystery Booster Commander Edition is named in full, so that
			// wording keeps it alone, and nothing else reaches it. Revisit
			// once it ships: MB2's foil-only Oracle of the Alpha has an MBC
			// twin told apart by finish alone, unloaded while it has none.
			if inCard.Contains("Mystery Booster Commander") != (set.Code == "MBC") {
				continue
			}
			noSymbol := inCard.Contains("No") && inCard.Contains("Symbol")
			second := noSymbol || strings.Contains(inCard.Variation, "V.2") ||
				(inCard.Contains("Playtest") && inCard.Contains("2021"))
			switch set.Code {
			case "MBC":
			case "CMB1":
				if second {
					continue
				}
			case "CMB2":
				if !second {
					continue
				}
			case "MB2":
				if !inCard.Contains("Mystery Booster 2") {
					continue
				}
			case "PLST":
				// Check if there is an exact match in plain SLD
				num := matcher.ExtractNumber(inCard.Variation)
				if len(b.MatchInSetNumber(inCard.Name, "SLD", num)) != 0 {
					// If there is a match, make sure there are no other cards in PLST with the same number
					shouldNotContinue := false
					cardsWithSameName := b.MatchInSet(inCard.Name, "PLST")
					for _, altCard := range cardsWithSameName {
						var altNum string
						altNums := strings.Split(altCard.Number, "-")
						if len(altNums) > 1 {
							altNum = altNums[1]
						}
						if altNum == num {
							shouldNotContinue = true
							break
						}
					}
					if !shouldNotContinue {
						continue
					}
				}
				if isSecretLair(inCard) && !namesSLDDeck(b, inCard) {
					continue
				}
			case "ULST":
			case "SLX", "SLU", "SLC", "SLP":
				// If these have no strict matches AND are not properly tagged, skip them
				number := matcher.ExtractNumber(inCard.Variation)
				tagged := hasSecretLairTag(b, inCard, set.Code)
				if len(b.MatchInSetNumber(inCard.Name, set.Code, number)) == 0 && !tagged {
					continue
				}
				// An untagged number the SLD set holds for this name is SLD's
				if !tagged && len(b.MatchInSetNumber(inCard.Name, "SLD", number)) != 0 {
					continue
				}
			case "SLD":
				skip := false
				number := matcher.ExtractNumber(inCard.Variation)
				sldNumber := len(b.MatchInSetNumber(inCard.Name, "SLD", number)) != 0

				// Iterate on all possible combinations of tags, and skip if a
				// condition is unmet
				for _, code := range []string{"SLU", "SLX", "SLC", "SLP"} {
					if len(b.MatchInSet(inCard.Name, code)) == 0 || !hasSecretLairTag(b, inCard, code) {
						continue
					}
					// A number the SLD set holds for this name is SLD's,
					// unless a tagged set holds it too
					if sldNumber && len(b.MatchInSetNumber(inCard.Name, code, number)) == 0 {
						continue
					}
					skip = true
					break
				}

				// No PLST in SLD
				if isMysteryList(inCard) {
					skip = true
				}

				// Check that the card reported is not coming from a SLD Deck
				// or if it does, make sure it is actually from SLD. Use
				// ExtractNumberAny so Secret Lair collector numbers above the
				// year cap (e.g. 2406) aren't dropped and misrouted to PLST,
				// mirroring the SLD number check further below.
				if len(b.MatchInSetNumber(inCard.Name, "SLD", matcher.ExtractNumberAny(inCard.Variation))) == 0 && len(b.MatchInSet(inCard.Name, "PLST")) > 0 && namesSLDDeck(b, inCard) {
					skip = true
				}

				if skip {
					continue
				}
			default:
				continue
			}

		// Some providers use "Textless" for MF cards
		case isRewards(inCard) && !isMagicFest(inCard):
			maybeYear = playerRewardsYear(inCard, maybeYear)
			switch {
			case strings.HasPrefix(set.Name, "Magic Player Rewards "+maybeYear):
			default:
				continue
			}

		case isWPNGateway(inCard):
			switch set.Name {
			case "DCI Promos":
			case "Innistrad: Crimson Vow",
				"The Lost Caverns of Ixalan":
				wpn := func(card matcher.Card) bool { return card.HasPromoType(PromoTypeWPN) }
				if !slices.ContainsFunc(b.MatchInSet(inCard.Name, set.Code), wpn) {
					continue
				}
			default:
				switch {
				case strings.HasPrefix(set.Name, "Wizards Play Network "+maybeYear):
				case strings.HasPrefix(set.Name, "Love Your LGS "+maybeYear):
				default:
					continue
				}
			}

		case isIDWMagazineBook(inCard):
			// No Media cards in these sets
			switch set.Code {
			case "P30A", "P30H", "P30M":
				continue
			case "PGPX", "PWOR", "PPRO", "PWCS":
				continue
			}

			switch {
			case !IsJPN(inCard) && (set.Name == "IDW Comics Inserts" || set.Name == "HarperPrism Book Promos"):
			case !IsJPN(inCard) && strings.HasPrefix(set.Name, "Duels of the Planeswalkers "+maybeYear):
			default:
				switch set.Code {
				case "PURL", "JP1", "DLGM":
				case "PDOM":
					// This set contains both FNM and Media cards
					skip := false
					foundCards := b.MatchInSet(inCard.Name, set.Code)
					for _, card := range foundCards {
						if card.HasPromoType(PromoTypeFNM) {
							skip = true
							break
						}
					}
					if skip {
						continue
					}
				case "P9ED":
					if IsJPN(inCard) {
						continue
					}
				case "PMEI":
					// This is the only card present in IDW and Media Inserts
					// so make sure it is properly tagged
					if inCard.Name == "Duress" && !IsJPN(inCard) {
						continue
					}
					// This could be mixed in P9ED Russian
					if inCard.Name == "Shivan Dragon" && !IsJPN(inCard) {
						continue
					}
				default:
					if !strings.HasSuffix(set.Name, "Promos") {
						continue
					}
				}
			}

		case isResale(inCard):
			switch set.Code {
			case "DCI", "P30A", "P30H", "P30M":
				continue
			case "PDOM":
				// This might conflict with Llanowar Elves
				continue
			case "PMEI", "PLTC":
				// These sets may actually contain Resale cards
			default:
				if !strings.HasSuffix(set.Name, "Promos") {
					continue
				}
			}

		case inCard.Contains("Hero") && inCard.Contains("Path"):
			switch set.Name {
			case "Born of the Gods Hero's Path",
				"Journey into Nyx Hero's Path",
				"Journey into Nyx Promos",
				"Theros Hero's Path",
				"Defat a God",
				"Face the Hydra",
				"Battle the Horde":
			default:
				continue
			}

		case inCard.Contains("Convention"):
			switch set.Name {
			case "URL/Convention Promos":
			default:
				if strings.HasPrefix(set.Name, "30th Anniversary") {
					continue
				}
				if !strings.HasSuffix(set.Name, "Promos") {
					continue
				}
			}

		case inCard.Contains("Premier Play") || inCard.Contains("Regional Championship"):
			switch {
			case set.Name == "Pro Tour Promos":
				// Special case for a card that could be in PR23
				if inCard.Name == "Snapcaster Mage" && !inCard.Contains("Pro Tour") {
					continue
				}
			case strings.HasPrefix(set.Name, "Regional Championship Qualifiers "+maybeYear):
			default:
				continue
			}

		case inCard.Contains("Game Day") || inCard.Contains("Store Championship"):
			switch set.Code {
			case "SCH":
			case "LTR":
			case "PEWK":
			default:
				switch {
				case strings.HasSuffix(set.Name, "Promos"):
					event := func(card matcher.Card) bool {
						return card.HasPromoType(PromoTypeStoreChampionship) ||
							card.HasPromoType(PromoTypeGameDay)
					}
					if !slices.ContainsFunc(b.MatchInSet(inCard.Name, set.Code), event) {
						continue
					}
				case strings.HasPrefix(set.Name, "Wizards Play Network "+maybeYear):
				default:
					continue
				}
			}

		case isWorldChamp(inCard):
			switch {
			case (maybeYear == "1996" || maybeYear == "") && set.Name == "Pro Tour Collector Set":
			case maybeYear != "" && strings.HasPrefix(set.Name, "World Championship Decks "+maybeYear):
			case maybeYear == "" && strings.HasPrefix(set.Name, "World Championship Decks"):
				num, _ := parseWorldChampPrefix(inCard.Variation)
				foundCards := b.MatchInSet(inCard.Name, set.Code)
				numbered := func(card matcher.Card) bool { return card.Number == num }
				if num != "" && len(foundCards) != 1 && !slices.ContainsFunc(foundCards, numbered) {
					continue
				}
			default:
				continue
			}

		case isMagicFest(inCard):
			// Some providers use GP2018 instead of MF2019
			if maybeYear == "2018" && isExactBasicLand(inCard.Name) {
				maybeYear = "2019"
			}
			switch {
			case strings.HasPrefix(set.Name, "MagicFest "+maybeYear):
				if len(b.MatchInSet(inCard.Name, "SLP")) > 0 && !inCard.Contains("Fest") && !inCard.Contains("MagicCon") {
					continue
				}
			case set.Code == "PLG21":
			case set.Code == "PEWK":
			case set.Code == "SLP":
				// If the 'Secret' tag is missing, confirm that this could not be found in other
				// MagicFest sets
				if (len(b.MatchInSet(inCard.Name, "PF19")) > 0 ||
					len(b.MatchInSet(inCard.Name, "PF25")) > 0) && !inCard.Contains("Secret") {
					continue
				}
			default:
				continue
			}

		case isSDCC(inCard):
			switch {
			case strings.HasPrefix(set.Name, "San Diego Comic-Con "+maybeYear):
			default:
				continue
			}

		case isDuelsOfThePW(inCard):
			switch {
			case strings.HasPrefix(set.Name, "Duels of the Planeswalkers"):
			default:
				continue
			}

		// DDA with number or deck variant specified in the variation
		case isDuelDecksAnthology(inCard):
			switch {
			case strings.HasPrefix(set.Name, "Duel Decks Anthology"):
				found := false
				// Look if the variation or the edition contain part of set name
				for _, location := range []string{inCard.Variation, inCard.Edition} {
					if !strings.Contains(strings.ToLower(location), "vs") {
						continue
					}
					fields := strings.FieldsSeq(location)
					for field := range fields {
						// Skip elements that are too short to be representative
						if len(field) < 4 {
							continue
						}
						if matcher.Contains(set.Name, field) {
							found = true
							break
						}
					}
				}
				// Do number check only if well known elements are missing
				wellKnownTags := inCard.Contains("Divine") || inCard.Contains("Garruk") ||
					inCard.Contains("Chandra") || inCard.Contains("Goblins")
				if !found && !wellKnownTags {
					num := matcher.ExtractNumber(inCard.Variation)
					if num != "" {
						foundCards := b.MatchInSet(inCard.Name, setCode)
						for _, card := range foundCards {
							if card.Number == num {
								found = true
								break
							}
						}
					}
				}
				if wellKnownTags && !found {
					continue
				}
			default:
				continue
			}

		case isDuelDecks(inCard):
			variant := duelDecksVariant(inCard)
			switch {
			case strings.HasPrefix(set.Name, "Duel Decks") &&
				!strings.Contains(set.Name, "Anthology"):
				if !matcher.Contains(set.Name, variant) {
					continue
				}
			default:
				continue
			}

		case strings.Contains(inCard.Variation, "Champs") ||
			strings.Contains(inCard.Variation, "States"):
			switch set.Name {
			case "Champs and States":
			case "Grand Prix Promos":
				continue
			default:
				if strings.HasPrefix(set.Name, "30th Anniversary") {
					continue
				}
				if !strings.HasSuffix(set.Name, "Promos") {
					continue
				}
			}

		case isPremiereShop(inCard):
			if maybeYear == "" {
				guilds := append(grnGuilds, arnGuilds...)
				for _, guild := range guilds {
					if strings.Contains(inCard.Variation, guild) {
						maybeYear = "2005"
						break
					}
				}
				if maybeYear == "" && strings.HasSuffix(inCard.Variation, "Cycle") {
					maybeYear = map[string]string{
						"Time Spiral Cycle":       "2006",
						"Lorwyn Cycle":            "2007",
						"Shards of Alara Cycle":   "2008",
						"Zendikar Cycle":          "2009",
						"Scars of Mirrodin Cycle": "2010",
						"Innistrad Cycle":         "2011",
					}[inCard.Variation]
				}
			}
			switch {
			case strings.HasPrefix(set.Name, "Magic Premiere Shop "+maybeYear):
			default:
				continue
			}

		case isExactBasicLand(inCard.Name) && strings.Contains(inCard.Variation, "APAC"):
			if set.Name != "Asia Pacific Land Program" {
				continue
			}

		case isExactBasicLand(inCard.Name) && matcher.Contains(inCard.Variation, "EURO"):
			if set.Name != "European Land Program" {
				continue
			}

		case strings.Contains(inCard.Edition, "Core Set") ||
			strings.Contains(inCard.Edition, "Core 20") ||
			strings.Contains(inCard.Edition, "Magic 20"):
			switch {
			case !isGenericPromo(b, inCard) && strings.HasSuffix(set.Name, "Promos"):
				continue
			case strings.HasPrefix(set.Name, "Core Set "+maybeYear):
			case strings.HasPrefix(set.Name, "Magic "+maybeYear):
			default:
				continue
			}

		case inCard.Contains("Grand Prix"):
			switch {
			case strings.HasPrefix(set.Name, "MagicFest "+maybeYear):
			default:
				if set.Name != "Grand Prix Promos" {
					continue
				}
			}

		case inCard.Contains("Lunar New Year") || inCard.Contains("Year of the "):
			switch set.Code {
			case "PLNY":
			default:
				if !strings.HasPrefix(set.Name, "Year of the ") {
					continue
				}
				// Each year is a set of its own, so a stated year picks it
				if maybeYear != "" && !strings.Contains(set.Name, maybeYear) {
					continue
				}
			}

		case isThickDisplay(inCard):
			switch set.Code {
			// The sets with thick display cards separate from the main commander set
			case "OC21", "OAFC", "OMIC", "OVOC":
			// SLD may contain DFC with thick display
			case "SLD":
			default:
				// Skip any set before this date if not from the sets above
				if setDate.Before(SeparateFinishCollectorNumberDate) {
					continue
				}
			}

		case inCard.Contains("Bring-A-Friend") ||
			inCard.Contains("Love Your LGS") ||
			inCard.Contains("Love Your Local Game Store") ||
			inCard.Contains("Welcome Back") ||
			inCard.Contains("Open House") ||
			inCard.Contains("LGS Promo"):
			switch {
			case strings.HasPrefix(set.Name, "Love Your LGS "+maybeYear):
			case strings.HasPrefix(set.Name, "Wizards Play Network "+maybeYear):
			case set.Name == "Grand Prix Promos":
				continue
			default:
				if strings.HasPrefix(set.Name, "30th Anniversary") ||
					strings.HasPrefix(set.Name, "Duels of the Planeswalkers") {
					continue
				}
				if !strings.HasSuffix(set.Name, "Promos") {
					continue
				}
			}

		case inCard.Contains("30th"):
			switch set.Code {
			case "P30A", "P30H", "P30M":
			case "P30T":
				if isRetro(inCard) || !IsJPN(inCard) {
					continue
				}
			default:
				continue
			}

		case inCard.Contains("Planeswalker") && inCard.Contains("Promos"):
			switch set.Code {
			case "PWCS":
			default:
				if !strings.HasSuffix(set.Name, "Promos") {
					continue
				}
			}

		// For all the promos with "Extended Art" which refer to the full art promo
		case isExtendedArt(inCard) && !inCard.Contains("Game Day"):
			if setDate.Before(PromosForEverybodyYay) {
				switch set.Code {
				case "DCI":
				default:
					continue
				}
			}

		case inCard.Contains("Marvel Legends"):
			switch set.Code {
			case "LMAR":
			default:
				continue
			}

		case inCard.Contains("Eternal Weekend"):
			switch set.Code {
			case "PEWK":
			default:
				continue
			}

		// Last resort, if this is set on the input card, and there were
		// no better descriptors earlier, try looking at the set type
		case inCard.PromoWildcard:
			switch set.Type {
			case "promo":
				// Skip common promos, they are usually correctly listed
				if strings.HasPrefix(set.Name, "Judge Gift") ||
					strings.HasPrefix(set.Name, "30th Anniversary") {
					continue
				}
				skip := false
				foundCards := b.MatchInSet(inCard.Name, setCode)
				// It is required to set a proper tag to parse non-English
				// cards or well-known promos
				for _, card := range foundCards {
					if card.Language == LanguageJapanese {
						skip = true
						break
					}
				}

				if skip {
					continue
				}
			case "starter":
				if !strings.HasSuffix(set.Name, "Clash Pack") {
					continue
				}
			case "expansion", "core", "masters", "draft_innovation":
				// Skip boosterfun because they are inherently non-promo
				promo := func(card matcher.Card) bool { return card.IsPromo && !card.HasPromoType(PromoTypeBoosterfun) }
				if !slices.ContainsFunc(b.MatchInSet(inCard.Name, setCode), promo) {
					continue
				}
			case "box":
				// Only keep the planeswalkers from SLD for this category
				planeswalker := func(card matcher.Card) bool { return slices.Contains(card.Types, "Planeswalker") }
				if setCode != "SLD" || !slices.ContainsFunc(b.MatchInSet(inCard.Name, setCode), planeswalker) {
					continue
				}
			case "funny":
				if set.Name != "HasCon 2017" {
					continue
				}
			default:
				continue
			}

		// Tokens need correct set names or special handling earlier
		case b.NameIsToken(inCard.Name):
			if !matcher.Equals(inCard.Edition, set.Name) {
				continue
			}
		}

		printings = append(printings, setCode)
	}

	return
}

// missingPromoTag names the promo types that take the longest to appear
// upstream. An input tagged with one is unsupported until the resolved card
// carries the tag too.
func missingPromoTag(b *matcher.Backend, inCard *matcher.InputCard, co *matcher.CardObject) bool {
	return (IsPrerelease(inCard) && !co.HasPromoType(PromoTypePrerelease)) ||
		(IsPromoPack(inCard) && !co.HasPromoType(PromoTypePromoPack)) ||
		(isSerialized(inCard) && !co.HasPromoType(PromoTypeSerialized))
}

// PlainNumber implements matcher.GameRules.
func (Rules) PlainNumber(number string) string {
	return plainNumber(number)
}

// sameSet reports whether every candidate is filed in one and the same set.
func sameSet(cards []matcher.Card) bool {
	for _, card := range cards[1:] {
		if card.SetCode != cards[0].SetCode {
			return false
		}
	}
	return true
}

// phyrexianPrinting narrows a listing naming the Phyrexian language to the
// printings in it, and leaves the candidates alone when there are none. The
// stages after it would veto such a printing for its flavor name, which is
// only the card's own in Phyrexian script, or for a showcase frame and a
// dual finish the listing never had to spell out.
func phyrexianPrinting(inCard *matcher.InputCard, cards []matcher.Card) []matcher.Card {
	if !isPhyrexian(inCard) {
		return cards
	}
	var phyrexian []matcher.Card
	for _, card := range cards {
		if card.Language == LanguagePhyrexian {
			phyrexian = append(phyrexian, card)
		}
	}
	if len(phyrexian) == 0 {
		return cards
	}
	return phyrexian
}

// unflavoredPrinting narrows a Secret Lair listing naming neither a
// collector number nor a flavor name to the one printing sold under the
// card's own name, and leaves the candidates alone when there is not one.
func unflavoredPrinting(inCard *matcher.InputCard, cards []matcher.Card) []matcher.Card {
	if !sameSet(cards) || cards[0].SetCode != "SLD" || isReskin(inCard) {
		return cards
	}
	num := matcher.ExtractNumber(inCard.Variation)
	if num == "" {
		num = matcher.ExtractNumberAny(inCard.Variation)
	}
	if num != "" {
		return cards
	}

	var plain []matcher.Card
	for _, card := range cards {
		if card.FlavorName == "" {
			plain = append(plain, card)
			continue
		}
		if namesFlavor(inCard, card) {
			return cards
		}
	}
	if len(plain) == 0 {
		return cards
	}
	// A number and its ★ foil are one printing
	for _, card := range plain[1:] {
		if card.PlainNumber != plain[0].PlainNumber {
			return cards
		}
	}
	return plain
}

// flavoredPrinting narrows a Secret Lair listing naming a flavor name to the
// printings sold under it, and leaves the candidates alone when it names none
// of theirs. The flavor name claims every treatment its printing wears, so the
// promo types must not veto one the listing only left unspelled.
func flavoredPrinting(inCard *matcher.InputCard, cards []matcher.Card) []matcher.Card {
	if !sameSet(cards) || cards[0].SetCode != "SLD" {
		return cards
	}
	var named []matcher.Card
	for _, card := range cards {
		if namesFlavor(inCard, card) {
			named = append(named, card)
		}
	}
	if len(named) == 0 {
		return cards
	}
	return named
}

// namesFlavor reports whether the listing names the flavor the printing is
// sold under, in its wording or in the name it arrived with.
func namesFlavor(inCard *matcher.InputCard, card matcher.Card) bool {
	for _, flavor := range []string{card.FlavorName, card.FaceFlavorName} {
		if flavor != "" && (inCard.Contains(flavor) || matcher.Contains(inCard.OriginalName, flavor)) {
			return true
		}
	}
	return false
}

// FilterCards narrows the printings within those sets to the one the input
// describes. See matcher.GameRules.
func (Rules) FilterCards(b *matcher.Backend, inCard *matcher.InputCard, candidates []matcher.CandidateSet) (outCards []matcher.Card) {
	// Use the result as-is if it comes from a single card in a single set,
	// as the Magic replay corpus expects: a lone candidate matches even
	// when the variation carries junk the filters below would reject.
	// Other games (Lorcana) validate strictly.
	if len(candidates) == 1 && len(candidates[0].Cards) == 1 {
		return candidates[0].Cards
	}

	for _, candidate := range candidates {
		set := b.Sets[candidate.Code]

		for _, card := range candidate.Cards {
			// Super lucky case, we were expecting the card
			num, found := VariantsTable[set.Name][card.Name][strings.ToLower(inCard.Variation)]
			if found {
				if num == card.Number {
					outCards = append(outCards, card)
				}

				// If a variant is expected we assume that all cases are covered
				continue
			}

			checkNum := true
			// Lucky case, variation is just the collector number
			num = matcher.ExtractNumber(inCard.Variation)
			// Special case for SLD, finally breaking the check against years
			if num == "" && card.SetCode == "SLD" {
				num = matcher.ExtractNumberAny(inCard.Variation)
			}
			if shouldIgnoreNumber(b, inCard, set.Name, num) {
				checkNum = false
				b.Log("Skipping number check")
			}
			if checkNum && num != "" {
				// The empty string will allow to test the number without any
				// additional prefix first
				possibleSuffixes := []string{""}
				variation := strings.Replace(inCard.Variation, "-", "", 1)
				fields := strings.Fields(strings.ToLower(variation))
				possibleSuffixes = append(possibleSuffixes, fields...)

				// Check if edition-specific numbers need special suffixes
				numFilterFunc, found := numberFilterCallbacks[set.Code]
				if found {
					overrides := numFilterFunc(inCard)
					if overrides != nil {
						possibleSuffixes = overrides
					}
				} else if card.Identifiers["tcgplayerAlternativeFoilProductId"] != "" {
					// Special case when we deal with duplicated entries
					if inCard.Foil || inCard.IsFoil() {
						possibleSuffixes = []string{SuffixSpecial}
					} else {
						possibleSuffixes = []string{""}
					}
				}

				// Add any possible extra suffixes if we know what we're dealing with
				switch {
				case IsPrerelease(inCard):
					possibleSuffixes = append(possibleSuffixes, "s")
				case IsPromoPack(inCard):
					possibleSuffixes = append(possibleSuffixes, "p")
				case isChineseAltArt(inCard):
					possibleSuffixes = append(possibleSuffixes, "s", SuffixSpecial+"s", SuffixVariant+"s")
				case isSerialized(inCard):
					possibleSuffixes = append(possibleSuffixes, "z")
				case isJudge(inCard) || isResale(inCard):
					possibleSuffixes = append(possibleSuffixes, SuffixSpecial)
				case IsJPN(inCard):
					possibleSuffixes = append(possibleSuffixes, "jpn", SuffixSpecial+"jpn")
				}

				for _, numSuffix := range possibleSuffixes {
					// The self test is already expressed by the empty string
					// This avoids an odd case of testing 1.1 = 11
					if num == numSuffix {
						continue
					}
					// The comparison is lowercase, so the suffix folds too
					number := strings.ToLower(num)
					suffix := strings.ToLower(numSuffix)
					if suffix != "" && !strings.HasSuffix(number, suffix) {
						number += suffix
					}

					if number == strings.ToLower(card.Number) {
						b.Log("Found match with card number", card.Number)
						outCards = append(outCards, card)

						// Card was found, skip any other suffix
						break
					}
				}

				// If a variant is a number we expect that this information is
				// reliable, so skip anything else
				continue
			}

			// The last-ditch effort from above - when this is set, only check
			// the non-promo sets as some promos can be mixed next to the
			// normal cards - in this way, promo sets can process as normal and
			// deduplicate all the various prerelease and promo packs
			switch set.Type {
			case "expansion", "core", "masters", "draft_innovation":
				if inCard.PromoWildcard &&
					!card.HasPromoType(PromoTypeBoosterfun) &&
					!card.HasPromoType(PromoTypePromoPack) &&
					!card.HasPromoType(PromoTypeStarterDeck) &&
					!card.HasPromoType(PromoTypeIntroPack) &&
					!strings.HasSuffix(card.Number, SuffixSpecial) &&
					!card.IsPromo {
					continue
				}
			}

			outCards = append(outCards, card)
		}
	}

	// Before the promo types and foilCheck, which veto the Phyrexian and the
	// plain printing for a treatment or a finish the listing never
	// mentioned, and the flavored one for a treatment its flavor name
	// already claimed
	if len(outCards) > 1 {
		outCards = phyrexianPrinting(inCard, outCards)
	}
	if len(outCards) > 1 {
		outCards = flavoredPrinting(inCard, outCards)
	}
	if len(outCards) > 1 {
		outCards = unflavoredPrinting(inCard, outCards)
	}

	// Sort through the array of promo types
	if len(outCards) > 1 {
		// A promo type that every candidate carries, or that none of them
		// does, cannot tell them apart, and letting it speak only empties the
		// list: the fallback below then hands the untouched list back and the
		// separation the discriminating elements had already made is lost
		discriminates := make([]bool, len(promoTypeElements))
		for i, promoElement := range promoTypeElements {
			var withTag, withoutTag bool
			for _, card := range outCards {
				if promoElement.carriedBy(&card) {
					withTag = true
				} else {
					withoutTag = true
				}
			}
			discriminates[i] = withTag && withoutTag
		}

		// The treatments the listing spells out. A card that carries all of
		// them answers everything the listing said about itself, whatever
		// else it carries in silence.
		var claimed []int

		var filteredOutCards []matcher.Card
		for _, card := range outCards {
			set, found := b.Sets[card.SetCode]
			if !found {
				continue
			}
			setDate := set.ReleaseDateTime

			var shouldContinue bool
			for i, promoElement := range promoTypeElements {
				if !discriminates[i] {
					continue
				}
				if setDate.Before(promoElement.ValidDate) {
					continue
				}
				if promoElement.CanBeWild && inCard.PromoWildcard {
					continue
				}

				tagPresent := promoElement.claimedBy(b, inCard)

				if tagPresent && !slices.Contains(claimed, i) {
					claimed = append(claimed, i)
				}

				if tagPresent && !promoElement.carriedBy(&card) {
					shouldContinue = true
					break
				} else if !tagPresent && promoElement.carriedBy(&card) && !promoElement.OnlyWhenNamed {
					shouldContinue = true
					break
				}
			}
			if shouldContinue {
				continue
			}

			if inCard.BeyondBaseSet {
				// Filter out any card that is located in the base set only
				num, err := strconv.Atoi(card.Number)
				if err == nil && num < set.BaseSetSize {
					continue
				}
			}
			filteredOutCards = append(filteredOutCards, card)
		}

		// Don't throw away what was found if filtering checks is too aggressive
		if len(filteredOutCards) > 0 {
			outCards = filteredOutCards
		} else if len(claimed) > 0 {
			// Every candidate was vetoed, so handing the list back untouched
			// is the only way not to lose the card - but a candidate that
			// carries every treatment the listing named was vetoed for
			// something the listing never mentioned, and the ones that
			// carry none of them were vetoed for contradicting it. Prefer
			// the former when it is the only one, or the base printing wins
			// on number and takes the premium card's price with it.
			var claimants []matcher.Card
			for _, card := range outCards {
				matches := true
				for _, claim := range claimed {
					if !promoTypeElements[claim].carriedBy(&card) {
						matches = false
						break
					}
				}
				if matches {
					claimants = append(claimants, card)
				}
			}
			if len(claimants) == 1 {
				outCards = claimants
			}
		}
	}

	// Sort through any custom per-edition filter(s)
	if len(outCards) > 1 {
		var filteredOutCards []matcher.Card
		for _, card := range outCards {
			cardFilterFunc, foundSimple := simpleFilterCallbacks[card.SetCode]
			cardFilterFuncs, foundComplex := complexFilterCallbacks[card.SetCode]
			if foundSimple {
				if cardFilterFunc(b, inCard, &card) {
					continue
				}
			} else if foundComplex {
				shouldContinue := false
				for _, fn := range cardFilterFuncs {
					if fn(b, inCard, &card) {
						shouldContinue = true
						break
					}
				}
				if shouldContinue {
					continue
				}
			} else {
				if misprintCheck(b, inCard, &card) {
					continue
				}
			}

			filteredOutCards = append(filteredOutCards, card)
		}

		// Don't throw away what was found if filtering checks is too aggressive
		if len(filteredOutCards) > 0 {
			outCards = filteredOutCards
		}
	}

	if len(outCards) > 1 {
		b.Log("Filtering status after main loop")
		for _, card := range outCards {
			b.Log(card.SetCode, card.Name, card.Number)
		}
	}

	// Check if there are multiple printings for Prerelease and Promo Pack cards
	// Sometimes these contain the ParentCode or the parent edition name in the field
	if len(outCards) > 1 && (IsPrerelease(inCard) || IsPromoPack(inCard)) {
		allSameEdition := true
		for _, card := range outCards {
			if card.Name != outCards[0].Name || !strings.HasPrefix(card.SetCode, "P") {
				allSameEdition = false
				break
			}
		}

		if allSameEdition {
			b.Log("allSameEdition pass needed")
			var filteredOutCards []matcher.Card
			for _, card := range outCards {
				set := b.Sets[card.SetCode]
				// The year is necessary to decouple PM20 and PM21 cards
				year := matcher.ExtractYear(set.Name)
				// Check if the parent set code is present in the variation or edition
				if strings.Contains(inCard.Variation, set.ParentCode) ||
					strings.Contains(inCard.Edition, set.ParentCode) ||
					(year != "" && inCard.Contains(year)) {
					filteredOutCards = append(filteredOutCards, card)
				} else {
					for probe, number := range MultiPromosTable[set.Name][card.Name] {
						if inCard.Contains(probe) && matcher.ExtractNumberValue(card.Number) == number {
							filteredOutCards = append(filteredOutCards, card)
						}
					}
				}
			}

			// Don't throw away what was found if filtering checks is too aggressive
			if len(filteredOutCards) > 0 {
				outCards = filteredOutCards
			}
		}
	}

	if len(outCards) > 1 && matcher.ExtractNumber(inCard.Variation) == "" {
		// Separate finishes have different collector numbers after this date
		if len(outCards) > 1 {
			var filteredOutCards []matcher.Card
			for _, card := range outCards {
				set, found := b.Sets[card.SetCode]
				if !found {
					continue
				}
				setDate := set.ReleaseDateTime
				if setDate.After(SeparateFinishCollectorNumberDate) && etchedCheck(b, inCard, &card) {
					continue
				}
				filteredOutCards = append(filteredOutCards, card)
			}

			// Don't throw away what was found if filtering checks are too aggressive
			if len(filteredOutCards) > 0 {
				outCards = filteredOutCards
			}
		}

		// If above filters were not enough, check the border, but don't skip card if it's showcase
		// ExtendedArt is fine as there cannot be a borderless one
		if len(outCards) > 1 {
			var filteredOutCards []matcher.Card
			for _, card := range outCards {
				if borderlessTreatment.vetoes(b, inCard, &card) {
					continue
				}
				filteredOutCards = append(filteredOutCards, card)
			}

			// Don't throw away what was found if filtering checks are too aggressive
			if len(filteredOutCards) > 0 {
				outCards = filteredOutCards
			}
		}

		// If above filters were not enough, check the frameEffects
		// Extended Art
		if len(outCards) > 1 {
			var filteredOutCards []matcher.Card
			for _, card := range outCards {
				// This needs date check because some old full art promos are marked
				// as extended art, in a different way of what modern Extended Art is
				set, found := b.Sets[card.SetCode]
				if !found {
					continue
				}
				setDate := set.ReleaseDateTime
				if setDate.After(PromosForEverybodyYay) && extendedArtTreatment.vetoes(b, inCard, &card) {
					continue
				}
				filteredOutCards = append(filteredOutCards, card)
			}

			// Don't throw away what was found if filtering checks are too aggressive
			if len(filteredOutCards) > 0 {
				outCards = filteredOutCards
			}
		}

		// Showcase
		if len(outCards) > 1 {
			var filteredOutCards []matcher.Card
			for _, card := range outCards {
				if showcaseTreatment.vetoes(b, inCard, &card) {
					continue
				}
				filteredOutCards = append(filteredOutCards, card)
			}

			// Don't throw away what was found if filtering checks are too aggressive
			if len(filteredOutCards) > 0 {
				outCards = filteredOutCards
			}
		}
	}

	// Within one set, a plain listing cannot be a printing sold in no plain
	// finish: the modern sets number their foil-only treatments apart from
	// the card they decorate - Aetherdrift's #434 beside #35 - so a feed
	// that gives no collector number aliases the two until the finish is
	// read. Only this direction, since a listing that says foil is making a
	// claim the filters above already weigh while saying nothing about the
	// finish is what half the feeds do; and only within one set, because a
	// foil-only printing in a set of its own is a product in its own right
	// - the premium reprint sets are the whole of it - and answering with
	// another set's plain card instead is a worse mistake than aliasing.
	if len(outCards) > 1 && !inCard.Foil && !inCard.IsEtched() && sameSet(outCards) {
		var filteredOutCards []matcher.Card
		for _, card := range outCards {
			if !card.HasFinish(matcher.FinishNonfoil) {
				continue
			}
			filteredOutCards = append(filteredOutCards, card)
		}
		// Don't throw away what was found if filtering checks are too aggressive
		if len(filteredOutCards) > 0 {
			outCards = filteredOutCards
		}
	}

	// A token shares its name with the other tokens of its kind in the
	// set, and what tells them apart on the card is the color: a set
	// prints a white Human and a red one, a blue Spirit and a white one.
	// A listing that names the color is answered with the token of that
	// color; one that names none keeps every candidate and aliases,
	// since nothing on the listing says which it is.
	if len(outCards) > 1 && b.IsToken(inCard.Name) {
		if colors := namedColors(inCard.Variation); colors != nil {
			var filteredOutCards []matcher.Card
			for _, card := range outCards {
				if sameColors(card.Colors, colors) {
					filteredOutCards = append(filteredOutCards, card)
				}
			}
			if len(filteredOutCards) > 0 {
				outCards = filteredOutCards
			}
		}
	}

	// Callers that keep the first of several - Match does for
	// gold-bordered sets, where every printing is an equally good answer -
	// name whichever card sorts first, so settle on set code and number.
	slices.SortStableFunc(outCards, func(a, b matcher.Card) int {
		if a.SetCode != b.SetCode {
			return cmp.Compare(a.SetCode, b.SetCode)
		}
		return cmp.Compare(a.Number, b.Number)
	})

	return
}

// Prefilter runs the Magic name preprocessing before the canonical-name
// lookup: the edition may be embedded in the name in a few scraper syntaxes,
// and a trailing variant may be parenthesized or dashed. It then applies the
// handful of name-specific token/playtest fixups.
func (Rules) Prefilter(b *matcher.Backend, inCard *matcher.InputCard) {
	// Binderpos weird syntax, with the edition embedded in the name
	if strings.Contains(inCard.Name, "[") {
		vars := strings.Split(inCard.Name, "[")
		inCard.Name = strings.TrimSpace(vars[0])
		if len(vars) > 1 {
			maybeEdition := strings.Join(vars[1:], " ")
			maybeEdition = strings.Replace(maybeEdition, "]", "", -1)
			maybeEdition = strings.TrimSpace(maybeEdition)

			set, err := b.GetSetByName(maybeEdition)
			if err != nil {
				inCard.Variation = maybeEdition
				// TCG Promo Pack prepends a second P to the edition
				if strings.HasPrefix(maybeEdition, "PP") {
					inCard.Variation = "Promo Pack"
				}
			} else {
				inCard.Edition = set.Name
			}
		}
	}
	// Simple case in which there is a variant embedded in the name
	if strings.Contains(inCard.Name, "(") {
		vars := matcher.SplitVariants(inCard.Name)
		if len(vars) > 1 {
			inCard.Name = vars[0]
			inCard.AddToVariant(strings.Join(vars[1:], " "))
		}
	}
	// Split a trailing " - <variant>" off the name
	if strings.Contains(inCard.Name, " - ") {
		vars := strings.Split(inCard.Name, " - ")
		if len(vars) > 1 {
			inCard.Name = vars[0]
			inCard.AddToVariant(strings.Join(vars[1:], " "))
		}
	}

	playtest := playtestPrinting(b, inCard)
	if playtest != nil {
		inCard.Name = playtest.Name
	}

	switch inCard.Name {
	case "Unquenchable Fury":
		if inCard.Contains("Battle the Horde") || inCard.Contains("Hero's Path") {
			inCard.Name += " Token"
		}
	case "Shapeshifter":
		// An edition named by set code (4ED/5ED/ATQ) or by a spelling none
		// of the words below cover (Renaissance) is asked for a real,
		// non-token Shapeshifter first.
		var realCards []matcher.Card
		set, err := b.GetSetByName(inCard.Edition)
		if err == nil {
			realCards = b.MatchInSet(inCard.Name, set.Code)
		}
		if !(inCard.Contains("Edition") ||
			inCard.Contains("Foreign") ||
			inCard.Contains("Antiquities") ||
			inCard.Contains("Reinassance") ||
			inCard.Contains("Rinascimento")) &&
			len(realCards) == 0 {
			inCard.Name += " Token"
		}
	}
}

// AdjustName normalizes the name a storefront published toward the one the
// datastore uses. See matcher.GameRules.
func (Rules) AdjustName(b *matcher.Backend, inCard *matcher.InputCard) {
	// Sticker sheet adjustments
	if strings.Contains(inCard.Name, "Sticker") {
		inCard.Name = strings.Replace(inCard.Name, "Sticker", "", 1)
		inCard.Name = strings.Replace(inCard.Name, "Sheet", "", 1)
		inCard.Name = strings.TrimSpace(inCard.Name)
	}

	// Skip for tokens, we need them to be exact or the prefix search interferes
	if strings.Contains(strings.ToLower(inCard.Name), "token") {
		return
	}
	_, found := b.CanonicalNames[matcher.Normalize(inCard.Name+" Token")]
	if found {
		inCard.Name += " Token"
		return
	}
	if b.IsToken(inCard.Name) {
		return
	}

	// Move the card number from name to variation
	num := matcher.ExtractNumber(inCard.Name)
	if num != "" {
		fields := strings.Fields(inCard.Name)
		for i, field := range fields {
			if strings.Contains(field, num) {
				fields = append(fields[:i], fields[i+1:]...)
				break
			}
		}
		// Check card exists before updating the name
		tmpName := strings.Join(fields, " ")
		_, found := b.CanonicalNames[matcher.Normalize(tmpName)]
		if found {
			inCard.Name = tmpName
			inCard.AddToVariant(num)
			return
		}
	}

	// Move any single letter variation from name to beginning variation
	if IsBasicLand(inCard.Name) {
		fields := strings.Fields(inCard.Name)
		if len(fields) > 1 {
			_, err := strconv.Atoi(strings.TrimPrefix(fields[1], "0"))
			isNum := err == nil
			isLetter := len(fields[1]) == 1

			if isNum || isLetter {
				oldVariation := inCard.Variation
				cuts := matcher.Cut(inCard.Name, " "+fields[1])

				inCard.Name = cuts[0]
				inCard.Variation = cuts[1]
				if oldVariation != "" {
					inCard.Variation += " " + oldVariation
				}
				return
			}
		}
	}

	// Rename cards that were translated differently
	if strings.Contains(inCard.Edition, "Ikoria") {
		if strings.Contains(inCard.Name, "Mothra's") && strings.Contains(inCard.Name, "Cocoon") {
			inCard.Name = "Mothra's Great Cocoon"
		} else if strings.Contains(inCard.Name, "Battra") {
			inCard.Name = "Battra, Dark Destroyer"
		} else if strings.Contains(inCard.Name, "Mechagodzilla") {
			inCard.Name = "Mechagodzilla, the Weapon"
		}
	}
	// Rename reskinned dual faced cards, only keep one side and keep the
	// flavor name, to make the following lookup in AlternateProps work
	if isSecretLair(inCard) {
		if strings.Contains(inCard.Name, "Hawkins National") {
			inCard.Name = "Hawkins National Laboratory"
		} else if strings.Contains(inCard.Name, "Plains") && strings.Contains(inCard.Name, "Battlefield Forge") {
			if inCard.Contains("Unpeeled") || inCard.Contains("669") {
				inCard.Name = "Battlefield Forge"
				inCard.Variation = "669"
			} else if inCard.Contains("Peeled") || inCard.Contains("670") {
				inCard.Name = "Plains"
				inCard.Variation = "670"
			}
		}
	}
	// Check if this card may be known as something else
	for altName, altProps := range b.AlternateProps {
		if !matcher.Equals(altName, inCard.Name) {
			continue
		}

		// Stash the number for later decoupling if available
		if altProps.OriginalNumber != "" {
			inCard.AddToVariant(altProps.OriginalNumber)
		}

		inCard.Name = altProps.OriginalName
		if altProps.IsFlavor {
			inCard.AddToVariant("Reskin")
		}

		// Adjust the token name in case it's a reskin
		if b.IsToken(inCard.Name) {
			inCard.Name += " Token"
			return
		}

		return
	}

	// Special case for Un-sets that sometimes drop the parenthesis
	if strings.Contains(inCard.Edition, "The List") ||
		strings.Contains(inCard.Edition, "Unglued") || inCard.Edition == "UGL" ||
		strings.Contains(inCard.Edition, "Unhinged") || inCard.Edition == "UNH" ||
		strings.Contains(inCard.Edition, "Unstable") || inCard.Edition == "UST" ||
		strings.Contains(inCard.Edition, "Unsanctioned") || inCard.Edition == "UND" {
		if matcher.HasPrefix(inCard.Name, "B.F.M.") || matcher.HasPrefix(inCard.Name, "BFM") {
			cardName := inCard.Name
			inCard.Name = "B.F.M. (Big Furry Monster)"
			if matcher.Contains(cardName, "Left") || matcher.Contains(inCard.Variation, "Left") {
				inCard.Variation = "28"
			} else if matcher.Contains(cardName, "Right") || matcher.Contains(inCard.Variation, "Right") {
				inCard.Variation = "29"
			}
			return
		}
		if matcher.HasPrefix(inCard.Name, "Our Market Research") {
			inCard.Name = LongestCardEver
			return
		}
		if matcher.HasPrefix(inCard.Name, "The Ultimate Nightmare") {
			inCard.Name = NightmareCard
			return
		}
		if matcher.Contains(inCard.Name, "Surgeon") && matcher.Contains(inCard.Name, "Commander") {
			inCard.Name = "Surgeon ~General~ Commander"
			return
		}
		if matcher.Contains(inCard.Name, "Who") && matcher.Contains(inCard.Name, "What") &&
			matcher.Contains(inCard.Name, "When") && matcher.Contains(inCard.Name, "Where") &&
			matcher.Contains(inCard.Name, "Why") {
			inCard.Name = "Who // What // When // Where // Why"
			return
		}

		uuids, err := b.SearchHasPrefix(inCard.Name)
		if err == nil {
			inCard.Name = b.UUIDs[uuids[0]].Name
			return
		}
	}

	// Rename a DFC with same name
	splits := strings.Split(inCard.Name, "//")
	if len(splits) == 2 && strings.TrimSpace(splits[0]) == strings.TrimSpace(splits[1]) {
		inCard.Name = strings.TrimSpace(splits[0])
		return
	}

	// Alternatively try checking across any prefix, as long as it's a double
	// sided card, for some particular cases, like meld cards, or Treasure Chest
	// Attempt first to check cards in the same edition if possible
	// Skip for tokens
	for _, set := range b.Sets {
		if matcher.Equals(set.Name, inCard.Edition) {
			for _, card := range set.Cards {
				if card.Layout != "normal" && card.Layout != "token" && matcher.HasPrefix(card.Name, inCard.Name) {
					inCard.Name = card.Name
					return
				}
			}
		}
	}
	uuids, _ := b.SearchHasPrefix(inCard.Name)
	for _, uuid := range uuids {
		co, _ := b.GetUUID(uuid)
		if co.Layout != "normal" && co.Layout != "token" {
			inCard.Name = co.Name
			return
		}
	}
}

// IsBasicLand reports whether the name may represent a basic land.
func IsBasicLand(name string) bool {
	switch {
	case strings.Contains(name, "Bear") && !strings.Contains(name, "Beard"), // G
		strings.Contains(name, "Mosquito"),                                     // B
		strings.Contains(name, "Stronghold"), strings.Contains(name, "Bandit"), // R
		strings.Contains(name, "Yeti"), strings.Contains(name, "Titan"), // R
		strings.Contains(name, "Valley"), strings.Contains(name, "Goat"), // R
		strings.Contains(name, "Fish"), strings.Contains(name, "Sanctuary"), // U
		strings.Contains(name, "Wak-Wak"): // U
	case strings.HasPrefix(name, "Plains"),
		strings.HasPrefix(name, "Island"),
		strings.HasPrefix(name, "Swamp"),
		strings.HasPrefix(name, "Mountain"),
		strings.HasPrefix(name, "Forest"),
		strings.HasPrefix(name, "Wastes"):
		return true
	case matcher.HasPrefix(name, "Snow-Covered"):
		return true
	}
	return false
}

// isExactBasicLand is the strict, exact-name basic-land check, distinct
// from the loose IsBasicLand the scrapers ask about raw listing names.
func isExactBasicLand(name string) bool {
	switch name {
	case "Plains", "Island", "Swamp", "Mountain", "Forest", "Wastes":
		return true
	}
	return false
}

// colorWords are the colors a listing spells out, as cards carry them.
var colorWords = map[string]bool{
	"white": true, "blue": true, "black": true, "red": true, "green": true,
}

// namedColors reads the colors a token listing names, or nil when it names
// none. "Colorless" names the empty set.
func namedColors(variation string) []string {
	colors := []string{}
	named := false
	for _, word := range strings.FieldsFunc(strings.ToLower(variation), func(r rune) bool {
		return r == ' ' || r == '-' || r == '/' || r == ','
	}) {
		if word == "colorless" {
			named = true
			continue
		}
		if colorWords[word] {
			named = true
			colors = append(colors, word)
		}
	}
	if !named {
		return nil
	}
	return colors
}

// sameColors reports whether a printing is exactly the colors named.
func sameColors(colors, named []string) bool {
	if len(colors) != len(named) {
		return false
	}
	for _, color := range named {
		if !slices.Contains(colors, color) {
			return false
		}
	}
	return true
}
