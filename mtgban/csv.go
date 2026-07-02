package mtgban

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mtgban/go-mtgban/mtgmatcher"
)

var (
	// The base Card fields for the canonical headers
	CardHeader = []string{
		"UUID", "Name", "Edition", "Finish", "Number", "Rarity",
	}

	// The canonical header that will be present in all inventory files
	InventoryHeader = append(CardHeader, "Conditions", "Price", "Quantity", "URL")

	// The canonical header that will be present in all market files
	MarketHeader = append(InventoryHeader, "Seller", "Bundle")

	// Additional fields for Markets neecessary for Carters
	CartHeader = append(MarketHeader, "Original Id", "Instance Id")

	// The canonical header that will be present in all buylist files
	BuylistHeader = append(CardHeader, "Conditions", "Buy Price", "Trade Price", "Quantity", "Price Ratio", "URL", "Vendor")

	ArbitHeader = append(CardHeader, "Conditions", "Available", "Sell Price", "Buy Price", "Difference", "Spread", "Abs Difference", "Profitability", "Buy Link", "Sell Link")

	MismatchHeader = append(CardHeader, "Conditions", "Price", "Reference", "Difference", "Spread")
)

func record2entry(record []string) (*InventoryEntry, error) {
	index := len(CardHeader)
	cardId := record[0]
	_, err := mtgmatcher.GetUUID(cardId)
	if err != nil && !strings.Contains(cardId, "|") {
		return nil, fmt.Errorf("error reading record: %v (%v)", err, record)
	}

	conditions := record[index]
	index++
	price, err := strconv.ParseFloat(record[index], 64)
	if err != nil {
		return nil, fmt.Errorf("error reading record: %v", err)
	}
	index++
	qty, err := strconv.Atoi(record[index])
	if err != nil {
		return nil, fmt.Errorf("error reading record: %v", err)
	}
	index++

	URL := record[index]
	index++

	sellerName := ""
	if len(record) > index {
		sellerName = record[index]
		index++
	}
	bundle := false
	if len(record) > index {
		bundle = record[index] == "Y"
		index++
	}
	ogId := ""
	if len(record) > index {
		ogId = record[index]
		index++
	}
	instanceId := ""
	if len(record) > index {
		instanceId = record[index]
		index++
	}

	return &InventoryEntry{
		Conditions: conditions,
		Price:      price,
		Quantity:   qty,
		URL:        URL,
		SellerName: sellerName,
		Bundle:     bundle,
		OriginalId: ogId,
		InstanceId: instanceId,
	}, nil
}

func LoadInventoryFromCSV(r io.Reader, flags ...bool) (InventoryRecord, error) {
	strict := true
	if len(flags) > 0 {
		strict = flags[0]
	}
	csvReader := csv.NewReader(r)
	csvReader.ReuseRecord = true

	first, err := csvReader.Read()
	if err == io.EOF {
		return InventoryRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error reading header: %v", err)
	}

	okHeader := true
	if len(first) < len(InventoryHeader) {
		okHeader = false
	} else {
		// Assume a normal header, then check what's the last element,
		// and adjust accordingly to what is detected
		header := InventoryHeader
		if first[len(first)-1] == MarketHeader[len(MarketHeader)-1] {
			header = MarketHeader
		} else if first[len(first)-1] == CartHeader[len(CartHeader)-1] {
			header = CartHeader
		}
		for i, tag := range header {
			if tag != first[i] {
				okHeader = false
				break
			}
		}
	}
	if !okHeader {
		return nil, fmt.Errorf("malformed inventory file")
	}

	inventory := InventoryRecord{}
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			if strict {
				return nil, fmt.Errorf("error reading record: %v", err)
			}
			continue
		}

		entry, err := record2entry(record)
		if err != nil {
			if strict {
				return nil, err
			}
			continue
		}

		inventory.Add(record[0], entry)
	}

	return inventory, nil
}

func LoadBuylistFromCSV(r io.Reader, flags ...bool) (BuylistRecord, error) {
	strict := true
	if len(flags) > 0 {
		strict = flags[0]
	}
	csvReader := csv.NewReader(r)
	csvReader.ReuseRecord = true

	first, err := csvReader.Read()
	if err == io.EOF {
		return BuylistRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error reading header: %v", err)
	}

	okHeader := true
	if len(first) < len(BuylistHeader) {
		okHeader = false
	} else {
		for i, tag := range BuylistHeader {
			if tag != first[i] {
				okHeader = false
				break
			}
		}
	}
	if !okHeader {
		return nil, fmt.Errorf("malformed buylist file")
	}

	buylist := BuylistRecord{}
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			if strict {
				return nil, fmt.Errorf("error reading record: %v", err)
			}
			continue
		}

		index := len(CardHeader)
		cardId := record[0]
		_, err = mtgmatcher.GetUUID(cardId)
		if err != nil {
			if strict {
				return nil, fmt.Errorf("error reading record: %v (%v)", err, record)
			}
			continue
		}

		cond := record[index]
		index++

		buyPrice, err := strconv.ParseFloat(record[index], 64)
		if err != nil {
			if strict {
				return nil, fmt.Errorf("error reading record %s: %v", record[index], err)
			}
			continue
		}
		index++

		// Skip TradePrice
		index++

		qty, err := strconv.Atoi(record[index])
		if err != nil {
			if strict {
				return nil, fmt.Errorf("error reading record %s: %v", record[index], err)
			}
			continue
		}
		index++
		priceRatio, err := strconv.ParseFloat(strings.TrimSuffix(record[index], "%"), 64)
		if err != nil {
			if strict {
				return nil, fmt.Errorf("error reading record %s: %v", record[index], err)
			}
			continue
		}
		index++

		URL := record[index]
		index++

		vendorName := ""
		if len(record) > index {
			vendorName = record[index]
			index++
		}

		entry := &BuylistEntry{
			Conditions: cond,
			BuyPrice:   buyPrice,
			Quantity:   qty,
			PriceRatio: priceRatio,
			URL:        URL,
			VendorName: vendorName,
		}

		buylist.Add(cardId, entry)
	}

	return buylist, nil
}

func cardId2record(cardId string) ([]string, error) {
	if strings.Contains(cardId, "|") {
		fields := strings.Split(cardId, "|")
		if len(fields) != 4 {
			return nil, fmt.Errorf("unsupported id format %s", cardId)
		}
		record := []string{
			cardId,
			fields[1],
			fields[2],
			fields[3],
			"",
			"",
		}
		return record, nil
	}

	co, err := mtgmatcher.GetUUID(cardId)
	if err != nil {
		return nil, err
	}

	finish := "nonfoil"
	if co.Sealed {
		finish = "sealed"
	} else if co.Foil {
		finish = "foil"
	} else if co.Etched {
		finish = "etched"
	}

	record := []string{
		cardId,
		co.Card.Name,
		co.Edition,
		finish,
		co.Card.Number,
		co.Card.Rarity,
	}
	return record, nil
}

func WriteInventoryToCSV(inventory InventoryRecord, w io.Writer) error {
	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	header := InventoryHeader
	for _, entries := range inventory {
		if entries[0].SellerName != "" {
			header = MarketHeader
		}
		if entries[0].OriginalId != "" || entries[0].InstanceId != "" {
			header = CartHeader
		}
		break
	}

	err := csvWriter.Write(header)
	if err != nil {
		return err
	}

	for cardId, entries := range inventory {
		cardHeader, err := cardId2record(cardId)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			record := append(cardHeader,
				entry.Conditions,
				fmt.Sprintf("%0.2f", entry.Price),
				fmt.Sprint(entry.Quantity),
				entry.URL,
			)
			if len(header) >= len(MarketHeader) {
				record = append(record, entry.SellerName)
				bundle := ""
				if entry.Bundle {
					bundle = "Y"
				}
				record = append(record, bundle)

				if len(header) >= len(CartHeader) {
					record = append(record, entry.OriginalId)
					record = append(record, entry.InstanceId)
				}
			}

			err = csvWriter.Write(record)
			if err != nil {
				return err
			}
		}
		csvWriter.Flush()
	}

	return nil
}

func WriteBuylistToCSV(buylist BuylistRecord, creditMuliplier float64, w io.Writer) error {
	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	err := csvWriter.Write(BuylistHeader)
	if err != nil {
		return err
	}

	for cardId, entries := range buylist {
		for _, entry := range entries {
			record, err := cardId2record(cardId)
			if err != nil {
				continue
			}

			record = append(record,
				entry.Conditions,
				fmt.Sprintf("%0.2f", entry.BuyPrice),
				fmt.Sprintf("%0.2f", entry.BuyPrice*creditMuliplier),
				fmt.Sprint(entry.Quantity),
				fmt.Sprintf("%0.2f", entry.PriceRatio),
				entry.URL,
				entry.VendorName,
			)

			err = csvWriter.Write(record)
			if err != nil {
				return err
			}
			csvWriter.Flush()
		}
	}

	return nil
}

func WriteArbitrageToCSV(arbitrage []ArbitEntry, w io.Writer) error {
	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	hasExtraSeller := false
	header := ArbitHeader
	if len(arbitrage) > 0 && arbitrage[0].InventoryEntry.SellerName != "" {
		header = append(header, "Seller")
		header = append(header, "Bundle")
		hasExtraSeller = true
	}
	err := csvWriter.Write(header)
	if err != nil {
		return err
	}

	for _, entry := range arbitrage {
		bl := entry.BuylistEntry
		inv := entry.InventoryEntry

		record, err := cardId2record(entry.CardId)
		if err != nil {
			continue
		}

		record = append(record,
			inv.Conditions,
			fmt.Sprintf("%d", inv.Quantity),
			fmt.Sprintf("%0.2f", inv.Price),
			fmt.Sprintf("%0.2f", bl.BuyPrice),
			fmt.Sprintf("%0.2f", entry.Difference),
			fmt.Sprintf("%0.2f", entry.Spread),
			fmt.Sprintf("%0.2f", entry.AbsoluteDifference),
			fmt.Sprintf("%0.2f", entry.Profitability),
			inv.URL,
			bl.URL,
		)
		if hasExtraSeller {
			record = append(record, inv.SellerName)
			bundle := ""
			if inv.Bundle {
				bundle = "Y"
			}
			record = append(record, bundle)
		}
		err = csvWriter.Write(record)
		if err != nil {
			return err
		}

		csvWriter.Flush()
	}

	return nil
}

func WriteMismatchToCSV(mismatch []ArbitEntry, w io.Writer) error {
	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	hasExtraSeller := false
	header := MismatchHeader
	if len(mismatch) > 0 && mismatch[0].InventoryEntry.SellerName != "" {
		header = append(header, "Seller")
		header = append(header, "Bundle")
		hasExtraSeller = true
	}

	err := csvWriter.Write(header)
	if err != nil {
		return err
	}

	for _, entry := range mismatch {
		inv := entry.InventoryEntry
		ref := entry.ReferenceEntry

		record, err := cardId2record(entry.CardId)
		if err != nil {
			continue
		}

		record = append(record,
			inv.Conditions,
			fmt.Sprintf("%0.2f", inv.Price),
			fmt.Sprintf("%0.2f", ref.Price),
			fmt.Sprintf("%0.2f", entry.Difference),
			fmt.Sprintf("%0.2f", entry.Spread),
		)
		if hasExtraSeller {
			record = append(record, inv.SellerName)
			bundle := ""
			if inv.Bundle {
				bundle = "Y"
			}
			record = append(record, bundle)
		}
		err = csvWriter.Write(record)
		if err != nil {
			return err
		}

		csvWriter.Flush()
	}

	return nil
}

func WriteCombineToCSV(root *CombineRoot, w io.Writer) error {
	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	header := append(CardHeader, root.Names...)
	err := csvWriter.Write(header)
	if err != nil {
		return err
	}

	for cardId, entries := range root.Entries {
		out, err := cardId2record(cardId)
		if err != nil {
			continue
		}
		for _, vendorName := range root.Names {
			out = append(out, fmt.Sprintf("%0.2f", entries[vendorName].Price))
		}

		err = csvWriter.Write(out)
		if err != nil {
			return err
		}

		csvWriter.Flush()
	}

	return nil
}

func WritePennyToCSV(penny []ArbitEntry, w io.Writer) error {
	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	hasExtraSeller := false
	header := InventoryHeader
	if len(penny) > 0 && penny[0].InventoryEntry.SellerName != "" {
		header = append(header, "Seller")
		header = append(header, "Bundle")
		hasExtraSeller = true
	}
	err := csvWriter.Write(header)
	if err != nil {
		return err
	}

	for _, entry := range penny {
		inv := entry.InventoryEntry

		record, err := cardId2record(entry.CardId)
		if err != nil {
			continue
		}

		record = append(record,
			inv.Conditions,
			fmt.Sprintf("%0.2f", inv.Price),
			fmt.Sprintf("%d", inv.Quantity),
			"",
		)
		if hasExtraSeller {
			record = append(record, inv.SellerName)
			bundle := ""
			if inv.Bundle {
				bundle = "Y"
			}
			record = append(record, bundle)
		}
		err = csvWriter.Write(record)
		if err != nil {
			return err
		}

		csvWriter.Flush()
	}

	return nil
}
