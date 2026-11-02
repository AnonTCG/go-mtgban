package matcher

import (
	"slices"
	"testing"
)

// TestSharePrintings pins the merge with no datastore: same-name variants
// share the union in the first entry's order, a token keeps apart from the
// card of its name, and an entry filed in the bucket only as an alias keeps
// its own list.
func TestSharePrintings(t *testing.T) {
	var b Backend
	b.UUIDs = map[string]*CardObject{
		"v1":    {Card: Card{Name: "Ineffable Blessing", Printings: []string{"UST"}}},
		"v2":    {Card: Card{Name: "Ineffable Blessing", Printings: []string{"ULST", "UST"}}},
		"v3":    {Card: Card{Name: "ineffable blessing", Printings: []string{"PLST"}}},
		"token": {Card: Card{Name: "Ineffable Blessing", Layout: "token", Printings: []string{"TUST"}}},
		"alias": {Card: Card{Name: "Other Card", Printings: []string{"OTH"}}},
	}
	b.Hashes = map[string][]string{
		Normalize("Ineffable Blessing"): {"v1", "token", "v2", "alias", "v3"},
		Normalize("Other Card"):         {"alias"},
	}

	b.sharePrintings()

	for _, uuid := range []string{"v1", "v2", "v3"} {
		got := b.UUIDs[uuid].Printings
		if !slices.Equal(got, []string{"UST", "ULST", "PLST"}) {
			t.Errorf("%s carries %v, want [UST ULST PLST]", uuid, got)
		}
	}
	got := b.UUIDs["token"].Printings
	if !slices.Equal(got, []string{"TUST"}) {
		t.Errorf("token carries %v, want [TUST]", got)
	}
	got = b.UUIDs["alias"].Printings
	if !slices.Equal(got, []string{"OTH"}) {
		t.Errorf("alias carries %v, want [OTH]", got)
	}
}
