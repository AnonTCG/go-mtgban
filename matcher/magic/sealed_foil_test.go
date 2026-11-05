package magic

import (
	"strings"
	"testing"
)

// The rule that decides whether a sealed product's contents are all foil.
// Each case is a real product name, since the rule is entirely about how the
// sets happen to be named.
func TestSealedHoldsOnlyFoils(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		// The name says so.
		{"Double Masters Foil Draft Booster Box", true},
		{"Modern Horizons Premium Booster Pack", true},
		{"Zendikar Rising VIP Edition Pack", true},
		{"Zendikar Rising Collector Booster Pack", false},
		// ... unless it says the opposite.
		{"Innistrad Non Foil Booster Box", false},

		// Collector-edition Commander decks, in all three spellings the sets
		// use. Matching "Collector Edition" would catch only the first.
		{"Zendikar Rising Commander Deck Collector Edition", true},
		{"Warhammer 40000 Commander Deck Tyranid Swarm Collectors Edition", true},
		{"Marvel Super Heroes Commander Deck Wakanda Forever Collector's Edition", true},
		{"Warhammer 40000 Commander Deck Tyranid Swarm", false},

		// Lines that are foil throughout are named once, not per product.
		{"From the Vault Dragons", true},
		{"From the Vault Transform", true},
		{"The Lord of the Rings Tales of Middle earth Scene Box The Might of Galadriel", true},
		{"Marvels Spider Man Scene Box Case", true},
		{"SDCC 2016 Zombie Planeswalker Set", true},
		{"SDCC 2013 Black Planeswalkers Set", true},
		// Each half of that rule is load-bearing: without "Planeswalker" it
		// reaches the 2019 Dragons set, and without "SDCC" it reaches every
		// Planeswalker Deck ever bundled. Neither is foil.
		{"SDCC 2019 Dragons Endgame Set", false},
		{"Kaladesh Planeswalker Decks Set of 2", false},
		{"Core Set 2020 Planeswalker Decks Set of 5", false},

		// Everything else is named.
		{"Secret Lair Drop Wild in Bloom", true},
		{"Secret Lair Drop OMG KITTIES", true},
		{"Commanders Arsenal", true},
		{"Commander's Arsenal", true},
		{"Secret Lair Drop International Women's Day 2020", true},
		{"Ponies The Galloping", true},
		// ... and only exactly: this one is a different, mixed product.
		{"Secret Lair Drop Extra Life 2023 Ponies The Galloping 2", false},
		{"Secret Lair Drop Countless Colors", false},
	} {
		if got := sealedHoldsOnlyFoils(tc.name); got != tc.want {
			t.Errorf("sealedHoldsOnlyFoils(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Every name in the table is there because no rule covers it. One a rule
// already catches is dead weight; one duplicated is a merge artifact.
func TestProductsWithOnlyFoilsHasNoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range productsWithOnlyFoils {
		if seen[name] {
			t.Errorf("%q is listed twice", name)
		}
		seen[name] = true
	}
}

// A name a rule already matches does not belong in the table.
func TestProductsWithOnlyFoilsAreNotRuleCovered(t *testing.T) {
	for _, name := range productsWithOnlyFoils {
		for _, rule := range []string{"From the Vault", "Scene Box", "Premium", "VIP Edition"} {
			if strings.Contains(name, rule) {
				t.Errorf("%q is already matched by the %q rule", name, rule)
			}
		}
		if strings.Contains(name, "SDCC") && strings.Contains(name, "Planeswalker") {
			t.Errorf("%q is already matched by the SDCC planeswalker rule", name)
		}
	}
}
