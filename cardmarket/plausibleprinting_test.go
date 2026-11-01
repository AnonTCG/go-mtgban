package cardmarket

import (
	"testing"

	cm "github.com/mtgban/go-cardmarket"

	_ "github.com/mtgban/go-mtgban/matcher/magic"
)

// TestPlausiblePrintingDefersOnImplausibleWCD pins plausiblePrinting's own
// rule directly: a World Championship Deck expansion only ever sells a
// WC97-WC04 memorabilia printing, a Pro Tour 1996 one a PTC printing, and
// an Oversized shelf only ever sells an oversized one. mtgjson links 249423,
// Rushing River (V.2) under WCD 2002: Raphael Levy, to The List's PLS-30
// rather than a WC02 printing, so this is not a synthetic shape.
func TestPlausiblePrintingDefersOnImplausibleWCD(t *testing.T) {
	b := realDatastore(t)

	for _, tt := range []struct {
		name          string
		expansionName string
		cardID        string
		want          bool
	}{
		{"a WCD product whose only candidate is a Brothers' War printing", "WCD 2000: Janosch Kühn", "a1a10730-b01e-5b6f-a9a4-1ae999729ef4", false},
		{"a WCD product whose candidate is its own WC00 printing", "WCD 2000: Janosch Kühn", "e2b509c6-cb8a-5170-b0cb-7093e0823747", true},
		{"a Pro Tour 1996 product whose only candidate is a Brothers' War printing", "Pro Tour 1996: Mark Justice", "a1a10730-b01e-5b6f-a9a4-1ae999729ef4", false},
		{"a Pro Tour 1996 product whose candidate is its own PTC printing", "Pro Tour 1996: Mark Justice", "693c127a-d748-5fc8-9b6e-94c3448a7ac3", true},
		{"an Oversized product whose only candidate is an ordinary-sized printing", "Oversized 6x9 Promos", "bf14fe40-3e5c-5790-bb75-4c8221c04883", false},
		{"a Foreign Black Bordered product whose only candidate is Final Fantasy's Plains", "Foreign Black Bordered", "920b17b6-beb9-510d-982a-01941cd2420d", false},
		{"a Foreign Black Bordered product whose candidate is its own FBB printing", "Foreign Black Bordered", "046f7759-47f4-5a8a-9627-ccb953da9394", true},
		{"an expansion neither guard constrains", "Judge Rewards Promos", "bf14fe40-3e5c-5790-bb75-4c8221c04883", true},
		{"a Portal product whose candidate is the Simplified Chinese alternate art", "Portal", "e019addd-eaf5-5aa7-865c-3c2c9a76f6e3", false},
		{"the alternate art's own shelf", "Simplified Chinese Alternate Art Cards", "e019addd-eaf5-5aa7-865c-3c2c9a76f6e3", true},
		{"an empty id decides nothing either way", "WCD 2000: Janosch Kühn", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := plausiblePrinting(b, tt.expansionName, tt.cardID); got != tt.want {
				t.Errorf("plausiblePrinting(%q, %q) = %v, want %v", tt.expansionName, tt.cardID, got, tt.want)
			}
		})
	}
}

// TestResolveUUIDsDefersOnImplausibleOversized pins the map route this guard
// closes: resolveMapped answers from resolveUUIDs before Fallback's own mcmId
// check ever runs, so without the guard a map entry naming an implausible
// uuid would price straight through it. 21364 and 21387 are two Oversized
// 6x9 Promos products, each handed the ordinary-sized 30th Anniversary
// Edition printing of its card in place of the oversized one the map links.
func TestResolveUUIDsDefersOnImplausibleOversized(t *testing.T) {
	b := realDatastore(t)
	r := &resolver{backend: b, gameID: cm.GameMagic}

	for _, tt := range []struct {
		name    string
		product cm.Product
		uuids   []string
	}{
		{
			"21364 Nether Shadow handed the ordinary 30A printing",
			cm.Product{IDProduct: 21364, Name: "Nether Shadow (V.2)", ExpansionName: "Oversized 6x9 Promos"},
			[]string{"bf14fe40-3e5c-5790-bb75-4c8221c04883"},
		},
		{
			"21387 Swords to Plowshares handed the ordinary 30A printing",
			cm.Product{IDProduct: 21387, Name: "Swords to Plowshares (V.2)", ExpansionName: "Oversized 6x9 Promos"},
			[]string{"e411a1e9-bc7a-58bb-b436-b0b45b37f27a"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cardID, cardIDFoil := r.resolveUUIDs(&tt.product, tt.uuids)
			if cardID != "" || cardIDFoil != "" {
				co, _ := b.GetUUID(cardID)
				t.Fatalf("resolveUUIDs = (%q, %q), want (\"\", \"\") - kept %s instead of falling through to resolveProduct", cardID, cardIDFoil, co)
			}
		})
	}
}

// TestResolveUUIDsFollowsTheProductNumber pins numberedPrinting: mtgjson
// links Double Masters 2022's Consecrated Sphinx #345 and #427 each to the
// other's Cardmarket product, and the product's own number, which CardTrader
// agrees with, names the printing. The ids and entries are the map's own.
func TestResolveUUIDsFollowsTheProductNumber(t *testing.T) {
	b := realDatastore(t)
	r := &resolver{backend: b, gameID: cm.GameMagic}

	for _, tt := range []struct {
		product          cm.Product
		uuids            []string
		wantID, wantFoil string
	}{
		{
			cm.Product{IDProduct: 664915, Name: "Consecrated Sphinx (V.1)", Number: "345", ExpansionName: "Double Masters 2022: Extras"},
			[]string{"bf3e52bb-4017-5760-8db6-838ee858421d"},
			"106eb1f8-1452-55cd-8bfb-7db43bc5a3ce", "106eb1f8-1452-55cd-8bfb-7db43bc5a3ce_f",
		},
		{
			cm.Product{IDProduct: 664250, Name: "Consecrated Sphinx (V.2)", Number: "427", ExpansionName: "Double Masters 2022: Extras"},
			[]string{"106eb1f8-1452-55cd-8bfb-7db43bc5a3ce"},
			"bf3e52bb-4017-5760-8db6-838ee858421d", "bf3e52bb-4017-5760-8db6-838ee858421d",
		},
	} {
		cardID, cardIDFoil := r.resolveUUIDs(&tt.product, tt.uuids)
		if cardID != tt.wantID || cardIDFoil != tt.wantFoil {
			t.Errorf("%d #%s: resolveUUIDs = (%q, %q), want (%q, %q)", tt.product.IDProduct, tt.product.Number, cardID, cardIDFoil, tt.wantID, tt.wantFoil)
		}
	}
}

// TestResolveUUIDsKeepsTokenProductsOnTokens pins the Token-rarity guard:
// mtgjson links 372511, the Nissa, Who Shakes the World emblem, and 775672,
// The Capitoline Triad emblem, to the real cards of the same name, whose own
// products would then lose their price to the emblem's.
func TestResolveUUIDsKeepsTokenProductsOnTokens(t *testing.T) {
	b := realDatastore(t)
	r := &resolver{backend: b, gameID: cm.GameMagic}

	for _, tt := range []struct {
		name    string
		product cm.Product
		uuid    string
		kept    bool
	}{
		{"Nissa emblem handed the BLC 84 card", cm.Product{IDProduct: 372511, Name: "Nissa, Who Shakes the World Emblem", ExpansionName: "War of the Spark", Rarity: "Token"}, "e4c4a598-77f3-5773-bf87-5acbf1cc4916", false},
		{"Nissa emblem handed its own TWAR 19 token", cm.Product{IDProduct: 372511, Name: "Nissa, Who Shakes the World Emblem", ExpansionName: "War of the Spark", Rarity: "Token"}, "7f8a2b3a-365f-5825-8d10-4ce16e3cd201", true},
		{"Capitoline Triad emblem handed the ACR 1 card", cm.Product{IDProduct: 775672, Name: "The Capitoline Triad Emblem", ExpansionName: "Universes Beyond: Assassin's Creed: Tokens", Rarity: "Token"}, "d1c00a42-14a5-5480-a704-b1c457f177db", false},
		{"a card product keeps its own card", cm.Product{IDProduct: 778271, Name: "Nissa, Who Shakes the World (V.1)", ExpansionName: "Commander: Bloomburrow"}, "e4c4a598-77f3-5773-bf87-5acbf1cc4916", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cardID, cardIDFoil := r.resolveUUIDs(&tt.product, []string{tt.uuid})
			if kept := cardID != "" || cardIDFoil != ""; kept != tt.kept {
				t.Errorf("resolveUUIDs kept = %v (%q, %q), want %v", kept, cardID, cardIDFoil, tt.kept)
			}
		})
	}
}
