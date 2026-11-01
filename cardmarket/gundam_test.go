package cardmarket

import (
	"testing"

	cm "github.com/mtgban/go-cardmarket"

	_ "github.com/mtgban/go-mtgban/matcher/gundam"
)

// gundamDatastore is the published Gundam datastore cut down to the rows
// these tests turn on, images left out.
const gundamDatastore = `{"data": {
 "game": "gundam",
 "sets": {
  "GCG-PR": {"name": "Gundam Promotional Cards", "releaseDate": "2025-03-01", "type": "promo"},
  "GD01": {"name": "Newtype Rising", "releaseDate": "2025-07-25"},
  "GD01-B": {"name": "Edition Beta", "releaseDate": "2025-02-06"},
  "GD02": {"name": "Dual Impact", "releaseDate": "2025-10-24"},
  "GD03": {"name": "Steel Requiem", "releaseDate": "2026-01-30"},
  "GD05": {"name": "Freedom Ascension", "releaseDate": "2026-07-24"},
  "GD06": {"name": "Stardust Trails", "releaseDate": "2026-10-30"},
  "ST03": {"name": "Starter Deck 03: Zeon's Rush", "releaseDate": "2025-07-11"},
  "ST09": {"name": "Starter Deck 09: Destiny Ignition", "releaseDate": "2026-03-27"},
  "ST12": {"name": "Starter Deck 12: Raging Onslaught", "releaseDate": "2026-09-25"}
 },
 "cards": [
  {"color": "White", "colors": ["White"], "externalLinks": {"tcgPlayerId": 655173}, "finish": "Holofoil", "id": "gd02-129_655173_holofoil", "name": "Argama", "number": "GD02-129", "rarity": "Rare", "setCode": "GD02", "type": "Base"},
  {"color": "White", "colors": ["White"], "externalLinks": {"tcgPlayerId": 659358}, "finish": "Holofoil", "id": "gd02-129_659358_holofoil", "name": "Argama", "number": "GD02-129", "rarity": "R+", "setCode": "GD02", "type": "Base"},
  {"color": "White", "colors": ["White"], "externalLinks": {"tcgPlayerId": 727898}, "finish": "Holofoil", "id": "gd02-129_727898_holofoil", "name": "Argama", "number": "GD02-129", "promoTypes": ["sp"], "rarity": "R+", "setCode": "GD06", "type": "Base", "variant": "SP"},
  {"color": "Blue", "colors": ["Blue"], "externalLinks": {"tcgPlayerId": 643150}, "finish": "Holofoil", "id": "gd01-001_643150_holofoil", "name": "Gundam", "number": "GD01-001", "rarity": "Legend Rare", "setCode": "GD01", "type": "Unit"},
  {"color": "Blue", "colors": ["Blue"], "externalLinks": {"tcgPlayerId": 645356}, "finish": "Holofoil", "id": "gd01-001_645356_holofoil", "name": "Gundam", "number": "GD01-001", "rarity": "LR+", "setCode": "GD01", "type": "Unit"},
  {"color": "Blue", "colors": ["Blue"], "externalLinks": {"tcgPlayerId": 645375}, "finish": "Holofoil", "id": "gd01-001_645375_holofoil", "name": "Gundam", "number": "GD01-001", "rarity": "LR++", "setCode": "GD01", "type": "Unit"},
  {"color": "Green", "colors": ["Green"], "externalLinks": {"tcgPlayerId": 670499}, "finish": "Holofoil", "id": "gd03-021_670499_holofoil", "name": "Gundam Deathscythe Hell", "number": "GD03-021", "rarity": "Rare", "setCode": "GD03", "type": "Unit"},
  {"color": "Green", "colors": ["Green"], "externalLinks": {"tcgPlayerId": 670579}, "finish": "Holofoil", "id": "gd03-021_670579_holofoil", "name": "Gundam Deathscythe Hell", "number": "GD03-021", "promoTypes": ["storetournament", "participationpack"], "rarity": "Rare", "setCode": "GCG-PR", "type": "Unit", "variant": "Store Tournament Participant Pack 03"},
  {"color": "Green", "colors": ["Green"], "externalLinks": {"tcgPlayerId": 670580}, "finish": "Holofoil", "id": "gd03-021_670580_holofoil", "name": "Gundam Deathscythe Hell", "number": "GD03-021", "promoTypes": ["storetournament", "winnerpack"], "rarity": "R+", "setCode": "GCG-PR", "type": "Unit", "variant": "Store Tournament Winner Pack 03"},
  {"externalLinks": {"tcgPlayerId": 673538}, "finish": "Normal", "id": "t-020_673538", "name": "GFreD Token", "number": "T-020", "rarity": "Common", "setCode": "GD03", "type": "Unit"},
  {"externalLinks": {"tcgPlayerId": 680689}, "finish": "Holofoil", "id": "t-020_680689_holofoil", "name": "GFreD Token", "number": "T-020", "promoTypes": ["premiumcardcollection"], "rarity": "Common", "setCode": "GCG-PR", "type": "Unit", "variant": "Premium Card Collection Gundam Assemble"},
  {"color": "Green", "colors": ["Green"], "externalLinks": {"tcgPlayerId": 641505}, "finish": "Holofoil", "id": "st03-006_641505_holofoil", "name": "Char's Zaku II", "number": "ST03-006", "rarity": "Legend Rare", "setCode": "ST03", "type": "Unit"},
  {"color": "Green", "colors": ["Green"], "finish": "Holofoil", "id": "st03-006-premium-card-collection-02_holofoil", "name": "Char's Zaku II", "number": "ST03-006", "promoTypes": ["premiumcardcollection"], "rarity": "Legend Rare", "setCode": "GCG-PR", "type": "Unit", "variant": "Premium Card Collection 02"},
  {"color": "Green", "colors": ["Green"], "externalLinks": {"tcgPlayerId": 616640}, "finish": "Holofoil", "id": "gd01-026_616640_holofoil", "name": "Char's Zaku II", "number": "GD01-026", "rarity": "Rare", "setCode": "GD01-B", "type": "Unit"},
  {"color": "Green", "colors": ["Green"], "externalLinks": {"tcgPlayerId": 616641}, "finish": "Holofoil", "id": "gd01-026_616641_holofoil", "name": "Char's Zaku II", "number": "GD01-026", "rarity": "R+", "setCode": "GD01-B", "type": "Unit"},
  {"color": "Red", "colors": ["Red"], "externalLinks": {"tcgPlayerId": 716381}, "finish": "Normal", "id": "st12-002_716381", "name": "Shining Gundam", "number": "ST12-002", "rarity": "Common", "setCode": "ST12", "type": "Unit"},
  {"color": "Red", "colors": ["Red"], "externalLinks": {"tcgPlayerId": 721022}, "finish": "Holofoil", "id": "st12-002_721022_holofoil", "name": "Shining Gundam", "number": "ST12-002", "rarity": "C+", "setCode": "ST12", "type": "Unit"},
  {"color": "Purple", "colors": ["Purple"], "externalLinks": {"tcgPlayerId": 684001}, "finish": "Holofoil", "id": "st09-002_684001_holofoil", "name": "Force Impulse Gundam", "number": "ST09-002", "rarity": "Legend Rare", "setCode": "ST09", "type": "Unit"},
  {"color": "Purple", "colors": ["Purple"], "externalLinks": {"tcgPlayerId": 684026}, "finish": "Holofoil", "id": "st09-001_684026_holofoil", "name": "Impulse Gundam", "number": "ST09-001", "rarity": "LR+", "setCode": "ST09", "type": "Unit"},
  {"color": "Purple", "colors": ["Purple"], "externalLinks": {"tcgPlayerId": 705650}, "finish": "Holofoil", "id": "gd05-114_705650_holofoil", "name": "Widespread Annihilation", "number": "GD05-114", "rarity": "Rare", "setCode": "GD05", "type": "Command"},
  {"color": "Purple", "colors": ["Purple"], "finish": "Holofoil", "id": "gd05-114-premium-card-collection-02_holofoil", "name": "Widespread Annihilation", "number": "GD05-114", "promoTypes": ["premiumcardcollection"], "rarity": "Rare", "setCode": "GCG-PR", "type": "Command", "variant": "Premium Card Collection 02"}
 ]
}}`

// TestGundamResolve pins how a Cardmarket Gundam product finds its printing:
// the parallel by its rarity spelled the catalog's way, a CardTrader link
// held to the number the product's name writes rather than to its spelling,
// a Premium Card Collection 02 print by its product id, and a promo shelf's
// product on the promo set's row rather than the main set's. The products
// and links are the catalog's and CardTrader's own.
func TestGundamResolve(t *testing.T) {
	b := datastoreBackend(t, "gundam", gundamDatastore)

	for _, tt := range []struct {
		desc    string
		id      int
		product cm.CatalogProduct
		shelf   string
		bridge  map[int]int
		want    string
	}{
		{
			"a parallel by its rarity", 905530,
			cm.CatalogProduct{Name: "Gundam (GD01-001) (V.2 - Legendary Rare +)", Number: "001", Rarity: "Legendary Rare +", Version: 2},
			"Newtype Rising", nil, "gd01-001_645356_holofoil",
		},
		{
			"the second parallel", 905531,
			cm.CatalogProduct{Name: "Gundam (GD01-001) (V.3 - Legendary Rare ++)", Number: "001", Rarity: "Legendary Rare ++", Version: 3},
			"Newtype Rising", nil, "gd01-001_645375_holofoil",
		},
		{
			"a link to another number is refused", 906571,
			cm.CatalogProduct{Name: "Force Impulse Gundam (ST09-002) (V.1 - Legendary Rare)", Number: "002", Rarity: "Legendary Rare", Version: 1},
			"Starter Deck: Destiny Ignition", map[int]int{906571: 684026}, "st09-002_684001_holofoil",
		},
		{
			"a link at the product's number stands, spelled apart", 908561,
			cm.CatalogProduct{Name: "GFreD (T-020) (V.1 - Token)", Number: "T-020", Rarity: "Token", Version: 1},
			"Premium Bandai Products", map[int]int{908561: 680689}, "t-020_680689_holofoil",
		},
		{
			"a Premium Card Collection 02 print", 908747,
			cm.CatalogProduct{Name: "Widespread Annihilation (GD05-114) (V.1 - Rare)", Number: "GD05-114", Rarity: "Rare", Version: 1},
			"Premium Bandai Products", nil, "gd05-114-premium-card-collection-02_holofoil",
		},
		{
			"a number contradicting the code in the name", 908748,
			cm.CatalogProduct{Name: "Char's Zaku II (ST03-006) (V.1 - Legendary Rare)", Number: "GD01-026", Rarity: "Legendary Rare", Version: 1},
			"Premium Bandai Products", nil, "st03-006-premium-card-collection-02_holofoil",
		},
		{
			"a parallel Cardmarket's version text names once more", 914345,
			cm.CatalogProduct{Name: "Shining Gundam (ST12-002) (V.2 - Common +)", Number: "002", Rarity: "Common +", Version: 2},
			"Starter Deck: Raging Onslaught", nil, "st12-002_721022_holofoil",
		},
		{
			"a promo shelf reads the promo set", 913375,
			cm.CatalogProduct{Name: "Gundam Deathscythe Hell (GD03-021) (V.1 - Rare)", Number: "GD03-021", Rarity: "Rare", Version: 1},
			"Unnumbered Promos", nil, "gd03-021_670579_holofoil",
		},
		{
			"a Special Rare reprint reads the sp row", 920585,
			cm.CatalogProduct{Name: "Argama (GD02-129)", Number: "GD02-129", Rarity: "Special Rare"},
			"Stardust Trails", nil, "gd02-129_727898_holofoil",
		},
		{
			"two links crossed by CardTrader are read the other way", 905979,
			cm.CatalogProduct{Name: "Char's Zaku II (GD01-026) (V.1 - Rare +)", Number: "GD01-026", Rarity: "Rare +", Version: 1},
			"Edition Beta", map[int]int{905979: 616640}, "gd01-026_616641_holofoil",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			mkm, err := NewScraperIndex(b)
			if err != nil {
				t.Fatalf("NewScraperIndex(b) = %v", err)
			}
			mkm.tcgBridge = tt.bridge
			got := mkm.resolveMapped(tt.id, tt.product, cm.Expansion{Name: tt.shelf})
			if got.err != nil || got.cardID != tt.want {
				t.Errorf("%d %q: resolveMapped = (%q, %v), want %q", tt.id, tt.product.Name, got.cardID, got.err, tt.want)
			}
		})
	}
}
