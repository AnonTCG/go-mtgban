# CLAUDE.md

Guidance for Claude Code (claude.ai/code) when working in this repo.

## What this repo is

AnonTCG's fork of [`mtgban/go-mtgban`](https://github.com/mtgban/go-mtgban) — a
Go scraper toolchain. It produces NDJSON dumps of retail + buylist prices from
~17 TCG marketplaces and writes them to our Cloudflare R2 bucket. The
downstream consumer is `scripts/etl/bantool_prices` in
[`AnonTCG-ETL`](../AnonTCG-ETL).

See `README.md` for required env vars and the full active-scraper table.

## Hard rules for this repo

1. **This is our fork — modifying Go source is allowed.** (Reversed 2026-06-09;
   the old "don't touch Go, upstream PR first" rule is retired.) We maintain
   AnonTCG-specific deviations directly in the Go source when our product needs
   diverge from mtgban's — e.g. the `TCGPlayer` source now emits the bare item
   `LowPrice` (+ shipping in `CustomFields`) instead of the delivered
   `LowestListingPrice`, because mtgban uses that source for retail *display*
   while we value cards at NM for sealed EV / exports and the shipping bundle
   inflated everything. Keep each deviation a focused, well-commented commit
   that explains *why we differ from upstream*, so rebases against
   `mtgban/master` stay tractable. Still pull upstream fixes; just don't treat
   lockstep as a mandate.
2. **Don't add B2 / Backblaze code paths.** We use Cloudflare R2 (S3-compatible
   via `AWS_*` env names). The upstream B2 paths were intentionally removed.
3. **Don't add `mtgban.com` API calls.** Upstream pings their own backend after
   bantool runs — we have no analog endpoint. Don't restore those steps.
4. **Match downstream contract.** The downstream parser at
   `scripts/etl/bantool_prices/parser.py` in AnonTCG-ETL is the authoritative
   shape consumer. Don't change the NDJSON row format without updating that
   parser in lockstep.
5. **Pre-launch clustering does NOT apply here.** This is a Go scraper fork
   tracking upstream — workflow patches should be self-contained commits that
   survive rebases against upstream master.

## Why workflows look like they do

- `cache-file.yml` — downloads MTGJSON `AllPrintings.json` and caches it for
  consumers. Upstream's B2-upload + ping steps stripped.
- `run-bantool.yml` — invoked by every per-scraper workflow. Reads
  `ANONTCG_OUTPUT_BASE` (repo variable) for the R2 destination,
  `ANONTCG_R2_*` (secrets) for credentials. Validates the AllPrintings
  datastore before running bantool to fail fast on bad fetches.
- `bantool-<vendor>.yml` files — one per active scraper. Each is a thin wrapper
  that calls `run-bantool.yml` with `target: <vendor>`.

## Common commands

```bash
# Build the scraper toolchain locally
go install ./cmd/bantool

# Run a single scraper to local disk (useful for testing finish/UUID handling).
# The output path is a bare path and the directory must exist — bantool has no
# file:// scheme (empty URL scheme maps to the local-filesystem bucket).
mkdir -p /tmp/bantool/magic/cardkingdom
bantool -datastore /path/to/AllPrintings.json \
        -output-path /tmp/bantool/magic/cardkingdom \
        -format ndjson \
        -cardkingdom

# Sync upstream into this fork
git fetch upstream
git merge upstream/master
```

## What's intentionally NOT here

- Per-game workflows (`bantool-*_lorcana.yml`, `*_onepiece.yml`,
  `*_riftbound.yml`) — the multi-game *code* (mtgmatcher/lorcana, onepiece,
  riftbound + per-game scraper variants) IS merged and compiles, but the
  workflows can't run: each needs a per-game datastore file that upstream
  builds with `mtgban/lorcana-datastore` / `riftbound-datastore` /
  `datastore-gen` and publishes only to their private B2 bucket. Enabling a
  game = self-host that builder + add the workflow + extend the AnonTCG-ETL
  parser/schema (per-game uuid formats and finish suffixes differ from
  MTGJSON's). Output would land under `<game>/` in R2, which the ETL ignores
  (it lists only `magic/`).
- `bantool-sealed_ev.yml` — upstream computes a sealed-EV dump; we compute our
  own in `AnonTCG-ETL/scripts/etl/sealed_ev`.
- `bantool-miniaturemarket_sealed.yml`, `bantool-mtgseattle.yml`,
  `bantool-cardmarket*.yml`, `bantool-trollandtoad.yml`,
  `bantool-arcanafrisia.yml` — vendors we don't currently consume.
- Upstream's `ci.yml` (style + per-game test matrix) — not adopted yet;
  candidate follow-up (would need the `DATASTORE_MAGIC` repo variable).

If we ever decide to enable one of these, copy the YAML back from upstream and
verify the corresponding scraper credentials are wired up in repo secrets.

## Fork deviations (post-realign)

The fork was re-cut on upstream/master (game-agnostic mtgmatcher, 7 games).
Everything below is the complete deviation surface — keep it small:

1. `mtgmatcher/magic/tcgplayer_id_override.*` — TCGplayer id-override
   subsystem. Call site in `mtgmatcher/magic/mtgjson.go`. Two maps, merged at
   load with the manual one winning:
   - `tcgplayer_id_override.json` — HAND-CURATED, for MTGJSON publishing a
     *wrong* id (currently the HOB Gleaming Splendor pair, which needs an
     alt-foil DELETE no report can express). Edit by hand; each entry records
     the broken upstream values so `classify()` can flag it stale.
   - `tcgplayer_id_override_generated.json` — GENERATED, for MTGJSON
     publishing *no* id at all (3,810 of AnonTCG's 110,299 entities on
     2026-09-24; Scryfall, MTGJSON's source for the field, has the same gap).
     Never hand-edit — regenerate with `cmd/gen-tcgid-override` from a
     `cmd/tcgid4scryfall` report. Entries carry an empty `upstream_primary`,
     which is what makes `classify()` report them Redundant once MTGJSON
     catches up.
2. `cmd/gen-tcgid-override` — builds the generated map above. Also writes
   AnonTCG-ETL's copy via `-etl-o`: that repo backfills
   `mtg_entities.identifiers.tcgplayer_product` at ingest (feeding
   `search_catalog`, the upload RPCs and the website's TCGplayer links), and
   reads our DB rather than the matcher, so it cannot use the embedded map.
   One command writes both so they cannot drift; `-check` fails on drift.
   `.github/workflows/tcgid_override_refresh.yml` runs it biweekly and opens
   a PR — it never pushes to master, and never writes the other repo.
   `cmd/tcgid4scryfall` itself is upstream's and stays UNMODIFIED.
3. `tcgplayer/tcgplayer.go` + `manapool/manapool.go` — raw item prices for
   valuation (bare `LowPrice`; shipping / buyer-fee rate in `CustomFields`).
   Also the `"NON FOIL"` SKU-refetch fix (upstream still has `"NORMAL"`), and
   the SKU gate: upstream checks `skusMap` BEFORE `needsNewTCGSKUs` and so
   skips any uuid missing from `TcgplayerSkus.json` — exactly the cards the
   generated override exists to fill. Ours lets the refetch flag through
   without a cached entry, which is what gets them per-SKU
   TCGPlayer/TCGDirect rows rather than index prices only.
4. `tcgplayer/utils.go` — affiliate id baked into `PartnerProductURL`.
5. `starcitygames/` — the WHOLE package is our fork's sell-list
   implementation, not upstream's catalog-API rewrite. Production runs
   buylist-only on a bearer scraped from the public sellyourcards app.js
   (no stored credential); upstream's catalog API needs an `x-api-key` we
   don't have (the scraped bearer gets 401 there — tested 2026-08-19).
   `simplesearch.go` carries a package-local copy of the deleted
   `mtgmatcher.SimpleSearch`; the serialized check uses
   `magic.HasSerializedPrinting`. The Magic-only bantool wiring keeps
   `SCG_GUID`/`SCG_BEARER`/`SCG_BUYLIST_ONLY`; upstream's other-game SCG
   targets were removed (they need the catalog scraper). On upstream syncs,
   keep `starcitygames/` ours and re-check the three core call sites.
6. `.github/workflows/` — R2 output, no B2/mtgban.com, our cron schedule,
   21 Magic targets (+ manapool_index). The exit-2 "Check bantool status"
   gate is deliberately `if: false`: SCG buylist-only runs ALWAYS exit 2
   ("seller SCG has no data" is expected), so a blanket gate turns every
   SCG run red — see the comment in `run-bantool.yml`.
