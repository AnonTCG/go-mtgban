package mtgban

import (
	"math"
	"slices"
	"strconv"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/matcher/sealed"
)

// ArbitOpts is the bag of optional filters and thresholds Arbit and Mismatch
// narrow their two sides with. Every field is optional, a nil ArbitOpts
// filters nothing, and zero-valued filters leave the two sides' common cards
// eligible. The datastore is not one of these: it is the reports' first
// parameter, since none of them can resolve a card without it. Custom
// callbacks doing auxiliary lookups should use that same backend.
type ArbitOpts struct {
	// Extra factor to modify Inventory prices
	Rate float64

	// Minimum Inventory price
	MinPrice float64

	// Minimum Buylist price
	MinBuyPrice float64

	// Minimum difference between prices
	MinDiff float64

	// Minimum Inventory quantities
	MinQuantity int

	// Minimum spread % between prices
	MinSpread float64

	// Maximum Spread % between prices
	MaxSpread float64

	// Maximum price ratio of Inventory
	MaxPriceRatio float64

	// Whether to consider foils
	NoFoil   bool
	OnlyFoil bool

	// Whether to skip non-rl cards
	OnlyReserveList bool

	// List of conditions to ignore
	Conditions []Condition

	// List of rarities to ignore
	Rarities []string

	// List of editions (or set codes) to ignore
	Editions []string

	// List of editions (or set codes) to select
	OnlyEditions []string

	// List of per-edition collector numbers to select
	OnlyCollectorNumberRanges map[string][2]int

	// Only run for products with static decklists
	SealedDecklist bool

	// Only select entries which are part of a bundle
	OnlyBundles bool

	// List of seller name that will be considered
	Sellers []string

	// Custom function to be run on the card object
	// It returns a custom factor to be applied on the compared price,
	// and whether the entry should be skipped or not
	CustomCardFilter func(co *matcher.CardObject) (float64, bool)

	// Custom function to be run on the probed inventory entry
	// It returns a custom factor to be applied on the compared price,
	// and whether the entry should be skipped or not
	CustomPriceFilter func(string, InventoryEntry) (float64, bool)

	// Constant used to offset prices (the higher the value, the less impactful
	// lower prices will be, and vice versa)
	ProfitabilityConstant float64

	// Minimum profitability value
	MinProfitability float64

	// List of languages to ignore
	Languages []string

	// List of languages to select
	OnlyLanguages []string
}

// ArbitEntry is one card worth acting on, carrying both sides of the
// comparison that produced it so a caller can show its working. Which side is
// filled depends on the function that returned it: Arbit sets BuylistEntry
// against InventoryEntry, Mismatch sets ReferenceEntry against InventoryEntry.
type ArbitEntry struct {
	// ID of the card
	CardID string

	// The buylist used to determine Arbit
	BuylistEntry BuylistEntry

	// The actual entry that matches either of the above
	InventoryEntry InventoryEntry

	// The inventory used to determine Mismatch
	ReferenceEntry InventoryEntry

	// Difference of the prices
	Difference float64

	// Spread between the the prices
	Spread float64

	// Difference of the prices accounting for quantities available
	AbsoluteDifference float64

	// Amount of cards that can be applied
	Quantity int

	// The higher the number the better the arbit is. Using this formula
	// Profitability Index (PI) =
	//   (Difference / (Sell Price + k)) * log10(1 + Spread) * Units^(1/4)
	// where k is ProfitabilityConstant, a denominator-stabilizing constant
	// that keeps cheap cards from dominating (reference value 10; configurable
	// via ArbitOpts, defaults to 0). The Units^(1/4) term is applied only when
	// Units > 1, and the index is 0 when Spread <= 0.
	Profitability float64
}

// resolvedOpts holds the resolved filter and threshold values from ArbitOpts,
// with defaults applied for nil opts.
type resolvedOpts struct {
	backend                *matcher.Backend
	minDiff                float64
	minSpread              float64
	maxSpread              float64
	minPrice               float64
	minBuyPrice            float64
	minQty                 int
	minProfitability       float64
	maxPriceRatio          float64
	rate                   float64
	profitabilityConstant  float64
	filterFoil             bool
	filterOnlyFoil         bool
	filterRLOnly           bool
	filterDecksOnly        bool
	filterBundle           bool
	filterConditions       []Condition
	filterRarities         []string
	filterEditions         []string
	filterSelectedEditions []string
	filterSelectedCNRange  map[string][2]int
	filterSellers          []string
	filterFunc             func(co *matcher.CardObject) (float64, bool)
	filterPriceFunc        func(string, InventoryEntry) (float64, bool)

	filterLanguages         []string
	filterSelectedLanguages []string
}

func resolveOpts(b *matcher.Backend, opts *ArbitOpts) resolvedOpts {
	r := resolvedOpts{
		rate:    1.0,
		backend: b,
	}
	if opts == nil {
		return r
	}

	if opts.MinDiff != 0 {
		r.minDiff = opts.MinDiff
	}
	if opts.MinSpread != 0 {
		r.minSpread = opts.MinSpread
	}
	if opts.Rate != 0 {
		r.rate = opts.Rate
	}
	if opts.ProfitabilityConstant > 0 {
		r.profitabilityConstant = opts.ProfitabilityConstant
	}
	r.minPrice = opts.MinPrice
	r.minBuyPrice = opts.MinBuyPrice
	r.minQty = opts.MinQuantity
	r.maxPriceRatio = opts.MaxPriceRatio
	r.maxSpread = opts.MaxSpread
	r.minProfitability = opts.MinProfitability
	r.filterFoil = opts.NoFoil
	r.filterOnlyFoil = opts.OnlyFoil
	r.filterRLOnly = opts.OnlyReserveList
	r.filterDecksOnly = opts.SealedDecklist
	r.filterBundle = opts.OnlyBundles
	r.filterFunc = opts.CustomCardFilter
	r.filterPriceFunc = opts.CustomPriceFilter
	r.filterConditions = opts.Conditions
	r.filterRarities = opts.Rarities
	r.filterEditions = opts.Editions
	r.filterSelectedEditions = opts.OnlyEditions
	r.filterLanguages = opts.Languages
	r.filterSelectedLanguages = opts.OnlyLanguages
	r.filterSelectedCNRange = opts.OnlyCollectorNumberRanges
	r.filterSellers = opts.Sellers

	return r
}

// filterCard checks whether a card should be skipped based on the resolved
// options. Returns the custom factor and true if the card should be kept.
func (r *resolvedOpts) filterCard(cardID string) (*matcher.CardObject, float64, bool) {
	co, err := r.backend.GetUUID(cardID)
	if err != nil {
		return nil, 0, false
	}
	if slices.Contains(r.filterRarities, co.Rarity) {
		return nil, 0, false
	}
	if r.filterFoil && (co.Foil || co.Etched) {
		return nil, 0, false
	}
	if r.filterOnlyFoil && !co.Foil && !co.Etched {
		return nil, 0, false
	}
	if r.filterDecksOnly && co.Sealed && !sealed.HasDecklist(r.backend, co.SetCode, cardID) {
		return nil, 0, false
	}
	if r.filterRLOnly && !co.IsReserved {
		return nil, 0, false
	}
	if slices.Contains(r.filterEditions, co.Edition) || slices.Contains(r.filterEditions, co.SetCode) {
		return nil, 0, false
	}
	if r.filterSelectedEditions != nil && !slices.Contains(r.filterSelectedEditions, co.Edition) && !slices.Contains(r.filterSelectedEditions, co.SetCode) {
		return nil, 0, false
	}
	if slices.Contains(r.filterLanguages, co.Language) {
		return nil, 0, false
	}
	if r.filterSelectedLanguages != nil && !slices.Contains(r.filterSelectedLanguages, co.Language) {
		return nil, 0, false
	}
	cnRange, found := r.filterSelectedCNRange[co.Edition]
	if found {
		cn, err := strconv.Atoi(co.Number)
		if err == nil && (cn < cnRange[0] || cn > cnRange[1]) {
			return nil, 0, false
		}
	}

	customFactor := 1.0
	if r.filterFunc != nil {
		factor, skip := r.filterFunc(co)
		if skip {
			return nil, 0, false
		}
		customFactor = factor
	}

	return co, customFactor, true
}

// buyable reports whether the copy on the shelf is one the options allow
// buying at all, and what it costs once every factor they carry is applied.
// Both reports are about a copy someone could buy, so both ask this.
func (r *resolvedOpts) buyable(cardID string, entry InventoryEntry, noQuantity bool, factor float64) (float64, bool) {
	if slices.Contains(r.filterConditions, entry.Conditions) {
		return 0, false
	}
	if r.filterSellers != nil && !slices.Contains(r.filterSellers, entry.SellerName) && !slices.Contains(r.filterSellers, entry.CustomFields["SubSellerName"]) {
		return 0, false
	}
	if r.filterBundle && !entry.Bundle {
		return 0, false
	}
	if !noQuantity && entry.Quantity < r.minQty {
		return 0, false
	}
	if entry.Price < r.minPrice {
		return 0, false
	}
	if r.filterPriceFunc != nil {
		custom, skip := r.filterPriceFunc(cardID, entry)
		if skip {
			return 0, false
		}
		factor *= custom
	}
	return entry.Price * factor * r.rate, true
}

// quote is what the other side is worth for one copy: the price it pays or
// asks, already brought to that copy's condition, how many it will take -
// zero for no limit - and the entry it was read from, which the row carries
// so a caller can show its working.
type quote struct {
	price    float64
	quantity int

	// Exactly one of these is set, and it says which report the row belongs
	// to: a vendor's offer to buy, or another shelf's asking price.
	buylist BuylistEntry
	shelf   InventoryEntry
}

// buys is the quote a vendor's offer makes.
func buys(entry BuylistEntry) quote {
	return quote{price: entry.BuyPrice, quantity: entry.Quantity, buylist: entry}
}

// asks is the quote another shelf makes, at the price its copy is worth once
// regraded to the one being bought.
func asks(entry InventoryEntry, regraded float64) quote {
	return quote{price: regraded, quantity: entry.Quantity, shelf: entry}
}

// arbitrage is the comparison both reports are: what a copy costs against
// what the other side gives for it.
func (r *resolvedOpts) arbitrage(cardID string, entry InventoryEntry, price float64, q quote) (ArbitEntry, bool) {
	if price == 0 || q.price == 0 {
		return ArbitEntry{}, false
	}

	spread := 100 * (q.price - price) / price
	difference := q.price - price

	if r.maxSpread != 0 && spread > r.maxSpread {
		return ArbitEntry{}, false
	}
	if difference < r.minDiff {
		return ArbitEntry{}, false
	}
	if spread < r.minSpread {
		return ArbitEntry{}, false
	}

	// Find the minimum amount tradable
	qty := entry.Quantity
	if q.quantity != 0 {
		qty = min(entry.Quantity, q.quantity)
	}

	// A spread at or below zero pays nothing and scores zero. log10(1+spread)
	// is NaN below -1% and -Inf at -1% (a +Inf index), and in between it
	// scores a loss above a gain of the same size
	profitability := 0.0
	if spread > 0 {
		profitability = (difference / (price + r.profitabilityConstant)) * math.Log10(1+spread)
		if qty > 1 {
			profitability *= math.Pow(float64(qty), 0.25)
		}
	}
	if profitability < r.minProfitability {
		return ArbitEntry{}, false
	}

	return ArbitEntry{
		CardID:             cardID,
		InventoryEntry:     entry,
		BuylistEntry:       q.buylist,
		ReferenceEntry:     q.shelf,
		Difference:         difference,
		AbsoluteDifference: difference * float64(qty),
		Spread:             spread,
		Quantity:           qty,
		Profitability:      profitability,
	}, true
}

// Arbit reports the cards a vendor buys for more than a seller asks, the
// trade that pays for itself. Every card id is resolved against b, only cards
// both sides carry are considered, and opts filters the rest.
func Arbit(b *matcher.Backend, opts *ArbitOpts, vendor Vendor, seller Seller) []ArbitEntry {
	// The CSV functions refuse a nil backend with an error naming themselves,
	// because they produce output and an unresolved id would silently write a
	// wrong file. A report has nothing to resolve instead of something to
	// write, so it answers empty: Arbit(nil, ...) reads as no opportunities,
	// which is what the caller chose by passing nil.
	if b == nil {
		return nil
	}

	var result []ArbitEntry

	r := resolveOpts(b, opts)

	buylist := vendor.Buylist()
	inventory := seller.Inventory()
	noQuantity := seller.Info().NoQuantityInventory

	for cardID, blEntries := range buylist {
		invEntries, found := inventory[cardID]
		if !found {
			continue
		}

		// The first entry is always NM, and gates the whole card
		nmEntry := blEntries[0]

		if r.maxPriceRatio != 0 && nmEntry.PriceRatio > r.maxPriceRatio {
			continue
		}

		if nmEntry.BuyPrice < r.minBuyPrice {
			continue
		}

		_, customFactor, ok := r.filterCard(cardID)
		if !ok {
			continue
		}

		for _, invEntry := range invEntries {
			price, ok := r.buyable(cardID, invEntry, noQuantity, customFactor)
			if !ok {
				continue
			}

			// Re-anchor on NM for every inventory entry: the condition
			// matching below only rebinds this when the entry is not NM,
			// so a value carried over from a previous iteration would
			// price this entry against another entry's grade
			blEntry := nmEntry

			// When invEntry is not NM, we need to account for conditions
			if invEntry.Conditions != NM {
				i := 0
				for i = range blEntries {
					if blEntries[i].Conditions == invEntry.Conditions {
						break
					}
				}
				blEntry = blEntries[i]
				// If, after looping, a matching condition was not found,
				// just skip the current invEntry
				if blEntry.Conditions != invEntry.Conditions {
					continue
				}
			}

			// Check again to account for conditions
			if blEntry.BuyPrice < r.minBuyPrice {
				continue
			}

			res, ok := r.arbitrage(cardID, invEntry, price, buys(blEntry))
			if !ok {
				continue
			}
			result = append(result, res)
		}
	}

	return result
}

// A generic grading map that estimates common deductions
var defaultGradeMap = map[Condition]float64{
	NM: 1, SP: 0.8, MP: 0.6, HP: 0.4, PO: 0,
}

// Mismatch compares two sellers rather than the two sides of one book,
// reporting where probe asks less than reference for the same card. Same
// arithmetic as Arbit, against a price the market has settled on instead of
// against an offer to buy.
func Mismatch(b *matcher.Backend, opts *ArbitOpts, reference, probe Seller) []ArbitEntry {
	if b == nil {
		return nil
	}

	var result []ArbitEntry

	r := resolveOpts(b, opts)

	refInventory := reference.Inventory()
	inventory := probe.Inventory()
	noQuantity := probe.Info().NoQuantityInventory

	for cardID, refEntries := range refInventory {
		invEntries, found := inventory[cardID]
		if !found {
			continue
		}

		_, customFactor, ok := r.filterCard(cardID)
		if !ok {
			continue
		}

		// The copy being bought leads, the way it does in Arbit: the
		// options describe what is worth buying, so they are asked once
		// about the copy rather than once per price quoted at it.
		for _, invEntry := range invEntries {
			price, ok := r.buyable(cardID, invEntry, noQuantity, customFactor)
			if !ok {
				continue
			}

			invGrade, found := defaultGradeMap[invEntry.Conditions]
			if !found || invGrade <= 0 {
				continue
			}

			for _, refEntry := range refEntries {
				if slices.Contains(r.filterConditions, refEntry.Conditions) {
					continue
				}
				if refEntry.Price < r.minPrice {
					continue
				}

				// Undo the reference's own grade before applying the copy's,
				// otherwise a non-NM reference is compared against a rescaled
				// copy of itself. A zero factor cannot be divided by, so skip
				// the pair rather than report an infinite spread.
				refGrade, found := defaultGradeMap[refEntry.Conditions]
				if !found || refGrade <= 0 {
					continue
				}

				res, ok := r.arbitrage(cardID, invEntry, price, asks(refEntry, refEntry.Price*invGrade/refGrade))
				if !ok {
					continue
				}
				result = append(result, res)
			}
		}
	}

	return result
}
