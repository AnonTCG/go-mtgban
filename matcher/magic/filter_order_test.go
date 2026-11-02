package magic

import (
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
)

// A card printed once per World Championship deck leaves Match several
// equally correct answers, and it keeps the first. Unless FilterCards
// returns them in the same order every time, the same listing resolves to
// a different physical card from one call to the next, and every
// inventory diff churns forever.
func TestFilterCardsOrderIsStable(t *testing.T) {
	realDatastore(t)
	if len(testBackend.GetUUIDs()) == 0 {
		t.Fatal("matcher datastore not loaded")
	}

	for _, name := range []string{"City of Brass", "Llanowar Elves", "Wasteland"} {
		first, err := testBackend.Match(&matcher.InputCard{Name: name, Edition: "World Championship Decks"})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for i := range 200 {
			got, err := testBackend.Match(&matcher.InputCard{Name: name, Edition: "World Championship Decks"})
			if err != nil {
				t.Fatalf("%s: call %d: %v", name, i, err)
			}
			if got != first {
				t.Fatalf("%s: call %d returned %s, first call returned %s", name, i, got, first)
			}
		}
	}
}
