package arcanafrisia

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/mtgban/go-mtgban/mtgban"
)

const buylistURL = "https://buylist.arcanafrisia.com/buylist.csv"

// Card is one entry of the published buylist.
type Card struct {
	Name            string
	SetCode         string
	CollectorNumber string
	ScryfallID      string
	TCGplayerID     string
	Condition       string
	Finish          string
	PriceEUR        float64
	BuyLimit        int
	Language        string
	URL             string
}

// GetBuylist downloads the whole buylist in one call. It also reports whether
// the server cut the feed short, in which case only the complete records are
// returned.
func GetBuylist(ctx context.Context) ([]Card, bool, error) {
	// Bust any cache in front of the feed so the daily scrape gets a fresh copy
	url := buylistURL + "?v=" + strconv.FormatInt(time.Now().Unix(), 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, false, err
	}

	resp, err := mtgban.NewHTTPClient().Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}
	return parseBuylist(body)
}

// parseBuylist reads a buylist body. The server stops the response after about
// a minute wherever it is, so the text after the last newline is a partial
// record and is dropped, which the returned bool reports; no field in the feed
// holds a newline.
func parseBuylist(body []byte) ([]Card, bool, error) {
	// Discard a possible UTF-8 BOM before it trips up the csv parser
	body = bytes.TrimPrefix(body, []byte("\ufeff"))
	end := bytes.LastIndexByte(body, '\n')
	if end < 0 {
		return nil, false, errors.New("could not read csv header")
	}
	cut := end < len(body)-1

	reader := csv.NewReader(bytes.NewReader(body[:end+1]))

	header, err := reader.Read()
	if err != nil {
		return nil, false, fmt.Errorf("could not read csv header: %w", err)
	}
	column := map[string]int{}
	for i, name := range header {
		column[name] = i
	}

	var cards []Card
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, false, err
		}

		price, err := strconv.ParseFloat(record[column["unit_price_eur"]], 64)
		if err != nil {
			continue
		}
		limit, _ := strconv.Atoi(record[column["buy_limit"]])

		cards = append(cards, Card{
			Name:            record[column["card_name"]],
			SetCode:         record[column["set_code"]],
			CollectorNumber: record[column["collector_number"]],
			ScryfallID:      record[column["scryfall_id"]],
			TCGplayerID:     record[column["tcgplayer_id"]],
			Condition:       record[column["condition"]],
			Finish:          record[column["finish"]],
			PriceEUR:        price,
			BuyLimit:        limit,
			Language:        record[column["language"]],
			URL:             record[column["card_url"]],
		})
	}

	return cards, cut, nil
}
