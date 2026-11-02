package matcher

import (
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// GetUUIDs returns every non-sealed uuid in the datastore. The result aliases
// the backend index and must not be modified.
func (b *Backend) GetUUIDs() []string {
	return b.AllUUIDs
}

// GetSealedUUIDs returns every sealed uuid in the datastore. The result
// aliases the backend index and must not be modified.
func (b *Backend) GetSealedUUIDs() []string {
	return b.AllSealedUUIDs
}

// GetUUIDsInSet returns every non-sealed uuid printed in the given set,
// foil and etched variants included, in sorted order. The result aliases
// the backend index and must not be modified; callers spanning multiple
// sets append the per-set results themselves.
func (b *Backend) GetUUIDsInSet(code string) []string {
	return b.SetUUIDs[strings.ToUpper(code)]
}

// GetSealedUUIDsInSet is the sealed-product counterpart of GetUUIDsInSet.
func (b *Backend) GetSealedUUIDsInSet(code string) []string {
	return b.SetSealedUUIDs[strings.ToUpper(code)]
}

// GetUUID returns the card object stored for the given uuid. The object
// is shared across all callers and must not be modified.
func (b *Backend) GetUUID(uuid string) (*CardObject, error) {
	if b.UUIDs == nil {
		return nil, ErrDatastoreEmpty
	}

	co, found := b.UUIDs[uuid]
	if !found {
		return nil, ErrCardUnknownID
	}

	return co, nil
}

// GetAllSets returns every set code in the datastore.
func (b *Backend) GetAllSets() []string {
	return b.AllSets
}

// GetSet returns the set with this code, matched case-insensitively. The set
// is shared and must not be modified.
func (b *Backend) GetSet(code string) (*Set, error) {
	if b.Sets == nil {
		return nil, ErrDatastoreEmpty
	}

	set, found := b.Sets[strings.ToUpper(code)]
	if !found {
		return nil, ErrCardNotInEdition
	}

	return set, nil
}

// GetSetByName returns the set an edition string names, trying the set code
// first, then the full name, then the many spellings storefronts use for it.
// The set is shared and must not be modified.
func (b *Backend) GetSetByName(edition string) (*Set, error) {
	if b.Sets == nil {
		return nil, ErrDatastoreEmpty
	}

	// 1. Check if input is just the set code
	set, err := b.GetSet(edition)
	if err == nil {
		return set, nil
	}

	// 2. Check if input is the full name of the set
	set, found := b.NormalizedSets[Normalize(edition)]
	if found {
		return set, nil
	}

	// 3. Ask the game to spell the edition the way the datastore does
	// (skipped when no GameRules are attached, e.g. a hand-built Backend)
	if b.rules != nil {
		set, found = b.NormalizedSets[Normalize(b.rules.AliasEdition(b, edition))]
		if found {
			return set, nil
		}
	}

	// 4. We tried
	return nil, ErrCardNotInEdition
}

// NameForm is the spelling Names lists the datastore's names in.
type NameForm int

const (
	// NameFormNormalized is each name as Normalize spells it, the keys of
	// Hashes.
	NameFormNormalized NameForm = iota
	// NameFormLowercase is each name as printed, in lower case.
	NameFormLowercase
	// NameFormCanonical is each name as printed.
	NameFormCanonical
)

// Names returns every card or sealed name in the datastore, spelled in the
// given form. A form outside the three returns nothing. It is not called
// AllNames because that is the normalized list's own field.
func (b *Backend) Names(form NameForm, sealed bool) []string {
	switch form {
	case NameFormNormalized:
		if sealed {
			return b.AllSealed
		}
		return b.AllNames
	case NameFormCanonical:
		if sealed {
			return b.AllCanonicalSealed
		}
		return b.AllCanonicalNames
	case NameFormLowercase:
		if sealed {
			return b.AllLowerSealed
		}
		return b.AllLowerNames
	}
	return nil
}

// SearchEquals returns the uuids of every printing whose name matches exactly,
// ignoring case and punctuation. An empty name returns everything.
func (b *Backend) SearchEquals(name string) ([]string, error) {
	if name == "" {
		return b.AllUUIDs, nil
	}

	results, found := b.Hashes[Normalize(name)]
	if !found {
		return nil, ErrCardDoesNotExist
	}

	return results, nil
}

// SearchSealedEquals is the sealed-product counterpart of SearchEquals.
func (b *Backend) SearchSealedEquals(name string) ([]string, error) {
	return b.searchFunc(name, b.AllSealed, func(a, c string) bool {
		return a == c
	})
}

func (b *Backend) searchFunc(name string, slice []string, f func(string, string) bool) ([]string, error) {
	var hashes []string
	name = Normalize(name)
	// One printing can answer to several entries - a game may index a card
	// under a qualified spelling as well as its bare name, and a prefix
	// match reaches both - so the buckets overlap and a caller would be
	// handed the same card twice.
	seen := map[string]bool{}
	for i := range slice {
		if !f(slice[i], name) {
			continue
		}
		for _, uuid := range b.Hashes[slice[i]] {
			if seen[uuid] {
				continue
			}
			seen[uuid] = true
			hashes = append(hashes, uuid)
		}
	}
	if hashes == nil {
		return nil, ErrCardDoesNotExist
	}
	return hashes, nil
}

// SearchHasPrefix returns the uuids of every printing whose name starts with
// the input, which is how a truncated listing is recovered.
func (b *Backend) SearchHasPrefix(name string) ([]string, error) {
	if name == "" {
		return b.AllUUIDs, nil
	}
	return b.searchFunc(name, b.AllNames, strings.HasPrefix)
}

// SearchContains returns the uuids of every printing whose name contains the
// input.
func (b *Backend) SearchContains(name string) ([]string, error) {
	return b.searchFunc(name, b.AllNames, strings.Contains)
}

// SearchRegexp returns the uuids of every printing whose name matches the
// expression.
func (b *Backend) SearchRegexp(name string) ([]string, error) {
	var hashes []string
	re, err := regexp.Compile(name)
	if err != nil {
		return nil, err
	}
	for i := range b.AllUUIDs {
		if re.MatchString(b.UUIDs[b.AllUUIDs[i]].Name) {
			hashes = append(hashes, b.AllUUIDs[i])
		}
	}
	if hashes == nil {
		return nil, ErrCardDoesNotExist
	}
	return hashes, nil
}

// SearchSealedContains is the sealed-product counterpart of SearchContains.
func (b *Backend) SearchSealedContains(name string) ([]string, error) {
	return b.searchFunc(name, b.AllSealed, strings.Contains)
}

// entry4Name returns the bucket entry actually named this way.
//
// Cards are deliberately hashed under their face, flavor and printed
// names as well, so that a query naming a single face still finds the
// card. One bucket therefore holds several distinct cards, each carrying
// the properties of its own card: "Servo" hashes both the Servo token and
// "Servo // Thopter".
//
// Lookups pick between them in this order: the card whose name matches
// verbatim, then any whose name normalizes the same, then the first entry
// in the bucket.
//
// The first case is what tells apart the cards that share a bucket outright:
// normalization folds case and punctuation, so "Mr. 1 (Daz.Bonez)" and
// "Mr.1 (Daz.Bonez)" are two cards under one key. The second prefers an
// entry that owns the name over one the bucket holds only as an alias, for
// a query spelled unlike any of them. The last covers buckets reached only
// by an alias, such as a flavor name.
func (b *Backend) entry4Name(name string) (*CardObject, bool) {
	norm := Normalize(name)
	uuids, found := b.Hashes[norm]
	if !found {
		return nil, false
	}
	var normalized *CardObject
	for _, uuid := range uuids {
		entry, found := b.UUIDs[uuid]
		if !found {
			continue
		}
		if strings.EqualFold(entry.Name, name) {
			return entry, true
		}
		if normalized == nil && Normalize(entry.Name) == norm {
			normalized = entry
		}
	}
	if normalized != nil {
		return normalized, true
	}
	entry, found := b.UUIDs[uuids[0]]
	return entry, found
}

// NameIsToken reports whether the card actually named this way is a token.
// Exported because the only caller lives in the per-game rules packages.
func (b *Backend) NameIsToken(name string) bool {
	entry, found := b.entry4Name(name)
	return found && entry.Layout == "token"
}

// Printings4Card returns the set codes a card by this name was printed in.
func (b *Backend) Printings4Card(name string) ([]string, error) {
	if b.Hashes == nil {
		return nil, ErrDatastoreEmpty
	}
	entry, found := b.entry4Name(name)
	if !found {
		return nil, ErrCardDoesNotExist
	}
	return slices.Clone(entry.Printings), nil
}

// HasNonfoilPrinting reports whether the card was ever sold nonfoil, narrowed
// to the given editions when any are named.
func (b *Backend) HasNonfoilPrinting(name string, editions ...string) bool {
	return b.HasPrinting(name, "finish", FinishNonfoil, editions...)
}

// HasFoilPrinting reports whether the named card carries the foil slot. A
// game whose finish is not foilness answers yes throughout: Yu-Gi-Oh's
// treatment is the rarity, so every printing points both flag slots at its
// default print run and none of them is a foil anybody sells.
func (b *Backend) HasFoilPrinting(name string, editions ...string) bool {
	return b.HasPrinting(name, "finish", FinishFoil, editions...)
}

// HasEtchedPrinting reports whether the card was ever sold etched, narrowed to
// the given editions when any are named.
func (b *Backend) HasEtchedPrinting(name string, editions ...string) bool {
	return b.HasPrinting(name, "finish", FinishEtched, editions...)
}

// HasPrinting reports whether any printing of the name carries the value
// in the named field, optionally restricted to the given editions.
func (b *Backend) HasPrinting(name, field, value string, editions ...string) bool {
	if b.Sets == nil {
		return false
	}

	var checkFunc func(Card, string) bool
	switch field {
	case "promo_type":
		checkFunc = func(card Card, value string) bool {
			return card.HasPromoType(value)
		}
	case "frame_effect":
		checkFunc = func(card Card, value string) bool {
			return card.HasFrameEffect(value)
		}
	case "border_color":
		checkFunc = func(card Card, value string) bool {
			return card.BorderColor == value
		}
	case "frame_version":
		checkFunc = func(card Card, value string) bool {
			return card.FrameVersion == value
		}
	case "finish":
		checkFunc = func(card Card, value string) bool {
			return card.HasFinish(value)
		}
	default:
		return false
	}

	// Resolve which real card name the query means, the way Printings4Card
	// does: the case-exact entry when one exists, the first normalized
	// match otherwise. The hash bucket conflates names that normalize the
	// same but belong to different cards ("Mr. 1 (Daz.Bonez)" beside
	// "Mr.1 (Daz.Bonez)"), and the printings of one must never answer for
	// the other.
	entry, found := b.entry4Name(name)
	if !found {
		if b.rules == nil {
			return false
		}
		cc := &InputCard{
			Name: name,
		}
		b.rules.AdjustName(b, cc)
		entry, found = b.entry4Name(cc.Name)
		if !found {
			return false
		}
	}
	canonicalName := entry.Name
	uuids := b.Hashes[Normalize(canonicalName)]

	// A pinned edition narrows the check to that set alone; when it cannot
	// be resolved, every printing is checked.
	var pinnedCode string
	if len(editions) > 0 {
		set := b.Sets[editions[0]]
		if set == nil {
			set, _ = b.GetSetByName(editions[0])
		}
		if set != nil {
			pinnedCode = set.Code
		}
	}

	// The hash bucket holds every card whose name (or alias) normalizes the
	// same, so this visits the cards a per-set scan comparing with Equals
	// would, without iterating full sets for every printing, which would
	// make this the dominant cost of Match for widely printed cards.
	// Aliases (flavor or printed names) hash into the same bucket too, so
	// the name is still checked against the real card name.
	for _, uuid := range uuids {
		co, found := b.UUIDs[uuid]
		if !found {
			continue
		}
		if pinnedCode != "" && co.SetCode != pinnedCode {
			continue
		}
		if strings.EqualFold(co.Name, canonicalName) && checkFunc(co.Card, value) {
			return true
		}
	}

	return false
}

// BuildSealedProductMap indexes the sealed products by one of their outside
// identifiers, skipping any product that does not carry it. A slice usually
// holds a single uuid, but an id shared by a foil and a nonfoil product holds
// both, foil last.
func (b *Backend) BuildSealedProductMap(idName string) map[int][]string {
	productMap := map[int][]string{}
	for _, uuid := range b.AllSealedUUIDs {
		co, err := b.GetUUID(uuid)
		if err != nil {
			continue
		}
		id := co.Identifiers[idName]
		if id == "" {
			id = b.secretLairFoilID(co, idName)
		}

		idNum, err := strconv.Atoi(id)
		if err != nil {
			continue
		}
		productMap[idNum] = append(productMap[idNum], uuid)

		// Preserve Foil variant at the end of the slice
		sort.Slice(productMap[idNum], func(i, j int) bool {
			coI := b.UUIDs[productMap[idNum][i]]
			coJ := b.UUIDs[productMap[idNum][j]]
			return coI.Name < coJ.Name
		})
	}
	return productMap
}

// secretLairFoilTags are the words a Secret Lair foil drop adds to the name
// of the drop whose listing it shares. Textured is absent: no drop sells
// those cards in another foil.
var secretLairFoilTags = []string{"Foil", "Rainbow", "Galaxy", "Confetti"}

// secretLairFoilID returns the identifier a Secret Lair foil drop carrying
// none shares with the drop its name extends, or "" for any other product.
func (b *Backend) secretLairFoilID(co *CardObject, idName string) string {
	if co.SetCode != "SLD" || !strings.HasSuffix(co.Name, " Foil") {
		return ""
	}

	var id string
	name := co.Name
	for _, tag := range secretLairFoilTags {
		name = strings.TrimSuffix(name, tag)
		name = strings.TrimSpace(name)

		uuids, err := b.SearchSealedEquals(name)
		if err != nil {
			continue
		}
		subco, found := b.UUIDs[uuids[0]]
		if !found {
			continue
		}
		id = subco.Identifiers[idName]
	}
	return id
}

// PromoTypeSlug renders a promo type as the single token that identifies it:
// lower case, with everything that is not a letter or a digit dropped. This
// is the form the games store, so that a promo type is one word wherever it
// is read - a search query is split on whitespace, and Magic's own types
// have always been single words for exactly that reason ("boosterfun").
func PromoTypeSlug(promoType string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(promoType) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// SlugDescribes reports whether a storefront's wording says the words a promo
// type's slug was made from, as a run of whole words.
//
// The slug has lost the spaces, so a plain comparison cannot be used and a
// substring test would be too generous - "metal" is inside "metallic". Words
// are joined back up a run at a time instead, which asks the question the
// slug's own spelling asks: does this wording name this promo type.
//
// A word that is punctuation alone ("-", "&") slugs to nothing and is dropped
// first: a run reads straight across it anyway, and left in, every run walked
// each stretch of them to the end, quadratic in the wording's length.
func SlugDescribes(wording, slug string) bool {
	if slug == "" {
		return false
	}
	var slugs []string
	for _, word := range strings.Fields(strings.ToLower(wording)) {
		s := PromoTypeSlug(word)
		if s != "" {
			slugs = append(slugs, s)
		}
	}
	for i := range slugs {
		rest := slug
		for _, s := range slugs[i:] {
			if !strings.HasPrefix(rest, s) {
				break
			}
			rest = rest[len(s):]
			if rest == "" {
				return true
			}
		}
	}
	return false
}

// containingLabel settles a tie between printings whose labels a catalog
// writes as one phrase rather than as separate tags, and leaves every other
// tie alone.
//
// TCGplayer sells "Monkey.D.Luffy (Super Alternate Art)" beside
// "Monkey.D.Luffy (Red Super Alternate Art)", one parenthetical each, so the
// printings wear one tag apiece and one tag contains the other whole. Both
// are named by a wording that spells the longer out, both name one tag and
// have none left over, and the count cannot separate them - which aliases
// away a wording that could not have been more specific. Containment is what
// the two labels share and what settles it: the tag that spells the other
// out is the one the wording was about.
//
// Nothing settles a tie between labels that are merely different. Flesh and
// Blood writes the finish into the same wording as the variant, so "UPR043
// Cold Foil" on a card whose name says "(Marvel)" describes a cold foil
// label and a marvel one at once - neither containing the other, and the
// longer of them not the answer. That tie is the caller's to break, by
// asking again without the words its finish already consumed.
func containingLabel(wording string, cards []Card) []Card {
	if len(cards) < 2 {
		return cards
	}
	named := make([][]string, len(cards))
	for i, card := range cards {
		for _, promoType := range card.PromoTypes {
			if SlugDescribes(wording, promoType) {
				named[i] = append(named[i], promoType)
			}
		}
	}
	for i := range cards {
		var wider bool
		beats := true
		for j := range cards {
			if i == j {
				continue
			}
			// Containment has to run one way only. Two printings wearing
			// the same label spell each other out, and answering with
			// whichever came first would settle by position what the
			// labels do not settle at all - the edition the printings sit
			// in is what tells those apart, further down.
			if !labelsContain(named[i], named[j]) || labelsContain(named[j], named[i]) {
				beats = false
				break
			}
			wider = true
		}
		if beats && wider {
			return []Card{cards[i]}
		}
	}
	return cards
}

// labelsContain reports whether every label in inner is spelled out by one of
// the labels in outer, which is true of a label against itself.
func labelsContain(outer, inner []string) bool {
	for _, in := range inner {
		if !slices.ContainsFunc(outer, func(out string) bool {
			return strings.Contains(out, in)
		}) {
			return false
		}
	}
	return true
}

// SlugDescribesAny reports whether a wording names any of the promo types a
// printing wears. Every one is asked rather than the first alone: a printing
// carrying two tags is named by either of them.
func SlugDescribesAny(wording string, slugs []string) bool {
	for _, slug := range slugs {
		if SlugDescribes(wording, slug) {
			return true
		}
	}
	return false
}

// DescribedVariants keeps the printings a wording names best, and nothing
// when it names none of them.
//
// A printing is asked about every promo type it wears rather than its first,
// which is what makes the rest of them reachable: the three alternate arts
// of one Yu-Gi-Oh number are told apart by the colour they wear beside the
// shared "Alternate Art", and the only extended art of a Flesh and Blood
// number wears those words behind a colour.
//
// Best is the most tags named, and among those the printing wearing the
// fewest the wording said nothing about. The count is what lets a fuller
// wording win - "Alternate Art Blue" names the printing wearing both over the
// ones wearing either - and the tie-break is what keeps a narrow wording
// narrow, so "Blue" still names the plain blue printing rather than the
// alternate-art one that merely contains the word.
//
// The third is for the labels a catalog writes as one phrase rather than as
// separate tags. TCGplayer sells "Monkey.D.Luffy (Super Alternate Art)"
// beside "Monkey.D.Luffy (Red Super Alternate Art)", one parenthetical
// each, so the printings wear one tag apiece and one tag contains the other
// whole. Both are named by a wording that spells the longer out, both name
// one tag and have none left over, and the first two rules call that a tie -
// which aliases away a wording that could not have been more specific.
// Containment settles it the way specificity does, and only where it runs
// one way: the tag that spells the other out is the one the wording was
// about, while two printings wearing the same label say nothing about which
// was meant and are left as they are. See containingLabel. A game whose
// catalog splits the same distinction into separate tags never reaches
// there, the count having already answered.
func DescribedVariants(wording string, cards []Card) []Card {
	var best []Card
	var most, unnamed int
	for _, card := range cards {
		var named int
		for _, promoType := range card.PromoTypes {
			if SlugDescribes(wording, promoType) {
				named++
			}
		}
		if named == 0 {
			continue
		}
		rest := len(card.PromoTypes) - named
		if named > most || (named == most && rest < unnamed) {
			best, most, unnamed = nil, named, rest
		}
		if named == most && rest == unnamed {
			best = append(best, card)
		}
	}
	best = containingLabel(wording, best)
	return best
}

// PromoTypeLabel spells a promo type the way it was written before it became
// a token, falling back on the token itself where no fuller spelling was
// kept. Callers displaying a promo type should ask for this rather than
// title-casing the token, which cannot put back the spaces it dropped.
func (b *Backend) PromoTypeLabel(promoType string) string {
	if label := b.PromoTypeLabels[promoType]; label != "" {
		return label
	}
	return Title(promoType)
}
