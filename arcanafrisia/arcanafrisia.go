// Package arcanafrisia scrapes Arcana Frisia.
package arcanafrisia

import (
	"context"
	"fmt"
	"time"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
)

// Arcanafrisia prices what Arcana Frisia buys; they publish no sale prices.
type Arcanafrisia struct {
	logCallback mtgban.LogCallbackFunc

	backend *matcher.Backend

	buylistDate time.Time
	buylist     mtgban.BuylistRecord
}

// NewScraper returns a buylist scraper matching against b.
func NewScraper(b *matcher.Backend) *Arcanafrisia {
	af := Arcanafrisia{backend: b}
	af.buylist = mtgban.BuylistRecord{}
	return &af
}

func (af *Arcanafrisia) printf(format string, a ...any) {
	if af.logCallback != nil {
		af.logCallback("[AF] "+format, a...)
	}
}

// afCondition maps the store's grades onto our own; it grades on
// Cardmarket's four steps, NM, EX, GD and PL, with PL below GD read as HP,
// so it keeps its own table.
func afCondition(s string) (mtgban.Condition, error) {
	switch s {
	case "NM":
		return mtgban.NM, nil
	case "EX":
		return mtgban.SP, nil
	case "GD":
		return mtgban.MP, nil
	case "PL":
		return mtgban.HP, nil
	default:
		return "", fmt.Errorf("unknown condition %q", s)
	}
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (af *Arcanafrisia) Load(ctx context.Context) error {
	rate, err := mtgban.GetExchangeRate(ctx, "EUR")
	if err != nil {
		return err
	}

	cards, err := GetBuylist(ctx)
	if err != nil {
		return err
	}
	af.printf("Found %d buylist entries", len(cards))

	for _, card := range cards {
		cardID, err := af.backend.MatchID(card.ScryfallID, card.Finish == "foil")
		if err != nil {
			if !af.backend.IsToken(card.Name) {
				af.printf("%v: %s %s (%s)", err, card.ScryfallID, card.Name, card.SetCode)
			}
			continue
		}

		cond, err := afCondition(card.Condition)
		if err != nil {
			af.printf("unsupported %s condition", card.Condition)
			continue
		}

		out := &mtgban.BuylistEntry{
			Conditions: cond,
			BuyPrice:   card.PriceEUR * rate,
			Quantity:   card.BuyLimit,
			URL:        card.URL,
		}
		err = af.buylist.AddRelaxed(cardID, out)
		if err != nil {
			af.printf("%v", err)
		}
	}

	af.buylistDate = time.Now()

	return nil
}

// Buylist returns what Load collected. See mtgban.Vendor.
func (af *Arcanafrisia) Buylist() mtgban.BuylistRecord {
	return af.buylist
}

// Info describes this scraper. See mtgban.Scraper.
func (af *Arcanafrisia) Info() (info mtgban.ScraperInfo) {
	info.Name = "Arcana Frisia"
	info.Shorthand = "AF"
	info.CountryFlag = "EU"
	info.BuylistTimestamp = &af.buylistDate
	info.Game = matcher.GameMagic
	return
}
