package fleshandblood

import (
	"cmp"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// Rules implements matcher.GameRules for Flesh and Blood. A card is
// identified by name + collector number, with the variant label separating
// the printings that share a number (extended arts, marvels and the like).
// Finish never gates anything: every print run and treatment of a product
// matches, resolving to the default entry of the requested foilness - but a
// named finish routes the match onto the entry it names, whether it arrives
// in the finish field or as the words storefronts append ("Rainbow Foil",
// "Cold Foil", the print-run edition suffixes).
type Rules struct {
	matcher.DefaultRules

	// The fused cards, indexed once by Load because the printings do not
	// change afterwards: their names by faceKey, and their printings by
	// each normalized face.
	fusedByFaces map[string][]string
	fusedByFace  map[string][]fusedFace
}

// fusedFace is a fused printing seen from one of its faces: the card's name
// and pair number, and the half of the pair that numbers the face.
type fusedFace struct {
	name, number, half string
}

// newRules returns the rules with the fused cards of b indexed.
func newRules(b *matcher.Backend) Rules {
	r := Rules{
		fusedByFaces: map[string][]string{},
		fusedByFace:  map[string][]fusedFace{},
	}
	for _, co := range b.UUIDs {
		if co.Sealed || !strings.Contains(co.Name, "//") {
			continue
		}
		key := faceKey(co.Name)
		if !slices.Contains(r.fusedByFaces[key], co.Name) {
			r.fusedByFaces[key] = append(r.fusedByFaces[key], co.Name)
		}

		// The pair is read on one slash as well as two, the way
		// numberMatches and sortedPair read it.
		if !strings.Contains(co.Number, "/") {
			continue
		}
		faces := strings.Split(co.Name, "//")
		pair := strings.Split(strings.ReplaceAll(co.Number, "//", "/"), "/")
		if len(faces) != len(pair) {
			continue
		}
		for i := range faces {
			face := matcher.Normalize(strings.TrimSpace(faces[i]))
			entry := fusedFace{co.Name, co.Number, strings.TrimSpace(pair[i])}
			if !slices.Contains(r.fusedByFace[face], entry) {
				r.fusedByFace[face] = append(r.fusedByFace[face], entry)
			}
		}
	}
	for key, names := range r.fusedByFaces {
		slices.Sort(names)
		r.fusedByFaces[key] = slices.Clip(names)
	}
	for _, entries := range r.fusedByFace {
		slices.SortFunc(entries, func(a, b fusedFace) int {
			return cmp.Or(strings.Compare(a.name, b.name), strings.Compare(a.number, b.number))
		})
	}
	return r
}

// fullNumberRe matches the game's collector number shapes: "WTR215",
// "1HP408", with an optional letter tail (cardtrader suffixes marvels
// "MST238m"). The dashed numbers ("MST158-A") are left to the variant
// wording, which carries the same tail.
var fullNumberRe = regexp.MustCompile(`^[0-9]?[A-Za-z]+[0-9]{1,4}[a-zA-Z]{0,2}$`)

// pairNumberRe matches the fused-card numbers ("WTR040 // WTR039",
// cardtrader's compact "UPR002//UPR165") so the pair survives extraction
// whole instead of field splitting cutting it at the separator.
var pairNumberRe = regexp.MustCompile(`[0-9]?[A-Za-z]+[0-9]{1,4}[a-zA-Z]?(?:\s*/{1,2}\s*[0-9]?[A-Za-z]+[0-9]{1,4}[a-zA-Z]?)+`)

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// Prefilter splits the parenthetical decorations off the name before the
// canonical-name lookup: storefronts write "Enigma, New Moon (Marvel)" for
// a variant the datastore labels beside the plain name. A full name that is
// itself canonical stays as it is - the pitch-color parentheticals ("Sink
// Below (Red)") are part of the name.
func (r Rules) Prefilter(b *matcher.Backend, inCard *matcher.InputCard) {
	inCard.Name = dashQualifierRe.ReplaceAllString(inCard.Name, " ($1)")
	r.adjustQualifier(b, inCard)
	r.adjustFusedName(b, inCard)
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; found {
		return
	}
	if name := regrownName(b, inCard); name != "" {
		inCard.Name = name
		return
	}
	if strings.Contains(inCard.Name, "(") {
		vars := matcher.SplitVariants(inCard.Name)
		if len(vars) > 1 {
			inCard.Name = vars[0]
			inCard.AddToVariant(strings.Join(vars[1:], " "))
		}
	}
}

// qualifierRe matches the parenthetical a Flesh and Blood name is qualified
// with. There are only four in the game: the three pitch colors and the
// marvel treatment.
var qualifierRe = regexp.MustCompile(`\s*\((?i:Red|Yellow|Blue|Marvel)\)$`)

// dashQualifierRe matches the same qualifier spelled with a dash, the way
// cardtrader writes it ("Impenetrable Belief - Red"). Normalize folds the
// dashed spelling onto the parenthesised one, so the canonical-name lookup
// accepts the dash and nothing downstream ever sees a qualifier at all:
// adjustQualifier cannot respell what it cannot read, and the listing dies
// at the number filter instead. No card in the game carries " - " in its
// name, so the rewrite reaches nothing but this.
var dashQualifierRe = regexp.MustCompile(`\s+-\s+((?i:Red|Yellow|Blue|Marvel))$`)

// pitches are the pitch spellings a qualified name is tried in, the empty
// one standing for the undecorated name, and marvelLabel is the treatment
// label any of them can grow: the datastore spells a marvel of a pitched
// card "Runechant of Greed (Yellow) (Marvel)".
var pitches = []string{"", " (Red)", " (Yellow)", " (Blue)"}

const marvelLabel = " (Marvel)"

// adjustQualifier re-spells the name's qualifier as the printing the input's
// collector number names spells it.
//
// The datastore mirrors the storefront each set was catalogued from, and the
// storefronts do not agree with each other: the same card is "Hyper Driver" at
// ARC036 and "Hyper Driver (Red)" at DYN110, and "Bloodrot Trap" at ARA019 but
// "Bloodrot Trap (Red)" at OUT171. Whichever spelling a feed sends, it is a
// canonical name of the other set's printing, so the name lookup succeeds, the
// number then matches nothing under it, and the printing the feed actually
// named is unreachable - in both directions.
//
// The collector number is what makes the re-spelling safe, and it has to be a
// full one: a pitch color is part of the name, so "Sink Below (Red)" and "Sink
// Below (Blue)" are two different cards, and only a number carrying its set
// code names one of them. A bare "036" is every set's thirty-sixth card and
// must never license a rename. An input whose own spelling has a printing at
// the number is already right, and an ambiguous one is left alone. The
// parenthetical the re-spelling drops is handed to the wording, since the same
// shape also spells a treatment label rather than a piece of the name.
func (r Rules) adjustQualifier(b *matcher.Backend, inCard *matcher.InputCard) {
	number := labelledNumber(inCard.Variation)
	if !fullNumberRe.MatchString(number) && !pairNumberRe.MatchString(number) &&
		!dashedNumberRe.MatchString(number) {
		return
	}
	// A spelling numbered as the input is the input's own only while a
	// printing under it is sold in the finish the input names: the plain
	// "Florian, Rotwood Harbinger" is ROS001 and sold plain, and a listing
	// demanding its cold foil names the "(Marvel)" spelled beside it, which
	// is the only ROS001 sold cold.
	if numberedAs(b, inCard.Name, number) && soldAs(b, inCard.Name, number, inCard.Finish) {
		return
	}
	// A face of a fused card outranks a treatment label: the plain
	// printing of a hero sold face-by-face IS the fused card, and the
	// datastore keeps the label's own printing beside it at the same
	// number. "Enigma" at MST026 is the Slither // Enigma hero; the
	// standalone MST026 row is the cold-foil "Enigma (Marvel)", which
	// stays reachable through the m the storefront writes on its number
	// or the label spelled in the name. Two fused claimants cannot say
	// which card is meant, so they refuse rather than guess.
	// A number wearing a letter tail is not a plain face number: the
	// storefront writes the tail to demand the labeled variant, and that
	// demand must keep its say.
	if qualifierWord(inCard.Name) == "" && !strings.ContainsAny(number[len(number)-1:], letters) {
		fused, pairs := r.fusedFaceAt(inCard.Name, number)
		// Refusing is the better answer only while the fused cards could
		// be what is priced. A listing demanding a finish none of them was
		// ever sold in - the pairs are plain printings, and the labelled
		// one is the only foil the number carries - names the label as
		// surely as spelling it would, and the refusal buys nothing.
		if len(fused) > 1 && fusedAnswers(b, fused, inCard) {
			return
		}
		// One fused claimant answers only in a finish it was sold in: a
		// listing demanding the cold foil of a hero the fused card keeps
		// plain names the labelled printing beside it, and a rename
		// would hide that printing behind a card that cannot be priced.
		if len(fused) == 1 && fusedAnswers(b, fused, inCard) {
			inCard.Name = fused[0]
			inCard.Variation = strings.Replace(inCard.Variation, number, pairs[0], 1)
			return
		}
	}

	base := qualifierRe.ReplaceAllString(inCard.Name, "")
	// A pitch color belongs to the name and a treatment label does not, so
	// a name already wearing the one can still grow the other: the
	// datastore spells the promo marvels "Wild Ride (Red) (Marvel)", and
	// stripping the single parenthetical the input carries never reaches
	// them. The whole name is a stem beside the stripped one, and the
	// number still has to name the printing either way.
	stems := []string{base}
	if inCard.Name != base {
		stems = append(stems, inCard.Name)
	}
	var numbered, sold []spelling
	var tried []string
	for _, candidate := range stems {
		for _, pitch := range pitches {
			for _, label := range []string{"", marvelLabel} {
				name := candidate + pitch + label
				if slices.Contains(tried, name) {
					continue
				}
				tried = append(tried, name)
				if !numberedAs(b, name, number) {
					continue
				}
				numbered = append(numbered, spelling{name, candidate})
				if soldAs(b, name, number, inCard.Finish) {
					sold = append(sold, spelling{name, candidate})
				}
			}
		}
	}
	match, stem := oneSpelling(sold)
	// A promo is one card, and its number names it whatever finish the
	// storefront wrote beside it: Boast FAB189 is sold as a cold foil where
	// the only FAB189 is the rainbow one, and the Tournament Pack marvels as
	// cold foils where they are rainbow. The finish then has nothing left to
	// say, and it is dropped so the number can answer. A set card is sold in
	// several treatments, and there the finish keeps its say.
	if match == "" && len(sold) == 0 {
		match, stem = oneSpelling(numbered)
		if match == "" || !promoNumbered(b, match, number) {
			return
		}
		inCard.Finish = ""
	}
	if match == "" {
		return
	}
	// The respelling swaps the input's parenthetical for the one the
	// numbered printing wears, and the two do not always say the same
	// thing. A pitch color belongs to the name, so swapping one for
	// another loses nothing; but the very same parenthetical also spells a
	// treatment label that tells the printings sharing a number apart -
	// "Golden Skull (Yellow)" is a name, and "(Marvel)" is the label on one
	// of the two printings filed under it. Handing the dropped
	// parenthetical to the wording keeps that printing reachable. Where no
	// printing at the number wears the label the wording describes nothing
	// and the tiering is the respelling's own. A label grown onto the whole
	// name swapped nothing, so it drops nothing.
	var dropped string
	if stem == base {
		dropped = qualifierWord(inCard.Name)
	}
	if dropped != "" && !strings.EqualFold(dropped, qualifierWord(match)) {
		inCard.AddToVariant(dropped)
	}
	inCard.Name = match
}

// spelling is a name a qualifier respelling tried, with the stem it grew from.
type spelling struct {
	name, stem string
}

// oneSpelling answers the spelling the respelling settles on, or nothing
// where the spellings name more than one card. The marvel label names a
// printing rather than a card, and the datastore indexes a marvel under the
// labelled spelling beside the plain one, so "Nebula Duality" and "Nebula
// Duality (Marvel)" are one answer: the plain spelling, since the label is
// the wording's to describe.
func oneSpelling(spellings []spelling) (string, string) {
	var chosen spelling
	for _, candidate := range spellings {
		if chosen.name != "" && strings.TrimSuffix(candidate.name, marvelLabel) != strings.TrimSuffix(chosen.name, marvelLabel) {
			return "", ""
		}
		if chosen.name == "" || strings.HasSuffix(chosen.name, marvelLabel) {
			chosen = candidate
		}
	}
	return chosen.name, chosen.stem
}

// promoNumbered reports whether every printing filed under the name at the
// number is a promo.
func promoNumbered(b *matcher.Backend, name, number string) bool {
	var numbered bool
	for _, uuid := range b.Hashes[matcher.Normalize(name)] {
		co, found := b.UUIDs[uuid]
		if !found || co.Sealed || !numberMatches(number, co.Number) {
			continue
		}
		if !co.IsPromo && co.Rarity != "promo" {
			return false
		}
		numbered = true
	}
	return numbered
}

// dashedNumberRe matches the number a catalog writes with the label it
// belongs to: "ROS002-MV" for the marvel of ROS002, "DTD193-CF" for a cold
// foil. extractNumber leaves these to the wording, since the tail is what
// the wording spells - but the re-spelling below reads one, because a
// number is the only thing that makes a re-spelling safe and numberMatches
// already reads the stem back out of it.
var dashedNumberRe = regexp.MustCompile(`^[0-9]?[A-Za-z]+[0-9]{1,4}-[A-Za-z]{1,3}$`)

// labelledNumber is extractNumber, falling back on a number written with
// its label. "Florian" listed at ROS002-MV read as no number at all, so the
// re-spelling never ran and the listing answered with the plain Florian of
// another set.
func labelledNumber(variation string) string {
	if number := extractNumber(variation); number != "" {
		return number
	}
	for _, field := range strings.Fields(variation) {
		if dashedNumberRe.MatchString(field) {
			return field
		}
	}
	return ""
}

// regrownName is the input's name with the qualifier its wording carries
// put back on, empty where the wording names none or the spelling it makes
// is not a name this datastore knows.
//
// A pitch color is part of a Flesh and Blood name, but a storefront writes
// it as a parenthetical like any other and the split before the lookup
// takes it off: "To the Point (Red)" arrives as "To the Point" with "Red"
// in the wording, and nothing is named that. The word is the storefront's
// own, so putting it back is reading the listing rather than guessing at
// it - unlike a number, which names a printing and never a name.
//
// It answers only for a name the datastore does not otherwise know, which
// is what keeps it off the other direction this file already handles: a
// marvel the datastore labels beside the plain name is reached by that
// plain name, and a plain name is canonical, so the split keeps its say.
func regrownName(b *matcher.Backend, inCard *matcher.InputCard) string {
	for _, word := range strings.Fields(inCard.Variation) {
		word = strings.Trim(word, "()")
		if !qualifierRe.MatchString(" (" + word + ")") {
			continue
		}
		if name := b.CanonicalNames[matcher.Normalize(inCard.Name+" ("+word+")")]; name != "" {
			return name
		}
	}
	return ""
}

// qualifierWord returns the label inside a name's qualifier parenthetical,
// empty for a name carrying none.
func qualifierWord(name string) string {
	return strings.Trim(strings.TrimSpace(qualifierRe.FindString(name)), "()")
}

// fusedAnswers reports whether any printing of the named fused cards could
// be the one the input prices. Only the finish is asked, and an input naming
// none is answered by them all: a raised foil flag on its own is a
// storefront's single bit for a game selling three treatments, while a named
// finish, or a flag on a game whose fused pairs are plain printings, is the
// listing saying outright that no pair is what it sells.
func fusedAnswers(b *matcher.Backend, names []string, inCard *matcher.InputCard) bool {
	if inCard.Finish == "" && !inCard.Foil {
		return true
	}
	for _, name := range names {
		for _, uuid := range b.Hashes[matcher.Normalize(name)] {
			co, found := b.UUIDs[uuid]
			if !found || co.Sealed {
				continue
			}
			if inCard.Finish != "" {
				if b.FinishUUID(&co.Card, inCard.Finish) != "" {
					return true
				}
				continue
			}
			if co.FoilUUIDs[matcher.FinishFoil] != "" {
				return true
			}
		}
	}
	return false
}

// numberedAs reports whether any printing filed under the name carries the
// collector number.
func numberedAs(b *matcher.Backend, name, number string) bool {
	for _, uuid := range b.Hashes[matcher.Normalize(name)] {
		co, found := b.UUIDs[uuid]
		if !found || co.Sealed {
			continue
		}
		if numberMatches(number, co.Number) {
			return true
		}
	}
	return false
}

// soldAs reports whether a printing filed under the name at the number is
// sold in the finish, any printing answering an input that names none.
func soldAs(b *matcher.Backend, name, number, finish string) bool {
	if finish == "" {
		return true
	}
	for _, uuid := range b.Hashes[matcher.Normalize(name)] {
		co, found := b.UUIDs[uuid]
		if !found || co.Sealed || !numberMatches(number, co.Number) {
			continue
		}
		if b.FinishUUID(&co.Card, finish) != "" {
			return true
		}
	}
	return false
}

// AdjustName provides a prefix fallback for truncated feeds, adopting the
// one name among the prefix matches that carries the input's number. The
// pitch-color names resolve here: a bare "Breakneck Battery" extends into
// "Breakneck Battery (Red)" when the number picks the red pitch. Names
// compare normalized, so punctuation variants of one name are not read as
// ambiguity.
func (r Rules) AdjustName(b *matcher.Backend, inCard *matcher.InputCard) {
	if _, found := b.CanonicalNames[matcher.Normalize(inCard.Name)]; found {
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
	if match == "" {
		return
	}
	// The same face-of-a-fused-card rule that gates the qualifier respell
	// gates the prefix extension: a bare face name must not grow into the
	// marvel label's own printing while fused cards claim the face at this
	// number.
	if strings.HasSuffix(match, "(Marvel)") && number != "" {
		if names, _ := r.fusedFaceAt(inCard.Name, number); len(names) > 0 && fusedAnswers(b, names, inCard) {
			return
		}
	}
	inCard.Name = match
}

// faceKey folds a fused name onto the set of its faces: the normalized face
// names, deduplicated and sorted, so the orders a card's faces can be
// written in share one key - and so does the doubled spelling cardtrader
// sells a pairing under ("Gold // Golden Cog // Gold // Golden Cog"). A name
// with no separator has no key.
func faceKey(name string) string {
	split := strings.Split(name, "//")
	if len(split) < 2 {
		return ""
	}
	var faces []string
	for _, face := range split {
		face = matcher.Normalize(face)
		if !slices.Contains(faces, face) {
			faces = append(faces, face)
		}
	}
	sort.Strings(faces)
	return strings.Join(faces, "//")
}

// fusedNamedBy answers the fused cards spelled from the given name's faces,
// in any order: the fused cards whose faces are the same set.
//
// The faces are compared as a set rather than tried as orderings. A storefront
// writes the name, so how many faces arrive is the storefront's to decide -
// asking after each ordering costs a factorial of that, and a name of eleven
// faces stalls the match for the better part of a minute. Which orderings
// exist is not the question anyway: the question is which printings wear
// these faces, and a key answers it in one lookup whatever the count.
// The slice is the index's own, shared by every match, so it is read-only.
func (r Rules) fusedNamedBy(name string) []string {
	key := faceKey(name)
	if key == "" {
		return nil
	}
	return r.fusedByFaces[key]
}

// fusedFaceAt answers the fused cards one of whose faces is the given name
// at the given collector number, with the pair number of each. The fused
// pair cannot be reconstructed from one face the way fusedNamedBy rebuilds
// it from both, so each face indexes the printings wearing it.
func (r Rules) fusedFaceAt(name, number string) (names, numbers []string) {
	for _, entry := range r.fusedByFace[matcher.Normalize(name)] {
		if !numberMatches(number, entry.half) || slices.Contains(names, entry.name) {
			continue
		}
		names = append(names, entry.name)
		numbers = append(numbers, entry.number)
	}
	return names, numbers
}

// adjustFusedName adopts the datastore's spelling of a fused card whose two
// faces the storefront wrote in the opposite order: cardtrader sells
// "Spectral Shield // Soul Shackle" where the datastore files "Soul Shackle
// // Spectral Shield", the same physical card. The faces identify the card
// whichever way they are written, so the unordered face set is what is
// looked up.
//
// The order cannot be flipped blindly, because both orders can be real
// cards: "Quicken // Harmonized Kodachi" is a promo of its own beside the
// Welcome to Rathe "Harmonized Kodachi // Quicken". So an input whose own
// spelling has a printing at its pair number is left alone, and past that
// the pair number picks among the spellings; only with no number to ask
// does a unique spelling answer by the faces alone.
//
// The collector number sometimes needs more than the flip the unordered
// compare in numberMatches already gives it: cardtrader's Monarch hero
// numbers disagree with the datastore's outright (its "MON220//MON068" pair
// is the datastore's MON088//MON002), so when the input's pair names no
// printing of the adopted name, the number every printing agrees on
// replaces it - the faces already identified the card, and a pair kept
// wrong would gate the match right back out. A number the printings
// disagree on is left alone for the edition to settle.
func (r Rules) adjustFusedName(b *matcher.Backend, inCard *matcher.InputCard) {
	if !strings.Contains(inCard.Name, "//") {
		return
	}
	pair := pairNumberRe.FindString(inCard.Variation)
	// The spelling is the input's own only while a printing under it is
	// sold in the finish named, the way adjustQualifier reads a spelling:
	// the token "Aether Ashwing // Ash" is plain, and its cold foil is
	// filed as "Ash // Aether Ashwing" beside it.
	if pair != "" && numberedAs(b, inCard.Name, pair) && soldAs(b, inCard.Name, pair, inCard.Finish) {
		return
	}
	names := r.fusedNamedBy(inCard.Name)
	if len(names) == 0 && pair != "" {
		names = fusedNumbered(b, inCard.Name, pair)
	}
	name := ""
	if pair != "" {
		for _, candidate := range names {
			if !numberedAs(b, candidate, pair) || !soldAs(b, candidate, pair, inCard.Finish) {
				continue
			}
			if name != "" {
				return
			}
			name = candidate
		}
	}
	if name == "" {
		// No spelling carries the number (or none was given): only a
		// unique spelling can answer, and an input already canonical
		// with no number saying otherwise stays as it is.
		if len(names) != 1 {
			return
		}
		if _, canonical := b.CanonicalNames[matcher.Normalize(inCard.Name)]; canonical && pair == "" {
			return
		}
		name = names[0]
	}
	inCard.Name = name

	if pair == "" || numberedAs(b, name, pair) {
		return
	}
	number := ""
	for _, uuid := range b.Hashes[matcher.Normalize(name)] {
		co, found := b.UUIDs[uuid]
		if !found || co.Sealed {
			continue
		}
		if number != "" && !strings.EqualFold(canonicalNumber(number), canonicalNumber(co.Number)) {
			return
		}
		number = co.Number
	}
	if number != "" {
		inCard.Variation = strings.Replace(inCard.Variation, pair, number, 1)
	}
}

// fusedNumbered answers the fused cards carrying the pair number whose faces
// the name also spells, for a name that spells them in a way no face set
// answers: Star City Games writes the double-sided token "Marked // Fealty"
// as "Fealty / Marked // Fealty / Marked", one face per side of the slash.
// The pair picks the printing, since two faces number one card, and a face
// of the printing found in the name is what vouches that the pair was read
// off this card and not off a neighbour's number.
func fusedNumbered(b *matcher.Backend, name, pair string) []string {
	norm := matcher.Normalize(name)
	var names []string
	for _, co := range b.UUIDs {
		if co.Sealed || !strings.Contains(co.Name, "//") || !strings.Contains(co.Number, "/") {
			continue
		}
		if !numberMatches(pair, co.Number) || slices.Contains(names, co.Name) {
			continue
		}
		for face := range strings.SplitSeq(co.Name, "//") {
			if strings.Contains(norm, matcher.Normalize(face)) {
				names = append(names, co.Name)
				break
			}
		}
	}
	return names
}

// AdjustEdition trims the game-name prefix and the "Singles" suffix
// storefronts decorate set names with, and moves the print-run suffixes
// cardtrader spells its expansions with ("Welcome to Rathe - 1st Edition")
// into the variation: the datastore's sets carry no print run, but the
// variation carrying it names the priced entry selectFinish resolves. An
// edition that still matches no set simply does not narrow the candidates.
func (Rules) AdjustEdition(b *matcher.Backend, inCard *matcher.InputCard) {
	edition, printRun := aliasEdition(inCard.Edition)
	if printRun != "" {
		inCard.AddToVariant(printRun)
	}
	if shelf := promoShelf(b, edition, inCard.Name, extractNumber(inCard.Variation)); shelf != "" {
		edition = shelf
	}
	inCard.Edition = edition
}

// promoShelf answers the set holding the promo a listing names, or nothing
// for a listing that is not a promo's or a number no promo set holds.
//
// The promos are filed on the set of their programme where the datastore
// keeps one and on the one set every programme was once filed in
// otherwise, and a storefront names neither reliably: "Promos" is the FAB
// programme's own set by name, and the older set is "Promo Cards". Under
// a promo edition the number says which set: its letters name the
// programme, and the programme's set is asked for the number before the
// older one is, so HER089 is found on the older set where the Hero
// programme's own holds only HER156. A number without letters names the
// card only with the name beside it.
func promoShelf(b *matcher.Backend, edition, name, number string) string {
	if number == "" || !strings.Contains(strings.ToLower(edition), "promo") {
		return ""
	}
	letters := numberLetters(number)
	var codes []string
	if letters != "" {
		codes = append(codes, strings.ToUpper(letters))
	}
	for _, code := range promoSetCodes(b) {
		if !slices.Contains(codes, code) {
			codes = append(codes, code)
		}
	}
	norm := matcher.Normalize(name)
	for _, code := range codes {
		set, found := b.Sets[code]
		if !found {
			continue
		}
		for _, card := range set.Cards {
			if !numberMatches(number, card.Number) {
				continue
			}
			if letters == "" && matcher.Normalize(card.Name) != norm {
				continue
			}
			return set.Name
		}
	}
	return ""
}

// promoSetCodes lists the sets that hand their cards out, by code and in a
// fixed order: the datastore's own promo type, and the sets named for it.
func promoSetCodes(b *matcher.Backend) []string {
	var codes []string
	for code, set := range b.Sets {
		if set.Type == setTypePromo || strings.Contains(strings.ToLower(set.Name), "promo") {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	return codes
}

// numberLetters answers the letters a collector number opens on, the
// programme a promo's number names.
func numberLetters(number string) string {
	for i, r := range number {
		if r >= '0' && r <= '9' {
			return number[:i]
		}
	}
	return ""
}

// AliasEdition spells an edition string toward a set name using the string
// alone. See matcher.GameRules.
func (Rules) AliasEdition(b *matcher.Backend, edition string) string {
	edition, _ = aliasEdition(edition)
	return edition
}

// aliasEdition trims the decorations storefronts hang on a set name and
// reports the print-run suffix it removed, if any.
func aliasEdition(edition string) (string, string) {
	edition = strings.TrimSpace(edition)
	for _, prefix := range []string{"Flesh and Blood", "Flesh & Blood"} {
		if strings.HasPrefix(edition, prefix) {
			edition = strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(edition, prefix), ":-"))
			break
		}
	}
	edition = strings.TrimSpace(strings.TrimSuffix(edition, "Singles"))
	// Card Trader shelves the runs as "Welcome to Rathe - Alpha" and
	// "Monarch - First" beside "Monarch - Unlimited".
	for _, suffix := range []string{"1st Edition", "Unlimited Edition", "Unlimited", "Alpha Print Run", "Alpha", "First"} {
		trimmed := strings.TrimSuffix(edition, suffix)
		if trimmed != edition {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(trimmed), "-")), suffix
		}
	}
	return edition, ""
}

// PlainNumber implements matcher.GameRules. A number carries its set code and pads the ordinal behind it, where a person writes the ordinal alone: 1HP085 is card 85.
func (Rules) PlainNumber(number string) string {
	return matcher.PlainOrdinal(number)
}

// FilterCards narrows candidates by edition, collector number and variant.
// The variant tiering mirrors the number sharing in the data: when the
// input's wording describes a variant label, the printings it describes
// win; a bare input keeps the base printing when one exists; the letter
// tail cardtrader appends to a marvel's number demands some variant without
// saying which.
func (Rules) FilterCards(b *matcher.Backend, inCard *matcher.InputCard, sets []matcher.CandidateSet) []matcher.Card {
	number := extractNumber(inCard.Variation)

	var candidates, exact, unsold []matcher.Card
	named := map[string]bool{}
	var cards []matcher.Card
	for _, set := range sets {
		cards = append(cards, set.Cards...)
	}
	for _, card := range cards {
		// A printing the catalog left unnumbered cannot disagree with a
		// number: the Antiquity Pack art cards and the armory decks'
		// counters carry none, and the storefront's number for them is
		// its own.
		//
		// A printing TCGplayer numbered apart from its Legend Story
		// Studios id is reachable by that id too - SCG's sku follows the
		// fabId where the catalog numbers it differently. A pair carries
		// no single id to compare, and an id that is only the number's
		// own label stem (both halves of MST158's lettered pair carry
		// fabId "MST158") would fold the halves onto each other, so both
		// are excluded.
		fabID := card.Identifiers["fabId"]
		if strings.Contains(card.Number, "/") || strings.EqualFold(fabID, labelStem(card.Number)) {
			fabID = ""
		}
		if number != "" && card.Number != "" && !numberMatches(number, card.Number) &&
			(fabID == "" || !numberMatchesOn(number, fabID, false)) {
			continue
		}
		numbered := number == "" || card.Number == "" || numberMatchesOn(number, card.Number, false) ||
			(fabID != "" && numberMatchesOn(number, fabID, false))
		// An input naming a print run or a treatment re-keys the copy's
		// FoilUUIDs so the flag-driven resolution downstream lands on that
		// printing's entry. Both slots move together: a printing spans one
		// exact foilness, so the vendor's foil flag must not pull the
		// resolution off it.
		uuid := finishUUID(b, inCard, &card)
		// A caller that names its finish in the field has named the
		// product it prices, and a product sold in no such printing is
		// not that product. The printings sharing a name and a number
		// differ in nothing else - ROS000 is the cold foil "Will of
		// Arcana" beside the rainbow one - so a treatment the product
		// does not wear is what tells them apart, and keeping the
		// candidate files two sku prices under one uuid. The wording
		// keeps its own say: only the field speaks here, and the print
		// runs a product answers by a bare treatment name are aliased
		// onto their own entry before this asks.
		if uuid == "" && inCard.Finish != "" {
			// A product sold in one printing is one card, whatever finish
			// the storefront writes beside it: a deck's hero is filed
			// plain where it was printed in rainbow foil. Whether that
			// number is otherwise alone in the backend is checked once,
			// below, rather than per candidate.
			if numbered && singlePrinting(&card) {
				unsold = append(unsold, card)
			}
			continue
		}
		if uuid != "" {
			foilUUIDs := make(map[string]string, len(card.FoilUUIDs))
			maps.Copy(foilUUIDs, card.FoilUUIDs)
			foilUUIDs[matcher.FinishNonfoil] = uuid
			foilUUIDs[matcher.FinishFoil] = uuid
			card.FoilUUIDs = foilUUIDs
			named[card.UUID] = true
		}
		candidates = append(candidates, card)
		if numbered {
			exact = append(exact, card)
		}
	}
	if len(candidates) == 0 && len(unsold) == 1 && aloneAt(b, number) {
		return unsold
	}
	// A printing wearing the number outranks one wearing it under a
	// label: the Unlimited "Helm of Isen's Peak" is WTR042 beside the
	// "WTR042-C" product sold in the same finish, and the label is the
	// storefront's to name.
	if len(exact) > 0 {
		candidates = exact
	}
	if len(candidates) <= 1 {
		return candidates
	}

	// A wording naming a pitch value names the printing that pitches for
	// it. The value is a field of its own - the same card is printed at
	// three of them and the catalog writes which into the product name -
	// so it is read off the field rather than off the label.
	if pitched := pitchNarrow(inCard, candidates); len(pitched) > 0 {
		candidates = pitched
	}
	return finishTwins(inCard, named, labelTier(inCard, number, candidates))
}

// aloneAt reports whether number identifies exactly one non-sealed printing
// in the whole backend, whatever card or set it belongs to.
func aloneAt(b *matcher.Backend, number string) bool {
	var rows int
	for _, co := range b.UUIDs {
		if !co.Sealed && co.Number != "" && numberMatches(number, co.Number) {
			rows++
		}
	}
	return rows == 1
}

// singlePrinting reports whether card was sold in one printing: its finish
// siblings, if any, all price the same uuid.
func singlePrinting(card *matcher.Card) bool {
	uuids := map[string]bool{}
	for _, uuid := range card.FoilUUIDs {
		uuids[uuid] = true
	}
	return len(uuids) == 1
}

// finishTwins settles a tie between printings that the finish tells apart.
//
// The catalog sells a few products twice at one number, once per finish and
// differing in nothing else: ROS000 is the cold foil "Will of Arcana" beside
// the rainbow one, and The Hunted's art card is sold plain and in cold foil.
// The catalog spells the finish into the variant, which is prose the datastore
// distils into the promo types, and a printing publishing none wears no label,
// so the twins tie on everything the tiering reads. The finish is a fact of the
// printing, and the one the wording speaks to: a wording naming a finish
// re-keys the copy sold in it and no other, and a wording naming
// none prices the copy sold in the foilness its flag says. Two copies sold
// in that foilness are still a tie.
func finishTwins(inCard *matcher.InputCard, named map[string]bool, cards []matcher.Card) []matcher.Card {
	if len(cards) < 2 {
		return cards
	}
	var kept []matcher.Card
	for _, card := range cards {
		if named[card.UUID] {
			kept = append(kept, card)
		}
	}
	if len(kept) > 0 {
		return kept
	}
	finish := matcher.FinishNonfoil
	if inCard.Foil {
		finish = matcher.FinishFoil
	}
	for _, card := range cards {
		if slices.Contains(card.Finishes, finish) {
			kept = append(kept, card)
		}
	}
	if len(kept) > 0 {
		return kept
	}
	return cards
}

// labelTier picks among the candidates by the labels the wording describes:
// the printings it describes, else the variants a letter tail demands, else
// the base printings.
func labelTier(inCard *matcher.InputCard, number string, candidates []matcher.Card) []matcher.Card {
	described, base, variants := tierByVariant(inCard, candidates)
	if len(described) > 0 {
		return described
	}
	if wantsVariant(number) {
		// The letter says which variant where the printings are lettered:
		// the three cold foils of Lightning Flow are OMN203's "a", "b"
		// and "c", and a storefront writes the letter onto the number.
		if lettered := labelledBy(number[len(number)-1:], variants); len(lettered) > 0 {
			return lettered
		}
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

// tierByVariant splits the candidates into the ones whose variant label the
// input's variation or finish describes, the base printings, and the
// variant printings. The set name is not consulted: set names carry the
// labels' words. The finish is, since a label is spelled from a treatment
// where nothing else tells two printings apart - the plain cold foil of
// Ash // Aether Ashwing is labelled "Cold Foil" beside the marvel - and a
// caller naming the treatment in the field names that label. When several
// labels are described, the finish tokens step aside - a label spelled
// from them alone ("Cold Foil" the label) defers to the label the rest of
// the wording still describes.
func tierByVariant(inCard *matcher.InputCard, candidates []matcher.Card) (described, base, variants []matcher.Card) {
	wording := strings.TrimSpace(inCard.Variation + " " + inCard.Finish)
	words := strings.Fields(strings.ToLower(wording))
	onePitch := len(pitchesOf(candidates)) <= 1
	for _, card := range candidates {
		if len(card.PromoTypes) == 0 || onePitch && len(treatments(card)) == 0 {
			base = append(base, card)
			continue
		}
		variants = append(variants, card)
	}
	// The tags are tokens, so the wording's words are joined back up a
	// run at a time to ask whether they name them.
	described = matcher.DescribedVariants(wording, variants)
	if len(described) > 1 {
		var bare []string
		for _, word := range words {
			if !finishToken(word) {
				bare = append(bare, word)
			}
		}
		refined := matcher.DescribedVariants(strings.Join(bare, " "), described)
		if len(refined) > 0 {
			described = refined
		}
	}
	return
}

// pitchLabel reports whether a label is a pitch color.
func pitchLabel(label string) bool {
	switch label {
	case "red", "yellow", "blue":
		return true
	}
	return false
}

// pitchesOf answers the distinct pitch colors labelled on the printings.
func pitchesOf(cards []matcher.Card) []string {
	var colors []string
	for _, card := range cards {
		for _, promoType := range card.PromoTypes {
			if pitchLabel(promoType) && !slices.Contains(colors, promoType) {
				colors = append(colors, promoType)
			}
		}
	}
	return colors
}

// treatments answers the labels on a printing that name a treatment. A
// pitch color is a piece of the name, whichever field the catalog filed it
// under, and where the printings at a number all wear the same one it tells
// none of them apart: "Drop of Dragon Blood" is red on every printing, and
// the label on the base printing would otherwise leave the extended art
// beside it with no base to be a variant of. Two pitches at one number are
// two cards, and there the label keeps telling them apart.
func treatments(card matcher.Card) []string {
	var labels []string
	for _, promoType := range card.PromoTypes {
		if !pitchLabel(promoType) {
			labels = append(labels, promoType)
		}
	}
	return labels
}

// labelledBy answers the printings carrying the label.
func labelledBy(label string, cards []matcher.Card) []matcher.Card {
	var labelled []matcher.Card
	for _, card := range cards {
		if slices.Contains(card.PromoTypes, label) {
			labelled = append(labelled, card)
		}
	}
	return labelled
}

// finishToken reports whether selectFinish consumes the word.
func finishToken(word string) bool {
	switch word {
	case "1st", "first", "unlimited", "alpha", "rainbow", "cold", "normal":
		return true
	}
	return false
}

// wantsVariant reports whether the input demands some variant printing
// without describing which: the lowercase letter tail cardtrader appends to
// the collector number ("MST238m"). The uppercase dashed tails ("MST158-A")
// are real collector numbers, not demands.
func wantsVariant(number string) bool {
	if number == "" {
		return false
	}
	last := number[len(number)-1]
	return last >= 'a' && last <= 'z'
}

// finishUUID resolves the entry the input names to its uuid. The finish
// field speaks first and through the shared resolution, which is what walks
// the printing's aliases onto the print run it does carry, so a caller
// pricing one sku names the treatment rather than spelling it into the
// wording; the wording is the fallback for the storefronts that only ever
// had one field, and the only one that can name the two axes separately.
func finishUUID(b *matcher.Backend, inCard *matcher.InputCard, card *matcher.Card) string {
	if uuid := b.FinishUUID(card, inCard.Finish); uuid != "" {
		return uuid
	}
	return card.FoilUUIDs[selectFinish(inCard, card)]
}

// selectFinish maps the input wording's print-run and treatment tokens
// onto one of the card's stored printings, so a listing spelling its
// finish out (TCGplayer's "1st Edition Rainbow Foil" printing name in the
// variation, cardtrader's "1st Edition" expansion suffix) resolves to that
// printing's entry instead of the flag-driven default. Only the variation
// speaks: set names carry the same words ("1st Strike"). The unsaid axis
// fills from the card itself - a bare print run is its plain printing, a
// bare treatment takes the plainest print run sold with it. A wording
// naming no finish, or a printing the product was not priced in, keeps the
// flag-driven default.
//
// A marvel names the cold foil it is printed in: 278 of the catalog's 289
// marvel rows are cold foil, the rest rainbow-foil hero promos filed at the
// rarity, and a product sold in no cold foil keeps the default as it does
// for any treatment it was not priced in. The word stays a label to the
// tiering, which is what tells the marvel from the plain cold foil beside
// it; here it tells the marvel from a plain copy the catalog also files at
// the marvel rarity, which the tiering cannot.
func selectFinish(inCard *matcher.InputCard, card *matcher.Card) string {
	var run, treatment string
	for word := range strings.FieldsSeq(strings.ToLower(inCard.Variation)) {
		switch word {
		case "1st", "first", "alpha":
			// cardtrader's alpha print run sells as TCGplayer's 1st Edition
			run = matcher.Run1stEdition
		case "unlimited":
			run = matcher.RunUnlimited
		case "rainbow":
			treatment = "Rainbow Foil"
		case "cold", "marvel":
			treatment = "Cold Foil"
		case "normal":
			treatment = "Normal"
		}
	}
	if run == "" && treatment == "" {
		return ""
	}
	if treatment == "" {
		treatment = "Normal"
	}
	return matcher.NamedFinish(card.FoilUUIDs, run, treatment)
}

// extractNumber pulls the collector number out of the scraper-supplied
// Variation: a fused pair whole ("WTR040 // WTR039"), then the full
// "WTR215" shape, else the first digit-leading field ("215").
func extractNumber(variation string) string {
	pair := pairNumberRe.FindString(variation)
	if pair != "" {
		return pair
	}
	fields := strings.Fields(variation)
	for _, field := range fields {
		if fullNumberRe.MatchString(field) {
			return field
		}
	}
	for _, field := range fields {
		// The print-run ordinal reads digit-first but is never a number
		// ("1st Edition")
		if strings.EqualFold(field, "1st") {
			continue
		}
		if field[0] >= '0' && field[0] <= '9' {
			return strings.Split(field, "/")[0]
		}
	}
	return ""
}

// canonicalNumber folds the pair-separator spellings together ("WTR040 //
// WTR039", "UPR002//UPR165" and "MON219 / MON220" all become
// "WTR040/WTR039"-shaped) so the pairs compare by content.
func canonicalNumber(number string) string {
	fields := strings.FieldsFunc(number, func(r rune) bool {
		return r == ' ' || r == '/'
	})
	return strings.Join(fields, "/")
}

// numberMatches compares an input number against a printing's collector
// number: equal codes, the same number written with different padding, a
// letter-tailed input matching its base number, a pair matched by its front
// half, or a bare digit input (its own letter tail aside, "238m") matching
// the digit tail of the front code. Leading zeros never decide ("40"
// matches "WTR040").
func numberMatches(input, full string) bool {
	return numberMatchesOn(input, full, true)
}

// numberMatchesOn is numberMatches with the label stems read or not.
func numberMatchesOn(input, full string, stems bool) bool {
	ci := canonicalNumber(input)
	cf := canonicalNumber(full)
	if strings.EqualFold(ci, cf) {
		return true
	}
	// The catalog pads a few numbers a digit wider than the printing wears
	// them - "JDG0077" for JDG077 - and a storefront writes whichever it was
	// given. Folding the padding away compares what the number says rather
	// than how wide it was written; no two numbers in a set fold together.
	if foldNumber(ci) == foldNumber(cf) {
		return true
	}
	// A fused card's pair is written in whichever order its faces were, so
	// two pairs compare as sets: cardtrader's "MON104//MON186" is the
	// catalog's "MON186//MON104". Two faces identify one card, so an
	// unordered compare cannot cross products.
	if strings.Contains(ci, "/") && strings.Contains(cf, "/") &&
		sortedPair(ci) == sortedPair(cf) {
		return true
	}
	trimmed := strings.TrimRight(ci, letters)
	if trimmed != ci && strings.EqualFold(trimmed, cf) {
		return true
	}
	// The catalog numbers a labelled printing with its label, "ROS001-MV"
	// for the marvel of ROS001 and "DTD193-CF" for the cold foil of
	// DTD193, where the card wears the plain number and so does every
	// storefront. The name and the finish are what tell the labelled
	// printing from the plain one, so the stem is the number.
	if stems {
		if stem := labelStem(cf); stem != cf && numberMatchesOn(ci, stem, false) {
			return true
		}
	}
	front, _, _ := strings.Cut(cf, "/")
	if strings.Contains(cf, "/") && strings.EqualFold(ci, front) {
		return true
	}
	// The catalog numbers some double-sided cards by one face alone: the
	// cold foil "Ash // Aether Ashwing" is UPR043, the token printing of
	// the same two faces UPR042//UPR043. A pair the storefront wrote for
	// it names that face among the two.
	if strings.Contains(ci, "/") && !strings.Contains(cf, "/") {
		for half := range strings.SplitSeq(ci, "/") {
			if numberMatchesOn(half, cf, stems) {
				return true
			}
		}
	}
	inFront, _, _ := strings.Cut(ci, "/")
	inFront = strings.TrimRight(inFront, letters)
	return isAllDigits(inFront) && matcher.CanonicalTail(inFront) == matcher.CanonicalTail(digitTail(front))
}

// labelStem strips the label a catalog number wears after a dash ("-MV"),
// answering the number unchanged when it wears none. A pair is never
// labelled.
func labelStem(number string) string {
	if strings.Contains(number, "/") {
		return number
	}
	stem, label, found := strings.Cut(number, "-")
	if !found || label == "" || strings.ToUpper(label) != label || strings.ContainsAny(label, "0123456789") {
		return number
	}
	return stem
}

// sortedPair folds a pair number's halves and sorts them, so the two orders
// a fused card's number is written in compare equal whatever padding or case
// either half arrived with, and whatever letter a storefront hung on a half
// to tell its pairings of one face apart ("MST158c"). Numbers that are not
// pairs fold whole.
func sortedPair(number string) string {
	halves := strings.Split(number, "/")
	for i, half := range halves {
		halves[i] = foldNumber(strings.TrimRight(half, letters))
	}
	sort.Strings(halves)
	return strings.Join(halves, "/")
}

func isAllDigits(number string) bool {
	if number == "" {
		return false
	}
	for i := 0; i < len(number); i++ {
		if number[i] < '0' || number[i] > '9' {
			return false
		}
	}
	return true
}

// digitTail returns the trailing digit run of a collector code ("215" from
// "WTR215"), empty when the code ends in a letter ("MST158-A").
func digitTail(number string) string {
	i := len(number)
	for i > 0 && number[i-1] >= '0' && number[i-1] <= '9' {
		i--
	}
	return number[i:]
}

// pitchValues are what a Flesh and Blood card pitches for.
var pitchValues = []string{"red", "yellow", "blue"}

// pitchNarrow keeps the printings pitching for the value a wording names,
// and says nothing where the wording names none, names two, or names one
// every candidate pitches for.
func pitchNarrow(inCard *matcher.InputCard, candidates []matcher.Card) []matcher.Card {
	wording := strings.ToLower(inCard.Variation)
	var named string
	for _, value := range pitchValues {
		if !matcher.SlugDescribes(wording, value) {
			continue
		}
		if named != "" {
			return nil
		}
		named = value
	}
	if named == "" {
		return nil
	}
	var kept []matcher.Card
	for _, card := range candidates {
		for _, color := range card.Colors {
			if strings.EqualFold(color, named) {
				kept = append(kept, card)
				break
			}
		}
	}
	if len(kept) == len(candidates) {
		return nil
	}
	return kept
}
