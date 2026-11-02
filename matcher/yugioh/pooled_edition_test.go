package yugioh

import (
	"errors"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestPooledEdition pins the storefront name that spans two of the catalog's
// sets. Cool Stuff Inc files both Speed Duel starter decks under one
// "Starter Deck: Speed Dueling", so the edition reaches no set and, left alone,
// every printing the card ever had would answer. The two decks share no card,
// so narrowing to the pair is all the name needs to pick one.
func TestPooledEdition(t *testing.T) {
	b := gametest.Backend(t, matcher.GameYuGiOh)

	for _, tt := range []struct {
		desc    string
		in      matcher.InputCard
		wantSet string
		wantNum string
	}{
		{
			desc:    "a card the first deck prints",
			in:      matcher.InputCard{Name: "A Cat of Ill Omen", Edition: "Starter Deck: Speed Dueling", Variation: "Common"},
			wantSet: "SS01", wantNum: "SS01-ENB11",
		},
		{
			desc:    "a card the second one prints",
			in:      matcher.InputCard{Name: "Alligator's Sword", Edition: "Starter Deck: Speed Dueling", Variation: "Common"},
			wantSet: "SS02", wantNum: "SS02-ENB05",
		},
		{
			desc:    "and each deck named outright still reaches itself",
			in:      matcher.InputCard{Name: "A Cat of Ill Omen", Edition: "Speed Duel Decks: Destiny Masters", Variation: "Common"},
			wantSet: "SS01", wantNum: "SS01-ENB11",
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
			if co.SetCode != tt.wantSet || co.Number != tt.wantNum {
				t.Errorf("Match(%v) = %s|%s, want %s|%s", tt.in, co.SetCode, co.Number, tt.wantSet, tt.wantNum)
			}
		})
	}
}

// TestPooledEditionYuyaDeclan pins that the 2-Player Starter Deck Yuya &
// Declan is refused outright rather than folded onto Saber Force and
// Dark Legion. The shelf is a real Europe/Oceania product (Cardmarket's
// YS15, TCGplayer never listed it) with its own two 21-card halves, and the
// datastore this loader builds from - TCGplayer's own catalog - has no row
// for either one; a fold would land every name on whichever of the two
// unrelated decks also prints it, at that deck's own number and
// rarity. See unsupportedEditions.
func TestPooledEditionYuyaDeclan(t *testing.T) {
	b := gametest.Backend(t, matcher.GameYuGiOh)
	for _, name := range []string{
		"Fabled Ashenveil",       // Declan's deck (Dark Legion) prints this
		"The Calculator",         // Yuya's deck (Saber Force) prints this
		"Mystical Space Typhoon", // both decks print this
		"Ancient Dragon",         // neither English deck prints this; the fold
		"Bright Star Dragon",     // would land it on Galactic Overlord instead
	} {
		in := matcher.InputCard{Name: name, Edition: "2-Player Starter Deck Yuya & Declan", Variation: "Common"}
		if id, err := b.Match(&in); err != matcher.ErrUnsupported {
			t.Errorf("Match(%q) = (%q, %v), want %v", name, id, err, matcher.ErrUnsupported)
		}
	}
}

// TestWGRTUnsupported pins that a War of the Giants Reinforcements listing
// is refused rather than landed on Battle Pack 2: War of the Giants Round 2
// at the same number, and that Round 2's own listing still matches.
func TestWGRTUnsupported(t *testing.T) {
	b := gametest.Backend(t, matcher.GameYuGiOh)

	wgrt := matcher.InputCard{Name: "Mystic Box", Edition: "War of the Giants Reinforcements", Variation: "073 Super Rare"}
	id, err := b.Match(&wgrt)
	if !errors.Is(err, matcher.ErrUnsupported) {
		t.Errorf("Match(%v) = (%q, %v), want %v", wgrt, id, err, matcher.ErrUnsupported)
	}

	round2 := matcher.InputCard{Name: "Mystic Box", Edition: "Battle Pack 2: Round 2", Variation: "073 Super Rare"}
	id, err = b.Match(&round2)
	if err != nil {
		t.Fatalf("Match(%v) = %v", round2, err)
	}
	co, err := b.GetUUID(id)
	if err != nil {
		t.Fatalf("GetUUID(%s) = %v", id, err)
	}
	if co.Number != "BPW2-EN073" {
		t.Errorf("Match(%v) = %s, want BPW2-EN073", round2, co.Number)
	}
}

// TestOTSPastRun pins that an OTS Tournament Pack listing numbered past the
// set's English run is skipped as a printing the datastore has no row for,
// where the same number would otherwise coincide with a card of another set
// ("Gladiator Beast Darius" 031 is Premium Gold's PTDN-EN031), and that a
// listing inside the run still matches.
func TestOTSPastRun(t *testing.T) {
	b := gametest.Backend(t, matcher.GameYuGiOh)

	past := matcher.InputCard{Name: "Gladiator Beast Darius", Edition: "OTS Tournament Pack 12", Variation: "031 Common"}
	if id, err := b.Match(&past); err != matcher.ErrUnsupported {
		t.Errorf("Match(%v) = (%q, %v), want %v", past, id, err, matcher.ErrUnsupported)
	}

	inRun := matcher.InputCard{Name: "Tenyi Spirit - Vishuda", Edition: "OTS Tournament Pack 12", Variation: "010 Super Rare"}
	id, err := b.Match(&inRun)
	if err != nil {
		t.Fatalf("Match(%v) = %v", inRun, err)
	}
	co, err := b.GetUUID(id)
	if err != nil {
		t.Fatalf("GetUUID(%s) = %v", id, err)
	}
	if co.Number != "OP12-EN010" {
		t.Errorf("Match(%v) = %s, want OP12-EN010", inRun, co.Number)
	}
}

// TestRespellWorldwideNumber pins that the "Gradius' Option" respelling
// moves a Legacy of Darkness listing onto the North American LOD-060 unless
// its number names the Worldwide English LOD-EN060, the card as written.
func TestRespellWorldwideNumber(t *testing.T) {
	b := gametest.Backend(t, matcher.GameYuGiOh)

	for _, tt := range []struct {
		variation string
		wantNum   string
	}{
		{"", "LOD-060"},
		{"1st Edition", "LOD-060"},
		{"LOD-060", "LOD-060"},
		{"LOD-EN060", "LOD-EN060"},
	} {
		in := matcher.InputCard{Name: "Gradius' Option", Edition: "Legacy of Darkness", Variation: tt.variation}
		id, err := b.Match(&in)
		if err != nil {
			t.Fatalf("Match(%q) = %v", tt.variation, err)
		}
		co, err := b.GetUUID(id)
		if err != nil {
			t.Fatalf("GetUUID(%s) = %v", id, err)
		}
		if co.Number != tt.wantNum {
			t.Errorf("Match(%q) = %s, want %s", tt.variation, co.Number, tt.wantNum)
		}
	}
}
