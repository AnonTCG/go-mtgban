package onepiece

import (
	"testing"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestPackPair pins the printing a wording reaches where an event's two
// cards carry two product names. The catalog names them "Tournament Pack
// Vol. 5" and "Winner Pack Vol. 5" for seven volumes of nine, and spells the
// winner's inline - "Tournament Pack Vol. 2 Winner" - for the other two. One
// storefront wording covers both, so the wording that names the label
// outright and the wording that leaves the word over must answer alike.
func TestPackPair(t *testing.T) {
	b := gametest.Backend(t, matcher.GameOnePiece)

	for _, tt := range []struct {
		desc    string
		in      matcher.InputCard
		wantSet string
		wantNum string
		wantLbl string
	}{
		{
			desc:    "the winner's pack, which the catalog names apart",
			in:      matcher.InputCard{Name: "Kaido", Edition: "Promo", Variation: "ST04-003 Tournament Pack Vol. 5 - Winner"},
			wantSet: "OP-PR", wantNum: "ST04-003", wantLbl: "Winner Pack Vol. 5",
		},
		{
			desc:    "the same event's playing card keeps its own",
			in:      matcher.InputCard{Name: "Kaido", Edition: "Promo", Variation: "ST04-003 Tournament Pack Vol. 5"},
			wantSet: "OP-PR", wantNum: "ST04-003", wantLbl: "Tournament Pack Vol. 5",
		},
		{
			desc:    "and the catalog's own spelling of the winner's",
			in:      matcher.InputCard{Name: "Kaido", Edition: "Promo", Variation: "ST04-003 Winner Pack Vol. 5"},
			wantSet: "OP-PR", wantNum: "ST04-003", wantLbl: "Winner Pack Vol. 5",
		},
		{
			desc:    "a volume the catalog spells inline is untouched",
			in:      matcher.InputCard{Name: "Nami", Edition: "Promo", Variation: "ST01-007 Tournament Pack Vol. 3 - Winner"},
			wantSet: "OP-PR", wantNum: "ST01-007", wantLbl: "Tournament Pack Vol. 3 Winner",
		},
		{
			desc:    "and so is its participant",
			in:      matcher.InputCard{Name: "Nami", Edition: "Promo", Variation: "ST01-007 Tournament Pack Vol. 3 - Participant"},
			wantSet: "OP-PR", wantNum: "ST01-007", wantLbl: "Tournament Pack Vol. 3 Participant",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			in := tt.in
			id, err := b.Match(&in)
			if err != nil {
				t.Fatalf("Match(%v) = %v", tt.in, err)
			}
			co := b.UUIDs[id]
			label := promoLabel(b, co.Card)
			if co.SetCode != tt.wantSet || co.Number != tt.wantNum || label != tt.wantLbl {
				t.Errorf("Match(%v) = %s|%s|%q, want %s|%s|%q", tt.in,
					co.SetCode, co.Number, label, tt.wantSet, tt.wantNum, tt.wantLbl)
			}
		})
	}
}
