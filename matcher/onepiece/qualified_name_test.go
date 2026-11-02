package onepiece

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestQualifiedNameSearch pins the discovery contract the catalog's
// parentheticals buy: the character name reaches every printing of that
// character, the qualified spelling reaches only the printing it names, and
// a name that owns its parenthetical is untouched by either.
func TestQualifiedNameSearch(t *testing.T) {
	b := gametest.Backend(t, matcher.GameOnePiece)

	bare, err := b.SearchEquals("Nami")
	if err != nil {
		t.Fatalf("bare name: %v", err)
	}
	if len(bare) < 50 {
		t.Errorf("bare name reached %d printings, expected the character's whole run", len(bare))
	}

	for _, tt := range []struct {
		name  string
		query string
	}{
		{"qualified spelling", "Nami (Premium Card Collection -Best Selection Vol. 6-)"},
		// Normalize drops the parentheses, so a reader typing the words
		// plainly lands on the same printing as one copying the product name.
		{"unparenthesized spelling", "Nami Premium Card Collection -Best Selection Vol. 6-"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := b.SearchEquals(tt.query)
			if err != nil {
				t.Fatalf("%q: %v", tt.query, err)
			}
			if len(got) != 1 {
				t.Fatalf("%q reached %d printings, want exactly the one it names", tt.query, len(got))
			}
			co, err := b.GetUUID(got[0])
			if err != nil {
				t.Fatal(err)
			}
			if co.Number != "OP15-108" || co.SetCode != "OP-PR" {
				t.Errorf("%q reached %s %s, want OP-PR OP15-108", tt.query, co.SetCode, co.Number)
			}
		})
	}

	// A qualifier every printing of a number carries is part of the name, and
	// the datastore leaves it there rather than in the variant; searching it
	// must still reach the printings rather than nothing.
	epithet, err := b.SearchEquals("Mr.1 (Daz.Bonez)")
	if err != nil || len(epithet) == 0 {
		t.Errorf("epithet name reached %d printings (%v), want its own run", len(epithet), err)
	}

	// The qualified entries share their printings with the bare name, and a
	// prefix search matches both, so the buckets overlap.
	prefix, err := b.SearchHasPrefix("Nami")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, uuid := range prefix {
		if seen[uuid] {
			t.Fatalf("prefix search returned %s twice", uuid)
		}
		seen[uuid] = true
	}
}
