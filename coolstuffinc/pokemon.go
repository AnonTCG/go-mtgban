package coolstuffinc

import (
	"cmp"
	"regexp"
	"slices"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// pokemonNonHolo matches the bracket a Pokemon name states a plain printing
// with. The storefront sells a card printed in both finishes as two products
// telling them apart by that bracket alone - the note is empty and the foil
// flag is off on both - and read as the holo, a $2.00 Team Aqua's Kyogre
// would be served at the $80.00 one's price. The bracket's own case varies
// ("(NON-HOLO)" on Black & White prints), so the match has to as well.
//
// The rarity is what says whether the plain printing was ever made. A holo
// rare is sold holo and nothing else, so a bracket asking for its plain
// printing asks for one that does not exist and the row is refused. A plain
// rare is the opposite: the catalog holding no nonfoil for it is the catalog
// missing a printing rather than the storefront inventing one, and refusing
// those would drop 25 real listings to catch nothing.
//
// "(Rare)" says the same of a holo rare's theme deck copy, sold beside the
// "(Holo Rare)" one, and "(Non-Holo Rare)" says it of a deck exclusive.
var pokemonNonHolo = regexp.MustCompile(`(?i)\((?:Non-?\s?Holo(?:\s+Rare)?|Rare)\)`)

// pokemonNonHoloNote matches the note a Black & White deck exclusive carries
// where its name carries no bracket.
var pokemonNonHoloNote = regexp.MustCompile(`(?i)\*Non-?\s?Holo Version\b`)

// pokemonNonHoloShelves are the catalog shelves that can hold the plain
// printing a "(Non-Holo)" bracket asks for, each with the set code the probe
// has to land on to be trusted.
var pokemonNonHoloShelves = []struct{ edition, set string }{
	{"Deck Exclusives", "PR-1840"},
	{"Miscellaneous Cards & Products", "MCAP"},
}

// pokemonNonHoloShelf answers the shelf of pokemonNonHoloShelves that carries
// the plain printing a "(Non-Holo)" bracket asks for, or "" where none does.
// It is taken only when the probe lands on that shelf's own nonfoil at the
// listing's own number.
func pokemonNonHoloShelf(b *matcher.Backend, name, numbered string, foil bool) string {
	num := cmp.Or(matcher.ExtractNumber(numbered), numbered)
	if num == "" {
		return ""
	}
	for _, shelf := range pokemonNonHoloShelves {
		co := pokemonLandsAt(b, matcher.InputCard{Name: name + " - " + numbered, Edition: shelf.edition, Foil: foil}, shelf.set, num)
		if co != nil && co.Finish == matcher.FinishNonfoil {
			return shelf.edition
		}
	}
	return ""
}

// pokemonHolo matches the bracket a Pokemon name states a holo printing
// with where the card's own set prints it plain, or in reverse holo only.
var pokemonHolo = regexp.MustCompile(`(?i)\s*\((?:Alt Art )?Holo(?: Promo)?\)`)

// pokemonHoloShelves are the catalog shelves that hold the holofoil of such
// a card, each with the set code the probe has to land on to be trusted.
var pokemonHoloShelves = []struct{ edition, set string }{
	{"Alternate Art Promos", "PR-1938"},
	{"Deck Exclusives", "PR-1840"},
	{"League & Championship Cards", "PR-1539"},
}

// pokemonHoloShelf answers a listing named by a holo bracket as the holofoil
// its own set holds at the number, else as the one the catalog files on one of
// pokemonHoloShelves, and leaves it as it is where neither does.
func pokemonHoloShelf(b *matcher.Backend, card *matcher.InputCard) *matcher.InputCard {
	head, numbered := numberedListing(card.Name)
	if !pokemonHolo.MatchString(head) && !pokemonHolo.MatchString(numbered) {
		return card
	}
	num := strings.TrimLeft(matcher.ExtractNumber(numbered), "0")
	if num == "" {
		return card
	}
	probe := *card
	probe.Name = strings.TrimSpace(pokemonHolo.ReplaceAllString(head, "")) + " - " + strings.TrimSpace(pokemonHolo.ReplaceAllString(numbered, ""))
	probe.Foil = true
	plain := *card
	plainID, err := b.Match(&plain)
	if err == nil {
		own, err := b.GetUUID(plainID)
		if err == nil {
			co := pokemonLandsAt(b, probe, own.SetCode, num)
			if co != nil && co.Finish == "holofoil" {
				return &probe
			}
		}
	}
	for _, shelf := range pokemonHoloShelves {
		probe.Edition = shelf.edition
		co := pokemonLandsAt(b, probe, shelf.set, num)
		if co != nil && co.Finish == "holofoil" {
			return &probe
		}
	}
	return card
}

// pokemonLandsAt matches a copy of a probe and answers the printing it lands
// on, or nil unless that is set's own at num, leading zeros aside.
func pokemonLandsAt(b *matcher.Backend, probe matcher.InputCard, set, num string) *matcher.CardObject {
	id, err := b.Match(&probe)
	if err != nil {
		return nil
	}
	co, err := b.GetUUID(id)
	if err != nil || co.SetCode != set || strings.TrimLeft(co.Number, "0") != num {
		return nil
	}
	return co
}

// numberedListing reads a listing name apart from the collector number this
// storefront glues onto it, answering the name alone and what it took, or the
// name whole and nothing where the storefront named the card by itself.
//
// Only a tail opening on a number is taken, since the storefront also hangs
// plain wording off a dash ("Ancient Mew - Movie Promo") and the catalog
// spells some cards with one of its own. What is taken is set aside rather
// than thrown away: the printing is written in parentheses behind the number
// ("16/111 (Reverse Foil)") and neither side's foil column says so, which the
// matcher reads off the name it is handed.
func numberedListing(name string) (string, string) {
	head, tail, found := strings.Cut(name, " - ")
	if !found {
		return name, ""
	}
	number, _, _ := strings.Cut(tail, " ")
	if !buylistNumberWord.MatchString(number) {
		return name, ""
	}
	return head, tail
}

// pokemonCatalogShelves names the listings this storefront files on a main
// or promo shelf for printings the catalog keeps on another: the listing's
// own name and shelf (and a marker its note has to carry, "" for none), then
// the card to ask for. All of them are holo printings and nothing else.
var pokemonCatalogShelves = []struct {
	listing, shelf, marker   string
	name, edition, variation string
}{
	{"Machamp - 8/102", "Base Set (Shadowless)", "", "Machamp - 8/102", "Deck Exclusives", "Base Set Shadowless"},
	{"Machamp - 8/102", "Base Set", "Stamp w/ Shadow", "Machamp - 8/102", "Deck Exclusives", ""},
	{"Ancient Mew - Movie Promo", "WOTC Black Star Promos", "", "Ancient Mew", "Miscellaneous Cards & Products", ""},
	{"Darkness Energy - 2017 (Reverse Foil)", "Shining Legends", "", "Darkness Energy", "Deck Exclusives", "2017 Wave Foil"},
	{"Fairy Energy - 2017 (Reverse Foil)", "Shining Legends", "", "Fairy Energy", "Deck Exclusives", "2017 Wave Foil"},
	{"Grass Energy - 2017 (Reverse Foil)", "Shining Legends", "", "Grass Energy", "Deck Exclusives", "2017 Wave Foil"},
	{"Lightning Energy - 2017 (Reverse Foil)", "Shining Legends", "", "Lightning Energy", "Deck Exclusives", "2017 Wave Foil"},
	{"Metal Energy - 2017 (Reverse Foil)", "Shining Legends", "", "Metal Energy", "Deck Exclusives", "2017 Wave Foil"},
	{"Psychic Energy - 2017 (Reverse Foil)", "Shining Legends", "", "Psychic Energy", "Deck Exclusives", "2017 Wave Foil"},
	{"Water Energy - 2017 (Reverse Foil)", "Shining Legends", "", "Water Energy", "Deck Exclusives", "2017 Wave Foil"},
	{"Fighting Energy - Starfoil Promo", "WOTC Black Star Promos", "", "Fighting Energy", "League & Championship Cards", "WotC League Promo"},
	{"Fire Energy - Starfoil Promo", "WOTC Black Star Promos", "", "Fire Energy", "League & Championship Cards", "WotC League Promo"},
	{"Grass Energy - Starfoil Promo", "WOTC Black Star Promos", "", "Grass Energy", "League & Championship Cards", "WotC League Promo"},
	{"Lightning Energy - Starfoil Promo", "WOTC Black Star Promos", "", "Lightning Energy", "League & Championship Cards", "WotC League Promo"},
	{"Psychic Energy - Starfoil Promo", "WOTC Black Star Promos", "", "Psychic Energy", "League & Championship Cards", "WotC League Promo"},
	{"Water Energy - Starfoil Promo", "WOTC Black Star Promos", "", "Water Energy", "League & Championship Cards", "WotC League Promo"},
	{"Articuno, Moltres, and Zapdos Oversize Card", "WOTC Black Star Promos", "", "Articuno, Moltres, and Zapdos", "Jumbo Cards", ""},
	{"Great Tusk ex (Shiny) - SVP072 (Paldean Fates Stamp)", "SV Promos", "", "Great Tusk ex - 072", "Miscellaneous Cards & Products", "Paldean Fates Stamped"},
	{"Iron Treads ex (Shiny) - SVP073 (Paldean Fates Stamp)", "SV Promos", "", "Iron Treads ex - 073", "Miscellaneous Cards & Products", "Paldean Fates Stamped"},
	{"Champions Festival - XY27", "XY Promos", "", "Champions Festival - XY27", "XY Promos", "2014"},
	{"Champions Festival - XY91", "XY Promos", "", "Champions Festival - XY91", "XY Promos", "2015"},
	{"Champions Festival - XY176", "XY Promos", "", "Champions Festival - XY176", "XY Promos", "2016"},
	{"Electrode (Holo) - 088/264", "SWSH Fusion Strike", "", "Electrode - 088", "Miscellaneous Cards & Products", "Cosmos Holo"},
}

// pokemonReverse2022Energy matches the 2022 reverse holo basic energies this
// storefront sells under Crown Zenith, which the catalog files with Brilliant
// Stars' unnumbered 2022 energies, apart from Crown Zenith's own textured ones.
var pokemonReverse2022Energy = regexp.MustCompile(`^(\w+ Energy) - 2022 \(Reverse Foil\)$`)

// pokemonListingIDs names the catalog product of the listings whose wording no
// matcher reads: the 30th Celebration Mew, one for each of the Red, Green and
// Blue scans, a jumbo's bare letter number, and the Pokemon Day stamped
// Pikachu the POP Series catalog labels without the word Stamp, the
// prerelease stamped Meganium that its note only calls Stamped, and the
// Non-Holo Professor's Research a promo type keeps the matcher off.
var pokemonListingIDs = map[string]string{
	"Meganium - MEP001|ME Promos":                                  "654594",
	"Professor's Research (Non-Holo) - SWSH178|SWSH Promos":        "246275",
	"Mew (Red) - R/RGB|30th Celebration":                           "717607",
	"Mew (Green) - G/RGB|30th Celebration":                         "717608",
	"Mew (Blue) - B/RGB|30th Celebration":                          "717609",
	"Mega Greninja ex - MEP081 Jumbo Size|Pokemon Oversized Cards": "704880",
	"Pikachu (Pokemon Day Stamp) - 9/17|POP Series 6":              "276377",
	"Pikachu (Pokemon Day Stamp) - 15/17|POP Series 9":             "701479",
	"Professor's Research (Holo) - 060/072|SWSH Shining Fates":     "239642",
}

// pokemonListing reads a Pokemon listing the way the catalog names it. The
// Classic Collection reprints are sold under Celebrations with the
// collection in the note; the special energies are named "Special Metal
// Energy" where the catalog names the energy and labels it special; the
// Base Set Professor Oak is spelled "Imposter" the way Base Set 2 prints
// it, though Base Set printed "Impostor"; Nidoran is named without the sex
// the catalog names it by, which the number settles; the Elite Four
// cards of the Platinum sets are named "Alakazam 4" for the catalog's
// "Alakazam E4"; and the metal cards are named for the metal, "Metal Mew ex"
// for the catalog's Mew ex labelled a metal card.
func pokemonListing(b *matcher.Backend, name, edition, variation string, foil bool) *matcher.InputCard {
	m2022 := pokemonReverse2022Energy.FindStringSubmatch(name)
	if m2022 != nil && edition == "SWSH Crown Zenith" {
		return &matcher.InputCard{Name: m2022[1], Edition: "SWSH Brilliant Stars", Variation: "2022", Foil: true}
	}
	id, found := pokemonListingIDs[name+"|"+edition]
	if found {
		return &matcher.InputCard{ID: id, Foil: true}
	}
	for _, r := range pokemonCatalogShelves {
		if name == r.listing && edition == r.shelf && strings.Contains(variation, r.marker) {
			return &matcher.InputCard{Name: r.name, Edition: r.edition, Variation: r.variation, Foil: true}
		}
	}
	if edition == "Pokemon Oversized Cards" {
		edition = "Jumbo Cards"
		name = promoProgrammeNumber.ReplaceAllString(strings.TrimSuffix(name, " Jumbo Size"), "$1")
	}
	name, numbered := numberedListing(pokemonRespellings.Replace(name))
	numbered = megaPromoNumber.ReplaceAllString(numbered, "MEP$1")
	variation = megaPromoNumber.ReplaceAllString(variation, "MEP$1")
	name = strings.TrimSpace(nonStampedName.ReplaceAllString(name, ""))
	card := &matcher.InputCard{Name: name, Edition: edition, Variation: variation, Foil: foil}
	m := basicEnergyName.FindStringSubmatch(name)
	if m != nil {
		bracket := strings.Trim(m[2], "()")
		fixed := pokemonBasicEnergy(b, m[1], bracket, edition, numbered, variation, foil)
		if fixed != nil {
			return fixed
		}
	}
	nonHolo := pokemonNonHolo.MatchString(name) || pokemonNonHolo.MatchString(numbered) || pokemonNonHoloNote.MatchString(variation)
	if nonHolo {
		strippedName := strings.TrimSpace(pokemonNonHolo.ReplaceAllString(name, ""))
		strippedNumbered := strings.TrimSpace(pokemonNonHolo.ReplaceAllString(numbered, ""))
		shelf := pokemonNonHoloShelf(b, strippedName, strippedNumbered, foil)
		if shelf != "" {
			card.Name = strippedName
			card.Edition = shelf
			numbered = strippedNumbered
		}
	}
	if edition == "Celebrations" && (strings.Contains(variation, "Classic Collection") || strings.Contains(numbered, "Classic Collection")) {
		card.Edition = "Celebrations: Classic Collection"
		if m := classicNumber.FindStringSubmatch(variation); m != nil {
			card.Variation = m[1]
		}
	}
	// A reprint note ("25th Anniversary Stamp Base Set Reprint") reaches
	// retail's variation whole; clear it before "Stamp" reads as a demand.
	reprint := buylistReprintNote.MatchString(variation)
	if reprint {
		card.Variation = ""
	}
	if strings.HasPrefix(name, "Vivillon") {
		name = pokemonVivillonColors.Replace(name)
		card.Name = name
		numbered = pokemonVivillonColors.Replace(numbered)
	}
	// This storefront numbers the Alph Lithographs as secret rares past their
	// set's total, where the catalog numbers each of the four by its set.
	if name == "Alph Lithograph" {
		numbered, card.Variation = "", ""
	}
	shelved := false
	if !nonHolo {
		shelf := pokemonNoteShelf(b, numbered, card)
		if shelf != nil {
			card, numbered, shelved = shelf, strings.TrimSpace(pokemonStampTail.ReplaceAllString(numbered, "")), true
		}
	}
	// A "(Non-Holo)" listing asks the redirect for the deck's plain copy.
	probe, probeFoil := "Cracked Ice Holo", true
	if nonHolo {
		probe, probeFoil = "Non-Holo", false
	}
	redirected := pokemonDeckHoloRedirect(b, strings.TrimSpace(pokemonNonHolo.ReplaceAllString(name, "")), strings.TrimSpace(pokemonNonHolo.ReplaceAllString(numbered, "")), variation, probe, probeFoil)
	if redirected != "" {
		card.Name = strings.TrimSpace(pokemonNonHolo.ReplaceAllString(card.Name, ""))
		card.Edition = redirected
		card.Variation = probe
		card.Foil = probeFoil
	}
	m = goldStar.FindStringSubmatch(name)
	if m != nil {
		card.Name = m[1] + " Star"
	}
	m = unownListing.FindStringSubmatch(name)
	if m != nil {
		card.Name = "Unown"
		card.Variation = m[2] + "/" + m[3]
	}
	if m := specialEnergy.FindStringSubmatch(name); m != nil {
		card.Name = m[1]
		card.Variation = strings.TrimSpace("Special " + card.Variation)
	}
	// Only where the whole name is no card and the rest is one: "Metal
	// Energy" is a card, and so is none of "Energy (Secret Rare)".
	m = metalCardName.FindStringSubmatch(card.Name)
	if m != nil {
		_, whole := b.SearchEquals(card.Name)
		_, rest := b.SearchEquals(m[1])
		if whole != nil && rest == nil {
			card.Name = m[1]
			if !matcher.SlugDescribes(card.Variation, "metalcard") {
				card.Variation = strings.TrimSpace(card.Variation + " Metal Card")
			}
		}
	}
	if name == "Imposter Professor Oak" && strings.HasPrefix(edition, "Base Set") && !strings.HasPrefix(edition, "Base Set 2") {
		card.Name = "Impostor Professor Oak"
	}
	if m := eliteFour.FindStringSubmatch(name); m != nil && strings.HasPrefix(edition, "Platinum") {
		card.Name = m[1] + " E4"
	}
	// An energy or promo sold under a main set with the energy or promo
	// set's own number ("MEE007" on Ascended Heroes) is that set's.
	if m := prefixedNumber.FindStringSubmatch(variation); m != nil {
		if set, found := pokemonNumberSets[m[1]]; found {
			card.Edition = set
			// The catalog numbers the older promo sets with the
			// programme on ("SM190") and the newer ones without.
			if m[1] != "SM" && m[1] != "SWSH" {
				card.Variation = strings.Replace(variation, m[0], m[2], 1)
			}
		}
	}
	// The notes say a printing is the plain one ("Non-Stamped Version"),
	// name the illustrator, or name the stamp a promo carries; the plain
	// words come off, and a stamp names a promo shelf the catalog files
	// apart from the set the listing arrived on.
	card.Variation = strings.TrimSpace(plainWords.ReplaceAllString(card.Variation, " "))
	if !reprint && !shelved && stamped.MatchString(card.Variation) {
		card.Edition = "Promo"
	}
	// The Team Galactic inventions are named by their invention alone
	// where the catalog names the invention with its number.
	if _, err := b.SearchEquals(card.Name); err != nil {
		if invention := galacticInvention(b, card.Name); invention != "" {
			card.Name = invention
		}
	}
	// The catalog letters the type in a special energy's name ("Bubbly W
	// Energy") where the storefront spells it out, but not in every one
	// ("Magnetic Metal Energy"), so the spelled name is kept where the
	// catalog knows it.
	if m := typedEnergy.FindStringSubmatch(card.Name); m != nil {
		if _, err := b.SearchEquals(card.Name); err != nil {
			card.Name = m[1] + " " + energyLetters[m[2]] + " Energy"
		}
	}
	if name == "Nidoran" || name == "Nidoran?" {
		for _, sex := range []string{"Nidoran M", "Nidoran F"} {
			probe := *card
			probe.Name = sex
			if numbered != "" {
				probe.Name += " - " + numbered
			}
			if _, err := b.Match(&probe); err == nil {
				card.Name = sex
				break
			}
		}
	}
	if numbered != "" {
		card.Name = pokemonProfessor(b, card, numbered)
		card.Name += " - " + numbered
	}
	return card
}

// pokemonProfessor answers the name a Professor card goes by in the catalog
// where this storefront spells it the other way round ("Prof." for
// "Professor" and back, each the way some sets print it). The other spelling
// is taken only when the listing's own does not land at its number and the
// other does.
func pokemonProfessor(b *matcher.Backend, card *matcher.InputCard, numbered string) string {
	var swapped string
	switch {
	case strings.HasPrefix(card.Name, "Prof. "):
		swapped = "Professor " + strings.TrimPrefix(card.Name, "Prof. ")
	case strings.HasPrefix(card.Name, "Professor "):
		swapped = "Prof. " + strings.TrimPrefix(card.Name, "Professor ")
	default:
		return card.Name
	}
	num := matcher.ExtractNumber(numbered)
	if num == "" {
		return card.Name
	}
	landsAtNumber := func(name string) bool {
		probe := *card
		probe.Name = name + " - " + numbered
		id, err := b.Match(&probe)
		if err != nil {
			return false
		}
		co, err := b.GetUUID(id)
		return err == nil && strings.TrimLeft(co.Number, "0") == num
	}
	if landsAtNumber(card.Name) || !landsAtNumber(swapped) {
		return card.Name
	}
	return swapped
}

// pokemonBuylistCard reads a buylist row the way a sell listing is read, the
// print run included. The run rides in the shelf's title on both sides of the
// storefront: a buy row read with its shelf spelled whole matches the set of
// that name and is published against the unlimited printing at the first
// edition's price.
func pokemonBuylistCard(b *matcher.Backend, product CSIPriceEntry) (*matcher.InputCard, []string) {
	// CSI's "0" placeholder for an unnumbered year energy otherwise reads
	// as a number word of its own at the head of the variation.
	if product.Number == "0" {
		product.Number = ""
	}
	// Zoroark-GX's alternate art number is typed with its letter transposed.
	if product.Number == "77/a73" {
		product.Number = "77a/73"
	}
	variation := catalogTreatment(buylistVariation(product))
	shelf, run := firstEditionShelf(product.ItemSet)
	shelf = pokemonPromoShelf(b, product.Name, shelf, product.RarityName, product.IsFoil == 1, variation)
	card := pokemonListing(b, product.Name, shelf, variation, product.IsFoil == 1)
	return pokemonHoloShelf(b, pokemonCosmosHolo(b, card, product.RarityName)), run
}

// pokemonNumberSets are the sets a number's prefix names outright.
var pokemonNumberSets = map[string]string{
	"MEE":  "MEE: Mega Evolution Energies",
	"SVE":  "SVE: Scarlet & Violet Energies",
	"MEP":  "ME: Mega Evolution Promo",
	"SVP":  "SV: Scarlet & Violet Promo Cards",
	"SWSH": "SWSH: Sword & Shield Promo Cards",
	"SM":   "SM Promos",
}

// pokemonRespellings pairs the names this storefront misspells with the
// catalog's own. The head is rewritten in place, so the number or bracket
// behind it is kept; Kyurem's number is run into its name without a dash.
var pokemonRespellings = strings.NewReplacer(
	"Galatic HQ", "Galactic HQ",
	"Sprigattito", "Sprigatito",
	"Unit Energy GFW", "Unit Energy GRW",
	"Delta Species Rainbow Energy", "Delta Rainbow Energy",
	"Kyurem 43/113", "Kyurem - 43/113",
	"Vivilion", "Vivillon",
	"Rayquaza-GX (Shiny) - 177a", "Rayquaza-GX (Alt Art) - 177a",
	"Zoroark-GX (Shiny) - 77a", "Zoroark-GX (Alt Art) - 77a",
	" - NON HOLO", " - NON-HOLO",
	"Nidoran(M)", "Nidoran M",
	"Victory Cup 1st Place", "Victory Cup",
	"Victory Cup 2nd Place", "Victory Cup",
	"Victory Cup 3rd Place", "Victory Cup",
)

var (
	plainWords = regexp.MustCompile(`(?i)\bNon-?Stamped(?: Version)?\b|\bIllus\. [^,]+,`)
	stamped    = regexp.MustCompile(`(?i)\bStamp(?:ed)?\b`)

	// nonStampedName matches the same bracket as plainWords where it sits
	// in the name's own head instead - "Psyduck (Non-Stamped) - SM199".
	nonStampedName = regexp.MustCompile(`(?i)\s*\(Non-?Stamped\)`)
)

// galacticInvention names the Team Galactic invention the catalog files
// under its number, or nothing when no invention ends in the name given.
func galacticInvention(b *matcher.Backend, name string) string {
	found := ""
	for _, candidate := range b.Names(matcher.NameFormCanonical, false) {
		if strings.HasPrefix(candidate, "Team Galactic's Invention") && strings.HasSuffix(candidate, " "+name) {
			if found != "" {
				return ""
			}
			found = candidate
		}
	}
	return found
}

var energyLetters = map[string]string{
	"Grass": "G", "Fire": "R", "Water": "W", "Lightning": "L", "Psychic": "P",
	"Fighting": "F", "Darkness": "D", "Metal": "M", "Fairy": "Y", "Dragon": "N",
	"Colorless": "C",
}

var (
	typedEnergy    = regexp.MustCompile(`^(\w+) (Grass|Fire|Water|Lightning|Psychic|Fighting|Darkness|Metal|Fairy|Dragon|Colorless) Energy$`)
	prefixedNumber = regexp.MustCompile(`\b(MEE|SVE|MEP|SVP|SWSH|SM)(\d{3})\b`)
	classicNumber  = regexp.MustCompile(`Classic Collection (\d+)`)

	// megaPromoNumber matches the Mega Evolution promo number as this
	// storefront writes it, "ME099", where the catalog's own is "MEP099".
	megaPromoNumber = regexp.MustCompile(`\bME(\d{3})\b`)

	// promoProgrammeNumber matches the programme prefix a jumbo card's number
	// carries, which the catalog files its Jumbo Cards under without.
	promoProgrammeNumber = regexp.MustCompile(`\b(?:SVP|MEP|ME)(\d{3})\b`)
	specialEnergy        = regexp.MustCompile(`^Special ((?:Metal|Darkness) Energy)$`)
	eliteFour            = regexp.MustCompile(`^(.+) 4( LV\.X)?$`)

	// goldStar matches a Gold Star the way this storefront names it, "Mew *
	// (Star)" for the catalog's "Mew Star" - $1,800 of buylist refusals on
	// Ex Dragon Frontiers' Mew alone.
	goldStar = regexp.MustCompile(`^(.+) \* \(Star\)$`)

	// unownListing matches an EX Unseen Forces Unown the way this
	// storefront names it, the letter written into both the name and its
	// own index ("Unown A - A/28"), where the catalog names every one of
	// them "Unown" and numbers it by the letter alone.
	unownListing = regexp.MustCompile(`^Unown ([A-Z!?]) - ([A-Z!?])/(\d+)$`)

	// metalCardName matches an Ultra-Premium Collection metal card the way
	// this storefront names it, for the metal and the set it copies ("Metal
	// Mew ex", "Metal Base Set Charizard"), where the catalog names the card.
	metalCardName = regexp.MustCompile(`^Metal (?:Base Set )?(.+)$`)
)

// pokemonVivillonColors spells the two Vivillon colours this storefront
// shortens where the catalog's own promo label is longer than the colour
// alone - "(Pink)" for "(Meadow Pink)", the only thing that tells the XY
// 17/146 Pink Pattern apart from the identically-numbered Orange one.
var pokemonVivillonColors = strings.NewReplacer(
	"(Pink)", "(Meadow Pink)",
	"(Orange)", "(High Plains Orange)",
)

// pokemonNoteShelves names what a note says about the shelf a listing
// belongs on, for the printings this storefront files on a main or promo
// shelf while its note names the catalog's own: the edition to ask for, the
// variation to ask it with instead of the note ("" keeps the note), and the
// set code the listing has to land on to be trusted.
var pokemonNoteShelves = []struct {
	marker, edition, variation, wantSet string
}{
	{"Regional Championship", "League & Championship Cards", "", "PR-1539"},
	{"Build & Battle", "Miscellaneous Cards & Products", "Prerelease", "MCAP"},
	{"Prerelease", "XY Promos", "", "PR-1451"},
	{"From Dragon Vault Blister Pack", "Blister Exclusives", "", "BLE"},
}

// pokemonStampTail matches the stamp the name carries behind its number for
// the printings pokemonNoteShelves redirects: the catalog labels them by the
// event and not by a stamp, so the word would only demand one that is not
// there.
var pokemonStampTail = regexp.MustCompile(`(?i)\s*\([^)]*\bStamp(?:ed)?\)|\s+-\s+Dragon Vault Stamped Mirror Holo`)

// pokemonNoteShelf answers the listing re-asked on the shelf its note names,
// or nil where no note names one or the redirected listing does not land on
// that shelf's own set.
func pokemonNoteShelf(b *matcher.Backend, numbered string, card *matcher.InputCard) *matcher.InputCard {
	cleaned := strings.TrimSpace(pokemonStampTail.ReplaceAllString(numbered, ""))
	for _, r := range pokemonNoteShelves {
		if !strings.Contains(card.Variation, r.marker) {
			continue
		}
		redirected := *card
		redirected.Edition = r.edition
		if r.variation != "" {
			redirected.Variation = r.variation
		}
		probe := redirected
		if cleaned != "" {
			probe.Name += " - " + cleaned
		}
		id, err := b.Match(&probe)
		if err != nil {
			continue
		}
		co, err := b.GetUUID(id)
		if err != nil || co.SetCode != r.wantSet {
			continue
		}
		return &redirected
	}
	return nil
}

// pokemonDeckHoloNotes names the edition a print-run note belongs to, and
// the set code the redirect has to land on to be trusted.
var pokemonDeckHoloNotes = []struct {
	marker, edition, wantSet string
}{
	{"Theme Deck", "Deck Exclusives", "PR-1840"},
	{"EX Battle Stadium", "EX Battle Stadium", "BST"},
	{"Prism Holo", "Miscellaneous Cards & Products", "MCAP"},
	{"Shattered Holo", "Miscellaneous Cards & Products", "MCAP"},
}

// pokemonDeckHoloRedirect answers the edition for a pokemonDeckHoloNotes
// marker whose probe lands on that marker's own set code at the listing's own
// number, or "" otherwise.
// The probe asks for the "Cracked Ice Holo" label and the foil finish so a
// cracked-ice twin outranks the plain printing of the same number, or for
// "Non-Holo" and the plain finish where the listing is the plain copy.
func pokemonDeckHoloRedirect(b *matcher.Backend, name, numbered, notes, variation string, foil bool) string {
	for _, r := range pokemonDeckHoloNotes {
		if !strings.Contains(notes, r.marker) && !strings.Contains(numbered, r.marker) {
			continue
		}
		tail := strings.TrimSpace(strings.Replace(numbered, r.marker, "", 1))
		tail = strings.TrimSpace(strings.TrimSuffix(tail, "-"))
		num := matcher.ExtractNumber(tail)
		if num == "" {
			continue
		}
		probe := matcher.InputCard{Name: name + " - " + tail, Edition: r.edition, Variation: variation, Foil: foil}
		if pokemonLandsAt(b, probe, r.wantSet, num) != nil {
			return r.edition
		}
	}
	return ""
}

// basicEnergyName matches this storefront's "<Type> Energy" and a treatment
// bracket it carries of its own ("(Cosmo Holo)"), apart from the number
// tail's. pokemonBasicEnergy gates the rename on the listing's own number.
var basicEnergyName = regexp.MustCompile(`^(Grass|Fire|Water|Lightning|Psychic|Fighting|Darkness|Metal|Fairy) Energy(?:\s+(\(.+\)))?$`)

// pokemonBasicEnergy answers a bare "<Type> Energy" listing the way the
// catalog spells its Scarlet & Violet / Mega Evolution era printings, read
// off the SVE/MEE-shaped product number glued onto the tail.
func pokemonBasicEnergy(b *matcher.Backend, energyType, bracket, edition, numbered, variation string, foil bool) *matcher.InputCard {
	name := "Basic " + energyType + " Energy"
	bracket = catalogTreatment(bracket)

	build := func(fromVariation bool, numberedTail string) *matcher.InputCard {
		card := &matcher.InputCard{Name: name, Edition: edition, Variation: bracket, Foil: foil}
		if fromVariation {
			card.Variation = strings.TrimSpace(bracket + " " + numberedTail)
		} else if numberedTail != "" {
			card.Name += " - " + numberedTail
		}
		return card
	}

	tail, fromVariation := csiTreatments.Replace(numbered), false
	pm := prefixedNumber.FindStringSubmatch(tail)
	if pm == nil {
		tail, fromVariation = csiTreatments.Replace(variation), true
		pm = prefixedNumber.FindStringSubmatch(tail)
	}
	if pm == nil {
		// A plain rename needs the tail's own number to say which
		// printing is meant, on the shelf's own set: the same gate
		// the on-shelf and SVE/MEE probes below both apply.
		num := matcher.ExtractNumber(numbered)
		if num == "" {
			return nil
		}
		shelf, err := b.GetSetByName(edition)
		if err != nil {
			return nil
		}
		plain := build(false, numbered)
		plain.Variation = strings.TrimSpace(bracket + " " + variation)
		id, err := b.Match(plain)
		if err != nil {
			return nil
		}
		co, err := b.GetUUID(id)
		if err != nil || co.SetCode != shelf.Code || strings.TrimLeft(co.Number, "0") != num {
			return nil
		}
		return plain
	}

	shelf, shelfErr := b.GetSetByName(edition)

	bare := strings.TrimLeft(pm[2], "0")
	if bare == "" {
		bare = "0"
	}
	onShelf := build(fromVariation, strings.TrimSpace(strings.Replace(tail, pm[0], bare, 1)))
	if shelfErr == nil {
		id, err := b.Match(onShelf)
		if err == nil {
			co, err := b.GetUUID(id)
			if err == nil && co.SetCode == shelf.Code {
				return onShelf
			}
		}
	}

	set, found := pokemonNumberSets[pm[1]]
	if !found {
		return nil
	}
	target, err := b.GetSetByName(set)
	if err != nil {
		return nil
	}
	redirected := build(fromVariation, strings.TrimSpace(strings.Replace(tail, pm[0], pm[2], 1)))
	redirected.Edition = set
	id, err := b.Match(redirected)
	if err != nil {
		return nil
	}
	co, err := b.GetUUID(id)
	if err != nil || co.SetCode != target.Code {
		return nil
	}
	return redirected
}

// pokemonCosmosBracket matches the bracket this storefront names a cosmos
// holo with: "Holo Promo", "Cosmo Holo" or the bare "Holo", or "- Cosmos
// Holo" ending the name. The bare one is also what a plain holo rare is
// called, so it counts only beside the Promo rarity or a note saying cosmos
// holo.
var pokemonCosmosBracket = regexp.MustCompile(`(?i)\s*(?:\((Holo Promo|Cosmos? Holo|Holo)\)|- (Cosmos? Holo)$)`)

// pokemonCosmosNote matches a note stating the listing is the cosmos holo,
// and pokemonCosmosHedge one saying it can be ("Can be Regular or Cosmo Holo").
var pokemonCosmosNote = regexp.MustCompile(`(?i)(?:^|[\d\s])Cosmos Holo\b`)
var pokemonCosmosHedge = regexp.MustCompile(`(?i)\bor Cosmos Holo\b`)

// pokemonCosmosShelves are the catalog shelves that hold a collection-box
// holo, each with the set code a probe has to land on to be trusted.
var pokemonCosmosShelves = []struct{ edition, set string }{
	{"Miscellaneous Cards & Products", "MCAP"},
	{"Blister Exclusives", "BLE"},
	{"Deck Exclusives", "PR-1840"},
}

// pokemonCosmosHolo answers a listing sold under a main set with one of those
// brackets as the collection-box holo the catalog files on one of
// pokemonCosmosShelves, and leaves the listing as it is unless that printing
// exists at the listing's own number.
//
// The storefront keeps the set's number and total on it, so the base set's
// nonfoil answers for it, a $2.99 Charmeleon priced as the $0.29 one. A
// listing that already lands on a cosmos holo is left there.
func pokemonCosmosHolo(b *matcher.Backend, card *matcher.InputCard, rarity string) *matcher.InputCard {
	m := pokemonCosmosBracket.FindStringSubmatch(card.Name)
	if m == nil {
		return card
	}
	cosmosNote := pokemonCosmosNote.MatchString(card.Variation) && !pokemonCosmosHedge.MatchString(card.Variation)
	if strings.EqualFold(m[1], "Holo") && rarity != "Promo" && !cosmosNote {
		return card
	}
	asked := *card
	id, err := b.Match(&asked)
	if err == nil {
		co, err := b.GetUUID(id)
		if err == nil && slices.Contains(co.PromoTypes, "cosmosholo") {
			return card
		}
	}
	cosmos := *card
	cosmos.Name = pokemonCosmosBracket.ReplaceAllString(card.Name, "")
	cosmos.Variation = "Cosmos Holo"
	cosmos.Foil = true
	_, tail := numberedListing(cosmos.Name)
	num := strings.TrimLeft(matcher.ExtractNumber(tail), "0")
	if num == "" {
		return card
	}
	for _, shelf := range pokemonCosmosShelves {
		cosmos.Edition = shelf.edition
		if pokemonLandsAt(b, cosmos, shelf.set, num) != nil {
			return &cosmos
		}
	}
	return card
}

// pokemonPromoShelf answers the shelf a Pokemon listing belongs to, which is
// the one it arrived on unless the catalog files the card as a promo.
//
// A promo carrying a main set's number is sold here under that set, with only
// the rarity field saying otherwise: the Pokemon Day 2025 Eevee sits on SV
// Prismatic Evolutions at 074/131, where that set's own Eevee already stands.
// The two meet there, and the $2.50 promo would be priced as the card it was
// stamped from. The catalog keeps those on a promo shelf instead.
//
// The promo shelf only decides where it answers at all. Twenty of the fifty
// buylist listings this can reach name a printing no promo shelf holds - the
// Pokemon Rumble cards, the holo promos that are their set's own foil - and
// those keep the set they arrived on. The same shape reproduces on retail -
// the identical "Eevee - 074/131 (Pokemon Day 2025)" listing sits on the same
// shelf, with the same breadcrumb rarity "Promo" - so both sides call this
// with their own name for the rarity field: RarityName on the buylist, the
// scraped breadcrumb text on retail.
func pokemonPromoShelf(b *matcher.Backend, name, itemSet, rarityName string, foil bool, variation string) string {
	if rarityName != "Promo" ||
		strings.Contains(strings.ToLower(itemSet), "promo") {
		return itemSet
	}
	probe := &matcher.InputCard{
		Name:      name,
		Edition:   "Promo",
		Variation: variation,
		Foil:      foil,
	}
	_, err := b.Match(probe)
	if err != nil {
		return itemSet
	}
	return "Promo"
}

// retailPokemon reads an offer's shelf and treatment, matching a first
// edition shelf in the run of finishes it was sold in.
func retailPokemon(csi *Coolstuffinc, l sellListing) (*matcher.InputCard, string, error) {
	b := csi.backend
	shelf, shelfRun := firstEditionShelf(l.edition)
	if shelfRun != nil {
		l.run = shelfRun
	}
	// A preorder's release date follows its note, and the
	// day of the month would read as a collector number.
	head, _, _ := strings.Cut(l.notes, "Preorder Notes:")
	variation := catalogTreatment(head)
	shelf = pokemonPromoShelf(b, l.name, shelf, l.rarity, l.foil, variation)
	theCard := pokemonHoloShelf(b, pokemonCosmosHolo(b, pokemonListing(b, l.name, shelf, variation, l.foil), l.rarity))
	cardID, err := l.match(b, theCard)
	return theCard, cardID, err
}

// buylistPokemon reads a buylist row, and drops a non-holo row that landed
// on a printing only ever made holo.
func buylistPokemon(csi *Coolstuffinc, p CSIPriceEntry) (*matcher.InputCard, string, error) {
	b := csi.backend
	theCard, run := pokemonBuylistCard(b, p)
	var cardID string
	var err error
	if run != nil {
		cardID, err = matchRun(b, theCard, run)
	} else {
		cardID, err = b.Match(theCard)
	}
	if err != nil {
		return theCard, "", err
	}
	if pokemonNonHolo.MatchString(p.Name) {
		co, cerr := b.GetUUID(cardID)
		if cerr == nil && !co.HasFinish(matcher.FinishNonfoil) &&
			strings.Contains(co.Rarity, "holo") {
			return theCard, "", matcher.ErrUnsupported
		}
	}
	return theCard, cardID, nil
}
