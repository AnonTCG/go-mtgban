package mtgmatcher

import (
	_ "embed"
	"encoding/json"
	"log"
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
	// UpstreamPrimary/UpstreamAltFoil record the BROKEN values MTGJSON was
	// publishing when this entry was written. They are not applied — they let
	// the loader distinguish "upstream is still wrong in the way we expect"
	// from "upstream has changed and this entry may now be stale or harmful".
	UpstreamPrimary string `json:"upstream_primary"`
	UpstreamAltFoil string `json:"upstream_alt_foil"`
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

// overrideState classifies live MTGJSON identifiers against an override entry.
type overrideState int

const (
	// overrideStillNeeded: upstream is broken exactly as recorded.
	overrideStillNeeded overrideState = iota
	// overrideRedundant: upstream now publishes our corrected values.
	overrideRedundant
	// overrideUpstreamMoved: upstream matches neither — the entry may be stale
	// and is now silently overwriting whatever MTGJSON currently publishes.
	overrideUpstreamMoved
)

func (ov tcgIDOverrideEntry) classify(gotPrimary, gotAltFoil string) overrideState {
	switch {
	case gotPrimary == ov.Primary && gotAltFoil == ov.AltFoil:
		return overrideRedundant
	case gotPrimary == ov.UpstreamPrimary && gotAltFoil == ov.UpstreamAltFoil:
		return overrideStillNeeded
	default:
		return overrideUpstreamMoved
	}
}

// applyTCGIDOverride rewrites a card's TCGplayer identifiers when it is one of
// the known MTGJSON mis-publications, so the caller's alt-foil split resolves
// the correct product(s). Keyed by the original MTGJSON uuid. No-op otherwise.
//
// These overrides are applied UNCONDITIONALLY and cannot self-clear, so the
// staleness signals below are the only warning that an entry has outlived its
// purpose — or worse, started overriding data upstream has since corrected.
// They go to the stdlib logger (stderr) rather than mtgmatcher's package
// `logger`, which defaults to io.Discard and is never wired up by cmd/bantool:
// a self-check nobody can see is worse than none at all.
func applyTCGIDOverride(identifiers map[string]string, uuid string) {
	ov, ok := tcgIDOverride[uuid]
	if !ok {
		return
	}

	switch ov.classify(identifiers["tcgplayerProductId"], identifiers["tcgplayerAlternativeFoilProductId"]) {
	case overrideRedundant:
		log.Printf("mtgmatcher: TCG id override for %s %s #%s (%s) is REDUNDANT - "+
			"MTGJSON now publishes the corrected ids (product=%s altFoil=%q). "+
			"Delete this entry from tcgplayer_id_override.json.",
			ov.Set, ov.Name, ov.Number, uuid, ov.Primary, ov.AltFoil)

	case overrideUpstreamMoved:
		log.Printf("mtgmatcher: WARNING - TCG id override for %s %s #%s (%s) matches no known "+
			"upstream state. MTGJSON now publishes product=%s altFoil=%q; the recorded broken "+
			"state was product=%s altFoil=%q; we are forcing product=%s altFoil=%q. Upstream "+
			"changed - RE-VERIFY this entry, it may now be overriding correct data.",
			ov.Set, ov.Name, ov.Number, uuid,
			identifiers["tcgplayerProductId"], identifiers["tcgplayerAlternativeFoilProductId"],
			ov.UpstreamPrimary, ov.UpstreamAltFoil, ov.Primary, ov.AltFoil)

	case overrideStillNeeded:
		// Broken exactly as recorded; the override is doing its job. Silent.
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
