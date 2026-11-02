package fleshandblood

import (
	"slices"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestPromoTagsAndQualifiedNames pins what tells sibling printings apart.
// The marvel and the plain printing of Enigma, New Moon share the number
// MST238, and only the label separates them; the site prints a qualifier
// only when the backend declares it, so both halves have to hold.
func TestPromoTagsAndQualifiedNames(t *testing.T) {
	b := gametest.Backend(t, matcher.GameFleshAndBlood)

	for _, tag := range []string{"marvel", "extendedart", "golden", "treasure"} {
		if !slices.Contains(b.AllPromoTypes, tag) {
			t.Errorf("promo type %q is not declared, so nothing will print it", tag)
		}
	}

	// A label the finish already says describes nothing: the catalog prices
	// its "(Cold Foil)" promos as Cold Foils. The card keeps the label for
	// FilterCards to tier by, only the declaration drops it.
	for _, tag := range []string{"coldfoil", "rainbowfoil", "rainbow"} {
		if slices.Contains(b.AllPromoTypes, tag) {
			t.Errorf("promo type %q repeats the finish, so a label would print it twice", tag)
		}
	}

	bare, err := b.SearchEquals("Enigma, New Moon")
	if err != nil {
		t.Fatal(err)
	}
	if len(bare) < 2 {
		t.Errorf("bare name reached %d printings, expected every printing of the name", len(bare))
	}

	for _, tt := range []struct{ query, promoType string }{
		{"Enigma, New Moon (Marvel)", "marvel"},
		{"Gold-Baited Hook (Treasure)", "treasure"},
	} {
		got, err := b.SearchEquals(tt.query)
		if err != nil {
			t.Errorf("%q: %v", tt.query, err)
			continue
		}
		if len(got) != 1 {
			t.Errorf("%q reached %d printings, want the one it names", tt.query, len(got))
			continue
		}
		co, err := b.GetUUID(got[0])
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(co.PromoTypes, tt.promoType) {
			t.Errorf("%q reached a printing tagged %v, want one carrying %q", tt.query, co.PromoTypes, tt.promoType)
		}
	}

	// A spelling the datastore already carries as a name of its own keeps
	// only its own printings: the pitch color belongs to the name, so the
	// promo that files "Red" as a label beside the plain name must not pour
	// into the bucket "Dig In (Red)" answers with.
	named, err := b.SearchEquals("Dig In (Red)")
	if err != nil {
		t.Fatal(err)
	}
	for _, uuid := range named {
		co, err := b.GetUUID(uuid)
		if err != nil {
			t.Fatal(err)
		}
		if co.Name != "Dig In (Red)" {
			t.Errorf("the name's bucket reached %q from %s, want only the card of that name", co.Name, co.SetCode)
		}
	}
}
