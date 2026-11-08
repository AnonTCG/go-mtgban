package abugames

import (
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/matcher/magic"
)

var cardTable = map[string]string{
	// Typos
	"Sakure-Tribe Elder":                  "Sakura-Tribe Elder",
	"Bogart Brute":                        "Boggart Brute",
	"Deathgazer Cockatrice":               "Deathgaze Cockatrice",
	"Elminate":                            "Eliminate",
	"Fireblade Artist Ravnica Allegiance": "Fireblade Artist",
	"Mindblade Rendor":                    "Mindblade Render",
	"Neglected Hierloom":                  "Neglected Heirloom",
	"Rathi Berserker":                     "Aerathi Berserker",
	"Smelt and Herd and Saw":              "Smelt // Herd // Saw",
	"Soulmemder":                          "Soulmender",
	"Svagthos, the Restless Tomb":         "Svogthos, the Restless Tomb",
	"Trial and Error":                     "Trial // Error",
	"Simic Signat":                        "Simic Signet",
	"Specmen 73":                          "Specimen 73",
	"Zilortha, Strength Incarnated":       "Zilortha, Strength Incarnate",
	"Makindi Siderunner":                  "Makindi Sliderunner",
	"Anti-Venom, Horrifying Hero":         "Anti-Venom, Horrifying Healer",
	"Singularity Rapture":                 "Singularity Rupture",

	"The Emperor of Palamecia /The Lord Master of Hell": "The Emperor of Palamecia",

	// The title runs the treatment into the name with no separator.
	"Kenrith, the Returned King Buy-A-Box": "Kenrith, the Returned King",

	// Funny cards
	"No Name":                         "_____",
	"Absolute Longest Card Name Ever": magic.LongestCardEver,
}

var promoTags = []string{
	"Alternate Art Duelist",
	"Arena",
	"Book",
	"Buy-a-Box",
	"Convention",
	"Draft Weekend",
	"FNM",
	"Into Pack",
	"Judge",
	"Launch",
	"OPEN HOUSE FULL ART",
	"Open House",
	"Planeswalker Weekend",
	"Prerelease",
	"Preview",
	"Promo",
	"Release",
	"SDCC",
	"Store Championship",
	"TopDeck Magazine",
}

// wearsMark reports a collector number carrying the mark a set adds to tell a
// second printing from the first - a star, a dagger, a letter after the
// digits. The storefront writes it where it writes no other word for the
// printing, so it says more than a finish the listing leaves out.
func wearsMark(number string) bool {
	if number == "" || number[0] < '0' || number[0] > '9' {
		return false
	}
	return strings.TrimLeft(number, "0123456789") != ""
}

// sharesNumber reports a collector number that is the one given wearing
// whatever mark a set adds to tell a second printing apart - a dagger in
// Arabian Nights, a letter in Portal, a star in a Secret Lair.
func sharesNumber(number, filed string) bool {
	rest, cut := strings.CutPrefix(filed, number)
	return cut && strings.TrimLeft(rest, "0123456789") == rest
}

// markedApart reports a set holding more than one printing of a card whose
// number is the one given, bare or wearing whatever the set marks its second
// printing with and the storefront does not carry - a dagger in Arabian
// Nights, a letter in Portal. A number naming several printings names none.
func markedApart(b *matcher.Backend, co *matcher.CardObject, number string) bool {
	if co == nil || number == "" {
		return false
	}
	set, err := b.GetSet(co.SetCode)
	if err != nil {
		return false
	}
	var found int
	for i := range set.Cards {
		card := &set.Cards[i]
		if card.Name != co.Name || card.Language != co.Language {
			continue
		}
		if sharesNumber(number, card.Number) {
			found++
		}
	}
	return found > 1
}

// promoShelves names the run of sets a wording belongs to, for the promos
// this storefront files under one edition and one number apiece. Two textless
// Lightning Bolts sit in "MagicFest 2019" numbered 1, and only the words tell
// the MagicFest printing from the Player Rewards one. The run is named rather
// than the set because most of these are handed out yearly and the storefront
// says which programme, never which year.
var promoShelves = map[string]string{
	"MagicFest Textless": "MagicFest",
	"Nationals":          "Nationals Promos",
	"RCQ":                "Regional Championship Qualifiers",
	"RPTQ":               "Pro Tour Promos",
	"Love Your LGS":      "Love Your LGS",
	"Standard Showdown":  "Standard Showdown",
}

// promoSet answers the set on that run holding this card. It answers none
// where the edition already names the run - the storefront keeps a shelf per
// year for some of them, and there the words repeat the shelf rather than
// correcting it - and none where the run holds the card more than once,
// which is a year this cannot pick between.
func promoSet(b *matcher.Backend, name, edition, variation string) string {
	for wording, shelf := range promoShelves {
		if !strings.Contains(variation, wording) {
			continue
		}
		if matcher.Contains(edition, strings.TrimSuffix(wording, " Textless")) {
			return ""
		}
		printings, err := b.Printings4Card(name)
		if err != nil {
			return ""
		}
		var found string
		for _, code := range printings {
			set, err := b.GetSet(code)
			if err != nil || !strings.Contains(set.Name, shelf) {
				continue
			}
			if found != "" {
				return ""
			}
			found = code
		}
		return found
	}
	return ""
}

// frontFace answers the name a split card's front face is filed under,
// where the listing still carries the title's own single "/" and both faces.
func frontFace(name string) string {
	index := strings.Index(name, " / ")
	if index >= 0 {
		return name[:index]
	}
	return name
}

// namesPromopack reports a number this edition's own promo pack prints
// under. Some promo packs keep the main set's own number instead of a
// separate promo-only set - AFR sends Power Word Kill's promo pack to #400,
// not the plain card's #114 - so a listing wearing that number and edition
// already names the exact printing. A promo-only set files its pack numbers
// with a "p" suffix (PECL 265p), which the shelf writes without it.
func namesPromopack(b *matcher.Backend, name, edition, number string) bool {
	set, err := b.GetSetByName(edition)
	if err != nil {
		return false
	}
	name = frontFace(name)
	for i := range set.Cards {
		card := &set.Cards[i]
		if card.Name == name && (card.Number == number || card.Number == number+"p") && slices.Contains(card.PromoTypes, magic.PromoTypePromoPack) {
			return true
		}
	}
	return false
}

// escapesFamily reports a "Prerelease" lookup landing on a printing outside
// the named set's family (its parent, or a set it is itself the parent of)
// instead of refusing outright. A set that stays in family, or holds no
// candidate at all, is left alone.
func escapesFamily(b *matcher.Backend, set *matcher.Set, name string) bool {
	related := func(code string) bool {
		if code == set.Code || code == set.ParentCode {
			return true
		}
		other, err := b.GetSet(code)
		return err == nil && other.ParentCode == set.Code
	}
	probe := matcher.InputCard{Name: name, Edition: set.Name, Variation: "Prerelease"}
	id, err := b.Match(&probe)
	if err == nil {
		co, err := b.GetUUID(id)
		return err == nil && !related(co.SetCode)
	}
	var alias *matcher.AliasingError
	if errors.As(err, &alias) {
		for _, candidate := range alias.Probe() {
			co, err := b.GetUUID(candidate)
			if err == nil && !related(co.SetCode) {
				return true
			}
		}
	}
	return false
}

// onlyForeignPrinting reports an edition filing a number in a language that
// never included English, scoped to the exact number so a number the set
// also prints in English still refuses.
func onlyForeignPrinting(b *matcher.Backend, name, edition, number string) bool {
	set, err := b.GetSetByName(edition)
	if err != nil {
		return false
	}
	var foreign bool
	for i := range set.Cards {
		card := &set.Cards[i]
		if card.Name != name || card.Number != number {
			continue
		}
		if card.Language == "English" {
			return false
		}
		foreign = true
	}
	return foreign
}

// numberCorroborates reports a storefront number that agrees with the one a
// reprint set files a card under, whole - "MH2-204" - or as the tail it ends
// with, where the storefront keeps the original set's own numbering.
func numberCorroborates(number, filed string) bool {
	if number == filed {
		return true
	}
	if index := strings.LastIndex(filed, "-"); index >= 0 {
		return number == filed[index+1:]
	}
	return false
}

// framedApart reports a printing the catalog sets apart from the plain one,
// by the mark it gives a set's own frames or by wearing a frame the plain one
// does not. A set outside the booster-fun run marks nothing - Teenage Mutant
// Ninja Turtles files its showcase at 226 with no mark at all - and there the
// frame is the whole of what says so.
func framedApart(marked, plain *matcher.CardObject) bool {
	if marked.HasPromoType(boosterFun) {
		return true
	}
	return len(marked.FrameEffects) > 0 && !slices.Equal(marked.FrameEffects, plain.FrameEffects)
}

// numberPrefix is the part of a collector number before its digits, which is
// the set a reprint came from where a set files its cards that way.
func numberPrefix(number string) string {
	return strings.TrimRight(number, "0123456789")
}

// The finishes a listing's own FOIL flag asks the catalog for, and the one it
// asks for when it carries no flag at all.
const (
	finishFoil    = "foil"
	finishEtched  = "etched"
	finishNonfoil = "nonfoil"
)

// finishAsked is the finish a listing's own FOIL flag asks the catalog for.
// Etched counts as one: this storefront spells it the same way.
func finishAsked(co *matcher.CardObject, foil bool) bool {
	if foil {
		return co.HasFinish(finishFoil) || co.HasFinish(finishEtched)
	}
	return co.HasFinish(finishNonfoil)
}

// finishSibling answers the number of the printing beside this one sold in
// the finish asked for, and none where the set holds no such printing or
// holds more than one. Everything that is not the finish has to agree - the
// language, the frame, and what the catalog marks it - so that a foil the
// set prints apart, a Japanese alternate art beside an English one or an
// etched showcase beside a plain one, is never mistaken for this card. So is
// an alternative: Avatar's tutorial cards carry the set's own names and only
// that flag tells them from the cards they teach. So is the set a reprint
// came from, which The List spells into the number it files each card under.
func finishSibling(b *matcher.Backend, co *matcher.CardObject, foil bool) string {
	set, err := b.GetSet(co.SetCode)
	if err != nil {
		return ""
	}
	var number string
	for i := range set.Cards {
		card := &set.Cards[i]
		if card.Name != co.Name || card.Number == co.Number ||
			card.Language != co.Language ||
			card.IsAlternative != co.IsAlternative ||
			numberPrefix(card.Number) != numberPrefix(co.Number) ||
			!slices.Equal(card.FrameEffects, co.FrameEffects) ||
			!slices.Equal(card.PromoTypes, co.PromoTypes) {
			continue
		}
		object := matcher.CardObject{Card: *card}
		if !finishAsked(&object, foil) {
			continue
		}
		if number != "" {
			return ""
		}
		number = card.Number
	}
	return number
}

// eternalNonfoil maps the number the catalog files an Avatar Eternal card
// under, foil only, to the alternative printing it sells in nonfoil.
var eternalNonfoil = map[string]string{
	"210": "265", "211": "266", "214": "268", "215": "269", "218": "273",
	"219": "274", "220": "275", "221": "276", "234": "277", "238": "280",
	"239": "281", "240": "282", "244": "285", "245": "286", "246": "287",
	"247": "288",
}

// errForeignListing marks a listing in a language the catalog never printed
// the card in, which has no printing of its own to be priced against.
var errForeignListing = errors.New("foreign listing")

// errConflictingNumber marks a listing whose number names a printing of the
// set it is filed under, where its wording names a reprint somewhere else.
// Both cannot be true and nothing says which is.
var errConflictingNumber = errors.New("conflicting number")

// errUnprintedFinish marks a listing whose finish the catalog never sold that
// printing in, which has no printing of its own to be priced against.
var errUnprintedFinish = errors.New("unprinted finish")

// boosterFun is the mark the catalog gives every frame a set prints beside
// its plain one - borderless, showcase, extended art, retro. A drop marker
// like sldbonus is not one of them: it says where a card came from, not what
// frame it wears.
const boosterFun = magic.PromoTypeBoosterfun

// finishNamed reports a printing sold in a finish this storefront would spell
// FOIL, which is the flag its own wording carries.
func finishNamed(co *matcher.CardObject) bool {
	return co.Foil || co.Etched
}

// unmarked reports a printing the catalog gives no treatment, the token
// promo type every token carries aside.
func unmarked(co *matcher.CardObject) bool {
	for _, promoType := range co.PromoTypes {
		if promoType != magic.PromoTypeToken {
			return false
		}
	}
	return true
}

// foilOfUnfoiled reports a listing spelled FOIL whose printing has no finish
// to name, which the storefront does for cards it never had in foil.
func foilOfUnfoiled(title string, co *matcher.CardObject) bool {
	return strings.Contains(title, "FOIL") && !finishNamed(co)
}

// resolved answers the printing a description names, and nil when it names
// none. It matches a copy, so the matcher's own edits stay in the probe.
func resolved(b *matcher.Backend, name, edition, variation, language string, foil bool) *matcher.CardObject {
	probe := matcher.InputCard{Name: name, Edition: edition, Variation: variation, Language: language, Foil: foil}
	id, err := b.Match(&probe)
	if err != nil {
		return nil
	}
	co, err := b.GetUUID(id)
	if err != nil {
		return nil
	}
	return co
}

// variantLetterRe matches the letter this storefront opens a variation with
// when a set prints one card several times over - "(a Dark)", "(b Spring)",
// "(a - Green Bottle)" - and the words it hangs off it.
var variantLetterRe = regexp.MustCompile(`^([a-f])(?: -)? [A-Z(]`)

// variantLetter reads that letter. The storefront spells a basic land's
// artwork in capitals and everything else in lower case, so the two never
// answer for each other.
func variantLetter(variation string) string {
	match := variantLetterRe.FindStringSubmatch(variation)
	if match == nil {
		return ""
	}
	return match[1]
}

// describedArtwork preserves exact artwork descriptions already cataloged by
// the matcher. ABU's letter is not necessarily the catalog's suffix: Strip
// Mine "d Tower" is 82c, while the bare letter "d" names another artwork.
func describedArtwork(b *matcher.Backend, name, edition, variation string) string {
	if variantLetter(variation) == "" {
		return ""
	}
	set, err := b.GetSetByName(edition)
	if err != nil {
		return ""
	}
	return magic.VariantsTable[set.Name][name][strings.ToLower(variation)]
}

// artworkLetterRe matches the letter a basic land's title opens its variation
// with, and the words the storefront hangs off it.
var artworkLetterRe = regexp.MustCompile(`^([A-F])\b`)

// artworkLetter answers the letter a basic land listing names its artwork by,
// and "" for everything else. Only a basic land is read this way: the letter
// is what the catalog files those printings under, and on any other card a
// bare letter is as likely to be the start of a word the wording spends on
// something else.
func artworkLetter(cardName, variation string) string {
	if !magic.IsBasicLand(cardName) {
		return ""
	}
	match := artworkLetterRe.FindStringSubmatch(variation)
	if match == nil {
		return ""
	}
	return match[1]
}

func preprocess(b *matcher.Backend, card *ABUCard) (*matcher.InputCard, error) {
	corrected := correctListing(*card)
	card = &corrected
	lang := ""
	if len(card.Language) > 0 {
		lang = card.Language[0]
	}
	// The language field says English on rows whose own title says otherwise
	// - the Japanese 30th Anniversary Serra Angel is filed as English and
	// sold as Japanese - so read the title where the two disagree.
	if named, found := strings.CutPrefix(card.Title, "Non-English - "); found && lang == "English" {
		lang = strings.TrimSpace(strings.SplitN(named, " - ", 2)[0])
	}

	// Non-Singles magic cards
	switch card.Layout {
	case "Scheme", "Plane", "Phenomenon":
		return nil, errors.New("non-single card")
	}

	// Non-existing cards
	switch card.DisplayTitle {
	case "Silent Submersible (Promo Pack)",
		"Silent Submersible (Promo Pack) - FOIL",
		"Hymn to Tourach (B - Mark Justice - 1996)",
		"Skyclave Shade (Extended Art)",
		"Test Animate Dead":
		return nil, errors.New("untracked card")
	}
	switch card.ID {
	case "1604919", "1604921", "1604922", // Living Twister
		"1604802", "1604801", "1604799": // Commence the Endgame
		return nil, errors.New("duplicated card")
	}

	// The flag is written with the space and without it, "- FOIL" beside
	// "-FOIL", and reading only the spaced form priced a foil as a nonfoil.
	// Read it from the words after the card's own name: a name that is
	// itself "Lavinia, Foil to Conspiracy" must not trip it.
	title := strings.ToLower(card.DisplayTitle)
	suffix := title
	simple := strings.ToLower(card.SimpleTitle)
	if simple != "" {
		if cut, ok := strings.CutPrefix(title, simple); ok {
			suffix = cut
		}
	}
	isFoil := strings.Contains(suffix, " foil") ||
		strings.Contains(suffix, "-foil") ||
		strings.Contains(suffix, " - fol") // SS3 Pyroblast

	edition := card.Edition
	title = card.DisplayTitle
	if title == "" {
		title = card.SimpleTitle
	}

	// Split by -, rebuild the cardname in a standardized way
	variation := ""
	vars := strings.Split(title, " - ")
	cardName := vars[0]
	if len(vars) > 1 {
		if vars[len(vars)-1] == edition {
			vars = vars[:len(vars)-1]
		}

		variation = strings.Join(vars[1:], " ")

		// Fix some untagged prerelease cards
		// Nahiri's Wrath, Tendershoot Dryad
		if strings.Contains(variation, edition+" FOIL") {
			variation = strings.Replace(variation, edition+" FOIL", "Prerelease", 1)
		}
	}

	// Split by ()
	vars = matcher.SplitVariants(cardName)
	cardName = vars[0]
	if len(vars) > 1 {
		oldVariation := variation
		variation = strings.Join(vars[1:], " ")
		if oldVariation != "" {
			variation += " " + oldVariation
		}
	}

	// Separate flavor names
	oracleName := ""
	if strings.Contains(cardName, " | ") {
		parts := strings.Split(cardName, " | ")
		cardName = parts[0]
		oracleName = parts[len(parts)-1]
	}

	// Cleanup variation as necessary
	if variation != "" {
		variation = strings.Replace(variation, "FOIL", "", -1)

		variation = strings.Replace(variation, "(", "", -1)
		variation = strings.Replace(variation, ")", "", -1)

		variation = strings.Replace(variation, "Not Tournament Legal", "", 1)

		variation = strings.Join(strings.Fields(variation), " ")

		isPromo := false
		for _, tag := range promoTags {
			if strings.Contains(variation, tag) {
				isPromo = true
				break
			}
		}
		// A bundle promo is printed in the set it comes with and numbered
		// there - Frodo, Sauron's Bane is LTR 448 - so the set the listing
		// names is the set it is in. Moving it to the promo shelf drops the
		// number that finds it, and the wording alone answers with whatever
		// else the set marks, its showcase among them.
		if strings.Contains(variation, "Bundle") {
			isPromo = false
		}
		// Unstamped prerelease cards such as MKM #431-432 are filed in the
		// expansion itself. Keep that exact set/number when the catalog
		// independently confirms the prerelease printing.
		if variation == "Prerelease no date stamp" {
			if set, err := b.GetSetByName(edition); err == nil {
				for _, candidate := range b.MatchInSet(cardName, set.Code) {
					if candidate.Number == card.Number && candidate.HasPromoType("prerelease") {
						isPromo = false
					}
				}
			}
		}
		if isPromo {
			switch {
			case strings.Contains(variation, "Promo Pack") && namesPromopack(b, cardName, card.Edition, card.Number):
				// The shelf's own set holds this promo pack printing;
				// nothing to reset.
			case strings.Contains(variation, "Prerelease"):
				set, setErr := b.GetSetByName(card.Edition)
				if setErr == nil && escapesFamily(b, set, cardName) {
					return nil, matcher.ErrUnsupported
				}
				// Handle promo cards appearing in multiple editions
				// like Sorcerous Spyglass
				variation += " " + edition
				edition = "Promo"
			case strings.Contains(variation, "Promo Pack"):
				variation += " " + edition
				edition = "Promo"
			default:
				// Reset edition, and trust matcher to find it by
				// its variation
				edition = "Promo"
			}
		}
	}

	// These are English cards with a misprinted copyright line. Passing
	// "Portuguese" to Match would override the listing's English language.
	// Only replace the exact description when its marked catalog number
	// establishes the Fifth Edition misprint independently of vendor IDs.
	if variation == "Portuguese Copyright" {
		if set, err := b.GetSetByName(edition); err == nil && set.Code == "5ED" && strings.HasSuffix(card.Number, "†") {
			for _, candidate := range b.MatchInSet(cardName, set.Code) {
				if candidate.Number == card.Number && candidate.Language == "English" {
					variation = card.Number
					break
				}
			}
		}
	}

	// The vendor IDs are the primary identity when they agree with the
	// listing's name, language, finish and printing descriptors. Known copied
	// IDs and descriptions the ID checks cannot establish use the text path.
	if identified := identifiedCard(b, card, matcher.InputCard{
		Name: cardName, Edition: edition, Variation: variation, Foil: isFoil, Language: lang,
	}); identified != nil {
		return identified, nil
	}

	if set := promoSet(b, cardName, card.Edition, variation); set != "" {
		edition = set
	}

	// This storefront shelves the textless Player Rewards cards with the
	// MagicFest ones, and the matcher reads that shelf first and never gets
	// to the rule that knows the programme by its own name and year. Say
	// nothing about the shelf and let that rule answer.
	if matcher.Contains(variation, "Player Rewards") && matcher.Contains(edition, "MagicFest") {
		edition = "Promo"
	}

	switch edition {
	case "":
		if magic.IsBasicLand(cardName) {
			card.Edition = "GK2"
		} else {
			return nil, errors.New("missing edition")
		}
	case "Promo":
		switch cardName {
		case "Skirk Marauder":
			edition = "Arena League 2003"
		case "Damnation(Secret Lair":
			cardName = "Damnation"
			edition = "SLD"
		case "Elvish Aberration":
			if variation == "FNM" {
				variation = "Arena"
			}
		case "Elvish Lyrist":
			if variation == "FNM" {
				variation = "JSS Foil"
			}
		case "Island":
			if variation == "Arena 1999 No Symbol Promo" {
				variation = "Arena 1999 misprint"
			}
		case "Stocking Tiger":
			if variation == "Target" {
				variation = "misprint"
			}
		case "Psychatog":
			if variation == "FNM" {
				variation = "Textless"
			}
		case "Sol Ring":
			if variation == "Commander" {
				variation = "MagicFest 2019"
			}
		case "Mountain":
			if variation == "APAC a Phillippines" {
				variation = "APAC a Philippines"
			}
		case "Beast of Burden":
			if variation == "Prerelease No Expansion Symbol FOIL" {
				variation = "Prerelease misprint"
			}
		case "Mechagodzilla, Battle Fortress / Hangarback Walker":
			if variation == "Welcome Back" {
				cardName = "Hangarback Walker"
				edition = "PLG20"
			}
		case "Serra Angel":
			if strings.Contains(variation, "25th Anniversary") {
				edition = "PDOM"
			}
		case "Hidetsugu, Devouring Chaos":
			edition = "NEO"
		case "Rafiq of the Many":
			edition = "SHA"
			variation = "250"
		case "Swiftfoot Boots":
			if strings.Contains(variation, "Launch") {
				edition = "PW22"
				variation = "4"
			}
		case "Brood Sliver":
			edition = "SLD"
		case "Lavinia, Azorius Renegade":
			edition = "PRNA"
			variation = "189"
		}
		if strings.Contains(variation, "Secret") || strings.Contains(variation, "Lair") {
			num := matcher.ExtractNumber(variation)
			if num != "" {
				variation = num
			} else if strings.Contains(variation, "Seb McKinnon") {
				variation = "119"
			}
			edition = "Secret Lair Drop"
		} else if magic.IsBasicLand(cardName) && strings.Contains(variation, "Full-Text") {
			edition = "SLD"
			variation = strings.TrimPrefix(variation, "Full-Text ")
		} else if strings.Contains(variation, "Play Promo") {
			variation = strings.Replace(variation, "FNM", "", 1)
		} else if card.Layout == "Planar" {
			edition = "Planechase Promos"
		} else if variation == "Preview" {
			if len(b.MatchInSet(cardName, "MGB")) > 0 {
				edition = "MGB"
			}
		}
	case "Secret Lair", "Secret Lair Drop":
		edition = "Secret Lair Drop"
		if len(b.MatchInSetNumber(cardName, "SLC", card.Number)) > 0 {
			edition = "SLC"
			variation = card.Number
		} else {
			// Check if variation contains a number as it's usually more
			// accurate. If not, check the card.Number property (yolo).
			// ExtractNumberAny (not the year-capped ExtractNumber) so a Secret
			// Lair collector number >= 1993 (e.g. 7010) is recognized and not
			// clobbered by a stale card.Number, while still ignoring dates and
			// ordinals.
			num := matcher.ExtractNumberAny(variation)
			if num == "" && card.Number != "" {
				variation += " " + card.Number
			}
		}
	case "Anthologies":
		if cardName == "Mountain" {
			if variation == "A" {
				variation = "B"
			} else if variation == "B" {
				variation = "A"
			}
		}
	case "World Championship":
		if cardName == "City of Brass" {
			if variation == "Leon Lindback 1996" {
				variation = "Sideboard Leon Lindback 1996"
			}
		}
	case "Ikoria: Lair of Behemoths":
		if strings.Contains(cardName, " / ") {
			cardName = card.SimpleTitle
			variation = "Godzilla"
		}
	case "Oath of the Gatewatch":
		if cardName == "Captain's Claws" && variation == "Goldnight Castigator Shadow FOIL" {
			variation = "misprint"
		}
	case "Core Set 2020 / M20":
		if cardName == "Corpse Knight" && variation == "2/3" {
			variation = "misprint"
		}
	case "Mystery Booster":
		if cardName == "Trial and Error" {
			// Hack to prevent aliasing with the real "Trial // Error"
			cardName = "Trial and Error "
		}
	case "Streets of New Capenna Commander":
		if strings.Contains(cardName, "Spellbinding Soprano") && strings.Contains(cardName, "The List") {
			cardName = "Spellbinding Soprano"
			variation = "Promo Pack"
		}
	case "Fourth Edition Foreign Black Border":
		variation = strings.TrimSuffix(variation, " Japanese")
		variation = strings.TrimSuffix(variation, " BB")
		variations := strings.Fields(variation)
		if len(variations) > 1 && len(variations[0]) == 1 {
			variation = strings.Join(variations[1:], " ")
		}
	case "Fallen Empires":
		variations := strings.Fields(variation)
		if len(variations) > 1 && len(variations[0]) == 1 {
			variation = strings.Join(variations[1:], " ")
		}
	case "Modern Horizons 2 Timeshifts":
		edition = "MH2"
	// This set has some non standard way to tag foil, but luckily variations are all on different CNs
	case "Edge of Eternities: Stellar Sights":
		variation = card.Number
	case "Aetherdrift":
		if variation == "Borderless" {
			variation += " " + card.Number
		}
	case "Final Fantasy Commander":
		variation += " " + card.Number
	}

	// Either Promo or "European Land Program"
	if strings.Contains(variation, "Scandanavia") {
		variation = strings.Replace(variation, "Scandanavia", "Scandinavia", 1)
	} else if strings.Contains(variation, "Phillippines") {
		variation = strings.Replace(variation, "Phillippines", "Philippines", 1)
	} else if strings.Contains(variation, "Extented") {
		variation = strings.Replace(variation, "Extented", "Extended", 1)
	} else if strings.Contains(variation, "CMDR Party") {
		variation = strings.Replace(variation, "CMDR Party", "Commander Party", 1)
	}

	// This storefront calls the black-bordered Fourth Edition BB, where the
	// catalog files it as the Japanese printing, and WB the white-bordered
	// one. There is no white-bordered printing in any language but English -
	// the catalog holds 4ED in English and white, 4BB in Japanese and black,
	// and nothing else - so a foreign one names a card that was never made,
	// and the matcher already knows to refuse it under that name.
	if matcher.Contains(edition, "4th Edition") {
		switch variation {
		case "BB":
			variation = "Japanese"
		case "WB":
			if lang != "" && lang != "English" {
				variation = "Foreign White Border"
			}
		}
	}

	// The wording repeats the number and the storefront's own field spells
	// it with the mark that tells two printings apart - "(Secret Lair
	// 1630)" beside a card_number of 1630 with a star. Take the fuller one.
	if spelled := matcher.ExtractNumberAny(variation); spelled != "" &&
		spelled != card.Number && sharesNumber(spelled, card.Number) {
		variation = strings.Replace(variation, spelled, card.Number, 1)
	}

	// The wording names an Extended Art treatment; trust the vendor's own
	// card_number over a conflicting number spelled in the title, once that
	// spelled number names a real printing rather than a stray year.
	if strings.Contains(variation, "Extended Art") {
		spelled := matcher.ExtractNumberAny(variation)
		if spelled != "" && card.Number != "" && spelled != card.Number {
			set, err := b.GetSetByName(edition)
			if err == nil && len(b.MatchInSetNumber(frontFace(cardName), set.Code, spelled)) > 0 {
				variation = strings.Replace(variation, spelled, card.Number, 1)
			}
		}
	}

	// Resolve a cataloged artwork description before adding the vendor's
	// number or reducing its letter. The same table also validates vendor IDs.
	if number := describedArtwork(b, cardName, edition, variation); number != "" {
		variation = number
	}

	// Use collector number data when the variation carries has none, unless for a couple of editions
	var numbered bool
	if card.Number != "" && matcher.ExtractNumberAny(variation) == "" {
		switch {
		case edition == "Unfinity":
		// A Secret Lair is numbered whatever shelf the storefront files it
		// on, and the number is the whole of what tells its drops apart.
		// MagicFest reprints can span years; its collector number separates
		// Counterspell in PF24 (#1) from PF26 (#5). A promo pack without a
		// shelf also needs its number to distinguish reprinted promo lands.
		case edition == "Promo" && !matcher.Contains(variation, "Secret Lair") &&
			!matcher.Contains(variation, "MagicFest") &&
			!(card.Edition == "" && matcher.Contains(variation, "Promo Pack")):
		default:
			if variation != "" {
				variation += " "
			}
			variation += card.Number
			numbered = true
		}
	}
	// A basic land's artwork is the only thing telling one of its printings
	// from another, and this storefront names it with a letter in the title
	// - "Swamp (C)", "Island (A) - Waterfall". Its own collector number is
	// the same for every one of them, so the number just read buries the
	// letter under one that names the whole group, and the words beside the
	// letter are the storefront's own name for the art, which the catalog
	// spells its own way. The letter alone reaches every one of them.
	letter := artworkLetter(cardName, variation)
	if letter != "" {
		variation = letter
	}

	// Older sets name their several printings of one card by what is drawn
	// on them - "Urza's Mine (d Tower)" - and there the words are the whole
	// identity and the letter counts for nothing. Newer ones number the
	// printings and leave the words as flavour, so take the letter alone
	// only where it reaches a printing on its own.
	if letter == "" {
		variant := variantLetter(variation)
		if variant != "" && !matcher.Contains(variation, "The List") && resolved(b, cardName, edition, variant, lang, isFoil) != nil {
			variation = variant
		}
	}

	// "The List" names the set a card was reprinted into, and this
	// storefront keeps the original set's own number beside it. That set
	// files its cards under the original's code and number - AVR-20, not 20
	// - so the bare number reaches the first printing and the words naming
	// the reprint go unread, putting the two beside each other.
	if numbered && matcher.Contains(variation, "The List") {
		bare := strings.TrimSpace(strings.TrimSuffix(variation, card.Number))
		reprint := resolved(b, cardName, edition, bare, lang, isFoil)
		if reprint != nil {
			// The storefront carries two numbers for one reprint and the
			// set holds the card once - Savage Lands is filed at ALA-228
			// and listed at both 228 and 1025. The second is Commander
			// Masters' own printing wearing the wrong words: the number
			// names a card of that set exactly, where 228 names none and
			// only echoes the reprint's own tail. Let the contradicted one
			// go rather than pick between them.
			if !numberCorroborates(card.Number, reprint.Number) {
				own := resolved(b, cardName, card.Edition, card.Number, lang, isFoil)
				if own != nil && own.Number == card.Number {
					// ULST assigns new catalog numbers to cards still printed
					// with their original set's numbers. Require the named variant
					// to identify that same original printing before dropping it.
					original := resolved(b, cardName, card.Edition, strings.ReplaceAll(bare, "The List", ""), lang, isFoil)
					if reprint.SetCode != "ULST" || original == nil || original.UUID != own.UUID {
						return nil, errConflictingNumber
					}
				}
			}
			variation = bare
		}
	}

	// A set can file two printings of a card under one number, telling them
	// apart with a mark the storefront does not carry - Arabian Nights
	// holds its two Erg Raiders at 25 and at 25 with a dagger. The number
	// then picks whichever it lands on first and the wording beside it,
	// which is the only thing that knows, goes unread. Drop it and let the
	// wording answer.
	if numbered {
		bare := strings.TrimSpace(strings.TrimSuffix(variation, card.Number))
		numbered := resolved(b, cardName, edition, variation, lang, isFoil)
		if bare != "" && markedApart(b, numbered, card.Number) {
			// Only where what is left says which of them - "The List"
			// names both printings of Grizzly Fate and neither - and only
			// where it stays among the printings that share the number.
			// "Secret Lair" says nothing of the star that tells 1721 from
			// 1721 and walks off to 2214, another drop of the same card.
			marked := resolved(b, cardName, edition, bare, lang, isFoil)
			if marked != nil && marked.UUID != numbered.UUID &&
				sharesNumber(card.Number, marked.Number) {
				variation = bare
			}
		}
	}

	// This storefront gives every frame of a card the base printing's own
	// collector number - the extended art of Platoon Dispenser is filed
	// under BRO 36 where the catalog has it at 310 - so a listing naming a
	// frame answers with the plain card the number just read states. Ask
	// again without that number, and keep the answer only where it reaches
	// a frame the catalog marks.
	//
	// Only a number this scraper appended is dropped: a listing spelling
	// its own is one where the wording and the number already agree.
	if numbered {
		bare := strings.TrimSpace(strings.TrimSuffix(variation, card.Number))
		plain := resolved(b, cardName, edition, variation, lang, isFoil)
		if bare != "" && plain != nil && unmarked(plain) {
			marked := resolved(b, cardName, edition, bare, lang, isFoil)
			if marked != nil && framedApart(marked, plain) &&
				(finishNamed(marked) == isFoil || finishNamed(marked) == finishNamed(plain)) {
				variation = bare
			}
		}
	}

	// The same number is wrong the other way round too: a listing whose
	// whole variation is the number can carry the number of a frame, and then the plain card
	// answers with its own showcase. Ask again without the number, and keep
	// the answer only where it reaches a printing the catalog leaves
	// unmarked - a set whose every printing is marked, like Special Guests,
	// has no plain card to reach and keeps the number it came with.
	if numbered && strings.TrimSpace(strings.TrimSuffix(variation, card.Number)) == "" {
		framed := resolved(b, cardName, edition, variation, lang, isFoil)
		if framed != nil && framed.HasPromoType(boosterFun) {
			plain := resolved(b, cardName, edition, "", lang, isFoil)
			if plain != nil && unmarked(plain) &&
				plain.SetCode == framed.SetCode && finishNamed(plain) == isFoil {
				variation = ""
			}
		}
	}

	// Restore canonical name for split cards
	cardName = strings.ReplaceAll(cardName, " / ", " // ")
	if strings.Contains(cardName, " // ") {
		cardName = strings.Split(cardName, " // ")[0]
		cardName = strings.TrimSpace(cardName)
	}

	// The playtest card is literally named Trial and Error; only the
	// ordinary split card needs the historical Trial // Error alias.
	name, found := cardTable[cardName]
	if found && !(cardName == "Trial and Error" && matcher.Contains(edition, "Playtest")) {
		cardName = name
	}

	// Stash the language information (filtered earlier)
	if lang != "" && lang != "English" {
		if variation != "" {
			variation += " "
		}
		variation += lang
	}

	// The finish a listing names has to be one the printing was sold in. A
	// set can keep the two apart at two numbers - Avatar prints Aang's
	// Defense in foil at 211 and in nonfoil at 266 - and the number names
	// only one of them, so the foil and the nonfoil answer alike and the
	// shop's two prices land on a single uuid.
	//
	// Take the printing beside it sold in the finish asked for. Where the
	// set has none, the listing names a card that was never printed, and
	// pricing it against the finish that was is the same collision by
	// another route - so let it go.
	printing := resolved(b, cardName, edition, variation, lang, isFoil)
	switch {
	case printing == nil:
	case printing.Number == card.Number && wearsMark(card.Number):
		// The storefront's number names this printing and wears the mark
		// that tells it from its sibling - 1630 with a star beside plain
		// 1630 - which says more than the FOIL it forgot to write.
	case isFoil && !printing.HasFinish(finishFoil) && printing.HasFinish(finishEtched):
		// A Secret Lair sells some of its cards etched and never in plain
		// foil, and this storefront calls both of them FOIL, so the etched
		// one answered with the nonfoil beside it.
		variation = strings.TrimSpace(variation + " Etched")
	case !finishAsked(printing, isFoil):
		sibling := finishSibling(b, printing, isFoil)
		if sibling == "" && !isFoil && printing.SetCode == "TLE" {
			sibling = eternalNonfoil[printing.Number]
		}
		if sibling == "" {
			return nil, errUnprintedFinish
		}
		variation = sibling
	}

	// A storefront selling a card in a language the catalog never printed it
	// in has nowhere to file it. The match falls back on the English
	// printing, and the shop's own price for the Italian copy lands beside
	// the English one on a single uuid - three prices on 9th Edition Elvish
	// Piper, English, Italian and Japanese, each of them real. A set that
	// does hold the printing is reached and kept, the Japanese Chronicles
	// and the foreign black borders among them.
	if lang != "" && lang != "English" {
		printing := resolved(b, cardName, edition, variation, lang, isFoil)
		if printing == nil || printing.Language != lang {
			idName := cardName
			if oracleName != "" {
				idName = oracleName
			}
			identified := foreignIdentified(b, card, idName, lang, isFoil)
			if identified == nil {
				return nil, errForeignListing
			}
			return identified, nil
		}
	} else if lang == "English" && onlyForeignPrinting(b, cardName, edition, card.Number) {
		lang = ""
	}

	return &matcher.InputCard{
		Name:      cardName,
		Variation: variation,
		Edition:   edition,
		Foil:      isFoil,
		Language:  lang,
	}, nil
}
