package cardgamecorner

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/mtgban"
)

// fakeStorefront answers the buylist endpoints from canned bodies, and the
// search with searchStatus when one is set.
type fakeStorefront struct {
	editions     string
	search       string
	searchStatus int
}

func (f fakeStorefront) RoundTrip(req *http.Request) (*http.Response, error) {
	body := f.search
	status := cmp.Or(f.searchStatus, http.StatusOK)
	if req.Method == http.MethodGet {
		body = f.editions
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

// TestBuylistNumber pins the number a buylist listing states outright. The
// store publishes one name for a card and prices its treatments separately,
// so the name alone puts every one of them on the plain printing: the three
// Aang listings below are bought at three prices and answered with one card.
func TestBuylistNumber(t *testing.T) {
	b := realDatastore(t)

	const (
		aang    = "Aang, at the Crossroads // Aang, Destined Savior"
		edition = "Avatar: The Last Airbender"
	)
	tests := []struct {
		desc    string
		name    string
		edition string
		id      string
		number  int
		wantSet string
		wantNum string
	}{
		{"the plain printing", aang, edition, "TLA0203", 203, "TLA", "203"},
		{"the borderless one beside it", aang, edition, "TLA0304", 304, "TLA", "304"},
		{"and the third treatment", aang, edition, "TLA0346", 346, "TLA", "346"},
		{"a numbered DCI Promo another set also numbers", "Living End", "DCI Promo", "DCI0030", 30, "SLP", "30"},
		{"and one no Secret Lair Promo holds", "Thing in the Ice / Awoken Horror", "DCI Promo", "DCI0003", 3, "PR23", "3"},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			card, err := preprocessBL(b, test.name, test.edition, test.id, test.number)
			if err != nil {
				t.Fatal(err)
			}
			id, err := b.Match(card)
			if err != nil {
				t.Fatalf("Match(%q) = %v", card, err)
			}
			co, err := b.GetUUID(id)
			if err != nil {
				t.Fatal(err)
			}
			if co.SetCode != test.wantSet || co.Number != test.wantNum {
				t.Errorf("Match(%q) = %s|%s, want %s|%s", card, co.SetCode, co.Number, test.wantSet, test.wantNum)
			}
		})
	}
}

// TestBuylistNameVersion pins that a version written on the card name, as the
// store does for a promo pack printing, reaches the same mapping one written
// on the edition does.
func TestBuylistNameVersion(t *testing.T) {
	b := realDatastore(t)

	card, err := preprocessBL(b, "Sublime Epiphany (V.2)", "Core 2021: Promos", "", 74)
	if err != nil {
		t.Fatal(err)
	}
	id, err := b.Match(card)
	if err != nil {
		t.Fatalf("Match(%q) = %v", card, err)
	}
	co, err := b.GetUUID(id)
	if err != nil {
		t.Fatal(err)
	}
	if co.SetCode != "PM21" || co.Number != "74p" {
		t.Errorf("Match(%q) = %s|%s, want PM21|74p", card, co.SetCode, co.Number)
	}
}

// TestScrapeBuylistFailure pins that a search the storefront reports as
// failed, or an empty one for every edition, is an error rather than a
// buylist with the cards missing.
func TestScrapeBuylistFailure(t *testing.T) {
	const editions = `{"Expansions":[{"Id":1,"Espansione":"Alpha","Enabled":true},{"Id":2,"Espansione":"Beta","Enabled":true}]}`
	const completed = `{"Result":{"Products":[],"Total":0},"Status":5,"IsCompleted":true}`
	tests := []struct {
		desc        string
		search      string
		status      int
		searchFails bool
	}{
		{"faulted", `{"Result":{"Products":[],"Total":0},"Status":7,"IsCompleted":true,"IsFaulted":true}`, 0, true},
		{"canceled", `{"Result":{"Products":[],"Total":0},"Status":6,"IsCompleted":true,"IsCanceled":true}`, 0, true},
		{"server error", completed, http.StatusInternalServerError, true},
		{"nothing bought anywhere", completed, 0, false},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			cgc := &Cardgamecorner{
				buylist: mtgban.BuylistRecord{},
				client: &Client{client: &http.Client{
					Transport: fakeStorefront{editions: editions, search: test.search, searchStatus: test.status},
				}},
			}
			_, err := cgc.client.GetBuylistForEdition(context.Background(), 1, 1)
			if (err != nil) != test.searchFails {
				t.Errorf("GetBuylistForEdition error = %v, want failure %t", err, test.searchFails)
			}

			err = cgc.scrapeBL(context.Background())
			if err == nil {
				t.Fatalf("scrapeBL answered no error with %d entries", len(cgc.buylist))
			}
			// A failed search has to be reported as one, not as an
			// empty buylist
			failed := strings.Contains(err.Error(), "editions failed to read")
			if failed != test.searchFails {
				t.Errorf("scrapeBL error = %v, want failed search %t", err, test.searchFails)
			}
			if !cgc.buylistDate.IsZero() {
				t.Errorf("buylist dated despite %v", err)
			}
		})
	}
}
