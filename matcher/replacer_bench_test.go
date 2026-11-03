package matcher

import (
	"fmt"
	"sync/atomic"
	"testing"
)

// Four times the cache's cap, so a key is reset out of the cache before
// the loop comes round to it again.
const uniqueKeysPool = 4 * normalizeCacheCap

var uniqueKeysRuns atomic.Int64

// uniqueKeys returns a pool of strings that no earlier call returned, so a
// run starts from keys Normalize has not seen.
func uniqueKeys() []string {
	run := uniqueKeysRuns.Add(1)
	keys := make([]string, uniqueKeysPool)
	for i := range keys {
		keys[i] = fmt.Sprintf("Listing %d-%d (Foil) - Near Mint", run, i)
	}
	return keys
}

// A scrape's listings: every string new, each normalized twice.
func BenchmarkNormalizeUniqueTwice(b *testing.B) {
	keys := uniqueKeys()
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		key := keys[i%len(keys)]
		Normalize(key)
		Normalize(key)
	}
}

// A scrape's listings normalized from parallel workers.
func BenchmarkNormalizeUniqueTwiceParallel(b *testing.B) {
	keys := uniqueKeys()
	var next atomic.Int64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := keys[int(next.Add(1)-1)%len(keys)]
			Normalize(key)
			Normalize(key)
		}
	})
}

// A match's hot keys: a few strings normalized from parallel workers.
func BenchmarkNormalizeHotParallel(b *testing.B) {
	keys := []string{"Foil", "Etched", "Oversize", "Lightning Bolt", "Secret Lair Drop", "Prerelease"}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			Normalize(keys[i%len(keys)])
			i++
		}
	})
}
