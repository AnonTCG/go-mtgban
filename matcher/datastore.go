package matcher

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"sort"
	"strings"
)

// A datastore entry's id is opaque. The builder that publishes it spells it,
// and no loader here reads one apart: what an id could be taken apart for -
// which product an entry belongs to, which printing of that product it is -
// every datastore publishes as a field of its own, so a loader asks for the
// fact instead of inferring it from a shape.
//
// This is not a style preference. A loader recovering an entry's product by
// trimming a literal tail off its id breaks as soon as a builder spells that
// tail from the printing's own name - "_holo" as "_holofoil", "_1e" as
// "_1stedition": the trimming stops matching and printings of one product
// become separate cards. Nothing throws, since the ids stay well formed, unique
// and stable. A grouping read off a published field
// cannot fail that way, and it leaves the builder free to respell an id, or
// to stop spelling it out of anything at all.

// GameLoader builds a Backend from a datastore reader for a particular game.
type GameLoader func(io.Reader) (*Backend, error)

// NewBackend returns an empty Backend for a datastore game's loader to fill,
// with every map it and the Add helpers file into made: UUIDs, Hashes,
// CanonicalNames, PromoTypeLabels, RarityLabels, Sets and SetSealedUUIDs.
// ExternalIdentifiers holds an empty index for each id space given, the ones
// the game's datastore publishes ids in, and no other.
func NewBackend(spaces ...IDSpace) *Backend {
	b := &Backend{
		UUIDs:               map[string]*CardObject{},
		Hashes:              map[string][]string{},
		CanonicalNames:      map[string]string{},
		PromoTypeLabels:     map[string]string{},
		RarityLabels:        map[string]string{},
		Sets:                map[string]*Set{},
		SetSealedUUIDs:      map[string][]string{},
		ExternalIdentifiers: map[IDSpace]map[string]string{},
	}
	for _, space := range spaces {
		b.ExternalIdentifiers[space] = map[string]string{}
	}
	return b
}

// Complete readies a Backend a datastore loader has filled with every set,
// card and sealed product: AllSets and the name, promo type and sealed lists
// are put in order, NormalizedSets and SetUUIDs are built, every entry of a
// name is given the Printings of the name, and the game's rules are
// attached, last because SetRules reads every printing's finishes.
func (b *Backend) Complete(rules GameRules) {
	sort.Strings(b.AllSets)
	b.IndexSets()
	sort.Strings(b.AllNames)
	sort.Strings(b.AllCanonicalNames)
	sort.Strings(b.AllLowerNames)
	sort.Strings(b.AllPromoTypes)
	b.SortSealed()
	b.IndexSetUUIDs()
	b.IndexRarities()
	b.sharePrintings()
	b.SetRules(rules)
}

// sharePrintings gives every entry of a name the sets any entry of that name
// is printed in, in the order the first one lists them: variants of a name
// can be reprinted apart (Unstable's Ineffable Blessing), and Printings
// answers for the name. A token and a card of one name are kept apart.
// Every entry of a name then holds one shared list, so it is read-only.
func (b *Backend) sharePrintings() {
	for norm, uuids := range b.Hashes {
		var groups [][]*CardObject
		for _, uuid := range uuids {
			co, found := b.UUIDs[uuid]
			if !found || Normalize(co.Name) != norm {
				continue
			}
			i := slices.IndexFunc(groups, func(group []*CardObject) bool {
				return strings.EqualFold(group[0].Name, co.Name) && (group[0].Layout == "token") == (co.Layout == "token")
			})
			if i < 0 {
				i = len(groups)
				groups = append(groups, nil)
			}
			groups[i] = append(groups[i], co)
		}
		for _, group := range groups {
			printings := group[0].Printings
			for _, co := range group[1:] {
				if slices.Equal(co.Printings, group[0].Printings) {
					continue
				}
				for _, code := range co.Printings {
					if !slices.Contains(printings, code) {
						printings = append(slices.Clip(printings), code)
					}
				}
			}
			for _, co := range group {
				co.Printings = printings
			}
		}
	}
}

// ProductKeyOf names the product a stored card is a printing of: the
// TCGplayer product id the datastore stamps on every printing it sells,
// and the entry's own uuid where it stamps none - an entry the builder
// mints is minted one printing at a time, so it is a product of one
// printing and stands for itself. The games whose datastores sell each
// finish as an entry of its own fold their candidates on it; see the note
// at the top of this file for why the uuid is never taken apart instead.
func ProductKeyOf(identifiers map[string]string, uuid string) string {
	if id := identifiers["tcgplayerProductId"]; id != "" {
		return id
	}
	return uuid
}

// GroupProducts gathers the entries of a datastore that sells each printing
// as an entry of its own into the products they are printings of, by the key
// each entry names its product with, read off what it publishes (see the note
// at the top of this file). Products come in the order each first appears,
// and each keeps its entries in the datastore's order.
func GroupProducts[T any](entries []T, key func(*T) string) [][]*T {
	var products [][]*T
	at := map[string]int{}
	for i := range entries {
		entry := &entries[i]
		k := key(entry)
		n, found := at[k]
		if !found {
			n = len(products)
			at[k] = n
			products = append(products, nil)
		}
		products[n] = append(products[n], entry)
	}
	return products
}

// SoldFinishes reads the printings a product is sold as, the uuid of each
// keyed by its finish (FinishSlug), into the Card fields saying so. finishes
// are the flags a storefront can raise for it, nonfoil then foil, each only
// where a printing of that foilness is sold, since output() folds an
// unreliable flag onto a class the product does sell. foilUUIDs holds every
// printing's uuid under its own finish, and each flag's under the printing
// DefaultPrinting answers it with.
func SoldFinishes(printings map[string]string) (finishes []string, foilUUIDs map[string]string) {
	foilUUIDs = map[string]string{}
	for _, flag := range []string{FinishNonfoil, FinishFoil} {
		uuid, found := DefaultPrinting(printings, flag == FinishFoil)
		if found {
			finishes = append(finishes, flag)
			foilUUIDs[flag] = uuid
		}
	}
	maps.Copy(foilUUIDs, printings)
	return finishes, foilUUIDs
}

// ColorNames is what a datastore lists as a card's colours - its colours,
// pitches, attributes or types, whatever the game calls them - in lower
// case as Magic names its colours, empty values dropped.
func ColorNames(values []string) []string {
	var names []string
	for _, value := range values {
		name := strings.ToLower(strings.TrimSpace(value))
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// SortByOrder sorts values the way order lists them, and those order leaves
// out after them alphabetically: all of them alphabetically for no order.
func SortByOrder(values, order []string) {
	position := func(value string) int {
		i := slices.Index(order, value)
		if i < 0 {
			return len(order)
		}
		return i
	}
	sort.Strings(values)
	sort.SliceStable(values, func(i, j int) bool {
		return position(values[i]) < position(values[j])
	})
}

// RaritiesOf lists the rarities of a set's cards once each, the way the set
// lists them: spelled by RarityName, in the game's order, rarest first.
func RaritiesOf(cards []Card, order []string) []string {
	var rarities []string
	for _, card := range cards {
		rarity := RarityName(card.Rarity)
		if !slices.Contains(rarities, rarity) {
			rarities = append(rarities, rarity)
		}
	}
	SortByOrder(rarities, order)
	return rarities
}

// RarityName spells a rarity the way a card carries it, a set lists it and a
// search compares it: lower case, without spaces, so "Super Rare" is
// "superrare" and a rarity is one word to a query. RarityLabels keeps the
// words a datastore published it as.
func RarityName(rarity string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(rarity)), " ", "")
}

// RarityNames spells each of a datastore's rarities by RarityName, empty
// values dropped.
func RarityNames(values []string) []string {
	var names []string
	for _, value := range values {
		name := RarityName(value)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// ColorsOf lists the colours of a set's cards once each, the way Magic's
// sets list theirs: the colours in the game's order, those it leaves out
// after them alphabetically; then "colorless" where a card carries none or
// names it, and "multicolor" where one carries more than one.
func ColorsOf(cards []Card, order []string) []string {
	var colors []string
	var colorless, multicolor bool
	for _, card := range cards {
		colorless = colorless || len(card.Colors) == 0
		multicolor = multicolor || len(card.Colors) > 1
		for _, color := range card.Colors {
			if color == "colorless" {
				colorless = true
				continue
			}
			if !slices.Contains(colors, color) {
				colors = append(colors, color)
			}
		}
	}
	SortByOrder(colors, order)
	if colorless {
		colors = append(colors, "colorless")
	}
	if multicolor {
		colors = append(colors, "multicolor")
	}
	return colors
}

type registeredGame struct {
	name Game
	load GameLoader
}

var registeredGames []registeredGame

// RegisterGame registers a game's datastore loader under a unique name, in the
// style of database/sql's Register. Game packages (matcher/magic,
// matcher/lorcana) call this from their init(); a consumer activates a game
// with a blank import, e.g. import _ "github.com/mtgban/go-mtgban/matcher/magic".
// It panics on a duplicate name or a nil loader.
func RegisterGame(name Game, load GameLoader) {
	if load == nil {
		panic("matcher: RegisterGame loader is nil for " + name)
	}
	for _, g := range registeredGames {
		if g.name == name {
			panic("matcher: RegisterGame called twice for " + name)
		}
	}
	registeredGames = append(registeredGames, registeredGame{name: name, load: load})
}

// RegisteredGames returns the names of the registered games in registration
// order.
func RegisteredGames() []Game {
	names := make([]Game, len(registeredGames))
	for i, g := range registeredGames {
		names[i] = g.name
	}
	return names
}

// Open loads the named game's datastore explicitly (sql.Open style) and
// returns the Backend with its sealed index built: a loader that does not
// call SortSealed would otherwise leave ResolveSealed rebuilding the index
// on every call.
func Open(name Game, reader io.Reader) (*Backend, error) {
	for _, g := range registeredGames {
		if g.name == name {
			b, err := g.load(reader)
			if err != nil {
				return nil, err
			}
			b.Game = name
			if b.sealedIdx == nil {
				b.sealedIdx = b.buildSealedIndex()
			}
			return b, nil
		}
	}
	return nil, fmt.Errorf("matcher: unknown game %q (registered: %v)", name, RegisteredGames())
}
