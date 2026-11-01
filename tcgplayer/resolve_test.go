package tcgplayer

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-tcgplayer"
)

// magicLanguages is the Magic catalog's language table.
var magicLanguages = []tcgplayer.Language{
	{LanguageID: 1, Name: "English"}, {LanguageID: 2, Name: "Chinese (S)"},
	{LanguageID: 3, Name: "Chinese (T)"}, {LanguageID: 4, Name: "French"},
	{LanguageID: 5, Name: "German"}, {LanguageID: 6, Name: "Italian"},
	{LanguageID: 7, Name: "Japanese"}, {LanguageID: 8, Name: "Korean"},
	{LanguageID: 9, Name: "Portuguese"}, {LanguageID: 10, Name: "Russian"},
	{LanguageID: 11, Name: "Spanish"},
}

func magicResolver(t *testing.T) *resolver {
	t.Helper()
	b := realDatastore(t)
	r, err := newResolver(b, &tcgplayer.CatalogDump{
		Category:  tcgplayer.Category{CategoryID: tcgplayer.CategoryMagic, Name: "Magic"},
		Languages: magicLanguages,
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestResolverLanguages pins that every Magic catalog language is read as
// the datastore spells it, the three that differ included.
func TestResolverLanguages(t *testing.T) {
	r := magicResolver(t)
	want := map[int]string{
		1: "English", 2: "Chinese Simplified", 3: "Chinese Traditional",
		4: "French", 5: "German", 6: "Italian", 7: "Japanese", 8: "Korean",
		9: "Portuguese (Brazil)", 10: "Russian", 11: "Spanish",
	}
	for id, name := range want {
		if r.languages[id] != name {
			t.Errorf("language %d reads as %q, want %q", id, r.languages[id], name)
		}
	}
	if r.english != 1 {
		t.Errorf("English is %d, want 1", r.english)
	}
}

// TestResolverSKU pins what a sku resolves to: its product in its finish,
// then its language's copy of that printing.
func TestResolverSKU(t *testing.T) {
	r := magicResolver(t)
	for _, tt := range []struct {
		desc       string
		product    int
		finish     string
		language   int
		set        string
		number     string
		foil       bool
		etched     bool
		derived    bool
		err        error
		errAnyKind bool
	}{
		{desc: "a plain printing", product: 3040, finish: "Normal", set: "7ED", number: "157"},
		{desc: "a star foil with no product of its own is its base product's foil", product: 3040, finish: "Foil", set: "7ED", number: "157★", foil: true},
		{desc: "another star foil", product: 95037, finish: "Foil", set: "FRF", number: "65★", foil: true},
		{desc: "a foil sold as a product of its own", product: 286677, finish: "Foil", set: "40K", number: "179", foil: true},
		{desc: "a foil row on a product sold plain alone", product: 679621, finish: "Foil", errAnyKind: true},
		{desc: "an etched product's foil is the etched printing", product: 233370, finish: "Foil", set: "STA", number: "10", etched: true},
		{desc: "an etched product sold plain is not its plain sibling", product: 233370, finish: "Normal", err: matcher.ErrCardWrongFinish},
		{desc: "an alternative foil product is its split copy", product: 636747, finish: "Foil", set: "FIC", number: "9★", foil: true},
		{desc: "a token sheet's second product is its pairing", product: 200320, finish: "Foil", derived: true},
		{desc: "a double-faced token is its own product", product: 179416, finish: "Normal", set: "TGK1", number: "8"},
		{desc: "a product two printings share is the first one's", product: 2372, finish: "Normal", set: "5ED", number: "267"},
		{desc: "an alternative printing sharing a product is not its home", product: 126455, finish: "Normal", set: "AER", number: "52"},
		{desc: "a playtest card's twins share one", product: 641692, finish: "Normal", set: "UNK", number: "RZ04k"},
		{desc: "a product no card owns", product: 1, finish: "Normal", err: errUnownedProduct},
		{desc: "a product the datastore files at a sealed product", product: 275414, finish: "Normal", err: errUnownedProduct},
		{desc: "an Italian sku is the Italian copy", product: 3789, finish: "Normal", language: 6, set: "LEGITA", number: "1"},
		{desc: "an English sku is the English printing", product: 3789, finish: "Normal", language: 1, set: "LEG", number: "1"},
		{desc: "a language the printing has no copy in", product: 32544, finish: "Normal", language: 7, err: matcher.ErrCardUnknownID},
		{desc: "a copy filed as a printing of its own in the set", product: 3040, finish: "Normal", language: 2, set: "7ED", number: "157s"},
		{desc: "that copy's foil is its star foil", product: 3040, finish: "Foil", language: 2, set: "7ED", number: "157★s", foil: true},
		{desc: "a foreign variant numbered with a dagger", product: 5197, finish: "Normal", language: 11, set: "MIR", number: "87†"},
		{desc: "a copy on The List", product: 222411, finish: "Normal", language: 7, set: "PLST", number: "M19-185j"},
		{desc: "a copy several products share a name with is nobody's", product: 109676, finish: "Normal", language: 5, err: matcher.ErrCardUnknownID},
		{desc: "an English sku of a printing in another language", product: 557380, finish: "Foil", language: 1, set: "ACR", number: "272", foil: true},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			language := tt.language
			if language == 0 {
				language = r.english
			}
			cardID, err := r.sku(tcgplayer.SKU{ProductID: tt.product, LanguageID: language}, tt.finish)
			if tt.err != nil || tt.errAnyKind {
				if err == nil || tt.err != nil && !errors.Is(err, tt.err) {
					co, _ := r.backend.GetUUID(cardID)
					t.Fatalf("got %s (%v), want %v", co, err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			co, err := r.backend.GetUUID(cardID)
			if err != nil {
				t.Fatal(err)
			}
			if tt.derived {
				if co.Identifiers["derivedTokenPair"] != "true" || !co.Foil {
					t.Errorf("got %s, want a foil token pairing", co)
				}
				return
			}
			if co.SetCode != tt.set || co.Number != tt.number || co.Foil != tt.foil || co.Etched != tt.etched {
				t.Errorf("got %s %s foil=%v etched=%v, want %s %s foil=%v etched=%v", co.SetCode, co.Number, co.Foil, co.Etched, tt.set, tt.number, tt.foil, tt.etched)
			}
		})
	}
}

// TestResolverOwns pins which products the index prices: those a card owns.
func TestResolverOwns(t *testing.T) {
	r := magicResolver(t)
	for product, want := range map[int]bool{3040: true, 275414: false, 1: false} {
		if r.owns(product) != want {
			t.Errorf("owns(%d) = %v, want %v", product, !want, want)
		}
	}
}

// TestResolverCopyFallback pins that a copy filed as a printing of its own
// is taken only when it is the one candidate, numbered after the card.
func TestResolverCopyFallback(t *testing.T) {
	r := magicResolver(t)
	key := "7ED|Raise Dead|Chinese Simplified"
	copies := r.copies[key]
	if len(copies) != 2 {
		t.Fatalf("7th Edition's Chinese Raise Dead has %d copies, want 157s and 157★s", len(copies))
	}
	sku := tcgplayer.SKU{ProductID: 3040, LanguageID: 2}

	other := copies[0]
	other.UUID += "-other"
	r.copies[key] = append(slices.Clone(copies), other)
	_, err := r.sku(sku, "Normal")
	if !errors.Is(err, matcher.ErrCardUnknownID) {
		t.Errorf("two copies in one finish: error %v, want %v", err, matcher.ErrCardUnknownID)
	}

	renumbered := slices.Clone(copies)
	for i := range renumbered {
		renumbered[i].Number = "200s"
	}
	r.copies[key] = renumbered
	_, err = r.sku(sku, "Normal")
	if !errors.Is(err, matcher.ErrCardUnknownID) {
		t.Errorf("a copy numbered apart: error %v, want %v", err, matcher.ErrCardUnknownID)
	}
}

func TestNumberedAfter(t *testing.T) {
	for _, tt := range []struct {
		number, base string
		want         bool
	}{
		{"157s", "157", true},
		{"157★s", "157", true},
		{"87†", "87", true},
		{"M19-185j", "M19-185", true},
		{"157", "157", false},
		{"1570", "157", false},
		{"15", "1", false},
		{"200s", "157", false},
	} {
		if numberedAfter(tt.number, tt.base) != tt.want {
			t.Errorf("numberedAfter(%q, %q) = %v, want %v", tt.number, tt.base, !tt.want, tt.want)
		}
	}
}

// TestSKUCards pins the sku map mintcard names its listings by: English
// singles skus in a grade, each in its own finish.
func TestSKUCards(t *testing.T) {
	b := realDatastore(t)
	dump := &tcgplayer.CatalogDump{
		Category:  tcgplayer.Category{CategoryID: tcgplayer.CategoryMagic, Name: "Magic"},
		Languages: magicLanguages,
		Printings: []tcgplayer.Printing{{PrintingID: 1, Name: "Normal"}, {PrintingID: 2, Name: "Foil"}},
		Products: []tcgplayer.Product{
			{ProductID: 3040, ProductType: "Cards", Skus: []tcgplayer.SKU{
				{SKUID: 1, ProductID: 3040, LanguageID: 1, PrintingID: 1, ConditionID: 1},
				{SKUID: 2, ProductID: 3040, LanguageID: 1, PrintingID: 2, ConditionID: 2},
				// Japanese, unopened, and a printing the catalog does not name
				{SKUID: 3, ProductID: 3040, LanguageID: 7, PrintingID: 1, ConditionID: 1},
				{SKUID: 4, ProductID: 3040, LanguageID: 1, PrintingID: 1, ConditionID: SKUConditionUnopened},
				{SKUID: 5, ProductID: 3040, LanguageID: 1, PrintingID: 9, ConditionID: 1},
			}},
			{ProductID: 275414, ProductType: "Sealed Products", Skus: []tcgplayer.SKU{
				{SKUID: 6, ProductID: 275414, LanguageID: 1, PrintingID: 1, ConditionID: SKUConditionUnopened},
			}},
		},
	}
	got, err := SKUCards(b, dump)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]string{1: "7ED 157 false", 2: "7ED 157★ true"}
	if len(got) != len(want) {
		t.Errorf("got %d skus, want %d: %v", len(got), len(want), got)
	}
	for sku, desc := range want {
		co, err := b.GetUUID(got[sku])
		if err != nil {
			t.Errorf("sku %d: %v", sku, err)
			continue
		}
		if fmt.Sprintf("%s %s %v", co.SetCode, co.Number, co.Foil) != desc {
			t.Errorf("sku %d is %s %s foil=%v, want %s", sku, co.SetCode, co.Number, co.Foil, desc)
		}
	}
}
