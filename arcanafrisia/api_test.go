package arcanafrisia

import "testing"

const testHeader = `"card_name","set_code","collector_number","unit_price_eur","buy_limit","quantity_limits_json"` + "\n"

func TestParseBuylistCutFeed(t *testing.T) {
	rows := `"Rule of Law","mrd","19","0.75","2","[{""key"":""a"",""maxBuy"":2}]"` + "\n" +
		`"Arcane Signet","eld","331","0.46","6","[{""key"":""b"",""maxBuy"":6}]"` + "\n"
	cut := `"Sol Ring","c21","263","1.10","4","[{""key"":""c"",""ma`

	cards, wasCut, err := parseBuylist([]byte(testHeader + rows))
	if err != nil || len(cards) != 2 || wasCut {
		t.Fatalf("clean feed: got %d cards, cut %v, err %v", len(cards), wasCut, err)
	}

	cards, wasCut, err = parseBuylist([]byte(testHeader + rows + cut))
	if err != nil || len(cards) != 2 || !wasCut {
		t.Fatalf("cut feed: got %d cards, cut %v, err %v", len(cards), wasCut, err)
	}
}
