package onepiece

import (
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
)

// labelWordingFixture holds the printings whose labels the catalog writes
// with a place, a mark or a number inside: the P-041 Luffy handed to every
// player of the 2024 championship beside the regional families awarded for
// a place, the P-069 Koala sold in the 25-26 championship's sets, and the
// 3rd Anniversary Tournament pack's leader card beside the numbered card
// packed with it. Every row is the datastore's.
const labelWordingFixture = `{"data": {
	"game": "onepiece",
	"sets": {
		"OP-PR": {"name": "One Piece Promotion Cards", "releaseDate": "2022-12-02"},
		"OP13-ANN": {"name": "Carrying On His Will: 3rd Anniversary Tournament Cards", "releaseDate": "2025-11-21"},
		"OP13": {"name": "Carrying On His Will", "releaseDate": "2025-11-21"},
		"ST-13": {"name": "Ultra Deck: The Three Brothers", "releaseDate": "2023-12-08"}
	},
	"cards": [
		{"id": "p-041_531486", "name": "Monkey.D.Luffy", "number": "P-041", "setCode": "OP-PR", "rarity": "PR", "finish": "Normal", "image": "x", "externalLinks": {"tcgPlayerId": 531486}},
		{"id": "p-041_544782_foil", "name": "Monkey.D.Luffy", "number": "P-041", "setCode": "OP-PR", "rarity": "PR", "finish": "Foil", "variant": "Offline Regional 2024 Vol. 2 Participant", "promoTypes": ["offlineregional", "participant"], "watermark": "vol. 2", "originalReleaseDate": "2024-01-01", "image": "x", "externalLinks": {"tcgPlayerId": 544782}},
		{"id": "p-041_544783_foil", "name": "Monkey.D.Luffy", "number": "P-041", "setCode": "OP-PR", "rarity": "PR", "finish": "Foil", "variant": "Online Regional 2024 Vol. 2 Participant", "promoTypes": ["onlineregional", "participant"], "watermark": "vol. 2", "originalReleaseDate": "2024-01-01", "image": "x", "externalLinks": {"tcgPlayerId": 544783}},
		{"id": "p-041_544784_foil", "name": "Monkey.D.Luffy", "number": "P-041", "setCode": "OP-PR", "rarity": "PR", "finish": "Foil", "variant": "Offline Regional 2024 Vol. 2 Finalist", "promoTypes": ["offlineregional", "finalist"], "watermark": "vol. 2", "originalReleaseDate": "2024-01-01", "image": "x", "externalLinks": {"tcgPlayerId": 544784}},
		{"id": "p-041_544785_foil", "name": "Monkey.D.Luffy", "number": "P-041", "setCode": "OP-PR", "rarity": "PR", "finish": "Foil", "variant": "Online Regional 2024 Vol. 2 Finalist", "promoTypes": ["onlineregional", "finalist"], "watermark": "vol. 2", "originalReleaseDate": "2024-01-01", "image": "x", "externalLinks": {"tcgPlayerId": 544785}},
		{"id": "p-041_544786_foil", "name": "Monkey.D.Luffy", "number": "P-041", "setCode": "OP-PR", "rarity": "PR", "finish": "Foil", "variant": "Offline Regional 2024 Vol. 2 Winner", "promoTypes": ["offlineregional", "winner"], "watermark": "vol. 2", "originalReleaseDate": "2024-01-01", "image": "x", "externalLinks": {"tcgPlayerId": 544786}},
		{"id": "p-041_544787_foil", "name": "Monkey.D.Luffy", "number": "P-041", "setCode": "OP-PR", "rarity": "PR", "finish": "Foil", "variant": "Online Regional 2024 Vol. 2 Winner", "promoTypes": ["onlineregional", "winner"], "watermark": "vol. 2", "originalReleaseDate": "2024-01-01", "image": "x", "externalLinks": {"tcgPlayerId": 544787}},
		{"id": "p-041_580055_foil", "name": "Monkey.D.Luffy", "number": "P-041", "setCode": "OP-PR", "rarity": "PR", "finish": "Foil", "variant": "CS 2024 Participation", "promoTypes": ["cs", "participant"], "originalReleaseDate": "2024-01-01", "image": "x", "externalLinks": {"tcgPlayerId": 580055}},
		{"id": "p-069_649621_foil", "name": "Koala", "number": "P-069", "setCode": "OP-PR", "rarity": "PR", "finish": "Foil", "variant": "CS 25-26 Event Pack", "promoTypes": ["cs", "eventpack"], "watermark": "25-26", "image": "x", "externalLinks": {"tcgPlayerId": 649621}},
		{"id": "p-069_649666_foil", "name": "Koala", "number": "P-069", "setCode": "OP-PR", "rarity": "PR", "finish": "Foil", "variant": "CS 25-26 Event Pack Finalist Ver.", "promoTypes": ["cs", "eventpack", "finalist"], "watermark": "25-26", "image": "x", "externalLinks": {"tcgPlayerId": 649666}},
		{"id": "p-069_668433_foil", "name": "Koala", "number": "P-069", "setCode": "OP-PR", "rarity": "PR", "finish": "Foil", "variant": "CS 25-26 Top Player Pack Vol. 2", "promoTypes": ["cs", "topplayerpack"], "watermark": "25-26 vol. 2", "image": "x", "externalLinks": {"tcgPlayerId": 668433}},
		{"id": "p-069_668434_foil", "name": "Koala", "number": "P-069", "setCode": "OP-PR", "rarity": "PR", "finish": "Foil", "variant": "CS 25-26 Finalist Card Set 2", "promoTypes": ["cs", "finalistcardset"], "watermark": "25-26 2", "image": "x", "externalLinks": {"tcgPlayerId": 668434}},
		{"id": "p-069_668435_foil", "name": "Koala", "number": "P-069", "setCode": "OP-PR", "rarity": "PR", "finish": "Foil", "variant": "CS 25-26 Winner Card Set 2", "promoTypes": ["cs", "winnercardset"], "watermark": "25-26 2", "image": "x", "externalLinks": {"tcgPlayerId": 668435}},
		{"id": "661879", "name": "Monkey.D.Luffy", "setCode": "OP13-ANN", "rarity": "L", "finish": "Normal", "variant": "3rd Anniversary Tournament 3 Brothers Pack", "promoTypes": ["anniversarytournament", "3brotherspack"], "watermark": "3rd", "image": "x", "externalLinks": {"tcgPlayerId": 661879}},
		{"id": "op13-081_661880", "name": "Monkey.D.Luffy", "number": "OP13-081", "setCode": "OP13-ANN", "rarity": "SR", "finish": "Normal", "image": "x", "externalLinks": {"tcgPlayerId": 661880}},
		{"id": "st01-012_661882", "name": "Monkey.D.Luffy", "number": "ST01-012", "setCode": "OP13-ANN", "rarity": "SR", "finish": "Normal", "variant": "3rd Anniversary Tournament 3 Brothers Pack", "promoTypes": ["anniversarytournament", "3brotherspack"], "watermark": "3rd", "image": "x", "externalLinks": {"tcgPlayerId": 661882}},
		{"id": "op13-081_657350_foil", "name": "Monkey.D.Luffy", "number": "OP13-081", "setCode": "OP13", "rarity": "SR", "finish": "Foil", "image": "x", "externalLinks": {"tcgPlayerId": 657350}},
		{"id": "st13-003_543605_foil", "name": "Monkey.D.Luffy", "number": "ST13-003", "setCode": "ST-13", "rarity": "L", "finish": "Foil", "image": "x", "externalLinks": {"tcgPlayerId": 543605}},
		{"id": "st13-003_600711_foil", "name": "Monkey.D.Luffy", "number": "ST13-003", "setCode": "OP-PR", "rarity": "L", "finish": "Foil", "variant": "2nd Anniversary Tournament", "promoTypes": ["anniversarytournament"], "watermark": "2nd", "image": "x", "externalLinks": {"tcgPlayerId": 600711}}
	]
}}`

// TestLabelWording pins the listings whose wording carries a place, a mark
// or a number inside a label. Each answered with another printing of the
// number: the plain P-041 for the championship's participant card, the
// finalist's event pack for the finalist card set, and the 2nd Anniversary
// Tournament's Luffy - a third card of some set - for the 3rd's leader.
func TestLabelWording(t *testing.T) {
	b, err := Load(strings.NewReader(labelWordingFixture))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		desc string
		in   matcher.InputCard
		want string
	}{
		{
			// The storefront writes the place in its own word, and the
			// regional families at the number hold a participant each.
			desc: "participation names the participant's printing",
			in: matcher.InputCard{
				Name: "Monkey.D.Luffy", Variation: "P-041 CS 2024 Participation",
				Edition: "One Piece Promotion Cards",
			},
			want: "p-041_580055_foil",
		},
		{
			desc: "and a wording naming no place keeps the plain printing",
			in: matcher.InputCard{
				Name: "Monkey.D.Luffy", Variation: "P-041",
				Edition: "One Piece Promotion Cards",
			},
			want: "p-041_531486",
		},
		{
			// "Finalist" is a word of the set's name, and the mark "25-26
			// 2" is written at both ends of it.
			desc: "a place inside a label the wording spells whole is not asked",
			in: matcher.InputCard{
				Name: "Koala", Variation: "P-069 CS 25-26 Finalist Card Set 2",
				Edition: "One Piece Promotion Cards",
			},
			want: "p-069_668434_foil",
		},
		{
			desc: "and the place on its own still picks the finalist's event pack",
			in: matcher.InputCard{
				Name: "Koala", Variation: "P-069 CS 25-26 Event Pack Finalist Ver.",
				Edition: "One Piece Promotion Cards",
			},
			want: "p-069_649666_foil",
		},
		{
			desc: "a mark of two words is named by a wording saying both apart",
			in: matcher.InputCard{
				Name: "Koala", Variation: "P-069 CS 25-26 Winner Card Set 2",
				Edition: "One Piece Promotion Cards",
			},
			want: "p-069_668435_foil",
		},
		{
			// Neither the ordinal nor the count is a collector number,
			// and the leader is the card the catalog writes none for.
			desc: "the words of a label are not the number",
			in: matcher.InputCard{
				Name: "Monkey.D.Luffy", Variation: "3rd Anniversary Tournament 3 Brothers Pack",
				Edition: "Carrying On His Will: 3rd Anniversary Tournament Cards",
			},
			want: "661879",
		},
		{
			desc: "and the card packed with it is named by its number",
			in: matcher.InputCard{
				Name: "Monkey.D.Luffy", Variation: "ST01-012 3rd Anniversary Tournament 3 Brothers Pack",
				Edition: "Carrying On His Will: 3rd Anniversary Tournament Cards",
			},
			want: "st01-012_661882",
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			in := tt.in
			got, err := b.Match(&in)
			if err != nil {
				t.Fatalf("Match(%v): %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("Match(%v) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

// TestUnlabelled pins what the number reading is left once the label words
// are gone: a full number stays whatever the label says, a bare one stays
// where no label holds it, and the words of a label go whether the wording
// spells the label as a run or the mark's words apart.
func TestUnlabelled(t *testing.T) {
	b, err := Load(strings.NewReader(labelWordingFixture))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, wording, want string
	}{
		{"Monkey.D.Luffy", "3rd Anniversary Tournament 3 Brothers Pack", ""},
		{"Monkey.D.Luffy", "ST01-012 3rd Anniversary Tournament 3 Brothers Pack", "ST01-012"},
		{"Monkey.D.Luffy", "041 Offline Regional 2024 Vol. 2 Participant", "041"},
		{"Koala", "CS 25-26 Top Player Pack Vol. 2", ""},
		{"Koala", "069 CS 25-26 Finalist Card Set 2", "069"},
	}
	for _, tt := range tests {
		got := extractNumber(unlabelled(b, tt.name, tt.wording))
		if got != tt.want {
			t.Errorf("unlabelled(%q, %q) reads number %q, want %q", tt.name, tt.wording, got, tt.want)
		}
	}
}
