package cardgamecorner

import (
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
)

func TestGameInput(t *testing.T) {
	for _, tt := range []struct {
		game    matcher.Game
		card    Card
		variant Variant
		want    matcher.InputCard
	}{
		{
			game:    matcher.GameFleshAndBlood,
			card:    Card{Name: "Zero to Sixty (Blue) (Normal)", Edition: "Arcane Rising - Unlimited", Number: "ARC028"},
			variant: Variant{Print: "Unl.", Foil: "Non Foil"},
			want:    matcher.InputCard{Name: "Zero to Sixty (Blue)", Edition: "Arcane Rising - Unlimited", Variation: "ARC028", Finish: "Unlimited Edition Normal"},
		},
		{
			game:    matcher.GameFleshAndBlood,
			card:    Card{Name: "Dash, Inventor Extraordinaire // Dash (Normal)", Edition: "Arcane Rising - Unlimited", Number: "001/002"},
			variant: Variant{Print: "Unl.", Foil: "Non Foil"},
			want:    matcher.InputCard{Name: "Dash, Inventor Extraordinaire // Dash", Edition: "Arcane Rising - Unlimited", Finish: "Unlimited Edition Normal"},
		},
		{
			game:    matcher.GameFleshAndBlood,
			card:    Card{Name: "Rhinar, Reckless Rampage // Harmonized Kodachi (No", Edition: "Welcome to Rathe - Unlimited", Number: "078/225"},
			variant: Variant{Print: "Unl.", Foil: "Non Foil"},
			want:    matcher.InputCard{Name: "Rhinar, Reckless Rampage // Harmonized Kodachi", Edition: "Welcome to Rathe - Unlimited", Finish: "Unlimited Edition Normal"},
		},
		{
			game:    matcher.GameFleshAndBlood,
			card:    Card{Name: "Embodiment of Earth // Embodiment of Lightning (", Edition: "Tales of Aria - Unlimited", Number: "109/110"},
			variant: Variant{Print: "Unl.", Foil: "Non Foil"},
			want:    matcher.InputCard{Name: "Embodiment of Earth // Embodiment of Lightning (", Edition: "Tales of Aria - Unlimited", Finish: "Unlimited Edition Normal"},
		},
		{
			game:    matcher.GameFleshAndBlood,
			card:    Card{Name: "Drone of Brutality (Red) (Rainbow Foil)", Edition: "Welcome to Rathe - Unlimited", Number: "WTR164"},
			variant: Variant{Print: "Unl.", Foil: "Non Foil"},
			want:    matcher.InputCard{Name: "Drone of Brutality (Red)", Edition: "Welcome to Rathe - Unlimited", Variation: "WTR164", Foil: true, Finish: "Unlimited Edition Rainbow Foil"},
		},
		{
			game:    matcher.GameYuGiOh,
			card:    Card{Name: "Star Ryzeal", Edition: "Alliance Insight", Number: "022", Rarity: "Secret Rare"},
			variant: Variant{Print: "First Edition", Foil: "Non Foil"},
			want:    matcher.InputCard{Name: "Star Ryzeal", Edition: "Alliance Insight", Variation: "022 Secret Rare", Finish: "1st Edition"},
		},
		{
			game:    matcher.GameOnePiece,
			card:    Card{Name: "White Snake (OP06-059)", Edition: "OP06 - Wings of the Captain: Promo", Number: "059"},
			variant: Variant{Foil: "Non Foil"},
			want:    matcher.InputCard{Name: "White Snake (OP06-059)", Edition: "Promos: Wings of the Captain", Variation: "059"},
		},
		{
			game:    matcher.GameYuGiOh,
			card:    Card{Name: "Wake Up Your Elemental HERO V.8 - Ultra Rare Overframe )", Edition: "Rarity Collection 5", Number: "142", Rarity: "Overframe"},
			variant: Variant{Print: "First Edition", Foil: "Non Foil"},
			want:    matcher.InputCard{Name: "Wake Up Your Elemental HERO (V.8 - Ultra Rare Overframe)", Edition: "Rarity Collection 5", Variation: "142 Overframe", Finish: "1st Edition"},
		},
		{
			game:    matcher.GameYuGiOh,
			card:    Card{Name: "Darklord Desire", Edition: "GX Manga Promos", Number: "5", Rarity: "Ultra Rare"},
			variant: Variant{Print: "Unl.", Foil: "Non Foil"},
			want:    matcher.InputCard{Name: "Darklord Desire", Edition: "GX Manga Promos", Variation: "YG05-EN001 Ultra Rare"},
		},
		{
			game:    matcher.GameYuGiOh,
			card:    Card{Name: "Infernity General", Edition: "5D's Manga Promos", Number: "03-001", Rarity: "Ultra Rare"},
			variant: Variant{Print: "Unl.", Foil: "Non Foil"},
			want:    matcher.InputCard{Name: "Infernity General", Edition: "5D's Manga Promos", Variation: "YF03-EN001 Ultra Rare"},
		},
		{
			game:    matcher.GameYuGiOh,
			card:    Card{Name: "Gorz the Emissary of Darkness", Edition: "R Comic Book Promos", Number: "01-003", Rarity: "Ultra Rare"},
			variant: Variant{Print: "Unl.", Foil: "Non Foil"},
			want:    matcher.InputCard{Name: "Gorz the Emissary of Darkness", Edition: "R Comic Book Promos", Variation: "YR01-EN003 Ultra Rare"},
		},
		{
			game:    matcher.GameRiftbound,
			card:    Card{Name: "Miss Fortune, Buccaneer (V.2 - Showcase)", Edition: "Origins", Number: "193a"},
			variant: Variant{Foil: "Foil"},
			want:    matcher.InputCard{Name: "Miss Fortune, Buccaneer", Edition: "Origins", Variation: "193a", Foil: true},
		},
		{
			game:    matcher.GameYuGiOh,
			card:    Card{Name: "Goblin Biker Big Cabonga (V.1 - Secret Rare)", Edition: "Phantom Nightmare", Number: "047", Rarity: "Secret Rare"},
			variant: Variant{Print: "First Edition", Foil: "Non Foil"},
			want:    matcher.InputCard{Name: "Goblin Biker Big Gabonga (V.1 - Secret Rare)", Edition: "Phantom Nightmare", Variation: "047 Secret Rare", Finish: "1st Edition"},
		},
		{
			game:    matcher.GameYuGiOh,
			card:    Card{Name: "Blue-Eyes White Dragon", Edition: "Legend of Blue Eyes White Dragon ENG (LOB)", Number: "001", Rarity: "Ultra Rare"},
			variant: Variant{Print: "Unl.", Foil: "Non Foil", Language: "EN"},
			want:    matcher.InputCard{Name: "Blue-Eyes White Dragon", Edition: "Legend of Blue Eyes White Dragon ENG (LOB)", Variation: "European Ultra Rare"},
		},
		{
			game:    matcher.GameYuGiOh,
			card:    Card{Name: "Blue-Eyes White Dragon", Edition: "Legend of Blue Eyes White Dragon ENG (LOB)", Number: "001", Rarity: "Ultra Rare"},
			variant: Variant{Print: "Unl.", Foil: "Non Foil", Language: "US"},
			want:    matcher.InputCard{Name: "Blue-Eyes White Dragon", Edition: "Legend of Blue Eyes White Dragon ENG (LOB)", Variation: "001 Ultra Rare"},
		},
		{
			game:    matcher.GameRiftbound,
			card:    Card{Name: "Vi, Piltover ZHorcer (V.1 - Rare)", Edition: "Unleashed", Number: "187"},
			variant: Variant{Foil: "Foil"},
			want:    matcher.InputCard{Name: "Vi, Piltover Enforcer", Edition: "Unleashed", Variation: "187", Foil: true},
		},
		{
			game:    matcher.GamePokemon,
			card:    Card{Name: "Gyarados d Delta Species ", Edition: "EX Holon Phantoms", Number: "8", Rarity: "Rare Holo"},
			variant: Variant{Print: "Unl.", Foil: "Non Foil"},
			want:    matcher.InputCard{Name: "Gyarados", Edition: "EX Holon Phantoms", Variation: "8", Finish: "Holofoil"},
		},
		{
			game:    matcher.GamePokemon,
			card:    Card{Name: "Larvitar d Delta Specie (Hide)", Edition: "EX Dragon Frontiers", Number: "52", Rarity: "Common"},
			variant: Variant{Print: "Unl.", Foil: "Non Foil"},
			want:    matcher.InputCard{Name: "Larvitar (Hide)", Edition: "EX Dragon Frontiers", Variation: "52"},
		},
		{
			game:    matcher.GamePokemon,
			card:    Card{Name: "Nidoran ? Lv.12", OrigName: "Nidoran female Lv.12", Edition: "Mysterious Treasures", Number: "91", Rarity: "Common"},
			variant: Variant{Print: "Unl.", Foil: "Non Foil"},
			want:    matcher.InputCard{Name: "Nidoran F Lv.12", Edition: "Mysterious Treasures", Variation: "91"},
		},
		{
			game:    matcher.GamePokemon,
			card:    Card{Name: "Stone [F] Energy", Edition: "Vivid Voltage", Number: "164", Rarity: "Uncommon"},
			variant: Variant{Print: "Unl.", Foil: "Reverse Holo"},
			want:    matcher.InputCard{Name: "Stone [F] Energy", Edition: "Vivid Voltage", Variation: "164", Finish: "Reverse Holofoil"},
		},
		{
			game:    matcher.GamePokemon,
			card:    Card{Name: "Fairy Energy", Edition: "Team Up", Number: "E002", Rarity: "Common"},
			variant: Variant{Print: "Unl.", Foil: "Reverse Holo"},
			want:    matcher.InputCard{Name: "Fairy Energy", Edition: "Team Up", Finish: "Reverse Holofoil"},
		},
		{
			game:    matcher.GamePokemon,
			card:    Card{Name: "Fire Energy", Edition: "Pokémon Trading Card Game Classic: Charizard & Ho-Oh Deck", Number: "033", Rarity: "Fixed"},
			variant: Variant{Print: "Unl.", Foil: "Non Foil"},
			want:    matcher.InputCard{Name: "Fire Energy", Edition: "Pokémon Trading Card Game Classic: Charizard & Ho-Oh Deck", Variation: "033 CLC"},
		},
		{
			game:    matcher.GamePokemon,
			card:    Card{Name: "Wooloo", Edition: "Sword & Shield: Black Star Promos", Number: "011", Rarity: "Promo"},
			variant: Variant{Print: "Unl.", Foil: "Reverse Holo"},
			want:    matcher.InputCard{Name: "Wooloo", Edition: "Sword & Shield: Black Star Promos", Variation: "SWSH011", Finish: "Reverse Holofoil"},
		},
	} {
		t.Run(tt.card.Name, func(t *testing.T) {
			tt.card.Variants = []Variant{tt.variant}
			got := gameInput(tt.game, &tt.card, 0)
			if *got != tt.want {
				t.Errorf("got %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestGameFinish(t *testing.T) {
	for _, tt := range []struct {
		game    matcher.Game
		rarity  string
		variant Variant
		rainbow bool
		want    string
	}{
		{game: matcher.GameYuGiOh, variant: Variant{Print: "First Edition"}, want: "1st Edition"},
		{game: matcher.GameYuGiOh, variant: Variant{Print: "Unl."}, want: ""},
		{game: matcher.GamePokemon, variant: Variant{Print: "Unl.", Foil: "Reverse Holo"}, want: "Reverse Holofoil"},
		{game: matcher.GamePokemon, rarity: "Rare Holo", variant: Variant{Print: "First Edition"}, want: "1st Edition Holofoil"},
		{game: matcher.GamePokemon, rarity: "Common", variant: Variant{Print: "Unl."}, want: ""},
		{game: matcher.GameFleshAndBlood, variant: Variant{Print: "Unl."}, want: "Unlimited Edition Normal"},
		{game: matcher.GameFleshAndBlood, variant: Variant{Print: "Unl."}, rainbow: true, want: "Unlimited Edition Rainbow Foil"},
		{game: matcher.GameFleshAndBlood, rainbow: true, want: "Rainbow Foil"},
		{game: matcher.GameLorcana, variant: Variant{Print: "Steel", Foil: "Foil"}, want: ""},
	} {
		got := gameFinish(tt.game, &Card{Rarity: tt.rarity}, tt.variant, tt.rainbow)
		if got != tt.want {
			t.Errorf("%s %+v rainbow %v: got %q, want %q", tt.game, tt.variant, tt.rainbow, got, tt.want)
		}
	}
}

func TestOldShelfSet(t *testing.T) {
	for _, tt := range []struct {
		edition string
		want    string
	}{
		{edition: "Legend of Blue Eyes White Dragon ENG (LOB)", want: "LOB"},
		{edition: "Spell Ruler / Magic Ruler ENG (MRL)", want: "MRL"},
		{edition: "Starter Deck: Yugi ENG (SDY)", want: "SDY"},
		{edition: "Legend of Blue Eyes White Dragon ITA (LDD)"},
		{edition: "Alliance Insight"},
	} {
		got, found := oldShelfSet(tt.edition)
		if got != tt.want || found != (tt.want != "") {
			t.Errorf("oldShelfSet(%q) = %q, %t; want %q", tt.edition, got, found, tt.want)
		}
	}
}
