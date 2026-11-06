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
