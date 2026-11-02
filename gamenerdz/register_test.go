package gamenerdz

import "testing"

// TestEveryGameHasAResolver keeps gameResolvers in step with gnGames, since a
// game NewScraper accepts without a resolver would refuse every product.
func TestEveryGameHasAResolver(t *testing.T) {
	for game := range gnGames {
		_, found := gameResolvers[game]
		if !found {
			t.Errorf("%s has no resolver in gameResolvers", game)
		}
	}
}
