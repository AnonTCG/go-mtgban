package magic

import "testing"

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

		// Nothing about the name says it here.
		{"Secret Lair Drop Wild in Bloom", false},
	} {
		got := sealedHoldsOnlyFoils(SealedProduct{Name: tc.name})
		if got != tc.want {
			t.Errorf("sealedHoldsOnlyFoils(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}

	flagged := SealedProduct{Name: "Secret Lair Drop Wild in Bloom", IsFoilOnly: true}
	if !sealedHoldsOnlyFoils(flagged) {
		t.Errorf("sealedHoldsOnlyFoils(%q) with IsFoilOnly = true: got false, want true", flagged.Name)
	}
}
