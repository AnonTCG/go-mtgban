package cardgamecorner

import (
	"os"
	"sync"
	"testing"

	"github.com/mtgban/go-mtgban/internal/datastore"
	"github.com/mtgban/go-mtgban/matcher"

	_ "github.com/mtgban/go-mtgban/matcher/yugioh"
)

var (
	yugiohOnce    sync.Once
	yugiohErr     error
	yugiohBackend *matcher.Backend
)

// yugiohDatastore reads the Yu-Gi-Oh datastore the first time a test asks
// for it, and skips where the run carries none.
func yugiohDatastore(t *testing.T) *matcher.Backend {
	t.Helper()
	yugiohOnce.Do(func() {
		path := os.Getenv("YUGIOH_PATH")
		if path == "" {
			return
		}
		yugiohBackend, yugiohErr = datastore.Read(matcher.GameYuGiOh, path)
	})
	if yugiohErr != nil {
		t.Fatal(yugiohErr)
	}
	if yugiohBackend == nil {
		t.Skip("Need YUGIOH_PATH set to run this test")
	}
	return yugiohBackend
}

func TestSKUDigits(t *testing.T) {
	for sku, want := range map[string]string{
		"DB20128":     "128",
		"RA02V3007":   "007",
		"RA03V1051V2": "051",
		"Y-MF01-001":  "001",
		"TKN":         "",
	} {
		got := skuDigits(sku)
		if got != want {
			t.Errorf("skuDigits(%q) = %q, want %q", sku, got, want)
		}
	}
}

// A listing whose number field the store got wrong is placed by the number
// its SKU ends in, and only where the printing carries it.
func TestMatchBySKUNumber(t *testing.T) {
	b := yugiohDatastore(t)

	tests := []struct {
		desc  string
		card  Card
		print string
		want  string
	}{
		{
			"the SKU names the card",
			Card{Name: "Winged Minion", Edition: "Dark Beginning 2", Number: "085", Rarity: "Common", SKU: "DB20128"},
			"Unl.", "db2-en128_24260_unlimited",
		},
		{
			"a version appended to the SKU",
			Card{Name: "Deep Sea Diva (V.6 - Collectors Rare)", Edition: "Quarter Century Bonanza", Number: "007", Rarity: "Collectors Rare", SKU: "RA03V6004"},
			"First Edition", "ra03-en004_592188_1stedition",
		},
		{
			"the number field agrees",
			Card{Name: "Winged Minion", Edition: "Dark Beginning 2", Number: "128", Rarity: "Common", SKU: "DB20128"},
			"Unl.", "",
		},
		{
			"the SKU is wrong too",
			Card{Name: "Graff, Malebranche of the Burning Abyss (V.5 - Quarter Century Secret Rare)", Edition: "Quarter Century Stampede", Number: "235", Rarity: "Quarter Century Secret Rare", SKU: "RA04V5235"},
			"First Edition", "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			tt.card.Variants = []Variant{{Print: tt.print, Foil: "Non Foil", Language: "EN"}}
			id, found := matchBySKUNumber(b, &tt.card, 0)
			if found != (tt.want != "") || id != tt.want {
				t.Errorf("matchBySKUNumber() = %q, %v, want %q", id, found, tt.want)
			}
		})
	}
}
