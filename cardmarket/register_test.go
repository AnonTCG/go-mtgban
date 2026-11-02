package cardmarket

import (
	"go/importer"
	"go/token"
	"go/types"
	"testing"

	cm "github.com/mtgban/go-cardmarket"

	"github.com/mtgban/go-mtgban/mtgban"
)

// secretsAsked records every secret a constructor asks for.
type secretsAsked []string

func (s *secretsAsked) Secret(name string) (string, error) {
	*s = append(*s, name)
	return "set", nil
}

// TestIndexMakesNoAuthenticatedCall pins a golden: Index prices from the
// published id map and the public price guide alone. Offered every secret,
// it asks for none and holds no client to spend one with.
func TestIndexMakesNoAuthenticatedCall(t *testing.T) {
	b := datastoreBackend(t, "lorcana", foilOnlyDatastore)
	var asked secretsAsked
	_, err := mtgban.NewScraper(b, "cardmarket", mtgban.WithAuthenticator(&asked), WithCatalog(&cm.Catalog{}))
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) > 0 {
		t.Errorf("Index asked for %v", asked)
	}

	// The fields are read by the type checker, not at run time. Market
	// holds a client, which shows the walk can still see one.
	pkg, err := importer.ForCompiler(token.NewFileSet(), "source", nil).Import("github.com/mtgban/go-mtgban/cardmarket")
	if err != nil {
		t.Fatal(err)
	}
	if clientPath(pkg.Scope().Lookup("Market").Type(), "Market", map[types.Type]bool{}) == "" {
		t.Fatal("the walk no longer finds the client Market holds")
	}
	path := clientPath(pkg.Scope().Lookup("Index").Type(), "Index", map[types.Type]bool{})
	if path != "" {
		t.Errorf("Index holds a Cardmarket client at %s", path)
	}
}

// clientPath names the first field under t that holds a Cardmarket client.
func clientPath(t types.Type, path string, seen map[types.Type]bool) string {
	// Pointers, slices, arrays, maps and channels hold what their Elem is.
	for {
		container, ok := t.(interface{ Elem() types.Type })
		if !ok {
			break
		}
		t = container.Elem()
	}
	named, ok := t.(*types.Named)
	if ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "github.com/mtgban/go-cardmarket" && named.Obj().Name() == "Client" {
		return path
	}
	if seen[t] {
		return ""
	}
	seen[t] = true
	fields, ok := t.Underlying().(*types.Struct)
	if !ok {
		return ""
	}
	for i := range fields.NumFields() {
		field := fields.Field(i)
		found := clientPath(field.Type(), path+"."+field.Name(), seen)
		if found != "" {
			return found
		}
	}
	return ""
}

// TestEveryGameHasAResolver keeps gameResolvers in step with the games the
// scrapers are built for, which would otherwise refuse every product.
func TestEveryGameHasAResolver(t *testing.T) {
	for game, id := range mkmGames {
		_, found := gameResolvers[id]
		if !found {
			t.Errorf("%s has no resolver in gameResolvers", game)
		}
	}
}
