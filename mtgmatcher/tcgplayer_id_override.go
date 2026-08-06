package mtgmatcher

import (
	_ "embed"
	"encoding/json"
)

// AnonTCG deviation (not upstream). MTGJSON periodically publishes wrong
// TCGplayer identifiers for cards that have a separate special-treatment
// product (surge foil, etc.). Two distinct failure shapes seen so far:
//
// MSC ("Commander: Marvel Super Heroes") — the nonfoil deck printings carried
// their FOIL (surge foil) product in the primary tcgplayerProductId slot with
// no tcgplayerAlternativeFoilProductId. The surge-foil product has no listings,
// so the nonfoil market — the one that actually has prices — was never scraped,
// leaving those cards blank in inventory / EV / decklist exports. MTGJSON has
// since corrected these (43/43 verified 2026-08-06, in both AllPrintings and
// TcgplayerSkus.json); the entries are retained until removal is validated.
//
// HOB ("The Hobbit") — #239 "Gleaming Splendor (Borderless)" and #275
// "Gleaming Splendor (Borderless) (Surge Foil)" were published with IDENTICAL
// identifiers (primary 709068 + altFoil 709470) and identical merged SKU lists.
// The alt-foil split below therefore priced #239's FOIL from the Surge Foil
// product (709470, $1,499.93 NM) — a card that cannot come out of a Play or
// Collector Booster at that treatment — while #275 got no TCGplayer rows at
// all. Correct shape, verified against tcgplayer_catalog:
//
//	709068 = "Gleaming Splendor (Borderless)"              #239, Normal + Foil SKUs
//	709470 = "Gleaming Splendor (Borderless) (Surge Foil)" #275, Foil SKUs only
//
// so #239 is an ordinary single-product dual-finish card (its own foil SKU
// 9434642 exists and was never reached) and #275 is product 709470, foil only.
//
// Applied in LoadDatastore's card loop BEFORE the
// tcgplayerAlternativeFoilProductId split reads the identifiers, so the
// standard split + needsNewTCGSKUs refetch path handles the corrected values
// with no other code changes.
//
// Cleanup obligation: drop an entry once MTGJSON publishes correct ids for it;
// delete this file, tcgplayer_id_override.json, and the applyTCGIDOverride call
// in backend.go once the map is empty.

//go:embed tcgplayer_id_override.json
var tcgIDOverrideRaw []byte

type tcgIDOverrideEntry struct {
	Set    string `json:"set"`
	Name   string `json:"name"`
	Number string `json:"number"`
	// Primary replaces tcgplayerProductId.
	Primary string `json:"primary"`
	// AltFoil replaces tcgplayerAlternativeFoilProductId. An EMPTY value
	// DELETES that identifier, which suppresses the alt-foil split entirely.
	AltFoil string `json:"alt_foil"`
}

var tcgIDOverride = loadTCGIDOverride()

func loadTCGIDOverride() map[string]tcgIDOverrideEntry {
	var payload struct {
		Cards map[string]tcgIDOverrideEntry `json:"cards"`
	}
	if err := json.Unmarshal(tcgIDOverrideRaw, &payload); err != nil {
		panic("mtgmatcher: invalid tcgplayer_id_override.json: " + err.Error())
	}
	return payload.Cards
}

// applyTCGIDOverride rewrites a card's TCGplayer identifiers when it is one of
// the known MTGJSON mis-publications, so the caller's alt-foil split resolves
// the correct product(s). Keyed by the original MTGJSON uuid. No-op otherwise.
func applyTCGIDOverride(identifiers map[string]string, uuid string) {
	ov, ok := tcgIDOverride[uuid]
	if !ok {
		return
	}

	identifiers["tcgplayerProductId"] = ov.Primary

	// An empty AltFoil means "this card is a single product covering every
	// finish it has" — remove the field so no split card is synthesized.
	if ov.AltFoil != "" {
		identifiers["tcgplayerAlternativeFoilProductId"] = ov.AltFoil
	} else {
		delete(identifiers, "tcgplayerAlternativeFoilProductId")
	}

	// Force a live SKU refetch: MTGJSON's cached TcgplayerSkus.json holds the
	// wrong SKUs for these uuids (for HOB it holds BOTH products' SKUs merged
	// onto both cards), so every affected card must re-pull from its corrected
	// product id rather than trust the cached list.
	identifiers["needsNewTCGSKUs"] = "true"
}
