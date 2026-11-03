# coolstuffinc

Prices Cool Stuff Inc's singles and sealed product, buy and sell, across
eight games: Magic, Lorcana, Riftbound, Yu-Gi-Oh, One Piece, Pokemon,
Gundam, Palworld. Wired into `cmd/bantool` as fourteen scrapers - one
singles scraper per game (`coolstuffinc`, `coolstuffinc_gundam`,
`coolstuffinc_lorcana`, `coolstuffinc_onepiece`, `coolstuffinc_palworld`,
`coolstuffinc_pokemon`, `coolstuffinc_riftbound`, `coolstuffinc_yugioh`)
via `NewScraper(game)`, plus one sealed scraper for six of those eight
(`coolstuffinc_sealed*`, everything but Gundam and Palworld) via
`NewScraperSealed(game)`. No Flesh and Blood shelf exists here - that is
Star City Games' territory, see `starcitygames/README.md` - and no
credential is needed: the only setting is an optional affiliate id,
`CSI_PARTNER`.

## The core problem

**CSI publishes no identifier at all, for any game, on either feed.** The
buylist (`CSIPriceEntry`) carries a `PID` - CSI's own internal product id,
used only to build a debug URL - and nothing else that names a printing
independently of its wording. The retail HTML search carries not even
that: no scryfall id, no tcgplayer id, in the row markup for any game. So
every resolution, for every game, on both sides, is **wording- and
shelf-driven**: the printing is whatever `Name` + `ItemSet`/shelf +
`Variation` (built from `Notes`/`Number`) + `Foil` happens to name, and
when the shelf itself names the wrong thing, nothing catches it.

This is the opposite failure mode from Star City Games, whose problem is a
vendor id that is *present and sometimes wrong*. Here there is usually no
id to be wrong - the vendor's whole data model for a listing is prose, and
the scraper is what turns prose into an identity. That makes this package
much larger than a single defect class fixed several times; it is one
translation layer per game, independently discovered and independently
broken.

The **one** identifier-driven path in the whole package is Magic sealed
product: `NewScraperSealed` builds `productMap` from `co.Identifiers["csiId"]`
on every sealed Magic uuid the datastore carries - a field this scraper
reads but never writes, populated upstream (MTGJSON/datastore-gen). Every
other resolution, sealed or singles, any game, goes through wording.

## Data sources

Three feeds, all live.

- **Retail/singles**: an HTML search, `POST https://www.coolstuffinc.com/sq/`,
  one request per storefront "Item Set" shelf (the checkbox list at
  `csiInventoryURL`), each shelf paged. `processSearch` parses one
  `product-search-row` per card and one `offers` block per condition/price
  row inside it.
- **Buylist**: `GET .../GeneratedFiles/SellList/Section-<shelf>.json` -
  `CSIPriceEntry`, one JSON array for the whole shelf (all editions, all
  conditions, tens of thousands of rows). `LoadBuylistEditions` separately
  scrapes an HTML `<select>` for the edition-name→id map used only to build
  a debug link.
- **Sealed**: Magic sealed rides a fixed saved-query URL (`sealedURL`,
  literally `/sq/2293832`) and resolves purely by the `csiId` product-map
  above; the other five sealed-wired games (`Sealed.scrapeBysets`) instead
  run the *same* search endpoint as singles, once per sealed-product word
  ("booster", "deck", "trove", "gift", "bundle", "case", "box", "kit",
  "vault", "collection"), and resolve each hit through
  `matcher.ResolveSealed` - a card row simply fails to resolve and drops
  out. Lorcana's sealed carries no `ItemSet` facet at all, which is why the
  sealed search goes by name rather than by shelf.

A fourth, dead one used to sit here: `csiPricelistURL`
(`gateway_json.php?k=`), `CSIClient`, `NewCSIClient`, `CSICard`, and
`Preprocess(CSICard)` were a whole second retail API - keyed, unlike
everything else in this package, with a scryfall id on every row - with
zero callers anywhere in the repo. Removed outright rather than kept
around as a maybe-someday alternate path.

Deleting `Preprocess(CSICard)` did not delete its knowledge for free,
though, and that is worth knowing on its own: five of its six edition
cases named shelves CSI is still selling on today (`Black Bordered
(foreign)`, `Ikoria: Lair of Behemoths: Variants`, `Portal 3 Kingdoms`,
`Double Masters: Variants`, `Dominaria United: Variants` - confirmed
against a live fetch of the shelf checkbox list), and the live retail
path (`preprocess()`, used by `retailMagic`) carries no equivalent case
for any of them. Checked live rows from all five against the real
datastore: four resolve fine anyway - the live path's generic wording
already covers what the dead function's special cases once bought it,
`Godzilla`/`Showcase Frame`/foreign-refusal included, at least in the
sense of "resolves without error," which is as far as this was checked.
`Black Bordered (foreign)` alone does not: a listing in any of the six
languages that shelf carries currently returns `ErrUnsupported` from
`preprocess()` - Italian and Japanese included, which the dead function's
own table would have routed to `FBB`/`4BB`, both real sets the datastore
carries. Nothing is mispriced by this; the listing is silently dropped
rather than priced onto the wrong card, which is presumably why nobody
noticed. Fixing it needs an answer this package doesn't have: `Edition:
"Foreign Black Border"` resolves a card correctly with no `Language` set,
and *stops* resolving it the moment `Language: "Italian"` is added - a
question for `matcher`'s own language filtering, not for this scraper.

## fetchWhole - the resume saga

The buylist download is an 18MB+ uncompressed file, and the storefront
truncates it mid-stream roughly one fetch in three: HTTP 200, a short body,
no transport error - nothing notices until the JSON decode reports an
unexpected EOF. This took four commits to get right, in order:

1. Retry the whole file on failure (original behaviour). Not enough: three
   bad attempts in a row is a ~1-in-25 event and it reddened the Pokemon
   CI job on 2026-09-09.
2. **`fetchWhole`** (#547): the response honours `Range` and states
   `Content-Length`, so ask only for the missing bytes each pass, and stop
   when the body is whole or a pass adds nothing.
3. **Stop resuming a body that stops growing** (6dcb7f68): a server that
   will not honour `Range` re-answers 200 with the whole file from byte 0
   every time, which "adds" the same truncated length forever. Track the
   *longest* body any pass has held and stop when a pass does not beat it,
   not when the latest pass alone adds nothing.
4. **Ask about the header before trusting it** (#548): `Content-Length`
   only means anything while the transport doesn't touch the body - ask
   for `identity` encoding explicitly (checked live: CSI serves this path
   uncompressed regardless of what's offered, though it gzips its PHP
   pages). A response with no length at all (`-1`, e.g. chunked) has
   nothing to resume against and must be reported as a short read, not
   silently accepted as the whole file - the old order did the latter,
   which is a truncation with **no error**, worse than the one this
   started from.

The fix works around what can be worked around: a chunked body that ends
cleanly at the wrong byte is indistinguishable from a real end of stream,
by construction.

## offerSeen - the two-shelf-twin dedupe

A card the search lists on two shelves at once (the One Piece Heroines
Edition card, filed both under its own set and a crossover promo shelf)
arrives as the same offer twice: same url, same condition, same seller.
`offerSeen`'s dedupe key used to be exactly that triple, and a product
row's *other* offers - its foil, its graded copies, its first-edition
run - are also filed at the same url and the same `NM` condition as the
plain copy. So the dedupe silently ate them as "the same offer already
seen": Magic's retail row count fell from 101,645 to 74,279 the day the
dedupe landed, Lorcana's by 40%, Riftbound's by 24% (#555). The key now
includes the **resolved card id**, not just url+condition+seller - an
offer is the printing it resolved to, at its condition, from its seller,
behind its url.

## Cross-cutting mechanics

- **The condition column is overloaded.** CSI's per-row condition text is
  sometimes a real condition ("Near Mint"), sometimes a **print run**
  ("1st Edition Holofoil" - `conditionRun`/`matchRun`, which refuses
  rather than silently pricing the run as the ordinary printing when the
  match doesn't actually carry that finish), sometimes a **printing of its
  own** ("PRE-ERRATA" - `conditionPrinting`), sometimes a **grading
  marker** ("BGS", "PSA", "Unique", "Shadowless", "No Set Symbol" -
  `isGraded`, which reroutes the row to the `Cool Stuff Inc (unique)`
  sub-seller via `MarketNames`/`availableMarketNames` rather than pricing
  it as a normal NM copy), and sometimes a **skip signal** ("Asian" -
  `isSkippedCondition`, since the one Evolving Wilds sold that way carries
  no language, set or number to resolve against). A grading service or print
  run this table doesn't know about is refused with a logged "unsupported
  <text> condition", not silently dropped as if it matched NM.
- **Graded/unique copies are a second seller, not a second condition.**
  `gradedMarkers`, `isGraded`, `AddRelaxed` (so a genuine finish collision -
  a Foil-etched card's Near Mint and Near Mint Foil rows - doesn't refuse
  itself out) and `availableMarketNames`/`name2shorthand` (`Cool Stuff Inc`
  / `Cool Stuff Inc (unique)`, shorthands `CSI`/`CSIUnique`) all serve one
  fact: a single graded/unique copy sold on a printing's page is not that
  printing's market price, and pricing it as one prints a $500 slab as a
  card's going rate.
- **Each entry keeps the store's own row id.** A sale offer's
  `InstanceID` is the row id its Add to Cart button names
  (`atc[<product>][<row>]`, `offerRowID`), one per condition and finish of
  a product; a buylist entry's is the row's `PPQID`. Sealed entries carry
  none.
- **Bundles divide the price, not the quantity.** `bundleRe`/`bundledCopies`
  ("Buy 1 get 3 free!") divides the listed price by the bundle size; the
  displayed quantity is left as the stock figure the page shows (capped at
  "20+" like any other row), not scaled to match.

## The recurring shape: a promo shelf collides with a real one

Found three times, independently, in three different games - first on the
**buylist** path in each case, where it was fixed and shipped; the retail
path turned out to carry the identical live listings, unfixed, until the
measurement lower down in this section closed that gap too:

> A card is sold under CSI's catch-all `Promo` shelf, and the wording
> beside it - a note, a bracket in the name - names the *real* set or
> event it actually belongs to. The promo shelf holds its own printing at
> that same number, so if nothing reads the wording, the two meet: the real
> printing's price lands on the promo shelf's card instead.

- **One Piece** (#379): `onePieceShelf` - a Starter Deck reprint sits on
  `Promo` with `(Starter Deck 18)` in the name; `P-041` is both the plain
  promo and the deck card. Redirect to `"Starter Deck " + N`, but **only**
  when the shelf is exactly `"Promo"` - every other listing naming a deck
  already arrives on a real set, where the bracket names the deck the card
  was *reprinted from*, not where it lives now.
- **Pokemon** (#391): `pokemonPromoShelf` - the Pokemon Day 2025 Eevee sits
  on `Promo`... no, sits on `SV Prismatic Evolutions` at `074/131`, the
  same number as that set's own Eevee, told apart from it only by
  `RarityName == "Promo"`. Here the direction is reversed from One Piece:
  the fix *probes* `Edition: "Promo"` first and only takes it if that
  resolves; twenty of the fifty candidate rows name a printing no promo
  shelf holds at all (the Pokemon Rumble cards, holo promos that are
  genuinely their own set's foil) and must keep the shelf they arrived on.
- **Riftbound** (#393): `riftboundShelf` - the Nexus Night runes sit on
  `Promo` with the issuing set written at the head of the note
  (`"UNL-R05b"`, `"SFD-R05b"`); the promo shelf holds an Organized Play
  printing at that same number. Redirect to the set the note's prefix
  names, but **only** when that set actually holds the printing - Vendetta
  issued no b-lettered rune of its own, so its six listings correctly stay
  on `Promo`, which is where their printing really is.

Two of the three - `pokemonPromoShelf` and `riftboundShelf` - share the
same guard: *redirect only when the redirect target resolves*, verified
with an actual `matcher.Match` probe before committing to it.
`riftboundShelf` also requires the probe to land in the set it asked for,
at the number the note or listing names (`riftboundLandsAt`).
`onePieceShelf` is looser: it redirects on the shelf-plus-bracket pattern
alone, with no resolve check, trusting that nothing else on the `Promo`
shelf ever carries a `(Starter Deck N)`-shaped bracket that isn't this
collision. That has held so far, but it is a real difference in rigor,
not just a simpler special case - if a fourth game needs this shape,
copy the probing version, not `onePieceShelf`'s.

All three are wired into each game's retail resolver as well as its
buylist one - confirmed as a live gap, not a hypothetical one, by fetching
each game's affected shelf live and replaying every row through the pre-
and post-fix resolution:

```
                          shelf                          compared  same  moved  gained  lost
One Piece                "Promo"                              220   213      7       0     0
Riftbound                "Promo"                               99    87     12       0     0
Pokemon                  "SV Prismatic Evolutions"             459   457      2       0     0
```

Every move landed on the correct printing - the same six Starter Deck
cards, the same twelve Nexus Night runes, and the Pokemon Day 2025 Eevee
*and* a Sylveon carrying the identical bracket that hadn't turned up in
the buylist capture. `pokemonPromoShelf` and `riftboundShelf` were
refactored to take primitive fields instead of `CSIPriceEntry` so both
paths could share one implementation; `onePieceShelf` needed no change,
since it already took primitives.

`nameQualifiers` remains buylist-only by design - One Piece's retail
`notes` field genuinely describes the artwork rather than the printing
(see "Per-game wording" below), which is a different, still-open question
from the shelf collision this section is about.

## Per-game wording, briefly

- **Magic**: two independent translation layers, not the game's own
  identifier - this game still has none in either feed. `preprocess.go`'s
  `Preprocess`/`preprocess` (retail) reconstructs set+number from the
  **image filename** when the wording alone won't resolve
  (`numFixes`, then a set-code/number split at length 3 and 4, then a
  letter-marked treatment split - `"TMCS0032"` is TMC #32's surge foil,
  where three or four raw characters read as no valid set/number pair at
  all); `PreprocessBuylist` instead trusts the buylist's own `Number`
  field first. Ahead of either, `finishSplitCard` reads a note offering one
  number per finish ("Card number can be 210 or 265", "Nonfoil is number
  0765, Foil is 0720") and answers the single printing at either number
  with the row's finish; a range or a longer list is not read. The image
  names the foil number on those listings, so the retail path needs the
  offer row's finish, passed into `preprocess`. Basic lands read their
  number off the image, or their letter's position among the set's plain
  numbers; a 32-character hash image names no number. `basicSibling`
  picks the unsuffixed or "a" printing where Zendikar or Battle for
  Zendikar files a lettered sibling under the same digits, and
  `anthologyDecks` moves a Duel Decks: Anthology basic to the deck its
  note names. Both funnel into `card2promo`, a large closed table of
  named promo printings the wording alone can't place (`card2promo` is a
  `switch cardName` - see AGENTS.md's "tables before code" on why this
  shape is a last resort, tolerated here only because the wording truly
  carries nothing else). `magicShelfFixups` is shared by both paths:
  emblems named "Emblem" with the planeswalker in the notes instead of the
  name, `"Duel Deck:"` singularized to the catalog's `"Duel Decks:"`,
  blank cards refused outright, and a `"NT Token"` note rewritten to
  `"N Token"`.
- **Pokemon**: `pokemonListing` is the single largest per-game function -
  about a dozen independent rewrites (Classic Collection reprints sold
  under Celebrations, "Special Metal Energy"→"Metal Energy" + Special
  variation, Imposter/Impostor Oak's Base-Set-only misspelling, Elite Four
  "Alakazam 4"→"Alakazam E4", prefixed energy/promo numbers like
  `MEE007`→set `MEE`, several literal misspellings, Non-Stamped/Illus.
  noise words stripped, a stamped promo redirected to the `Promo` shelf,
  Team Galactic invention names completed from their number, typed-energy
  letter spelling, and Nidoran's sex resolved by trying both). Layered on
  top: `firstEditionShelf`/`conditionRuns`/`matchRun` (a "1st Edition
  <Set>" *shelf name*, not a condition, is a whole other print run -
  refuse rather than silently answer with Unlimited), `pokemonNonHolo` (a
  `(Non-Holo)` bracket refused when the rarity says the card was never
  printed that way, and deliberately *not* refused the other direction - a
  holo bracket on a plain-rare row - because the catalog simply holding no
  nonfoil for a plain rare is a catalog gap rather than the storefront
  inventing a printing, and refusing those would drop real listings to
  catch nothing; see the function's own doc comment for the full
  reasoning), and `pokemonPromoShelf` above. Four more rules re-ask a
  listing where the catalog files it elsewhere, each kept only where the
  finished listing lands on the shelf's own set: `pokemonNoteShelves`
  (a note naming a regional championship, a Build & Battle kit, an XY
  prerelease or a Dragon Vault blister, whose stamp wording would
  otherwise demand a stamp the catalog does not label),
  `pokemonCosmosHolo` (a Promo-rarity listing bracketed Holo Promo, Cosmo
  Holo or Holo is the collection-box cosmos holo on the miscellaneous
  shelf, at the set's own number), `pokemonProfessor` (the other
  Prof./Professor spelling, taken only when it lands at the listing's
  number and the listing's own does not), and the jumbo shelf
  (`Pokemon Oversized Cards` is the catalog's Jumbo Cards, without the
  programme prefix on its number). `ME094` is the catalog's `MEP094`.
  `pokemonRespellings` is a replacer, so a bracket behind a misspelled
  name keeps matching. `pokemonCatalogShelves` is a closed table of
  listings the storefront files on a main or promo shelf for holo
  printings the catalog keeps elsewhere (the Shadowless Machamp deck
  card, Ancient Mew, the 2017 wave foil energies), and `pokemonListingIDs`
  names the product, keyed by name and edition, of the listings no matcher
  reads: the 30th Celebration Mews and a jumbo by their bare-letter
  numbers, the Pokemon Day Pikachus the POP Series catalog labels
  without the word Stamp, the prerelease Meganium its note only calls
  Stamped, and the Non-Holo Professor's Research SWSH178 a promo type
  keeps the matcher off. The shelf probes (`pokemonNonHoloShelf`,
  `pokemonCosmosHolo`, `pokemonDeckHoloRedirect`) take a shelf only where
  the probe lands on that shelf's set at the listing's number
  (`pokemonLandsAt`).
- **One Piece**: `onePiecePIDs` is a closed table of storefront product
  ids mapped to their TCGplayer product id, for the DON!! cards and
  reprinted promos the storefront words in prose the catalog never uses;
  it is read first, on both feeds, because the pid is shared by both and
  no wording reaches those printings (the catalog tells DON!! apart by
  promo types and watermarks the storefront does not write). Past that,
  both feeds build the variation the same way: `nameQualifiers` (the
  brackets the name carries behind the card's own) plus the notes, then
  `onePieceEvents`/`eventNamed` (a closed table of storefront nicknames
  for a catalog event - "afro luffy promo", a playmat SKU, a participation
  pack - one entry per nickname, deliberately not generalized: a nickname
  names one product). After the match, `onePieceRefined` corrects a
  landing the wording could not reach: `onePieceRenamedTreatment` (the
  Gear5 starter deck calls its premium printing "Full Art" where the
  catalog calls every premium printing in that set "Parallel" - the guard
  requires the word to name a real catalog label *somewhere*, the card's
  own number to hold no printing under that label, a single alternate
  printing at that number, and the name to carry no second qualifier, so a
  genuine Full Art is never touched) and `onePieceParallelPrinting` (OP01
  and OP02 file their alternate arts as Parallel, so the storefront's
  "Alternate Art" matched the PRB-01 reprint or the manga row: the shelf's
  own set answers when it holds one parallel printing of the number and no
  plain alternate art of it, unless the note names the matched set as the
  reprint, "PRB01 Reprint - OP01-120"). `onePieceAlternatePromo` files the 25th
  Edition premium card collection on the promo shelf as the card's
  Alternate Art. `onePieceSpellings` is a replacer that rewrites the part
  that differs and keeps the number or bracket behind it, and
  `onePieceShelf` is above.
- **Yu-Gi-Oh**: `catalogSpelling`/`csiSpellings` (nine hand-typos, kept as
  an exact table rather than nearest-match - 255 name pairs inside one set
  are a single edit apart, e.g. "Harpie Lady 1"/"2", so a distance-based
  corrector would silently answer a card the datastore is merely missing
  with its differently-numbered neighbour), `catalogColor`/`csiColors`
  ("(Light Blue)"→"(Silver)" for Duelist League prints, a *replace* not an
  *add* - the word "blue" on its own already names the blue printing, so
  leaving both would tie two printings where one used to resolve),
  `catalogRarity`/`csiRarities` (three foil-tier spelling differences),
  `printRunEdition`/`csiPrintRuns` (nine editions reprinted under their
  own original name, told apart only by the copyright date CSI writes into
  the note - deliberately left out of `matcher`'s own edition aliases
  for this exact reason), and `unknownPrinting`/`csiUnknownPrintings` (two
  hand-verified listings selling a rarity/number pair the catalog does not
  carry at all - refusing the general shape "rarity the card's number
  doesn't have" would drop 55 listings to catch these 2, and ~35 of the 55
  are legitimately-named decorated rarities). `yugiohWording` rewrites the
  words in a name or note the matcher would misread (a bare digit taken
  for the number, "No Stamp" read as the stamp, a mistyped GBI-001), and
  `yugiohArtNotes` asks the 26LP alternate art, described by its border
  and lettering, as the catalog's Emblazoned Alternate Art without the
  bracket or rarity that make the matcher read the Emblazoned Secret Rare.
  The buylist also needs `yugiohCodes` (SD9/HL3 for the catalog's
  SD09/HL03, in the Number and the note) and `yugiohNumbers` (three
  numbers it mistypes, keyed by name and the trimmed number since one typo
  is another card's real number). Both feeds retry a refused listing by its image, and
  accept the retry only on a card of the same name at that number:
  `yugiohImageCard` puts the code the buylist image sku carries where the
  row's corrected Number stood, and `yugiohSKUCard` reads a sale image's
  file name as a catalog number with its dash left out, through an index
  built once per run (`skuNumbers`). `yugiohRetries` keeps the retry off a
  listing the matcher skipped and off a "Can be" note on both.
  `csiAmbiguousNote` is shared with Pokemon: a note opening "Can be" says
  the product is one of several printings, and a refusal of one is not
  logged.
- **Gundam**: `gundam.go`, one file, four problems - the shelf name
  carries a set-code prefix that narrows nothing (the same number repeats
  across GD01/its beta/the deck-build box) and is stripped
  (`gundamShelf`); the card's own name and the run's wording are split by
  the **position** of the collector-number parenthetical between them,
  not by bracket-counting, because a card's own name can itself end in a
  parenthetical (`gundamCard`); a handful of storefront misspellings
  (`gundamNames`, exact-keyed for the reason `csiSpellings` gives) and one
  symbol-name rewrite (`gundamSymbolWord`, "Xi (Symbol) Gundam" → the
  literal Greek letter); and the storefront's number spelling disagrees
  with the catalog's in three ways at once - a dropped digit, an extra
  zero, and a rarity letter suffixed onto a card number that carries none
  (`gundamNumberSpelling`). `gundamNicknames` reads a promo run the
  storefront names after its art, the Green Dress Cagalli being the
  Championship Participation Pack 01 printing. `gundamPIDs` names by
  product id the starter deck reprints (ST05, ST07, ST08, and most of all
  the Destiny Ignition ST09) the storefront shelves under the card's first
  set, told apart only by a description of the art.
- **Palworld**: `palworld.go`, one file, one problem - the storefront
  sometimes glues a rarity code onto the number's tail
  (`"EBP01-025RR"`), inconsistently even with itself (plain-spaced on one
  shelf, glued on another). Its buylist sometimes leaves the number empty
  and writes it in the note, so that feed reads `buylistVariation`.
  `palworldBaseRarities` is an *enumerated allow-list* of codes that are
  safe to split off, deliberately excluding the six rarities (OSR, SP, SR,
  SSP, TSP, TSR) the catalog numbers as printings of their own - splitting
  those apart would answer the base card for a parallel. A rarity code
  nobody here has heard of is left glued to the number rather than
  guessed at. The LA Release Party souls are named "Soul - Dawn of
  Palpagos" with a colour tail where the catalog names every Soul promo
  "Soul" (`palworldSpellings`).
- **Lorcana**: `lorcana.go`, one file, three problems - a storefront
  misspelling (`lorcanaSpellings`, exact-keyed for the reason
  `csiSpellings` gives, and the more so here: a name is a character and a
  title joined by a dash, the title is the whole of what tells one
  printing of a character from another, and Rise of the Floodborn sells
  three Basils at consecutive numbers 138-140), and the Illumineer's Quest
  prize cards, which the storefront shelves on the set whose card they
  repeat at a number that set does not reach: `lorcanaQuestNotes` maps the
  frame the note names to its quest, kept only where the card exists
  there. Both feeds otherwise pass edition and the raw note/number
  straight through to `matcher.Match` untranslated. The third is
  sealed wording: `lorcanaSealedWords` turns the storefront's "Two-Player
  Starter Deck" and "Booster Case" into the catalog's "2-Player Starter
  Set" and "Booster Box Case" before the sealed resolver reads them.
- **Riftbound**: `riftboundShelf` redirects a Nexus Night rune by the set
  code its note opens with, and a promo whose name carries `(Prerelease)`
  or `(Origins Stamp)` to the Release Event Promos shelf, where the promo
  shelf holds a printing of its own at the same number. `riftboundNotes`
  drops the year from "Worlds Bundle 2025 Promo", and `riftboundSpellings`
  corrects a storefront misspelling of a card name before the shelf probe
  reads it.

## Known gaps

- `GameDragonBallSuper` (`"dbs"`) and `GameStarWarsUnlimited` (`"swu"`) are
  shelf-name constants CSI itself uses, sitting unwired in this file -
  `matcher.Game` has no constant for either game yet (`matcher/game.go`),
  so `csiGames` can't map them. Whoever adds either game to `matcher`
  should check back here; the shelf name is already known.
- Sealed is wired in `cmd/bantool` for six of the eight singles-wired
  games - Gundam and Palworld have no `coolstuffinc_sealed_*` scraper
  option, though `NewScraperSealed` itself would accept either game (both
  are in `csiGames`).
- The `Black Bordered (foreign)` shelf resolves to `ErrUnsupported` on
  every listing it carries, in every language - see "Data sources" above
  for the specifics. Not a mispricing, just a silent, total coverage gap
  on a shelf CSI actively sells on.

## Environment

- `CSI_PARTNER` - optional affiliate id appended to every outbound link
  (`utm_referrer`). Not a credential; nothing here requires auth.
- Tests load a live datastore per game through `datastore_test.go`'s
  `readGameDatastore`, which reads each game's file once and skips a test
  whose variable is unset. All eight games have tests here, gated on
  `ALLPRINTINGS5_PATH`, `POKEMON_PATH`, `ONEPIECE_PATH`, `RIFTBOUND_PATH`,
  `YUGIOH_PATH`, `LORCANA_PATH`, `GUNDAM_PATH` and `PALWORLD_PATH`. CI runs
  `coolstuffinc/...` under each non-Magic game's job, and the Magic job's
  repo-wide `go test ./... -v` picks up the Magic tests.
