package merlion

import (
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
)

func TestInfoReadsTheBackendsGame(t *testing.T) {
	// A game merlion does not register, so a hardcoded game cannot pass.
	b := &matcher.Backend{Game: matcher.GameLorcana}
	got := NewScraper(b).Info().Game
	if got != b.Game {
		t.Errorf("Info().Game = %q, want %q", got, b.Game)
	}
}
