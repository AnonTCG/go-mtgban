package onepiece

import (
	"flag"
	"strings"
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

var updateOnepiece = flag.Bool("update-onepiece", false,
	"rewrite testdata/onepiece_test_data.json from what Match returns")

// The seeds cover the storefront shapes the scrapers feed: cardtrader's
// clean name beside a full collector number (with the letter tail its
// alternate arts carry), cardmarket's "(V.n)" positional wording,
// coolstuffinc's bare-tail numbers with qualifier parentheticals, and the
// identifier index the TCGplayer feed resolves through.
var onepieceSeeds = []gametest.Case{
	{
		Desc: "clean name with the full collector number",
		In:   matcher.InputCard{Name: "Roronoa Zoro", Variation: "OP01-025"},
	},
	{
		Desc: "epithet parenthetical is part of the name",
		In:   matcher.InputCard{Name: "Mr.2.Bon.Kurei (Bentham)", Variation: "OP01-084"},
	},
	{
		Desc: "storefront wording picks the variant it describes",
		In:   matcher.InputCard{Name: "Nami (OP01-016) (Manga)"},
	},
	{
		Desc: "cardmarket V.1 keeps the base printing",
		In:   matcher.InputCard{Name: "Mr.2.Bon.Kurei (Bentham) (OP01-084) (V.1)"},
	},
	// A number printing one variant: OP01-084 also prints a pre-errata
	// Mr.2 beside its promo, so "some variant" is two there.
	{
		Desc: "cardmarket V.2 demands the variant",
		In:   matcher.InputCard{Name: "Mr.2.Bon.Kurei (Bentham) (OP14-091) (V.2)"},
	},
	{
		Desc: "cardtrader letter tail demands a variant",
		In:   matcher.InputCard{Name: "Mr.2.Bon.Kurei (Bentham)", Variation: "OP14-091a"},
	},
	{
		Desc: "bare tail number narrows within the edition",
		In:   matcher.InputCard{Name: "Mr.2.Bon.Kurei (Bentham)", Variation: "084", Edition: "Romance Dawn"},
	},
	{
		Desc: "a number with a slash in it is asked for whole",
		In:   matcher.InputCard{Name: "Monkey.D.Luffy", Variation: "1/1000 Certificate of our crew Oversized", Edition: "One Piece Promotion Cards"},
	},
	{
		Desc: "tcgplayer product id resolves through the identifier index",
		In:   matcher.InputCard{ID: "454615"},
	},
	{
		Desc: "coolstuffinc edition wears the set code before a dash",
		In:   matcher.InputCard{Name: "Brannew", Edition: "OP03 - Pillars of Strength", Variation: "OP03-089"},
	},
	{
		Desc: "dash-number promo name resolves through its qualifier",
		In:   matcher.InputCard{Name: "Monkey.D.Luffy - P-043 (Convention Promo 2024)", Edition: "Promo"},
	},
	{
		Desc: "set-qualified number refuses another set's same tail",
		In:   matcher.InputCard{Name: "Ain", Variation: "OP07-002"},
	},
	{
		Desc: "promo line names the set's event printings",
		In:   matcher.InputCard{Name: "Adio (OP03-002)", Variation: "002", Edition: "Promos: Pillars of Strength"},
	},
	{
		Desc: "a promo marker naming no set keeps the regular printing",
		In:   matcher.InputCard{Name: "Dogura", Variation: "OP02-010", Edition: "Promos:"},
	},
	{
		Desc: "a promo marker trailing punctuation names no set either",
		In:   matcher.InputCard{Name: "Dogura", Variation: "OP02-010", Edition: "Promos: -"},
	},
	{
		Desc: "a promo marker trailing a possessive names no set either",
		In:   matcher.InputCard{Name: "Dogura", Variation: "OP02-010", Edition: "Promo: 's"},
	},
	{
		Desc: "the set itself keeps the regular printing",
		In:   matcher.InputCard{Name: "Adio (OP03-002)", Variation: "002", Edition: "Pillars of Strength"},
	},
	{
		Desc: "an edition naming neither set keeps the regular printing",
		In:   matcher.InputCard{Name: "Ain (OP07-002)", Variation: "002", Edition: "500 Years into the Future"},
	},
	{
		Desc: "the set code picks the volume its family shares a name with",
		In:   matcher.InputCard{Name: "Roronoa Zoro", Variation: "OP06-118", Edition: "PRB-02: Premium Booster"},
	},
	{
		Desc: "negative: a family name without its volume stays ambiguous",
		In:   matcher.InputCard{Name: "Roronoa Zoro", Variation: "OP06-118", Edition: "Premium Booster"},
	},
	{
		Desc: "the set code outranks a wording naming the other volume",
		In:   matcher.InputCard{Name: "Roronoa Zoro", Variation: "OP06-118", Edition: "PRB-02: Premium Booster -The Best-"},
	},
	{
		Desc: "an event set spelled a word off selects the event printing",
		In:   matcher.InputCard{Name: "Sterry", Variation: "OP07-006", Edition: "500 Years into the Future Pre-Release Cards"},
	},
	{
		Desc: "a code short of the wording does not outrank it",
		In:   matcher.InputCard{Name: "Bepo", Variation: "OP14-012", Edition: "OP14 - The Azure Sea's Seven Release"},
	},
	{
		Desc: "the same code with no marker spelled keeps the base set",
		In:   matcher.InputCard{Name: "Bepo", Variation: "OP14-012", Edition: "OP14 - The Azure Sea's Seven"},
	},
	{
		Desc: "a code naming another set yields to a wording naming one",
		In:   matcher.InputCard{Name: "Kouzuki Oden", Variation: "EB01-001", Edition: "EB-02 - Extra Booster: Memorial Collection"},
	},
	{
		Desc: "a word appended behind the coded name keeps the coded set",
		In:   matcher.InputCard{Name: "Adio", Variation: "OP03-002", Edition: "OP03 - Pillars of Strength Cards"},
	},
	{
		Desc: "the same set reached by spelling its marker in order",
		In:   matcher.InputCard{Name: "Adio", Variation: "OP03-002", Edition: "OP03 - Pillars of Strength Pre-Release"},
	},
	{
		Desc: "a word appended past a family name keeps the coded volume",
		In:   matcher.InputCard{Name: "Monkey.D.Luffy", Variation: "OP01-024", Edition: "PRB-01 - Premium Booster -The Best- Promo"},
	},
	{
		Desc: "a family name cut short still resolves through its code",
		In:   matcher.InputCard{Name: "Lucky.Roux", Variation: "PRB02-003", Edition: "PRB-02: Premium Booster"},
	},
	{
		Desc: "an event set missing its deck ordinal still selects it",
		In:   matcher.InputCard{Name: "Usopp", Variation: "ST01-002", Edition: "Super Pre-Release Starter Deck: Straw Hat Crew"},
	},
	{
		Desc: "a wording spelling no event marker keeps the base set",
		In:   matcher.InputCard{Name: "Blast Breath", Variation: "ST04-016", Edition: "Animal Kingdom Pirates: Starter Deck 4"},
	},
	{
		Desc: "storefront drops the deck ordinal out of the set name",
		In:   matcher.InputCard{Name: "Blast Breath (ST04-016)", Variation: "016", Edition: "Starter Deck: Animal Kingdom Pirates"},
	},
	{
		Desc: "a set name spelled a word off still selects its set",
		In:   matcher.InputCard{Name: "Basil Hawkins (OP07-029)", Variation: "029", Edition: "500 Years into the Future"},
	},
	{
		Desc: "a promo line spelled a word off still selects the event set",
		In:   matcher.InputCard{Name: "Aladine (OP07-020)", Variation: "020", Edition: "Promos: 500 Years into the Future"},
	},
	{
		Desc: "bare number hung off the name after a dash",
		In:   matcher.InputCard{Name: `"Buddha" Sengoku - 077`, Variation: "OP16-077", Edition: "OP16 - The Time Of Battle"},
	},
	{
		Desc: "bare number hung off the name before a qualifier",
		In:   matcher.InputCard{Name: "Trafalgar Law - 047 (Parallel)", Edition: "OP01 - Romance Dawn"},
	},
	{
		Desc: "a qualifier carrying an ordinal does not become the number",
		In:   matcher.InputCard{Name: "Killer - 106 (Judge Pack Vol. 5)", Edition: "One Piece Promotion Cards"},
	},
	{
		Desc: "the ordinal a promo label opens with is not the number either",
		In:   matcher.InputCard{Name: "Sanji - 013 (English Version 1st Anniversary Set)", Edition: "One Piece Promotion Cards"},
	},
	{
		Desc: "negative: a vendor bucket edition selects no set",
		In:   matcher.InputCard{Name: "Arlong (OP06-023)", Variation: "023", Edition: "One Piece Products"},
	},
	{
		Desc: "negative: a foreign set code refuses another set's tail",
		In:   matcher.InputCard{Name: "Dogura", Variation: "ST05-010", Edition: "OP02 - Paramount War"},
	},
	{
		Desc: "negative: unknown card name",
		In:   matcher.InputCard{Name: "Nonexistent Imaginary Pirate", Variation: "OP01-001"},
	},
	{
		Desc: "negative: known name with wrong collector number",
		In:   matcher.InputCard{Name: "Roronoa Zoro", Variation: "OP01-999"},
	},
	{
		Desc: "negative: reprinted base printings stay ambiguous on a bare input",
		In:   matcher.InputCard{Name: "Nami", Variation: "OP01-016"},
	},
	{
		// The bare number is the same number in every set, and OP11 files a
		// card at -118 whose label is literally "Manga" where OP13 spells
		// the same treatment "Super Alternate Art". The edition names OP13
		// and must keep it.
		Desc: "a bare number does not let another set's label unpin the edition",
		In: matcher.InputCard{Name: "Monkey.D.Luffy",
			Variation: "118 Manga",
			Edition:   "Carrying On His Will", Foil: true},
	},
	{
		Desc: "and the printings filed under the base card's own number still answer",
		In: matcher.InputCard{Name: "Nami",
			Variation: "OP01-016 Manga",
			Edition:   "Romance Dawn", Foil: true},
	},
	{
		Desc: "a later set's reprint is reached by the code the wording names",
		In: matcher.InputCard{Name: "Monkey.D.Luffy",
			Variation: "OP07-109 OP08 Treasure Rare",
			Edition:   "OP07 - 500 Years in the Future"},
	},
	{
		Desc: "a code naming a set of many printings leaves the wording to choose",
		In: matcher.InputCard{Name: "Brannew",
			Variation: "OP03-089 PRB01 Alternate Art",
			Edition:   "OP03 - Pillars of Strength"},
	},
	{
		Desc: "the manga art crossed with a parallel is not the plain parallel",
		In: matcher.InputCard{Name: "Monkey.D.Luffy",
			Variation: "OP13-118 118 Alternate Art Manga Red Parallel",
			Edition:   "One Piece"},
	},
	{
		Desc: "and the catalog's own crossed label answers the same wording",
		In: matcher.InputCard{Name: "Shanks",
			Variation: "OP01-120 120 Parallel Alternate Art Manga",
			Edition:   "One Piece"},
	},
	{
		Desc: "manga beside a plain alternate art keeps the manga",
		In: matcher.InputCard{Name: "Monkey.D.Luffy",
			Variation: "EB02-061 Alternate Art Manga",
			Edition:   "One Piece"},
	},
	{
		Desc: "the place a family differs by answers when the wording names it",
		In: matcher.InputCard{Name: "Capone\"Gang\"Bege",
			Variation: "OP04-100w Championship 2023 | Winner",
			Edition:   "Championships Promo"},
	},
	{
		Desc: "negative: two events awarding one place settle nothing",
		In: matcher.InputCard{Name: "Gum-Gum Jet Pistol",
			Variation: "ST01-015f Regional 2024 | Finalist",
			Edition:   "Store Tournaments Promos"},
	},
	{
		Desc: "a letter tail no label answers falls to the edition",
		In: matcher.InputCard{Name: "Four Thousand-Brick Fist",
			Variation: "OP05-020s 1st Anniversary Stamp",
			Edition:   "OP-05: 1st Anniversary Tournament Cards"},
	},
	{
		Desc: "the same, for a starter deck's pre-release printings",
		In: matcher.InputCard{Name: "Edward Weevil",
			Variation: "ST03-002p Super Pre-Release Stamp",
			Edition:   "ST-03: Starter Deck: The Seven Warlords of the Sea"},
	},
	{
		Desc: "corrected run named by the letter on the number",
		In: matcher.InputCard{Name: "Trafalgar Law",
			Variation: `OP01-047α Alpha Pre-Errata Card | "1 Character to your hand: Play 1 Character card"`,
			Edition:   "Pre-Errata Cards"},
	},
	{
		Desc: "the art word picks the treated printing of that same run",
		In: matcher.InputCard{Name: "Trafalgar Law",
			Variation: `OP01-047αa Alternate Art | Alpha Pre-Errata Card | "1 Character to your hand: Play 1 Character card"`,
			Edition:   "Pre-Errata Cards"},
	},
	{
		Desc: "a corrected printing is not the base card at its number",
		In: matcher.InputCard{Name: "Officer Agents",
			Variation: `OP01-087α Alpha Pre-Errata Card | "Play 1 {Baroque Works} type card"`,
			Edition:   "Pre-Errata Cards"},
	},
	{
		Desc: "the letter tail names the alternate art the version text leaves out",
		In: matcher.InputCard{Name: "Dracule Mihawk",
			Variation: `OP01-070ae Pre-Errata Card | Missing Text "Up to"`,
			Edition:   "Pre-Errata Cards"},
	},
	{
		Desc: "a letter tail without the art letter is plain whatever the text says",
		In: matcher.InputCard{Name: "Dracule Mihawk",
			Variation: `OP01-070e Alternate Art | Pre-Errata Card | Missing Text "Up to"`,
			Edition:   "Pre-Errata Cards"},
	},
	{
		Desc: "the alternate art the catalog files as a box topper",
		In: matcher.InputCard{Name: "Alvida",
			Variation: `OP01-064a Alternate Art | Pre-Errata Card | Missing text "Up to"`,
			Edition:   "Pre-Errata Cards"},
	},
	{
		Desc: "a treatment the wording spells out answers before the art word",
		In: matcher.InputCard{Name: "Monkey.D.Luffy",
			Variation: "OP01-003d Pre-Errata Card | Demo Deck Map Design",
			Edition:   "Pre-Errata Cards"},
	},
	{
		Desc: "the word's other sense keeps the base printing it names",
		In: matcher.InputCard{Name: "Marshall.D.Teach",
			Variation: `ST03-014 1st Print Errata Card | "Return 1 Character"`,
			Edition:   "ST03 - The Seven Warlords of The Sea"},
	},
	{
		Desc: "cardmarket V.1 on a Premium Booster shelf means the plain Reprint",
		In:   matcher.InputCard{Name: "Arlong", Variation: "OP06-023 V.1", Edition: "The Best"},
	},
	{
		Desc: "negative: a Premium Booster V.1 refuses where no Reprint row exists",
		In:   matcher.InputCard{Name: "Trafalgar Law", Variation: "OP01-047 V.1", Edition: "The Best"},
	},
	{
		Desc: "cardmarket V.1 on a Premium Booster shelf also means an untagged row",
		In:   matcher.InputCard{Name: "Koby", Variation: "PRB02-001 V.1", Edition: "The Best Vol.2"},
	},
	{
		Desc: "TCGplayer files the Learn Together non-foil under Premium Booster Vol. 2",
		In:   matcher.InputCard{Name: "Roronoa Zoro", Variation: "OP06-118 Non-Foil", Edition: "Learn Together Deck Set"},
	},
	{
		Desc: "the Learn Together fold applies to its second numbered card too",
		In:   matcher.InputCard{Name: "Basil Hawkins", Variation: "OP10-109 Non-Foil", Edition: "Learn Together Deck Set"},
	},
	{
		Desc: "a PRB-coded bracket narrows the edition even where the code holds two printings",
		In: matcher.InputCard{Name: "Baby 5 - 034 (PRB01 Alternate Art)",
			Variation: "OP05-034 PRB01 Alternate Art",
			Edition:   "OP05 - Awakening of the New Era", Foil: true},
	},
	{
		Desc: "a bare Alternate Art wording keeps the plain treatment over a crossed one",
		In: matcher.InputCard{Name: "Sabo - 120 (Alternate Art)",
			Variation: "OP13-120 Alternate Art",
			Edition:   "OP13 - Carrying On His Will", Foil: true},
	},
	{
		Desc: "a Store Championship's Participation Pack tag does not stand for a Regional's",
		In: matcher.InputCard{Name: "Sanji - 102 (Offline Regional Participation Pack 2024 Vol. 1)",
			Variation: "OP03-102 Offline Regionals 24 Stamp Fire Kick",
			Edition:   "OP03 - Pillars of Strength", Foil: true},
	},
	{
		Desc: "the wording's own words break a tie between two families sharing a place",
		In: matcher.InputCard{Name: "Brannew - 089 (Offline Regional Participation Pack 2024 Vol. 1)",
			Variation: "OP03-089 Offline Regional Participation Pack 2024 Vol. 1",
			Edition:   "OP03 - Pillars of Strength", Foil: true},
	},
	{
		Desc: "a bracket's Starter Deck NN reads as the code it names",
		In: matcher.InputCard{Name: "Monkey.D.Luffy - 001 (Starter Deck 31)",
			Variation: "ST21-001 Starter Deck 31",
			Edition:   "ST21 - Starter Deck: Gear5", Foil: true},
	},
	{
		Desc: "the shelf's own unspaced marker still narrows without a bracket",
		In: matcher.InputCard{Name: "Monkey.D.Luffy - 001",
			Variation: "ST21-001 Red Background Left Hand Open",
			Edition:   "ST21 - Starter Deck: Gear5", Foil: true},
	},
	{
		Desc: "a bracket naming the Learn Together Deck Set outright finds its own row",
		In: matcher.InputCard{Name: "Monkey.D.Luffy - 001 (Learn Together Deck Set)",
			Variation: "ST21-001 Hat Floating Behind Learn Together Deck Set",
			Edition:   "ST21 - Starter Deck: Gear5", Foil: true},
	},
	{
		Desc: "the catalog's own word for a textured run answers a wording naming it",
		In: matcher.InputCard{Name: "Blast Breath - 016 (PRB01 Textured Art)",
			Variation: "ST04-016 PRB01 Textured Art",
			Edition:   "ST04 - Starter Deck: Animal Kingdom Pirates", Foil: true},
	},
	{
		Desc: "negative: no plain row still lists the candidates it aliased",
		In: matcher.InputCard{Name: "Leo (057) (Alternate Art Promo)",
			Variation: "OP10-057 Standing on Woman 057 Alternate Art Promo",
			Edition:   "OP10 - Royal Blood", Foil: true},
	},
}

func TestOnepieceMatch(t *testing.T) {
	b := gametest.Backend(t, matcher.GameOnePiece)
	gametest.Replay(t, b, onepieceSeeds, *updateOnepiece)
}

// TestOnepieceDashTailNames pins the datastore invariant the bare-tail
// rule rests on: no card is named with a dash and a number at the end, so
// a tail found there is always the collector number a storefront hung off
// the name. Two names do carry a full code of their own ("Monkey.D.Luffy
// - OP14-34"), which is why Prefilter leaves a name that is canonical as
// it stands alone, both before the parentheticals are split off and after.
func TestOnepieceDashTailNames(t *testing.T) {
	b := gametest.Backend(t, matcher.GameOnePiece)

	for _, name := range b.AllCanonicalNames {
		if dashTailRe.MatchString(name) {
			t.Errorf("card name ends in a dash number: %q", name)
		}
	}
}

// TestOnepieceSealed pins the sealed namespace: products load, resolve by
// name, and never shadow the card index.
func TestOnepieceSealed(t *testing.T) {
	b := gametest.Backend(t, matcher.GameOnePiece)

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

	uuid, err := b.ResolveSealed("One Piece Card Game Romance Dawn Booster Pack")
	if err != nil {
		t.Fatalf("booster pack did not resolve: %s", err)
	}
	co, err := b.GetUUID(uuid)
	if err != nil || !co.Sealed {
		t.Fatalf("resolved uuid %s is not a sealed product", uuid)
	}

	// The OP01 booster box is two real products (the Wave 1 - Blue and
	// Wave 2 - White print waves); a wording naming neither must stay
	// unresolved rather than pick one.
	if uuid, err := b.ResolveSealed("One Piece Card Game Romance Dawn Booster Box"); err == nil {
		t.Fatalf("wave-ambiguous booster box resolved to %s", uuid)
	}
}

// filedWithNoNumber reports whether every printing of a name is filed with
// no number at all, as opposed to under a code such as DON!!'s.
func filedWithNoNumber(b *matcher.Backend, name string) bool {
	var printings int
	for _, uuid := range b.Hashes[matcher.Normalize(name)] {
		co, found := b.UUIDs[uuid]
		if !found || co.Sealed {
			continue
		}
		if co.Number != "" {
			return false
		}
		printings++
	}
	return printings > 0
}

// TestOnepieceNumbered pins the datastore invariant the DON!! rules rest
// on: the resource card is the one name in the game none of whose printings
// carries a number, so a storefront's number field can say nothing about
// which of them it means. A name with even one numbered printing is not
// that case, and Monkey.D.Luffy is the proof: a handful of his event
// printings carry no number while the rest wear real numbers, and those
// still have to be told apart by number.
//
// Another card the catalog files with no number has none to compare either
// - the Flame-Flame Fruit Trophy Card is one - and the rule is right to
// ignore a storefront's number for it too. A new name filed
// under a code of its own still trips this, because its number is not empty.
func TestOnepieceNumbered(t *testing.T) {
	b := gametest.Backend(t, matcher.GameOnePiece)

	// CanonicalNames holds the card names alone; the qualified spellings
	// naming one printing apiece are searchable but never canonical, and a
	// qualified name reaching a printing with no number has none to compare
	// either.
	for _, name := range b.CanonicalNames {
		want := !strings.HasPrefix(name, "DON!! Card") && !filedWithNoNumber(b, name)
		if got := numbered(b, name); got != want {
			t.Errorf("numbered(%q) = %v, want %v", name, got, want)
		}
	}
}
