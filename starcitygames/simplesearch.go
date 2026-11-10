package starcitygames

import (
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// simpleSearch resolves a card by exact (or prefix) name plus collector number
// and finish. It is a package-local copy of the matcher.SimpleSearch helper
// that upstream deleted in the game-agnostic refactor (scrapers are expected
// to move to Match); this scraper still keys off the sell-list payload's
// name/number/finish triple, so the old resolution semantics are preserved
// verbatim, built only on exported core primitives.
func simpleSearch(b *matcher.Backend, cardName, number string, foil bool) (string, error) {
	number = strings.TrimLeft(number, "0")
	number = strings.Split(number, "/")[0]

	cardName = matcher.SplitVariants(cardName)[0]

	uuids, err := b.SearchEquals(cardName)
	if err != nil {
		uuids, err = b.SearchHasPrefix(cardName)
		if err != nil {
			return "", err
		}
	}

	if len(uuids) == 1 {
		return uuids[0], nil
	}

	var cardIds []string
	for _, uuid := range uuids {
		co, err := b.GetUUID(uuid)
		if err != nil {
			continue
		}

		if foil && !co.Foil {
			continue
		} else if !foil && co.Foil {
			continue
		}

		if number != "" && number != co.Number {
			continue
		}
		cardIds = append(cardIds, uuid)
	}

	if len(cardIds) < 1 {
		return "", matcher.ErrCardWrongVariant
	}

	if len(cardIds) > 1 {
		return "", matcher.NewAliasingError(uuids...)
	}

	return cardIds[0], nil
}
