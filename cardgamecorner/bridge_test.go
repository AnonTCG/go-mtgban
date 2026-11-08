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

// A Yu-Gi-Oh number field states a printing's code by its end or by its
// digits, and a numberless One Piece DON!! is stated by its name alone.
func TestStatesNumber(t *testing.T) {
	tests := []struct {
		desc   string
		game   matcher.Game
		name   string
		listed string
		number string
		want   bool
	}{
		{"the end of the code", matcher.GameYuGiOh, "", "b00", "YGLD-ENB00", true},
		{"the same digits", matcher.GameYuGiOh, "", "X003", "STAX-EN003", true},
		{"another card", matcher.GameYuGiOh, "", "X004", "STAX-EN003", false},
		{"no number listed", matcher.GameYuGiOh, "", "", "STAX-EN003", false},
		{"digits that only end the code", matcher.GameYuGiOh, "", "1", "LOB-EN011", false},
		{"zeros ending the code", matcher.GameYuGiOh, "", "000", "ABPF-EN000", true},
		{"a DON!! has no number", matcher.GameOnePiece, "Don!! (PRB Rebecca) (V.3)", "", "", true},
		{"a card without a number is not a DON!!", matcher.GameOnePiece, "Sabo (OP01-001)", "", "", false},
		{"a DON!! with a number is read as a card", matcher.GameOnePiece, "DON!! (OP09)", "", "OP09-001", false},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cgc := &Cardgamecorner{backend: &matcher.Backend{Game: tt.game}}
			got := cgc.statesNumber(&Card{Name: tt.name, Number: tt.listed}, tt.number)
			if got != tt.want {
				t.Errorf("statesNumber() = %v, want %v", got, tt.want)
			}
		})
	}
}

// A Yu-Gi-Oh image refines the wording's answer within its set and rarity, and
// places what the wording refuses, for the card and print run listed. An EN
// copy on an old shelf keeps its wording.
func TestMatchYuGiOh(t *testing.T) {
	b := yugiohDatastore(t)
	cgc := &Cardgamecorner{backend: b, bridge: map[int]int{881339: 687251, 797782: 593867, 109543: 58482, 102098: 21771, 573505: 245779}}

	tests := []struct {
		desc  string
		card  Card
		print string
		want  string
	}{
		{
			"the image names the stamped version",
			Card{Name: "Raigeki (V.2 - Ultra Rare)V1", Edition: "Rarity Collection 5", Rarity: "Ultra Rare", Number: "110", SKU: "RA05V2110V1", Extra: "/prodotti/singles/ygo/ra05/raigeki-v2-ultra-rare-v2_881339.jpg"},
			"First Edition", "ra05-en110_687251_1stedition",
		},
		{
			"the image places what the wording refuses",
			Card{Name: "Polymerization (V.3 - Secret Rare)", Edition: "Quarter Century Bonanza", Rarity: "Secret Rare", Number: "051", SKU: "RA03V3051V1", Extra: "/prodotti/singles/ygo/ra03/polymerization-v3-secret-rare_797782.jpg"},
			"First Edition", "ra03-en051_593867_1stedition",
		},
		{
			"an image of another rarity",
			Card{Name: "Flamvell Commando (V.1 - Ultra Rare)", Edition: "Ancient Prophecy", Rarity: "Ultra Rare", Number: "086", SKU: "ANPR0086V1", Extra: "/products/ygo/fla/flamvell-commando-v1-ultra-rare/ancient-prophecy-flamvell-commando-v1-ultra-rare-109543.jpg"},
			"Unl.", "anpr-en086_33209_unlimited",
		},
		{
			"an EN copy on an old shelf",
			Card{Name: "Basic Insect", Edition: "Legend of Blue Eyes White Dragon ENG (LOB)", Rarity: "Common", Number: "008", SKU: "LOB0008", Extra: "/products/ygo/bas/basic-insect/legend-of-blue-eyes-white-dragon-basic-insect-102098.jpg"},
			"Unl.", "lob-e006_unlimited",
		},
		{
			"an image of another print run",
			Card{Name: "Protecting Spirit Loagaeth", Edition: "Dawn of Majesty", Rarity: "Super Rare", Number: "025", SKU: "DAMA0025", Extra: "/products/ygo/pro/protecting-spirit-loagaeth/dawn-of-majesty-protecting-spirit-loagaeth-573505.jpg"},
			"Unl.", "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			tt.card.Variants = []Variant{{Print: tt.print, Foil: "Non Foil", Language: "EN"}}
			input, err := preprocess(b, &tt.card, 0)
			if err != nil {
				t.Fatal(err)
			}
			id, err := matchYuGiOh(cgc, &tt.card, 0, input)
			if (err == nil) != (tt.want != "") || id != tt.want {
				t.Errorf("matchYuGiOh() = %q, %v, want %q", id, err, tt.want)
			}
		})
	}
}
