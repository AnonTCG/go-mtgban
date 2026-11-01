# Game Nerdz

The storefront publishes two feeds per game. Retail carries the catalog's
TCGplayer id for nearly every product; the buylist carries none, but a product
shares the storefront's id with its retail record, and the crawl keeps that
record's TCGplayer id. `resolveProduct` reads Magic by id first - retail by its
own, the buylist by its retail record's, kept only where the printing it names
is the card the display writes. Every other row is read by wording, and the id
places it only where the wording is refused and the id names the same card.
A buylist-only run (`-vendors gamenerdz`) crawls no retail record, so its
buylist is read by wording alone.

## Guards on the wording rules

Four guards keep the buylist rules from widening:

- The typed-energy and Greek respellings apply only where the storefront's own
  name is unknown to the catalog and the respelled one is known. Applied
  unguarded, the energy rule rewrote `Basic Darkness Energy` and
  `Aromatic Grass Energy` and lost 98 landings.
- The numberless basic-energy retry is kept only where it lands a printing the
  catalog files with no number.
- A One Piece listing with no card code is kept only where it lands a printing
  numbered `LEADER`; its label alone is too weak to trust anywhere else.
- The sized-number retry runs only after a refusal, on a dashed listing, and is
  taken only when it lands the shelf's own set at that number. Stripping the
  size always lost five Miscellaneous Cards cosmos promos.
- The finish tiebreak (every game but Magic) runs only on an aliasing refusal,
  and answers only when one candidate has the listing's finish and either all
  share a set and number or it is the shelf's own set. Without the shelf test
  `Seel 41`, whose body says Platinum, would land the plain Base Set card
  instead of the Shadowless one.
- The finish a Pokemon display name ends on wins over `selectedFinish` only by
  way of the landed card's own siblings (`MatchIDFinish`), so a finish the
  card was never printed in changes nothing. `VS Seeker (CLC)` is named
  Holofoil while its id and sku say Normal, and stays Normal for that reason.
  Only a print run the card was not printed in drops the listing: Lapras and
  Miracle Energy are sold as a plain `1st Edition` beside their real
  holofoils, and landed on the Unlimited Holofoil's id.

Converting every `[...]` to `(...)` is wrong as well: `Dragapult - 091/192
[Rebel Clash]` names an origin set, so only `[Winner]` is read.

## Known remaining

- Fairy Energy in Kalos Starter Set carries the id 85378, which TCGplayer
  deleted, and the datastore has no such energy: not a datastore gap. It is
  skipped by its storefront id with seven other listings for printings never
  made (`unmadeProducts`).
- Aquapolis listings written over a plain number (`Drowzee 74/147`) name two
  printings (74a, 74b) and stay refused unless the retail id names one.
- `Monkey.D.Luffy (Release Event Leader) (P)` stays refused. Its label alone
  lands the OP-PR P-135 Green character, while the vendor's retail id 634531
  is one TCGplayer deleted.
- A few retail ids name another card than their display: `Seel 41` carries
  Ludicolo's, `Beedrill 17/102` Ekans's, `Clickslither (VMA-156)` Necroblossom
  Snarl's. The buylist refuses them; Magic retail lands them as the id says.
- `Gum-Gum Mole Pistol (Premium Card Collection -Best Selection Vol.5)` has no
  card code and is not a leader, so it stays refused.
