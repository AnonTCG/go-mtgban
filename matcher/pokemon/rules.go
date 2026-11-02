package pokemon

import (
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// Rules implements matcher.GameRules for Pokemon. A card is identified by
// name + collector number, with the qualifier label breaking ties: the same
// number recurs as several products differing only by the treatment or the
// promotion TCGplayer decorates the name with, and that label is the only
// thing between them.
//
// The foil treatment is real foilness here, unlike the Yu-Gi-Oh print runs:
// a storefront's foil flag has to reach the Holofoil printing. So the flag
// resolves through the loader's FoilUUIDs, and only an input naming a
// treatment outright re-keys onto the exact crossing it names.
type Rules struct {
	matcher.DefaultRules

	// The sets indexed by the name left after the catalog's era-and-number
	// prefix, built once by Load because the sets do not change afterwards.
	// A Rules built without it still answers, by indexing on the spot.
	setsByTail map[string][]string
}

// NewRules returns the rules with whatever the backend lets them precompute.
func NewRules(b *matcher.Backend) Rules {
	return Rules{setsByTail: indexSetsByTail(b)}
}

// fullNumberRe matches the game's collector number shapes: "001/102",
// "SWSH001", "TG01/TG30", "H1/H32", with the set total that follows a slash
// kept out of the capture.
var fullNumberRe = regexp.MustCompile(`(?i)\b([A-Z]{0,4}\d+[a-z]?)(?:/\d+)?\b`)

// dashedNumberRe matches a collector number written with a dash where the
// card prints a slash, digits on both sides and nothing else.
var dashedNumberRe = regexp.MustCompile(`^\d{1,3}-\d{1,3}$`)

// numberTailRe matches a collector number standing alone, which is the shape
// the storefronts writing one into the name leave behind once the name is
// split off it. A LEGEND pair's joined number is one number.
var numberTailRe = regexp.MustCompile(`(?i)^[A-Z]{0,4}\d+[a-z]?(?:/[A-Z]{0,4}\d+)?(?:[&+]\d+/\d+)?$`)

// pairedNumberRe matches the number the catalog gives a LEGEND pair printed
// as one card, both halves' numbers joined: "99/102 & 100/102", "101/102 +
// 102/102".
var pairedNumberRe = regexp.MustCompile(`\b(\d+/\d+)\s*([&+])\s*(\d+/\d+)\b`)

// joinPairedNumbers spells a pair's number as the one token the datastore
// keeps it as, "99/102&100/102", so it is not read as the two halves.
func joinPairedNumbers(s string) string {
	return pairedNumberRe.ReplaceAllString(s, "$1$2$3")
}

// Prefilter splits off the decorations storefronts write into the name: the
// parentheticals TCGplayer adds ("Pikachu (Cosmos Holo)", "Charizard
// (Prerelease)"), and the collector number Cool Stuff Inc glues on after a
// dash ("Wingull - 70/100"). A name that is itself canonical stays as it is,
// which is what keeps the real card names carrying either decoration - a
// parenthetical of their own, or the World Championship reprints named for
// the number they reprint - from being taken apart.
func (Rules) Prefilter(b *matcher.Backend, inCard *matcher.InputCard) {
	inCard.Variation = joinPairedNumbers(withoutNegatedStamp(inCard.Variation))

	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; found {
		return
	}
	if strings.Contains(inCard.Name, deltaMark) {
		inCard.Name = strings.ReplaceAll(inCard.Name, deltaMark, "")
		inCard.AddToVariant(deltaSpecies)
	}
	if name := cardMarks.Replace(inCard.Name); name != inCard.Name {
		inCard.Name = name
	}
	if name := strings.Join(strings.Fields(inCard.Name), " "); name != inCard.Name {
		inCard.Name = name
	}
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; found {
		return
	}
	// A name the catalog misspells and the datastore corrects, respelled
	// before anything below reads it: the tail strip and the decoration
	// split both work on a name they can find, and neither can find this
	// one until it is spelled the way the datastore carries it.
	//
	// It reads after the marks are folded out because the table is keyed
	// the way the catalog writes a name, and the catalog writes every mark
	// as a word. A name arriving with the symbol on it has to become the
	// catalog's spelling before it can match a key that is one.
	if carried, found := normalizedRespellings()[matcher.Normalize(inCard.Name)]; found {
		inCard.Name = carried
		return
	}
	stripStorefrontTails(b, inCard)
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; found {
		return
	}
	name, tags := splitDecorations(b, inCard.Name)
	if name == inCard.Name {
		return
	}
	inCard.Name = name
	for _, tag := range tags {
		tag = withoutNegatedStamp(tag)
		if tag != "" {
			inCard.AddToVariant(tag)
		}
	}
	// A name the catalog corrects is keyed on the bare spelling, which a
	// dash-numbered listing only becomes once splitDecorations has taken the
	// number off it: "Dark Exeggcutor - 33/105" fails the lookup above
	// wearing its tail and only matches once the tail is gone.
	carried, found := normalizedRespellings()[matcher.Normalize(inCard.Name)]
	if found {
		inCard.Name = carried
	}
}

// negatedStampRe matches a listing's own denial that a card wears a stamp -
// "Non-Stamped", "Unstamped", "Not Stamped", and a denial naming which one
// it lacks, "No Detective Pikachu Stamp" - so that demandsStamp and the
// label tiering never read the denial as the demand: without stripping it,
// a Cool Stuff Inc "Psyduck (Non-Stamped)" prices the stamped SM199 under
// the plain one's listing.
var negatedStampRe = regexp.MustCompile(`(?i)\bnon-?stamped\b|\bunstamped\b|\bnot stamped\b|\bno(?:\s+\S+){0,4}\s+stamps?\b`)

// withoutNegatedStamp drops a stamp denial from a wording, both where a
// storefront writes it into the variation and where it is peeled off the
// name as a decoration.
func withoutNegatedStamp(wording string) string {
	stripped := negatedStampRe.ReplaceAllString(wording, "")
	if stripped == wording {
		return wording
	}
	return strings.Join(strings.Fields(stripped), " ")
}

// levelTailRe matches the level a storefront glues onto a Diamond &
// Pearl-era name ("Moltres Lv.33"). Digits only, on purpose: the catalog
// carries real "LV.X" names, and those must never be taken apart.
var levelTailRe = regexp.MustCompile(`(?i)\s+Lv\.\s*\d+$`)

// letterTailRe matches the single bracketed letter cardtrader tells the
// Unown apart with ("Unown [J]").
var letterTailRe = regexp.MustCompile(`\s+\[([A-Za-z])\]$`)

// deltaTail is the delta-species wording some storefronts spell into the
// EX-era names ("Deoxys δ Delta Species") where the catalog writes the bare
// name.
const deltaTail = " δ Delta Species"

// stripStorefrontTails takes off the decorations a storefront writes into
// the name itself, each only when the undecorated spelling is a name the
// catalog knows - a real name wearing the same shape is never touched. The
// stripped words move into the variation, where they can still tier the
// label.
//
// The dash subtitle is a respelling rather than a strip: the catalog
// brackets the trainer a Supporter names ("Boss's Orders [Cyrus]") where
// cardtrader writes a dash, and the bracketed spelling is tried before the
// bare head - the subtitle is what separates two Supporters sharing a
// number's shape, so it must not be thrown away when the catalog kept it.
func stripStorefrontTails(b *matcher.Backend, inCard *matcher.InputCard) {
	known := func(name string) bool {
		_, found := b.CanonicalNames[matcher.Normalize(name)]
		return found
	}

	if idx := strings.LastIndex(inCard.Name, " - "); idx > 0 {
		head, tail := inCard.Name[:idx], strings.TrimSpace(inCard.Name[idx+3:])
		if bracketed := head + " [" + tail + "]"; known(bracketed) {
			inCard.Name = bracketed
			return
		}
	}

	for {
		name := inCard.Name
		// The level stays out of the variation: its digits would read as
		// one more collector number and be asked before the real one.
		if tail := levelTailRe.FindString(name); tail != "" && known(strings.TrimSuffix(name, tail)) {
			inCard.Name = strings.TrimSuffix(name, tail)
		}
		if strings.HasSuffix(name, deltaTail) && known(strings.TrimSuffix(name, deltaTail)) {
			inCard.Name = strings.TrimSuffix(name, deltaTail)
			inCard.AddToVariant("Delta Species")
		}
		if m := letterTailRe.FindStringSubmatch(name); m != nil && known(strings.TrimSuffix(name, m[0])) {
			inCard.Name = strings.TrimSuffix(name, m[0])
			inCard.AddToVariant(m[1])
		}
		if inCard.Name == name {
			return
		}
	}
}

// splitDecorations peels a storefront's decorations off a product name and
// hands back the name with everything it took.
//
// Both halves have to keep what follows them. A parenthetical is lifted out
// in place rather than truncating the string, because the collector number
// is often written after it - "Greninja V-Union (Bottom Left) - SWSH157" is
// the only thing that tells four identically named quarter cards apart, and
// cutting at the parenthesis throws it away. The dashed segments are then
// read from the right, so a number with wording after it is still found
// ("Dragonite - 5/20 - Cosmos Holo") and the wording travels with it.
//
// A split whose head is a name the catalog knows is taken before the whole
// string is asked about. The catalog spells a handful of promos with the
// number inside the name ("Bouffalant -119/142"), which a decorated listing
// of an ordinary card collides with once its parenthetical is gone; the head
// says which of the two readings the listing meant. Names the catalog really
// spells with a dashed tail and nothing before it to recognise - the World
// Championship reprints, "Torchic - 2004" - are what the whole-string check
// is still there for.
func splitDecorations(b *matcher.Backend, raw string) (string, []string) {
	var tags []string
	name := raw
	for {
		begin := strings.Index(name, "(")
		if begin < 0 {
			break
		}
		end := strings.Index(name[begin:], ")")
		if end < 0 {
			break
		}
		tags = append(tags, strings.TrimSpace(name[begin+1:begin+end]))
		name = strings.Join(strings.Fields(name[:begin]+" "+name[begin+end+1:]), " ")
	}

	known := func(name string) bool {
		_, found := b.CanonicalNames[matcher.Normalize(name)]
		return found
	}
	parts := strings.Split(name, " - ")
	numbered := func(knownHead bool) (string, []string, bool) {
		for i := len(parts) - 1; i >= 1; i-- {
			segment := strings.TrimSpace(parts[i])
			// The catalog spells one number with a dash for its slash,
			// "Morpeko - 072-167" at 072/167; a run of letters and digits
			// like "SWSH287-290" is a span of numbers and not this.
			if dashedNumberRe.MatchString(segment) {
				segment = strings.Replace(segment, "-", "/", 1)
			}
			segment = joinPairedNumbers(segment)
			if !numberTailRe.MatchString(segment) {
				continue
			}
			head := strings.TrimSpace(strings.Join(parts[:i], " - "))
			if head == "" || (knownHead && !known(head)) {
				continue
			}
			split := append([]string{}, tags...)
			for _, extra := range parts[i+1:] {
				split = append(split, strings.TrimSpace(extra))
			}
			return head, append(split, segment), true
		}
		return "", nil, false
	}

	if head, split, found := numbered(true); found {
		return head, split
	}
	if known(name) {
		return name, tags
	}
	// The catalog drops the space after the dash on a few promos,
	// "Ninetales -199/197", where the datastore names the card "Ninetales".
	idx := strings.LastIndex(name, " -")
	if idx > 0 {
		head, segment := name[:idx], strings.TrimSpace(name[idx+2:])
		if numberTailRe.MatchString(segment) && known(head) {
			return head, append(append([]string{}, tags...), segment)
		}
	}
	if head, split, found := numbered(false); found {
		return head, split
	}
	// A bracketed qualifier behind the number is read last, and only where
	// nothing above could read the name with it on: the catalog writes the
	// set a Burger King promo reprints in brackets after the number,
	// "Pichu - 45/106 [Platinum]", and a name carrying a bracket of its
	// own - "Ancient Technical Machine [Ice]" - was found whole above and
	// never reaches this.
	if strings.Contains(name, "[") {
		peeled, bracketed := name, tags
		for {
			begin := strings.Index(peeled, "[")
			if begin < 0 {
				break
			}
			end := strings.Index(peeled[begin:], "]")
			if end < 0 {
				break
			}
			bracketed = append(bracketed, strings.TrimSpace(peeled[begin+1:begin+end]))
			peeled = strings.Join(strings.Fields(peeled[:begin]+" "+peeled[begin+end+1:]), " ")
		}
		if peeled != name {
			if head, split := splitDecorations(b, peeled); head != peeled || known(peeled) {
				return head, append(bracketed[len(tags):], split...)
			}
		}
	}
	return name, tags
}

// AdjustName provides a prefix fallback for truncated feeds, adopting the
// one name among the prefix matches that carries the input's number. Names
// compare normalized, so punctuation variants of one name are not read as
// ambiguity.
func (Rules) AdjustName(b *matcher.Backend, inCard *matcher.InputCard) {
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; found {
		return
	}
	numbers := extractNumbers(inCard.Variation)
	if len(numbers) == 0 {
		return
	}
	uuids, err := b.SearchHasPrefix(inCard.Name)
	if err != nil {
		return
	}
	name := ""
	for _, uuid := range uuids {
		co, found := b.UUIDs[uuid]
		if !found || co.Sealed {
			continue
		}
		if reprintedByYear(inCard.Name, co.Name) {
			continue
		}
		if !numbersMatchCard(b, numbers, &co.Card) {
			continue
		}
		if name != "" && matcher.Normalize(name) != matcher.Normalize(co.Name) {
			return
		}
		name = co.Name
	}
	if name != "" {
		inCard.Name = name
	}
}

// reprintYearRe matches what a World Championship reprint adds to the name
// it reprints: a dash and the year the deck was played.
var reprintYearRe = regexp.MustCompile(` - (?:19|20)\d{2}$`)

// reprintedByYear reports whether a name is another name spelled with the
// year it was played. The World Championship decks reprint 1,951 printings
// that way and keep the original's collector number, so a prefix search for
// a bare name nearly always turns one up - and since it carries the number
// too, it ties with the spelling the storefront actually meant and the
// ambiguity guard abandons the search.
//
// The dash is what makes this the World Championship spelling and not any
// name that merely ends in a year: 29 other printings do - the Victory
// Medals, the Code Cards, the Champion's Festivals - and every one of them
// writes the year without it, inside a parenthetical or glued to the words
// before. The head compares normalized, so punctuation variants of the name
// being reprinted still answer.
func reprintedByYear(name, candidate string) bool {
	tail := reprintYearRe.FindString(candidate)
	if tail == "" {
		return false
	}
	head := strings.TrimSuffix(candidate, tail)
	return matcher.Normalize(head) == matcher.Normalize(name)
}

// AdjustEdition trims the game-name prefix and "Singles" suffix storefronts
// decorate set names with, rewrites the names editionAliases carries, and
// drops the headings that name no set of ours.
func (r Rules) AdjustEdition(b *matcher.Backend, inCard *matcher.InputCard) {
	edition := r.AliasEdition(b, inCard.Edition)
	// The sequin holo printings are the General Mills cereal promos, which
	// the catalog files in Miscellaneous Cards & Products under the promo
	// set's own numbering. The promo set a sequin listing names carries
	// only the plain printing, and landing there would file the sequin
	// price under the plain product's uuid.
	if matcher.SlugDescribes(inCard.Variation, "sequin") {
		edition = "Miscellaneous Cards & Products"
	}
	// A championship is a set of its own, named for the year it was held,
	// and a storefront writes one shelf for all of them. The year is
	// on the card instead - "Torchic - 2004" - so it says which.
	if worldsShelfRe.MatchString(edition) {
		if year := worldsYear(inCard); year != "" {
			edition = year + " " + edition
		}
	}
	inCard.Edition = edition

	adjustEnergyYear(b, inCard)

	widenQualifiedName(b, inCard)
}

// basicEnergyRe matches a bare basic energy name, the shape TCGplayer prices
// every printing of one under regardless of the game era.
var basicEnergyRe = regexp.MustCompile(`(?i)^(?:Grass|Fire|Water|Lightning|Psychic|Fighting|Darkness|Metal|Fairy) Energy$`)

// adjustEnergyYear redirects a basic energy's edition to the set the catalog
// actually filed that copyright year's unnumbered printing under, for a
// shelf that packs the energy but sells no product of its own for it:
// Hidden Fates and Champion's Path each hand out one, filed under "SM - Team
// Up" and "SWSH01" instead. It runs only where the named edition holds no
// card of the name at all, so a shelf that really sells the energy - the
// League & Championship Cards run - is never touched.
func adjustEnergyYear(b *matcher.Backend, inCard *matcher.InputCard) {
	if !basicEnergyRe.MatchString(inCard.Name) || len(extractNumbers(inCard.Variation)) > 0 {
		return
	}
	if editionHasName(b, inCard.Edition, inCard.Name) {
		return
	}
	set, found := energyYearSets[energyYear(b, inCard)]
	if !found {
		return
	}
	inCard.Edition = set
	inCard.Variation = stripEnergyYearWording(inCard.Variation)
}

// editionHasName reports whether the set an edition names carries some
// printing of the name.
func editionHasName(b *matcher.Backend, edition, name string) bool {
	code := editionSetCode(b, edition)
	if code == "" {
		return false
	}
	for _, uuid := range b.Hashes[matcher.Normalize(name)] {
		if co, found := b.UUIDs[uuid]; found && co.SetCode == code {
			return true
		}
	}
	return false
}

// energyYear reads the copyright year a basic energy's wording carries, or
// the release year of the shelf its edition names where the wording carries
// none.
func energyYear(b *matcher.Backend, inCard *matcher.InputCard) string {
	m := yearRe.FindStringSubmatch(inCard.Variation)
	if m != nil {
		return m[1]
	}
	set, found := b.Sets[editionSetCode(b, inCard.Edition)]
	if found && len(set.ReleaseDate) >= 4 {
		return set.ReleaseDate[:4]
	}
	return ""
}

// energyYearWordingRe matches what stripEnergyYearWording removes from a
// basic energy's variation once adjustEnergyYear has redirected its edition:
// the bare or copyright-marked year that named it, and the run wording that
// is redundant with the foil flag once the printing is unambiguous.
var energyYearWordingRe = regexp.MustCompile(`(?i)©?(?:19|20)[0-9]{2}|reverse holo(?:foil)?|reverse foil`)

// stripEnergyYearWording removes the wording energyYearWordingRe matches.
func stripEnergyYearWording(variation string) string {
	stripped := strings.ReplaceAll(energyYearWordingRe.ReplaceAllString(variation, ""), "|", " ")
	return strings.Join(strings.Fields(stripped), " ")
}

// worldsShelfRe matches the shelf every World Championship deck was filed
// on before each championship became a set, and yearRe the year that says
// which one a card belongs to.
var (
	worldsShelfRe = regexp.MustCompile(`(?i)^world championship decks?$`)
	yearRe        = regexp.MustCompile(`\b((?:19|20)\d{2})\b`)
)

// worldsYear reads the championship year off whatever the caller filled in.
func worldsYear(inCard *matcher.InputCard) string {
	for _, field := range []string{inCard.Name, inCard.Variation} {
		if m := yearRe.FindStringSubmatch(field); m != nil {
			return m[1]
		}
	}
	return ""
}

// AliasEdition spells an edition string toward a set name using the string
// alone. See matcher.GameRules.
func (r Rules) AliasEdition(b *matcher.Backend, edition string) string {
	edition = strings.TrimSpace(edition)
	for _, prefix := range []string{"Pokemon TCG", "Pokemon", "Pokémon"} {
		if strings.HasPrefix(edition, prefix) {
			trimmed := strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(edition, prefix), ":-"))
			// A set really named for the game keeps its name: "Pokemon Go"
			// and "Pokemon Futsal Promos" are sets, not decorations.
			if _, err := b.GetSetByName(trimmed); err == nil {
				edition = trimmed
			}
			break
		}
	}
	edition = strings.TrimSpace(strings.TrimSuffix(edition, "Singles"))
	// The generic promo headings resolve to no set on purpose: the heading
	// spans every promotional set the game has - and it has more than
	// thirty - so it can gate the promo names but must not narrow the
	// candidates to whichever one happens to wear the name.
	if matcher.IsPromoHeading(edition) {
		edition = ""
	}
	// A storefront spelling the era but not the set's number within it
	// ("SWSH Darkness Ablaze") names a set the catalog calls something
	// longer ("SWSH03: Darkness Ablaze"); ask what the two have in common
	// rather than leaving the edition to narrow nothing.
	// The lookups here are the direct ones on purpose: GetSetByName asks
	// the rules to alias the edition, which is this function.
	if edition != "" {
		_, known := b.NormalizedSets[matcher.Normalize(edition)]
		if !known {
			_, err := b.GetSet(edition)
			known = err == nil
		}
		// A pooled name is left as it is: it spans two sets, so no single
		// rewrite can carry it, and FilterCards restricts on the name
		// itself instead.
		if _, pooled := normalizedPooledEditions()[matcher.Normalize(edition)]; !known && !pooled {
			name := normalizedEditionAliases()[matcher.Normalize(edition)]
			if name == "" {
				name = promoSetNamed(b, edition)
			}
			if name == "" {
				name = setNamedByHead(b, edition)
			}
			if name == "" {
				name = r.setNamedByTail(b, edition)
			}
			if name != "" {
				edition = name
			}
		}
	}
	return edition
}

// widenQualifiedName adopts the qualified spelling of a name the catalog
// bakes the disambiguator into. "Accelgor (Team Plasma)" is how the catalog
// writes the 8/101 of Plasma Blast and "Professor's Research [Professor
// Juniper]" the 085/086 of Black Bolt, while the storefront writes the bare
// name and the collector number - so the name hash holds nothing in the set
// the listing names and the row dies on the number instead.
//
// It runs once the edition is resolved, and only when no printing named
// exactly as the input carries that number there: a name the catalog already
// knows at that number is never rewritten. Two qualified spellings sharing
// the number mean the listing does not say which, so ambiguity is left for
// the caller to report rather than guessed at.
func widenQualifiedName(b *matcher.Backend, inCard *matcher.InputCard) {
	numbers := extractNumbers(inCard.Variation)
	if len(numbers) == 0 {
		return
	}
	code := editionSetCode(b, inCard.Edition)
	// An edition naming no set widens nothing. The storefronts carry whole
	// Japanese catalogs under set names ours has never heard of ("Plasma
	// Storm Promos", the JP "Scarlet & Violet Promos"), and widening there
	// is how a Japanese listing walked onto an English printing: the JP
	// promo's number is real, some qualified English name carries it, and
	// with no edition to gate on the rename went through.
	if code == "" {
		return
	}
	inEdition := func(co *matcher.CardObject) bool {
		return co.SetCode == code
	}
	for _, uuid := range b.Hashes[matcher.Normalize(inCard.Name)] {
		co, found := b.UUIDs[uuid]
		if !found || co.Sealed || !inEdition(co) {
			continue
		}
		if numbersMatchCard(b, numbers, &co.Card) {
			return
		}
	}
	uuids, err := b.SearchHasPrefix(inCard.Name)
	if err != nil {
		return
	}
	name := ""
	for _, uuid := range uuids {
		co, found := b.UUIDs[uuid]
		if !found || co.Sealed || !inEdition(co) {
			continue
		}
		if !numbersMatchCard(b, numbers, &co.Card) {
			continue
		}
		if name != "" && matcher.Normalize(name) != matcher.Normalize(co.Name) {
			return
		}
		name = co.Name
	}
	if name != "" {
		inCard.Name = name
	}
}

// editionSetCode answers the code of the set an already-adjusted edition
// names, and an empty string when it names none. The lookups are the direct
// ones because GetSetByName would ask the rules to adjust the edition again.
func editionSetCode(b *matcher.Backend, edition string) string {
	if edition == "" {
		return ""
	}
	if set, found := b.NormalizedSets[matcher.Normalize(edition)]; found {
		return set.Code
	}
	if set, err := b.GetSet(edition); err == nil {
		return set.Code
	}
	return ""
}

// promoSetNamed answers the promo set an era's heading means. The catalog
// names them with the era, a colon and a longer title - "SV: Scarlet &
// Violet Promo Cards", "SWSH: Sword & Shield Promo Cards" - while the
// storefronts head them "SV Promos" and "SWSH Promos", which agree on
// nothing after the era and so name no set at all. With no edition to narrow
// on, the row is matched against every printing of the name and aliases
// against the Jumbo and Deck Exclusive reprints, which carry the promo's own
// number.
//
// Only a single word can be the era, and only one promo set may answer to
// it: an era whose promos the catalog split across several sets does not say
// which one a listing means.
func promoSetNamed(b *matcher.Backend, edition string) string {
	era := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(edition, "Promos"), "Promo"))
	if era == "" || era == edition || strings.Contains(era, " ") {
		return ""
	}
	name := ""
	for _, set := range b.Sets {
		if set.Type != setTypePromo || !strings.HasPrefix(set.Name, era+": ") {
			continue
		}
		if name != "" {
			return ""
		}
		name = set.Name
	}
	return name
}

// setNamedByHead answers the set a storefront decorates with a trailing
// "Base Set", which is how three eras' first sets are headed: "Diamond and
// Pearl Base Set", "Platinum Base Set", "Expedition Base Set". Dropping the
// leading words instead lands every one of them on the set literally named
// "Base Set", 1999's, and the row then either dies or is priced as a card it
// is not.
//
// Only the trailing "Base Set" comes off, and only onto a name that is a set
// outright. Dropping trailing words in general is what must not happen:
// "Diamond and Pearl Stormfront" would resolve to "Diamond and Pearl", and
// measured that way it costs 335 otherwise correct entries.
func setNamedByHead(b *matcher.Backend, edition string) string {
	head := strings.TrimSpace(strings.TrimSuffix(edition, "Base Set"))
	if head == "" || head == edition {
		return ""
	}
	if _, known := b.NormalizedSets[matcher.Normalize(head)]; !known {
		return ""
	}
	return head
}

// setTail drops the prefix the catalog decorates a set name with: both
// "SWSH03: Darkness Ablaze" and "XY - Steam Siege" are an era, a separator,
// and the name the set is actually known by.
func setTail(name string) string {
	if _, after, found := strings.Cut(name, ": "); found {
		return after
	}
	if _, after, found := strings.Cut(name, " - "); found {
		return after
	}
	return name
}

// setNamedByTail answers the set an era-prefixed spelling means. Storefronts
// write the era and the name where the catalog writes the era, the set's
// number within it, and the name, so what follows either prefix is what has
// to agree. Leading words are dropped one at a time, since how much of the
// prefix is era is not knowable in advance - "Diamond and Pearl Great
// Encounters" spends three words on it, and Cardmarket spends none at all,
// which is why the whole name is tried before any of it is dropped. A tail
// two sets share names neither.
func indexSetsByTail(b *matcher.Backend) map[string][]string {
	index := map[string][]string{}
	for _, set := range b.Sets {
		tail := matcher.Normalize(setTail(set.Name))
		index[tail] = append(index[tail], set.Name)
	}
	return index
}

func (r Rules) setNamedByTail(b *matcher.Backend, edition string) string {
	index := r.setsByTail
	if index == nil {
		index = indexSetsByTail(b)
	}

	fields := strings.Fields(edition)
	for i := range fields {
		names := index[matcher.Normalize(strings.Join(fields[i:], " "))]
		if len(names) == 1 {
			return names[0]
		}
		if len(names) > 1 {
			return ""
		}
	}
	return ""
}

// FilterCards narrows candidates by edition, collector number and label, in
// that order. A bare input facing several labels keeps them all and surfaces
// as an aliasing error rather than a guess.
func (Rules) FilterCards(b *matcher.Backend, inCard *matcher.InputCard, sets []matcher.CandidateSet) []matcher.Card {
	// A wording naming a metal card is answered by the metal printing before
	// its numbers are read one at a time: "205 151 Metal Card" is Mew ex 205,
	// and 151 is both the set's name and a Mew ex of its own.
	if matcher.SlugDescribes(inCard.Variation, "metalcard") {
		if metal := metalCandidates(b, extractNumbers(inCard.Variation), sets); len(metal) > 0 {
			return metal
		}
	}
	candidates := filterCandidates(b, inCard, sets)

	// A wording naming a stamp or a misprint prices a physical variant the
	// plain printing is not: "Pokemon Center Stamped", "Professor Program
	// Stamp", "Partial 1st Ed Stamp Misprint". When no surviving candidate
	// wears a stamp label, whatever survived is the unstamped product, and
	// pricing it would file the variant's price under the plain uuid - the
	// wording can even describe a label the candidate does wear ("Cosmos
	// Holo" beside the stamp) without the stamp being the one it wears.
	// Candidates that do wear stamp labels are left to the ordinary
	// tiering: the wording and the label can spell the same stamp
	// differently.
	if len(candidates) > 0 && demandsStamp(inCard.Variation) {
		stamped := false
		for _, card := range candidates {
			for _, promoType := range card.PromoTypes {
				if strings.Contains(promoType, "stamp") {
					stamped = true
				}
			}
		}
		if !stamped {
			return nil
		}
	}

	// A wording naming the Cosmos Holo outright has to reach a printing
	// that can be one: a candidate wearing the label, or at least selling
	// a holo entry the finish resolution can land on. What survives
	// otherwise is a plain printing that happens to share the number -
	// the treated printing lives in a set the edition does not admit -
	// and pricing it would file the holo's price under the plain uuid.
	// A wording naming the plain treatment too is the ambiguity the
	// tiering already surfaces, and is left alone.
	if len(candidates) > 0 && !describesPlain(inCard.Variation) &&
		matcher.SlugDescribes(inCard.Variation, "cosmosholo") {
		var cosmos []matcher.Card
		for _, card := range candidates {
			if wearsCosmosOrHolo(&card) {
				cosmos = append(cosmos, card)
			}
		}
		candidates = cosmos
	}

	// A wording naming a placement - Staff, Champion, Top 32 - prices a
	// copy the plain printing is not, and a survivor wearing none of the
	// placements it names is that plain printing, or another placement's:
	// refuse rather than price it. A wording naming the placement of a
	// card the edition sells only plain is a card we do not carry.
	if len(candidates) > 0 {
		if named := placementsNamed(b, labelWording(inCard.Variation)); len(named) > 0 && !wearsAny(candidates, named) {
			return nil
		}
	}

	// A wording spelling a LEGEND pair's number prices the one card printed
	// with both halves' numbers, and a survivor printing one of them alone is
	// a half the edition or the name let through: refuse rather than price it.
	if len(candidates) > 0 && pairedNumberRe.MatchString(inCard.Variation) {
		var pairs []matcher.Card
		for _, card := range candidates {
			if pairedNumberRe.MatchString(printedFace(&card)) {
				pairs = append(pairs, card)
			}
		}
		candidates = pairs
	}

	// The five CSI shelves editionAliases narrows to one specific set also
	// hold listings of that number from a different, unnamed set - a Deck
	// Exclusives or Black and White Promos twin selling the treatment the
	// resolved set does not. This runs for every listing that resolves to
	// one of those five sets, whichever vendor named it or aliased onto it -
	// Card Trader's own "BW Black Star Promos" among them - so a candidate
	// sold in only one finish class is left alone regardless of what the
	// wording asks for, per Card.Finishes' own contract: only a candidate
	// actually sold in more than one class is narrowed, and refused when
	// the narrowed set can't supply the class the wording names.
	if len(candidates) > 0 && shelfFinishChecked[inCard.Edition] {
		switch {
		case describesPlain(inCard.Variation):
			var plain []matcher.Card
			for _, card := range candidates {
				if slices.Contains(card.Finishes, matcher.FinishNonfoil) {
					plain = append(plain, card)
				}
			}
			if len(plain) == 0 {
				return nil
			}
			candidates = plain
		case describesHolo(inCard.Variation) && !matcher.SlugDescribes(inCard.Variation, "cosmosholo"):
			var holo []matcher.Card
			for _, card := range candidates {
				if len(card.Finishes) < 2 || matcher.NamedFinish(card.FoilUUIDs, "", "Holofoil") != "" {
					holo = append(holo, card)
				}
			}
			if len(holo) == 0 {
				return nil
			}
			candidates = holo
		}
	}
	return candidates
}

// shelfFinishChecked names the sets editionAliases resolves a CSI era-only
// shelf to, each also holding listings of that number from a different,
// unnamed set. The plain/holo guard above runs for every listing that
// resolves to one of these five sets, not only Cool Stuff Inc's own
// shelves: Card Trader's "BW Black Star Promos" aliases onto the same
// "Black and White Promos" set and reaches it too.
var shelfFinishChecked = map[string]bool{
	"SV: Scarlet & Violet 151":        true,
	"SV01: Scarlet & Violet Base Set": true,
	"SM Base Set":                     true,
	"SWSH01: Sword & Shield Base Set": true,
	"Black and White Promos":          true,
}

// describesHolo reports whether the wording names a true Holofoil copy by
// itself, as opposed to the Reverse Holofoil a shared number can carry
// instead, or the plain copy describesPlain already reads.
func describesHolo(variation string) bool {
	v := strings.ToLower(variation)
	return strings.Contains(v, "holo") && !strings.Contains(v, "reverse") && !describesPlain(variation)
}

// placements are the labels that tell the copies of one promo apart by
// where its holder placed, which no storefront names by accident.
var placements = []string{"staff", "champion", "finalist", "semifinalist", "quarterfinalist", "top32", "top16", "top8", "top4", "winner"}

// placementsNamed lists the placements the datastore labels with that the
// wording names.
func placementsNamed(b *matcher.Backend, wording string) []string {
	var named []string
	for _, placement := range placements {
		if _, labelled := b.PromoTypeLabels[placement]; !labelled {
			continue
		}
		if matcher.SlugDescribes(wording, placement) {
			named = append(named, placement)
		}
	}
	return named
}

// wearsAny reports whether some candidate wears one of the labels.
func wearsAny(candidates []matcher.Card, labels []string) bool {
	for _, card := range candidates {
		for _, promoType := range card.PromoTypes {
			if slices.Contains(labels, promoType) {
				return true
			}
		}
	}
	return false
}

// wearsCosmosOrHolo reports whether the printing can be the Cosmos Holo a
// wording names: it wears a cosmos label, or it is sold in a foil, every one
// of which the game prints is a holo.
func wearsCosmosOrHolo(card *matcher.Card) bool {
	for _, promoType := range card.PromoTypes {
		if strings.Contains(promoType, "cosmos") {
			return true
		}
	}
	return slices.Contains(card.Finishes, matcher.FinishFoil)
}

// demandsStamp reports whether the wording names a stamped or misprinted
// variant.
func demandsStamp(variation string) bool {
	variation = strings.ToLower(variation)
	return strings.Contains(variation, "stamp") || strings.Contains(variation, "misprint")
}

func filterCandidates(b *matcher.Backend, inCard *matcher.InputCard, sets []matcher.CandidateSet) []matcher.Card {
	// A pooled storefront name restricts the candidates to its pair of
	// sets outright: the edition kept the name, so nothing upstream could
	// narrow, and falling through past the pool would price a card the
	// pool does not carry as some same-numbered printing elsewhere. A
	// listing saying oversized is already held to its oversized printing,
	// which no pool carries.
	if pool, found := normalizedPooledEditions()[matcher.Normalize(inCard.Edition)]; found && !inCard.Contains("Oversize") {
		inPool := map[string]bool{}
		for _, name := range pool {
			if set, ok := b.NormalizedSets[matcher.Normalize(name)]; ok {
				inPool[set.Code] = true
			}
		}
		var pooled []matcher.CandidateSet
		for _, set := range sets {
			parent, found := parentOf(b, set.Code)
			if inPool[set.Code] || (found && inPool[parent] &&
				slices.ContainsFunc(sets, func(s matcher.CandidateSet) bool { return s.Code == parent })) {
				pooled = append(pooled, set)
			}
		}
		sets = pooled
	}

	// A storefront can write more than one number into the wording: Cool
	// Stuff Inc prepends its own catalogue field, which is empty, "0" or
	// "001" for whole editions, and the number the listing prints is the
	// one Prefilter appends after it. Read them from the back, so the
	// number the card carries on it is asked first and the vendor's index
	// only answers when nothing else does.
	numbers := extractNumbers(inCard.Variation)
	number := ""
	var candidates, promo []matcher.Card
	for _, number0 := range slices.Backward(numbers) {
		number = number0
		candidates = filterByNumber(b, inCard, sets, number)
		// A lettered number the edition admits no printing of names a
		// promo printing filed in a set of its own. The storefronts sell
		// the alternate arts under the set the card was first printed in
		// - Cool Stuff Inc lists "Garbodor (Alt Art) - 51a/145" against
		// "SM Guardians Rising" - where the catalog files them in
		// "Alternate Art Promos", which the edition then keeps out. The
		// letter says which printing this is, so it is asked of the
		// catalog entire, and before an earlier number the vendor wrote
		// without it. Where the edition holds the number only as a row
		// TCGplayer never priced, the promo printing carrying the id wins.
		if letteredNumberRe.MatchString(number0) && len(withProductID(candidates)) == 0 {
			promo = letteredPromo(b, inCard, []string{number0})
			if len(candidates) > 0 {
				promo = withProductID(promo)
			}
			if len(promo) > 0 {
				break
			}
		}
		if len(candidates) > 0 {
			break
		}
	}
	if len(promo) == 1 {
		return promo
	}
	// The catalog carries the lettered number more than once: TCGplayer
	// sells several of these promos twice, once at their own size and once
	// as an oversized jumbo, with the same name, number and finish and
	// nothing on the card to tell them apart. The set is what tells them
	// apart, and the wording names it - "(Alt Art)" against "Alternate Art
	// Promos" beside "Jumbo Cards". A set holding the number once per
	// finish is left to the label tiers below. A wording naming neither set
	// leaves the letter standing: dropping it would answer with the plain
	// card the promo reprints, which is a third card again.
	if len(promo) > 1 {
		named := setNamed(b, inCard, promo)
		switch {
		case len(named) == 1:
			return named
		case len(named) > 1:
			candidates = named
		case len(candidates) == 0:
			return nil
		}
	}
	// A letter hung off the end of a number the catalog carries without one
	// is the storefront's own marker for a printing it prices apart:
	// Strikezone numbers the Master Ball patterns "074M" beside the plain
	// "074", and the run-marked reprints "001A". The number as written is
	// asked first and every time, so the stripped form only ever answers
	// where the written one reached nothing, and the label tier below is
	// what picks the marked printing out of the ones it reaches.
	if len(candidates) == 0 {
		for _, number0 := range slices.Backward(numbers) {
			bare := strings.TrimRight(number0, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
			if bare == "" || bare == number0 {
				continue
			}
			number = bare
			candidates = filterByNumber(b, inCard, sets, bare)
			if len(candidates) > 0 {
				break
			}
		}
	}
	// The unnumbered pass answers only for a wording that wrote no number
	// at all: any number the wording did write overwrites it on the loop's
	// first turn, kept or not.
	if len(numbers) == 0 {
		candidates = filterByNumber(b, inCard, sets, "")
	}
	if len(candidates) <= 1 {
		return candidates
	}
	// A wording that wrote no number means the copy that carries none,
	// where the catalog files one beside a numbered twin under the same
	// mark: the World Championship decks sell an energy both ways under
	// one player's name, and the deck's own rule keeps whichever came
	// first.
	if len(numbers) == 0 {
		if bare := unnumberedTwins(candidates); len(bare) > 0 {
			candidates = bare
		}
	}

	// A wording naming a printing's label has said which copy it prices,
	// and the tiers below - the verbatim number, the mark - are read among
	// the copies it named: the placement tiers of a Champions Festival
	// wear no year where the plain copy does, and the year alone kept the
	// plain. A wording naming the plain treatment too is the ambiguity
	// tierByLabel surfaces, and keeps every copy for it.
	wording := labelWording(inCard.Variation)
	// A wording naming a stamp prices a stamped copy, and the stamp is a
	// label spelled its own way ("Stamp" for the stamped): the copies
	// wearing one are the candidates, before anything else is read.
	if demandsStamp(wording) {
		if stamped := wearingStamp(candidates); len(stamped) > 0 {
			candidates = stamped
		}
	}
	if !describesPlain(wording) {
		described, _, _ := labelReading(b, wording, candidates)
		marked := marksNamed(wording, candidates)
		// A wording naming a mark and a label both means the copy wearing
		// both - the Stellar Crown stamp on the Cosmos Holo - and where no
		// copy wears both, the label says more about which copy is priced
		// than the mark does.
		both := intersection(described, marked)
		switch {
		case len(both) > 0:
			candidates = both
		case len(described) > 0:
			candidates = described
		case len(marked) > 0:
			candidates = marked
		}
	}

	// A verbatim collector number beats a prefix-folded match, and so does
	// one whose set total agrees. The total is what separates a reprint
	// from its original - Cascoon is 44/130 in Diamond & Pearl and 44/127
	// in Platinum - and it is exactly the part the fold drops, which left
	// this tier inert for the nine printings in ten the catalog writes a
	// total on. An agreeing total also beats a verbatim bare number: the
	// wording that spells "13/147" names the 147-card set's printing, not
	// the promo the catalog numbers a bare "13".
	if number != "" {
		var plain, full []matcher.Card
		for _, card := range candidates {
			switch {
			case fullNumberMatches(inCard.Variation, printedFace(&card)):
				full = append(full, card)
			case strings.EqualFold(number, printedFace(&card)):
				plain = append(plain, card)
			}
		}
		exact := full
		if len(exact) == 0 {
			exact = plain
		}
		if len(exact) > 0 && len(exact) < len(candidates) {
			candidates = exact
		}
		if len(candidates) <= 1 {
			return candidates
		}
	}

	if marked := tierByMark(wording, candidates); len(marked) > 0 {
		candidates = marked
	}
	candidates = tierByLabel(b, wording, candidates)
	candidates = tierByQualifier(b, inCard, candidates)
	// A wording naming a bare year - the league energies' only way of
	// saying which year's row they price, since every year's copy of one
	// shares its name, number and label - narrows by it last, among
	// whatever the mark, label and qualifier tiers above left standing.
	candidates = tierByYear(inCard, wording, candidates)
	candidates = tierByFinish(wording, candidates)
	// TCGplayer sells a handful of stamped promos twice, once at their own
	// size and once as an oversized Jumbo Card, with nothing but the size
	// to tell the two apart; a wording that never says so means the
	// ordinary one.
	return excludeJumbo(wording, candidates)
}

// FilterPrintings holds a listing that says oversized to the sets printing
// the card oversized at its number. Core lets the word through only for a
// card with such a printing, and a storefront files a jumbo under the
// ordinary card's set, where the number alone answers with the ordinary
// card. A card can be printed oversized more than once, so with no number,
// or no oversized printing at it wearing the placement the wording names
// ("Winner"), the list comes back empty and core refuses the listing. A
// listing naming a metal card is held to its metal printing instead; see
// printingMetal. See matcher.GameRules.
func (Rules) FilterPrintings(b *matcher.Backend, inCard *matcher.InputCard, editions []string) []string {
	// Asked first: Card Trader calls its metal cards "Oversized Thick Card",
	// and none of them is the jumbo that word otherwise means.
	if matcher.SlugDescribes(inCard.Variation, "metalcard") {
		return printingMetal(b, inCard.Name, extractNumbers(inCard.Variation), editions)
	}
	if !inCard.Contains("Oversize") {
		return editions
	}
	numbers := extractNumbers(inCard.Variation)
	named := placementsNamed(b, labelWording(inCard.Variation))
	held := printingOversized(b, inCard.Name, numbers, named, editions)
	// A letter hung off the number is dropped the way filterCandidates
	// drops it, only where the number as written reaches nothing.
	if len(held) == 0 {
		var bare []string
		for _, number := range numbers {
			if trimmed := strings.TrimRight(number, plainNumberTail); trimmed != "" && trimmed != number {
				bare = append(bare, trimmed)
			}
		}
		held = printingOversized(b, inCard.Name, bare, named, editions)
	}
	return held
}

// printingOversized returns the editions holding an oversized printing of
// the card at one of the numbers, wearing one of the named placements when
// there are any.
func printingOversized(b *matcher.Backend, name string, numbers, named, editions []string) []string {
	var held []string
	for _, code := range editions {
		set, found := b.Sets[code]
		if !found {
			continue
		}
		for i := range set.Cards {
			card := &set.Cards[i]
			if card.IsOversized && matcher.Equals(card.Name, name) && numbersMatchCard(b, numbers, card) &&
				(len(named) == 0 || wearsAny([]matcher.Card{*card}, named)) {
				held = append(held, code)
				break
			}
		}
	}
	return held
}

// printingMetal returns the editions holding the card's metal printing, at
// one of the numbers when the wording gives any. TCGplayer files the metal
// cards on its miscellaneous shelf, which a storefront writing the card's own
// set never reaches: "Greninja ex / Twilight Masquerade / 106 Metal Card"
// answered with the ordinary 106. With no metal printing to hold to, the
// editions stand, and IsUnsupported refuses the plain card they answer with.
func printingMetal(b *matcher.Backend, name string, numbers, editions []string) []string {
	var held []string
	for _, code := range editions {
		set, found := b.Sets[code]
		if !found {
			continue
		}
		for i := range set.Cards {
			card := &set.Cards[i]
			if matcher.Equals(card.Name, name) && metalAt(b, numbers, card) {
				held = append(held, code)
				break
			}
		}
	}
	if len(held) == 0 {
		return editions
	}
	return held
}

// metalAt reports whether the card is a metal printing at one of the
// numbers, or at all when the wording gives none. A letter hung off a number
// is dropped the way filterCandidates drops it; no metal card is lettered.
func metalAt(b *matcher.Backend, numbers []string, card *matcher.Card) bool {
	if !card.HasPromoType("metalcard") {
		return false
	}
	if len(numbers) == 0 {
		return true
	}
	for _, number := range numbers {
		trimmed := strings.TrimRight(number, plainNumberTail)
		if numberMatchesCard(b, number, card) || (trimmed != "" && numberMatchesCard(b, trimmed, card)) {
			return true
		}
	}
	return false
}

// metalCandidates returns the metal printings among the candidates, at one of
// the numbers when the wording gives any, in set code order.
func metalCandidates(b *matcher.Backend, numbers []string, sets []matcher.CandidateSet) []matcher.Card {
	var metal []matcher.Card
	for _, set := range sets {
		for i := range set.Cards {
			if metalAt(b, numbers, &set.Cards[i]) {
				metal = append(metal, set.Cards[i])
			}
		}
	}
	return metal
}

// IsUnsupported refuses a listing naming a metal card that answered with a
// printing that is not one: that metal card is not carried, and the plain
// card is not what the listing sells. See matcher.GameRules.
func (Rules) IsUnsupported(b *matcher.Backend, inCard *matcher.InputCard, co *matcher.CardObject, stage matcher.Stage) bool {
	return stage == matcher.StageAnswer && matcher.SlugDescribes(inCard.Variation, "metalcard") && !co.HasPromoType("metalcard")
}

// jumboSetCode is the set the Jumbo Cards oversized reprints are filed
// under.
const jumboSetCode = "PR-1528"

// excludeJumbo drops the Jumbo Cards printings among the candidates when a
// non-jumbo alternative remains and the wording never says the copy is the
// oversized one. A promo sold only as a Jumbo has no alternative to keep,
// and is left alone.
func excludeJumbo(wording string, candidates []matcher.Card) []matcher.Card {
	if matcher.SlugDescribes(wording, "jumbo") || matcher.SlugDescribes(wording, "oversize") || matcher.SlugDescribes(wording, "oversized") {
		return candidates
	}
	var kept []matcher.Card
	var hasJumbo bool
	for _, card := range candidates {
		if card.SetCode == jumboSetCode {
			hasJumbo = true
			continue
		}
		kept = append(kept, card)
	}
	if !hasJumbo || len(kept) == 0 {
		return candidates
	}
	return kept
}

// tierByFinish keeps, among copies still tied, the ones sold in the finish
// the wording names: "Blaziken (Holo)" beside the plain Blaziken at the same
// EX Battle Stadium number, two products nothing else tells apart. It only
// reads a tie within one set and number: across several, the finish would be
// picking between cards the wording never named.
func tierByFinish(wording string, candidates []matcher.Card) []matcher.Card {
	if len(candidates) <= 1 {
		return candidates
	}
	for _, card := range candidates {
		if card.SetCode != candidates[0].SetCode || card.Number != candidates[0].Number {
			return candidates
		}
	}
	// "Non-Holo / Cosmos Holo" names both, and stays the ambiguity it is.
	if describesPlain(wording) && strings.Count(strings.ToLower(wording), "holo") > 1 {
		return candidates
	}
	var sold []matcher.Card
	for _, card := range candidates {
		plain := slices.Contains(card.Finishes, matcher.FinishNonfoil)
		holo := matcher.NamedFinish(card.FoilUUIDs, "", "Holofoil") != ""
		if (describesPlain(wording) && plain) || (describesHolo(wording) && holo) {
			sold = append(sold, card)
		}
	}
	if len(sold) > 0 {
		return sold
	}
	return candidates
}

// tierByYear narrows candidates sharing a number and label by the bare year
// the wording carries, the league energies' only way of saying which year's
// copy they price - the catalog gives every year's copy the same name,
// number and label, and only their release date tells them apart. It reads
// Card.OriginalReleaseDate rather than the set's own release date: PR-1539
// "League & Championship Cards" is dated 2016 and would agree with none of
// them.
func tierByYear(inCard *matcher.InputCard, wording string, candidates []matcher.Card) []matcher.Card {
	if len(candidates) <= 1 {
		return candidates
	}
	sold := matcher.FinishNonfoil
	if !describesPlain(wording) && (strings.Contains(wording, "holo") || inCard.Foil) {
		sold = matcher.FinishFoil
	}
	var byYear []matcher.Card
	for field := range strings.FieldsSeq(wording) {
		if !bareYearRe.MatchString(field) {
			continue
		}
		year := strings.TrimPrefix(field, "©")
		for _, card := range candidates {
			if slices.Contains(card.Finishes, sold) && strings.HasPrefix(card.OriginalReleaseDate, year) {
				byYear = append(byYear, card)
			}
		}
		break
	}
	if len(byYear) > 0 {
		return byYear
	}
	return candidates
}

// tierByQualifier keeps the candidates whose product name TCGplayer
// qualifies with exactly the listing's Variation, the spelling the loader
// indexes as "Name (qualifier)": "2017" is what tells "Psychic Energy (2017)"
// from the set's numbered secret rare. Nothing is narrowed unless a
// candidate carries that exact spelling, so a qualifier left in the name or
// spelled another way ("2017 Unnumbered") does not fire it.
func tierByQualifier(b *matcher.Backend, inCard *matcher.InputCard, candidates []matcher.Card) []matcher.Card {
	if len(candidates) <= 1 || inCard.Variation == "" {
		return candidates
	}
	uuids := b.Hashes[matcher.Normalize(inCard.Name+" ("+inCard.Variation+")")]
	var named []matcher.Card
	for _, card := range candidates {
		for _, uuid := range card.FoilUUIDs {
			if slices.Contains(uuids, uuid) {
				named = append(named, card)
				break
			}
		}
	}
	if len(named) == 0 {
		return candidates
	}
	return named
}

// wearingStamp keeps the candidates wearing a stamp label.
func wearingStamp(candidates []matcher.Card) []matcher.Card {
	var stamped []matcher.Card
	for _, card := range candidates {
		for _, promoType := range card.PromoTypes {
			if strings.Contains(promoType, "stamp") {
				stamped = append(stamped, card)
				break
			}
		}
	}
	return stamped
}

// unnumberedTwins keeps the candidates carrying no number where the rest
// carry one and every one of them wears the same mark, and nothing
// otherwise: the twins are one card the catalog files twice.
func unnumberedTwins(candidates []matcher.Card) []matcher.Card {
	var bare []matcher.Card
	for _, card := range candidates {
		if card.Watermark == "" || card.Watermark != candidates[0].Watermark {
			return nil
		}
		if card.Number == "" {
			bare = append(bare, card)
		}
	}
	if len(bare) == len(candidates) {
		return nil
	}
	return bare
}

// spelledNumberRe matches the placements a storefront spells out where
// the labels carry figures: "Top Thirty-Two" is the top32 card.
var spelledNumberRe = regexp.MustCompile(`(?i)\b(thirty[- ]two|sixteen|eight|four)\b`)

var spelledNumbers = map[string]string{
	"thirty-two": "32", "thirty two": "32", "sixteen": "16", "eight": "8", "four": "4",
}

// accents folds the one accent a storefront writes into a label's words:
// the catalog spells the Pokemon Center without it.
var accents = strings.NewReplacer("é", "e", "É", "E")

// labelWording is the variation read the way the labels are written, in
// lower case, without accents, and with its spelled-out placements as
// figures.
func labelWording(variation string) string {
	variation = accents.Replace(strings.ToLower(variation))
	return spelledNumberRe.ReplaceAllStringFunc(variation, func(word string) string {
		return spelledNumbers[word]
	})
}

// wordNamed reports whether a word of a label or a mark is in the wording,
// a plural counting as its singular: the catalog writes "Regional
// Championships" where a storefront writes the one it is pricing.
func wordNamed(words []string, word string) bool {
	word = strings.TrimSuffix(matcher.PromoTypeSlug(word), "s")
	for _, candidate := range words {
		candidate = strings.TrimSuffix(matcher.PromoTypeSlug(candidate), "s")
		// A word of some length is named by a word it opens: the catalog
		// writes "Cosmos Holo" where the storefront writes "Cosmos
		// Holofoil". Short words open too many others to be read that way.
		if candidate == word || (len(word) >= 4 && strings.HasPrefix(candidate, word)) {
			return true
		}
	}
	return false
}

// labelNamed reports whether a wording names a label: by its token joined
// up the way SlugDescribes reads it, or word by word in the label's own
// order with other words between - "WotC 2002 League Promo" names the
// WotC League Promo, and only the words of a label of more than one word
// are read that way, since a lone word names too much.
func labelNamed(b *matcher.Backend, wording, promoType string) bool {
	if matcher.SlugDescribes(wording, promoType) {
		return true
	}
	label := strings.Fields(b.PromoTypeLabels[promoType])
	if len(label) < 2 {
		return false
	}
	words := strings.Fields(wording)
	for _, word := range label {
		var at int
		for at < len(words) && !wordNamed(words[at:at+1], word) {
			at++
		}
		if at == len(words) {
			return false
		}
		words = words[at+1:]
	}
	return true
}

// setStopWords are the words a Pokemon set name shares with too many others
// to tell one from another.
var setStopWords = map[string]bool{
	"promo": true, "promos": true, "card": true, "cards": true,
	"set": true, "sets": true, "the": true, "and": true,
	"products": true, "collection": true,
}

// The marks an old card wears in its name, read the way the catalog keeps
// each: it writes the star and the prism star into the name it files, and
// files the delta species beside the name instead, as the variant the
// printing carries. They are read only once the name has failed to name a
// card, so the one printing the catalog itself spells with a symbol is
// never rewritten.
const (
	deltaMark    = "δ"
	deltaSpecies = "Delta Species"
)

var cardMarks = strings.NewReplacer(
	"◇", "Prism Star",
	"★", "Star",
)

// altArtSpellings are the ways a storefront writes the treatment the catalog
// files a whole set of, so the set's own name can be looked for in a wording
// that abbreviates it.
var altArtSpellings = strings.NewReplacer("alt art", "alternate art")

// setNamed keeps the printings whose set the wording names, by the words of
// that set's name which are its own.
//
// A set name shared down to nothing - "Miscellaneous Cards & Products" is
// every word a stop word but one - cannot be named by accident this way,
// and a wording naming none of the sets keeps them all, which the caller
// reads as the wording having failed to choose.
func setNamed(b *matcher.Backend, inCard *matcher.InputCard, cards []matcher.Card) []matcher.Card {
	said := map[string]bool{}
	wording := altArtSpellings.Replace(strings.ToLower(inCard.Variation + " " + inCard.Edition))
	for field := range strings.FieldsSeq(wording) {
		if word := matcher.PromoTypeSlug(field); word != "" {
			said[word] = true
		}
	}
	var out []matcher.Card
	for _, card := range cards {
		set, found := b.Sets[card.SetCode]
		if !found {
			continue
		}
		whole, own := true, false
		for field := range strings.FieldsSeq(strings.ToLower(set.Name)) {
			word := matcher.PromoTypeSlug(field)
			if word == "" || setStopWords[word] {
				continue
			}
			own = true
			if !said[word] {
				whole = false
				break
			}
		}
		if own && whole {
			out = append(out, card)
		}
	}
	return out
}

// withProductID keeps the printings that carry a TCGplayer product id.
func withProductID(cards []matcher.Card) []matcher.Card {
	var out []matcher.Card
	for _, card := range cards {
		if card.Identifiers["tcgplayerProductId"] != "" {
			out = append(out, card)
		}
	}
	return out
}

// letteredNumberRe matches a collector number carrying the letter a promo
// printing is told apart by, "51a" and "182b" and "SM30a".
var letteredNumberRe = regexp.MustCompile(`(?i)^[A-Z]{0,4}[0-9]+[a-z]$`)

// letteredPromo collects the printings of this card that carry a lettered
// number the edition admits none of, wherever the catalog files them.
//
// The letter is the storefront's own marker and the catalog's alike: the
// alternate art promos are numbered for the set they reprint and lettered
// apart from it, so "51a" names one printing in the whole catalog where
// "51" names one per set. Reading it across sets is safe for exactly that
// reason, and only where the edition reached nothing - a lettered number
// the edition does admit has already answered.
//
// Nothing is stripped here. Where the letter names more than one printing
// the wording has not chosen, and the caller falls through to the tiers
// that read the rest of it.
func letteredPromo(b *matcher.Backend, inCard *matcher.InputCard, numbers []string) []matcher.Card {
	var out []matcher.Card
	seen := map[string]bool{}
	for _, number := range numbers {
		if !letteredNumberRe.MatchString(number) {
			continue
		}
		for _, uuid := range b.Hashes[matcher.Normalize(inCard.Name)] {
			co, found := b.UUIDs[uuid]
			if !found || co.Sealed || !numberMatches(number, co.Number) {
				continue
			}
			key := productKeyOf(co.Card.Identifiers, uuid)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, co.Card)
		}
	}
	return out
}

// filterByNumber collects the printings of the input's name that the edition
// admits and that carry the given collector number, one per product. An
// empty number asks for every printing the edition admits.
func filterByNumber(b *matcher.Backend, inCard *matcher.InputCard, sets []matcher.CandidateSet, number string) []matcher.Card {
	var candidates []matcher.Card
	// The sizes the edition's shelf can lend a listing, read once and only
	// where a total has to be told from the shelf's.
	var figures map[string]bool
	var cards []matcher.Card
	for _, set := range sets {
		cards = append(cards, set.Cards...)
	}
	for _, card := range cards {
		if number != "" && !numberMatchesCard(b, number, &card) {
			continue
		}
		if totalDisagrees(inCard.Variation, printedFace(&card)) {
			if figures == nil {
				figures = shelfFigures(b, sets)
			}
			if !shelfTotal(inCard.Variation, &card, figures) {
				continue
			}
		}
		// An input naming a treatment re-keys the copy's FoilUUIDs so the
		// flag-driven resolution downstream lands on that printing. Both
		// slots move together: the input named the printing outright, and
		// a storefront's foil flag is the less reliable of the two.
		uuid := finishUUID(b, inCard, &card)
		if uuid != "" {
			foilUUIDs := make(map[string]string, len(card.FoilUUIDs))
			maps.Copy(foilUUIDs, card.FoilUUIDs)
			foilUUIDs[matcher.FinishNonfoil] = uuid
			foilUUIDs[matcher.FinishFoil] = uuid
			card.FoilUUIDs = foilUUIDs
		}
		candidates = append(candidates, card)
	}
	return candidates
}

// tierByMark keeps the candidates whose mark the wording names - the player
// whose World Championship deck a card came in, the theme deck a promo was
// packed in, the set it was reprinted from, the place a Battle Academy stamp
// gives it, the blister a code card was sold on. A mark says which copy of a
// number a printing is rather than what promoted it, so it is not among the
// promo types and narrows on its own and before them.
//
// Where the wording names no mark the unmarked printings are kept, the way
// tierByLabel keeps the plain ones over the labelled: a listing that says
// nothing about which copy it prices means the copy that needs nothing said.
// Narrowing to a mark the listing never mentioned would answer with a copy
// picked at random, and a number whose printings are all marked, or none of
// them, is left to the tiers below exactly as before.
func tierByMark(wording string, candidates []matcher.Card) []matcher.Card {
	if marked := marksNamed(wording, candidates); len(marked) > 0 {
		return marked
	}
	var unmarked []matcher.Card
	for _, card := range candidates {
		if card.Watermark == "" {
			unmarked = append(unmarked, card)
		}
	}
	if len(unmarked) == len(candidates) {
		return nil
	}
	return unmarked
}

// marksNamed keeps the candidates whose mark the wording names, and among
// them the ones named most fully: a mark contained in a longer one -
// "Zacian V" in "Zacian V International Version" - is named by every
// wording that names the longer.
func marksNamed(wording string, candidates []matcher.Card) []matcher.Card {
	var marked []matcher.Card
	var most int
	for _, card := range candidates {
		if card.Watermark == "" {
			continue
		}
		words := strings.Fields(card.Watermark)
		if !markNamed(wording, words) {
			continue
		}
		if len(words) > most {
			most, marked = len(words), nil
		}
		if len(words) == most {
			marked = append(marked, card)
		}
	}
	return marked
}

// intersection keeps the cards of the first list that the second holds too.
func intersection(a, b []matcher.Card) []matcher.Card {
	var both []matcher.Card
	for _, card := range a {
		for _, other := range b {
			if card.UUID == other.UUID {
				both = append(both, card)
				break
			}
		}
	}
	return both
}

// markNamed reports whether a wording names a mark: joined up the way
// SlugDescribes reads a token, or, for a mark of several words, by every
// one of its words in any order - the storefront writes the tin behind
// the version where the catalog writes it before.
func markNamed(wording string, mark []string) bool {
	if matcher.SlugDescribes(wording, matcher.PromoTypeSlug(strings.Join(mark, ""))) {
		return true
	}
	if len(mark) < 2 {
		return false
	}
	words := strings.Fields(wording)
	for _, word := range mark {
		if !wordNamed(words, word) {
			return false
		}
	}
	return true
}

// tierByLabel splits the candidates into the ones whose label the wording
// describes, the base printings, and the labelled printings. Only the
// variation is consulted: set names carry the label words themselves
// ("Team Plasma" is a label and part of three set names).
func tierByLabel(b *matcher.Backend, wording string, candidates []matcher.Card) []matcher.Card {
	described, base, labelled := labelReading(b, wording, candidates)
	if len(described) > 0 {
		// A wording can spell two treatments at once: CardTrader files the
		// plain and the Cosmos Holo printing of a number under one
		// blueprint and writes "Non-Holo / Cosmos Holo" for both. The
		// listing does not say which it prices, so both stand and the
		// ambiguity surfaces rather than every copy repricing the holo.
		if len(base) > 0 && describesPlain(wording) {
			return append(base, described...)
		}
		return described
	}
	if len(base) > 0 {
		return base
	}
	if len(labelled) == 0 {
		return labelled
	}
	// Every candidate carries a label, which is what a promo run looks like
	// once the run's own name is a label rather than part of the card's:
	// "Archeops (Prerelease)" carries "prerelease" and a "Prerelease Staff"
	// beside it carries two. The least decorated is the one a wording
	// naming none of them means, and a tie at the fewest still surfaces
	// rather than guessing between them.
	fewest := len(labelled[0].PromoTypes)
	for _, card := range labelled {
		if len(card.PromoTypes) < fewest {
			fewest = len(card.PromoTypes)
		}
	}
	var plainest []matcher.Card
	for _, card := range labelled {
		if len(card.PromoTypes) == fewest {
			plainest = append(plainest, card)
		}
	}
	// A placement is never named by accident, so a wording naming none
	// means the copy that wears none: "Toxtricity - 017" beside its Staff.
	var unplaced []matcher.Card
	for _, card := range plainest {
		if !wearsAny([]matcher.Card{card}, placements) {
			unplaced = append(unplaced, card)
		}
	}
	if len(unplaced) > 0 {
		return unplaced
	}
	return plainest
}

// labelReading sorts the candidates by what the wording says of their
// labels: the ones it names, most fully first, the base printings wearing
// none, and every labelled one.
func labelReading(b *matcher.Backend, wording string, candidates []matcher.Card) (described, base, labelled []matcher.Card) {
	var most [4]int
	for _, card := range candidates {
		if len(card.PromoTypes) == 0 {
			base = append(base, card)
			continue
		}
		labelled = append(labelled, card)
		// A label is weighed by how much of it the wording spells, so the
		// one a wording names whole outranks the one it names inside it:
		// "Quarter Finalist" names the quarter-finalist and, with its last
		// word alone, the finalist too.
		var named, opened, spelled int
		for _, promoType := range card.PromoTypes {
			switch {
			case labelNamed(b, wording, promoType):
				named++
				spelled += len(promoType)
			case labelOpened(b, wording, promoType):
				opened++
				// A label opened with two words or more is as good as
				// named: "Pokemon Center" is the Pokemon Center Exclusive,
				// where "Pokemon" alone opens half the labels there are.
				if opened := labelOpening(b, wording, promoType); strings.Count(opened, " ") >= 1 {
					spelled += len(matcher.PromoTypeSlug(opened))
				}
			}
		}
		if named == 0 && spelled == 0 {
			continue
		}
		// A printing sharing a label with its siblings is told from them by
		// the labels it does not share, so the candidate the wording names
		// most fully wins outright: the retailer stampings of one promo all
		// carry "cosmos holo" and differ only in the retailer.
		// Naming a label outright outranks naming what one opens with, and
		// a printing wearing nothing the wording left unsaid outranks one
		// carrying a label besides - the plain prerelease against the staff
		// one, both of which a wording saying "Prerelease" names once.
		depth := [4]int{spelled, named, opened, named + opened - len(card.PromoTypes)}
		if depth != most && deeper(depth, most) {
			most, described = depth, nil
		}
		if depth == most {
			described = append(described, card)
		}
	}
	return described, base, labelled
}

// deeper reports whether one reading of a printing's labels says more about
// it than another, weighing the counts in the order they are worth.
func deeper(depth, than [4]int) bool {
	for i := range depth {
		if depth[i] != than[i] {
			return depth[i] > than[i]
		}
	}
	return false
}

// labelOpened reports whether a wording names the words a label opens with
// but not the whole of it. The catalog closes a retailer's label with what
// the retailer's copies are - "gamestop exclusive" - where the storefront
// names the retailer alone, and the shared tail is exactly what makes the
// retailers' labels alike.
func labelOpened(b *matcher.Backend, wording, promoType string) bool {
	return labelOpening(b, wording, promoType) != ""
}

// labelOpening is the longest run of words a label opens with that the
// wording names, or nothing.
func labelOpening(b *matcher.Backend, wording, promoType string) string {
	words := strings.Fields(b.PromoTypeLabels[promoType])
	for i := len(words) - 1; i > 0; i-- {
		if matcher.SlugDescribes(wording, matcher.PromoTypeSlug(strings.Join(words[:i], ""))) {
			return strings.Join(words[:i], " ")
		}
	}
	return ""
}

// describesPlain reports whether the wording names the untreated printing:
// the "Non-Holo" the storefronts write it with.
func describesPlain(variation string) bool {
	variation = strings.ToLower(variation)
	return strings.Contains(variation, "non-holo") || strings.Contains(variation, "non holo")
}

// finishUUID resolves the entry an input's finish names, from the finish
// field where a caller fills one in and from the variation wording where
// only the listing speaks.
func finishUUID(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) string {
	if uuid := b.FinishUUID(card, inCard.Finish); uuid != "" {
		return uuid
	}
	return card.FoilUUIDs[selectFinish(inCard, card)]
}

// selectFinish maps the input wording's treatment tokens onto one of the
// card's stored printings, so a listing spelling its printing out ("Reverse
// Holo", "1st Edition") resolves to that entry instead of the default one.
//
// The wording names axes rather than a printing: it can say a run, a
// treatment, or both, and the printing that answers is the plainest one sold
// in everything the wording asked for (matcher.NamedFinish). That is what
// lets "1st Edition" reach the 1st Edition Holofoil on a card sold in no
// other first-edition printing, while still preferring the plain 1st Edition
// on a card that has one.
//
// Only the variation speaks: set names carry the same words ("Unlimited" and
// "1st Edition" name the base-set reprints). A wording naming no axis, or a
// printing the product was not priced in, keeps the flag-driven default.
func selectFinish(inCard *matcher.InputCard, card *matcher.Card) string {
	words := strings.Fields(strings.ToLower(inCard.Variation))

	var run, treatment string
	if hasAllTokens(words, []string{"1st"}) || hasAllTokens(words, []string{"first"}) {
		run = matcher.Run1stEdition
	}
	if hasAllTokens(words, []string{"unlimited"}) {
		run = matcher.RunUnlimited
	}
	if hasAllTokens(words, []string{"holo"}) {
		treatment = "Holofoil"
	}
	if hasAllTokens(words, []string{"reverse"}) {
		treatment = "Reverse Holofoil"
	}
	if run == "" && treatment == "" {
		return ""
	}
	return matcher.NamedFinish(card.FoilUUIDs, run, treatment)
}

// hasAllTokens reports whether every token is carried by some word of the
// wording. A token matches a word it opens, so "holo" is found in "Holofoil"
// and "1st" in "1st".
func hasAllTokens(words, tokens []string) bool {
	for _, token := range tokens {
		found := false
		for _, word := range words {
			if strings.HasPrefix(word, token) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// PlainNumber implements matcher.GameRules. The catalog pads an ordinal
// out to three digits, and a person writes card 1 as "1", not "001".
func (Rules) PlainNumber(number string) string {
	return plainNumber(number)
}

// extractNumbers pulls the collector numbers out of a storefront's variation
// wording, which mixes them with treatment words and set totals. Every token
// that looks like one is returned in the order it was written, because a
// wording carrying two numbers does not say which of them is the card's.
func extractNumbers(variation string) []string {
	var numbers []string
	for field := range strings.FieldsSeq(variation) {
		// A year is not a collector number: the league energies are
		// unnumbered and labelled by the year they were handed out, and
		// a wording saying "2006 Non-Holo Promo" names the label.
		if bareYearRe.MatchString(field) {
			continue
		}
		// Card Trader's own code for a league energy, three dashed groups
		// of three ("FFE-9JT-SUX"), is not a number either: read as one, it
		// hands filterByNumber the number "FFE" and every candidate the
		// year alone would have reached is lost.
		if dashedCodeRe.MatchString(field) {
			continue
		}
		if m := letterNumberRe.FindStringSubmatch(field); m != nil {
			numbers = append(numbers, m[1])
			continue
		}
		if m := fullNumberRe.FindStringSubmatch(field); m != nil {
			numbers = append(numbers, m[1])
		}
	}
	return numbers
}

// letterNumberRe matches a collector number whose numerator is letters
// alone, written over its total: the twenty-eight Unown of EX Unseen Forces
// are "A/28" through "Z/28", "!/28" and "?/28", and the Alph Lithographs
// "ONE/123" through "FOUR/95". Read by the shape above, the digits behind
// the slash were the number and the card was never found - the face itself
// answered "unknown variant". The total is required, since a bare letter
// beside a name is a label ("Unown (Z)") and reads as one.
var letterNumberRe = regexp.MustCompile(`^([A-Z]{1,5}|[!?])/\d+$`)

// bareYearRe matches a field that is nothing but a year, optionally marked
// with the copyright sign Card Trader's Champion's Path energies carry it
// behind ("©2020"); no collector number is a digit short of one.
var bareYearRe = regexp.MustCompile(`^©?(?:19|20)[0-9]{2}$`)

// dashedCodeRe matches Card Trader's own tracking code for a league energy,
// three dashed groups of three ("LX4-T8B-BH1", "SPN-3D2-6AM"): no card of
// ours is numbered that way, but its first group alone is a number shape
// fullNumberRe would otherwise read as the card's.
var dashedCodeRe = regexp.MustCompile(`(?i)^[A-Z0-9]{3}-[A-Z0-9]{3}-[A-Z0-9]{3}$`)

// numberMatches compares a storefront's collector number against the
// catalog's, which carries the set total the storefront usually drops
// ("001/102" against "001", and either against "1").
func numberMatches(input, number string) bool {
	if number == "" {
		return false
	}
	return foldNumber(input) == foldNumber(number)
}

// totalDisagrees reports whether the wording spells the card's own
// numerator with a set total that contradicts the card's. The total names
// the set as surely as the number names the card - "Non-Holo | 053/167" is
// the 167-card set's 53, and the fold that drops it is what let that
// wording price the 53/094 of another collection. Only agreement between
// two spelled-out totals can clear a contradiction, and a wording spelling
// the numerator with no total at all vetoes nothing.
func totalDisagrees(variation, number string) bool {
	numerator, total, found := strings.Cut(number, "/")
	if !found {
		return false
	}
	want := foldTotal(total)
	if want == "" {
		return false
	}
	saw := false
	for _, field := range strings.Fields(variation) {
		fieldNumerator, fieldTotal, cut := strings.Cut(field, "/")
		if !cut || foldNumber(fieldNumerator) != foldNumber(numerator) {
			continue
		}
		got := foldTotal(fieldTotal)
		if got == "" {
			continue
		}
		if got == want {
			return false
		}
		saw = true
	}
	return saw
}

// yearlessRe is the year a set of a yearly run is named with, in front of
// the run's name: "2015 World Championship Decks".
var yearlessRe = regexp.MustCompile(`^(?:19|20)\d{2} `)

// shelfTotal reports whether the total the wording spells for the card's
// numerator is the shelf's figure rather than the card's. A storefront that
// composes number/size from the set it files a card under is right wherever
// the shelf is the printing's own set and wrong wherever the set pools
// cards from elsewhere: Unown prints Z/28 and is sold as Z/115 under EX
// Unseen Forces, and every Celebrations Classic Collection card keeps its
// original set's total against a shelf of 25. Such a total names the shelf
// the listing already names in its edition, so it contradicts nothing -
// where a total the shelf could not have produced still vetoes, since that
// one names another set's printing.
func shelfTotal(variation string, card *matcher.Card, figures map[string]bool) bool {
	numerator, _, _ := strings.Cut(printedFace(card), "/")
	for _, field := range strings.Fields(variation) {
		fieldNumerator, fieldTotal, cut := strings.Cut(field, "/")
		if !cut || foldNumber(fieldNumerator) != foldNumber(numerator) {
			continue
		}
		if got := foldTotal(fieldTotal); got != "" && figures[got] {
			return true
		}
	}
	return false
}

// shelfFigures are the sizes a storefront can read off the shelf a listing
// names to write behind a slash: the base size the datastore states for a
// set the edition admits, the total most cards on the shelf print, and how
// many cards the shelf holds. Game Nerdz writes the second for EX Unseen
// Forces (115) and the third for the Classic Collection (25).
//
// The shelf is every set the edition admits, read together with the years
// of the same run. The catalog files all the World Championship Decks under
// one group where the datastore keeps a set per year, and the year in a
// listing's name narrows the edition to its own year before this runs; a
// storefront with the one shelf derives its figure from the whole run - 100,
// the total most of the decks' cards print - which no single year's figures
// would recognise. A set named as another with a year in front is a year of
// it, and the run is the only such family the datastore holds.
func shelfFigures(b *matcher.Backend, sets []matcher.CandidateSet) map[string]bool {
	out := map[string]bool{}
	totals := map[string]int{}
	products := map[string]bool{}
	shelf := map[string]bool{}
	for _, admitted := range sets {
		code := admitted.Code
		set := b.Sets[code]
		if set == nil {
			continue
		}
		// A subset is admitted beside its parent, but a shelf's figures
		// are its parent's own
		parent, found := parentOf(b, code)
		if found && slices.ContainsFunc(sets, func(s matcher.CandidateSet) bool { return s.Code == parent }) {
			continue
		}
		shelf[code] = true
		run := yearlessRe.ReplaceAllString(set.Name, "")
		for other, candidate := range b.Sets {
			if yearlessRe.ReplaceAllString(candidate.Name, "") == run {
				shelf[other] = true
			}
		}
	}
	for code := range shelf {
		set := b.Sets[code]
		if set.BaseSetSize > 0 {
			out[strconv.Itoa(set.BaseSetSize)] = true
		}
		for i := range set.Cards {
			card := &set.Cards[i]
			products[productKeyOf(card.Identifiers, card.UUID)] = true
			if total := foldTotal(card.SetTotal); total != "" {
				totals[total]++
			}
		}
	}
	out[strconv.Itoa(len(products))] = true
	var dominant string
	var most int
	for total, n := range totals {
		if n > most || n == most && total < dominant {
			dominant, most = total, n
		}
	}
	if dominant != "" {
		out[dominant] = true
	}
	return out
}

// foldTotal reduces a set total to its digits, dropping the zero padding
// and whatever trails the digit run ("017" and "17h" both fold to "17").
// A total that does not open with a digit folds away entirely.
func foldTotal(total string) string {
	end := 0
	for end < len(total) && total[end] >= '0' && total[end] <= '9' {
		end++
	}
	return strings.TrimLeft(total[:end], "0")
}

// fullNumberMatches compares a storefront's collector number against the
// catalog's with the set total kept, which is what the ordinary fold drops.
// Both sides have to carry a total for the question to mean anything.
func fullNumberMatches(variation, number string) bool {
	numerator, total, found := strings.Cut(number, "/")
	if !found {
		return false
	}
	want := foldNumber(numerator) + "/" + strings.TrimLeft(total, "0")
	for field := range strings.FieldsSeq(variation) {
		numerator, total, found := strings.Cut(field, "/")
		if !found {
			continue
		}
		got := foldNumber(numerator) + "/" + strings.TrimLeft(strings.TrimRight(total, " ,."), "0")
		if strings.EqualFold(got, want) {
			return true
		}
	}
	return false
}

// printedFace is the collector number as the card's face prints it, total
// and all: the number and the set total the loader keeps apart, rejoined
// for the three comparisons that read the total. A card printing no total
// answers with its bare number, and that is the whole of the difference
// the verbatim tier reads - a bare-numbered promo is not the printing its
// totalled reprint is.
func printedFace(card *matcher.Card) string {
	if card.SetTotal == "" {
		return card.Number
	}
	return card.Number + "/" + card.SetTotal
}

// numbersMatchCard reports whether any of the collector numbers a wording
// carries names the card's.
func numbersMatchCard(b *matcher.Backend, numbers []string, card *matcher.Card) bool {
	for _, number := range numbers {
		if numberMatchesCard(b, number, card) {
			return true
		}
	}
	return false
}

// numberMatchesCard compares a storefront's collector number against one
// card's, which is where the set the number belongs to is known.
func numberMatchesCard(b *matcher.Backend, input string, card *matcher.Card) bool {
	return numberMatches(input, card.Number) || numberMatchesPrefixed(b, input, card)
}

// numberMatchesPrefixed accepts the set code a storefront glues onto a
// number the catalog writes bare: the promo sets are numbered "001".."282"
// and the storefronts write "SVP001", "MEP003". The prefix has to be the
// set's own code and it has to stand in front of the whole number, so this
// can only ever admit the set the storefront already named - stripping
// letters freely would read "XY144" as the 144 of every set there is.
func numberMatchesPrefixed(b *matcher.Backend, input string, card *matcher.Card) bool {
	folded := foldNumber(card.Number)
	if folded == "" || strings.ContainsFunc(folded, func(r rune) bool { return r < '0' || r > '9' }) {
		return false
	}
	in := foldNumber(input)
	digits := strings.IndexFunc(in, func(r rune) bool { return r >= '0' && r <= '9' })
	if digits <= 0 || in[digits:] != folded {
		return false
	}
	set := b.Sets[card.SetCode]
	return set != nil && strings.EqualFold(in[:digits], set.Code)
}

// foldNumber reduces a collector number to the letters and digits that carry
// it, dropping the set total and the zeros each digit run is padded with.
// The two Unown that are numbered "!" and "?" are carried by those marks.
func foldNumber(number string) string {
	number = strings.Split(number, "/")[0]
	var out strings.Builder
	digits := false
	for _, r := range strings.ToLower(number) {
		switch {
		case r >= '0' && r <= '9':
			if !digits && r == '0' {
				continue
			}
			digits = true
			out.WriteRune(r)
		case r >= 'a' && r <= 'z', r == '!', r == '?':
			digits = false
			out.WriteRune(r)
		}
	}
	return out.String()
}
