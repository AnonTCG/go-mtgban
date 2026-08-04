package coolstuffinc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/hashicorp/go-cleanhttp"
)

type CSICard struct {
	Id             int     `json:"id,string"`
	URL            string  `json:"url"`
	Name           string  `json:"name"`
	ScryfallId     string  `json:"scryfallid"`
	Variation      string  `json:"variation"`
	Edition        string  `json:"edition"`
	Language       string  `json:"language"`
	IsFoil         bool    `json:"is_foil,string"`
	PriceRetail    float64 `json:"price_retail,string"`
	QuantityRetail int     `json:"qty_retail,string"`
	PriceBuy       float64 `json:"price_buy,string"`
	QuantityBuy    int     `json:"qty_buying,string"`
}

const (
	csiPricelistURL = "https://www.coolstuffinc.com/gateway_json.php?k="

	csiBuylistURL  = "https://www.coolstuffinc.com/GeneratedFiles/SellList/Section-%s.json"
	csiBuylistLink = "https://www.coolstuffinc.com/main_selllist.php?s="
)

type CSIClient struct {
	client *http.Client
	key    string
}

func NewCSIClient(key string) *CSIClient {
	csi := CSIClient{}
	csi.client = cleanhttp.DefaultClient()
	csi.key = key
	return &csi
}

func (csi *CSIClient) GetPriceList(ctx context.Context) ([]CSICard, error) {
	link := csiPricelistURL + csi.key
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := csi.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var pricelist struct {
		Meta struct {
			CreatedAt string `json:"created_at"`
		} `json:"meta"`
		Data []CSICard `json:"data"`
	}
	err = json.Unmarshal(data, &pricelist)
	if err != nil {
		return nil, err
	}

	return pricelist.Data, nil
}

type CSIPriceEntry struct {
	PID string `json:"PID"`
	// Ppqid         string `json:"PPQID"`
	Name string `json:"Name"`
	// Rarity        string `json:"Rarity"`
	ItemSet string `json:"ItemSet"`
	// Image   string `json:"Image"`
	Notes string `json:"Notes"`
	// SName         string `json:"sName"`
	// SAbbreviation string `json:"sAbbreviation"`
	Price string `json:"Price"`
	// TName         string `json:"tName"`
	// Color         string `json:"Color"`
	Number string `json:"Number"`
	// Code   string `json:"Code"`
	// BuyListNotes  string `json:"BuyListNotes"`
	// FullImage     struct {} `json:"FullImage"`
	RarityName  string `json:"RarityName"`
	IsFoil      int    `json:"isFoil"`
	CreditPrice string `json:"CreditPrice"`
}

func GetBuylist(ctx context.Context, game string) ([]CSIPriceEntry, error) {
	link := fmt.Sprintf(csiBuylistURL, game)

	// The sell list is a large uncompressed download that occasionally
	// truncates mid-stream (unexpected EOF), so retry the whole fetch.
	const attempts = 3
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		var entries []CSIPriceEntry
		entries, err = fetchBuylist(ctx, link)
		if err == nil {
			return entries, nil
		}
		time.Sleep(time.Duration(attempt) * time.Second)
	}

	return nil, err
}

func fetchBuylist(ctx context.Context, link string) ([]CSIPriceEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, http.NoBody)
	if err != nil {
		return nil, err
	}

	// Disable gzip compression
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := cleanhttp.DefaultClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("unexpected %d status code", resp.StatusCode)
	}

	var entries []CSIPriceEntry
	err = json.NewDecoder(resp.Body).Decode(&entries)
	if err != nil {
		return nil, err
	}

	return entries, nil
}

// Load the list of editions to id used to build links
func LoadBuylistEditions(ctx context.Context, game string) (map[string]string, error) {
	link := csiBuylistLink + game
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := cleanhttp.DefaultClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	edition2id := map[string]string{}

	doc.Find(`option`).Each(func(_ int, s *goquery.Selection) {
		ed := s.Text()
		if ed == "" {
			return
		}
		id, found := s.Attr("value")
		if !found || id == "" {
			return
		}
		_, found = edition2id[ed]
		if found {
			return
		}

		edition2id[ed] = id
	})

	return edition2id, nil
}

type SearchResult struct {
	PageId string
	Data   []byte
}

// Convert the item name to the id and the first page of results
func Search(ctx context.Context, game, itemName string, skipOOS bool) (*SearchResult, error) {
	v := url.Values{}
	v.Set("name", "")
	v.Set("f[Artist][]", "")
	v.Add("f[Cost][]", "")
	v.Add("f[Cost][]", "")
	v.Set("f[Number][]", "")
	v.Set("f[Type][]", "")
	v.Set("f[Card+Text][]", "")
	v.Set("notes", "")
	v.Set("sign-Cost", "<")
	v.Set("sign-Power", "<")
	v.Set("f[Power][]", "")
	v.Set("sign-Toughness", "<")
	v.Set("f[Toughness][]", "")
	v.Set("sign-Loyalty", "<")
	v.Set("f[Loyalty][]", "")
	v.Set("signprice", "<")
	v.Set("price", "")
	if skipOOS {
		// This excludes all cards that lack a NM copy
		v.Set("options[instock]", "1")
	}
	v.Add("f[Rarity][]", "C")
	v.Add("f[Rarity][]", "MR")
	v.Add("f[Rarity][]", "R")
	v.Add("f[Rarity][]", "U")
	v.Add("f[Rarity][]", "TC")
	v.Add("f[Rarity][]", "F")
	v.Add("f[Rarity][]", "PO")
	v.Set("f[ItemSet][]", itemName)
	v.Set("s", game)
	v.Set("page", "1")
	v.Set("resultsPerPage", "50")
	v.Set("submit", "Search")

	link := "https://www.coolstuffinc.com/sq/"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, link, strings.NewReader(v.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("User-Agent", "curl/8.6.0")

	resp, err := cleanhttp.DefaultClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	nextLink, _ := doc.Find(`span[id="nextLink"]`).Find("a").Attr("href")
	u, err := url.Parse(nextLink)
	if err != nil {
		return nil, err
	}

	clean := strings.Split(strings.TrimPrefix(u.Path, "/sq/"), "&")[0]

	return &SearchResult{
		PageId: clean,
		Data:   data,
	}, nil
}
