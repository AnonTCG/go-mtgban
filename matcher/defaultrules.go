package matcher

// DefaultRules answers the GameRules hooks a game has no use for, so a
// loader writes only the ones that do something. Embed it in a game's Rules
// and override whatever that game actually needs:
//
//	type Rules struct{ matcher.DefaultRules }
//
// The answers are the ones that change nothing: every candidate edition
// survives and nothing is unsupported at any stage. A game that
// means any of those has to say so itself, which is the point - a hook
// nobody wrote is a hook nobody has to read. The one answer that changes
// the input, AdjustEdition, applies only the game's own AliasEdition.
//
// Magic implements its policies explicitly and embeds nothing.
type DefaultRules struct{}

// AdjustEdition spells the edition through the game's own AliasEdition,
// which is the whole fixup for a game whose edition wording says nothing
// about the card. It asks the Backend's rules, since an embedded value
// cannot reach the AliasEdition of the game embedding it.
func (DefaultRules) AdjustEdition(b *Backend, inCard *InputCard) {
	inCard.Edition = b.rules.AliasEdition(b, inCard.Edition)
}

// FilterPrintings keeps every candidate edition.
func (DefaultRules) FilterPrintings(b *Backend, inCard *InputCard, editions []string) []string {
	return editions
}

// IsUnsupported reports that nothing is unsupported at any stage, which is
// the answer for a game whose catalog holds nothing it cannot sell. A game
// that files something else under its card products - Lorcana's
// puzzle-piece inserts and multi-card lots - says so itself.
func (DefaultRules) IsUnsupported(b *Backend, inCard *InputCard, co *CardObject, stage Stage) bool {
	return false
}

// IsToken reports that the game knows no token names beyond the ones its
// datastore already lists.
func (DefaultRules) IsToken(b *Backend, name string) bool {
	return false
}

// CandidateSets prefers an exact edition name, then a partial name. When
// neither identifies a set, or PromoWildcard requests a wider search, all
// printings reach the game's card filter.
func (DefaultRules) CandidateSets(b *Backend, inCard *InputCard, editions []string) []string {
	if len(editions) <= 1 || inCard.PromoWildcard {
		return editions
	}
	var exact, loose []string
	for _, code := range editions {
		set := b.Sets[code]
		if Equals(set.Name, inCard.Edition) {
			exact = append(exact, code)
		} else if Contains(set.Name, inCard.Edition) {
			loose = append(loose, code)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	if len(loose) > 0 {
		return loose
	}
	return editions
}

// FinalizeCandidates leaves ambiguity for the shared pipeline to report.
func (DefaultRules) FinalizeCandidates(b *Backend, inCard *InputCard, cards []Card) []Card {
	return cards
}
