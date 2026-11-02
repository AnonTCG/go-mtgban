package gundam

import (
	"flag"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

var updateGundam = flag.Bool("update-gundam", false,
	"rewrite testdata/gundam_test_data.json from what Match returns")

// gundamSeeds pin the contract edges this game turns on - the rarity that
// tells same-numbered printings apart, the shelves storefronts spell the sets
// with, and the names carrying a parenthetical of their own.
var gundamSeeds = []gametest.Case{
	{
		// The collision this game is built around: GD01, its beta edition
		// and the deck build box print the same card at the same number
		// and the same rarity, so nothing but the edition tells them apart.
		Desc: "edition picks between three printings of one number",
		In:   matcher.InputCard{Name: "Guntank", Edition: "Newtype Rising", Variation: "GD01-008"},
	},
	{
		Desc: "the beta edition of that same number",
		In:   matcher.InputCard{Name: "Guntank", Edition: "Edition Beta", Variation: "GD01-008"},
	},
	{
		// A parallel run is marked by suffixing the rarity, so the rarity
		// is what separates printings the number cannot.
		Desc: "rarity separates a parallel from the card it parallels",
		In:   matcher.InputCard{Name: "Gundam", Edition: "Newtype Rising", Variation: "GD01-001 LR+"},
	},
	{
		Desc: "the doubled rarity is its own printing",
		In:   matcher.InputCard{Name: "Gundam", Edition: "Newtype Rising", Variation: "GD01-001 LR++"},
	},
	{
		// Card Trader letters the parallel onto the number where the
		// catalog letters nothing; the scraper trims it and lets the
		// version say the rarity instead.
		Desc: "a plain number keeps the plain printing",
		In:   matcher.InputCard{Name: "Gundam", Edition: "Newtype Rising", Variation: "GD01-001"},
	},
	{
		// A card whose own name ends in a parenthetical. Storefronts hang
		// their wording behind it, and taking the last bracket asks for a
		// card the catalog has not got.
		Desc: "parenthetical is part of the real name",
		In:   matcher.InputCard{Name: "Gelgoog (GQ)", Edition: "Starter Deck 06: Clan Unity", Variation: "ST06-004"},
	},
	{
		Desc: "that name with its rarity beside it",
		In:   matcher.InputCard{Name: "Gelgoog (GQ)", Edition: "Starter Deck 06: Clan Unity", Variation: "ST06-004 C+"},
	},
	{
		// The starter decks carry their number in the set name, so a shelf
		// spelling only the title still has to reach them.
		Desc: "starter deck named by its title alone",
		In:   matcher.InputCard{Name: "Suletta Mercury", Edition: "Heroic Beginnings", Variation: "ST01-011"},
	},
	{
		Desc: "starter deck named the way the catalog names it",
		In:   matcher.InputCard{Name: "Suletta Mercury", Edition: "Starter Deck 01: Heroic Beginnings", Variation: "ST01-011"},
	},
	{
		// "Beta" carries no set code and needs no rule: the edition tier
		// that matches on containment already reaches Edition Beta.
		Desc: "bare Beta reaches the beta edition",
		In:   matcher.InputCard{Name: "Aile Strike Gundam", Edition: "Beta", Variation: "ST04-001"},
	},
	{
		// The one title that contains-matches two sets, GD05 and the deck
		// build box named for it. Tier one wins and this pins that.
		Desc: "a title naming two sets keeps the set it equals",
		In:   matcher.InputCard{Name: "Destiny Gundam", Edition: "Freedom Ascension", Variation: "GD05-055 R+"},
	},
	{
		// The promotional token shelves are named exactly as the catalog
		// names those sets, and must keep resolving verbatim.
		Desc: "promotional resource token shelf names its own set",
		In:   matcher.InputCard{Name: "Resource", Edition: "Promotional Resource Tokens", Variation: "RP-067"},
	},
	{
		Desc: "promotional EX base shelf names its own set",
		In:   matcher.InputCard{Name: "EX Base", Edition: "Promotional EX Base Tokens", Variation: "EXBP-019"},
	},
	{
		// A token is named for the art it wears; the word is what reaches
		// the token rather than the unit card sharing the name. The set
		// still has to be said - another set prints this token at this
		// number too.
		Desc: "the token word reaches the token",
		In:   matcher.InputCard{Name: "Gundam", Edition: "Edition Beta", Variation: "T-001 Token"},
	},
	{
		// Two sets print the same token at the same number, so a token
		// named without its set names two.
		Desc: "negative: a token named without its set",
		In:   matcher.InputCard{Name: "Gundam", Variation: "T-001 Token"},
	},
	{
		// A storefront shelves a promo under the set it is numbered for,
		// and the promo set is where the stamped reprint lives. The label
		// beside the name is what says so, and unpinning the edition is
		// what lets it be heard.
		Desc: "an event label reaches the promo set the shelf does not name",
		In:   matcher.InputCard{Name: "Gundam Dynames", Edition: "Celestial Drive", Variation: "ST07-005 Newtype Challenge 2026 Legend Rare"},
	},
	{
		// The same shelf, no label: the base printing is what a listing
		// naming no event means, and the unpin must not reach past it.
		Desc: "the same shelf without a label keeps the base printing",
		In:   matcher.InputCard{Name: "Gundam Dynames", Edition: "Celestial Drive", Variation: "ST07-005 Legend Rare"},
	},
	{
		// The SP of a starter deck's card is sold in a booster released
		// later, and the storefront still shelves it under the deck.
		Desc: "an SP label reaches the booster that carries it",
		In:   matcher.InputCard{Name: "Suletta Mercury", Edition: "Heroic Beginnings", Variation: "ST01-011 SP C+"},
	},
	{
		// No label to go on here - the second parallel carries none - so
		// the rarity is what says the printing is filed elsewhere.
		Desc: "a rarity the shelf does not carry reaches the set that does",
		In:   matcher.InputCard{Name: "Phoenix Gundam (Power Unleashed) (EX)", Edition: "Generation Pulse", Variation: "ST10-006 Alt-Art ++ LR++"},
	},
	{
		Desc: "the rarities that shelf does carry stay on it",
		In:   matcher.InputCard{Name: "Phoenix Gundam (Power Unleashed) (EX)", Edition: "Generation Pulse", Variation: "ST10-006 Alt-Art + LR+"},
	},
	{
		// The unpin must not reach the beta edition: the deck and its beta
		// print this parallel alike, and a shelf that carries the rarity
		// named is answered by the shelf.
		Desc: "a parallel the shelf carries is not unpinned toward beta",
		In:   matcher.InputCard{Name: "Gundam", Edition: "Starter Deck 01: Heroic Beginnings", Variation: "ST01-001 LR+"},
	},
	{
		Desc: "negative: a number belonging to no set of this game",
		In:   matcher.InputCard{Name: "Gundam", Variation: "ZZ99-999"},
	},
	{
		Desc: "negative: a name the catalog does not carry",
		In:   matcher.InputCard{Name: "Not A Gundam Card At All", Variation: "GD01-001"},
	},
}

func TestGundamMatch(t *testing.T) {
	b := gametest.Backend(t, matcher.GameGundam)
	gametest.Replay(t, b, gundamSeeds, *updateGundam)
}
