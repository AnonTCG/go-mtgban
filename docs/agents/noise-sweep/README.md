# mtgban scraper noise sweep

A step-by-step method for turning scraper matching failures - refusals, and
the more expensive silent mispricings - into either a landed price or a
recorded, verified reason a listing cannot land. Read this whole document
before starting the work it describes; it is written for any AI coding agent
or human contributor working in this repository, not for one tool
specifically. `detectors.md` and `harnesses.md` sit beside it and are read
on demand, as the sections below point to them.

---

Guessing at a rule from an error message alone produces rules that fire on
nothing; the loop below is what actually moves the number.

## Three ways to find what is broken

A refusal is the loud failure. The other two are silent - the run prices the
card, at the wrong card's price - and they are where the money is.

1. **Refusal lines** (`unknown card name`, `unknown variant`, `aliasing
   detected`, `duplicate entry`) in a CI run. The listing went unpriced.
   Corpus: `~/src/claude-scratchpad/ci-sweep/<YYYY-MM-DD>/incidents/`, one
   directory per sweep.
2. **Cross-vendor spread** on the published site: capture `/arbit`, `/reverse`
   and `/global` per game and look for prices that are too far apart. A shop
   never really sells at $1.50 what another buys at $1,000 - a spread that
   extreme usually means two printings folded onto one id. Check `BAN_SIG`
   has not expired *before* capturing - it decodes offline and an expired one
   answers HTTP 200 with an Unauthorized body, so the capture looks fine and
   parses to nothing (`detectors.md`).
3. **Intra-scraper buy-vs-ask ratio**: one scraper that both buys and sells,
   pricing a card on both sides at nearly the same number. A shop buys to
   resell, so its buy price sits well under its ask; when it does not, retail
   and buylist derived the id differently. bantool already prints this.

Detectors 2 and 3 produce **candidates, not verdicts** - plenty are legitimate.
Every candidate goes through the same verification as a refusal: replay the
vendor's feed through master (step 2 of the loop) and prove which side is wrong.
Mechanics for both in `detectors.md`.

## The loop, per target

### 0. Sync first, always

`git fetch origin` and rebase onto fresh `master` in `go-mtgban` **and** check
`datastore-gen`. A stale checkout produces confident wrong findings. Stamp the
datastore's mtime and size - it can be regenerated mid-session, and a
measurement taken on either side of that is not comparable.

Work in your own worktree, never the shared checkout:

```
git worktree add ~/src/claude-scratchpad/worktrees/go-mtgban/<tag> -b <branch> origin/master
```

Source credentials with `set -a; . ~/src/go-mtgban/.env; set +a` and never
print their values.

### 1. Rank the targets

Pull the last run of each GitHub Action, extract every refusal line, and count
per `game × scraper`. Work the noisiest first. Keep the ranking in
`~/src/claude-scratchpad/ci-sweep/<date>/ranked_raw.txt` and the per-target
corpora in `<date>/incidents/<game>--<scraper>.jsonl` (one JSON object per
line: `{"tag","kind","head","ctx":[...]}`).

**Discount two classes before you read the ranking**, or they will send you at
the wrong targets - neither is where the value is:

- **Magic tokens.** They dominated the count before the token work (tcg_market
  ~12.9k lines on 2026-09-10, 382 on 2026-09-24) and they are all one upstream
  shape: mtgjson files each face of a double-faced token as its own row, so
  many sellable products collapse onto one uuid. That is datastore work, not
  scraper work - do not write scraper-side token dedupe or DFC pricing.
  Subtract them and re-rank.
- **Sealed, in every game.** Coverage tops out around 84-91% by construction and
  the remaining misses are mostly products no catalog carries. Leave sealed
  targets parked unless asked for them specifically.

Only a couple of games in flight at once. That limit is for fixes, not the
survey. Before picking a target or fixing anything, present the whole
inventory, categorized by symptom: causes marked unverified until replayed,
exclusions and uninspected sources listed, raw refusal lines kept apart from
distinct listings. An inventory-only request stops there.

### 2. Replay the corpus through the *production path*

This is the step that makes the difference. Do **not** call `matcher.Match`
directly with hand-built inputs - replay the run's own log through the same
function the scraper calls (`resolveProduct` / `resolveProductID` for Star
City, `(*Market).processProducts` for CardTrader, which tries the blueprint's
ids before `Match`), because the scraper's preprocessing is usually where the
fix belongs.

Write the harness as a `zz_*_test.go` file, copy it into the package, run it,
then delete it - never commit it. Keep the source in the durable scratchpad so
the next session can re-run it. Templates: `harnesses.md`.

Output one TSV row per listing attempt, header `side key verdict class where`.
`key` is the vendor's own unit: product id x finish x language, never
condition. `verdict`: landed / twin (a real card left unpriced because a
sibling product holds its printing) / refused / silent (a real card dropped
with no log line) / skipped. `class`: card, or why the row is out of scope
(token, sealed, noncard, foreign, unreleased, unmade, nostock). **Mapping %** =
landed / (landed + twin + refused + silent) over class=card keys, taking the
best verdict per key and counting a skipped card as silent (`harnesses.md`
§G). If a fix moves rows out of scope, recompute Before on the After scope.
This file is the baseline every later measurement is diffed against.

### 3. Probe the datastore before writing any rule

For every refusal shape, dump the actual rows the datastore holds for that
name / number / set - set code, number, name, finish, rarity, promoTypes, uuid.
A rule written from the error message alone will be wrong about which printing
exists. Small Go probe binaries or a python pass over the datastore JSON both
work; see `harnesses.md`.

### 4. Classify every shape before fixing any of them

| Shape | Belongs in |
|---|---|
| Vendor misspells a name/number | scraper preprocess table, keyed literally - **never** edit-distance in the matcher |
| Vendor names a shelf the catalog spells differently | matcher `editionAliases`, or `pooledEditions` when one shelf spans two sets |
| Vendor's number is its own index, not the card's | scraper, keyed by blueprint/sku id when the index names another card |
| Treatment / finish / print-run semantics | matcher rules |
| Vendor sells a non-card (marker, insert, binder label) | scraper → `ErrUnsupported` |
| Vendor sells a printing that was never made | scraper → skip (`ErrUnsupported`), class `unmade`, after a census of the whole shelf or edition; replay the full catalog so only the bogus rows move (#750, #756) |
| One vendor product covers two printings | leave refused, record it |
| A Magic printing the datastore has lacks its TCGplayer id, or carries the wrong one | upstream: a row in a Scryfall CSV (scryfall id, old and new TCGplayer id), checked live; no local id table (#1326) |
| No row exists at all | datastore gap - record, do not paper over |

Datastore gaps are the last resort, not the first explanation. A missing row
says nothing about what was printed: check the vendor's own SKUs first.

A skip, silence or refusal rule fires on every row it matches, not just the
shape it was written for: run its predicate over the whole capture and check
each hit against the loaded backend. "Emblem" caught Heraldic Banner and
Emblem of the Warmind twice (mintcard #476, hareruya #737).

### 5. Fix, then re-measure both directions

Re-run the replay (did it land?) **and** a regression corpus for a vendor that
already works (did anything move?). Grade per listing - gained / lost / moved -
never on recall alone, and read the gains: a new landing can be a new misprice.
A row that goes refused→skipped counts as lost, not landed, unless the backend
holds no printing it could land on. For the `BridgeHelps` games (One Piece,
Riftbound, Lorcana) bantool also runs when the CardTrader bridge fails to load,
so replay with the bridge and again without it: the bridge resolves by id first
and hid 23 One Piece products a name-route change broke. Record the mapping
line (formula in step 2, harnesses.md §G) at the base sha and at the tip, over
the same N. The Cardmarket walk harness is the standing regression corpus for
Flesh and Blood; build the equivalent before touching shared matcher code for
any other game.

### 6. Pin each rule with a test

Inline fixture of datastore rows copied **verbatim** from the real datastore,
in the matcher package; table tests in the scraper package. A rule with no test
is a rule the next datastore release silently breaks.

Revert the rule alone and re-run its test: the test must fail. A test that
still passes, or that calls a copy of the code it means to check, pins
nothing.

### 7. Gate, commit, PR

```bash
export GOTOOLCHAIN=$(go mod edit -json | jq -r .Toolchain)   # as CI
go build ./... && go vet ./... && ! "$(go env GOROOT)/bin/gofmt" -s -l . | grep . \
  && go run github.com/mgechev/revive@v1.17.1 -set_exit_status -config .revive.toml ./... \
  && go run honnef.co/go/tools/cmd/staticcheck@2026.2.2 ./... \
  && go test -count=1 ./...
```

This mirrors ci.yml's `style` job, which also runs `go test -race ./...` with
no datastore set, so a new test that reads one must skip without it.

Commit matcher and scraper **separately**, subject and body wrapped at 80
columns, the *why* in the message rather than in comments. Open the PR against
`master` - never stack a PR on another PR's branch.

### 8. A fix is invisible until the next run publishes

The site serves the last published run, so a merged matcher fix changes nothing
you can see until a new GitHub Action run uploads. Schedule one if you need to
confirm, and never grade a fix by reloading the page - grade it by replaying
the feed. Related: a local run is not what the site serves; read the published
dump (`b2://mtgban-dumps`) when the question is what customers actually saw.

### 9. Report - always, whichever detector started it

Two deliverables, every time. Neither is optional and neither is a paragraph of
prose: the reader is deciding what to work on next.

**a. Results - mapping % first, one row per target.** N is the same on both
sides; After is measured at the merged tip, by a replay or the first
post-merge CI tally - a projection is not an After, and unmerged work gets
none:

| Game | Target | N | Before | After | +Landed | Moved | Left (why) | Basis |
|---|---|---|---|---|---|---|---|---|
| Flesh and Blood | cardmarket (+market) | 16,926 products | 99.24% | 99.25% | +1 | 0 | 127: 87 twins, 40 refused | replay base→tip |

Moved counts wrong landings corrected; turning one into a refusal lowers the
rate. Roll up per game as "k of n targets ≥ 99%, worst target", never a mean
of percentages; raw refusal lines go in a separate noise table. Every "left"
figure gets a one-line reason, and if a target regressed anywhere, say so
before the wins.

**b. Gaps - per game, with example listings.** Everything the datastore does
not carry, quoted as the vendor wrote it plus the row that ought to exist:

```
Romance Dawn (Pre-Errata): 39 of 78 products have no row   [rows-absent, 39 lines]
  cardmarket 768139 "Caribou (OP01-007)"  ->  want OP01 / OP01-007 / Caribou, variant "Pre-Errata"
```

Classify each as set-absent / rows-absent / finish-absent / numbering-absent /
name-wrong / product-not-a-card, and give a line count so the work can be
ranked. **Before listing a gap, have a separate pass try to refute it**
against the loaded backend: another spelling, another set code (promo and
sibling sets too), the detail in `variant`/`promoTypes`/finish, the number
with a prefix or suffix, another language, and the vendor's own id
(`b.MatchID`, or `b.ConvertID(matcher.IDSpaceCardmarket, id)`). It refuted
26 of 59 claimed gaps on 2026-09-07 and 16 of 22, in whole or part, on
2026-09-24.

Land both in `~/src/claude-scratchpad/ci-sweep/STATUS.md` (PR, harness path,
before/after, a `Residue:` line per leftover shape) and put the verified gaps in
the `project-datastore-gaps` memory so datastore-gen work can be batched.

## Hard rules

- Never work in `/Users/koda/src/go-mtgban` or `mtgban-website` directly; use a worktree.
- Never `git stash` in these repos (the stack is hundreds deep), never `git add -A`.
- Never push `master`; never open a PR based on another PR's branch.
- Magic's `allprintings5.json` is golden: a Magic mismatch is go-mtgban's bug,
  not a gap. Magic tokens are the one exception and they are **low priority** -
  they need datastore work first, so do not spend a sweep on them and do not
  write scraper-side token dedupe or DFC pricing.
- Sealed is **low priority** in every game: the ceiling is ~84-91%, so a sealed
  target that is not at zero is usually finished, not broken.
- The Magic loader does not carry every mtgjson row: `art_series` and
  `front_card` layouts are deliberately dropped (`matcher/magic/mtgjson.go`).
  A row being in `allprintings5.json` does not mean the matcher can reach it -
  check the loaded backend, not the raw JSON, before calling something present.
- Vendor ids are theirs: a wrong authoritative id is the vendor's bug, not
  something to steer around with a heuristic.
- Game Nerdz: one crawl per session, buylist only; prefer replaying a capture.
- `tcgdirectnet` and `syp` are wrong at the source. Their spreads and ratios
  cannot be fixed here - exclude them from any ranking rather than chasing them.
- No arbitrary caps: fix the real termination condition instead.
- Each CI job configures exactly ONE game. A test asks for the datastore it
  needs through the package's helper (`withMagic(t)`, `withLorcana(t)`) and
  skips where that one is not configured; there is no shared datastore for
  another test to leave behind. Before pushing a new test file, re-run the
  package under each job's single datastore (`env -u ... ONE_PATH=... go test
  ./pkg/`), not just your full local env.

## Numbers this method produced (2026-09-06/07)

| Target | Refusal lines | After |
|---|---|---|
| Star City Flesh and Blood | 158 | 116 land, 2 skipped, 40 datastore |
| CardTrader Flesh and Blood | 161 | 95 land |
| CardTrader Yu-Gi-Oh | 1,029 | 701 land |
| CardTrader Pokemon | 746 | 366 land, 20 skipped |
| Cool Stuff One Piece | 223 | 69 fixed (dedupe + spelling) |

Every one of those was found by replaying the run's own log, not by reading the
error text and guessing.
