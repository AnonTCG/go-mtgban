package abugames

import (
	"errors"
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
)

// TestUnprintedFinish pins the listings this storefront prices in a finish
// the catalog never sold the printing in. There is no printing of their own
// to price them against, and answering with the finish that was printed puts
// two of the storefront's prices on one uuid.
func TestUnprintedFinish(t *testing.T) {
	b := realDatastore(t)
	for _, test := range []struct {
		desc string
		card ABUCard
	}{
		{"a commander deck's extended art, which is sold in no foil", ABUCard{
			DisplayTitle: "Bladewing, Deathless Tyrant (Extended Art) - FOIL",
			Edition:      "Dominaria United Commander", Number: "9"}},
		{"and another set's", ABUCard{
			DisplayTitle: "Tributary Instructor (Extended Art) - FOIL",
			Edition:      "The Lost Caverns of Ixalan Commander", Number: "64"}},
	} {
		t.Run(test.desc, func(t *testing.T) {
			card := test.card
			in, err := preprocess(b, &card)
			if !errors.Is(err, errUnprintedFinish) {
				t.Errorf("preprocess(%q) = %v, %v, want %v", card.DisplayTitle, in, err, errUnprintedFinish)
			}
		})
	}
}

// TestNeoJapaneseOnlyBasics pins Kamigawa: Neon Dynasty's full-art basics,
// 293 through 302, which this storefront tags English though the catalog
// prints them only in Japanese.
func TestNeoJapaneseOnlyBasics(t *testing.T) {
	b := realDatastore(t)
	for _, test := range []struct {
		name, number string
	}{
		{"Plains", "293"}, {"Plains", "294"},
		{"Island", "295"}, {"Island", "296"},
		{"Swamp", "297"}, {"Swamp", "298"},
		{"Mountain", "299"}, {"Mountain", "300"},
		{"Forest", "301"}, {"Forest", "302"},
	} {
		t.Run(test.name+" "+test.number, func(t *testing.T) {
			card := ABUCard{DisplayTitle: test.name + " (" + test.number + ")",
				Edition: "Kamigawa: Neon Dynasty", Number: test.number, Language: []string{"English"}}
			in, err := preprocess(b, &card)
			if err != nil {
				t.Fatalf("preprocess(%q) = %v", card.DisplayTitle, err)
			}
			id, err := b.Match(in)
			if err != nil {
				t.Fatalf("Match(%q) = %v", in, err)
			}
			co, err := b.GetUUID(id)
			if err != nil {
				t.Fatal(err)
			}
			if co.SetCode != "NEO" || co.Number != test.number {
				t.Errorf("Match(%q) = %s|%s, want NEO|%s", in, co.SetCode, co.Number, test.number)
			}
		})
	}
}

// TestPrereleaseWildcardEscape pins the two Lost Caverns of Ixalan
// double-faced prerelease promos PLCI does not hold. Left alone, a
// Prerelease shelf naming a set the backend carries wildcards past it and
// lands on 2017 Ixalan's own same-named prerelease instead of refusing.
func TestPrereleaseWildcardEscape(t *testing.T) {
	b := realDatastore(t)
	for _, test := range []struct {
		desc, title, number string
	}{
		{"Growing Rites of Itlimoc", "Growing Rites of Itlimoc / Itlimoc, Cradle of the Sun (Prerelease) - FOIL", "188s"},
		{"Treasure Map", "Treasure Map / Treasure Cove (Prerelease) - FOIL", "267s"},
	} {
		t.Run(test.desc, func(t *testing.T) {
			card := ABUCard{DisplayTitle: test.title, Edition: "The Lost Caverns of Ixalan Promos",
				Number: test.number, Language: []string{"English"}}
			in, err := preprocess(b, &card)
			if !errors.Is(err, matcher.ErrUnsupported) {
				t.Errorf("preprocess(%q) = %v, %v, want %v", test.title, in, err, matcher.ErrUnsupported)
			}
		})
	}
}

// TestForeignListing pins the listings priced in a language the catalog never
// printed the card in. The match falls back on the English printing, so the
// storefront's Italian and Japanese prices land beside its English one. A
// listing whose ids all name its own-language printing in the finish it asks
// for is kept on that printing instead.
func TestForeignListing(t *testing.T) {
	b := realDatastore(t)
	for _, test := range []struct {
		desc   string
		card   ABUCard
		want   error
		wantID string
	}{
		{"an Italian printing this set never had", ABUCard{
			DisplayTitle: "Elvish Piper - FOIL", Edition: "9th Edition", Number: "239",
			Title: "Non-English - Italian", Language: []string{"Italian"}}, errForeignListing, ""},
		{"ids naming the English printing leave it foreign", ABUCard{
			DisplayTitle: "Elvish Piper", Edition: "9th Edition", Number: "239",
			Title: "Non-English - Italian", Language: []string{"Italian"},
			ScryfallIDs: []string{"cb459fb2-b0d3-4be9-99cb-fcff4518abc2"}, TCGplayerIDs: []int64{12635}}, errForeignListing, ""},
		{"a Japanese one it never had either", ABUCard{
			DisplayTitle: "Elvish Piper - FOIL", Edition: "9th Edition", Number: "239",
			Title: "Non-English - Japanese", Language: []string{"Japanese"}}, errForeignListing, ""},
		{"a set that does hold the Japanese printing is kept", ABUCard{
			DisplayTitle: "Urza's Mine (d Tower)", Edition: "Chronicles", Number: "114d",
			Title: "Non-English - Japanese", Language: []string{"Japanese"}}, nil, ""},
		{"ids naming the listing's own Japanese printing are kept", ABUCard{
			DisplayTitle: "Flusterstorm (JP Alternate Art 73)", Edition: "Secrets of Strixhaven Mystical Archive",
			Number: "83", Title: "Non-English - Japanese", Language: []string{"Japanese"},
			ScryfallIDs: []string{"9df19f7c-2922-4d71-999a-818ae69045c3"}, TCGplayerIDs: []int64{690170}}, nil,
			"96d8a2e9-2bde-55ec-9487-2f218779b589"},
		{"a flavor name is validated by the oracle half of the title", ABUCard{
			DisplayTitle: "Witch's Prophecy | Force of Negation (Buy-A-Box)", Edition: "Promo", Number: "1",
			Title: "Non-English - Japanese", Language: []string{"Japanese"},
			ScryfallIDs: []string{"bc5376cb-82f6-43c7-bf8b-43515791e082"}, TCGplayerIDs: []int64{637609}}, nil,
			"64d30071-456d-582c-9fcc-68d21f238ca6"},
		{"the same ids on a finish that printing was never sold in stay foreign", ABUCard{
			DisplayTitle: "Flusterstorm (JP Alternate Art 73) - FOIL", Edition: "Secrets of Strixhaven Mystical Archive",
			Number: "83", Title: "Non-English - Japanese", Language: []string{"Japanese"},
			ScryfallIDs: []string{"9df19f7c-2922-4d71-999a-818ae69045c3"}}, errForeignListing, ""},
	} {
		t.Run(test.desc, func(t *testing.T) {
			card := test.card
			in, err := preprocess(b, &card)
			if test.want == nil {
				if err != nil {
					t.Errorf("preprocess(%q) = %v, want no error", card.DisplayTitle, err)
				} else if test.wantID != "" && in.ID != test.wantID {
					t.Errorf("preprocess(%q) = %s, want %s", card.DisplayTitle, in.ID, test.wantID)
				}
				return
			}
			if !errors.Is(err, test.want) {
				t.Errorf("preprocess(%q) = %v, want %v", card.DisplayTitle, err, test.want)
			}
		})
	}
}
