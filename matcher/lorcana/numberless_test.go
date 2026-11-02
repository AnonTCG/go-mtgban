package lorcana

import (
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestNumberlessCards pins that a card the datastore files with no number
// carries none. The number is a string here because that is the only
// spelling an absence survives: an integer gives a card that prints nothing
// the same 0 that "Bruno Madrigal - Undetected Uncle" really prints, and 173
// puzzle inserts, lore cards and oversized components would land on it,
// one crowd per set.
func TestNumberlessCards(t *testing.T) {
	b := gametest.Backend(t, matcher.GameLorcana)

	var numberless, zero int
	var zeroNames []string
	// By card rather than by printing: a card sold in two finishes wears
	// two uuids and would be counted twice.
	seen := map[string]bool{}
	for _, co := range b.UUIDs {
		card := co.Name + "|" + co.SetCode + "|" + co.Number
		if co.Sealed || seen[card] {
			continue
		}
		seen[card] = true
		switch co.Number {
		case "":
			numberless++
		case "0":
			zero++
			zeroNames = append(zeroNames, co.Name)
		}
	}

	// One card prints a 0 and keeps it. Any crowd beside it is the
	// absence being spelled as a number again.
	if zero > 1 {
		if len(zeroNames) > 5 {
			zeroNames = zeroNames[:5]
		}
		t.Errorf("%d cards on number 0, want the one that prints it; first few: %v",
			zero, zeroNames)
	}
	if zero == 1 && !strings.HasPrefix(zeroNames[0], "Bruno Madrigal") {
		t.Errorf("the card numbered 0 is %q, want Bruno Madrigal", zeroNames[0])
	}
	if numberless == 0 {
		t.Error("no card carries an absent number; the inserts print none")
	}
	t.Logf("%d cards carry no number, %d carry a 0", numberless, zero)
}
