package cardmarket

import (
	"testing"

	cm "github.com/mtgban/go-cardmarket"

	"github.com/mtgban/go-mtgban/matcher"
)

// TestResolveMagicResolvesTokenPairing pins a two-sided token product
// resolving to the combined entity matcher/magic derives for it, anchored
// by both face names plus the product's own edition, its Number naming the
// same faces. Preprocess/Match alone is not built to read "X Token (...) //
// Y Token (...)" and refuses every one of these listings outright.
func TestResolveMagicResolvesTokenPairing(t *testing.T) {
	b := realDatastore(t)
	res := &resolver{backend: b, gameID: cm.GameMagic}

	product := &cm.Product{
		IDProduct: 362664, Name: "Manifest Token // Angel Token (W 4/4)",
		ExpansionName: "Commander 2018", Number: "T 01/03", // as the catalog lists it
	}
	cardID, _, byName, err := resolveMagic(res, product)
	if err != nil {
		t.Fatalf("resolveMagic(%d) = %v", product.IDProduct, err)
	}
	if cardID == "" {
		t.Fatal("resolveMagic returned no id, want the derived Manifest // Angel pairing")
	}
	if !byName {
		t.Error("resolveMagic said the pairing was found by id, want it named")
	}
	co, err := b.GetUUID(cardID)
	if err != nil {
		t.Fatalf("GetUUID(%s) = %v", cardID, err)
	}
	if co.Identifiers["derivedTokenPair"] != "true" {
		t.Errorf("resolved to %q, want a derived token pairing", co.Name)
	}
	if co.Name != "Manifest // Angel" {
		t.Errorf("resolved to %q, want \"Manifest // Angel\"", co.Name)
	}
}

// TestResolveMagicRefusesAmbiguousTokenPairing pins the reverse: Commander
// 2016 prints more than one "Bird" token and more than one "Spirit" token,
// so a listing naming only "Bird" and "Spirit" cannot say which pairing it
// means - the same collision TestTokenPairIndexCollision already pins at
// the matcher/magic level. A refusal here is correct: guessing one of
// several candidate pairings would risk pricing the wrong physical product.
func TestResolveMagicRefusesAmbiguousTokenPairing(t *testing.T) {
	b := realDatastore(t)
	res := &resolver{backend: b, gameID: cm.GameMagic}

	product := &cm.Product{
		IDProduct: 294192, Name: "Bird Token (W 1/1) // Spirit Token (W 1/1)",
		ExpansionName: "Commander 2016", Number: "T 2/6",
	}
	cardID, cardIDFoil, _, err := resolveMagic(res, product)
	if err != nil {
		t.Fatalf("resolveMagic(%d) = %v", product.IDProduct, err)
	}
	if cardID != "" || cardIDFoil != "" {
		t.Fatalf("resolveMagic(%d) = (%q, %q), want both empty: Commander 2016 prints several Bird and Spirit tokens, name+edition alone cannot say which pairing this is", product.IDProduct, cardID, cardIDFoil)
	}
}

// TestResolveMagicRefusesOtherFaces pins TMH3's two Copy // Eldrazi Angel
// products onto the one pair the datastore has: "CT 1/2" lands on it, and
// "CT 37/2", Copy #37's, names no printing of ours.
func TestResolveMagicRefusesOtherFaces(t *testing.T) {
	b := realDatastore(t)
	res := &resolver{backend: b, gameID: cm.GameMagic}
	for _, tt := range []struct {
		number string
		landed bool
	}{
		{"CT 1/2", true},
		{"CT 37/2", false},
	} {
		product := &cm.Product{Name: "Copy Token // Eldrazi Angel Token (C 4/4)", ExpansionName: "Modern Horizons 3: Tokens", Number: tt.number}
		cardID, cardIDFoil, _, err := resolveMagic(res, product)
		if err != nil || (cardID != "") != tt.landed || (cardIDFoil != "") != tt.landed {
			t.Errorf("resolveMagic(%q) = (%q, %q, %v), want landed %v", tt.number, cardID, cardIDFoil, err, tt.landed)
		}
	}
}

// TestOtherFaces pins the face numbers a token product's Number states
// against its pair's: TMH3 sells Copy #1 and Copy #37 beside Eldrazi Angel
// #2, and only the first is the pair the datastore has.
func TestOtherFaces(t *testing.T) {
	b := &matcher.Backend{UUIDs: map[string]*matcher.CardObject{
		"pair":  {Card: matcher.Card{Number: "1 // 2"}},
		"other": {Card: matcher.Card{Number: "2 // 15"}},
	}}
	res := &resolver{backend: b, gameID: cm.GameMagic}
	for _, tt := range []struct {
		number, cardID string
		want           bool
	}{
		{"CT 1/2", "pair", false},
		{"CT 2/1", "pair", false},
		{"CT 37/2", "pair", true},
		{"T 15/REX02", "other", false},
		{"", "pair", false},
		{"T 1", "pair", false},
	} {
		got := res.otherFaces(&cm.Product{Number: tt.number}, tt.cardID)
		if got != tt.want {
			t.Errorf("otherFaces(%q, %s) = %v, want %v", tt.number, tt.cardID, got, tt.want)
		}
	}
}
