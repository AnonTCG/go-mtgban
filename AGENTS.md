# AGENTS.md

Guidance for AI coding agents working on **go-mtgban**, a trading-card
market-data platform: scrape store inventories and buylists, normalize every
listing to a canonical card identity, then compute arbitrage across stores.
Nine games are supported today - Magic: The Gathering, Lorcana, Riftbound,
One Piece, Yu-Gi-Oh, Flesh and Blood, Pokemon, Gundam, and Palworld. Read
`SPECIFICATIONS.md` for the full architecture and `docs/adr/` for the
reasoning behind the load-bearing decisions.

## The one rule that matters

**Everything keys on the `matcher` UUID.** Scrapers are thin translators;
correctness lives in `matcher`. If a card matches wrong, fix it in
`matcher` - usually in a data table, and for Magic inside the
`matcher/magic` sub-package - not in the scraper.

The exception is a vendor's own identifier being wrong, or shared between
two genuinely different products - that's the vendor's data to distrust,
which makes it the scraper's fix, not `matcher`'s.
`starcitygames/README.md` works through that whole defect class end to end:
how it's found, the fix shapes (a general rule, a closed table, a
refusal), and the repeatable method for catching the next one.

The opposite exception is a vendor that publishes **no** identifier at
all: Cool Stuff Inc's buylist and retail feeds carry none, for any of the
eight games it prices, so every resolution is wording- and shelf-driven
and the shelf itself is sometimes wrong. `coolstuffinc/README.md` names
the recurring shape found three times independently across three
games - a promo shelf holding a printing that collides with a real set's,
told apart only by a note or bracket the code has to read - and where it
has, and hasn't yet, been fixed.

`cardmarket/README.md` is a different kind of read: not a defect class,
but the design of `Market` (the live-listing singles scraper built
alongside `Sealed`'s existing one) and the investigation trail behind
it - a per-token concurrency limit measured directly rather than assumed,
listings that turned out not to be strictly price-ascending once replayed
against real data, and an offline pre-filter whose price snapshot is
published one host per game, not shared, where a fetch from the wrong host
reads as "nothing passed the filter" rather than "the fetch hit the wrong
server." Read it before touching any of the three
Cardmarket scrapers or the `resolver` they share.

`sealedev/README.md` covers the one "scraper" with no site behind it: it
prices sealed product by what opening it is worth, against the MTGBAN price
API rather than a storefront. Read it before changing anything an EV reads,
because two of its price sources are *estimated* rather than quoted - TCG
Direct (net) where the buylist has no number or an implausible one, and
Cardmarket for the ~86% of the catalog its market scraper never polls - so
an EV can move between runs without any price having moved. It also names
what silently never counts toward a total (serialized, cosmic foil,
unfixed Secret Lair bonuses, anything under the bulk threshold) and why
that floor is deliberately the same for every source.

`cardkingdom/README.md` covers what a store's feed cannot say: Card Kingdom
publishes one buy price per card, so every lower grade comes from its schedule.
Read it before changing `gradedBuyPrices` or `paidAsFoil`, because `is_foil` is
not what picks the Foils row, and a URL suffix inferred from the data is. It
also says which rows of the schedule no payout has confirmed, and how to measure
them.

A third case tempers "fix it in `matcher`" without contradicting it: a
shared matcher rule reaches every vendor and every printing of its class,
so size the fix to the evidence behind it. When one vendor's evidence would
justify a rule that moves far more landings than that evidence covers, put
a guard in that vendor's scraper instead. A fix made for one vendor must
not move TCGplayer, Star City Games, Card Kingdom or Cool Stuff Inc
landings unless each move is a named correction - those four packages'
tests passing is not that proof, since their fixtures need not reach
the changed path; a count in the datastore of what the rule would
touch does (#716).

## Layout

```
mtgban/                    interfaces (Scraper/Seller/Vendor), records,
                           Arbit/Mismatch, CSV I/O, WorkerPool
matcher/                   game-agnostic core: Backend, b.Match/b.MatchID,
                           the GameRules seam, the game registry, and the
                           search API
matcher/magic/             Magic rules, MTGJSON loader, promo/frame vocabulary,
                           replay corpus
matcher/lorcana/           Lorcana rules, loader, replay corpus
matcher/riftbound/         Riftbound rules, loader, replay corpus
matcher/fleshandblood/     Flesh and Blood rules, loader, replay corpus
matcher/gundam/            Gundam rules, loader, replay corpus
matcher/onepiece/          One Piece rules, loader, replay corpus
matcher/palworld/          Palworld rules, loader, replay corpus
matcher/pokemon/           Pokemon rules, loader (no replay corpus yet)
matcher/yugioh/            Yu-Gi-Oh rules, loader, replay corpus
matcher/games/             meta-package that blank-imports all nine games
matcher/sealed/            opening sealed product: picks, decklists and
                           expected counts per product, booster and deck
<store>/                   one package per store (tcgplayer, cardkingdom,
                           cardmarket, ...)
cmd/                       tools; cmd/bantool is the production orchestrator
```

Magic's edition aliases, variants, replay corpus and promo dates live in
`matcher/magic`. Its `CandidateSets` hook owns promo sibling expansion,
Secret Lair widening and edition-selection policy; `FinalizeCandidates` owns
the World Championship trim. Core materializes the selected sets and applies
language filtering after the final game hook.

The dependency runs one way: a game package imports core `matcher`, and no
non-test file in core imports a game package. That one-directional rule is
what keeps the core game-agnostic, and it is worth remembering before reaching
for a Magic symbol from core, since the import would cycle. Core's own tests
get away with importing `magic`, to load the Magic datastore, only because
they are an external test package (`package matcher_test`), which is
outside the import graph of the library itself.

## Build, test, format

```sh
go build ./...              # must stay green
gofmt -s -l .               # must print nothing
go vet ./...
go run github.com/mgechev/revive@v1.17.1 -set_exit_status -config .revive.toml ./...
go run honnef.co/go/tools/cmd/staticcheck@2026.2.2 ./...
go test ./... -v
```

Run all six before committing. CI (`.github/workflows/ci.yml`) runs every
one, plus a datastore-free `go test -race ./...` and, nightly, the whole tree
under `-race` with the Magic datastore, reporting coverage. The formatting
check is a hard gate: the job lists the offending files and exits non-zero,
so a stray unformatted file fails the build rather than merely drawing a
review comment. The tree is gofmt-clean today; keep it that way.

Do not narrow the test or vet invocation to a subset of packages. Tests live
in `mtgban/`, in `matcher/`, every one of its nine `matcher/<game>`
sub-packages and `matcher/sealed`, and in 19 of the 24 scraper packages
(only `arcanafrisia`, `cardsphere`, `mtgstocks`, `secretdeskorrigans` and
`toamagic` have none).
A subset run can pass while CI fails. `matcher/magic` carries the largest
test suite of any package in the repo, nearly forty `*_test.go` files with
the Magic replay suite among them (see "The golden suites" below).

### Datastores

The matcher suites exercise real data, one datastore per game, located through
environment variables:

- `ALLPRINTINGS5_PATH` - MTGJSON `AllPrintings5.json`. Feeds the core
  `matcher` suite and `matcher/magic`, both of which load it through
  `magic.Load`, as well as every scraper suite that needs a real Magic
  datastore to test its `preprocess.go` (abugames, cardgamecorner,
  cardkingdom, cardmarket, cardtrader, coolstuffinc, gamenerdz, hareruya,
  manapool, mintcard, sealedev, starcitygames, strikezone, tcgplayer,
  vegassingles; grep a package's tests for `ALLPRINTINGS5_PATH` rather than
  for a skip message, which is not worded the same everywhere). Magic is the
  one game whose datastore is not built by `datastore-gen`.
- `LORCANA_PATH`, `RIFTBOUND_PATH`, `ONEPIECE_PATH`, `YUGIOH_PATH`,
  `FLESHANDBLOOD_PATH`, `POKEMON_PATH`, `GUNDAM_PATH`, and `PALWORLD_PATH` -
  one datastore per remaining game, all built by
  `github.com/mtgban/datastore-gen`. Each feeds its matching
  `matcher/<game>` suite (`matcher/lorcana`, `matcher/riftbound`,
  and so on).

Every suite, Magic and core `matcher` included, loads its datastore
lazily, once per test binary, and calls
`t.Skip("Need <VAR> set to run this test")` on the tests that need it, so a
contributor missing every one of the nine datastores still gets a green, if
much thinner, `go test ./...` run. Magic and core keep a `sync.Once`-guarded
`realDatastore(t)` helper; the eight other games all call
`gametest.Backend(t, game)` from `internal/gametest`. `matcher/magic`'s `TestMain` calls
`log.Fatalln` only if its own golden `testdata/magic_test_data.json` fails
to open or parse - a repo integrity fault, not a missing-env-var one.

Reuse a datastore a suite already loads rather than adding another loader.
Each helper costs a parse of about ten seconds and three gigabytes per test
binary, and a test file cannot import another package's helper (which is
why `internal/gametest` is an ordinary package), so tests
for a new package that only needs the Magic datastore belong in a suite that
already loads it: `matcher/sealed`'s datastore-backed tests live in
`matcher`'s external test package, on its `testBackend`, and only the ones
needing no datastore sit in its own directory.

Use absolute paths for every variable. A relative path is resolved against
the directory of the package under test, so a single relative value cannot
serve suites that sit at different depths in the tree - and they do:
`ALLPRINTINGS5_PATH` is read from `matcher` one level down and from
`matcher/magic` two. CI passes every one as an absolute path, per game,
for that reason.

**Locally**, `go test` does not source `.env` on its own - only
`cmd/bantool` does, via a blank `godotenv/autoload` import in its own
`main.go`. Export the variables yourself before running a suite that needs
one, e.g. `set -a; . .env; set +a; go test ./matcher/...`, or
`source .env && go test ./...` if your shell's `source` does the same. CI
needs none of this: it exports every `<GAME>_PATH` directly as job env
(below), so this is a local-checkout step only.

CI restores all nine datastores from `actions/cache` before testing, one
`cache-<game>` job per game. Only Magic has a public URL: `cache-datastore`
calls the reusable `cache-file.yml` with `vars.DATASTORE_MAGIC`. Every other
game calls the reusable `cache-b2.yml` with its object in the private
`mtgban-datastore` B2 bucket, which it caches under a key built from the
object's own metadata, since B2 serves no HTTP etag.

`internal/vocabulary` runs two checks across the datastore-gen boundary, each
gated on the `<GAME>_PATH` variables above and skipping the games whose
variable is unset. `TestLoadersReadWhatIsPublished` holds every loader's
promo-type word table to what the published datastore actually states, so a
loader that folds or drops a word cannot drift unnoticed.
`TestReplayCatalogNames` matches every product name in a `REPLAY_CATALOG`
TCGplayer catalog dump and writes the verdicts to `REPLAY_OUT`, for diffing
two checkouts against each other rather than against a fixed assertion.

### The golden suites

Eight of the nine games own a replay corpus of *(input card → expected
verdict)* pairs under their own `testdata/` directory, each with its own
regeneration flag; only Pokemon has none yet. The flag belongs to the test
binary, not to `go test`, so the package has to come first - `go test -u
./matcher/magic/` fails with "no Go files in" the repository root, having
read `-u` as a `go` flag and dropped the path.

```sh
# matcher/magic        -> testdata/magic_test_data.json
go test ./matcher/magic/ -run TestMatch -u

# every other game     -> testdata/<game>_test_data.json
go test ./matcher/lorcana/        -update-lorcana
go test ./matcher/riftbound/      -update-riftbound
go test ./matcher/onepiece/       -update-onepiece
go test ./matcher/yugioh/         -update-yugioh
go test ./matcher/fleshandblood/ -update-fleshandblood
go test ./matcher/gundam/         -update-gundam
go test ./matcher/palworld/       -update-palworld
```

The Magic regeneration flag is the odd one out: it is the bare `-u` it has
always been, rather than `-update-magic`. Every other game follows
`-update-<game>` exactly.

Regenerate only after an *intentional* matching change, and read the
resulting diff line by line. Every regenerator refuses to flip a case
between success and error: a verdict-class flip fails the test and leaves
the golden file untouched. The seven non-Magic games share one runner,
`gametest.Replay`; each package registers its own flag and hands over its
own hand-authored seed cases (`<game>Seeds`, e.g. `lorcanaSeeds`,
`riftboundSeeds`), which the regeneration re-inserts and which pin the
contract edges the sampled corpus cannot reach. A seed's description
opening with `negative:` declares it expects an error. Magic's corpus is
curated by hand and has none. The
Magic regenerator still rewrites the expected uuid of any case that now
resolves to a different card, and trusts you to read that diff.

**The Magic corpus is an invariant, not a scoreboard.** It pins Magic
behavior exactly, quirks included - `magic.Rules.FilterCards` short-circuits
a lone candidate only to keep the answers the corpus records. Refactoring
must therefore leave `magic/testdata/magic_test_data.json` byte-identical.
If a change forces a regeneration, Magic matching has drifted: stop and find
the cause. Do not accept the diff as a new baseline.

## Conventions

- **gofmt always.** CI enforces it; `gofmt -s -l .` must print nothing.
- **No `reflect`**, tests included: revive's `imports-blocklist` rejects the
  import. Compare with `slices`/`maps` or a comparison written for the type,
  and ask questions about types through `go/types` (see
  `cardmarket/register_test.go`). The reasons are mtgban-website's
  `docs/adr/0002-no-reflect.md`.
- **No global loggers.** Each scraper takes a
  `LogCallback mtgban.LogCallbackFunc` and logs through a tagged `printf`
  helper (`[TAG] `-prefixed). The matcher logs through the `Logger` on the
  `Backend` it is asked through (`b.Logf`), and is quiet while that is nil.
- **Insert via the `Add*` family**, never by appending to the map directly.
  `Add`/`AddRelaxed`/`AddStrict`/`AddUnique` enforce defaults (NM, qty 1),
  validate conditions against `FullGradeTags`, refuse a NaN, infinite or
  negative price and a negative quantity, merge duplicates, and keep each
  slice sorted. `Arbit` depends on that sort leaving the NM entry at
  `entries[0]`. `AddUnique` is the strictest gate and **ignores price**.
- **Grades go through `mtgban.ParseCondition`.** It reads the spellings
  stores share; a word a store reads its own way (CardTrader's "Played",
  Cardmarket's scale) is mapped first, in that scraper's `<tag>Condition`
  helper. A refusal logs `printf("unsupported %s condition", text)`.
- **`ErrUnsupported` is a silent-skip signal**, not a failure. In a scraper's
  preprocess loop: skip `ErrUnsupported`, but log `AliasingError` (call
  `.Probe()` to dump the candidates) and every other error as a data-quality
  alarm.
- **Concurrency = `mtgban.WorkerPool`.** New fetch code uses it; do not
  hand-roll goroutine/channel pools. Cancelling the context stops dispatch but
  lets in-flight workers finish.
- **`sync.Map` is a last resort.** First, no shared state: compute per
  call, or precompute at load into the immutable `Backend`, as the game
  rules do with their tables. Then the simple shapes: an immutable
  snapshot swapped through an `atomic.Pointer`, an atomic counter or flag,
  or one goroutine that owns the state. Only when none of those fit, a
  plain map behind a `sync.Mutex`, which says what it is and is read with
  the lock held. Reach for `sync.Map` only for a read-mostly cache of
  unbounded keys whose reads measurably cannot afford a lock, and put the
  measurement in the comment. The one such use today is the `Normalize`
  cache in `matcher/replacer.go`, and #1380 is about removing the need
  for it.
- **No call folded into an `if`'s init-statement**, tests included: assign
  first, then test - `id := f(x)` then `if id != ""`, and `err := f()` then
  `if err != nil`. A bare comma-ok map lookup, `if v, ok := m[k]; ok`, is the
  only form that stays combined; a comma-ok with an added condition splits
  too. This is for new code - older code still folds calls, and that is not
  a sweep target.

## matcher: the game seam

### GameRules

`Match()` is one pipeline shared by every game. The steps that differ per game
are dispatched through the `GameRules` interface in `matcher/rules.go`, 11
methods in all: `Prefilter`, `AdjustName`, `AdjustEdition`, `AliasEdition`,
`FilterPrintings`, `CandidateSets`, `FinalizeCandidates`, `FilterCards`,
`IsUnsupported`, `IsToken`, and `PlainNumber` (read `rules.go` itself - each
method carries a paragraph explaining what it owns and why). `IsUnsupported`
is asked at three `Stage`s, each seeing different text: the storefront's
wording, the canonical name and edition, and the resolved printing. A game's
loader
attaches its implementation with `Backend.SetRules` when it builds the
`Backend`; a `Backend` that never got rules returns `ErrDatastoreEmpty` from
`Match` rather than panicking.

Two properties of the seam matter when writing a hook. First, core retains
language handling and shared token/oversize admission, but games own candidate
edition selection and final ambiguity policy. `DefaultRules.CandidateSets`
tries exact names, partial names, then all editions; `PromoWildcard` requests
all editions. Magic overrides this policy and trims World Championship cards
before the core language filter, preserving its historical ordering. Second,
hooks receive `InputCard` by pointer and may mutate it; those mutations persist
through the rest of the pipeline and remain visible to the caller.

`FilterCards` receives its candidates as a `[]CandidateSet`, one per set
`CandidateSets` admitted, sorted by set code. An implementation keeps that
order, or imposes its own as Magic does, so the slice it returns is the same
on every call: it feeds the user-visible aliasing diagnostics.

### Games register themselves

Each game package ships `rules.go` (its `GameRules` implementation), a
`Load(io.Reader) (*matcher.Backend, error)` function, and a `register.go`
whose `init()` calls `matcher.RegisterGame(name, Load)` - the
`database/sql` driver idiom - under the name of its own package: `magic`,
`lorcana`, `riftbound`, `onepiece`, `yugioh`, `fleshandblood`, `pokemon`,
`gundam`, and `palworld`.

The consequence is that **a game only exists if something imports it**. A
consumer blank-imports the games it needs, or blank-imports
`matcher/games` to get all of them (`cmd/bantool` does the latter); asking
`Open` for a game nothing registered is an error naming the games that are.

### Datastore loading

`matcher.Open(name, reader)` loads the named game's datastore: it runs
exactly one loader and hands back the `*Backend`, stamped with the game it
was loaded as (`b.Game`). There is no global datastore: every lookup is a
method on the backend a caller holds, a scraper is built on one
(`mtgban.NewScraper(b, name, opts...)`) and matches against it alone,
and a test builds the backend it needs and passes it. `RegisteredGames()`
lists what is currently linked in. `internal/datastore.Read(game, path)` is
`Open` over a path that may be a file, an `http(s)://` URL or a `b2://`
object, `.xz` or not; a suite reads its game's file that way once, in a
helper that hands the backend to each test.

There is no auto-detection. The caller always knows the game - bantool reads
it off the registry key its target sits under, a test off the package it sits
in - and trying every registered loader in turn would decode AllPrintings
once per game before reaching Magic's, behind a buffer of the whole file.

`Backend` is exported and every lookup is one of its methods (`b.Match`,
`b.GetUUID`, `b.GetSetByName`, ...); there are no package-level functions
of the same name, and no backend the package keeps for you.

The eight non-Magic loaders share one shape, and a new or changed loader
keeps it:
- `Load` reads only the `{"meta":…,"data":…}` envelope. It refuses
  anything else, and any card missing its id or name, as "not a <Game>
  datastore". A card with no printing is skipped, not refused.
- Cards are read by the names every datastore publishes: `setCode`,
  `number`, `image`, `externalLinks.tcgPlayerId`, and `baseSetSize`,
  `language` and `watermark` where the game has them. Lorcana's `images` is
  the one upstream field read instead, for a thumbnail the common `image`
  lacks.
- `IsOversized` comes from the game's oversized promo type. `Language` is
  `cmp.Or(card.Language, "English")`, since core's filter drops a
  non-English candidate. `AllSets` is sorted before `IndexSets`.
- `b.Rarities` and `b.Colors` are the datastore's `properties` orders,
  through `RarityNames` and `ColorNames`, and each set's `Rarities` and
  `Colors` are sorted by them with `RaritiesOf` and `ColorsOf`. A card's
  rarity is stored through `b.AddRarity`, spelled by `RarityName`
  ("superrare"); a rule or display wanting the published words ("Super
  Rare") asks `b.RarityLabel`.

### Tables before code

New-set and new-promo support is almost always **data**, not logic:

- edition name aliases → `matcher/magic/editions.go` (`EditionTable`) for
  Magic; One Piece, Pokemon and Yu-Gi-Oh keep their own `editionAliases`,
  Riftbound its `storefrontEditions`;
- card↔number disambiguation → `matcher/magic/variants.go`
  (`VariantsTable`), which scrapers reach as `magic.VariantsTable`;
- Magic promo detection rules → `matcher/magic/callbacks.go`
  (`promoTypeElements`);
- per-set special cases that genuinely need code → the filter callbacks in
  `magic/callbacks.go`, or the load-time patch tables in `magic/mtgjson.go`
  (the `switch set.Code` blocks) together with the companion data in
  `magic/table.go`.

Adding a `switch` case to a `Rules` hook is the last resort. The terminal
per-card `switch inCard.Name` at the end of `magic.Rules.AdjustEdition` is
itself a standing "tables before code" violation; do not grow it without a
good reason. Read the `Match` pipeline in `matcher/matcher.go`, and the
ordering notes in `SPECIFICATIONS.md`, before editing any stage.

Every non-Magic game needs far less of this, and shares the shape: each
embeds `matcher.DefaultRules` (`Rules struct{ DefaultRules }`) and
overrides only what it actually needs different. All eight rely on
`DefaultRules` - a real no-op - for `FinalizeCandidates` and `IsToken`, and
all but Pokemon for `FilterPrintings`, which Pokemon uses to hold oversized
and metal-card listings to those printings. All eight
implement their own `Prefilter`, `AdjustName`, `AliasEdition`, `FilterCards`
and `PlainNumber`, which is where a game's actual vocabulary - its editions
and its number shapes - lives, and all but Lorcana and Riftbound their own
`AdjustEdition`; `DefaultRules.AdjustEdition` applies the game's
`AliasEdition` and nothing else. Finishes are
not a game's to name: every one is read through `matcher.FinishSlug`, as
TCGplayer prices it, and `matcher.Finishes` is the table of those names
(`docs/finishes.md`). A few games additionally override one hook for a
narrow, real check: Lorcana and Yu-Gi-Oh override `IsUnsupported` at the
wording stage (Lorcana drops puzzle-insert and cruise-promo products;
Yu-Gi-Oh drops storefront character-art cards that carry no collector
number, and OTS Tournament Pack numbers past the set's English run),
Pokemon at the answer stage (a metal-card listing that answered
with a plain printing), and Pokemon overrides `CandidateSets` to fold
`*Promos` shelves into its loose-edition pass and to add, on every pass,
the subsets filed inside a set it selects: a set coded `<parent>-<suffix>`
whose name opens with its parent's, such as a Trainer Gallery. The
real, shared work across all eight is edition and number normalization in
`Prefilter`/`AdjustName`/`AdjustEdition`/`AliasEdition` and the
number-and-finish disambiguation in `FilterCards`.

### Search API

Every scraper matches through `Match()`; no game has a separate search
path. The core lookup surface is in `matcher/api.go`: `GetUUIDs`,
`GetUUIDsInSet`, `GetSealedUUIDsInSet`, `Names`, and the `Search*` family.

## Adding a scraper

1. New package with the standard layout: `<store>.go` (struct plus
   `Load`/`Inventory`/`Buylist`/`Info`), `api.go` (client and auth),
   `preprocess.go` (store text → `InputCard` → `b.Match()`), optional
   `sealed.go`.
2. Keep the scraper's state in unexported fields: `logCallback`,
   `maxConcurrency` where it fans out, inventory/buylist plus timestamps,
   and the `*matcher.Backend` the constructor was handed. Follow
   `starcitygames` for an API-backed store or `mtgseattle` for an
   HTML-scraped one. The `register.go` constructor (step 4) copies what
   bantool passes in `mtgban.Options` (`LogCallback`, `Affiliate`,
   `TargetEdition`, and secrets through `opts.Secret`) onto those fields.
   A store whose retail and buylist halves can run alone implements
   `mtgban.ScraperConfig`, whose `SetConfig` receives `DisableRetail` and
   `DisableBuylist`; the registry refuses to build half of a store that
   does not.
3. Fetch with `WorkerPool` plus a client from `mtgban.NewHTTPClient`.
4. Add a `register.go` whose `init()` calls `mtgban.Register(name, games,
   constructor)`: `name` is the bare registry key the store has always
   been known by (`coolstuffinc_sealed`, `starcitygames_sealed`), the same
   spelling under every game, and `games` is every game the scraper prices.
   Blank-import the package in `cmd/bantool/main.go`: `targets()` walks
   `matcher.AllGames` × `mtgban.Registered(game)` to build the option
   table, so a registered scraper needs no entry written there by hand,
   only the import, and is immediately selectable as `-game <game> -store
   name`. Add one row per target to `internal/workflowgen/targets.go`
   and run `go generate ./internal/workflowgen`, which writes its
   `bantool-<game>-<store>.yml` workflow, dispatched on `<game>-<store>` and
   `<game>-all`; never edit a generated file by hand, since the package's
   test fails CI on any file its row does not reproduce.
   `cmd/bantool/workflows_test.go`'s
   `TestEveryTargetIsScheduledByItsOwnWorkflow` fails the build if a
   registered target has no workflow scheduling it, a workflow names a
   target that is not registered, or a file's dispatch types are wrong.
   `.github/workflows/README.md` says what a row holds.
5. Set the right `ScraperInfo` flags: `MetadataOnly`, `NoQuantityInventory`,
   `SealedMode`, `CreditMultiplier`, `Family`, and `Game` - every scraper sets
   `Game` explicitly, `matcher.GameMagic` included; nothing reads as Magic
   by default.
6. The constructor takes the datastore first and nothing naming a game:
   `NewScraper(b *matcher.Backend, ...) (*T, error)`. A scraper that prices
   more than one game reads which one from `b.Game`, so it cannot be
   told one game and matched against another's datastore. The vendor's own
   naming for its games - slugs, catalog ids, department numbers - stays
   exported, because the package's own API helpers take one (`Search`,
   `SCGBuylistURL`, `NewGNClient`); what a caller does not need it for is
   building a scraper. One `map[matcher.Game]<vendor value>` per package sits
   between the two and both converts and validates, and a game the map does
   not hold is refused at the constructor, a datastore that names no game
   included, rather than anything silently defaulting to Magic. `Info()`
   reads the game back off the backend, `info.Game = x.backend.Game`, rather
   than switching a vendor value back into one; keep every read that drives
   a run on the vendor value.

### Adding a game

A game is named in `matcher` first and reaches the scrapers from there:

1. Add the `matcher.Game` constant in `matcher/game.go` and list it in
   `matcher.AllGames`. Its value is the game's own lowercase name, the one
   its loader registers under and the one flags, workflows, bucket paths and
   every dump spell it with. `matcher/game_test.go` pins it and checks
   every constant has a loader behind it, so pick it once and do not rename
   it later.
2. Register the matcher side: a loader, a `GameRules` implementation and a
   `register.go` under `matcher/<game>/` calling `RegisterGame` with the
   constant from step 1, plus the blank import in `matcher/games/games.go`.
3. Per storefront that carries it: one constant naming the vendor's own
   spelling beside that package's existing ones, and one line in its
   `<recv>Games` map. Nothing else in the scraper changes.
4. Per scraper that should run it: add the game to that store's own
   `register.go` (its `mtgban.Register` games list) alongside the map entry
   from step 3, and add its row to `internal/workflowgen/targets.go`, then
   run `go generate ./internal/workflowgen` (`.github/workflows/README.md`).
   Nothing is
   written in `cmd/bantool/main.go` itself - `targets()` derives the entry
   from the registry, and it is reachable at `-game <game> -store <store>`
   as soon as the registration lands.
5. Wire the game's datastore into `.github/workflows/ci.yml`: a cache job
   calling `cache-b2.yml`, and a `test-<game>` job. Add its path variable to
   `internal/vocabulary/read.go`'s `Games`, keyed by that constant.

For TCGplayer specifically, the per-game scrapers `TCGGame` and `TCGGameIndex`
are built from the `tcgplayer` package's `tcgGames` map, which associates a
game with the TCGplayer category serving it. Adding a game there is one table
entry plus the bantool options and workflows; both scrapers read the products
they price from the game's catalog dump, which the datastore publish writes
beside the datastore. Magic is in that map like every other game.

## Gotchas

- A `Backend` is immutable once loaded: its maps, slices, cards and rules
  are shared by every scraper built on it and must not be mutated. Replacing
  a datastore means loading a new backend and building new scrapers on it;
  nothing is published process-wide. A report reads the backend it is
  handed as its first argument, and a nil one returns no rows at all;
  `ArbitOpts` only filters. See
  `docs/adr/0004-localized-matcher-and-scraper-registry.md`.
- Everything that reads a datastore is a method on `*Backend`, including
  the `magic.Has*Printing` helpers (they take the backend first).
  `ExtractNumber` and `ExtractNumberAny` read no datastore and stay
  package-level functions, along with `Normalize`, `Title`, `ExtractYear`,
  `SplitVariants` and the rest.
  Keep new identification lookups on `b`, including in callbacks.
- The Magic promo-type constants (`PromoTypeBoosterfun`, `PromoTypeBuyABox`,
  `PromoTypePrerelease`, `PromoTypePromoPack`, `PromoTypeThickDisplay` and the
  rest) live only in `matcher/magic`. Core keeps no shim copies, so code
  that resolved them from `matcher` fails to compile until it imports
  `magic` - deliberately, since core cannot import `magic` and a duplicated
  constant would drift silently where a build error names the symbol.
- Foil and etched flags coming from scrapers are often wrong. `output()`
  clamps them against the printing's real finishes - trust the matcher, not
  the input.
- A scraper probe (a retry or shelf redirect that calls `b.Match` on
  rewritten wording) must check where it landed, not only `err == nil`: an
  edition naming no set that holds the card widens to every printing, and
  Magic's `FilterCards` passes a lone candidate without reading its number.
  Compare the landed `SetCode` and `Number` with what the probe asked for,
  as `coolstuffinc`'s `pokemonNonHoloDeckExclusive` does.
- `Normalize()` has deliberate *protection* entries that map a string to
  itself (`"waste land"`, `"trial and error"`, `"goblin // soldier"`).
  Changing the replacer table can silently re-alias unrelated cards; run the
  full matcher suite after any edit there.
- The insert-time sort invariant matters: `Arbit` assumes `entries[0]` is the
  NM entry, and that ordering is a *side effect* of the sort in `add()`. A
  change to that sort breaks `Arbit` silently. Pin the ordering in
  `mtgban/base_test.go` before touching it.
- `Mismatch`'s `defaultGradeMap` covers NM/SP/MP/HP plus `PO` at 0. A
  condition that is unknown, or that maps to zero, causes the pair to be
  *skipped* - not zero-priced - because a zero factor cannot be divided by.
  The reference's own grade is divided out before the probe's is applied, so a
  non-NM reference is not compared against a rescaled copy of itself.
- `Card.Legalities` is populated only by the MTGJSON loader, so it is nil for
  every non-Magic card, all eight other games alike.
- `WriteBuylistToCSV` is the one CSV writer taking a middle `creditMultiplier`
  argument; `GetExchangeRate` returns the *reciprocal* - a multiply-to-USD
  factor.

## Git / commits

- **No `Co-Authored-By` lines** in commit messages.
- **Wrap commit subject and body at 80 columns.**
- **Keep the subject short, case by case, and the body to a few lines**:
  the why and the number that proves it. An investigation or table that
  runs longer goes in a file under `docs/`, committed with the change, and
  the body points at it. Comments stay two or three lines the same way.
- Commit messages follow the existing `area: imperative summary` style, e.g.
  `matcher: dedup hashes via per-norm membership set`. Game sub-packages
  use their full path as the area, e.g. `matcher/riftbound: ...`.
- Do not commit compiled binaries, datastore JSON, or scraped CSVs. The
  current `.gitignore` catches `*.exe`/`*.dll`/`*.so`/`*.dylib`/`*.test` and
  `*.csv`, but **not** extensionless Go binaries and **not** datastore
  `*.json` files, so check `git status` before staging.

## Task-specific guides

- **Scraper log noise, refusals, or a suspicious cross-vendor / buy-vs-ask
  price**: read `docs/agents/noise-sweep/README.md` before starting. It
  covers ranking targets from CI refusal lines and site-captured spreads,
  replaying a vendor's feed through the production path rather than
  hand-built `Match()` calls, classifying each shape into scraper table /
  matcher rule / datastore gap, and the report format the work is graded by.
- **Aligning the datastore loaders, or changing what a datastore
  publishes**: read `docs/agents/loader-cleanup/README.md` before starting.
  It covers:
  - auditing what the datastores publish against what the loaders read;
  - proving a refactor changes no backend, with the harness beside it;
  - proving a matching change on the scrapers' own paths;
  - the order a field change needs across datastore-gen, this repo and the
    site.
- **Auditing a game's whole Cardmarket catalog**: read
  `docs/agents/cardmarket-census/README.md` before starting. It walks every
  product the way `Index.walkCatalog` does, grades each landing against the
  datastore's Cardmarket ids, CardTrader's links and the published dumps, and
  lists the shapes found so far with the layer that fixed each.
