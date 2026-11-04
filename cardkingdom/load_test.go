package cardkingdom

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mtgban/go-cardkingdom"
)

// loadFixture is rows of Card Kingdom's pricelist of 2026-10-02 as
// published, but for the last two: Air Elemental is given no NM price, and
// Blood Lust no price at all.
const loadFixture = `{"meta": {"created_at": "2026-10-02 05:05:11", "base_url": "https://www.cardkingdom.com/"}, "data": [
{"id": 65155, "sku": "PTK-014", "scryfall_id": "f484d47a-fb1d-4746-8f1d-dd9d24e67c1a", "url": "mtg/portal-3k/pang-tong-young-phoenix", "name": "Pang Tong, \"Young Phoenix\"", "variation": "", "edition": "Portal 3K", "is_foil": "false", "price_retail": "69.99", "qty_retail": 4, "price_buy": "32.00", "qty_buying": 2, "condition_values": {"nm_price": "69.99", "nm_qty": 0, "ex_price": "59.49", "ex_qty": 1, "vg_price": "52.49", "vg_qty": 3, "g_price": "45.49", "g_qty": 0}},
{"id": 13048, "sku": "2ED-055", "scryfall_id": "7c666b4b-c4ff-40ca-9d16-c76aafebaa83", "url": "mtg/unlimited/counterspell", "name": "Counterspell", "variation": "", "edition": "Unlimited", "is_foil": "false", "price_retail": "84.99", "qty_retail": 0, "price_buy": "42.50", "qty_buying": 6, "condition_values": {"nm_price": "84.99", "nm_qty": 0, "ex_price": "67.99", "ex_qty": 0, "vg_price": "50.99", "vg_qty": 0, "g_price": "34.00", "g_qty": 0}},
{"id": 21218, "sku": "LEG-292", "scryfall_id": "c062cbae-ce5e-43be-9932-c81a0a3622e8", "url": "mtg/legends/relic-barrier", "name": "Relic Barrier", "variation": "", "edition": "Legends", "is_foil": "false", "price_retail": "12.99", "qty_retail": 20, "price_buy": "5.25", "qty_buying": 0, "condition_values": {"nm_price": "12.99", "nm_qty": 12, "ex_price": "10.39", "ex_qty": 2, "vg_price": "9.09", "vg_qty": 6, "g_price": "6.50", "g_qty": 0}},
{"id": 39000, "sku": "FNEM-001", "scryfall_id": "871ad2f3-1dd2-45ea-881d-529aad3b76ec", "url": "mtg/nemesis/angelic-favor-foil", "name": "Angelic Favor", "variation": "", "edition": "Nemesis", "is_foil": "true", "price_retail": "1.49", "qty_retail": 4, "price_buy": "0.45", "qty_buying": 10, "condition_values": {"nm_price": "1.49", "nm_qty": 0, "ex_price": "1.19", "ex_qty": 4, "vg_price": "0.89", "vg_qty": 0, "g_price": "0.60", "g_qty": 0}},
{"id": 10001, "sku": "4ED-059", "scryfall_id": "e5cfaefb-764c-4c56-bdb3-5f0375168597", "url": "mtg/4th-edition/air-elemental", "name": "Air Elemental", "variation": "", "edition": "4th Edition", "is_foil": "false", "price_retail": "0.35", "qty_retail": 23, "price_buy": "0.02", "qty_buying": 14, "condition_values": {"nm_price": "0.00", "nm_qty": 2, "ex_price": "0.28", "ex_qty": 3, "vg_price": "0.25", "vg_qty": 13, "g_price": "0.18", "g_qty": 5}},
{"id": 10033, "sku": "4ED-178", "scryfall_id": "426e477f-2873-4115-9527-a50a97769dd1", "url": "mtg/4th-edition/blood-lust", "name": "Blood Lust", "variation": "", "edition": "4th Edition", "is_foil": "false", "price_retail": "0.00", "qty_retail": 35, "price_buy": "0.01", "qty_buying": 10, "condition_values": {"nm_price": "0.00", "nm_qty": 3, "ex_price": "0.28", "ex_qty": 20, "vg_price": "0.25", "vg_qty": 11, "g_price": "0.18", "g_qty": 1}}
]}`

// TestLoadPricesEachGrade pins what a pricelist row becomes: a retail entry
// per grade in stock, and an offer per grade from Card Kingdom's schedule.
func TestLoadPricesEachGrade(t *testing.T) {
	b := realDatastore(t)
	path := filepath.Join(t.TempDir(), "pricelist.json")
	err := os.WriteFile(path, []byte(loadFixture), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	ck := NewScraperLocal(b, path)
	ck.partner = "ban"
	ck.preserveOOS = true
	err = ck.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	got := map[string][]string{}
	keyOf := func(uuid string) string {
		co, _ := b.GetUUID(uuid)
		key := co.Name + " " + co.SetCode
		if co.Foil {
			key += " foil"
		}
		return key
	}
	for uuid, entries := range ck.inventory {
		for _, entry := range entries {
			got[keyOf(uuid)] = append(got[keyOf(uuid)], fmt.Sprintf("sells %s %.2f x%d", entry.Conditions, entry.Price, entry.Quantity))
		}
	}
	for uuid, entries := range ck.buylist {
		for _, entry := range entries {
			got[keyOf(uuid)] = append(got[keyOf(uuid)], fmt.Sprintf("%s buys %s %.2f", entry.VendorName, entry.Conditions, entry.BuyPrice))
		}
	}

	want := map[string][]string{
		`Pang Tong, "Young Phoenix" PTK`: {
			"sells SP 59.49 x1", "sells MP 52.49 x3",
			"Card Kingdom buys NM 32.00", "Card Kingdom buys SP 27.20",
			"Card Kingdom buys MP 24.00", "Card Kingdom buys HP 20.80",
		},
		// Out of stock: only the link is kept
		"Counterspell 2ED": {
			"sells NM 0.00 x1",
			"Card Kingdom buys NM 42.50", "Card Kingdom buys SP 34.00",
			"Card Kingdom buys MP 25.50", "Card Kingdom buys HP 17.00",
		},
		// Not buying: the price stands as the last one known
		"Relic Barrier LEG": {
			"sells NM 12.99 x12", "sells SP 10.39 x2", "sells MP 9.09 x6",
			"Card Kingdom (last known) buys NM 5.25", "Card Kingdom (last known) buys SP 4.20",
			"Card Kingdom (last known) buys MP 3.68", "Card Kingdom (last known) buys HP 2.63",
		},
		"Angelic Favor NEM foil": {
			"sells SP 1.19 x4",
			"Card Kingdom buys NM 0.45", "Card Kingdom buys SP 0.34",
			"Card Kingdom buys MP 0.23", "Card Kingdom buys HP 0.14",
		},
		// The NM price falls back to the retail price
		"Air Elemental 4ED": {
			"sells NM 0.35 x2", "sells SP 0.28 x3", "sells MP 0.25 x13", "sells HP 0.18 x5",
			"Card Kingdom buys NM 0.02", "Card Kingdom buys SP 0.02",
			"Card Kingdom buys MP 0.01", "Card Kingdom buys HP 0.01",
		},
	}
	for key, lines := range want {
		slices.Sort(lines)
		slices.Sort(got[key])
		if !slices.Equal(got[key], lines) {
			t.Errorf("%s:\ngot  %q\nwant %q", key, got[key], lines)
		}
	}
	for key := range got {
		_, found := want[key]
		if !found {
			t.Errorf("%s priced, want nothing: %q", key, got[key])
		}
	}

	// The fields a link and the buylist import read, on one card
	for uuid, entries := range ck.inventory {
		if keyOf(uuid) != `Pang Tong, "Young Phoenix" PTK` {
			continue
		}
		if entries[0].URL != "https://www.cardkingdom.com/mtg/portal-3k/pang-tong-young-phoenix?partner=ban&utm_campaign=ban&utm_medium=affiliate&utm_source=ban" {
			t.Errorf("retail link %s", entries[0].URL)
		}
		if entries[0].CustomFields["RetailPrice"] != "69.99" {
			t.Errorf("retail fields %v, want the NM price beside a lower grade", entries[0].CustomFields)
		}
		offer := ck.buylist[uuid][0]
		if offer.CustomFields["CKTitle"] != `Pang Tong, "Young Phoenix"` || offer.CustomFields["CKSKU"] != "PTK-014" {
			t.Errorf("NM offer fields %v", offer.CustomFields)
		}
		if fmt.Sprintf("%.2f", offer.PriceRatio) != "45.72" {
			t.Errorf("price ratio %.2f, want 45.72", offer.PriceRatio)
		}
		link, err := url.Parse(offer.URL)
		if err != nil {
			t.Fatal(err)
		}
		query := link.Query()
		if query.Get("filter[name]") != "Pang Tong, Young Phoenix" || query.Get("filter[edition]") != "portal-3k" || query.Get("partner") != "ban" {
			t.Errorf("buylist link %s", offer.URL)
		}
	}
}

// TestGradedBuyPrices pins each row of Card Kingdom's schedule, the prices that
// open them, and which URLs put a foil on the Foils row.
func TestGradedBuyPrices(t *testing.T) {
	tests := []struct {
		name string
		card cardkingdom.Product
		want [4]float64
	}{
		{
			name: "a labelled foil stays on the foil row under the top price row",
			card: gradedRow("Nemesis", "", "mtg/nemesis/blinding-angel-foil", true, 55),
			want: [4]float64{55, 41.25, 27.5, 16.5},
		},
		{
			name: "a labelled foil stays on the foil row over the top price row",
			card: gradedRow("Nemesis", "", "mtg/nemesis/parallax-wave-foil", true, 325),
			want: [4]float64{325, 243.75, 162.5, 97.5},
		},
		{
			name: "a cheap foil rounds half up",
			card: gradedRow("Nemesis", "", "mtg/nemesis/cloudskate-foil", true, 0.7),
			want: [4]float64{0.7, 0.53, 0.35, 0.21},
		},
		{
			name: "a labelled foil whose variation ends in Foil has it twice",
			card: gradedRow("Universes Beyond: Warhammer 40,000", "004 - Surge Foil", "mtg/universes-beyond-warhammer-40000/the-swarmlord-004-surge-foil-foil", true, 40),
			want: [4]float64{40, 30, 20, 12},
		},
		{
			name: "a labelled foil whose variation holds Foil elsewhere",
			card: gradedRow("Universes Beyond: Doctor Who Variants", "0931 - Surge Foil - Extended Art", "mtg/universes-beyond-doctor-who-variants/four-knocks-0931-surge-foil-extended-art-foil", true, 8),
			want: [4]float64{8, 6, 4, 2.4},
		},
		{
			name: "a Judge foil follows the price rows",
			card: gradedRow("Promotional", "Judge Foil", "mtg/promotional/balance-judge-foil", true, 11.5),
			want: [4]float64{11.5, 9.2, 8.05, 5.75},
		},
		{
			name: "a Secret Lair foil follows the price rows",
			card: gradedRow("Secret Lair", "011 - Foil", "mtg/secret-lair/the-ur-dragon-011-foil", true, 45),
			want: [4]float64{45, 38.25, 33.75, 29.25},
		},
		{
			name: "a foil with no Foil in its URL follows the price rows",
			card: gradedRow("From the Vault: Dragons", "", "mtg/from-the-vault-dragons/bladewing-the-risen", true, 20),
			want: [4]float64{20, 17, 14, 10},
		},
		{
			name: "a foil whose URL holds Foil elsewhere follows the price rows",
			card: gradedRow("Commander Legends Variants", "Foil Etched", "mtg/commander-legends-variants/prossh-skyraider-of-kher-foil-etched", true, 6),
			want: [4]float64{6, 4.8, 4.2, 3},
		},
		{
			name: "a labelled Foil Etched is on the foil row",
			card: gradedRow("March of the Machine: The Aftermath Variants", "Foil Etched", "mtg/march-of-the-machine-the-aftermath-variants/metropolis-reformer-foil-etched-foil", true, 0.9),
			want: [4]float64{0.9, 0.68, 0.45, 0.27},
		},
		{
			name: "a nonfoil whose URL ends in foil follows the price rows",
			card: gradedRow("Promotional", "MPS 2007 Non-Foil", "mtg/promotional/plains-mps-2007-non-foil", false, 1),
			want: [4]float64{1, 0.8, 0.7, 0.5},
		},
		{
			name: "under 15 by the buy price",
			card: gradedRow("4th Edition", "", "mtg/4th-edition/strip-mine", false, 9),
			want: [4]float64{9, 7.2, 6.3, 4.5},
		},
		{
			name: "15 opens the second price row",
			card: gradedRow("4th Edition", "", "mtg/4th-edition/birds-of-paradise", false, 15),
			want: [4]float64{15, 12.75, 10.5, 7.5},
		},
		{
			name: "25 to 100 by the buy price",
			card: gradedRow("4th Edition", "", "mtg/4th-edition/mana-vault", false, 65),
			want: [4]float64{65, 55.25, 48.75, 42.25},
		},
		{
			name: "100 opens the top price row",
			card: gradedRow("Alliances", "", "mtg/alliances/lake-of-the-dead", false, 100),
			want: [4]float64{100, 90, 80, 70},
		},
		{
			name: "a cheap card rounds half up",
			card: gradedRow("5th Edition", "", "mtg/5th-edition/force-spike", false, 0.85),
			want: [4]float64{0.85, 0.68, 0.6, 0.43},
		},
		{
			name: "Beta has its own row at any price",
			card: gradedRow("Beta", "", "mtg/beta/animate-dead", false, 275),
			want: [4]float64{275, 220, 165, 110},
		},
		{
			name: "Alpha has its own row at any price",
			card: gradedRow("Alpha", "", "mtg/alpha/animate-dead", false, 100),
			want: [4]float64{100, 80, 60, 40},
		},
		{
			name: "Unlimited has its own row at any price",
			card: gradedRow("Unlimited", "", "mtg/unlimited/counterspell", false, 42.5),
			want: [4]float64{42.5, 34, 25.5, 17},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gradedBuyPrices(tt.card)
			if got != tt.want {
				t.Errorf("gradedBuyPrices() = %v, want %v", got, tt.want)
			}
		})
	}
}

func gradedRow(edition, variation, url string, foil bool, buy float64) cardkingdom.Product {
	return cardkingdom.Product{
		Edition:   edition,
		Variation: variation,
		URL:       url,
		IsFoil:    foil,
		PriceBuy:  buy,
	}
}
