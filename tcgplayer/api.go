package tcgplayer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
	"github.com/mtgban/go-tcgplayer"
)

const (
	defaultConcurrency = 8

	minimumDirectPrice = 0.4

	tcgLatestSalesURL = "https://mpapi.tcgplayer.com/v2/product/%s/latestsales"

	staticUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36"
)

// GetProductNumber returns the collector number the catalog carries for the
// product, normalized for matching.
func GetProductNumber(tcgp *tcgplayer.Product) string {
	num := RawProductNumber(tcgp)
	num = strings.TrimLeft(num, "0")
	num = strings.Split(num, "/")[0]
	return strings.TrimSpace(num)
}

// RawProductNumber returns the collector number exactly as the catalog spells
// it, where GetProductNumber normalizes.
func RawProductNumber(tcgp *tcgplayer.Product) string {
	for _, extData := range tcgp.ExtendedData {
		if extData.Name == "Number" {
			return extData.Value
		}
	}
	return ""
}

// GetProductNameAndVariant splits a catalog product into the card name and the
// qualifiers TCGplayer appends to it.
func GetProductNameAndVariant(tcgp *tcgplayer.Product) (string, string) {
	cardName := tcgp.Name
	variant := ""

	if strings.Contains(cardName, " - ") {
		fields := strings.Split(cardName, " - ")
		cardName = fields[0]
		if len(fields) > 1 {
			variant = strings.Join(fields[1:], " ")
		}
	}
	if strings.Contains(cardName, " [") {
		cardName = strings.Replace(cardName, "[", "(", -1)
		cardName = strings.Replace(cardName, "]", ")", -1)
	}
	if strings.Contains(cardName, " (") {
		fields := matcher.SplitVariants(cardName)
		cardName = fields[0]
		if len(fields) > 1 {
			if variant != "" {
				variant += " "
			}
			variant += strings.Join(fields[1:], " ")

			variant = strings.TrimSuffix(variant, " CE")
			variant = strings.TrimSuffix(variant, " IE")
		}
	}

	variant = strings.Replace(variant, "(", "", -1)
	variant = strings.Replace(variant, ")", "", -1)

	return cardName, variant
}

func isToken(tcgp *tcgplayer.Product) bool {
	for _, extData := range tcgp.ExtendedData {
		if extData.Name == "SubType" && strings.Contains(extData.Value, "Token") {
			return true
		}
	}
	// There are some tokens not marked as such
	if strings.Contains(tcgp.CleanName, "Token") {
		return true
	}
	return false
}

// EditionMap indexes a category's groups, which is what TCGplayer calls an
// edition, by their id. A page answering short, or a group repeated in place
// of another, fails it rather than leaving the products of a missing
// edition with nothing to match against.
func EditionMap(ctx context.Context, tcg *tcgplayer.Client, category int) (map[int]tcgplayer.Group, error) {
	results := map[int]tcgplayer.Group{}
	for group, err := range tcg.Groups(ctx, category) {
		if err != nil {
			return nil, err
		}
		results[group.GroupID] = group
	}
	return results, nil
}

// genericChan is one inventory entry a page produced, for the card it is
// filed under, and the buylist entry it makes beside it, if any.
type genericChan struct {
	key   string
	entry mtgban.InventoryEntry
	bl    *mtgban.BuylistEntry
}

// loadPages hands every page offset of total to process across concurrency
// workers and fails when any page did. WorkerPool only logs a worker's
// error, which would leave an inventory missing whole pages looking loaded.
func loadPages(ctx context.Context, concurrency, total int, process func(context.Context, int, chan<- genericChan) error, consume func(genericChan), logf func(string, ...any)) error {
	pageNums := make([]int, 0, total/tcgplayer.MaxItemsInResponse+1)
	for i := 0; i < total; i += tcgplayer.MaxItemsInResponse {
		pageNums = append(pageNums, i)
	}

	var failed atomic.Int64
	mtgban.WorkerPool(ctx, concurrency, pageNums,
		func(ctx context.Context, page int, channel chan<- genericChan) error {
			err := process(ctx, page, channel)
			if err != nil {
				failed.Add(1)
			}
			return err
		},
		consume,
		logf,
	)
	// A cancelled walk stops dispatching, and the pages it never sent
	// are not counted as failed
	err := ctx.Err()
	if err != nil {
		return err
	}
	if n := failed.Load(); n > 0 {
		return fmt.Errorf("%d of %d pages failed, so the inventory is incomplete", n, len(pageNums))
	}
	return nil
}

// SKUConditionMap maps the condition ids the APIs use to the grades the
// records are keyed by.
var SKUConditionMap = map[int]mtgban.Condition{
	1: mtgban.NM,
	2: mtgban.SP,
	3: mtgban.MP,
	4: mtgban.HP,
	5: mtgban.PO,
}

// SKUConditionUnopened is the condition sealed products are sold under,
// deliberately absent from the map above since it never applies to a single.
const SKUConditionUnopened = 6

type latestSalesRequest struct {
	Variants    []int  `json:"variants"`
	Conditions  []int  `json:"conditions"`
	Languages   []int  `json:"languages"`
	ListingType string `json:"listingType"`
	Limit       int    `json:"limit"`
}

type latestSalesResponse struct {
	PreviousPage string            `json:"previousPage"`
	NextPage     string            `json:"nextPage"`
	ResultCount  int               `json:"resultCount"`
	TotalResults int               `json:"totalResults"`
	Data         []LatestSalesData `json:"data"`
}

// LatestSalesData is one recent sale of a product, as the storefront reports
// it.
type LatestSalesData struct {
	Condition       string    `json:"condition"`
	Variant         string    `json:"variant"`
	Language        string    `json:"language"`
	Quantity        int       `json:"quantity"`
	Title           string    `json:"title"`
	ListingType     string    `json:"listingType"`
	CustomListingID string    `json:"customListingId"`
	PurchasePrice   float64   `json:"purchasePrice"`
	ShippingPrice   float64   `json:"shippingPrice"`
	OrderDate       time.Time `json:"orderDate"`
}

const (
	defaultListingTypeLatestSales = "All"
	defaultLimitLastestSales      = 25
)

// LatestSales returns the recent sales recorded for a product. The paging
// envelope the endpoint wraps them in carries nothing a caller has asked for,
// so it stays inside.
func LatestSales(ctx context.Context, tcgProductID string, flags ...bool) ([]LatestSalesData, error) {
	link := fmt.Sprintf(tcgLatestSalesURL, tcgProductID)

	var params latestSalesRequest
	params.ListingType = defaultListingTypeLatestSales
	params.Limit = defaultLimitLastestSales

	if len(flags) > 0 {
		foil := flags[0]
		if foil {
			params.Variants = []int{2, 133, 141}
		} else {
			params.Variants = []int{1, 132}
		}
	}

	anyLang := len(flags) > 1 && flags[1]
	if !anyLang {
		// 1 being English
		params.Languages = []int{1}
	}

	payload, err := json.Marshal(&params)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, link, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", staticUA)
	req.Header.Set("Content-Type", "application/json")

	resp, err := cleanhttp.DefaultClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var response latestSalesResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	return response.Data, nil
}

// The storefront endpoints SellerClient reads. They are not part of the
// partner API and need no credentials, but they answer only what the site
// itself shows.
const (
	SellersPageURL     = "https://mpapi.tcgplayer.com/v2/ShopBySeller/GetSellerSearchResults"
	SellerInventoryURL = "https://mp-search-api.tcgplayer.com/v1/search/request?q=&isList=true&mpfev=1953"
	SellerListingURL   = "https://mp-search-api.tcgplayer.com/v1/product/%d/listings"

	DefaultSellerRequestSize = 50
)

// SellerKeyExists reports whether the storefront still serves a page for the
// seller key.
func SellerKeyExists(ctx context.Context, sellerKey string) bool {
	// Do not follow redirects
	client := mtgban.NewHTTPClient(mtgban.WithHTTPCheckRedirect(func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}))

	link := "https://shop.tcgplayer.com/sellerfeedback/" + sellerKey
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, http.NoBody)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200
}

// SellerSearchResponse is what the shop-by-seller endpoint answers with.
type SellerSearchResponse struct {
	Errors  []any `json:"errors"`
	Results []struct {
		StartRecordIndex int `json:"startRecordIndex"`
		EndRecordIndex   int `json:"endRecordIndex"`
		TotalItemCount   int `json:"totalItemCount"`
		SearchResults    []struct {
			SellerID             int      `json:"sellerId"`
			DisplayName          string   `json:"displayName"`
			SellerKey            string   `json:"sellerKey"`
			IsLive               bool     `json:"isLive"`
			IsDirect             bool     `json:"isDirect"`
			IsGoldStar           bool     `json:"isGoldStar"`
			IsCertifiedHobbyShop bool     `json:"isCertifiedHobbyShop"`
			Rating               float64  `json:"rating"`
			CompletedSalesRank   string   `json:"completedSalesRank"`
			Location             string   `json:"location"`
			ProductLines         []string `json:"productLines"`
		} `json:"searchResults"`
	} `json:"results"`
}

// SellerName2ID resolves a seller's display name to the key the APIs index
// them by.
func SellerName2ID(ctx context.Context, sellerName string) (string, error) {
	if sellerName == "" {
		return "", errors.New("missing seller name")
	}

	requestPayload := struct {
		SellerName string `json:"sellerName"`
		Page       int    `json:"page"`
	}{
		SellerName: sellerName,
		Page:       1,
	}
	payload, err := json.Marshal(requestPayload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, SellersPageURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := cleanhttp.DefaultClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var response SellerSearchResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return "", fmt.Errorf("unmarshal error: %w", err)
	}

	if len(response.Errors) != 0 {
		return "", fmt.Errorf("errors in response: %v", response.Errors)
	}

	var sellerKey string
	for _, results := range response.Results {
		for _, result := range results.SearchResults {
			if sellerName != result.DisplayName {
				continue
			}
			sellerKey = result.SellerKey
			break
		}
	}
	if sellerKey == "" {
		return "", fmt.Errorf("seller not found in %v", response)
	}

	return sellerKey, nil
}

type sellerInventoryRequest struct {
	Algorithm string `json:"algorithm"`
	From      int    `json:"from"`
	Size      int    `json:"size"`
	Filters   struct {
		Term struct {
			ProductLineName []string `json:"productLineName,omitempty"`
			ProductTypeName []string `json:"productTypeName,omitempty"`
			SetName         []string `json:"setName,omitempty"`
		} `json:"term"`
		Range struct {
		} `json:"range"`
		Match struct {
		} `json:"match"`
	} `json:"filters"`
	ListingSearch struct {
		Filters struct {
			Term struct {
				SellerStatus  string   `json:"sellerStatus,omitempty"`
				ChannelID     int      `json:"channelId"`
				DirectSeller  bool     `json:"direct-seller,omitempty"`
				DirectProduct bool     `json:"directProduct,omitempty"`
				SellerKey     []string `json:"sellerKey,omitempty"`
				Printing      []string `json:"printing,omitempty"`
			} `json:"term"`
			Range struct {
				Quantity struct {
					GreaterThanOrEqual int `json:"gte"`
				} `json:"quantity"`
				DirectInventory struct {
					GreaterThanOrEqual int `json:"gte"`
				} `json:"directInventory"`
			} `json:"range"`
			Exclude struct {
				ChannelExclusion int `json:"channelExclusion"`
			} `json:"exclude"`
		} `json:"filters"`
		Context struct {
			Cart struct {
			} `json:"cart"`
		} `json:"context"`
	} `json:"listingSearch"`
	Context struct {
		Cart struct {
		} `json:"cart"`
		ShippingCountry string `json:"shippingCountry"`
	} `json:"context"`
	Settings struct {
		UseFuzzySearch bool `json:"useFuzzySearch"`
	} `json:"settings"`
	Sort struct {
		Field string `json:"field"`
		Order string `json:"order"`
	} `json:"sort"`
}

type sellerInventoryResponse struct {
	Title  string `json:"title"`
	Status int    `json:"status"`

	Results []SellerInventoryPage `json:"results"`
}

// SellerInventoryPage is one page of a seller's inventory, as the endpoint
// returns it: alongside the aggregations describing the whole query.
type SellerInventoryPage struct {
	Aggregations struct {
		SetName []struct {
			URLValue string  `json:"urlValue"`
			IsActive bool    `json:"isActive"`
			Value    string  `json:"value"`
			Count    float64 `json:"count"`
		} `json:"setName"`
	} `json:"aggregations"`
	TotalResults int                     `json:"totalResults"`
	ResultID     string                  `json:"resultId"`
	Algorithm    string                  `json:"algorithm"`
	SearchType   string                  `json:"searchType"`
	Results      []SellerInventoryResult `json:"results"`
}

// SellerListing is one seller's offer on a product, with the price, condition
// and quantity they published.
type SellerListing struct {
	ChannelID            float64 `json:"channelId"`
	Condition            string  `json:"condition"`
	ConditionID          float64 `json:"conditionId"`
	DirectInventory      float64 `json:"directInventory"`
	DirectProduct        bool    `json:"directProduct"`
	DirectSeller         bool    `json:"directSeller"`
	ForwardFreight       bool    `json:"forwardFreight"`
	GoldSeller           bool    `json:"goldSeller"`
	Language             string  `json:"language"`
	LanguageAbbreviation string  `json:"languageAbbreviation"`
	LanguageID           float64 `json:"languageId"`
	ListingID            float64 `json:"listingId"`
	ListingType          string  `json:"listingType"`
	Price                float64 `json:"price"`
	Printing             string  `json:"printing"`
	ProductConditionID   float64 `json:"productConditionId"`
	ProductID            float64 `json:"productId"`
	Quantity             float64 `json:"quantity"`
	RankedShippingPrice  float64 `json:"rankedShippingPrice"`
	Score                float64 `json:"score"`
	SellerID             string  `json:"sellerId"`
	SellerKey            string  `json:"sellerKey"`
	SellerName           string  `json:"sellerName"`
	SellerRating         float64 `json:"sellerRating"`
	SellerSales          string  `json:"sellerSales"`
	SellerShippingPrice  float64 `json:"sellerShippingPrice"`
	ShippingPrice        float64 `json:"shippingPrice"`
	VerifiedSeller       bool    `json:"verifiedSeller"`
}

// SellerInventoryResult is one product in a seller's inventory, carrying every
// listing they have for it.
type SellerInventoryResult struct {
	FoilOnly                bool    `json:"foilOnly"`
	ImageCount              float64 `json:"imageCount"`
	LowestPrice             float64 `json:"lowestPrice"`
	LowestPriceWithShipping float64 `json:"lowestPriceWithShipping"`
	MarketPrice             float64 `json:"marketPrice"`
	MaxFulfillableQuantity  float64 `json:"maxFulfillableQuantity"`
	NormalOnly              bool    `json:"normalOnly"`
	ProductID               float64 `json:"productId"`
	ProductLineID           float64 `json:"productLineId"`
	ProductLineName         string  `json:"productLineName"`
	ProductLineURLName      string  `json:"productLineUrlName"`
	ProductName             string  `json:"productName"`
	ProductStatusID         float64 `json:"productStatusId"`
	ProductTypeID           float64 `json:"productTypeId"`
	ProductTypeName         string  `json:"productTypeName"`
	ProductURLName          string  `json:"productUrlName"`
	RarityName              string  `json:"rarityName"`
	Score                   float64 `json:"score"`
	Sealed                  bool    `json:"sealed"`
	SellerListable          bool    `json:"sellerListable"`
	Sellers                 float64 `json:"sellers"`
	SetCode                 string  `json:"setCode"`
	SetID                   float64 `json:"setId"`
	SetName                 string  `json:"setName"`
	SetURLName              string  `json:"setUrlName"`
	ShippingCategoryID      float64 `json:"shippingCategoryId"`
	TotalListings           float64 `json:"totalListings"`
	CustomAttributes        struct {
		Number string `json:"number"`
	} `json:"customAttributes"`
	Listings []SellerListing `json:"listings"`
}

// SellerClient reads the storefront's search API, which needs no partner
// credentials but answers only what the site itself shows.
type SellerClient struct {
	client *http.Client
}

// NewSellerClient returns a client for the storefront's search API.
func NewSellerClient() *SellerClient {
	tcg := SellerClient{}
	tcg.client = mtgban.NewHTTPClient()
	return &tcg
}

// InventoryForSeller returns the pages the endpoint answered with. It only
// ever fills one, but the shape is its own, so callers index it rather than
// have the choice made for them; the slice is never empty on a nil error.
func (tcg *SellerClient) InventoryForSeller(ctx context.Context, sellerKeys []string, size, page int, useDirect bool, finishes []string, sets ...string) ([]SellerInventoryPage, error) {
	var params sellerInventoryRequest
	params.Algorithm = "revenue_synonym_v2"
	params.From = size * page
	params.Size = size
	params.Filters.Term.ProductLineName = []string{"magic"}
	params.Filters.Term.ProductTypeName = []string{"Cards"}
	params.Filters.Term.SetName = sets
	params.ListingSearch.Filters.Term.SellerStatus = "Live"
	params.ListingSearch.Filters.Term.SellerKey = sellerKeys
	params.ListingSearch.Filters.Term.Printing = finishes
	if useDirect {
		params.ListingSearch.Filters.Term.DirectProduct = true
		params.ListingSearch.Filters.Term.DirectSeller = true
		params.ListingSearch.Filters.Range.DirectInventory.GreaterThanOrEqual = 1
	}
	params.ListingSearch.Filters.Range.Quantity.GreaterThanOrEqual = 1
	params.Context.ShippingCountry = "US"
	params.Settings.UseFuzzySearch = true
	params.Sort.Field = "product-sorting-name"
	params.Sort.Order = "asc"

	payload, err := json.Marshal(&params)
	if err != nil {
		return nil, err
	}

	link := SellerInventoryURL
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, link, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := tcg.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var response sellerInventoryResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	if response.Title != "" {
		return nil, fmt.Errorf("error: %s (%d)", response.Title, response.Status)
	}
	if len(response.Results) == 0 {
		return nil, fmt.Errorf("empty results in response at page %d", page)
	}

	return response.Results, nil
}

type sellerInventoryListingRequest struct {
	Filters struct {
		Term struct {
			SellerStatus  string   `json:"sellerStatus"`
			ChannelID     int      `json:"channelId"`
			DirectSeller  bool     `json:"direct-seller,omitempty"`
			DirectProduct bool     `json:"directProduct,omitempty"`
			Language      []string `json:"language,omitempty"`
		} `json:"term"`
		Range struct {
			Quantity struct {
				Gte int `json:"gte"`
			} `json:"quantity"`
			DirectInventory struct {
				Gte int `json:"gte,omitempty"`
			} `json:"directInventory"`
		} `json:"range"`
		Exclude struct {
			ChannelExclusion int `json:"channelExclusion"`
		} `json:"exclude"`
	} `json:"filters"`
	Context struct {
		ShippingCountry string `json:"shippingCountry"`
		Cart            struct {
		} `json:"cart"`
	} `json:"context"`
	Aggregations []string `json:"aggregations"`
	From         int      `json:"from"`
	Size         int      `json:"size"`
	Sort         struct {
		Field string `json:"field"`
		Order string `json:"order"`
	} `json:"sort"`
}

type sellerInventoryListingResponse struct {
	Errors []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Results []struct {
		TotalResults int             `json:"totalResults"`
		Results      []SellerListing `json:"results"`
	} `json:"results"`
}

// InventoryListing returns one page of the offers on a single product, across
// every seller rather than for one of them.
func (tcg *SellerClient) InventoryListing(ctx context.Context, productID, size, page int, useDirect bool) ([]SellerListing, error) {
	var params sellerInventoryListingRequest
	params.Filters.Term.SellerStatus = "Live"
	if useDirect {
		params.Filters.Term.SellerStatus = "Live"
		params.Filters.Term.DirectSeller = true
		params.Filters.Term.DirectProduct = true
		params.Filters.Term.Language = []string{"English"}
		params.Filters.Range.DirectInventory.Gte = 1
	}
	params.Filters.Range.Quantity.Gte = 1
	params.Context.ShippingCountry = "US"
	params.Aggregations = []string{"listingType"}
	params.From = size * page
	params.Size = size
	params.Sort.Field = "price"
	params.Sort.Order = "asc"

	payload, err := json.Marshal(&params)
	if err != nil {
		return nil, err
	}

	link := fmt.Sprintf(SellerListingURL, productID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, link, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", staticUA)

	resp, err := tcg.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var response sellerInventoryListingResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}
	if len(response.Errors) > 0 {
		return nil, fmt.Errorf("%s: %s", response.Errors[0].Code, response.Errors[0].Message)
	}
	if len(response.Results) == 0 {
		return nil, fmt.Errorf("empty results in response at page %d", page)
	}

	return response.Results[0].Results, nil
}

// CookieClient drives the storefront as a signed-in user would, for the
// operations the APIs do not expose: the cart, the address book, the account.
type CookieClient struct {
	client     *http.Client
	cookieLine string
}

// NewCookieClient returns a client acting as the user whose auth cookie is
// given.
func NewCookieClient(authKey string) *CookieClient {
	tcg := CookieClient{}
	tcg.cookieLine = "TCGAuthTicket_Production=" + authKey + ";"
	tcg.client = mtgban.NewHTTPClient(mtgban.WithHTTPWritesOnce())
	return &tcg
}

// NewCookieSetClient returns a client carrying a whole cookie jar, for the
// pages that need more than the auth cookie alone.
func NewCookieSetClient(cookies map[string]string) *CookieClient {
	tcg := CookieClient{}
	for name, value := range cookies {
		tcg.cookieLine += fmt.Sprintf("%s=%s; ", name, value)
	}
	tcg.client = mtgban.NewHTTPClient(mtgban.WithHTTPWritesOnce())
	return &tcg
}

// UserData is the signed-in account the storefront reports.
type UserData struct {
	UserName                string `json:"userName"`
	UserID                  int    `json:"userId"`
	UserKey                 string `json:"userKey"`
	IsSubscriber            bool   `json:"isSubscriber"`
	LastUsedShippingAddress struct {
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		Address1  string `json:"address1"`
		Address2  string `json:"address2"`
		City      string `json:"city"`
		State     string `json:"state"`
		Zipcode   string `json:"zipcode"`
		Country   string `json:"country"`
		Phone     string `json:"phone"`
	} `json:"lastUsedShippingAddress"`
	ShippingCountry     string `json:"shippingCountry"`
	CreatedAt           string `json:"createdAt"`
	ExternalUserID      string `json:"externalUserId"`
	IsGuest             any    `json:"isGuest"`
	IsValidated         bool   `json:"isValidated"`
	CartKey             string `json:"cartKey"`
	SaveForLaterKey     string `json:"saveForLaterKey"`
	ProductLineAffinity any    `json:"productLineAffinity"`
	Traits              struct {
		ProductLineAffinityMostViewed any `json:"product_line_affinity_most_viewed"`
		Apv                           int `json:"apv"`
	} `json:"traits"`
	SellerKeys []string `json:"sellerKeys"`
}

// UserResponse wraps UserData as the endpoint returns it.
type UserResponse struct {
	Errors []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Results []UserData `json:"results"`
}

const tcgUserDataURL = "https://mpapi.tcgplayer.com/v2/user?isGuest=false"

// Get performs a signed-in GET.
func (tcg *CookieClient) Get(ctx context.Context, link string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", tcg.cookieLine)
	req.Header.Set("User-Agent", "curl/8.6.0")

	return tcg.client.Do(req)
}

// Post performs a signed-in POST.
func (tcg *CookieClient) Post(ctx context.Context, link, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, link, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", tcg.cookieLine)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "curl/8.6.0")

	return tcg.client.Do(req)
}

// Delete performs a signed-in DELETE.
func (tcg *CookieClient) Delete(ctx context.Context, link string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, link, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", tcg.cookieLine)
	req.Header.Set("User-Agent", "curl/8.6.0")

	return tcg.client.Do(req)
}

// GetUserData returns the account the cookie belongs to.
func (tcg *CookieClient) GetUserData(ctx context.Context) (*UserData, error) {
	resp, err := tcg.Get(ctx, tcgUserDataURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var response UserResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}
	if len(response.Errors) > 0 {
		return nil, fmt.Errorf("%s: %s", response.Errors[0].Code, response.Errors[0].Message)
	}
	if len(response.Results) == 0 {
		return nil, errors.New("empty results in user request")
	}
	if response.Results[0].UserName == "" {
		return nil, errors.New("empty username from user request")
	}

	return &response.Results[0], nil
}

const tcgCreateCartURL = "https://mpgateway.tcgplayer.com/v1/cart/create/usercart"

// CreateCartKey opens a cart for the user and returns the key that identifies
// it.
func CreateCartKey(ctx context.Context, userID string) (string, error) {
	var params struct {
		ExternalUserID string `json:"externalUserId"`
	}
	params.ExternalUserID = userID

	payload, err := json.Marshal(&params)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tcgCreateCartURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := cleanhttp.DefaultClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var response UserResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return "", fmt.Errorf("unmarshal error: %w", err)
	}
	if len(response.Errors) > 0 {
		return "", fmt.Errorf("%s: %s", response.Errors[0].Code, response.Errors[0].Message)
	}
	if len(response.Results) == 0 {
		return "", errors.New("empty results in user request")
	}

	return response.Results[0].CartKey, nil
}

// EmptyCart removes everything from the cart.
func (tcg *CookieClient) EmptyCart(ctx context.Context, cartKey string) error {
	link := fmt.Sprintf("https://mpgateway.tcgplayer.com/v1/cart/%s/items/all", cartKey)

	resp, err := tcg.Delete(ctx, link)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// AddressConfig is a shipping address, in the shape the storefront's account
// pages expect.
type AddressConfig struct {
	FirstName                 string `json:"firstName"`
	LastName                  string `json:"lastName"`
	AddressLine1              string `json:"addressLine1"`
	AddressLine2              string `json:"addressLine2"`
	City                      string `json:"city"`
	ZipCode                   string `json:"zipCode"`
	StateProvinceRegion       string `json:"stateProvinceRegion"`
	Phone                     string `json:"phone"`
	IsDefaultAddress          bool   `json:"isDefaultAddress"`
	SaveAddressOnPaymentSave  bool   `json:"saveAddressOnPaymentSave"`
	ExternalUserID            string `json:"externalUserId"`
	CountryCode               string `json:"countryCode"`
	ID                        int    `json:"id"`
	EasyPostShippingAddressID string `json:"easyPostShippingAddressId"`
	IsEasyPostVerified        bool   `json:"isEasyPostVerified"`
	CreatedAt                 string `json:"createdAt"`
	LastUsedAt                string `json:"lastUsedAt"`
}

const (
	addressUpdateURL = "https://mpgateway.tcgplayer.com/v2/useraddressbooks/update"
)

// SetAddress replaces the account's shipping address.
func (tcg *CookieClient) SetAddress(ctx context.Context, address AddressConfig) error {
	payload, err := json.Marshal(&address)
	if err != nil {
		return err
	}

	resp, err := tcg.Post(ctx, addressUpdateURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New(http.StatusText(resp.StatusCode))
	}

	return nil
}
