package cardgamecorner

import (
	"errors"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/matcher/magic"
)

// gameMatchers places a listing in the games where its wording alone leaves
// some unplaced, by what else the listing states: the card, the index of the
// variant read, and the matcher input its wording gave. A game named nowhere
// here is matched on its wording alone.
var gameMatchers = map[matcher.Game]func(cgc *Cardgamecorner, card *Card, index int, input *matcher.InputCard) (string, error){
	matcher.GameMagic:    matchMagic,
	matcher.GamePokemon:  matchPokemon,
	matcher.GameYuGiOh:   matchYuGiOh,
	matcher.GameOnePiece: matchOnePiece,
}

// matchWording answers a listing on its wording alone.
func matchWording(cgc *Cardgamecorner, _ *Card, _ int, input *matcher.InputCard) (string, error) {
	return cgc.backend.Match(input)
}

// matchOnePiece answers a listing from its product image before its
// wording.
func matchOnePiece(cgc *Cardgamecorner, card *Card, _ int, input *matcher.InputCard) (string, error) {
	cardID, found := cgc.matchByProductImage(card, input)
	if found {
		return cardID, nil
	}
	return cgc.backend.Match(input)
}

// matchPokemon answers from the product image what the wording cannot
// place.
func matchPokemon(cgc *Cardgamecorner, card *Card, _ int, input *matcher.InputCard) (string, error) {
	retry := *input
	cardID, err := cgc.backend.Match(input)
	if err != nil && !errors.Is(err, matcher.ErrUnsupported) {
		id, found := cgc.matchByProductImage(card, &retry)
		if found {
			return id, nil
		}
	}
	return cardID, err
}

// matchYuGiOh answers a listing on its wording, then looks for the version
// the product image names: it refines the answer to a printing of the same
// set and rarity, the alt art or colour the store copied from Cardmarket,
// and places what the wording refuses. The image of a European copy on an
// old shelf names the North American print, so it only places what the
// wording refuses. A listing whose number field is wrong, as it is on a few,
// is retried with the SKU's number while the SKU still ends in the card's
// own.
func matchYuGiOh(cgc *Cardgamecorner, card *Card, index int, input *matcher.InputCard) (string, error) {
	retry := *input
	cardID, err := cgc.backend.Match(input)
	if errors.Is(err, matcher.ErrCardWrongVariant) {
		id, found := matchBySKUNumber(cgc.backend, card, index)
		if found {
			return id, nil
		}
	}
	europeanCopy := oldShelfRe.MatchString(card.Edition) && card.Variants[index].Language == "EN"
	if err == nil && !europeanCopy {
		id, found := cgc.matchYuGiOhImage(card, index, &retry)
		if found && cgc.sameSetAndRarity(id, cardID) {
			return id, nil
		}
	}
	if err != nil && !errors.Is(err, matcher.ErrUnsupported) {
		id, found := cgc.matchYuGiOhImage(card, index, &retry)
		if found {
			return id, nil
		}
	}
	return cardID, err
}

// sameSetAndRarity reports whether two printings sit in one set with one
// rarity.
func (cgc *Cardgamecorner) sameSetAndRarity(a, b string) bool {
	first, err := cgc.backend.GetUUID(a)
	if err != nil {
		return false
	}
	second, err := cgc.backend.GetUUID(b)
	return err == nil && first.SetCode == second.SetCode && first.Rarity == second.Rarity
}

// matchYuGiOhImage answers from the product image, accepting the printing
// only when it is the card the listing names, in the print run it lists. The
// store's version tail is not part of the name.
func (cgc *Cardgamecorner) matchYuGiOhImage(card *Card, index int, input *matcher.InputCard) (string, bool) {
	name, _, _ := strings.Cut(input.Name, " (")
	id, found := cgc.matchByProductImage(card, input)
	if !found {
		return "", false
	}
	co, err := cgc.backend.GetUUID(id)
	if err != nil || !matcher.Equals(co.Name, name) {
		return "", false
	}
	if (card.Variants[index].Print == "First Edition") != (co.Finish == "1stedition") {
		return "", false
	}
	return id, true
}

// matchMagic retries what the wording cannot place with the number the
// listing states. The basic lands carry nothing but the store's own version
// tag, so they are retried with the number their SKU spells, then the one
// their image names, and the ones neither places are unsupported.
func matchMagic(cgc *Cardgamecorner, card *Card, _ int, input *matcher.InputCard) (string, error) {
	retry := *input
	cardID, err := cgc.backend.Match(input)
	if err == nil || errors.Is(err, matcher.ErrUnsupported) {
		return cardID, err
	}
	if magic.IsBasicLand(card.Name) {
		id, found := matchBasicByImage(cgc.backend, card, retry)
		if !found {
			return "", matcher.ErrUnsupported
		}
		return id, nil
	}
	id, found := matchByListedNumber(cgc.backend, card, retry)
	if found {
		return id, nil
	}
	return cardID, err
}
