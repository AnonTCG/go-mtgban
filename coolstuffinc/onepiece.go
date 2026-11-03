package coolstuffinc

import (
	"regexp"
	"slices"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// onePieceEvents spells a One Piece event the way the catalog names it, for
// the names this storefront gives it instead. They are its own: the catalog
// sells the card in "BANDAI Card Games Fest 25-26" and it goes up here as
// the Afro Luffy promo, after the art rather than the pack. A nickname
// names one product, or one numbered series of it, and nothing else, which
// is why they are listed one at a time rather than read for.
var onePieceEvents = map[string]string{
	"afro luffy promo":    "BANDAI Card Games Fest 25-26",
	"l.a. dodgers promo":  "Dodgers x One Piece",
	"best selection vol.": "Premium Card Collection",
	// The playmat and the participation pack name the product the card came
	// in, where the catalog names the event that handed it out.
	"bcgf playmat promo":                             "Official Playmat -Bandai Card Games Fest 24-25 Edition-",
	"offline regional participation pack 2024 vol.2": "Offline Regional 2024 Vol. 2 Participant",
	"premium card collection - film red edition":     "Premium Card Collection ONE PIECE FILM RED Edition",
}

// onePiecePIDs names the TCGplayer product a storefront product is, for the
// listings whose wording cannot reach it: the storefront describes a DON!!
// card and a reprinted promo in prose the catalog never uses, and the catalog
// tells them apart by promo types and watermarks the storefront does not
// write. A few more are listings whose art or number the shelf's own edition
// keeps the matcher from reaching. The product id is the storefront's own and
// both feeds share it.
var onePiecePIDs = map[string]string{
	"353757": "456059", // OP01 DON "The King of the Pirates" = OP01 Manga Alternate Art
	"357696": "482273", // OP02 DON "To Put An End To This War!!" = OP02 Manga
	"368617": "483145", // DON (Tournament Pack Vol. 2) participant, no stamp
	"368620": "482237", // DON Red (DON!! Card Pack Vol. 1)
	"368621": "483170", // DON Teal (Vol. 2)
	"368674": "482239", // DON Blue (Vol. 1)
	"368675": "483171", // DON Bronze (Vol. 2)
	"368676": "483167", // DON Green (Vol. 2)
	"371229": "483168", // DON Orange (Vol. 2)
	"371230": "483169", // DON Pink (Vol. 2)
	"371231": "482240", // DON Purple (Vol. 1)
	"371232": "482241", // DON Silver (Vol. 1)
	"371233": "482238", // DON Yellow (Vol. 1)
	"371821": "517476", // OP04 DON "Will you call me your Shipmate?!!" = OP04 Alternate Art
	"406129": "517477", // OP04 DON Rosinante & Law (Double Pack Set Vol. 1) = Special DON!! Card Pack Color
	"406130": "517478", // OP04 DON Crocodile & Luffy (Double Pack Set Vol. 1) = Special DON!! Card Pack Black & White
	"378041": "529792", // OP05 DON "Luffy Punching Kaido" = OP05 Alternate Art
	"384872": "541671", // OP06 DON "Zoro & Sanji" = OP06 Alternate Art
	"391902": "555894", // OP07 DON "Five Remaining Warlords" = OP07 Alternate Art
	"404440": "604246", // OP09 DON "Are you that afraid of the new era?!" = OP09 Alternate Art
	"417147": "636742", // OP11 DON "... They Believe In Me" = OP11 Alternate Art
	"422589": "646574", // OP12 DON "It's My Student's Farewell." = OP12 Alternate Art
	"433483": "672738", // OP14 DON (Sand Tornado) DP Vol. 9 = Crocodile
	"433484": "672736", // OP14 DON (Smile and Strings) DP Vol. 9 = Donquixote Doflamingo
	"440300": "698313", // OP16 DON "We'll Have To Break Out" = OP16 Alternate Art
	"440301": "698314", // OP16 DON same (GOLD) = Alternate Art Gold
	"447228": "712747", // OP17 DON Luffy = Alternate Art
	"447229": "710859", // OP17 DON Luffy (GOLD) = Alternate Art Gold
	"447230": "712748", // OP17 DON Luffy & Loki
	"447231": "710860", // OP17 DON Luffy & Loki (GOLD)
	"447232": "712750", // OP17 DON Four Emperors
	"447233": "711421", // OP17 DON Four Emperors (GOLD)
	"447234": "712749", // OP17 DON Xebec = Alternate Art Rocks
	"447235": "711420", // OP17 DON Xebec (BLUE) = Rocks Special Foil
	"447236": "715682", // OP17 DON Rocks.D.Xebec (Double Pack Vol. 12)
	"447636": "681871", // DON (Red Bull Don!!)
	"354630": "457032", // Shanks P-016 nonfoil = Film Red
	"354622": "457039", // Monkey.D.Luffy P-022 (Film Red) nonfoil
	"379043": "518702", // Monkey.D.Luffy P-055 nonfoil = Sealed Battle Kit Vol. 1
	"379048": "518696", // Usopp P-049 nonfoil = Sealed Battle Kit Vol. 1
	"387345": "537438", // Roronoa Zoro P-045 (OP06 Pre-Release Tournament) nonfoil = Participant
	"400504": "586897", // Donquixote Doflamingo OP04-031 (Alternate Art) = PRB-01 Alternate Art
	"424023": "656166", // Sabo P-044 White Border = PRB-02 Reprint
	"424027": "656192", // Koala P-069 White Border = PRB-02 Reprint
	"424028": "656199", // Carrot P-070 White Border = PRB-02 Reprint
	"424029": "656209", // Sabo P-073 = PRB-02 Reprint
	"424030": "656212", // Portgas.D.Ace P-074 = PRB-02 Reprint
	"424031": "656216", // Monkey.D.Luffy P-075 = PRB-02 Reprint
	"424032": "656223", // Adio P-078 = PRB-02 plain
	"424033": "656229", // Lim P-079 = PRB-02 plain
	"424036": "656237", // Shanks P-083 = PRB-02 Reprint
	"424037": "656230", // Jewelry Bonney P-085 = PRB-02 Reprint
	"424038": "656220", // Trafalgar Law P-088 = PRB-02 Reprint
	"425300": "656177", // Jinbe P-063 = PRB-02 Reprint
	"384878": "541670", // Rebecca OP05-091 (SP) In Sunflower Field = OP06 SP
	"423973": "654571", // Rebecca OP05-091 (SP) Stitched Together Border = PRB-02 SP
	"386653": "539201", // Scratchmen Apoo ST02-008 Offline Regional 2024: the feed's Number says ST03-008
	"354624": "450300", // Monkey.D.Luffy P-001 (Super Pre-Release) nonfoil = participant, no stamp
	"354623": "450299", // Monkey.D.Luffy P-001 White Border Large Fist nonfoil = Promotion Pack 2022
	"378075": "527027", // Monkey.D.Luffy ST01-012 (OP05 1st Anniversary Special) = OP05 Alternate Art
	"417100": "632776", // Monkey.D.Luffy OP09-061 (Starter Deck) = ST-26
	"417106": "632819", // Marshall.D.Teach OP09-081 (Starter Deck) = ST-27
	"417112": "633760", // Shanks OP09-001 (Starter Deck) = ST-23
	"417118": "633944", // Buggy OP09-042 (Starter Deck) = ST-25
	"417125": "634599", // Jewelry Bonney OP07-019 (Starter Deck) = ST-24
	"417130": "634623", // Yamato OP06-022 (Starter Deck) = ST-28
	"434532": "681614", // Jewelry Bonney P-113 (Heroines Battle Pack - Winner) foil = Winner Pack
	"386661": "530120", // Vol. 2 DON Luffy & Kaido = Manga, beside 386659 = Silhouette
	"397181": "577568", // OP08 DON "My Beloved Son..." = Alternate Art, beside 398704 and 398705
	"447221": "705991", // Roronoa Zoro EB04-007 (Haki SP) = OP17 SP: the feed's Number says EB04-061
	"447143": "712091", // Kaido OP17-062 (Baki Alternate Art) = Super Alternate Art, beside 447141 = Alternate Art
	"421641": "643883", // Shanks OP09-001 (PSA Magazine Promo) = 2025 PSA Magazine, not the OP09 Parallel
	"426348": "648101", // Boa Hancock OP07-051 (Binder Set Alt-Art) = Binder Set Alternate Art
	"447218": "712119", // Shanks OP13-028 SP = OP17 SP: the feed's Number says OP13-029
	"377222": "527620", // Roronoa Zoro OP01-025 (Starter Deck 10) = Ultra Deck: The Three Captains
	"433349": "675746", // Koala OP05-006 (Dash Pack) = EB03 reprint, the one sibling without a Dash Pack label
}

// onePieceStarterDeck matches the starter deck a name states in brackets.
var onePieceStarterDeck = regexp.MustCompile(`\(Starter Deck (\d+)\)`)

// onePieceNotePlace is the finishing place a buy row's note names. The name
// carries the event and the note the place, and the catalog labels the
// printing by both: without the place, the plain card wins.
var onePieceNotePlace = regexp.MustCompile(`(?i)\b(?:participant|winner|finalist)\b`)

// onePieceSpellings spells the One Piece wording this storefront writes its
// own way: the Heroines Edition event card lost a word, the Event Pack card
// lost the dash before its number, the eighth Winner Pack is the catalog's
// October to December one, a Jolly Roger foil is named by half its name, and
// an Alternate Art Promo is a Premium Card Collection card.
// Only the part that differs is rewritten, so the number or bracket behind it
// is kept.
var onePieceSpellings = strings.NewReplacer(
	"But If We See Each Other Again...Will You Call Me Your Shipmate?!!", "But If We Ever See Each Other Again... Will You Call Me Your Shipmate?!!",
	"Kouzuki Momonosuke P-064", "Kouzuki Momonosuke - P-064",
	"Winner Pack Vol. 8", "Winner Pack 2024 Oct.-Dec.",
	"(Jolly Roger)", "(Jolly Roger Foil)",
	"(Jolly Roger Art)", "(Jolly Roger Foil)",
	"(Alternate Art Promo)", "(Premium Card Collection)",
)

// onePieceSpelling spells One Piece wording the way the catalog does.
func onePieceSpelling(wording string) string {
	return onePieceSpellings.Replace(wording)
}

// onePieceShelf answers the set a One Piece listing belongs to, which is the
// shelf it arrived on except where that shelf says only "Promo".
//
// A starter deck card reprinted as a promo is filed here under the promo
// shelf with the deck named in brackets, and the promo shelf holds a printing
// of its own at the same number: the P-041 Luffy is both the plain promo and
// the Starter Deck 18 card. The two meet, and the $0.50 deck card would be
// priced as the $60.00 promo standing beside it.
//
// The bracket only decides where the shelf has nothing to say. Every other
// listing naming a deck arrives on a real set already - a Backlight on ST11,
// a Kuzan on OP12 - and there the shelf is what the catalog agrees with,
// while the bracket names the deck the card was reprinted from.
func onePieceShelf(shelf, name string) string {
	if shelf != "Promo" {
		return shelf
	}
	match := onePieceStarterDeck.FindStringSubmatch(name)
	if match == nil {
		return shelf
	}
	return "Starter Deck " + match[1]
}

// onePieceRenamedTreatment answers the printing a One Piece listing means
// when it names its treatment with a word the catalog does not use for that
// card, and "" wherever the listing is already answered.
//
// This storefront calls the Gear5 starter deck's premium printing "Full Art"
// where the catalog files every alternate printing of that set as "Parallel",
// so the word named no label there and the row settled on the plain card - a
// $15.00 Monkey.D.Luffy priced as the $0.50 one beside it.
//
// The guard is what keeps it from touching a real Full Art. The word must
// name a label the catalog uses somewhere, so a typo reaches nothing; the
// card's own number must hold no printing of it, which is false for every
// real Full Art printing; and the number must hold a single alternate
// printing, so the one printing the storefront can mean is the one the
// number carries. A listing naming a second qualifier is not answered by the
// treatment alone, so it is left where it landed.
func onePieceRenamedTreatment(b *matcher.Backend, id, name string) string {
	co, err := b.GetUUID(id)
	if err != nil || len(co.PromoTypes) > 0 {
		return ""
	}
	set, err := b.GetSet(co.SetCode)
	if err != nil {
		return ""
	}

	labels := map[string]bool{}
	var alternate string
	for _, card := range set.Cards {
		if card.Number != co.Number || len(card.PromoTypes) == 0 {
			continue
		}
		if alternate != "" && alternate != card.UUID {
			return ""
		}
		alternate = card.UUID
		for _, promoType := range card.PromoTypes {
			labels[promoType] = true
		}
	}
	if alternate == "" {
		return ""
	}

	qualifiers := nameQualifierList(name)
	if len(qualifiers) != 1 {
		return ""
	}
	slug := matcher.PromoTypeSlug(qualifiers[0])
	if slug == "" || labels[slug] || !slices.Contains(b.AllPromoTypes, slug) {
		return ""
	}
	return alternate
}

// onePieceAlternatePromo reports a listing of the 25th Edition premium card
// collection. The storefront shelves its cards on their own set, where the
// catalog files them on the promo shelf as the card's Alternate Art, and the
// "Manga Background" its notes describe routes the card to a manga printing.
func onePieceAlternatePromo(name string) bool {
	return strings.Contains(strings.ToLower(name), "premium card collection - 25th edition")
}

// onePieceParallelPrinting answers the printing a "(Alternate Art)" listing
// means when the catalog calls that printing "Parallel", and "" otherwise.
//
// The wording names a label another set uses for the number: the PRB-01
// reprints and the manga rows are filed as Alternate Art, so the listing
// lands there beside the cheaper printing it means. The shelf's own set
// settles it when it holds one parallel printing of the number and no plain
// Alternate Art of it, which is false for every set filing its alternate
// arts under that name. Only a printing in the finish the match settled on
// counts, and a match on the set the note names as the reprint stays there.
func onePieceParallelPrinting(b *matcher.Backend, id, edition, name, notes string) string {
	if !slices.Equal(nameQualifierList(name), []string{"Alternate Art"}) {
		return ""
	}
	co, err := b.GetUUID(id)
	if err != nil {
		return ""
	}
	reprint := onePieceReprintNote.FindStringSubmatch(notes)
	if reprint != nil && strings.EqualFold(strings.ReplaceAll(co.SetCode, "-", ""), reprint[1]) {
		return ""
	}
	set, err := b.GetSetByName(edition)
	if err != nil {
		return ""
	}

	var parallel string
	for _, card := range set.Cards {
		if card.Number != co.Number || !slices.Equal(card.Finishes, co.Finishes) {
			continue
		}
		switch {
		case slices.Equal(card.PromoTypes, []string{"parallel"}):
			if parallel != "" {
				return ""
			}
			parallel = card.UUID
		case slices.Contains(card.PromoTypes, "alternateart") && !slices.Contains(card.PromoTypes, "manga"):
			return ""
		}
	}
	return parallel
}

// onePieceReprintNote matches the set a note names a listing as a reprint
// from, "PRB01 Reprint - OP01-120", spelled without the catalog's dash.
var onePieceReprintNote = regexp.MustCompile(`(?i)\b([a-z]+\d+) reprint\b`)

// onePieceRefined answers the printing a One Piece listing means once the
// match has settled on one, where its own wording says it is another.
func onePieceRefined(b *matcher.Backend, id, edition, name, notes string) string {
	renamed := onePieceRenamedTreatment(b, id, name)
	if renamed != "" {
		return renamed
	}
	parallel := onePieceParallelPrinting(b, id, edition, name, notes)
	if parallel != "" {
		return parallel
	}
	return id
}

// onePieceNonfoilPrinting answers the printing a nonfoil listing means once
// the match settled on id. A printing sold in no nonfoil finish is not the
// card a nonfoil row prices: the storefront's nonfoil row of such a rare is
// the starter deck or reprint printing of the same name and number, so that
// is the answer where exactly one exists. Where none or several do, the row
// is a foil-only printing listed without the foil marker and stays on id. A
// printing with a promo type (an alternate art, a parallel, an event promo)
// stays too: the plain printing at its number is not it in another finish.
func onePieceNonfoilPrinting(b *matcher.Backend, id string) string {
	co, err := b.GetUUID(id)
	if err != nil || co.HasFinish(matcher.FinishNonfoil) || len(co.PromoTypes) > 0 {
		return id
	}
	siblings, err := b.SearchEquals(co.Name)
	if err != nil {
		return id
	}
	var found string
	for _, sibling := range siblings {
		other, err := b.GetUUID(sibling)
		if err != nil || other.Number != co.Number || other.Foil || !other.HasFinish(matcher.FinishNonfoil) {
			continue
		}
		if found != "" {
			return id
		}
		found = sibling
	}
	if found == "" {
		return id
	}
	return found
}

// eventNamed adds the catalog's name for every event the wording gives its
// own name to. The storefront's words stay: they are what the listing says
// about the art, and the catalog's name is only what files it.
func eventNamed(wording string) string {
	lower := strings.ToLower(wording)
	for nickname, event := range onePieceEvents {
		if strings.Contains(lower, nickname) {
			wording += " " + event
		}
	}
	return wording
}

// onePieceDonHead matches the wrapping the storefront hangs in front of
// a DON!! card's description, numbered or not.
var onePieceDonHead = regexp.MustCompile(`(?i)^DON!!\s*(?:\([0-9]+\))?\s*-?\s*`)

// onePieceDonCharacters spells the four characters this storefront
// names in full where the catalog names them by the name the promo type
// carries. They are not misspellings and not this storefront's
// invention - both names are the character's - so a listing saying
// "Edward Newgate" is describing the card the catalog files under
// "whitebeard", and neither wording reaches the other on its own.
//
// Read as a second attempt, never as a correction. Some promo types
// carry the full name themselves - the first anniversary's DON!! is
// "monkeydluffy1st" - so rewriting every listing would lose the cards
// whose catalog wording the storefront already matched. Only the four
// the storefront actually differs on are here, and a character the
// catalog spells differently again stays refused: every DON!! in a set
// shares a name and a number, so a guess buys another card.
var onePieceDonCharacters = strings.NewReplacer(
	"Edward Newgate", "Whitebeard",
	"Charlotte Linlin", "Big Mom",
	"Monkey.D.Luffy", "Luffy",
	"Portgas.D.Ace", "Ace",
)

// onePieceDonName answers a DON!! listing the way the catalog files it,
// and reports whether the listing is one at all.
//
// The catalog names all 238 of the game's DON!! cards "DON!! Card" and
// tells them apart by promo type alone - the character, the artwork,
// the border - which the matcher already reads out of a listing's own
// wording. The storefront writes those same words into the product name
// and publishes no collector number for a DON!! at all, so the name
// reaches nothing and the wording never gets as far as the rules that
// would have read it. Hand over the name the catalog uses and leave the
// description where a description belongs.
func onePieceDonName(name string) (string, string, bool) {
	if !strings.HasPrefix(strings.ToUpper(name), "DON!!") {
		return "", "", false
	}
	return "DON!! Card", onePieceDonHead.ReplaceAllString(name, ""), true
}

// onePieceDonRenamed answers the same description with the characters
// the catalog names differently swapped in, and an empty string when it
// names none of them.
func onePieceDonRenamed(description string) string {
	renamed := onePieceDonCharacters.Replace(description)
	if renamed == description {
		return ""
	}
	return renamed
}

// retailOnePiece answers an offer by its product id where the wording cannot
// reach it, reads the promos and DON!! cards apart, and refines the printing
// the wording lands on.
func retailOnePiece(csi *Coolstuffinc, l sellListing) (*matcher.InputCard, string, error) {
	b := csi.backend
	theCard := onePieceRetailCard(b, l)
	cardID, err := l.match(b, theCard)
	if err != nil {
		return theCard, "", err
	}
	cardID = onePieceRefined(b, cardID, theCard.Edition, l.name, l.notes)
	// A slab's finish, or a named printing's, is not told by the
	// foil prefix, and a product id names its own printing.
	if !l.foil && !l.graded && !l.slabbed && l.printing == "" && theCard.ID == "" {
		cardID = onePieceNonfoilPrinting(b, cardID)
	}
	return theCard, cardID, nil
}

// onePieceRetailCard reads an offer: its product id, an alternate-art promo,
// a DON!! card, or else its name, shelf and wording.
func onePieceRetailCard(b *matcher.Backend, l sellListing) *matcher.InputCard {
	tcgID, named := onePiecePIDs[l.pid]
	if named {
		return &matcher.InputCard{ID: tcgID, Foil: l.foil}
	}
	shelf := onePieceShelf(l.edition, l.name)
	if onePieceAlternatePromo(l.name) {
		return &matcher.InputCard{Name: onePieceSpelling(l.name), Edition: "One Piece Promotion Cards", Variation: "Alternate Art", Foil: l.foil}
	}
	donName, donDescription, isDon := onePieceDonName(l.name)
	if isDon {
		theCard := &matcher.InputCard{Name: donName, Edition: shelf, Variation: donDescription, Foil: l.foil}
		_, err := b.Match(theCard)
		if err != nil {
			renamed := onePieceDonRenamed(donDescription)
			if renamed != "" {
				theCard = &matcher.InputCard{Name: donName, Edition: shelf, Variation: renamed, Foil: l.foil}
			}
		}
		return theCard
	}
	spelled := onePieceSpelling(jpArtWording(l.name))
	return &matcher.InputCard{Name: spelled, Edition: shelf, Variation: eventNamed(strings.TrimSpace(nameQualifiers(spelled) + " " + onePieceSpelling(jpArtWording(l.notes)))), Foil: l.foil}
}

// buylistOnePiece reads a buylist row the way retailOnePiece reads an offer.
func buylistOnePiece(csi *Coolstuffinc, p CSIPriceEntry) (*matcher.InputCard, string, error) {
	b := csi.backend
	theCard := onePieceBuylistCard(b, p)
	cardID, err := b.Match(theCard)
	if err != nil {
		return theCard, "", err
	}
	cardID = onePieceRefined(b, cardID, theCard.Edition, p.Name, p.Notes)
	if p.IsFoil == 0 && theCard.ID == "" {
		cardID = onePieceNonfoilPrinting(b, cardID)
	}
	return theCard, cardID, nil
}

// onePieceBuylistCard reads the row's number and only the place its note
// names: One Piece spends the rest of the note describing the artwork.
func onePieceBuylistCard(b *matcher.Backend, p CSIPriceEntry) *matcher.InputCard {
	foil := p.IsFoil == 1
	tcgID, named := onePiecePIDs[p.PID]
	if named {
		return &matcher.InputCard{ID: tcgID, Foil: foil}
	}
	if onePieceAlternatePromo(p.Name) {
		return &matcher.InputCard{Name: onePieceSpelling(p.Name), Edition: "One Piece Promotion Cards", Variation: "Alternate Art", Foil: foil}
	}
	donName, donDescription, isDon := onePieceDonName(p.Name)
	if isDon {
		donShelf := onePieceShelf(p.ItemSet, p.Name)
		theCard := &matcher.InputCard{Name: donName, Edition: donShelf, Variation: donDescription, Foil: foil}
		_, err := b.Match(theCard)
		if err != nil {
			renamed := onePieceDonRenamed(donDescription)
			if renamed != "" {
				theCard = &matcher.InputCard{Name: donName, Edition: donShelf, Variation: renamed, Foil: foil}
			}
		}
		return theCard
	}
	spelled := onePieceSpelling(jpArtWording(p.Name))
	return &matcher.InputCard{Name: spelled, Edition: onePieceShelf(p.ItemSet, p.Name), Variation: eventNamed(strings.TrimSpace(p.Number + " " + nameQualifiers(spelled) + " " + onePieceNotePlace.FindString(p.Notes))), Foil: foil}
}
