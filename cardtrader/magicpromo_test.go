package cardtrader

import (
	"errors"
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
)

// TestPreprocessPromoWordingVeto pins the shelf/version wording guard in the
// id path against a shelf wording that disagrees with the id's real promo.
func TestPreprocessPromoWordingVeto(t *testing.T) {
	b := realDatastore(t)

	t.Run("wording is overridden when the id names a real promo", func(t *testing.T) {
		// Quicksmith Rebel: Prerelease shelf, but its id names the
		// datestamped release promo.
		bp := &Blueprint{
			ID:          5731,
			Name:        "Quicksmith Rebel",
			CategoryID:  CategoryMagicSingles,
			ScryfallID:  "54b8a03a-de93-43ed-b90d-7d168a21801a",
			TCGplayerID: 125842,
		}
		bp.Expansion.Name = "Aether Revolt Prerelease"
		bp.Properties.Number = "093"

		want := b.ConvertID(matcher.IDSpaceScryfall, bp.ScryfallID)
		if want == "" {
			t.Fatal("datastore carries no scryfall id 54b8a03a-de93-43ed-b90d-7d168a21801a")
		}

		in, err := Preprocess(b, bp)
		if err != nil {
			t.Fatal(err)
		}
		if in.ID != want {
			t.Fatalf("ID = %q, want %q", in.ID, want)
		}
		if in.Edition != "Aether Revolt Promos" {
			t.Errorf("Edition = %q, want the release promo's own edition", in.Edition)
		}
		if in.Variation != "" {
			t.Errorf("Variation = %q, want the Prerelease wording dropped", in.Variation)
		}

		id, err := b.Match(in)
		if err != nil {
			t.Fatalf("Match refused a correctly id-resolved card: %v", err)
		}
		if id != want {
			t.Errorf("Match landed on %q, want %q", id, want)
		}
	})

	t.Run("wording is overridden when both ids name the same plain card", func(t *testing.T) {
		// Domri's Nodorog: Prerelease shelf, but its scryfall and tcgplayer
		// ids both name the plain RNA printing - no prerelease copy exists.
		bp := &Blueprint{
			ID:          48983,
			Name:        "Domri's Nodorog",
			CategoryID:  CategoryMagicSingles,
			ScryfallID:  "1abe58d8-67d1-4719-8e84-27747dea3506",
			TCGplayerID: 183302,
		}
		bp.Expansion.Name = "Ravnica Allegiance Prerelease"
		bp.Properties.Number = "272"

		in, err := Preprocess(b, bp)
		if err != nil {
			t.Fatal(err)
		}
		if in.Edition != "Ravnica Allegiance" {
			t.Errorf("Edition = %q, want the shelf wording replaced", in.Edition)
		}

		id, err := b.Match(in)
		if err != nil {
			t.Fatalf("Match refused a correctly id-resolved card: %v", err)
		}
		if id != in.ID {
			t.Errorf("Match landed on %q, want %q", id, in.ID)
		}
		co, err := b.GetUUID(id)
		if err != nil {
			t.Fatal(err)
		}
		if co.SetCode != "RNA" || co.Number != "272" {
			t.Errorf("landed on %s %s, want RNA 272", co.SetCode, co.Number)
		}

		// One id alone naming the plain card is not enough to lift the veto.
		bp.TCGplayerID = 0
		in, err = Preprocess(b, bp)
		if err != nil {
			t.Fatal(err)
		}
		_, err = b.Match(in)
		if !errors.Is(err, matcher.ErrUnsupported) {
			t.Errorf("Match with the scryfall id alone = %v, want ErrUnsupported", err)
		}

		// Nor are two ids naming the plain card of one that has a prerelease
		// copy (Hydroid Krasis, PRNA 183s): the shelf sells that copy.
		krasis := &Blueprint{
			Name:        "Hydroid Krasis",
			CategoryID:  CategoryMagicSingles,
			ScryfallID:  "801dd9c6-b159-4e1c-af2c-214c1f573633",
			TCGplayerID: 182840,
		}
		krasis.Expansion.Name = "Ravnica Allegiance Prerelease"
		krasis.Properties.Number = "183"
		in, err = Preprocess(b, krasis)
		if err != nil {
			t.Fatal(err)
		}
		_, err = b.Match(in)
		if !errors.Is(err, matcher.ErrUnsupported) {
			t.Errorf("Match with both ids on a card with a prerelease copy = %v, want ErrUnsupported", err)
		}
	})
}

// TestPlstNumber pins the bare-number-to-PLST-number lookup The List and the
// Secret Lair Commander Deck shelves both need: Card Trader gives the bare
// number a card was originally printed at, but PLST spells its own number
// "<SET>-<n>".
func TestPlstNumber(t *testing.T) {
	b := realDatastore(t)

	tests := []struct {
		desc     string
		cardName string
		number   string
		want     string
	}{
		{"a lone match answers", "Congregate", "4", "DMR-4"},
		{"a lone match answers, three-digit number", "Nature's Lore", "904", "CMM-904"},
		{"two printings share the same trailing number, refuse rather than guess", "Laboratory Maniac", "61", ""},
		{"no printing carries this number", "Congregate", "999", ""},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := plstNumber(b, tt.cardName, tt.number)
			if got != tt.want {
				t.Errorf("plstNumber(%q, %q) = %q, want %q", tt.cardName, tt.number, got, tt.want)
			}
		})
	}
}

// TestPreprocessPromoShelves pins the id-less shelf rules end to end, with
// blueprints copied from Card Trader's own catalog.
func TestPreprocessPromoShelves(t *testing.T) {
	b := realDatastore(t)

	tests := []struct {
		desc       string
		id         int
		category   int
		shelf      string
		name       string
		version    string
		number     string
		scryfall   string
		tcgplayer  int
		wantSet    string
		wantNumber string
	}{
		{"a promo shelf resolves by name to its Promo Pack printing", 322895, CategoryMagicSingles,
			"Foundations Promos", "Niv-Mizzet, Visionary", "", "123", "", 0, "PFDN", "123p"},
		{"an id-less card past the base set is trimmed to the base edition", 147610, CategoryMagicSingles,
			"Zendikar Rising Promos", "Sea Gate Restoration // Sea Gate, Reborn", "", "333", "", 0, "ZNR", "333"},
		{"an unresolved vendor id is not guessed at by wording", 245681, CategoryMagicSingles,
			"March of the Machine Promos", "Essence of Orthodoxy", "", "323", "", 492886, "", ""},
		{"a promo-shelf token stays refused", 240405, CategoryMagicTokens,
			"Dominaria United Promos", "Angel", "SEA Exclusive", "002", "", 477481, "", ""},
		{"Store Championships reads the set name off Version", 380880, CategoryMagicSingles,
			"Store Championships", "Sheltered by Ghosts", "Japan Standard Cup", "2026-001", "1977f6e9-6023-4e5d-82a5-b7c7ae555c10", 0, "PJSC", "2026-1"},
		{"a Secret Lair Commander Deck card printed in SLD stays there", 390118, CategoryMagicOversized,
			"Secret Lair Commander Deck: Goblin Storm", "Zada, Hedron Grinder", "Display Commander", "2423", "", 0, "SLD", "2423"},
		{"a Secret Lair Commander Deck reprint lands on its List printing", 405781, CategoryMagicSingles,
			"Secret Lair Commander Deck: Hatsune Miku", "Congregate", "", "004", "", 0, "PLST", "DMR-4"},
		{"the Mystery Booster Commander List shelf reads as The List", 418612, CategoryMagicSingles,
			"The List - Mystery Booster Commander Edition", "Akroma's Will", "", "003", "", 0, "PLST", "CMR-3"},
		{"a Secret Lair Commander Deck reprint with an unresolved id is not guessed by number", 415442, CategoryMagicSingles,
			"Secret Lair Commander Deck: Odds and Ends", "Doom Whisperer", "", "128", "", 999999999, "", ""},
		{"an overridden TCGplayer id outranks a Scryfall id naming another card", 155490, CategoryMagicSingles,
			"Strixhaven: School of Mages Promos", "Nicol Bolas, God-Pharaoh", "", "140", "256ff8d2-0f0d-4717-a58f-ce3be1d6340c", 0, "PHOU", "140p"},
		{"a Welcome Deck 2024 reprint whose id resolves lands on it", 331110, CategoryMagicSingles,
			"Welcome Deck 2024", "Charmed Sleep", "", "040", "9aba5b08-ba40-4c67-bf0d-dedbef0af1cc", 0, "PLST", "ELD-40"},
		{"a Welcome Deck 2024 reprint whose id resolves nowhere is skipped", 331102, CategoryMagicSingles,
			"Welcome Deck 2024", "Serra Angel", "", "025", "b804473a-0dc6-473a-89c2-0b35ef6856de", 0, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			bp := &Blueprint{
				ID:          tt.id,
				Name:        tt.name,
				Version:     tt.version,
				CategoryID:  tt.category,
				ScryfallID:  tt.scryfall,
				TCGplayerID: tt.tcgplayer,
			}
			bp.Expansion.Name = tt.shelf
			bp.Properties.Number = tt.number

			in, err := Preprocess(b, bp)
			if tt.wantSet == "" {
				if err == nil {
					t.Fatalf("Preprocess = %+v, want a refusal", in)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			id, err := b.Match(in)
			if err != nil {
				t.Fatalf("Match(%+v): %v", in, err)
			}
			co, err := b.GetUUID(id)
			if err != nil {
				t.Fatal(err)
			}
			if co.SetCode != tt.wantSet || co.Number != tt.wantNumber {
				t.Errorf("landed on %s %s, want %s %s", co.SetCode, co.Number, tt.wantSet, tt.wantNumber)
			}
		})
	}
}

// TestPreprocessReprintedPrereleaseShelf pins that a reprint slot Card Trader
// files on a prerelease shelf is dropped, while the shelf's real prerelease
// cards still land on their promo set.
func TestPreprocessReprintedPrereleaseShelf(t *testing.T) {
	b := realDatastore(t)

	cursed := &Blueprint{ID: 290480, Name: "Cursed Mirror", CategoryID: CategoryMagicSingles, TCGplayerID: 554278}
	cursed.Expansion.Name = "Modern Horizons 3 Prerelease"
	cursed.Properties.Number = "279"
	_, err := Preprocess(b, cursed)
	if err == nil {
		t.Error("Preprocess kept a prerelease blueprint no prerelease printing exists for")
	}

	devourer := &Blueprint{ID: 290392, Name: "Devourer of Destiny", CategoryID: CategoryMagicSingles}
	devourer.Expansion.Name = "Modern Horizons 3 Prerelease"
	devourer.Properties.Number = "002"
	in, err := Preprocess(b, devourer)
	if err != nil {
		t.Fatal(err)
	}
	in.Foil = true
	id, err := b.Match(in)
	if err != nil {
		t.Fatal(err)
	}
	co, err := b.GetUUID(id)
	if err != nil {
		t.Fatal(err)
	}
	if co.SetCode != "PMH3" || co.Number != "2s" {
		t.Errorf("landed on %s %s, want PMH3 2s", co.SetCode, co.Number)
	}
}
