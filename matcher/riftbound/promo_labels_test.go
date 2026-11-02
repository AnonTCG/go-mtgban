package riftbound

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestPromoTypeLabels pins that a token can be read back as the words it was
// made from. The token is what a search query carries; the label is what a
// reader is shown, and title-casing the token cannot put back the spaces it
// dropped.
//
// A tag nobody has written down yet is reported rather than failed, for the
// reason Magic's suite gives: the spelling table is ours and the tags are the
// gallery's, so a promo Riot adds tomorrow would otherwise turn this suite
// red for having nothing to do with us. The fallback spells the token plainly
// until the entry is written.
func TestPromoTypeLabels(t *testing.T) {
	b := gametest.Backend(t, matcher.GameRiftbound)

	var unlabelled []string
	for _, tag := range b.AllPromoTypes {
		if b.PromoTypeLabels[tag] == "" {
			unlabelled = append(unlabelled, tag)
		}
	}
	if len(unlabelled) > 0 {
		t.Logf("%d tags have no spelling yet, so they read as a run-together word until promolabels.go spells them: %v",
			len(unlabelled), unlabelled)
	}
	for _, tag := range b.AllPromoTypes {
		if b.PromoTypeLabel(tag) == "" {
			t.Errorf("tag %q reads back as nothing", tag)
		}
	}
	for _, tt := range []struct{ promoType, want string }{
		{"alternateart", "Alternate Art"},
		{"bestof", "Best Of"},
		{"prizewall", "Prize Wall"},
		// The case that says why the spelling is looked up rather than
		// guessed: title-casing this token gives "Gg Ez".
		{"ggez", "GG EZ"},
		{"kdaspecialartcard", "K/DA Special Art Card"},
	} {
		if got := b.PromoTypeLabel(tt.promoType); got != tt.want {
			t.Errorf("PromoTypeLabel(%q) = %q, want %q", tt.promoType, got, tt.want)
		}
	}
	// An unknown token still reads as something rather than empty.
	if got := b.PromoTypeLabel("nosuchtag"); got == "" {
		t.Error("an undeclared tag reads back as nothing")
	}
}
