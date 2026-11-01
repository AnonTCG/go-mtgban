package starcitygames

import (
	"errors"
	"testing"

	"github.com/mtgban/go-mtgban/matcher"

	_ "github.com/mtgban/go-mtgban/matcher/fleshandblood"
)

func TestFabArtPosition(t *testing.T) {
	for _, tt := range []struct{ name, sku, want string }{
		{"Eye of Ophidia Art Card", "SGL-FAB-ANQ-P01-ENR", "top left"},
		{"Eye of Ophidia Art Card", "SGL-FAB-ANQ-P05-ENR", "middle center"},
		{"Heart of Fyendal Art Card", "SGL-FAB-ANQ-P10-ENR", "top left"},
		{"Heart of Fyendal Art Card", "SGL-FAB-ANQ-P14-ENR", "middle center"},
		{"Riches of Tropal-Dhani Art Card", "SGL-FAB-ANQ-P27-ENR", "bottom right"},
		{"Eye of Ophidia", "SGL-FAB-ANQ-002-ENN", ""},
		{"Eye of Ophidia Art Card", "SGL-FAB-ANQ-002-ENN", ""},
	} {
		if got := fabArtPosition(tt.name, tt.sku); got != tt.want {
			t.Errorf("fabArtPosition(%q, %q) = %q, want %q", tt.name, tt.sku, got, tt.want)
		}
	}
}

func TestFabPrintRunFinishes(t *testing.T) {
	for _, tt := range []struct{ set, finish, wantSet, wantFinish string }{
		{"Promotional Cards", "Gold Foil", "Promotional Cards", "Cold Foil"},
		{"Welcome to Rathe (1st Edition)", "Gold Foil", "Welcome to Rathe", "1st Edition Cold Foil"},
		{"Promotional Cards", "Cold Foil", "Promotional Cards", "Cold Foil"},
	} {
		set, finish := fabPrintRun(tt.set, tt.finish)
		if set != tt.wantSet || finish != tt.wantFinish {
			t.Errorf("fabPrintRun(%q, %q) = (%q, %q), want (%q, %q)", tt.set, tt.finish, set, finish, tt.wantSet, tt.wantFinish)
		}
	}
}

func TestFabTreatments(t *testing.T) {
	got := fabTreatments([]string{"red", "extended art", "blue"})
	if len(got) != 1 || got[0] != "extended art" {
		t.Errorf("fabTreatments = %v, want [extended art]", got)
	}
	if got := fabTreatments([]string{"red"}); len(got) != 0 {
		t.Errorf("fabTreatments(red) = %v, want none", got)
	}
}

// TestCatalogFabShapes pins the listings the catalog spells in ways the
// datastore does not, through the resolve path: a gold foil is the cold foil
// promo, a promo's finish cannot misname the one printing its number holds,
// an art card's number is its piece of the picture, the marker on a
// pitch-labelled base finds the extended art, a misspelt name and a
// misspelt number reach the printing, and an insert is no card.
func TestCatalogFabShapes(t *testing.T) {
	b := withGameDatastore(t, "fleshandblood", "FLESHANDBLOOD_PATH")

	product := func(sku, name, set, finish, group string) CatalogProduct {
		return CatalogProduct{
			SKU: sku, Name: name, Game: "Flesh and Blood", Set: set, ProductType: ProductTypeSingles,
			Finish: finish, FinishGroup: group, Language: "English",
		}
	}
	for _, tt := range []struct {
		desc string
		p    CatalogProduct
		want string
	}{
		{"gold foil", product("SGL-FAB-PRM-FAB_066-ENG", "Talismanic Lens", "Promotional Cards", "Gold Foil", "Alt Foil"), "fab066_275543_coldfoil"},
		{"promo finish misnamed", product("SGL-FAB-PRM-FAB_189-ENC", "Boast", "Promotional Cards", "Cold Foil", "Alt Foil"), "fab189_580618_rainbowfoil"},
		{"promo marvel misnamed", product("SGL-FAB-PRM-TNP_001-ENC", "Wild Ride (Red)", "Promotional Cards", "Cold Foil", "Alt Foil"), "tnp001_692575_rainbowfoil"},
		{"art card piece", product("SGL-FAB-ANQ-P08-ENR", "Eye of Ophidia Art Card", "Antiquity Pack", "Rainbow Foil", "Foil"), "688788_rainbowfoil"},
		{"art card of the next picture", product("SGL-FAB-ANQ-P14-ENR", "Heart of Fyendal Art Card", "Antiquity Pack", "Rainbow Foil", "Foil"), "688794_rainbowfoil"},
		{"pitch-labelled base", withTCGPlayerID(product("SGL-FAB-HNT-155-ENR", "Drop of Dragon Blood", "The Hunted", "Rainbow Foil", "Foil"), "614550"), "hnt155_611179_rainbowfoil"},
		{"pitch-swapped id", withTCGPlayerID(product("SGL-FAB-SEA-142-ENN", "Saltwater Swell (Yellow)", "High Seas", "Non-foil", "Non-foil"), "633285"), "sea142_633284"},
		{"first edition run", withTCGPlayerID(product("SGL-FAB-ELE1-080-ENR", "Rites of Replenishment (Yellow)", "Tales of Aria (1st Edition)", "Rainbow Foil", "Foil"), "248353"), "ele080_248353_1steditionrainbowfoil"},
		{"marker finds the extended art past the pitch label", product("SGL-FAB-HNT2-155-ENR", "Drop of Dragon Blood", "The Hunted", "Rainbow Foil", "Foil"), "hnt155_614550_rainbowfoil"},
		{"misspelt name", product("SGL-FAB-PEN-190-ENN", "Deep Recess of Existence", "Compendium of Rathe", "Non-foil", "Non-foil"), "pen190_676395"},
		{"art card named as the card", product("SGL-FAB-PRM-FAB_404-ENC", "Peace Art Card", "Promotional Cards", "Cold Foil", "Alt Foil"), "fab404_685513_coldfoil"},
		{"misspelt number", product("SGL-FAB-APS-056-ENR", "Attention Grabbers", "Armory Deck: Pleiades", "Rainbow Foil", "Foil"), "aps006_653806_rainbowfoil"},
		{"lettered marvels", product("SGL-FAB-MPG2-112b-ENC", "Seismic Surge", "Mastery Pack Guardian", "Cold Foil", "Alt Foil"), "mpg112_647746_coldfoil"},
	} {
		got, err := resolveProduct(b, GameFleshAndBlood, tt.p)
		if err != nil {
			t.Errorf("%s: resolveProduct(%s) = %v", tt.desc, tt.p.SKU, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%s: resolveProduct(%s) = %q, want %q", tt.desc, tt.p.SKU, got, tt.want)
		}
	}
	// A set card's finish keeps its say: the datastore sells no rainbow
	// Hyper Driver (Red), and the listing is refused rather than priced
	// as the cold foil.
	if got, err := resolveProduct(b, GameFleshAndBlood, product("SGL-FAB-DYN-110-ENR", "Hyper Driver (Red)", "Dynasty", "Rainbow Foil", "Foil")); err == nil {
		t.Errorf("resolveProduct(DYN-110 Rainbow Foil) = %q, want a refusal", got)
	}
	_, err := resolveProduct(b, GameFleshAndBlood, product("SGL-FAB-ROS-ART_000a-ENC", "Binder Label", "Rosetta", "Cold Foil", "Alt Foil"))
	if !errors.Is(err, matcher.ErrUnsupported) {
		t.Errorf("resolveProduct(Binder Label) = %v, want ErrUnsupported", err)
	}
}

// TestCatalogFabDoubleSided pins the three ways a "Front // Back" listing
// resolves: a pair the datastore files under the fused name lands on it, a
// pair it holds only as two faces is skipped, and a single-numbered listing
// is the front face with its token on the back.
func TestCatalogFabDoubleSided(t *testing.T) {
	b := withGameDatastore(t, "fleshandblood", "FLESHANDBLOOD_PATH")

	product := func(sku, name, set, rarity string) CatalogProduct {
		return CatalogProduct{
			SKU: sku, Name: name, Game: "Flesh and Blood", Set: set, Rarity: rarity,
			ProductType: ProductTypeSingles, Finish: "Non-foil", FinishGroup: "Non-foil",
			Language: "English",
		}
	}
	for _, tt := range []struct {
		desc string
		p    CatalogProduct
		want string
	}{
		{"fused pair", product("SGL-FAB-MST-003_159-ENN", "Beckoning Mistblade // Tiger Taming Khakkara", "Part the Mistveil", "Token"), "mst003-mst159_553466"},
		{"fused pair named in the other order", product("SGL-FAB-OMN-048_047-ENN", "Aurora, Emissary of Lightning // Aurora, Legacy of Tempest", "Omens of the Third Age", "Basic"), "omn047-omn048_724643"},
		{"front with its token", product("SGL-FAB-NUU-026-ENN", "Pass Over // Inner Chi", "Nuu Blitz Deck", "Common"), "nuu026_549970"},
	} {
		got, err := resolveProduct(b, GameFleshAndBlood, tt.p)
		if err != nil {
			t.Errorf("%s: resolveProduct(%s) = %v", tt.desc, tt.p.SKU, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%s: resolveProduct(%s) = %q, want %q", tt.desc, tt.p.SKU, got, tt.want)
		}
	}
	for _, p := range []CatalogProduct{
		product("SGL-FAB-MST-003_026-ENN", "Beckoning Mistblade // Enigma", "Part the Mistveil", "Token"),
	} {
		got, err := resolveProduct(b, GameFleshAndBlood, p)
		if !errors.Is(err, matcher.ErrUnsupported) {
			t.Errorf("faces only: resolveProduct(%s) = %q, %v, want ErrUnsupported", p.SKU, got, err)
		}
	}
}

// withTCGPlayerID gives a product the vendor id the catalog files it under:
// one id covers every print run and treatment of a card, so the product
// still has to land on its own.
func withTCGPlayerID(p CatalogProduct, id string) CatalogProduct {
	p.TCGPlayerID = id
	return p
}
