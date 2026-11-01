package mintcard

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/mtgban/go-mtgban/internal/datastore"
	"github.com/mtgban/go-mtgban/matcher"

	_ "github.com/mtgban/go-mtgban/matcher/magic"
)

var (
	datastoreOnce    sync.Once
	datastoreErr     error
	datastoreBackend *matcher.Backend
)

// realDatastore loads the Magic datastore the first time a test asks for it,
// and skips where the run carries none.
func realDatastore(t *testing.T) *matcher.Backend {
	t.Helper()
	datastoreOnce.Do(func() {
		path := os.Getenv("ALLPRINTINGS5_PATH")
		if path == "" {
			return
		}
		b, err := datastore.Read("magic", path)
		if err != nil {
			datastoreErr = err
			return
		}
		datastoreBackend = b
	})
	if datastoreErr != nil {
		t.Fatal(datastoreErr)
	}
	if datastoreBackend == nil {
		t.Skip("Need ALLPRINTINGS5_PATH set to run this test")
	}
	return datastoreBackend
}

// TestPreprocessInserts pins that the inserts a booster carries beside its
// cards - theme cards, the helper cards of a set the datastore files none
// for - and the emblems and signed cards are refused quietly: 364 of the
// 411 lines of the night of 2026-09-06.
func TestPreprocessInserts(t *testing.T) {
	// No datastore lookup is reached on this path, so an empty backend is
	// enough.
	b := &matcher.Backend{}
	for _, name := range []string{
		"Angels Theme Card",
		"Helper Card",
		"Emblem Garruk, Caller of Beasts",
		"Chrome Mox (Manuel Bevand Signature)",
	} {
		_, err := preprocess(b, name, "", "Regular", "English", "Foundations Jumpstart", "J25")
		if !errors.Is(err, matcher.ErrUnsupported) {
			t.Errorf("preprocess(%q) = %v, want ErrUnsupported", name, err)
		}
	}
}

// TestPreprocessShelves pins the storefront's own spellings against the
// datastore's: a duel deck code of its own, a Final Fantasy buy-a-box
// promo listed under its flavor name with the card's own in parentheses,
// the helper card of a set whose substitute cards the datastore files, the
// two cards whose names read like inserts, a Phyrexian printing filed as
// English beside a List card that only names New Phyrexia, and a catch-all
// promo shelf numbering its cards by TCGplayer's position, not the card's.
func TestPreprocessShelves(t *testing.T) {
	b := realDatastore(t)
	for _, tt := range []struct {
		name, edition, code, language string
		wantSet, wantNumber           string
	}{
		{"Coalition Relic", "Duel Decks: Phyrexia vs. Coalition", "PVC", "English", "DDE", "54"},
		{"Fatalism (Arcane Denial) (Final Fantasy Buy-a-Box)", "Promo: Buy-A-Box", "PBAB", "Japanese", "RFIN", "J2"},
		{"Helper Card (9/9)", "Kaldheim", "KHM", "English", "SKHM", "9"},
		{"Signature Slam", "Modern Horizons 3", "MH3", "English", "MH3", "168"},
		{"Emblem of the Warmind", "Future Sight", "FUT", "English", "FUT", "112"},
		{"Plains (267) (Phyrexian)", "Phyrexia: All Will Be One", "ONE", "English", "ONE", "267"},
		{"Beast Within (New Phyrexian)", "Mystery Booster/The List", "MYS", "English", "PLST", "NPH-103"},
		{"Lightning Bolt (0002)", "Promo:\u00a0Unique and Miscellaneous", "PMSC", "English", "PW26", "5"},
		{"La Madre Tierra (Xenagos, God of Revels) (Spanish)", "Secret Lair", "SLD", "English", "SLD", "2061"},
		{"Carnage, Crimson Chaos", "Promo: Standard Showdown", "PSSP", "English", "PW25", "13"},
	} {
		theCard, err := preprocess(b, tt.name, "", "Regular", tt.language, tt.edition, tt.code)
		if err != nil {
			t.Fatalf("preprocess(%q) = %v", tt.name, err)
		}
		cardID, err := b.Match(theCard)
		if err != nil {
			t.Fatalf("Match(%q) = %v", theCard, err)
		}
		co, _ := b.GetUUID(cardID)
		if co.SetCode != tt.wantSet || co.Number != tt.wantNumber {
			t.Errorf("Match(%q) = %s %s, want %s %s", theCard, co.SetCode, co.Number, tt.wantSet, tt.wantNumber)
		}
	}
}

// TestPreprocessCatchAllShelves pins that a promo shelf naming no set, the
// Wizards Play Network and 30th Anniversary ones as well as the
// miscellaneous one, sends a listed card to the set the datastore files it
// in, and that a name the miscellaneous shelf maps stays on it alone.
func TestPreprocessCatchAllShelves(t *testing.T) {
	b := realDatastore(t)
	for _, tt := range []struct {
		name, edition, code string
		wantSet, wantNumber string
	}{
		{"Ruby, Daring Tracker (PWFM 2024 Spring)", "Promo: Unique and Miscellaneous", "PMSC", "PWCS", "2024-02"},
		{"Saheeli, the Sun's Brilliance (JP Exclusive)", "Promo: WPN & Gateway", "PWPN", "PWCS", "2024-05"},
		{"Destroy Evil (30th Anniversary Celebration Tokyo)", "Promo: 30th Anniversary", "P30", "P30T", "1"},
	} {
		theCard, err := preprocess(b, tt.name, "", "Foil", "Japanese", tt.edition, tt.code)
		if err != nil {
			t.Fatalf("preprocess(%q) = %v", tt.name, err)
		}
		cardID, err := b.Match(theCard)
		if err != nil {
			t.Fatalf("Match(%q) = %v", theCard, err)
		}
		co, _ := b.GetUUID(cardID)
		if co.SetCode != tt.wantSet || co.Number != tt.wantNumber {
			t.Errorf("Match(%q) = %s %s, want %s %s", theCard, co.SetCode, co.Number, tt.wantSet, tt.wantNumber)
		}
	}

	theCard, err := preprocess(b, "Serra Angel (30th Anniversary)", "", "Foil", "Japanese", "Promo: 30th Anniversary", "P30")
	if err != nil {
		t.Fatalf("preprocess() = %v", err)
	}
	if theCard.Edition != "Promo: 30th Anniversary" {
		t.Errorf("preprocess() edition = %q, want the shelf's own", theCard.Edition)
	}
}

// TestPreprocessLanguage pins that the feed's blanket Language "English" is
// dropped before it reaches matcher: an Arabic prerelease foil shelved
// under that tag, with no language of its own beside "English" to pass,
// would otherwise demand an English candidate and refuse.
func TestPreprocessLanguage(t *testing.T) {
	b := realDatastore(t)
	theCard, err := preprocess(b, "Stone-Tongue Basilisk (Odyssey Prerelease)(Arabic)", "", "Foil Prerelease", "English", "Odyssey", "ODY")
	if err != nil {
		t.Fatalf("preprocess() = %v", err)
	}
	cardID, err := b.Match(theCard)
	if err != nil {
		t.Fatalf("Match(%q) = %v", theCard, err)
	}
	co, _ := b.GetUUID(cardID)
	if co.SetCode != "PODY" || co.Number != "276" || !co.Foil {
		t.Errorf("Match(%q) = %s %s foil=%v, want PODY 276 foil", theCard, co.SetCode, co.Number, co.Foil)
	}
}

// TestPreprocessSignatureSpellbook pins that a Signature Spellbook card, a
// real set, escapes the insert guard the word "Signature" otherwise trips.
func TestPreprocessSignatureSpellbook(t *testing.T) {
	b := realDatastore(t)
	theCard, err := preprocess(b, "Brainstorm (Signature Spellbook: Jace)", "", "Regular", "English", "Mystery Booster/The List", "MYS")
	if err != nil {
		t.Fatalf("preprocess() = %v", err)
	}
	cardID, err := b.Match(theCard)
	if err != nil {
		t.Fatalf("Match(%q) = %v", theCard, err)
	}
	co, _ := b.GetUUID(cardID)
	if co.SetCode != "PLST" || co.Number != "SS1-3" {
		t.Errorf("Match(%q) = %s %s, want PLST SS1-3", theCard, co.SetCode, co.Number)
	}
}

// TestPreprocessTokenFlavorSwap pins that a token's own parenthetical names
// the set it comes from, not a flavor: "Goblin Soldier Token (Apocalypse)"
// keeps its own name rather than swapping to Apocalypse, also a card.
func TestPreprocessTokenFlavorSwap(t *testing.T) {
	b := realDatastore(t)
	theCard, err := preprocess(b, "Goblin Soldier Token (Apocalypse) (Player Rewards)", "", "Regular", "English", "Promo: Magic Player Rewards", "PMPR")
	if err != nil {
		t.Fatalf("preprocess() = %v", err)
	}
	if theCard.Name != "Goblin Soldier Token" {
		t.Errorf("preprocess() name = %q, want %q", theCard.Name, "Goblin Soldier Token")
	}
	cardID, err := b.Match(theCard)
	if err != nil {
		t.Fatalf("Match(%q) = %v", theCard, err)
	}
	co, _ := b.GetUUID(cardID)
	if co.SetCode != "MPR" || co.Number != "6" {
		t.Errorf("Match(%q) = %s %s, want MPR 6", theCard, co.SetCode, co.Number)
	}
}

// TestProcessEntryAliasing pins how a listing that matches several printings
// is settled: the finish it is sold in picks between a nonfoil promo and a
// foil-only one of the same card, a foil listing both candidates are sold in
// stays refused with its log line well formed, and a basic land stocked
// without a number is skipped quietly.
func TestProcessEntryAliasing(t *testing.T) {
	b := realDatastore(t)
	for _, tt := range []struct {
		name, finish, edition, code string
		wantSet, wantNumber         string
		wantRefused                 bool
	}{
		{"Counterspell (MagicFest)", "Regular", "Promo: MagicFest", "PMF", "PF24", "1", false},
		{"Lightning Bolt (Magicfest)", "Regular", "Promo: MagicFest", "PMF", "PF19", "1", false},
		{"Counterspell (MagicFest)", "Foil", "Promo: MagicFest", "PMF", "", "", true},
		{"Mountain", "Regular", "Avatar: The Last Airbender Eternal", "TLE", "", "", false},
		{"Plains", "Regular", "Avatar: The Last Airbender Eternal", "TLE", "", "", false},
	} {
		mint := NewScraper(b)
		var logged []string
		mint.logCallback = func(format string, a ...any) {
			logged = append(logged, fmt.Sprintf(format, a...))
		}
		card := Card{ID: "1", Name: tt.name, Quantity: 1, Price: "1.00", BuyPrice: "0.50"}
		mint.processEntry(nil, card, "NM", tt.finish, "English", tt.edition, tt.code, "1")

		if tt.wantRefused != (len(logged) > 0) {
			t.Errorf("processEntry(%q, %s) logged %v, want refused = %v", tt.name, tt.finish, logged, tt.wantRefused)
		}
		for _, line := range logged {
			if strings.Contains(line, "%!") {
				t.Errorf("processEntry(%q) logged a malformed line %q", tt.name, line)
			}
		}
		if tt.wantSet == "" {
			if len(mint.inventory)+len(mint.buylist) != 0 {
				t.Errorf("processEntry(%q, %s) priced %v %v, want nothing", tt.name, tt.finish, mint.inventory, mint.buylist)
			}
			continue
		}
		for id := range mint.inventory {
			co, _ := b.GetUUID(id)
			if co.SetCode != tt.wantSet || co.Number != tt.wantNumber {
				t.Errorf("processEntry(%q, %s) = %s %s, want %s %s", tt.name, tt.finish, co.SetCode, co.Number, tt.wantSet, tt.wantNumber)
			}
		}
		if len(mint.inventory) != 1 || len(mint.buylist) != 1 {
			t.Errorf("processEntry(%q, %s) priced %d sell and %d buy ids, want one each", tt.name, tt.finish, len(mint.inventory), len(mint.buylist))
		}
	}
}
