package abugames

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/hashicorp/go-retryablehttp"
)

type ABUCard struct {
	Id           string `json:"id"`
	DisplayTitle string `json:"display_title"`
	SimpleTitle  string `json:"simple_title"`

	CompleteDescription string `json:"complete_description"`

	Edition   string `json:"magic_edition_sort"`
	Condition string `json:"condition"`
	Layout    string `json:"layout"`

	Rarity   string   `json:"rarity"`
	Language []string `json:"language"`
	Title    string   `json:"title"`
	Number   string   `json:"card_number"`

	SellPrice       float64 `json:"price"`
	SellQuantity    int     `json:"quantity"`
	SubSellQuantity int     `json:"sub_qty"`
	BuyQuantity     int     `json:"buy_list_quantity"`
	BuyPrice        float64 `json:"buy_price"`
	TradePrice      float64 `json:"trade_price"`
}

type ABUProduct struct {
	Grouped struct {
		ProductId struct {
			Count  int `json:"ngroups"`
			Groups []struct {
				GroupValue string `json:"groupValue"`
				Doclist    struct {
					Docs []ABUCard `json:"docs"`
				} `json:"doclist"`
			} `json:"groups"`
		} `json:"product_id"`
	} `json:"grouped"`
}

type ABUResponse struct {
	Response struct {
		NumFound      int       `json:"numFound"`
		Start         int       `json:"start"`
		NumFoundExact bool      `json:"numFoundExact"`
		Docs          []ABUCard `json:"docs"`
	} `json:"response"`
}

const (
	maxEntryPerRequest = 1000

	// abuFieldList restricts Solr responses to the fields ABUCard decodes.
	// Without it each doc returns all ~56 stored fields (oracle text, ebay
	// metadata, images, ...), inflating the page payload ~9x.
	abuFieldList = "id,display_title,simple_title,complete_description," +
		"magic_edition_sort,condition,layout,rarity,language,title," +
		"card_number,price,quantity,sub_qty,buy_list_quantity," +
		"buy_price,trade_price"

	abuBaseUrl = `https://data.abugames.com/solr/nodes/select?q=*:*&group=true&group.field=product_id&group.limit=10&start=0&rows=0&wt=json&fq=%2Bcategory%3A%22Magic%20the%20Gathering%20Singles%22%20%2Blanguage%3A(%22English%22%20OR%20%22Italian%22%20OR%20%22Japanese%22%20OR%20%22Phyrexian%22)%20-offline_item%3Atrue%20-magic_features%3A(%22Actual%20Picture%20Card%22)`

	// This URL will include pics, but queries will be slower
	abuBaseUrlFull = `https://data.abugames.com/solr/nodes/select?q=*:*&group=true&group.field=product_id&group.limit=10&start=0&rows=0&wt=json&fq=%2Bcategory%3A%22Magic%20the%20Gathering%20Singles%22%20%2Blanguage%3A(%22English%22%20OR%20%22Italian%22%20OR%20%22Japanese%22%20OR%20%22Phyrexian%22)%20-offline_item%3Atrue%20`

	abuBaseSealedUrl = `https://data.abugames.com/solr/nodes/select?q=*:*&fq=%2Bcategory%3A%22Magic%20the%20Gathering%20Sealed%20Product%22%20-offline_item%3Atrue%20OR%20-title%3A%22STORE%22%20OR%20-title%3A%22AUCTION%22%20OR%20-title%3A%22OVERSTOCK%22%20%2Blanguage_magic_sealed_product%3A(%22English%22)&sort=display_title%20asc&wt=json&start=0&rows=0`
)

type ABUClient struct {
	client        *http.Client
	authorization string
}

func NewABUClient() *ABUClient {
	abu := ABUClient{}
	client := retryablehttp.NewClient()
	client.Logger = nil
	abu.client = client.StandardClient()
	return &abu
}

func NewABUClientWithBearer(token string) *ABUClient {
	abu := NewABUClient()
	abu.authorization = token
	return abu
}

func (abu *ABUClient) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}

	if abu.authorization != "" {
		req.Header.Set("Authorization", "Bearer "+abu.authorization)
	}

	return abu.client.Do(req)
}

func (abu *ABUClient) Post(ctx context.Context, url, contentType string, reader io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, reader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)
	if abu.authorization != "" {
		req.Header.Set("Authorization", "Bearer "+abu.authorization)
	}

	return abu.client.Do(req)
}

func (abu *ABUClient) sendRequest(ctx context.Context, url string) (*ABUProduct, error) {
	resp, err := abu.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var product ABUProduct
	err = json.NewDecoder(resp.Body).Decode(&product)
	if err != nil {
		return nil, err
	}

	return &product, nil
}

func (abu *ABUClient) sendSealedRequest(ctx context.Context, url string) (*ABUResponse, error) {
	resp, err := abu.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var response ABUResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// GetTotalItems fetches the number of product groups. group.ngroups (the total
// group count) is expensive for Solr to compute — a full enumeration of all
// ~213k groups — so it is requested only here, not baked into the base URL used
// for page fetches.
func (abu *ABUClient) GetTotalItems(ctx context.Context, extra string) (int, error) {
	link := abuBaseUrl
	if extra != "" {
		link = abuBaseUrlFull + url.QueryEscape(extra)
	}
	u, err := url.Parse(link)
	if err != nil {
		return 0, err
	}
	q := u.Query()
	q.Set("group.ngroups", "true")
	u.RawQuery = q.Encode()

	product, err := abu.sendRequest(ctx, u.String())
	if err != nil {
		return 0, err
	}
	return product.Grouped.ProductId.Count, nil
}

func (abu *ABUClient) GetTotalSealedItems(ctx context.Context) (int, error) {
	response, err := abu.sendSealedRequest(ctx, abuBaseSealedUrl)
	if err != nil {
		return 0, err
	}
	return response.Response.NumFound, nil
}

func (abu *ABUClient) GetProduct(ctx context.Context, extra string, pageStart int) (*ABUProduct, error) {
	link := abuBaseUrl
	if extra != "" {
		link = abuBaseUrlFull + url.QueryEscape(extra)
	}
	u, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Set("rows", fmt.Sprintf("%d", maxEntryPerRequest))
	q.Set("start", fmt.Sprintf("%d", pageStart))
	// fl trims each doc to the fields ABUCard decodes (~9x smaller payload).
	// group.ngroups is deliberately absent from the base URL — only the count
	// path (GetTotalItems) requests it, since per page it would make Solr
	// enumerate all ~213k groups for nothing.
	q.Set("fl", abuFieldList)
	u.RawQuery = q.Encode()

	return abu.sendRequest(ctx, u.String())
}

func (abu *ABUClient) GetSealedProduct(ctx context.Context, pageStart int) (*ABUResponse, error) {
	u, err := url.Parse(abuBaseSealedUrl)
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Set("rows", fmt.Sprintf("%d", maxEntryPerRequest))
	q.Set("start", fmt.Sprintf("%d", pageStart))
	u.RawQuery = q.Encode()

	return abu.sendSealedRequest(ctx, u.String())
}

type CartRequest struct {
	ItemId   string `json:"item_id"`
	Quantity int    `json:"quantity"`
	// Ignored on buylist
	Call string `json:"call,omitempty"`
}

type CartResponse struct {
	BuyList string `json:"buyList"`
	NqData  struct {
		Maxqty int `json:"maxqty"`
	} `json:"nqData"`
	ConditionRowID int `json:"condition_row_id"`
	Resp           struct {
		Exception any   `json:"exception"`
		Headers   []any `json:"headers"`
		Original  any   `json:"original"`
	} `json:"resp"`

	Message    string `json:"message"`
	Code       string `json:"code"`
	StatusCode int    `json:"status_code"`
}

const (
	abuInventoryAddURL = "https://api.abugames.com/cart/item"
	abuBuylistAddURL   = "https://api.abugames.com/buy-list-cart/item"
)

func (abu *ABUClient) SetCartInventory(ctx context.Context, abuId string, qty int) (*CartResponse, error) {
	return abu.setCart(ctx, abuInventoryAddURL, abuId, qty)
}

func (abu *ABUClient) SetCartBuylist(ctx context.Context, abuId string, qty int) (*CartResponse, error) {
	return abu.setCart(ctx, abuBuylistAddURL, abuId, qty)
}

func (abu *ABUClient) setCart(ctx context.Context, link, abuId string, qty int) (*CartResponse, error) {
	payload := CartRequest{
		ItemId:   abuId,
		Quantity: qty,
		Call:     "add",
	}

	reqBody, err := json.Marshal(&payload)
	if err != nil {
		return nil, err
	}

	resp, err := abu.Post(ctx, link, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var response CartResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, err
	}

	if len(response.Message) > 0 {
		return nil, errors.New(response.Message)
	}

	return &response, nil
}
