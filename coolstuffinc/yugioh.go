package coolstuffinc

import (
	"errors"
	"regexp"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// yugiohPIDs names the TCGplayer product a storefront product is, for the
// listings whose wording cannot reach it: the storefront tells the arts of one
// number apart by the background it describes where the catalog numbers them
// "Version N" or "A, B, C", and its Promo shelf cannot tell a set from the date
// reprint made of it later. The product id is the storefront's own and both
// feeds share it.
var yugiohPIDs = map[string]string{
	"255629": "162087", // LCKC-EN001 Blue-Eyes White Dragon (Blue Ripple Background) = Version 2
	"255628": "161927", // LCKC-EN001 (Red Sparks Background) = Version 1
	"255630": "162088", // LCKC-EN001 (Space and Earth Background) = Version 3
	"255626": "162089", // LCKC-EN001 (Tablet Background) = Version 4
	"255382": "161972", // LCKC-EN046 Crush Card Virus (Yellow and Orange Background) = Version 1
	"255631": "162086", // LCKC-EN046 (Red and Black Background) = Version 2
	"233930": "123525", // LDK2-ENK01 Blue-Eyes White Dragon (Blue Ripple Background) = Version 2
	"233816": "123621", // LDK2-ENK01 (Red Sparks Background) = Version 1
	"233929": "123620", // LDK2-ENK01 (Tablet Background) = Version 4
	"402767": "593847", // RA03-EN123 Dark Magician Girl (Platinum Secret Rare) Gold Circle Background = B
	"402768": "593849", // RA03-EN123 (Quarter Century Secret Rare) Gold Circle Background = B
	"402769": "592812", // RA03-EN123 (Platinum Secret Rare) Giant Hearts Background = A
	"402770": "592813", // RA03-EN123 (Quarter Century Secret Rare) Giant Hearts Background = A
	"402771": "593848", // RA03-EN123 (Platinum Secret Rare) Little Hearts, Raising Staff Up = C
	"402772": "593850", // RA03-EN123 (Quarter Century Secret Rare) Little Hearts, Raising Staff Up = C
	"255618": "161922", // LC06-EN001 Goddess of Sweet Revenge, not the 2020 date reprint
	"255619": "161923", // LC06-EN002 The King of D.
	"255622": "161926", // LC06-EN003 Destruction Dragon
	"255621": "161925", // LC06-EN004 Dragon Revival Rhapsody
	"255620": "161924", // LC06-EN005 Loop of Destruction
	"222257": "108333", // YGLD Glory of the King's Hand, not the 2021 date reprint
	"222308": "108334", // YGLD Set Sail for The Kingdom
	"222244": "108335", // YGLD Duelist Kingdom
	"112537": "80163",  // RGBT-ENPP1 Iron Core of Koa'ki Meiru, sold as the Raging Battle card
	"112538": "80160",  // RGBT-ENPP2 Blackwing - Shura the Blue Flame
	"112539": "80293",  // RGBT-ENPP3 Koa'ki Meiru Guardian
	"112540": "80162",  // RGBT-ENPP4 Moja
	"112541": "174115", // RGBT-ENPP5 Master Gig
	"112542": "153436", // RGBT-ENPP6 Level Retuner
	"292911": "207877", // MVP1-ENSV4 Blue-Eyes White Dragon, the Secret Edition art the buylist numbers
	"264850": "175258", // MVP1-ENSE4 Blue-Eyes White Dragon, the Special Edition art the sku numbers
	"207108": "92623",  // MP14-EN073 Bujingi Crane, sold as the 2014 Mega-Tins card
	"264847": "175255", // MVP1-ENSE1 Duza the Meteor Cubic Vessel
	"264848": "175256", // MVP1-ENSE2 Krystal Dragon
	"264849": "175257", // MVP1-ENSE3 Dark Magician
	"292910": "207876", // MVP1-ENSV3 Dark Magician
	"292913": "207878", // MVP1-ENSV6 Slifer the Sky Dragon
	"207123": "92650",  // MP14-EN100 Bujintei Susanowo, not the CT11 Secret Rare
	"334728": "266073", // SGX1-ENA12 Polymerization, whose catalog name keeps the deck letter
	"334730": "266075", // SGX1-ENA14 Reinforcement of the Army
	"282814": "196260", // SS03-ENA22 White Elephant's Gift
	"114599": "26594",  // STON-ENSP1 Grandmaster of the Six Samurai (Super Rare), not the Secret Rare STON-EN000
	"412920": "626808", // RA04-EN024 Aleister the Invoker (Platinum Secret Rare) Facing Forward, not EN278
	"412921": "626809", // RA04-EN024 (Quarter Century Secret Rare) Facing Forward
	"413375": "627267", // RA04-EN106 Dark Magician (Platinum Secret Rare) Red Armor = Arkana
	"413376": "627272", // RA04-EN106 (Quarter Century Secret Rare) Red Armor = Arkana
	"413377": "627268", // RA04-EN106 (Platinum Secret Rare) Magic Ring Around Body = 6th Art
	"413378": "627273", // RA04-EN106 (Quarter Century Secret Rare) Magic Ring Around Body = 6th Art
	"413379": "627271", // RA04-EN106 (Platinum Secret Rare) Pointing Staff with One Hand = 9th Art
	"413380": "627276", // RA04-EN106 (Quarter Century Secret Rare) Pointing Staff with One Hand = 9th Art
	"413381": "627269", // RA04-EN106 (Platinum Secret Rare) Black Armor, Black Staff = 7th Art
	"413382": "627274", // RA04-EN106 (Quarter Century Secret Rare) Black Armor, Black Staff = 7th Art
	"413383": "627270", // RA04-EN106 (Platinum Secret Rare) Both Hands on Staff = 8th Art
	"413384": "627275", // RA04-EN106 (Quarter Century Secret Rare) Both Hands on Staff = 8th Art
	"413385": "627278", // RA04-EN107 Dark Magician Girl (Platinum Secret Rare) Pointing Staff to Top Left = 7th Art
	"413386": "627280", // RA04-EN107 (Quarter Century Secret Rare) Pointing Staff to Top Left = 7th Art
	"413387": "627277", // RA04-EN107 (Platinum Secret Rare) Holding Hat = 3rd Art
	"413388": "627279", // RA04-EN107 (Quarter Century Secret Rare) Holding Hat = 3rd Art
	"413389": "627281", // RA04-EN108 Blue-Eyes White Dragon (Platinum Secret Rare) Facing Left = 7th Art
	"413390": "627283", // RA04-EN108 (Quarter Century Secret Rare) Facing Left = 7th Art
	"413391": "627282", // RA04-EN108 (Platinum Secret Rare) Facing Right = 8th Art
	"413392": "627284", // RA04-EN108 (Quarter Century Secret Rare) Facing Right = 8th Art
	"413393": "627285", // RA04-EN109 Red-Eyes Black Dragon (Platinum Secret Rare) White Light in Mouth = 4th Art
	"413394": "627288", // RA04-EN109 (Quarter Century Secret Rare) White Light in Mouth = 4th Art
	"413395": "627286", // RA04-EN109 (Platinum Secret Rare) Facing Left = 7th Art
	"413396": "627289", // RA04-EN109 (Quarter Century Secret Rare) Facing Left = 7th Art
	"413397": "627287", // RA04-EN109 (Platinum Secret Rare) Flying Right = 8th Art
	"413398": "627290", // RA04-EN109 (Quarter Century Secret Rare) Flying Right = 8th Art
	"327178": "254483", // MGED-EN141 Firewall Dragon (Orange Background) = Red Alternate Art
}

// yugiohCodes spells the set codes the buylist writes unpadded the way the
// catalog does: the Dinosaurs Rage deck is SD9 here and SD09 there, and the
// third Hobby League is HL3 where the catalog says HL03. Both the Number and
// the note carry them.
var yugiohCodes = strings.NewReplacer(
	"SD9-SS1", "SD09-ENSS1",
	"SD9-EN", "SD09-EN",
	"HL3EN", "HL03-EN",
	"HL3-EN", "HL03-EN",
)

// yugiohWording corrects the words a listing's name or note types the catalog
// cannot read. The matcher takes a bare number in the wording for the
// collector number, so "Version 1 - " and "4 Corners" are rewritten away from
// it; "No Stamp" says the opposite of the word the stamp promo type is read
// from; and GB1-001 is the number GBI-001 mistyped.
var yugiohWording = strings.NewReplacer(
	"Version 1 - ", "",
	"Version 2 - ", "",
	"Flowers in 4 Corners", "Flowers in Four Corners",
	"GB1-001", "GBI-001",
	"(No Stamp ", "(",
	" - No Stamp", "",
	"(Normal Art) Dragon Art", "A Alternate Art",
	"(Alt Art) Hero Art", "B HERO Art",
)

// yugiohNumbers holds the numbers the buylist mistypes, keyed by name and the
// number as typed, less any space around it. A blind replacement cannot hold
// them: LCGX-EN040 is the real number of another card.
var yugiohNumbers = map[string]string{
	"Ten Thousand Dragon|BLAR-EN093":     "BLAR-EN10K",
	"Gladiator Beast Secutor|LCGX-EN040": "LCGX-EN240",
	"Nordic Relic Laevateinn|BPW2-EN088": "BPW2-EN089",
}

// yugiohAlternateArts lists the Gold Letter alternate arts of Battles of
// Legend - Chapter 1 whose listing leaves "Alternate Art" out, keyed by name
// and edition. Both printings of these numbers are alternate arts, and only
// that word tells the Gold Letter one from the Silver.
var yugiohAlternateArts = map[string]bool{
	"Borrelsword Dragon|Battles of Legend - Chapter 1":                  true,
	"Number 39: Utopia (Astral Language)|Battles of Legend - Chapter 1": true,
}

// yugiohSkillCards names the Speed Duel skill cards a listing sells under the
// bare name, keyed by name and edition. The catalog files them as
// "<name> (Skill Card)", and the bare name finds another card of that name.
var yugiohSkillCards = map[string]string{
	"Call of the Haunted|Speed Duel: Arena of Lost Souls": "Call of the Haunted (Skill Card)",
	"Zombie Master|Speed Duel: Trials of the Kingdom":     "Zombie Master (Skill Card)",
}

// yugiohListing answers the card a Yu-Gi-Oh listing describes. The variation
// is the note on the sell listing and the number and note on the buylist, and
// notes is the note alone, which tells the print runs apart.
func yugiohListing(name, edition, notes, variation, rarity string, foil bool) *matcher.InputCard {
	listed := name + "|" + edition
	if skill, found := yugiohSkillCards[listed]; found {
		name = skill
	}
	variation = jpArtWording(yugiohWording.Replace(variation))
	if yugiohAlternateArts[listed] {
		variation += " Alternate Art"
	}
	return &matcher.InputCard{
		Name:      catalogColor(catalogSpelling(jpArtWording(yugiohWording.Replace(name)))),
		Edition:   printRunEdition(edition, notes),
		Variation: strings.TrimSpace(variation + " " + catalogRarity(rarity)),
		Foil:      foil,
	}
}

// yugiohBuylistVariation is buylistVariation with the number a vendor typo
// replaced and the codes spelled the catalog's way.
func yugiohBuylistVariation(product CSIPriceEntry) string {
	product.Number = yugiohBuylistNumber(product)
	return yugiohCodes.Replace(buylistVariation(product))
}

// yugiohBuylistNumber is a buylist row's Number as yugiohBuylistVariation
// writes it.
func yugiohBuylistNumber(product CSIPriceEntry) string {
	number := strings.TrimSpace(product.Number)
	fixed, found := yugiohNumbers[product.Name+"|"+number]
	if found {
		return fixed
	}
	return yugiohCodes.Replace(number)
}

// yugiohArtNotes names the artwork a note describes as the catalog's
// qualifier for it. The Limited Pack World Championship 2026 prints an
// alternate art of five Secret Rares, and the note describes its border and
// lettering where the catalog names it Emblazoned.
var yugiohArtNotes = map[string]string{
	"Red Inner Border, Japanese Letters in Art": "Emblazoned Alternate Art",
}

// yugiohArtCard answers a listing whose note is one of yugiohArtNotes, or nil.
// The qualifier names exactly one printing of the number, so the bracket the
// name carries and the rarity beside it are left behind: either one makes the
// matcher read the Emblazoned Secret Rare of the same number instead.
func yugiohArtCard(name, edition, number, notes string, foil bool) *matcher.InputCard {
	qualifier, found := yugiohArtNotes[strings.TrimSpace(notes)]
	if !found {
		return nil
	}
	head, _, _ := strings.Cut(name, " (")
	return &matcher.InputCard{
		Name:      catalogColor(catalogSpelling(head)),
		Edition:   printRunEdition(edition, notes),
		Variation: strings.TrimSpace(number + " " + qualifier),
		Foil:      foil,
	}
}

// yugiohRetries reports whether a refused listing is asked again by its
// image, on either feed: not where the matcher skips it, and not where the
// note says it is one of several printings.
func yugiohRetries(err error, notes string) bool {
	return err != nil && !errors.Is(err, matcher.ErrUnsupported) && !csiAmbiguousNote.MatchString(notes)
}

// yugiohImageCode matches the set code and number a Yu-Gi-Oh image sku
// carries: "BLMMEN053" is BLMM-EN053.
var yugiohImageCode = regexp.MustCompile(`^([A-Za-z0-9]{2,5}?)EN(\d{3})`)

// yugiohImageCard retries a refused buylist row with the code its own image
// sku carries in place of its Number, as yugiohBuylistNumber spells it, where
// the two disagree, and answers the id it lands on. The landing has to be the
// same card, so a sku naming another card's code reaches nothing.
func yugiohImageCard(b *matcher.Backend, card *matcher.InputCard, product CSIPriceEntry) string {
	match := yugiohImageCode.FindStringSubmatch(product.Image)
	number := yugiohBuylistNumber(product)
	if match == nil || number == "" {
		return ""
	}
	code := match[1] + "-EN" + match[2]
	if strings.EqualFold(code, number) {
		return ""
	}
	return yugiohRetry(b, card, strings.Replace(card.Variation, number, code, 1), code)
}

// yugiohSKUCard retries a refused sell listing with the number its product
// image names, and answers the id it lands on. The image file is the code with
// its dash left out, so the number is the datastore's own that spells it:
// LEHDENA23 is LEHD-ENA23 and MRL103 is MRL-103, which no pattern could split.
// numbers is skuNumbers of the backend.
func yugiohSKUCard(b *matcher.Backend, numbers map[string][]string, card *matcher.InputCard, imgURL string) string {
	match := imageStem.FindStringSubmatch(imgURL)
	if match == nil {
		return ""
	}
	// The sku can carry a tail past the number - an art ("V1QCR"), a letter -
	// so the longest head the index knows is the number it names.
	key := strings.ToUpper(match[1])
	for key != "" && numbers[key] == nil {
		key = key[:len(key)-1]
	}
	for _, number := range numbers[key] {
		id := yugiohRetry(b, card, number+" "+card.Variation, number)
		if id != "" {
			return id
		}
	}
	return ""
}

// yugiohRetry matches the card again with another variation and answers the id
// only if it is the printing at number and the same card, the catalog's deck
// letter "Monster Reborn (A)" aside.
func yugiohRetry(b *matcher.Backend, card *matcher.InputCard, variation, number string) string {
	retried := *card
	retried.Variation = variation
	id, err := b.Match(&retried)
	if err != nil {
		return ""
	}
	co, err := b.GetUUID(id)
	if err != nil || !strings.EqualFold(co.Number, number) {
		return ""
	}
	head, _, _ := strings.Cut(card.Name, " (")
	landed, _, _ := strings.Cut(co.Name, " (")
	if !matcher.Equals(landed, head) {
		return ""
	}
	return id
}
