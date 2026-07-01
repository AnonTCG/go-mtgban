package mtgmatcher

import (
	_ "embed"
	"encoding/json"
)

// AnonTCG deviation (not upstream). MTGJSON published the MSC "Commander: Marvel
// Super Heroes" nonfoil deck printings with their FOIL (surge foil) TCGplayer
// product in the primary tcgplayerProductId slot and no
// tcgplayerAlternativeFoilProductId. The surge-foil product has no listings, so
// the nonfoil market — the product that actually has prices — is never scraped,
// leaving every one of these cards blank in inventory / EV / decklist exports.
//
// This override reconstructs the two identifier fields MTGJSON should have
// published (primary = nonfoil product, alt = foil product), sourced from
// AnonTCG's tcgplayer_catalog (TCGplayer catalog API, independent of MTGJSON).
// It is applied in LoadDatastore's card loop before the normal
// tcgplayerAlternativeFoilProductId split, so the standard split + needsNewTCGSKUs
// refetch path handles both finishes with no other code changes.
//
// Cleanup obligation: DELETE this file, tcgplayer_msc_override.json, and the
// applyMSCTCGOverride call in backend.go once MTGJSON corrects the upstream ids
// (the primary tcgplayerProductId points at the Normal product again).

//go:embed tcgplayer_msc_override.json
var mscTCGOverrideRaw []byte

type mscTCGOverrideEntry struct {
	Nonfoil string `json:"nonfoil"`
	Foil    string `json:"foil"`
}

var mscTCGOverride = loadMSCTCGOverride()

func loadMSCTCGOverride() map[string]mscTCGOverrideEntry {
	var payload struct {
		Cards map[string]mscTCGOverrideEntry `json:"cards"`
	}
	if err := json.Unmarshal(mscTCGOverrideRaw, &payload); err != nil {
		panic("mtgmatcher: invalid tcgplayer_msc_override.json: " + err.Error())
	}
	return payload.Cards
}

// applyMSCTCGOverride rewrites a card's TCGplayer identifiers when it is one of
// the MSC surge-foil/base-swap entries, so the caller's alt-foil split resolves
// the correct nonfoil (and foil) products. Keyed by the original MTGJSON uuid.
func applyMSCTCGOverride(identifiers map[string]string, uuid string) {
	ov, ok := mscTCGOverride[uuid]
	if !ok {
		return
	}
	identifiers["tcgplayerProductId"] = ov.Nonfoil
	identifiers["tcgplayerAlternativeFoilProductId"] = ov.Foil
	// Force a live SKU refetch: MTGJSON's cached TcgplayerSkus.json holds the
	// wrong (foil) SKUs for this uuid, so both split cards must re-pull.
	identifiers["needsNewTCGSKUs"] = "true"
}
