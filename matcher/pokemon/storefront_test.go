package pokemon

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestPromoPseudoExpansionAlias pins the storefront promo pseudo-expansions
// resolving to the set the catalog files those printings in. None of the
// names shares a tail with its set's, so without the table the edition
// narrows nothing and every printing of the name stands.
func TestPromoPseudoExpansionAlias(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	for _, tt := range []struct {
		desc string
		in   matcher.InputCard
		want string
	}{
		{"wizards black star names the wotc promos", matcher.InputCard{
			Name: "Mewtwo", Edition: "Wizards Black Star Promos", Variation: "014 14"},
			"14-53_87417"},
		{"mcdonald's collection names its promo set", matcher.InputCard{
			Name: "Froakie", Edition: "McDonald's Collection 25th Anniversary", Variation: "022h Holo Promo | 022/025"},
			"022-025_232336_holofoil"},
		// The SV promos are in neither table: the edition stays
		// unresolved and the name and number find the printing. Every
		// number the set files twice, all 65, pairs a printing with a
		// labelled sibling - a Pokemon Center exclusive, a prerelease
		// staff stamp - so a listing naming no label reaches the plain one.
		{"sv black star reaches the plain printing of a pair", matcher.InputCard{
			Name: "Charmander", Edition: "SV Black Star Promos", Variation: "044 SVP 044"},
			"044_512035_holofoil"},
		{"and the prerelease one rather than its staff stamp", matcher.InputCard{
			Name: "Quaquaval", Edition: "SV Black Star Promos", Variation: "005 SVP 005"},
			"005_487751_holofoil"},
		// The prize pack series is pooled rather than aliased. Its
		// siblings carry the treatment labels - every one of the 130
		// numbers the set files more than one product at is told apart
		// by a label - so restricting to the set flips no coin.
		// Replayed the same way over the whole set, the pool answers
		// 937 right against 2 wrong where the unrestricted name
		// manages 191 against 4, and the 4 it drops are printings in
		// other sets entirely: a prize pack Zekrom ex priced as the
		// Black Bolt one.
		{"prize pack series reaches its own set", matcher.InputCard{
			Name: "Grotle", Edition: "Play! Pokémon Prize Pack Series", Variation: "007 BRS 007"},
			"007-172_489316"},
		{"w promos reach the w-stamped printing", matcher.InputCard{
			Name: "Misty's Psyduck", Edition: "W Promos", Variation: "054 W Promo | 54/132"},
			"054-132_166296"},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			in := tt.in
			id, err := b.Match(&in)
			if err != nil {
				t.Fatalf("Match(%v) = %v", tt.in, err)
			}
			if id != tt.want {
				t.Errorf("Match(%v) = %s (%v), want %s", tt.in, id, b.UUIDs[id], tt.want)
			}
		})
	}
}

// TestPooledBlisterDeckHeading pins the one storefront name spanning two
// sets: the candidates narrow to the Blister Exclusives / Deck Exclusives
// pair, the collector number picks within it, and a card the pool does not
// carry refuses rather than falling through to a same-numbered printing in
// some other set.
func TestPooledBlisterDeckHeading(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	for _, tt := range []struct {
		desc string
		in   matcher.InputCard
		want string
	}{
		{"a build and battle kit promo lands in deck exclusives", matcher.InputCard{
			Name: "Chi-Yu", Edition: "Theme Deck & Blisters Exclusives", Variation: "029h Holo Build & Battle Kit | 029/182"},
			"029-182_528309"},
		{"the version's treatment picks the plain half of the pool", matcher.InputCard{
			Name: "Zacian", Edition: "Theme Deck & Blisters Exclusives", Variation: "045 Non-Holo | 045/094"},
			"045-094_664005"},
		{"the cosmos holo of an ex box lands with the miscellaneous cards", matcher.InputCard{
			Name: "Maschiff", Edition: "Theme Deck & Blisters Exclusives", Variation: "146 Cosmos Holo | 142/193"},
			"142-193_544444_holofoil"},
		{"a card the pool does not carry refuses", matcher.InputCard{
			Name: "Chansey", Edition: "Theme Deck & Blisters Exclusives", Variation: "003 Cosmos Holo | 003/102"},
			""},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			in := tt.in
			id, err := b.Match(&in)
			if tt.want == "" {
				if err == nil {
					t.Fatalf("Match(%v) = %s (%v), want an error", tt.in, id, b.UUIDs[id])
				}
				return
			}
			if err != nil {
				t.Fatalf("Match(%v) = %v", tt.in, err)
			}
			if id != tt.want {
				t.Errorf("Match(%v) = %s (%v), want %s", tt.in, id, b.UUIDs[id], tt.want)
			}
		})
	}
}

// TestVersionWording pins what the ride-along Version buys and what its
// guards refuse: an agreeing set total outranks a bare verbatim number, a
// contradicting total vetoes the candidate, a wording spelling both
// treatments stays ambiguous, and the stamp, sequin and cosmos wordings
// never land on a plain printing.
func TestVersionWording(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	for _, tt := range []struct {
		desc string
		in   matcher.InputCard
		want string
	}{
		{"an agreeing total beats the bare promo number", matcher.InputCard{
			Name: "Venusaur", Edition: "Supreme Victors Promos", Variation: "013/147 Non-Holo Theme Deck | 13/147"},
			"013-147_125073"},
		{"a wording spelling both treatments stays ambiguous", matcher.InputCard{
			Name: "Counter Catcher", Edition: "Play! Pokémon Prize Pack Series", Variation: "160 Non-Holo / Cosmos Holo | PAR 160"},
			""},
		{"a stamp wording refuses the unstamped printing", matcher.InputCard{
			Name: "Rapidash", Edition: "Wizards Black Star Promos", Variation: "051pc Pokemon Center Stamped | 51"},
			""},
		{"a stamp wording still reaches a stamped sibling", matcher.InputCard{
			Name: "Buddy-Buddy Poffin", Edition: "League Promos", Variation: "144staff STAFF | Reverse Holo 144",
			Finish: "Reverse Holofoil", Foil: true},
			"144-162_638139_reverseholofoil"},
		{"a sequin wording reaches the general mills printing", matcher.InputCard{
			Name: "Jangmo-o", Edition: "SM Black Star Promos", Variation: "SM40sq Sequin Holo Promo | SM40"},
			"sm40_161466_holofoil"},
		{"a cosmos wording refuses a printing that cannot be one", matcher.InputCard{
			Name: "Fezandipiti", Edition: "Theme Deck & Blisters Exclusives", Variation: "096 Cosmos Holo | 096/167"},
			""},
		{"a holo wording picks the product sold only as holofoil", matcher.InputCard{
			Name: "Blaziken", Edition: "EX Battle Stadium", Variation: "3 Holo"},
			"3_125113_holofoil"},
		{"a non-holo wording picks the product sold as nonfoil", matcher.InputCard{
			Name: "Blaziken", Edition: "EX Battle Stadium", Variation: "3 Non-Holo"},
			"3_125112"},
		{"a cosmos wording keeps a printing selling the holo", matcher.InputCard{
			Name: "Eevee", Edition: "Wizards Black Star Promos", Variation: "011 Cosmos Holo 11"},
			"11-53_85074_holofoil"},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			in := tt.in
			id, err := b.Match(&in)
			if tt.want == "" {
				if err == nil {
					t.Fatalf("Match(%v) = %s (%v), want an error", tt.in, id, b.UUIDs[id])
				}
				return
			}
			if err != nil {
				t.Fatalf("Match(%v) = %v", tt.in, err)
			}
			if id != tt.want {
				t.Errorf("Match(%v) = %s (%v), want %s", tt.in, id, b.UUIDs[id], tt.want)
			}
		})
	}
}

// TestTotalDisagrees pins the veto's edges: only two spelled-out totals can
// contradict, an agreeing field clears a decorated one, and a wording with
// no total for the numerator vetoes nothing.
func TestTotalDisagrees(t *testing.T) {
	for _, tt := range []struct {
		variation, number string
		want              bool
	}{
		{"Non-Holo | 053/167", "053/094", true},
		{"Holo Promo | 013/025", "013/025", false},
		// The blueprint's own decorated field disagrees, the version's
		// clean one agrees, and agreement wins.
		{"02/17h Holo Promo | 2/17", "002/017", false},
		// No slashed field for the numerator: nothing to compare.
		{"144staff STAFF | Reverse Holo 144", "144/162", false},
		// A card number with no total cannot be contradicted.
		{"Non-Holo | 053/167", "053", false},
	} {
		if got := totalDisagrees(tt.variation, tt.number); got != tt.want {
			t.Errorf("totalDisagrees(%q, %q) = %v, want %v", tt.variation, tt.number, got, tt.want)
		}
	}
}

// TestStorefrontNameTails pins the decorations cardtrader writes into the
// name itself coming off - and the real catalog names wearing the same
// shapes staying whole.
func TestStorefrontNameTails(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	for _, tt := range []struct {
		desc string
		in   matcher.InputCard
		want string
	}{
		{"a numeric level strips", matcher.InputCard{
			Name: "Moltres Lv.33", Edition: "Wizards Black Star Promos", Variation: "021 21"},
			"21-53_87557"},
		{"the delta species tail strips", matcher.InputCard{
			Name: "Deoxys δ Delta Species", Edition: "POP Series 4", Variation: "02/17h Holo Promo | 2/17"},
			"002-017_84765_holofoil"},
		{"a bracketed letter strips", matcher.InputCard{
			Name: "Unown [J]", Edition: "Wizards Black Star Promos", Variation: "038 38"},
			"38-53_90215"},
		// Named by the set rather than by the storefront's prize pack
		// wording, which is unaliased on purpose and refuses: what is
		// under test is the name, and an edition that cannot narrow
		// leaves the case resting on there being one candidate. There
		// were two the day the catalog filed this card in Deck
		// Exclusives as well.
		{"a dash subtitle respells as the bracketed supporter", matcher.InputCard{
			Name: "Boss's Orders - Cyrus", Edition: "Prize Pack Series Cards", Variation: "132 132/172"},
			"132-172_515539"},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			in := tt.in
			id, err := b.Match(&in)
			if err != nil {
				t.Fatalf("Match(%v) = %v", tt.in, err)
			}
			if id != tt.want {
				t.Errorf("Match(%v) = %s (%v), want %s", tt.in, id, b.UUIDs[id], tt.want)
			}
		})
	}

	// The catalog's own LV.X names wear the level shape with an X, and the
	// strip must never take them apart.
	in := matcher.InputCard{Name: "Heatran LV.X", Variation: "97"}
	Rules{}.Prefilter(b, &in)
	if in.Name != "Heatran LV.X" {
		t.Errorf("Prefilter rewrote %q to %q", "Heatran LV.X", in.Name)
	}
}

// TestUnresolvedEditionDoesNotWiden pins the widening gate: a Japanese
// catalog's set name resolves to nothing, and with no edition to gate on
// the qualified-name widening must not walk the listing onto an English
// printing that happens to carry the number.
func TestUnresolvedEditionDoesNotWiden(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	in := matcher.InputCard{
		Name: "Moltres ex", Edition: "Plasma Storm Promos", Variation: "014/135 Holo Promo | 14/135"}
	if id, err := b.Match(&in); err == nil {
		t.Fatalf("Match(%v) = %s (%v), want an error", in, id, b.UUIDs[id])
	}

	// A resolved edition still widens onto the qualified spelling.
	in = matcher.InputCard{
		Name: "Cherrim", Edition: "SWSH Black Star Promos", Variation: "088 Prerelease Promo | SWSH088"}
	id, err := b.Match(&in)
	if err != nil {
		t.Fatalf("Match(%v) = %v", in, err)
	}
	if id != "swsh088_234276_holofoil" {
		t.Errorf("Match(%v) = %s (%v), want swsh088_234276_holo", in, id, b.UUIDs[id])
	}
}

// TestExtractNumbersSkipsYears pins that a bare year in the wording is not
// read as a collector number: the league energies are unnumbered and
// labelled by year, and "2006 Non-Holo Promo" names the label.
func TestExtractNumbersSkipsYears(t *testing.T) {
	for _, tt := range []struct {
		wording string
		want    []string
	}{
		{"FFE-9JT-SUX | 2006 Non-Holo Promo", nil},
		{"2006 Unnumbered", nil},
		{"Cosmos Holo | 024/167", []string{"024"}},
		{"SVP 205", []string{"205"}},
	} {
		got := extractNumbers(tt.wording)
		if len(got) != len(tt.want) {
			t.Errorf("extractNumbers(%q) = %v, want %v", tt.wording, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("extractNumbers(%q) = %v, want %v", tt.wording, got, tt.want)
			}
		}
	}
}
