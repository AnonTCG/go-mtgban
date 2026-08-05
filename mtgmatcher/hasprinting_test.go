package mtgmatcher

import (
	"math/rand"
	"testing"
)

// oldHasPrinting is the pre-index implementation, kept verbatim as the
// equivalence reference: for every printing of the named card it scanned the
// whole set comparing names with Equals.
func oldHasPrinting(name, field, value string, editions ...string) bool {
	if defaultBackend.Sets == nil {
		return false
	}

	var checkFunc func(Card, string) bool
	switch field {
	case "promo_type":
		checkFunc = func(card Card, value string) bool {
			return card.HasPromoType(value)
		}
	case "frame_effect":
		checkFunc = func(card Card, value string) bool {
			return card.HasFrameEffect(value)
		}
	case "border_color":
		checkFunc = func(card Card, value string) bool {
			return card.BorderColor == value
		}
	case "frame_version":
		checkFunc = func(card Card, value string) bool {
			return card.FrameVersion == value
		}
	case "finish":
		checkFunc = func(card Card, value string) bool {
			return card.HasFinish(value)
		}
	case "field":
		switch value {
		case "attractionLights":
			checkFunc = func(card Card, value string) bool {
				return card.AttractionLights != nil
			}
		default:
			return false
		}
	default:
		return false
	}

	printings, err := Printings4Card(name)
	if err != nil {
		cc := &InputCard{
			Name: name,
		}
		adjustName(cc)
		name = cc.Name
		printings, err = Printings4Card(name)
		if err != nil {
			return false
		}
	}
	for _, code := range printings {
		var set *Set
		if len(editions) > 0 {
			set = defaultBackend.Sets[editions[0]]
			if set == nil {
				set, _ = GetSetByName(editions[0])
			}
		}
		if set == nil {
			set = defaultBackend.Sets[code]
			if set == nil {
				continue
			}
		}
		for _, in := range set.Cards {
			if Equals(name, in.Name) && checkFunc(in, value) {
				return true
			}
		}
	}

	return false
}

// TestHasPrintingEquivalence diffs the indexed hasPrinting against the old
// per-set scans over a broad sample of real cards, with and without pinned
// editions, across every check the public wrappers use.
func TestHasPrintingEquivalence(t *testing.T) {
	uuids := GetUUIDs()
	if len(uuids) == 0 {
		t.Skip("datastore not loaded")
	}

	checks := []struct {
		field string
		value string
	}{
		{"finish", FinishNonfoil},
		{"finish", FinishFoil},
		{"finish", FinishEtched},
		{"frame_effect", FrameEffectExtendedArt},
		{"frame_effect", FrameEffectShowcase},
		{"border_color", BorderColorBorderless},
		{"frame_version", "1997"},
		{"promo_type", PromoTypeBundle},
		{"field", "attractionLights"},
		{"bogus", "value"},
	}

	rng := rand.New(rand.NewSource(42))
	var compared int
	for i := 0; i < 1000; i++ {
		uuid := uuids[rng.Intn(len(uuids))]
		co, err := GetUUID(uuid)
		if err != nil {
			continue
		}
		for _, check := range checks {
			editionArgs := [][]string{
				nil,
				{co.SetCode},
				{co.Edition},
				{"Nonexistent Edition"},
			}
			for _, editions := range editionArgs {
				oldRes := oldHasPrinting(co.Name, check.field, check.value, editions...)
				newRes := hasPrinting(co.Name, check.field, check.value, editions...)
				if oldRes != newRes {
					t.Errorf("divergence: name=%q field=%s value=%s editions=%v old=%v new=%v",
						co.Name, check.field, check.value, editions, oldRes, newRes)
				}
				compared++
			}
		}
	}
	t.Logf("compared %d combinations", compared)
}

// The old scans made widely printed cards pathological: every printing
// re-scanned a full set with two normalizations per card.
func BenchmarkHasPrintingWide(b *testing.B) {
	if len(GetUUIDs()) == 0 {
		b.Skip("datastore not loaded")
	}
	b.Run("new", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			hasPrinting("Island", "finish", FinishFoil)
		}
	})
	b.Run("old", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			oldHasPrinting("Island", "finish", FinishFoil)
		}
	})
}
