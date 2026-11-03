package starcitygames

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/mtgban/go-mtgban/mtgban"
)

// TestCatalogSpellings covers how a product only the index lists is worded:
// each id takes the spelling most of the products both list give it, and an id
// the catalog never spelled leaves the product unworded rather than plain.
func TestCatalogSpellings(t *testing.T) {
	var s catalogSpellings
	hit := sellListProduct{Finish: 35.0, Rarity: 8.0}
	for range 3 {
		s.learn(hit, CatalogProduct{Game: "Magic: The Gathering", Finish: "Double Rainbow Foil", FinishGroup: "Alt Foil", Rarity: "Promo"})
	}
	s.learn(hit, CatalogProduct{Game: "Magic: The Gathering", Finish: "Foil", Rarity: "Promo"})

	finish, _ := spelling(s.finishes, "35")
	group, _ := spelling(s.finishGroups, "35")
	if finish != "Double Rainbow Foil" || group != "Alt Foil" {
		t.Errorf("finish 35 spelled %q/%q, want the majority's Double Rainbow Foil/Alt Foil", finish, group)
	}
	_, found := spelling(s.finishes, "1")
	if found {
		t.Error("a finish the catalog never spelled was given a spelling")
	}

	// The index has no number for a sealed product's finish or rarity.
	s.learn(sellListProduct{Finish: "", Rarity: ""}, CatalogProduct{Finish: "Non-foil", Rarity: "Common"})
	if len(s.finishes) != 1 || len(s.rarities) != 1 {
		t.Errorf("a product with no finish or rarity number taught a spelling: %v %v", s.finishes, s.rarities)
	}

	language, found := catalogLanguage("zt")
	if !found || language != "Chinese - Traditional" {
		t.Errorf("zt read as %q, want Chinese - Traditional", language)
	}
	_, found = catalogLanguage("xx")
	if found {
		t.Error("an unknown language code was read as a language")
	}
}

// TestFileListed covers the singles the index bids on that the catalog does
// not list, as both carried them on 2026-11-01: each is priced on the printing
// its sku names or not at all, and one the catalog does list is left to it.
func TestFileListed(t *testing.T) {
	b := withMagic(t)

	const (
		nonfoil = 1.0
		foil    = 2.0
		rare    = 3.0
		promo   = 8.0
	)
	hit := func(name, set, language, number string, finish, rarity float64) sellListProduct {
		return sellListProduct{Name: name, SetName: set, Language: language, CollectorNumber: number, Finish: finish, Rarity: rarity}
	}
	listed := map[string]sellListProduct{
		// Catalogued, and where the catalog's spellings are learned from.
		"SGL-MTG-3BB-88-ITN":           hit("Vesuvan Doppelganger", "3rd Edition - Black Border", "it", "88", nonfoil, rare),
		"SGL-MTG-PRM-30A_2023_004-ENF": hit("Wall of Roots", "Promo", "en", "004", foil, promo),
		// Only the index lists these.
		"SGL-MTG-3BB-105-ITN":           hit("Demonic Tutor", "3rd Edition - Black Border", "it", "105", nonfoil, rare),
		"SGL-MTG-WCHP-99KB_ULG_126-ENN": hit("Grim Monolith", "World Championships", "en", "126", nonfoil, rare),
		// Foreign Black Border is Italian only.
		"SGL-MTG-3BB-105-DEN": hit("Demonic Tutor", "3rd Edition - Black Border", "de", "105", nonfoil, rare),
		// The year leads the number, so 2000-2004 keep it.
		"SGL-MTG-WCHP-01JT_MMQ_324-ENN": hit("Rishadan Port", "World Championships", "en", "324", nonfoil, rare),
		// A sideboard card the main deck also plays.
		"SGL-MTG-WCHP-03PK_JUD_062s-ENN": hit("Cabal Therapy", "World Championships", "en", "062", nonfoil, rare),
		// The foil 30th Anniversary Tokyo card, whose name finds only the
		// unfoiled media promo.
		"SGL-MTG-PRM-30C_2023_002-JAF": hit("Shivan Dragon", "Promo", "ja", "002", foil, promo),
		"SGL-MTG-PRM-DM_2023_003-JAF":  hit("Serra Angel", "Promo", "ja", "DM_2023_003", foil, promo),
		"SGL-MTG-PRM-JSS_J03_001-ENF":  hit("Giant Growth", "Promo", "en", "008", foil, promo),
	}

	scg, err := NewScraper(b, "")
	if err != nil {
		t.Fatal(err)
	}
	var logs []string
	scg.logCallback = func(format string, a ...any) { logs = append(logs, fmt.Sprintf(format, a...)) }
	scg.listed = listed
	scg.offers = map[string]map[mtgban.Condition]sellListVariant{}
	for sku := range listed {
		scg.offers[sku] = map[mtgban.Condition]sellListVariant{
			mtgban.NM: {SKU: sku + "1", BuyPrice: 10},
			mtgban.HP: {SKU: sku + "3", BuyPrice: 5},
		}
	}
	scg.processProduct(CatalogProduct{
		SKU: "SGL-MTG-3BB-88-ITN", Name: "Vesuvan Doppelganger", Game: "Magic: The Gathering",
		Set: "3rd Edition - Black Border", ProductType: ProductTypeSingles, Language: "Italian",
		CollectorNumber: "88", Finish: "Non-foil", FinishGroup: "Non-foil", Rarity: "Rare",
	})
	scg.processProduct(CatalogProduct{
		SKU: "SGL-MTG-PRM-30A_2023_004-ENF", Name: "Wall of Roots", Game: "Magic: The Gathering",
		Set: "Promo", ProductType: ProductTypeSingles, Language: "English", ScryfallID: "148bbff4-ac34-45d1-9091-323947a6ff67",
		CollectorNumber: "004", Finish: "Foil", FinishGroup: "Foil", Rarity: "Promo",
	})
	scg.fileListed()

	got := map[string]string{}
	grades := map[string]int{}
	for id, entries := range scg.buylist {
		co, _ := b.GetUUID(id)
		for _, entry := range entries {
			got[entry.OriginalID] = co.SetCode + " " + co.Number
			grades[entry.OriginalID]++
		}
	}
	want := map[string]string{
		"SGL-MTG-3BB-88-ITN":             "FBB 88",
		"SGL-MTG-PRM-30A_2023_004-ENF":   "P30A 4",
		"SGL-MTG-3BB-105-ITN":            "FBB 105",
		"SGL-MTG-WCHP-99KB_ULG_126-ENN":  "WC99 kb126",
		"SGL-MTG-WCHP-01JT_MMQ_324-ENN":  "WC01 jt324",
		"SGL-MTG-WCHP-03PK_JUD_062s-ENN": "WC03 pk62sb",
		"SGL-MTG-PRM-30C_2023_002-JAF":   "P30T 2",
		"SGL-MTG-PRM-DM_2023_003-JAF":    "PMDA 3",
		"SGL-MTG-PRM-JSS_J03_001-ENF":    "PSUS 8",
	}
	if !maps.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	for sku := range want {
		if grades[sku] != 4 {
			t.Errorf("%s filed at %d grades, want NM, HP, and PL and MP at HP's price", sku, grades[sku])
		}
	}

	summary := "[SCG] 8 singles only the sell list carries, 0 worded in a way the catalog never spelled, 0 landed on another printing"
	if !slices.Contains(logs, summary) {
		t.Errorf("the run said %q, want the summary %q", logs, summary)
	}
}
