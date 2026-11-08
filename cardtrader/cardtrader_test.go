package cardtrader

import (
	"maps"
	"testing"

	"github.com/mtgban/go-mtgban/mtgban"
)

// TestAddCheapestOfferPerSeller pins that each storefront keeps its cheapest
// offer per condition, holding the copies of every offer of its own, and that
// Zero and 1 Day Ready, both bundle listings, keep entries of their own.
func TestAddCheapestOfferPerSeller(t *testing.T) {
	inventory := mtgban.InventoryRecord{}
	for _, entry := range []mtgban.InventoryEntry{
		{Price: 10, Quantity: 2, SellerName: availableMarketNames[0]},
		{Price: 11, Quantity: 3, SellerName: availableMarketNames[0]},
		{Price: 12, Quantity: 1, SellerName: availableMarketNames[0]},
		{Price: 13, Quantity: 4, SellerName: availableMarketNames[1], Bundle: true},
		{Price: 15, Quantity: 1, SellerName: availableMarketNames[2], Bundle: true},
		{Price: 14, Quantity: 2, SellerName: availableMarketNames[1], Bundle: true},
	} {
		entry.Conditions = mtgban.NM
		addCheapestOffer(inventory, resultChan{cardID: "uuid", invEntry: &entry}, t.Logf)
	}

	prices := map[string]float64{}
	available := map[string]int{}
	for _, entry := range inventory["uuid"] {
		prices[entry.SellerName] = entry.Price
		available[entry.SellerName] = entry.Available
	}
	wantPrices := map[string]float64{
		availableMarketNames[0]: 10,
		availableMarketNames[1]: 13,
		availableMarketNames[2]: 15,
	}
	wantAvailable := map[string]int{
		availableMarketNames[0]: 6,
		availableMarketNames[1]: 6,
		availableMarketNames[2]: 1,
	}
	if len(inventory["uuid"]) != len(wantPrices) || !maps.Equal(prices, wantPrices) {
		t.Errorf("got %v, want %v", inventory["uuid"], wantPrices)
	}
	if !maps.Equal(available, wantAvailable) {
		t.Errorf("available %v, want %v", available, wantAvailable)
	}
}

// TestAddCheapestOfferAcrossBlueprints pins that two blueprints landing on one
// card keep the cheaper offer whichever arrives first, with that offer's own
// link and ids, and the entries stay sorted cheapest first.
func TestAddCheapestOfferAcrossBlueprints(t *testing.T) {
	offers := []mtgban.InventoryEntry{
		{Price: 9, Quantity: 1, SellerName: availableMarketNames[1], Bundle: true, OriginalID: "1", InstanceID: "10", URL: "z"},
		{Price: 12, Quantity: 2, SellerName: availableMarketNames[0], OriginalID: "75164", InstanceID: "100", URL: "a"},
		{Price: 13, Quantity: 1, SellerName: availableMarketNames[0], OriginalID: "75164", InstanceID: "101", URL: "a"},
		{Price: 8, Quantity: 3, SellerName: availableMarketNames[0], OriginalID: "69769", InstanceID: "200", URL: "b"},
		{Price: 8, Quantity: 4, SellerName: availableMarketNames[0], OriginalID: "75164", InstanceID: "102", URL: "a"},
	}
	for _, order := range [][]int{{0, 1, 2, 3, 4}, {0, 3, 4, 1, 2}, {4, 3, 2, 1, 0}} {
		inventory := mtgban.InventoryRecord{}
		for _, i := range order {
			entry := offers[i]
			entry.Conditions = mtgban.NM
			addCheapestOffer(inventory, resultChan{cardID: "uuid", invEntry: &entry}, t.Logf)
		}
		entries := inventory["uuid"]
		if len(entries) != 2 {
			t.Fatalf("order %v: got %d entries, want 2", order, len(entries))
		}
		got := entries[0]
		if got.SellerName != availableMarketNames[0] || got.Price != 8 || got.InstanceID != "200" ||
			got.OriginalID != "69769" || got.URL != "b" || got.Quantity != 3 || got.Available != 10 {
			t.Errorf("order %v: first entry %+v, want blueprint 69769's offer at 8 holding 10", order, got)
		}
		if entries[1].Price != 9 {
			t.Errorf("order %v: second entry %+v, want the Zero offer at 9", order, entries[1])
		}
	}
}

func TestNotTheListedCopy(t *testing.T) {
	tests := []struct {
		description string
		want        bool
	}{
		{"New Professional Store in Italy", false},
		{"Dienstags und Freitags", false},
		{"more foil from IXALAN", false},
		{"Warning: Mixed Languages", true},
		{"TSP ITA", true},
		{"MIX3", true},
		{"bymix", false},
		{"", false},
	}
	for _, tt := range tests {
		got := notTheListedCopy(tt.description)
		if got != tt.want {
			t.Errorf("notTheListedCopy(%q) = %t, want %t", tt.description, got, tt.want)
		}
	}
}
