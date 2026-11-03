package fleshandblood

import (
	"strings"
	"testing"
	"time"

	"github.com/mtgban/go-mtgban/internal/gametest"
	"github.com/mtgban/go-mtgban/matcher"
)

// TestFaceKey pins the fold that makes two spellings of one pairing answer
// alike: order, repetition and case are all things a storefront decides and
// the card does not.
func TestFaceKey(t *testing.T) {
	for _, tt := range []struct {
		desc, name, want string
	}{
		{"the faces sort", "Soul Shackle // Spectral Shield", "soulshackle//spectralshield"},
		{"and sort the same written the other way",
			"Spectral Shield // Soul Shackle", "soulshackle//spectralshield"},
		{"a doubled pairing folds onto its faces",
			"Gold // Golden Cog // Gold // Golden Cog", "gold//goldencog"},
		{"case is the storefront's, not the card's",
			"SOUL SHACKLE // Spectral Shield", "soulshackle//spectralshield"},
		{"a name with no separator has no key", "Spectral Shield", ""},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			if got := faceKey(tt.name); got != tt.want {
				t.Errorf("faceKey(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// TestFusedNamedByManyFaces pins the cost of a name whose face count the
// storefront chose. Asking after each ordering costs a factorial of that: at
// eleven faces a spelling-by-spelling search takes the better part of a
// minute inside Prefilter, which a vendor can aim at any run at will.
// Comparing the faces as a set costs one lookup whatever the count.
func TestFusedNamedByManyFaces(t *testing.T) {
	b := gametest.Backend(t, matcher.GameFleshAndBlood)

	faces := make([]string, 11)
	for i := range faces {
		faces[i] = string(rune('A' + i))
	}
	name := strings.Join(faces, " // ")

	done := make(chan []string, 1)
	go func() { done <- newRules(b).fusedNamedBy(name) }()

	select {
	case got := <-done:
		if got != nil {
			t.Errorf("fusedNamedBy(%q) = %v, want none", name, got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("fusedNamedBy did not answer an eleven-faced name in ten seconds")
	}
}
