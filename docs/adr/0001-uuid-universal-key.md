# ADR-0001: The mtgmatcher UUID is the universal key

**Status:** Accepted
**Date:** 2026-06-28 (amended 2026-08-08 for the game-agnostic matcher;
2026-09-14 for the `mtgban.Game` type)
**Deciders:** Maintainer (Vittorio Giovara)

> **Amendment note.** The decision below was not reversed; the world it
> describes grew. `mtgmatcher` now serves three games instead of one, so the
> text has been updated to say what the universal key *is* today and which of
> the original sub-claims were superseded. Superseded statements are called
> out where they occur rather than silently rewritten.
>
> **Amendment, 2026-09-14.** A scraper is now *built* from the `mtgban.Game`
> type rather than from each vendor's own spelling of a game, so the per-game
> tables named below are unexported and keyed by it; `tcgplayer.SupportedGames`
> is now `tcgGames`. The vendors' own spellings remain exported, since each
> package's API helpers still take one. The decision is untouched - the uuid is
> still the universal key, and the tables still hold the only multi-game
> knowledge a scraper has.

> **Amendment (2026-09-10).** `LoadDatastore` and `LoadDatastoreFile` were
> removed in #519. A datastore is opened by the name of its game,
> `Open(name, reader)`, and installed with `SetGlobalDatastore`; nothing
> sniffs a format any more, and bantool names the game from the scraper it
> runs. The text below describes the loaders as they were.

> **Amendment (2026-09-15).** There is no global backend left to install into:
> `SetGlobalDatastore` and the package-level wrappers are gone, `Open` hands
> the `*Backend` to whoever asked for it, and every lookup is a method on it.
> Two statements here lean on the global - "installed with
> `SetGlobalDatastore`" in the note just above, and "The global backend holds
> exactly one game at a time" under Decision - and describe the world as it
> was; read them as one backend per game, held by whoever loaded it. The
> decision is untouched: the uuid is still the universal key, and it is still
> only meaningful against the backend that minted it. See
> [ADR-0004](0004-localized-matcher-and-scraper-registry.md).

> **Amendment (2026-10-02).** "Never in the scraper" under Decision is
> narrower in practice, and AGENTS.md says where. A vendor's own identifier
> that is wrong, or shared by two products, is the scraper's to distrust
> (`starcitygames/README.md`). A vendor that publishes no identifier resolves
> by its wording and shelves in its own preprocess (`coolstuffinc/README.md`).
> A rule one vendor's evidence would justify, but that moves far more
> landings than that evidence covers, is guarded in that vendor's scraper
> (#716). The decision is untouched: the uuid is still the universal key, and
> what card a listing is still comes out of `mtgmatcher`.

## Context

go-mtgban ingests inventory/buylist data from ~23 stores, each with its own
naming, set labels, foil/finish conventions, promo tags, and internal IDs.
Downstream analysis (arbitrage, price comparison, sealed EV) only works if "the
same card" from two different stores collapses to one identity. Two broad
designs exist: let each scraper emit its own normalized identity, or route
every store record through one central identity resolver.

Since the original acceptance, the repository stopped being Magic-only.
`mtgmatcher` carries datastores for Magic, Lorcana and Riftbound, each with a
different source format and a different native card id. That widens the
question the decision has to answer: not only *which* key, but *whose* key, and
what a key means when several games could in principle be resolved in one
process.

## Decision

Every record keys on a single canonical identity — the **mtgmatcher UUID**.
Its value is the native card id of whichever game's datastore is loaded:

- **Magic:** the MTGJSON `uuid` (`mtgmatcher/magic/mtgjson.go`).
- **Lorcana:** the LorcanaJSON numeric card id rendered as a decimal string —
  the loader sets `UUID: fmt.Sprint(card.ID)` in `mtgmatcher/lorcana/lorcana.go`
  — so a Lorcana uuid looks like `1040`.
- **Riftbound:** the official card-gallery id, which is set-code shaped:
  `ogn-246-298`, `ven-t04`, `pr-678052` (`mtgmatcher/riftbound/riftbound.go`).

*(Superseded: the original text said "the MTGJSON UUID, or a Lorcana
equivalent". There are now three schemes, one per game. What a game registers
with `mtgmatcher.RegisterGame` from its `register.go` is its datastore loader;
the uuid scheme comes along with it, because the loader is what mints the
uuids.)*

A uuid identifies a **(printing, finish) pair**, not merely a printing. Magic
lets the nonfoil printing keep mtgjson's bare id and suffixes the others — `_f`
for foil, `_e` for etched, both declared in `mtgmatcher/backend.go`. Every
datastore game takes its uuids as datastore-gen publishes them: one per
printing TCGplayer prices, ending in that printing's name, as Ariel - Singing
Mermaid's `1951`, `1951_coldfoil` and `1951_holofoil` do. A loader reads the
finish off the entry's `finish` field, never off the uuid (`docs/finishes.md`),
so for these games the suffix is the builder's spelling, not the key's
definition.
Loaders do not leave resolution to string surgery: each `Card` carries a `FoilUUIDs` map from finish
name to the uuid that carries it, and `Backend.output()` (`mtgmatcher/card.go`)
pulls the requested finish out of that map, falling back to the suffix rules
only for cards built without one. Registering finishes explicitly is what lets
Lorcana express finishes that no `_f`/`_e` vocabulary could name.

Scrapers stay **thin translators**: they parse store text into an `InputCard`
and call `mtgmatcher.Match()` / `MatchId()`. All correctness about *what card
this is* lives inside `mtgmatcher` — the game-agnostic `Match()` pipeline in
the core package plus the per-game `GameRules` implementations in
`mtgmatcher/magic`, `mtgmatcher/lorcana` and `mtgmatcher/riftbound` — never in
the scraper. Core `Match()` dispatches its game-specific stages (`Prefilter`,
`AdjustName`, `AdjustEdition`, `FilterPrintings`, `FilterCards`, the
unsupported and promo-tag predicates) through the `GameRules` interface in
`mtgmatcher/rules.go`, which a loader attaches to its `Backend` via `SetRules`.
The parallel `SimpleSearch` entry point is gone. It was Lorcana's own matching
path — name, number and foil, bypassing the Magic pipeline entirely — and a
couple of Magic scrapers had begun calling it directly too, so it was well on
its way to becoming a second identity resolver. Every game, and therefore every
scraper, now resolves through the one `Match()`.

Two properties follow from the key being the game's own id, and both matter to
anyone storing these strings:

**The namespace is per-game and only meaningful against the loaded
datastore.** The global backend holds exactly one game at a time —
`LoadDatastore` installs the first registered loader that succeeds — and the id
shapes overlap. A Lorcana uuid is a plain number, and `MatchId` also accepts
plain numbers as *external* TCGplayer product ids resolved through
`ExternalIdentifiers`. Nothing in the string says which game or which
namespace it came from; that is decided entirely by which datastore is loaded.
A uuid persisted in a price record is therefore only interpretable together
with the game it was produced under.

**`ExternalIdentifiers` is the sanctioned bridge from a store-native id.**
Rather than teaching scrapers to reconstruct identity from a vendor id, each
loader registers the foreign ids its source data carries into
`Backend.ExternalIdentifiers`, mapping each one to the mtgmatcher uuid. The
Magic loader registers four (`mtgjsonId`, `scryfallId`, `tcgplayerProductId`
and `tcgplayerEtchedProductId`, first id to claim a uuid wins); the Lorcana and
Riftbound loaders register the one their exports expose,
`tcgplayerProductId`. `MatchId` looks up the input in `UUIDs` first and falls
back to `ExternalIdentifiers`, so a scraper that has a clean product id can
hand it over verbatim and still land on the canonical key.

On the scraper side, the only multi-game knowledge that exists is a lookup
table: `tcgGames` (`tcgplayer/game.go`) maps an `mtgmatcher.Game` to the TCGplayer
category serving it, and both generic TCGplayer scrapers — `TCGGame` and
`TCGGameIndex` — refuse to be constructed for a game absent from it. Magic
is in it like every other game. The generic pair resolves identity solely
through `mtgmatcher`; supporting one more game there is one table entry plus
a matcher datastore.

## Alternatives considered

- **Per-scraper identity** — each scraper resolves its own canonical name/set.
  Rejected: N copies of the hardest logic, drifting independently; a matching
  bug would need fixing in many places and would silently differ across stores.
  The multi-game work re-tested this and reached the same answer more strongly:
  the one place where a second identity path had grown, `SimpleSearch`, was
  deleted rather than extended to a third game.
- **Match on `(name, set, number)` tuples.** Rejected: promos, variants, foils,
  languages, and reprints make a tuple ambiguous; the UUID already encodes all
  of it.

## Consequences

- **Easier:** adding a store is mostly preprocessing — identity is free; a
  matching fix in `mtgmatcher` corrects every store at once. Adding a *game* is
  likewise contained: a loader, a `GameRules` implementation and a
  `register.go`. No identity logic moves into a scraper and nothing downstream
  changes at all, because everything downstream keys on the uuid and never
  inspects it; the scraper-side cost is one `mtgmatcher.Game` constant (alongside
  `GameLorcana` and `GameRiftbound`, and listed in `mtgmatcher.AllGames`) and, for
  TCGplayer coverage, one `tcgGames` entry.
- **Harder:** `mtgmatcher` is a large, central, data-heavy package and the
  single point of failure for correctness — hence its per-game regression
  replays and the "tables before code" rule. There are three corpora now, one
  beside each game's rules: `mtgmatcher/magic/testdata/magic_test_data.json`
  (gated on `ALLPRINTINGS5_PATH`),
  `mtgmatcher/lorcana/testdata/lorcana_test_data.json` and
  `mtgmatcher/riftbound/testdata/riftbound_test_data.json`, each driven by a
  `matcher_test.go` in its own package. *(Superseded: the original text named a
  single replay corpus.)*
- **Sharp edge:** because the key is the game's own id, uuids from different
  games are not distinguishable by inspection and must not be pooled in one
  index without an out-of-band game tag.
- **Rule of thumb:** if a card matches wrong, fix it in `mtgmatcher`, not in
  the scraper — and usually in a data table rather than in code. For Magic that
  means `mtgmatcher/magic/variants.go`, `mtgmatcher/magic/callbacks.go` and
  `mtgmatcher/magic/table.go`; for Lorcana and Riftbound it means the game
  package's loader and its `rules.go` (for example the Riftbound promo-set
  gating in `mtgmatcher/riftbound/rules.go`). *(Superseded: the original text
  listed only the Magic file set, and `callbacks.go` has since moved into the
  `magic` package.)*

## Action items

1. [ ] Keep new identity logic in `mtgmatcher` — a data table where possible,
   otherwise the game's `GameRules` — never in a scraper (enforced by review;
   see `AGENTS.md`).
2. [ ] Make `SPECIFICATIONS.md` §2.5 describe the post-refactor heuristic
   layer. Its title names a `filter.go` that `mtgmatcher` does not have; the
   filtering it describes is now `FilterPrintings`/`FilterCards` in
   `mtgmatcher/magic/rules.go`, with the per-card fixups in
   `mtgmatcher/magic/callbacks.go`.
