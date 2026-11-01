package magic

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// derivedTokenPairSuffix marks a uuid minted here for a two-sided token
// pairing TCGplayer sells as one product (mtgjson only describes those via
// tokenProducts on each single face). Derived cards reach Match by
// TCGplayer id only - the combined name is not unique in-set.
const derivedTokenPairSuffix = "_tp_"

// pairKey identifies a physical pairing by its two uuids, always ordered
// a < b so the same pairing hashes the same way regardless of which face's
// tokenProducts entry it was read from.
type pairKey struct {
	a, b string
}

func newPairKey(u1, u2 string) pairKey {
	if u1 < u2 {
		return pairKey{u1, u2}
	}
	return pairKey{u2, u1}
}

// deriveTokenPairs mints one combined Card per unique physical pairing
// tokenProducts describe. tcgIDs refuses ids a real printing already owns
// and records each derived entity's own ids at its base sibling.
func deriveTokenPairs(sets map[string]*Set, uuids map[string]*matcher.CardObject, tcgIDs map[string]string) []Card {
	// pairIDs / idPairs: an id claimed by more than one pairing is
	// ambiguous - refuse it (same rule as crossSetProductIDs).
	pairIDs := map[pairKey]map[string]bool{}
	idPairs := map[string]map[pairKey]bool{}

	// Tokens may already be merged into Cards; read both slices but
	// skip a uuid already visited so tokenProducts is not double-counted.
	seenCard := map[string]bool{}
	for _, set := range sets {
		for _, group := range [2][]Card{set.Cards, set.Tokens} {
			for _, card := range group {
				if card.UUID != "" {
					if seenCard[card.UUID] {
						continue
					}
					seenCard[card.UUID] = true
				}
				for _, tp := range card.TokenProducts {
					if len(tp.TokenParts) != 2 {
						continue
					}
					id := tp.Identifiers["tcgplayerProductId"]
					u1, u2 := tp.TokenParts[0].UUID, tp.TokenParts[1].UUID
					if id == "" || u1 == "" || u2 == "" {
						continue
					}
					if u1 == u2 {
						continue
					}

					key := newPairKey(u1, u2)
					if pairIDs[key] == nil {
						pairIDs[key] = map[string]bool{}
					}
					pairIDs[key][id] = true
					if idPairs[id] == nil {
						idPairs[id] = map[pairKey]bool{}
					}
					idPairs[id][key] = true
				}
			}
		}
	}

	// Deterministic order: map iteration would make a re-run's derived
	// uuid set identical but its log/slice order arbitrary.
	keys := make([]pairKey, 0, len(pairIDs))
	for key := range pairIDs {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].a != keys[j].a {
			return keys[i].a < keys[j].a
		}
		return keys[i].b < keys[j].b
	})

	// idCanonicalKey picks one pairKey for a tcgplayerProductId claimed by
	// several keys that name the same two faces (memorabilia/oversized
	// siblings repeating a base set's id). Prefer non-memorabilia faces so
	// vendor listings anchored on the ordinary set still hit the index.
	// Different face-name pairs stay refused (no entry). Face-name agreement
	// alone is the test; no false collapse has been seen in real data.
	idCanonicalKey := map[string]pairKey{}
	for id, claimants := range idPairs {
		if len(claimants) <= 1 {
			continue
		}
		sorted := make([]pairKey, 0, len(claimants))
		for key := range claimants {
			sorted = append(sorted, key)
		}
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].a != sorted[j].a {
				return sorted[i].a < sorted[j].a
			}
			return sorted[i].b < sorted[j].b
		})

		var namePair [2]string
		consistent := true
		var best pairKey
		bestScore := -1
		for i, key := range sorted {
			co1, found1 := uuids[key.a]
			co2, found2 := uuids[key.b]
			if !found1 || !found2 {
				consistent = false
				break
			}
			n1, n2 := co1.Card.Name, co2.Card.Name
			if n2 < n1 {
				n1, n2 = n2, n1
			}
			if i == 0 {
				namePair = [2]string{n1, n2}
			} else if namePair != [2]string{n1, n2} {
				consistent = false
				break
			}
			score := 0
			if !isMemorabiliaSet(sets, co1.SetCode) {
				score++
			}
			if !isMemorabiliaSet(sets, co2.SetCode) {
				score++
			}
			if score > bestScore {
				bestScore = score
				best = key
			}
		}
		if consistent {
			idCanonicalKey[id] = best
		}
	}

	var derived []Card
	for _, key := range keys {
		co1, found1 := uuids[key.a]
		co2, found2 := uuids[key.b]
		if !found1 || !found2 {
			continue
		}
		if co1.Layout == "double_faced_token" || co2.Layout == "double_faced_token" {
			continue
		}
		if !isTokenPairLayout(co1.Layout) || !isTokenPairLayout(co2.Layout) {
			continue
		}

		// A usable external id is optional: empty usableIDs still mint the
		// pairing; tokenProducts already confirmed it is real.
		var usableIDs []int
		for id := range pairIDs[key] {
			if len(idPairs[id]) > 1 {
				if canon, ok := idCanonicalKey[id]; !ok || canon != key {
					continue
				}
			}
			if _, claimed := tcgIDs[id]; claimed {
				continue
			}
			n, err := strconv.Atoi(id)
			if err != nil {
				continue
			}
			usableIDs = append(usableIDs, n)
		}
		sort.Ints(usableIDs)

		// Fall back to the first face's set when faces share no ancestor;
		// tokenProducts already confirmed the pairing, and home is cosmetic
		// for buildDerivedCard.
		home := homeSet(sets, co1.SetCode, co2.SetCode)
		if home == "" {
			home = tokenSetCodeOf(sets, co1.SetCode)
		}

		card := buildDerivedCard(co1, co2, home, usableIDs)
		derived = append(derived, card)

		for _, n := range usableIDs {
			id := strconv.Itoa(n)
			if _, claimed := tcgIDs[id]; !claimed {
				tcgIDs[id] = card.UUID
			}
		}
	}

	return derived
}

// tcgProductKeys are the identifiers that give a printing a TCGplayer product.
var tcgProductKeys = [...]string{"tcgplayerProductId", "tcgplayerEtchedProductId", "tcgplayerAlternativeFoilProductId"}

// ownTokenProducts returns the distinct product ids tokenProducts names as
// selling this printing alone: a single-part entry whose part is the
// printing itself.
func ownTokenProducts(card *Card) []string {
	var ids []string
	for _, tp := range card.TokenProducts {
		if len(tp.TokenParts) != 1 || tp.TokenParts[0].UUID != card.UUID {
			continue
		}
		id := tp.Identifiers["tcgplayerProductId"]
		if id != "" && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids
}

// adoptOwnTokenProducts gives a printing with no TCGplayer product the one
// product tokenProducts says sells it alone, so the id reaches it the way
// any other product id does. It is adopted only when nothing is in doubt:
// the printing names exactly one such product, no other printing names
// that product as its own, and no printing or sealed product carries it
// under any id.
func adoptOwnTokenProducts(sets map[string]*Set) {
	owned := map[string]bool{}
	claimants := map[string]int{}
	for _, set := range sets {
		for _, product := range set.SealedProduct {
			for _, key := range tcgProductKeys {
				id := product.Identifiers[key]
				if id != "" {
					owned[id] = true
				}
			}
		}
		for i := range set.Cards {
			card := &set.Cards[i]
			for _, key := range tcgProductKeys {
				id := card.Identifiers[key]
				if id != "" {
					owned[id] = true
				}
			}
			for _, id := range ownTokenProducts(card) {
				claimants[id]++
			}
		}
	}

	for _, set := range sets {
		for i := range set.Cards {
			card := &set.Cards[i]
			if card.Identifiers["tcgplayerProductId"] != "" {
				continue
			}
			ids := ownTokenProducts(card)
			if len(ids) != 1 || claimants[ids[0]] != 1 || owned[ids[0]] {
				continue
			}
			if card.Identifiers == nil {
				card.Identifiers = map[string]string{}
			}
			card.Identifiers["tcgplayerProductId"] = ids[0]
		}
	}
}

// isTokenPairLayout is an allowlist (layout is upstream-extensible).
// Excludes art_series and reversible_card (only self-paired instances
// found); both stay as an explicit backstop, not dead code.
func isTokenPairLayout(layout string) bool {
	return layout == "token" || layout == "emblem"
}

// homeSet picks the release both faces belong to when they disagree on
// set: walk ParentCode chains to the first shared code (Commander/bonus/
// promo sheets point back to the main set).
func homeSet(sets map[string]*Set, setA, setB string) string {
	if setA == setB {
		return tokenSetCodeOf(sets, setA)
	}

	chainA := parentChain(sets, setA)
	seen := make(map[string]bool, len(chainA))
	for _, code := range chainA {
		seen[code] = true
	}
	for _, code := range parentChain(sets, setB) {
		if seen[code] {
			return tokenSetCodeOf(sets, code)
		}
	}
	return ""
}

// tokenSetCodeOf resolves a set code to the code its tokens are filed
// under, falling back to the code itself when the set is unknown or names
// no token sheet of its own.
func tokenSetCodeOf(sets map[string]*Set, code string) string {
	if set, found := sets[code]; found && set.TokenSetCode != "" {
		return set.TokenSetCode
	}
	return code
}

// isMemorabiliaSet reports a memorabilia-type set (oversized/reference
// sibling). Vendors resolve listings through the ordinary set, not these.
func isMemorabiliaSet(sets map[string]*Set, code string) bool {
	set, found := sets[code]
	return found && set.Type == "memorabilia"
}

// parentChain walks a set's ParentCode links up to the root, returning the
// codes visited starting with code itself. A cycle stops the walk rather
// than looping forever - defensively, mtgjson has never introduced one.
func parentChain(sets map[string]*Set, code string) []string {
	chain := []string{code}
	seen := map[string]bool{code: true}
	for {
		set, found := sets[code]
		if !found || set.ParentCode == "" || seen[set.ParentCode] {
			return chain
		}
		code = set.ParentCode
		chain = append(chain, code)
		seen[code] = true
	}
}

// buildDerivedCard builds the combined Card. usableIDs may be empty:
// tokenProducts already confirms the pairing; a TCGplayer id is optional.
func buildDerivedCard(co1, co2 *matcher.CardObject, home string, usableIDs []int) Card {
	uuidLo, uuidHi := co1.UUID, co2.UUID
	if uuidHi < uuidLo {
		uuidLo, uuidHi = uuidHi, uuidLo
	}

	// The uuid is built from lexical order so it survives mtgjson
	// renumbering a set; the display order below is independent and is
	// chosen to read like a real card, lower collector number first.
	first, second := co1, co2
	if numberLess(second.Number, first.Number) {
		first, second = second, first
	}

	name := first.Name + " // " + second.Name
	// Number may mix two sets' numbering on a cross-set pairing - cosmetic
	// only; derived cards are excluded from every name/number index.
	number := first.Number + " // " + second.Number
	uuid := uuidLo + derivedTokenPairSuffix + uuidHi

	ids := make([]string, len(usableIDs))
	for i, n := range usableIDs {
		ids[i] = strconv.Itoa(n)
	}

	identifiers := map[string]string{
		"derivedTokenPair": "true",
		"tokenPairPartA":   uuidLo,
		"tokenPairPartB":   uuidHi,
		"tokenSetCode":     home,
	}
	if len(ids) > 0 {
		identifiers["tcgplayerProductId"] = ids[0]
		identifiers["tcgplayerProductIds"] = strings.Join(ids, ",")
	}

	images := map[string]string{}
	if scryfallID := first.Identifiers["scryfallId"]; len(scryfallID) > 1 {
		for version, size := range map[string]string{"full": "normal", "thumbnail": "small", "crop": "art_crop"} {
			images[version] = scryfallImageURL(scryfallID, size)
		}
	}

	return Card{
		Name:        name,
		Number:      number,
		PlainNumber: plainNumber(number),
		SetCode:     home,
		Layout:      "token",
		Rarity:      "token",
		PromoTypes:  []string{PromoTypeToken},
		Language:    "English",
		Finishes:    unionFinishes(co1.Finishes, co2.Finishes),
		Identifiers: identifiers,
		Images:      images,
		UUID:        uuid,
	}
}

// scryfallImageURL mirrors generateImageURL, which takes this package's own
// Card rather than the matcher.CardObject a derived entity is built
// from; a derived entity has no scryfallId of its own; the image shown is
// honestly one face's, not a composite.
func scryfallImageURL(scryfallID, version string) string {
	return fmt.Sprintf("https://cards.scryfall.io/%s/front/%c/%c/%s.jpg", version, scryfallID[0], scryfallID[1], scryfallID)
}

// numberLess orders two collector numbers the way a person would: by their
// leading numeric run first, falling back to a plain string compare for
// numbers that do not start with (or share) one.
func numberLess(a, b string) bool {
	na, oka := leadingNumber(a)
	nb, okb := leadingNumber(b)
	if oka && okb && na != nb {
		return na < nb
	}
	return a < b
}

func leadingNumber(s string) (int, bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(s[:i])
	return n, err == nil
}

// unionFinishes keeps nonfoil/foil only (token sheets are never etched/
// signed). Union is safer than intersection: MatchIDFinish errors if a
// finish is missing; a too-wide set is simply never asked about.
func unionFinishes(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range [2][]string{a, b} {
		for _, finish := range list {
			if finish != matcher.FinishNonfoil && finish != matcher.FinishFoil {
				continue
			}
			if seen[finish] {
				continue
			}
			seen[finish] = true
			out = append(out, finish)
		}
	}
	sort.Strings(out)
	return out
}

// tokenPairIndicesData holds byFace, byUUIDPair, and byBothNames from one
// walk of derived pairings (shared input, three key shapes).
type tokenPairIndicesData struct {
	byFace map[string]map[string]string

	// byUUIDPair: both faces already anchored by uuid. Blank on collision
	// (refuse, don't last-write-wins) even though deriveTokenPairs should
	// already have deduped the unordered pair.
	byUUIDPair map[[2]string]string

	// byBothNames: neither face anchored by identity. Blank on collision -
	// generic names like "Soldier // Spirit" recur across sheets.
	byBothNames map[[2]string]string
}

// buildTokenPairIndices builds byFace / byUUIDPair / byBothNames once per
// Load. Colliding (face, partner-name) slots blank rather than last-write-
// wins so a vendor listing never resolves to the wrong pairing id. Indexed
// on the Backend itself: two datastores (or a reload) hold different
// pairings/uuids.
func buildTokenPairIndices(b *matcher.Backend) tokenPairIndicesData {
	byFace := map[string]map[string]string{}
	byUUIDPair := map[[2]string]string{}
	byUUIDPairAmbiguous := map[[2]string]bool{}
	byBothNames := map[[2]string]string{}
	byBothNamesAmbiguous := map[[2]string]bool{}
	ambiguous := map[string]map[string]bool{}

	addPairing := func(face, partnerName, id string) {
		key := NormalizeTokenFace(partnerName)
		if ambiguous[face] == nil {
			ambiguous[face] = map[string]bool{}
		}
		if ambiguous[face][key] {
			return
		}
		if byFace[face] == nil {
			byFace[face] = map[string]string{}
		}
		if existing, found := byFace[face][key]; found {
			if existing == id {
				return
			}
			delete(byFace[face], key)
			ambiguous[face][key] = true
			return
		}
		byFace[face][key] = id
	}

	seen := map[string]bool{}
	for uuid, co := range b.UUIDs {
		if co.Identifiers["derivedTokenPair"] != "true" || seen[uuid] {
			continue
		}
		seen[uuid] = true

		partA := co.Identifiers["tokenPairPartA"]
		partB := co.Identifiers["tokenPairPartB"]
		coA, errA := b.GetUUID(partA)
		coB, errB := b.GetUUID(partB)
		// No tcgplayerProductId: index the base (nonfoil) derived uuid
		// buildDerivedCard minted, not this loop's foil/nonfoil sibling
		// uuid - otherwise iteration order would publish two different
		// ids for one physical pairing.
		id := co.Identifiers["tcgplayerProductId"]
		if id == "" && partA != "" && partB != "" {
			baseLo, baseHi := partA, partB
			if baseHi < baseLo {
				baseLo, baseHi = baseHi, baseLo
			}
			id = baseLo + derivedTokenPairSuffix + baseHi
		}

		if errA == nil && errB == nil {
			addPairing(partA, coB.Card.Name, id)
			addPairing(partB, coA.Card.Name, id)
		}

		if partA != "" && partB != "" {
			key := [2]string{partA, partB}
			if partB < partA {
				key = [2]string{partB, partA}
			}
			if byUUIDPairAmbiguous[key] {
				continue
			}
			if existing, found := byUUIDPair[key]; found && existing != id {
				delete(byUUIDPair, key)
				byUUIDPairAmbiguous[key] = true
				continue
			}
			byUUIDPair[key] = id
		}

		if errA == nil && errB == nil {
			faceA, faceB := NormalizeTokenFace(coA.Card.Name), NormalizeTokenFace(coB.Card.Name)
			key := [2]string{faceA, faceB}
			if faceB < faceA {
				key = [2]string{faceB, faceA}
			}
			if byBothNamesAmbiguous[key] {
				continue
			}
			if existing, found := byBothNames[key]; found && existing != id {
				delete(byBothNames, key)
				byBothNamesAmbiguous[key] = true
				continue
			}
			byBothNames[key] = id
		}
	}
	return tokenPairIndicesData{byFace: byFace, byUUIDPair: byUUIDPair, byBothNames: byBothNames}
}

// NormalizeTokenFace folds a face name for vendor/datastore compare: strip
// braces, artist/variant parentheticals, and a trailing " Token", then
// lower-case. Parenthetical must be stripped before the suffix ("X Token
// (Artist)" does not end in " Token").
func NormalizeTokenFace(name string) string {
	return strings.ToLower(strings.TrimSpace(CleanFaceName(name)))
}

// StripFaceWrapping removes braces and artist/variant parentheticals but
// keeps case and a " Token" suffix - some real Card.Names include it.
// See CleanFaceName to also strip the suffix. Exported for vendor packages
// that anchor each face by set/number themselves.
func StripFaceWrapping(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "{")
	if idx := strings.Index(name, "}"); idx >= 0 {
		name = name[:idx] + name[idx+1:]
	}
	name = strings.TrimSpace(name)
	if idx := strings.Index(name, " ("); idx >= 0 {
		name = name[:idx]
	}
	return strings.TrimSpace(name)
}

// CleanFaceName is StripFaceWrapping plus vendor type suffixes the
// datastore omits (" Token", and SCG's " Dungeon" on dungeon cards).
func CleanFaceName(name string) string {
	name = StripFaceWrapping(name)
	for _, suffix := range []string{" Token", " Dungeon"} {
		if trimmed := strings.TrimSuffix(name, suffix); trimmed != name {
			return trimmed
		}
	}
	return name
}

// SplitTokenPairName splits on "//" first, then "-". "//" wins so vendors
// that never use "-" need no special case to avoid a false split.
func SplitTokenPairName(name string) (first, second string) {
	for _, sep := range []string{" // ", " - "} {
		if before, after, found := strings.Cut(name, sep); found {
			return before, after
		}
	}
	return name, ""
}

// MatchTokenPairing resolves a two-sided listing from one face's external
// id (Scryfall) plus the listing name: id alone can pair with several
// partners; name alone is not set-unique. Returns "" for a foil request
// when the pairing was never sold foil - derived uuids have no separate
// foil identity, so callers must fall back themselves.
func MatchTokenPairing(b *matcher.Backend, externalID, listingName string, foil bool) string {
	if externalID == "" {
		return ""
	}
	first, second := SplitTokenPairName(listingName)
	if second == "" {
		return ""
	}
	uuid := b.ConvertID(matcher.IDSpaceScryfall, externalID)
	if uuid == "" {
		return ""
	}

	// A vendor usually carries the id against the first half of its own
	// listing name, with the second half naming the partner - but not
	// always. Where the id's own real name matches the second half instead,
	// the first half is the partner to look up.
	partner := second
	if co, err := b.GetUUID(uuid); err == nil {
		anchor := NormalizeTokenFace(co.Card.Name)
		if NormalizeTokenFace(second) == anchor && NormalizeTokenFace(first) != anchor {
			partner = first
		}
	}
	id := b.TokenPairIndex[uuid][NormalizeTokenFace(partner)]
	return tokenPairingFinishOK(b, id, foil)
}

// MatchTokenPairingBySetNumber anchors the first face by set+number
// (len==1 or refuse) when the vendor publishes no external id. Callers
// must already trust that (set, number); see MatchTokenPairing for foil.
func MatchTokenPairingBySetNumber(b *matcher.Backend, setCode, number, listingName string, foil bool) string {
	first, second := SplitTokenPairName(listingName)
	if second == "" || strings.Contains(second, " // ") || strings.Contains(second, " - ") {
		return ""
	}
	// Most token Card.Names drop the " Token" suffix a vendor spells, but
	// some carry it as part of their own real name - try both forms
	// rather than assume either.
	for _, face := range []string{StripFaceWrapping(first), CleanFaceName(first)} {
		cards := matchNumberedFace(b, face, setCode, number)
		if len(cards) != 1 {
			continue
		}
		id := b.TokenPairIndex[cards[0].UUID][NormalizeTokenFace(second)]
		if id := tokenPairingFinishOK(b, id, foil); id != "" {
			return id
		}
	}
	return ""
}

// MatchNativeTokenPair matches a listing to mtgjson's own combined
// "X // Y" printing at set+number (not a derived entity). Tries both face
// orders; callers handle finish on the returned uuid.
func MatchNativeTokenPair(b *matcher.Backend, setCode, number, listingName string) string {
	first, second := SplitTokenPairName(listingName)
	if second == "" {
		return ""
	}
	// mtgjson's own combined name carries neither face's " Token" suffix
	// ("Copy // Horror", never "Copy Token // Horror Token"), the same as
	// a derived pairing's own split-apart faces - CleanFaceName strips it
	// alongside the wrapping, case preserved for this exact-match lookup.
	faceA, faceB := CleanFaceName(first), CleanFaceName(second)
	for _, combined := range []string{faceA + " // " + faceB, faceB + " // " + faceA} {
		out := matchNumberedFace(b, combined, setCode, number)
		if len(out) == 1 {
			return out[0].UUID
		}
	}
	return ""
}

// matchNumberedFace is Backend.MatchInSetNumber, widened to also search
// set.Tokens. The loader already merges a token-type set's own Tokens into
// its Cards (so an ordinary set+number search already sees most token
// printings without this), but a token name that collides with a real
// card elsewhere in the game - a small, known set (Shapeshifter, Ninja,
// Ornithopter, Storm Crow, Faerie Dragon, Kobolds of Kher Keep; see
// project todo/magic-token-support.md) - is suffixed " Token" in that
// merged Cards copy to stay unambiguous, while its own Tokens entry keeps
// the plain name every caller here actually searches for (StripFaceWrapping/
// CleanFaceName never re-add the suffix). Cards alone therefore silently
// misses exactly these names; Tokens alone would miss ordinary ones for
// sets whose own tokenSetCode split off before this merge ever ran. A uuid
// appearing in both slices (the common case) is deduplicated rather than
// double-counted into a false ambiguity.
//
// A reversible token card's two faces are each their own mtgjson entry,
// same combined name and number, different uuid - and the loader indexes
// only one side into b.UUIDs, the one Match resolves to. The other is a
// real set.Tokens entry that GetUUID cannot see, not a genuine second
// printing, so it is excluded here the same way rather than read as an
// ambiguous pair.
func matchNumberedFace(b *matcher.Backend, cardName, setCode, number string) []matcher.Card {
	set, found := b.Sets[setCode]
	if !found {
		return nil
	}
	seen := map[string]bool{}
	var out []matcher.Card
	for _, group := range [2][]matcher.Card{set.Cards, set.Tokens} {
		for _, card := range group {
			if card.Name != cardName || card.Number != number {
				continue
			}
			if card.UUID == "" || seen[card.UUID] {
				continue
			}
			if _, indexed := b.UUIDs[card.UUID]; !indexed {
				continue
			}
			seen[card.UUID] = true
			out = append(out, card)
		}
	}
	return out
}

// tokenPairingFinishOK returns id, or "" when foil was requested but the
// pairing was never sold foil (derived uuids have no separate foil id).
func tokenPairingFinishOK(b *matcher.Backend, id string, foil bool) string {
	if id == "" || !foil {
		return id
	}
	// Prefer GetUUID(id) first: id may already be the derived uuid when no
	// TCGplayer product id exists. Numeric tcgplayer ids never collide with
	// the "_tp_"-infixed uuid form.
	uuid := id
	co, err := b.GetUUID(uuid)
	if err != nil {
		uuid = b.ConvertID(matcher.IDSpaceTCGplayer, id)
		if uuid == "" {
			return ""
		}
		co, err = b.GetUUID(uuid)
		if err != nil {
			return ""
		}
	}

	// Finishes on the derived card are the union of both faces, which is
	// too wide here: require both source faces to carry foil, or a vendor
	// foil claim would pass on one face alone.
	coA, errA := b.GetUUID(co.Identifiers["tokenPairPartA"])
	coB, errB := b.GetUUID(co.Identifiers["tokenPairPartB"])
	if errA != nil || errB != nil || !coA.Card.HasFinish("foil") || !coB.Card.HasFinish("foil") {
		return ""
	}
	return id
}

// VerifyTokenPairingFinish is tokenPairingFinishOK for an unverified id
// (e.g. Card Trader's bare tcgplayerId). Always confirm derivedTokenPair
// first - unlike in-file callers, a nonfoil ordinary card id must not pass.
func VerifyTokenPairingFinish(b *matcher.Backend, id string, foil bool) string {
	uuid := b.ConvertID(matcher.IDSpaceTCGplayer, id)
	if uuid == "" {
		return ""
	}
	co, err := b.GetUUID(uuid)
	if err != nil || co.Identifiers["derivedTokenPair"] != "true" {
		return ""
	}
	return tokenPairingFinishOK(b, id, foil)
}

// MatchTokenPairingByUUIDs looks up a pairing both faces already anchored
// by identity. "" means no derived product for that uuid pair, or finish
// refuse (see tokenPairingFinishOK).
func MatchTokenPairingByUUIDs(b *matcher.Backend, uuidA, uuidB string, foil bool) string {
	if uuidA == "" || uuidB == "" {
		return ""
	}
	key := [2]string{uuidA, uuidB}
	if uuidB < uuidA {
		key = [2]string{uuidB, uuidA}
	}
	return tokenPairingFinishOK(b, b.TokenPairIDByUUIDs[key], foil)
}

// EditionTokenSetCode walks ParentCode like homeSet, against the loaded
// backend, for callers with only edition wording (CK/SCG usually embed the
// set code and skip this). "" if no token sheet is found.
func EditionTokenSetCode(b *matcher.Backend, edition string) string {
	set, err := b.GetSetByName(edition)
	if err != nil {
		return ""
	}
	return SetTokenSetCode(b, set.Code)
}

// SetTokenSetCode is EditionTokenSetCode for a caller that already has a
// set code (a vendor's own product code) rather than only edition wording.
func SetTokenSetCode(b *matcher.Backend, code string) string {
	seen := map[string]bool{}
	for !seen[code] {
		seen[code] = true
		s, err := b.GetSet(code)
		if err != nil {
			return ""
		}
		if s.TokenSetCode != "" {
			return s.TokenSetCode
		}
		if s.ParentCode == "" {
			return ""
		}
		code = s.ParentCode
	}
	return ""
}

// MatchTokenPairingByNamesAndEdition trusts both face names (no id
// anchor) plus listingEdition as a second check: byBothNames can hit a
// currently-unique generic name that belongs to a different set than the
// vendor claimed. Refuse when edition disagrees or does not resolve.
func MatchTokenPairingByNamesAndEdition(b *matcher.Backend, listingName, listingEdition string, foil bool) string {
	first, second := SplitTokenPairName(listingName)
	if second == "" {
		return ""
	}
	faceA, faceB := NormalizeTokenFace(first), NormalizeTokenFace(second)
	key := [2]string{faceA, faceB}
	if faceB < faceA {
		key = [2]string{faceB, faceA}
	}
	id := b.TokenPairIDByBothNames[key]
	if id == "" {
		return ""
	}

	wantSet := EditionTokenSetCode(b, listingEdition)
	if wantSet == "" {
		return ""
	}
	uuid := b.ConvertID(matcher.IDSpaceTCGplayer, id)
	co, err := b.GetUUID(uuid)
	if err != nil || !strings.EqualFold(co.SetCode, wantSet) {
		return ""
	}

	return tokenPairingFinishOK(b, id, foil)
}
