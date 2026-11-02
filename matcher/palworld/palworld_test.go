package palworld

import (
	"cmp"
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestFinishIdentity pins the invariant the per-printing entries exist for:
// one uuid per finish, and no uuid answering for two.
func TestFinishIdentity(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePalworld)

	for uuid, co := range b.UUIDs {
		if co.Sealed {
			continue
		}
		if co.Finish == "" {
			t.Errorf("%s carries no finish", uuid)
			continue
		}
		if got := co.FoilUUIDs[co.Finish]; got != uuid {
			t.Errorf("%s is stored under finish %q, which resolves to %q", uuid, co.Finish, got)
		}
	}
}

// TestPlainNumberIsPlain pins the contract every game keeps: PlainNumber
// is Number with the game's decorations stripped, never anything wider. The
// rarity code this game's numbers end in is the decoration - EBP01-001OSR
// is the Jormuntide Ignis that EBP01-001 is - so the plain number is the
// run's number, the code off, and the number itself where it wears none.
func TestPlainNumberIsPlain(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePalworld)

	for uuid, co := range b.UUIDs {
		if co.Sealed || co.Number == "" {
			continue
		}
		want := co.Number
		if run, tail := splitNumber(co.Number); tail != "" {
			want = run
		}
		if co.PlainNumber != want {
			t.Errorf("%s: PlainNumber %q, want %q for Number %q", uuid, co.PlainNumber, want, co.Number)
		}
	}
}

// TestNumbersAreUnique pins what identifies a printing in this game. Unlike
// the other Bandai-shaped games, a parallel is numbered apart from the card
// it parallels rather than sharing its number under a different rarity, so
// a number names one product but for a reprint run, which its promo type
// tells apart, and the rules never have to tier.
func TestNumbersAreUnique(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePalworld)

	seen := map[string]string{}
	named := map[string]string{}
	for _, co := range b.UUIDs {
		if co.Sealed || co.Number == "" {
			continue
		}
		key := strings.Join(append([]string{co.SetCode, co.Number}, co.PromoTypes...), "|")
		words := strings.Join(append([]string{co.Name, co.Rarity}, co.PromoTypes...), "|")
		// The finishes of one product fold onto the product: the uuid of
		// its plain printing where it has one, the foil's otherwise.
		printing := co.FoilUUIDs[matcher.FinishNonfoil]
		if printing == "" {
			printing = co.FoilUUIDs[matcher.FinishFoil]
		}
		// Alike in every word a listing names is a card TCGplayer lists
		// twice, which the datastore publishes; anything else is two cards.
		if other, found := seen[key]; found && other != printing && named[key] != words {
			t.Errorf("%s is carried by both %s and %s", key, other, printing)
		}
		seen[key] = printing
		named[key] = words
	}
}

// TestRarityTailSplit pins the reading of a number's tail, which is the
// rarity's code and the whole of what tells a parallel from the card it
// parallels.
func TestRarityTailSplit(t *testing.T) {
	for _, tt := range []struct{ number, run, tail string }{
		{"ETD01-001", "ETD01-001", ""},
		{"ETD01-001TSR", "ETD01-001", "TSR"},
		{"EBP01-002SP", "EBP01-002", "SP"},
		{"EBP01-001SSP", "EBP01-001", "SSP"},
		{"EPR-008-S", "EPR-008", "S"},
		{"EBP02-003-SSP", "EBP02-003", "SSP"},
		{"EPR-004", "EPR-004", ""},
		{"", "", ""},
	} {
		run, tail := splitNumber(tt.number)
		if run != tt.run || tail != tt.tail {
			t.Errorf("splitNumber(%q) = %q, %q, want %q, %q", tt.number, run, tail, tt.run, tt.tail)
		}
	}
}

// TestTailReachesItsParallel pins that a listing naming the rarity code
// reaches the parallel, and that one naming none reaches the plain card.
// TCGplayer names its own products this way - "Grizzbolt - Rumbling Tank
// (TSR)" is ETD01-001TSR - so the code is what usually arrives.
func TestTailReachesItsParallel(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePalworld)

	for _, tt := range []struct {
		desc       string
		in         matcher.InputCard
		wantNumber string
	}{
		{
			desc:       "the number as written decides",
			in:         matcher.InputCard{Name: "Grizzbolt - Rumbling Tank", Variation: "ETD01-001TSR"},
			wantNumber: "ETD01-001TSR",
		},
		{
			desc:       "the run's number with the code beside it reaches the parallel",
			in:         matcher.InputCard{Name: "Grizzbolt - Rumbling Tank", Variation: "ETD01-001 TSR"},
			wantNumber: "ETD01-001TSR",
		},
		{
			desc:       "and the code spelled out reaches it too",
			in:         matcher.InputCard{Name: "Grizzbolt - Rumbling Tank", Variation: "ETD01-001 Trial Deck Super Rare"},
			wantNumber: "ETD01-001TSR",
		},
		{
			desc:       "a bare number reaches the plain card",
			in:         matcher.InputCard{Name: "Grizzbolt - Rumbling Tank", Variation: "ETD01-001"},
			wantNumber: "ETD01-001",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			in := tt.in
			id, err := b.Match(&in)
			if err != nil {
				t.Fatalf("Match(%v) = %v", tt.in, err)
			}
			co, err := b.GetUUID(id)
			if err != nil {
				t.Fatalf("GetUUID(%s) = %v", id, err)
			}
			if co.Number != tt.wantNumber {
				t.Errorf("Match(%v) = %s (%s), want number %s", tt.in, co.UUID, co.Number, tt.wantNumber)
			}
		})
	}
}

// doubleListed is every name, set and number two different products answer
// to: a card TCGplayer lists twice, which the datastore publishes rather than
// refuse the whole game over. TestNumbersAreUnique still refuses two cards
// that differ at one number.
func doubleListed(b *matcher.Backend) map[string]bool {
	products := map[string]map[string]bool{}
	for uuid, co := range b.UUIDs {
		if co.Sealed || co.Number == "" {
			continue
		}
		// A product's finishes fold onto one printing, as their shared map names it.
		product := cmp.Or(co.FoilUUIDs[matcher.FinishNonfoil], co.FoilUUIDs[matcher.FinishFoil], uuid)
		words := co.Name + "|" + co.SetCode + "|" + co.Number
		if products[words] == nil {
			products[words] = map[string]bool{}
		}
		products[words][product] = true
	}
	twice := map[string]bool{}
	for words, of := range products {
		if len(of) > 1 {
			twice[words] = true
		}
	}
	return twice
}

// TestNumberReachesItsPrinting replays every printing under its own name,
// edition and collector number. Anything that fails to come back is a
// printing no storefront writing the catalog's own words could reach.
func TestNumberReachesItsPrinting(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePalworld)

	var probes, hits int
	misses := map[string]int{}
	listedTwice := doubleListed(b)
	for _, co := range b.UUIDs {
		if co.Sealed || co.Number == "" {
			continue
		}
		// Two products alike in every word probed are a card TCGplayer
		// lists twice; no wording can name one of them, so neither is asked.
		if listedTwice[co.Name+"|"+co.SetCode+"|"+co.Number] {
			continue
		}
		set := b.Sets[co.SetCode]
		if set == nil {
			continue
		}
		in := matcher.InputCard{Name: co.Name, Edition: set.Name, Variation: co.Number}
		probes++
		id, err := b.Match(&in)
		if err != nil {
			misses[err.Error()]++
			continue
		}
		got, err := b.GetUUID(id)
		if err != nil {
			misses["GetUUID: "+err.Error()]++
			continue
		}
		if got.SetCode == co.SetCode && got.Number == co.Number {
			hits++
			continue
		}
		misses["wrong printing"]++
	}
	if probes == 0 {
		t.Fatal("no printing to probe")
	}
	t.Logf("%d of %d printings answered (%.1f%%)", hits, probes, 100*float64(hits)/float64(probes))
	for reason, n := range misses {
		t.Logf("   %5d %s", n, reason)
	}
	if hits != probes {
		t.Errorf("%d printings did not answer to their own number", probes-hits)
	}
}

// TestSetCodeIsNotTheNumberPrefix pins the mismatch this game carries: the
// card list numbers a card with an "E" the set code does not have, so BP01's
// cards are numbered EBP01-nnn. Nothing may read a set out of a number's
// prefix here. The one agreement is a set TCGplayer itself codes with the E
// (ESS01, the Sleeve and Card Set).
func TestSetCodeIsNotTheNumberPrefix(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePalworld)

	var agree, disagree int
	for _, co := range b.UUIDs {
		// The one set TCGplayer codes with the E.
		if co.Sealed || co.Number == "" || co.SetCode == "ESS01" {
			continue
		}
		run, _ := splitNumber(co.Number)
		prefix, _, _ := cutPrefix(run)
		if prefix == co.SetCode {
			agree++
		} else {
			disagree++
		}
	}
	if agree > 0 {
		t.Errorf("%d printings have a number prefix matching their set code, which this game does not do", agree)
	}
	if disagree == 0 {
		t.Error("no printing to check")
	}
}

// cutPrefix takes the letters and digits before a collector number's dash.
func cutPrefix(number string) (string, string, bool) {
	for i := 0; i < len(number); i++ {
		if number[i] == '-' {
			return number[:i], number[i+1:], true
		}
	}
	return number, "", false
}
