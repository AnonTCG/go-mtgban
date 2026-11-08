package abugames

// These listings copy another printing's shelf and number, misspell the
// title, or copy an id. The product image supplies the printed set/number/
// title; match it and the original wording so a revised listing is
// rechecked. An empty corrected field leaves that part of the listing as
// the vendor wrote it.
var listingCorrections = map[string]struct {
	title, edition, number                            string
	correctedTitle, correctedEdition, correctedNumber string
	ignoreIDs                                         bool
}{
	"8124578": {title: "Path of Ancestry (The List)", edition: "Outlaws of Thunder Junction Commander", number: "310", correctedEdition: "Modern Horizons 3 Commander", correctedNumber: "363"},
	"8098901": {title: "Gempalm Incinerator (The List)", edition: "Elves vs. Goblins", number: "37", correctedEdition: "Duel Decks: Merfolk vs. Goblins", correctedNumber: "39"},
	"8107290": {title: "Gempalm Incinerator (The List)", edition: "Duel Decks Anthology: Elves vs. Goblins", number: "37", correctedEdition: "Duel Decks: Merfolk vs. Goblins", correctedNumber: "39"},
	"8149750": {title: "Mana Geyser (The List)", edition: "Secrets of Strixhaven Commander", number: "247", correctedEdition: "Commander 2021", correctedNumber: "176"},
	// The pictured WPN Premium artwork identifies PW24 #1, not #22.
	"8111219": {title: "Serra Angel (WPN Borderless) - FOIL", edition: "Wizards Play Network 2022", number: "1", correctedEdition: "Wizards Play Network 2024", correctedNumber: "1"},
	// The copied ids name the untagged schematic twin; the image doesn't.
	"7842864": {title: "Pristine Talisman", edition: "The Brothers' War Retro Artifacts", number: "106", correctedNumber: "43", ignoreIDs: true},
	"7843549": {title: "Pristine Talisman - FOIL", edition: "The Brothers' War Retro Artifacts", number: "106", correctedNumber: "43", ignoreIDs: true},
	// The copied ids and card_number both name the Extended Art twin.
	"7918400": {title: "Cirdan the Shipwright", edition: "The Lord of the Rings: Tales of Middle-earth Commander", number: "133", correctedNumber: "50", ignoreIDs: true},
	// card_number (208) is the set's Tutorial Card, not this one's own 218.
	"8114685": {title: "Sledding Otter-Penguin - FOIL", edition: "Avatar: The Last Airbender Eternal", number: "208", correctedNumber: "218"},
	// The numbers are swapped between the nonfoil and FOIL products; only
	// the TCGplayer id names the nonfoil 820, the Scryfall id is the 820★.
	"8037997": {title: "Arcane Signet (Secret Lair Sketch 820)", edition: "Secret Lair Drop", number: "820★", correctedNumber: "820"},
	// The title's parenthetical is a typo for the card_number.
	"7895279": {title: "Harnessed Snubhorn (HALO 168) - FOIL", edition: "March of the Machine: The Aftermath", number: "188", correctedTitle: "Harnessed Snubhorn (HALO 188) - FOIL"},
	"8054274": {title: "Searslicer Goblin (Borderless Mana 393) - FOIL", edition: "Foundations", number: "394", correctedTitle: "Searslicer Goblin (Borderless Mana 394) - FOIL"},
	"8147346": {title: "Island (346 Surge) - FOIL", edition: "Marvel Super Heroes", number: "436", correctedTitle: "Island (436 Surge) - FOIL"},
	"7966260": {title: "Missy (Surge 614) - FOIL", edition: "Doctor Who", number: "613", correctedTitle: "Missy (Surge 613) - FOIL"},
	// The title's Secret Lair number is a typo; card_number and the ids
	// name the other drop.
	"8036192": {title: "Homeward Path (Secret Lair 2811)", edition: "Secret Lair Drop", number: "1467", correctedTitle: "Homeward Path (Secret Lair 1467)"},
	"8036194": {title: "Homeward Path (Secret Lair 2811) - FOIL", edition: "Secret Lair Drop", number: "1467★", correctedTitle: "Homeward Path (Secret Lair 1467) - FOIL"},
	"8167207": {title: "Arcane Signet (Secret Lair 2464) - FOIL", edition: "Secret Lair Drop", number: "2816", correctedTitle: "Arcane Signet (Secret Lair 2816) - FOIL"},
	// The Elves vs. Goblins run (167847-167866) is the 2007 deck; the
	// Anthology edition and the ids belong to The List copy.
	"167849": {title: "Ambush Commander - FOIL", edition: "Duel Decks Anthology: Elves vs. Goblins", number: "1", correctedEdition: "Elves vs. Goblins", ignoreIDs: true},
	// The plain printing's number and ids are copied onto the variant.
	"7911360": {title: "Gandalf, White Rider (Extended Art)", edition: "The Lord of the Rings: Tales of Middle-earth", number: "290", correctedNumber: "389", ignoreIDs: true},
	"7429110": {title: "Captain's Claws (Goldnight Castigator Shadow) - FOIL", edition: "Oath of the Gatewatch", number: "162", correctedNumber: "162†", ignoreIDs: true},
	// The listings misspell the flavor name, or splice the finish into it.
	"8100899": {title: "Chancla relampagos | Lightning Greaves (Secret Lair)", edition: "Secret Lair Drop", number: "2062", correctedTitle: "Chancla relámpago | Lightning Greaves (Secret Lair)"},
	"8100901": {title: "Chancla relampagos | Lightning Greaves (Secret Lair) - FOIL", edition: "Secret Lair Drop", number: "2062", correctedTitle: "Chancla relámpago | Lightning Greaves (Secret Lair) - FOIL"},
	"8035587": {title: "Inigo, Avenging  - FOILSwordsman | Samut, Voice of Dissent (Secret Lair) - FOIL", edition: "Secret Lair Drop", number: "1451", correctedTitle: "Inigo, Avenging Swordsman | Samut, Voice of Dissent (Secret Lair) - FOIL"},
	// Filed as "(Prerelease)"; the catalog only carries the release promo.
	"162808": {title: "Reya Dawnbringer (Prerelease) - FOIL", edition: "Tenth Edition Promos", number: "35", correctedTitle: "Reya Dawnbringer (Release) - FOIL"},
	"206639": {title: "Reya Dawnbringer (Prerelease) - FOIL", edition: "Tenth Edition Promos", number: "35", correctedTitle: "Reya Dawnbringer (Release) - FOIL"},
	"206640": {title: "Reya Dawnbringer (Prerelease) - FOIL", edition: "Tenth Edition Promos", number: "35", correctedTitle: "Reya Dawnbringer (Release) - FOIL"},
	// The number or shelf is a typo; both ids name the printing the
	// corrected number and edition below describe.
	"8091744": {title: "Traveling Chocobo (Neon Ink Yellow) - FOIL", edition: "Final Fantasy", number: "511a", correctedNumber: "551a"},
	"8091746": {title: "Traveling Chocobo (Neon Ink Pink) - FOIL", edition: "Final Fantasy", number: "511b", correctedNumber: "551b"},
	"8091748": {title: "Traveling Chocobo (Neon Ink Blue) - FOIL", edition: "Final Fantasy", number: "511c", correctedNumber: "551c"},
	"8091750": {title: "Traveling Chocobo (Neon Ink Green) - FOIL", edition: "Final Fantasy", number: "511d", correctedNumber: "551d"},
	"8129394": {title: "Mox Jasper (Retro Frame Serial Numbered) - FOIL", edition: "Tarkir: Dragonstorm", number: "325", correctedNumber: "419"},
	"8129659": {title: "Birds of Paradise (Retro Frame Serial Numbered) - FOIL", edition: "Dominaria Remastered", number: "336z", correctedEdition: "Ravnica Remastered", correctedNumber: "344z"},
	"8130042": {title: "Lord of the Pit (Secret Lair Serial Numbered)", edition: "Secret Lair Drop", number: "893z", correctedNumber: "715"},
	"8111157": {title: "Gandalf, Friend of the Shire (Play) - FOIL", edition: "The Lord of the Rings: Tales of Middle-earth", number: "1", correctedEdition: "MagicFest 2023"},
	// The title leaves out or misspells the finish; card_style names the foil.
	"7968107": {title: "Faramir, Steward of Gondor (Scroll Showcase)", edition: "The Lord of the Rings: Tales of Middle-earth Commander", number: "458", correctedTitle: "Faramir, Steward of Gondor (Scroll Showcase) - FOIL"},
	"8111178": {title: "Exhume (Borderless) - FOIOL", edition: "Duskmourn: House of Horror Commander", number: "370", correctedTitle: "Exhume (Borderless) - FOIL"},
	"7790548": {title: "Shelldock Isle (Secret Lair 437 Amayagido)", edition: "Secret Lair Drop", number: "437", correctedTitle: "Shelldock Isle (Secret Lair 437 Amayagido) - FOIL"},
	"8093462": {title: "Porom's Silence Magic | Silence (Secret Lair)", edition: "Promo", number: "7003", correctedTitle: "Porom's Silence Magic | Silence (Secret Lair) - FOIL"},
	"8134488": {title: "Angel's Grace (Japanese Alternate Art 132)", edition: "Secrets of Strixhaven Mystical Archive", number: "132", correctedTitle: "Angel's Grace (Japanese Alternate Art 132) - FOIL"},
	"8163734": {title: "Angel's Grace (Japanese Alternate Art 132)", edition: "Secrets of Strixhaven Mystical Archive", number: "132", correctedTitle: "Angel's Grace (Japanese Alternate Art 132) - FOIL"},
	"8149603": {title: "Angel's Grace (Japanese Alternate Art 132)", edition: "Secrets of Strixhaven Mystical Archive", number: "132", correctedTitle: "Angel's Grace (Japanese Alternate Art 132) - FOIL"},
	// The ids and card_number name a printing the wording cannot reach.
	"8169031": {title: "Cloud, Midgar Mercenary (Game Edition) - FOIL", edition: "Media Inserts", number: "1", correctedNumber: "2025-21"},
	"7759001": {title: "Vito, Thorn of the Dusk Rose (Resale Promo) - FOIL", edition: "Resale Promos", number: "127", correctedTitle: "Vito, Thorn of the Dusk Rose - FOIL", correctedEdition: "Core Set 2021 Promos", correctedNumber: "127★"},
	"226811":  {title: "Pristine Talisman (Game Day Mirrodin Pure Preview)", edition: "New Phyrexia Promos", number: "151", correctedTitle: "Pristine Talisman (Game Day)"},
	"8127183": {title: "Shredder, Shadow Master (Surge 202) - FOIL", edition: "Teenage Mutant Ninja Turtles Eternal", number: "20", correctedTitle: "Shredder, Shadow Master (Surge 20) - FOIL"},
	"8118922": {title: "Spider-Ham, Peter Porker (Commander Play)", edition: "Promo", number: "02", correctedTitle: "Spider-Ham, Peter Porker"},
	"8118924": {title: "Spider-Ham, Peter Porker (Commander Play) - FOIL", edition: "Promo", number: "02", correctedTitle: "Spider-Ham, Peter Porker - FOIL"},
	"8100302": {title: "Nexus of Fate (Buy-a-Box Promo) - (The List)", edition: "Core Set 2019 / M19", number: "306", correctedTitle: "Nexus of Fate (The List)"},
	"8102253": {title: "The Haunt of Hightower (Buy-a-Box Promo) - (The List)", edition: "Ravnica Allegiance", number: "273", correctedTitle: "The Haunt of Hightower (The List)"},
}

func correctListing(card ABUCard) ABUCard {
	fix, ok := listingCorrections[card.ProductID]
	if !ok || card.DisplayTitle != fix.title || card.Edition != fix.edition || card.Number != fix.number {
		return card
	}
	if fix.correctedTitle != "" {
		card.DisplayTitle = fix.correctedTitle
	}
	if fix.correctedEdition != "" {
		card.Edition = fix.correctedEdition
	}
	if fix.correctedNumber != "" {
		card.Number = fix.correctedNumber
	}
	if fix.ignoreIDs {
		card.ScryfallIDs, card.TCGplayerIDs, card.MultiverseIDs = nil, nil, nil
	}
	return card
}
