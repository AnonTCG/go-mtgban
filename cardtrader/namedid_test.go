package cardtrader

import (
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
)

// TestNamedID pins which of a blueprint's two ids answers for it. The ids are
// the surest thing Card Trader publishes and the scryfall one is preferred,
// but they are not checked against each other on the way in: eight Strixhaven
// promos each carry the scryfall id of the card before them on the shelf, so
// "Exponential Growth" priced as Ecological Appreciation for as long as the
// listing stood.
func TestNamedID(t *testing.T) {
	b := realDatastore(t)
	uuid := func(space matcher.IDSpace, id string) string {
		out := b.ConvertID(space, id)
		if out == "" {
			t.Fatalf("%s id %q resolves to nothing", space, id)
		}
		return out
	}
	// The shifted blueprint: its scryfall id is Ecological Appreciation's and
	// its TCGplayer id is its own.
	shifted := uuid(matcher.IDSpaceScryfall, "357ce522-4872-4386-b409-264e23953a53")
	itsOwn := uuid(matcher.IDSpaceTCGplayer, "237134")
	// A blueprint whose ids agree on the card, which must not move.
	plainScryfall := uuid(matcher.IDSpaceScryfall, "e939194a-6993-412c-bbd8-540b9872a721")
	plainTCGplayer := uuid(matcher.IDSpaceTCGplayer, "237341")

	for _, tt := range []struct {
		desc                          string
		uuidFromScryfall, uuidFromTCG string
		cardName                      string
		want                          string
	}{
		{"neither id says anything", "", "", "Exponential Growth", ""},
		{"only the scryfall id", shifted, "", "Exponential Growth", shifted},
		{"only the TCGplayer id", "", itsOwn, "Exponential Growth", itsOwn},
		{
			// Both name Verdant Mastery, so the order stands and the
			// preferred id answers as it always has.
			desc:             "ids agreeing on the card leave the order alone",
			uuidFromScryfall: plainScryfall, uuidFromTCG: plainTCGplayer,
			cardName: "Verdant Mastery", want: plainScryfall,
		},
		{
			// The wording is the third thing the blueprint says, and it is
			// what breaks a tie the ids cannot.
			desc:             "the name picks the id that names its card",
			uuidFromScryfall: shifted, uuidFromTCG: itsOwn,
			cardName: "Exponential Growth", want: itsOwn,
		},
		{
			desc:             "and leaves the preferred id where the name agrees with it",
			uuidFromScryfall: shifted, uuidFromTCG: itsOwn,
			cardName: "Ecological Appreciation", want: shifted,
		},
		{
			// A pair of tokens sold under one blueprint names neither, and
			// guessing between them would be worse than keeping the order.
			desc:             "a name settling nothing changes nothing",
			uuidFromScryfall: shifted, uuidFromTCG: itsOwn,
			cardName: "Soldier // Spirit", want: shifted,
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			got := namedID(b, &Blueprint{Name: tt.cardName}, tt.uuidFromScryfall, tt.uuidFromTCG)
			if got != tt.want {
				t.Errorf("namedID = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestNamedIDSamePrinting pins the choice between two ids that name one card
// in two printings, with the blueprints' own ids and collector numbers.
func TestNamedIDSamePrinting(t *testing.T) {
	b := realDatastore(t)
	scryfall := func(id string) string {
		out := b.ConvertID(matcher.IDSpaceScryfall, id)
		if out == "" {
			t.Fatalf("scryfall id %q resolves to nothing", id)
		}
		return out
	}
	tcgplayer := func(id string) string {
		out := b.ConvertID(matcher.IDSpaceTCGplayer, id)
		if out == "" {
			t.Fatalf("TCGplayer id %q resolves to nothing", id)
		}
		return out
	}

	for _, tt := range []struct {
		desc               string
		name, number       string
		scryfallID, tcgID  string
		wantTCGplayerPrint bool
	}{
		{
			// The scryfall id names the etched twin, 2X2 427.
			desc: "the printing with the blueprint's number wins",
			name: "Consecrated Sphinx", number: "345",
			scryfallID: "2fddeced-9d04-4b4c-bf3f-e9add96f4f5a", tcgID: "277162",
			wantTCGplayerPrint: true,
		},
		{
			// The player prefix is not part of the blueprint's number.
			desc: "a World Championship prefix is set aside",
			name: "Swamp", number: "440",
			scryfallID: "12681e76-6ffc-489e-8dcd-131a31170ced", tcgID: "162302",
			wantTCGplayerPrint: true,
		},
		{
			// The scryfall id names the Chinese alt art, 5ED 147s.
			desc: "the number outweighs a foreign alt art",
			name: "Bog Wraith", number: "147",
			scryfallID: "fe305196-fd9d-4f92-a37c-6c4fe5abad1f", tcgID: "2053",
			wantTCGplayerPrint: true,
		},
		{
			// No number to go by, and the scryfall id names the Japanese
			// printing, PMEI 2020-7.
			desc:       "the English printing beats a foreign one",
			name:       "Crop Rotation",
			scryfallID: "0f8b6160-84cc-4052-8c19-6184a72c16a6", tcgID: "672480",
			wantTCGplayerPrint: true,
		},
		{
			// The scryfall id agrees with the number, et364.
			desc: "a scryfall id agreeing with the number is kept",
			name: "Plains", number: "364",
			scryfallID: "7d39625b-cfc5-439e-b888-fbd7849f47a6", tcgID: "158561",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			bp := &Blueprint{Name: tt.name}
			bp.Properties.Number = tt.number
			scry, tcg := scryfall(tt.scryfallID), tcgplayer(tt.tcgID)
			want := scry
			if tt.wantTCGplayerPrint {
				want = tcg
			}
			got := namedID(b, bp, scry, tcg)
			if got != want {
				t.Errorf("namedID = %q, want %q", got, want)
			}
		})
	}
}
