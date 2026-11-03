package coolstuffinc

import "testing"

// TestEveryGameHasAResolver keeps the resolver tables in step with the games
// the singles scraper registers for, which would otherwise fail every load.
func TestEveryGameHasAResolver(t *testing.T) {
	for _, game := range singlesGames {
		_, found := retailResolvers[game]
		if !found {
			t.Errorf("%s has no resolver in retailResolvers", game)
		}
		_, found = buylistResolvers[game]
		if !found {
			t.Errorf("%s has no resolver in buylistResolvers", game)
		}
	}
}
