package cardtrader

import "testing"

// TestTCGplayerIDOverrides pins tcgIDOverrides' corrections against a few of
// the blueprints measured on the live catalog: three whose own id names a
// sibling card, one One Piece DON!! promo Card Trader sends no id for at
// all, and one ordinary blueprint outside the table, which must pass its
// own id through unchanged.
func TestTCGplayerIDOverrides(t *testing.T) {
	tests := []struct {
		desc string
		bp   *Blueprint
		want int
	}{
		{"Saltwater Swell (Yellow) SEA142 Cold Foil, id named Blue", &Blueprint{ID: 334549, TCGplayerID: 633633}, 633634},
		{"Saltwater Swell (Blue) SEA143 Cold Foil, id named Yellow", &Blueprint{ID: 334550, TCGplayerID: 633634}, 633633},
		{"Judge Professor Program Stamp 167/182, id named a different Judge", &Blueprint{ID: 357942, TCGplayerID: 658743}, 685999},
		{"Professor Turo's Scenario Professor Program Stamp 171/182, no id sent at all", &Blueprint{ID: 285780, TCGplayerID: 0}, 658742},
		{"Riftbound The Exile 149, id named Blade of the Exile", &Blueprint{ID: 415816, TCGplayerID: 722279}, 723434},
		{"Riftbound Seraphine 065a Alternate Art, id named the plain card", &Blueprint{ID: 417516, TCGplayerID: 723430}, 723429},
		{"One Piece DON!! Pop Art, no id sent at all", &Blueprint{ID: 290971, TCGplayerID: 0}, 544805},
		{"One Piece DON!! EB-03 Heroines, no id sent at all", &Blueprint{ID: 358981, TCGplayerID: 0}, 710745},
		{"Riftbound Xin Zhao T1 Worlds Showcase, no id sent at all", &Blueprint{ID: 400778, TCGplayerID: 0}, 716016},
		{"O-Nami Dash Pack 2025, id named the Illustration Box", &Blueprint{ID: 326797, TCGplayerID: 623070}, 712033},
		{"Team Rocket's Giovanni DRI 174, no id sent at all", &Blueprint{ID: 337188, TCGplayerID: 0}, 639952},
		{"Cintari Saber FAB184 Golden, id sent was the Cardmarket one", &Blueprint{ID: 281847, TCGplayerID: 760040}, 580614},
		{"an ordinary blueprint outside the table", &Blueprint{ID: 999999, TCGplayerID: 12345}, 12345},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if got := tt.bp.TCGplayerProductID(); got != tt.want {
				t.Errorf("TCGplayerProductID(%d) = %d, want %d", tt.bp.ID, got, tt.want)
			}
		})
	}
}
