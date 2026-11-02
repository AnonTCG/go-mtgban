package yugioh

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestPrintRunUUIDs walks every printing and asks each print run it carries
// for the uuid pricing it. Each has to answer with the entry stamped with
// that run, no two runs of a printing and no two printings may answer with
// one uuid, and every stored entry has to be some printing's run - a run
// silently overwritten in FoilUUIDs would leave a uuid nothing can reach.
// The shared names stay refused throughout: they are the flag slots aimed
// at the default run, and answering one with a run would hand a bare foil
// flag a printing nobody asked for.
func TestPrintRunUUIDs(t *testing.T) {
	b := gametest.Backend(t, matcher.GameYuGiOh)

	owner := map[string]string{}
	var printings, runs int
	for _, code := range b.AllSets {
		for _, card := range b.Sets[code].Cards {
			printings++

			runOf := map[string]string{}
			for key, target := range card.FoilUUIDs {
				if key == matcher.FinishNonfoil || key == matcher.FinishFoil {
					continue
				}
				if other, found := runOf[target]; found {
					t.Errorf("%s: runs %q and %q share uuid %s", card.UUID, other, key, target)
				}
				runOf[target] = key
				if prev, found := owner[target]; found && prev != card.UUID {
					t.Errorf("uuid %s answers for printings %s and %s", target, prev, card.UUID)
				}
				owner[target] = card.UUID

				co, err := b.GetUUID(target)
				if err != nil {
					t.Errorf("%s: run %q names unknown uuid %s", card.UUID, key, target)
					continue
				}
				if co.Finish != key {
					t.Errorf("%s: run %q names uuid %s carrying finish %q",
						card.UUID, key, target, co.Finish)
				}
				got, err := b.MatchIDFinish(card.UUID, key)
				if err != nil || got != target {
					t.Errorf("MatchIDFinish(%s, %q) = (%q, %v), want %q",
						card.UUID, key, got, err, target)
				}
				runs++
			}

			// The flag slots are a compatibility layer, never a run of
			// their own: they have to land on a run the printing carries.
			for _, slot := range []string{matcher.FinishNonfoil, matcher.FinishFoil} {
				target := card.FoilUUIDs[slot]
				if target == "" {
					t.Errorf("%s: flag slot %q names no run", card.UUID, slot)
					continue
				}
				if _, found := runOf[target]; !found {
					t.Errorf("%s: flag slot %q names uuid %s that no run carries",
						card.UUID, slot, target)
				}
			}

			// The shared names name no run and answer as the flags do
			for _, shared := range [][2]string{
				{"Nonfoil", matcher.FinishNonfoil},
				{"Normal", matcher.FinishNonfoil},
				{"Foil", matcher.FinishFoil},
			} {
				got, err := b.MatchIDFinish(card.UUID, shared[0])
				if want := card.FoilUUIDs[shared[1]]; err != nil || got != want {
					t.Errorf("MatchIDFinish(%s, %q) = (%q, %v), want the %s slot's %q",
						card.UUID, shared[0], got, err, shared[1], want)
				}
			}
		}
	}

	var orphans int
	for uuid, co := range b.UUIDs {
		if co.Sealed {
			continue
		}
		if _, found := owner[uuid]; !found {
			orphans++
			t.Errorf("uuid %s carries finish %q but no printing names it", uuid, co.Finish)
		}
	}
	if printings == 0 || runs == 0 {
		t.Fatalf("datastore carries no print runs: %d printings, %d runs", printings, runs)
	}
	t.Logf("%d printings, %d print runs, %d orphaned entries", printings, runs, orphans)
}
