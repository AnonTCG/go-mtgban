package matcher

import (
	"errors"
	"slices"
	"sort"
	"testing"
)

type candidateTestRules struct {
	DefaultRules
	codes    []string
	finalize func([]Card) []Card
}

func (candidateTestRules) Prefilter(*Backend, *InputCard)                 {}
func (candidateTestRules) AdjustName(*Backend, *InputCard)                {}
func (candidateTestRules) AdjustEdition(*Backend, *InputCard)             {}
func (candidateTestRules) AliasEdition(_ *Backend, edition string) string { return edition }
func (candidateTestRules) PlainNumber(number string) string               { return number }
func (r candidateTestRules) CandidateSets(b *Backend, in *InputCard, editions []string) []string {
	if r.codes != nil {
		return r.codes
	}
	return r.DefaultRules.CandidateSets(b, in, editions)
}
func (candidateTestRules) FilterCards(_ *Backend, _ *InputCard, sets []CandidateSet) []Card {
	var cards []Card
	for _, set := range sets {
		cards = append(cards, set.Cards...)
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].UUID < cards[j].UUID })
	return cards
}
func (r candidateTestRules) FinalizeCandidates(_ *Backend, _ *InputCard, cards []Card) []Card {
	if r.finalize != nil {
		return r.finalize(cards)
	}
	return cards
}

func candidateTestBackend() *Backend {
	b := &Backend{
		Sets: map[string]*Set{}, UUIDs: map[string]*CardObject{},
		CanonicalNames: map[string]string{Normalize("Test Card"): "Test Card"},
		Hashes:         map[string][]string{Normalize("Test Card"): {"a", "b"}},
	}
	for i, code := range []string{"A", "B"} {
		id := []string{"a", "b"}[i]
		card := Card{UUID: id, Name: "Test Card", SetCode: code, Language: "English",
			Printings: []string{"A", "B"}, Finishes: []string{FinishNonfoil},
			FoilUUIDs: map[string]string{FinishNonfoil: id}}
		b.Sets[code] = &Set{Code: code, Name: "Edition " + code, Cards: []Card{card}}
		b.UUIDs[id] = &CardObject{Card: card}
	}
	b.SetRules(candidateTestRules{})
	return b
}

func TestDefaultCandidateSelection(t *testing.T) {
	b := candidateTestBackend()
	b.Sets["A"].Name = "World Championship"
	b.Sets["B"].Name = "World Championship Promos"
	for _, tt := range []struct {
		name string
		in   InputCard
		want []string
	}{
		{"exact", InputCard{Edition: "World Championship"}, []string{"A"}},
		{"partial", InputCard{Edition: "World"}, []string{"A", "B"}},
		{"unknown", InputCard{Edition: "Elsewhere"}, []string{"A", "B"}},
		{"wildcard", InputCard{Edition: "World Championship", PromoWildcard: true}, []string{"A", "B"}},
		{"no Magic promo expansion", InputCard{Edition: "World Championship", Variation: "Prerelease"}, []string{"A"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := (DefaultRules{}).CandidateSets(b, &tt.in, []string{"A", "B"})
			if !slices.Equal(got, tt.want) {
				t.Fatalf("candidates = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchLeavesOtherGamesWorldChampionshipAmbiguous(t *testing.T) {
	b := candidateTestBackend()
	b.Sets["A"].Name = "World Championship"
	b.Sets["B"].Name = "World Championship"
	_, err := b.Match(&InputCard{Name: "Test Card", Edition: "World Championship"})
	var alias *AliasingError
	if !errors.As(err, &alias) || !slices.Equal(alias.Probe(), []string{"a", "b"}) {
		t.Fatalf("non-Magic candidates were trimmed: %v", err)
	}
}

func TestMatchUsesGameCandidateSets(t *testing.T) {
	b := candidateTestBackend()
	// B is not in the name's printing list; the game admits it as a sibling.
	b.UUIDs["a"].Printings = []string{"A"}
	b.SetRules(candidateTestRules{codes: []string{"B"}})
	id, err := b.Match(&InputCard{Name: "Test Card", Edition: "Edition A"})
	if err != nil || id != "b" {
		t.Fatalf("expanded candidate = %q, %v", id, err)
	}
}

func TestMatchFinalizesBeforeLanguageFiltering(t *testing.T) {
	b := candidateTestBackend()
	b.UUIDs["a"].Language = "French"
	var seen []string
	b.SetRules(candidateTestRules{finalize: func(cards []Card) []Card {
		for _, card := range cards {
			seen = append(seen, card.UUID)
		}
		return cards[:1]
	}})
	_, err := b.Match(&InputCard{Name: "Test Card", Language: "English"})
	if !slices.Equal(seen, []string{"a", "b"}) || err != ErrUnsupported {
		t.Fatalf("finalization saw %v and returned %v; want both candidates then language rejection", seen, err)
	}
}
