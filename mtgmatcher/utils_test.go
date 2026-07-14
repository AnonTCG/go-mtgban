package mtgmatcher

import "testing"

type ExtractTest struct {
	In  string
	Out string
}

var NumberTests = []ExtractTest{
	{
		In:  "123",
		Out: "123",
	},
	{
		In:  "(321)",
		Out: "321",
	},
	{
		In:  "#24",
		Out: "24",
	},
	{
		In:  "100 - A",
		Out: "100",
	},
	{
		In:  "1a",
		Out: "1a",
	},
	{
		In:  "*4",
		Out: "*4",
	},
	{
		In:  "37A Text",
		Out: "37a",
	},
	{
		In:  "A08",
		Out: "a08",
	},
	{
		In:  "2000",
		Out: "",
	},
	{
		In:  "M19",
		Out: "",
	},
	{
		In:  "26 April",
		Out: "",
	},
	{
		In:  "181/185",
		Out: "181",
	},
	{
		In:  "001/006",
		Out: "1",
	},
	{
		In:  "May 25",
		Out: "",
	},
	{
		In:  "yaviMAYa 25",
		Out: "25",
	},
	{
		In:  "22 January, 2016",
		Out: "",
	},
	{
		In:  "7/4/1999",
		Out: "",
	},
	{
		In:  "37★",
		Out: "37★",
	},
	{
		In:  "1F★",
		Out: "1f★",
	},
	{
		In:  "659Φ",
		Out: "659φ",
	},
	{
		In:  "A25-141",
		Out: "A25-141",
	},
	{
		In:  "P09-008",
		Out: "P09-8",
	},
	{
		In:  "118†s",
		Out: "118†s",
	},
	{
		In:  "8th Edition 332 Julien Nuijten 2004",
		Out: "332",
	},
	{
		In:  "2001 Tom van de Logt 7ED 337",
		Out: "337",
	},
	{
		In:  "30a",
		Out: "30a",
	},
}

func TestExtractNumber(t *testing.T) {
	for _, probe := range NumberTests {
		test := probe
		t.Run(test.In, func(t *testing.T) {
			t.Parallel()
			out := ExtractNumber(test.In)
			if out != test.Out {
				t.Errorf("FAIL %s: Expected '%s' got '%s'", test.In, test.Out, out)
				return
			}
			t.Log("PASS:", test.In)
		})
	}
}

var YearTests = []ExtractTest{
	{
		In:  "Judge 2007",
		Out: "2007",
	},
	{
		In:  "Judge Foil (2020)",
		Out: "2020",
	},
	{
		In:  "FNM '06",
		Out: "2006",
	},
	{
		In:  "20 - multiples 2012",
		Out: "2012",
	},
	{
		In:  "not a 96 year",
		Out: "",
	},
	{
		In:  "missing year",
		Out: "",
	},
	{
		In:  "Urza's Saga Arena 1999",
		Out: "1999",
	},
	{
		In:  "M14 Core Set",
		Out: "2014",
	},
	{
		In:  "WCD 2002:",
		Out: "2002",
	},
	{
		In:  "7/4/1999",
		Out: "",
	},
}

func TestExtractYear(t *testing.T) {
	for _, probe := range YearTests {
		test := probe
		t.Run(test.In, func(t *testing.T) {
			t.Parallel()
			out := ExtractYear(test.In)
			if out != test.Out {
				t.Errorf("FAIL %s: Expected '%s' got '%s'", test.In, test.Out, out)
				return
			}
			t.Log("PASS:", test.In)
		})
	}
}

var NumericTests = []ExtractTest{
	{
		In:  "123",
		Out: "123",
	},
	{
		In:  "(321)",
		Out: "321",
	},
	{
		In:  "#24",
		Out: "24",
	},
	{
		In:  "100 - A",
		Out: "100",
	},
	{
		In:  "1a",
		Out: "1",
	},
	{
		In:  "*4",
		Out: "4",
	},
	{
		In:  "37A Text",
		Out: "37",
	},
	{
		In:  "A08",
		Out: "8",
	},
	{
		In:  "2000",
		Out: "2000",
	},
	{
		In:  "181/185",
		Out: "181",
	},
	{
		In:  "001/006",
		Out: "1",
	},
	{
		In:  "May 25",
		Out: "25",
	},
	{
		In:  "22 January, 2016",
		Out: "22",
	},
	{
		In:  "7/4/1999",
		Out: "7",
	},
	{
		In:  "37★",
		Out: "37",
	},
	{
		In:  "1F★",
		Out: "1",
	},
	{
		In:  "659Φ",
		Out: "659",
	},
	{
		In:  "118†s",
		Out: "118",
	},
}

func TestExtractNumberValue(t *testing.T) {
	for _, probe := range NumericTests {
		test := probe
		t.Run(test.In, func(t *testing.T) {
			t.Parallel()
			out := ExtractNumberValue(test.In)
			if out != test.Out {
				t.Errorf("FAIL %s: Expected '%s' got '%s'", test.In, test.Out, out)
				return
			}
			t.Log("PASS:", test.In)
		})
	}
}

type CutTest struct {
	In  string
	Tag string
	Out []string
}

var CutTests = []CutTest{
	{
		In:  "A B C",
		Tag: "A",
		Out: []string{"", "A B C"},
	},
	{
		In:  "A B C",
		Tag: "C",
		Out: []string{"A B", "C"},
	},
	{
		In:  "A B C",
		Tag: "D",
		Out: []string{"A B C"},
	},
	{
		In:  "A B C D",
		Tag: "B C",
		Out: []string{"A", "B C D"},
	},
}

func TestCut(t *testing.T) {
	for _, probe := range CutTests {
		test := probe
		t.Run(test.In, func(t *testing.T) {
			t.Parallel()
			out := Cut(test.In, test.Tag)
			for i := range out {
				if out[i] != test.Out[i] {
					t.Errorf("FAIL %s: Expected '%s' got '%q'", test.In, test.Out, out)
					return
				}
			}
			t.Log("PASS:", test.In)
		})
	}
}

func TestAlias(t *testing.T) {
	inCard := &InputCard{
		Name:      "Forest",
		Variation: "Full-Art",
		Edition:   "Zendikar",
	}
	// These are ZEN Forest 246, 247, 248, 249
	outCards := []string{
		"8b2b4813-187c-53d1-8ee6-d9109ce4c427",
		"7c0ffc88-34ff-5436-bfe7-ac9f1dd62888",
		"59cea094-ebc9-5afa-bdf3-f0cc832a2136",
		"41d883ae-9018-5218-887e-502b03a2b89f",
	}

	_, err := Match(inCard)
	if err == nil {
		t.Errorf("FAIL: this call is supposed to return an error")
		return
	}

	alias, ok := err.(*AliasingError)
	if !ok {
		t.Errorf("FAIL: the returned error is not AliasingError")
		t.Errorf("%s", err.Error())
		return
	}

	dupes := alias.Probe()
	if len(dupes) != len(outCards) {
		t.Errorf("FAIL: wrong number of dupes returned")
		t.Errorf("%v", dupes)
		return
	}

	for i := range dupes {
		if dupes[i] != outCards[i] {
			t.Errorf("FAIL: incorrect duplicate returned")
			t.Errorf("%v vs %v", dupes[i], outCards[i])
		}
	}
	t.Log("PASS: Aliasing")
}

type SplitTest struct {
	In  string
	Out []string
}

var SplitTests = []SplitTest{
	{
		In:  "A",
		Out: []string{"A"},
	},
	{
		In:  "A B",
		Out: []string{"A B"},
	},
	{
		In:  "A (B)",
		Out: []string{"A", "B"},
	},
	{
		In:  "A (B) (C)",
		Out: []string{"A", "B", "C"},
	},
	{
		In:  "A B (C)",
		Out: []string{"A B", "C"},
	},
	{
		In:  "A (B) C",
		Out: []string{"A", "B"},
	},
	{
		In:  "A (B) Token",
		Out: []string{"A Token", "B"},
	},
	{
		In:  "Erase (Not the Urza's Legacy One)",
		Out: []string{"Erase (Not the Urza's Legacy One)"},
	},
	{
		In:  "B.F.M. (Big Furry Monster) (Left)",
		Out: []string{"B.F.M. (Big Furry Monster)", "Left"},
	},
	{
		In:  "A (B)(C)",
		Out: []string{"A", "B", "C"},
	},
	{
		In:  "A (B (C))",
		Out: []string{"A", "B", "C"},
	},
}

func TestSplit(t *testing.T) {
	for _, probe := range SplitTests {
		test := probe
		t.Run(test.In, func(t *testing.T) {
			t.Parallel()
			out := SplitVariants(test.In)
			if len(out) != len(test.Out) {
				t.Errorf("FAIL %s: Expected '%q' got '%q'", test.In, test.Out, out)
				return
			}
			for i := range out {
				if out[i] != test.Out[i] {
					t.Errorf("FAIL %s: Expected '%q' got '%q'", test.In, test.Out, out)
					return
				}
			}
			t.Log("PASS:", test.In)
		})
	}
}

var TitleTests = []ExtractTest{
	{
		In:  "abc abc",
		Out: "Abc Abc",
	},
	{
		In:  "Abc abC",
		Out: "Abc Abc",
	},
	{
		In:  "ABC ABC",
		Out: "Abc Abc",
	},
}

func TestTitle(t *testing.T) {
	for _, probe := range TitleTests {
		test := probe
		t.Run(test.In, func(t *testing.T) {
			t.Parallel()
			out := Title(test.In)
			if out != test.Out {
				t.Errorf("FAIL %s: Expected '%s' got '%s'", test.In, test.Out, out)
				return
			}
			t.Log("PASS:", test.In)
		})
	}
}
