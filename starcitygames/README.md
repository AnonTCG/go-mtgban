# starcitygames

Prices Star City Games' singles and sealed product, buy and sell, across
every game they carry: Magic, Flesh and Blood, Lorcana, Riftbound. Wired into
`cmd/bantool` as eight scrapers - one singles and one sealed scraper per
game - via `NewScraper(game, apiKey)` / `NewScraperSealed(game, apiKey)`.

This file is the accumulated knowledge from fixing a long run of mispriced
listings in this scraper - `mtgban/go-mtgban` PRs #493, #572, #573, #574,
#578, #582. The short version: **SCG hands two different printings the same
identifier often enough that this is the normal case to defend against, not
an edge case.** Read on for why, how it's caught, and how to catch the next
one.

## The data source

Retail, and the identity of every product it lists, come from one export:
`scgCatalogURL`, SCG's HawkSearch catalog download, authenticated with an
`x-api-key` header. It streams as one long JSON array of every product
across every game, and a single `StreamCatalog` pass fills the inventory.

**The export is exhaustive for what it covers - there is nothing else in it
to find.** Dumped raw and diffed against `CatalogProduct`/`CatalogVariant`:
14 top-level keys, 8 per variant, nothing else. No artist, no promo-series
name, no language beyond the one `language` string, no second identifier
space. Before writing a rule that reads a vendor field, confirm the field is
actually there (`python3 -c "import json;
print(json.load(open('catalog.json'))[0])"` on a fresh pull) - several fixes
in this history were held up by assuming a field existed, or assuming one
didn't, without checking.

The buylist comes from the storefront's own search instead (see Buylist,
below). Two MeiliSearch indexes at `search.starcitygames.com` share the
search-only key SCG's sell-your-cards frontend embeds:

- **`sell_list_products_v2`**, read by `sellListOffers` for every buy
  price. `Hit`/`Variant` in `api.go` model its products too: `catalogHit`
  synthesizes a compatible `Hit` from a `CatalogProduct` so `preprocess.go`'s
  wording path reads one shape. It carries fields the catalog export
  doesn't - a per-variant `status`, `bulk_category_name`, and a `subtitle`
  close to a display name (`"Pleiades, Superstar"` / `"(Maori)"`) that is
  the only place SCG writes some distinguishing detail down at all.
- **`sets_v2`**, queried by `SetIDs` to build sell-your-cards bookmark links
  (`SCGBuylistURL`).

Resolution reads the catalog export, except for the singles only the index
lists, which are worded from its own fields (see Buylist, below). When a
fix needs data the catalog doesn't carry, check the index before assuming
the data doesn't exist anywhere (see Aurora, below, where it did -
`datastore-gen` grew the field, not this scraper).

## The core problem

SCG's own `scryfall_id` / `tcgplayer_id` are the fast path: `resolveProductID`
tries them first (after a handful of sku-driven steers that have to run
*before* the identifiers, described below), and when the id is present and
un-contradicted, it resolves directly with no wording involved. The id is
read in the finish the product names (`matchVendorID`: `MatchIDFinish` with
FaB's print run and treatment or Lorcana's rainbow foil, Magic's foil and
etched flags otherwise). Outside Magic the printing it lands on also has to
carry the sku's own number (`carriesSkuNumber`), or the wording decides.

The catch: **those ids are sometimes wrong, and sometimes shared between two
genuinely different products.** A League Tokens promo carries the *set
token's* Scryfall id, not its own. A Timeshifts retro-frame carries the
original printing's id. An Ampersand embossed card and its plain promo-pack
twin carry each other's. When that happens, both catalog products resolve to
the same uuid, and `mtgban/base.go`'s `Add` used to keep whichever arrived
with the higher price - silently, no error, no log line. A $0.29 token
listing would carry a $34.99 promo's price, or vice versa, and nothing said
so.

**The fix is never "trust the vendor id less in general."** A vendor's id is
theirs, and overriding one is a per-vendor, per-shape call, made after
proving the specific shape wrong, not a blanket discount. Every fix in this
history is a **named, provable shape**: replay the product through
`resolveProduct`, dump what the datastore actually holds at that
(set, number), and either write a rule keyed on a real field the product
carries, or - when nothing distinguishes the two - a closed table.

## `resolveProduct` - the pipeline

`resolveProduct` wraps `resolveProductID` with two checks that apply after
resolution, regardless of game or path taken:

1. **Language.** A non-English `language` field has to match the resolved
   printing's language, or refuse - this is what keeps the inherently
   foreign sets (Foreign Black Border, Rinascimento, …) from collapsing onto
   their English twin. English itself asserts nothing (it's the sku
   grammar's default, not a claim).
2. **Store Championship foil-only refusal.** A `SCHP_`-numbered sku sold
   unfoiled, landing on a foil-only printing, is refused - there is no
   unfoiled version of that card to be. Scoped to `SCHP_` specifically after
   checking the general shape (any unfoiled product landing on a foil-only
   printing) also caught three unrelated, real, differently-broken products
   that should have been *fixed*, not silenced - see Method, below.

`resolveProductID` then hands the product to its game's resolver
(`catalogResolvers`). In every game, **sku-driven
steers run before the identifiers get a say** wherever the identifier is
known to name the wrong thing outright (Portal's `b`-suffixed second
printing, a lettered Magic prerelease, `promoShelfPrintings`) - for those,
trying the id first and discarding it would still cost the wording path a
chance to run cleanly. Everywhere else, the id is tried first and
**vetted** by `idContradictsProduct` before being trusted.

### Magic

Order: Portal `b`-suffix override → lettered prerelease (`PRE_..b`) → Duel
Decks: Anthology → `promoShelfPrintings` (sku-keyed, closed table) →
`ScryfallID` then `TCGPlayerID` through `idContradictsProduct` (with
`resalePrinting` applied to a surviving id) → WAR Japanese planeswalker /
`PP_` promo-pack fixups → generic `preprocess` + `matcher.Match` wording
fallback.

**`idContradictsProduct`** is the general defense - a resolved id is
rejected (falling through to the wording path) when the *product itself*
says something the id doesn't back up:

| guard | catches |
|---|---|
| `Double Rainbow` finish | id isn't tagged `doublerainbow`/`rainbowfoil`/`serialized` |
| `-AMP_` sku | id isn't tagged `embossed` |
| tagged `embossed` | sku isn't `-AMP_` and finish doesn't say `Embossed` (the dual of the row above - an Ampersand and its plain promo-pack twin share an id both ways) |
| `MH12` shelf | id isn't tagged `boosterfun` (Modern Horizons Timeshifts vs. the original printing) |
| `WCHP` shelf | id's set isn't the year's own deck set (`worldsSetFromSKU`) |
| `SSD_` shelf | id isn't tagged `standardshowdown` (Standard Showdown is an *event*, not a set - the sku's year has to pick among several) |

Each guard exists because a *survey* (replay the whole catalog, group by
resolved uuid, look for two skus on one id) found the shape live and
measured its blast radius before shipping - never guessed from the error
text alone. See Method.

**`resalePrinting`** handles WotC resale (`RESL_`) skus the same way: the id
names the right card in the right set but the wrong treatment (a promo pack
instead of the resale reprint), and the sibling search is bounded to the
same set + name so it can't wander.

### Flesh and Blood

FaB carries a `tcgplayer_id` on almost every product. It lands through the
id loop when the printing carries one of the sku's own numbers, read the way
`fabNumbers` reads them, which keeps SCG's pitch swaps (SEA142 filed under
SEA143's id) on the wording path. SCG files one id on a card's plain sku and
on its marked one ("HNT-155" and "HNT2-155"), so an unmarked sku whose id
lands on a treatment is left to the wording too (`fabUnmarkedTreatment`).
Otherwise resolution is sku- and wording-driven, via `fabMatch` trying the
sku's own numbers (most specific first) against `matcher.Match`. Four
steers (`fabSkuPrinting`) run on a successful match or an id landing, each
closing a *different* shape of "two products, one number, nothing else says
which is which":

| steer | what it fixes | keyed on |
|---|---|---|
| `fabPlainSibling` | a non-`"Marvel"` rarity landing on the Marvel-tagged twin (nothing excluded it) | a **general rule** - `rarity != "Marvel"` excludes a `marvel`-promoType candidate. Scoped: surveyed the whole datastore, 36 `(set, number, foil)` keys hold both a Marvel and non-Marvel row, 32 clean |
| `fabRenamedTwin` | SCG sends the same name for two *actually different cards* (English "Pleiades, Superstar" vs. the Maori printing, filed as "Hinewhitu, Hautipua" with the English name folded into a promo type) | closed table, one entry - surveyed for any other renamed pair, found none |
| `fabCreditedTwin` | two artist-commissioned prints share a name once "(Marvel)" is folded in (Aurora ROS008) | closed table, keyed on `Artist` - a field `datastore-gen` only started publishing (see below); **the go-to move once a field exists that the catalog itself never will** |
| `fabMarkedSibling` | a `fabVariantMarker`-suffixed set code (`SUP2`/`ROS2`/…, distinct from a lettered *number*) names a second printing at the same number, told apart only by a treatment the datastore carries in `PromoTypes` | reads the sibling directly, no table |

And one **refusal** table: `fabDuplicateStock` - Sanctuary of Aria's `027a`
(titled "Assorted Back" on the storefront) and `027b` ("Feet") both sell the
*one* row the datastore has for ROS027, at their own price, and nothing
says which is the card's price. Resolving either was letting whichever
streamed first win the id, silently, forever - refusing both is
deterministic. This is the shape to reach for when a survey finds *no*
distinguishing field anywhere, not even in the sell-list index - see the
next section for what happens when one *does* turn up.

**These four mechanisms are easy to blur together; keep them apart by what
each is keyed on:**
`fabPlainSibling` - a *rule*, general, no table. `fabRenamedTwin` /
`fabCreditedTwin` - *closed tables*, because a survey proved the shape is a
genuine singleton with nothing to generalize. `fabDuplicateStock` - a
*refusal* table, because nothing distinguishes the pair at all, in any
source checked. Reach for the rule first; drop to a table only once a
survey shows the rule can't be derived; refuse only once a survey shows
even a table can't be built honestly.

A **double-sided** listing ("Front // Back", a sku naming two numbers) whose
faces the datastore holds only as separate printings is skipped, not refused.
`fabNumberRespellings` corrects the numbers the catalog's own skus misspell.

### Lorcana

Reads its number off the sku (`lorcanaNumber`), which is more specific than
the product's own `collector_number` field (it keeps a printing letter the
product field drops). A lettered number is resolved one of two ways the
datastore uses interchangeably: the sibling carries the same letter as its
own number (`resolveLorcana` finds it directly), or the datastore instead
numbers both printings alike and tells them apart by name - a longer name
extending the base card's own (`"Bucky - Squirrel Squeak Tutor (Errata
Version)"` beside plain `"Bucky - Squirrel Squeak Tutor"`) - in which case
`lorcanaSibling` searches by name and picks the sole agreeing candidate,
refusing outright if several cards answer, and skipping when none does (a
printing the datastore does not carry, such as a "Misprinted Version"). `lorcanaFinish`
separately handles the one treatment (rainbow foil / `RainbowPillars`) two
products can share a bare foil flag but not a uuid. Three more shapes are
read before or after the main match: an Illumineer's Quest promo shelved
under the set it promotes is found by name and number across every set
(`lorcanaQuest`), a Japanese or Simplified Chinese promo asks for the name
plus "(JP Exclusive)" or "(CS Exclusive)" first (`lorcanaExclusives`), and
`lorcanaNumberRespellings` fixes two promo skus that carry the wrong number.

### Riftbound

The `tcgplayer_id` of a signature (`s`) sku names the overnumbered card it
signs, so the sku-number check sends it to the wording, which lands the
signature. Outside the `PRM` shelf the sku's set segment has to be the
printing's set too. Otherwise straight `matcher.Match` on name + set +
number + foil. The catalog's game name is "Riftbound: League of Legends
TCG"; `gameFromCatalog` also takes the bare "Riftbound", both by exact
match.

## Buylist (`buylist.go`)

The catalog export lists a Played or Heavily Played variant for only some
products, though SCG buys nearly every grade of every single: the index has
NM, PL and HP on all but a few. So `Load` reads every product of the game
the index marks as bought (`primary_status` `buying_at_cost` or `hotlist`)
from `sell_list_products_v2` first, and `processProduct` prices each catalog
product's grades from it, joined by sku: a variant sku is its product's
plus one grade digit, `1` NM, `2` PL, `3` HP.

- A variant counts only while its own `status` is `buying_at_cost` or
  `hotlist`. `buying_in_bulk` is a flat rate for a whole rarity/finish tier
  (every Magic Rare $0.08), not a bid on the card, and `not_buying` and
  `do_not_show` are no offer at all.
- A product with no NM bid is dropped whole. Its `primary_status` can read
  `buying_at_cost` off a played grade alone, but the storefront shows it as
  "We are not currently buying this product".
- `scgMinBuyPrice` drops the sub-cent figures the index also carries.
- A grade is the lowest SCG takes at that price. SCG grades no MP, so the
  HP price is filed at MP as well, and at PL where SCG makes no PL bid.

The index serves at most 45,000 hits per filter, offset included, so
`sellListSlices` splits a game by `finish`, then by `rarity` where a slice
still holds that many, and `sellListOffers` refuses a crawl whose pages
come back short of what the facets count. A failed crawl fails the buylist
alone (`mtgban.ErrBuylistLoad`), and the inventory still publishes.

The index also lists singles the catalog export does not carry at all:
foreign-language printings, World Championship cards and Flesh and Blood
promos, for the most part. Once the catalog has streamed, `fileListed`
words each one as a catalog product (`listedProduct`) and files it through
the same path, with no retail and no vendor id. The index numbers its
finishes and rarities, so each id takes the spelling the catalog gives most
of the products both list (`catalogSpellings`); a product whose language,
finish or rarity the catalog never spelled that run is skipped, since a
blank would read as English or plain.

With no id, a wording miss widens to any printing of the name, so
`listedLanding` holds the landing to the sku: its finish letter has to agree
with the printing's foil, and a `WCHP` card has to stay in its year's set,
its player's deck and its half of it (main or sideboard). On the
2026-11-01 catalog the rules disagree with ten of its own landings (eight
Flesh and Blood promos, two 1996 Order of the Ebon Hand), and refuse 33
index-only ones that went to another year, player, half or finish.

## `AddUnique` - the backstop

`mtgban.InventoryRecord`/`BuylistRecord.Add*` come in four strictness
levels (`AddRelaxed` < `Add` < `AddStrict` < `AddUnique`). This scraper's
**primary** records call `AddUnique`: a second entry at one card, one grade
is this whole defect class's own signature, and it's now rare enough (post
`#572`, `#573`, `#574`, `#578`, `#582`) that refusing it loudly - one log
line per run - beats letting the dearer one silently win, which is what
`Add`/`AddStrict` used to do.

The **`_CC`-marked second-stock bucket** (`secondStock`, `bucketKey`,
`secondBucketMarker`) is a genuinely different, already-correct mechanism -
SCG occasionally splits one listing's stock across two product records
(`"SGL-FAB-AGB-014-ENN"` beside `"SGL-FAB-AGB-014_CC-ENN"`), agreeing on
everything but quantity, meant to be **summed**, not refused. It still calls
`Add`/`AddRelaxed`. Don't conflate the two: a `_CC` pair is one listing
written twice; an `AddUnique` refusal is two listings that shouldn't be one
price.

Switching to `AddUnique` was itself measured, not assumed safe - replayed
the full catalog through the real production `Add` calls before and after,
across all four games, checking the delta was small, expected, and
attributable (mostly Magic's known-deprioritized double-faced-token
collisions, already muted; a handful of genuine catches). See PR #578.

## Method - how to find and fix the next one

This is the actual repeatable process, extracted from doing it five times
(`#572`, `#573`, `#574`, `#578`, `#582`):

1. **Pull a fresh catalog.** `curl -H "x-api-key: $SCG_API_KEY"
   https://api.starcitygames.com/hawksearch/catalog/download/json -o
   catalog.json` (~108MB). A catalog from even a few days ago can miss a
   product SCG has since added or repriced - don't reuse an old dump for a
   fresh finding.
2. **Replay, don't guess.** Decode into `[]CatalogProduct`, call
   `resolveProduct(b, game, p)` for real products against the backend the
   test loaded, group by returned uuid. Anything with more than one sku is a
   live collision. This is the same replay used throughout this history -
   see any of `#572`, `#573`, `#574`, `#578`, `#582` for the harness shape (a
   throwaway `zz_*_test.go`, never committed).
3. **Dump the raw record, every field, before writing anything.** `python3
   -c "print(json.dumps(next(p for p in catalog if p['sku']=='...'),
   indent=1))"`. This is what caught the Aurora case having genuinely
   nothing to key on in the catalog, and separately caught that the
   MeiliSearch index *did* carry the distinguishing detail (a `subtitle`)
   the catalog omitted.
4. **Check the datastore, not just the catalog.** `b.GetUUID(id)` and
   `b.MatchWithNumber("", setCode, number)` (name may be empty) to see every
   candidate row at that number, and what actually distinguishes them -
   `PromoTypes`, `Rarity`, `Artist`, `Finishes`. A field the struct exposes
   and the vendor's raw record also carries is worth trusting; a field
   neither carries is not there to key on, however plausible a rule sounds.
5. **If nothing distinguishes the rows anywhere - catalog, datastore, index,
   storefront page - that's a refusal, not a guess.** (`fabDuplicateStock`.)
   If the storefront page or the MeiliSearch index has the answer but the
   scraper's own inputs don't, that's either (a) a case for the closed-table
   pattern keyed on sku, verified once by hand against the live page/index
   (`fabRenamedTwins`, `fabCreditedTwins`, `promoShelfPrintings`), or (b) if
   it's genuinely general and datastore-gen can be taught to carry the field
   (as happened for FaB's `Artist`), the better fix is upstream - see next.
6. **Survey the blast radius before shipping either a rule or a table.**
   Every guard and steer above was sized this way: group the whole datastore
   by the shape the rule would key on, count how many `(set, number, foil)`
   keys it touches, check how many are "clean" (exactly the two-way tie the
   rule is meant to break) versus genuinely ambiguous. A rule that fires on
   30 clean cases and silently mishandles 4 ambiguous ones needs the
   ambiguous ones excluded explicitly, not ignored.
7. **Verify against the real production path, before and after, with a line-
   level diff - not just a count.** Replay the full catalog through
   `resolveProduct` with and without the change; confirm the exact
   collisions predicted are the ones that close, and that nothing new
   opens. A count going from 40 to 38 could mean two closed and zero opened,
   or four closed and two opened - check the lines.
8. **Pin it with a test using fixtures copied from the export verbatim** -
   not hand-typed approximations. `catalog_shared_id_test.go`,
   `catalog_fab_twins_test.go`, `catalog_showdown_test.go` are the pattern.
   A wrong hand-typed fixture would test nothing; a copied one is the shape
   that actually broke.
9. **If the fix depends on a datastore field this checkout might not have
   yet** (a fresh `datastore-gen` field not yet republished - see Aurora),
   guard the test with a skip, not a hard failure: `requireCredit` mirrors
   `requireSibling` (already established for Lorcana). CI's `cache-*` jobs
   pull a **pre-built** datastore from `b2://mtgban-datastore/<game>/
   <game>.json.xz` - republishing that is a separate, deliberate step, not
   something a PR does as a side effect.

## Upstream, not scraper-side: the two things left alone on purpose

- **Magic double-faced tokens.** mtgjson models each face of a token as its
  own row; the catalog sells one physical product covering both. This is by
  far the largest collision class by count: a full replay of 151,337 Magic
  singles through `resolveProduct` found 931 uuids receiving more than one
  product, 803 of them these token pairs. It is **deliberately not fixed
  here**: this is a datastore matter, not a scraper one, and has been
  treated as low priority across this whole project. No scraper-side
  dedupe or DFC-pricing logic; it needs the datastore to carry a
  `tokenProducts`-style mapping from a token uuid to the individual faces
  it's sold as.
- **Aurora's artist field, until republished.** `fabCreditedTwin` reads
  `Card.Artist`, which `datastore-gen@698813b` now populates for FaB and
  `matcher/fleshandblood@4f21ab43` now reads - but CI's pinned datastore
  is whatever's currently published to B2, and republishing that is
  someone's deliberate call, not this scraper's. Until it happens, the
  steer is a safe no-op (`Artist == ""` on both candidates never matches
  `want`) and its test (`TestResolveFleshAndBloodCreditedTwins`) skips
  rather than fails.

## Environment

- `SCG_API_KEY` - the HawkSearch catalog download. Local: `go-mtgban/.env`.
- `FLESHANDBLOOD_PATH` / `ALLPRINTINGS5_PATH` / `LORCANA_PATH` /
  `RIFTBOUND_PATH` - the datastores tests load lazily (`withMagic`,
  `withGameDatastore`); each CI job sets exactly one, so a test needing a
  specific game skips cleanly everywhere else.
- Rebuilding a fresh Flesh and Blood datastore locally (needed to verify
  anything that reads `Card.Artist`, or any FaB fix generally, against
  current upstream data rather than whatever's cached): pull
  `b2://mtgban-datastore/fleshandblood/tcgplayer-catalog.json.xz`
  (credentials: `B2_APPLICATION_KEY_ID_DATASTORE` /
  `B2_APPLICATION_KEY_DATASTORE`), decompress, then in a `datastore-gen`
  worktree: `go run ./cmd/fleshandblood -tcg-catalog tcgplayer-catalog.json
  -o fleshandblood.json`.
