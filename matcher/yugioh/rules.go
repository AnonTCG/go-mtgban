package yugioh

import (
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/mtgban/go-mtgban/matcher"
)

// Rules implements matcher.GameRules for Yu-Gi-Oh!. A card is identified
// by name + collector number + rarity: the same number exists as several
// products differing only by rarity, so the rarity wording (or the rarity
// suffix cardtrader appends to its collector numbers) is what tells them
// apart. The print run never gates anything: a product carries every run it
// was priced in, exactly as foil never gates One Piece. It does route: an
// input naming a run - in the finish field, or in the wording for the
// storefronts that have no such field - resolves to that run's entry
// instead of the default one.
type Rules struct {
	matcher.DefaultRules

	// qualifiers are the name-qualifiers the catalog sells a printing
	// under, by uuid: "Mayhem Fur Hire (Starlight Rare)" beside a plain
	// "Mayhem Fur Hire" at the same number, "Dark Magician (Arkana)"
	// beside its 6th to 9th Arts. The matcher's Card has no field for the
	// qualifier, and the tiers need two things of it: whether a printing
	// wears one at all, since a listing that says nothing means the plain
	// product, and whether the wording spells it whole, since then the
	// listing has named the product. They are the backend's that loaded
	// them, built with it and gone with it.
	qualifiers map[string]string
}

// qualifierOf is the qualifier a printing was sold under, or "".
func qualifierOf(qualifiers map[string]string, uuid string) string {
	return qualifiers[uuid]
}

// fullNumberRe matches the game's collector number shapes: "LOB-001",
// "RA01-EN019", "YGLD-ENA03", with an optional letter tail (cardtrader
// suffixes rarities "RA01-EN019qsec", Konami suffixes misprints
// "EOJ-EN004K").
var fullNumberRe = regexp.MustCompile(`^[A-Za-z0-9]+-[A-Za-z]*[0-9]+[a-zA-Z]*$`)

// suffixRarities maps the lowercase letter tail cardtrader appends to a
// collector number to the rarity it encodes; a further "a" tail marks the
// alternate-art printing.
var suffixRarities = map[string]string{
	"u":    "Ultra Rare",
	"sec":  "Secret Rare",
	"qsec": "Quarter Century Secret Rare",
	"cr":   "Collector's Rare",
	"ul":   "Ultimate Rare",
	"psec": "Platinum Secret Rare",
	"sh":   "Shatterfoil Rare",
}

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// Prefilter splits the parenthetical decorations off the name before the
// canonical-name lookup: TCGplayer writes "Eldlich the Golden Lord (Quarter
// Century Secret Rare)" and "Harpie Lady (Original Artwork)". A full name
// that is itself canonical stays as it is - some catalog names carry their
// qualifier parenthetical ("Dark Magician (A)"). The name then takes the
// spelling its own edition files it under, when the two disagree.
func (Rules) Prefilter(b *matcher.Backend, inCard *matcher.InputCard) {
	_, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]
	if !found && strings.Contains(inCard.Name, "(") {
		vars := matcher.SplitVariants(inCard.Name)
		if len(vars) > 1 {
			inCard.Name = vars[0]
			inCard.AddToVariant(strings.Join(vars[1:], " "))
		}
	}
	// A storefront brackets the same thing it elsewhere parenthesizes -
	// "Knightmare Unicorn [Alt Art]" - and the bracket says as much about
	// the printing as the parenthesis does, so it is read the same way
	// rather than being carried into a name no card answers to.
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; !found {
		if name, bracketed, ok := splitBracket(inCard.Name); ok {
			inCard.Name = name
			inCard.AddToVariant(bracketed)
		}
	}
	inCard.Variation = variantRespellings.Replace(inCard.Variation)
	respellName(b, inCard)
	adoptQualifiedName(b, inCard)
}

// bracketRe matches the decoration a storefront brackets onto a name, which
// says what a parenthetical says: "Knightmare Unicorn [Alt Art]".
var bracketRe = regexp.MustCompile(`^(.*?)\s*\[([^\]]+)\]\s*$`)

// splitBracket separates a bracketed decoration from the name it decorates,
// reporting whether there was one to take.
func splitBracket(name string) (string, string, bool) {
	match := bracketRe.FindStringSubmatch(name)
	if match == nil {
		return name, "", false
	}
	return strings.TrimSpace(match[1]), strings.TrimSpace(match[2]), true
}

// qualifiedNameRe matches the trailing parenthetical the catalog keeps inside
// some names instead of distilling it into a variant label: "Cyber Dragon
// (Alternate Art)", "White Elephant's Gift (A)". 141 rows are spelled this
// way while 1979 others have the same kind of qualifier taken out.
var qualifiedNameRe = regexp.MustCompile(`\s*\([^()]*\)$`)

// adoptQualifiedName adopts the catalog's decorated spelling of a name the
// storefront wrote bare, and the collector number is what licenses it.
//
// A name filed with its qualifier inside it has no bare-name bucket, so the
// listing looks up nothing at all: every candidate is deleted and the row
// dies as "unknown variant" - all six of the cardtrader Yu-Gi-Oh failures of
// this shape are cards the catalog spells "... (Alternate Art)" or "... (A)".
// Adding the bare spelling as a second key would reach them, and would also
// hand the deck-letter sets a coin flip: the number compare reads only the
// trailing digit run, so ENA32 and ENB32 are one number to it.
//
// The number keeps that from happening, three ways. The set is the one the
// edition names, so no printing outside it is reachable. A set printing the
// bare name at that very number keeps it, since the storefront's spelling is
// then a spelling the set has. And two decorated siblings on one number
// decide nothing, so they refuse instead of picking.
//
// A number the set spells exactly as the input wrote it speaks with that
// exactness throughout. The loose compare reads only the trailing digit
// run, which serves the storefronts writing shorthand, but the Speed Duel
// sets file a card per deck letter under one run: ENA01 and ENG01 are one
// number to it, so the bare name at ENG01 would answer for the qualified
// printing at ENA01 and the qualified spelling would never be adopted, so
// the listing would leave with the sibling's printing rather than its own.
func adoptQualifiedName(b *matcher.Backend, inCard *matcher.InputCard) {
	number := extractNumber(inCard.Variation)
	if number == "" {
		return
	}
	set := editionSet(b, inCard.Edition)
	if set == nil {
		return
	}
	matches := numberMatches
	if setSpellsNumber(set, number) {
		matches = strings.EqualFold
	}
	if carries(set, inCard.Name, number, matches) {
		return
	}
	var adopt string
	for i := range set.Cards {
		card := &set.Cards[i]
		stem := qualifiedNameRe.FindStringIndex(card.Name)
		if stem == nil || !matcher.Equals(card.Name[:stem[0]], inCard.Name) ||
			!matches(number, card.Number) {
			continue
		}
		if adopt != "" && !matcher.Equals(adopt, card.Name) {
			return
		}
		adopt = card.Name
	}
	if adopt != "" {
		inCard.Name = adopt
	}
}

// spellsNumber reports whether the input writes the collector number out in
// full, language infix included, ignoring a rarity or misprint letter tail.
func spellsNumber(input, full string) bool {
	return strings.EqualFold(input, full) ||
		strings.EqualFold(strings.TrimRight(input, letters), full)
}

// setSpellsNumber reports whether the set prints the number exactly as the
// input wrote it.
func setSpellsNumber(set *matcher.Set, number string) bool {
	for i := range set.Cards {
		if strings.EqualFold(number, set.Cards[i].Number) {
			return true
		}
	}
	return false
}

// carries is editionCarries under a caller-chosen number compare.
func carries(set *matcher.Set, name, number string, matches func(string, string) bool) bool {
	normalized := matcher.Normalize(name)
	for i := range set.Cards {
		if matcher.Normalize(set.Cards[i].Name) != normalized {
			continue
		}
		if number == "" || matches(number, set.Cards[i].Number) {
			return true
		}
	}
	return false
}

// respellName adopts the spelling the input's own edition files the card
// under, when the edition resolves to a set that does not carry the name as
// written: the storefront respellings nameRespellings pairs (cardtrader
// writes Magician's Force's "Vampire Orchis" as "Vampiric Orchis", the name
// of the real newer DASA card) and the token word-order flip in either
// direction ("Sky Striker Ace Token" against OTS Tournament Pack 8's
// "Token: Sky Striker Ace"). A token name the spellings cannot place is
// asked of the set's own token sheet by collector number ("Yugi & Dark
// Magician Token" T01 is Legendary Decks II's "Token: Yugi").
//
// The edition is the whole guard, three ways. A name the resolved set does
// carry is never touched, so the DASA "Vampiric Orchis" and the sets naming
// a token the storefront's way keep their own printing. A set carrying
// several distinct respellings, or several token names on one number (OTS
// Tournament Pack 9 prints three "Token: Mecha Phantom Beast" arts under
// 026), decides nothing and the name stays as written. And an edition
// naming no set at all decides nothing either - AdjustName's fallbacks own
// that input.
func respellName(b *matcher.Backend, inCard *matcher.InputCard) {
	alternates := alternateNames(inCard.Name)
	token := isTokenName(inCard.Name)
	if len(alternates) == 0 && !token {
		return
	}
	set := editionSet(b, inCard.Edition)
	if set == nil {
		return
	}
	number := extractNumber(inCard.Variation)
	// A set printing the name as written settles it whatever the number
	// says: cardtrader numbers a few tokens one off ("Mask Token" 028 for
	// OTS Tournament Pack 19's EN029), and following such a number across
	// a respelling would price the neighbor.
	if editionCarries(set, inCard.Name, "") {
		return
	}
	// A number naming the card as written in the set's Worldwide English
	// reprint ("LOD-EN060") is that printing, which the loose number
	// compare would read as the respelled one's ("LOD-060").
	if number != "" {
		worldwide, err := b.GetSet(set.Code + "-EN")
		if err == nil && carries(worldwide, inCard.Name, number, spellsNumber) {
			return
		}
	}
	var adopt string
	for _, alternate := range alternates {
		if !editionCarries(set, alternate, number) {
			continue
		}
		if adopt != "" && matcher.Normalize(adopt) != matcher.Normalize(alternate) {
			return
		}
		adopt = alternate
	}
	if adopt == "" {
		// A spelling the set prints under some other number is the same
		// name-against-number contradiction, so the number decides nothing
		// here either.
		for _, alternate := range alternates {
			if editionCarries(set, alternate, "") {
				return
			}
		}
		if token && number != "" {
			adopt = editionTokenAt(set, number)
		}
	}
	if adopt != "" {
		inCard.Name = adopt
	}
}

// numberedNameRe matches the mark the catalog numbers a repeated name with,
// "Skull Knight #2", as the storefronts spell it instead. No catalog name
// ends in the wording and 32 end in the mark, so the rewrite has one reading.
var numberedNameRe = regexp.MustCompile(`\s+No\.?\s*([0-9]+)$`)

// variantRespellings spells a decoration the way the catalog files it, for
// the shorter form a storefront writes instead. The catalog calls a printing's
// second artwork "Alternate Art"; Cool Stuff Inc brackets it "[Alt Art]", and
// the short spelling named no label at all - nine of its cards asked for an
// alternate art and were answered with the plain printing standing at the same
// number and rarity, a $37.99 House Dragonmaid served as the $4.99 one.
var variantRespellings = strings.NewReplacer(
	"Alt Art", "Alternate Art",
)

// greekLetters spells the letters the catalog writes as words, "Falchion
// Beta", and no catalog name carries the letter itself.
var greekLetters = strings.NewReplacer(
	"\u03b1", "Alpha", "\u0391", "Alpha",
	"\u03b2", "Beta", "\u0392", "Beta",
	"\u03b3", "Gamma", "\u0393", "Gamma",
)

// alternateNames lists the other spellings a name might be filed under: its
// nameRespellings sibling, read in both directions, the token word-order
// flip, either way around, and the marks the catalog spells where a
// storefront writes the words.
func alternateNames(name string) []string {
	var alternates []string
	normalized := matcher.Normalize(name)
	for _, pair := range nameRespellings {
		if matcher.Normalize(pair[0]) == normalized {
			alternates = append(alternates, pair[1])
		}
		if matcher.Normalize(pair[1]) == normalized {
			alternates = append(alternates, pair[0])
		}
	}
	if base, cut := strings.CutSuffix(name, " Token"); cut {
		alternates = append(alternates, "Token: "+base)
	}
	if base, cut := strings.CutPrefix(name, "Token: "); cut {
		alternates = append(alternates, base+" Token")
	}
	// The field centers wear the same flip around a longer phrase, and the
	// generic one would leave the words it names inside the base.
	if base, cut := strings.CutSuffix(name, " Field Center Token"); cut {
		alternates = append(alternates, "Field Center Token: "+base)
	}
	if numbered := numberedNameRe.ReplaceAllString(name, " #$1"); numbered != name {
		alternates = append(alternates, numbered)
	}
	if spelled := greekLetters.Replace(name); spelled != name {
		alternates = append(alternates, spelled)
	}
	return alternates
}

// solePrefixName answers the one name the given spellings are a prefix of,
// reporting whether the prefixes reached exactly that name and no other.
func solePrefixName(b *matcher.Backend, prefixes []string) (string, bool) {
	var match, matchNorm string
	for _, prefix := range prefixes {
		uuids, err := b.SearchHasPrefix(prefix)
		if err != nil {
			continue
		}
		for _, uuid := range uuids {
			co, err := b.GetUUID(uuid)
			if err != nil || co.Sealed {
				continue
			}
			norm := matcher.Normalize(co.Name)
			if match != "" && matchNorm != norm {
				return "", false
			}
			match, matchNorm = co.Name, norm
		}
	}
	return match, match != ""
}

// isTokenName reports whether a name names a token outright, in any of the
// word orders the catalog and the storefronts use - including the bare
// "Token" the sheets that never name their art are filed under.
func isTokenName(name string) bool {
	return strings.HasPrefix(name, "Token: ") || strings.HasSuffix(name, " Token") ||
		name == "Token"
}

// editionSet answers the set the input's edition names, through the same
// aliasing and decoration-trimming AdjustEdition applies, without touching
// the input.
func editionSet(b *matcher.Backend, edition string) *matcher.Set {
	edition = strings.TrimSpace(edition)
	named, found := namedSet(b, edition)
	if !found {
		named, found = namedSet(b, trimEditionDecorations(edition))
	}
	if !found {
		return nil
	}
	return b.NormalizedSets[matcher.Normalize(named)]
}

// editionCarries reports whether the set prints the name, under the given
// collector number when one is known.
func editionCarries(set *matcher.Set, name, number string) bool {
	return carries(set, name, number, numberMatches)
}

// editionHoldsNumber is editionCarries widened by the same loose volume-index
// reading FilterCards itself relaxes into within a named edition ("5-001"
// for DL5-EN001): a caller deciding whether to keep trusting the edition has
// to honor it too, or a listing that only resolves through it loses the
// edition that reading depends on.
func editionHoldsNumber(set *matcher.Set, name, number string) bool {
	if editionCarries(set, name, number) {
		return true
	}
	if loosePrefixNumber(number) {
		return carries(set, name, number, loosePrefixMatches)
	}
	return false
}

// editionTokenAt answers the one token the set prints under the number, or
// nothing when the number names none - or several, which no storefront
// wording tells apart.
func editionTokenAt(set *matcher.Set, number string) string {
	var name string
	for i := range set.Cards {
		card := &set.Cards[i]
		if !isTokenName(card.Name) || !numberMatches(number, card.Number) {
			continue
		}
		if name != "" && matcher.Normalize(name) != matcher.Normalize(card.Name) {
			return ""
		}
		name = card.Name
	}
	return name
}

// letteredNumber reads the letter-led numbers the token sheets and deck
// reprints use ("T01"), which extractNumber refuses everywhere else because
// outside a token's variation a letter-led field is a label word rather
// than a number.
func letteredNumber(variation string) string {
	for _, field := range strings.Fields(variation) {
		if len(field) < 2 || len(field) > 4 || !unicode.IsLetter(rune(field[0])) {
			continue
		}
		if !strings.ContainsFunc(field[1:], func(r rune) bool {
			return r < '0' || r > '9'
		}) {
			return field
		}
	}
	return ""
}

// AdjustName flips a token's name onto the word order the catalog files it
// under, and otherwise provides a prefix fallback for truncated feeds,
// adopting the one name among the prefix matches that carries the input's
// number. Names compare normalized, so punctuation variants of one name are
// not read as ambiguity.
func (Rules) AdjustName(b *matcher.Backend, inCard *matcher.InputCard) {
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; found {
		return
	}
	// The catalog files a name a storefront spells another way: the token
	// word order ("Token: Sheep" for "Sheep Token"), the mark numbering a
	// repeated name, the letters it writes as words. A spelling only ever
	// answers for a name the datastore does not already know, above, so the
	// tokens a set does name the storefront's way keep their own printing.
	// It also only answers for an edition naming no set: respellName has
	// already asked a resolved edition for every spelling it carries, so
	// adopting one here anyway would hand the input another set's token and
	// the printing filter a name its own edition never prints. Two
	// spellings the catalog both knows decide nothing.
	if editionSet(b, inCard.Edition) == nil {
		var adopt string
		for _, alternate := range alternateNames(inCard.Name) {
			if _, found := b.CanonicalNames[matcher.Normalize(alternate)]; !found {
				continue
			}
			if adopt != "" && matcher.Normalize(adopt) != matcher.Normalize(alternate) {
				adopt = ""
				break
			}
			adopt = alternate
		}
		if adopt != "" {
			inCard.Name = adopt
			return
		}
		// A character's field center is filed with the monster it shares
		// its art with, "Field Center Token: Seto Kaiba & Blue-Eyes White
		// Dragon", which only the spelling's prefix reaches.
		if name, found := solePrefixName(b, alternateNames(inCard.Name)); found {
			inCard.Name = name
			return
		}
	}
	// A name nothing answers to, beside a number only one card carries, is
	// a card this reader can still name: the number is the stronger key of
	// the two, and a storefront naming a token its own way ("Orange Kuriboh
	// Token" for "Token: Kuriboh") has spelled the one thing it could get
	// wrong. Uniqueness across the whole datastore is the guard - a number
	// two cards share names neither of them.
	if name := soleNameAt(b, extractNumber(inCard.Variation)); sharesWord(inCard.Name, name) {
		inCard.Name = name
		return
	}

	uuids, err := b.SearchHasPrefix(inCard.Name)
	if err != nil {
		return
	}

	number := extractNumber(inCard.Variation)
	var match, matchNorm string
	for _, uuid := range uuids {
		co, err := b.GetUUID(uuid)
		if err != nil || co.Sealed {
			continue
		}
		if number != "" && !numberMatches(number, co.Number) {
			continue
		}
		norm := matcher.Normalize(co.Name)
		if match != "" && matchNorm != norm {
			return
		}
		match, matchNorm = co.Name, norm
	}
	if match != "" {
		inCard.Name = match
	}
}

// AdjustEdition trims the game-name prefix and "Singles" suffix storefronts
// decorate set names with, and rewrites the names editionAliases carries. A
// handful of real set names carry the game name themselves ("Yu-Gi-Oh!
// Championship Series 2025 Prize Cards"), so an edition that already names a
// set is left alone - as it is asked, before the decorations come off and
// again after, the alias table only ever answering for a name no set has.
//
// An edition still naming no set is answered by the collector number, which
// carries the set code the wording never spelled.
func (Rules) AdjustEdition(b *matcher.Backend, inCard *matcher.InputCard) {
	edition := strings.TrimSpace(inCard.Edition)
	if named, found := namedSet(b, edition); found {
		// The number first: where it names a sibling outright it has
		// said which edition and nothing is left to infer. Only a
		// number naming this very set leaves the rarity to answer.
		edition := siblingSetNamed(b, inCard, named)
		if edition == named {
			edition = siblingSetRarity(b, inCard, named)
		}
		if edition == named {
			edition = droppedIfNumberless(b, inCard, named)
		}
		inCard.Edition = edition
		spellNumber(b, inCard)
		return
	}
	edition = trimEditionDecorations(edition)
	if named, found := namedSet(b, edition); found {
		edition = droppedIfNumberless(b, inCard, named)
	} else if set := numberSet(b, inCard, edition); set != nil {
		edition = set.Name
	} else if isPromoHeading(edition) {
		// Cool Stuff Inc's catch-all "Promo" shelf answers for no set of its
		// own, so once the number has failed to place it in one, the
		// heading itself has nothing left to say - and left standing it can
		// still wrongly restrict CandidateSets to some unrelated set whose
		// own name happens to contain the word "Promo".
		number := extractNumber(inCard.Variation)
		if number != "" {
			edition = ""
		}
	}
	inCard.Edition = edition
	spellNumber(b, inCard)
}

// droppedIfNumberless answers the named set, unless the wording's own
// collector number is one that set does not carry - "Moja" sells plainly at
// RGBT-EN084, but Cool Stuff Inc also files its Duelist Pack Collection Tin
// reprint, numbered RGBT-ENPP4, under "Raging Battle" beside it, and RGBT
// never printed that number. Restricting the search to the named set then
// finds nothing; dropping the edition instead lets the number's own set
// answer. Both AdjustEdition branches reach a named set this way - directly,
// or after trimEditionDecorations - and a number contradicting it is the same
// tell either way.
func droppedIfNumberless(b *matcher.Backend, inCard *matcher.InputCard, named string) string {
	set := b.NormalizedSets[matcher.Normalize(named)]
	number := extractNumber(inCard.Variation)
	if number != "" && (set == nil || !editionHoldsNumber(set, inCard.Name, number)) {
		return ""
	}
	return named
}

// isPromoHeading reports whether an edition is Cool Stuff Inc's catch-all
// "Promo" shelf, the one heading seen in practice that answers for no set
// at all rather than merely spelling a real one incompletely - dropping an
// edition CardTrader spells short ("Legendary Collection" for "...1") would
// remove the one thing keeping an unrelated same-tail-digit printing from
// aliasing in beside it, so the check stays this narrow on purpose.
func isPromoHeading(edition string) bool {
	return matcher.Normalize(edition) == matcher.Normalize("Promo")
}

// AliasEdition spells an edition string toward a set name using the string
// alone: the edition-only projection of AdjustEdition, whose sibling and
// number inference needs a card to read. See matcher.GameRules.
func (Rules) AliasEdition(b *matcher.Backend, edition string) string {
	edition = strings.TrimSpace(edition)
	if named, found := namedSet(b, edition); found {
		return named
	}
	edition = trimEditionDecorations(edition)
	if named, found := namedSet(b, edition); found {
		return named
	}
	return edition
}

// bareTailRe matches a collector number written without its set code, the
// language infix and deck letter still on it: "ENF17".
var bareTailRe = regexp.MustCompile(`^[A-Za-z]{1,4}[0-9]{1,3}$`)

// spellNumber writes a bare tail out as the number its own set spells, so
// that a Speed Duel listing saying "ENF17" is read as SGX3-ENF17.
//
// The sets that need it file a card once per deck letter under one run, and
// nothing downstream reads a tail that carries neither set code nor dash:
// the listing arrives with no number at all, and every deck's printing
// survives the filter. The set is what supplies the missing half, and it
// only ever answers where the wording gave no number of its own and the
// set spells exactly one that fits - two of them decide nothing, as they
// decide nothing anywhere else.
func spellNumber(b *matcher.Backend, inCard *matcher.InputCard) {
	if extractNumber(inCard.Variation) != "" {
		return
	}
	set := editionSet(b, inCard.Edition)
	if set == nil {
		return
	}
	for _, field := range strings.Fields(inCard.Variation) {
		if !bareTailRe.MatchString(field) {
			continue
		}
		if full := editionNumberAt(set, inCard.Name, field); full != "" {
			inCard.Variation = strings.Replace(inCard.Variation, field, full, 1)
			return
		}
	}
}

// sharesWord reports whether two names have a whole word in common, which is
// what says a listing is talking about the card its number points at rather
// than about nothing at all.
//
// The number is the stronger key, but it is not the only one: on its own it
// would let any name whatsoever be replaced by whatever the number happens to
// hold, and a listing naming no card we know would come back as the card at
// that number instead of as the miss it is. A word in common is the cheapest
// thing that tells "Orange Kuriboh Token" for "Token: Kuriboh" apart from a
// name that means nothing here.
func sharesWord(listed, named string) bool {
	if named == "" {
		return false
	}
	words := map[string]bool{}
	for _, word := range strings.Fields(strings.ToLower(named)) {
		if word := matcher.Normalize(word); word != "" {
			words[word] = true
		}
	}
	for _, word := range strings.Fields(strings.ToLower(listed)) {
		if word := matcher.Normalize(word); word != "" && words[word] {
			return true
		}
	}
	return false
}

// soleNameAt names the one card a collector number belongs to, or nothing
// where the number is absent, unknown, or shared by cards of more than one
// name. Printings of one card under one number are not a share: they are the
// same name however many rarities carry it.
func soleNameAt(b *matcher.Backend, number string) string {
	if number == "" {
		return ""
	}
	var name string
	for _, code := range b.AllSets {
		for _, card := range b.Sets[code].Cards {
			if !strings.EqualFold(card.Number, number) {
				continue
			}
			if name != "" && matcher.Normalize(name) != matcher.Normalize(card.Name) {
				return ""
			}
			name = card.Name
		}
	}
	return name
}

// siblingSetNamed answers the edition a listing's collector number names,
// where that set is a sibling of the one the edition names, and the edition
// unchanged otherwise.
//
// A set family shares one name and splits into editions filed as sets of
// their own - the Movie Pack prints a Gold Edition and a Secret Edition - and
// a storefront sells all of them under the family's name, saying which is
// which in the number alone. Cool Stuff Inc lists Dark Magician at
// MVP1-EN054, MVP1-ENG54, MVP1-ENGV3, MVP1-ENS54 and MVP1-ENSV3: five
// products, one name, one shelf. The edition pins the first of them, so the
// other four answered with it and one printing carried five prices.
//
// Only a sibling is read this way. A number naming an unrelated set is a
// storefront filing a reprint under the set it was first printed in, which
// the edition is already answering correctly, and numberSet's own guards
// still apply: the number has to hold this card in that set and hold it
// nowhere else.
func siblingSetNamed(b *matcher.Backend, inCard *matcher.InputCard, named string) string {
	current := b.NormalizedSets[matcher.Normalize(named)]
	if current == nil {
		return named
	}
	set := numberSet(b, inCard, named)
	if set == nil || set.Code == current.Code || setFamily(set.Code) != setFamily(current.Code) {
		return named
	}
	return set.Name
}

// siblingSetRarity answers the edition holding this card at the rarity the
// wording asks for, where the edition named holds it at no such rarity and
// exactly one sibling does.
//
// siblingSetNamed above reads the number, and answers when the number names
// a sibling. This reads the rarity, for the listings whose number names the
// family's first edition and whose only other word is the tier. Cool Stuff
// Inc sells the Movie Pack's Dark Magician three times over - Gold at $2.50,
// Secret at $1.50, Ultra at $1.25 - every one of them numbered MVP1-EN054,
// which is the base edition's number and the base edition holds only the
// Ultra. All three answered with the Ultra, so one printing carried three
// prices, and the two dearer cards were sold at the cheap one's.
//
// Only an unambiguous answer is given. A rarity two siblings both hold is a
// question the listing has not answered - the wording says which tier, never
// which edition - so the edition stands and the pool stays whole, which is
// the refusal Match is built to make. The same holds for a rarity the family
// does not print at all: a wording naming a tier no sibling has is naming
// something else, and this rule has nothing to say about it.
func siblingSetRarity(b *matcher.Backend, inCard *matcher.InputCard, named string) string {
	current := b.NormalizedSets[matcher.Normalize(named)]
	if current == nil {
		return named
	}
	family := setFamily(current.Code)
	name := matcher.Normalize(inCard.Name)
	// The rarities this card is printed at across the family, and the
	// editions each of them is printed in.
	editions := map[string]map[string]bool{}
	for _, code := range b.AllSets {
		set := b.Sets[code]
		if set == nil || setFamily(set.Code) != family {
			continue
		}
		for _, card := range set.Cards {
			if matcher.Normalize(card.Name) != name {
				continue
			}
			rarity := strings.ToLower(b.RarityLabel(card.Rarity))
			if editions[rarity] == nil {
				editions[rarity] = map[string]bool{}
			}
			editions[rarity][set.Code] = true
		}
	}
	// The tier the wording spells out, read the way tierByRarity reads
	// it: every tier the words cover, then the ones another tier's words
	// contain dropped, so "Gold Secret Rare" is never taken for the
	// "Gold Rare" whose words it happens to hold. What survives has to be
	// the one tier, or the wording has not said which.
	words := strings.Fields(strings.ToLower(inCard.Variation))
	described := map[string]bool{}
	for rarity := range editions {
		if allWordsIn(words, rarity) {
			described[rarity] = true
		}
	}
	for a := range described {
		for other := range described {
			if a != other && wordSubset(a, other) {
				delete(described, a)
				break
			}
		}
	}
	if len(described) != 1 {
		return named
	}
	var asked string
	for rarity := range described {
		asked = rarity
	}
	holders := editions[asked]
	if holders[current.Code] || len(holders) != 1 {
		return named
	}
	for code := range holders {
		if set := b.Sets[code]; set != nil {
			return set.Name
		}
	}
	return named
}

// setFamily is the code a set's editions share, which is everything a dash
// leaves in front: MVP1-ENG and MVP1-ENS are both MVP1.
func setFamily(code string) string {
	base, _, _ := strings.Cut(code, "-")
	return base
}

// numberSet answers the set the input's collector number is filed in, for a
// bucket that names none.
//
// A bucket is an edition a storefront wrote and no set answers. An edition
// holding nothing is not one: a listing naming no set at all has claimed
// nothing to be wrong about, and the number alone must not stand in for the
// claim - a full code names one printing per set and several sets reprint it
// under exactly that code, so reading it as a set would hand back one of them
// where the whole pool is the honest answer.
//
// A Yu-Gi-Oh number opens with its set's code - "UBP1-EN005" is UBP1 - so a
// storefront filing a whole shelf of printings under one catch-all bucket
// still says, number by number, which set each of them lives in. Without
// that, Match has no set to search and falls through to every printing of the
// name, where the rarity and variant tiering pick whichever one the wording
// happens not to contradict: Cool Stuff Inc files three Exodias under
// "Promo", and the $170 Secret Rare and the $9 Quarter Century Rare were both
// answered with the $22 Ultra Rare.
//
// The code is read dashed or run together, because the feeds that need this
// most are the ones that drop the dash. The set is only adopted when it
// carries the card at that number: the number is the storefront's own claim,
// and one naming a set the card was never printed in says nothing about where
// the listing belongs. Nor when another set prints the card under that very
// number - the print-run twins are reissued under their original's numbers,
// and only the copyright date on the card tells those apart, which is the
// scraper's to read and not a code's to guess.
func numberSet(b *matcher.Backend, inCard *matcher.InputCard, edition string) *matcher.Set {
	// Punctuation is what the normalizing drops first, so a bucket spelled
	// with nothing but it says as little as an absent one.
	if matcher.Normalize(edition) == "" {
		return nil
	}
	// A letter-tailed number that only one set prints this name under is
	// read verbatim before the prefix inference below gets a say: Konami's
	// own misprint reissues carry the base card's prefix plus a tail letter
	// ("EOJ-EN004K" beside "EOJ-EN004"), and reading the prefix alone sends
	// the reissue to the base set's edition, which then drops the reissue's
	// own row before FilterCards' verbatim-number rule can keep it.
	for _, field := range strings.Fields(inCard.Variation) {
		set := verbatimSet(b, inCard.Name, field)
		if set != nil {
			return set
		}
	}
	for _, field := range strings.Fields(inCard.Variation) {
		set := fieldSet(b, field)
		if set == nil {
			continue
		}
		full := editionNumberAt(set, inCard.Name, field)
		if full == "" || !onlySetAt(b, inCard.Name, full, set.Code) {
			continue
		}
		return set
	}
	return nil
}

// editionNumberAt answers the set's own spelling of the collector number the
// input fits, and nothing when the set prints the card at several numbers
// that fit - which decides no more than none at all.
func editionNumberAt(set *matcher.Set, name, number string) string {
	normalized := matcher.Normalize(name)
	var full string
	for i := range set.Cards {
		card := &set.Cards[i]
		if matcher.Normalize(card.Name) != normalized ||
			!numberMatches(number, card.Number) {
			continue
		}
		if full != "" && !strings.EqualFold(full, card.Number) {
			return ""
		}
		full = card.Number
	}
	return full
}

// onlySetAt reports whether the set is the only one printing the card under
// this collector number. The comparison is against the number as its own set
// spells it, never the storefront's: an input that dropped the dash compares
// on its digits alone, and every set numbering a reprint -005 would answer it.
func onlySetAt(b *matcher.Backend, name, full, code string) bool {
	for _, uuid := range b.Hashes[matcher.Normalize(name)] {
		co, found := b.UUIDs[uuid]
		if !found || co.Sealed || co.SetCode == code {
			continue
		}
		if strings.EqualFold(co.Number, full) {
			return false
		}
	}
	return true
}

// foreignInfixes are the language segments a collector number carries in
// place of the English "EN". Konami numbers a printing by the language it was
// printed in, so the segment is the one thing that says a listing is not the
// card this datastore holds - the datastore is the English catalog, and a
// German Nitro Warrior is a different piece of card from the English one, not
// a spelling of it.
//
// The list is the languages Konami prints, and a segment outside it is left
// alone rather than guessed at: an unknown infix is far likelier to be a set
// code this reader has not seen than a language Konami has started using.
var foreignInfixes = map[string]bool{
	"DE": true, // German
	"FR": true, // French
	"IT": true, // Italian
	"JP": true, // Japanese
	"KR": true, // Korean
	"PT": true, // Portuguese
	"SP": true, // Spanish
}

// foreignNumberRe splits a collector number into the set code, the language
// segment and the digits behind it: "AC14-DE021" is AC14, DE and 021.
var foreignNumberRe = regexp.MustCompile(`^([A-Za-z0-9]+)-([A-Za-z]{2})[0-9]`)

// IsUnsupported reports that a listing names a printing this datastore has no
// row for rather than one it failed to find. Saying so lets the caller skip it
// in silence, where a refusal would be reported as a miss every run.
//
// A foreign collector number is the sign of one, because the catalog sells
// far more of the language printings than it files: the number says which
// language and the datastore has only the English row. It is a sign and not
// a rule, so it yields to the datastore whenever the number is one this
// datastore actually carries.
func (Rules) IsUnsupported(b *matcher.Backend, inCard *matcher.InputCard, co *matcher.CardObject, stage matcher.Stage) bool {
	if stage != matcher.StageWording {
		return false
	}
	// A shelf naming no set of its own, see unsupportedEditions.
	if normalizedUnsupportedEditions()[matcher.Normalize(inCard.Edition)] {
		return true
	}
	// The character art cards are the storefront's own product rather than
	// a printing: they carry no collector number and the catalog has no row
	// for them, so they are skipped rather than reported missing every run.
	if strings.HasSuffix(inCard.Name, "Character Art Card") {
		return true
	}
	if pastOTSRun(b, inCard) {
		return true
	}
	for _, field := range strings.Fields(inCard.Variation) {
		match := foreignNumberRe.FindStringSubmatch(field)
		if match == nil || !foreignInfixes[strings.ToUpper(match[2])] {
			continue
		}
		// A printing this datastore carries is not one it has no row for,
		// whatever language it was printed in. The advent calendars are
		// German and carried as German, all 48 of them; "Magi Magi
		// Magician Gal" WJMP-JP018 is a Japanese promo the catalog sells;
		// and "Sinister Serpent" TFK-SP001 is not Spanish at all, the SP
		// being The Falsebound Kingdom's own numbering rather than a
		// language. Asking whether the number is carried answers all three
		// without a list of exceptions, and goes on refusing the language
		// printings of a number this datastore has only in English.
		if set, found := b.Sets[strings.ToUpper(match[1])]; found && setSpellsNumber(set, field) {
			continue
		}
		return true
	}
	return false
}

// trailingDigitsRe reads the digits a collector number ends on.
var trailingDigitsRe = regexp.MustCompile(`[0-9]+$`)

// pastOTSRun reports whether an OTS Tournament Pack listing is numbered past
// the last card its set prints. The packs' extra cards past the English run
// exist only as Portuguese printings, which no English datastore carries, so
// a plain leading number beyond the set's own run names a printing it has no
// row for. A number inside the run is left to the match.
func pastOTSRun(b *matcher.Backend, inCard *matcher.InputCard) bool {
	if !strings.HasPrefix(inCard.Edition, "OTS Tournament Pack") {
		return false
	}
	set, err := b.GetSetByName(inCard.Edition)
	if err != nil {
		return false
	}
	fields := strings.Fields(inCard.Variation)
	if len(fields) == 0 {
		return false
	}
	listed, err := strconv.Atoi(fields[0])
	if err != nil {
		return false
	}
	var last int
	for i := range set.Cards {
		digits := trailingDigitsRe.FindString(set.Cards[i].Number)
		n, err := strconv.Atoi(digits)
		if err == nil {
			last = max(last, n)
		}
	}
	return last > 0 && listed > last
}

// numberTailRe matches what a collector number holds behind its set code: the
// language infix, the digits, and the letter tail a misprint reissue or a
// storefront's rarity suffix adds.
var numberTailRe = regexp.MustCompile(`^[A-Za-z]*[0-9]+[a-zA-Z]*$`)

// fieldSet reads one field of the variation as a collector number and answers
// the set its code names, or nothing when the field is not one. An undashed
// code is taken as long as it can be, so "UBP1EN005" is UBP1 rather than a
// shorter code that happens to open it. Longest wins is what keeps the answer
// off the map's iteration order too: two codes of one length cannot both open
// one field, so the longest fitting code is unique.
func fieldSet(b *matcher.Backend, field string) *matcher.Set {
	// The longest code a number opens with is the set it names, whether or
	// not the code carries a dash of its own. A family's editions are sets
	// whose codes extend the family's - MVP1-ENG beside MVP1 - so cutting
	// at the first dash reads "MVP1-ENG54" as the family and hands back a
	// printing the number was not naming.
	var best *matcher.Set
	var bestCode string
	for code, set := range b.Sets {
		if code == "" || len(code) >= len(field) ||
			!strings.EqualFold(field[:len(code)], code) ||
			!numberTailRe.MatchString(field[len(code):]) {
			continue
		}
		if best == nil || len(code) > len(bestCode) {
			best, bestCode = set, code
		}
	}
	if best != nil {
		return best
	}
	if prefix, tail, dashed := strings.Cut(field, "-"); dashed {
		if !numberTailRe.MatchString(tail) {
			return nil
		}
		return setByCode(b, prefix)
	}
	return nil
}

// setByCode answers the set filed under a code, however a storefront cased it.
func setByCode(b *matcher.Backend, code string) *matcher.Set {
	if set, found := b.Sets[code]; found {
		return set
	}
	for stored, set := range b.Sets {
		if strings.EqualFold(stored, code) {
			return set
		}
	}
	return nil
}

// trimEditionDecorations strips the game-name prefix and "Singles" suffix
// off a storefront's edition wording.
func trimEditionDecorations(edition string) string {
	for _, prefix := range []string{"Yu-Gi-Oh!", "Yu-Gi-Oh", "YuGiOh"} {
		if strings.HasPrefix(edition, prefix) {
			edition = strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(edition, prefix), ":-"))
			break
		}
	}
	return strings.TrimSpace(strings.TrimSuffix(edition, "Singles"))
}

// namedSet answers the set an edition names: itself when the datastore
// carries it under that name, else whatever the alias table maps it onto.
func namedSet(b *matcher.Backend, edition string) (string, bool) {
	normalized := matcher.Normalize(edition)
	if _, found := b.NormalizedSets[normalized]; found {
		return edition, true
	}
	if set, found := normalizedEditionAliases()[normalized]; found {
		return set, true
	}
	return "", false
}

// PlainNumber implements matcher.GameRules. A number carries its set code, then the language its print run was published in, then a padded ordinal, where a person writes the ordinal alone: LOB-EN001 is card 1.
func (Rules) PlainNumber(number string) string {
	return matcher.PlainOrdinal(number)
}

// FilterCards narrows candidates by edition, collector number, rarity and
// variant, in that order. Rarity only ever narrows on an explicit signal -
// the input wording spelling out a rarity, or the suffix map when only the
// number's tail speaks; a bare input facing several rarities keeps them all
// and surfaces as an aliasing error rather than a guess. The variant label
// tiering mirrors One Piece: a described label wins, a demanded-but-unnamed
// variant (the "a" tail) drops the base art, and a plain input keeps it.
func (r Rules) FilterCards(b *matcher.Backend, inCard *matcher.InputCard, sets []matcher.CandidateSet) []matcher.Card {
	// A pooled storefront name restricts the candidates to its pair of sets
	// outright: the edition keeps the name, so nothing upstream can narrow,
	// and every printing the card ever had would answer instead.
	if pool, found := normalizedPooledEditions()[matcher.Normalize(inCard.Edition)]; found {
		inPool := map[string]bool{}
		for _, name := range pool {
			set, ok := b.NormalizedSets[matcher.Normalize(name)]
			if !ok {
				continue
			}
			inPool[set.Code] = true
		}
		var pooled []matcher.CandidateSet
		for _, set := range sets {
			if inPool[set.Code] {
				pooled = append(pooled, set)
			}
		}
		// Narrow to the pair only where the pair holds the card. A name
		// neither set carries - a misfiled row, a product the datastore has
		// yet to learn - would otherwise be narrowed to nothing at all, and
		// answer with no candidates rather than with its real printing.
		if len(pooled) > 0 {
			sets = pooled
		}
	}

	number := extractNumber(inCard.Variation)

	// A storefront writing the deck or volume index alone where the catalog
	// writes the whole set code - "5-001" for DL5-EN001, "05-001" for
	// YR05-EN001 - is saying the same number in fewer letters, and the strict
	// prefix compare throws it away. Reading the index as the code's own
	// digits recovers it, but the digits are a weak key on their own: "1-E002"
	// reads MC1-EN002 as readily as anything in DL. So the relaxation is
	// spent only inside the set the listing names, and a listing whose
	// edition names no set does not get it at all.
	var looseSet *matcher.Set
	if loosePrefixNumber(number) {
		looseSet = editionSet(b, inCard.Edition)
	}

	var candidates []matcher.Card
	named := map[string]bool{}
	var cards []matcher.Card
	for _, set := range sets {
		cards = append(cards, set.Cards...)
	}
	for _, card := range cards {
		if number != "" && !numberMatches(number, card.Number) &&
			!(looseSet != nil && strings.EqualFold(card.SetCode, looseSet.Code) &&
				loosePrefixMatches(number, card.Number)) {
			continue
		}
		// An input naming a print run re-keys the copy's FoilUUIDs so the
		// flag-driven resolution downstream lands on that run's entry. Both
		// slots move together: a run spans both foilnesses, so the vendor's
		// foil flag must not pull the resolution off it.
		uuid := finishUUID(b, inCard, &card)
		if uuid != "" {
			foilUUIDs := make(map[string]string, len(card.FoilUUIDs))
			maps.Copy(foilUUIDs, card.FoilUUIDs)
			foilUUIDs[matcher.FinishNonfoil] = uuid
			foilUUIDs[matcher.FinishFoil] = uuid
			card.FoilUUIDs = foilUUIDs
			named[card.UUID] = true
		}
		candidates = append(candidates, card)
	}
	if len(candidates) <= 1 {
		return candidates
	}

	// A verbatim collector number beats a suffix-stripped match, so the
	// "K" misprint reissues ("EOJ-EN004K") never pull in their base card.
	if number != "" {
		var exact []matcher.Card
		for _, card := range candidates {
			if strings.EqualFold(number, card.Number) {
				exact = append(exact, card)
			}
		}
		if len(exact) > 0 && len(exact) < len(candidates) {
			candidates = exact
		}
		if len(candidates) <= 1 {
			return candidates
		}
	}

	candidates = tierByRarity(b, inCard, candidates, number, r.qualifiers)
	if len(candidates) <= 1 {
		return candidates
	}
	candidates = tierByVariant(inCard, candidates, number, r.qualifiers)

	// Copies left tied are told apart by the run: one never printed in the
	// named run cannot be the listing while another copy holds it.
	var holders []matcher.Card
	for _, card := range candidates {
		if named[card.UUID] {
			holders = append(holders, card)
		}
	}
	if len(holders) > 0 {
		return holders
	}
	return candidates
}

// tierByRarity keeps the candidates whose rarity the input's wording spells
// out, most specific description first ("Quarter Century Secret Rare" is not
// read as "Secret Rare"); with no described rarity, the collector number's
// suffix narrows through the suffix map. No signal keeps every candidate.
// Only the variation speaks: set names carry rarity words themselves
// ("McDonald's Promo").
func tierByRarity(b *matcher.Backend, inCard *matcher.InputCard, candidates []matcher.Card, number string, qualifiers map[string]string) []matcher.Card {
	words := strings.Fields(strings.ToLower(inCard.Variation))

	described := map[string]bool{}
	for _, card := range candidates {
		rarity := strings.ToLower(b.RarityLabel(card.Rarity))
		if !described[rarity] && allWordsIn(words, rarity) {
			described[rarity] = true
		}
	}
	for a := range described {
		for other := range described {
			if a != other && wordSubset(a, other) {
				delete(described, a)
				break
			}
		}
	}
	if len(described) > 0 {
		var out []matcher.Card
		for _, card := range candidates {
			if described[strings.ToLower(b.RarityLabel(card.Rarity))] {
				out = append(out, card)
			}
		}
		return out
	}

	// A wording that spells a sibling's own qualifier whole has named that
	// product, and no word of the qualifier is read as a piece of a rarity
	// (26LP-EN001's "Monster Reborn (Emblazoned)" beside "...(Emblazoned
	// Secret Rare)"). The rest of the wording may still name a rarity
	// outright, though: the Rarity Collections sell every tier of a number
	// decorated ("Alt Art"), so the qualifier alone does not say which of
	// the tiers sharing it is meant. Only the qualifier's own words are
	// dropped before asking; nothing left over falls back to the number's
	// suffix, same as an unqualified wording without one.
	matched := matchedQualifiers(words, candidates, qualifiers)
	if len(matched) > 0 {
		rest := withoutQualifierWords(words, matched)
		out, found := rarityFilter(b, rest, candidates)
		if found {
			return out
		}
		return suffixNarrowed(candidates, number)
	}

	out, found := rarityFilter(b, words, candidates)
	if found {
		return out
	}

	return suffixNarrowed(candidates, number)
}

// rarityFilter narrows the candidates to the one rarity decoratedRarity or
// narrowedRarity names in the wording, or reports false where neither does.
func rarityFilter(b *matcher.Backend, words []string, candidates []matcher.Card) ([]matcher.Card, bool) {
	rarity, found := decoratedRarity(b, words, candidates)
	if !found {
		rarity, found = narrowedRarity(b, words, candidates)
	}
	if !found {
		return nil, false
	}
	var out []matcher.Card
	for _, card := range candidates {
		if strings.EqualFold(card.Rarity, rarity) {
			out = append(out, card)
		}
	}
	return out, true
}

// suffixNarrowed keeps the candidates at the rarity the collector number's
// suffix encodes, or every candidate where the number carries none.
func suffixNarrowed(candidates []matcher.Card, number string) []matcher.Card {
	rarity := matcher.RarityName(suffixRarity(number))
	if rarity == "" {
		return candidates
	}
	var out []matcher.Card
	for _, card := range candidates {
		if card.Rarity == rarity {
			out = append(out, card)
		}
	}
	return out
}

// narrowedRarity names the one tier among the candidates that says everything
// the wording said about rarity, where the wording did not say all of it.
//
// A set decorates its tiers with a word of its own - King's Court sells one
// number as "Secret Pharaoh's Rare" and "Ultra Pharaoh's Rare" - and a
// storefront writes the tier it knows, "Ultra Rare". Asking whether the
// wording spells the whole tier answers no for both, and the decoration is in
// the middle rather than at the front, so reading a tail off it (the rule
// above) does not reach it either. What the wording did name is the word that
// tells the two apart.
//
// So the question is turned around: of the words the candidates use for
// rarity, which did the wording say, and does exactly one tier say all of
// them. Only the candidates' own vocabulary counts, so the set code and the
// rest of the wording say nothing here; and one tier has to answer alone,
// since a wording naming what several tiers share has not chosen between them.
func narrowedRarity(b *matcher.Backend, words []string, candidates []matcher.Card) (string, bool) {
	vocabulary := map[string]bool{}
	for _, card := range candidates {
		for _, word := range strings.Fields(strings.ToLower(b.RarityLabel(card.Rarity))) {
			vocabulary[word] = true
		}
	}
	named := map[string]bool{}
	for _, word := range words {
		if vocabulary[word] {
			named[word] = true
		}
	}
	if len(named) == 0 {
		return "", false
	}
	var narrowed string
	for _, card := range candidates {
		has := map[string]bool{}
		for _, word := range strings.Fields(strings.ToLower(b.RarityLabel(card.Rarity))) {
			has[word] = true
		}
		saysAll := true
		for word := range named {
			if !has[word] {
				saysAll = false
				break
			}
		}
		if !saysAll {
			continue
		}
		if narrowed != "" && !strings.EqualFold(narrowed, card.Rarity) {
			return "", false
		}
		narrowed = card.Rarity
	}
	return narrowed, narrowed != ""
}

// decoratedRarity answers the candidate rarity whose tail the wording spells
// out, for a tier a set prints only decorated: the Rarity Collections sell a
// "Prismatic Collector's Rare" and no plain one, so a listing saying
// "Collector's Rare" has still said which of the printings at its number it
// means.
//
// Only a proper suffix of two words or more is read. Every tier this game has
// ends in "Rare" and almost every wording says it, so a one-word tail would
// name them all. A tail two candidates share - a number sold as both a
// Platinum and a Prismatic Secret Rare, with no plain one - says nothing
// about which was meant, and answers nothing rather than guess.
//
// This is reached only where no rarity was named in full, which is what keeps
// it off the sets printing a tier both ways: 25LP sells an Ultra Rare beside
// its Emblazoned Ultra Rare, and a plain wording names the plain one outright
// before this is asked.
func decoratedRarity(b *matcher.Backend, words []string, candidates []matcher.Card) (string, bool) {
	longest := 0
	var named string
	shared := false
	for _, card := range candidates {
		labelWords := strings.Fields(strings.ToLower(b.RarityLabel(card.Rarity)))
		for n := len(labelWords) - 1; n >= 2; n-- {
			if !allWordsIn(words, strings.Join(labelWords[len(labelWords)-n:], " ")) {
				continue
			}
			switch {
			case n > longest:
				longest, named, shared = n, card.Rarity, false
			case n == longest && !strings.EqualFold(card.Rarity, named):
				shared = true
			}
			break
		}
	}
	if longest == 0 || shared {
		return "", false
	}
	return named, true
}

// tierByVariant splits the candidates into the ones whose variant label the
// input's variation describes, the base printings, and the variant
// printings. Only the variation is consulted: set names carry the color
// words the labels use ("Blue" against "Legend of Blue Eyes White Dragon").
// numberSegmentRe matches the segment a collector number opens on, the part
// naming the set: "DL18" of "DL18-EN002".
var numberSegmentRe = regexp.MustCompile(`^([A-Za-z]+[0-9]*)-`)

// numberedWording appends the collector number's set segment to the wording,
// so a promo tag spelled with the set it belongs to is reachable by the
// colour or label a storefront writes on its own.
//
// The segment is added rather than substituted: everything the wording
// already said still reads, and a tag that never names a set is unaffected
// by a word appended after it.
func numberedWording(variation, number string) string {
	match := numberSegmentRe.FindStringSubmatch(number)
	if match == nil {
		return variation
	}
	return variation + " " + match[1]
}

// tierByMark keeps the candidates whose mark the wording names - the ink a
// printing was made in, the version it is, or the letter its artwork is
// filed under. A mark says which printing of a number this is rather than
// what promoted it, so it is not among the promo types and narrows on its
// own and before them: Duelist League alone is 325 groups of printings at
// one name, one number and one rarity, told apart by the ink and nothing
// else, and a wording naming one has named the printing.
//
// Nothing is kept where the wording names no mark. Narrowing on a mark the
// listing never mentioned would answer with a printing picked at random,
// and the tiers below still have their say - a wording of "Blue" reaches
// the plain blue printing rather than the alternate-art one wearing the
// same ink, because that one is a variant and the plain one is not.
func tierByMark(wording string, candidates []matcher.Card) []matcher.Card {
	var marked []matcher.Card
	for _, card := range candidates {
		if card.Watermark != "" && matcher.SlugDescribes(wording, matcher.PromoTypeSlug(card.Watermark)) {
			marked = append(marked, card)
		}
	}
	return marked
}

func tierByVariant(inCard *matcher.InputCard, candidates []matcher.Card, number string, qualifiers map[string]string) []matcher.Card {
	if marked := tierByMark(inCard.Variation, candidates); len(marked) > 0 {
		candidates = marked
	}
	// A wording spelling a printing's qualifier whole has named it: "Dark
	// Magician (Arkana)" is the Arkana artwork among five arts of
	// RA04-EN106, and "Blue" is the blue ink of DLCS-EN006 rather than the
	// alternate art in the same ink.
	if named := qualifierNamed(strings.Fields(strings.ToLower(inCard.Variation)), candidates, qualifiers); len(named) > 0 {
		return named
	}
	// A printing is plain when it carries no label and the catalog sells it
	// under the bare name. One sold as "Mayhem Fur Hire (Starlight Rare)"
	// beside a plain "Mayhem Fur Hire" is the decorated one even once its
	// qualifier has become its rarity rather than a label: a listing that
	// says nothing means the plain product, the way it does for labels.
	var base, variants []matcher.Card
	for _, card := range candidates {
		if len(card.PromoTypes) == 0 && qualifierOf(qualifiers, card.UUID) == "" {
			base = append(base, card)
			continue
		}
		variants = append(variants, card)
	}
	// The tags are tokens, so the wording's words are joined back up a
	// run at a time to ask whether they name them. The collector number's
	// own set segment goes on the end of the wording first: a tag naming
	// the set it belongs to is spelled by a storefront without it, because
	// the number beside it already said which set. Handing the segment back
	// lets the run close, and it can only ever close a tag that names the
	// set the number named.
	described := matcher.DescribedVariants(numberedWording(inCard.Variation, number), variants)
	if len(described) > 0 {
		return described
	}
	if wantsVariant(number) {
		if len(variants) > 0 {
			return variants
		}
		return candidates
	}
	if len(base) > 0 {
		return base
	}
	return candidates
}

// matchedQualifiers names the printings' own qualifiers the wording spells
// whole, most specific first: "Alternate Art Blue" spells "Blue" as well,
// and a wording saying all three words means only the longer qualifier.
func matchedQualifiers(words []string, candidates []matcher.Card, qualifiers map[string]string) map[string]bool {
	named := map[string]bool{}
	for _, card := range candidates {
		if qualifier := strings.ToLower(qualifierOf(qualifiers, card.UUID)); qualifier != "" && allWordsIn(words, qualifier) {
			named[qualifier] = true
		}
	}
	for a := range named {
		for other := range named {
			if a != other && wordSubset(a, other) {
				delete(named, a)
				break
			}
		}
	}
	return named
}

// withoutQualifierWords drops a matched qualifier's own words out of a
// wording, so what is left can still be asked whether it names a rarity.
func withoutQualifierWords(words []string, matched map[string]bool) []string {
	drop := map[string]bool{}
	for qualifier := range matched {
		for _, word := range strings.Fields(qualifier) {
			drop[word] = true
		}
	}
	var rest []string
	for _, word := range words {
		if !drop[word] {
			rest = append(rest, word)
		}
	}
	return rest
}

// qualifierNamed keeps the candidates whose qualifier the wording spells
// whole, and of those the ones saying the most. Nothing is kept where the
// wording spells none, so the tiers around it still have their say.
func qualifierNamed(words []string, candidates []matcher.Card, qualifiers map[string]string) []matcher.Card {
	named := matchedQualifiers(words, candidates, qualifiers)
	var out []matcher.Card
	for _, card := range candidates {
		if named[strings.ToLower(qualifierOf(qualifiers, card.UUID))] {
			out = append(out, card)
		}
	}
	return out
}

// allWordsIn reports whether the wording's words include every word of the
// label. Words compare whole: the one-letter artwork labels ("A") would
// otherwise hide inside almost any wording. They compare without their
// punctuation too, and without the possessive itself, because the
// storefronts spell the game's premium tier all three ways it can be
// written ("Collector's Rare", "Collectors Rare", "Collector Rare"), and a
// wording that names the rarity has to beat the number's suffix rather than
// fall through it onto every printing of the number.
//
// Only the word heading the label may drop its possessive. There it is the
// tier's own identity; further in it merely qualifies a base tier that is a
// complete rival label of its own ("Ultra Pharaoh's Rare" beside "Ultra
// Rare"), and relaxing it there lets any stray word promote the plain tier
// to the dearer one - the game names a set "Pharaoh Tour Promos".
func allWordsIn(words []string, label string) bool {
	labelWords := strings.Fields(strings.ToLower(label))
	if len(labelWords) == 0 {
		return false
	}
	for i, word := range labelWords {
		if !slices.ContainsFunc(words, func(spoken string) bool {
			return unpunctuated(spoken) == unpunctuated(word) ||
				(i == 0 && unpossessed(spoken) == unpossessed(word))
		}) {
			return false
		}
	}
	return true
}

// unpossessed drops the possessive a word may be written with, so a wording
// that leaves it off names what the catalog spells with one. Only a word
// actually written possessively loses its s; a plain plural keeps it.
// Either apostrophe counts, straight or curly, because the catalog writes
// both.
func unpossessed(word string) string {
	stem, found := strings.CutSuffix(word, "s")
	if !found {
		return unpunctuated(word)
	}
	if !strings.HasSuffix(stem, "'") && !strings.HasSuffix(stem, "\u2019") {
		return unpunctuated(word)
	}
	return unpunctuated(stem)
}

// unpunctuated drops the marks a word is written with or without.
func unpunctuated(word string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, word)
}

// wordSubset reports whether a's words are a strict subset of b's.
func wordSubset(a, b string) bool {
	aWords := strings.Fields(a)
	bWords := strings.Fields(b)
	for _, word := range aWords {
		if !slices.Contains(bWords, word) {
			return false
		}
	}
	return len(bWords) > len(aWords)
}

// finishUUID resolves the entry the input names to its uuid. The finish
// field speaks first and through the shared resolution, so a caller pricing
// one sku names the run rather than spelling it into the wording; the
// wording is the fallback for the storefronts that only ever had one field.
func finishUUID(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) string {
	if uuid := b.FinishUUID(card, inCard.Finish); uuid != "" {
		return uuid
	}
	return card.FoilUUIDs[selectFinish(inCard, card)]
}

// selectFinish maps the input wording's print-run tokens onto one of the
// card's stored runs, so a listing spelling its run out ("LOB-001 1st
// Edition") resolves to that run's entry instead of the default one. Only
// the variation speaks: set names carry the same words ("Limited Pack World
// Championship 2025"). Words compare whole, so "Unlimited" is never read as
// "Limited". A wording naming no run, or a run the product was not priced
// in, keeps the flag-driven default.
func selectFinish(inCard *matcher.InputCard, card *matcher.Card) string {
	var run string
	for word := range strings.FieldsSeq(strings.ToLower(inCard.Variation)) {
		switch word {
		case "1st", "first":
			run = matcher.Run1stEdition
		case "unlimited":
			run = matcher.RunUnlimited
		case "limited":
			run = matcher.RunLimited
		default:
			continue
		}
		break
	}
	if run == "" {
		return ""
	}
	return matcher.NamedFinish(card.FoilUUIDs, run, "")
}

// extractNumber pulls the collector number out of the scraper-supplied
// Variation: the full "RA01-EN019" shape when present (rarity tail and
// all), else a cardtrader-style bare tail ("019qsec"). A bare digit run is
// only believed when it fits the game's three-digit numbering and any tail
// is a known rarity suffix, so the years variant labels start with ("2012
// Pre-registration") are never read as numbers.
//
// A letter-led tail is read last of all. The deck reprints number their
// cards by the deck the card came in - Legendary Decks II files
// Polymerization at LDK2-ENJ26 beside LDK2-ENK22 - and cardtrader publishes
// that tail bare, as "J26". Read after both other shapes, so a full number
// and a plain digit run keep their precedence, and only where nothing else
// answered: outside a number a letter-led field is a label word, and
// letteredNumber's own shape - one letter, then digits, four bytes at most -
// is what tells the two apart.
func extractNumber(variation string) string {
	fields := strings.Fields(variation)
	for _, field := range fields {
		if fullNumberRe.MatchString(field) {
			return field
		}
	}
	for _, field := range fields {
		if field[0] < '0' || field[0] > '9' {
			continue
		}
		digits := strings.TrimRight(field, letters)
		if len(digits) > 3 || strings.ContainsFunc(digits, func(r rune) bool {
			return r < '0' || r > '9'
		}) {
			continue
		}
		suffix := strings.ToLower(field[len(digits):])
		if suffix == "" || suffix == "a" || suffixRarity(field) != "" {
			return field
		}
	}
	return letteredNumber(variation)
}

// numberSuffix returns the lowercase letter tail of a collector number.
func numberSuffix(number string) string {
	return strings.ToLower(number[len(strings.TrimRight(number, letters)):])
}

// suffixRarity resolves a collector number's letter tail through the
// cardtrader suffix map, tolerating the further "a" alternate-art tail.
func suffixRarity(number string) string {
	suffix := numberSuffix(number)
	if rarity, found := suffixRarities[suffix]; found {
		return rarity
	}
	if strings.HasSuffix(suffix, "a") {
		if rarity, found := suffixRarities[strings.TrimSuffix(suffix, "a")]; found {
			return rarity
		}
	}
	return ""
}

// wantsVariant reports whether the collector number's tail demands some
// variant printing without describing which: the "a" cardtrader appends to
// its alternate arts, alone or after a rarity suffix.
func wantsVariant(number string) bool {
	suffix := numberSuffix(number)
	if !strings.HasSuffix(suffix, "a") {
		return false
	}
	if suffix == "a" {
		return true
	}
	_, found := suffixRarities[strings.TrimSuffix(suffix, "a")]
	return found
}

// numberMatches compares an input number against a printing's full
// collector number: equal full codes (rarity tail stripped or not), or a
// matching numeric tail - with equal set prefixes when the input carries
// one, and whatever the language infix ("LOB-EN001" matches "LOB-001").
// Leading zeros never decide ("19" matches "RA01-EN019").
func numberMatches(input, full string) bool {
	if strings.EqualFold(input, full) {
		return true
	}
	input = strings.TrimRight(input, letters)
	if strings.EqualFold(input, full) {
		return true
	}
	inPrefix, inTail, inDashed := strings.Cut(input, "-")
	if !inDashed {
		inTail = input
	}
	_, fullTail, _ := strings.Cut(full, "-")
	if inDashed {
		fullPrefix, _, _ := strings.Cut(full, "-")
		if !strings.EqualFold(inPrefix, fullPrefix) {
			return false
		}
	}
	inDigits := digitRun(inTail)
	fullDigits := digitRun(fullTail)
	return inDigits != "" && fullDigits != "" &&
		matcher.CanonicalTail(inDigits) == matcher.CanonicalTail(fullDigits)
}

// loosePrefixNumber reports whether an input number is dashed with a prefix
// made of digits alone, the shape a storefront writes a deck or volume index
// in where the catalog writes the whole set code ("5-001" for DL5-EN001).
func loosePrefixNumber(number string) bool {
	prefix, _, dashed := strings.Cut(number, "-")
	return dashed && prefix != "" && digitRun(prefix) == prefix
}

// loosePrefixMatches compares such a number against a printing's own, reading
// the input's prefix as the digits of the printing's set code and the tails as
// numberMatches reads them. The set code's letters are what it stops asking
// for, which is why the caller only asks inside one set.
func loosePrefixMatches(input, full string) bool {
	inPrefix, inTail, _ := strings.Cut(input, "-")
	fullPrefix, fullTail, dashed := strings.Cut(full, "-")
	if !dashed {
		return false
	}
	fullCode := digitRun(fullPrefix)
	if fullCode == "" || matcher.CanonicalTail(inPrefix) != matcher.CanonicalTail(fullCode) {
		return false
	}
	inDigits := digitRun(inTail)
	fullDigits := digitRun(fullTail)
	return inDigits != "" && fullDigits != "" &&
		matcher.CanonicalTail(inDigits) == matcher.CanonicalTail(fullDigits)
}

// digitRun extracts the trailing digit run of a collector number tail, the
// letter tail aside: "EN004K" yields "004".
func digitRun(tail string) string {
	tail = strings.TrimRight(tail, letters)
	idx := strings.LastIndexFunc(tail, func(r rune) bool {
		return r < '0' || r > '9'
	})
	return tail[idx+1:]
}

// letterTailRe matches a full collector number carrying a letter after its
// digits, the shape Konami's misprint reissues and cardtrader's rarity
// suffixes both wear: "EOJ-EN004K", "RA01-EN019qsec".
var letterTailRe = regexp.MustCompile(`[0-9][A-Za-z]+$`)

// verbatimSet is the one set printing name under exactly this collector
// number, read before numberSet's own prefix inference gets a say - and
// only for a letter-tailed number, since without that limit a number two
// sets share names neither of them (see numberSet).
func verbatimSet(b *matcher.Backend, name, number string) *matcher.Set {
	if !fullNumberRe.MatchString(number) || !letterTailRe.MatchString(number) {
		return nil
	}
	code := ""
	for _, uuid := range b.Hashes[matcher.Normalize(name)] {
		co, found := b.UUIDs[uuid]
		if !found || co.Sealed || !strings.EqualFold(co.Number, number) {
			continue
		}
		if code != "" && code != co.SetCode {
			return nil
		}
		code = co.SetCode
	}
	if code == "" {
		return nil
	}
	set, err := b.GetSet(code)
	if err != nil {
		return nil
	}
	return set
}
