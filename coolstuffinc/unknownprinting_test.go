package coolstuffinc

import (
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/matcher"

	_ "github.com/mtgban/go-mtgban/matcher/games"
)

// TestUnknownPrinting pins the two listings dropped for naming a printing the
// catalog does not carry, and the shelf each is keyed to: the same name on
// another shelf is another card, and the printing sold beside each of them is
// the one they would otherwise land on.
func TestUnknownPrinting(t *testing.T) {
	tests := []struct {
		desc    string
		name    string
		edition string
		want    bool
	}{
		{
			desc:    "the plain secret rare of a set that prints only emblazoned ones",
			name:    "Exodia the Forbidden One (Secret Rare)",
			edition: "Limited Pack World Championship 2025", want: true,
		},
		{
			desc:    "and the plain ultra rare of the same card",
			name:    "Exodia the Forbidden One (Ultra Rare)",
			edition: "Limited Pack World Championship 2025", want: true,
		},
		{
			desc:    "the super rare of a card that is secret rare only",
			name:    "Gladiator Beast Octavius (Super Rare)",
			edition: "Gladiators Assault", want: true,
		},
		{
			desc:    "a rarity the card is not printed in",
			name:    "Dark Magician Knight",
			edition: "Promo", want: true,
		},
		{
			desc:    "a Paris museum giveaway the catalog has no product for",
			name:    "Monkey.D.Luffy - 001 (Grevin Museum Promo)",
			edition: "OP13 - Carrying On His Will", want: true,
		},
		{
			desc:    "the printing each of them was landing on is kept",
			name:    "Gladiator Beast Octavius (Secret Rare)",
			edition: "Gladiators Assault", want: false,
		},
		{
			desc:    "and so is the emblazoned one, which is the $500 card",
			name:    "Exodia the Forbidden One (Emblazoned Secret Rare)",
			edition: "Limited Pack World Championship 2025", want: false,
		},
		{
			desc:    "the same name on another shelf is another card",
			name:    "Exodia the Forbidden One (Secret Rare)",
			edition: "Battles of Legend - Glorious Gallery", want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			if got := unknownPrinting(test.name, test.edition); got != test.want {
				t.Errorf("unknownPrinting(%q, %q) = %v, want %v", test.name, test.edition, got, test.want)
			}
		})
	}
}

// TestUnknownPrintingKept pins that the printing each dropped listing would
// land on still resolves: the drop is one listing, not the card.
func TestUnknownPrintingKept(t *testing.T) {
	b := readGameDatastore(t, "yugioh", "YUGIOH_PATH")

	for _, test := range []struct {
		name, edition, variation, wantSet, wantNumber string
	}{
		{"Gladiator Beast Octavius (Secret Rare)", "Gladiators Assault", "GLAS-EN000 Secret Rare", "GLAS", "GLAS-EN000"},
		{"Exodia the Forbidden One (Emblazoned Secret Rare)", "Limited Pack World Championship 2025", "Emblazoned Secret Rare", "25LP", "25LP-EN000"},
		{"Exodia the Forbidden One (Emblazoned Ultra Rare)", "Limited Pack World Championship 2025", "Emblazoned Ultra Rare", "25LP", "25LP-EN000"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if unknownPrinting(test.name, test.edition) {
				t.Fatalf("unknownPrinting(%q) dropped the printing it should keep", test.name)
			}
			card := &matcher.InputCard{
				Name:      catalogColor(test.name),
				Edition:   printRunEdition(test.edition, ""),
				Variation: strings.TrimSpace(test.variation),
			}
			id, err := b.Match(card)
			if err != nil {
				t.Fatalf("Match(%q) = %v", card, err)
			}
			co, err := b.GetUUID(id)
			if err != nil {
				t.Fatal(err)
			}
			if co.SetCode != test.wantSet || co.Number != test.wantNumber {
				t.Errorf("Match(%q) = %s|%s, want %s|%s", card, co.SetCode, co.Number, test.wantSet, test.wantNumber)
			}
		})
	}
}
