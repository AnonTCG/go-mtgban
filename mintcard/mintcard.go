// Package mintcard scrapes MTG Mint Card.
package mintcard

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/mtgban"
	"github.com/mtgban/go-mtgban/tcgplayer"

	api "github.com/mtgban/go-tcgplayer"
)

// MTGMintCard prices MTG Mint Card's singles, both what they sell and what
// they buy.
type MTGMintCard struct {
	logCallback mtgban.LogCallbackFunc
	partner     string

	inventoryDate time.Time
	buylistDate   time.Time

	inventory mtgban.InventoryRecord
	buylist   mtgban.BuylistRecord

	// catalog is TCGplayer's catalog dump, which names the sku a listing
	// carries.
	catalog *api.CatalogDump

	backend *matcher.Backend
}

// NewScraper returns a scraper.
func NewScraper(b *matcher.Backend) *MTGMintCard {
	mint := MTGMintCard{}
	mint.backend = b
	mint.inventory = mtgban.InventoryRecord{}
	mint.buylist = mtgban.BuylistRecord{}
	return &mint
}

func (mint *MTGMintCard) printf(format string, a ...any) {
	if mint.logCallback != nil {
		mint.logCallback("[MMC] "+format, a...)
	}
}

// onlyTheList reports whether err is an aliasing error whose candidates are
// all The List printings.
func (mint *MTGMintCard) onlyTheList(err error) bool {
	var alias *matcher.AliasingError
	if !errors.As(err, &alias) || len(alias.Dupes) == 0 {
		return false
	}
	for _, probe := range alias.Probe() {
		card, uuidErr := mint.backend.GetUUID(probe)
		if uuidErr != nil || card.SetCode != "PLST" {
			return false
		}
	}
	return true
}

// soldInFinish answers an aliasing error with the one candidate sold in the
// finish the listing names, and returns "" when none or several are, or when
// the listing is etched.
func (mint *MTGMintCard) soldInFinish(theCard *matcher.InputCard, err error) string {
	var alias *matcher.AliasingError
	if theCard.IsEtched() || !errors.As(err, &alias) {
		return ""
	}
	found := ""
	for _, probe := range alias.Probe() {
		co, uuidErr := mint.backend.GetUUID(probe)
		if uuidErr != nil || co.Foil != theCard.Foil || co.Etched {
			continue
		}
		if found != "" {
			return ""
		}
		found = probe
	}
	return found
}

// lumpedArts reports whether err is an aliasing error over several arts of
// one basic land in one set, for a listing that gave no number: one product
// stocking every art of a basic names none of them.
func (mint *MTGMintCard) lumpedArts(card Card, theCard *matcher.InputCard, err error) bool {
	var alias *matcher.AliasingError
	if !errors.As(err, &alias) || len(alias.Dupes) == 0 || card.Number != "" || theCard.Variation != "" {
		return false
	}
	setCode := ""
	for _, probe := range alias.Probe() {
		co, uuidErr := mint.backend.GetUUID(probe)
		if uuidErr != nil || !matcher.Equals(co.Name, theCard.Name) || !slices.Contains(co.Supertypes, "Basic") {
			return false
		}
		if setCode != "" && co.SetCode != setCode {
			return false
		}
		setCode = co.SetCode
	}
	return true
}

func (mint *MTGMintCard) processEntry(sku2uuid map[int]string, card Card, condition, finish, language, edition, setCode, editionID string) {
	cond, err := mtgban.ParseCondition(condition)
	if err != nil {
		mint.printf("unsupported %s condition", condition)
		return
	}
	if strings.Contains(card.Name, "(HP)") {
		cond = mtgban.HP
	}
	if strings.Contains(card.Name, "(DMG)") || strings.Contains(card.Name, "(Damaged)") {
		cond = mtgban.PO
	}

	link := "https://www.mtgmintcard.com/index.php?main_page=product_info&products_id=" + card.ID
	if mint.partner != "" {
		link += "&utm_source=" + url.QueryEscape(mint.partner) + "&utm_medium=referral&utm_campaign=" + url.QueryEscape(mint.partner)
	}

	cardID, found := sku2uuid[card.TCGplayerID]
	if !found {
		theCard, err := preprocess(mint.backend, card.Name, card.Number, finish, language, edition, setCode)
		if err != nil {
			if !errors.Is(err, matcher.ErrUnsupported) {
				mint.printf("%v", err)
			}
			return
		}

		cardID, err = mint.backend.Match(theCard)
		if errors.Is(err, matcher.ErrUnsupported) {
			return
		} else if err != nil {
			// Skip errors on tokens
			if strings.Contains(card.Name, "Token") {
				return
			}
			// A bare name names no origin printing, so one held only by
			// The List cannot be told apart.
			if setCode == "MYS" && theCard.Variation == "" && mint.onlyTheList(err) {
				return
			}
			if mint.lumpedArts(card, theCard, err) {
				return
			}
			cardID = mint.soldInFinish(theCard, err)
			if cardID == "" {
				mint.logRefusal(card, theCard, err, finish, language, edition, setCode, link)
				return
			}
		}
	}

	var sellPrice float64
	if card.Price != "" && card.Quantity > 0 {
		sellPrice, err = strconv.ParseFloat(card.Price, 64)
		if err != nil {
			mint.printf("%v", err)
		}

		if sellPrice > 0 {
			out := &mtgban.InventoryEntry{
				Conditions: cond,
				Price:      sellPrice,
				Quantity:   card.Quantity,
				URL:        link,
				OriginalID: card.ID,
				InstanceID: card.ID,
			}
			err = mint.inventory.Add(cardID, out)
			if err != nil {
				mint.printf("%v", err)
			}
		}
	}

	if card.BuyPrice != "" {
		buyPrice, err := strconv.ParseFloat(card.BuyPrice, 64)
		if err != nil {
			mint.printf("%v", err)
		}

		var priceRatio float64
		if sellPrice > 0 {
			priceRatio = buyPrice / sellPrice * 100
		}

		link := "https://www.mtgmintcard.com/buylist?action=advanced_search&ed=" + editionID + "&mo_1=1&mo_2=1&card_name=" + url.QueryEscape(card.Name)
		if mint.partner != "" {
			link += "&utm_source=" + url.QueryEscape(mint.partner) + "&utm_medium=referral&utm_campaign=" + url.QueryEscape(mint.partner)
		}

		gradeMap := grading(mint.backend, cardID, buyPrice)
		for _, grade := range mtgban.DefaultGradeTags {
			price := buyPrice * gradeMap[grade]
			if price > 0 {
				out := &mtgban.BuylistEntry{
					Conditions: grade,
					BuyPrice:   price,
					PriceRatio: priceRatio,
					URL:        link,
					OriginalID: card.ID,
					InstanceID: card.ID,
				}
				err = mint.buylist.Add(cardID, out)
				if err != nil {
					mint.printf("%v", err)
				}
			}
		}
	}
}

// logRefusal reports a listing that matched no printing, with the candidates
// of an aliasing error.
func (mint *MTGMintCard) logRefusal(card Card, theCard *matcher.InputCard, err error, finish, language, edition, setCode, link string) {
	mint.printf("%v", err)
	mint.printf("%q", theCard)
	mint.printf("%s|%s|%s|%s|%s|%s|%d", card.Name, card.Number, finish, language, edition, setCode, card.TCGplayerID)
	mint.printf("%s", link)

	var alias *matcher.AliasingError
	if errors.As(err, &alias) {
		for _, probe := range alias.Probe() {
			candidate, _ := mint.backend.GetUUID(probe)
			mint.printf("- %s", candidate)
		}
	}
}

// Load fetches everything this scraper offers. See mtgban.Scraper.
func (mint *MTGMintCard) Load(ctx context.Context) error {
	mint.printf("Loading MTGMintCard data")
	mintClient, err := NewMintClient(ctx)
	if err != nil {
		return err
	}
	productList, err := mintClient.GetProductList(ctx)
	if err != nil {
		return err
	}
	mint.printf("Found %d editions", len(productList))

	sku2uuid, err := tcgplayer.SKUCards(mint.backend, mint.catalog)
	if err != nil {
		return err
	}
	mint.printf("Found %d skus", len(sku2uuid))

	for edition, product := range productList {
		for language, finishes := range product.Cards {
			for finish, conditions := range finishes {
				for cond, rarities := range conditions {
					for _, cards := range rarities {
						for _, card := range cards {
							mint.processEntry(sku2uuid, card, cond, finish, language, edition, product.Abbreviation, product.EditionID)
						}
					}
				}
			}
		}
	}

	mint.inventoryDate = time.Now()
	mint.buylistDate = time.Now()

	return nil
}

// Inventory returns what Load collected. See mtgban.Seller.
func (mint *MTGMintCard) Inventory() mtgban.InventoryRecord {
	return mint.inventory
}

// Buylist returns what Load collected. See mtgban.Vendor.
func (mint *MTGMintCard) Buylist() mtgban.BuylistRecord {
	return mint.buylist
}

func grading(b *matcher.Backend, cardID string, price float64) map[mtgban.Condition]float64 {
	co, err := b.GetUUID(cardID)
	if err != nil {
		return nil
	}

	if co.Foil {
		return map[mtgban.Condition]float64{
			mtgban.NM: 1, mtgban.SP: 0.75, mtgban.MP: 0.5, mtgban.HP: 0.3,
		}
	}

	switch co.SetCode {
	case "LEA", "LEB", "2ED", "3ED":
		return map[mtgban.Condition]float64{
			mtgban.NM: 1,
		}
	}

	if price >= 30.25 {
		return map[mtgban.Condition]float64{
			mtgban.NM: 1, mtgban.SP: 0.85, mtgban.MP: 0.75, mtgban.HP: 0.65,
		}
	}
	if price >= 10.25 {
		return map[mtgban.Condition]float64{
			mtgban.NM: 1, mtgban.SP: 0.80, mtgban.MP: 0.7, mtgban.HP: 0.6,
		}
	}
	if price >= 0.25 {
		return map[mtgban.Condition]float64{
			mtgban.NM: 1, mtgban.SP: 0.75, mtgban.MP: 0.6, mtgban.HP: 0.35,
		}
	}
	return map[mtgban.Condition]float64{
		mtgban.NM: 1, mtgban.SP: 0.5, mtgban.MP: 0.5,
	}
}

// Info describes this scraper. See mtgban.Scraper.
func (mint *MTGMintCard) Info() (info mtgban.ScraperInfo) {
	info.Name = "MTG Mint Card"
	info.Shorthand = "MMC"
	info.InventoryTimestamp = &mint.inventoryDate
	info.BuylistTimestamp = &mint.buylistDate
	info.CreditMultiplier = 1.1
	info.Game = mint.backend.Game
	return
}
