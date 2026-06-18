package tcgplayer

import (
	"math"
	"testing"
)

// TestDirectPriceAfterFees pins the item-based Direct fee model (effective
// 2026-06-18) against TCGplayer's four published worked examples plus the
// $75 commission-cap and band-edge cases. Expected nets are hand-computed
// literals (NOT re-derived from the formula) so a wrong formula is caught.
// Net = price - fee, where for price >= $2.50:
//
//	fee = $1.12 + min($75, 8.95%*price) + 2.5%*price
//
// and for price < $2.50: fee = 50%*price.
func TestDirectPriceAfterFees(t *testing.T) {
	const eps = 1e-6
	cases := []struct {
		name    string
		price   float64
		wantNet float64
	}{
		// Example 1: $4.50 -> fee 1.12 + 0.40275 + 0.1125 = 1.63525 (TCG rounds to $1.64).
		{"example1_mid_value", 4.50, 2.86475},
		// Example 2: $25.00 -> fee 1.12 + 2.2375 + 0.625 = 3.9825 (TCG shows $3.98).
		{"example2_high_value", 25.00, 21.0175},
		// Example 3: $0.50 -> 50% band, fee 0.25 (TCG shows $0.25/card).
		{"example3_low_value", 0.50, 0.25},
		// Example 4: $55.00 -> fee 1.12 + 4.9225 + 1.375 = 7.4175 (TCG total $22.25 for 3).
		{"example4_high_value", 55.00, 47.5825},
		// Commission cap: 8.95%*1000 = 89.50, capped at 75.00 -> fee 1.12 + 75 + 25 = 101.12.
		{"commission_cap_engaged", 1000.00, 898.88},
		// Band edge: just below $2.50 stays in the 50% band.
		{"band_edge_below", 2.49, 1.245},
		// Band edge: at $2.50 the flat $1.12 item fee engages -> fee 1.12 + 0.22375 + 0.0625 = 1.40625.
		{"band_edge_at", 2.50, 1.09375},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DirectPriceAfterFees(tc.price)
			if math.Abs(got-tc.wantNet) > eps {
				t.Errorf("DirectPriceAfterFees(%.2f) = %.6f, want %.6f", tc.price, got, tc.wantNet)
			}
		})
	}
}
