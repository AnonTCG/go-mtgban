package strikezone

// Tags that should be wrapped in a variant () but they aren't
var tagsTable = []string{
	"2011 Holiday",
	"2015 Judge Promo",
	"BIBB",
	"BIBB Promo",
	"Blueprint",
	"Bundle Promo",
	"Buy a Box",
	"Convention Foil M19",
	"Draft Weekend",
	"FNM",
	"Full Art Game Day Promo",
	"Full Box Promo",
	"Full Text",
	"Godzilla",
	"GP Promo",
	"Grand Prix 2018",
	"Holiday Promo",
	"Judge 2020",
	"Judge Promo",
	"Jumpstart",
	"League Promo",
	"LGS Promo",
	"M19 Prerelease",
	"MCQ Promo",
	"MagicFest 2019",
	"MagicFest 2020",
	"Media Promo",
	"Players Tour Promo",
	"Players Tour Qualifier PTQ Promo",
	"Prerelease Ixalan",
	"Prerelease Promo",
	"Prerelease Prom",
	"Prerelease",
	"SDCC 2015",
	"SDCC 2017",
	"Secret Lair",
	"Shooting Star Promo",
	"Stained Glass",
	"Standard Showdown 2017",
	"Standard Showdown 2018",
	"Store Champ",
	"Store Championship",
	"X Box Promo 2013",
	"Walmart Promo",
}

// Table for typos and errors in variants and card names
var cardTable = map[string]string{
	// Various promos
	"Flusterstorm Judge Promo (DCI)":         "Flusterstorm (Judge Promo)",
	"Island Ravnica Weekend A003":            "Island Ravnica Weekend A03",
	"Naughty Nice Holiday Promo":             "Naughty (Happy Holidays)",
	"Sol Ring GP MagicFest Promo (Foil)":     "Sol Ring (MagicFest 2019)",
	"Sol Ring GP MagicFest Promo (Non Foil)": "Sol Ring (MagicFest 2019)",
	"Spellseeker Judge":                      "Spellseeker (Judge)",
	"Underworld Dreams (DCI)":                "Underworld Dreams (2HG)",
	"Yixlid Jailer (DCI)":                    "Yixlid Jailer (WPN)",

	// SDCC
	"Ajani Caller of the Pride 2013 SDCC Comicon Promo":  "Ajani Caller of the Pride (2013 SDCC)",
	"Ajani Steadfast (2104 SDCC)":                        "Ajani Steadfast (2014 SDCC)",
	"Chandra Fire of Kaladesh SDCC 2015":                 "Chandra Fire of Kaladesh (2015 SDCC)",
	"Chandra Pyromaster SDCC Comicon Promo (2013)":       "Chandra Pyromaster (2013 SDCC)",
	"Chandra Torch of Defiance SDCC 2017":                "Chandra Torch of Defiance (2017 SDCC)",
	"Chandra Torch of Defiance SDCC 2018":                "Chandra Torch of Defiance (2018 SDCC)",
	"Garruk Apex Predator SDCC 2014":                     "Garruk Apex Predator (2014 SDCC)",
	"Garruk Caller of Beast 2013 SDCC Comicon Promo":     "Garruk Caller of Beasts (2013 SDCC)",
	"Jace Memory Adept 2013 SDCC Comicon Promo":          "Jace Memory Adept (2013 SDCC)",
	"Jace Vryns Prodigy SDCC 2015":                       "Jace Vryns Prodigy (2015 SDCC)",
	"Liliana Heretical Healer SDCC 2015":                 "Liliana Heretical Healer (2015 SDCC)",
	"Liliana of the Dark Realms 2013 SDCC Comicon Promo": "Liliana of the Dark Realms (2013 SDCC)",
	"Nissa Vastwood Seer SDCC 2015":                      "Nissa Vastwood Seer (2015 SDCC)",
	"Nissa Vital Force 2018 SDCC":                        "Nissa Vital Force (2018 SDCC)",

	// Real typos
	"Goldari Thug":               "Golgari Thug",
	"Ink-Eyes Servan of Oni":     "Ink-Eyes Servant of Oni",
	"Laughing Hyenas":            "Laughing Hyena",
	"Teshar Acnestor s Aspostle": "Teshar, Ancestor's Apostle",
	"Rin and Seri Inseperable":   "Rin and Seri, Inseparable",
	"Bloodrot Apothecary":        "Bloodroot Apothecary",
	"Narset, Perter of Veils":    "Narset, Parter of Veils",
	"Fact of Fiction":            "Fact or Fiction",
	"Teferi Hero of Dominaria":   "Teferi, Hero of Dominaria",
	"Sun Ce, Young Conqueror":    "Sun Ce, Young Conquerer",

	// Facet foil digits swapped: FRA 415 is a different card.
	"Return to the Light Realms (Borderless) (Facet Foil) (0415)": "Return to the Light Realms (Borderless) (Facet Foil) (0451)",

	// Typos AND edition
	"Faithless Lootoing (IDW Promo)":                "Faithless Looting (IDW)",
	"Geist of Sain Traft (WMCQ)":                    "Geist of Saint Traft (WMCQ)",
	"Grafdiffer s Cage (Prerelease)":                "Grafdigger's Cage (Prerelease)",
	"Honor the Pure (M10 Release Event Promo)":      "Honor of the Pure (M10 Release Event Promo)",
	"Jace, Weilder of Mysteries Stained Glass":      "Jace, Wielder of Mysteries (Stained Glass)",
	"Karador Ghost Chieftan (Judge Promo)":          "Karador Ghost Chieftain (Judge Promo)",
	"Polukranos Unchanied (Prerelease)":             "Polukranos Unchained (Prerelease)",
	"Selfless Spirt (Eldritch Moon Prerelease)":     "Selfless Spirit (Prerelease)",
	"Showcase Aanax, Hardened in the Forge":         "Anax, Hardened in the Forge (Showcase)",
	"ShowcaseTeferi's Protege":                      "Teferi's Protege (Showcase)",
	"Some Disassembly Require (2017 Holiday Promo)": "Some Disassembly Required (Happy Holidays)",
	"SwampMagic Fest 2019":                          "Swamp (MagicFest 2019)",
	"Sengir, the Dark Baron (BIBB Alt Art Promo)":   "Sengir, the Dark Baron (Prerelease)",
	"Thousand-Year Elixer":                          "Thousand-Year Elixir",
	"Stripe Mine":                                   "Strip Mine",

	"Growing Rites of Ittlimoc (Itlimoc Cradle of the Sun BIBB Alt Art)": "Growing Rites of Itlimoc (BIBB Alt Art)",
	"Legion s Landing Adanto the First Fort (Prerelease)":                "Legion's Landing (Prerelease)",

	// Split cards
	"Discovery Dispersal": "Discovery",
	"Expansion Explosion": "Expansion",
	"Find Finality":       "Find",
	"Fire/Ice (Fire)":     "Fire",
	"Revival Revenge":     "Revival",
	"Thrash Threat":       "Thrash",
	"Connive Concoct":     "Connive",
	"Smelt / Herd / Saw":  "Smelt // Herd // Saw",

	"Needleverge Pathway / Pillarverge Pathway(Borderless": "Needleverge Pathway // Pillarverge Pathway",
	"Sejiri Shelter Sejiri Glacier":                        "Sejiri Shelter // Sejiri Glacier",
	"Silundi Vision Silundi Isle":                          "Silundi Vision // Silundi Isle",
	"Turntimber Symbyosis / Turntimber, Serpentine Wood":   "Turntimber Symbiosis // Turntimber, Serpentine Wood",
	// An "Extended Art" prefix is stripped after the " / " -> " // " pass
	// above already ran, so the second lookup sees the typo with the split
	// this package uses rather than the vendor's own.
	"Turntimber Symbyosis // Turntimber, Serpentine Wood": "Turntimber Symbiosis // Turntimber, Serpentine Wood",
	"Spiked Corridor / Tortured Pit":                      "Spiked Corridor // Torture Pit",

	// The vendor's own product number, wrong or not the card's own.
	"Thanos, the Mad Titan (Borderless) (0367)":                 "Thanos, the Mad Titan (Borderless) (376)",
	"Smaug the Magnificent (247) (Gold Headliner) (Borderless)": "Smaug the Magnificent (Gold Headliner)",

	// Secret Lair drops numbered after the set the card was reprinted from,
	// the Pool Party foil being the dazzle foil printing.
	"Lightning Bolt (042)":                      "Lightning Bolt (IFIYW-2)",
	"Lightning Bolt (042) (Pool Party Foil)":    "Lightning Bolt (IFIYW-7)",
	"Lightning Greaves (382)":                   "Lightning Greaves (IFIYW-4)",
	"Lightning Greaves (382) (Pool Party Foil)": "Lightning Greaves (IFIYW-9)",
	"Sol Ring (288)":                            "Sol Ring (IFIYW-5)",
	"Sol Ring (288) (Pool Party Foil)":          "Sol Ring (IFIYW-10)",

	// Duskmourn double exposure frames, which the store calls Showcase.
	"The Wandering Rescuer (Showcase)":       "The Wandering Rescuer (351)",
	"Valgavoth, Terror Eater (Showcase)":     "Valgavoth, Terror Eater (352)",
	"Tyvar, the Pummeler (Showcase)":         "Tyvar, the Pummeler (353)",
	"Kaito, Bane of Nightmares (Showcase)":   "Kaito, Bane of Nightmares (354)",
	"Niko, Light of Hope (Showcase)":         "Niko, Light of Hope (355)",
	"Toby, Beastie Befriender (Showcase)":    "Toby, Beastie Befriender (356)",
	"The Mindskinner (Showcase)":             "The Mindskinner (357)",
	"Kona, Rescue Beastie (Showcase)":        "Kona, Rescue Beastie (358)",
	"Marvin, Murderous Mimic (Showcase)":     "Marvin, Murderous Mimic (367)",
	"Kaito, Bane of Nightmares (Borderless)": "Kaito, Bane of Nightmares (328)",

	// Listings whose number or treatment word names the base printing
	// instead of the one on sale.
	"The Capitoline Triad (Showcase)":                "The Capitoline Triad (117)",
	"The Arkenstone (Borderless)":                    "The Arkenstone (247)",
	"Narset, Parter of Veils (Borderless)":           "Narset, Parter of Veils (1141)",
	"Dust Bowl (0057) (Borderless) (Galaxy Foil)":    "Dust Bowl (147) (Borderless) (Galaxy Foil)",
	"Boseiju, Who Shelters All (2657) (Neon Yellow)": "Boseiju, Who Shelters All (Neon Yellow)",
	"Kiora, Behemoth Beckoner (Rainbow Foil)":        "Kiora, Behemoth Beckoner (1700)",
	"Mindcrank (Rainbow Foil)":                       "Mindcrank (2186)",
	"Chromatic Lantern (2314) (Confetti Foil)":       "Chromatic Lantern (2329) (Confetti Foil)",
	"Distant Melody (2311) (Confetti Foil)":          "Distant Melody (2326) (Confetti Foil)",
	"Explore (2312) (Confetti Foil)":                 "Explore (2327) (Confetti Foil)",
	"Serum Visions (2323) (Confetti Foil)":           "Serum Visions (2338) (Confetti Foil)",
	"Sol Ring (2315) (Confetti Foil)":                "Sol Ring (2330) (Confetti Foil)",
	"Sphere of Safety (2316) (Confetti Foil)":        "Sphere of Safety (2331) (Confetti Foil)",

	// The number carries the artist's name, a mistyped number, or a lowercase
	// set code.
	"Birds of Paradise 176 Poole":    "Birds of Paradise (176)",
	"Flusterstorm (196 Retro Frame)": "Flusterstorm (496 Retro Frame)",
	"Vivien Reid (m19)":              "Vivien Reid (M19 Prerelease)",

	// Misspelled or mistyped Secret Lair, MagicFest and Step-and-Compleat
	// listings.
	"Spira's Punishment - Day of Judgement":                                "Spira's Punishment - Day of Judgment",
	"Spira's Punishment - Day of Judgement (Rainbow Foil)":                 "Spira's Punishment - Day of Judgment (Rainbow Foil)",
	"Spell Pierce (61)":                                                    "Spell Pierce (41)",
	"Archangel of Tunes - Archangel of Thune (234)":                        "Archangel of Tunes - Archangel of Thune (2430)",
	"Counterspell (Retro Frame) (2)":                                       "Counterspell (Retro Frame) (5)",
	"Nissa, Who Shakes the World (Retro Frame)":                            "Nissa, Who Shakes the World (1042)",
	"Elesh Norn, Mother of Machines (Borderless) (Step-and-Compleat Foil)": "Elesh Norn, Mother of Machines (419)",
	"Elesh Norn, Mother of Machines (Showcase) (Step-and-Compleat Foil)":   "Elesh Norn, Mother of Machines (420)",

	// The rainbow foil 741 star is the Chaos Theory listing, which names
	// itself, so the bare name is the 823 bonus card.
	"Chaos Warp (Rainbow Foil)": "Chaos Warp (823)",

	// The List art words, each naming the one reprint drawn that way.
	"Aura Shards (Commander Art)":    "Aura Shards (CMD-182)",
	"Demonic Tutor (Duel Deck Art)":  "Demonic Tutor (DDC-49)",
	"Lightning Bolt (Magicfest Art)": "Lightning Bolt (PF19-1)",

	// Premiere Shop lands spelled with a dash.
	"Plains - Orzhov Syndicate": "Plains (The Orzhov Syndicate)",
	"Swamp - Orzhov Syndicate":  "Swamp (The Orzhov Syndicate)",

	// Funny cards
	"(Untitled Card": "_____",

	// Misspellings, each read off the printing the set holds
	"Assassin s Trophy":         "Assassin's Trophy",
	"Frosteliff Siege":          "Frostcliff Siege",
	"Kefka, Court Magem":        "Kefka, Court Mage // Kefka, Ruler of Ruin",
	"Maha, Its Feather Night":   "Maha, Its Feathers Night",
	"Nine Live":                 "Nine Lives",
	"Samut, Hazorer's Champion": "Samut, Hazoret's Champion",
	"Sauron, Dino Devoteee":     "Sauron, Dino Devotee",
	"Swords to Plowshare":       "Swords to Plowshares",
}

// Map a card/edition set to a correct edition
var card2setTable = map[string]string{
	"Counterspell (Arena Non Foil)":        "DCI Legend Membership",
	"Nicol Bolas Planeswalker (Archenemy)": "Archenemy: Nicol Bolas",
	"Serra Angel (Beta Art)":               "Wizards of the Coast Online Store",

	"Glistener Elf (WPN)":             "Friday Night Magic 2012",
	"Lingering Souls (WPN)":           "Friday Night Magic 2012",
	"Lightning Bolt (Beta Art Promo)": "Judge Gift Cards 1998",
	"Wood Elves (Promo)":              "DCI",
	"Elvish Lyrist (FNM)":             "Junior Super Series",

	"Tempered Steel (Full Art Textless)": "Scars of Mirrodin Promos",

	// Future Sight frame promos the store files under its Secret Lair
	// shelf, which never says the drop the way secretLairDrops needs.
	"The First Sliver (Future Sight) (003)": "PF25",
	"The Ur-Dragon (003) (Future Sight)":    "PF25",
	"Sliver Hive (Retro Frame)":             "PF25",
	"Ugin, the Spirit Dragon (Retro Frame)": "PF25",
	"Ponder (Future Sight) (002)":           "PF25",
}

// shelfPrintings are the cards a shelf lists that another printing of the
// same name shares, keyed by shelf, name and the variation the listing
// leaves after the name is split, with the set and number of the one the
// shelf holds. A Secret Lair Commander deck's decklist says which reprint it
// contains, and the Lord of the Rings shelf holds the plain Sauron beside the
// Pro Tour promo and the plain borderless One Ring beside the poster one.
var shelfPrintings = map[[3]string][2]string{
	{"Secret Lair Commander: From Cute to Brute", "Pongify", ""}:           {"PLST", "PLC-44"},
	{"Secret Lair Commander: From Cute to Brute", "Rhys the Redeemed", ""}: {"PLST", "SHM-237"},
	{"Secret Lair Commander: Heads I Win", "Arcane Signet", ""}:            {"PLST", "C21-234"},
	{"Secret Lair Commander: Heads I Win", "Ponder", ""}:                   {"PLST", "C21-125"},
	{"Secret Lair Commander: Heads I Win", "Reshape", ""}:                  {"PLST", "2XM-64"},

	{"Promos: Magicfest", "Arcane Signet", ""}:  {"P30M", "1F"},
	{"Promos: Magicfest", "Lightning Bolt", ""}: {"PF19", "1"},

	{"Secret Lair", "Counterspell", "0002"}: {"PURL", "2"},

	{"Promos: Unique and Miscellaneous", "Jace Beleren", ""}:                  {"PSPL", "13"},
	{"Promos: Unique and Miscellaneous", "Lightning Bolt", "0002 Borderless"}: {"PW26", "5"},

	{"Universes Beyond: The Lord of the Rings: Tales of Middle-earth", "Sauron, the Dark Lord", ""}:  {"LTR", "224"},
	{"Universes Beyond: The Lord of the Rings: Tales of Middle-earth", "The One Ring", "Borderless"}: {"LTR", "451"},
}

// These cards don't have any variant, we know they are Promotional Cards,
// but we can't rely on the automatic matching
var promo2setTable = map[string]string{
	"Show and Tell":             "Judge Gift Cards 2013",
	"Overwhelming Forces":       "Judge Gift Cards 2013",
	"Nekusar the Mindrazer":     "Judge Gift Cards 2014",
	"Hanna Ship s Navigator":    "Judge Gift Cards 2014",
	"Riku of Two Reflections":   "Judge Gift Cards 2014",
	"Feldon of the Third Path":  "Judge Gift Cards 2015",
	"Yuriko the Tiger s Shadow": "Judge Gift Cards 2019",
	"Teetering Peaks":           "Friday Night Magic 2011",
	"Dismember":                 "Friday Night Magic 2012",
	"Ancient Grudge":            "Friday Night Magic 2012",
	"Prophet of Kruphix":        "Magic 2015 Clash Pack",
}
