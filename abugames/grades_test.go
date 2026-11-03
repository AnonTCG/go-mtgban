package abugames

import (
	"slices"
	"testing"

	"github.com/mtgban/go-mtgban/mtgban"
)

// TestFloorGrades pins that each grade is filed at the best offer at it or
// below it, for its own trader alone.
func TestFloorGrades(t *testing.T) {
	cash, credit := availableTraderNames[0], availableTraderNames[1]
	type offer struct {
		trader string
		cond   mtgban.Condition
		price  float64
	}
	tests := []struct {
		name   string
		listed []offer
		want   []offer
	}{
		{
			// The Secret Lair Swamp 1480 foil on 2026-11-02, bought at PLD,
			// MP for a foil, and HP.
			"foil with no nm",
			[]offer{{cash, mtgban.MP, 1.33}, {cash, mtgban.HP, 1.2}, {credit, mtgban.MP, 1.66}},
			[]offer{
				{cash, mtgban.NM, 1.33}, {credit, mtgban.NM, 1.66},
				{cash, mtgban.SP, 1.33}, {credit, mtgban.SP, 1.66},
				{cash, mtgban.MP, 1.33}, {credit, mtgban.MP, 1.66},
				{cash, mtgban.HP, 1.2},
			},
		},
		{
			// The grade a non-foil's PLD skips, paid at HP.
			"non-foil gap",
			[]offer{{cash, mtgban.NM, 10}, {cash, mtgban.SP, 8}, {cash, mtgban.HP, 5}},
			[]offer{{cash, mtgban.NM, 10}, {cash, mtgban.SP, 8}, {cash, mtgban.MP, 5}, {cash, mtgban.HP, 5}},
		},
		{
			// The Chain Veil, Commander Masters 943, on 2026-11-02: every
			// played grade paid more than NM, so NM takes the SP offer.
			"better grade paid less",
			[]offer{{cash, mtgban.NM, 3.86}, {cash, mtgban.SP, 5.9}, {cash, mtgban.HP, 5.25}},
			[]offer{{cash, mtgban.NM, 5.9}, {cash, mtgban.SP, 5.9}, {cash, mtgban.MP, 5.25}, {cash, mtgban.HP, 5.25}},
		},
		{
			"every grade listed in order",
			[]offer{{cash, mtgban.NM, 10}, {cash, mtgban.SP, 8}, {cash, mtgban.MP, 6}, {cash, mtgban.HP, 4}},
			[]offer{{cash, mtgban.NM, 10}, {cash, mtgban.SP, 8}, {cash, mtgban.MP, 6}, {cash, mtgban.HP, 4}},
		},
	}
	for _, tt := range tests {
		abu := &ABUGames{buylist: mtgban.BuylistRecord{}}
		for _, o := range tt.listed {
			err := abu.buylist.AddRelaxed("card", &mtgban.BuylistEntry{VendorName: o.trader, Conditions: o.cond, BuyPrice: o.price, Quantity: 15})
			if err != nil {
				t.Fatal(err)
			}
		}

		abu.floorGrades()

		entries := abu.buylist["card"]
		if len(entries) != len(tt.want) {
			t.Errorf("%s: got %v; want %v", tt.name, entries, tt.want)
			continue
		}
		for _, want := range tt.want {
			found := slices.ContainsFunc(entries, func(e mtgban.BuylistEntry) bool {
				return e.VendorName == want.trader && e.Conditions == want.cond && e.BuyPrice == want.price
			})
			if !found {
				t.Errorf("%s: no %s %s offer at %v in %v", tt.name, want.trader, want.cond, want.price, entries)
			}
		}
	}
}
