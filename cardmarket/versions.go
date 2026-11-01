package cardmarket

import (
	cm "github.com/mtgban/go-cardmarket"

	"github.com/mtgban/go-mtgban/matcher"
)

// versionPrintings names the printing a Magic product sells where the id map
// cannot tell: a "(V.N)" version the map links to no printing of its shelf,
// to a sibling's, or to several, or the starred foil twin of a Secret Lair
// printing the map links to the plain one. Keyed by Cardmarket's product id;
// the tag after each name says what placed the row, and
// docs/agents/cardmarket-census/versions.md what each tag means.
var versionPrintings = map[int]struct{ set, number string }{
	// Arabian Nights
	6794: {"ARN", "33"}, // Stone-Throwing Devils (V.1), map lists two

	// Pro Tour 1996: Bertrand Lestree
	22465: {"PTC", "bl17sb"}, // Circle of Protection: Red (V.1), map lists two
	22471: {"PTC", "bl16b"},  // Order of Leitbur (V.1), map lists two
	22496: {"PTC", "bl366"},  // Plains (V.3), map lists two

	// Pro Tour 1996: Eric Tam
	22658: {"PTC", "et83sb"}, // Autumn Willow (V.2), map lists two
	22683: {"PTC", "et374"},  // Mountain (V.1), map lists two
	22690: {"PTC", "et365"},  // Plains (V.3), map lists two
	22697: {"PTC", "et52sb"}, // Swords to Plowshares (V.2), map lists two

	// Pro Tour 1996: George Baxter
	22709: {"PTC", "gb38c"}, // Hymn to Tourach (V.3), map lists two
	22728: {"PTC", "gb371"}, // Swamp (V.1), map lists two

	// Pro Tour 1996: Leon Lindback
	22750: {"PTC", "ll54sb"}, // Jalum Tome (V.2), map lists two
	22757: {"PTC", "ll42c"},  // Order of the Ebon Hand (V.2), map lists two
	22774: {"PTC", "ll38b"},  // Hymn to Tourach (V.4), map lists two

	// Pro Tour 1996: Mark Justice
	22781: {"PTC", "mj184sb"}, // Detonate (V.2), map lists two
	22804: {"PTC", "mj365"},   // Plains (V.3), map lists two

	// Pro Tour 1996: Michael Locanto
	22841: {"PTC", "ml365"}, // Plains (V.2), map lists two

	// Pro Tour 1996: Preston Poulter
	22872: {"PTC", "pp377"}, // Forest (V.2), map lists two
	22883: {"PTC", "pp366"}, // Plains (V.1), map lists two

	// Pro Tour 1996: Shawn Regnier
	22908: {"PTC", "shr368"}, // Island (V.1), map lists two
	22919: {"PTC", "shr365"}, // Plains (V.3), map lists two

	// WCD 1997: Jakub Slemr
	242161: {"WC97", "js440"}, // Swamp (V.3), image
	242162: {"WC97", "js438"}, // Swamp (V.4), swap

	// WCD 1997: Janosch Kuhn
	242108: {"WC97", "jk26"},   // Disenchant (V.1), sideboard
	242127: {"WC97", "jk26sb"}, // Disenchant (V.2), sideboard
	242131: {"WC97", "jk445"},  // Mountain (V.2), image
	242133: {"WC97", "jk442"},  // Mountain (V.4), swap
	242135: {"WC97", "jk433"},  // Plains (V.3), CardTrader

	// WCD 1997: Paul McCabe
	250104: {"WC97", "pm434"}, // Island (V.1), swap
	250106: {"WC97", "pm436"}, // Island (V.3), image
	250108: {"WC97", "pm442"}, // Mountain (V.1), swap
	250110: {"WC97", "pm444"}, // Mountain (V.3), image
	250111: {"WC97", "pm445"}, // Mountain (V.4), CardTrader

	// WCD 1997: Svend Geertsen
	250070: {"WC97", "sg341"},   // Whirling Dervish (V.1), sideboard
	250071: {"WC97", "sg341sb"}, // Whirling Dervish (V.2), sideboard
	250073: {"WC97", "sg123sb"}, // Uktabi Orangutan (V.2), CardTrader
	250080: {"WC97", "sg446"},   // Forest (V.1), swap
	250082: {"WC97", "sg448"},   // Forest (V.3), image

	// WCD 1998: Randy Buehler
	249897: {"WC98", "rb335"}, // Island (V.1), swap
	249900: {"WC98", "rb338"}, // Island (V.4), image

	// WCD 1998: Brian Hacker
	249872: {"WC98", "bh7bsb"}, // Aura of Silence (V.2), CardTrader
	249878: {"WC98", "bh331"},  // Plains (V.1), swap
	249881: {"WC98", "bh334"},  // Plains (V.4), image

	// WCD 1998: Ben Rubin
	249851: {"WC98", "br343"}, // Mountain (V.1), image
	249852: {"WC98", "br344"}, // Mountain (V.2), swap
	249854: {"WC98", "br346"}, // Mountain (V.4), CardTrader

	// WCD 1998: Brian Selden
	249818: {"WC98", "bs347"}, // Forest (V.1), swap
	249820: {"WC98", "bs349"}, // Forest (V.3), image

	// WCD 1999: Jakub Šlemr
	249777: {"WC99", "js67"},   // Rapid Decay (V.1), sideboard
	249778: {"WC99", "js67sb"}, // Rapid Decay (V.2), sideboard
	249786: {"WC99", "js340b"}, // Swamp (V.3), left over

	// WCD 1999: Matt Linde
	249756: {"WC99", "ml347b"}, // Forest (V.3), left over

	// WCD 1999: Mark Le Pine
	249720: {"WC99", "mlp173"},   // Fireslinger (V.1), sideboard
	249721: {"WC99", "mlp173sb"}, // Fireslinger (V.2), sideboard
	249734: {"WC99", "mlp344"},   // Mountain (V.2), image
	249735: {"WC99", "mlp346"},   // Mountain (V.3), left over

	// WCD 1999: Kai Budde
	249700: {"WC99", "kb302"},   // Mishra's Helix (V.1), sideboard
	249701: {"WC99", "kb302sb"}, // Mishra's Helix (V.2), sideboard

	// WCD 2000: Tom Van de Logt
	249640: {"WC00", "tvdl18sb"}, // Seal of Cleansing (V.2), CardTrader
	249648: {"WC00", "tvdl54sb"}, // Wrath of God (V.2), CardTrader

	// WCD 2000: Janosch Kühn
	249607: {"WC00", "jk134"},   // Masticore (V.1), sideboard
	249608: {"WC00", "jk134sb"}, // Masticore (V.2), sideboard
	249611: {"WC00", "jk220"},   // Creeping Mold (V.1), sideboard
	249612: {"WC00", "jk220sb"}, // Creeping Mold (V.2), sideboard

	// WCD 2000: Jon Finkel
	249585: {"WC00", "jf302sb"}, // Mishra's Helix (V.2), CardTrader

	// WCD 2001: Jan Tomcani
	249555: {"WC01", "jt191sb"}, // Kavu Chameleon (V.1), CardTrader
	249561: {"WC01", "jt348"},   // Forest (V.1), image
	249562: {"WC01", "jt328"},   // Forest (V.2), swap
	249565: {"WC01", "jt343a"},  // Mountain (V.1), image
	249566: {"WC01", "jt337"},   // Mountain (V.2), swap
	250142: {"WC01", "jt349a"},  // Forest (V.6), CardTrader

	// WCD 2001: Antoine Ruel
	249523: {"WC01", "ar67"},  // Counterspell (V.1), swap
	250137: {"WC01", "ar338"}, // Island (V.5), image
	250140: {"WC01", "ar334"}, // Island (V.8), left over
	316302: {"WC01", "ar69"},  // Counterspell (V.2), image

	// WCD 2001: Alex Borteh
	249512: {"WC01", "ab338"},  // Island (V.4), CardTrader
	250127: {"WC01", "ab336"},  // Island (V.6), image
	250129: {"WC01", "ab338a"}, // Island (V.8), CardTrader
	250130: {"WC01", "ab332"},  // Island (V.9), swap

	// WCD 2001: Tom van de Logt
	249474: {"WC01", "tvdl97"},   // Crypt Angel (V.1), sideboard
	249475: {"WC01", "tvdl97sb"}, // Crypt Angel (V.2), sideboard
	249482: {"WC01", "tvdl339"},  // Swamp (V.1), swap
	249487: {"WC01", "tvdl343b"}, // Mountain (V.2), image
	249488: {"WC01", "tvdl337"},  // Mountain (V.3), swap
	250135: {"WC01", "tvdl347"},  // Swamp (V.5), image

	// WCD 2002: Sim Han How
	249453: {"WC02", "shh347"},  // Forest (V.1), image
	249454: {"WC02", "shh348"},  // Forest (V.2), CardTrader
	249455: {"WC02", "shh349"},  // Forest (V.3), CardTrader
	249457: {"WC02", "shh335"},  // Island (V.1), CardTrader
	249458: {"WC02", "shh336a"}, // Island (V.2), CardTrader
	249459: {"WC02", "shh337"},  // Island (V.3), CardTrader
	249460: {"WC02", "shh338"},  // Island (V.4), image
	250041: {"WC02", "shh328"},  // Forest (V.5), swap
	250042: {"WC02", "shh329"},  // Forest (V.6), CardTrader
	250043: {"WC02", "shh330"},  // Forest (V.7), CardTrader
	250044: {"WC02", "shh331"},  // Forest (V.8), CardTrader
	250045: {"WC02", "shh334"},  // Island (V.5), swap

	// WCD 2002: Raphael Levy
	249423: {"WC02", "rl30sb"}, // Rushing River (V.2), CardTrader
	249427: {"WC02", "rl347"},  // Forest (V.1), CardTrader
	249428: {"WC02", "rl348"},  // Forest (V.2), CardTrader
	249429: {"WC02", "rl349"},  // Forest (V.3), CardTrader
	249430: {"WC02", "rl350"},  // Forest (V.4), CardTrader
	250047: {"WC02", "rl329"},  // Forest (V.6), CardTrader
	250048: {"WC02", "rl330"},  // Forest (V.7), CardTrader
	250049: {"WC02", "rl331"},  // Forest (V.8), CardTrader
	250052: {"WC02", "rl332"},  // Island (V.7), swap
	250053: {"WC02", "rl333"},  // Island (V.8), image
	250054: {"WC02", "rl334"},  // Island (V.9), CardTrader

	// WCD 2002: Brian Kibler
	249382: {"WC02", "bk11sb"},  // Glory (V.2), CardTrader
	249390: {"WC02", "bk347"},   // Forest (V.1), image
	249393: {"WC02", "bk328"},   // Forest (V.4), swap
	249396: {"WC02", "bk327sb"}, // City of Brass (V.2), CardTrader
	249402: {"WC02", "bk331a"},  // Plains (V.1), swap
	249403: {"WC02", "bk333"},   // Plains (V.2), image

	// WCD 2002: Carlos Romao
	249351: {"WC02", "cr57sb"}, // Fact or Fiction (V.2), CardTrader
	249364: {"WC02", "cr340"},  // Swamp (V.1), swap
	249365: {"WC02", "cr341"},  // Swamp (V.2), image
	250057: {"WC02", "cr332"},  // Island (V.7), swap
	250060: {"WC02", "cr335b"}, // Island (V.10), image

	// WCD 2003: Peer Kröger
	249313: {"WC03", "pk118"},   // Buried Alive (V.1), sideboard
	249314: {"WC03", "pk118sb"}, // Buried Alive (V.2), sideboard
	249317: {"WC03", "pk62sb"},  // Cabal Therapy (V.2), CardTrader
	249320: {"WC03", "pk216"},   // Recoup (V.1), sideboard
	249321: {"WC03", "pk216sb"}, // Recoup (V.2), sideboard
	249323: {"WC03", "pk72sb"},  // Stitch Together (V.2), CardTrader
	249327: {"WC03", "pk344"},   // Mountain (V.1), swap
	249329: {"WC03", "pk346"},   // Mountain (V.3), image
	249332: {"WC03", "pk339"},   // Swamp (V.1), swap
	249333: {"WC03", "pk340"},   // Swamp (V.2), image

	// WCD 2003: Wolfgang Eder
	249286: {"WC03", "we170sb"}, // Smother (V.2), CardTrader
	249295: {"WC03", "we340"},   // Swamp (V.1), image
	249296: {"WC03", "we339"},   // Swamp (V.2), swap

	// WCD 2003: Dave Humpherys
	249247: {"WC03", "dh54sb"},  // Wonder (V.2), CardTrader
	249249: {"WC03", "dh122"},   // Krosan Reclamation (V.1), sideboard
	249250: {"WC03", "dh122sb"}, // Krosan Reclamation (V.2), sideboard
	249251: {"WC03", "dh20"},    // Ray of Revelation (V.1), sideboard
	249252: {"WC03", "dh20sb"},  // Ray of Revelation (V.2), sideboard
	249254: {"WC03", "dh112sb"}, // Unsummon (V.2), CardTrader
	249257: {"WC03", "dh36sb"},  // Deep Analysis (V.2), CardTrader
	249262: {"WC03", "dh348"},   // Forest (V.1), swap
	249263: {"WC03", "dh349"},   // Forest (V.2), image
	249266: {"WC03", "dh336"},   // Island (V.1), swap
	249267: {"WC03", "dh337"},   // Island (V.2), image

	// WCD 2003: Daniel Zink
	249208: {"WC03", "dz33"},   // Circular Logic (V.1), sideboard
	249209: {"WC03", "dz33sb"}, // Circular Logic (V.2), sideboard
	249214: {"WC03", "dz50sb"}, // Renewed Faith (V.2), CardTrader
	249215: {"WC03", "dz21"},   // Vengeful Dreams (V.1), sideboard
	249216: {"WC03", "dz21sb"}, // Vengeful Dreams (V.2), sideboard
	249222: {"WC03", "dz347"},  // Forest (V.1), swap
	249223: {"WC03", "dz348"},  // Forest (V.2), image
	249226: {"WC03", "dz335"},  // Island (V.1), swap
	249228: {"WC03", "dz337"},  // Island (V.3), image
	249231: {"WC03", "dz331"},  // Plains (V.1), swap
	249232: {"WC03", "dz332"},  // Plains (V.2), image

	// WCD 2004: Gabriel Nassif
	249195: {"WC04", "gn331"}, // Plains (V.1), swap
	249196: {"WC04", "gn332"}, // Plains (V.2), image

	// WCD 2004: Julien Nuijten
	249116: {"WC04", "jn272"},   // Plow Under (V.1), sideboard
	249117: {"WC04", "jn272sb"}, // Plow Under (V.2), sideboard
	249126: {"WC04", "jn333"},   // Plains (V.3), CardTrader

	// Chronicles: Japanese
	272488: {"BCHR", "114b"}, // Urza's Mine (V.2), map lists four
	272502: {"BCHR", "115b"}, // Urza's Power Plant (V.2), map lists four
	272552: {"BCHR", "116a"}, // Urza's Tower (V.1), map lists four

	// Mirage
	8069:   {"MIR", "120"},  // Ebony Charm (V.1), CardTrader
	878087: {"MIR", "120†"}, // Ebony Charm (V.2), CardTrader

	// Summer Magic
	16934: {"SUM", "306"}, // Forest (V.1), CardTrader
	16939: {"SUM", "305"}, // Forest (V.2), CardTrader
	16944: {"SUM", "304"}, // Forest (V.3), CardTrader
	16935: {"SUM", "296"}, // Island (V.1), CardTrader
	16940: {"SUM", "295"}, // Island (V.2), CardTrader
	16945: {"SUM", "297"}, // Island (V.3), CardTrader
	16936: {"SUM", "302"}, // Mountain (V.1), CardTrader
	16941: {"SUM", "303"}, // Mountain (V.2), CardTrader
	16946: {"SUM", "301"}, // Mountain (V.3), CardTrader
	16937: {"SUM", "294"}, // Plains (V.1), CardTrader
	16942: {"SUM", "292"}, // Plains (V.2), CardTrader
	16947: {"SUM", "293"}, // Plains (V.3), CardTrader
	16938: {"SUM", "300"}, // Swamp (V.1), CardTrader
	16943: {"SUM", "298"}, // Swamp (V.2), CardTrader
	16948: {"SUM", "299"}, // Swamp (V.3), CardTrader

	// Simplified Chinese Alternate Art Cards: Portal, the V.N order of the
	// English shelf, which each product's CardTrader blueprint art confirms
	257595: {"POR", "212s"}, // Forest (V.1), shelf order
	257596: {"POR", "213s"}, // Forest (V.2), shelf order
	257597: {"POR", "214s"}, // Forest (V.3), shelf order
	257598: {"POR", "215s"}, // Forest (V.4), shelf order
	257602: {"POR", "200s"}, // Island (V.1), shelf order
	257603: {"POR", "201s"}, // Island (V.2), shelf order
	257604: {"POR", "202s"}, // Island (V.3), shelf order
	257605: {"POR", "203s"}, // Island (V.4), shelf order
	257606: {"POR", "208s"}, // Mountain (V.1), shelf order
	257607: {"POR", "209s"}, // Mountain (V.2), shelf order
	257608: {"POR", "210s"}, // Mountain (V.3), shelf order
	257609: {"POR", "211s"}, // Mountain (V.4), shelf order
	257610: {"POR", "196s"}, // Plains (V.1), shelf order
	257611: {"POR", "197s"}, // Plains (V.2), shelf order
	257612: {"POR", "198s"}, // Plains (V.3), shelf order
	257613: {"POR", "199s"}, // Plains (V.4), shelf order

	// Simplified Chinese Alternate Art Cards: Eighth and Tenth Edition. A
	// nonfoil and a foil product share each printing and the table holds
	// one, the product whose guide price is the more valuable column; the
	// other stays skipped.
	257636: {"8ED", "121s"}, // Carrion Wall (V.2), foil price
	257637: {"8ED", "125s"}, // Death Pits of Rath (V.1), nonfoil price
	257640: {"8ED", "129s"}, // Drudge Skeletons (V.5), foil price
	259545: {"10E", "139s"}, // Drudge Skeletons (V.7), CardTrader

	// Promos: media inserts that MTGJSON links to a printing and its later
	// reprint at once, kept on the printing of the product's own era
	382566: {"PMEI", "2019-3"}, // Shock, era
	412234: {"PMEI", "2019-6"}, // Duress, era
	448968: {"PMEI", "2020-1"}, // Voltaic Key, era
	530787: {"PMEI", "2020-4"}, // Dark Ritual, era
	530797: {"PMEI", "2020-7"}, // Crop Rotation, era
	569213: {"PMEI", "2021-1"}, // Counterspell (V.1), era
	573875: {"PMEI", "2021-2"}, // Bone Shredder, era
	690039: {"PMEI", "2022-2"}, // Wild Growth, era
	690040: {"PMEI", "2022-4"}, // Frantic Search, era
	750367: {"PMEI", "2022-1"}, // Disenchant, era
	772038: {"PMEI", "2023-2"}, // Worn Powerstone, era

	// MagicCon Products
	830163: {"PPRO", "2025-1"}, // Cloud, Midgar Mercenary, era

	// 30th Anniversary History Promos
	678517: {"P30H", "4★"}, // Shivan Dragon (V.2), CardTrader
	678520: {"P30H", "5★"}, // Llanowar Elves (V.2), CardTrader

	// The List
	826306: {"PLST", "MUL-12"}, // Tetsuko Umezawa, Fugitive, CardTrader

	// Secret Lair Drop Series: Secretversary 2021
	687581: {"PLST", "CMR-395"}, // Counterspell (V.2), Scryfall

	// Secret Lair Drop Series: Chaos Vault
	839898: {"SLD", "2102★"}, // Ethersworn Canonist, CardTrader
	839900: {"SLD", "2103★"}, // Goblin Engineer, CardTrader
	839902: {"SLD", "2104★"}, // Dance of the Manse, CardTrader
	839904: {"SLD", "2105★"}, // Arcbound Ravager, CardTrader
	839906: {"SLD", "2106★"}, // Foundry Inspector, CardTrader
	839908: {"SLD", "2107★"}, // Reprocess, CardTrader
	839910: {"SLD", "2108★"}, // Aggressive Mining, CardTrader
	839912: {"SLD", "2109★"}, // Sylvan Safekeeper, CardTrader
	839914: {"SLD", "2110★"}, // Crucible of Worlds, CardTrader
	839916: {"SLD", "2111★"}, // Zuran Orb, CardTrader

	// Lorwyn Eclipsed: Promos
	866276: {"PECL", "244p"}, // Sygg's Command, CardTrader
}

// etchedPrintings is versionPrintings for the products that are the etched
// finish of a printing. Cardmarket files an etched foil as a second product of
// the card, and the datastore links it to the plain printing, which its V.1
// already sells. Each row is CardTrader's blueprint for the product, an
// "Etched Foil" at the number shown.
var etchedPrintings = map[int]struct{ set, number string }{
	// Secret Lair Drop Series: October Superdrop 2022
	680700: {"SLD", "1114"}, // Carrion Feeder
	680704: {"SLD", "1116"}, // Plaguecrafter
	680709: {"SLD", "1117"}, // Thoughtseize
	680713: {"SLD", "1115"}, // Doomsday

	// Secret Lair Drop Series
	674316: {"SLD", "1053"}, // Talisman of Dominance
	674320: {"SLD", "1055"}, // Talisman of Impulse
	674319: {"SLD", "1056"}, // Talisman of Unity
	674311: {"SLD", "1057"}, // Talisman of Hierarchy
	674315: {"SLD", "1058"}, // Talisman of Creativity
	674308: {"SLD", "1059"}, // Talisman of Resilience
	674312: {"SLD", "1060"}, // Talisman of Conviction
	674324: {"SLD", "1061"}, // Talisman of Curiosity
	609643: {"SLD", "163"},  // Razaketh, the Foulblooded
	666814: {"SLD", "305"},  // Ravenous Chupacabra
	666813: {"SLD", "306"},  // Managorger Hydra
	666810: {"SLD", "309"},  // Winding Constrictor
	609622: {"SLD", "316"},  // Fleet Swallower
	660446: {"SLD", "328"},  // Snow-Covered Mountain
	674280: {"SLD", "46"},   // Plains
}

// unplacedProducts are Magic products no evidence places on a printing, or
// that name a printing never made: the V.2 to V.4 of an October Superdrop 2022
// card, whose Japanese printings sit beside the English one; the Modern
// Horizons 3 and March of the Machine promo products for cards that have no
// promo; and duplicates of a product on a shelf that never held the card. The
// name route would file them on another printing, so they are skipped.
var unplacedProducts = map[int]bool{
	680717: true, 680718: true, 680719: true, // Tezzeret the Seeker
	680721: true, 680722: true, 680723: true, // Phyrexian Metamorph
	680725: true, 680726: true, 680727: true, // Skullclamp
	680729: true, 680730: true, 680731: true, // Solemn Simulacrum

	// Modern Horizons 3: Promos, every product but Urza's Incubator (773590)
	773571: true, 773572: true, 773573: true, 773574: true, 773575: true,
	773576: true, 773577: true, 773578: true, 773579: true, 773580: true,
	773581: true, 773582: true, 773583: true, 773584: true, 773585: true,
	773586: true, 773587: true, 773588: true, 773589: true, 773591: true,
	773592: true,

	// Cards no promo or list printing exists for
	706398: true, // Orthion, Hero of Lavabrink, March of the Machine
	706399: true, // Orthion, Hero of Lavabrink, March of the Machine
	706400: true, // Surrak and Goreclaw, March of the Machine
	706401: true, // Surrak and Goreclaw, March of the Machine
	530692: true, // Warstorm Surge, Kaldheim
	609000: true, // Nissa, Voice of Zendikar, Commander: Kamigawa
	744396: true, // Growing Rites of Itlimoc, The Lost Caverns of Ixalan
	744397: true, // Growing Rites of Itlimoc, The Lost Caverns of Ixalan
	851490: true, // Guardian Project, Secret Lair Drop Series, duplicate of 851489
	851530: true, // Distant Melody, Secret Lair Drop Series
	893828: true, // Loki, God of Mischief, Marvel Super Heroes: Promos
	893841: true, // Elektra, Daughter of the Hand, Marvel Super Heroes: Promos
	893849: true, // Quicksilver, Brash Blur, Marvel Super Heroes: Promos
	791815: true, // Iymrith, Desert Doom, Mystery Booster 2
	791938: true, // Call to the Feast, Mystery Booster 2
}

// versionPrinting answers the nonfoil and foil printings versionPrintings and
// etchedPrintings name for a Magic product, or two empty ids where they name
// none the datastore carries. An etched product answers its etched printing
// for both, the way a lone-finish product does elsewhere.
func versionPrinting(b *matcher.Backend, product *cm.Product) (string, string) {
	printing, found := versionPrintings[product.IDProduct]
	etched := false
	if !found {
		printing, found = etchedPrintings[product.IDProduct]
		etched = true
	}
	if !found {
		return "", ""
	}
	cards := b.MatchInSetNumber(matcher.SplitVariants(product.Name)[0], printing.set, printing.number)
	if len(cards) != 1 {
		return "", ""
	}
	if !etched {
		cardIDFoil, _ := b.MatchID(cards[0].UUID, true)
		return cards[0].UUID, cardIDFoil
	}
	cardID, err := b.MatchID(cards[0].UUID, false, true)
	if err != nil {
		return "", ""
	}
	co, err := b.GetUUID(cardID)
	if err != nil || !co.Etched {
		return "", ""
	}
	return cardID, cardID
}
