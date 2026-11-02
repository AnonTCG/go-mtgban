package lorcana

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/mtgban/go-mtgban/internal/datastore"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestPublishedPrintingIDsWin pins that the datastore's own uuids are the
// ones stored. A uuid is what a price is keyed on: if this package spelled
// one and the builder published another, the two would disagree silently,
// since a uuid nobody stored resolves to nothing rather than erroring.
func TestPublishedPrintingIDsWin(t *testing.T) {
	data := readDatastore(t)

	stamped, want := stampPrintingIDs(t, data)
	b, err := Load(bytes.NewReader(stamped))
	if err != nil {
		t.Fatal(err)
	}
	if len(want) == 0 {
		t.Fatal("no printing was stamped, so nothing is being tested")
	}
	for uuid := range want {
		if _, err := b.GetUUID(uuid); err != nil {
			t.Fatalf("published uuid %s was not stored: %v", uuid, err)
		}
	}
	t.Logf("%d published uuids honoured", len(want))

	// The uuids are the datastore's to spell, and a card sold in two
	// finishes is still one card to Match: folding its finishes by the
	// shape of their uuids aliased every such card the moment the builder
	// spelled them another way.
	var twoFinish *matcher.CardObject
	for _, uuid := range b.AllUUIDs {
		co := b.UUIDs[uuid]
		if co.Sealed || len(co.Finishes) < 2 {
			continue
		}
		twoFinish = co
		break
	}
	if twoFinish == nil {
		t.Fatal("no card is sold in two finishes, so the fold is not being tested")
	}
	in := matcher.InputCard{Name: twoFinish.Name, Edition: twoFinish.Edition, Variation: twoFinish.Number}
	got, err := b.Match(&in)
	if err != nil {
		t.Fatalf("Match(%v) = %v, want the card's plain printing", in, err)
	}
	if got != twoFinish.FoilUUIDs[matcher.FinishNonfoil] {
		t.Errorf("Match(%v) = %s, want %s", in, got, twoFinish.FoilUUIDs[matcher.FinishNonfoil])
	}
}

// readDatastore hands a test the datastore's own bytes, from wherever
// LORCANA_PATH points - a file, a URL, or the bucket the production
// datastores are published to. These tests stamp the document and read it
// back, so they need the file gametest.Backend would have decoded rather than
// the backend it answers with.
func readDatastore(t *testing.T) []byte {
	t.Helper()
	path := os.Getenv("LORCANA_PATH")
	if path == "" {
		t.Skip("LORCANA_PATH not set; skipping Lorcana matcher suite")
	}
	reader, err := datastore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// stampPrintingIDs renames the uuid of every printing a card carries to one
// no spelling could arrive at, and answers the uuids it wrote.
func stampPrintingIDs(t *testing.T, data []byte) ([]byte, map[string]bool) {
	t.Helper()
	want := map[string]bool{}
	out := restamp(t, data, func(id int, finish string) string {
		uuid := fmt.Sprintf("published-%d-%s", id, finish)
		want[uuid] = true
		return uuid
	})
	return out, want
}

// restamp rewrites the uuid of every printing a card's printings[] carries,
// naming each through name.
func restamp(t *testing.T, data []byte, name func(id int, finish string) string) []byte {
	t.Helper()
	payload, err := datastore.Payload(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		t.Fatal(err)
	}
	rows, ok := doc["cards"].([]any)
	if !ok {
		t.Fatal("the payload holds no cards")
	}
	for _, item := range rows {
		row, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("a card is %T, not an object", item)
		}
		id, ok := row["id"].(float64)
		if !ok {
			t.Fatalf("a card id is %T, not a number", row["id"])
		}
		printings, listed := row["printings"].([]any)
		if !listed {
			continue
		}
		for _, raw := range printings {
			printing, ok := raw.(map[string]any)
			if !ok {
				t.Fatalf("a printing is %T, not an object", raw)
			}
			finish, ok := printing["finish"].(string)
			if !ok || finish == "" {
				t.Fatalf("a printing of card %d names no finish", int(id))
			}
			printing["id"] = name(int(id), finish)
		}
	}
	doc["cards"] = rows
	out, err := json.Marshal(map[string]any{"data": doc})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestPrintingIDsNamedTheVendorsWay pins the crossing between the two
// vocabularies. The builder names a finish the way TCGplayer prices it
// ("Normal", "Cold Foil", "Holofoil"); this package spells finishes its own
// way. Read the keys without crossing and every one of them files a finish
// nothing ever asks for, stranding the uuid it names - silently, since a
// uuid nobody stored resolves to nothing rather than erroring.
func TestPrintingIDsNamedTheVendorsWay(t *testing.T) {
	data := readDatastore(t)

	stamped, want := stampVendorPrintingIDs(t, data)
	b, err := Load(bytes.NewReader(stamped))
	if err != nil {
		t.Fatal(err)
	}
	if len(want) == 0 {
		t.Fatal("no printing was stamped, so nothing is being tested")
	}
	for uuid, finish := range want {
		co, err := b.GetUUID(uuid)
		if err != nil {
			t.Fatalf("uuid %s published under a TCGplayer name was not stored: %v", uuid, err)
		}
		if co.Finish != finish {
			t.Errorf("uuid %s carries finish %q, want %q", uuid, co.Finish, finish)
		}
	}
	t.Logf("%d uuids keyed by TCGplayer's own names reached their finish", len(want))
}

// stampVendorPrintingIDs renames every printing's uuid to one naming the
// finish it should be stored under, keyed the way the builder keys it - by
// the TCGplayer printing name - and answers each against that finish.
func stampVendorPrintingIDs(t *testing.T, data []byte) ([]byte, map[string]string) {
	t.Helper()
	want := map[string]string{}
	out := restamp(t, data, func(id int, printing string) string {
		finish := matcher.FinishSlug(printing)
		// One finish named twice is one printing, stored once.
		uuid := fmt.Sprintf("vendor-%d-%s", id, finish)
		want[uuid] = finish
		return uuid
	})
	return out, want
}
