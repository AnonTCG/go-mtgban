package lorcana

import (
	"slices"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestPromoTypeLabels pins that a token can be read back as the words it was
// made from. The token is what a search query carries; the label is what a
// reader is shown, and title-casing the token cannot put back the spaces it
// dropped.
func TestPromoTypeLabels(t *testing.T) {
	b := gametest.Backend(t, matcher.GameLorcana)

	if len(b.PromoTypeLabels) != len(b.AllPromoTypes) {
		t.Errorf("%d tags declared but %d labelled", len(b.AllPromoTypes), len(b.PromoTypeLabels))
	}
	for _, tag := range b.AllPromoTypes {
		if b.PromoTypeLabel(tag) == "" {
			t.Errorf("tag %q reads back as nothing", tag)
		}
	}
	// The datastore publishes a slug and nothing else, so the words come
	// from this side. A title-caser cannot put back the boundaries the slug
	// dropped, which is what the table is for.
	if got := b.PromoTypeLabel("organizedplay"); got != "Organized Play" {
		t.Errorf("PromoTypeLabel(%q) = %q, want %q", "organizedplay", got, "Organized Play")
	}
	// A numbering pool is not a promotion and is not declared as one. It
	// is the denominator the card prints - "5/PD1" the way a set card is
	// "87/204" - and it lives in SetTotal, where totalTiebreak reads it.
	for _, pool := range []string{"p1", "p2", "p3", "p4", "c1", "c2", "cc1", "pd1", "dis"} {
		if slices.Contains(b.AllPromoTypes, pool) {
			t.Errorf("%q is declared a promo type; it is a numbering pool", pool)
		}
	}
	// An unknown token still reads as something rather than empty.
	if got := b.PromoTypeLabel("nosuchtag"); got == "" {
		t.Error("an undeclared tag reads back as nothing")
	}
}
