package cardgamecorner

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
)

// listedCodeRe matches the collector code a One Piece name carries, ahead of
// the store's "(V.N)" index; some are written without the dash.
var listedCodeRe = regexp.MustCompile(`\(([A-Za-z]+[0-9]*-?[0-9]+[a-zA-Z]*)\)`)

// matchByProductImage answers a listing from the Cardmarket product id its
// image is named after: the id the datastore publishes, else the TCGplayer id
// the bridge links it to. The store numbers its versions the way Cardmarket
// does, which the datastore cannot derive. The answer is accepted only when
// the printing carries a collector number the listing itself states, since
// the store also reuses one image across products.
func (cgc *Cardgamecorner) matchByProductImage(card *Card, input *matcher.InputCard) (string, bool) {
	image := mcmImageIDRe.FindStringSubmatch(card.Extra)
	if image == nil {
		return "", false
	}

	b := cgc.backend
	id := b.ConvertID(matcher.IDSpaceCardmarket, image[1])
	if id == "" {
		productID, err := strconv.Atoi(image[1])
		if err != nil {
			return "", false
		}
		tcgID, found := cgc.bridge[productID]
		if !found {
			return "", false
		}
		id = fmt.Sprint(tcgID)
	}

	var cardID string
	var err error
	if input.Finish != "" {
		cardID, err = b.MatchIDFinish(id, input.Finish)
	} else {
		cardID, err = b.MatchID(id, input.Foil)
	}
	if err != nil {
		return "", false
	}
	co, err := b.GetUUID(cardID)
	if err != nil || !cgc.statesNumber(card, co.Number) {
		return "", false
	}
	return cardID, true
}

// statesNumber reports whether the listing states the collector number a
// printing carries. A One Piece name states its code, which the store cuts
// short or gets wrong on a few, where the number field still holds the right
// card's; a Yu-Gi-Oh number field states the end of the code or its digits;
// a Pokemon listing states a number, or has none and its SKU ends in the
// card's.
func (cgc *Cardgamecorner) statesNumber(card *Card, number string) bool {
	switch cgc.backend.Game {
	case matcher.GameOnePiece:
		// A DON!! printing has no number to state; its image is the only
		// witness it has
		if number == "" && strings.HasPrefix(strings.ToUpper(card.Name), "DON!!") {
			return true
		}
		code := listedCodeRe.FindStringSubmatch(card.Name)
		if code != nil && sameCode(number, code[1]) {
			return true
		}
		field := strings.TrimSpace(card.Number)
		if field == "" {
			return false
		}
		// A bare "057" is the part of "OP06-057" after the set
		return sameCode(number, field) || strings.HasSuffix(number, "-"+field)
	case matcher.GameYuGiOh:
		listed := strings.ToUpper(strings.TrimSpace(card.Number))
		if listed == "" {
			return false
		}
		digits := trailingNumber(listed)
		if digits != "" && trailingNumber(number) == digits {
			return true
		}
		// The end of the code starts at a boundary: "1" is not the end of EN011
		head, found := strings.CutSuffix(strings.ToUpper(number), listed)
		return found && (head == "" || !isDigit(listed[0]) || !isDigit(head[len(head)-1]))
	case matcher.GamePokemon:
		listed := trailingNumber(card.Number)
		if card.Number == "" {
			listed = trailingNumber(skuDigits(card.SKU))
		}
		return listed != "" && trailingNumber(number) == listed
	}
	return false
}

// sameCode reports whether two collector codes are one, ignoring dashes.
func sameCode(a, b string) bool {
	return strings.EqualFold(strings.ReplaceAll(a, "-", ""), strings.ReplaceAll(b, "-", ""))
}

// trailingNumber returns the digits a collector number ends with, without
// their leading zeros: "SWSH144" and "BS 58" are 144 and 58, "058" is 58.
func trailingNumber(number string) string {
	i := len(number)
	for i > 0 && isDigit(number[i-1]) {
		i--
	}
	return strings.TrimLeft(number[i:], "0")
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
