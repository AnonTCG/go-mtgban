package cardgamecorner

import (
	"os"
	"sync"
	"testing"

	"github.com/mtgban/go-mtgban/internal/datastore"
	"github.com/mtgban/go-mtgban/matcher"

	_ "github.com/mtgban/go-mtgban/matcher/magic"
)

// TestMain loads the datastore when one is configured. The rest of this
// package's tests read no cards, so a checkout without it still runs them;
// the promo-edition test asks the datastore whether a name is a set, and
// says so when it cannot.
var (
	datastoreOnce    sync.Once
	datastoreErr     error
	datastoreBackend *matcher.Backend
)

// realDatastore reads the Magic datastore the first time a test asks for it,
// and skips where the run carries none.
func realDatastore(t *testing.T) *matcher.Backend {
	t.Helper()
	datastoreOnce.Do(func() {
		path := os.Getenv("ALLPRINTINGS5_PATH")
		if path == "" {
			return
		}
		backend, err := datastore.Read("magic", path)
		if err != nil {
			datastoreErr = err
			return
		}
		datastoreBackend = backend
	})
	if datastoreErr != nil {
		t.Fatal(datastoreErr)
	}
	if datastoreBackend == nil {
		t.Skip("Need ALLPRINTINGS5_PATH set to run this test")
	}
	return datastoreBackend
}

// TestPromoSetBase pins which editions are read as an expansion's promos.
// The store spells the same thing with and without the colon, so the colon
// cannot decide; what follows from getting this wrong is that every card
// with a promo pack printing anywhere gets stamped as one.
func TestPromoSetBase(t *testing.T) {
	b := realDatastore(t)

	for _, tt := range []struct {
		desc, edition, wantBase string
		wantPromo               bool
	}{
		{"the store's own colon spelling", "Tarkir: Dragonstorm: Promos", "Tarkir: Dragonstorm", true},
		{"and the same edition without it", "Tarkir: Dragonstorm Promos", "Tarkir: Dragonstorm", true},
		{"a core set reads the same way", "Foundations Promos", "Foundations", true},
		{"an edition naming an event names no expansion", "Game Day Promos", "", false},
		{"nor does this one", "Judge Rewards Promos", "", false},
		{"nor this one", "Friday Night Magic Promos", "", false},
		// Normalize drops a standalone "s", so this reaches the promo set
		// spelled "Store Championships" unless the names compare exactly.
		{"an event whose name is a promo set but for one letter", "Store Championship Promos", "", false},
		// Already a promo set: decorating it again sends its cards away.
		{"an edition that is itself a promo set", "San Diego Comic-Con 2013 Promos", "", false},
		{"an edition that is not promos at all", "Tarkir: Dragonstorm", "", false},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			base, _, promo := promoSetBase(b, tt.edition)
			if promo != tt.wantPromo || base != tt.wantBase {
				t.Errorf("promoSetBase(%q) = (%q, %v), want (%q, %v)", tt.edition, base, promo, tt.wantBase, tt.wantPromo)
			}
		})
	}
}

// TestPromoVersionMapping pins which "(V.N)" tag lands on the prerelease
// card and which on the promo pack card - the store numbers them the
// opposite way a first reading suggests, on every one of 27 shelves checked.
func TestPromoVersionMapping(t *testing.T) {
	b := realDatastore(t)

	for _, tt := range []struct {
		version, want string
	}{
		{"V.1", "Prerelease"},
		{"V.2", "Promo Pack"},
	} {
		t.Run(tt.version, func(t *testing.T) {
			_, _, got := internalPreprocess(b, "Ertai Resurrected", "Dominaria United Promos", tt.version, "")
			if got != tt.want {
				t.Errorf("internalPreprocess(..., %q) variation = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

// TestPromoVersionMappingFallsBackToBundle pins the per-card, not
// per-shelf, fallback: Innistrad: Crimson Vow has promo packs for other
// cards (PVOW 5p), just not for Sigarda's Summons.
func TestPromoVersionMappingFallsBackToBundle(t *testing.T) {
	b := realDatastore(t)

	for _, tt := range []struct {
		desc, name, edition, wantEdition string
	}{
		{"Modern Horizons 3", "Powerbalance", "Modern Horizons 3: Promos", "Modern Horizons 3"},
		{"Modern Horizons 2", "Yusri, Fortune's Flame", "Modern Horizons 2: Promos", "Modern Horizons 2"},
		{"Innistrad: Crimson Vow", "Sigarda's Summons", "Innistrad: Crimson Vow: Promos", "Innistrad: Crimson Vow"},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			_, gotEdition, gotVariation := internalPreprocess(b, tt.name, tt.edition, "V.2", "")
			if gotVariation != "Bundle" || gotEdition != tt.wantEdition {
				t.Errorf("internalPreprocess(%q, %q, V.2) = (%q, %q), want (%q, Bundle)",
					tt.name, tt.edition, gotEdition, gotVariation, tt.wantEdition)
			}
		})
	}
}

// TestPromoShelfVersionNeedsPrerelease pins that the Promo shelf's second
// version is read as the prerelease only for a card that has one; claiming a
// prerelease Lightning Bolt names a printing that does not exist.
func TestPromoShelfVersionNeedsPrerelease(t *testing.T) {
	b := realDatastore(t)

	for _, tt := range []struct {
		name, version, want string
	}{
		{"Mizzium Meddler", "Version 2", "Prerelease"},
		{"Lightning Bolt", "Version 2", ""},
		{"Snapcaster Mage", "V.2", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, got := internalPreprocess(b, tt.name, "Promo", tt.version, "")
			if got != tt.want {
				t.Errorf("internalPreprocess(%q, Promo, %q) variation = %q, want %q", tt.name, tt.version, got, tt.want)
			}
		})
	}
}
