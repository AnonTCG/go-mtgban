"""SCRATCH (not for master): compare old vs new bantool NDJSON dumps.

usage: python3 diff_outputs.py <out_dir> <AllPrintings.json> <report.json>
"""
import json
import os
import re
import statistics
import sys
from collections import Counter, defaultdict

OUT, DATASTORE, REPORT = sys.argv[1], sys.argv[2], sys.argv[3]
UUID_RE = re.compile(r"^([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(.*)$")
SUFFIX_UUID = re.compile(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")
CONDS = {"NM", "SP", "MP", "HP", "PO", "UNOPENED"}

umap = {}
with open(DATASTORE) as fh:
    for code, s in json.load(fh)["data"].items():
        for kind in ("cards", "tokens"):
            for c in s.get(kind, []):
                umap[c["uuid"]] = [code, c.get("name", "?"), c.get("number", "?")]
        for p in s.get("sealedProduct", []):
            umap[p["uuid"]] = [code, p.get("name", "?"), "sealed"]
print("datastore uuids:", len(umap))


def load(path):
    rows = defaultdict(list)
    stats = Counter()
    fields = Counter()
    suffixes = Counter()
    if not os.path.exists(path):
        return None, stats, fields, suffixes
    with open(path) as fh:
        for line in fh:
            r = json.loads(line)
            stats["rows"] += 1
            for k in r:
                fields[k] += 1
            for k in (r.get("custom_fields") or {}):
                fields["custom_fields." + k] += 1
            m = UUID_RE.match(r["UUID"])
            if not m:
                stats["bad_uuid"] += 1
            else:
                suffixes[SUFFIX_UUID.sub("<uuid>", m.group(2)) or "(none)"] += 1
                if m.group(2) not in ("", "_f", "_e"):
                    stats["bad_suffix"] += 1
                if m.group(1) not in umap:
                    stats["uuid_not_in_mtgjson"] += 1
            if r.get("conditions") not in CONDS:
                stats["bad_condition"] += 1
            if not r.get("price", r.get("buy_price")):
                stats["zero_price"] += 1
            rows[(r["UUID"], r.get("conditions"))].append(r)
    stats["keys"] = len(rows)
    stats["dup_keys"] = sum(1 for v in rows.values() if len(v) > 1)
    return rows, stats, fields, suffixes


def price(rs):
    # duplicate keys: the lowest ask / the highest bid
    if "buy_price" in rs[0]:
        return max(r.get("buy_price") or 0.0 for r in rs)
    return min(r.get("price") or 0.0 for r in rs)


def describe(key):
    info = umap.get(key[0][:36], ["?", "?", "?"])
    return f"{info[0]} #{info[2]} {info[1]} [{key[0][36:] or 'nf'} {key[1]}]"


def pct(xs, q):
    xs = sorted(xs)
    return xs[min(len(xs) - 1, int(q * len(xs)))] if xs else None


files = set()
for side in ("old", "new"):
    for root, _, names in os.walk(os.path.join(OUT, side)):
        for n in names:
            if n.endswith(".ndjson"):
                files.add(os.path.relpath(os.path.join(root, n), os.path.join(OUT, side)))

report = {}
for rel in sorted(files):
    old, so, fo, xo = load(os.path.join(OUT, "old", rel))
    new, sn, fn, xn = load(os.path.join(OUT, "new", rel))
    entry = {"old": dict(so), "new": dict(sn),
             "suffixes_old": dict(xo), "suffixes_new": dict(xn),
             "fields_only_old": sorted(set(fo) - set(fn)),
             "fields_only_new": sorted(set(fn) - set(fo))}
    if old is None or new is None:
        entry["status"] = "only in " + ("new" if old is None else "old")
        present = new if old is None else old
        entry["conditions"] = dict(Counter(k[1] for k in present))
        entry["samples"] = [(describe(k), price(v)) for k, v in list(present.items())[:8]]
        report[rel] = entry
        continue
    ko, kn = set(old), set(new)
    both = ko & kn
    uo, un = {k[0] for k in ko}, {k[0] for k in kn}
    entry.update({
        "keys_both": len(both), "keys_only_old": len(ko - kn), "keys_only_new": len(kn - ko),
        "uuids_old": len(uo), "uuids_new": len(un),
        "uuids_only_old": len(uo - un), "uuids_only_new": len(un - uo),
        "conditions_old": dict(Counter(k[1] for k in ko)),
        "conditions_new": dict(Counter(k[1] for k in kn)),
    })
    ratios, same, moved = [], 0, []
    sku_same = sku_diff = ship_same = ship_diff = 0
    sku_diff_samples = []
    for k in both:
        po, pn = price(old[k]), price(new[k])
        if abs(po - pn) < 0.005:
            same += 1
        else:
            moved.append((abs(pn - po), k, po, pn))
        if po > 0 and pn > 0:
            ratios.append(pn / po)
        io = {r.get("instance_id") for r in old[k]}
        inn = {r.get("instance_id") for r in new[k]}
        if io == inn:
            sku_same += 1
        else:
            sku_diff += 1
            if len(sku_diff_samples) < 8:
                sku_diff_samples.append((describe(k), sorted(map(str, io)), sorted(map(str, inn))))
        if len(old[k]) == 1 and len(new[k]) == 1:
            co = (old[k][0].get("custom_fields") or {}).get("lowestShipping")
            cn = (new[k][0].get("custom_fields") or {}).get("lowestShipping")
            if co is not None or cn is not None:
                if co == cn:
                    ship_same += 1
                else:
                    ship_diff += 1
    entry["price_same"] = same
    entry["price_changed"] = len(moved)
    entry["instance_id_same"] = sku_same
    entry["instance_id_diff"] = sku_diff
    entry["instance_id_diff_samples"] = sku_diff_samples
    entry["lowestShipping_same"] = ship_same
    entry["lowestShipping_diff"] = ship_diff
    if ratios:
        entry["ratio_new_over_old"] = {
            "p05": round(pct(ratios, 0.05), 4), "median": round(statistics.median(ratios), 4),
            "p95": round(pct(ratios, 0.95), 4), "mean": round(statistics.fmean(ratios), 4)}
    entry["zero_old_nonzero_new"] = sum(1 for k in both if not price(old[k]) and price(new[k]))
    entry["nonzero_old_zero_new"] = sum(1 for k in both if price(old[k]) and not price(new[k]))
    moved.sort(reverse=True)
    entry["biggest_moves"] = [(describe(k), po, pn) for _, k, po, pn in moved[:15]]
    for label, keys, side in (("only_old", ko - kn, old), ("only_new", kn - ko, new)):
        sets = Counter(umap.get(k[0][:36], ["?"])[0] for k in keys)
        entry[label + "_top_sets"] = sets.most_common(15)
        entry[label + "_suffixes"] = dict(Counter(SUFFIX_UUID.sub("<uuid>", k[0][36:]) or "(none)" for k in keys))
        entry[label + "_zero_price"] = sum(1 for k in keys if not price(side[k]))
        entry[label + "_value_sum"] = round(sum(price(side[k]) for k in keys), 2)
        entry[label + "_samples"] = [(describe(k), price(side[k])) for k in sorted(keys, key=lambda k: -price(side[k]))[:12]]
    report[rel] = entry

json.dump(report, open(REPORT, "w"), indent=1)
