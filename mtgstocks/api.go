package mtgstocks

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mtgban/go-mtgban/mtgban"

	"github.com/hashicorp/go-retryablehttp"
)

// StocksInterest is one card whose price moved, with the old and new numbers.
type StocksInterest struct {
	InterestType string  `json:"interest_type"`
	Foil         bool    `json:"foil"`
	Percentage   float64 `json:"percentage"`
	PastPrice    float64 `json:"past_price"`
	PresentPrice float64 `json:"present_price"`
	Date         int64   `json:"date"`
	Print        struct {
		ID        int    `json:"id"`
		Slug      any    `json:"slug"` // string & int
		Name      string `json:"name"`
		Rarity    string `json:"rarity"`
		SetID     int    `json:"set_id"`
		SetName   string `json:"set_name"`
		IconClass string `json:"icon_class"`
		Reserved  bool   `json:"reserved"`
		SetType   string `json:"set_type"`
		Legal     struct {
			Frontier  string `json:"frontier"`
			Pauper    string `json:"pauper"`
			Pioneer   string `json:"pioneer"`
			Modern    string `json:"modern"`
			Standard  string `json:"standard"`
			Commander string `json:"commander"`
			Vintage   string `json:"vintage"`
			Legacy    string `json:"legacy"`
		}
		IncludeDefault bool   `json:"include_default"`
		Image          string `json:"image"`
	} `json:"print"`
}

// Interests is what the interests endpoint answers with.
type Interests struct {
	Error     string           `json:"error"`
	Date      string           `json:"date"`
	Interests []StocksInterest `json:"interests"`
}

const (
	stksAverageURL = "https://api.mtgstocks.com/interests/average"
	stksMarketURL  = "https://api.mtgstocks.com/interests/market"
	stksSetsURL    = "https://api.mtgstocks.com/card_sets"

	stksUserAgent = "curl/8.5.0"
)

// STKSClient reads the MTGStocks API.
type STKSClient struct {
	client *http.Client
}

// NewClient returns a client.
func NewClient() *STKSClient {
	stks := STKSClient{}
	stks.client = mtgban.NewHTTPClient(
		mtgban.WithHTTPBackoff(retryablehttp.LinearJitterBackoff),
		mtgban.WithHTTPRetryWait(2*time.Second, 10*time.Second),
		mtgban.WithHTTPRetries(10),
		mtgban.WithHTTPCheckRetry(customCheckRetry),
	)
	return &stks
}

// Implement our own retry policy to leverage the internal retry mechanism
func customCheckRetry(ctx context.Context, resp *http.Response, err error) (bool, error) {
	// A transport error leaves no response to inspect, so let the default
	// policy decide before reaching for the body
	if err != nil || resp == nil {
		return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
	}

	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	if strings.ToLower(resp.Header.Get("Content-Encoding")) == "gzip" {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return false, err
		}
		defer gzipReader.Close()
		reader = gzipReader
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return false, err
	}

	resp.Body = io.NopCloser(bytes.NewBuffer(data))
	if strings.Contains(string(data), "HTML") || strings.Contains(string(data), "ERROR") {
		return true, errors.New(string(data))
	}
	return false, nil
}

// AverageInterests returns the movers measured against the average price.
func (s *STKSClient) AverageInterests(ctx context.Context, foil bool) ([]StocksInterest, error) {
	out, err := s.query(ctx, stksAverageURL, foil)
	if err != nil {
		return nil, err
	}
	return out.Interests, nil
}

// MarketInterests returns the movers measured against the market price.
func (s *STKSClient) MarketInterests(ctx context.Context, foil bool) ([]StocksInterest, error) {
	out, err := s.query(ctx, stksMarketURL, foil)
	if err != nil {
		return nil, err
	}
	return out.Interests, nil
}

func (s *STKSClient) query(ctx context.Context, link string, foil bool) (*Interests, error) {
	extra := "/regular"
	if foil {
		extra = "/foil"
	}
	link += extra

	req, err := http.NewRequest("GET", link, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", stksUserAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", "https://www.mtgstocks.com/")
	req.Header.Set("Origin", "https://www.mtgstocks.com")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, identity")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("DNT", "1")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	if strings.ToLower(resp.Header.Get("Content-Encoding")) == "gzip" {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	var interests Interests
	err = json.NewDecoder(reader).Decode(&interests)
	if err != nil {
		return nil, err
	}

	if interests.Error != "" {
		return nil, errors.New(interests.Error)
	}

	return &interests, nil
}
