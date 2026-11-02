package mtgban

import (
	"errors"
	"math/rand"
	"testing"
)

func TestAddRelaxed(t *testing.T) {
	entryNM := InventoryEntry{
		Quantity:   5,
		Available:  7,
		Conditions: NM,
		Price:      20.0,
		URL:        "https://mtgban.com",
		SellerName: "BANNED",
	}
	entrySP := InventoryEntry{
		Quantity:   4,
		Conditions: SP,
		Price:      10.0,
		URL:        "https://mtgban.com",
		SellerName: "BANNED",
	}
	inventory := InventoryRecord{}

	// Empty inventory, add an entry
	err := inventory.AddRelaxed("A", &entryNM)
	if err != nil {
		t.Errorf("FAIL: Unexpected error: %s", err.Error())
		return
	}
	if len(inventory["A"]) == 0 {
		t.Error("FAIL: inventory does not contain entries for A")
		return
	}

	// Add entry with same ID, but it's a different conditions
	err = inventory.AddRelaxed("A", &entrySP)
	if err != nil {
		t.Errorf("FAIL: Unexpected error: %s", err.Error())
		return
	}
	if len(inventory["A"]) != 2 {
		t.Errorf("FAIL: inventory only contains %d entries for A", len(inventory["A"]))
		return
	}

	// Same entry, same id
	err = inventory.AddRelaxed("A", &entryNM)
	if err != nil {
		t.Errorf("FAIL: Unexpected error: %s", err.Error())
		return
	}
	if len(inventory["A"]) != 2 {
		t.Errorf("FAIL: inventory contains a differen number of entries (%d) than expected for A", len(inventory["A"]))
		return
	}
	if inventory["A"][0].Quantity != 10 || inventory["A"][0].Available != 14 {
		t.Error("FAIL: inventory did not merge quantities")
		return
	}

	// Similar but different
	entryNM.SellerName = "NOTBANNED"
	err = inventory.AddRelaxed("A", &entryNM)
	if err != nil {
		t.Errorf("FAIL: Unexpected error: %s", err.Error())
		return
	}
	if len(inventory["A"]) != 3 {
		t.Errorf("FAIL: inventory contains a differen number of entries %d than expected for A", len(inventory["A"]))
		return
	}
	if inventory["A"][0].Quantity != 10 {
		t.Error("FAIL: inventory merged quantities")
		return
	}

	// Similar but different
	entryNM.SellerName = "NOTBANNED"
	err = inventory.AddRelaxed("A", &entryNM)
	if err != nil {
		t.Errorf("FAIL: Unexpected error: %s", err.Error())
		return
	}
	if len(inventory["A"]) != 3 {
		t.Errorf("FAIL: inventory contains a differen number of entries %d than expected for A", len(inventory["A"]))
		return
	}
	if inventory["A"][0].Quantity != 10 {
		t.Error("FAIL: inventory merged quantities")
		return
	}

	t.Log("PASS: AddRelaxed")
}

func TestAdd(t *testing.T) {
	entryNM := InventoryEntry{
		Quantity:   5,
		Conditions: NM,
		Price:      20.0,
		URL:        "https://mtgban.com",
		SellerName: "BANNED",
	}
	entrySP := InventoryEntry{
		Quantity:   4,
		Conditions: SP,
		Price:      10.0,
		URL:        "https://mtgban.com",
		SellerName: "BANNED",
	}
	inventory := InventoryRecord{}

	// Empty inventory, add an entry
	err := inventory.Add("A", &entryNM)
	if err != nil {
		t.Errorf("FAIL: Unexpected error: %s", err.Error())
		return
	}
	if len(inventory["A"]) == 0 {
		t.Error("FAIL: inventory does not contain entries for A")
		return
	}

	// Add entry with same ID, but it's a different conditions
	err = inventory.Add("A", &entrySP)
	if err != nil {
		t.Errorf("FAIL: Unexpected error: %s", err.Error())
		return
	}
	if len(inventory["A"]) != 2 {
		t.Errorf("FAIL: inventory only contains %d entries for A", len(inventory["A"]))
		return
	}

	// Same entry, same id
	err = inventory.Add("A", &entryNM)
	if err == nil {
		t.Error("FAIL: Tried to add the same entry twice")
		return
	}
	if len(inventory["A"]) != 2 {
		t.Errorf("FAIL: inventory contains a differen number of entries (%d) than expected for A", len(inventory["A"]))
		return
	}
	if inventory["A"][0].Quantity != 5 {
		t.Errorf("FAIL: inventory merged quantities (got %d)", inventory["A"][0].Quantity)
		return
	}

	// Similar but different
	entryNM.SellerName = "NOTBANNED"
	err = inventory.Add("A", &entryNM)
	if err != nil {
		t.Errorf("FAIL: Unexpected error: %s", err.Error())
		return
	}
	if len(inventory["A"]) != 3 {
		t.Errorf("FAIL: inventory contains a differen number of entries %d than expected for A", len(inventory["A"]))
		return
	}
	if inventory["A"][0].Quantity != 5 {
		t.Errorf("FAIL: inventory merged quantities (got %d)", inventory["A"][0].Quantity)
		return
	}

	t.Log("PASS: Add")
}

func TestAddStrict(t *testing.T) {
	entryNM := InventoryEntry{
		Quantity:   5,
		Conditions: NM,
		Price:      20.0,
		URL:        "https://mtgban.com",
		SellerName: "BANNED",
	}
	entrySP := InventoryEntry{
		Quantity:   4,
		Conditions: SP,
		Price:      10.0,
		URL:        "https://mtgban.com",
		SellerName: "BANNED",
	}
	inventory := InventoryRecord{}

	// Empty inventory, add an entry
	err := inventory.AddStrict("A", &entryNM)
	if err != nil {
		t.Errorf("FAIL: Unexpected error: %s", err.Error())
		return
	}
	if len(inventory["A"]) == 0 {
		t.Error("FAIL: inventory does not contain entries for A")
		return
	}

	// Add entry with same ID, but it's a different conditions
	err = inventory.AddStrict("A", &entrySP)
	if err != nil {
		t.Errorf("FAIL: Unexpected error: %s", err.Error())
		return
	}
	if len(inventory["A"]) != 2 {
		t.Errorf("FAIL: inventory only contains %d entries for A", len(inventory["A"]))
		return
	}

	// Same entry, same id
	err = inventory.AddStrict("A", &entryNM)
	if err == nil {
		t.Error("FAIL: Tried to add the same entry twice")
		return
	}
	if len(inventory["A"]) != 2 {
		t.Errorf("FAIL: inventory contains a differen number of entries (%d) than expected for A", len(inventory["A"]))
		return
	}
	if inventory["A"][0].Quantity != 5 {
		t.Errorf("FAIL: inventory merged quantities (got %d)", inventory["A"][0].Quantity)
		return
	}

	// Similar but different
	err = inventory.AddStrict("A", &entryNM)
	if err == nil {
		t.Error("FAIL: Tried to add a similar entry twice")
		return
	}
	if len(inventory["A"]) != 2 {
		t.Errorf("FAIL: inventory contains a differen number of entries (%d) than expected for A", len(inventory["A"]))
		return
	}
	if inventory["A"][0].Quantity != 5 {
		t.Errorf("FAIL: inventory merged quantities (got %d)", inventory["A"][0].Quantity)
		return
	}

	t.Log("PASS: AddStrict")
}

func TestSort(t *testing.T) {
	testEntries := []InventoryEntry{
		{
			Quantity:   5,
			Conditions: NM,
			Price:      20.0,
			URL:        "https://mtgban.com",
			SellerName: "BANNED",
		},
		{
			Quantity:   4,
			Conditions: SP,
			Price:      8.0,
			URL:        "https://mtgban.com",
			SellerName: "BANNED",
		},
		{
			Quantity:   4,
			Conditions: SP,
			Price:      10.0,
			URL:        "https://mtgban.com",
			SellerName: "BANNED",
		},
		{
			Quantity:   1,
			Conditions: SP,
			Price:      10.0,
			URL:        "https://mtgban.com",
			SellerName: "BANNED_TWO",
		},
	}

	rand.Shuffle(len(testEntries), func(i, j int) {
		testEntries[i], testEntries[j] = testEntries[j], testEntries[i]
	})

	expectedCond := []Condition{NM, SP, SP, SP}
	expectedPrice := []float64{20.0, 8.0, 10.0, 10.0}
	expectedQty := []int{5, 4, 4, 1}

	inventory := InventoryRecord{}

	// Add all the entries in wrong order
	for _, testEntry := range testEntries {
		err := inventory.AddStrict("A", &testEntry)
		if err != nil {
			t.Errorf("FAIL: Unexpected error: %s", err.Error())
			return
		}
	}
	if len(inventory["A"]) != len(testEntries) {
		t.Errorf("FAIL: inventory contains a differen number of entries (%d) than expected (%d) for A", len(inventory["A"]), len(testEntries))
		return
	}

	for _, entries := range inventory {
		for i := range entries {
			if entries[i].Conditions != expectedCond[i] {
				t.Errorf("FAIL: array not sorted: condition of %d is not %s (got %s)", i, expectedCond[i], entries[i].Conditions)
				return
			}
			if entries[i].Price != expectedPrice[i] {
				t.Errorf("FAIL: array not sorted: price of %d is not %f (got %f)", i, expectedPrice[i], entries[i].Price)
				return
			}
			if entries[i].Quantity != expectedQty[i] {
				t.Errorf("FAIL: array not sorted: quantity of %d is not %d (got %d)", i, expectedQty[i], entries[i].Quantity)
				return
			}
		}
	}

	t.Log("PASS: Sort")
}

// TestDuplicateEntryIsRecognisable pins that a record refusing a duplicate
// says so in a way a caller can test for. A storefront listing one card under
// two products produces the refusal by the thousand, and a scraper that knows
// its storefront does that has to tell it from a real failure.
func TestDuplicateEntryIsRecognisable(t *testing.T) {
	t.Run("buylist", func(t *testing.T) {
		bl := BuylistRecord{}
		entry := &BuylistEntry{Conditions: NM, BuyPrice: 4.34}
		if err := bl.Add("id", entry); err != nil {
			t.Fatalf("first add: %v", err)
		}
		err := bl.Add("id", entry)
		if !errors.Is(err, ErrDuplicateEntry) {
			t.Errorf("second add: %v, want ErrDuplicateEntry", err)
		}
		if len(bl["id"]) != 1 {
			t.Errorf("the record kept %d entries, want the first one alone", len(bl["id"]))
		}
	})

	t.Run("a differing price is not a duplicate", func(t *testing.T) {
		bl := BuylistRecord{}
		if err := bl.Add("id", &BuylistEntry{Conditions: NM, BuyPrice: 4.34}); err != nil {
			t.Fatalf("first add: %v", err)
		}
		if err := bl.Add("id", &BuylistEntry{Conditions: NM, BuyPrice: 9.99}); err != nil {
			t.Errorf("a second price was refused: %v", err)
		}
		if len(bl["id"]) != 2 {
			t.Errorf("the record kept %d entries, want both prices", len(bl["id"]))
		}
	})

	t.Run("inventory", func(t *testing.T) {
		inv := InventoryRecord{}
		entry := &InventoryEntry{Conditions: NM, Price: 4.34, Quantity: 3, URL: "u"}
		if err := inv.Add("id", entry); err != nil {
			t.Fatalf("first add: %v", err)
		}
		err := inv.Add("id", entry)
		if !errors.Is(err, ErrDuplicateEntry) {
			t.Errorf("second add: %v, want ErrDuplicateEntry", err)
		}
		if len(inv["id"]) != 1 || inv["id"][0].Quantity != 3 {
			t.Errorf("the record holds %+v, want the first entry with its own quantity", inv["id"])
		}
	})

	t.Run("a bad condition is still its own error", func(t *testing.T) {
		bl := BuylistRecord{}
		err := bl.Add("id", &BuylistEntry{Conditions: "XX"})
		if errors.Is(err, ErrDuplicateEntry) {
			t.Errorf("an invalid condition reported %v", err)
		}
		if !errors.Is(err, ErrInvalidCondition) {
			t.Errorf("got %v, want ErrInvalidCondition", err)
		}
	})
}

// TestAddKeepsInstancesApart pins that offers the store tells apart by id,
// or by having none, stay apart in whatever order they arrive, each with its
// own id and quantity, while the NM entry still sorts first.
func TestAddKeepsInstancesApart(t *testing.T) {
	adds := map[string]func(InventoryRecord, string, *InventoryEntry) error{
		"relaxed": InventoryRecord.AddRelaxed,
		"add":     InventoryRecord.Add,
	}
	nm := []InventoryEntry{
		{Conditions: NM, Price: 5, Quantity: 2, URL: "u", InstanceID: "a"},
		{Conditions: NM, Price: 5, Quantity: 1, URL: "u", InstanceID: "b"},
		{Conditions: NM, Price: 5, Quantity: 3, URL: "u"},
	}
	orders := map[string][]int{"id-less last": {0, 1, 2}, "id-less first": {2, 0, 1}}
	for name, add := range adds {
		for order, idx := range orders {
			t.Run(name+"/"+order, func(t *testing.T) {
				inv := InventoryRecord{}
				rows := []InventoryEntry{{Conditions: SP, Price: 3, Quantity: 1, URL: "u", InstanceID: "c"}}
				for _, i := range idx {
					rows = append(rows, nm[i])
				}
				for i := range rows {
					err := add(inv, "A", &rows[i])
					if err != nil {
						t.Fatalf("row %d: %v", i, err)
					}
				}

				entries := inv["A"]
				if len(entries) != 4 {
					t.Fatalf("got %d entries, want 4: %v", len(entries), entries)
				}
				got := map[string]int{}
				for _, entry := range entries[:3] {
					if entry.Conditions != NM {
						t.Fatalf("NM entries do not lead: %v", entries)
					}
					got[entry.InstanceID] = entry.Quantity
				}
				if got["a"] != 2 || got["b"] != 1 || got[""] != 3 {
					t.Errorf("offers merged across ids: %v", entries)
				}
				if entries[3].InstanceID != "c" {
					t.Errorf("SP entry does not trail: %v", entries)
				}
			})
		}
	}

	t.Run("merge resorts", func(t *testing.T) {
		inv := InventoryRecord{}
		for _, row := range []InventoryEntry{
			{Conditions: NM, Price: 5, Quantity: 2, InstanceID: "a"},
			{Conditions: NM, Price: 5, Quantity: 1, InstanceID: "b"},
			{Conditions: NM, Price: 5, Quantity: 3, InstanceID: "b"},
		} {
			err := inv.AddRelaxed("A", &row)
			if err != nil {
				t.Fatal(err)
			}
		}
		entries := inv["A"]
		if len(entries) != 2 || entries[0].InstanceID != "b" || entries[0].Quantity != 4 {
			t.Errorf("merged offer does not lead by quantity: %v", entries)
		}
	})

	t.Run("same id", func(t *testing.T) {
		inv := InventoryRecord{}
		for range 2 {
			err := inv.AddRelaxed("A", &InventoryEntry{Conditions: NM, Price: 5, Quantity: 1, InstanceID: "a"})
			if err != nil {
				t.Fatal(err)
			}
		}
		if len(inv["A"]) != 1 || inv["A"][0].Quantity != 2 {
			t.Errorf("one id not merged: %v", inv["A"])
		}
	})

	t.Run("strict", func(t *testing.T) {
		inv := InventoryRecord{}
		err := inv.AddStrict("A", &InventoryEntry{Conditions: NM, Price: 5, InstanceID: "a"})
		if err != nil {
			t.Fatal(err)
		}
		err = inv.AddStrict("A", &InventoryEntry{Conditions: NM, Price: 5, InstanceID: "b"})
		if !errors.Is(err, ErrDuplicateEntry) {
			t.Errorf("AddStrict accepted a second offer at one price: %v", err)
		}
	})
}

// TestBuylistAddReportsAcrossInstances pins that the buylist alarm still
// fires for two ids at one price: there it names two products folded onto one
// card, not two offers.
func TestBuylistAddReportsAcrossInstances(t *testing.T) {
	bl := BuylistRecord{}
	err := bl.Add("A", &BuylistEntry{Conditions: NM, BuyPrice: 5, Quantity: 1, InstanceID: "a"})
	if err != nil {
		t.Fatal(err)
	}
	err = bl.Add("A", &BuylistEntry{Conditions: NM, BuyPrice: 5, Quantity: 1, InstanceID: "b"})
	if !errors.Is(err, ErrDuplicateEntry) {
		t.Errorf("Add accepted a duplicate buylist entry: %v", err)
	}
}

// TestBuylistMergeResorts pins that a merge which raises a quantity moves the
// entry ahead of a same-priced one it now outnumbers.
func TestBuylistMergeResorts(t *testing.T) {
	bl := BuylistRecord{}
	for _, entry := range []BuylistEntry{
		{Conditions: NM, BuyPrice: 9, Quantity: 1},
		{Conditions: NM, BuyPrice: 5, Quantity: 3},
		{Conditions: NM, BuyPrice: 5, Quantity: 2},
	} {
		err := bl.AddRelaxed("A", &entry)
		if err != nil {
			t.Fatal(err)
		}
	}

	err := bl.AddRelaxed("A", &BuylistEntry{Conditions: NM, BuyPrice: 5, Quantity: 2})
	if err != nil {
		t.Fatal(err)
	}

	expectedPrice := []float64{9, 5, 5}
	expectedQty := []int{1, 4, 3}
	entries := bl["A"]
	if len(entries) != len(expectedQty) {
		t.Fatalf("got %d entries, want %d", len(entries), len(expectedQty))
	}
	for i := range entries {
		if entries[i].BuyPrice != expectedPrice[i] || entries[i].Quantity != expectedQty[i] {
			t.Errorf("entry %d is %v x%d, want %v x%d", i, entries[i].BuyPrice, entries[i].Quantity, expectedPrice[i], expectedQty[i])
		}
	}
}
