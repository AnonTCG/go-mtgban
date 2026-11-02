// Package gametest is what the game suites under matcher/ share: the
// datastore each one matches against. It is not a _test.go file because a
// test file cannot import another package's.
package gametest

import (
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
