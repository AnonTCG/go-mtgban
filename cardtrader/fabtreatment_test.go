package cardtrader

import (
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
)

// TestFabListingID pins that a Flesh and Blood listing lands only on a
// printing in its own treatment, whatever the print run: Volzar's Regular
// copies are not its Cold Foil token (ROS021), since a Normal printing
// carries that number as a face. A treatment no printing at the number is
// sold in names nothing, so the blueprint's own product stands, unless a
// promo printing was reached by name rather than by that product. A sibling
// whose product no blueprint carries is the listing's own: Hex Gauntlet's
// only Cold Foil is its extended art, which nothing else lists. Two such
// siblings name neither: Star Fall // Volzar's Normal has a Cold Foil at
// each of its numbers.
func TestFabListingID(t *testing.T) {
	b := &matcher.Backend{UUIDs: map[string]*matcher.CardObject{
		"ros021_coldfoil":           {Card: matcher.Card{UUID: "ros021_coldfoil", Finish: "Cold Foil", Number: "ROS021", Identifiers: map[string]string{"tcgplayerProductId": "565399"}}, Foil: true},
		"ros009_584673_coldfoil":    {Card: matcher.Card{UUID: "ros009_584673_coldfoil", Finish: "Cold Foil", Number: "ROS009", Identifiers: map[string]string{"tcgplayerProductId": "584673"}}, Foil: true},
		"ros009_ros021_normal":      {Card: matcher.Card{Finish: "Normal", Number: "ROS009//ROS021"}},
		"her009_1steditioncoldfoil": {Card: matcher.Card{Finish: "1st Edition Cold Foil", Number: "HER009"}, Foil: true},
		"wtr001_normal":             {Card: matcher.Card{Finish: "Normal", Number: "WTR001"}},
		"wtr001_unlimitednormal":    {Card: matcher.Card{Finish: "Unlimited Edition Normal", Number: "WTR001"}},
		"iar004_706647":             {Card: matcher.Card{Finish: "Normal", Number: "IAR004", Identifiers: map[string]string{"tcgplayerProductId": "706647"}}},
		"iar004_724676_coldfoil":    {Card: matcher.Card{UUID: "iar004_724676_coldfoil", Finish: "Cold Foil", Number: "IAR004", PromoTypes: []string{"extendedart"}, Identifiers: map[string]string{"tcgplayerProductId": "724676"}}, Foil: true},
		"sea245_rainbowfoil":        {Card: matcher.Card{Finish: "Rainbow Foil", Number: "SEA245", PromoTypes: []string{"treasure"}, Identifiers: map[string]string{"tcgplayerProductId": "633612"}}, Foil: true},
		"puzzle_rainbowfoil":        {Card: matcher.Card{Name: "Macro Puzzle Card", Finish: "Rainbow Foil", Identifiers: map[string]string{"tcgplayerProductId": "688781"}}, Foil: true},
		"artcard_normal":            {Card: matcher.Card{Name: "Art Card", Finish: "Normal", Identifiers: map[string]string{"tcgplayerProductId": "618220"}}},
	}}
	b.AllUUIDs = []string{"ros021_coldfoil", "ros009_584673_coldfoil", "ros009_ros021_normal", "her009_1steditioncoldfoil", "wtr001_normal", "wtr001_unlimitednormal", "iar004_706647", "iar004_724676_coldfoil", "sea245_rainbowfoil", "puzzle_rainbowfoil", "artcard_normal"}
	siblings := fabNumbers(b)
	for _, tc := range []struct {
		cardID, treatment string
		tcgID             int
		listed            map[string]bool
		want              string
	}{
		{"ros021_coldfoil", "Regular", 0, nil, ""},
		{"ros021_coldfoil", "Cold Foil", 0, nil, "ros021_coldfoil"},
		{"ros021_coldfoil", "Gold Foil", 0, nil, "ros021_coldfoil"},
		{"her009_1steditioncoldfoil", "Cold Foil", 0, nil, "her009_1steditioncoldfoil"},
		{"wtr001_normal", "", 0, nil, "wtr001_normal"},
		{"wtr001_normal", "Rainbow Foil", 0, nil, "wtr001_normal"},
		{"wtr001_normal", "Cold Foil", 0, nil, "wtr001_normal"},
		{"wtr001_unlimitednormal", "Regular", 0, nil, "wtr001_unlimitednormal"},
		{"sea245_rainbowfoil", "Regular", 0, nil, ""},
		{"sea245_rainbowfoil", "Regular", 633612, nil, "sea245_rainbowfoil"},
		{"iar004_706647", "Cold Foil", 706647, nil, "iar004_724676_coldfoil"},
		{"iar004_706647", "Cold Foil", 706647, map[string]bool{"724676": true}, ""},
		{"ros009_ros021_normal", "Cold Foil", 0, nil, ""},
		{"puzzle_rainbowfoil", "Regular", 688781, nil, "puzzle_rainbowfoil"},
		{"artcard_normal", "Regular", 0, nil, "artcard_normal"},
		{"unknown", "Regular", 0, nil, ""},
	} {
		var p Product
		p.Properties.FabFoilNew = tc.treatment
		got := fabListingID(b, siblings, tc.listed, tc.cardID, tc.tcgID, p)
		if got != tc.want {
			t.Errorf("%q listed %q: lands on %q, want %q", tc.cardID, tc.treatment, got, tc.want)
		}
	}
}
