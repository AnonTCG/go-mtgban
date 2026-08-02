package abugames

import (
	"log"
	"os"
	"testing"

	"github.com/mtgban/go-mtgban/mtgmatcher"
)

func TestMain(m *testing.M) {
	path := os.Getenv("ALLPRINTINGS5_PATH")
	if path == "" {
		log.Fatalln("Need ALLPRINTINGS5_PATH variable set to run tests")
	}
	if err := mtgmatcher.LoadDatastoreFile(path); err != nil {
		log.Fatalln(err)
	}
	os.Exit(m.Run())
}

// TestSecretLairNumberOverStaleCardNumber guards the case where ABU's
// card_number disagrees with the collector number in the title. Secret Lair
// numbers can be >= 1993, which the year-capped ExtractNumber can't see; a stale
// card.Number ("1933") must not clobber the authoritative title number ("7010").
func TestSecretLairNumberOverStaleCardNumber(t *testing.T) {
	tests := []struct {
		name    string
		number  string // ABU card_number
		wantSet string
		wantNum string
	}{
		{"stale card_number", "1933", "SLD", "7010"},
		{"matching card_number", "7010", "SLD", "7010"},
		{"empty card_number", "", "SLD", "7010"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, err := preprocess(&ABUCard{
				DisplayTitle: "Counterspell (Secret Lair 7010) - FOIL",
				Edition:      "Secret Lair Drop",
				Number:       tt.number,
				Language:     []string{"English"},
			})
			if err != nil {
				t.Fatalf("preprocess: %v", err)
			}
			id, err := mtgmatcher.Match(in)
			if err != nil {
				t.Fatalf("match (variation %q): %v", in.Variation, err)
			}
			co, _ := mtgmatcher.GetUUID(id)
			if co.SetCode != tt.wantSet || co.Number != tt.wantNum {
				t.Errorf("got %s #%s, want %s #%s (variation %q)", co.SetCode, co.Number, tt.wantSet, tt.wantNum, in.Variation)
			}
		})
	}
}
