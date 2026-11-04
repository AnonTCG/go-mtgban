# cardkingdom

Scrapes Card Kingdom through its public pricelist. Retail prices per grade are
in the feed, read straight from `condition_values`. Buy prices per grade are
not: the feed has one `price_buy`, for NM, and `gradedBuyPrices` works out the
rest from the schedule Card Kingdom publishes. This file is about that
schedule, and the part of it the feed makes hard to read.

## The schedule

| Row | Applies to | EX | VG | G |
| --- | --- | --- | --- | --- |
| Under $15 | NM buy price | 80% | 70% | 50% |
| $15.00 to $24.99 | NM buy price | 85% | 70% | 50% |
| $25.00 to $99.99 | NM buy price | 85% | 75% | 65% |
| $100.00 and up | NM buy price | 90% | 80% | 70% |
| Alpha, Beta, Unlimited | `edition` | 80% | 60% | 40% |
| Foils | see below | 75% | 50% | 30% |

The rows are in the order Card Kingdom publishes them, which does not say which
one wins. Alpha, Beta, Unlimited and Foils both win over the price rows at any
price: a foil buying at 325.00 at NM pays 243.75 at EX, not the 292.50 of the
top price row.

Percentages apply to the NM buy price, never the retail price, and the result
rounds half up to the cent. So a price row is chosen by what Card Kingdom pays,
and a card buying at 9.00 and selling at 19.99 is on the first row, not the
second.

The retail grade prices are no guide to these. Retail foils step down
80/60/40, not 75/50/30, and retail's price rows go by the retail price. Scaling
`price_buy` by the retail ratios overprices the Foils row.

## Which foils are on the Foils row

The row covers what Card Kingdom's carts and invoices label `<Edition> Foil`,
such as "Odyssey Foil". That is narrower than `is_foil`. All of `Promotional`
and `Secret Lair`, the From the Vault sets, Masterpiece Series and others are
`is_foil` too, are not labelled, and are paid on the price rows.

The feed has no such edition: `edition` reads "Odyssey" on both printings. What
it has is the URL, which `paidAsFoil` reads. A labelled foil gets `-foil` after
the name and variation:

| Variation | URL ends | Row |
| --- | --- | --- |
| none | `angelic-favor-foil` | Foils |
| `004 - Surge Foil` | `the-swarmlord-004-surge-foil-foil` | Foils |
| `Foil Etched` | `metropolis-reformer-foil-etched-foil` | Foils |
| `Judge Foil` | `living-death-judge-foil` | price rows |
| `001 - Foil` | `snow-covered-plains-001-foil` | price rows |
| `Foil Etched` | `prossh-skyraider-of-kher-foil-etched` | price rows |
| none | `bladewing-the-risen` | price rows |

A variation ending in "Foil" already ends the URL in `-foil`, so there only
`-foil-foil` shows the label. `is_foil` still has to be true, because
`plains-mps-2007-non-foil` ends the same way.

This reading of the URL is inferred from the data. Card Kingdom does not
document it, and if it builds URLs differently one day, foils land on the wrong
row with no error.

## The 65% check

Retail prices show the same split independently, which makes them the way to
catch that. On an `is_foil` row, retail VG sells at about 60% of retail NM on a
labelled foil and at 70% or more on the rest, and no row falls between, so 65%
is only a line drawn in the gap. Card Kingdom publishes no such number and the
code does not read it.

The two agree on all 64,748 foil rows of the 2026-11-03 pricelist. If they ever
part, Card Kingdom has changed its URLs or its retail steps, and `paidAsFoil`
needs another look. Nothing runs this comparison automatically.

## How far this is proven

Measured on 1,361 real sell orders from 2018 to 2026, in totals only:

- The label and the 65% check agree on all 5,826 foil products.
- Of the 2,565 lines whose prices tell the Foils row from a price row, 2,562 fit
  the label, against 2,246 for `is_foil` alone. All 2,174 such lines since 2021
  fit.
- Older orders are less tidy: 3 lines missed, and another 9 fit no row,
  apparently because in 2019 and 2020 Card Kingdom sometimes rounded down.
- All 88 lines that can tell a buy price row from a retail price row follow the
  buy price.

Some of the table rests on Card Kingdom's word alone, with no payout behind it:
$25.00 to $99.99 at G, $100.00 and up at VG and G, and Alpha, Beta, Unlimited at
EX and G. $100.00 and up at EX and Alpha, Beta, Unlimited at VG rest on one card
each, and foils over $100 on two.

## Measuring it again

Payouts are the ground truth, on a signed-in account's selling history:
`/myaccount/selling_history` lists the orders, and
`/myaccount/po_invoice/<order>` lists one. An invoice groups its lines under an
edition header, which carries the label, and prints each line as
`<quantity> <grade> @ $<price>`.

- Only completed orders count. A canceled or unverified one has not been
  graded.
- An invoice shows an NM price only when a copy sold at NM. A lone EX line tests
  nothing, since its NM price is unknown. Two grades of one card in one order do
  test a row, even without NM, by searching for an NM price that fits both.
- Many low prices fit several rows, and 80% and 85% often round alike. Count
  only lines that tell the rows apart.
- An order is a real account's data: commit totals, never its cards, prices or
  order ids. Tests and examples use rows from the public pricelist.
