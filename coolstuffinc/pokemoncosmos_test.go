package coolstuffinc

import (
	"testing"

	_ "github.com/mtgban/go-mtgban/matcher/games"
)

// TestPokemonCosmosHolo pins the collection-box holos sold under a main set:
// the catalog's miscellaneous shelf answers where it holds the printing at the
// listing's number, and the listing keeps its own answer where it does not.
func TestPokemonCosmosHolo(t *testing.T) {
	b := readGameDatastore(t, "pokemon", "POKEMON_PATH")

	for _, tt := range []struct {
		desc, name, edition, notes, rarity, wantID string
	}{
		{"a holo promo with a cosmos holo of its number",
			"Charmeleon (Holo Promo) - 005/165", "SV 151", "", "Promo", "005-165_586829_holofoil"},
		{"a holo promo of any rarity is the collection box printing",
			"Squirtle - 33/214 (Holo Promo)", "SM Unbroken Bonds", "", "Common", "033-214_193275_holofoil"},
		{"a bare holo bracket beside a cosmos holo note",
			"Mimikyu (Holo) - 081/189", "SWSH Darkness Ablaze", "Cosmos Holo", "Holo Rare", "081-189_247469_holofoil"},
		{"a dashed cosmos holo on the blister shelf",
			"Dragonite - 5/20 - Cosmos Holo", "Dragon Vault", "", "Fixed", "005-020_233835_holofoil"},
		{"a bare holo bracket on a holo rare is the holo rare",
			"Munkidori (Holo) - 095/167", "SV Twilight Masquerade", "", "Holo Rare", "095-167_550139_holofoil"},
		{"a bare holo bracket beside a note saying it may be cosmos holo",
			"Mimikyu (Holo) - 081/189", "SWSH Darkness Ablaze", "Can be Regular or Cosmos Holo", "Holo Rare", "081-189_219467"},
		{"a Deck Exclusives promo, holo and cosmos holo alike",
			"Professor's Research (Cosmo Holo) - 060/072", "SWSH Shining Fates", "Professor Juniper, Cosmo Holo Foiling", "Promo", "060-072_276703_holofoil"},
		{"the cosmos step leaves a printing no cosmos shelf holds alone",
			"Pikachu (Holo Promo) - 025/185", "SWSH Vivid Voltage", "", "Promo", "25_161749"},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			card := pokemonCosmosHolo(b, pokemonListing(b, tt.name, tt.edition, tt.notes, false), tt.rarity)
			id, err := b.Match(card)
			if err != nil {
				t.Fatalf("Match(%v) = %v", card, err)
			}
			if id != tt.wantID {
				co, _ := b.GetUUID(id)
				t.Errorf("Match(%v) = %q (%s %s), want %q", card, id, co.SetCode, co.Number, tt.wantID)
			}
		})
	}
}

// TestPokemonHoloShelf pins a holo bracket to the holofoil the catalog files
// on a promo shelf when the listing's own set prints that number plain or in
// reverse holo only, and to the listing's own holofoil where it has one.
func TestPokemonHoloShelf(t *testing.T) {
	b := readGameDatastore(t, "pokemon", "POKEMON_PATH")

	for _, tt := range []struct {
		desc, name, edition, rarity, wantID string
	}{
		{"a deck exclusive sold under its set's reverse holo",
			"Boss's Orders (Holo) - 058/072", "SWSH Shining Fates", "Holo Rare", "058-072_234447_holofoil"},
		{"an alternate art promo sold as its set's plain card",
			"Delinquent (Holo) - 98a/122", "XY BREAKpoint", "Uncommon", "098a-122_177167_holofoil"},
		{"a league promo sold as its set's plain card",
			"Salandit (Holo Promo) - 15/145", "SM Guardians Rising", "Common", "015-145_148431_holofoil"},
		{"the bracket beside a plain copy of the same promo",
			"Professor's Letter (Alt Art Holo) - 146a/162", "XY BREAKthrough", "Promo", "146a-162_177166_holofoil"},
		{"a holo promo the miscellaneous shelf lacks is the deck exclusive",
			"Greavard (Holo Promo) - 100/197", "SV Obsidian Flames", "Promo", "100-197_613974_holofoil"},
		{"its own set's holofoil wins over a league promo at the number",
			"Charizard (Holo) - 100/97", "Ex Dragon", "Ultra Rare", "100-97_84187_holofoil"},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			card := pokemonHoloShelf(b, pokemonCosmosHolo(b, pokemonListing(b, tt.name, tt.edition, "", false), tt.rarity))
			id, err := b.Match(card)
			if err != nil {
				t.Fatalf("Match(%v) = %v", card, err)
			}
			if id != tt.wantID {
				co, _ := b.GetUUID(id)
				t.Errorf("Match(%v) = %q (%s %s), want %q", card, id, co.SetCode, co.Number, tt.wantID)
			}
		})
	}
}
