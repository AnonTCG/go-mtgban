package strikezone

import (
	"regexp"
	"slices"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/matcher/magic"
)

// neonInkColors are the Neon Ink treatments the catalog files under a colour.
var neonInkColors = []string{"Red", "Green", "Blue", "Yellow", "Pink"}

// neonInkWording spells a Neon Ink colour the way the catalog files it. The
// buylist drops the word the treatment is named for and writes "Neon Red",
// which names no treatment at all and reads as the plain printing standing
// beside the four coloured ones.
func neonInkWording(variation string) string {
	if !strings.HasPrefix(variation, "Neon ") || strings.HasPrefix(variation, "Neon Ink") {
		return variation
	}
	for _, color := range neonInkColors {
		if strings.HasPrefix(variation, "Neon "+color) {
			return "Neon Ink " + strings.TrimPrefix(variation, "Neon ")
		}
	}
	return variation
}

// duelDeck matches the duel deck shelves as this storefront heads them.
var duelDeck = regexp.MustCompile(`^Duel Deck (.+) VS (.+)$`)

// fromCuteToBrutePathways are the ten pathways of Secret Lair Commander:
// From Cute to Brute, each reprinted on PLST but colliding by bare name
// with a foil-only Secret Lair Ultimate printing.
var fromCuteToBrutePathways = []string{
	"Barkchannel Pathway", "Blightstep Pathway", "Branchloft Pathway",
	"Brightclimb Pathway", "Clearwater Pathway", "Cragcrown Pathway",
	"Darkbore Pathway", "Hengegate Pathway", "Needleverge Pathway",
	"Riverglide Pathway",
}

func preprocess(b *matcher.Backend, cardName, edition, notes string) (*matcher.InputCard, error) {
	var variation string

	// Skip tokens, too many variations
	if strings.Contains(cardName, "Token") {
		return nil, matcher.ErrUnsupported
	}
	// A signed copy prices the signature, not the printing, and the
	// datastore carries no separate row for one.
	if strings.Contains(cardName, "(Autographed)") {
		return nil, matcher.ErrUnsupported
	}

	cn, found := cardTable[cardName]
	if found {
		cardName = cn
	}
	cardName = strings.ReplaceAll(cardName, " / ", " // ")
	// A flavor name written before the card's own ("Astral Tiran - Primeval
	// Titan") keeps the card's own. Ikoria's Godzilla names are read below.
	var droppedFlavor bool
	if head, tail, dashed := strings.Cut(cardName, " - "); dashed && edition != "Ikoria: Lair of Behemoths" {
		if _, err := b.SearchEquals(matcher.SplitVariants(tail)[0]); err == nil {
			if _, err := b.SearchEquals(head); err != nil && !isFlavorOf(b, head, tail) {
				cardName = tail
				droppedFlavor = true
			}
		}
	}
	// The same flavor name arrives split by the storefront's own dash
	// reading, the card's own name carried in the notes behind its
	// treatment ("Astral Tiran" with "Showcase Primeval Titan"): the notes
	// name the card, and the treatment stays a note.
	if _, err := b.SearchEquals(cardName); err != nil && notes != "" {
		for _, treatment := range []string{"Showcase ", "Borderless ", "Extended Art "} {
			if tail, found := strings.CutPrefix(notes, treatment); found {
				if _, err := b.SearchEquals(tail); err == nil {
					cardName, notes = tail, strings.TrimSpace(treatment)
					droppedFlavor = true
					break
				}
			}
		}
	}
	// The duel decks are headed "Duel Deck X VS Y" where the catalog says
	// "Duel Decks: X vs. Y".
	if m := duelDeck.FindStringSubmatch(edition); m != nil {
		edition = "Duel Decks: " + m[1] + " vs. " + m[2]
	}

	// Sometimes the buylist specifies tags at the end of the card name,
	// but without parenthesis, so make sure they are present.
	for _, tag := range tagsTable {
		if strings.HasSuffix(cardName, tag) {
			cardName = strings.Replace(cardName, tag, "("+tag+")", 1)
			break
		}
	}

	cardName = strings.Replace(cardName, "[", "(", 1)
	cardName = strings.Replace(cardName, "]", ")", 1)

	switch {
	// Tyop
	case strings.HasPrefix(cardName, "Snow-Cover "):
		cardName = strings.Replace(cardName, "Snow-Cover ", "Snow-Covered ", 1)
	// Found at beginning, move it to variation
	case strings.HasPrefix(cardName, "Borderless"):
		cardName = strings.TrimPrefix(cardName, "Borderless ")
		cardName = strings.TrimPrefix(cardName, "Alt Art ")
		variation = "Borderless"
	// Found at beginning, move it to variation
	case strings.HasPrefix(cardName, "Showcase"):
		cardName = strings.TrimPrefix(cardName, "Showcase ")
		variation = "Showcase"
	// Found at beginning, move it to variation
	case strings.HasPrefix(cardName, "Extended Art"):
		cardName = strings.TrimPrefix(cardName, "Extended Art ")
		variation = "Extended Art"
	// Found at end, move it to variation
	case strings.HasSuffix(cardName, "JPN ALT ART PRERELEASE"):
		cardName = strings.TrimSuffix(cardName, " JPN ALT ART PRERELEASE")
		variation = "Prerelease Japanese"
	// Found at end, move it to edition
	case strings.HasSuffix(cardName, "Ultimate Edition"):
		cardName = strings.TrimSuffix(cardName, " Ultimate Edition")
		edition = "Secret Lair: Ultimate Edition"
	// Found at end, move it to edition
	case strings.HasSuffix(cardName, "Godzilla") && magic.IsBasicLand(cardName):
		cardName = strings.TrimSuffix(cardName, " Godzilla")
		edition = "SLD"
	// Found at beginning, just drop it
	case strings.HasPrefix(cardName, "Alt Art"):
		cardName = strings.TrimPrefix(cardName, "Alt Art ")
		edition = "SLD"
	// APAC and EURO lands, drop specifier
	case strings.Contains(cardName, "APAC") || strings.Contains(cardName, "EURO"):
		variants := matcher.SplitVariants(cardName)
		cardName = variants[0]
		if magic.IsBasicLand(cardName) {
			edition = "European Land Program"
			if strings.Contains(cardName, "APAC") {
				edition = "Asia Pacific Land Program"
			}
			fields := strings.Fields(cardName)
			cardName = fields[0]
			variation = variants[1]
		}
	// Ravnica weekend lands, move to variation
	case strings.Contains(cardName, "Ravnica Weekend"):
		fields := strings.Fields(cardName)
		cardName = fields[0]
		variation = strings.Join(fields[1:], " ")
	case strings.HasPrefix(cardName, "B.F.M.") && strings.Contains(cardName, "#"):
		for field := range strings.FieldsSeq(cardName) {
			if strings.HasPrefix(field, "#") {
				cardName = "B.F.M."
				variation = field[1:]
				break
			}
		}
	}

	ed, found := card2setTable[cardName]
	if found {
		edition = ed
	}

	variants := matcher.SplitVariants(cardName)
	cardName = variants[0]
	if len(variants) > 1 {
		if variation != "" {
			variation += " "
		}
		variation += strings.Join(variants[1:], " ")
	}

	variation = neonInkWording(variation)

	// Repeat to catch numbers
	if magic.IsBasicLand(cardName) {
		num := matcher.ExtractNumber(cardName)
		if num != "" {
			cardName = strings.Replace(cardName, num, "", 1)
			cardName = strings.TrimSpace(cardName)
			if variation != "" {
				variation += " "
			}
			variation += num
		}

		for _, tag := range tagsTable {
			if strings.HasSuffix(cardName, tag) {
				cardName = strings.TrimSuffix(cardName, " "+tag)
				if variation != "" {
					variation += " "
				}
				variation += tag
				break
			}
		}
	}

	switch variation {
	case "6E", "VI DCI", "DCI", "US":
		switch cardName {
		case "Crusade",
			"Lord of Atlantis",
			"Serra Avatar",
			"Thran Quarry",
			"Two-Headed Dragon":
			edition = "Junior Super Series"
		case "Forest",
			"Island",
			"Mountain",
			"Swamp",
			"Plains":
			edition = "Arena League 1999"
		case "Argothian Enchantress",
			"Balance",
			"Ball Lightning",
			"Gaea's Cradle",
			"Intuition",
			"Living Death",
			"Memory Lapse",
			"Oath of Druids",
			"Stroke of Genius",
			"Tradewind Rider":
			variation = "Judge"
		case "Vampiric Tutor":
			variation = "Judge 2000"
		case "Arc Lightning",
			"Chill",
			"Duress",
			"Enlightened Tutor",
			"Karn, Silver Golem",
			"Mana Leak",
			"Rewind",
			"Serum Visions",
			"Stupor":
			variation = "Arena"
		case "Powder Keg",
			"Voidmage Prodigy",
			"Wasteland":
			variation = "Magic Player Rewards"
		case "Zoetic Cavern":
			variation = "DCI Promos"
		default:
			variation = "FNM"
		}
	case "DotP 2012 - Xbox", "X Box Promo 2013", "X Box Promo",
		"Playstation", "PS3 Promo",
		"Duels of the Planeswalkers - PC",
		"Duel of the Planeswalkers":
		variation = "Duels"
	}

	switch {
	case strings.HasPrefix(variation, "The "):
		edition = "Magic Premiere Shop 2005"
	case strings.Contains(variation, "Holiday"):
		edition = "Happy Holidays"
	case matcher.HasPrefix(cardName, "Teferi Master of Time"):
		if edition == "Promotional Cards" {
			variation += "s"
		} else if edition == "Promo Pack" {
			variation += "p"
		}
	}

	printing, found := shelfPrintings[[3]string{edition, cardName, variation}]
	if found {
		edition, variation = printing[0], printing[1]
	}

	switch edition {
	case "Promotional Cards":
		if variation == "" {
			ed, found = promo2setTable[cardName]
			if found {
				edition = ed
			}
		}
	case "Ikoria: Lair of Behemoths":
		if strings.Contains(cardName, " - ") {
			s := strings.Split(cardName, " - ")
			cardName = s[1]
			variation = "Godzilla"
			if strings.Contains(notes, "Japanese") {
				variation += " Japanese"
			}
		}
	case "Promos: Play":
		edition = "Promotional"
		variation = "playpromo"
		printing, unique := promoPrinting(b, cardName, magic.PromoTypePlayPromo)
		if unique {
			edition, variation = printing.SetCode, printing.Number
		}
	case "Promos: Standard Showdown":
		printing, unique := promoPrinting(b, cardName, magic.PromoTypeStandardShowdown)
		if unique {
			edition, variation = printing.SetCode, printing.Number
		}
	case "Promos: Champs":
		edition = "PCMP"
	case "Promos: Pro Tour":
		// A plain Pro Tour listing keeps PPRO unless the wording names an
		// RCQ series (Snapcaster Mage sits in both PPRO and PR23).
		rcq := matcher.Contains(variation, "Regional Championship") || matcher.Contains(variation, "RCQ")
		for _, code := range []string{"PPRO", "SLP", "LTR", "PRCQ", "PR23"} {
			if len(b.MatchInSet(cardName, code)) == 0 {
				continue
			}
			if edition == "PPRO" && (code == "PRCQ" || code == "PR23") && !rcq {
				continue
			}
			edition = code
		}
		// LTR holds the tournament promo beside the plain printing.
		if edition == "LTR" {
			printing, unique := promoPrinting(b, cardName, magic.PromoTypeTourney)
			if unique {
				variation = printing.Number
			}
		}
		// This category never spells out the Secret Lair drop, so say Play.
		if edition == "SLP" {
			variation = strings.TrimSpace(variation + " Play")
		}
	case "Promos: Media":
		// SDCC wording resolves through the core SDCC rule instead.
		if !matcher.Contains(variation, "SDCC") {
			// The last hit wins, so PF25 goes first and a media insert of the
			// same card keeps it.
			for _, code := range []string{
				"PF25", "PHPR", "PMEI", "PURL",
				"PDTP", "PDP10", "PDP12", "PDP13", "PDP14", "PDP15",
			} {
				if len(b.MatchInSet(cardName, code)) > 0 {
					edition = code
				}
			}
			// What the codes miss is the resale promo of a set's release.
			if edition == "Promos: Media" {
				printing, unique := promoPrinting(b, cardName, magic.PromoTypeResale)
				if unique {
					edition, variation = printing.SetCode, printing.Number
				}
			}
		}
	case "Promos: Junior Series":
		for _, code := range []string{"PSUS"} {
			if len(b.MatchInSet(cardName, code)) > 0 {
				edition = code
			}
		}
	case "Promos: Arena":
		cardName = strings.TrimSuffix(cardName, " Arena Promo")
	case "Promos: Judge":
		cardName = strings.TrimSuffix(cardName, " Full Art")
		// The shelf lists the 2018 copy with its code, so the bare name is
		// the 2000 one.
		if cardName == "Vampiric Tutor" && variation == "" {
			edition = "G00"
		}
	case "Promos: Planeswalker Event":
		edition = "PWCS"
		if variation == "Top 8" {
			variation = ""
		}
	case "Promos: Unique and Miscellaneous":
		switch cardName {
		case "Lotus Petal":
			edition = "P30M"
		case "Serra Angel":
			edition = "PWOS"
		case "J. Jonah Jameson":
			edition = "PF25"
		case "Flooded Strand":
			edition = "PNAT"
		}
	case "Promos: Launch Party and Release Event":
		if magic.IsBasicLand(cardName) {
			edition = "Ravnica Weekend"
		}
	case "Promos: WPN and Gateway":
		switch cardName {
		case "Orb of Dragonkind":
			edition = "PLG21"
			variation = "J" + strings.TrimLeft(variation, "0")
		case "Cavern of Souls":
			edition = "LCI"
			variation = "410b"
		}
	case "Promos: 30th Anniversary Promos":
		// The shelf calls a History promo's starred copy "Retro Frame", which
		// P30A's datestamped retro copies would otherwise take
		if matcher.Contains(variation, "Retro Frame") && len(b.MatchInSet(cardName, "P30H")) > 0 {
			edition = "P30H"
			variation = "Retro Frame"
		}
		// Serra Angel is the one History promo P30A also numbers (001), and
		// the shelf lists its two copies as the plain one and the Retro Frame.
		if cardName == "Serra Angel" && variation == "001" {
			edition = "P30H"
			variation = "1"
		}
	case "Promos: Magicfest":
		// The plain Arcane Signet is a shelfPrintings row; the etched one is
		// steered here, its variation naming the finish.
		if cardName == "Arcane Signet" && matcher.Contains(variation, "Etched") {
			edition = "P30M"
		}
	case "Alpha", "Beta":
		// A bare basic land is the first art, the others being listed as B
		// (and C).
		if variation == "" && magic.IsBasicLand(cardName) && !strings.Contains(cardName, " ") {
			variation = "A"
		}
	case "Aetherdrift":
		// The Aetherdrift shelf also holds the Spotlight Series Chrome Mox.
		if cardName == "Chrome Mox" {
			edition = "SPG"
		}
	case "Duel of the Planeswalkers":
		edition = "Duels of the Planeswalkers"
	case "Hours of Devestation":
		edition = "HOU"
	case "Secret Lair Commander: Heads I Win":
		edition = "Secret Lair Commander: Heads I Win, Tails You Lose"
		// A set code names the set the card was reprinted from.
		_, isSet := b.Sets[variation]
		if isSet && len(b.MatchInSet(cardName, "PLST")) > 0 {
			edition = "PLST"
		}
	case "Secret Lair Commander: From Cute to Brute":
		if slices.Contains(fromCuteToBrutePathways, cardName) {
			edition = "PLST"
		}
	case "Battlebond":
		if strings.HasSuffix(cardName, "Alternate Art") {
			cardName = strings.TrimSuffix(cardName, " Alternate Art")
		}
	}

	// Second pass in case some tags interfered with the lookup
	cn, found = cardTable[cardName]
	if found {
		cardName = cn
	}

	if variation == "Extemded Art" {
		variation = "Extended Art"
	}

	// Set finish
	// Etched is its own finish, so an etched listing is not also a foil one,
	// whether the notes or the card name ("(Foil Etched)") say so.
	lowNotes := strings.ToLower(notes)
	etchedNote := strings.Contains(lowNotes, "etched")
	if etchedNote {
		if variation != "" {
			variation += " "
		}
		variation += "Etched"
	}
	isFoil := strings.Contains(lowNotes, "foil") && !etchedNote && !matcher.Contains(variation, "Etched")

	var language string
	for lang := range matcher.LanguageTag2LanguageCode {
		// Skip empty (or it would match everything) and skip English as sometimes
		// non-English cards are mistakenly tagged as such
		if lang != "" && lang != "English" && strings.Contains(notes, lang) {
			language = lang
			break
		}
	}

	// Secret Lair files a card under one drop or under several, and the store
	// says which only where it writes something beside the name. Where the
	// set holds several, the match that follows picks one of them for no
	// reason and prices the others as it, so refuse instead of choosing,
	// unless the finish on sale leaves one of them to be it.
	if edition == "Secret Lair" && variation == "" {
		drops, inFinish := secretLairDrops(b, cardName, droppedFlavor, requestedFinish(isFoil, variation))
		if len(drops) > 1 {
			if len(inFinish) != 1 {
				return nil, matcher.ErrUnsupported
			}
			variation = inFinish[0]
		}
		// A card Secret Lair files only under Play has no drop to pick.
		otherDrop := slices.ContainsFunc([]string{"SLU", "SLX", "SLC"}, func(code string) bool {
			return len(b.MatchInSet(cardName, code)) > 0
		})
		if len(drops) == 0 && !otherDrop && len(b.MatchInSet(cardName, "SLP")) == 1 {
			edition, variation = "SLP", "Play"
		}
	}

	return &matcher.InputCard{
		Name:      cardName,
		Variation: variation,
		Edition:   edition,
		Foil:      isFoil,
		Language:  language,
	}, nil
}

// promoPrinting returns the one printing of the card wearing the promo type,
// and false when the card has none or several to tell apart.
func promoPrinting(b *matcher.Backend, cardName, promoType string) (matcher.Card, bool) {
	var found []matcher.Card
	sets, err := b.Printings4Card(cardName)
	if err != nil {
		return matcher.Card{}, false
	}
	for _, code := range sets {
		for _, card := range b.MatchInSet(cardName, code) {
			if card.HasPromoType(promoType) {
				found = append(found, card)
			}
		}
	}
	if len(found) != 1 {
		return matcher.Card{}, false
	}
	return found[0], true
}

// isFlavorOf reports whether the catalog sells the card heading tail under
// the flavor name head. The search indexes a reversible card's flavor only
// doubled ("Optimus Prime // Optimus Prime"), but Match reads the bare one
// off the whole name, as it does every other flavor name.
func isFlavorOf(b *matcher.Backend, head, tail string) bool {
	alt, found := b.AlternateProps[head]
	return found && alt.IsFlavor && matcher.Equals(alt.OriginalName, matcher.SplitVariants(tail)[0])
}

// secretLairDrops returns the distinct drops the set files a card under, and
// those of them printed in finish. The suffixes a number can end on - the
// star of a foil twin, the phi of a step-and-compleat - mark twins the
// wording picks, not drops of their own, and PlainNumber is the number with
// all of them already stripped.
//
// A flavor-named drop does not count beside the card's own, which the matcher
// lands a listing naming no flavor on - unless preprocess cut one off the
// name (droppedFlavor), which the match then never sees.
func secretLairDrops(b *matcher.Backend, cardName string, droppedFlavor bool, finish string) (drops, inFinish []string) {
	cards := b.MatchInSet(cardName, "SLD")
	var unflavored []matcher.Card
	for _, card := range cards {
		if card.FlavorName == "" {
			unflavored = append(unflavored, card)
		}
	}
	if len(unflavored) > 0 && !droppedFlavor {
		cards = unflavored
	}
	for _, card := range cards {
		if !slices.Contains(drops, card.PlainNumber) {
			drops = append(drops, card.PlainNumber)
		}
		if card.HasFinish(finish) && !slices.Contains(inFinish, card.PlainNumber) {
			inFinish = append(inFinish, card.PlainNumber)
		}
	}
	return drops, inFinish
}

// retroFrameVersion is the frame the catalog files the retro printings under.
const retroFrameVersion = "1997"

// treatmentClaims pairs a word the store writes with what the catalog files
// that treatment under. The store spells these out only where it is selling
// the printing wearing one, so they are read as claims and never as denials.
//
// Only the words the catalog answers as a promo type belong here. The store
// also writes "Rainbow Foil" and "Confetti Foil", which name the finish a
// Secret Lair is sold in rather than a treatment the printing wears, and
// reading those as claims refuses 62 of its listings for saying so.
var treatmentClaims = []struct {
	tag   string
	wears func(co *matcher.CardObject) bool
}{
	// The serialized printings carry the retro look in frame 2003.
	{"Retro Frame", func(co *matcher.CardObject) bool {
		return co.FrameVersion == retroFrameVersion ||
			(co.FrameVersion == "2003" && co.HasPromoType(magic.PromoTypeSerialized))
	}},
	{"Textured", func(co *matcher.CardObject) bool {
		return co.HasPromoType(magic.PromoTypeTextured)
	}},
	{"Surge Foil", func(co *matcher.CardObject) bool {
		return co.HasPromoType(magic.PromoTypeSurgeFoil)
	}},
}

// namesAbsentTreatment reports whether the listing spells out a treatment the
// printing it was answered with does not wear. The store says these words
// about the card in hand, so an answer without one is a different card, and
// the price it carries belongs to that card rather than this listing.
func namesAbsentTreatment(variation string, co *matcher.CardObject) bool {
	for _, claim := range treatmentClaims {
		if matcher.Contains(variation, claim.tag) && !claim.wears(co) {
			return true
		}
	}
	return false
}

// wearsUnnamedTextured reports whether the answer is a textured printing the
// listing never called textured. The store spells the word out on every one
// of these it stocks, so silence beside a card the set also prints untextured
// names that other printing, and the textured one is only what the wording
// reached for want of anywhere else to land.
//
// A number the wording carries names the printing as plainly as the word
// itself would, so it stands aside there the same way: the store has not
// been seen writing one for these listings, but a number matching the
// answer's own is a claim, not the silence this guard exists to catch.
func wearsUnnamedTextured(b *matcher.Backend, variation string, co *matcher.CardObject) bool {
	if !co.HasPromoType(magic.PromoTypeTextured) || matcher.Contains(variation, "Textured") {
		return false
	}
	if number := matcher.ExtractNumber(variation); number != "" && number == co.Number {
		return false
	}
	return slices.ContainsFunc(b.MatchInSet(co.Name, co.SetCode), func(card matcher.Card) bool {
		return !card.HasPromoType(magic.PromoTypeTextured)
	})
}

// premiumFoilTreatments are the foil-only treatments this storefront always
// spells out; a candidate wearing one the variation is silent about is not the
// card being sold.
var premiumFoilTreatments = []struct {
	promoType string
	tag       string
}{
	{magic.PromoTypeSurgeFoil, "Surge"},
	{magic.PromoTypeTextured, "Textured"},
	{magic.PromoTypeRaisedFoil, "Raised"},
	{magic.PromoTypeManaFoil, "Mana Foil"},
	{magic.PromoTypeFractureFoil, "Fracture"},
	{magic.PromoTypeHaloFoil, "Halo"},
	{magic.PromoTypeGalaxyFoil, "Galaxy"},
	{magic.PromoTypeHeadliner, "Headliner"},
	{magic.PromoTypeNeonInk, "Neon"},
	{magic.PromoTypeFirstPlaceFoil, "First Place"},
	{magic.PromoTypeStepAndCompleat, "Compleat"},
}

// requestedFinish is the finish a listing is on sale in.
func requestedFinish(foil bool, variation string) string {
	switch {
	case matcher.Contains(variation, "Etched"):
		return matcher.FinishEtched
	case foil:
		return matcher.FinishFoil
	}
	return matcher.FinishNonfoil
}

// finishPrinted reports whether the printing a Magic listing resolved to was
// sold in the finish the listing named, mirroring gamenerdz's guard of the
// same name. A mismatch only means a wrong card when the landed set holds
// another printing of the name in the requested finish, or a sibling row on
// this shelf already prices it there; otherwise the printing is the only one
// this name and set ever had, so dropping it would lose its only price.
func finishPrinted(b *matcher.Backend, co *matcher.CardObject, foil bool, variation string, hasFoilSibling bool) bool {
	requested := requestedFinish(foil, variation)
	if co.HasFinish(requested) {
		return true
	}
	if slices.ContainsFunc(b.MatchInSet(co.Name, co.SetCode), func(card matcher.Card) bool {
		return card.HasFinish(requested)
	}) {
		return false
	}
	return !hasFoilSibling
}
