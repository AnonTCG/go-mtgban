package pokemon

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestAdjustEnergyYearRedirect pins the year-to-set redirect for a basic
// energy sold under a shelf that packs it but carries no product of its
// own: Hidden Fates and Champion's Path each hand out one, dated only by
// copyright year, and the catalog filed both under "SM - Team Up" and
// "SWSH01" instead.
func TestAdjustEnergyYearRedirect(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	tests := []struct {
		desc                     string
		name, edition, variation string
		foil                     bool
		wantUUID                 string
	}{
		{
			desc: "Card Trader's Hidden Fates blueprint carries no year at all",
			name: "Grass Energy", edition: "Hidden Fates", foil: false,
			wantUUID: "184429",
		},
		{
			desc: "the same blueprint, foil",
			name: "Grass Energy", edition: "Hidden Fates", foil: true,
			wantUUID: "184429_reverseholofoil",
		},
		{
			desc: "Champion's Path names the year twice, copyright-marked",
			name: "Fire Energy", edition: "Champion's Path", variation: "©2020 Reverse Holo | ©2020", foil: true,
			wantUUID: "208033_reverseholofoil",
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			in := &matcher.InputCard{Name: tt.name, Edition: tt.edition, Variation: tt.variation, Foil: tt.foil}
			id, err := b.Match(in)
			if err != nil {
				t.Fatalf("Match(%+v) = %v", tt, err)
			}
			if id != tt.wantUUID {
				t.Errorf("Match(%+v) = %s, want %s", tt, id, tt.wantUUID)
			}
		})
	}

	t.Run("a shelf that really sells the energy is never redirected", func(t *testing.T) {
		// League & Championship Cards carries a Grass Energy of its own
		// for several years; with no year in the wording, this stays the
		// ambiguity it always was rather than landing on Hidden Fates'
		// redirect target.
		in := &matcher.InputCard{Name: "Grass Energy", Edition: "League Promos"}
		_, err := b.Match(in)
		if err == nil {
			t.Errorf("Match(%+v) = no error, want a refusal", in)
		}
	})
}

// TestTierByYear pins the year tier that disambiguates a league energy once
// its edition and label alone leave several years standing: Card Trader's
// own tracking code for one, "FFE-9JT-SUX", carries no number a card wears,
// and dashedCodeRe has to keep extractNumbers from reading its first group
// as one.
func TestTierByYear(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	tests := []struct {
		desc                     string
		name, edition, variation string
		foil                     bool
		wantUUID                 string
	}{
		{
			desc: "the 2005 non-holo copy, dashed code ahead of the year",
			name: "Fire Energy", edition: "League Promos", variation: "FFE-9JT-SUX | 2005 Non-Holo Promo",
			wantUUID: "178491",
		},
		{
			desc: "the 2006 copy, a different dashed code and no label",
			name: "Water Energy", edition: "League Promos", variation: "LX4-T8B-BH1 | 2006",
			wantUUID: "617930",
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			in := &matcher.InputCard{Name: tt.name, Edition: tt.edition, Variation: tt.variation, Foil: tt.foil}
			id, err := b.Match(in)
			if err != nil {
				t.Fatalf("Match(%+v) = %v", tt, err)
			}
			if id != tt.wantUUID {
				t.Errorf("Match(%+v) = %s, want %s", tt, id, tt.wantUUID)
			}
		})
	}

	// A bare year names a league energy; it must not also settle an
	// unrelated card that merely carries one in its own wording. Roxanne's
	// World Championship reprint names the year the deck was played, and
	// the guard's finish check keeps it off the reverse-holo-only Play!
	// Pokemon printing of the same name and number.
	t.Run("a bare year on an unrelated card picks nothing", func(t *testing.T) {
		in := &matcher.InputCard{Name: "Roxanne", Edition: "World Championship Decks", Variation: "WCD 2022 | Rikuto Ohashi | 150/189"}
		_, err := b.Match(in)
		if err == nil {
			t.Errorf("Match(%+v) = no error, want a refusal", in)
		}
	})
}

// TestExcludeJumbo pins that a stamped promo lands on its ordinary printing
// rather than its Jumbo Cards twin, and that a wording naming neither still
// leaves the ambiguity TestLetteredPromoAmbiguous pins standing for a
// lettered number.
func TestExcludeJumbo(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	in := &matcher.InputCard{
		Name:      "Lucario ex",
		Edition:   "Promo",
		Variation: "Prismatic Evolutions Stamped Version Prismatic Evolutions Stamp 051/131",
		Foil:      true,
	}
	id, err := b.Match(in)
	if err != nil {
		t.Fatalf("Match(%+v) = %v", in, err)
	}
	const want = "051-131_652715_holofoil"
	if id != want {
		t.Errorf("Match(%+v) = %s, want %s", in, id, want)
	}
	co, err := b.GetUUID(id)
	if err != nil {
		t.Fatal(err)
	}
	if co.SetCode == jumboSetCode {
		t.Errorf("landed on the Jumbo twin %s, want the Miscellaneous Cards & Products printing", id)
	}
}

// TestTierByQualifier pins that a qualifier the catalog spells into a
// product name settles a conflict with the numbered card it shares the name
// with: SM01's unnumbered "Psychic Energy (2017)" against the holo secret
// rare Psychic Energy 162.
func TestTierByQualifier(t *testing.T) {
	b := gametest.Backend(t, matcher.GamePokemon)

	in := &matcher.InputCard{Name: "Psychic Energy", Edition: "SM Base Set", Variation: "2017"}
	id, err := b.Match(in)
	if err != nil {
		t.Fatalf("Match(%+v) = %v", in, err)
	}
	co, err := b.GetUUID(id)
	if err != nil {
		t.Fatal(err)
	}
	if co.SetCode != "SM01" || co.Number != "" {
		t.Errorf("Match(%+v) = %s (%s %s), want the unnumbered SM01 energy", in, id, co.SetCode, co.Number)
	}

	// A league energy spells year and program as one qualifier; without
	// it the 2010 and 2011 copies both stood.
	league := &matcher.InputCard{Name: "Grass Energy", Edition: "League & Championship Cards", Variation: "2011 Play! Pokemon"}
	id, err = b.Match(league)
	if err != nil {
		t.Fatalf("Match(%+v) = %v", league, err)
	}
	const wantLeague = "136750_holofoil"
	if id != wantLeague {
		t.Errorf("Match(%+v) = %s, want %s", league, id, wantLeague)
	}

	// A qualifier no row carries narrows nothing, so the conflict stands.
	for _, variation := range []string{"", "1999"} {
		in := &matcher.InputCard{Name: "Psychic Energy", Edition: "SM Base Set", Variation: variation}
		_, err := b.Match(in)
		if err == nil {
			t.Errorf("Match(%+v) = no error, want a refusal", in)
		}
	}
}
