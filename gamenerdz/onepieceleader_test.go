package gamenerdz

import (
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/matcher"

	_ "github.com/mtgban/go-mtgban/matcher/onepiece"
)

// codelessLeaderDatastore holds the published rows of two promo leaders the
// catalog gives no collector number and of the trophy card it gives none
// either. NUMBER and STEM spell the leaders in the datastore's two shapes:
// filed under "LEADER" (leader_552137), or with no number on the bare
// product id (552137).
const codelessLeaderDatastore = `{"data": {
 "game": "onepiece",
 "sets": {
  "OP-PR": {"name": "One Piece Promotion Cards", "releaseDate": "2022-09-30", "type": "promo"},
  "OP13-ANN": {"name": "Carrying On His Will: 3rd Anniversary Tournament Cards", "releaseDate": "2025-10-31"}
 },
 "cards": [
  {"color": "Blue;Green;Purple;Red;Black;Yellow", "colors": ["Blue", "Green", "Purple", "Red", "Black", "Yellow"], "externalLinks": {"tcgPlayerId": 552137}, "finish": "Normal", "id": "STEM552137", NUMBER"name": "Monkey.D.Luffy", "originalReleaseDate": "2024-01-01", "promoTypes": ["sealedbattle"], "rarity": "Leader", "setCode": "OP-PR", "type": "Leader", "variant": "Sealed Battle 2024 Vol. 2", "watermark": "vol. 2"},
  {"color": "Blue;Green;Purple;Red;Black;Yellow", "colors": ["Blue", "Green", "Purple", "Red", "Black", "Yellow"], "externalLinks": {"tcgPlayerId": 552137}, "finish": "Foil", "id": "STEM552137_foil", NUMBER"name": "Monkey.D.Luffy", "originalReleaseDate": "2024-01-01", "promoTypes": ["sealedbattle"], "rarity": "Leader", "setCode": "OP-PR", "type": "Leader", "variant": "Sealed Battle 2024 Vol. 2", "watermark": "vol. 2"},
  {"color": "Blue;Green;Purple;Red;Black;Yellow", "colors": ["Blue", "Green", "Purple", "Red", "Black", "Yellow"], "externalLinks": {"tcgPlayerId": 661879}, "finish": "Normal", "id": "STEM661879", NUMBER"name": "Monkey.D.Luffy", "promoTypes": ["anniversarytournament", "3brotherspack"], "rarity": "Leader", "setCode": "OP13-ANN", "type": "Leader", "variant": "3rd Anniversary Tournament 3 Brothers Pack", "watermark": "3rd"},
  {"externalLinks": {"tcgPlayerId": 719668}, "finish": "Foil", "id": "719668_foil", "name": "Flame-Flame Fruit Trophy Card", "promoTypes": ["flameflamefruitcoliseum"], "rarity": "None", "setCode": "OP-PR", "variant": "Flame-Flame Fruit Coliseum"}
 ]
}}`

// TestCodelessLeader pins that a listing with no card code is kept where it
// lands on a promo leader with no collector number, in either shape the
// datastore has filed them, and refused where it lands on any other card
// with none.
func TestCodelessLeader(t *testing.T) {
	for _, shape := range []struct{ desc, number, stem string }{
		{"filed under LEADER", `"number": "LEADER", `, "leader_"},
		{"filed with no number", "", ""},
	} {
		raw := strings.NewReplacer("NUMBER", shape.number, "STEM", shape.stem).Replace(codelessLeaderDatastore)
		b, err := matcher.Open("onepiece", strings.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		gn, err := NewScraper(b)
		if err != nil {
			t.Fatal(err)
		}
		for _, tt := range []struct {
			name, shelf, finish, want string
		}{
			{"Monkey.D.Luffy (Sealed Battle 2024 Vol. 2)", "One Piece Promotion Cards", "Normal", shape.stem + "552137"},
			{"Monkey.D.Luffy (3rd Anniversary Tournament 3 Brothers Pack)", "Carrying On His Will: 3rd Anniversary Tournament Cards", "Normal", shape.stem + "661879"},
			{"Flame-Flame Fruit Trophy Card (Flame-Flame Fruit Coliseum)", "One Piece Promotion Cards", "Foil", ""},
		} {
			product := GNProduct{
				DisplayName:    tt.name,
				SelectedFinish: tt.finish,
				ProductData:    GNProductData{SetName: tt.shelf},
			}
			got, err := resolveOnePiece(gn, "", product, 0)
			if err != nil || got != tt.want {
				t.Errorf("%s: resolveOnePiece(%q) = (%q, %v), want %q", shape.desc, tt.name, got, err, tt.want)
			}
		}
	}
}
