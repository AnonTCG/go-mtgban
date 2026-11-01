package cardgamecorner

import (
	"os"
	"sync"
	"testing"

	"github.com/mtgban/go-mtgban/internal/datastore"
	"github.com/mtgban/go-mtgban/matcher"

	_ "github.com/mtgban/go-mtgban/matcher/onepiece"
)

var (
	onePieceOnce    sync.Once
	onePieceErr     error
	onePieceBackend *matcher.Backend
)

// onePieceDatastore reads the One Piece datastore the first time a test asks
// for it, and skips where the run carries none.
func onePieceDatastore(t *testing.T) *matcher.Backend {
	t.Helper()
	onePieceOnce.Do(func() {
		path := os.Getenv("ONEPIECE_PATH")
		if path == "" {
			return
		}
		onePieceBackend, onePieceErr = datastore.Read(matcher.GameOnePiece, path)
	})
	if onePieceErr != nil {
		t.Fatal(onePieceErr)
	}
	if onePieceBackend == nil {
		t.Skip("Need ONEPIECE_PATH set to run this test")
	}
	return onePieceBackend
}

// A listing is placed by the Cardmarket product id its image is named after,
// through the id the datastore publishes or the bridge, and only where the
// printing carries the code the listing's own name or number field states.
func TestMatchByProductImage(t *testing.T) {
	b := onePieceDatastore(t)
	cgc := &Cardgamecorner{backend: b, bridge: map[int]int{747537: 527020, 760553: 541619, 747879: 527027}}

	tests := []struct {
		desc   string
		name   string
		image  string
		number string
		want   string
	}{
		{"the bridge names the printing", "Eustass \"Captain\" Kid (OP05-074) (V.3)", "/prodotti/singles/onepiece/op05/kid-op05074-v3_747537.jpg", "", "op05-074_527020_foil"},
		{"a doubled extension", "Eustass \"Captain\" Kid (OP05-074) (V.3)", "/prodotti/11/747537.jpg.png", "", "op05-074_527020_foil"},
		{"the published id needs no bridge", "Card (OP05-060)", "/prodotti/one_piece/814902.png", "", "op05-060_594591_foil"},
		{"a code the printing does not carry", "Eustass \"Captain\" Kid (OP05-075) (V.3)", "/prodotti/one_piece/747537.png", "", ""},
		{"a code written without its dash", "Eustass \"Captain\" Kid (OP05074) (V.3)", "/prodotti/one_piece/747537.png", "", "op05-074_527020_foil"},
		{"a name stating no code", "Eustass \"Captain\" Kid", "/prodotti/one_piece/747537.png", "", ""},
		{"an id no source links", "Eustass \"Captain\" Kid (OP05-074)", "/prodotti/one_piece/999999.png", "", ""},
		{"an image naming no id", "Eustass \"Captain\" Kid (OP05-074)", "/noimage.png", "", ""},
		{"a name cut short, the number field names it", "But I Will Never Doubt a Woman's Tears!!!! (OP06-0", "/prodotti/singles/onepiece/op06/but-i-will-never-doubt-a-womans-tears-op06-0_760553.jpg", "057", "op06-057_541619"},
		{"a wrong code, the full number field names it", "Monkey.D.Luffy (ST01-001) (V.3) (OP05)", "/prodotti/one_piece/747879.png", "ST01-012", "st01-012_527027_foil"},
		{"a number field the printing does not carry", "But I Will Never Doubt a Woman's Tears!!!! (OP06-0", "/prodotti/singles/onepiece/op06/but-i-will-never-doubt-a-womans-tears-op06-0_760553.jpg", "058", ""},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			id, found := cgc.matchByProductImage(&Card{Name: tt.name, Extra: tt.image, Number: tt.number}, &matcher.InputCard{})
			if found != (tt.want != "") || id != tt.want {
				t.Errorf("matchByProductImage() = %q, %v, want %q", id, found, tt.want)
			}
		})
	}
}

// A Pokemon listing the wording refused is placed by its image, where the
// printing's number is the listing's, or the SKU's where the listing has none.
func TestMatchByProductImagePokemon(t *testing.T) {
	b := pokemonDatastore(t)
	cgc := &Cardgamecorner{backend: b, bridge: map[int]int{512480: 226508, 576743: 248731}}

	leon := "/products/pokemon/leo/leon-vix-154/vivid-voltage-leon-viv-154-512480.jpg"
	greninja := "/products/pokemon/gre/greninja/greninja-576743.jpg"
	tests := []struct {
		desc   string
		image  string
		number string
		sku    string
		want   string
	}{
		{"the listing has no number, the SKU ends in it", leon, "", "SWSH40154", "154-185_226508_holofoil"},
		{"the SKU names another card", leon, "", "SWSH40155", ""},
		{"the number names the printing", greninja, "144", "SWSH0144", "swsh144_248731_holofoil"},
		{"the number names another card", greninja, "145", "SWSH0144", ""},
		{"the number is the store's own", greninja, "SWSH144", "SWSH0144", "swsh144_248731_holofoil"},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			card := &Card{Extra: tt.image, Number: tt.number, SKU: tt.sku}
			id, found := cgc.matchByProductImage(card, &matcher.InputCard{Finish: "Holofoil"})
			if found != (tt.want != "") || id != tt.want {
				t.Errorf("matchByProductImage() = %q, %v, want %q", id, found, tt.want)
			}
		})
	}
}
