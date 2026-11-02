package pokemon

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// aliasBackend holds the set names the cases below need, indexed the way
// AdjustEdition asks for them. Only the set index is consulted, so the
// entries need nothing but their name.
func aliasBackend(t *testing.T, names ...string) *matcher.Backend {
	t.Helper()
	b := &matcher.Backend{NormalizedSets: map[string]*matcher.Set{}}
	for _, name := range names {
		b.NormalizedSets[matcher.Normalize(name)] = &matcher.Set{Name: name}
	}
	return b
}

// TestAdjustEditionAliases pins that the table only ever answers for an
// edition no set is named by. A rewrite of a name the datastore carries
// would move every printing under it onto another set.
func TestAdjustEditionAliases(t *testing.T) {
	b := aliasBackend(t,
		"Prize Pack Series Cards",
		"Trading Card Game Classic",
		"Trick or Trade BOOster Bundle",
		"WoTC Promo",
	)

	tests := []struct {
		name    string
		edition string
		want    string
	}{
		{"a series files under the shared set", "Play! Pokémon Prize Pack Series Two", "Prize Pack Series Cards"},
		{"a publisher name the catalog abbreviates", "Wizards Black Star Promos", "WoTC Promo"},
		{"a deck the catalog collects in one set", "Pokémon Trading Card Game Classic: Venusaur & Lugia ex Deck", "Trading Card Game Classic"},
		{"the wordplay the storefronts drop", "Trick or Trade", "Trick or Trade BOOster Bundle"},
		{"a set the datastore carries is never rewritten", "Trick or Trade BOOster Bundle", "Trick or Trade BOOster Bundle"},
		{"an edition naming nothing is left as it is", "Some Storefront Heading", "Some Storefront Heading"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inCard := &matcher.InputCard{Edition: test.edition}
			Rules{}.AdjustEdition(b, inCard)
			if inCard.Edition != test.want {
				t.Errorf("AdjustEdition(%q) = %q, want %q", test.edition, inCard.Edition, test.want)
			}
		})
	}
}

// TestEditionAliasesAreDistinct pins that no two spellings in the table
// normalize to one key, which would leave whichever the map iterated last
// silently deciding for both.
func TestEditionAliasesAreDistinct(t *testing.T) {
	seen := map[string]string{}
	for name := range editionAliases {
		normalized := matcher.Normalize(name)
		if other, found := seen[normalized]; found {
			t.Errorf("%q and %q both normalize to %q", name, other, normalized)
		}
		seen[normalized] = name
	}
	if len(normalizedEditionAliases()) != len(editionAliases) {
		t.Errorf("indexed %d aliases of %d", len(normalizedEditionAliases()), len(editionAliases))
	}
}

// TestEditionAliasesNameRealSets pins the table against the datastore: every
// value has to be a set carried under exactly that name, and every key must
// not be, or the alias would shadow a real set's own printings.
func TestEditionAliasesNameRealSets(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	for name, set := range editionAliases {
		if _, found := b.NormalizedSets[matcher.Normalize(set)]; !found {
			t.Errorf("%q maps to %q, which names no set", name, set)
		}
		if _, found := b.NormalizedSets[matcher.Normalize(name)]; found {
			t.Errorf("%q is a set of its own and must not be aliased", name)
		}
	}
}
