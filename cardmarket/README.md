# cardmarket

Prices Cardmarket across the eight games it and `mtgban` both carry: Magic,
Lorcana, Riftbound, One Piece, Yu-Gi-Oh, Flesh and Blood, Pokemon and
Gundam, via three scrapers built on `github.com/mtgban/go-cardmarket`, the
signed API client. Palworld is absent on purpose: Cardmarket itself does not
sell it (`mkmGames` names the eight it does, and `go-cardmarket`'s own
`Game*` constants list twenty-two games total, fourteen of which `mtgban`
has never modeled at all; see "Known gaps").

- **`Index`** (`cardmarket.go`, `idmap.go`) reads Cardmarket's published price
  guide: one bulk download per game, no credential, the low and trend columns
  as a lagging aggregate. One product prices a printing in both columns:
  the one pricing it in the most columns, then the one the datastore files
  it under, else one found by id, else one named, first in the catalog's
  order (`namedLast`).
- **`Sealed`** (`sealed.go`) reads live listings for sealed product through
  the authenticated `Articles` endpoint, one request per product.
- **`Market`** (`market.go`, `market_filter.go`) reads the same live listings
  for singles, per printing rather than per product - the newest of the
  three, and the one most of this document is about.

`Sealed` and `Market` need `MKM_APP_TOKEN`/`MKM_APP_SECRET`. `Index` is
public and makes no authenticated call, even when the credentials are set:
that is a golden, pinned by `TestIndexMakesNoAuthenticatedCall`, and a gap
in the published id map is never filled from the API. `Market`
additionally needs `BAN_API_KEY` (see "The offline pre-filter").

## The shared resolver

`resolver.go` holds what `Index` and `Market` both need and neither owns
alone: turning a Cardmarket product - its name, number, expansion - into an
`matcher` uuid. Id-map lookup first, then name/number matching per game,
twins, foreign shelves, print runs. It used to live entirely on `Index`;
`Market` needed the identical logic and inherited it by embedding `resolver`
rather than reimplementing a thinner version of it, once for the eleven
original methods and twice more (`disownBridged`, `reportRefused`) for two
found only once `Market` actually needed them and `Index`'s own tests turned
out not to exercise directly enough to have caught the gap on the first
pass. Pure extraction - no logic changed, only where it's defined - verified
by running `Index`'s full pre-existing test suite unchanged before and after.

**A shared mcmId can name the wrong printing, and `Preprocess`'s own default
could throw away the one thing that would have caught it.** Found from a
live run: id 272488, "Urza's Mine (V.2)" under Chronicles, kept a Chronicles
Foreign Black Border (Japanese) printing instead of one of its own four
plain English arts. Two bugs, both real and both in code `Index` shares.
`Fallback`'s `checkLoadedID` trusts whatever mcmId-matching id it finds last
when none of them agrees with the product's own number - fine for genuine
cosmetic siblings (its own doc comment's "30A frame pairs"), wrong here,
because mtgjson stamps all four BCHR arts with 272488, the id of one of the
four *different* CHR arts instead; the four never agree with each other on
number, let alone the product's. Fixed by deferring to Preprocess/Match
instead of guessing when that shape shows up. Second, separately: Chronicles
has no dedicated case in `Preprocess`'s edition switch, so it fell to the
generic default ("old editions keep the V.1/V.2 style for variants.go to
resolve") - except that default overwrote `Variation` with Cardmarket's own
Number unconditionally, discarding the "(V.2)" `magic.VariantsTable`'s
`chrVariants` already carries a `"v.2"` key for, whenever Cardmarket's
Number field was non-empty - which for these same-numbered siblings, it
always is. Fixed by only taking the number when `VariantsTable` (resolved
through `magic.EditionTable`'s alias first, the same one `AdjustEdition`
consults later - Cardmarket's own edition spelling rarely matches the
matcher's) has no entry for this exact edition/card/variant to consult
instead. The same gap, found the same way, turned up one edition over:
"Fourth Edition: Alternate"'s three-art basic lands were failing to resolve
with "unknown variant" in the same run, for the same reason - fixed by the
same guard, plus a real, separate bug the fix surfaced: `ed4Variants`
itself named the bare 4ED number for a card mtgban's own loader only ever
synthesizes with "alt" appended (`Alternate Fourth Edition` is not a real
mtgjson set - `duplicate()` builds it from 4ED at load time), so none of
its values ever matched the card FilterCards was checking them against.
Pinned with a test that would have caught it directly:
`TestEd4VariantsNumbersAreReal` checks every `ed4Variants` value against the
loaded set's own numbers, not just that the card name exists there the way
the existing `TestVariants` did. The loader has since stopped appending
"alt" to a copy filed under a set of its own, so the Alternate Fourth
Edition now shares `ed4Variants` with the other two editions.

**A single, confident, wrong candidate isn't ambiguity - it's just wrong,**
and the number-disagreement guard above only ever fires on more than one
candidate. Found live on `/arbit`: World Championship Decks products (sold
one per player/year, Cardmarket edition `"WCD <year>: <player>"`) had
mtgjson's mcmId linking several of them onto a printing with nothing to do
with the deck at all - id 249617, "Phyrexian Processor (V.2)" under
`WCD 2000: Janosch Kühn`, onto The Brothers' War Retro Artifacts' foil
printing; id 249533, "Duress (V.2)" under `WCD 2001: Antoine Ruel`, onto a
starred Seventh Edition Duress - both real, both `/arbit`-visible fake
spreads (a cheap old WCD listing priced as an unrelated, pricier card),
neither ambiguous in Fallback's own terms, since there was only ever the
one id. `Fallback` now also checks a WCD product's lone candidate against
what a WCD printing actually looks like (set codes `WC97`-`WC04`, every one
"memorabilia"): not one, and it defers the same way an ambiguous one does.
A sweep of every WCD product in MTGJSON's own `CardmarketIdentifiers.json`
(931 products, 32 expansions) found 23 more candidates with this exact
shape - spot-checked live, all confirmed genuine - so this is systematic on
mtgjson's side, not a one-off; worth filing upstream. Deferring to the name
route was not enough either: by name every "(V.N)" version of a card lands
on one printing (both examples above on the main deck's, not the sideboard
one their "(V.2)" names). `versionPrintings` names by product id the
printings of the versions the id map does not place;
`docs/agents/cardmarket-census/versions.md` has where each row comes from.

The same drift reaches a product through the id map too: `resolveMapped`
answers from the map's own uuids before `Fallback`'s mcmId route ever runs,
so a map entry carrying one of these same wrong links used to price
straight through it untouched. The check above is now `plausiblePrinting`,
called from both `Fallback` and `resolveUUIDs`. It covers the Pro Tour
Collector Set's `"Pro Tour 1996: <player>"` shelves, which only sell `PTC`,
and Oversized shelves, which only sell an oversized printing.

**The Yu-Gi-Oh bridge is held to the product's number.** CardTrader links
a set's Special Editions ("S01"), box toppers (LC05, LC06) and European
prints ("EN001") to the TCGplayer id of the set's base row, and the product
numbered for that row gave way to them as a twin. `yugiohOtherNumber` sends
such a product to the name path when the digits disagree, or when a
European number lands on a row without the EN infix; the other letters are
notation ("SP02" is the datastore's "ENS02"). `offShelf` was measured
instead and rejected: it would refuse 178 bridged products, 109 of them
Legendary Collection Kaiba Mega Pack cards the datastore files under LC06.
Replaying the 2026-09-24 Index, 121 uuids gain a price, 124 are repriced
by the product numbered for them, and the (uuid, column) slots two
products claim fall from 22 to 6. Two lose their price: the MP24 Kaiba
token, whose TCGplayer number reads EN02 for EN052, and MRL-103, which was
priced by the MRL-E130 misprint.

**The datastore's own Cardmarket id answers before the bridge.** One Piece,
Lorcana and Pokemon publish `cardmarketId`, and the loaders file it under
`matcher.IDSpaceCardmarket`. The One Piece ones are the hand-carried
pre-errata and reprint printings, which no TCGplayer product sells, so no
bridge can reach them. Before the id route, 6 of those 26 were priced by
their own product, 3 by a sibling's and 17 not at all. Now all 26 are
priced by their own. A One Piece name that reaches a printing another product's id
owns is refused (`ownedElsewhere`), and so is a product on the pre-errata shelf
that reaches a printing the datastore does not tag pre-errata (`notPreErrata`).
Pokemon's arm reads the published id first, then `pokemonStampLinks` (the
stamped and league copies Cardmarket sells under the plain card's name and
number, linked to their TCGplayer products), then CardTrader's link and the
wording. CardTrader reads the same space through each blueprint's
`card_market_ids`.

**Which products count as refusals.** A One Piece product on a `Starter Deck:`
shelf that the bridge does not link reaches the booster printing its code
names, which another shelf prices already, so it is a twin
(`onePieceDeckReprint`); a bridge-linked one names a printing of its own and
stays a refusal, unless the link names a TCGplayer product the datastore does
not carry, which counts as no link. An event DON!! no catalog carries, and a Reprints/Demo Decks
card the reprint set does not number the same way, are matcher errors the walk
names (`product id N ... returned ...`), not refusals. A serialized trophy
card that no id links is skipped (`onePieceTrophy`). A foil that
Cardmarket sells as its own product (`foilSold`) is not handed to the plain
version as a second printing. Yu-Gi-Oh's `yugiohNoRow` holds the products of a
printing the datastore lacks. For Magic, where MTGJSON lists a printing and
its decorated variants (a dagger, a star, a letter) under one Cardmarket id
and no number settles the pick, the undecorated printing is kept
(`undecorated`).

## Market's design

**A printing sold foil-only is queried as a foil.** Such a printing answers
both of a product's id slots with one uuid, and `queryPrintings` returns
after the first query for exactly that reason - so the plain-listing query
is the only one it ever makes, and `acceptArticle` then rejects every
listing the product has. `marketLoneFlag` picks the flag off the printing
itself instead. Lorcana's Enchanted cards are the bulk of these: 366 of its
611 pre-filter candidates are foil-only, and only 35 of them carried a
Market price before this, EUR 67k of trend value unpriced. Etched counts as
foil - the datastore keeps it as its own finish with `Foil` false, while
Cardmarket has no etched at all and files those listings under `isFoil`.

**A Yu-Gi-Oh printing made only in 1st Edition is queried as one.** It is
the same shape on `isFirstEd`: the resolver's 1st Edition id is the card's
own id, so the first query was the only one, asked for unflagged listings,
and filed them under the `_1stedition` uuid - 230 uuids in the 2026-09-24
dump. Asking for any flag instead was measured and rejected, against the
live English listings of 14 of those products:

| Set | Card | Flagged | Unflagged | Cheapest NM flagged / unflagged |
|---|---|---|---|---|
| MFC | Dark Paladin | 14 | 3 | 375 / 280 |
| ABPF | Battle Fader | 27 | 22 | 70 / 30 |
| TAEV | Rainbow Dragon | 21 | 33 | 499.99 / 199.99 |
| DP11, LC5D, MP15, HSRD, DUSA, BLLR, SHVA, AC18, HISU, DESO | ten cards | 490 | 74 | at or below in 9 of 10 |
| Misc. promo | Dance Princess of the Ice Barrier | 0 | 4 | - / 19.99 |

On the modern 1st-Edition-only sets 87% of listings carry the flag, and
the strict query's cheapest is rarely beaten. On the older sets the
unflagged copies are the Unlimited run, 25-60% below the 1st Edition price.
The one promo nobody flags goes unpriced.

**A Yu-Gi-Oh version index mostly counts rarities, not runs.** Of the
catalog's 11,849 indexed cards (2026-09-23), grouped by shelf and name:

| Shape | Cards |
|---|---|
| every version a different rarity | 8,852 |
| every version the same rarity | 948 |
| some rarities repeat | 316 |
| versions at different numbers | 1,601 |
| one version only | 132 |

A product whose rarity no sibling at its number shares sells both runs -
Battle Fader's V.2 Ultimate Rare lists 27 flagged and 22 unflagged English
copies. Reading its index as a run left 98 such products on a lone 1st
Edition, 29 of them among the 30 lone `_1stedition` uuids with an
`_unlimited` sibling in the 2026-09-24 18:06 dump.

The same-rarity shape does not name a run either. Three such cards, one page
of English listings each: Duelist Pack: Kaiba Blue-Eyes V.1 21/34 flagged,
V.2 0/11 ("American Version"); Metal Raiders Black Skull Dragon V.1 13/48,
V.2 10/41, V.3 0/7 (the MRD-EN018 reprint); Cannon Soldier V.1 12/55, V.2
3/40, V.3 0/5. There the index separates regional prints. So `matchYugioh`
asks for no run, and every product lands on the card's default run, as the
bridge does.

**On the seven oldest shelves the index names a regional print.** Cardmarket
split LOB, MRD, MRL, PSV, LON, SDY and SDK into one product per print in
2021: European first, North American second, then the Asian English and
worldwide reprints (`yugiohIndexPrints`). The European numbering shows it
where it differs - Magic Ruler's V.1 runs to 130, LOB's, SDY's and SDK's
renumber (SDK's V.1 matches the datastore's number on 1 of 46 cards, V.2 on
49 of 50) - and the comments show the rest. One page of English listings
each, 2026-09-24; "US" counts comments naming an American print, "EU" a
European or E-numbered one:

| Card | V.1 US / EU / listings | V.2 US / EU / listings |
|---|---|---|
| LON Torrential Tribute | 3 / 1 / 23 | 14 / 0 / 42 |
| LON Gemini Elf | 8 / 2 / 37 | 13 / 0 / 59 |
| MRL Mystical Space Typhoon | 2 / 3 / 30 | 11 / 0 / 35 |
| MRL Relinquished | 1 / 0 / 16 | 12 / 0 / 48 |
| MRD Cannon Soldier | 2 / 6 / 55 | 8 / 1 / 40 |
| PSV Jinzo | 2 / 1 / 26 | 18 / 2 / 68 |

CardTrader links only the product carried before the split, which on LON
and MRL is V.1, so there the European print held the North American row and
the North American product was its twin. Writing the index's prefix onto
the bare number sends the European print to its own row where one is
published (datastore-gen mints one for the European first prints of all
seven shelves) or refuses it, and a reprint numbered bare to its EN set.
Replaying the 2026-09-24 catalog: 208 North American products take their
pair back, 64 bare reprints price 63 EN rows nothing priced before, and no
printing loses its price. Duelist Pack: Kaiba and Yugi split the same way
in 2024 but number both prints alike, one row of ours, so their V.2 stays a
twin.

**Strictly sequential, not pooled.** Measured directly: a concurrency ladder
(1→24 workers) plateaus at ~1.4 successful calls/sec regardless of worker
count, with the excess coming back as 429s, while 20 fully sequential
requests at zero delay cleared 20/20 with no 429s at all, at essentially the
same throughput. This is a per-token *concurrency* limiter, not a rate
limiter - parallelism buys nothing here and only manufactures wasted 429s -
so `Load` walks the catalog one product at a time. `Sealed` still pools with
`WorkerPool`; do not copy that shape into `Market`.

**Every edition ends its walk with one summary line, always.** A CI run
showed why: `"Processing Guru Lands (88) [86/760]"` followed straight by the
next edition's own `"Processing ..."` line reads as a hang or a silent
no-op, not as "nothing here needed a mention" - the skip/refusal lines
`walkExpansion` already prints only appear when there is something to
explain, so an edition with nothing skipped or refused ended in silence.
`"<edition>: priced %d/%d products"` prints unconditionally now, counting
every product that reached a live query and came back without error -
unlike the other lines, this one is never gated on being non-zero.

**`Content-Range`-driven early stop**, shared with `Sealed` as
`contentRangeCovered` in `utils.go`: once the pages already fetched have
covered everything a listing's `Content-Range` total promises, another page
can only come back empty. A `capped` response reports Cardmarket's 1000-result
ceiling rather than the real total; the API rejects a request starting at 1000,
so the ceiling is still the stopping point and listings beyond it cannot be
recovered by pagination.

**Server-side filters are never trusted alone.** `isFoil`, `isReverseHolo`,
`isFirstEd` are documented to fail open on a game or value they don't apply
to - silently answering the unfiltered list, not erroring - so every listing
is also checked client-side against its own article flags (`acceptArticle`)
before being priced onto a printing. `marketFinishParam` names the games this
is actually verified for: Magic/`isFoil`, Pokemon/`isReverseHolo`,
YuGiOh/`isFirstEd`, and - confirmed directly, not assumed from `isFoil`'s own
documentation, which names only Magic and Pokemon (deprecated) - Lorcana and
Riftbound both read `article.IsFoil` too. That documentation is about the
request-side filter, a separate question from whether the response's own
flag is populated for other games. A spread sample of thirty live products
(fifteen per game) confirmed it is: products selling nothing but foil copies
at a clear premium over their game's bulk floor (a Riftbound rare at
$15–$1000, a Lorcana one at $35–$1174, against $0.02–$0.03 commons), and
products mixing a handful of foil listings into an otherwise-plain one with
those listings priced distinctly above the plain floor - exactly the shape
`cardID`/`cardIDFoil` being the nonfoil/foil match of the same product
predicts, not noise. The request-side `isFoil` param's own narrowing effect
for these two games was not separately checked, and does not need to be -
correctness rests on the article-level check either way.

**Listings are not strictly price-ascending.** Replayed a real 100-listing
page (`testdata/articles_riftbound.json`, a heavily bulk-priced Riftbound
common) through `acceptArticle` rather than trusting a hand-built fixture,
and it proved a documented assumption wrong: the raw JSON prices ran
`0.02, 0.02, 0.03, 0.03, 0.02, 0.03, 0.02, ...` - the same handful of
cent-level prices repeating out of order, not climbing, at full wire
precision (not a rounding artifact on a longer float). "Hold the first
acceptable listing per condition" would occasionally hold one that wasn't
actually the cheapest. Fixed by holding the *cheapest* seen per condition
instead of the first, deferring emission to the end of the scan rather than
streaming immediately - both changes cost nothing extra, since the whole
page is scanned either way. `TestReplayListingsAreNotStrictlyPriceAscending`
pins the finding directly so a future tightening on Cardmarket's side would
surface as a test starting to skip, not as a silently stale assumption.

`CARDMARKET_MARKET_PATH` overrides the checked-in fixture with a fresher
export from `Articles()` itself, in the same `[]cm.Article` JSON shape - the
same `<GAME>_PATH` convention every datastore variable already uses.

**A 20-consecutive-failure circuit breaker.** `bounce`/`maxBounced` track
requests since the last usable answer (a price, or a clean empty result);
crossing the threshold aborts the whole `Load` run rather than grinding
through the rest of the catalog against a token Cardmarket is rejecting
outright.

**A second, Powerseller-only view.** Market implements `mtgban.Market`
(`MarketNames`/`InfoForScraper`), the same interface `cardtrader.Market`
uses to split into "Card Trader"/"Card Trader Zero"/"Card Trader 1DR" -
every entry gets one of two fixed `SellerName`s, `"Cardmarket"` or
`"Cardmarket Powersellers"`, with the article's own username moved to
`CustomFields["SubSellerName"]` instead. The Powerseller bucket holds its
own independent cheapest-per-condition view, live-restricted to German and
Dutch sellers whose `isCommercial` reads `2` - confirmed directly against
two real accounts on Cardmarket's own seller pages, `1` shows a
"Professional" badge and `2` shows "Powerseller"; nothing else in the
documented `0`/`1`/`2` scale reads as a power seller. Once the *main*
bucket's NM/SP/MP are all held, `queryOnePrinting`'s page loop spends up to
`marketPowersellerExtraPages` (4) more pages specifically chasing at least
one Powerseller listing before giving up on this product - a bounded,
paid-once fail-safe (`shouldStopPaging`), not a guarantee: gating the stop
on full Powerseller completeness (all three conditions, not just one)
would mean every product with no qualifying seller at all - most of them -
pages all the way to `Content-Range` coverage instead of stopping early,
which is exactly the budget this scraper is built around not spending. A
product with no qualifying seller anywhere in its listings still
contributes nothing to this bucket, the same as a product neither Zero nor
1DR carries for Card Trader's own split. `bantool` needs no new
registration for this:
`UnfoldScrapers` (already called for every scraper) dumps one output file
per `MarketNames` entry automatically, keyed by `InfoForScraper`'s own
Shorthand (`MKM`, `MKMPS`).

The Powerseller bucket's links narrow the same way,
`sellerCountry=7%2C23&sellerType=2`, so they open on the sellers the price was
drawn from rather than on a cheaper one the bucket turned away. The bucket
and its links both read `mkmPowersellerCountries`, so they cannot name
different countries.

## The offline pre-filter

Every game but Gundam is restricted to a candidate set (`marketCandidates`)
computed from a snapshot
of `mtgban`'s own published prices, before `Market` ever queries Cardmarket
live: a card priced over $7, or an Arbit-style spread against a buylist
vendor (CK → SCG → CSI, first available) over 20%, or a Mismatch-style
spread against another retail seller over 80% - each spread leg guarded by a
price floor and minimum absolute difference (`marketFilterParams`) so a raw
percentage spread isn't dominated by cent-level noise on bulk cards. Magic,
Pokemon and YuGiOh need this to fit a nightly scrape budget at all; Lorcana,
Riftbound, Flesh and Blood and One Piece fit their budget unfiltered too,
but only clear the filter on 9–26% of their own priced uuids (measured
directly against each game's own snapshot), so filtering them still saves
real call volume against the one daily allowance this app token shares
across every game.

### The wrong-host saga

The price snapshot is published **one per game subdomain**
(`pokemon.mtgban.com`, `yugioh.mtgban.com`, ...), not shared. `loadBanSnapshot`
originally hardcoded `www.mtgban.com` - copied from `sealedev`'s own loader,
which is correct there only because `sealedev` is wired for Magic alone.
Querying the wrong host for another game answers with a real `200` and an
empty result: no error, just none of that game's own uuids in it. That is
exactly how Pokemon and YuGiOh's candidate sets went silently empty during
development - `marketCandidates` built a real, non-nil, *empty* map, and
`walkCatalog` read that as "skip everything," so `Market` would have priced
**zero** cards for either game had it actually run. Fixed by building the
snapshot URL per game (`banAPIURL`), and it also corrected the candidate counts
measured earlier against the wrong host: Pokemon dropped from an estimated
14,012 candidates to a measured 11,247; YuGiOh from 8,170 to 6,085.

Two more ways the same fetch can go wrong without ever returning a non-200
status, both confirmed live and both guarded in `parseBanSnapshot`:

- **A rejected signature** answers `200` with `{"error": "..."}` instead of
  the `retail`/`buylist` fields - confirmed against One Piece's own host
  during development, which briefly rejected this app token's signature
  outright: a misconfiguration on a separate backend deployment (its
  `x-do-app-origin` header named a different DigitalOcean app instance than
  every other game subdomain), fixed server-side and re-measured clean
  afterward. Decoding that body into `banSnapshot` without checking `Error`
  first would silently produce the same empty-but-real-looking result as the
  wrong-host bug did.
- **A snapshot with no prices at all** - belt-and-braces past the two cases
  above, since nothing rules out some other reason a game's snapshot could
  come back genuinely empty.

Both read as "the fetch itself failed," not "nothing passed the filter" -
the same silent-failure shape the empty-body-on-error bug in
`go-cardmarket`'s own `get()` was, and the same principle: a non-200 or an
explicit error field is a real failure regardless of what status line
carries it.

## Known gaps

- **Fourteen of `go-cardmarket`'s twenty-two `Game*` constants have no
  `mtgban` equivalent**: World of Warcraft, The Spoils, Force of Will,
  Cardfight!! Vanguard, Final Fantasy, Weiss Schwarz, Dragoborne, My Little
  Pony, Dragon Ball Super, Star Wars Destiny, Digimon, Battle Spirits Saga,
  Star Wars Unlimited and Cyberpunk. `mkmGames` covers exactly the games `matcher/games` blank-
  imports; adding any of these needs the full "Adding a game" checklist in
  `AGENTS.md` first; this package is the last, not the first, step.
- **Index cannot price Yu-Gi-Oh's 1st Edition apart.** Cardmarket keeps
  the run as a flag on each listing, not as a product, and the guide has no
  1st Edition column. In the 2026-09-24 Market dump, 373 of 470 V.2+
  products and 171 of 289 V.1 products hold both flagged and unflagged
  listings, and Gate Guardian's V.1 and V.2 (MRD-000) each sell 1st Edition
  near 150 and Unlimited near 30. So a product lands on the card's default
  run whether the bridge or the name names it, and V.1/V.2 products of one
  printing are twins. Market prices the runs apart by the listing flag.

## Environment

- `MKM_APP_TOKEN`, `MKM_APP_SECRET` - Cardmarket API credentials, needed by
  `Sealed` and `Market`, not by `Index`.
- `BAN_API_KEY` - authenticates the offline pre-filter's snapshot fetch; the
  same key `sealedev`'s own loader uses. Wired unconditionally for every
  `Market` scraper in `cardmarket/register.go`. An unset key never surfaces
  at construction, only at `Load` time, and only for Magic, Pokemon and
  YuGiOh (`marketFilterRequired`) - the three whose catalogs don't fit a
  nightly budget unfiltered, so `Load` refuses to run them without it. The
  other four games in `marketFilterParams` fall back to running unfiltered
  instead: a missing key there loses call-volume savings, not the ability
  to run.
- `CARDMARKET_CATALOG_PATH` - the id-map catalog `Index` and `Market` both resolve
  products from, as `go-cardmarket`'s `mkmcatalog` builds it for every game.
- `CARDTRADER_TOKEN_BEARER` - the bridge Flesh and Blood, Pokemon and Yu-Gi-Oh
  require and One Piece merely improves with, for all three scrapers alike
  (`TestCardmarketNeedsItsBridge` pins the refusal).
- `CARDMARKET_MARKET_PATH` - overrides the checked-in live-listing fixture
  `market_replay_test.go` reads by default.
