package mtgseattle

import (
	"cmp"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/matcher/sealed"
)

var cardTable = map[string]string{
	"Pir, Imaginitive Rascal":      "Pir, Imaginative Rascal",
	"Battra, Terror of the City":   "Dirge Bat",
	"Mechagodzilla":                "Crystalline Giant",
	"B.F.M. 2 (Big Furry Monster)": "B.F.M. (Big Furry Monster Right)",
	"B.F.M. 1 (Big Furry Monster)": "B.F.M. (Big Furry Monster Left)",
	"Selesnya Guildate":            "Selesnya Guildgate",
	"Daretti Emblem":               "Daretti, Scrap Savant Emblem",

	"_________": "_____",
}

// tokenLetters numbers, by edition, the tokens this store tells apart with a
// letter of its own; each was read off the product's photo.
var tokenLetters = map[string]map[string]string{
	"Scars of Mirrodin":             {"Wurm Token B": "8"},
	"Duel Decks: Anthology":         {"Beast Token A": "8"},
	"Duel Decks: Garruk vs Liliana": {"Beast Token B": "T2"},
}

// tokenLetter reads a token title closed by such a letter, "Wurm Token (B)"
// or "Beast Token A".
var tokenLetter = regexp.MustCompile(`^(.+ Token) \(?([A-Z])\)?$`)

var promoTags = []string{
	"2012 Holiday Promo",
	"Alternate Art Foil",
	"Buy-a-Box Promo",
	"Foil Beta Picture",
	"Game Day Promo",
	"SDCC 2019 Exclusive",
}

func preprocess(b *matcher.Backend, cardName, edition, variant string) (*matcher.InputCard, error) {
	m := tokenLetter.FindStringSubmatch(cardName)
	if m != nil {
		number, found := tokenLetters[edition][m[1]+" "+m[2]]
		if found {
			cardName, variant = m[1], strings.TrimSpace(number+" "+variant)
		}
	}

	s := matcher.SplitVariants(cardName)
	cardName = s[0]
	if len(s) > 1 {
		if variant != "" {
			variant += " "
		}
		variant += strings.Join(s[1:], " ")
	}

	if strings.Contains(variant, "BGS") ||
		matcher.Contains(cardName, "Deprecated") ||
		strings.Contains(cardName, "Does not exist") {
		return nil, errors.New("unsupported")
	}

	isFoil := strings.Contains(variant, "Foil")
	if isFoil {
		variant = strings.Replace(variant, "Foil", "", 1)
	}
	variant = strings.TrimSpace(variant)

	// The store files an emblem as "Emblem - <planeswalker>", which the
	// datastore names "<planeswalker> Emblem".
	if cardName == "Emblem" && variant != "" {
		cardName, variant = variant+" Emblem", ""
	}

	switch edition {
	case "French Revised FBB",
		"German Revised FBB",
		"German Revised FWB",
		"German Renaissance":
		return nil, errors.New("foreign")
	case "Italian Renaissance":
		if cardName == "Blood Moon" {
			return nil, errors.New("does not exist")
		}
	case "Ikoria: Lair of Behemoths":
		if strings.HasSuffix(variant, "JP Alternate Art") {
			variant = "Godzilla"
		}
	case "Commander Anthology Vol. II":
		if cardName == "Bonehoard" {
			return nil, errors.New("does not exist")
		}
	case "9th Edition":
		if cardName == "Goblin Raider" && isFoil {
			return nil, errors.New("does not exist")
		}
	case "Starter 2000":
		switch cardName {
		case "Spined Wurm", "Counterspell", "Shock", "Llanowar Elves":
			return nil, errors.New("does not exist")
		}
	case "Portal 1":
		if variant == "2" {
			variant = "reminder text"
		}
	case "Pre-Release Promos":
		switch cardName {
		case "Pir, Imaginitive Rascal":
			variant = ""
		case "In Garruk's Wake":
			edition = "PM15"
		}
		if strings.HasSuffix(cardName, "Foil") {
			cardName = strings.TrimSuffix(cardName, " Foil")
			isFoil = true
		}
		if strings.Contains(variant, "Launch") {
			edition = "Launch Party"
		}
	case "Homelands":
		// "(N)" is the N-th art of the card, not a collector number
		n, err := strconv.Atoi(variant)
		if err == nil {
			cards := b.MatchInSet(cardName, "HML")
			slices.SortFunc(cards, func(x, y matcher.Card) int {
				return cmp.Compare(x.Number, y.Number)
			})
			if n >= 1 && n <= len(cards) {
				variant = cards[n-1].Number
			}
		}
	case "FNM Promos":
		switch cardName {
		case "Elvish Mystic":
			variant = "2014"
		case "Shrapnel Blast":
			variant = "2008"
		case "Chandra's Fury":
			edition = "URL/Convention Promos"
		}
	case "Judge Rewards Promos",
		"Judge Academy Promo":
		var year string
		switch cardName {
		case "Demonic Tutor":
			if variant == "DCI Judge Promo" {
				year = "2008"
			} else if variant == "Judge Academy Promo" {
				year = "2020"
			}
		case "Vampiric Tutor":
			year = matcher.ExtractYear(variant)
			if year == "" {
				year = "2000"
			}
		case "Wasteland":
			year = matcher.ExtractYear(variant)
		}
		if year != "" {
			variant = year
		}
	case "Unique & Misc Promos":
		switch cardName {
		case "1996 World Champion",
			"Fraternal Exaltation",
			"Splendid Genesis":
			edition = "Special Occasion"
		case "Reliquary Tower":
			edition = "PLG20"
		case "Tember City":
			edition = "PHOP"
		case "Lavinia, Azorius Renegade":
			edition = "PRNA"
			variant = ""
		case "Warmonger":
			edition = "PMEI"
		case "Sanctum Prelate":
			edition = "MH2"
		case "Gala Greeters":
			if variant == "Box Topper" {
				edition = "SNC"
				variant = "Borderless"
			}
		}
		for _, tag := range promoTags {
			if strings.HasSuffix(cardName, tag) {
				cardName = strings.TrimSuffix(cardName, " "+tag)
				variant = tag
			}
		}
		if variant == "Winner" {
			return nil, errors.New("unsupported")
		}
	case "Mystery Booster Cards":
		number := mb1PLSTNumber(b, cardName, isFoil)
		if number != "" {
			edition = "PLST"
			variant = number
		}
	case "Core Set 2021":
		if strings.Contains(variant, "Alternate Art") && matcher.ExtractNumber(variant) == "" {
			variant = "Borderless"
		}
	case "Adventures in the Forgotten Realms":
		if variant == "Dungeon Module" {
			variant = "Showcase"
		}
	case "Secret Lair Drop Series":
		if variant == "Borderless" && len(b.MatchInSet(cardName, "SLC")) > 0 {
			edition = "SLC"
		}
	}

	lutName, found := cardTable[cardName]
	if found {
		cardName = lutName
	}

	return &matcher.InputCard{
		Name:      cardName,
		Variation: variant,
		Edition:   edition,
		Foil:      isFoil,
	}, nil
}

// mb1PLSTBooster is the Mystery Booster sealed product mb1PLSTNumber reads
// booster contents from.
const mb1PLSTBooster = "Mystery Booster Booster Pack (Retail Edition)"

// mb1PLSTNumber answers which PLST printing Mystery Booster's own booster
// bundles for cardName in the listing's finish, or "" when the booster names
// none sold in that finish.
func mb1PLSTNumber(b *matcher.Backend, cardName string, isFoil bool) string {
	finish := matcher.FinishNonfoil
	if isFoil {
		finish = matcher.FinishFoil
	}
	for _, uuid := range b.GetSealedUUIDsInSet("MB1") {
		co, err := b.GetUUID(uuid)
		if err != nil || co.Name != mb1PLSTBooster {
			continue
		}

		probs, err := sealed.ProductCounts(b, "MB1", uuid)
		if err != nil {
			return ""
		}
		for _, p := range probs {
			card, err := b.GetUUID(p.UUID)
			if err == nil && card.SetCode == "PLST" && card.Name == cardName && card.HasFinish(finish) {
				return card.Number
			}
		}
		return ""
	}
	return ""
}
