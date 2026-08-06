package mtgmatcher

import (
	"testing"
)

// The override entries are applied unconditionally, so the staleness
// classification is the only thing that will ever tell us an entry has gone
// bad. These tests pin that classification and the identifier writes.

func TestTCGIDOverrideClassify(t *testing.T) {
	ov := tcgIDOverrideEntry{
		Primary:         "709068",
		AltFoil:         "",
		UpstreamPrimary: "709068",
		UpstreamAltFoil: "709470",
	}

	tests := []struct {
		name       string
		gotPrimary string
		gotAltFoil string
		want       overrideState
	}{
		{
			name:       "upstream still broken as recorded",
			gotPrimary: "709068",
			gotAltFoil: "709470",
			want:       overrideStillNeeded,
		},
		{
			name:       "upstream fixed to our corrected values",
			gotPrimary: "709068",
			gotAltFoil: "",
			want:       overrideRedundant,
		},
		{
			name:       "upstream moved to a brand new alt product",
			gotPrimary: "709068",
			gotAltFoil: "999999",
			want:       overrideUpstreamMoved,
		},
		{
			name:       "upstream repointed the primary entirely",
			gotPrimary: "800001",
			gotAltFoil: "709470",
			want:       overrideUpstreamMoved,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ov.classify(tt.gotPrimary, tt.gotAltFoil); got != tt.want {
				t.Errorf("classify(%q, %q) = %v, want %v",
					tt.gotPrimary, tt.gotAltFoil, got, tt.want)
			}
		})
	}
}

// An empty AltFoil must DELETE tcgplayerAlternativeFoilProductId. That deletion
// is the whole HOB fix: leaving the field set sends the card's foil printing to
// the Surge Foil product via the alt-foil split in backend.go.
func TestApplyTCGIDOverrideClearsAltFoil(t *testing.T) {
	const uuid = "b44d95e0-9dc3-59d2-b309-454e35d6f35c" // HOB #239

	ov, ok := tcgIDOverride[uuid]
	if !ok {
		t.Skipf("override entry for %s no longer present (upstream fixed?)", uuid)
	}
	if ov.AltFoil != "" {
		t.Fatalf("expected HOB #239 to clear alt-foil, got %q", ov.AltFoil)
	}

	identifiers := map[string]string{
		"tcgplayerProductId":                ov.UpstreamPrimary,
		"tcgplayerAlternativeFoilProductId": ov.UpstreamAltFoil,
	}
	applyTCGIDOverride(identifiers, uuid)

	if got := identifiers["tcgplayerProductId"]; got != ov.Primary {
		t.Errorf("tcgplayerProductId = %q, want %q", got, ov.Primary)
	}
	if _, present := identifiers["tcgplayerAlternativeFoilProductId"]; present {
		t.Error("tcgplayerAlternativeFoilProductId should have been deleted, " +
			"otherwise the alt-foil split still prices this card from the Surge Foil product")
	}
	if got := identifiers["needsNewTCGSKUs"]; got != "true" {
		t.Errorf("needsNewTCGSKUs = %q, want \"true\" (cached SKUs hold both products merged)", got)
	}
}

// A non-empty AltFoil must still SET the field (the shape MSC used), so the
// generalisation stays capable of the original swap.
func TestApplyTCGIDOverrideSetsAltFoil(t *testing.T) {
	const uuid = "test-uuid-not-in-datastore"

	tcgIDOverride[uuid] = tcgIDOverrideEntry{
		Primary: "111", AltFoil: "222",
		UpstreamPrimary: "222", UpstreamAltFoil: "",
	}
	defer delete(tcgIDOverride, uuid)

	identifiers := map[string]string{"tcgplayerProductId": "222"}
	applyTCGIDOverride(identifiers, uuid)

	if got := identifiers["tcgplayerProductId"]; got != "111" {
		t.Errorf("tcgplayerProductId = %q, want \"111\"", got)
	}
	if got := identifiers["tcgplayerAlternativeFoilProductId"]; got != "222" {
		t.Errorf("tcgplayerAlternativeFoilProductId = %q, want \"222\"", got)
	}
}

func TestApplyTCGIDOverrideIgnoresUnknownUUID(t *testing.T) {
	identifiers := map[string]string{"tcgplayerProductId": "12345"}
	applyTCGIDOverride(identifiers, "definitely-not-an-override-uuid")

	if got := identifiers["tcgplayerProductId"]; got != "12345" {
		t.Errorf("tcgplayerProductId = %q, want it untouched", got)
	}
	if _, present := identifiers["needsNewTCGSKUs"]; present {
		t.Error("needsNewTCGSKUs must not be set for a non-override card")
	}
}

// Every entry must record the broken state it was written against, or the
// staleness check silently degrades to "always warn".
func TestTCGIDOverrideEntriesRecordUpstreamState(t *testing.T) {
	for uuid, ov := range tcgIDOverride {
		if ov.UpstreamPrimary == "" && ov.UpstreamAltFoil == "" {
			t.Errorf("%s (%s %s #%s): no upstream_primary/upstream_alt_foil recorded",
				uuid, ov.Set, ov.Name, ov.Number)
		}
		if ov.Primary == "" {
			t.Errorf("%s (%s %s #%s): empty primary product id",
				uuid, ov.Set, ov.Name, ov.Number)
		}
		// An entry whose corrected values equal the broken ones does nothing.
		if ov.Primary == ov.UpstreamPrimary && ov.AltFoil == ov.UpstreamAltFoil {
			t.Errorf("%s (%s %s #%s): corrected ids identical to the recorded broken ids",
				uuid, ov.Set, ov.Name, ov.Number)
		}
	}
}
