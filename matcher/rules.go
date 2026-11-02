package matcher

// Stage names the point of Match at which GameRules.IsUnsupported is asked.
type Stage int

const (
	// StageWording is before the name lookup: the storefront's own name,
	// edition and variation, once Prefilter has run.
	StageWording Stage = iota
	// StageEdition is after AdjustEdition: the canonical name and the
	// edition the game resolved.
	StageEdition
	// StageAnswer is the one printing resolved, by id or by wording. A
	// listing claiming a promo the printing does not carry is refused here.
	StageAnswer
)

// CandidateSet is one set CandidateSets admitted for an input, with the
// printings of the input's name filed there. A set holding none of them is
// still listed, with no Cards.
type CandidateSet struct {
	Code  string
	Cards []Card
}

// GameRules abstracts the game-specific steps of the Match pipeline so that a
// datastore loaded for a non-Magic game can supply its own card-identification
// logic. Magic implements these hooks in the matcher/magic sub-package and
// Lorcana in matcher/lorcana; a datastore loader attaches its implementation
// via SetRules when it builds a Backend, and Match dispatches the major
// pipeline stages through the stored rules. Name preprocessing belongs to the
// game: Prefilter is what splits bracketed editions and parenthesized or
// dashed variants off the name. What stays in Match itself is the language
// handling (normalizing the requested language, then dropping candidates that
// do not carry it) and the token/oversize gate on an unresolved name, so a
// hook implementation must expect the InputCard to have been mutated by those
// steps before it runs.
//
// Hooks receive the InputCard by pointer and may mutate it; mutations persist
// for the rest of the pipeline and are also visible to the caller after Match
// returns. AdjustEdition in particular may set flags on the InputCard (the
// Magic rules set PromoWildcard and BeyondBaseSet) that Match and later hooks
// read.
type GameRules interface {
	// Prefilter mutates the input card before the canonical-name lookup.
	Prefilter(b *Backend, inCard *InputCard)
	// AdjustName fixes up the input card name to match a canonical name.
	AdjustName(b *Backend, inCard *InputCard)
	// AdjustEdition fixes up the input card edition to a known set.
	AdjustEdition(b *Backend, inCard *InputCard)
	// AliasEdition spells an edition string the way the datastore names its
	// set, using only the string itself - the card-free part of
	// AdjustEdition. GetSetByName's last resort is this hook, never the
	// full fixup, so an implementation must not route back through it.
	AliasEdition(b *Backend, edition string) string
	// FilterPrintings narrows the candidate editions for the input card.
	FilterPrintings(b *Backend, inCard *InputCard, editions []string) []string
	// CandidateSets selects set codes after FilterPrintings and may expand
	// them to related editions. Match materializes those sets before calling
	// FilterCards. DefaultRules tries exact edition, loose edition, then all.
	CandidateSets(b *Backend, inCard *InputCard, editions []string) []string
	// FinalizeCandidates applies the game's final ambiguity policy after
	// FilterCards and before language filtering. DefaultRules keeps every
	// candidate; order is the deterministic order returned by FilterCards.
	FinalizeCandidates(b *Backend, inCard *InputCard, cards []Card) []Card
	// FilterCards narrows the candidate cards for the input card.
	// candidates holds every set CandidateSets admitted, sorted by code,
	// each with the printings filed under the input's name there, once per
	// printing. The result feeds user-visible aliasing diagnostics, so an
	// implementation keeps it in an order that does not change between
	// calls.
	FilterCards(b *Backend, inCard *InputCard, candidates []CandidateSet) []Card
	// IsUnsupported reports whether the input names something the game has
	// no printing for, which Match answers with ErrUnsupported so a caller
	// skips it in silence. It is asked at every Stage, since each sees
	// different text; co is the resolved printing at StageAnswer and nil
	// before it.
	IsUnsupported(b *Backend, inCard *InputCard, co *CardObject, stage Stage) bool
	// IsToken reports whether a name is one this game knows as a token
	// without its carrying a token type of its own - the rules tips, the
	// checklists, the storefront spellings that only ever name a token. The
	// datastore's own token list is checked before this is asked, so a game
	// with nothing to add answers false.
	IsToken(b *Backend, name string) bool
	// PlainNumber reduces a collector number to the one a person writes,
	// which is the number the loader stores as Card.PlainNumber: Magic
	// drops the mark and the letters that name a printing, Pokemon the zeros
	// its catalog pads an ordinal with, Riftbound its star, Lorcana its
	// variant letter, and the games that number their cards behind a set
	// code - Flesh and Blood, One Piece, Gundam, Yu-Gi-Oh - everything but
	// the ordinal itself, through PlainOrdinal. A number reduced to nothing
	// is answered two ways, and each game says which: the games numbering
	// behind a code yield nothing for a number carrying no ordinal, since
	// a word like DON is not the ordinal a search folds towards, while
	// Magic, Pokemon, Lorcana and Palworld hand such a number back, there
	// being no other number for a person to write. The game owns this the
	// way it owns its finish names, so a caller folding a number a person
	// typed need not know which marks belong to which game - and one that
	// rebuilds the vocabulary for itself can get the case of Magic's phi
	// wrong.
	PlainNumber(number string) string
}

// SetRules attaches the game-specific identification hooks used by Match. A
// game's datastore loader calls this when it builds a Backend.
func (b *Backend) SetRules(r GameRules) {
	b.rules = r

	// The finishes this datastore actually sells, which is what tells a
	// vendor spelling nobody has taught the game yet from a name that names
	// a real finish this one printing is not sold in. A game keying finishes
	// by the catalog's own names places any name, so it has no other way to
	// say "I have never heard of this".
	b.knownFinishes = map[string]bool{}
	for _, co := range b.UUIDs {
		for key := range co.FoilUUIDs {
			b.knownFinishes[key] = true
		}
	}
}
