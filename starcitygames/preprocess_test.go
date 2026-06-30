package starcitygames

import (
	"log"
	"os"
	"testing"

	"github.com/mtgban/go-mtgban/mtgmatcher"
)

func TestMain(m *testing.M) {
	allprintingsPath := os.Getenv("ALLPRINTINGS5_PATH")
	if allprintingsPath == "" {
		log.Fatalln("Need ALLPRINTINGS5_PATH variable set to run tests")
	}

	allPrintingsReader, err := os.Open(allprintingsPath)
	if err != nil {
		log.Fatalln(err)
	}
	defer allPrintingsReader.Close()

	err = mtgmatcher.LoadDatastore(allPrintingsReader)
	if err != nil {
		log.Fatalln(err)
	}

	mtgmatcher.SetGlobalLogger(log.New(os.Stderr, "", 0))

	os.Exit(m.Run())
}

type SKUTest struct {
	Name string
	In   string
	Out  string
	Err  string
}

var SKUTests = []SKUTest{
	{
		In:  "SGL-MTG-FDN2-428-ENF1",
		Out: "f91b4613-1633-5428-90ca-420174ceb533",
	},
	{
		In:  "SGL-MTG-PWSB-ELD2_291-ENN1",
		Out: "728fa13b-e7e3-5193-a7a3-d18439bb7ee5",
	},
	{
		In:  "SGL-MTG-LTR4-741-ENF1",
		Out: "9d1b6dca-03ce-5c5c-b0d4-ee294bb89421",
	},
	{
		In:  "SGL-MTG-MPS3-001-ENF1",
		Out: "cb4a260d-8f98-5e81-b881-6727d5323917",
	},
	{
		Name: "Cavern of Souls",
		In:   "SGL-MTG-UMA2-32-ENF",
		Out:  "2b0cfd28-e73e-5519-8aea-608854b0ef43",
	},
	{
		In:  "SGL-MTG-WCHP-97SG_VIS_123s-ENN1",
		Out: "a6d53759-0830-5380-b7fe-a0d51858ec1b",
	},
	{
		Name: "Ponder",
		In:   "SGL-MTG-PRM-FEST_2025_002-ENF1",
		Out:  "0d239a5a-c6ad-54b5-b899-50ca15fdc6c9",
	},
	{
		In:  "SGL-MTG-PWSB-PRM_DRFT_UST_108-ENF1",
		Out: "e95a606c-0ac2-5a4f-a9e1-d3a29e518bb2",
	},
	{
		Name: "Counterspell",
		In:   "SGL-MTG-PRM-NYCC24_002-ENF1",
		Out:  "406b39be-799b-5da8-bc24-e1a8fa056680",
	},
	{
		Name: "Cloud, Midgar Mercenary",
		In:   "SGL-MTG-PRM-PT_2025_001-ENF1",
		Out:  "1830749f-55f4-52ba-8b3c-3e8d1a3805e6",
	},
	{
		Name: "Rograkh, Son of Rohgahh",
		In:   "SGL-MTG-PRM-CF_2025_002-ENF1",
		Out:  "7618d60e-3790-5e8f-bd44-3154001bf975",
	},
	{
		Name: "Ponder",
		In:   "SGL-MTG-PRM-SECRET_SLD_019-ENF1",
		Out:  "d25135dd-3a0b-5b7f-b23b-79b483324759",
	},
	{
		Name: "Ephemerate",
		In:   "SGL-MTG-PRM-SECRET_SLD_1990-ENF1",
		Out:  "5c67ed18-02b5-597a-a78f-fe7cb154b819_f",
	},
	{
		Name: "Agonizing Remorse",
		In:   "SGL-MTG-STA2-087-JAF1",
		Out:  "d50b8669-352c-58d9-8cb4-6352e1f0a5ee_e",
	},
	{
		In:  "SGL-MTG-PRM-CD_Q06_006-ENN1",
		Out: "f7bd93a8-58b7-58d0-849f-0c25dc2e56fb",
	},
}

func TestSCGSKU(t *testing.T) {
	for _, probe := range SKUTests {
		test := probe
		t.Run(test.In, func(t *testing.T) {
			t.Parallel()
			out, err := ProcessSKU(test.Name, test.In)
			if err == nil && test.Err != "" {
				t.Errorf("FAIL: Expected error: %s", test.Err)
				return
			}
			if err != nil {
				if test.Err == "" {
					t.Errorf("FAIL: Unexpected error: %s", err.Error())
					return
				}
				if test.Err != err.Error() {
					t.Errorf("FAIL: Mismatched error: expected '%s', got '%s'", test.Err, err.Error())
					return
				}
			}
			if out.Id != test.Out {
				co, _ := mtgmatcher.GetUUID(out.Id)
				t.Errorf("FAIL %s: Expected '%s' got '%s' (%s)", test.In, test.Out, out.Id, co)
				return
			}
			t.Log("PASS:", test.In)
		})
	}
}
