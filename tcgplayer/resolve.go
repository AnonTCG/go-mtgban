package tcgplayer

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mtgban/go-mtgban/matcher"

	"github.com/mtgban/go-tcgplayer"
)

// errUnownedProduct is a product the datastore names no card for. The
// catalog lists every product the category sells, art cards and the like
// among them, so it is skipped without a word.
var errUnownedProduct = errors.New("no card owns the product")

// errRefusedProduct is a product id the datastore files under cards of more
// than one set (see crossSetProductIDs): pricing it would be a guess.
var errRefusedProduct = errors.New("product id claimed by more than one set")

// resolver ties a catalog sku, or a product's price row, to the printing it
// sells: its product id and finish name the printing, through the matcher,
// and its language the copy of that printing in it, where the datastore
// carries one.
type resolver struct {
	backend *matcher.Backend

	// languages spells each catalog language id the way the datastore
	// spells the language, for the languages the datastore carries.
	languages map[int]string
	english   int

	// refused are the product ids claimed by more than one set.
	refused map[string][]string

	// shared names, in set order, the printings that share a product id
	// and a finish, where the datastore files the id at one of them.
	shared map[string][]string

	// links ties each catalog product the datastore names no card for to
	// the card it sells; see catalogLinks.
	links map[int]string

	// copies names, by set, card name and language, the printings in a
	// language other than English that have no product of their own; see
	// languageCopies.
	copies map[string][]matcher.Card
}

// newResolver reads the catalog's language table against the languages the
// datastore carries. A catalog language the datastore carries none of is
// left out, and its skus resolve to nothing. Magic's datastore takes its
// product ids from Scryfall, which leaves some printings without one, so
// for Magic the catalog links stand in for them.
func newResolver(b *matcher.Backend, dump *tcgplayer.CatalogDump) (*resolver, error) {
	known := map[string]string{}
	for _, co := range b.UUIDs {
		known[tcgLanguageName(co.Language)] = co.Language
		for _, fd := range co.ForeignData {
			known[tcgLanguageName(fd.Language)] = fd.Language
		}
	}

	english, err := catalogEnglish(dump)
	if err != nil {
		return nil, err
	}
	r := resolver{
		backend:   b,
		english:   english,
		languages: map[int]string{},
		refused:   crossSetProductIDs(b),
		shared:    sharedProductIDs(b),
		copies:    languageCopies(b),
	}
	if b.Game == matcher.GameMagic {
		r.links = catalogLinks(b, dump)
	}
	for _, language := range dump.Languages {
		name, found := known[language.Name]
		if found {
			r.languages[language.LanguageID] = name
		}
	}
	return &r, nil
}

// catalogEnglish is the id the catalog dump's language table gives English.
func catalogEnglish(dump *tcgplayer.CatalogDump) (int, error) {
	for _, language := range dump.Languages {
		if language.Name == "English" {
			return language.LanguageID, nil
		}
	}
	return 0, errors.New("catalog dump lists no English")
}

// product resolves a product id in a finish, as a price row names it.
func (r *resolver) product(productID int, finish string) (string, error) {
	id := strconv.Itoa(productID)
	if r.refused[id] != nil {
		return "", errRefusedProduct
	}
	uuid := r.backend.ConvertID(matcher.IDSpaceTCGplayer, id)
	if uuid == "" {
		uuid = r.links[productID]
		if uuid == "" {
			return "", errUnownedProduct
		}
		id = uuid
	}
	// A product the datastore files at a sealed product sells no card
	co, err := r.backend.GetUUID(uuid)
	if err == nil && co.Sealed {
		return "", errUnownedProduct
	}
	// An etched product is sold etched alone, which TCGplayer calls
	// Foil; its sibling sold plain is a product of its own
	if err == nil && co.Etched && matcher.FinishSlug(finish) == matcher.FinishNonfoil {
		return "", matcher.ErrCardWrongFinish
	}
	// A product two printings share in one finish prices the first one
	// sold in it, in set order; a linked product, named by its card's
	// uuid by now, is never one
	for _, printing := range r.shared[id] {
		cardID, err := r.match(printing, finish)
		if err == nil {
			return cardID, nil
		}
	}
	return r.match(id, finish)
}

func (r *resolver) match(id, finish string) (string, error) {
	return r.backend.Match(&matcher.InputCard{
		ID:     id,
		Finish: finish,
		Foil:   finish != "Normal",
	})
}

// owns reports whether the product is one a card owns, claimed by the
// datastore or tied by a link, and not refused.
func (r *resolver) owns(productID int) bool {
	id := strconv.Itoa(productID)
	if r.refused[id] != nil {
		return false
	}
	uuid := r.backend.ConvertID(matcher.IDSpaceTCGplayer, id)
	if uuid == "" {
		return r.links[productID] != ""
	}
	co, err := r.backend.GetUUID(uuid)
	return err == nil && !co.Sealed
}

// sku resolves a sku: its product in its printing's finish, then that
// printing's copy in the sku's language. A sku in a language the printing
// has no copy in loaded is matcher.ErrCardUnknownID.
func (r *resolver) sku(sku tcgplayer.SKU, finish string) (string, error) {
	cardID, err := r.product(sku.ProductID, finish)
	if err != nil || sku.LanguageID == r.english {
		return cardID, err
	}
	language, found := r.languages[sku.LanguageID]
	if !found {
		return "", fmt.Errorf("%w: language %d", matcher.ErrCardUnknownID, sku.LanguageID)
	}
	co, _ := r.backend.GetUUID(cardID)
	copyID, err := r.backend.MatchIDLanguage(cardID, language)
	if err == nil {
		// The copy is found by flags, which settle for a finish it lacks
		copied, _ := r.backend.GetUUID(copyID)
		if copied.Foil != co.Foil || copied.Etched != co.Etched {
			return "", matcher.ErrCardWrongFinish
		}
		return copyID, nil
	}
	if !errors.Is(err, matcher.ErrCardUnknownID) {
		return "", err
	}
	// A copy filed as a printing of its own in the card's set, rather than
	// as a translation, is sold as this sku when it is the only one: a
	// printing numbered after the card's own, as #157s and #157★s are
	// 7th Edition #157's Chinese alternate arts
	base := strings.TrimSuffix(co.Number, "★")
	var hits []string
	for _, card := range r.copies[co.SetCode+"|"+co.Name+"|"+language] {
		if card.HasFinish(matcher.FinishSlug(finish)) && numberedAfter(card.Number, base) {
			hits = append(hits, card.UUID)
		}
	}
	if len(hits) != 1 {
		return "", err
	}
	return r.match(hits[0], finish)
}

// numberedAfter reports whether number is base with a suffix that does not
// carry on its digits: 157s and 157★s are numbered after 157, 1570 is not.
func numberedAfter(number, base string) bool {
	rest, found := strings.CutPrefix(number, base)
	if !found || rest == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return !unicode.IsDigit(r)
}

// SKUCards resolves every English singles sku the catalog dump lists, in a
// grade, to the printing it sells, by sku id: how a store reselling
// TCGplayer's own skus names its listings.
func SKUCards(b *matcher.Backend, dump *tcgplayer.CatalogDump) (map[int]string, error) {
	if dump == nil {
		return nil, errors.New("no catalog dump")
	}
	r, err := newResolver(b, dump)
	if err != nil {
		return nil, err
	}
	products, err := catalogProducts(dump, tcgplayer.SinglesProductTypes(dump.Category.CategoryID))
	if err != nil {
		return nil, err
	}
	printings := map[int]string{}
	for _, printing := range dump.Printings {
		printings[printing.PrintingID] = printing.Name
	}
	out := map[int]string{}
	for _, product := range products {
		for _, sku := range product.Skus {
			_, found := SKUConditionMap[sku.ConditionID]
			printing, named := printings[sku.PrintingID]
			if !found || !named || sku.LanguageID != r.english {
				continue
			}
			cardID, err := r.sku(sku, printing)
			if err == nil {
				out[sku.SKUID] = cardID
			}
		}
	}
	return out, nil
}

// crossSetProductIDs reports every TCGplayer product id (regular or etched)
// claimed by cards from more than one set, keyed to the sets that claim it.
// A double-faced card's two faces, or a plain misprint beside the copy it
// doubles, always share a set - a same-named oversized sibling does not, and
// neither does an id MTGJSON has simply copied onto the wrong printing (the
// Dungeon of the Mad Mage report this guards against: AFR's ordinary card
// and OAFR's oversized one both carry 245106, which belongs only to the
// oversized product). Spanning more than one set means the id is wrong on
// at least one of them, and pricing from it would be a guess.
func crossSetProductIDs(b *matcher.Backend) map[string][]string {
	setsByID := map[string]map[string]bool{}
	for _, code := range b.GetAllSets() {
		set, err := b.GetSet(code)
		if err != nil {
			continue
		}
		for _, card := range set.Cards {
			for _, key := range [...]string{"tcgplayerProductId", "tcgplayerEtchedProductId"} {
				id, found := card.Identifiers[key]
				if !found || soldAsCopy(b, id, card.Language) {
					continue
				}
				if setsByID[id] == nil {
					setsByID[id] = map[string]bool{}
				}
				setsByID[id][code] = true
			}
		}
	}

	collisions := map[string][]string{}
	for id, sets := range setsByID {
		if len(sets) <= 1 {
			continue
		}
		for code := range sets {
			collisions[id] = append(collisions[id], code)
		}
		sort.Strings(collisions[id])
	}
	return collisions
}

// sharedProductIDs names, for each TCGplayer product id two printings of
// one set and language share in a finish both are sold in, those printings
// in set order. A product a star foil shares with its card is not one: the
// finish tells the two apart.
func sharedProductIDs(b *matcher.Backend) map[string][]string {
	claims := map[string][]matcher.Card{}
	for _, code := range b.GetAllSets() {
		set, err := b.GetSet(code)
		if err != nil {
			continue
		}
		for _, card := range set.Cards {
			id := card.Identifiers["tcgplayerProductId"]
			if id != "" {
				claims[id] = append(claims[id], card)
			}
		}
	}
	shared := map[string][]string{}
	for id, cards := range claims {
		if len(cards) < 2 || !shareFinish(cards) {
			continue
		}
		for _, card := range cards {
			shared[id] = append(shared[id], card.UUID)
		}
	}
	return shared
}

// shareFinish reports whether the printings, all of one set and language,
// have a finish in common.
func shareFinish(cards []matcher.Card) bool {
	for _, card := range cards[1:] {
		if card.SetCode != cards[0].SetCode || card.Language != cards[0].Language {
			return false
		}
	}
	for _, finish := range cards[0].Finishes {
		for _, card := range cards[1:] {
			if card.HasFinish(finish) {
				return true
			}
		}
	}
	return false
}

// languageCopies names, by set, card name and language, the printings in
// a language other than English that have no product of their own, for the
// names only one printing of the set has a product for: TCGplayer sells such
// a printing as a sku in its language of that one product, as 7th Edition's
// Chinese alternate arts. A name more than one product shares is left out,
// since nothing says which of them sells the copy.
func languageCopies(b *matcher.Backend) map[string][]matcher.Card {
	copies := map[string][]matcher.Card{}
	for _, code := range b.GetAllSets() {
		set, err := b.GetSet(code)
		if err != nil {
			continue
		}
		// A star foil shares its card's product, so products are counted
		// by id
		products := map[string]map[string]bool{}
		for _, card := range set.Cards {
			id := card.Identifiers["tcgplayerProductId"]
			if id == "" {
				continue
			}
			if products[card.Name] == nil {
				products[card.Name] = map[string]bool{}
			}
			products[card.Name][id] = true
		}
		for _, card := range set.Cards {
			if card.Language == "English" || card.Identifiers["tcgplayerProductId"] != "" || len(products[card.Name]) != 1 {
				continue
			}
			key := code + "|" + card.Name + "|" + card.Language
			copies[key] = append(copies[key], card)
		}
	}
	return copies
}

// soldAsCopy reports whether a printing in language is a copy sold under id,
// a product the datastore files at its original in another language.
func soldAsCopy(b *matcher.Backend, id, language string) bool {
	original, err := b.GetUUID(b.ConvertID(matcher.IDSpaceTCGplayer, id))
	return err == nil && original.Language != language
}
