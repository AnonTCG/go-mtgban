package cardmarket

import (
	"strconv"
	"testing"

	cm "github.com/mtgban/go-cardmarket"

	"github.com/mtgban/go-mtgban/matcher"
)

// promoLink is one product a promo link table names, as Cardmarket
// catalogs it, and the printing it must land on.
type promoLink struct {
	mkmID                   int
	name, number, expansion string
	setCode, cardNumber     string
	rarity                  string
}

// checkPromoLinks resolves every product of a link table and checks it
// lands on its card, set, number and, where given, rarity; a table entry
// with no row here fails too.
func checkPromoLinks(t *testing.T, b *matcher.Backend, table map[int]int, links []promoLink) {
	t.Helper()
	mkm, err := NewScraperIndex(b)
	if err != nil {
		t.Fatalf("NewScraperIndex(b) = %v", err)
	}
	mkm.tcgBridge = map[int]int{}

	seen := map[int]bool{}
	for _, link := range links {
		seen[link.mkmID] = true
		_, found := table[link.mkmID]
		if !found {
			t.Errorf("product %d is not in the table", link.mkmID)
			continue
		}
		product := cm.Product{
			IDProduct:     link.mkmID,
			Name:          link.name,
			Number:        link.number,
			ExpansionName: link.expansion,
		}
		cardID, _, _, err := mkm.resolveProduct(&product)
		if err != nil {
			t.Errorf("resolveProduct(%d %q) = %v", link.mkmID, link.name, err)
			continue
		}
		co, err := b.GetUUID(cardID)
		if err != nil {
			t.Errorf("resolveProduct(%d %q) = %q, %v", link.mkmID, link.name, cardID, err)
			continue
		}
		name := versionTail.ReplaceAllString(link.name, "")
		if co.Name != name || co.SetCode != link.setCode || co.Number != link.cardNumber ||
			(link.rarity != "" && b.RarityLabel(co.Rarity) != link.rarity) {
			t.Errorf("product %d %q landed on %s %s %s %s, want %s %s %s",
				link.mkmID, link.name, co.Name, co.SetCode, co.Number, b.RarityLabel(co.Rarity),
				link.setCode, link.cardNumber, link.rarity)
		}
	}
	for mkmID := range table {
		if !seen[mkmID] {
			t.Errorf("table entry %d has no expected landing", mkmID)
		}
	}
}

// TestPokemonHoloLinks checks every pokemonHoloLinks entry lands on the
// Holofoil printing of the TCGplayer product it names.
func TestPokemonHoloLinks(t *testing.T) {
	b := loadPokemonBackend(t)
	mkm, err := NewScraperIndex(b)
	if err != nil {
		t.Fatalf("NewScraperIndex(b) = %v", err)
	}
	holofoil := matcher.FinishSlug("Holofoil")
	for mkmID, tcgID := range pokemonHoloLinks {
		cardID, err := mkm.pokemonByID(&cm.Product{IDProduct: mkmID})
		if err != nil {
			t.Errorf("pokemonByID(%d) = %v", mkmID, err)
			continue
		}
		co, err := b.GetUUID(cardID)
		if err != nil {
			t.Errorf("pokemonByID(%d) = %q, %v", mkmID, cardID, err)
			continue
		}
		if co.Finish != holofoil || co.Identifiers["tcgplayerProductId"] != strconv.Itoa(tcgID) {
			t.Errorf("product %d landed on %s (%s, tcgplayer %s), want the %s of %d",
				mkmID, co.Name, co.Finish, co.Identifiers["tcgplayerProductId"], holofoil, tcgID)
		}
	}
}

// TestPokemonStampLinks pins every pokemonStampLinks entry to the stamped
// printing its Cardmarket product sells.
func TestPokemonStampLinks(t *testing.T) {
	b := loadPokemonBackend(t)
	checkPromoLinks(t, b, pokemonStampLinks, []promoLink{
		{280585, "Beldum", "29", "EX Hidden Legends", "MCAP", "029", ""},
		{295325, "Shiinotic", "17", "Sun & Moon", "PR-1840", "017", ""},
		{295411, "Kangaskhan", "99", "Sun & Moon", "PR-1840", "099", ""},
		{295426, "Oranguru", "113", "Sun & Moon", "PR-1840", "113", ""},
		{275578, "Rainbow Energy", "", "Promos", "PR-1539", "", ""},
		{319255, "Alolan Vulpix", "21", "Guardians Rising", "MCAP", "021", ""},
		{359292, "Great Ball", "SUM 119", "SM Trainer Kit: Alolan Sandslash & Alolan Ninetales", "SMK2", "21", ""},
		{359295, "Great Ball", "SUM 119", "SM Trainer Kit: Alolan Sandslash & Alolan Ninetales", "SMK2", "119", ""},
		{359322, "Great Ball", "SUM 119", "SM Trainer Kit: Alolan Sandslash & Alolan Ninetales", "SMK2", "21", ""},
		{359325, "Great Ball", "SUM 119", "SM Trainer Kit: Alolan Sandslash & Alolan Ninetales", "SMK2", "119", ""},
		{371657, "Counter Catcher", "91", "Crimson Invasion", "PR-1539", "091", ""},
		{550116, "Flareon", "C27", "Battle Academy 2020", "BTA", "SM186", ""},
		{550326, "Potion", "MWT", "Battle Academy 2020", "BTA", "127", ""},
		{567076, "Ghetsis", "101", "Plasma Freeze", "PR-1840", "101", ""},
		{722693, "Ponyta", "14", "Generations", "MCAP", "014", ""},
		{722697, "Geodude", "43", "Generations", "GEN", "043", ""},
		{722699, "Meowth", "53", "Generations", "MCAP", "053", ""},
		{749041, "Forest Seal Stone", "156", "Silver Tempest", "PR-1840", "156", ""},
		{749909, "Iron Moth", "028", "Paradox Rift", "MCAP", "028", ""},
		{762531, "Koraidon", "119", "Temporal Forces", "MCAP", "119", ""},
		// TCGplayer numbers the card 177/182; it is Paradox Rift 171.
		{764550, "Professor Turo's Scenario", "PAR 171", "Professor Program", "PPP", "177", ""},
		{800144, "Pikachu", "051", "Temporal Forces", "MCAP", "051", ""},
		{800837, "Skeledirge", "038", "Scarlet & Violet", "MCAP", "038", ""},
		{804338, "Miraidon", "121", "Temporal Forces", "MCAP", "121", ""},
		{819347, "Hop's Dubwool", "136", "Journey Together: Additionals", "MCAP", "136", ""},
		{833946, "Team Rocket's Mimikyu", "087", "Destined Rivals: Additionals", "MCAP", "087", ""},
		{833950, "Flareon", "C44", "Battle Academy 2020", "BTA", "SM186", ""},
		{833952, "Eevee", "C36", "Battle Academy 2020", "BTA", "049", ""},
		{833969, "Switch", "C58", "Battle Academy 2020", "BTA", "132", ""},
		{834011, "Switch", "P52", "Battle Academy 2020", "BTA", "132", ""},
		{841258, "Zekrom", "066", "Paradox Rift", "MCAP", "066", ""},
		{841273, "Latias ex", "076", "Surging Sparks", "MCAP", "076", ""},
		{855028, "Air Balloon", "BLK 079", "Southeast Asia Promos", "SEA", "079", ""},
		{855691, "Wally's Compassion", "MEG 132", "Southeast Asia Promos", "SEA", "132", ""},
		{858716, "Tyranitar", "135", "Paldea Evolved", "MCAP", "135", ""},
		{858724, "Sinistcha", "022", "Twilight Masquerade", "BLE", "022", ""},
		{858734, "Sylveon", "040", "Prismatic Evolutions: Additionals", "MCAP", "040", ""},
		{862171, "Team Rocket's Zapdos", "070", "Destined Rivals: Additionals", "MCAP", "070", ""},
		{868033, "Dual Ball", "UL 72", "WCD 2012", "WCD2012", "78", ""},
		{868432, "Lillie's Determination", "MEG 119", "Southeast Asia Promos", "SEA", "119", ""},
		{882196, "Riolu", "61", "Diamond & Pearl", "CCP", "061", ""},
		{882197, "Glameow", "83", "Diamond & Pearl", "CCP", "083", ""},
		{882198, "Skorupi", "99", "Diamond & Pearl", "CCP", "099", ""},
		{882199, "Stunky", "102", "Diamond & Pearl", "CCP", "102", ""},
		{882200, "Abomasnow", "19", "Mysterious Treasures", "CCP", "019", ""},
		{882201, "Chingling", "42", "Mysterious Treasures", "CCP", "042", ""},
		{882202, "Gabite", "48", "Mysterious Treasures", "CCP", "048", ""},
		{882203, "Happiny", "52", "Mysterious Treasures", "CCP", "052", ""},
		{882205, "Bidoof", "73", "Mysterious Treasures", "CCP", "073", ""},
		{882207, "Bronzor", "74", "Mysterious Treasures", "CCP", "074", ""},
		{882210, "Kricketot", "86", "Mysterious Treasures", "CCP", "086", ""},
		{882214, "Shinx", "98", "Mysterious Treasures", "CCP", "098", ""},
		{882215, "Snover", "101", "Mysterious Treasures", "CCP", "101", ""},
		{882216, "Weavile", "40", "Secret Wonders", "CCP", "040", ""},
		{882217, "Burmy", "78", "Secret Wonders", "CCP", "078", ""},
		{882218, "Floatzel", "37", "Great Encounters", "CCP", "037", ""},
		{882219, "Buizel", "61", "Great Encounters", "CCP", "061", ""},
		{882220, "Glaceon", "20", "Majestic Dawn", "CCP", "020", ""},
		{882221, "Chimchar", "57", "Majestic Dawn", "CCP", "057", ""},
		{882222, "Combee", "59", "Majestic Dawn", "CCP", "059", ""},
		{882223, "Munchlax", "68", "Majestic Dawn", "CCP", "068", ""},
		{882224, "Piplup", "72", "Majestic Dawn", "CCP", "072", ""},
		{882225, "Turtwig", "78", "Majestic Dawn", "CCP", "078", ""},
		{882856, "Bayleef", "35", "EX Unseen Forces", "PR-1423", "35", ""},
		{894193, "Zekrom ex", "BLK 034", "Play! Pokémon Prize Pack Series Nine", "G22880", "034", ""},
		{901195, "Ho-Oh", "27", "EX Unseen Forces", "PR-1840", "027", ""},
		{916454, "Air Balloon", "BLK 079", "Southeast Asia Promos", "SEA", "079", ""},
		{916463, "Lillie's Determination", "MEG 119", "Southeast Asia Promos", "SEA", "119", ""},
		{916469, "Wally's Compassion", "MEG 132", "Southeast Asia Promos", "SEA", "132", ""},
		{917602, "Pokemon Run Pikachu", "131", "MEP Black Star Promos", "SEA", "131", ""},
	})
}

// TestYugiohPromoLinks pins every yugiohPromoLinks entry to the Reshef of
// Destruction promo or Hidden Arsenal Duel Terminal copy its Cardmarket
// product sells. The id skips the number and other-card checks, so the rarity
// is held too.
func TestYugiohPromoLinks(t *testing.T) {
	b := loadYugiohBackend(t)
	checkPromoLinks(t, b, yugiohPromoLinks, []promoLink{
		{244606, "Dark Magician Knight (V.1 - Secret Rare)", "EN1", "Reshef of Destruction Promos", "VDP", "ROD-EN001", "Secret Rare"},
		{244607, "Knight's Title (V.1 - Secret Rare)", "EN1", "Reshef of Destruction Promos", "VDP", "ROD-EN002", "Secret Rare"},
		{363275, "Knight's Title (V.2 - Super Rare)", "001", "Reshef of Destruction Promos", "VDP", "ROD-EN002", "Super Rare"},
		{363276, "Dark Magician Knight (V.2 - Super Rare)", "001", "Reshef of Destruction Promos", "VDP", "ROD-EN001", "Super Rare"},
		{611464, "Ally of Justice Searcher (V.2 - Parallel Rare)", "081", "Hidden Arsenal: Chapter 1", "HAC1", "HAC1-EN081", "Duel Terminal Technology Common"},
		{611467, "Ally of Justice Reverse Break (V.2 - Parallel Rare)", "085", "Hidden Arsenal: Chapter 1", "HAC1", "HAC1-EN085", "Duel Terminal Technology Common"},
		{611468, "Ally of Justice Unlimiter (V.2 - Parallel Rare)", "086", "Hidden Arsenal: Chapter 1", "HAC1", "HAC1-EN086", "Duel Terminal Technology Common"},
	})
}
