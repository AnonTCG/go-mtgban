package tcgplayer

import "testing"

// TestProductPrintings pins that a price row reaches only the printing its
// own product sells in that finish: a base product's foil row has nowhere to
// go when the foil is its own product, a star foil with no product of its
// own is its base product's foil, and the English foil wins over a
// foreign-language twin.
func TestProductPrintings(t *testing.T) {
	b := realDatastore(t)
	printings := productPrintings(b, crossSetProductIDs(b), nil)

	tests := []struct {
		id, subtype, set, number string
	}{
		{"286677", "Foil", "40K", "179"},
		{"3040", "Foil", "7ED", "157★"},
		{"3040", "Normal", "7ED", "157"},
		{"679621", "Foil", "", ""},
		{"95037", "Foil", "FRF", "65★"},
	}
	for _, tt := range tests {
		t.Run(tt.id+"/"+tt.subtype, func(t *testing.T) {
			cardID, found := printings[tt.id][tt.subtype]
			if tt.set == "" {
				if found {
					co, _ := b.GetUUID(cardID)
					t.Errorf("got %s, want no printing", co)
				}
				return
			}
			co, err := b.GetUUID(cardID)
			if err != nil {
				t.Fatalf("no printing for %s/%s", tt.id, tt.subtype)
			}
			if co.SetCode != tt.set || co.Number != tt.number {
				t.Errorf("got %s %s, want %s %s", co.SetCode, co.Number, tt.set, tt.number)
			}
		})
	}
}
