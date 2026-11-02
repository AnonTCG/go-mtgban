package yugioh

import (
	"encoding/json"
	"flag"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

var updateYugioh = flag.Bool("update-yugioh", false, "regenerate the Yu-Gi-Oh golden test data")

const yugiohTestData = "testdata/yugioh_test_data.json"

type matchTest struct {
	Desc string            `json:"description"`
	In   matcher.InputCard `json:"input"`
	ID   string            `json:"uuid"`
	Err  string            `json:"error,omitempty"`
}

// The seeds cover the storefront shapes the scrapers feed: cardtrader's
// clean name beside a bare collector number with its lowercase rarity tail
// (and the further "a" its alternate arts carry), TCGplayer's rarity and
// artwork parentheticals, the print-run wording that routes to its run's
// entry without ever gating, the identifier index the TCGplayer feed
// resolves through, and the run named beside that id.
var yugiohSeeds = []matchTest{
	{
		Desc: "clean name with the full collector number",
		In:   matcher.InputCard{Name: "Blue-Eyes White Dragon", Variation: "LOB-001"},
	},
	{
		Desc: "language infix on the input number is ignored within the edition",
		In:   matcher.InputCard{Name: "Blue-Eyes White Dragon", Variation: "LOB-EN001", Edition: "The Legend of Blue Eyes White Dragon"},
	},
	{
		Desc: "negative: sibling sets sharing the exact number alias without an edition",
		In:   matcher.InputCard{Name: "Blue-Eyes White Dragon", Variation: "LOB-EN001"},
	},
	{
		Desc: "1st edition wording routes to its print run's entry",
		In:   matcher.InputCard{Name: "Blue-Eyes White Dragon", Variation: "LOB-001 1st Edition"},
	},
	{
		Desc: "1st edition finish drops a date reprint that never had the run",
		In:   matcher.InputCard{Name: "Minerva, Lightsworn Maiden", Edition: "Structure Deck: The Realm of Light", Variation: "002 Super Rare", Finish: "1st Edition"},
	},
	{
		Desc: "1st edition wording drops a date reprint that never had the run",
		In:   matcher.InputCard{Name: "Minerva, Lightsworn Maiden", Variation: "SDLI-EN002 1st Edition"},
	},
	{
		Desc: "limited wording routes to the limited print run",
		In:   matcher.InputCard{Name: "Blizzed, Defender of the Ice Barrier", Variation: "HA01-EN001 Limited"},
	},
	{
		Desc: "unlimited wording is not read as limited",
		In:   matcher.InputCard{Name: "Blizzed, Defender of the Ice Barrier", Variation: "HA01-EN001 Unlimited"},
	},
	{
		Desc: "single product for the number needs no rarity signal",
		In:   matcher.InputCard{Name: "Dark Hole", Variation: "LOB-052"},
	},
	{
		Desc: "cardtrader rarity suffix picks the rarity",
		In:   matcher.InputCard{Name: "Eldlich the Golden Lord", Variation: "019qsec", Edition: "25th Anniversary Rarity Collection"},
	},
	{
		Desc: "cardtrader suffix keeps the base art",
		In:   matcher.InputCard{Name: "Eldlich the Golden Lord", Variation: "019u", Edition: "25th Anniversary Rarity Collection"},
	},
	{
		Desc: "cardtrader alternate-art tail drops the base art",
		In:   matcher.InputCard{Name: "Eldlich the Golden Lord", Variation: "019ua", Edition: "25th Anniversary Rarity Collection"},
	},
	{
		Desc: "tcgplayer rarity parenthetical picks the rarity",
		In:   matcher.InputCard{Name: "Eldlich the Golden Lord (Secret Rare)", Edition: "25th Anniversary Rarity Collection"},
	},
	{
		Desc: "nested rarity wording reads as the most specific rarity",
		In:   matcher.InputCard{Name: "Eldlich the Golden Lord (Quarter Century Secret Rare)", Edition: "25th Anniversary Rarity Collection"},
	},
	{
		Desc: "tcgplayer artwork parenthetical picks the variant",
		In:   matcher.InputCard{Name: "Harpie Lady (Original Artwork)", Variation: "MRD-008"},
	},
	{
		Desc: "storefront edition decorations strip away",
		In:   matcher.InputCard{Name: "Blue-Eyes White Dragon", Edition: "Yu-Gi-Oh! The Legend of Blue Eyes White Dragon Singles", Variation: "1st Edition"},
	},
	{
		Desc: "letter label beside the rarity wording picks its product",
		In:   matcher.InputCard{Name: "Dark Magician Girl (A) (Quarter Century Secret Rare)", Edition: "Quarter Century Bonanza"},
	},
	{
		Desc: "tcgplayer product id resolves through the identifier index",
		In:   matcher.InputCard{ID: "525011"},
	},
	{
		Desc: "the run named beside the id prices that run's entry",
		In:   matcher.InputCard{ID: "69074", Finish: "1st Edition"},
	},
	{
		Desc: "a second run reaches its own sibling of the same product",
		In:   matcher.InputCard{ID: "69074", Finish: "Limited"},
	},
	{
		Desc: "a foil flag names no run, and the wording answers instead",
		In:   matcher.InputCard{ID: "22708", Name: "Giant Flea", Variation: "TP1-017", Finish: "Foil"},
	},
	{
		Desc: "negative: a run the product was never priced in refuses the sibling",
		In:   matcher.InputCard{ID: "22708", Name: "Giant Flea", Variation: "TP1-017", Finish: "1st Edition"},
	},
	{
		Desc: "negative: rarity suffix cannot pick among the lettered artworks",
		In:   matcher.InputCard{Name: "Dark Magician Girl", Variation: "RA03-EN123qsec"},
	},
	// The two marks that are not an ink. Neither says what promoted the
	// printing; both say which printing of the number it is, and for these
	// two numbers they are the whole of what says it.
	{
		Desc: "a letter picks among the artworks the rarity cannot",
		In:   matcher.InputCard{Name: "Dark Magician Girl", Variation: "RA03-EN123qsec B"},
	},
	{
		Desc: "a version picks among the printings of one number and rarity",
		In: matcher.InputCard{Name: "Blue-Eyes White Dragon", Variation: "LDK2-ENK01 Version 4",
			Edition: "Legendary Decks II"},
	},
	{
		// Every version of these numbers is sold twice, once by the set
		// and once by its reprint, so the version says which printing
		// without saying which set - and the set is what is missing here.
		Desc: "negative: a version alone does not choose between a set and its reprint",
		In:   matcher.InputCard{Name: "Blue-Eyes White Dragon", Variation: "LCKC-EN001 Version 3"},
	},
	{
		// The advent calendars are printed in German and named in it, so
		// the name reaches one printing on its own and the language filter
		// has nothing to choose between.
		Desc: "a German printing is reached by the name it is printed under",
		In:   matcher.InputCard{Name: `Junk Synchron - "Gerumpelsynchronisierer"`, Variation: "AC11-DE001"},
	},
	{
		Desc: "negative: unknown card name",
		In:   matcher.InputCard{Name: "Nonexistent Imaginary Duelist", Variation: "LOB-001"},
	},
	{
		Desc: "negative: known name with wrong collector number",
		In:   matcher.InputCard{Name: "Blue-Eyes White Dragon", Variation: "LOB-999"},
	},
	// A name nothing answers to, beside a number only one card carries, is
	// still a card this reader can name: the number is the stronger key.
	{
		Desc: "an unshared number names the card its listing misspells",
		In:   matcher.InputCard{Name: "Doube-Edged Sword Technique", Edition: "Structure Deck Samurai Warlords", Variation: "SDWA-EN035 Common"},
	},
	{
		Desc: "a token the storefront names its own way answers to its number",
		In:   matcher.InputCard{Name: "Yami Token", Edition: "Tokens", Variation: "TKN4-EN029 Super Rare"},
	},
	// The catalog numbers the cards that share a name; a listing spells that
	// number without the mark.
	{
		Desc: "a numbered name reads the same with or without its mark",
		In:   matcher.InputCard{Name: "Sasuke Samurai 2", Edition: "Dark Revelation 1", Variation: "DR1-EN221 Common"},
	},
	{
		Desc: "negative: a character art card is the storefront's product, not a printing",
		In:   matcher.InputCard{Name: "Mai Valentine Character Art Card", Edition: "Tokens", Variation: "Super Rare"},
	},
	// A set decorates its tiers with a word of its own, and a storefront
	// writes the tier it knows: neither spells the whole of the other, and
	// what the wording did name is the word telling the two apart.
	{
		Desc: "a tier named in part picks the one printing that says all of it",
		In:   matcher.InputCard{Name: "Slifer the Sky Dragon", Edition: "King's Court", Variation: "KICO-EN063 Ultra Rare"},
	},
	// Cool Stuff Inc files every promo shelf it cannot otherwise place under
	// one catch-all "Promo" heading - a tin, a Legendary Collection, a
	// Quarter Century Rare reprint - naming no set of its own. Restricted to
	// it, the search would reach only the sets whose own name happens to
	// contain the word "Promo", which BPT-1341 does not; dropped, the
	// number is free to answer on its own.
	{
		Desc: "a heading naming no set of its own is dropped for the number to answer",
		In:   matcher.InputCard{Name: "Dark Magician", Edition: "Promo", Variation: "BPT001 BPT-001 Secret Rare"},
	},
	// LC01 (Legendary Collection 1) and its 25th Anniversary reprint spell
	// this number and rarity identically, but the reprint's own copy is
	// tagged "25th Anniversary Edition" and the original's is not - so a
	// bare "Ultra Rare", naming no tag, still means the untagged original
	// once the heading is out of the way.
	{
		Desc: "a bare rarity still means the plain printing over a tagged reprint",
		In:   matcher.InputCard{Name: "Blue-Eyes White Dragon", Edition: "Promo", Variation: "LC01-EN004 Ultra Rare"},
	},
	// A print-run twin reissued under its original's exact number and
	// rarity, tagged neither way - only the copyright date on the card
	// tells the two apart, which is the scraper's to read and not a
	// number's to guess.
	{
		Desc: "negative: a number two sets share names neither of them",
		In:   matcher.InputCard{Name: "Honest", Edition: "Promo", Variation: "LODT-EN001 Secret Rare"},
	},
	// Moja sells plainly at RGBT-EN084, but Cool Stuff Inc also files the
	// Duelist Pack Collection Tin reprint, numbered RGBT-ENPP4, under
	// "Raging Battle" beside it - a set that never printed that number.
	{
		Desc: "a named edition without the number is dropped for the number to answer",
		In:   matcher.InputCard{Name: "Moja", Edition: "Raging Battle", Variation: "RGBT-ENPP4 Ultra Rare Ultra Rare"},
	},
	// Konami numbers a printing by the language it was printed in, and this
	// datastore is mostly the English catalog - so a foreign number is a
	// sign of a printing it has no row for. A sign and not a rule: the
	// advent calendars are German and carried as German, so the number is
	// asked of the datastore rather than assumed about.
	{
		Desc: "a German printing this datastore carries is reached by its number",
		In:   matcher.InputCard{Name: "Nitrokrieger", Edition: "Promo", Variation: "AC11-DE021 Ultra Rare"},
	},
	{
		Desc: "negative: a German printing of a number carried only in English is not ours",
		In:   matcher.InputCard{Name: "Dark Magician", Edition: "Promo", Variation: "LOB-DE005"},
	},
	{
		// The sign is not always a language. The Falsebound Kingdom
		// numbers its two promos SP001 and SP002, English cards whose SP
		// the infix read as Spanish; asking the datastore answers this
		// the same way it answers the German ones.
		Desc: "a promo whose number only looks foreign is reached",
		In:   matcher.InputCard{Name: "Sinister Serpent", Variation: "TFK-SP001"},
	},
	// The storefront brackets what it elsewhere parenthesizes, and the
	// bracket says the same thing about the printing.
	{
		Desc: "a bracketed decoration reads as the parenthetical it is",
		In:   matcher.InputCard{Name: "Knightmare Unicorn [Alt Art]", Edition: "25th Anniversary Rarity Collection", Variation: "RA01-EN043 Platinum Secret Rare"},
	},
	// DL18 files one number as four colours and tags each with the league
	// it belongs to ("bluedl18"); a storefront writes the colour alone,
	// because the number beside it already said which league.
	{
		Desc: "a colour names the league printing its number already placed",
		In:   matcher.InputCard{Name: "Penguin Soldier", Edition: "Duelist League Promo", Variation: "DL18-EN002 Rare Blue"},
	},
	{
		Desc: "a second colour of the same number names its own printing",
		In:   matcher.InputCard{Name: "Penguin Soldier", Edition: "Duelist League Promo", Variation: "DL18-EN002 Rare Red"},
	},
	// The league's own number is written both padded and bare; the set is
	// the same one either way, so neither spelling has to be enumerated.
	{
		Desc: "a league written without its padding still names its set",
		In:   matcher.InputCard{Name: "Blue-Eyes White Dragon", Edition: "Duelist League 9", Variation: "DL09-EN001 Rare Blue"},
	},
	{
		Desc: "negative: a league printing named by no colour stays ambiguous",
		In:   matcher.InputCard{Name: "Crimson Ninja", Edition: "Duelist League 13", Variation: "DL13-EN004 Rare"},
	},
	{
		Desc: "negative: multi-rarity number stays ambiguous without a rarity signal",
		In:   matcher.InputCard{Name: "Diabellstar the Black Witch", Variation: "25LP-EN001"},
	},
	// DLCS-EN006 prints one number in eight products: a base art, three
	// colours, and the same four again as alternate arts. Only the tag the
	// colour is written beside tells the last four apart.
	{
		Desc: "a wording naming two tags reaches the printing wearing both",
		In:   matcher.InputCard{Name: "Dark Magician Girl the Dragon Knight", Variation: "DLCS-EN006 Alternate Art Blue"},
	},
	{
		Desc: "a wording naming one tag keeps the printing wearing only it",
		In:   matcher.InputCard{Name: "Dark Magician Girl the Dragon Knight", Variation: "DLCS-EN006 Blue"},
	},
	// The box the set is named for is what the storefront drops.
	{
		Desc: "a set named without the box it is sold in",
		In:   matcher.InputCard{Name: "Polymerization", Variation: "ENG11 Common", Edition: "Speed Duel GX: Duel Academy"},
	},
	// The deck letter is what the two printings differ by, and it only
	// decides once the set has spelled the tail out in full.
	{
		Desc: "a bare tail is spelled out by the set it names",
		In:   matcher.InputCard{Name: "Different Dimension Gate", Variation: "ENF17 Common Common", Edition: "Speed Duel GX: Duelists of Shadows"},
	},
	{
		Desc: "the sibling deck letter keeps its own printing",
		In:   matcher.InputCard{Name: "Different Dimension Gate", Variation: "ENG14 Common Common", Edition: "Speed Duel GX: Duelists of Shadows"},
	},
	// The catalog spells a mark, a word and a phrase order the storefront
	// writes another way; the last is reached only through its prefix, and
	// a character the catalog never paired decides nothing.
	{
		Desc: "the mark numbering a repeated name is read from the wording",
		In:   matcher.InputCard{Name: "Skull Knight No 2", Variation: "Common", Edition: "Legacy of Darkness"},
	},
	{
		Desc: "a letter the catalog writes as a word",
		In:   matcher.InputCard{Name: "Falchion\u03b2", Variation: "Common", Edition: "Starter Deck: Speed Duel - Battle City Box"},
	},
	{
		Desc: "a field center flips onto the phrase order the catalog files",
		In:   matcher.InputCard{Name: "Ash Blossom & Joyous Spring Field Center Token", Variation: "Cat ears and tail Common", Edition: "Tokens"},
	},
	{
		Desc: "a character's field center is reached through its prefix",
		In:   matcher.InputCard{Name: "Seto Kaiba Field Center Token", Variation: "Blue background w/ Blue-Eyes White Dragon Common", Edition: "Tokens"},
	},
	{
		Desc: "negative: a character the catalog pairs two ways decides nothing",
		In:   matcher.InputCard{Name: "Yugi Muto Field Center Token", Variation: "Yellow background w/ Dark Magician Common", Edition: "Tokens"},
	},
	// The mark is invisible, so the name it decorates reads as spelled.
	{
		Desc: "an invisible formatting mark is not part of the name",
		In:   matcher.InputCard{Name: "Pendulum Encore\u200e", Variation: "Common", Edition: "Blazing Vortex"},
	},
	// Konami renamed this card; the catalog still files Starstrike Blast's
	// printing under the name it was printed with.
	{
		Desc: "Darkfall respells to the name its own set prints",
		In:   matcher.InputCard{Name: "Darkfall", Edition: "Starstrike Blast", Variation: "080 Common"},
	},
	// The K misprint reissue carries EOJ's own prefix plus a tail letter;
	// read as a prefix it would answer with the base card's EOJ edition
	// instead of the Redemption Replacement row the K actually names.
	{
		Desc: "a letter-tailed misprint number is read verbatim over its prefix",
		In:   matcher.InputCard{Name: "Destiny Hero - Dreadmaster", Edition: "Misprints", Variation: "EOJ-EN004K"},
	},
	// The Rarity Collections sell every tier of a number decorated ("Alt
	// Art"), so the qualifier being named settles nothing about which of
	// the seven rarities sharing it is meant - the rarity still has its
	// own say once the qualifier's own words are out of the way.
	{
		Desc: "a decorated qualifier still lets the rarity narrow behind it",
		In:   matcher.InputCard{Name: "Eldlich the Golden Lord [Alt Art]", Edition: "25th Anniversary Rarity Collection", Variation: "Sitting Down - Ultimate Rare"},
	},
}

func TestYugiohMatch(t *testing.T) {
	b := gametest.Backend(t, matcher.GameYuGiOh)

	data, err := os.ReadFile(yugiohTestData)
	if err != nil {
		if *updateYugioh && os.IsNotExist(err) {
			data = []byte("[]")
		} else {
			t.Fatal(err)
		}
	}
	var tests []matchTest
	if err := json.Unmarshal(data, &tests); err != nil {
		t.Fatal(err)
	}

	if *updateYugioh {
		regenerateYugiohTestData(t, b, tests)
		return
	}

	if len(tests) == 0 {
		t.Fatal("no Yu-Gi-Oh test cases")
	}

	for _, tt := range tests {
		t.Run(tt.Desc, func(t *testing.T) {
			t.Parallel()
			in := tt.In
			id, err := b.Match(&in)
			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if id != tt.ID || gotErr != tt.Err {
				t.Errorf("Match(%q num=%q) = (%q, %q), want (%q, %q)",
					tt.In.Name, tt.In.Variation, id, gotErr, tt.ID, tt.Err)
			}
		})
	}
}

// regenerateYugiohTestData re-runs Match over every committed input plus
// the hand-authored seeds, bakes the resulting uuid/error, and rewrites the
// golden file sorted by description. Flipping a case between success and
// error aborts the rewrite unless its description owns the error with a
// "negative:" prefix: acknowledging a change of that magnitude requires
// editing the entry by hand.
func regenerateYugiohTestData(t *testing.T, b *matcher.Backend, tests []matchTest) {
	isSeed := map[string]bool{}
	for _, seed := range yugiohSeeds {
		isSeed[seed.Desc] = true
	}
	kept := tests[:0]
	for _, tt := range tests {
		if !isSeed[tt.Desc] {
			kept = append(kept, tt)
		}
	}
	tests = append(kept, yugiohSeeds...)

	for i := range tests {
		in := tests[i].In
		id, err := b.Match(&in)
		gotErr := ""
		if err != nil {
			gotErr = err.Error()
		}
		wasError := tests[i].Err != ""
		isError := gotErr != ""
		if (tests[i].ID != "" || tests[i].Err != "") && wasError != isError &&
			!strings.HasPrefix(tests[i].Desc, "negative:") {
			t.Fatalf("%s: flipped between success and error (%q/%q -> %q/%q); edit the entry by hand",
				tests[i].Desc, tests[i].ID, tests[i].Err, id, gotErr)
		}
		if tests[i].ID != id || tests[i].Err != gotErr {
			t.Logf("%s: (%q, %q) -> (%q, %q)", tests[i].Desc, tests[i].ID, tests[i].Err, id, gotErr)
		}
		tests[i].ID = id
		tests[i].Err = gotErr
	}

	sort.Slice(tests, func(i, j int) bool {
		return tests[i].Desc < tests[j].Desc
	})
	data, err := json.MarshalIndent(tests, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(yugiohTestData, append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("rewrote %s with %d cases", yugiohTestData, len(tests))
}

// TestYugiohSealed pins the sealed namespace: products load, resolve by
// name, and never shadow the card index.
func TestYugiohSealed(t *testing.T) {
	b := gametest.Backend(t, matcher.GameYuGiOh)

	if len(b.AllSealedUUIDs) == 0 {
		t.Fatal("no sealed products loaded")
	}
	productMap := b.BuildSealedProductMap("tcgplayerProductId")
	if len(productMap) == 0 {
		t.Fatal("no sealed product ids")
	}
	for id, uuids := range productMap {
		if len(uuids) != 1 {
			t.Errorf("tcgplayer id %d shared by %d sealed products", id, len(uuids))
		}
	}

	uuid, err := b.ResolveSealed("Force of the Breaker - Booster Box [1st Edition]")
	if err != nil {
		t.Fatalf("booster box did not resolve: %s", err)
	}
	co, err := b.GetUUID(uuid)
	if err != nil || !co.Sealed {
		t.Fatalf("resolved uuid %s is not a sealed product", uuid)
	}

	// The booster box is a real product per print run (1st Edition and
	// Unlimited); a wording naming neither must stay unresolved rather
	// than pick one.
	if uuid, err := b.ResolveSealed("Force of the Breaker - Booster Box"); err == nil {
		t.Fatalf("edition-ambiguous booster box resolved to %s", uuid)
	}
}
