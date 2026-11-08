package strikezone

import (
	"testing"
)

// TestUnnamedTextured pins which listings the textured guard refuses. The
// store shelves the Duskmourn mythics three times over - plain, "(Showcase)"
// and "(Showcase) (Textured)" - and the set files the showcase frame on the
// textured printing. A "(Showcase)" wording is the borderless double
// exposure, which the table sends to its number.
func TestUnnamedTextured(t *testing.T) {
	b := realDatastore(t)

	for _, tt := range []struct {
		desc, name, edition, notes string
		wantNumber                 string
	}{
		{
			desc: "the wording that names it keeps the textured printing",
			name: "Valgavoth, Terror Eater (Showcase) (Textured)", edition: "Duskmourn: House of Horror",
			notes: "Near Mint Foil English", wantNumber: "407",
		},
		{
			desc: "the one standing beside it names the double exposure",
			name: "Valgavoth, Terror Eater (Showcase)", edition: "Duskmourn: House of Horror",
			notes: "Near Mint Foil English", wantNumber: "352",
		},
		{
			desc: "and says so in either finish",
			name: "Valgavoth, Terror Eater (Showcase)", edition: "Duskmourn: House of Horror",
			notes: "Near Mint Normal English", wantNumber: "352",
		},
		{
			desc: "Tyvar sells the same three",
			name: "Tyvar, the Pummeler (Showcase)", edition: "Duskmourn: House of Horror",
			notes: "Near Mint Foil English", wantNumber: "353",
		},
		{
			desc: "so does Kaito",
			name: "Kaito, Bane of Nightmares (Showcase)", edition: "Duskmourn: House of Horror",
			notes: "Near Mint Foil English", wantNumber: "354",
		},
		{
			desc: "and the Wandering Rescuer",
			name: "The Wandering Rescuer (Showcase)", edition: "Duskmourn: House of Horror",
			notes: "Near Mint Foil English", wantNumber: "351",
		},
		{
			desc: "Niko sells the same three",
			name: "Niko, Light of Hope (Showcase)", edition: "Duskmourn: House of Horror",
			notes: "Near Mint Foil English", wantNumber: "355",
		},
		{
			desc: "a number matching the textured printing names it as plainly as the word",
			name: "Kaito, Bane of Nightmares (Showcase) (409)", edition: "Duskmourn: House of Horror",
			notes: "Near Mint Foil English", wantNumber: "409",
		},
		{
			desc: "silence over a printing wearing nothing is not a claim",
			name: "Valgavoth, Terror Eater", edition: "Duskmourn: House of Horror",
			notes: "Near Mint Foil English", wantNumber: "120",
		},
		{
			desc: "nor is it one where the wording reaches the untextured sibling",
			name: "Solitude (Borderless)", edition: "Special Guests",
			notes: "Near Mint Foil English", wantNumber: "44",
		},
		{
			desc: "a set spelling the word out is left where it landed",
			name: "Solitude (Borderless) (Textured Foil)", edition: "Special Guests",
			notes: "Near Mint Foil English", wantNumber: "49",
		},
		{
			desc: "a serialized retro printing wears the claim in frame 2003",
			name: "Emeritus of Ideation (Retro Frame) (Serial Numbered) (Double Rainbow Foil)", edition: "Secrets of Strixhaven",
			notes: "Near Mint Foil English", wantNumber: "306",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			card, err := preprocess(b, tt.name, tt.edition, tt.notes)
			if err != nil {
				t.Fatalf("preprocess(%q) = %v", tt.name, err)
			}
			cardID, err := b.Match(card)
			if err != nil {
				t.Fatalf("Match(%q) = %v", tt.name, err)
			}
			co, err := b.GetUUID(cardID)
			if err != nil {
				t.Fatal(err)
			}

			refused := namesAbsentTreatment(card.Variation, co) ||
				wearsUnnamedTextured(b, card.Variation, co)
			if refused {
				if tt.wantNumber != "" {
					t.Fatalf("%q was refused, want %s #%s", tt.name, co.SetCode, tt.wantNumber)
				}
				return
			}
			if tt.wantNumber == "" {
				t.Fatalf("%q kept %s #%s %v, want refused",
					tt.name, co.SetCode, co.Number, co.PromoTypes)
			}
			if co.Number != tt.wantNumber {
				t.Errorf("%q got #%s, want #%s", tt.name, co.Number, tt.wantNumber)
			}
		})
	}
}
