package cardgamecorner

import (
	"cmp"
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
)

// The older sets' reprints are told apart in the variants table by the
// store's own image name, and the store's "(Version N)" tag names none of
// them. These pin that the tag steps aside there and nowhere else.
func TestInternalPreprocessVersionTag(t *testing.T) {
	tests := []struct {
		desc      string
		name      string
		edition   string
		variation string
		extra     string
		want      string
	}{
		{
			desc:      "version tag yields to the image name",
			name:      "Abbey Matron",
			edition:   "Homelands",
			variation: "Version 1",
			extra:     "OR003",
			want:      "OR003",
		},
		{
			desc:      "the other art reaches its own image name",
			name:      "Abbey Matron",
			edition:   "Homelands",
			variation: "Version 2",
			extra:     "AbbeyMatron",
			want:      "AbbeyMatron",
		},
		{
			desc:      "an empty variation still reaches the image name",
			name:      "Aesthir Glider",
			edition:   "Alliances",
			variation: "",
			extra:     "AZ002",
			want:      "AZ002",
		},
		{
			desc:      "a variation that names something keeps it",
			name:      "Aesthir Glider",
			edition:   "Alliances",
			variation: "Foreign White Border",
			extra:     "AZ002",
			want:      "Foreign White Border",
		},
		{
			desc:      "the tag survives where no table keys on the image",
			name:      "Agent of Treachery",
			edition:   "Core Set 2020 Promos",
			variation: "Version 1",
			extra:     "M20001",
			want:      "Version 1",
		},
	}

	b := &matcher.Backend{}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			_, _, got := internalPreprocess(b, tt.name, tt.edition, tt.variation, tt.extra)
			if got != tt.want {
				t.Errorf("variation = %q, want %q", got, tt.want)
			}
		})
	}
}

// Card Game Corner's generic "Promo" shelf sells this card under two different
// physical listings and never tags either one with the "(V.N)" suffix its
// own DCI Promo shelf puts on the same listing, leaving the matcher a bare
// name shared by half a dozen printings - it lands on the Secret Lair Promo
// pair every time. The image name is the only place either listing still
// says which one it is.
func TestInternalPreprocessUginPromoShelf(t *testing.T) {
	tests := []struct {
		desc          string
		extra         string
		wantEdition   string
		wantVariation string
	}{
		{
			desc:          "the MagicFest 2025 foil, shared with the DCI Promo (V.2) listing",
			extra:         "ugin-the-spirit-dragon-v2_823824",
			wantEdition:   "MagicFest 2025",
			wantVariation: "6",
		},
		{
			desc:          "the Ugin's Fate nonfoil, named in Italian with no version at all",
			extra:         "ugin-lo-spirito-drago_271991",
			wantEdition:   "Ugin's Fate",
			wantVariation: "1",
		},
		{
			desc:          "no image to read leaves the shelf as ambiguous as it arrived",
			extra:         "noimage",
			wantEdition:   "Promo",
			wantVariation: "",
		},
	}

	b := &matcher.Backend{}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			_, gotEdition, gotVariation := internalPreprocess(b, "Ugin, the Spirit Dragon", "Promo", "", tt.extra)
			if gotEdition != tt.wantEdition || gotVariation != tt.wantVariation {
				t.Errorf("internalPreprocess() = edition %q, variation %q, want %q, %q",
					gotEdition, gotVariation, tt.wantEdition, tt.wantVariation)
			}
		})
	}
}

// The WAR Japanese shelf tags its Prerelease promos "(Version 2)", and every
// other listing there is the booster printing.
func TestInternalPreprocessWarJapanese(t *testing.T) {
	tests := []struct {
		variation     string
		extra         string
		wantEdition   string
		wantVariation string
	}{
		{"Version 2", "jaya-venerated-firemage-version-2_373440", "War of the Spark Promos", "Japanese Prerelease"},
		{"Version 1", "WAR135b", "War of the Spark", "Japanese"},
	}

	b := &matcher.Backend{}
	for _, tt := range tests {
		t.Run(tt.variation, func(t *testing.T) {
			_, gotEdition, gotVariation := internalPreprocess(b, "Jaya, Venerated Firemage", warSparkJapaneseEdition, tt.variation, tt.extra)
			if gotEdition != tt.wantEdition || gotVariation != tt.wantVariation {
				t.Errorf("internalPreprocess() = edition %q, variation %q, want %q, %q",
					gotEdition, gotVariation, tt.wantEdition, tt.wantVariation)
			}
		})
	}
}

// The Ravnica guildgates are told apart by the number in their image name,
// and the tag steps aside only where a number is actually there to read.
func TestNamesTheArt(t *testing.T) {
	tests := []struct {
		desc    string
		edition string
		extra   string
		want    bool
	}{
		{
			desc:    "a numbered image says which art",
			edition: "Guilds of Ravnica",
			extra:   "GRN243",
			want:    true,
		},
		{
			desc:    "a slug in the same edition says nothing",
			edition: "Guilds of Ravnica",
			extra:   "guilds-of-ravnica-boros-guildgate-260044",
			want:    false,
		},
		{
			desc:    "the older sets are keyed on the image itself",
			edition: "Homelands",
			extra:   "AbbeyMatron",
			want:    true,
		},
		{
			desc:    "everywhere else the wording is what speaks",
			edition: "Core Set 2020 Promos",
			extra:   "M20001",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := namesTheArt(tt.edition, tt.extra)
			if got != tt.want {
				t.Errorf("namesTheArt(%q, %q) = %v, want %v",
					tt.edition, tt.extra, got, tt.want)
			}
		})
	}
}

// The image name carries the English card name for the listings whose own
// name field does not, along with the art index when it has one.
func TestImageName(t *testing.T) {
	tests := []struct {
		desc          string
		extra         string
		edition       string
		wantName      string
		wantVariation string
	}{
		{
			desc:     "the id comes off the end",
			extra:    "basal-sliver_695839",
			edition:  "Secret Lair Drop Series",
			wantName: "basal sliver",
		},
		{
			desc:     "so does a hexadecimal one",
			extra:    "ember-island-production_0f79a7fc",
			edition:  "Avatar: The Last Airbender",
			wantName: "ember island production",
		},
		{
			desc:     "the edition comes off the front",
			extra:    "the-brothers-war-curate-683384",
			edition:  "The Brothers' War",
			wantName: "curate",
		},
		{
			desc:          "the art index is kept as the variation",
			extra:         "the-brothers-war-island-v3-683345",
			edition:       "The Brothers' War",
			wantName:      "island",
			wantVariation: "V.3",
		},
		{
			desc:    "a collector number names no card",
			extra:   "GRN243",
			edition: "Guilds of Ravnica",
		},
		{
			desc:    "neither does an image with no id",
			extra:   "catToken",
			edition: "Scars of Mirrodin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			name, variation := imageName(tt.extra, tt.edition)
			if name != tt.wantName || variation != tt.wantVariation {
				t.Errorf("imageName(%q, %q) = %q, %q, want %q, %q",
					tt.extra, tt.edition, name, variation,
					tt.wantName, tt.wantVariation)
			}
		})
	}
}

// TestImageProductID pins the guards the image-carried id is resolved
// under, on rows copied verbatim from the datastore: same-name collapse,
// cross-set image reuse, a Phyrexian-language hit let through, a same-set jpn
// twin resolving to the English printing, a basic land told apart by its id,
// and a listing that may not take another card's image.
func TestImageProductID(t *testing.T) {
	b := realDatastore(t)

	t.Run("the id picks the treatment an ambiguous wording could not", func(t *testing.T) {
		got := imageProductID(b, "Dominaria United", "/x/ertai-resurrected_672617.jpg", "Ertai Resurrected", "Dominaria United", "", false)
		if got == "" {
			t.Fatal("imageProductID returned no id, want DMU 298")
		}
		co, err := b.GetUUID(got)
		if err != nil || co.SetCode != "DMU" || co.Number != "298" {
			t.Errorf("imageProductID resolved to %v, want DMU 298", co)
		}
	})

	t.Run("a bare numeric image name carries the id too", func(t *testing.T) {
		got := imageProductID(b, "The Lord of the Rings: Tales of Middle-earth: Extras", "/prodotti/magic_the_gathering/717774.jpeg",
			"Faramir, Field Commander", "The Lord of the Rings: Tales of Middle-earth", "", false)
		if got == "" {
			t.Fatal("imageProductID returned no id, want LTR 303")
		}
		co, err := b.GetUUID(got)
		if err != nil || co.SetCode != "LTR" || co.Number != "303" {
			t.Errorf("imageProductID resolved to %v, want LTR 303", co)
		}
	})

	t.Run("the Scryfall digits in the image name pick the treatment", func(t *testing.T) {
		const name = "Aang, at the Crossroads // Aang, Destined Savior"
		for image, number := range map[string]string{
			"/prodotti/1/tla/aang-at-the-crossroads--aang-destined-savior_fea89ca0.jpg": "203",
			"/prodotti/1/tla/aang-at-the-crossroads--aang-destined-savior_d7f35cd9.jpg": "346",
			"/prodotti/1/tla/aang-at-the-crossroads--aang-destined-savior_ea886239.jpg": "304",
		} {
			got := imageProductID(b, "Avatar: The Last Airbender", image, name, "Avatar: The Last Airbender", "", false)
			co, err := b.GetUUID(got)
			if err != nil || co.SetCode != "TLA" || co.Number != number {
				t.Errorf("%s resolved to %v, want TLA %s", image, co, number)
			}
		}
	})

	t.Run("a whole Scryfall id names the printing", func(t *testing.T) {
		got := imageProductID(b, "Ikoria: Lair of Behemoths", "/prodotti/1/IKO2/9c348494-f60c-4bd1-9077-bff24f2e634b.jpg",
			"Forest", "Ikoria: Lair of Behemoths", "V.1", false)
		co, err := b.GetUUID(got)
		if err != nil || co.Identifiers["scryfallId"] != "9c348494-f60c-4bd1-9077-bff24f2e634b" {
			t.Errorf("imageProductID resolved to %v, want the printing that Scryfall id names", co)
		}
	})

	t.Run("a Scryfall id naming another card is refused", func(t *testing.T) {
		got := imageProductID(b, "Avatar: The Last Airbender", "/prodotti/1/tla/aang-at-the-crossroads--aang-destined-savior_d7f35cd9.jpg",
			"Zuko, Conflicted", "Avatar: The Last Airbender", "", false)
		if got != "" {
			co, _ := b.GetUUID(got)
			t.Errorf("imageProductID = %v, want \"\" - the id names Aang", co)
		}
	})

	t.Run("an image in the wording set's promo set is accepted", func(t *testing.T) {
		got := imageProductID(b, "Adventures in the Forgotten Realms: Promos",
			"/prodotti/1/pafr/a-adventures-in-the-forgotten-realms-promos-treasure-vault-v3-574677.jpg",
			"Treasure Vault", "Adventures in the Forgotten Realms", "Bundle", true)
		co, err := b.GetUUID(got)
		if err != nil || co.SetCode != "PAFR" || co.Number != "261a" {
			t.Errorf("imageProductID resolved to %v, want PAFR 261a", co)
		}
	})

	t.Run("a cross-set id is refused, deferring to the wording", func(t *testing.T) {
		got := imageProductID(b, "Secret Lair Drop Series", "/x/ertai-resurrected_672617.jpg", "Ertai Resurrected", "Secret Lair Drop Series", "", false)
		if got != "" {
			co, _ := b.GetUUID(got)
			t.Errorf("imageProductID = %v, want \"\" - the id names DMU 298, not a Secret Lair printing", co)
		}
	})

	t.Run("a promo shelf naming no set takes the id's printing", func(t *testing.T) {
		got := imageProductID(b, "Promo", "/prodotti/singles/magic/pmf/lightning-bolt-v1_367404.jpg", "Lightning Bolt", "Promo", "", false)
		co, err := b.GetUUID(got)
		if err != nil || co.SetCode != "PF19" || co.Number != "1" {
			t.Errorf("imageProductID resolved to %v, want PF19 1", co)
		}
	})

	t.Run("an Italian shelf takes an id naming the Italian printing", func(t *testing.T) {
		got := imageProductID(b, "Rinascimento", "/prodotti/singles/magic/ri/urzas-mine_19398.jpg", "Urza's Mine", "Rinascimento", "053", false)
		co, err := b.GetUUID(got)
		if err != nil || co.SetCode != "RIN" || co.Number != "178" {
			t.Errorf("imageProductID resolved to %v, want RIN 178", co)
		}
	})

	t.Run("a serialized hit lands on its plain twin unless the wording says so", func(t *testing.T) {
		image := "/prodotti/singles/magic/xmh3/emrakul-the-world-anew-v4_774085.jpg"
		got := imageProductID(b, "Modern Horizons 3: Extras", image, "Emrakul, the World Anew", "Modern Horizons 3: Extras", "V.1", false)
		co, err := b.GetUUID(got)
		if err != nil || co.SetCode != "MH3" || co.Number != "381" {
			t.Errorf("imageProductID resolved to %v, want MH3 381", co)
		}

		got = imageProductID(b, "Modern Horizons 3: Extras", image, "Emrakul, the World Anew", "Modern Horizons 3: Extras", "Serial Numbered", false)
		co, err = b.GetUUID(got)
		if err != nil || co.SetCode != "MH3" || co.Number != "381z" {
			t.Errorf("imageProductID resolved to %v, want MH3 381z", co)
		}
	})

	t.Run("a Phyrexian-language hit is accepted for an English listing", func(t *testing.T) {
		got := imageProductID(b, "Phyrexia: All Will Be One: Extras",
			"/x/phyrexia-all-will-be-one-extras-jace-the-perfected-mind-v5-692910.jpg",
			"Jace, the Perfected Mind", "Phyrexia: All Will Be One: Extras", "V.5", true)
		if got == "" {
			t.Fatal("imageProductID returned no id, want ONE 429")
		}
		co, err := b.GetUUID(got)
		if err != nil || co.SetCode != "ONE" || co.Number != "429" || co.Language != "Phyrexian" {
			t.Errorf("imageProductID resolved to %v, want ONE 429 Phyrexian", co)
		}
	})

	t.Run("a same-set jpn twin resolves to the English printing", func(t *testing.T) {
		got := imageProductID(b, "Secret Lair Drop Series", "/x/the-royal-scions_791211.jpg", "The Royal Scions", "Secret Lair Drop Series", "", false)
		if got == "" {
			t.Fatal("imageProductID returned no id, want SLD 1600")
		}
		co, err := b.GetUUID(got)
		if err != nil || co.SetCode != "SLD" || co.Number != "1600" || co.Language != "English" {
			t.Errorf("imageProductID resolved to %v, want SLD 1600 English", co)
		}
	})

	t.Run("a basic land is told apart by its image id", func(t *testing.T) {
		got := imageProductID(b, "Phyrexia: All Will Be One", "/x/phyrexia-all-will-be-one-plains-v2-689730.jpg", "Plains", "Phyrexia: All Will Be One", "V.2", false)
		co, err := b.GetUUID(got)
		if err != nil || co.Identifiers["mcmId"] != "689730" {
			t.Errorf("imageProductID resolved to %v, want the printing Cardmarket 689730 names", co)
		}
	})

	t.Run("a basic land does not take a same-set sibling's image", func(t *testing.T) {
		got := imageProductID(b, "Kaldheim", "/x/kaldheim-snow-covered-plains-530262.jpg", "Plains", "Kaldheim", "", false)
		if got != "" {
			co, _ := b.GetUUID(got)
			t.Errorf("imageProductID = %v, want \"\" - the id names Snow-Covered Plains", co)
		}
	})

	t.Run("a failed wording does not take another card's image", func(t *testing.T) {
		got := imageProductID(b, "Dominaria United", "/x/wastescape-battlemage_772110.jpg", "Wastes", "Dominaria United", "", false)
		if got != "" {
			co, _ := b.GetUUID(got)
			t.Errorf("imageProductID = %v, want \"\" - the id names Wastescape Battlemage", co)
		}
	})
}

// A name holding an apostrophe sometimes arrives quoted the way a database
// quotes it.
func TestUnquote(t *testing.T) {
	tests := []struct {
		desc string
		name string
		want string
	}{
		{
			desc: "the quoting comes off",
			name: "'Skyseer''s Chariot (V.1)'",
			want: "Skyseer's Chariot (V.1)",
		},
		{
			desc: "a plain name is left alone",
			name: "Skyseer's Chariot",
			want: "Skyseer's Chariot",
		},
		{
			desc: "so is a name that only starts with one",
			name: "'Ol' Buzzbark",
			want: "'Ol' Buzzbark",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := unquote(tt.name)
			if got != tt.want {
				t.Errorf("unquote(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// A basic land's image names its set code, of 2 to 4 characters, then its
// number. Anything else yields no number.
func TestBasicImageNumber(t *testing.T) {
	for extra, want := range map[string]string{
		"LO299":   "299",
		"BFZ250":  "250",
		"SVSC078": "78",
		"ELD402":  "",
		"817906":  "",
		"571283":  "",
		"10E123":  "123",
		"C13345":  "345",
		"noimage": "",
	} {
		got := basicImageNumber(extra)
		if got != want {
			t.Errorf("basicImageNumber(%q) = %q, want %q", extra, got, want)
		}
	}
}

// A basic land's image is sometimes another SKU's picture, so the number its
// SKU spells wins where the two differ, and is left alone where they agree or
// the SKU names none.
func TestBasicSKUNumber(t *testing.T) {
	tests := []struct {
		name, edition, variation, extra, sku, want string
	}{
		{"Island", "Invasion", "335", "IV335", "IV337", "337"},
		{"Plains", "Lorwyn", "283", "LO283", "LO282", "282"},
		{"Plains", "Battle for Zendikar", "250", "BFZ250", "BFZ250", "250"},
		{"Plains", "Battle for Zendikar", "250", "BFZ250", "noimage", "250"},
		{"Plains", "Battle for Zendikar", "Version 1", "BFZ250", "BFZ251", "Version 1"},
		{"Plains", "Battle for Zendikar", "", "noimage", "BFZ251", ""},
		{"Lightning Bolt", "Invasion", "335", "IV335", "IV337", "335"},
		{"Plains", "Foreign Black Border", "293", "FBB293", "FBB294", "293"},
		{"Swamp", "Collector's Edition", "295", "CED295", "CED296", "295"},
	}

	for _, tt := range tests {
		got := basicSKUNumber(tt.name, tt.edition, tt.variation, tt.extra, tt.sku)
		if got != tt.want {
			t.Errorf("basicSKUNumber(%q, %q, %q, %q, %q) = %q, want %q", tt.name, tt.edition, tt.variation, tt.extra, tt.sku, got, tt.want)
		}
	}
}

// A basic the store tags only by its own version index is placed by the number
// its SKU spells, then the one its image names, and only where the landed card
// carries that number. Editions whose SKUs count in the store's own order skip
// the SKU.
func TestMatchBasicByImage(t *testing.T) {
	b := realDatastore(t)
	tests := []struct {
		desc    string
		image   string
		sku     string
		edition string
		want    string
	}{
		{"the image number places the land", "/prodotti/1/1050/BFZ250.jpg", "", "", "BFZ 250"},
		{"a number the land was never printed with is refused", "/prodotti/1/1050/BFZ300.jpg", "", "", ""},
		{"an image naming no number is refused", "/prodotti/1/1050/noimage.jpg", "", "", ""},
		{"the SKU number places the land when the image names none", "/prodotti/1/1050/noimage.jpg", "BFZ250", "", "BFZ 250"},
		{"the SKU number places the land over a wrong image number", "/prodotti/1/1050/BFZ300.jpg", "BFZ0250", "", "BFZ 250"},
		{"a SKU number the land was never printed with is refused", "/prodotti/1/1050/noimage.jpg", "BFZ300", "", ""},
		{"a store-order edition skips the SKU number", "/prodotti/1/1050/noimage.jpg", "FBB293", "Foreign Black Border", ""},
		{"a promo run's SKU places the land in the run's set", "/prodotti/1/1113/p2018PRWK10.jpg", "p2018PRWK10", "Promo", "PRWK A10"},
		{"a promo run's SKU index is its collector number", "/prodotti/1/1113/p2017GIFT01.jpg", "p2017GIFT01", "Promo", "G17 1"},
		{"a promo run's SKU naming another land is refused", "/prodotti/1/1113/p2017GIFT02.jpg", "p2017GIFT02", "Promo", ""},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			input := matcher.InputCard{Name: "Plains", Edition: cmp.Or(tt.edition, "Battle for Zendikar"), Variation: "Version 1"}
			id, found := matchBasicByImage(b, &Card{Extra: tt.image, SKU: tt.sku}, input)
			if tt.want == "" {
				if found {
					co, _ := b.GetUUID(id)
					t.Errorf("matchBasicByImage() = %v, want no match", co)
				}
				return
			}
			co, err := b.GetUUID(id)
			if !found || err != nil || co.SetCode+" "+co.Number != tt.want {
				t.Errorf("matchBasicByImage() = %v, want %s", co, tt.want)
			}
		})
	}
}

// A Holiday Release or Secret Lair Promo SKU counts the store's own index, so
// the number the listing states names the printing over its version tag.
func TestNumberedSKUPrefix(t *testing.T) {
	b := realDatastore(t)
	for _, tt := range []struct {
		name, edition, number, sku, want string
	}{
		{"Goldberry, River-Daughter (V.1)", "The Lord of The Rings: Tales of Middle-earth Holiday Release", "503", "LTRH0503A", "LTR 503"},
		{"Innkeeper's Talent", "DCI Promo", "45", "SLP0040", "SLP 45"},
	} {
		card := Card{Name: tt.name, Edition: tt.edition, Number: tt.number, SKU: tt.sku, Variants: []Variant{{Foil: "Non Foil", Language: "EN"}}}
		input, err := preprocess(b, &card, 0)
		if err != nil {
			t.Fatal(err)
		}
		id, err := b.Match(input)
		if err != nil {
			t.Fatal(err)
		}
		co, err := b.GetUUID(id)
		if err != nil || co.SetCode+" "+co.Number != tt.want {
			t.Errorf("%s (%s) = %v, want %s", tt.name, tt.sku, co, tt.want)
		}
	}
}

// A card the store's version tag cannot place is retried with the number the
// listing states, and answered only where the landed printing carries it.
func TestMatchByListedNumber(t *testing.T) {
	b := realDatastore(t)
	tests := []struct {
		desc    string
		name    string
		edition string
		number  string
		sku     string
		wantSet string
		wantNum string
	}{
		{"a number settles the version tag", "Ransom Note (V.2)", "Commander: Murders at Karlov Manor", "46", "", "MKC", "46"},
		{"a The List number is the tail of its own", "Rout (V.1)", "The List", "34", "", "PLST", "INV-34"},
		{"a number the card was never printed with is refused", "Ransom Note (V.2)", "Commander: Murders at Karlov Manor", "999", "", "", ""},
		{"a listing stating no number is refused", "Ransom Note (V.2)", "Commander: Murders at Karlov Manor", "", "", "", ""},
		{"the SKU's number settles a wrong one", "Vivien Reid (Pre-Release Promo)", "Promo", "0", "p2018M19208", "PM19", "208s"},
		{"a Secret Lair deck sells The List's printing", "Rhys the Redeemed", "Secret Lair Commander Deck: From Cute to Brute", "237", "SLCFCB0237", "PLST", "SHM-237"},
		{"and only one a deck holds", "Rhys the Redeemed", "Secret Lair Commander Deck: From Cute to Brute", "999", "SLCFCB0999", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			card := Card{Name: tt.name, Edition: tt.edition, Number: tt.number, SKU: tt.sku, Variants: []Variant{{Foil: "Non Foil", Language: "EN"}}}
			input, err := preprocess(b, &card, 0)
			if err != nil {
				t.Fatal(err)
			}
			retry := *input

			id, found := matchByListedNumber(b, &card, retry)
			if tt.wantSet == "" {
				if found {
					co, _ := b.GetUUID(id)
					t.Errorf("matchByListedNumber() = %v, want no match", co)
				}
				return
			}
			co, err := b.GetUUID(id)
			if !found || err != nil || co.SetCode != tt.wantSet || co.Number != tt.wantNum {
				t.Errorf("matchByListedNumber() = %v, want %s %s", co, tt.wantSet, tt.wantNum)
			}
		})
	}
}

// A foil listing that landed on the nonfoil printing of a card the set also
// prints as a foil-only star twin is answered with the twin.
func TestFoilStarTwin(t *testing.T) {
	b := realDatastore(t)
	for _, tt := range []struct {
		name    string
		edition string
		wantNum string
	}{
		{"Professional Face-Breaker", "Commander: Modern Horizons 3", "216★"},
		{"Hourglass of the Lost", "Commander: Modern Horizons 3", "92"},
	} {
		input := matcher.InputCard{Name: tt.name, Edition: tt.edition, Foil: true}
		id, err := b.Match(&input)
		if err != nil {
			t.Fatal(err)
		}
		co, err := b.GetUUID(foilStarTwin(b, id))
		if err != nil || co.Number != tt.wantNum {
			t.Errorf("foilStarTwin(%s) = %v, want %s", tt.name, co, tt.wantNum)
		}
	}
}
