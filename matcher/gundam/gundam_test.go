package gundam

import (
	"cmp"
	"slices"
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestFinishIdentity pins the invariant the per-printing entries exist for:
// one uuid per finish, and no uuid answering for two. A product sold both
// plain and holofoil is two price points, and folding them together would
// file one price under the other's uuid. The flag defaults are not finishes:
// the bare foil flag reaching the holofoil is what they are for.
func TestFinishIdentity(t *testing.T) {
	b := gametest.Backend(t, matcher.GameGundam)

	for uuid, co := range b.UUIDs {
		if co.Sealed {
			continue
		}
		if co.Finish == "" {
			t.Errorf("%s carries no finish", uuid)
			continue
		}
		if got := co.FoilUUIDs[co.Finish]; got != uuid {
			t.Errorf("%s is stored under finish %q, which resolves to %q", uuid, co.Finish, got)
		}
		for finish, other := range co.FoilUUIDs {
			if finish == matcher.FinishNonfoil || finish == matcher.FinishFoil {
				continue
			}
			if finish != co.Finish && other == uuid {
				t.Errorf("%s answers for both %q and %q", uuid, co.Finish, finish)
			}
		}
	}
}

// TestPlainNumberIsPlain pins the contract every game keeps: PlainNumber
// is Number with the game's decorations stripped, never anything wider. The
// website's "cn:" search filters on it and "cns:" on Number.
func TestPlainNumberIsPlain(t *testing.T) {
	b := gametest.Backend(t, matcher.GameGundam)

	for uuid, co := range b.UUIDs {
		if co.Sealed || co.Number == "" {
			continue
		}
		if len(co.PlainNumber) > len(co.Number) {
			t.Errorf("%s: PlainNumber %q is wider than Number %q", uuid, co.PlainNumber, co.Number)
		}
	}
}

// TestAFinishThisGameDoesNotSellIsStillRefused is the other half: opening
// the vocabulary moved the refusal, it did not remove it.
func TestAFinishThisGameDoesNotSellIsStillRefused(t *testing.T) {
	b := gametest.Backend(t, matcher.GameGundam)
	var checked int
	for uuid, co := range b.UUIDs {
		if co.Sealed {
			continue
		}
		for _, name := range []string{"Cold Foil", "Rainbow Pillars", "Prismatic Foil"} {
			if got, err := b.MatchIDFinish(uuid, name); err == nil {
				t.Fatalf("MatchIDFinish(%s, %q) = %q with no error", uuid, name, got)
			}
		}
		checked++
		if checked > 200 {
			break
		}
	}
	if checked == 0 {
		t.Fatal("no printing was checked")
	}
	t.Logf("%d printings, each refusing all three", checked)
}

// TestRarityTellsParallelsApart pins what identifies a printing in this
// game. 439 of its collector numbers are carried by several products under
// the same name with no variant label between them, and the only thing
// that differs is the rarity: the parallel run suffixes it with "+".
// A wording that names neither has to reach the plain run, since that is
// what a listing without a qualifier means.
func TestRarityTellsParallelsApart(t *testing.T) {
	b := gametest.Backend(t, matcher.GameGundam)

	for _, tt := range []struct {
		desc       string
		in         matcher.InputCard
		wantRarity string
	}{
		{
			desc:       "a bare listing reaches the plain run",
			in:         matcher.InputCard{Name: "Gundam", Edition: "Edition Beta", Variation: "ST01-001"},
			wantRarity: "Legend Rare",
		},
		{
			desc:       "the parallel is reached by its own rarity code",
			in:         matcher.InputCard{Name: "Gundam", Edition: "Edition Beta", Variation: "ST01-001 LR+"},
			wantRarity: "LR+",
		},
		{
			desc:       "the plain run is not reached by the parallel's code",
			in:         matcher.InputCard{Name: "Gundam", Edition: "Edition Beta", Variation: "ST01-001 Legend Rare"},
			wantRarity: "Legend Rare",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			in := tt.in
			id, err := b.Match(&in)
			if err != nil {
				t.Fatalf("Match(%v) = %v", tt.in, err)
			}
			co, err := b.GetUUID(id)
			if err != nil {
				t.Fatalf("GetUUID(%s) = %v", id, err)
			}
			rarity := b.RarityLabel(co.Rarity)
			if rarity != tt.wantRarity {
				t.Errorf("Match(%v) = %s (%s), want rarity %s", tt.in, co.UUID, rarity, tt.wantRarity)
			}
		})
	}
}

// probeWords is what TestNumberReachesItsPrinting names a printing by.
func probeWords(co *matcher.CardObject) string {
	return strings.Join(append([]string{co.Name, co.SetCode, co.Number, co.Rarity}, co.PromoTypes...), "|")
}

// doubleListed is every probe two different products answer to: a card
// TCGplayer lists twice, which the datastore publishes rather than refuse
// the whole game over.
func doubleListed(b *matcher.Backend) map[string]bool {
	products := map[string]map[string]bool{}
	for uuid, co := range b.UUIDs {
		if co.Sealed || co.Number == "" {
			continue
		}
		// A product's finishes fold onto one printing, as their shared map names it.
		product := cmp.Or(co.FoilUUIDs[matcher.FinishNonfoil], co.FoilUUIDs[matcher.FinishFoil], uuid)
		words := probeWords(co)
		if products[words] == nil {
			products[words] = map[string]bool{}
		}
		products[words][product] = true
	}
	twice := map[string]bool{}
	for words, of := range products {
		if len(of) > 1 {
			twice[words] = true
		}
	}
	return twice
}

// TestNumberReachesItsPrinting replays every printing under its own name,
// edition and collector number. Where the number is shared the rarity is
// added, which is the whole of what tells those printings apart; anything
// that fails to come back is a printing no storefront writing the catalog's
// own words could reach.
func TestNumberReachesItsPrinting(t *testing.T) {
	b := gametest.Backend(t, matcher.GameGundam)

	var probes, hits int
	misses := map[string]int{}
	listedTwice := doubleListed(b)
	for _, co := range b.UUIDs {
		if co.Sealed || co.Number == "" {
			continue
		}
		// Two products alike in every word probed are a card TCGplayer
		// lists twice; no wording can name one of them, so neither is asked.
		if listedTwice[probeWords(co)] {
			continue
		}
		set := b.Sets[co.SetCode]
		if set == nil {
			continue
		}
		variation := co.Number
		if co.Rarity != "" {
			variation += " " + b.RarityLabel(co.Rarity)
		}
		// The promo set reprints a card once per event it was handed out
		// at, every one of them at the main set's number and rarity, so the
		// event is part of what names the printing, and so is the part of
		// a multi-part token it is, which the builder keeps among them for
		// that reason.
		for _, promoType := range co.PromoTypes {
			variation += " " + b.PromoTypeLabels[promoType]
		}
		in := matcher.InputCard{Name: co.Name, Edition: set.Name, Variation: variation}
		probes++
		id, err := b.Match(&in)
		if err != nil {
			misses[err.Error()]++
			continue
		}
		got, err := b.GetUUID(id)
		if err != nil {
			misses["GetUUID: "+err.Error()]++
			continue
		}
		// A product sold in two finishes answers with whichever the flags
		// resolve to, so the printing is what has to match, not the uuid.
		if got.SetCode == co.SetCode && got.Number == co.Number && got.Rarity == co.Rarity &&
			slices.Equal(got.PromoTypes, co.PromoTypes) {
			hits++
			continue
		}
		misses["wrong printing"]++
	}
	if probes == 0 {
		t.Fatal("no printing to probe")
	}
	t.Logf("%d of %d printings answered (%.1f%%)", hits, probes, 100*float64(hits)/float64(probes))
	for reason, n := range misses {
		t.Logf("   %5d %s", n, reason)
	}
	if hits != probes {
		t.Errorf("%d printings did not answer to their own number and rarity", probes-hits)
	}
}

// TestSealedIsNotACard pins that the sealed products stay in the sealed
// namespace: they share the name buckets, and a sealed product answering a
// card query would price a box as a single.
func TestSealedIsNotACard(t *testing.T) {
	b := gametest.Backend(t, matcher.GameGundam)

	var sealed int
	for _, co := range b.UUIDs {
		if !co.Sealed {
			continue
		}
		sealed++
		if co.Name == "" {
			t.Errorf("%s is sealed but unnamed", co.UUID)
			continue
		}
		in := matcher.InputCard{Name: co.Name}
		id, err := b.Match(&in)
		if err != nil {
			continue
		}
		if answered := b.UUIDs[id]; answered != nil && answered.Sealed {
			t.Errorf("Match(%q) answered the sealed product %s", co.Name, id)
		}
	}
	if sealed == 0 {
		t.Error("the datastore carries no sealed product")
	}
}
