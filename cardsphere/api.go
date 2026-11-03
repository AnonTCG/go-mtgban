package cardsphere

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"github.com/mtgban/go-mtgban/mtgban"

	"github.com/hashicorp/go-retryablehttp"
)

// Client reads the Cardsphere API.
type Client struct {
	client *http.Client
}

// NewClient returns a client using the given token.
func NewClient(token string) *Client {
	cs := Client{}
	// The api is very sensitive to multiple concurrent requests,
	// This backoff strategy lets the system chill out a bit before retrying
	cs.client = mtgban.NewHTTPClient(
		mtgban.WithHTTPBackoff(retryablehttp.LinearJitterBackoff),
		mtgban.WithHTTPRetryWait(2*time.Second, 10*time.Second),
		mtgban.WithHTTPRetries(20),
	)

	jar, _ := cookiejar.New(nil)

	var cookies []*http.Cookie
	cookie := &http.Cookie{
		Name:   "cardsphere-session-5",
		Value:  token,
		Path:   "/",
		Domain: ".cardsphere.com",
	}
	cookies = append(cookies, cookie)

	u, _ := url.Parse(csURL)
	u.RawQuery = ""
	jar.SetCookies(u, cookies)

	cs.client.Jar = jar
	return &cs
}

// OfferList is one page of standing offers.
type OfferList struct {
	WantID      int     `json:"wantId"`
	MinOffer    int     `json:"minOffer"`
	MaxOffer    int     `json:"maxOffer"`
	MinIndex    int     `json:"minIndex"`
	MaxIndex    int     `json:"maxIndex"`
	MinEff      int     `json:"minEff"`
	MaxEff      int     `json:"maxEff"`
	MinRelEff   float64 `json:"minRelEff"`
	MaxRelEff   float64 `json:"maxRelEff"`
	MasterID    int     `json:"masterId"`
	Image       string  `json:"image"`
	UserID      int     `json:"userId"`
	UserDisplay string  `json:"userDisplay"`
	Country     string  `json:"country"`
	CountryName string  `json:"countryName"`
	Balance     int     `json:"balance"`
	CardName    string  `json:"cardName"`
	Kind        string  `json:"kind"`
	Sets        []struct {
		Code   string `json:"code"`
		Name   string `json:"name"`
		Rarity string `json:"rarity"`
	} `json:"sets"`
	Languages  []string `json:"languages"`
	Conditions []int    `json:"conditions"`
	Finishes   []string `json:"finishes"`
	Quantity   int      `json:"quantity"`
}

const csURL = "https://www.cardsphere.com/rest/v1/offers?offset=0&order=minrel&absge=50&country=USMIL,UM,US,CA&kind=S&language=EN"

// GetOfferList returns one page of offers, starting at the given offset.
func (cs *Client) GetOfferList(ctx context.Context, offset int) ([]OfferList, error) {
	u, err := url.Parse(csURL)
	if err != nil {
		return nil, err
	}
	v := u.Query()
	v.Set("offset", fmt.Sprint(offset))
	u.RawQuery = v.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "curl/8.6.0")

	resp, err := cs.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var pricelist []OfferList
	err = json.NewDecoder(resp.Body).Decode(&pricelist)
	if err != nil {
		return nil, err
	}

	return pricelist, nil
}
