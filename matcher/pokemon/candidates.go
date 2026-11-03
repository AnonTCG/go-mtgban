package pokemon

import (
	"slices"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// CandidateSets admits the sets the edition reaches and the subsets filed
// inside them.
func (r Rules) CandidateSets(b *matcher.Backend, in *matcher.InputCard, editions []string) []string {
	codes := r.editionSets(b, in, editions)
	return append(codes, r.subsetsOf(b, codes)...)
}

// editionSets preserves Pokemon's promo-shelf fallback. An unresolved
// heading such as Plasma Storm Promos with Holo Promo wording must not widen
// straight onto every ordinary printing sharing the collector number. Exact
// editions still win; in the loose pass, promo shelves join edition matches.
func (r Rules) editionSets(b *matcher.Backend, in *matcher.InputCard, editions []string) []string {
	if len(editions) <= 1 || in.PromoWildcard || !isGenericPromo(in) {
		return r.DefaultRules.CandidateSets(b, in, editions)
	}
	var exact, loose []string
	for _, code := range editions {
		set := b.Sets[code]
		if matcher.Equals(set.Name, in.Edition) {
			exact = append(exact, code)
		}
		if matcher.Contains(set.Name, in.Edition) || strings.HasSuffix(set.Name, "Promos") {
			loose = append(loose, code)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	if len(loose) > 0 {
		return loose
	}
	return editions
}

// subsetsOf lists the sets filed inside ones the edition admits, under codes
// of their own. The catalog splits the collections printed inside a set out
// into a set of their own - "Legendary Treasures: Radiant Collection" beside
// "Legendary Treasures", the four Trainer Galleries beside their parents -
// while the storefronts file those cards under the parent, so an edition
// naming the parent reaches the RC- and TG-numbered printings only through
// these.
//
// The suffixed code is not enough on its own: the same shape spells 32
// unrelated sets, from "Burger King Promos" under BKP to every POP series
// and every promo set under PR. The subset's name opening with its parent's
// is what tells the two apart, and it costs nothing to require: every real
// subset spells its parent out.
func (r Rules) subsetsOf(b *matcher.Backend, codes []string) []string {
	index := r.subsets
	if index == nil {
		index = indexSubsets(b)
	}

	var subsets []string
	for _, parent := range codes {
		for _, code := range index[parent] {
			if !slices.Contains(codes, code) {
				subsets = append(subsets, code)
			}
		}
	}
	slices.Sort(subsets)
	return slices.Compact(subsets)
}

// indexSubsets lists the subsets filed inside each set by the set's code.
func indexSubsets(b *matcher.Backend) map[string][]string {
	index := map[string][]string{}
	for code := range b.Sets {
		parent, found := parentOf(b, code)
		if found {
			index[parent] = append(index[parent], code)
		}
	}
	return index
}

// parentOf names the set a subset is filed inside, false for a set that is
// not one.
func parentOf(b *matcher.Backend, code string) (string, bool) {
	parent, _, found := strings.Cut(code, "-")
	if !found {
		return "", false
	}
	set, subset := b.Sets[parent], b.Sets[code]
	return parent, set != nil && subset != nil && strings.HasPrefix(subset.Name, set.Name)
}

// isGenericPromo reports a promo listing with no more specific kind: it says
// Promo, League or Miscellaneous, and is not a prerelease or Comic-Con stamp.
func isGenericPromo(in *matcher.InputCard) bool {
	return !in.Contains("Prerelease") &&
		!in.Contains("SDCC") && !in.Contains("San Diego Comic-Con") &&
		(matcher.Contains(in.Variation, "Promo") ||
			in.Contains("League") ||
			in.Contains("Miscellaneous"))
}
