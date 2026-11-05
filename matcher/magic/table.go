package magic

var missingPELPtags = map[string]string{
	"1":  "Schwarzwald, Germany",
	"2":  "Danish Island, Scandinavia",
	"3":  "Vesuvio, Italy",
	"4":  "Scottish Highlands, United Kingdom, U.K.",
	"5":  "Ardennes Fagnes, Belgium",
	"6":  "Brocéliande, France",
	"7":  "Venezia, Italy",
	"8":  "Pyrenees, Spain",
	"9":  "Lowlands, Netherlands",
	"10": "Lake District National Park, United Kingdom, U.K.",
	"11": "Nottingham Forest, United Kingdom, U.K.",
	"12": "White Cliffs of Dover, United Kingdom, U.K.",
	"13": "Mont Blanc, France",
	"14": "Steppe Tundra, Russia",
	"15": "Camargue, France",
}

var missingPALPtags = map[string]string{
	"1":  "Japan",
	"2":  "Hong Kong",
	"3":  "Banaue Rice Terraces, Philippines",
	"4":  "Japan",
	"5":  "New Zealand",
	"6":  "China",
	"7":  "Meoto Iwa, Japan",
	"8":  "Taiwan",
	"9":  "Uluru, Australia",
	"10": "Japan",
	"11": "Korea",
	"12": "Singapore",
	"13": "Mount Fuji, Japan",
	"14": "Great Wall of China",
	"15": "Indonesia",
}

// The language a foreign-only set is sold in, of the several it was printed in
var forcedLanguages = map[string]string{
	"FBB": "Italian",
	"4BB": "Japanese",
}

// List of numbers in SLD that need to be decoupled, each with its starred
// sibling where it has one
var sldJPNLangDupes = []string{
	// Special Guests Yoji Shinkawa
	"1110", "1111", "1112", "1113",
	// Special Guests Junji Ito
	"1114", "1115", "1116", "1117",
	// Miku Sakura Superstar
	"1587", "1594", "1596", "1597", "805", "808",
	// Miku Digital Sensation
	"1592", "1595", "1599", "1603", "1604", "1607", "806",
	// Miku Electric Entourage
	"1585", "1590", "1593", "1598", "1600", "807",
	// Miku Winter Diva
	"1586", "1588", "1589", "1591", "1601", "1606", "804",
	// Feather, the Redeemed and Inspiring Vantage
	"1602", "1605",
	// Final Fantasy Game Over
	"1858", "1859", "1860", "1861", "1862",
	// Final Fantasy Weapons
	"1863", "1864", "1865", "1866", "1867",
	// Final Fantasy Grimoire
	"1868", "1869", "1870", "1871", "1872",
	// Secret Lair bonus cards
	"7001", "7002", "7003",
	// Summer Superdrop 2025 promo
	"909",
}

// productsWithOnlyFoils names sealed products whose every card is foil and
// whose name does not say so. A product belonging to a line that is foil
// throughout needs no entry: sealedHoldsOnlyFoils names the line instead.
//
// Verified against the sealed decklists: every non-token card each of these
// holds is foil.
var productsWithOnlyFoils = []string{
	"2017 Magic The Gathering Hascon Collection",
	"Commander's Arsenal",
	"Commanders Arsenal",
	"Final Fantasy Box Set Camp Comrades",
	"Final Fantasy Box Set Children of Fate",
	"Final Fantasy Box Set Garland at the Chaos Shrine",
	"Final Fantasy Box Set The Siege of Alexandria",
	"Judge Promo Pack 2014 Full Art Land Set",
	"Ponies The Galloping",
	"Secret Lair Bundle Theros Stargazing Vol I-V",
	"Secret Lair Drop Animar and Friends",
	"Secret Lair Drop April Fools",
	"Secret Lair Drop Calling All Hydra Heads",
	"Secret Lair Drop Can You Feel with a Heart of Steel",
	"Secret Lair Drop Eldraine Wonderland",
	"Secret Lair Drop Extra Life 2020",
	"Secret Lair Drop Here Be Dragons",
	"Secret Lair Drop International Women's Day 2020",
	"Secret Lair Drop International Womens Day 2020",
	"Secret Lair Drop Kaleidoscope Killers",
	"Secret Lair Drop LOOK AT THE KITTIES",
	"Secret Lair Drop MagicCon The Gathering",
	"Secret Lair Drop More Borderless Planeswalkers",
	"Secret Lair Drop Mountain Go",
	"Secret Lair Drop OMG KITTIES",
	"Secret Lair Drop Secret Lair x MSCHF",
	"Secret Lair Drop Seeing Visions",
	"Secret Lair Drop Showcase March of the Machine Vol 1",
	"Secret Lair Drop Showcase March of the Machine Vol 2",
	"Secret Lair Drop Showcase March of the Machine Vol 3",
	"Secret Lair Drop Slay the Day",
	"Secret Lair Drop So Salty",
	"Secret Lair Drop Thalia Beyond the Helvault",
	"Secret Lair Drop The Godzilla Lands",
	"Secret Lair Drop The Path Not Traveled",
	"Secret Lair Drop The Walking Dead",
	"Secret Lair Drop Theros Stargazing Vol I Heliod",
	"Secret Lair Drop Theros Stargazing Vol II Thassa",
	"Secret Lair Drop Theros Stargazing Vol III Erebos",
	"Secret Lair Drop Theros Stargazing Vol IV Purphoros",
	"Secret Lair Drop Theros Stargazing Vol V Nylea",
	"Secret Lair Drop Viva Las Rakdos",
	"Secret Lair Drop We Hope You Like Squirrels",
	"Secret Lair Drop Wild in Bloom",
	"Secret Lair Drop Year of the Rat",
	"Secret Lair Festival in a Box Minneapolis 2023 Bundle",
	"Secret Lair Festival in a Box Philadelphia 2023 Bundle",
	"Secret Lair Ultimate Edition 2 Box Black",
	"Secret Lair Ultimate Edition 2 Box Grey",
}

var mtgColorNameMap = map[string]string{
	"W": "white",
	"U": "blue",
	"B": "black",
	"R": "red",
	"G": "green",
}

// Cards whose names are long enough or odd enough to break naive parsing, kept
// here so tests and callers can reach for them by name.
const (
	LongestCardEver = "Our Market Research Shows That Players Like Really Long Card Names So We Made this Card to Have the Absolute Longest Card Name Ever Elemental"
	NightmareCard   = "The Ultimate Nightmare of Wizards of the Coast® Customer Service"
)

// grnGuilds are the guilds printed in Guilds of Ravnica.
var grnGuilds = []string{"Boros", "Dimir", "Golgari", "Izzet", "Selesnya"}

// arnGuilds are the guilds printed in Ravnica Allegiance.
var arnGuilds = []string{"Azorius", "Gruul", "Orzhov", "Rakdos", "Simic"}
