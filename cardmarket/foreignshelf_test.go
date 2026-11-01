package cardmarket

import (
	"errors"
	"testing"

	cm "github.com/mtgban/go-cardmarket"
)

// TestForeignShelf pins which expansion names are a catalog of their own.
// Cardmarket shelves a set's non-English printings beside the English ones
// under the same card names, and the datastore carries only the English, so a
// price from one of those shelves lands on a printing it is not.
func TestForeignShelf(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{"Metal Raiders (Japanese)", true},
		{"Metal Raiders (Korean)", true},
		{"Metal Raiders (PMT)", true},
		{"Spell Ruler (SDM)", true},
		{"Metal Raiders", false},
		{"Metal Raiders (25th Anniversary Edition)", false},
		{"Legend of Blue Eyes White Dragon (25th Anniversary Edition)", false},
		// The tail has to end the name: a set spelling one of those words
		// somewhere else is still the English catalog.
		{"Japanese Collection Tin", false},
		{"", false},
	} {
		if got := foreignShelf(tt.name); got != tt.want {
			t.Errorf("foreignShelf(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestForeignExpansion pins which expansions each game drops before the
// walk: Flesh and Blood by its own five foreign-only programs, Riftbound by
// its Chinese shelf, One Piece and Yu-Gi-Oh by the shared -JP suffix and
// shelf-name test.
func TestForeignExpansion(t *testing.T) {
	for _, tt := range []struct {
		name    string
		gameID  cm.Game
		exp     cm.Expansion
		foreign bool
	}{
		{"FaB Black Label history pack", cm.GameFleshAndBlood, cm.Expansion{Name: "History Pack 2 - Black Label", SetCode: "2HP-BL"}, true},
		{"FaB English set", cm.GameFleshAndBlood, cm.Expansion{Name: "Welcome to Rathe", SetCode: "WTR"}, false},
		{"One Piece Japanese shelf", cm.GameOnePiece, cm.Expansion{Name: "Romance Dawn (Japanese)", SetCode: "OP01"}, true},
		{"One Piece -JP code", cm.GameOnePiece, cm.Expansion{Name: "Romance Dawn", SetCode: "OP01-JP"}, true},
		{"One Piece English set", cm.GameOnePiece, cm.Expansion{Name: "Romance Dawn", SetCode: "OP01"}, false},
		{"Riftbound Project K", cm.GameRiftbound, cm.Expansion{Name: "Project K Promos", SetCode: "PROK"}, true},
		{"Riftbound English set", cm.GameRiftbound, cm.Expansion{Name: "Origins", SetCode: "OGN"}, false},
		{"Pokemon is never filtered", cm.GamePokemon, cm.Expansion{Name: "Base Set (Japanese)", SetCode: "BS"}, false},
	} {
		if got := foreignExpansion(tt.gameID, tt.exp); got != tt.foreign {
			t.Errorf("%s: foreignExpansion(%v, %v) = %v, want %v", tt.name, tt.gameID, tt.exp, got, tt.foreign)
		}
	}
}

// TestNonEnglishProduct pins that a product Cardmarket shelves among the
// English cards, printed in another language alone, is a catalog we do not
// carry. The ids are the catalog's own, so no datastore is read.
func TestNonEnglishProduct(t *testing.T) {
	for _, tt := range []struct {
		name   string
		gameID cm.Game
		id     int
	}{
		{"Riftbound Korean Ahri", cm.GameRiftbound, 907588},
		{"Riftbound Korean Ahri V.4", cm.GameRiftbound, 916878},
		{"Riftbound Chinese Kennen", cm.GameRiftbound, 916943},
		{"Riftbound Chinese Seraphine", cm.GameRiftbound, 917605},
		{"Riftbound Chinese signed Seraphine", cm.GameRiftbound, 917606},
		{"Lorcana Chinese Snow White", cm.GameLorcana, 895480},
		{"Lorcana Chinese Stitch", cm.GameLorcana, 835454},
		{"One Piece Japanese Thousand Sunny DON", cm.GameOnePiece, 901320},
	} {
		r := &resolver{gameID: tt.gameID}
		_, _, _, err := r.resolveProduct(&cm.Product{IDProduct: tt.id})
		if !errors.Is(err, errForeign) {
			t.Errorf("%s: resolveProduct = %v, want %v", tt.name, err, errForeign)
		}
	}
}
