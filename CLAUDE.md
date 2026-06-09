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

# Run a single scraper to local disk (useful for testing finish/UUID handling)
bantool -datastore /path/to/AllPrintings.json \
        -output-path file:///tmp/bantool/magic/cardkingdom \
        -format ndjson \
        -cardkingdom

# Sync upstream into this fork
git fetch upstream
git merge upstream/master
```

## What's intentionally NOT here

- Lorcana scrapers (`bantool-cardmarket_lorcana.yml`,
  `bantool-coolstuffinc_lorcana.yml`, etc.) — AnonTCG is MTG-only.
- `bantool-sealed_ev.yml` — upstream computes a sealed-EV dump; we compute our
  own in `AnonTCG-ETL/scripts/etl/sealed_ev`.
- `bantool-miniaturemarket_sealed.yml`, `bantool-mtgseattle.yml`,
  `bantool-cardmarket*.yml` — vendors we don't currently consume.

If we ever decide to enable one of these, copy the YAML back from upstream and
verify the corresponding scraper credentials are wired up in repo secrets.
