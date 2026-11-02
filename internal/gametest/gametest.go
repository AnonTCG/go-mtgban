// Package gametest is what the game suites under matcher/ share: the
// datastore each one matches against, and the replay of its golden corpus.
// It is not a _test.go file because a test file cannot import another
// package's.
package gametest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/mtgban/go-mtgban/internal/datastore"
	"github.com/mtgban/go-mtgban/internal/vocabulary"
	"github.com/mtgban/go-mtgban/matcher"
)

// read is each game's datastore once a test has asked for it, or the
// error reading it gave.
var (
	readMu sync.Mutex
	read   = map[matcher.Game]loaded{}
)

type loaded struct {
	backend *matcher.Backend
	err     error
}

// Backend hands a test the game's datastore, read the first time a test of
// the binary asks for it, and skips the test where the run carries none:
// each suite runs under the job holding its own game's file, and not the
// others'.
func Backend(t testing.TB, game matcher.Game) *matcher.Backend {
	t.Helper()
	variable, ok := vocabulary.Games[game]
	if !ok {
		t.Fatalf("no datastore variable for %s", game)
	}
	path := vocabulary.PathOf(game)
	if path == "" {
		t.Skipf("Need %s set to run this test", variable)
	}

	readMu.Lock()
	got, done := read[game]
	if !done {
		got.backend, got.err = datastore.Read(game, path)
		read[game] = got
	}
	readMu.Unlock()

	if got.err != nil {
		t.Fatal(got.err)
	}
	return got.backend
}

// Case is one entry of a game's replay corpus: an input card and the
// verdict Match returns for it, a uuid or an error string.
type Case struct {
	Desc string            `json:"description"`
	In   matcher.InputCard `json:"input"`
	ID   string            `json:"uuid,omitempty"`
	Err  string            `json:"error,omitempty"`
}

// Replay matches every case in the game's testdata/<game>_test_data.json
// against b and fails each whose verdict moved.
//
// With update set it rewrites that file instead: every committed case plus
// the seeds is matched again and its verdict baked in. Seeds are the
// hand-authored cases pinning the contract edges the sampled corpus cannot
// reach; their verdicts are never typed, and a "negative:" description
// declares the author expects an error. A verdict may change in detail -
// each change is logged - but a case flipping between success and error
// leaves the file untouched and fails: a change of that size has to be
// acknowledged by editing the entry or the seed by hand.
func Replay(t *testing.T, b *matcher.Backend, seeds []Case, update bool) {
	t.Helper()
	path := filepath.Join("testdata", string(b.Game)+"_test_data.json")

	data, err := os.ReadFile(path)
	if err != nil {
		if update && os.IsNotExist(err) {
			data = []byte("[]")
		} else {
			t.Fatal(err)
		}
	}
	var tests []Case
	err = json.Unmarshal(data, &tests)
	if err != nil {
		t.Fatal(err)
	}

	if update {
		regenerate(t, b, path, tests, seeds)
		return
	}

	if len(tests) == 0 {
		t.Fatalf("no test cases in %s", path)
	}

	for _, tt := range tests {
		t.Run(tt.Desc, func(t *testing.T) {
			t.Parallel()
			in := tt.In
			id, err := b.Match(&in)
			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if id != tt.ID || gotErr != tt.Err {
				t.Errorf("Match(%q ed=%q num=%q foil=%v) = (%q, %q), want (%q, %q)",
					tt.In.Name, tt.In.Edition, tt.In.Variation, tt.In.Foil, id, gotErr, tt.ID, tt.Err)
			}
		})
	}
}

// regenerate rewrites path with the verdicts Match now returns for tests
// and seeds, sorted by description for a stable diff.
func regenerate(t *testing.T, b *matcher.Backend, path string, tests, seeds []Case) {
	// Seeds are authoritative for their inputs: drop any committed entry
	// sharing a seed description, so editing a seed takes effect.
	isSeed := map[string]bool{}
	for _, seed := range seeds {
		isSeed[seed.Desc] = true
	}
	kept := tests[:0]
	for _, tt := range tests {
		if !isSeed[tt.Desc] {
			kept = append(kept, tt)
		}
	}
	tests = append(kept, seeds...)

	for i := range tests {
		in := tests[i].In
		id, err := b.Match(&in)
		gotErr := ""
		if err != nil {
			gotErr = err.Error()
		}

		wantNegative := tests[i].Err != ""
		if isSeed[tests[i].Desc] {
			wantNegative = strings.HasPrefix(tests[i].Desc, "negative:")
		}
		if wantNegative != (gotErr != "") {
			t.Errorf("refusing to flip %q: (%q, %q) -> (%q, %q); edit the entry or seed by hand",
				tests[i].Desc, tests[i].ID, tests[i].Err, id, gotErr)
			continue
		}

		if tests[i].ID != id || tests[i].Err != gotErr {
			t.Logf("updating %q: (%q, %q) -> (%q, %q)",
				tests[i].Desc, tests[i].ID, tests[i].Err, id, gotErr)
		}
		tests[i].ID = id
		tests[i].Err = gotErr
	}
	if t.Failed() {
		t.Fatal("verdict-class flips detected; golden file left untouched")
	}

	sort.Slice(tests, func(i, j int) bool {
		return tests[i].Desc < tests[j].Desc
	})

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "    ")
	// A set or card name's "&" stays itself rather than an HTML escape.
	enc.SetEscapeHTML(false)
	err := enc.Encode(tests)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(path, buf.Bytes(), 0644)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d test cases to %s", len(tests), path)
}
