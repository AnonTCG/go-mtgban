package riftbound

import (
	"flag"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

var updateRiftbound = flag.Bool("update-riftbound", false,
	"rewrite testdata/riftbound_test_data.json from what Match returns")

// riftboundSeeds pin the contract edges: number affixes (letters, stars,
// prefixes), parenthetical and dashed real names, edition narrowing, and
// error paths. Concrete cards referenced here exist in the real datastore
// (checked against the card gallery at authoring time).
var riftboundSeeds = []gametest.Case{
	{
		Desc: "name and number resolve the base printing",
		In:   matcher.InputCard{Name: "Ahri, Alluring", Variation: "66"},
	},
	{
		Desc: "zero-padded letter variant has its own number",
		In:   matcher.InputCard{Name: "Ahri, Alluring", Variation: "066a"},
	},
	{
		Desc: "foil resolves to its own uuid",
		In:   matcher.InputCard{Name: "Ahri, Alluring", Variation: "66", Foil: true},
	},
	{
		Desc: "full public code works as a number",
		In:   matcher.InputCard{Name: "Ahri, Alluring", Variation: "OGN-066/298"},
	},
	{
		Desc: "starred variant number",
		In:   matcher.InputCard{Name: "Ahri, Inquisitive", Variation: "227*"},
	},
	{
		Desc: "name alone keeps the base art over its lettered sibling",
		In:   matcher.InputCard{Name: "Ahri, Alluring", Edition: "Origins"},
	},
	{
		Desc: "name alone keeps the base art over the numbers past the set",
		In:   matcher.InputCard{Name: "Aphelios, Exalted", Edition: "Spiritforged"},
	},
	{
		Desc: "legend named off a comma instead of a dash",
		In:   matcher.InputCard{Name: "Ahri, Nine-Tailed Fox", Variation: "255 V.1 - Rare", Edition: "Origins"},
	},
	{
		Desc: "legend named off a comma keeps its own showcase number",
		In:   matcher.InputCard{Name: "Ahri, Nine-Tailed Fox", Variation: "303* V.3 - Signed Showcase", Edition: "Origins"},
	},
	{
		// The prefix fallback must not narrow on the finish flag: this
		// printing is only sold foil and the input does not say so.
		Desc: "truncated name of a foil-only printing without the flag",
		In:   matcher.InputCard{Name: "Ahri, Inquis", Variation: "227*"},
	},
	{
		// The other half of that trade. Low numbers repeat across sets:
		// the OGS starter's nonfoil "Annie, Fiery" and the OPP promo's
		// foil "Annie - Fiery" both answer to number 1 and normalize to
		// the same name, so a pass that cannot fall back on the finish
		// sees two names and adopts neither.
		Desc: "truncated name colliding with a promo of the same number",
		In:   matcher.InputCard{Name: "Annie, F", Variation: "1"},
	},
	{
		// Storefronts spell the star as a trailing "s"
		Desc: "signed showcase number resolves to the starred printing",
		In:   matcher.InputCard{Name: "Darius - Hand of Noxus", Variation: "302s"},
	},
	{
		Desc: "unstarred number keeps the plain showcase printing",
		In:   matcher.InputCard{Name: "Darius - Hand of Noxus", Variation: "302"},
	},
	{
		Desc: "special prefixed number",
		In:   matcher.InputCard{Name: "Ahri, Inquisitive", Variation: "SP3"},
	},
	{
		Desc: "token number with zero-padded letter prefix",
		In:   matcher.InputCard{Name: "Gold", Variation: "T05"},
	},
	// Vendetta numbers the plain token and its full-art sibling alike, in a
	// set no promo tier reaches, so the finish is all that separates them.
	// Seeded as CardTrader sends it, front face only, so the prefix fallback
	// is on the path too.
	{
		Desc: "same-numbered main-set siblings answer by the listed finish",
		In:   matcher.InputCard{Name: "Shadow Clone", Variation: "T05", Edition: "Vendetta"},
	},
	{
		Desc: "the foil half of that pair is the full-art printing",
		In:   matcher.InputCard{Name: "Shadow Clone", Variation: "T05", Edition: "Vendetta", Foil: true},
	},
	// The other shape the finish settles: the organized-play promos issue a
	// Best Of foil and a Prize Wall nonfoil under one number, and a bare
	// number describes neither type, so both sit in the same tier.
	{
		Desc: "promo siblings of one number answer by the listed finish",
		In:   matcher.InputCard{Name: "Rumble, Mechanized Menace", Variation: "181", Edition: "Riftbound Organized Play Promotional Cards"},
	},
	{
		Desc: "the foil half of that promo pair is the Best Of printing",
		In:   matcher.InputCard{Name: "Rumble, Mechanized Menace", Variation: "181", Edition: "Riftbound Organized Play Promotional Cards", Foil: true},
	},
	{
		Desc: "parenthetical is part of the real name",
		In:   matcher.InputCard{Name: "Recruit (NX)", Variation: "272"},
	},
	{
		Desc: "parenthetical name resolves without a number",
		In:   matcher.InputCard{Name: "Recruit (DE)"},
	},
	{
		Desc: "dashed name stays intact",
		In:   matcher.InputCard{Name: "Dark Child - Starter"},
	},
	// Legends are exported title-only; storefronts prepend the champion.
	{
		Desc: "champion-prefixed legend resolves to its title",
		In:   matcher.InputCard{Name: "Kai'Sa - Daughter of the Void", Variation: "247"},
	},
	{
		Desc: "champion-prefixed signature legend",
		In:   matcher.InputCard{Name: "Ahri - Nine-Tailed Fox (Signature)", Variation: "303*"},
	},
	{
		Desc: "champion-prefixed overnumbered legend",
		In:   matcher.InputCard{Name: "Kai'Sa - Daughter of the Void", Variation: "299"},
	},
	{
		// The other side of the same line: the promo sets carry the
		// champion-first spelling, so an input that names a promo edition
		// must reach the promo rather than the main-set legend.
		Desc: "champion-first name reaches the promo when the edition says so",
		In:   matcher.InputCard{Name: "Master Yi - Meditative", Variation: "4", Edition: "Promos"},
	},
	{
		Desc: "shortened gallery champion resolves",
		In:   matcher.InputCard{Name: "Master Yi - Meditative", Variation: "4"},
	},
	// CoolStuffInc's "Teemo - Scout" carries no number (see commit message).
	{
		Desc: "a qualified name's own lettered sibling still yields its number",
		In:   matcher.InputCard{Name: "Teemo - Scout", Edition: "Origins", Foil: true},
	},
	{
		Desc: "starter legend resolves through its storefront name",
		In:   matcher.InputCard{Name: "Lux - Lady of Luminosity (Starter)", Variation: "21"},
	},
	// Edition narrowing: the Match skeleton restricts candidates to the sets
	// matching a supplied edition; without one (or with one that resolves to
	// no set name) the number-driven contract is unchanged.
	{
		Desc: "edition narrows a name printed in two sets",
		In:   matcher.InputCard{Name: "Recruit (NX)", Edition: "Vendetta"},
	},
	{
		Desc: "storefront-prefixed edition still narrows",
		In:   matcher.InputCard{Name: "Ahri, Inquisitive", Variation: "227", Edition: "Riftbound: Spiritforged"},
	},
	// A storefront edition the gallery names differently still has to narrow,
	// or the number alone answers with the base-set printing. The pair is
	// seeded together: the same name and number has to reach the promotional
	// set from one edition and the base set from the other.
	{
		Desc: "storefront-named promo edition reaches the promotional set",
		In:   matcher.InputCard{Name: "Yone - Blademaster", Variation: "116", Edition: "Organized Play"},
	},
	{
		Desc: "the same number stays in the base set for its own edition",
		In:   matcher.InputCard{Name: "Yone - Blademaster", Variation: "116", Edition: "Spiritforged"},
	},
	// Promotional printings live in their own promo-typed sets and only
	// match when the edition names one; every other input keeps resolving
	// to the main printings, storefront promo name shapes included.
	{
		Desc: "promo qualifier name resolves inside its promo set",
		In:   matcher.InputCard{Name: "Sett - The Boss (Metal) (Best Of)", Variation: "269", Edition: "Riftbound Organized Play Promotional Cards"},
	},
	{
		Desc: "promo edition disambiguates a number shared across promo sets",
		In:   matcher.InputCard{Name: "Jinx - Rebel", Variation: "202", Edition: "Riftbound Promotional Cards"},
	},
	{
		Desc: "same promo number under the other promo set",
		In:   matcher.InputCard{Name: "Jinx - Rebel", Variation: "202", Edition: "Riftbound Organized Play Promotional Cards"},
	},
	// Storefronts file promos under one heading that names no set, and
	// qualify them more fully than the gallery does. The pairs below are
	// seeded together: the promo heading has to reach the promotional
	// printing, and the set's own name has to keep reaching the main one,
	// since both carry the number.
	{
		Desc: "a promo heading naming no set still reaches the promo printing",
		In:   matcher.InputCard{Name: "Jinx - Loose Cannon", Variation: "251", Edition: "Promo"},
	},
	{
		Desc: "the same number stays in the main set for its own edition",
		In:   matcher.InputCard{Name: "Loose Cannon", Variation: "251", Edition: "Origins"},
	},
	{
		Desc: "a fuller storefront qualifier picks the promo it describes",
		In:   matcher.InputCard{Name: "Edge of Night (Champion Stamp)", Variation: "139", Edition: "Promo"},
	},
	{
		Desc: "the unqualified name keeps the main printing of that number",
		In:   matcher.InputCard{Name: "Edge of Night", Variation: "139", Edition: "Spiritforged"},
	},
	{
		// An unknown qualifier under a promo heading refuses to pick:
		// falling back to the main printing is the same mispricing
		// shape CoolStuffInc showed for champion-stamped cards.
		Desc: "negative: a promo qualifier no printing carries refuses to pick one",
		In:   matcher.InputCard{Name: "Edge of Night (Winner Stamp)", Variation: "139", Edition: "Promo"},
	},
	{
		Desc: "a per-set promo heading reaches the promo printing",
		In:   matcher.InputCard{Name: "Stacked Deck", Edition: "Origins: Promos", Variation: "183", Foil: true},
	},
	// CardTrader files the prerelease and launch cards under a heading of
	// its own. It ends in "Promos" like the generic one, so it could
	// collapse onto it, but it names a set outright - every listing under it
	// has an organized-play printing at its number - and the generic heading
	// cannot answer for it: this is one of the few numbers two promotional
	// sets both carry.
	{
		Desc: "the release-event heading narrows to the set that issued it",
		In:   matcher.InputCard{Name: "Jinx - Rebel", Variation: "202", Edition: "Release Event Promos", Foil: true},
	},
	{
		Desc: "the release-event heading tolerates the storefront qualifier",
		In:   matcher.InputCard{Name: "Jinx - Rebel", Variation: "202 Prerelease Stamped", Edition: "Release Event Promos", Foil: true},
	},
	{
		// The other half of the narrowing: 169 is a main-set number too.
		Desc: "the release-event heading reaches the promo of a main-set number",
		In:   matcher.InputCard{Name: "Ashe - Focused", Variation: "169", Edition: "Release Event Promos", Foil: true},
	},
	// CardTrader spells the champion stamp as a letter on the number ("058c"
	// against the gallery's plain 58) and says the word itself in the
	// blueprint version. The letter is no code - Poppy's champion is 178b,
	// because 178a was already an alternate art - so it is the wording that
	// decides, and the retry on the base number only accepts a printing that
	// wording describes.
	{
		Desc: "a champion-suffixed number reaches the stamp its wording names",
		In:   matcher.InputCard{Name: "Lillia - Protector of Dreams", Variation: "058c Summoner Skirmish | Champion", Edition: "Promos", Foil: true},
	},
	{
		Desc: "the unlettered sibling still answers for the plain promo",
		In:   matcher.InputCard{Name: "Lillia - Protector of Dreams", Variation: "058 Summoner Skirmish | Top Cut", Edition: "Promos", Foil: true},
	},
	{
		// The bare number describes nothing, so it still picks neither.
		Desc: "negative: a champion-suffixed number picks neither promo sibling",
		In:   matcher.InputCard{Name: "Lillia - Protector of Dreams", Variation: "058c", Edition: "Promos", Foil: true},
	},
	{
		// Poppy's champion stamp is not in the gallery at all: the retry
		// must leave the listing unmatched rather than hand it the plain 178.
		Desc: "negative: a stamp the gallery does not carry keeps its miss",
		In:   matcher.InputCard{Name: "Poppy - Defender of the Meek", Variation: "178b Summoner Skirmish | Champion", Edition: "Promos", Foil: true},
	},
	// Cardmarket's own "V.N" ordinal (see commit message).
	{
		Desc: "cardmarket ordinal V.1 reaches the non-champion stamp of a pair",
		In:   matcher.InputCard{Name: "Guardian Angel", Edition: "Spiritforged: Promos", Variation: "051 V.1 - Rare", Foil: true},
	},
	{
		Desc: "cardmarket ordinal V.2 reaches the champion stamp of a pair",
		In:   matcher.InputCard{Name: "Guardian Angel", Edition: "Spiritforged: Promos", Variation: "051 V.2 - Rare", Foil: true},
	},
	{
		Desc: "cardmarket ordinal V.2 outranks the plain OPP tier",
		In:   matcher.InputCard{Name: "Rengar, Trophy Hunter", Edition: "Unleashed: Promos", Variation: "120 V.2 - Epic", Foil: true},
	},
	{
		// The datastore carries the rune alternate arts, so an R01a row
		// answers this input.
		Desc: "an alternate-art rune resolves to its own printing",
		In:   matcher.InputCard{Name: "Fury Rune", Variation: "R01a Alternate Art", Edition: "Vendetta", Foil: true},
	},
	{
		// R01 is a real plain printing of the same name, so stripping the
		// letter off an art that does not exist must not reach it.
		Desc: "negative: an absent alternate art does not fall back on its base number",
		In:   matcher.InputCard{Name: "Fury Rune", Variation: "R01d Alternate Art", Edition: "Vendetta", Foil: true},
	},
	{
		Desc: "the storefront wording picks the promo variant it describes",
		In:   matcher.InputCard{Name: "Jinx - Loose Cannon (Metal) (Best Of)", Edition: "Promo"},
	},
	{
		Desc: "without a qualifier the plain promo answers, not a variant",
		In:   matcher.InputCard{Name: "Jinx - Loose Cannon", Edition: "Promo", Variation: "251"},
	},
	{
		Desc: "promo-shaped legend name still reaches the main set",
		In:   matcher.InputCard{Name: "Teemo - Swift Scout", Variation: "263", Edition: "Origins"},
	},
	{
		Desc: "promos never match without their edition",
		In:   matcher.InputCard{Name: "Teemo - Swift Scout", Variation: "263"},
	},
	{
		Desc: "base printing wins over its promos by default",
		In:   matcher.InputCard{Name: "Viktor - Leader", Variation: "246"},
	},
	// The datastore builder stamps every printing with its TCGplayer
	// product id (652842 is Ahri, Alluring OGN-066).
	{
		Desc: "tcgplayer product id resolves through the identifier index",
		In:   matcher.InputCard{ID: "652842", Foil: true},
	},
	// Error contract.
	{
		Desc: "negative: unknown card name",
		In:   matcher.InputCard{Name: "Nonexistent Imaginary Champion", Variation: "1"},
	},
	{
		Desc: "negative: known name with wrong collector number",
		In:   matcher.InputCard{Name: "Ahri, Alluring", Variation: "99999"},
	},
	{
		Desc: "negative: name printed in two sets stays ambiguous without a number",
		In:   matcher.InputCard{Name: "Recruit (NX)"},
	},
	{
		Desc: "negative: edition excludes the printing carrying the number",
		In:   matcher.InputCard{Name: "Ahri, Inquisitive", Variation: "227", Edition: "Vendetta"},
	},
}

func TestRiftboundMatch(t *testing.T) {
	b := gametest.Backend(t, matcher.GameRiftbound)
	gametest.Replay(t, b, riftboundSeeds, *updateRiftbound)
}

// TestRiftboundIdentifiers checks the TCGplayer identifier index the
// datastore builder stamps into the datastore: the external map and the
// per-card identifier round-trip to each other, and the id lookup path
// resolves finishes.
func TestRiftboundIdentifiers(t *testing.T) {
	b := gametest.Backend(t, matcher.GameRiftbound)

	// Walk the identifiers rather than the cards: the uuid a product id
	// points at is the plain printing where one is sold and the foil
	// otherwise, so there is no single finish to filter the cards by.
	var n int
	for pid, uuid := range b.ExternalIdentifiers[matcher.IDSpaceTCGplayer] {
		co, found := b.UUIDs[uuid]
		if !found {
			t.Errorf("external identifier %s resolves to %q, which is not a card", pid, uuid)
			continue
		}
		n++
		if got := co.Identifiers["tcgplayerProductId"]; got != pid {
			t.Errorf("%s: reached through %s but carries %q", uuid, pid, got)
			continue
		}
		// Asking for foil yields the foil printing where one is sold, and
		// otherwise the printing that is: the product id names the printing
		// rather than a finish, and output() clamps to what exists instead
		// of inventing a uuid for a finish nobody prints.
		want := co.FoilUUIDs[matcher.FinishFoil]
		if want == "" {
			want = uuid
		}
		if id, err := b.MatchID(pid, true); err != nil || id != want {
			t.Errorf("%s: MatchID(%s, foil) = (%q, %v), want %q", uuid, pid, id, err, want)
		}
	}
	if n == 0 {
		t.Fatal("no tcgplayer identifiers loaded; rebuild the datastore with github.com/mtgban/datastore-gen")
	}
	t.Logf("%d printings are reachable by tcgplayer product id", n)
}

// TestRiftboundFinishUUIDs checks the datastore invariants of the two-finish
// scheme: every uuid spells its finish out, each nonfoil has a foil sibling,
// both carry the right Finish and Foil markers, and the FoilUUIDs map
// round-trips to them.
func TestRiftboundFinishUUIDs(t *testing.T) {
	b := gametest.Backend(t, matcher.GameRiftbound)

	var bases int
	for uuid, co := range b.UUIDs {
		// Sealed products live outside the finish machinery: no finish,
		// no uuid to round-trip.
		if co.Sealed {
			continue
		}
		// What an entry is, it says. Reading the finish off the id's tail
		// would pin a "_nonfoil" the datastore does not spell,
		// and a uuid is a name rather than a description.
		if co.Finish == matcher.FinishFoil {
			if !co.Foil {
				t.Errorf("%s: foil finish with foil=%v", uuid, co.Foil)
			}
		} else {
			if co.Foil || co.Finish != matcher.FinishNonfoil {
				t.Errorf("%s: plain entry with foil=%v finish=%q", uuid, co.Foil, co.Finish)
			}
			bases++
		}
		if co.FoilUUIDs[co.Finish] != uuid {
			t.Errorf("%s: FoilUUIDs do not round-trip for %q: %v", uuid, co.Finish, co.FoilUUIDs)
		}
		// A foil sibling is not guaranteed: the datastore records the
		// finishes each printing is sold in, and about half of Riftbound is
		// sold in one. What must hold is that a uuid is named for exactly
		// the finishes recorded, and that every one it names is stored.
		for _, finish := range []string{matcher.FinishNonfoil, matcher.FinishFoil} {
			named, found := co.FoilUUIDs[finish]
			if found != co.HasFinish(finish) {
				t.Errorf("%s: FoilUUIDs names %q=%v but HasFinish(%s)=%v",
					uuid, finish, found, finish, co.HasFinish(finish))
				continue
			}
			if found {
				if _, stored := b.UUIDs[named]; !stored {
					t.Errorf("%s: %q names %s, which is no printing of ours", uuid, finish, named)
				}
			}
		}
	}
	if bases == 0 {
		t.Fatal("no cards loaded")
	}
	t.Logf("%d plain printings, %d uuids", bases, len(b.UUIDs))
}
