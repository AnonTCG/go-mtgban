package tcgplayer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/mtgban/go-tcgplayer"
)

// LoadCatalog reads a category's catalog dump, as go-tcgplayer's tcgdumper
// writes it each night: every product with its skus and extended data, the
// groups, and the category's printing, condition and language tables. The
// game datastores are built from the same dump, so it lists every product
// they can name.
func LoadCatalog(reader io.Reader) (*tcgplayer.CatalogDump, error) {
	var dump tcgplayer.CatalogDump
	err := json.NewDecoder(reader).Decode(&dump)
	if err != nil {
		return nil, err
	}
	if len(dump.Products) == 0 {
		return nil, errors.New("empty catalog dump")
	}
	return &dump, nil
}

// catalogProducts are the products the dump files under any of types, once
// each: the dumper lists a product once for every type it is filed under.
// A dump with none, as one written without the types would be, is refused
// rather than priced as an empty inventory.
func catalogProducts(dump *tcgplayer.CatalogDump, types []string) ([]tcgplayer.Product, error) {
	seen := map[int]bool{}
	var out []tcgplayer.Product
	for _, product := range dump.Products {
		if seen[product.ProductID] || !slices.Contains(types, product.ProductType) {
			continue
		}
		seen[product.ProductID] = true
		out = append(out, product)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("catalog dump lists no %q products for category %d", types, dump.Category.CategoryID)
	}
	return out, nil
}

// checkCatalog fails a dump of some other category than the scraper's.
func checkCatalog(dump *tcgplayer.CatalogDump, category int) error {
	if dump == nil {
		return errors.New("no catalog dump")
	}
	if dump.Category.CategoryID != category {
		return fmt.Errorf("catalog dump is of category %d (%s), not %d", dump.Category.CategoryID, dump.Category.Name, category)
	}
	return nil
}
