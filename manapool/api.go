package manapool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/go-cleanhttp"
)

// Product is a single priced item from a manapool price list. It covers both
// endpoints: the singles-only fields (Number, ScryfallID, ConditionID, FinishID)
// are empty for sealed products, which the sealed feed doesn't provide.
// ProductType distinguishes the two (e.g. "mtg_sealed").
type Product struct {
	URL                string `json:"url"`
	ProductType        string `json:"product_type"`
	ProductID          string `json:"product_id"`
	SetCode            string `json:"set_code"`
	Number             string `json:"number"`
	Name               string `json:"name"`
	ScryfallID         string `json:"scryfall_id"`
	TcgplayerProductID int    `json:"tcgplayer_product_id"`
	LanguageID         string `json:"language_id"`
	ConditionID        string `json:"condition_id"`
	FinishID           string `json:"finish_id"`
	LowPrice           int    `json:"low_price"`

	// Total quantity across listings
	AvailableQuantity int `json:"available_quantity"`
}

const (
	manapoolURL = "https://manapool.com/api/v1/prices/variants"
	sealedURL   = "https://manapool.com/api/v1/prices/sealed"
)

func GetPriceList(ctx context.Context) ([]Product, error) {
	return getList(ctx, manapoolURL)
}

func GetSealedList(ctx context.Context) ([]Product, error) {
	return getList(ctx, sealedURL)
}

func getList(ctx context.Context, link string) ([]Product, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := cleanhttp.DefaultClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var pricelist struct {
		Meta struct {
			AsOf time.Time `json:"as_of"`
		} `json:"meta"`
		Data []Product `json:"data"`
	}
	err = json.NewDecoder(resp.Body).Decode(&pricelist)
	if err != nil {
		return nil, fmt.Errorf("unmarshal error for list, got: %w", err)
	}

	return pricelist.Data, nil
}
