package yugioh

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
		"2-Player Starter Set",
		"2025 Mega-Pack",
		"Duelist League Promo",
		"Duelist Pack 11: Crow",
		"Duelist Pack: Crow",
		"Gold Series 2008",
		"Hidden Arsenal 5: Steelswarm Invasion",
		"Legendary Collection Kaiba",
		"Premium Gold: Return of the Bling",
		"Speed Duel: Battle City Box",
		"Speed Duel Decks: Ultimate Predators",
		"Starter Deck 2006",
		"Turbo Pack: Booster Seven",
		"Yu-Gi-Oh! Championship Series 2025 Prize Cards",
	)

	tests := []struct {
		name    string
		edition string
		want    string
	}{
		{"a storefront name the catalog numbers", "Legendary Collection Kaiba Mega Pack", "Legendary Collection Kaiba"},
		{"a deck the catalog files as a speed duel one", "Starter Deck: Ultimate Predators", "Speed Duel Decks: Ultimate Predators"},
		{"the game-name prefix comes off first", "Yu-Gi-Oh! Legendary Collection Kaiba Mega Pack", "Legendary Collection Kaiba"},
		{"and so does the singles suffix", "Legendary Collection Kaiba Mega Pack Singles", "Legendary Collection Kaiba"},
		{"a set the datastore carries is never rewritten", "Duelist Pack: Crow", "Duelist Pack: Crow"},
		{"a subtitle the storefronts drop", "Hidden Arsenal 5", "Hidden Arsenal 5: Steelswarm Invasion"},
		{"a numbered name the catalog subtitles", "Premium Gold 2", "Premium Gold: Return of the Bling"},
		{"a league series files under its own season", "Duelist League Series 10", "Duelist League Series 10 participation card"},
		{"a set name carrying the game name is left whole", "Yu-Gi-Oh! Championship Series 2025 Prize Cards", "Yu-Gi-Oh! Championship Series 2025 Prize Cards"},
		{"an edition naming nothing is left as it is", "Some Storefront Heading", "Some Storefront Heading"},
		{"a speed duel deck headed as a starter deck", "Starter Deck: Speed Duel - Battle City Box", "Speed Duel: Battle City Box"},
		{"the box a set of decks is sold in", "2025 Mega-Pack Bundle", "2025 Mega-Pack"},
		{"the digit the catalog writes as a word", "Two-Player Starter Set", "2-Player Starter Set"},
		{"a booster the catalog numbers in words", "Turbo Pack 7", "Turbo Pack: Booster Seven"},
		{"a series the catalog heads by its year", "Gold Series 1", "Gold Series 2008"},
		{"a deck the catalog heads by its year", "Starter Deck Yu-Gi-Oh! GX", "Starter Deck 2006"},
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

// TestEditionAliasesNameRealSets holds the table to the datastore it
// rewrites onto: every target has to name a set, or the alias hands the
// matcher an edition nothing is filed under and the input ends up worse
// off than the spelling it arrived with. The keys are held to the mirror
// image - a key naming a set of its own is never reached, since
// AdjustEdition asks the backend before the table.
func TestEditionAliasesNameRealSets(t *testing.T) {
	b := gametest.Backend(t, matcher.GameYuGiOh)

	for name, set := range editionAliases {
		if _, found := b.NormalizedSets[matcher.Normalize(set)]; !found {
			t.Errorf("%q maps to %q, which names no set", name, set)
		}
		if _, found := b.NormalizedSets[matcher.Normalize(name)]; found {
			t.Errorf("%q is a set of its own, so the alias never answers", name)
		}
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
