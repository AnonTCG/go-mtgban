package tcgplayer

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/mtgban/go-mtgban/mtgmatcher"
)

// The storefront links a listing is published under. The partner form wraps a
// product link for affiliate attribution.
const (
	BaseProductURL    = "https://www.tcgplayer.com/product/"
	// AnonTCG: our affiliate id is baked in (upstream keeps a %s placeholder
	// filled from the affiliate arg).
	PartnerProductURL = "https://partner.tcgplayer.com/c/6226515/1830156/21018"
)

// GenerateProductURL builds the storefront link for a product, narrowed to a
// printing, condition and language, and carrying an affiliate tag when one is
// given.
func GenerateProductURL(productID int, printing, affiliate, condition, language string, isDirect bool) string {
	u, err := url.Parse(BaseProductURL + fmt.Sprint(productID))
	if err != nil {
		return ""
	}

	v := u.Query()
	if printing != "" {
		v.Set("Printing", printing)
	}
	if condition != "" {
		for full, short := range conditionMap {
			if short == condition {
				condition = full
				break
			}
		}
		v.Set("Condition", condition)
	}
	if language != "" {
		language = mtgmatcher.Title(language)
		switch language {
		case "Portuguese (Brazil)":
			language = "Portuguese"
		case "Chinese Simplified":
			language = "Chinese (S)"
		case "Chinese Traditional":
			language = "Chinese (T)"
		}
		v.Set("Language", language)
	} else {
		v.Set("Language", "all")
	}
	v.Set("direct", "false")
	if isDirect {
		v.Set("direct", "true")
	}

	// This chunk needs to be last, stash the built link in a query param
	// and use the impact URL instead
	if affiliate != "" {
		u.RawQuery = v.Encode()
		link := u.String()

		// AnonTCG: our affiliate id is baked into PartnerProductURL (see the
		// const above), so parse it directly. The affiliate arg still gates
		// whether we wrap the link in the impact URL at all.
		u, err = url.Parse(PartnerProductURL)
		if err != nil {
			return ""
		}

		v = url.Values{}
		v.Set("u", link)
	}

	u.RawQuery = v.Encode()

	return u.String()
}

// TCGSku is one sellable variant of a product: a printing in a condition and a
// language, which is the unit TCGplayer prices.
type TCGSku struct {
	Condition string `json:"condition"`
	Language  string `json:"language"`
	Printing  string `json:"printing"`
	Finish    string `json:"finish"`
	ProductID int    `json:"productId"`
	SkuID     int    `json:"skuId"`
}

// SKUMap indexes every sku by the uuid of the printing it belongs to, so a
// scraper can go from a matched card to the ids the APIs want.
type SKUMap map[string][]TCGSku

// LoadTCGSKUs reads the sku catalog, which is published as a file rather than
// served, and indexes it by uuid.
func LoadTCGSKUs(reader io.Reader) (SKUMap, error) {
	var payload struct {
		Data map[string][]TCGSku `json:"data"`
	}
	err := json.NewDecoder(reader).Decode(&payload)
	if err != nil {
		return nil, err
	}
	if len(payload.Data) == 0 {
		return nil, errors.New("empty SKU file")
	}
	return payload.Data, nil
}

const (
	// SYPCSVURL serves the Sell Your Playset list as a CSV
	SYPCSVURL = "https://store.tcgplayer.com/admin/direct/ExportSYPList?categoryid=1&setNameId=All&conditionId=All"
)

// TCGSYP is one entry of the Sell Your Playset list.
type TCGSYP struct {
	SkuID       int
	MarketPrice float64
	MaxQty      int
}

// LoadSyp downloads the Sell Your Playset list as a CSV.
func LoadSyp(ctx context.Context, auth string) ([]TCGSYP, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, SYPCSVURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", "TCGAuthTicket_Production="+auth)

	resp, err := cleanhttp.DefaultClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	csvReader := csv.NewReader(resp.Body)
	csvReader.ReuseRecord = true

	var result []TCGSYP
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			// A malformed record is worth skipping, but this reader wraps a
			// live response body: a truncated download or a reset connection
			// is returned again on every call, and skipping it never ends
			var parseErr *csv.ParseError
			if errors.As(err, &parseErr) {
				continue
			}
			return nil, err
		}

		if len(record) < 9 {
			continue
		}

		id, err := strconv.Atoi(record[0])
		if err != nil {
			continue
		}
		price, err := strconv.ParseFloat(record[7], 64)
		if err != nil {
			continue
		}
		qty, err := strconv.Atoi(record[8])
		if err != nil {
			continue
		}

		result = append(result, TCGSYP{
			SkuID:       id,
			MarketPrice: price,
			MaxQty:      qty,
		})
	}

	if len(result) == 0 {
		return nil, errors.New("empty syp csv")
	}

	return result, nil
}

// DirectPriceAfterFees returns the per-item net a seller is credited under
// TCGplayer Direct's item-based fee model (effective 2026-06-18), which
// replaced the prior order-based SRC model. Accurate for non-Pro Direct
// offers, with no taxes applied.
//
//	item >= $2.50:  $1.12 + 8.95% commission (capped at $75) + 2.5% transaction
//	item <  $2.50:  50% of item price (commission + transaction waived)
func DirectPriceAfterFees(price float64) float64 {
	var fee float64
	if price >= 2.50 {
		fee = 1.12 + math.Min(75.0, price*0.0895) + price*0.025
	} else {
		fee = price * 0.50
	}
	return price - fee
}

// DirectSYPPriceAfterFees returns what a Direct SYP sale actually pays, once
// TCGplayer's commission and fees come out.
func DirectSYPPriceAfterFees(price float64) float64 {
	var fee float64
	if price < 2 {
		fee = price * 0.50
	} else {
		fee = 0.6 + math.Min(75.0, price*0.0895) + price*0.025
	}
	return price - fee
}

const (
	defaultListingSize = 20
)

// ListingData is one live listing of a product, with the quantity behind it.
type ListingData struct {
	ProductID       int     `json:"product_id"`
	SkuID           int     `json:"sku_id"`
	Quantity        int     `json:"quantity"`
	SellerKey       string  `json:"seller_key"`
	Price           float64 `json:"price"`
	DirectInventory int     `json:"direct_inventory"`
	ConditionFull   string  `json:"condition_full"`
	Condition       string  `json:"condition"`
	Printing        string  `json:"printing"`
	Foil            bool    `json:"foil"`
}

// GetDirectQtysForProductID returns the live listings for a product, optionally
// only the Direct ones.
func GetDirectQtysForProductID(ctx context.Context, productID int, onlyDirect bool) []ListingData {
	client := NewSellerClient()

	var result []ListingData
	for i := 0; ; i++ {
		listings, err := client.InventoryListing(ctx, productID, defaultListingSize, i, onlyDirect)
		if err != nil || len(listings) == 0 {
			break
		}

		for _, listing := range listings {
			if !listing.DirectProduct || !listing.DirectSeller {
				continue
			}

			result = append(result, ListingData{
				ProductID:       productID,
				SkuID:           int(listing.ProductConditionID),
				Quantity:        int(listing.Quantity),
				SellerKey:       listing.SellerKey,
				Price:           listing.Price,
				DirectInventory: int(listing.DirectInventory),
				ConditionFull:   listing.Condition,
				Condition:       conditionMap[listing.Condition],
				Printing:        listing.Printing,
				Foil:            listing.Printing != "Normal",
			})
		}
	}

	return result
}

func getDirectPrice(price float64) float64 {
	if price == 0 {
		return 0
	}
	return math.Max(price, minimumDirectPrice)
}
