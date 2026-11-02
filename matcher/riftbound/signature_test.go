package riftbound

import (
	"slices"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestSignedPromoTypes pins the qualifier a starred collector number stands
// for. The gallery marks the signed showcase printings with a star on the
// public code and says nothing else about them, so without this the star is
// only a number and the printing carries no tag at all.
func TestSignedPromoTypes(t *testing.T) {
	for _, tt := range []struct {
		desc       string
		promoTypes []string
		number     string
		want       []string
	}{
		{"a starred number signs the printing", nil, "235*", []string{"signature"}},
		{"the plain number beside it says nothing", nil, "235", nil},
		{"a star keeps the qualifiers the card already had",
			[]string{"metal"}, "227*", []string{"metal", "signature"}},
		{"a card already saying it is not said twice",
			[]string{"signature"}, "227*", []string{"signature"}},
		{"a lettered variant is not a signature", nil, "205a", nil},
		{"nor is a star anywhere but the end", nil, "2*35", nil},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			got := signedPromoTypes(tt.promoTypes, tt.number)
			if !slices.Equal(got, tt.want) {
				t.Errorf("signedPromoTypes(%v, %q) = %v, want %v", tt.promoTypes, tt.number, got, tt.want)
			}
		})
	}
}

// TestSignedPrintingsAreTagged reads the datastore itself: every printing the
// gallery stars carries the tag, and the printing it shares a number with
// does not.
func TestSignedPrintingsAreTagged(t *testing.T) {
	b := gametest.Backend(t, matcher.GameRiftbound)

	var starred, tagged int
	for uuid, co := range b.UUIDs {
		if co.Sealed || co.Number == "" || co.Number[len(co.Number)-1] != '*' {
			continue
		}
		starred++
		if co.HasPromoType(signaturePromoType) {
			tagged++
			continue
		}
		t.Errorf("%s (%s %s) is starred but carries %v", uuid, co.SetCode, co.Number, co.PromoTypes)
	}
	if starred == 0 {
		t.Fatal("the datastore holds no starred printing, so this proves nothing")
	}
	t.Logf("%d starred printings, %d tagged", starred, tagged)

	if !slices.Contains(b.AllPromoTypes, signaturePromoType) {
		t.Errorf("%q is not declared, so is:%s cannot reach it", signaturePromoType, signaturePromoType)
	}

	// The printing sharing the number must not pick the tag up.
	for uuid, co := range b.UUIDs {
		if co.Sealed || co.Number == "" || co.Number[len(co.Number)-1] == '*' {
			continue
		}
		if co.HasPromoType(signaturePromoType) {
			t.Errorf("%s (%s %s) is unstarred but tagged %v", uuid, co.SetCode, co.Number, co.PromoTypes)
		}
	}
}
