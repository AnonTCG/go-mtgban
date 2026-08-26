package starcitygames

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/mtgban/go-mtgban/mtgban"
	"github.com/mtgban/go-mtgban/mtgmatcher"
	"github.com/mtgban/go-mtgban/mtgmatcher/magic"
)

const (
	defaultConcurrency = 3

	buylistBookmark = "https://sellyourcards.starcitygames.com/"
)

type Starcitygames struct {
	LogCallback    mtgban.LogCallbackFunc
	inventoryDate  time.Time
	buylistDate    time.Time
	MaxConcurrency int

	Affiliate string

	TargetEdition string

	DisableRetail  bool
	DisableBuylist bool

	inventory mtgban.InventoryRecord
	buylist   mtgban.BuylistRecord

	client *SCGClient
	game   int

	styleMap  map[int]string
	finishMap map[int]string
	setMap    map[int]string
}

func NewScraper(game int, guid, bearer string) *Starcitygames {
	scg := Starcitygames{}
	scg.inventory = mtgban.InventoryRecord{}
	scg.buylist = mtgban.BuylistRecord{}
	scg.client = NewSCGClient(guid, bearer)
	scg.MaxConcurrency = defaultConcurrency
	scg.game = game
	return &scg
}

type responseChan struct {
	cardId   string
	invEntry *mtgban.InventoryEntry
	buyEntry *mtgban.BuylistEntry
	pageURL  string

	ignoreErr bool
}

func (scg *Starcitygames) printf(format string, a ...interface{}) {
	if scg.LogCallback != nil {
		scg.LogCallback("[SCG] "+format, a...)
	}
}

func (scg *Starcitygames) processPage(ctx context.Context, channel chan<- responseChan, page int) error {
	results, err := scg.client.GetPage(ctx, scg.game, page)
	if err != nil {
		return err
	}

	for _, result := range results {
		if len(result.Document.ProductType) == 0 {
			return errors.New("malformed product_type")
		}
		if result.Document.ProductType[0] != "Singles" {
			scg.printf("Skipping product_type %s", result.Document.ProductType[0])
			continue
		}

		if len(result.Document.CardName) == 0 {
			return errors.New("malformed card_name")
		}
		if len(result.Document.Set) == 0 {
			return errors.New("malformed set")
		}
		if len(result.Document.Finish) == 0 {
			return errors.New("malformed finish")
		}
		if len(result.Document.Language) == 0 {
			return errors.New("malformed language")
		}
		if len(result.Document.UniqueID) == 0 {
			return errors.New("malformed unique_id")
		}
		cardName := result.Document.CardName[0]
		edition := result.Document.Set[0]
		finish := result.Document.Finish[0]
		language := result.Document.Language[0]
		id := fmt.Sprint(result.Document.UniqueID[0])

		var number string
		if len(result.Document.CollectorNumber) > 0 {
			number = result.Document.CollectorNumber[0]
		}
		var variant string
		if len(result.Document.Subtitle) > 0 {
			variant += result.Document.Subtitle[0]
		}
		var sku string
		if len(result.Document.HawkChildAttributes) > 0 &&
			len(result.Document.HawkChildAttributes[0].VariantSKU) > 0 {
			sku = result.Document.HawkChildAttributes[0].VariantSKU[0]
			// Strip the last number that points to the condition
			if sku != "" {
				sku = sku[:len(sku)-1]
			}
		}

		link := ""
		if len(result.Document.URLDetail) > 0 {
			link = BaseProductURL + result.Document.URLDetail[0]
		}

		var cardId string
		var err error
		switch scg.game {
		case GameMagic:
			hit := Hit{
				Name:            cardName,
				SetName:         edition,
				Language:        language,
				CollectorNumber: number,
				Variants: []Variant{{
					Subtitle: strings.TrimSpace(variant),
					Sku:      sku,
				}},
				FinishPricingTypeID: 1,
			}
			if finish == "Foil" {
				hit.FinishPricingTypeID = 2
			}
			var theCard *mtgmatcher.InputCard
			theCard, err = preprocess(hit)
			if err != nil {
				continue
			}

			cardId, err = mtgmatcher.Match(theCard)
			if errors.Is(err, mtgmatcher.ErrUnsupported) {
				continue
			} else if err != nil {
				// Skip errors from tokens and similar
				if strings.Contains(cardName, "Token") ||
					strings.Contains(variant, "Token") ||
					strings.HasPrefix(cardName, "{") {
					continue
				}
				scg.printf("%v", err)
				scg.printf("%q", theCard)
				scg.printf("%v", hit)
				scg.printf("-> %s", link)

				var alias *mtgmatcher.AliasingError
				if errors.As(err, &alias) {
					probes := alias.Probe()
					for _, probe := range probes {
						co, _ := mtgmatcher.GetUUID(probe)
						scg.printf("%s", co)
					}
				}
				continue
			}
		case GameLorcana:
			cardId, err = simpleSearch(cardName, number, finish != "Non-foil")
			if errors.Is(err, mtgmatcher.ErrUnsupported) {
				continue
			} else if err != nil {
				scg.printf("%v", err)
				scg.printf("%+v", result)
				scg.printf("-> %s", link)

				var alias *mtgmatcher.AliasingError
				if errors.As(err, &alias) {
					probes := alias.Probe()
					for _, probe := range probes {
						co, _ := mtgmatcher.GetUUID(probe)
						scg.printf("%s", co)
					}
				}
				continue
			}
		default:
			return errors.New("unsupported game")
		}

		customFields := map[string]string{
			"SCGName":     cardName,
			"SCGEdition":  edition,
			"SCGLanguage": language,
			"SCGFinish":   finish,
			"scgSubtitle": variant,
			"scgNumber":   number,
			"scgSKU":      sku,
		}

		for _, attribute := range result.Document.HawkChildAttributes {
			if len(attribute.VariantLanguage) == 0 {
				return errors.New("malformed variant_language")
			}

			if attribute.VariantLanguage[0] != language {
				continue
			}

			if len(attribute.Price) == 0 {
				return errors.New("malformed price")
			}
			if len(attribute.Qty) == 0 {
				return errors.New("malformed qty")
			}
			if len(attribute.Condition) == 0 {
				return errors.New("malformed condition")
			}
			priceStr := attribute.Price[0]
			qty := attribute.Qty[0]
			condition := attribute.Condition[0]

			switch condition {
			case "Near Mint":
				condition = "NM"
			case "Played":
				condition = "SP"
			case "Heavily Played":
				condition = "HP"
			default:
				scg.printf("unknown condition %s for %s", condition, cardName)
				continue
			}

			price, err := mtgmatcher.ParsePrice(priceStr)
			if err != nil {
				scg.printf("invalid price for %s: %s", cardName, err.Error())
				continue
			}

			if qty == 0 || price == 0 {
				continue
			}

			skuCond := ""
			if len(attribute.VariantSKU) > 0 {
				skuCond = attribute.VariantSKU[0]
			}

			out := responseChan{
				cardId: cardId,
				invEntry: &mtgban.InventoryEntry{
					Price:      price,
					Conditions: condition,
					Quantity:   qty,
					OriginalID: sku,
					InstanceID: skuCond,
					URL:        SCGProductURL(result.Document.URLDetail, attribute.VariantSKU, scg.Affiliate),
					CustomFields: map[string]string{
						"SCGID": id,
					},
				},
				ignoreErr: strings.Contains(edition, "World Championship") || strings.Contains(cardName, "Token"),
				pageURL:   link,
			}
			if condition == "NM" {
				out.invEntry.CustomFields = customFields
			}
			channel <- out
		}
	}

	return nil
}

func (scg *Starcitygames) scrape(ctx context.Context) error {
	totalPages, err := scg.client.NumberOfPages(ctx, scg.game)
	if err != nil {
		return err
	}
	scg.printf("Found %d pages", totalPages)

	pages := make([]int, 0, totalPages)
	for i := 1; i <= totalPages; i++ {
		pages = append(pages, i)
	}

	mtgban.WorkerPool(ctx, scg.MaxConcurrency, pages,
		func(ctx context.Context, page int, results chan<- responseChan) error {
			scg.printf("Processing page %d", page)
			return scg.processPage(ctx, results, page)
		},
		func(record responseChan) {
			err := scg.inventory.AddStrict(record.cardId, record.invEntry)
			if err != nil && !record.ignoreErr {
				scg.printf("%s", err.Error())
				scg.printf("-> %s", record.pageURL)
			}
		},
		scg.printf,
	)

	scg.inventoryDate = time.Now()

	return nil
}

func (scg *Starcitygames) processBuylistEdition(ctx context.Context, channel chan<- responseChan, setID int) error {
	i := 0

	for {
		search, err := scg.client.SearchAll(ctx, scg.game, i, buylistRequestLimit, setID)
		if err != nil {
			return err
		}

		scg.processBuylistEditionHits(channel, search.Hits)

		if len(search.Hits) < buylistRequestLimit {
			break
		}

		i++
	}

	return nil
}

func (scg *Starcitygames) processBuylistEditionHits(channel chan<- responseChan, hits []Hit) {
	var gamePath string
	switch scg.game {
	case GameLorcana:
		gamePath = "lorcana"
	case GameMagic:
		gamePath = "mtg"
	default:
		panic("unsupported game")
	}

	for _, hit := range hits {
		// Generate URL
		link, _ := url.JoinPath(
			buylistBookmark,
			gamePath,
			"bookmark",
			url.QueryEscape(hit.Name),
			"0/1/0/0", // various faucets (bulk, hotlist, etc)
			fmt.Sprint(hit.SetID),
			hit.Language,
			",",                                 // rarity
			"0/999999.99",                       // min/max price range
			fmt.Sprint(hit.FinishPricingTypeID), // 0 = any, 1 = nf, 2 = f
			"default",
		)

		// Convert ids into human readable tags
		var finish int
		switch v := hit.Finish.(type) {
		case int:
			finish = v
		case float64:
			finish = int(v)
		default:
		}

		cardStyle := scg.styleMap[hit.CardStyleID]
		cardFinish := scg.finishMap[finish]

		// Workaround for missing double styles
		isSerial := strings.HasSuffix(hit.Image, "z.jpg") ||
			strings.HasSuffix(hit.Image, "-vs.jpg") ||
			strings.Contains(hit.Image, "serial")

		// Unfortunately there are ~200 cards with no such tags, get creative
		if !isSerial {
			code := scg.setMap[hit.SetID]
			if len(mtgmatcher.MatchInSetNumber(hit.Name, code, hit.CollectorNumber)) == 1 &&
				magic.HasSerializedPrinting(hit.Name, code) &&
				len(hit.Variants) > 0 &&
				hit.Variants[0].BuyPrice >= 50 {
				isSerial = true
			}
		}

		if isSerial {
			if cardFinish != "" {
				cardFinish += " "
			}
			cardFinish += "Serialized"
		}

		var cardId string
		var err error
		var theCard *mtgmatcher.InputCard
		if scg.game == GameMagic {
			// Add back tags into subtitle, but skip the default foil/nonfoil to
			// keep variants simple and compatible with the existing ones
			if cardFinish == "Foil" || cardFinish == "Non-foil" {
				cardFinish = ""
			}
			hit.Variants[0].Subtitle += " " + cardStyle + " " + cardFinish
			hit.Variants[0].Subtitle = strings.Replace(hit.Variants[0].Subtitle, "  ", " ", -1)
			hit.Variants[0].Subtitle = strings.TrimSpace(hit.Variants[0].Subtitle)

			theCard, err = preprocess(hit)
			if err != nil {
				continue
			}

			cardId, err = mtgmatcher.Match(theCard)
		} else if scg.game == GameLorcana {
			cardName := hit.Name
			number := hit.CollectorNumber
			cardId, err = simpleSearch(cardName, number, hit.FinishPricingTypeID == 2)
		}
		if errors.Is(err, mtgmatcher.ErrUnsupported) {
			continue
		} else if err != nil {
			scg.printf("%v for %+v", err, theCard)
			scg.printf("%+v", hit)
			scg.printf("-> %s", link)

			var alias *mtgmatcher.AliasingError
			if errors.As(err, &alias) {
				probes := alias.Probe()
				for _, probe := range probes {
					co, _ := mtgmatcher.GetUUID(probe)
					scg.printf("%s", co)
				}
			}
			continue
		}

		for _, variant := range hit.Variants {
			conditions := variant.VariantValue
			switch conditions {
			case "NM", "NM/M":
				conditions = "NM"
			case "PL":
				conditions = "SP"
				// Stricter grading for foils
				if hit.FinishPricingTypeID == 2 {
					conditions = "MP"
				}
			case "HP":
				conditions = "MP"
				// Stricter grading for foils
				if hit.FinishPricingTypeID == 2 {
					conditions = "HP"
				}
			default:
				scg.printf("unknown condition %s for %v", conditions, variant)
				continue
			}

			var priceRatio, sellPrice float64
			price := variant.BuyPrice
			if price == 0 {
				continue
			}

			invCards := scg.inventory[cardId]
			for _, invCard := range invCards {
				sellPrice = invCard.Price
				break
			}
			if sellPrice > 0 {
				priceRatio = price / sellPrice * 100
			}

			// Add the line entry as needed by the csv import
			var customFields map[string]string
			if conditions == "NM" {
				customFields = map[string]string{
					"SCGName":     hit.Name,
					"SCGEdition":  hit.SetName,
					"SCGLanguage": hit.Language,
					"SCGFinish":   fmt.Sprint(hit.FinishPricingTypeID),
					// custom, helps debugging
					"scgSubtitle": hit.Subtitle,
					"scgNumber":   hit.CollectorNumber,
					"scgSKU":      variant.Sku,
					"scgCard":     fmt.Sprint(theCard),
				}
			}

			channel <- responseChan{
				cardId: cardId,
				buyEntry: &mtgban.BuylistEntry{
					Conditions:   conditions,
					BuyPrice:     price,
					Quantity:     0,
					PriceRatio:   priceRatio,
					URL:          link,
					CustomFields: customFields,
					OriginalID:   variant.Sku,
				},
				ignoreErr: strings.Contains(hit.Name, "Token"),
			}
		}
	}
}

type setData struct {
	Name  string
	SetID int
	Index int
}

func (scg *Starcitygames) scrapeBL(ctx context.Context) error {
	settings, err := SearchSettings(ctx)
	if err != nil {
		return err
	}

	scg.styleMap = make(map[int]string)
	scg.finishMap = make(map[int]string)
	for _, style := range settings.CardStyles {
		if style.GameID != scg.game {
			continue
		}

		scg.styleMap[style.ID] = style.Name
	}
	for _, finish := range settings.CardFinishes {
		if finish.GameID != scg.game {
			continue
		}

		scg.finishMap[finish.ID] = finish.Name
	}

	scg.printf("Found %d styles and %d finishes", len(scg.styleMap), len(scg.finishMap))

	search, err := scg.client.SearchBuylistEditions(ctx)
	if err != nil {
		return err
	}

	scg.setMap = make(map[int]string)
	editions := make([]setData, 0, len(search.Hits))
	for _, hit := range search.Hits {
		if hit.GameID != scg.game {
			continue
		}

		editions = append(editions, setData{
			Name:  hit.Name,
			SetID: hit.SetID,
		})
		scg.setMap[hit.SetID] = hit.WizardsCode
	}

	scg.printf("Found %d editions", len(editions))

	for i := range editions {
		editions[i].Index = i + 1
	}

	mtgban.WorkerPool(ctx, scg.MaxConcurrency, editions,
		func(ctx context.Context, page setData, results chan<- responseChan) error {
			if scg.TargetEdition != "" && scg.TargetEdition != page.Name {
				return nil
			}

			scg.printf("Processing edition %s (%d/%d)", page.Name, page.Index, len(editions))
			return scg.processBuylistEdition(ctx, results, page.SetID)
		},
		func(record responseChan) {
			err := scg.buylist.Add(record.cardId, record.buyEntry)
			if err != nil && !record.ignoreErr {
				scg.printf("%s", err.Error())
			}
		},
		scg.printf,
	)

	scg.buylistDate = time.Now()

	return nil
}

func (scg *Starcitygames) SetConfig(opt mtgban.ScraperOptions) {
	scg.DisableRetail = opt.DisableRetail
	scg.DisableBuylist = opt.DisableBuylist
}

func (scg *Starcitygames) Load(ctx context.Context) error {
	var errs []error

	if !scg.DisableRetail {
		err := scg.scrape(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("inventory load failed: %w", err))
		}
	}

	if !scg.DisableBuylist {
		err := scg.scrapeBL(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("buylist load failed: %w", err))
		}
	}

	return errors.Join(errs...)
}

func (scg *Starcitygames) Inventory() mtgban.InventoryRecord {
	return scg.inventory
}

func (scg *Starcitygames) Buylist() mtgban.BuylistRecord {
	return scg.buylist
}

func (scg *Starcitygames) Info() (info mtgban.ScraperInfo) {
	info.Name = "Star City Games"
	info.Shorthand = "SCG"
	info.InventoryTimestamp = &scg.inventoryDate
	info.BuylistTimestamp = &scg.buylistDate
	info.CreditMultiplier = 1.3
	switch scg.game {
	case GameMagic:
		info.Game = mtgban.GameMagic
	case GameLorcana:
		info.Game = mtgban.GameLorcana
	}
	return
}
