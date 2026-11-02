package lorcana

import (
	"flag"
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

var updateLorcana = flag.Bool("update-lorcana", false,
	"rewrite testdata/lorcana_test_data.json from what Match returns")

// lorcanaSeeds pin the contract edges the sampled corpus cannot reach: error
// paths, case-variant spellings, parenthetical suffixes, prefix names, and
// zero-numbered cards. Concrete cards referenced here exist in the real
// datastore (checked against LorcanaJSON at authoring time).
var lorcanaSeeds = []gametest.Case{
	{
		// TCGplayer prices this printing as three skus; the id names the
		// printing and the finish beside it names which sku.
		Desc: "product id plus the sku's finish reaches the foil sub-type",
		In:   matcher.InputCard{ID: "647652", Name: "Ariel - Singing Mermaid", Edition: "Fabled", Variation: "15/204", Finish: "Holofoil", Foil: true},
	},
	{
		// The same product id, each of the other skus TCGplayer prices it
		// as, so no two of them can answer with one uuid.
		Desc: "product id plus the plain sku's finish",
		In:   matcher.InputCard{ID: "647652", Name: "Ariel - Singing Mermaid", Edition: "Fabled", Variation: "15/204", Finish: "Normal"},
	},
	{
		Desc: "product id plus the standard foil sku's finish",
		In:   matcher.InputCard{ID: "647652", Name: "Ariel - Singing Mermaid", Edition: "Fabled", Variation: "15/204", Finish: "Cold Foil", Foil: true},
	},
	{
		// Promotion runs from any sibling: a sub-type's own uuid demotes
		// to the plain printing when the caller prices that sku.
		Desc: "a sub-type uuid plus a finish reaches its plain sibling",
		In:   matcher.InputCard{ID: "1951_holofoil", Finish: "Normal"},
	},
	{
		// An id with no finish beside it is still the flag's question, so
		// the wording does not override the printing the id names.
		Desc: "an id sent without a finish answers with the flag's foil",
		In:   matcher.InputCard{ID: "647652", Name: "Ariel - Singing Mermaid", Edition: "Fabled", Variation: "15/204 Holofoil", Foil: true},
	},
	{
		// A storefront that sends no id still spells the sub-type, and
		// the wording reaches it.
		Desc: "wording alone still picks the foil sub-type",
		In:   matcher.InputCard{Name: "Ariel - Singing Mermaid", Edition: "Fabled", Variation: "15/204 Holofoil", Foil: true},
	},
	{
		// Holofoil is a finish this game sells and this printing does not, so
		// the contradiction stands rather than answering with a sibling.
		Desc: "negative: a finish the printing is not sold in",
		In:   matcher.InputCard{ID: "494102", Name: "Ariel - On Human Legs", Edition: "The First Chapter", Variation: "1/204", Finish: "Holofoil", Foil: true},
	},
	{
		// A treatment is the card's promo type, not a finish, so as a finish
		// it is a name the game does not know, and the wording answers.
		Desc: "a treatment sent as the finish falls through to the wording",
		In:   matcher.InputCard{ID: "647652", Name: "Ariel - Singing Mermaid", Edition: "Fabled", Variation: "15/204", Finish: "Satin", Foil: true},
	},
	{
		// A name the game cannot place at all is a vendor spelling nobody
		// has taught it yet, which says nothing about the printing: the
		// wording answers, exactly as it does when no finish is sent.
		Desc: "a finish name the game does not know falls through to the wording",
		In:   matcher.InputCard{ID: "647652", Name: "Ariel - Singing Mermaid", Edition: "Fabled", Variation: "15/204 Holofoil", Finish: "Reverse Holofoil", Foil: true},
	},
	{
		Desc: "product id two cards claim falls back to the name",
		In:   matcher.InputCard{ID: "544501", Name: "Let It Go (Disney Lorcana Challenge Top 128)", Edition: "Disney Lorcana Promo Cards", Variation: "2 Holofoil", Foil: true},
	},
	{
		// A card sold in one finish is reached by a listing that names no
		// finish at all, because requiring a plain printing would delete the
		// only candidate there is, and 16 promos are sold foil alone.
		Desc: "foil-only promo listed without the flag",
		In:   matcher.InputCard{Name: "Maleficent - Exultant Spellcaster", Variation: "004B", Edition: "Disney Lorcana Promo Cards"},
	},
	{
		Desc: "the letter the data gives a sibling still selects it",
		In:   matcher.InputCard{Name: "Dalmatian Puppy - Tail Wagger", Variation: "4c"},
	},
	{
		// The truncated name and the missing flag at once: an enchanted
		// printing is sold foil only, so requiring a plain finish would
		// delete every candidate before the name could be adopted.
		Desc: "truncated name reaches a foil-only card listed without the flag",
		In:   matcher.InputCard{Name: "Hades - King of", Variation: "205", Edition: "The First Chapter"},
	},
	{
		// "Bolt" prefixes four distinct names, one of which also has two
		// foil-only printings: the tier set aside must not break the tie
		// the first tier could not.
		Desc: "negative: a foil-only sibling does not settle an ambiguous prefix",
		In:   matcher.InputCard{Name: "Bolt"},
	},
	{
		Desc: "name typeset with a hyphen character instead of a dash",
		In:   matcher.InputCard{Name: "Fix‐It Felix, Jr. - Delighted Sightseer", Variation: "17", Edition: "Shimmering Skies"},
	},
	// Case-variant spellings: pairs of distinct cards whose names differ only
	// in letter case, reachable from either spelling via the shared name hash.
	{
		Desc: "case variant: enchanted spelling with its own number",
		In:   matcher.InputCard{Name: "Cruella De Vil - Miserable As Usual", Variation: "4", Foil: true},
	},
	{
		Desc: "case variant: base spelling with its own number",
		In:   matcher.InputCard{Name: "Cruella De Vil - Miserable as Usual", Variation: "72"},
	},
	{
		Desc: "case variant: base spelling reaches the enchanted printing",
		In:   matcher.InputCard{Name: "Cruella De Vil - Miserable as Usual", Variation: "4", Foil: true},
	},
	{
		Desc: "case variant: song spelling with its own number",
		In:   matcher.InputCard{Name: "The Islands I Pulled from the Sea", Variation: "130"},
	},
	// Core Match splits parenthesized chunks off the name and appends them to
	// Variation; the number filter must keep working on decorated listings.
	{
		Desc: "parenthetical suffix does not poison the collector number",
		In:   matcher.InputCard{Name: "Hades - King of Olympus (Enchanted)", Variation: "205", Foil: true},
	},
	// Truncated names resolve through the prefix fallback plus the number.
	{
		Desc: "prefix name resolves via collector number",
		In:   matcher.InputCard{Name: "Stitch", Variation: "22"},
	},
	{
		Desc: "prefix name resolves among lettered variants",
		In:   matcher.InputCard{Name: "Dalmatian Puppy", Variation: "4c"},
	},
	// The one genuinely 0-numbered card, plus zero-padding tolerance.
	{
		Desc: "zero collector number is a real number",
		In:   matcher.InputCard{Name: "Bruno Madrigal - Undetected Uncle", Variation: "0/204"},
	},
	{
		Desc: "zero-padded collector number",
		In:   matcher.InputCard{Name: "99 Puppies", Variation: "024"},
	},
	{
		// Stripping the leading zeros must not take the digit with them
		// when a letter follows it.
		Desc: "zero collector number carrying a storefront letter",
		In:   matcher.InputCard{Name: "Bruno Madrigal - Undetected Uncle", Variation: "000B"},
	},
	{
		Desc: "negative: the restored zero is a number, not a wildcard",
		In:   matcher.InputCard{Name: "A Whole New World", Variation: "000B"},
	},
	// Error contract.
	{
		Desc: "negative: unknown card name",
		In:   matcher.InputCard{Name: "Nonexistent Imaginary Hero", Variation: "1"},
	},
	{
		Desc: "negative: known name with wrong collector number",
		In:   matcher.InputCard{Name: "Ariel - On Human Legs", Variation: "99999"},
	},
	{
		// A feed that never says "foil" is not claiming nonfoil, so a
		// foil-only printing still answers it; the opposite direction
		// below is a claim, and still refused.
		Desc: "foil-only printing listed without the flag",
		In:   matcher.InputCard{Name: "Hades - King of Olympus", Variation: "205"},
	},
	{
		Desc: "negative: nonfoil-only printing requested as foil",
		In:   matcher.InputCard{Name: "Anna - Ensnared Sister", Variation: "1", Foil: true},
	},
	{
		// A single-printing name still has its number validated:
		// ignoring the number would return this card.
		Desc: "negative: single-printing name with wrong collector number",
		In:   matcher.InputCard{Name: "Anna - Ensnared Sister", Variation: "77777"},
	},
	{
		Desc: "negative: wrong number zero does not disable the filter",
		In:   matcher.InputCard{Name: "99 Puppies", Variation: "0"},
	},
	{
		Desc: "negative: same name and number across sets aliases",
		In:   matcher.InputCard{Name: "Let It Go", Variation: "163"},
	},
	// A Special printing Ravensburger reprints wave to wave carries the same
	// name and, by coincidence rather than design, sometimes the same local
	// number in each wave's own promo pool - "Maleficent - Monstrous Dragon"
	// is card 5 of Promos Year 1 and card 5 of Promos Year 3. Cardmarket
	// sends a bare number, no denominator, so only its own "(V.N)" wording -
	// appended to Variation by Core Match - tells the two apart, and
	// Cardmarket writes it on every wave but the first.
	{
		Desc: "promo wave disambiguated by Cardmarket's own (V.2) wording",
		In:   matcher.InputCard{Name: "Maleficent - Monstrous Dragon", Variation: "5 V.2", Edition: "Promos Year 3", Foil: true},
	},
	{
		// The wave Cardmarket never suffixes - its own way of saying "V.1".
		Desc: "promo wave's first listing carries no (V.N) wording at all",
		In:   matcher.InputCard{Name: "Maleficent - Monstrous Dragon", Variation: "5", Edition: "Promos Year 1", Foil: true},
	},
	// An edition naming no set of its own - Cardmarket's yearly promo
	// headings, Cool Stuff Inc's plain "Promo" - is read against the DLPC
	// promo pool: a bare name unique to one number there is adopted, the
	// way a named set's own qualifier wording would license it.
	{
		Desc: "bare name unique at its number in the promo pool adopts the decoration",
		In:   matcher.InputCard{Name: "Snow White - Unexpected Houseguest", Variation: "41", Edition: "Promos Year 1"},
	},
	{
		// The pool holds two decorated siblings at 25 (CS Exclusive, JP
		// Exclusive) and Cardmarket sends no wording to tell them apart.
		Desc: "negative: two decorated siblings in the promo pool refuse",
		In:   matcher.InputCard{Name: "Mickey Mouse - True Friend", Variation: "25", Edition: "Promos Year 1"},
	},
	// Edition narrowing: the Match skeleton restricts candidates to the sets
	// matching a supplied edition; without one (or with one that resolves to
	// no set name) the number-driven contract above is unchanged.
	{
		// AdjustName's prefix fallback: "Flotsam" prefixes both The First
		// Chapter's 43 and Ursula's Return's own 43, and the edition is what
		// narrows the tier to one.
		Desc: "truncated name narrowed to one set by the edition",
		In:   matcher.InputCard{Name: "Flotsam", Variation: "43/204", Edition: "The First Chapter"},
	},
	{
		// Fabled reprints five different Mickeys. A tier holder must be
		// tracked per (name, set) rather than per name alone, or the first
		// set seen for one of the five silently wins the edition filter and
		// narrows a genuine five-way tie down to a wrong single answer.
		Desc: "negative: a name reprinted across several candidates in one edition stays ambiguous",
		In:   matcher.InputCard{Name: "Mickey Mouse", Edition: "Fabled"},
	},
	{
		Desc: "edition disambiguates a name and number shared across sets",
		In:   matcher.InputCard{Name: "Let It Go", Variation: "163", Edition: "The First Chapter"},
	},
	{
		Desc: "edition disambiguates toward the later printing",
		In:   matcher.InputCard{Name: "Let It Go", Variation: "163", Edition: "Winterspell"},
	},
	{
		Desc: "storefront-prefixed edition still narrows",
		In:   matcher.InputCard{Name: "Let It Go", Variation: "163", Edition: "Disney Lorcana: Winterspell"},
	},
	{
		Desc: "unrecognized edition does not block a unique number",
		In:   matcher.InputCard{Name: "99 Puppies", Variation: "024", Edition: "Not A Real Set"},
	},
	{
		Desc: "negative: edition excludes the printing carrying the number",
		In:   matcher.InputCard{Name: "Let It Go", Variation: "10", Edition: "Winterspell"},
	},
	{
		Desc: "negative: prefix without a number stays ambiguous",
		In:   matcher.InputCard{Name: "Dalmatian Puppy"},
	},
	// Non-card products TCGplayer files under its "Cards" type, verbatim from
	// the live catalog. The lot spellings carry a real card's name or a real
	// card's promotion, so the last case pins that neither is dragged down
	// with them.
	{
		Desc: "negative: puzzle insert piece is not a card",
		In:   matcher.InputCard{Name: "Azurite Sea Puzzle Insert (Top Left)", Variation: "Normal", Edition: "Azurite Sea"},
	},
	{
		Desc: "negative: puzzle insert lot named after a card is not a card",
		In:   matcher.InputCard{Name: "Mickey Mouse - Brave Little Tailor Puzzle Insert (Set of 4)", Variation: "Normal", Edition: "The First Chapter"},
	},
	{
		Desc: "negative: multi-card promo lot is not a card",
		In:   matcher.InputCard{Name: "Disney Cruise Promos (Set of 5)", Variation: "Normal", Edition: "Disney Lorcana Promo Cards"},
	},
	{
		Desc: "the promotion behind the lot still names a real card",
		In:   matcher.InputCard{Name: "Mickey Mouse - True Friend (Disney Cruise Promo)", Variation: "10 Holofoil", Edition: "Disney Lorcana Promo Cards", Foil: true},
	},
	// Core refuses wording that says oversize unless a printing of the name
	// is flagged IsOversized; verbatim from Cool Stuff Inc's retail feed.
	{
		Desc: "oversized Illumineer's Quest battleground is licensed by its oversized printing",
		In:   matcher.InputCard{Name: "The Encounter // A Dark Realm - Battleground (Oversized)", Variation: "Oversized Easy and Medium Battleground Card from Deep Trouble", Edition: "Illumineer's Quest - Deep Trouble"},
	},
}

func TestLorcanaMatch(t *testing.T) {
	b := gametest.Backend(t, matcher.GameLorcana)
	gametest.Replay(t, b, lorcanaSeeds, *updateLorcana)
}

// TestLorcanaFinishNames checks the invariants of the per-uuid Finish field:
// foil entries carry their exported foil-type name, nonfoil entries carry
// "nonfoil", and a sub-type suffixed uuid agrees with the name that derived
// it.
func TestLorcanaFinishNames(t *testing.T) {
	b := gametest.Backend(t, matcher.GameLorcana)

	subTypes := map[string]int{}
	for uuid, co := range b.UUIDs {
		// Sealed products live outside the finish machinery: no finish,
		// no finish-suffixed uuid, nothing to derive
		if co.Sealed {
			continue
		}
		if co.Foil {
			if co.Finish == "" || co.Finish == matcher.FinishNonfoil {
				t.Errorf("%s: foil entry with finish %q", uuid, co.Finish)
			}
		} else if co.Finish != matcher.FinishNonfoil {
			t.Errorf("%s: nonfoil entry with finish %q", uuid, co.Finish)
		}
		// Every foil uuid is suffixed with the finish's canonical name, so
		// the entry's own finish spells the suffix outright - and a bare
		// uuid is the nonfoil, or the one finish a foil-only printing has.
		idx := strings.IndexByte(uuid, '_')
		if idx < 0 {
			if co.Foil {
				t.Errorf("%s: foil entry carries no finish suffix", uuid)
			}
			continue
		}
		if co.Finish != uuid[idx+1:] {
			t.Errorf("%s: finish %q does not spell suffix %q", uuid, co.Finish, uuid[idx+1:])
		}
		subTypes[co.Finish]++
	}
	if len(subTypes) == 0 {
		t.Fatal("no foil sub-type entries found in the datastore")
	}
	t.Logf("sub-type finishes: %v", subTypes)
}
