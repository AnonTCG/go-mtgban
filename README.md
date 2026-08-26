# go-mtgban (AnonTCG fork)

AnonTCG's fork of [`mtgban/go-mtgban`](https://github.com/mtgban/go-mtgban) — the
Go scraper toolchain we use to pull retail and buylist prices from a curated set
of TCG marketplaces, write them as NDJSON to our R2 bucket, and feed the
downstream `bantool_prices` ETL in
[`AnonTCG-ETL`](https://github.com/AnonTCG/AnonTCG-ETL).

## What's different from upstream

This fork is intentionally minimal. Three categories of change vs
`mtgban/go-mtgban` master:

1. **Workflow set is trimmed** to only the scrapers AnonTCG actually runs (see
   `.github/workflows/`). Lorcana, miniaturemarket, cardmarket, mtgseattle, and
   the sealed-EV workflow live upstream but are removed here.
2. **Two utility workflows are patched** for our infra:
   - `cache-file.yml` drops upstream's "upload to Backblaze B2" and "ping
     mtgban.com" steps — we use Cloudflare R2 and don't ping mtgban.
   - `run-bantool.yml` (a) targets `ANONTCG_OUTPUT_BASE` instead of upstream's
     hardcoded `b2://mtgban-dumps/...`, (b) emits `ndjson` not `json.xz` (the
     format the downstream ETL expects), (c) adds an AllPrintings validation
     step, and (d) drops the mtgban API ping at the end.
3. **One scraper workflow added**: `bantool-vegassingles.yml`.

All Go source code is untouched from upstream. If you find yourself reaching for
a scraper code change, file it upstream first.

## Required GitHub Actions configuration

Settings → Secrets and variables → Actions.

### Repository variables

| Name | Example | Notes |
|---|---|---|
| `ANONTCG_OUTPUT_BASE` | `s3://anontcg-bantool` | Base path bantool writes NDJSON under. Bantool appends `/<game>/<target>` (e.g. `/magic/cardkingdom`). For local testing: `file:///tmp/bantool`. |
| `DATASTORE_MAGIC` | `https://mtgjson.com/api/v5/AllPrintings.json` | Where to fetch the Magic datastore from. |
| `MAX_CONCURRENCY` | `8` | Optional — per-scraper goroutine cap. |
| `CK_PARTNER` | `MTGSOLD` | Card Kingdom affiliate ID. |
| `CSI_PARTNER` | — | Cool Stuff Inc affiliate ID. |
| `CT_PARTNER` | — | Card Trader affiliate ID. |
| `MINT_PARTNER` | — | Mint Card affiliate ID. |
| `MP_PARTNER` | — | Manapool affiliate ID. |
| `SCG_PARTNER` | — | Star City Games affiliate ID. |
| `TCG_PARTNER` | — | TCGplayer affiliate ID. |

### Repository secrets

| Name | Notes |
|---|---|
| `ANONTCG_R2_KEY_ID` | Cloudflare R2 access key id (S3-compatible). |
| `ANONTCG_R2_SECRET` | Cloudflare R2 secret access key. |
| `ANONTCG_R2_ENDPOINT` | Cloudflare R2 endpoint URL, e.g. `https://<acct>.r2.cloudflarestorage.com`. |
| `CARDTRADER_TOKEN_BEARER` | Card Trader bearer token. |
| `MKM_APP_SECRET` | Cardmarket OAuth — unused unless you re-enable the cardmarket workflow. |
| `MKM_APP_TOKEN` | Cardmarket OAuth — same. |
| `SCG_BEARER` | Star City Games API bearer. |
| `SCG_GUID` | Star City Games session GUID. |
| `TCGPLAYER_AUTH` | TCGplayer API auth token. |
| `TCGPLAYER_PRIVATE_ID` | TCGplayer OAuth private id. |
| `TCGPLAYER_PUBLIC_ID` | TCGplayer OAuth public id. |

### What's NOT used (intentionally)

These upstream secrets/variables are not consumed by this fork:

- `B2_KEY_ID_DATASTORE` / `B2_APP_KEY_DATASTORE` / `B2_KEY_ID` / `B2_APP_KEY` — we
  publish to Cloudflare R2, not Backblaze B2.
- `BAN_API_KEY` / `BAN_SECRET` / `BAN_SIGNED_URL_SECRET` — upstream signs URLs
  to ping `mtgban.com/api/load/<target>`; we have no such endpoint.
- `MTGBAN_PUBLISH_DATASTORE` — upstream's gate to skip B2 uploads from forks.
  Since we don't have any B2 steps left, we don't need this gate.
- `OUTPUT_BASE` — superseded by `ANONTCG_OUTPUT_BASE`.

Don't create them in this fork — leaving them unset is the documented state.

## Active scrapers

| Workflow | Output path under `${ANONTCG_OUTPUT_BASE}/magic/` |
|---|---|
| `bantool-abugames.yml` | `abugames/{retail,buylist}/...` |
| `bantool-abugames_sealed.yml` | `abugames_sealed/...` |
| `bantool-cardkingdom.yml` | `cardkingdom/{retail,buylist}/...` |
| `bantool-cardkingdom_graded.yml` | `cardkingdom_graded/...` |
| `bantool-cardkingdom_sealed.yml` | `cardkingdom_sealed/...` |
| `bantool-cardtrader.yml` | `cardtrader/retail/{CT,CT0,CT1DR}.ndjson` |
| `bantool-cardtrader_sealed.yml` | `cardtrader_sealed/...` |
| `bantool-coolstuffinc.yml` | `coolstuffinc/{retail,buylist}/...` |
| `bantool-hareruya.yml` | `hareruya/{retail,buylist}/...` |
| `bantool-magiccorner.yml` | `magiccorner/{retail,buylist}/...` |
| `bantool-manapool.yml` | `manapool/...` |
| `bantool-manapool_sealed.yml` | `manapool_sealed/...` |
| `bantool-mintcard.yml` | `mintcard/...` |
| `bantool-starcitygames.yml` | `starcitygames/...` |
| `bantool-strikezone.yml` | `strikezone/{retail,buylist}/...` |
| `bantool-tcg_index.yml` | `tcg_index/retail/{TCGLow,TCGMid,TCGMarket,TCGDirectLow}.ndjson` |
| `bantool-tcg_market.yml` | `tcg_market/{retail/{TCGPlayer,TCGDirect},buylist/TCGDirectNet}.ndjson` |
| `bantool-vegassingles.yml` | `vegassingles/{retail,buylist}/...` |

## Downstream contract

`AnonTCG-ETL`'s `scripts/etl/bantool_prices` reads from
`${ANONTCG_OUTPUT_BASE}/magic/` via the R2 S3 API and writes to the
`inventory` / `buylist` Postgres tables. The NDJSON shape per row is the
upstream mtgban `InventoryEntry` / `BuylistEntry` plus a top-level `UUID`
field. The downstream parser at
`scripts/etl/bantool_prices/parser.py` is the source of truth for fields
consumed.

## Local development

```bash
go install ./cmd/bantool
bantool -datastore /path/to/AllPrintings.json \
        -output-path file:///tmp/bantool/magic/cardkingdom \
        -format ndjson \
        -cardkingdom
```

The downstream ETL accepts `file://` paths during dev — useful for testing
finish-tagging or new vendor support without round-tripping through R2.

## Upstream sync

```bash
git remote add upstream https://github.com/mtgban/go-mtgban.git
git fetch upstream
git merge upstream/master
# resolve conflicts in .github/workflows/ files we own;
# leave upstream Go source unchanged
```
