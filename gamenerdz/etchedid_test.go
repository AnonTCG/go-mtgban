package gamenerdz

import (
	"strconv"
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
)

// TestResolveProductEtchedByID pins the etched flag on the id path. The
// retail feed carries the catalog's own TCGplayer id for nearly every
// product, and an etched product carries the etched printing's; asked for
// with the foil flag alone, the matcher hands back the foil sibling. The id
// path answers before the wording path, so it asks for etched as well, and
// does so whenever the id names an etched printing, whatever the display says.
func TestResolveProductEtchedByID(t *testing.T) {
	b := realDatastore(t)
	// An etched printing filed beside its foil on one card is the shape
	// that folds: one sold etched alone has nothing to fold to.
	var etchedUUID, tcgID string
	for _, uuid := range b.GetUUIDs() {
		co, err := b.GetUUID(uuid)
		if err != nil || !co.Etched || !co.HasFinish(matcher.FinishFoil) {
			continue
		}
		if id := co.Identifiers["tcgplayerEtchedProductId"]; id != "" {
			etchedUUID, tcgID = uuid, id
			break
		}
	}
	if etchedUUID == "" {
		t.Fatal("no etched printing carries a TCGplayer id")
	}
	co, _ := b.GetUUID(etchedUUID)

	gn, err := NewScraper(b)
	if err != nil {
		t.Fatal(err)
	}
	id, err := strconv.ParseInt(tcgID, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	product := GNProduct{
		ID:             "etched",
		DisplayName:    co.Name + " (Foil Etched)",
		SelectedFinish: "Foil",
		ProductData:    GNProductData{TCGProductID: id},
	}
	got, err := gn.resolveProduct(modeRetail, product)
	if err != nil {
		t.Fatal(err)
	}
	if got != etchedUUID {
		t.Errorf("resolveProduct(%q, tcg %s) = %s, want the etched printing %s", product.DisplayName, tcgID, got, etchedUUID)
	}

	// A shelf that does not say etched is still told by the id's own printing.
	product.DisplayName = co.Name
	got, err = gn.resolveProduct(modeRetail, product)
	if err != nil {
		t.Fatal(err)
	}
	if got != etchedUUID {
		t.Errorf("resolveProduct(%q, tcg %s) = %s, want the etched printing %s", product.DisplayName, tcgID, got, etchedUUID)
	}
}

// TestResolveProductBuylistByRetailID pins that a buylist product, which
// carries no TCGplayer id, answers with the id its retail product carried,
// and only where that id names the card the display does. The Swarmlord is
// written 40K-004 in both feeds and is the surge foil numbered 4; Game Nerdz
// also carries an id on a Clickslither that TCGplayer files as a Necroblossom
// Snarl.
func TestResolveProductBuylistByRetailID(t *testing.T) {
	gn, err := NewScraper(realDatastore(t))
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(id, display string, tcgID int64) string {
		t.Helper()
		retail := GNProduct{ID: id, DisplayName: display, SelectedFinish: "foil"}
		retail.ProductData.TCGProductID = tcgID
		_, err := gn.resolveProduct(modeRetail, retail)
		if err != nil {
			t.Fatal(err)
		}
		buylist := GNProduct{ID: id, DisplayName: display, SelectedFinish: "foil"}
		got, err := gn.resolveProduct(modeBuylist, buylist)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	got := resolve("swarmlord", "The Swarmlord (40K-004) - Warhammer 40,000 Commander Foil", 285793)
	co, err := gn.backend.GetUUID(got)
	if err != nil || co.SetCode != "40K" || co.Number != "4" {
		t.Errorf("The Swarmlord: got %q (%v); want 40K 4", got, err)
	}

	got = resolve("clickslither", "Clickslither (VMA-156) - Vintage Masters", 237191)
	co, err = gn.backend.GetUUID(got)
	if err == nil && co.Name == "Necroblossom Snarl" {
		t.Errorf("Clickslither landed on %s through an id that names another card", co.Name)
	}
}
