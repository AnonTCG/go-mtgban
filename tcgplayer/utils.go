package tcgplayer

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
)

// The storefront links a listing is published under. The partner form wraps a
// product link for affiliate attribution.
const (
	BaseProductURL    = "https://www.tcgplayer.com/product/"
	PartnerProductURL = "https://partner.tcgplayer.com/c/%s/1830156/21018"
)

// conditionNames spells our grade the way TCGplayer's storefront names it in
// its own product-link query parameter.
var conditionNames = map[mtgban.Condition]string{
	mtgban.NM: "Near Mint",
	mtgban.SP: "Lightly Played",
	mtgban.MP: "Moderately Played",
	mtgban.HP: "Heavily Played",
	mtgban.PO: "Damaged",
}

// tcgLanguageName spells a language the way TCGplayer's catalog and links
// do, which differs from the datastore's for three of them.
func tcgLanguageName(language string) string {
	language = matcher.Title(language)
	switch language {
	case "Portuguese (Brazil)":
		return "Portuguese"
	case "Chinese Simplified":
		return "Chinese (S)"
	case "Chinese Traditional":
		return "Chinese (T)"
	}
	return language
}

// GenerateProductURL builds the storefront link for a product, narrowed to a
// printing, condition and language, and carrying an affiliate tag when one is
// given.
func GenerateProductURL(productID int, printing, affiliate string, condition mtgban.Condition, language string, isDirect bool) string {
	u, err := url.Parse(BaseProductURL + fmt.Sprint(productID))
	if err != nil {
		return ""
	}

	v := u.Query()
	if printing != "" {
		v.Set("Printing", printing)
	}
	if name, found := conditionNames[condition]; found {
		v.Set("Condition", name)
	}
	if language != "" {
		v.Set("Language", tcgLanguageName(language))
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

		u, err = url.Parse(fmt.Sprintf(PartnerProductURL, affiliate))
		if err != nil {
			return ""
		}

		v = url.Values{}
		v.Set("u", link)
	}

	u.RawQuery = v.Encode()

	return u.String()
}

// ErrSYPNotAuthenticated is returned when the store answers the SYP export
// with its logon page rather than the list: the TCGAuthTicket_Production
// cookie in TCGPLAYER_AUTH is expired or missing.
var ErrSYPNotAuthenticated = errors.New("syp export not authenticated: TCGPLAYER_AUTH is expired or missing")

const (
	// SYPCSVURL serves the Store Your Products list as a CSV
	SYPCSVURL = "https://store.tcgplayer.com/admin/direct/ExportSYPList"
)

// TCGSYP is one entry of the Store Your Products list.
type TCGSYP struct {
	SkuID       int
	MarketPrice float64
	MaxQty      int
}

// LoadSYP downloads the Store Your Products list as a CSV.
func LoadSYP(ctx context.Context, category int, auth string) ([]TCGSYP, error) {
	return loadSYPFrom(ctx, SYPCSVURL, category, auth)
}

// loadSYPFrom is LoadSYP against any address, so the store's answers can be
// stood in for.
func loadSYPFrom(ctx context.Context, address string, category int, auth string) ([]TCGSYP, error) {
	u, err := url.Parse(address)
	if err != nil {
		return nil, err
	}

	v := url.Values{}
	v.Set("categoryid", fmt.Sprint(category))
	v.Set("setNameId", "All")
	v.Set("conditionId", "All")
	u.RawQuery = v.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", "TCGAuthTicket_Production="+auth)

	resp, err := mtgban.NewHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// A ticket the store no longer honours is answered with a redirect to
	// the logon page, which the client follows. The CSV reader finds no
	// record in that page, so an expired ticket would read as an empty
	// list; the page the response ended on tells the two apart.
	if strings.Contains(resp.Request.URL.Path, "/account/logon") {
		return nil, ErrSYPNotAuthenticated
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("syp export: unexpected status code %d", resp.StatusCode)
	}

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
	ProductID       int              `json:"product_id"`
	SkuID           int              `json:"sku_id"`
	Quantity        int              `json:"quantity"`
	SellerKey       string           `json:"seller_key"`
	Price           float64          `json:"price"`
	DirectInventory int              `json:"direct_inventory"`
	ConditionFull   string           `json:"condition_full"`
	Condition       mtgban.Condition `json:"condition"`
	Printing        string           `json:"printing"`
	Foil            bool             `json:"foil"`
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

			cond, _ := mtgban.ParseCondition(listing.Condition)
			result = append(result, ListingData{
				ProductID:       productID,
				SkuID:           int(listing.ProductConditionID),
				Quantity:        int(listing.Quantity),
				SellerKey:       listing.SellerKey,
				Price:           listing.Price,
				DirectInventory: int(listing.DirectInventory),
				ConditionFull:   listing.Condition,
				Condition:       cond,
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
