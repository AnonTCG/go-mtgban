package coolstuffinc

import (
	"errors"
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/matcher"

	_ "github.com/mtgban/go-mtgban/matcher/games"
)

// TestPokemonDeckHoloRedirectPrefersSetCode pins a print-run note past the
// plain printing of the same number: Undaunted's Espeon and Umbreon are
// each sold both as the set's own nonfoil and as the theme-deck
// cracked-ice pull a "Shattered Holo from Theme Deck" note names, tied on
// number and edition alone. The probe asks for the catalog's "Cracked Ice
// Holo" label so the cracked-ice twin wins the match over the plain one;
// the redirect is then accepted once that match lands on the marker's own
// set code, not because the landing itself is checked for the label.
func TestPokemonDeckHoloRedirectPrefersSetCode(t *testing.T) {
	b := readGameDatastore(t, "pokemon", "POKEMON_PATH")
	for _, tt := range []struct {
		row    CSIPriceEntry
		wantID string
	}{
		{CSIPriceEntry{Name: "Espeon - 2/90", ItemSet: "HS Undaunted", Notes: "Shattered Holo from Theme Deck", Number: "2/90", RarityName: "Rare Holo"}, "002-090_125042_holofoil"},
		{CSIPriceEntry{Name: "Umbreon - 10/90", ItemSet: "HS Undaunted", Notes: "Shattered Holo from Theme Deck", Number: "10/90", RarityName: "Rare Holo"}, "010-090_125044_holofoil"},
		{CSIPriceEntry{Name: "Charizard - 1/99 - Shattered Holo", ItemSet: "Platinum Arceus", Notes: "*** Shattered Holo Promo (From Collector Tin) ***", Number: "1", RarityName: "Promo"}, "001-099_153234_holofoil"},
		// A number the redirect's own set prints only once is unaffected
		// by the label: there is no plain twin to prefer over it.
		{CSIPriceEntry{Name: "Charizard - 14/181", ItemSet: "SM Team Up", Notes: "Shattered Holo Theme Deck Version", Number: "14/181", RarityName: "Rare Holo"}, "014-181_184217_holofoil"},
		// The 30th Celebration Battle Deck copy is its own reverse holo
		// product in Deck Exclusives, not the set's reverse holo.
		{CSIPriceEntry{Name: "Night Stretcher - 196/217 (30th Reverse Foil)", ItemSet: "ME Ascended Heroes", Notes: "30th Celebration Battle Deck Reverse Foil", Number: "196/217", RarityName: "Uncommon"}, "196-217_729932_reverseholofoil"},
	} {
		t.Run(tt.row.Name, func(t *testing.T) {
			card, run := pokemonBuylistCard(b, tt.row)
			var id string
			var err error
			if run != nil {
				id, err = matchRun(b, card, run)
			} else {
				id, err = b.Match(card)
			}
			if err != nil {
				t.Fatalf("match(%q) = %v", card, err)
			}
			if id != tt.wantID {
				co, _ := b.GetUUID(id)
				t.Errorf("match(%q) = %q (%s/%s/%s), want %q", card, id, co.SetCode, co.Number, co.Finish, tt.wantID)
			}
		})
	}
}

// TestPokemonDeckHoloRedirectAsksForThePlainCopy pins that a "(Non-Holo)"
// listing is redirected for the plain copy and never for the cracked-ice
// one. The bracket sits in the number's own tail here (CSI's "Chandelure -
// 16/116 (NON-HOLO)" shape), not the name's head, and Theme Deck's own note
// would otherwise force Espeon and Umbreon (where a plain nonfoil and a
// cracked-ice twin both exist) onto the wrong one.
func TestPokemonDeckHoloRedirectAsksForThePlainCopy(t *testing.T) {
	b := readGameDatastore(t, "pokemon", "POKEMON_PATH")
	for _, tt := range []struct {
		row    CSIPriceEntry
		wantID string
	}{
		{CSIPriceEntry{Name: "Team Magma's Groudon - 9/95 (Non-Holo)", ItemSet: "Ex Team Magma vs. Team Aqua", Notes: "Non-Holo Theme Deck Version", Number: "9", RarityName: "Rare"}, "009-095_125255"},
		{CSIPriceEntry{Name: "Espeon - 2/90 (Non-Holo)", ItemSet: "HS Undaunted", Notes: "Non-Holo Version from Theme Deck", Number: "2", RarityName: "Rare"}, "002-090_125043"},
		{CSIPriceEntry{Name: "Umbreon - 10/90 (Non-Holo)", ItemSet: "HS Undaunted", Notes: "Non-Holo Version from Theme Deck", Number: "10", RarityName: "Rare"}, "010-090_125045"},
	} {
		t.Run(tt.row.Name, func(t *testing.T) {
			card, run := pokemonBuylistCard(b, tt.row)
			var id string
			var err error
			if run != nil {
				id, err = matchRun(b, card, run)
			} else {
				id, err = b.Match(card)
			}
			if err != nil {
				t.Fatalf("match(%q) = %v", card, err)
			}
			co, _ := b.GetUUID(id)
			if id != tt.wantID {
				t.Errorf("match(%q) = %q (%s/%s/%s), want %q", card, id, co.SetCode, co.Number, co.Finish, tt.wantID)
			}
			if co.Foil || co.Etched {
				t.Errorf("a (Non-Holo) listing landed on a foil printing: %q", co)
			}
		})
	}
}

// TestPokemonDeckHoloRedirectNeedsTheListingsNumber pins that a print-run
// note without a collector number redirects nothing: a probe asked for a
// bare name would land on any printing of the card the shelf holds.
func TestPokemonDeckHoloRedirectNeedsTheListingsNumber(t *testing.T) {
	b := readGameDatastore(t, "pokemon", "POKEMON_PATH")
	for _, name := range []string{"Charizard", "Espeon"} {
		redirected, _ := pokemonDeckHoloRedirect(b, name, "", "Shattered Holo from Theme Deck", "Cracked Ice Holo", true)
		if redirected != "" {
			t.Errorf("%s without a number redirected to %q", name, redirected)
		}
		redirected, _ = pokemonDeckHoloRedirect(b, name, "Shattered Holo", "", "Cracked Ice Holo", true)
		if redirected != "" {
			t.Errorf("%s with only the marker redirected to %q", name, redirected)
		}
	}
}

// TestPokemonUnlisted30thSkipsOffTheDeckSet pins the skip of a 30th
// Celebration Battle Deck copy that lands outside Deck Exclusives, on the
// buylist and in retail alike, and that a listing named by its product id
// is exempt from it.
func TestPokemonUnlisted30thSkipsOffTheDeckSet(t *testing.T) {
	b := readGameDatastore(t, "pokemon", "POKEMON_PATH")
	csi := &Coolstuffinc{backend: b}
	toxtricity := CSIPriceEntry{Name: "Toxtricity - 068/094 (30th Reverse Foil)", ItemSet: "ME Phantasmal Flames", Notes: "30th Celebration Battle Deck Reverse Foil", Number: "068/094", RarityName: "Holo Rare"}
	_, id, err := buylistPokemon(csi, toxtricity)
	if !errors.Is(err, matcher.ErrUnsupported) {
		t.Errorf("buylist %q = %q, %v, want ErrUnsupported", toxtricity.Name, id, err)
	}
	_, id, err = retailPokemon(csi, sellListing{name: toxtricity.Name, edition: toxtricity.ItemSet, notes: toxtricity.Notes, rarity: toxtricity.RarityName})
	if !errors.Is(err, matcher.ErrUnsupported) {
		t.Errorf("retail %q = %q, %v, want ErrUnsupported", toxtricity.Name, id, err)
	}

	energy := sellListing{name: "Fire Energy - MEE002 (30th Reverse Foil)", edition: "ME Ascended Heroes", notes: "30th Celebration Battle Deck Reverse Foil", rarity: "Fixed"}
	_, id, err = retailPokemon(csi, energy)
	if err != nil {
		t.Fatalf("retail %q = %v, want product 729817", energy.name, err)
	}
	co, err := b.GetUUID(id)
	if err != nil || co.SetCode == pokemonDeckSet || !strings.Contains(id, "_729817") {
		t.Errorf("retail %q = %q, %v, want product 729817 outside %s", energy.name, id, err, pokemonDeckSet)
	}
}
