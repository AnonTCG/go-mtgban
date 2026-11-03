package strikezone

import (
	"testing"

	"github.com/mtgban/go-mtgban/mtgban"
)

// TestFillBetterGrades pins that an offer buys every grade above its own at
// its price and through its cart id, and that a grade with an offer of its
// own keeps it.
func TestFillBetterGrades(t *testing.T) {
	type offer struct {
		cond  mtgban.Condition
		price float64
		id    string
	}
	tests := []struct {
		name   string
		listed []offer
		want   []offer
	}{
		{
			// Dark Raichu, Legendary Collection 1st Edition, bought at
			// Light Play for $30.
			"lp only",
			[]offer{{mtgban.SP, 30, "lp"}},
			[]offer{{mtgban.NM, 30, "lp"}, {mtgban.SP, 30, "lp"}},
		},
		{
			"hp only",
			[]offer{{mtgban.HP, 180, "hp"}},
			[]offer{{mtgban.NM, 180, "hp"}, {mtgban.SP, 180, "hp"}, {mtgban.MP, 180, "hp"}, {mtgban.HP, 180, "hp"}},
		},
		{
			"gap between offers",
			[]offer{{mtgban.NM, 50, "nm"}, {mtgban.MP, 20, "mp"}},
			[]offer{{mtgban.NM, 50, "nm"}, {mtgban.SP, 20, "mp"}, {mtgban.MP, 20, "mp"}},
		},
		{
			"every grade listed",
			[]offer{{mtgban.NM, 10, "nm"}, {mtgban.SP, 8, "lp"}, {mtgban.MP, 6, "mp"}, {mtgban.HP, 4, "hp"}},
			[]offer{{mtgban.NM, 10, "nm"}, {mtgban.SP, 8, "lp"}, {mtgban.MP, 6, "mp"}, {mtgban.HP, 4, "hp"}},
		},
		{
			// A better grade priced below a worse one is left as listed.
			"better grade paid less",
			[]offer{{mtgban.NM, 420, "nm"}, {mtgban.SP, 3500, "lp"}},
			[]offer{{mtgban.NM, 420, "nm"}, {mtgban.SP, 3500, "lp"}},
		},
	}
	for _, tt := range tests {
		sz := &Strikezone{buylist: mtgban.BuylistRecord{}}
		for _, o := range tt.listed {
			err := sz.buylist.Add("card", &mtgban.BuylistEntry{Conditions: o.cond, BuyPrice: o.price, Quantity: 2, InstanceID: o.id})
			if err != nil {
				t.Fatal(err)
			}
		}

		sz.fillBetterGrades()

		entries := sz.buylist["card"]
		if len(entries) != len(tt.want) {
			t.Errorf("%s: got %v; want %v", tt.name, entries, tt.want)
			continue
		}
		for i, entry := range entries {
			want := tt.want[i]
			if entry.Conditions != want.cond || entry.BuyPrice != want.price || entry.InstanceID != want.id {
				t.Errorf("%s: entry %d is %s at %v through %q; want %s at %v through %q", tt.name, i,
					entry.Conditions, entry.BuyPrice, entry.InstanceID, want.cond, want.price, want.id)
			}
		}
	}
}
