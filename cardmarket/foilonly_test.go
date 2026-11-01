package cardmarket

import (
	"fmt"
	"testing"

	cm "github.com/mtgban/go-cardmarket"

	"github.com/mtgban/go-mtgban/matcher"

	_ "github.com/mtgban/go-mtgban/matcher/lorcana"
	_ "github.com/mtgban/go-mtgban/matcher/yugioh"
)

// foilOnlyDatastore carries two printings of one Lorcana set: a card sold
// both plain and foiled, and an Enchanted sold in a holofoil alone. The
// second is the shape marketLoneFlag exists for - Cardmarket sells it as one
// product, so it resolves to a single id in both of a product's slots.
const foilOnlyDatastore = `{"data": {
  "metadata": {"formatVersion": "2.3.5", "language": "en"},
  "sets": {"13": {"name": "Attack of the Vine!", "type": "expansion", "releaseDate": "2026-08-14"}},
  "cards": [
    {
      "id": 1, "name": "Meilin Lee", "fullName": "Meilin Lee - Popular Red Panda",
      "setCode": "13", "number": "125", "rarity": "Legendary", "type": "Character",
      "color": "Amber", "colors": ["Amber"], "story": "Turning Red", "printings": [{"finish": "Cold Foil", "id": "1_foil"}, {"finish": "Normal", "id": "1"}],
      "externalLinks": {"cardmarketId": 897323}
    },
    {
      "id": 2, "name": "Meilin Lee", "fullName": "Meilin Lee - Popular Red Panda",
      "setCode": "13", "number": "240", "rarity": "Enchanted", "type": "Character",
      "color": "Amber", "colors": ["Amber"], "story": "Turning Red", "printings": [{"finish": "Holofoil", "id": "2_holofoil"}],
      "externalLinks": {"cardmarketId": 897451}
    }
  ]
}}`

// TestMarketFoilOnly pins which finish a product's first query asks for. The
// plain printing is asked for as a plain card and its foil twin separately;
// the Enchanted, which answers both of a product's ids with one uuid, is
// asked for as a foil, because a query for its non-foil listings rejects
// every listing the product has.
func TestMarketFoilOnly(t *testing.T) {
	b := datastoreBackend(t, "lorcana", foilOnlyDatastore)

	// The ids the resolver hands queryPrintings, as the datastore mints
	// them: the plain printing and its foil twin are two, the Enchanted is
	// one that stands in both slots.
	for _, tt := range []struct {
		name               string
		cardID, cardIDFoil string
		want               bool
	}{
		{"a printing with both finishes asks for its plain listings", "1", "1_foil", false},
		{"a foil-only printing asks for its foil listings", "2_holofoil", "2_holofoil", true},
		{"an empty foil slot names that same single printing", "2_holofoil", "", true},
		{"a plain id with no foil twin stays a plain query", "1", "", false},
		{"an unknown id asks for nothing in particular", "nope", "nope", false},
	} {
		if got := marketLoneFlag(b, "isFoil", tt.cardID, tt.cardIDFoil); got != tt.want {
			t.Errorf("%s: marketLoneFlag(%q, %q) = %v, want %v",
				tt.name, tt.cardID, tt.cardIDFoil, got, tt.want)
		}
	}
}

// TestMarketFirstEdOnly pins the same for Yu-Gi-Oh's print runs: a printing
// made only in 1st Edition answers both ids with itself and asks for its 1st
// Edition listings, where a card printed in both runs asks for its
// Unlimited ones first.
func TestMarketFirstEdOnly(t *testing.T) {
	b := datastoreBackend(t, "yugioh", ygoDatastore)

	for _, tt := range []struct {
		name               string
		cardID, cardIDFoil string
		want               bool
	}{
		{"a card in both runs asks for its Unlimited listings", "dcr-005_22823_unlimited", "dcr-005_22823_1stedition", false},
		{"a 1st Edition only printing asks for its 1st Edition listings", "sgx1-end19_266282_1stedition", "sgx1-end19_266282_1stedition", true},
		{"a product resolved to the 1st Edition run asks for that run", "dcr-005_22823_1stedition", "dcr-005_22823_1stedition", true},
		{"a Limited printing carries no 1st Edition flag", "sece-ens14_96145_limited", "", false},
	} {
		if got := marketLoneFlag(b, "isFirstEd", tt.cardID, tt.cardIDFoil); got != tt.want {
			t.Errorf("%s: marketLoneFlag(%q, %q) = %v, want %v",
				tt.name, tt.cardID, tt.cardIDFoil, got, tt.want)
		}
	}
}

// TestFoilOnlyShelf pins the closed set of shelves whose guide entry prices
// the shown treatment's foil alone - verbatim product names from the shelves
// the README documents, plus the edge cases the closed table is built
// around: M3C Extras' "(V.1)" and "(V.3)" printings are not foil-only, and
// the Holiday Release name is shared by LTC's silverfoil box topper and
// LTR's silverfoil showcase, both foil-only in "(V.2)".
func TestFoilOnlyShelf(t *testing.T) {
	for _, tt := range []struct {
		name    string
		product cm.Product
		setCode string
		want    bool
	}{
		{"FIC Collector's Edition", cm.Product{Name: "Summon: Esper Valigarmanda", ExpansionName: "Commander: FINAL FANTASY: Collector's Edition"}, "FIC", true},
		{"MSC Collector's Edition", cm.Product{Name: "Iron Man, Armored Avenger", ExpansionName: "Commander: Marvel Super Heroes: Collector's Edition"}, "MSC", true},
		{"TMC Extras", cm.Product{Name: "Baxter, Fly in the Ointment", ExpansionName: "Commander: Teenage Mutant Ninja Turtles: Extras"}, "TMC", true},
		{"M3C Extras with no (V.N) suffix", cm.Product{Name: "Drowner of Hope", ExpansionName: "Commander: Modern Horizons 3: Extras"}, "M3C", true},
		{"M3C Extras (V.2)", cm.Product{Name: "Localized Destruction (V.2)", ExpansionName: "Commander: Modern Horizons 3: Extras"}, "M3C", true},
		{"M3C Extras (V.1) is the ordinary nonfoil printing", cm.Product{Name: "Sunken Palace (V.1)", ExpansionName: "Commander: Modern Horizons 3: Extras"}, "M3C", false},
		{"M3C Extras (V.3) is an extended art sold in both finishes", cm.Product{Name: "Coram, the Undertaker (V.3)", ExpansionName: "Commander: Modern Horizons 3: Extras"}, "M3C", false},
		{"LTC Holiday Release (V.2) is the silverfoil box topper", cm.Product{Name: "Kenrith, the Returned King (V.2)", ExpansionName: "The Lord of the Rings: Tales of Middle-earth Holiday Release"}, "LTC", true},
		{"LTR Holiday Release (V.2) is the silverfoil showcase", cm.Product{Name: "Boromir, Warden of the Tower (V.2)", ExpansionName: "The Lord of the Rings: Tales of Middle-earth Holiday Release"}, "LTR", true},
		{"LTC Holiday Release (V.1) is the ordinary printing", cm.Product{Name: "Kenrith, the Returned King (V.1)", ExpansionName: "The Lord of the Rings: Tales of Middle-earth Holiday Release"}, "LTC", false},
		{"an unrelated shelf", cm.Product{Name: "Lightning Bolt", ExpansionName: "Modern Horizons 2"}, "MH2", false},
	} {
		if got := foilOnlyShelf(&tt.product, tt.setCode); got != tt.want {
			t.Errorf("%s: foilOnlyShelf(%+v, %q) = %v, want %v", tt.name, tt.product, tt.setCode, got, tt.want)
		}
	}
}

// TestResolveUUIDsFoilOnlyShelf replays id-map rows of the foil-only shelves
// through resolveUUIDs, which answers them before resolveMagic is asked.
// Each foil-only row lands on the foil it sells; the rows beside it that are
// not foil-only keep the printing the map names.
func TestResolveUUIDsFoilOnlyShelf(t *testing.T) {
	b := realDatastore(t)
	r := &resolver{backend: b, gameID: cm.GameMagic}

	const holiday = "The Lord of the Rings: Tales of Middle-earth Holiday Release"
	const m3c = "Commander: Modern Horizons 3: Extras"
	for _, tt := range []struct {
		id                  int
		name, number, shelf string
		uuid                string
		wantCard, wantFoil  string
	}{
		{737046, "Champions of Minas Tirith (V.2)", "412", holiday, "98cba698-44ab-5e8a-8b53-0b3ce52055ae", "LTC 412 foil", "LTC 412 foil"},
		{737499, "Boromir, Warden of the Tower (V.2)", "455", holiday, "95dbc038-e376-5baa-ac6e-13ad4f876e2b", "LTR 455 foil", "LTR 455 foil"},
		{774790, "Talon Gates of Madara (V.2)", "82", m3c, "9d648f31-1b88-5276-a9e9-597a81e68b3e", "M3C 82★ foil", "M3C 82★ foil"},
		{738041, "Eagle of Deliverance (V.2)", "829", holiday, "ab52a745-6c92-5260-9555-79e3a725ac9c", "LTR 829 nonfoil", "LTR 829 nonfoil"},
		{771323, "Coram, the Undertaker (V.3)", "27", m3c, "18a9a4b6-6953-5cc3-a1db-194ca5ecaf08", "M3C 27 nonfoil", "M3C 27 foil"},
		{772487, "Azlask, the Swelling Scourge (V.2)", "17", m3c, "12d79207-f3e8-5ef8-b736-b4f979cc1c42", "M3C 17 etched", "M3C 17 etched"},
	} {
		product := &cm.Product{IDProduct: tt.id, Name: tt.name, Number: tt.number, ExpansionName: tt.shelf}
		cardID, cardIDFoil := r.resolveUUIDs(product, []string{tt.uuid})
		if got := describePrinting(b, cardID); got != tt.wantCard {
			t.Errorf("%d %q: card = %s, want %s", tt.id, tt.name, got, tt.wantCard)
		}
		if got := describePrinting(b, cardIDFoil); got != tt.wantFoil {
			t.Errorf("%d %q: foil card = %s, want %s", tt.id, tt.name, got, tt.wantFoil)
		}
	}
}

func describePrinting(b *matcher.Backend, id string) string {
	co, err := b.GetUUID(id)
	if err != nil {
		return fmt.Sprintf("%q (%v)", id, err)
	}
	finish := "nonfoil"
	switch {
	case co.Etched:
		finish = "etched"
	case co.Foil:
		finish = "foil"
	}
	return co.SetCode + " " + co.Number + " " + finish
}

// TestSoldFoilApart replays the V.1 rows of the shelves that sell a card's
// foil as the V.2 product through resolveMapped: a V.1 with a V.2 beside it
// gives its foil column up, and one without keeps it.
func TestSoldFoilApart(t *testing.T) {
	b := realDatastore(t)
	const m3c, w40k = 5657, 5074
	catalog := &cm.Catalog{Data: cm.CatalogData{
		Expansions: map[int]cm.CatalogExpansion{
			m3c:  {Name: "Commander: Modern Horizons 3: Extras", Code: "XM3C"},
			w40k: {Name: "Universes Beyond: Warhammer 40,000", Code: "40K"},
		},
		Products: map[int]cm.CatalogProduct{
			773244: {ExpansionID: m3c, Name: "Barrowgoyf (V.1)", Number: "50", Version: 1},
			774752: {ExpansionID: m3c, Name: "Barrowgoyf (V.2)", Number: "50", Version: 2},
			675703: {ExpansionID: w40k, Name: "Celestine, the Living Saint (V.1)", Number: "10", Version: 1},
			764123: {ExpansionID: w40k, Name: "Celestine, the Living Saint (V.2)", Number: "10", Version: 2},
			671290: {ExpansionID: w40k, Name: "Sol Ring (V.1)", Number: "249", Version: 1},
		},
	}}
	r := &resolver{backend: b, gameID: cm.GameMagic, catalog: catalog}

	for _, tt := range []struct {
		id        int
		wantApart bool
	}{
		{773244, true},
		{675703, true},
		{671290, false},
	} {
		product := catalog.Data.Products[tt.id]
		exp := catalog.Data.Expansions[product.ExpansionID]
		res := r.resolveMapped(tt.id, product, cm.Expansion{IDExpansion: product.ExpansionID, Name: exp.Name, SetCode: exp.Code})
		if res.cardID == "" {
			t.Errorf("%d %q: no printing", tt.id, product.Name)
			continue
		}
		if apart := res.cardIDFoil == res.cardID; apart != tt.wantApart {
			t.Errorf("%d %q: foil column given up = %v, want %v", tt.id, product.Name, apart, tt.wantApart)
		}
		if !tt.wantApart && res.cardIDFoil == "" {
			t.Errorf("%d %q: no foil printing kept", tt.id, product.Name)
		}
	}
}
