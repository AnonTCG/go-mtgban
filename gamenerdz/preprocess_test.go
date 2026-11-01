package gamenerdz

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mtgban/go-mtgban/internal/datastore"
	"github.com/mtgban/go-mtgban/internal/jsonflex"
	"github.com/mtgban/go-mtgban/matcher"

	_ "github.com/mtgban/go-mtgban/matcher/magic"
	_ "github.com/mtgban/go-mtgban/matcher/pokemon"
)

// TestMain loads the datastore once for the whole package: the prerelease
// shelf is read against the catalog, so preprocessing needs one.
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

func TestPreprocess(t *testing.T) {
	b := realDatastore(t)
	tests := []struct {
		game    matcher.Game
		product GNProduct

		name      string
		edition   string
		variation string
		finish    string
		foil      bool
		err       bool
	}{
		{
			// The etched printing is a finish of its own. The bracketed
			// wording is one of the two ways this storefront spells it,
			// and the only one on a Modern Horizons 2 retro frame, whose
			// number carries all three finishes at once.
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName:    "Aeromoeba (Retro Frame) (Foil Etched) (MH2-389) - Modern Horizons 2 Foil",
				SelectedFinish: "foil",
				ProductData:    GNProductData{Set: "mh2", SetName: "Modern Horizons 2"},
			},
			name:      "Aeromoeba",
			edition:   "Modern Horizons 2",
			variation: "389 Etched",
			foil:      true,
		},
		{
			// The other way is the segment the sku carries.
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName:    "Aeromoeba (MH2-389) - Modern Horizons 2 Etched Foil",
				SelectedFinish: "foil",
				ProductData:    GNProductData{Set: "mh2", SetName: "Modern Horizons 2"},
				RetailVariants: []GNRetailVariant{{SKU: "MTG-MH2-389-EF-7NTYUWLWDO"}},
			},
			name:      "Aeromoeba",
			edition:   "Modern Horizons 2",
			variation: "389 Etched",
			foil:      true,
		},
		{
			// The shelf tail alone is not one of them: it reads "Etched
			// Foil" on ordinary foils too, and every product that claims
			// it there and nowhere else is a second listing of a foil
			// sibling at the same price.
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName:    "Arid Mesa - Modern Horizons 2 Etched Foil",
				SelectedFinish: "foil",
				ProductData:    GNProductData{Set: "mh2", SetName: "Modern Horizons 2", NumberDigits: "436"},
				RetailVariants: []GNRetailVariant{{SKU: "MTG-MH2-436-F-BEUFVXJ561"}},
			},
			name:      "Arid Mesa",
			edition:   "Modern Horizons 2",
			variation: "436",
			foil:      true,
		},
		{
			// A card whose only finish is etched answers by number alone
			// and must not be moved off it.
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName:    "Abbot of Keral Keep (2X2-446) - Double Masters 2022 Etched Foil",
				SelectedFinish: "foil",
				ProductData:    GNProductData{Set: "2x2", SetName: "Double Masters 2022"},
				RetailVariants: []GNRetailVariant{{SKU: "MTG-2X2-446-EF-FD4P1K77CT"}},
			},
			name:      "Abbot of Keral Keep",
			edition:   "Double Masters 2022",
			variation: "446 Etched",
			foil:      true,
		},
		{
			// A prerelease stamp, filed under the pseudo-set with the
			// card's own number: naming the stamp reaches the printing
			// the shelf means, whatever set that printing belongs to.
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName:    "Avatar of Hope (PRE-003) - Prophecy Promos Foil",
				SelectedFinish: "foil",
				ProductData:    GNProductData{Set: "pre", SetName: "Prerelease Cards"},
			},
			name:      "Avatar of Hope",
			edition:   "Prophecy Promos",
			variation: "003 Prerelease",
			foil:      true,
		},
		{
			// A set's promos get a shelf code of the storefront's own,
			// which names no set; the shelf's name is the catalog's own
			// name for it, and a number with no promo-pack letter still
			// means the pack, since prereleases shelve elsewhere.
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName: "Purphoros, Bronze-Blooded (PPTHB-150) - Theros Beyond Death Promos: (enchantment)",
				ProductData: GNProductData{Set: "ppthb", CatalogSet: "Theros Beyond Death Promos"},
			},
			name:      "Purphoros, Bronze-Blooded",
			edition:   "pthb",
			variation: "150p",
		},
		{
			// The shelf label and the catalog's name for a set are two
			// different fields, and only the second one names a set: this
			// product is shelved under "Promo Pack: Theros Beyond Death",
			// which the catalog knows nothing by.
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName: "Archon of Sun's Grace (PPTHB-003) - Theros Beyond Death Promos",
				ProductData: GNProductData{
					Set:        "ppthb",
					SetName:    "Promo Pack: Theros Beyond Death",
					CatalogSet: "Theros Beyond Death Promos",
				},
			},
			name:      "Archon of Sun's Grace",
			edition:   "pthb",
			variation: "003p",
		},
		{
			// The same shelf, where the number already carries the letter
			// the catalog files the pack under: it answers as it is.
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName: "Atris, Oracle of Half-Truths (PPTHB-209P) - Theros Beyond Death Promos",
				ProductData: GNProductData{Set: "ppthb", CatalogSet: "Theros Beyond Death Promos"},
			},
			name:      "Atris, Oracle of Half-Truths",
			edition:   "pthb",
			variation: "209P",
		},
		{
			// An ordinary card the storefront shelved as a stamp by
			// mistake. No stamped printing to reach, so the reading it
			// already had stands.
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName: "Culling Scales (MRD-160) - Mirrodin",
				ProductData: GNProductData{Set: "pre", SetName: "Prerelease Cards"},
			},
			name:      "Culling Scales",
			edition:   "Mirrodin",
			variation: "160",
		},
		{
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName:    "Teferi, Time Raveler (PPELD-221P) - War of the Spark",
				SelectedFinish: "nonfoil",
				ProductData:    GNProductData{SetName: "War of the Spark"},
			},
			name: "Teferi, Time Raveler", edition: "War of the Spark", variation: "221P",
		},
		{
			// The name stops at the first parenthesis, and the number tag is
			// the last one, past the bare display number the name also shows.
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName:    "Aang, Air Nomad (0265) (TLE-265) - Avatar: The Last Airbender: Eternal Decks Foil",
				SelectedFinish: "foil",
				ProductData:    GNProductData{Set: "TLE"},
			},
			name: "Aang, Air Nomad", edition: "TLE", variation: "265", foil: true,
		},
		{
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName:    "Aang's Shelter - Teferi's Protection (Borderless) (TLE-007) - Avatar",
				SelectedFinish: "nonfoil",
				ProductData:    GNProductData{Set: "TLE"},
			},
			name: "Aang's Shelter - Teferi's Protection", edition: "TLE", variation: "007",
		},
		{
			// A List number with no set size still takes its origin from
			// the tag ahead of it.
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName:    "Impact Tremors (MOC) (LIST-285) - The List Reprints",
				SelectedFinish: "Normal",
				ProductData:    GNProductData{Set: "list", SetName: "The List Reprints"},
			},
			name: "Impact Tremors", edition: "The List", variation: "MOC-285",
		},
		{
			game: matcher.GameLorcana,
			product: GNProduct{
				DisplayName:    "4*Town - Hottest Band of the Year (17/204) - Attack of the Vine",
				SelectedFinish: "Normal",
				ProductData:    GNProductData{SetName: "Attack of the Vine"},
			},
			name: "4*Town - Hottest Band of the Year", edition: "Attack of the Vine", variation: "17",
		},
		{
			game: matcher.GameLorcana,
			product: GNProduct{
				DisplayName:    "99 Puppies (24/204) - Into the Inklands Cold Foil",
				SelectedFinish: "Cold Foil",
				ProductData:    GNProductData{SetName: "Into the Inklands"},
			},
			name: "99 Puppies", edition: "Into the Inklands", variation: "24",
			finish: "Cold Foil", foil: true,
		},
		{
			// A promo shelf that publishes no set size writes the number
			// alone.
			game: matcher.GameLorcana,
			product: GNProduct{
				DisplayName:    "Bruno Madrigal - Undetected Uncle (4) - D23 Promos Holofoil",
				SelectedFinish: "Holofoil",
				ProductData:    GNProductData{SetName: "D23 Promos"},
			},
			name: "Bruno Madrigal - Undetected Uncle", edition: "D23 Promos",
			variation: "4", finish: "Holofoil", foil: true,
		},
		{
			// A promo card's own wording brackets a qualifier ahead of the
			// number, and the number's denominator is the promo set's own
			// number rather than a plain total.
			game: matcher.GameLorcana,
			product: GNProduct{
				DisplayName:    "Minnie Mouse - Pirate Lookout (Disney Cruise Promo) (17/P3) - Disney Lorcana Promo Cards Holofoil",
				SelectedFinish: "Holofoil",
				ProductData:    GNProductData{SetName: "Disney Lorcana Promo Cards"},
			},
			name: "Minnie Mouse - Pirate Lookout (Disney Cruise Promo)", edition: "Disney Lorcana Promo Cards",
			variation: "17", finish: "Holofoil", foil: true,
		},
		{
			// A doubled slash ahead of the set size.
			game: matcher.GameLorcana,
			product: GNProduct{
				DisplayName:    "Rafiki - Mystical Fighter (54//204) - Into the Inklands",
				SelectedFinish: "Normal",
				ProductData:    GNProductData{SetName: "Into the Inklands"},
			},
			name: "Rafiki - Mystical Fighter", edition: "Into the Inklands", variation: "54",
		},
		{
			game: matcher.GamePokemon,
			product: GNProduct{
				DisplayName:    "Abra 65/130 - Base Set 2",
				SelectedFinish: "Normal",
				ProductData:    GNProductData{SetName: "Base Set 2"},
			},
			name: "Abra", edition: "Base Set 2", variation: "65/130",
		},
		{
			game: matcher.GamePokemon,
			product: GNProduct{
				DisplayName:    "Abra 63 - SV Scarlet and Violet 151 Reverse Holofoil",
				SelectedFinish: "Reverse Holofoil",
				ProductData:    GNProductData{SetName: "SV: Scarlet & Violet 151"},
			},
			name: "Abra", edition: "SV: Scarlet & Violet 151", variation: "63",
			finish: "Reverse Holofoil",
		},
		{
			// A card whose own name ends in something number-shaped. The
			// number written over the set's size is the collector number,
			// and the suffix in front of it belongs to the name.
			game: matcher.GamePokemon,
			product: GNProduct{
				DisplayName:    "Alakazam E4 38/111 - Rising Rivals",
				SelectedFinish: "Normal",
				ProductData:    GNProductData{SetName: "Rising Rivals"},
			},
			name: "Alakazam E4", edition: "Rising Rivals", variation: "38/111",
		},
		{
			game: matcher.GamePokemon,
			product: GNProduct{
				DisplayName:    "Energy Removal 2 80/109 - Ruby  Sapphire",
				SelectedFinish: "Normal",
				ProductData:    GNProductData{SetName: "Ruby and Sapphire"},
			},
			name: "Energy Removal 2", edition: "Ruby and Sapphire", variation: "80/109",
		},
		{
			// A promo number is written over its series, not a set size.
			game: matcher.GamePokemon,
			product: GNProduct{
				DisplayName:    "Clefairy - 381/SM-P (Dream League)  - SM Promos Holofoil",
				SelectedFinish: "Holofoil",
				ProductData:    GNProductData{SetName: "SM Promos"},
			},
			name: "Clefairy", edition: "SM Promos", variation: "381/SM-P (Dream League)",
			finish: "Holofoil",
		},
		{
			// The catalog's own number repeated behind the printed one, and
			// the wording between them. The name ends at the printed number
			// whether or not a dash of its own opens the description.
			game: matcher.GamePokemon,
			product: GNProduct{
				DisplayName:    "Dragonite 149/165 (Cosmos Holo) 149 - Miscellaneous Cards  Products Holofoil",
				SelectedFinish: "Holofoil",
				ProductData:    GNProductData{SetName: "Miscellaneous Cards & Products"},
			},
			name: "Dragonite", edition: "Miscellaneous Cards & Products",
			variation: "149/165 (Cosmos Holo)", finish: "Holofoil",
		},
		{
			// A shelf whose numbers are letters.
			game: matcher.GamePokemon,
			product: GNProduct{
				DisplayName:    "Unown Z/115 - Unseen Forces Holofoil",
				SelectedFinish: "Holofoil",
				ProductData:    GNProductData{SetName: "Unseen Forces"},
			},
			name: "Unown", edition: "Unseen Forces", variation: "Z/115",
			finish: "Holofoil",
		},
		{
			// The symbol this storefront prints where the catalog writes
			// the letter.
			game: matcher.GamePokemon,
			product: GNProduct{
				DisplayName:    "Nidoran ♀ 81/144 - Skyridge",
				SelectedFinish: "Normal",
				ProductData:    GNProductData{SetName: "Skyridge"},
			},
			name: "Nidoran F", edition: "Skyridge", variation: "81/144",
		},
		{
			// The finishing place is written in brackets where every other
			// qualifier gets parentheses. Left as it is the matcher reads no
			// place at all and answers with the printing awarded none, which
			// at this number is a $4 card standing against a $558 one.
			game: matcher.GameOnePiece,
			product: GNProduct{
				DisplayName: "Monkey.D.Luffy (Offline Regional 2024 Vol. 2) [Finalist] (P-041) One Piece Promotion Cards",
				ProductData: GNProductData{SetName: "One Piece Promotion Cards"},
			},
			name:      "Monkey.D.Luffy (Offline Regional 2024 Vol. 2) (Finalist)",
			edition:   "One Piece Promotion Cards",
			variation: "P-041",
		},
		{
			// Variant wording before the code stays in the name for the
			// matcher to read.
			game: matcher.GameOnePiece,
			product: GNProduct{
				DisplayName:    "Ama no Murakumo Sword (Jolly Roger Foil) (OP06-056) Premium Booster -The Best-",
				SelectedFinish: "Foil",
				ProductData:    GNProductData{SetName: "Premium Booster -The Best-"},
			},
			name: "Ama no Murakumo Sword (Jolly Roger Foil)", edition: "Premium Booster -The Best-",
			variation: "OP06-056", foil: true,
		},
		{
			// The Premium Booster listings write the code twice, around the
			// variant wording; the earlier copy is shed, the wording stays.
			game: matcher.GameOnePiece,
			product: GNProduct{
				DisplayName:    "Baby 5 (OP04-032) (Jolly Roger Foil) (OP04-032) Premium Booster -The Best- Foil",
				SelectedFinish: "Foil",
				ProductData:    GNProductData{SetName: "Premium Booster -The Best-"},
			},
			name: "Baby 5 (Jolly Roger Foil)", edition: "Premium Booster -The Best-",
			variation: "OP04-032", foil: true,
		},
		{
			// A DON!! listing carries no card code at all - the catalog
			// files every one as "DON!! Card" and tells them apart by the
			// wording between the name and the shelf's double space.
			game: matcher.GameOnePiece,
			product: GNProduct{
				DisplayName:    "DON!! Card (Ace)  Premium Booster -The Best- Foil",
				SelectedFinish: "Foil",
				ProductData:    GNProductData{SetName: "Premium Booster -The Best-"},
			},
			name: "DON!! Card", edition: "Premium Booster -The Best-", variation: "Ace", foil: true,
		},
		{
			// A set's own DON!! carries no wording at all.
			game: matcher.GameOnePiece,
			product: GNProduct{
				DisplayName: "DON!! Card  Paramount War",
				ProductData: GNProductData{SetName: "Paramount War"},
			},
			name: "DON!! Card", edition: "Paramount War", variation: "",
		},
		{
			// Two DON!! qualifiers stack in their own parens.
			game: matcher.GameOnePiece,
			product: GNProduct{
				DisplayName:    "DON!! Card (2Y) (Double Pack Set Vol. 8)  Legacy of the Master Foil",
				SelectedFinish: "Foil",
				ProductData:    GNProductData{SetName: "Legacy of the Master"},
			},
			name: "DON!! Card", edition: "Legacy of the Master", variation: "2Y Double Pack Set Vol. 8", foil: true,
		},
		{
			// Two DON!! listings carry a "//" as part of the card's own
			// name, which the catalog files literally.
			game: matcher.GameOnePiece,
			product: GNProduct{
				DisplayName: "DON!! Card // Green Compass  Starter Deck 1: Straw Hat Crew",
				ProductData: GNProductData{SetName: "Starter Deck 1: Straw Hat Crew"},
			},
			name: "DON!! Card // Green Compass", edition: "Starter Deck 1: Straw Hat Crew", variation: "",
		},
		{
			// On this shelf the storefront's "(Parallel)" is the manga
			// printing.
			game: matcher.GameOnePiece,
			product: GNProduct{
				DisplayName:    "Shanks (004) (Parallel) (OP09-004) Emperors in the New World Foil",
				SelectedFinish: "Foil",
				ProductData:    GNProductData{SetName: "Emperors in the New World"},
			},
			name: "Shanks (004) (Manga)", edition: "Emperors in the New World",
			variation: "OP09-004", foil: true,
		},
		{
			// A code padded with a trailing space before the parenthesis.
			game: matcher.GameOnePiece,
			product: GNProduct{
				DisplayName: "Tony Tony.Chopper (Store Tournament 2025 Vol. 4) (P-101 ) One Piece Promotion Cards",
				ProductData: GNProductData{SetName: "One Piece Promotion Cards"},
			},
			name: "Tony Tony.Chopper (Store Tournament 2025 Vol. 4)", edition: "One Piece Promotion Cards",
			variation: "P-101",
		},
		{
			// The Extra Booster dash packs write the code bare off the
			// name's own dash, with the variant wording trailing it rather
			// than leading it.
			game: matcher.GameOnePiece,
			product: GNProduct{
				DisplayName:    "Koala - OP05-006 (Dash Pack) Extra Booster: One Piece Heroines Edition Foil",
				SelectedFinish: "Foil",
				ProductData:    GNProductData{SetName: "Extra Booster: One Piece Heroines Edition"},
			},
			name: "Koala (Dash Pack)", edition: "Extra Booster: One Piece Heroines Edition",
			variation: "OP05-006", foil: true,
		},
		{
			game:    matcher.GameLorcana,
			product: GNProduct{DisplayName: "Illumineer's Trove - Sapphire and Steel"},
			err:     true,
		},
		{
			// A Jumpstart front card is not a card.
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName: "Lands (JMP-019) - Jumpstart Front Cards",
				ProductData: GNProductData{Set: "jmp", SetName: "Jumpstart Front Cards"},
			},
			err: true,
		},
		{
			// A Secret Lair Commander deck numbers its cards by the deck's
			// own index, so the printing the deck lists places them in the
			// List, also where the name is a List card's twin.
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName:    "Fiery Gambit (PHED-022) - Heads I Win, Tails You Lose Foil",
				SelectedFinish: "foil",
				ProductData:    GNProductData{Set: "phed", SetName: "Heads I Win, Tails You Lose"},
			},
			name:      "Fiery Gambit",
			edition:   "PLST",
			variation: "MRD-90",
			foil:      true,
		},
		{
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName:    "Great Furnace (PHED-066) - Heads I Win, Tails You Lose Foil",
				SelectedFinish: "foil",
				ProductData:    GNProductData{Set: "phed", SetName: "Heads I Win, Tails You Lose"},
			},
			name:      "Great Furnace",
			edition:   "PLST",
			variation: "MRD-282",
			foil:      true,
		},
		{
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName:    "Rhys the Redeemed (PCTB-045) - From Cute to Brute",
				SelectedFinish: "nonfoil",
				ProductData:    GNProductData{Set: "pctb", SetName: "From Cute to Brute"},
			},
			name:      "Rhys the Redeemed",
			edition:   "PLST",
			variation: "SHM-237",
		},
		{
			// The miscellaneous shelves index their promos themselves, so
			// the card is looked for by name in the sets they hold.
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName:    "Tifa Lockhart (Borderless) (MFP-002) - MagicFest Cards Foil",
				SelectedFinish: "foil",
				ProductData:    GNProductData{Set: "mfp", SetName: "MagicFest Cards"},
			},
			name:    "Tifa Lockhart",
			edition: "MagicFest 2025",
			foil:    true,
		},
		{
			// A play shelf number is the card's in the set its promo pack
			// came from, so the stamp reaches the pack.
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName: "Cemetery Desecrator (PLAY-100) - Play Promos",
				ProductData: GNProductData{Set: "play", SetName: "Play Promos"},
			},
			name:      "Cemetery Desecrator",
			edition:   "play",
			variation: "100 Promo Pack",
		},
		{
			// A card's own parenthesis is part of its name.
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName:    "Erase (Not the Urza's Legacy One) (UNH-010) - Unhinged Foil",
				SelectedFinish: "foil",
				ProductData:    GNProductData{Set: "unh", SetName: "Unhinged"},
			},
			name:      "Erase (Not the Urza's Legacy One)",
			edition:   "Unhinged",
			variation: "010",
			foil:      true,
		},
		{
			// The bare number is the printing's own where the code's is
			// another printing of the card.
			game: matcher.GameMagic,
			product: GNProduct{
				DisplayName:    "Eddie Brock (0233) (Borderless) (SPM-224) - Marvel's Spider-Man Foil",
				SelectedFinish: "foil",
				ProductData:    GNProductData{Set: "spm", SetName: "Marvel's Spider-Man"},
			},
			name:      "Eddie Brock",
			edition:   "Marvel's Spider-Man",
			variation: "233",
			foil:      true,
		},
		{
			// The storefront's typo for the manga label.
			game: matcher.GameOnePiece,
			product: GNProduct{
				DisplayName:    "Buggy (051) (Paralle) (OP09-051) Emperors in the New World Foil",
				SelectedFinish: "foil",
				ProductData:    GNProductData{SetName: "Emperors in the New World"},
			},
			name: "Buggy (051) (Manga)", edition: "Emperors in the New World",
			variation: "OP09-051", foil: true,
		},
		{
			// A DON!! card's finishing place is bracketed, not parenthesized.
			game: matcher.GameOnePiece,
			product: GNProduct{
				DisplayName: "DON!! Card (Tournament Pack Vol. 2) [Winner]  One Piece Promotion Cards",
				ProductData: GNProductData{SetName: "One Piece Promotion Cards"},
			},
			name: "DON!! Card", edition: "One Piece Promotion Cards",
			variation: "Tournament Pack Vol. 2 Winner",
		},
	}
	for _, tt := range tests {
		card, err := preprocess(b, tt.product, tt.game)
		if tt.err {
			if err == nil {
				t.Errorf("%s/%s: expected an error", tt.game, tt.product.DisplayName)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s/%s: unexpected error %v", tt.game, tt.product.DisplayName, err)
			continue
		}
		if card.Name != tt.name || card.Edition != tt.edition ||
			card.Variation != tt.variation || card.Finish != tt.finish || card.Foil != tt.foil {
			t.Errorf("%s/%s: got %q/%q/%q/%q/%v; want %q/%q/%q/%q/%v",
				tt.game, tt.product.DisplayName,
				card.Name, card.Edition, card.Variation, card.Finish, card.Foil,
				tt.name, tt.edition, tt.variation, tt.finish, tt.foil)
		}
	}
}

// sizedDatastore holds a Burger King promo whose printed size belongs to the
// set it was first printed in, two cosmos holo promos of one set and number
// told apart by their own size, and the Evolving Skies card a cosmos holo
// promo of the miscellaneous shelf is cut from, and a plain 1st Edition so the
// datastore sells that finish, rows copied verbatim.
const sizedDatastore = `{"data": {
 "game": "pokemon",
 "sets": {
  "BKP": {"abbreviation": "BKP", "baseSetSize": 122, "name": "XY - BREAKpoint", "releaseDate": "2016-02-03"},
  "BKP-2175": {"abbreviation": "BKP", "name": "Burger King Promos", "releaseDate": "2008-07-07", "type": "promo"},
  "FO": {"abbreviation": "FO", "baseSetSize": 62, "name": "Fossil", "releaseDate": "1999-10-10"},
  "LA": {"abbreviation": "LA", "baseSetSize": 146, "name": "Legends Awakened", "releaseDate": "2008-08-20"},
  "MCAP": {"abbreviation": "MCAP", "name": "Miscellaneous Cards & Products", "releaseDate": "1999-01-09"},
  "N4": {"abbreviation": "N4", "baseSetSize": 105, "name": "Neo Destiny", "releaseDate": "2002-02-28"},
  "ROS": {"abbreviation": "ROS", "baseSetSize": 108, "name": "XY - Roaring Skies", "releaseDate": "2015-05-06"},
  "SWSH07": {"abbreviation": "SWSH07", "baseSetSize": 203, "name": "SWSH07: Evolving Skies", "releaseDate": "2021-08-27"}
 },
 "cards": [
  {"externalLinks": {"tcgPlayerId": 155609}, "finish": "Reverse Holofoil", "id": "052-123_155609_reverseholofoil", "name": "Happiny", "number": "052", "rarity": "Promo", "setCode": "BKP-2175", "total": "123", "type": "Colorless", "variant": "Diamond & Pearl", "watermark": "diamond & pearl"},
  {"externalLinks": {"tcgPlayerId": 220117}, "finish": "Holofoil", "id": "013-098_220117_holofoil", "name": "Flareon", "number": "013", "promoTypes": ["cosmosholo"], "rarity": "Promo", "setCode": "MCAP", "total": "098", "type": "Fire", "variant": "Cosmos Holo"},
  {"externalLinks": {"tcgPlayerId": 654775}, "finish": "Holofoil", "id": "013-131_654775_holofoil", "name": "Flareon", "number": "013", "promoTypes": ["cosmosholo"], "rarity": "Rare", "setCode": "MCAP", "total": "131", "type": "Fire", "variant": "Cosmos Holo"},
  {"externalLinks": {"tcgPlayerId": 98141, "tcgdexId": "xy6-105"}, "finish": "Holofoil", "id": "105-108_98141_holofoil", "name": "M Rayquaza EX", "number": "105", "promoTypes": ["fullart"], "rarity": "Ultra Rare", "setCode": "ROS", "total": "108", "type": "Colorless", "types": ["Colorless"], "variant": "Full Art"},
  {"externalLinks": {"tcgPlayerId": 246875, "tcgdexId": "swsh7-82"}, "finish": "Holofoil", "id": "082-203_246875_holofoil", "name": "Galarian Zapdos", "number": "082", "rarity": "Holo Rare", "setCode": "SWSH07", "total": "203", "type": "Fighting", "types": ["Fighting"]},
  {"externalLinks": {"tcgPlayerId": 84534, "tcgdexId": "dp6-90"}, "finish": "Normal", "id": "90-146_84534", "name": "Cubone", "number": "90", "rarity": "Common", "setCode": "LA", "total": "146", "type": "Fighting", "types": ["Fighting"]},
  {"externalLinks": {"tcgPlayerId": 84534, "tcgdexId": "dp6-90"}, "finish": "Reverse Holofoil", "id": "90-146_84534_reverseholofoil", "name": "Cubone", "number": "90", "rarity": "Common", "setCode": "LA", "total": "146", "type": "Fighting", "types": ["Fighting"]},
  {"externalLinks": {"tcgPlayerId": 84660, "tcgdexId": "neo4-11"}, "finish": "1st Edition Holofoil", "id": "011-105_84660_1steditionholofoil", "name": "Dark Tyranitar", "number": "011", "rarity": "Holo Rare", "setCode": "N4", "total": "105", "type": "Fighting", "types": ["Fighting"]},
  {"externalLinks": {"tcgPlayerId": 84660, "tcgdexId": "neo4-11"}, "finish": "Unlimited Holofoil", "id": "011-105_84660_unlimitedholofoil", "name": "Dark Tyranitar", "number": "011", "rarity": "Holo Rare", "setCode": "N4", "total": "105", "type": "Fighting", "types": ["Fighting"]},
  {"externalLinks": {"tcgPlayerId": 44419, "tcgdexId": "base3-10"}, "finish": "1st Edition Holofoil", "id": "10-62_44419_1steditionholofoil", "name": "Lapras", "number": "10", "rarity": "Holo Rare", "setCode": "FO", "total": "62", "type": "Water", "types": ["Water"]},
  {"externalLinks": {"tcgPlayerId": 44419, "tcgdexId": "base3-10"}, "finish": "Unlimited Holofoil", "id": "10-62_44419_unlimitedholofoil", "name": "Lapras", "number": "10", "rarity": "Holo Rare", "setCode": "FO", "total": "62", "type": "Water", "types": ["Water"]},
  {"externalLinks": {"tcgPlayerId": 44425, "tcgdexId": "base3-17"}, "finish": "1st Edition", "id": "17-62_44425_1stedition", "name": "Articuno", "number": "17", "rarity": "Rare", "setCode": "FO", "total": "62", "type": "Water", "types": ["Water"]}
 ]
}}`

// TestResolveProductSizedNumber pins that a dashed listing whose printed size
// names no printing is asked for again by its number alone, and that a size
// naming one is read as written.
func TestResolveProductSizedNumber(t *testing.T) {
	b, err := matcher.Open("pokemon", strings.NewReader(sizedDatastore))
	if err != nil {
		t.Fatal(err)
	}
	gn, err := NewScraper(b)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		displayName string
		setName     string
		finish      string
		uuid        string
	}{
		{"Happiny - 52/124 [Diamond & Pearl] 52 - Burger King Promos Reverse Holofoil", "Burger King Promos", "Reverse Holofoil", "052-123_155609_reverseholofoil"},
		{"Flareon - 13/131 (Cosmos Holo) 13 - Miscellaneous Cards  Products Holofoil", "Miscellaneous Cards & Products", "Holofoil", "013-131_654775_holofoil"},
		{"Flareon - 13/98 (Cosmos Holo) 13 - Miscellaneous Cards  Products Holofoil", "Miscellaneous Cards & Products", "Holofoil", "013-098_220117_holofoil"},
		// The shelf holds no Galarian Zapdos, and the card of the set it is
		// cut from is not it.
		{"Galarian Zapdos - 082/203 (Cosmos Holo) 82 - Miscellaneous Cards  Products Holofoil", "Miscellaneous Cards & Products", "Holofoil", ""},
	}
	for _, tt := range tests {
		product := GNProduct{
			DisplayName:    tt.displayName,
			SelectedFinish: tt.finish,
			ProductData:    GNProductData{SetName: tt.setName},
		}
		uuid, err := gn.resolveProduct(modeBuylist, product)
		if err != nil || uuid != tt.uuid {
			t.Errorf("%q: got %q (%v); want %q", tt.displayName, uuid, err, tt.uuid)
		}
	}
}

// TestResolveProductPokemonStatedFinish pins that the finish a display name
// ends its shelf on places the listing where the storefront's selectedFinish
// says another, that a finish the card was never printed in leaves it alone,
// and that a print run nobody made is no printing, unless the datastore sells
// no such finish at all.
func TestResolveProductPokemonStatedFinish(t *testing.T) {
	b, err := matcher.Open("pokemon", strings.NewReader(sizedDatastore))
	if err != nil {
		t.Fatal(err)
	}
	gn, err := NewScraper(b)
	if err != nil {
		t.Fatal(err)
	}

	// The same rows without Articuno, the one plain 1st Edition.
	head, row, found := strings.Cut(sizedDatastore, `,
  {"externalLinks": {"tcgPlayerId": 44425`)
	if !found {
		t.Fatal("no plain 1st Edition row to trim")
	}
	_, tail, _ := strings.Cut(row, "\n")
	b, err = matcher.Open("pokemon", strings.NewReader(head+"\n"+tail))
	if err != nil {
		t.Fatal(err)
	}
	trimmed, err := NewScraper(b)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		scraper     *Gamenerdz
		displayName string
		setName     string
		finish      string
		uuid        string
	}{
		{gn, "Cubone 90/146 - Legends Awakened Reverse Holofoil", "Legends Awakened", "Normal", "90-146_84534_reverseholofoil"},
		{gn, "Cubone 90/146 - Legends Awakened Reverse Holofoil (LP)", "Legends Awakened", "Reverse Holofoil", "90-146_84534_reverseholofoil"},
		{gn, "Cubone 90/146 - Legends Awakened", "Legends Awakened", "Normal", "90-146_84534"},
		{gn, "Dark Tyranitar 11/105 - Neo Destiny 1st Edition Holofoil", "Neo Destiny", "Unlimited Holofoil", "011-105_84660_1steditionholofoil"},
		{gn, "Dark Tyranitar 11/105 - Neo Destiny Unlimited Holofoil", "Neo Destiny", "Unlimited Holofoil", "011-105_84660_unlimitedholofoil"},
		{gn, "Lapras 10/62 - Fossil 1st Edition", "Fossil", "1st Edition", ""},
		{trimmed, "Lapras 10/62 - Fossil 1st Edition", "Fossil", "1st Edition", "10-62_44419_unlimitedholofoil"},
		{gn, "Flareon - 13/131 (Cosmos Holo) 13 - Miscellaneous Cards  Products Holofoil", "Miscellaneous Cards & Products", "Holofoil", "013-131_654775_holofoil"},
	}
	for _, tt := range tests {
		product := GNProduct{
			DisplayName:    tt.displayName,
			SelectedFinish: tt.finish,
			ProductData:    GNProductData{SetName: tt.setName},
		}
		uuid, err := tt.scraper.resolveProduct(modeBuylist, product)
		if err != nil || uuid != tt.uuid {
			t.Errorf("%q: got %q (%v); want %q", tt.displayName, uuid, err, tt.uuid)
		}
	}
}

// TestResolveProductPokemonByRetailID pins that a listing the wording leaves
// between two printings is placed by the id its retail product carried: both
// Flareon are the cosmos holo 13 of one shelf, told apart only by the id.
func TestResolveProductPokemonByRetailID(t *testing.T) {
	b, err := matcher.Open("pokemon", strings.NewReader(sizedDatastore))
	if err != nil {
		t.Fatal(err)
	}
	gn, err := NewScraper(b)
	if err != nil {
		t.Fatal(err)
	}

	display := "Flareon - 13 (Cosmos Holo) 13 - Miscellaneous Cards  Products Holofoil"
	retail := GNProduct{
		ID:             "flareon",
		DisplayName:    display,
		SelectedFinish: "Holofoil",
		ProductData:    GNProductData{SetName: "Miscellaneous Cards & Products", TCGProductID: 654775},
	}
	buylist := retail
	buylist.ProductData.TCGProductID = 0

	uuid, err := gn.resolveProduct(modeBuylist, buylist)
	if err != nil || uuid != "" {
		t.Fatalf("without an id: got %q (%v); want the listing left unplaced", uuid, err)
	}
	_, err = gn.resolveProduct(modeRetail, retail)
	if err != nil {
		t.Fatal(err)
	}
	uuid, err = gn.resolveProduct(modeBuylist, buylist)
	if err != nil || uuid != "013-131_654775_holofoil" {
		t.Errorf("with its retail id: got %q (%v); want 013-131_654775_holofoil", uuid, err)
	}
}

// TestResolveProductPokemonIDNumberConflict pins that a retail id does not
// place a listing on a printing whose number is not the one the display name
// writes: Ancient Origins 98 is not Roaring Skies 105.
func TestResolveProductPokemonIDNumberConflict(t *testing.T) {
	b, err := matcher.Open("pokemon", strings.NewReader(sizedDatastore))
	if err != nil {
		t.Fatal(err)
	}
	gn, err := NewScraper(b)
	if err != nil {
		t.Fatal(err)
	}

	retail := GNProduct{
		ID:             "rayquaza",
		DisplayName:    "M Rayquaza-EX 98/98 - Ancient Origins Holofoil (MP)",
		SelectedFinish: "Holofoil",
		ProductData:    GNProductData{SetName: "XY - Roaring Skies", TCGProductID: 98141},
	}
	buylist := retail
	buylist.ProductData.TCGProductID = 0

	for _, mode := range []string{modeRetail, modeBuylist} {
		product := retail
		if mode == modeBuylist {
			product = buylist
		}
		uuid, err := gn.resolveProduct(mode, product)
		if err != nil || uuid != "" {
			t.Errorf("%s: got %q (%v); want the listing left unplaced", mode, uuid, err)
		}
	}
}

// TestResolveProductMagicSkips pins the listings that name no printing: a
// letterless product beside the lettered ones the set prints, and a storefront
// id known to sell a printing nobody made. The lettered sibling still lands.
func TestResolveProductMagicSkips(t *testing.T) {
	b := realDatastore(t)
	tests := []struct {
		id      string
		display string
		set     string
		setName string
		finish  string
		landed  bool
	}{
		{"bg5oG1xUUE", "Very Cryptic Command (UST-049) - Unstable Foil", "ust", "Unstable", "foil", false},
		{"xa36fGO5Kh", "Swinging Ship (UNF-231) - Unfinity Foil", "unf", "Unfinity", "foil", false},
		{"Yhb4Bmw66A", "Very Cryptic Command (UST-49F) - Unstable Foil", "ust", "Unstable", "foil", true},
		{"gNj4crD7UK", "Plains (0267) (WOE) (LIST-0267) - The List", "plst", "The List", "nonfoil", false},
	}
	for _, tt := range tests {
		gn, err := NewScraper(b)
		if err != nil {
			t.Fatal(err)
		}
		var logs []string
		gn.logCallback = func(format string, a ...any) {
			logs = append(logs, format)
		}
		product := GNProduct{
			ID:             tt.id,
			DisplayName:    tt.display,
			SelectedFinish: tt.finish,
			ProductData:    GNProductData{Set: jsonflex.String(tt.set), SetName: tt.setName},
		}
		got, err := gn.resolveProduct(modeBuylist, product)
		if err != nil {
			t.Errorf("%q: unexpected error %v", tt.display, err)
			continue
		}
		if (got != "") != tt.landed {
			t.Errorf("%q: got %q; want landed %v", tt.display, got, tt.landed)
		}
		if len(logs) != 0 {
			t.Errorf("%q: logged %v; want nothing", tt.display, logs)
		}
	}
}

// TestLetteredOnly pins that a promo pack and prerelease pair at one number
// is not read as the art letters of a parent product.
func TestLetteredOnly(t *testing.T) {
	b := realDatastore(t)
	for _, tt := range []struct {
		name, edition, number string
		want                  bool
	}{
		{"Very Cryptic Command", "Unstable", "49", true},
		{"Disallow", "Aether Revolt Promos", "31", false},
		{"Flumph", "Adventures in the Forgotten Realms Promos", "15", false},
	} {
		got := letteredOnly(b, &matcher.InputCard{Name: tt.name, Edition: tt.edition, Variation: tt.number})
		if got != tt.want {
			t.Errorf("letteredOnly(%s %s %s) = %t, want %t", tt.name, tt.edition, tt.number, got, tt.want)
		}
	}
}

// TestResolveProductRetailLinkedToAnotherCard pins that a retail id naming
// another card than the display is dropped when the sku's own number is not
// that card's, and still answers for a misspelled name whose number agrees,
// a List sku's number being its origin set and number together.
func TestResolveProductRetailLinkedToAnotherCard(t *testing.T) {
	b := realDatastore(t)
	gn, err := NewScraper(b)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		uuid    string
		display string
		sku     string
		finish  string
		landed  bool
	}{
		{"84952e72-e45d-57dd-8b29-9f2e53949724", "Gruul Guildgate (RNA-249) - Ravnica Allegiance", "MTG-RNA-249-6NUARTU9FH", "foil", false},
		{"f942bbc8-da13-5983-a60e-5c0f98852d73", "Airbender Lesson (TLA-008) - Avatar: The Last Airbender Foil", "MTG-TLA-008-F-8T1BMOCB1O", "foil", true},
		{"f71d2683-7f4c-584e-b5b8-dc87440b719c", "Treacherous Terain (LIST-C16-47) - The List", "MTG-PLST-C16-47-AK6D7ICGZV", "nonfoil", true},
	}
	for _, tt := range tests {
		co, err := b.GetUUID(tt.uuid)
		if err != nil {
			t.Fatal(err)
		}
		tcgID, err := strconv.ParseInt(co.Identifiers["tcgplayerProductId"], 10, 64)
		if err != nil {
			t.Fatalf("%s: %v", tt.uuid, err)
		}
		product := GNProduct{
			ID:             tt.sku,
			DisplayName:    tt.display,
			SelectedFinish: tt.finish,
			ProductData:    GNProductData{TCGProductID: tcgID},
			RetailVariants: []GNRetailVariant{{SKU: tt.sku}},
		}
		got, err := gn.resolveProduct(modeRetail, product)
		if err != nil {
			t.Errorf("%q: unexpected error %v", tt.display, err)
			continue
		}
		if (got != "") != tt.landed {
			t.Errorf("%q: got %q; want landed %v", tt.display, got, tt.landed)
		}
	}
}
