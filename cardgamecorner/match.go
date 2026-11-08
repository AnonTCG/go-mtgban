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

// matchYuGiOh retries with the SKU's number a listing whose number field is
// wrong, as it is on a few, while the SKU still ends in the card's own; what
// the wording still refuses is placed by its product image.
func matchYuGiOh(cgc *Cardgamecorner, card *Card, index int, input *matcher.InputCard) (string, error) {
	retry := *input
	cardID, err := cgc.backend.Match(input)
	if errors.Is(err, matcher.ErrCardWrongVariant) {
		id, found := matchBySKUNumber(cgc.backend, card, index)
		if found {
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
