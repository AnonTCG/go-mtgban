# Workflows

Most files here are bantool workflows, one per store a game prices:
`bantool-<game>-<store>.yml`. Each is a thin caller of the reusable
`run-bantool.yml`. The rest:

| File | What it does |
|---|---|
| `run-bantool.yml` | Reusable: runs one store and publishes its dumps (below). |
| `cache-file.yml` | Reusable: downloads a file (Magic's datastore, Cardmarket ids) and caches it for a run. |
| `cache-b2.yml` | Reusable: caches another game's datastore from the private bucket for `ci.yml`'s tests. |
| `ci.yml` | Build, vet, lint and the test suites. |
| `hostile-review-wake.yml` | Wakes the hostile review on PR activity. |

## A store's workflow

```yaml
name: lorcana / tcg_index

on:
  workflow_dispatch: {}
  repository_dispatch:
    types: [lorcana-tcg_index, lorcana-all]
  schedule:
    - cron: "17 1,13 * * *"

permissions:
  contents: read

concurrency:
  group: bantool-tcgplayer
  cancel-in-progress: false
  queue: max

jobs:
  deploy:
    name: ${{ github.workflow }}
    uses: ./.github/workflows/run-bantool.yml
    with:
      store: tcg_index
      game: "lorcana"
      affiliate: ${{ vars.TCG_PARTNER }}
      datastore-filepath: b2://mtgban-datastore/lorcana/lorcana.json.xz
    secrets: inherit
```

The same game and store appear four times: in the file name, the `name:`
line, the `store:` and `game:` inputs, and the first dispatch type. Each one
is matched on somewhere else:

- the site's admin page opens the workflow file by name, dispatches
  `<game>-<store>`, and spots a running store by its run name,
  `<game> / <store>`;
- `run-bantool.yml` builds the dump path and the reload URL from the inputs.

`cmd/bantool/workflows_test.go` fails CI unless every registered store has
exactly one workflow and all four agree.

## What a run does

`run-bantool.yml`:

1. Restores whatever the caller cached (datastore, Cardmarket ids) and
   builds bantool.
2. Runs `bantool -game <game> -store <store> -datastore <datastore-filepath>
   -output-path b2://mtgban-dumps/<game>/<store> -format json.xz`. Each
   retail and buylist list the store makes lands at
   `<game>/<store>/<retail|buylist>/<shorthand>.json.xz`.
3. If `vars.RELOAD_GAMES` names the game, signs
   `http://<game>.mtgban.com/api/load/<store>` with `BAN_SECRET` and calls
   it. The site lists `<game>/<store>/` and loads what it finds, so a new
   store goes live on its first run (mtgban-website SPECIFICATION.md §2.3).
   A game not in `RELOAD_GAMES` prices without telling its site.
4. bantool's exit code 2 means some of the store's lists failed. The run
   still uploads and reloads the rest, then fails the job.

A job may run for 24 hours (`timeout-minutes: 1440`).

## Datastores

The two shapes differ on purpose, so copy a workflow of the same game:

- **Magic** runs a `cache-datastore` job first (`cache-file.yml` on
  `vars.DATASTORE_MAGIC`) and passes the cached path and its key.
- **Every other game** passes `b2://mtgban-datastore/<game>/<game>.json.xz`,
  which bantool reads straight from the bucket.

A Cardmarket workflow of any game, Magic included, passes
`b2://mtgban-datastore/<game>/cardmarket_catalog.json.xz` as its id map.

## Triggers

- **`schedule`:** each workflow has its own cron.
- **`workflow_dispatch`:** the "Run workflow" button on the workflow's page,
  or `gh workflow run bantool-<game>-<store>.yml -R mtgban/go-mtgban`.
- **`repository_dispatch`:** `<game>-<store>` reruns one store, which is
  what the site's admin refresh sends. `<game>-all` starts every store of a
  game at once. A repository dispatch has no button, so send it with
  `gh api repos/mtgban/go-mtgban/dispatches -f event_type=lorcana-all`.

## Concurrency and runners

Most stores share their vendor's concurrency group (`bantool-tcgplayer`,
`bantool-cardmarket`, `bantool-cardtrader`, ...) with `queue: max`, so runs
against one vendor wait for each other instead of overlapping or
cancelling. A workflow without a group runs whenever its cron fires.

Everything runs on `ubuntu-latest` except Magic, Pokemon and Yu-Gi-Oh's
`cardmarket_market`, whose catalogs take longer than GitHub's hosted jobs
allow. They pass `runs-on: '["self-hosted", "cardmarket-market"]'` and run
on the runner in [`.github/runner/`](../runner/README.md).

## Variables and secrets

Workflows pass `secrets: inherit`. They read:

- **vars:** each vendor's affiliate code (`TCG_PARTNER`, `CK_PARTNER`,
  `MKM_PARTNER`, `CT_PARTNER`, `CSI_PARTNER` and the rest),
  `DATASTORE_MAGIC`, `MAX_CONCURRENCY` and
  `RELOAD_GAMES`;
- **secrets:** the dumps bucket key (`B2_KEY_ID`, `B2_APP_KEY`), the
  datastore bucket key, `BAN_SECRET` for the reload signature, and each
  vendor's API credentials.

## Adding a store

1. Register the scraper (AGENTS.md, "Adding a scraper").
2. Copy a workflow of the same game. Change the store in the file name,
   `name:`, `store:` and the first dispatch type, then pick a cron slot, the
   vendor's concurrency group and its affiliate variable.
3. Merge. The store's first run publishes its dumps and the site picks it
   up, with nothing to configure there.
