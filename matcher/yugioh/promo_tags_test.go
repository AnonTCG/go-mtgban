package yugioh

import (
	"slices"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestPromoTagsAndQualifiedNames pins what tells sibling products apart.
// Four Duelist League printings of Dark Magician share the number
// DL11-EN001, the rarity Rare and the print run, and only the color
// separates them; the site prints a qualifier only when the backend
// declares it, so both halves have to hold.
func TestPromoTagsAndQualifiedNames(t *testing.T) {
	b := gametest.Backend(t, matcher.GameYuGiOh)

	for _, tag := range []string{"alternateart", "duelterminal", "otsstamp"} {
		if !slices.Contains(b.AllPromoTypes, tag) {
			t.Errorf("promo type %q is not declared, so nothing will print it", tag)
		}
	}

	// The rarity is the axis most sibling products differ on, but it is a
	// field of its own that the card already carries: as a tag it would
	// declare a third of the catalog promotional.
	for _, rarity := range []string{"common", "ultrarare", "quartercenturysecretrare"} {
		if slices.Contains(b.AllPromoTypes, rarity) {
			t.Errorf("rarity %q is declared as a promo type", rarity)
		}
	}

	// A mark says which printing of a number this is - the ink the Duelist
	// League printings differ on, the version four Blue-Eyes differ on, the
	// letter three Dark Magician Girls differ on - and it is moving out of
	// the promo types into a watermark, a mark being no more a promotion
	// than a rarity is. Which side of that move this datastore is on is
	// read from the datastore rather than assumed, so this holds either way
	// round and the two halves can land in any order.
	if marked(b) {
		for _, mark := range []string{"purple", "green", "blue", "red", "silver", "bronze",
			"version1", "version2", "version3", "version4", "a", "b", "c"} {
			if slices.Contains(b.AllPromoTypes, mark) {
				t.Errorf("mark %q is declared as a promo type as well", mark)
			}
		}
	}

	bare, err := b.SearchEquals("Dark Magician")
	if err != nil {
		t.Fatal(err)
	}
	if len(bare) < 2 {
		t.Errorf("bare name reached %d printings, expected every printing of the name", len(bare))
	}

	// The mark reaches its printing and is said once: a Duelist League
	// printing wears no promotion, so a tag saying "purple" beside a
	// watermark saying the same would be the fact written twice. The
	// version and the artwork letter are the same kind of mark and are
	// checked beside it, three axes that are not promotions.
	for _, tt := range []struct{ query, color string }{
		{"Dark Magician (Purple)", "purple"},
		{"Dark Magician (Green)", "green"},
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
		// Said once, wherever it is said. A printing carrying the colour
		// as a watermark and again as a tag is the fact written twice,
		// which is what a variant read back per card would do.
		held := 0
		if co.Watermark == tt.color {
			held++
		}
		if slices.Contains(co.PromoTypes, tt.color) {
			held++
		}
		if held != 1 {
			t.Errorf("%q reached a printing inked %q and tagged %v, want %q said exactly once",
				tt.query, co.Watermark, co.PromoTypes, tt.color)
		}
	}

	// A spelling the datastore already carries as a name of its own keeps
	// only its own printings: "Dark Magician Girl (A)" is the YGLD card's
	// name, and the RA03 printing that files the same "A" as a label beside
	// the plain name must not pour into its bucket.
	named, err := b.SearchEquals("Dark Magician Girl (A)")
	if err != nil {
		t.Fatal(err)
	}
	for _, uuid := range named {
		co, err := b.GetUUID(uuid)
		if err != nil {
			t.Fatal(err)
		}
		if co.Name != "Dark Magician Girl (A)" {
			t.Errorf("the name's bucket reached %q from %s, want only the card of that name", co.Name, co.SetCode)
		}
	}
}

// marked reports whether this datastore gives a printing's mark a field of
// its own. Nothing else in the game wears a watermark, so one is enough.
func marked(b *matcher.Backend) bool {
	for _, co := range b.UUIDs {
		if co.Watermark != "" {
			return true
		}
	}
	return false
}
