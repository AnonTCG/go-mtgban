package mtgmatcher

import (
	"math/rand"
	"slices"
	"testing"
)

// The "servo" bucket holds two distinct cards, the WAR token and L16's
// "Servo // Thopter" through its face name: lookups must resolve to the
// card actually carrying the queried name, regardless of the bucket
// order the load process produced.
func TestPrintings4CardExactName(t *testing.T) {
	if len(GetUUIDs()) == 0 {
		t.Skip("datastore not loaded")
	}

	printings, err := Printings4Card("Servo")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(printings, "WAR") {
		t.Errorf("Servo printings = %v, expected to contain WAR", printings)
	}
	if slices.Contains(printings, "L16") {
		t.Errorf("Servo printings = %v, L16 belongs to Servo // Thopter", printings)
	}

	printings, err = Printings4Card("Servo // Thopter")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(printings, "L16") {
		t.Errorf("Servo // Thopter printings = %v, expected to contain L16", printings)
	}

	// Normalization folds plurals, so the Cat Warrior token and the Cat
	// Warriors card are distinct names sharing a bucket: verbatim matches
	// must win over normalized ones.
	printings, err = Printings4Card("Cat Warriors")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(printings, "LEG") || slices.Contains(printings, "DMU") {
		t.Errorf("Cat Warriors printings = %v, expected LEG without DMU", printings)
	}
	if !nameIsToken("Cat Warrior") {
		t.Errorf("the card named exactly Cat Warrior is the DMU token")
	}
	if nameIsToken("Cat Warriors") {
		t.Errorf("Cat Warriors is a regular card, not a token")
	}
}

// Token names clashing with a real card name must be excluded from the
// token table no matter the order sets are iterated during load: these
// names used to flip classification from process to process.
func TestIsTokenClashingNames(t *testing.T) {
	if len(GetUUIDs()) == 0 {
		t.Skip("datastore not loaded")
	}
	for _, name := range []string{"Scarecrow", "Spark Elemental", "Spellgorger Weird"} {
		if IsToken(name) {
			t.Errorf("IsToken(%q) = true, but a real card carries this name", name)
		}
	}
}

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
