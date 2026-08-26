package starcitygames

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
)

const (
	scgInventoryURL = "https://starcitygamesv2.searchapi-na.hawksearch.com/api/v2/search"
	scgBuylistURL   = "https://search.starcitygames.com/indexes/sell_list_products_v2/search"
	scgBuylistSets  = "https://search.starcitygames.com/indexes/sets_v2/search"

	scgSettingsURL = "https://sellyourcards.starcitygames.com/api/settings"

	maxResultsPerPage = 300
	// Anything above this limit will be ignored and capped at 50
	buylistRequestLimit = 50

	GameMagic         = 1
	GameFleshAndBlood = 2
	GameLorcana       = 3
)

type SCGClient struct {
	client *http.Client
	guid   string
	bearer string

	SealedMode bool
}

func NewSCGClient(guid, bearer string) *SCGClient {
	scg := SCGClient{}
	cli := retryablehttp.NewClient()
	cli.Logger = nil
	cli.RetryMax = 10
	cli.RetryWaitMin = 2 * time.Second
	scg.client = cli.StandardClient()
	scg.guid = guid
	scg.bearer = bearer
	return &scg
}

// https://bridgeline.atlassian.net/wiki/spaces/HSKB/pages/3462479664/Hawksearch+v4.0+-+Search+API
type scgRetailRequest struct {
	Keyword           string              `json:"Keyword"`
	FacetSelections   map[string][]string `json:"FacetSelections"`
	PageNo            int                 `json:"PageNo"`
	MaxPerPage        int                 `json:"MaxPerPage"`
	PaginationSetCode string              `json:"PaginationSetCode"`
	ClientGUID        string              `json:"clientguid"`
}

type scgSealedFacetSelection struct {
	VariantInStockOnly []string `json:"variant_instockonly"`
	ProductType        []string `json:"product_type"`
	Game               string   `json:"game"`
}

type scgRetailSealedRequest struct {
	Keyword           string                  `json:"Keyword"`
	FacetSelections   scgSealedFacetSelection `json:"FacetSelections"`
	PageNo            int                     `json:"PageNo"`
	PaginationSetCode string                  `json:"PaginationSetCode"`
	MaxPerPage        int                     `json:"MaxPerPage"`
	ClientGUID        string                  `json:"clientguid"`
}

type scgRetailResponse struct {
	Pagination struct {
		NofResults  int `json:"NofResults"`
		CurrentPage int `json:"CurrentPage"`
		MaxPerPage  int `json:"MaxPerPage"`
		NofPages    int `json:"NofPages"`
	} `json:"Pagination"`
	Results []scgRetailResult `json:"Results"`
}

type scgRetailResult struct {
	Document struct {
		Subtitle            []string `json:"subtitle"`
		UniqueID            []int    `json:"unique_id"`
		CardName            []string `json:"card_name"`
		Language            []string `json:"language"`
		Set                 []string `json:"set"`
		CollectorNumber     []string `json:"collector_number"`
		Finish              []string `json:"finish"`
		ProductType         []string `json:"product_type"`
		URLDetail           []string `json:"url_detail"`
		ItemDisplayName     []string `json:"item_display_name"`
		HawkChildAttributes []struct {
			Price           []string `json:"price"`
			VariantSKU      []string `json:"variant_sku"`
			Qty             []int    `json:"qty"`
			VariantLanguage []string `json:"variant_language"`
			Condition       []string `json:"condition"`
		} `json:"hawk_child_attributes"`
	} `json:"Document"`
	IsVisible bool `json:"IsVisible"`
}

func (scg *SCGClient) sendRetailRequest(ctx context.Context, game, page int) (*scgRetailResponse, error) {
	gameStr := "Magic: The Gathering"
	switch game {
	case GameFleshAndBlood:
		gameStr = "Flesh and Blood"
	case GameLorcana:
		gameStr = "Lorcana"
	}

	var payload []byte
	var err error
	if scg.SealedMode {
		q := scgRetailSealedRequest{
			ClientGUID: scg.guid,
			MaxPerPage: maxResultsPerPage,
			PageNo:     page,
			FacetSelections: scgSealedFacetSelection{
				VariantInStockOnly: []string{"Yes"},
				ProductType:        []string{"Sealed"},
				Game:               gameStr,
			},
			PaginationSetCode: "previews",
		}
		payload, err = json.Marshal(&q)
	} else {
		q := scgRetailRequest{
			ClientGUID: scg.guid,
			MaxPerPage: maxResultsPerPage,
			PageNo:     page,
			FacetSelections: map[string][]string{
				"variant_instockonly": {"Yes"},
				"product_type":        {"Singles"},
				"game":                {gameStr},
			},
			PaginationSetCode: "previews",
		}
		payload, err = json.Marshal(&q)
	}
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, scgInventoryURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-HawkSearch-IgnoreTracking", "true")
	req.Header.Set("Content-Type", "application/json")

	resp, err := scg.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var search scgRetailResponse
	err = json.NewDecoder(resp.Body).Decode(&search)
	if err != nil {
		return nil, err
	}

	return &search, nil
}

func (scg *SCGClient) NumberOfPages(ctx context.Context, game int) (int, error) {
	response, err := scg.sendRetailRequest(ctx, game, 0)
	if err != nil {
		return 0, err
	}
	return response.Pagination.NofPages, nil
}

func (scg *SCGClient) GetPage(ctx context.Context, game, page int) ([]scgRetailResult, error) {
	response, err := scg.sendRetailRequest(ctx, game, page)
	if err != nil {
		return nil, err
	}
	return response.Results, nil
}

type BuylistRequest struct {
	Q                string   `json:"q"`
	Filter           string   `json:"filter"`
	MatchingStrategy string   `json:"matchingStrategy,omitempty"`
	Limit            int      `json:"limit"`
	Offset           int      `json:"offset,omitempty"`
	Sort             []string `json:"sort,omitempty"`
}

type BuylistResponse struct {
	Message            string `json:"message,omitempty"`
	Code               string `json:"code,omitempty"`
	Type               string `json:"type,omitempty"`
	Link               string `json:"link,omitempty"`
	Hits               []Hit  `json:"hits"`
	Query              string `json:"query"`
	ProcessingTimeMs   int    `json:"processingTimeMs"`
	Limit              int    `json:"limit"`
	Offset             int    `json:"offset"`
	EstimatedTotalHits int    `json:"estimatedTotalHits"`
}

type Hit struct {
	Name                string    `json:"name"`
	ID                  int       `json:"id"`
	Subtitle            string    `json:"subtitle"`
	ProductType         string    `json:"product_type"`
	Finish              any       `json:"finish"`
	FinishPricingTypeID int       `json:"finish_pricing_type_id"`
	CardStyleID         int       `json:"card_style_id"`
	Language            string    `json:"language"`
	Rarity              any       `json:"rarity"`
	IsBuying            int       `json:"is_buying"`
	Hotlist             int       `json:"hotlist"`
	BorderColor         string    `json:"border_color"`
	CollectorNumber     string    `json:"collector_number"`
	GameID              int       `json:"game_id"`
	SetID               int       `json:"set_id"`
	SetName             string    `json:"set_name"`
	SetReleaseDate      int       `json:"set_release_date"`
	SetSymbol           string    `json:"set_symbol"`
	Variants            []Variant `json:"variants"`
	Image               string    `json:"image"`
	WizardsCode         string    `json:"wizards_code"`
}

type Variant struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Subtitle     string  `json:"subtitle"`
	Sku          string  `json:"sku"`
	IsBuying     int     `json:"is_buying"`
	Hotlist      float64 `json:"hotlist"`
	VariantName  string  `json:"variant_name"`
	VariantValue string  `json:"variant_value"`
	BuyPrice     float64 `json:"buy_price"`
	TradePrice   float64 `json:"trade_price"`
}

type BuylistSettings struct {
	CardRarities []struct {
		ID               int       `json:"id"`
		Name             string    `json:"name"`
		GameID           int       `json:"game_id"`
		Abbr             string    `json:"abbr"`
		ExternalRarityID int       `json:"external_rarity_id"`
		SortOrder        int       `json:"sort_order"`
		CreatedAt        time.Time `json:"created_at"`
		UpdatedAt        time.Time `json:"updated_at"`
	} `json:"cardRarities"`
	CardFinishes []struct {
		ID               int       `json:"id"`
		Name             string    `json:"name"`
		GameID           int       `json:"game_id"`
		SkuCode          string    `json:"sku_code"`
		ExternalFinishID int       `json:"external_finish_id"`
		SortOrder        int       `json:"sort_order"`
		CreatedAt        time.Time `json:"created_at"`
		UpdatedAt        time.Time `json:"updated_at"`
	} `json:"cardFinishes"`
	CardStyles []struct {
		ID                  int       `json:"id"`
		Name                string    `json:"name"`
		GameID              int       `json:"game_id"`
		ExternalCardStyleID int       `json:"external_card_style_id"`
		CreatedAt           time.Time `json:"created_at"`
		UpdatedAt           time.Time `json:"updated_at"`
	} `json:"cardStyles"`
}

func SearchSettings(ctx context.Context) (*BuylistSettings, error) {
	link := scgSettingsURL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := cleanhttp.DefaultClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var search BuylistSettings
	err = json.NewDecoder(resp.Body).Decode(&search)
	if err != nil {
		return nil, err
	}

	return &search, nil
}

func (scg *SCGClient) SearchBuylistEditions(ctx context.Context) (*BuylistResponse, error) {
	filter := "source_type = set"
	limit := 1000

	q := BuylistRequest{
		Filter: filter,
		Limit:  limit,
	}
	payload, err := json.Marshal(&q)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, scgBuylistSets, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+scg.bearer)
	req.Header.Set("Content-Type", "application/json")

	resp, err := scg.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var search BuylistResponse
	err = json.NewDecoder(resp.Body).Decode(&search)
	if err != nil {
		return nil, err
	}

	if search.Message != "" {
		return nil, fmt.Errorf("%s", search.Message)
	}

	return &search, nil
}

func (scg *SCGClient) SearchAll(ctx context.Context, game, page, limit, setID int) (*BuylistResponse, error) {
	filter := `game_id = %d AND price_category_id = %s AND primary_status IN ["hotlist", "buying_at_cost"]`
	mode := "1"
	if scg.SealedMode {
		mode = "2"
	}
	query := fmt.Sprintf(filter, game, mode)

	if setID != 0 {
		query = fmt.Sprintf("%s AND set_id = %d", query, setID)
	}

	q := BuylistRequest{
		Filter:           query,
		MatchingStrategy: "all",
		Limit:            limit,
		Offset:           page * limit,
		Sort:             []string{"name:asc", "set_name:asc", "finish:desc"},
	}
	payload, err := json.Marshal(&q)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, scgBuylistURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+scg.bearer)
	req.Header.Set("Content-Type", "application/json")

	resp, err := scg.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var search BuylistResponse
	err = json.NewDecoder(resp.Body).Decode(&search)
	if err != nil {
		return nil, err
	}

	if search.Message != "" {
		return nil, fmt.Errorf("%s", search.Message)
	}

	return &search, nil
}

const (
	BaseProductURL    = "https://starcitygames.com"
	PartnerProductURL = "https://goto.starcitygames.com/c/%s/3052179/37198"
)

func SCGProductURL(URLDetail, variantSKU []string, affiliate string) string {
	if len(URLDetail) == 0 {
		return ""
	}

	link := BaseProductURL + URLDetail[0]
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}

	v := u.Query()
	if len(variantSKU) > 0 {
		v.Set("sku", variantSKU[0])
	}
	u.RawQuery = v.Encode()

	if affiliate == "" {
		return u.String()
	}

	q := url.Values{}
	q.Set("u", u.String())

	link = fmt.Sprintf(PartnerProductURL, affiliate)
	u, err = url.Parse(link)
	if err != nil {
		return ""
	}
	u.RawQuery = q.Encode()

	return u.String()
}
