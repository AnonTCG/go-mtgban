# Harness templates

Copies of the harnesses that worked, generalised. Drop into the scraper's
package as `zz_*_test.go`, run, then delete the copy - the source of truth
lives in `~/src/claude-scratchpad/ci-sweep/<date>/<tag>/`.

Datastore env vars: `ALLPRINTINGS5_PATH`, `FLESHANDBLOOD_PATH`, `YUGIOH_PATH`,
`POKEMON_PATH`, `ONEPIECE_PATH`, `LORCANA_PATH`, `RIFTBOUND_PATH`,
`GUNDAM_PATH`, `PALWORLD_PATH`. Source them with
`set -a; . ~/src/go-mtgban/.env; set +a` and never echo their neighbours.

## A. Catalog-style replay (Star City, Cool Stuff, Strike Zone …)

The incident's `head` carries the whole listing, so a regex over it rebuilds the
product. Reconstruct the vendor's own struct and call the scraper's resolve
function - not `Match` - so the scraper's preprocessing is under test too.

```go
package starcitygames

import (
	"bufio"; "encoding/json"; "errors"; "fmt"; "os"; "regexp"; "strings"; "testing"

	"github.com/mtgban/go-mtgban/internal/datastore"
	"github.com/mtgban/go-mtgban/matcher"

	_ "github.com/mtgban/go-mtgban/matcher/fleshandblood"
)

var zzHead = regexp.MustCompile(`(unknown variant|unknown card name|aliasing detected) for "(.*?)" \[(.*?) #(.*?)\] sku=(\S+)`)

func TestZZReplay(t *testing.T) {
	path := os.Getenv("ZZ_INCIDENTS")
	if path == "" { t.Skip("ZZ_INCIDENTS not set") }
	b, err := datastore.Read("fleshandblood", os.Getenv("FLESHANDBLOOD_PATH"))
	if err != nil { t.Fatal(err) }

	f, err := os.Open(path); if err != nil { t.Fatal(err) }
	defer f.Close()
	out, _ := os.Create(os.Getenv("ZZ_OUT")); defer out.Close()
	fmt.Fprintln(out, "side\tkey\tverdict\tclass\twhere")

	seen := map[string]bool{}
	var n, landed, skipped int
	sc := bufio.NewScanner(f); sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var rec struct{ Head string `json:"head"` }
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil { t.Fatal(err) }
		m := zzHead.FindStringSubmatch(rec.Head)
		if m == nil { continue }
		key := m[5] // the sku is this catalog's own product id; fold in finish/language where the head carries them
		if seen[key] { continue }
		seen[key] = true; n++

		p := CatalogProduct{Name: m[2], Set: m[3], SKU: m[5], /* … */}
		id, err := resolveProductID(b, GameFleshAndBlood, p)   // the PRODUCTION path

		verdict, where := "landed", ""
		switch {
		case errors.Is(err, matcher.ErrUnsupported):
			verdict, where, skipped = "skipped", "unsupported", skipped+1
		case err != nil:
			verdict, where = "refused", err.Error()
		default:
			landed++
			co, _ := b.GetUUID(id)
			where = co.Name + "|" + co.SetCode + "|" + co.Number + "|" + co.Finish + "|" + co.Language + "|" + strings.Join(co.PromoTypes, "+") + "|" + id
		}
		fmt.Fprintf(out, "\t%s\t%s\tcard\t%s\n", key, verdict, where) // non-card skips: set class by hand
	}
	t.Logf("replayed %d keys: %d land, %d skipped, %d refused", n, landed, skipped, n-landed-skipped)
}
```

Run and clean up in one go (`D=~/src/claude-scratchpad/ci-sweep/<date>`):

```bash
cp $D/<tag>/zz_replay_test.go <pkg>/ && \
  ZZ_INCIDENTS=$D/incidents/<game>--<scraper>.jsonl ZZ_OUT=$D/<tag>/replay-1.tsv \
  go test -count=1 -run TestZZReplay ./<pkg>/ -v 2>&1 | grep 'zz_replay\|FAIL'
rm <pkg>/zz_replay_test.go
```

## B. CardTrader replay (blueprint-driven)

CardTrader's incident carries the raw blueprint in `ctx[1]`. Parse it back into
a `Blueprint` and replay it through `processProducts`, the listing loop
itself. It tries the blueprint's TCGplayer and Cardmarket ids before `Match`,
and it applies filters that a hand-built `Match` skips. A listing's finish is
not in the log, so send one synthetic listing per plausible finish, and count
the line as landed if any of them lands.

```go
var zzBP = regexp.MustCompile(`&\{ID:(\d+) Name:(.*?) Version:(.*?) GameID:\d+ CategoryID:(\d+) ExpansionID:(\d+) ScryfallID:\S* TCGplayerID:(\d+) CardMarketIDs:\[(.*?)\] Expansion:\{Name:(.*?) Code:(.*?)\} Properties:\{Number:(.*?) Language:(.*?)\}`)

// …per blueprint, per plausible finish:
bp.GameID = GameFleshAndBlood // processProducts drops any other GameID silently
ct := &Market{backend: b, gameID: GameFleshAndBlood, blueprints: map[int]*Blueprint{bp.ID: &bp},
	logCallback: func(f string, a ...any) { logged = append(logged, fmt.Sprintf(f, a...)) }}
var p Product
p.ID, p.BlueprintID, p.Quantity = i, bp.ID, 1
p.Price = CTPrice{Cents: 100, Currency: "USD"} // any other currency is dropped without exchangeRates
p.Properties.FabLanguage = "en"                // the game's own field; anything else is skipped
p.Properties.FabFoilNew = treatment            // or PokemonReverse / FirstEdition …
ch := make(chan resultChan, 1)
ct.processProducts(ch, bp.ID, []Product{p}); close(ch)
```

A listing with no result on `ch` was refused if `logCallback` printed
something, and skipped otherwise. `cardtrader/cardmarketid_test.go` is a
minimal working example.

Weight rows by how many log lines each blueprint accounted for - one blueprint
can be 28 lines, and fixing it is worth more than 28 singletons.

## C. Datastore probes

Python over the game JSON (`{"meta":…,"data":{game, sets, cards, sealed}}`;
`sets` is a dict code→{name}):

```python
import json, sys
d = json.load(open(sys.argv[1]))["data"]; cards = d["cards"]; sets = d["sets"]
def show(title, pred, limit=30):
    rows = [c for c in cards if pred(c)]
    print("==", title, len(rows))
    for c in rows[:limit]:
        print("  %-9s %-14s %-46s %-22s %-9s %s %s" % (
            c.get("setCode"), c.get("number") or "", c["name"][:46],
            c.get("finish"), (c.get("rarity") or "")[:9],
            c.get("promoTypes") or "", c["id"]))
show("by name", lambda c: "Ogerpon" in c["name"])
show("by number", lambda c: (c.get("number") or "").startswith("SVP"))
```

A reusable Go probe (built once from a temp `cmd/zzX` dir in a worktree)
supports `set:CODE`, `name:SUBSTR`, `setname:NAME`, `sets:` - worth building
when you will probe a game more than a handful of times.

Magic is ~1 GB: stream it or grep it, do not `json.load` it.

## D. Regression diff (per listing, never recall)

Any shared matcher change needs a corpus that already works, walked before and
after, diffed per listing.

```python
# walkdiff.py before.jsonl after.jsonl
import json, sys, collections
def load(p):
    return {r["id"]: r for r in (json.loads(l) for l in open(p))}
a, b = load(sys.argv[1]), load(sys.argv[2])
gained = lost = moved = 0; ex = collections.defaultdict(list)
for k in a.keys() | b.keys():
    ra, rb = a.get(k, {}), b.get(k, {})
    la = ra.get("landed") if not ra.get("dropped") else ""
    lb = rb.get("landed") if not rb.get("dropped") else ""
    if la == lb: continue
    kind = "gained" if not la else "lost" if not lb else "moved"
    ex[kind].append((k, ra.get("exp_code"), ra.get("name"), ra.get("number"), la, lb))
    gained += kind == "gained"; lost += kind == "lost"; moved += kind == "moved"
print("gained", gained, "lost", lost, "moved", moved)
for kind in ("lost", "moved", "gained"):
    for e in ex[kind]: print(kind, *e, sep="\t")
```

Grade all three. `moved` is only acceptable when every move is a correction
you can name, and a gain is not a win until read: a listing that used to
refuse and now lands on a sibling (another set or language, a foil-only
printing for a nonfoil listing, the Hyper Rare) is a misprice lost/moved
cannot show. Read every gain beside its wording.

If a change only alters which `InputCard` fields the scraper passes (#739
dropped the wording for the product id), no second checkout is needed. Match
both shapes per listing in one run, and build the id-only card first, because
`Match` mutates its input:

```go
byID := &matcher.InputCard{ID: in.ID, Finish: in.Finish, Foil: in.Foil}
full, err := b.Match(in)
idOnly, idErr := b.Match(byID) // bucket same/moved/lost/gained, split by b.ConvertID(space, in.ID) != ""
```

A `lost` row is not automatically a regression. #739's six were the wording
path's guesses that the id did not support (Usopp 719663 priced at $1,999 on
another product's printing).

## E. Pinning a rule with an inline fixture

Copy the rows **verbatim** out of the real datastore so the test states what the
data actually looks like, then load them as a backend:

```go
const labelFixture = `{"data": {
  "game": "fleshandblood",
  "sets": {"ROS": {"name": "Rosetta", "releaseDate": "2024-09-20"}},
  "cards": [
    {"externalLinks": {"fabId": "ROS001", "tcgPlayerId": 561243}, "finish": "Normal",
     "id": "ros001_561243", "name": "Florian, Rotwood Harbinger", "number": "ROS001",
     "rarity": "Majestic", "setCode": "ROS"}
  ]
}}`

b, err := Load(strings.NewReader(labelFixture))
if err != nil { t.Fatal(err) }
// hand b to what is under test: b.Match(...), or the scraper built on it
```

Generate the fixture rather than typing it:

```python
ids = ["ros001_561243", "ros001-mv_561247_coldfoil"]
d = json.load(open(path))["data"]
rows = [c for c in d["cards"] if c["id"] in ids]
sets = {c["setCode"]: d["sets"][c["setCode"]] for c in rows}
print(json.dumps({"data": {"game": d["game"], "sets": sets, "cards": rows}}, indent=2))
```

Include the negative case: the listing that must still refuse.

## F. Golden test data

`matcher/<game>/testdata/<game>_test_data.json` holds baked
`{description, input, uuid, error}` cases; regenerate with
`go test ./matcher/<game>/ -update-<game>`. It refuses to
flip a committed case between success and error, and a seed case whose
error-or-success disagrees with its `negative:` prefix - when a flip is
genuinely intended, edit that entry or seed by hand first, and diff the
regenerated file case-by-case before committing:

```python
old = {x["description"]: x for x in json.load(open("HEAD-copy.json"))}
new = {x["description"]: x for x in json.load(open("testdata/….json"))}
for d in set(old) & set(new):
    if old[d].get("uuid") != new[d].get("uuid"): print("CHANGED", d)
```

Magic's goldens must stay byte-identical - if a change forces a Magic regen,
the change is wrong.

## G. Mapping %

Reads the `side key verdict class` TSV from step 2 and prints, per file and
side, N and mapping % (`landed / (landed + twin + refused + silent)`, class
`card` only). Rows sharing a key collapse to their best verdict; `x-<class>`
counts are tallied but excluded from N, and never printed.

```python
#!/usr/bin/env python3
import csv, sys, collections

RANK = {'landed': 0, 'twin': 1, 'refused': 2, 'silent': 3, 'skipped': 4}

for path in sys.argv[1:]:
    best = {}
    for r in csv.DictReader(open(path), delimiter='\t', quoting=csv.QUOTE_NONE):
        k = (r.get('side', ''), r['key'])
        if k not in best or RANK[r['verdict']] < RANK[best[k]['verdict']]:
            best[k] = r
    by = collections.defaultdict(collections.Counter)
    for (side, _), r in best.items():
        v = r['verdict'] if r['verdict'] != 'skipped' else 'silent'
        by[side][v if r['class'] == 'card' else 'x-' + r['class']] += 1
    if len(by) > 1:
        by['all'] = sum(by.values(), collections.Counter())
    for side, c in sorted(by.items()):
        n = c['landed'] + c['twin'] + c['refused'] + c['silent']
        pct = 100.0 * c['landed'] / n if n else 0.0
        print('%s\t%s\tN=%d\t%.3f%%' % (path, side, n, pct))
```
