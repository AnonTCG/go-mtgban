package coolstuffinc

import (
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// lorcanaSpellings corrects the Lorcana names this storefront misspells.
// Each key is a name no set in the game has and each value is the card it
// means; left as typed they look up nothing at all and the listing goes
// unpriced.
//
// The storefront's own catalog is what says it is a slip rather than a
// name: it files "Basil - Perspective Investigator" at 140/204 of Rise of
// the Floodborn, under its own image sku dl_RotFB_140, and that set's
// number 140 is "Basil - Perceptive Investigator". Everything but the one
// word agrees.
//
// Spelled out rather than found by nearest match, for the reason
// csiSpellings gives - and the more so here, where a name is a character
// and a title joined by a dash and the titles are the whole of what tells
// one printing of a character from another. This set alone sells Basil
// under three of them at consecutive numbers, 138 through 140.
var lorcanaSpellings = map[string]string{
	"Basil - Perspective Investigator": "Basil - Perceptive Investigator",
}

// lorcanaSpelling spells a Lorcana name the way the catalog does, where
// this storefront has typed it wrong.
func lorcanaSpelling(name string) string {
	if spelled, found := lorcanaSpellings[name]; found {
		return spelled
	}
	return name
}

// lorcanaSealedWords spells this storefront's sealed products the way the
// catalog does: its "Two-Player Starter Deck" is the catalog's "2-Player
// Starter Set", and its "Booster Case" a "Booster Box Case".
var lorcanaSealedWords = strings.NewReplacer(
	"Two-Player Starter Deck", "2-Player Starter Set",
	"Booster Case", "Booster Box Case",
)

// lorcanaVariation spells "Rainbow Foil" the way TCGplayer names the printing
// it is, "Holofoil" - a Starter Deck Exclusive's rainbow foil otherwise names
// no finish the matcher knows and lands on the set's ordinary cold foil.
func lorcanaVariation(variation string) string {
	return strings.ReplaceAll(variation, "Rainbow Foil", "Holofoil")
}

// lorcanaQuestNotes names the Illumineer's Quest a prize card's note belongs
// to, and the set code a probe has to land on to be trusted. The storefront
// files the prize on the shelf of the set whose card it repeats, at a number
// that set does not reach, and the frame its note names is the only thing
// saying which quest handed it out.
var lorcanaQuestNotes = []struct{ marker, edition, wantSet string }{
	{"Ink Tentacles Card Frame", "Illumineer's Quest: Deep Trouble", "Q1"},
	{"Shifting Sands", "Illumineer's Quest: Palace Heist", "Q2"},
	{"Ink Vines Card Frame", "Illumineer's Quest: The Great Hunny Rescue", "Q3"},
}

// lorcanaShelf answers the shelf a Lorcana listing belongs to: the quest its
// note names when a probe of the card lands in that quest's set, and the shelf
// it arrived on otherwise. An edition holding no such card widens to every
// printing, so the landed set is what is checked.
func lorcanaShelf(b *matcher.Backend, name, shelf, notes string) string {
	for _, quest := range lorcanaQuestNotes {
		if !strings.Contains(notes, quest.marker) {
			continue
		}
		probe := &matcher.InputCard{Name: lorcanaSpelling(name), Edition: quest.edition, Variation: lorcanaVariation(notes), Foil: true}
		id, err := b.Match(probe)
		if err != nil {
			continue
		}
		co, err := b.GetUUID(id)
		if err == nil && co.SetCode == quest.wantSet {
			return quest.edition
		}
	}
	return shelf
}

// retailLorcana reads an offer's name, shelf and note.
func retailLorcana(csi *Coolstuffinc, l sellListing) (*matcher.InputCard, string, error) {
	theCard := &matcher.InputCard{Name: lorcanaSpelling(l.name), Edition: lorcanaShelf(csi.backend, l.name, l.edition, l.notes), Variation: lorcanaVariation(l.notes), Foil: l.foil}
	cardID, err := l.match(csi.backend, theCard)
	return theCard, cardID, err
}

// buylistLorcana reads a buylist row's note, which starts with the same
// number Number carries and spends the rest on the qualifier; Number alone
// misses a Starter Deck Exclusive.
func buylistLorcana(csi *Coolstuffinc, p CSIPriceEntry) (*matcher.InputCard, string, error) {
	theCard := &matcher.InputCard{Name: lorcanaSpelling(p.Name), Edition: lorcanaShelf(csi.backend, p.Name, p.ItemSet, p.Notes), Variation: lorcanaVariation(p.Notes), Foil: p.IsFoil == 1}
	cardID, err := csi.backend.Match(theCard)
	return theCard, cardID, err
}
