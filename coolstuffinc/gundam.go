package coolstuffinc

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// gundamShelfCode is the set code this storefront opens a Gundam shelf with,
// "GD01 - Newtype Rising". The catalog names the set by its title alone, so
// the code narrows nothing and leaves the shelf naming no set at all - and
// this game prints the same card at the same number in GD01, its beta
// edition and the deck build box, so a shelf that narrows nothing leaves the
// number aliasing three ways.
var gundamShelfCode = regexp.MustCompile(`^[A-Za-z]{1,4}-?[0-9]{1,2}\s*-\s+`)

// gundamShelf reads the set a Gundam shelf names.
func gundamShelf(shelf string) string {
	return strings.TrimSpace(gundamShelfCode.ReplaceAllString(shelf, ""))
}

// gundamNameCode is the collector number this storefront writes inside the
// name as well as in its own field. It is also the divider: what stands
// before it is the card's own name, parentheticals and all, and what stands
// after it is the wording naming the run.
var gundamNameCode = regexp.MustCompile(`\s*\(([A-Z]+[0-9]*-[0-9]+[a-z]?)\)\s*`)

// gundamNames are the names this storefront spells its own way. Every entry
// is one letter or one word off the catalog, keyed literally: a storefront
// typo is a typo of one product, and matching it by shape would answer for
// the cards it is a near-miss of.
var gundamNames = map[string]string{
	"Adbul's Maganac":                                                   "Abdul's Maganac",
	"Tiffa Adill & Freedom":                                             "Tiffa Adill & Freeden",
	`Prototype Asshimar TR-3 "Keharr"`:                                  `Prototype Asshimar TR-3 "Kehaar"`,
	"Elan Ceres (Enchanted Person Number 5)":                            "Elan Ceres (Enhanced Person Number 5)",
	"AI-Saachez's AEU Enact Custom Moralia Development Experiment Type": "Al-Saachez's AEU Enact Custom Moralia Development Experiment Type",
	"Patimus Scirocco":                                                  "Paptimus Scirocco",
}

// gundamSymbolWord is the storefront naming a glyph the catalog writes out.
// The Xi Gundam and the Turn A Gundam wear a Greek letter and a turned A on
// their faces, and this storefront types the glyph's name in brackets -
// "Xi (Symbol) Gundam", "(Turn A Symbol) Gundam" - where the catalog spells
// the reading straight through.
var gundamSymbolWord = regexp.MustCompile(`\s*\(([^)]*?)\s*Symbol\)\s*`)

// gundamFusedName is the storefront numbering each half of a card printing
// two: "Guncannon (108) & Guncannon (109)". The catalog keeps the two
// numbers behind the joined name instead, so the halves' numbers move to
// the tail.
var gundamFusedName = regexp.MustCompile(`^(.+?)\s*\((\d+)\)\s*&\s*(.+?)\s*\((\d+)\)$`)

// gundamName spells a card the way the catalog does.
func gundamName(name string) string {
	if spelled, found := gundamNames[name]; found {
		return spelled
	}
	if gundamSymbolWord.MatchString(name) {
		name = strings.TrimSpace(gundamSymbolWord.ReplaceAllString(name, " $1 "))
		name = strings.Join(strings.Fields(name), " ")
	}
	if halves := gundamFusedName.FindStringSubmatch(name); halves != nil {
		name = fmt.Sprintf("%s & %s (%s) (%s)", halves[1], halves[3], halves[2], halves[4])
	}
	return name
}

// gundamNumberDigits is a collector number as this game writes them, the run
// code and three digits. Every one of the 1,866 printings the datastore
// carries is spelled that way, and nothing carries a letter tail, so a
// number that is not is the storefront's own typing.
var gundamNumberDigits = regexp.MustCompile(`^([A-Za-z0-9]+)-0*([0-9]+)([A-Za-z]?)$`)

// gundamNumberSpelling writes a collector number the way the catalog does.
// The storefront drops a digit ("GD02-57"), types an extra zero
// ("EXBP-0013"), and letters a parallel onto the number the catalog letters
// nothing ("R-008A", whose rarity column already says C+). A letter is only
// dropped where the name spells the same number without one, so a real
// lettered number - were the game ever to print one - is left alone.
func gundamNumberSpelling(number, name string) string {
	fields := gundamNumberDigits.FindStringSubmatch(number)
	if fields == nil || len(fields[2]) > 3 {
		return number
	}
	spelled := fmt.Sprintf("%s-%03s", fields[1], fields[2])
	if fields[3] != "" && !strings.Contains(name, spelled) {
		return number
	}
	return spelled
}

// gundamCard splits what the storefront writes into the card's own name and
// the wording that picks between the printings sharing its number.
//
// The split is by position rather than by shape. A card whose own name ends
// in a parenthetical - "GQuuuuuuX (Omega Psycommu)", "Unicorn Gundam
// (Destroy Mode)" - is indistinguishable from a run's wording by looking at
// the brackets alone, and taking the last one asks for a card the catalog
// does not have. The number is written between the two, so it says which is
// which.
func gundamCard(b *matcher.Backend, name, number string) (string, string) {
	written := name
	variation := gundamNumberSpelling(number, written)
	loc := gundamNameCode.FindStringSubmatchIndex(name)
	if loc != nil {
		// The sell listing carries no number of its own, and this is the
		// same number the buylist files in its own field.
		if variation == "" {
			variation = gundamNumberSpelling(name[loc[2]:loc[3]], written)
		}
		num := variation
		cardName := gundamName(strings.TrimSpace(name[:loc[0]]))
		for _, wording := range gundamQualifier.FindAllStringSubmatch(name[loc[1]:], -1) {
			variation = strings.TrimSpace(variation + " " + gundamWording(b, cardName, num, wording[1]))
		}
		name = cardName
	} else {
		name = gundamName(name)
	}
	// The token shelf names the art the token wears where the catalog names
	// the token, and the number is what says which it is.
	if strings.HasPrefix(variation, "T-") {
		variation = strings.TrimSpace(variation + " Token")
		name = gundamTokenName(name)
	}
	return strings.TrimSpace(name), strings.TrimSpace(variation)
}

// gundamWCSCode is the World Championship's own code, which the season
// changes ("WCS26-27") where the label behind it - "Participation Pack",
// "Regional Championship" - does not.
var gundamWCSCode = regexp.MustCompile(`^WCS[0-9-]*`)

// gundamNicknames are the promo runs this storefront names after the art,
// one product per nickname: the Green Dress Cagalli is the GD01-096 of the
// first Championship Participation Pack.
var gundamNicknames = map[string]string{
	"Green Dress": "Championship Participation Pack 01",
}

// gundamWording spells a storefront's own code for a promo run the way the
// catalog's label reads, so the wording match in FilterCards can reach the
// printing it names.
func gundamWording(b *matcher.Backend, name, number, wording string) string {
	if label, found := gundamNicknames[wording]; found {
		return label
	}
	switch {
	case wording == "PB01" || wording == "PB02":
		label := gundamPremiumBandai(b, name, number)
		if label != "" {
			return label
		}
	case gundamWCSCode.MatchString(wording):
		return gundamWCSCode.ReplaceAllString(wording, "World Championship")
	}
	return wording
}

// gundamPremiumBandaiTypes are the two products CSI shelves under its own
// "PB01" run code: the Premium Card Collection insert and the Premium
// Accessory Set. A number carries at most one of the two.
var gundamPremiumBandaiTypes = []string{"premiumcardcollection", "premiumaccessory"}

// gundamPremiumBandai spells "PB01" as whichever of the two the catalog
// files at this number, or "" where neither does.
func gundamPremiumBandai(b *matcher.Backend, name, number string) string {
	uuids, err := b.SearchEquals(name)
	if err != nil {
		return ""
	}
	for _, uuid := range uuids {
		co, err := b.GetUUID(uuid)
		if err != nil || !strings.EqualFold(co.Number, number) {
			continue
		}
		for _, promoType := range gundamPremiumBandaiTypes {
			if co.HasPromoType(promoType) {
				return b.PromoTypeLabel(promoType)
			}
		}
	}
	return ""
}

// gundamTokenQualifier is the parenthetical a token's own name ends in,
// which the catalog writes behind the word Token rather than in front of it.
var gundamTokenQualifier = regexp.MustCompile(`\s*(\([^)]*\))$`)

// gundamTokenName writes a token the way the catalog names it. A token whose
// name carries no qualifier is reached by the word alone, which Match adds
// for itself; one that does has to have the word put in the right place,
// "GQuuuuuuX (Omega Psycommu)" being filed as "GQuuuuuuX Token (Omega
// Psycommu)".
func gundamTokenName(name string) string {
	fields := gundamTokenQualifier.FindStringSubmatch(name)
	if fields == nil {
		return name
	}
	return strings.TrimSpace(strings.TrimSuffix(name, fields[0])) + " Token " + fields[1]
}

// gundamQualifier is one bracketed wording, of however many the storefront
// hangs behind the number - "(Alt-Art +)", "(SP)", "(Beam Blast)".
var gundamQualifier = regexp.MustCompile(`\(([^)]*)\)`)

// gundamRarity maps the storefront's spelling of a rarity onto the catalog's.
// This game files a parallel run at the base printing's number and tells the
// two apart by suffixing the rarity, so the column is what picks between
// them - and the storefront abbreviates the suffix the catalog writes out.
var gundamRarity = map[string]string{
	"CP":    "C+",
	"CPP":   "C++",
	"UP":    "U+",
	"RP":    "R+",
	"LGRP":  "LR+",
	"LGRPP": "LR++",
	"U":     "Uncommon",
}

// gundamParallelCodes are the letters the catalog spells a base rarity's
// parallel with, "C+" being the Common's.
var gundamParallelCodes = map[string]string{
	"Common":      "C",
	"Uncommon":    "U",
	"Rare":        "R",
	"Legend Rare": "LR",
}

// gundamPlusRun is the parallel marker a name carries, "(Alt-Art +)".
var gundamPlusRun = regexp.MustCompile(`\s(\++)\)`)

// gundamTier reads the rarity the storefront publishes beside a Gundam card.
// A few parallels carry the base rarity in that column while the name says
// "+", and the column alone would answer with the base card.
func gundamTier(rarity, name string) string {
	rarity = strings.TrimSpace(rarity)
	if spelled, found := gundamRarity[rarity]; found {
		rarity = spelled
	}
	code, found := gundamParallelCodes[rarity]
	if !found {
		return rarity
	}
	run := gundamPlusRun.FindStringSubmatch(name)
	if run == nil {
		return rarity
	}
	return code + run[1]
}

// gundamNumberNotes is the collector number as the sell listing's notes write
// it. Every number this catalog carries spells three digits, which is what
// tells one from the art the token shelf spends the same field on.
var gundamNumberNotes = regexp.MustCompile(`^[A-Za-z]+[0-9]*-[0-9]{3}[a-zA-Z]*$`)

// gundamNumber reads the collector number out of a sell listing's notes,
// which spell it in full where the name sometimes drops a digit.
func gundamNumber(notes string) string {
	for _, field := range strings.Fields(notes) {
		field = strings.Trim(field, "()[],.")
		if gundamNumberNotes.MatchString(field) {
			return field
		}
	}
	return ""
}

// gundamPIDs names the TCGplayer product a storefront product is, for the
// starter deck reprints (ST05, ST07, ST08 and most of all ST09) it shelves
// under the card's first set and tells apart only by a description of the
// art, which names no label the catalog could be matched on. The product id
// is the storefront's own, shared by both feeds.
var gundamPIDs = map[string]string{
	"435239": "684017", // GD01-049 Blitz Gundam (Dodging Yellow Laser) = ST09
	"435240": "684018", // GD01-054 Duel Gundam (Smoke Cloud)
	"435241": "684019", // GD01-111 Battle of Aces (Two Mechs Fighting)
	"435242": "684020", // GD01-118 Overflowing Affection (Singer in Mech's Hand)
	"435243": "684021", // GD02-076 Buster Gundam (Shot by Green Laser)
	"435244": "684022", // GD02-110 Awakened Power (Mech with Sword in Ground)
	"435222": "684010", // ST01-014 Unforeseen Incident (Two People in Red Outfits)
	"435223": "684011", // ST04-002 Strike Gundam (In Water)
	"435225": "684013", // ST04-010 Kira Yamato (Two People in Blue and Red Outfits)
	"435226": "684014", // ST04-011 Athrun Zala (Two People in Red and Blue Outfits)
	"435227": "684015", // ST04-015 Archangel (Explosions)
	"435228": "684016", // ST04-016 Vesalius (Up Close with Lasers)
	"424310": "653635", // ST01-014 Unforeseen Incident (Purple Hair) = ST05
	"431164": "671926", // ST05-014 Fatal Strike (Reflection) = ST07
	"431198": "672000", // ST05-014 Fatal Strike (Alt-Art +) (Reflection) = ST07
	"431220": "671979", // ST02-014 Siege Ploy (Alt-Art +) (Sword Fight) = ST08
}

// retailGundam answers an offer by its product id where the wording cannot
// reach it.
func retailGundam(csi *Coolstuffinc, l sellListing) (*matcher.InputCard, string, error) {
	var theCard *matcher.InputCard
	tcgID, named := gundamPIDs[l.pid]
	if named {
		theCard = &matcher.InputCard{ID: tcgID, Foil: l.foil}
	} else {
		name, variation := gundamCard(csi.backend, l.name, gundamNumber(l.notes))
		theCard = &matcher.InputCard{Name: name, Edition: gundamShelf(l.edition), Variation: strings.TrimSpace(variation + " " + l.notes + " " + gundamTier(l.rarity, l.name)), Foil: l.foil}
	}
	cardID, err := l.match(csi.backend, theCard)
	return theCard, cardID, err
}

// buylistGundam reads a buylist row the way retailGundam reads an offer.
// Gundam prints the same card at the same number in three sets, so the shelf
// has to narrow and the storefront's own code prefix stops it naming one; the
// wording it hangs behind the name is what tells the parallel runs apart.
func buylistGundam(csi *Coolstuffinc, p CSIPriceEntry) (*matcher.InputCard, string, error) {
	var theCard *matcher.InputCard
	tcgID, named := gundamPIDs[p.PID]
	if named {
		theCard = &matcher.InputCard{ID: tcgID, Foil: p.IsFoil == 1}
	} else {
		name, variation := gundamCard(csi.backend, p.Name, p.Number)
		theCard = &matcher.InputCard{Name: name, Edition: gundamShelf(p.ItemSet), Variation: strings.TrimSpace(variation + " " + gundamTier(p.RarityName, p.Name)), Foil: p.IsFoil == 1}
	}
	cardID, err := csi.backend.Match(theCard)
	return theCard, cardID, err
}
