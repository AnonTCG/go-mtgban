package riftbound

import (
	"slices"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestPromoTagsAreSlugs pins the form the tags are stored in. A search query
// is split on whitespace before any filter sees it, so a tag is only askable
// as one word - the same shape Magic's own promo types have always had.
func TestPromoTagsAreSlugs(t *testing.T) {
	b := gametest.Backend(t, matcher.GameRiftbound)

	for _, tag := range b.AllPromoTypes {
		if slug := matcher.PromoTypeSlug(tag); slug != tag {
			t.Errorf("declared tag %q is not its own slug (%q), so is:%s cannot reach it", tag, slug, slug)
		}
	}
	for _, tag := range []string{"metal", "bestof", "prizewall", "alternateart"} {
		if !slices.Contains(b.AllPromoTypes, tag) {
			t.Errorf("tag %q is not declared", tag)
		}
	}
	// A qualifier that only repeats the collector number describes nothing.
	for _, tag := range b.AllPromoTypes {
		if len(tag) > 1 && tag[0] == 'r' && tag[1] >= '0' && tag[1] <= '9' {
			t.Errorf("declared tag %q is a collector number, not a description", tag)
		}
	}

	// The three printings that share name and number are told apart by tag.
	var bestOf int
	for _, uuid := range b.AllUUIDs {
		co, err := b.GetUUID(uuid)
		if err != nil {
			continue
		}
		if co.Name == "Teemo, Swift Scout" && co.HasPromoType("bestof") {
			bestOf++
		}
	}
	if bestOf == 0 {
		t.Error("no Teemo, Swift Scout printing carries bestof")
	}
}

// TestProductNameReachesOnePrinting pins the other half of the contract: the
// catalog's own spelling names a single printing, where the bare name names
// the card. The builder splits that spelling into a name and its qualifiers,
// so the rebuilt one has to answer to what the storefront actually writes.
func TestProductNameReachesOnePrinting(t *testing.T) {
	b := gametest.Backend(t, matcher.GameRiftbound)

	bare, err := b.SearchEquals("Teemo, Swift Scout")
	if err != nil {
		t.Fatal(err)
	}
	if len(bare) < 2 {
		t.Errorf("bare name reached %d printings, want the whole name", len(bare))
	}

	for _, tt := range []struct {
		query    string
		promoTag string
	}{
		{"Teemo, Swift Scout (Metal) (Best Of)", "bestof"},
		{"Teemo, Swift Scout (Metal) (Prize Wall)", "prizewall"},
		{"Edge of Night (Champion)", "champion"},
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
		if !co.HasPromoType(tt.promoTag) {
			t.Errorf("%q reached a printing tagged %v, want one carrying %q", tt.query, co.PromoTypes, tt.promoTag)
		}
	}
}
