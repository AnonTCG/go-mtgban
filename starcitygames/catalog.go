package starcitygames

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/mtgban/go-mtgban/matcher"
	"github.com/mtgban/go-mtgban/matcher/magic"
	"github.com/mtgban/go-mtgban/mtgban"
)

// scgCatalogURL is the HawkSearch catalog export. It returns the full product
// catalog (both retail price/qty and buylist sell_list_price per variant) as a
// single JSON array, authenticated with an x-api-key header.
const scgCatalogURL = "https://api.starcitygames.com/hawksearch/catalog/download/json"

// catalogAttempts bounds the replays of a broken export, matching the retry
// budget the client gives an ordinary request.
const catalogAttempts = 10

// Product types the catalog reports. Everything SCG sells shares one
// export, so this is what separates cards from boxes from playmats.
const (
	ProductTypeSingles = "Singles"
	ProductTypeSealed  = "Sealed"
)

// CatalogProduct is a single card printing in the catalog export.
type CatalogProduct struct {
	ID              int              `json:"id"`
	SKU             string           `json:"sku"`
	ProductType     string           `json:"product_type"`
	ScryfallID      string           `json:"scryfall_id"`
	TCGPlayerID     string           `json:"tcgplayer_id"`
	URL             string           `json:"url"`
	Name            string           `json:"name"`
	Game            string           `json:"game"`
	Set             string           `json:"set"`
	Finish          string           `json:"finish"`
	FinishGroup     string           `json:"finish_group"`
	Language        string           `json:"language"`
	Rarity          string           `json:"rarity"`
	CollectorNumber string           `json:"collector_number"`
	Variants        []CatalogVariant `json:"variants"`
}

// CatalogVariant is a product in a specific condition, with its own retail
// price/quantity and buylist (sell_list) price.
type CatalogVariant struct {
	ID            int    `json:"id"`
	SKU           string `json:"sku"`
	URL           string `json:"url"`
	Condition     string `json:"condition"`
	Qty           int    `json:"qty"`
	Price         string `json:"price"`
	IsOnDiscount  bool   `json:"is_on_discount"`
	SellListPrice string `json:"sell_list_price"`
}

// DownloadCatalog fetches the catalog export stream. The caller must close the
// returned reader.
func (scg *SCGClient) DownloadCatalog(ctx context.Context) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scg.catalogURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", scg.apiKey)

	resp, err := scg.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, fmt.Errorf("catalog download failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp.Body, nil
}

// productRefused marks an error that came out of the product callback rather
// than out of the stream, which tells the replay apart from the one failure
// re-reading the export cannot mend.
type productRefused struct {
	err error
}

func (p productRefused) Error() string { return p.err.Error() }

func (p productRefused) Unwrap() error { return p.err }

// StreamCatalog hands every product in the catalog export to fn, restarting
// the download if the stream breaks partway through. An error fn returns ends
// the stream and comes back as it is; only a broken one is read again.
//
// The export is one long-lived response of a hundred-odd megabytes, and the
// client's own retry can only replay a request that never returned a status.
// A connection dropped after the header - "stream error: stream ID 1;
// INTERNAL_ERROR" is the one seen in practice - lands past it, and would
// otherwise end the run with an empty catalog. Replaying from the top costs a
// second download and is the only recovery available, since the export is not
// resumable; reset undoes whatever the abandoned pass accumulated.
func (scg *SCGClient) StreamCatalog(ctx context.Context, reset func(), fn func(CatalogProduct) error) error {
	var err error
	for attempt := range catalogAttempts {
		if attempt > 0 {
			reset()
		}
		err = scg.streamCatalogOnce(ctx, fn)
		if err == nil {
			return nil
		}
		// A cancelled context is the caller giving up, not a flaky peer.
		if ctx.Err() != nil {
			return err
		}
		// Neither is a product the caller turned down. Downloading the
		// export again would hand it the very same product to turn down
		// again, at a hundred megabytes a go, so it comes straight back.
		var refused productRefused
		if errors.As(err, &refused) {
			return refused.err
		}
	}
	return err
}

func (scg *SCGClient) streamCatalogOnce(ctx context.Context, fn func(CatalogProduct) error) error {
	body, err := scg.DownloadCatalog(ctx)
	if err != nil {
		return err
	}
	defer body.Close()
	return decodeCatalog(body, fn)
}

// decodeCatalog streams the catalog array, invoking fn for each product without
// buffering the whole (large) response in memory.
func decodeCatalog(r io.Reader, fn func(CatalogProduct) error) error {
	dec := json.NewDecoder(r)

	// Opening '['
	if _, err := dec.Token(); err != nil {
		return err
	}
	for dec.More() {
		var p CatalogProduct
		if err := dec.Decode(&p); err != nil {
			return err
		}
		if err := fn(p); err != nil {
			return productRefused{err}
		}
	}
	// Closing ']'
	_, err := dec.Token()
	return err
}

// gameFromCatalog maps the catalog game string to the internal game constant.
// An unknown string maps to 0, which matches no configured scraper, so those
// products are skipped.
func gameFromCatalog(game string) int {
	switch game {
	case "Magic: The Gathering":
		return GameMagic
	case "Flesh and Blood":
		return GameFleshAndBlood
	// The catalog switches these two games between spellings; mapping
	// only one of them empties every run of that game.
	case "Lorcana", "Disney Lorcana":
		return GameLorcana
	case "Riftbound", "Riftbound: League of Legends TCG":
		return GameRiftbound
	default:
		return 0
	}
}

// catalogFoil reports the foil flag from the broad finish grouping. "Non-foil"
// is plain; "Foil" and "Alt Foil" (etched, surge, rainbow, cold, …) are foil.
func catalogFoil(p CatalogProduct) bool {
	return p.FinishGroup != "Non-foil"
}

// catalogHit synthesizes the minimal Hit that preprocess needs from a catalog
// product, used as the fallback when the Scryfall shortcut doesn't apply.
// The finish rides in as the subtitle, where preprocess reads the wording
// that names a different printing than the sku's number.
func catalogHit(p CatalogProduct, foil bool) Hit {
	finishType := 1
	if foil {
		finishType = 2
	}
	var subtitle string
	if strings.Contains(p.Finish, "Double Rainbow") {
		subtitle = p.Finish
	}
	return Hit{
		Name:                p.Name,
		SetName:             p.Set,
		Language:            p.Language,
		CollectorNumber:     p.CollectorNumber,
		FinishPricingTypeID: finishType,
		Variants:            []Variant{{Sku: p.SKU, Subtitle: subtitle}},
	}
}

// resolveProduct returns the mtgban card id for a catalog product.
//
// The Scryfall id is authoritative: when present it resolves directly through
// the identifier index, skipping preprocess entirely. Etched is the only
// alt-foil that changes the printing (and only for two sets); it shares the
// plain foil's Scryfall id, so it is detected from the finish name and handed to
// MatchID, which associates the foil id with its etched sibling. Every other
// alt-foil (surge/rainbow/cold) resolves to the plain foil. When the id is
// missing or unresolved, it falls back to the SKU-driven preprocess path.
func resolveProduct(b *matcher.Backend, game int, p CatalogProduct) (string, error) {
	id, err := resolveProductID(b, game, p)
	if err != nil {
		return "", err
	}

	// A language other than English is the catalog saying which of a card's
	// language printings this product is, and it has to be the one that was
	// found: the inherently foreign sets hold a single printing each while
	// SCG lists them once per language it believes it has, and without this
	// they all collapse onto it and fight over the same key.
	//
	// English says nothing. It is what the sku grammar emits by default, and
	// the shelves where the shop does mean a language never carry it, so it
	// cannot disqualify a printing made in one tongue only - whose collector
	// number has already said which. This is the matcher's own rule, which
	// filters on the language tag only where the tag is set or the candidates
	// did not come down to one.
	co, cerr := b.GetUUID(id)
	claimed := catalogLanguageTag(p.Language)
	if cerr == nil && claimed != "" && !strings.Contains(co.Language, claimed) {
		return "", matcher.ErrUnsupported
	}

	// A Store Championship promo was struck in foil alone, so the shop's
	// unfoiled listing of one has nothing to land on. Both listings resolve
	// to the single printing there is - the id index and the wording path
	// alike hand back the one finish a printing was made in, whichever was
	// asked for - and the pair then meets on one key, where the record keeps
	// both and the dearer prices the card: a $99.99 City of Brass beside the
	// $249.99 foil. Only this shelf is guarded, because only here is the
	// absence proven; elsewhere an unfoiled product landing on a foil-only
	// printing is a resolution that went wrong earlier, and refusing it would
	// bury the mistake rather than fix it.
	if cerr == nil && !catalogFoil(p) && co.Foil &&
		strings.HasPrefix(skuNumber(p.SKU), "SCHP_") {
		return "", matcher.ErrUnsupported
	}
	return id, nil
}

// catalogLanguageTag turns the language a product is sold in into the tag the
// matcher files a printing under, which is empty for English. The catalog
// spells the two-part languages with a dash mtgjson does not use, so the name
// goes through the matcher's own code table rather than being compared as
// prose - and the spelling there is the shop's, so its case is not held
// against it. One the table does not know is left as it is spelled.
func catalogLanguageTag(catalogLanguage string) string {
	spelled := strings.ReplaceAll(catalogLanguage, " - ", " ")
	for tag, code := range matcher.LanguageTag2LanguageCode {
		if !strings.EqualFold(tag, spelled) {
			continue
		}
		return matcher.LanguageCode2LanguageTag[code]
	}
	return spelled
}

// skuSetCode returns the set segment of a catalog sku, which carries
// detail the product's own set name has lost.
func skuSetCode(sku string) string {
	fields := strings.Split(sku, "-")
	if len(fields) < 3 {
		return ""
	}
	return fields[2]
}

// skuNumber returns the collector number segment of a catalog sku,
// which keeps the variant letter the product's number field drops.
func skuNumber(sku string) string {
	fields := strings.Split(sku, "-")
	if len(fields) < 4 {
		return ""
	}
	return fields[3]
}

// leadingTokenNumber returns the number embedded at the very start of a
// two-sided token sku's own number segment, stripping one leading "T" -
// SCG's own treatment/token-shelf marker, distinct from the target token
// set's own "T" prefix - and leading zeros. "T16_AFR_T14" and "T9T8" both
// give "16" and "9": the first face's own number, whether the sku spells
// the second face's set and number after an underscore or runs directly
// into it.
func leadingTokenNumber(numberField string) string {
	s := strings.TrimPrefix(numberField, "T")
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	return strings.TrimLeft(s[:end], "0")
}

// tokenPairSkuAnchor is one face's own filing set and number, read
// directly off a composite two-sided-token sku rather than guessed from
// a vendor's own wording.
type tokenPairSkuAnchor struct {
	set, number string
}

// tokenPairSkuAnchors splits a two-sided listing's own composite sku
// number segment into its (at most two) face anchors, in listing order.
// A face's own set is whichever code token comes immediately before its
// "T<digits>" number token in the segment, or the sku's own leading set
// code (skuSetCode) when none precedes it - covering every shape
// measured in SCG's real catalog: an explicit code before each number
// ("AFR_T01_GRN_T02"), one shared code before both fused numbers with no
// separator between them ("T03T15"), and a shelf-marker prefix (PWSB,
// SECRET, ...) that never itself names a real set and is simply the
// "nothing preceded this number" case, same as a plain "T03T15".
func tokenPairSkuAnchors(sku string) []tokenPairSkuAnchor {
	number := skuNumber(sku)
	var anchors []tokenPairSkuAnchor
	lastCode := ""
	i := 0
	isDigit := func(c byte) bool { return c >= '0' && c <= '9' }
	for i < len(number) {
		switch {
		case number[i] == '_':
			i++
		case number[i] == 'T' && i+1 < len(number) && isDigit(number[i+1]):
			j := i + 1
			for j < len(number) && isDigit(number[j]) {
				j++
			}
			code := lastCode
			if code == "" {
				code = skuSetCode(sku)
			}
			anchors = append(anchors, tokenPairSkuAnchor{code, strings.TrimLeft(number[i+1:j], "0")})
			i = j
		default:
			j := i
			for j < len(number) && number[j] != '_' && !(number[j] == 'T' && j+1 < len(number) && isDigit(number[j+1])) {
				j++
			}
			if j == i {
				j++ // a lone "T" with no following digit - part of the code run
			}
			lastCode = number[i:j]
			i = j
		}
	}
	return anchors
}

// tokenPairSkuAnchorSet resolves one anchor's own code to the real set it
// names, trying the token-set prefix first (most of these skus anchor a
// token, not the set's own cards), then the bare code, then one trailing
// print-run/sheet digit trimmed off the code (SCG appends one to some of
// its own shelf codes that mtgjson's own code never carries - "T40K2" and
// "TWHO3" name the same real sets as "T40K"/"TWHO").
func tokenPairSkuAnchorSet(b *matcher.Backend, code string) string {
	for _, candidate := range []string{"T" + code, code} {
		if _, err := b.GetSet(candidate); err == nil {
			return candidate
		}
	}
	if len(code) > 1 {
		trimmed := code[:len(code)-1]
		for _, candidate := range []string{"T" + trimmed, trimmed} {
			if _, err := b.GetSet(candidate); err == nil {
				return candidate
			}
		}
	}
	return ""
}

// tokenPairAnchorUUIDs anchors both faces of a two-sided token listing
// independently by identity - each face's own filing set and number, read
// directly off the listing's own composite sku (tokenPairSkuAnchors) -
// rather than guessing either from the vendor's own wording.
// ok is false whenever the sku doesn't carry two such anchors, either
// face's own set can't be resolved, or either face's own name doesn't
// match exactly one printing in that set and number - the same
// "don't know, refuse" the rest of this file already applies.
func tokenPairAnchorUUIDs(b *matcher.Backend, p CatalogProduct) (uuidA, uuidB string, ok bool) {
	if gameFromCatalog(p.Game) != GameMagic || !strings.Contains(p.Name, " // ") {
		return "", "", false
	}
	anchors := tokenPairSkuAnchors(p.SKU)
	if len(anchors) != 2 {
		return "", "", false
	}
	first, second := magic.SplitTokenPairName(p.Name)
	faces := [2]string{first, second}
	var uuids [2]string
	for i, a := range anchors {
		set := tokenPairSkuAnchorSet(b, a.set)
		if set == "" {
			return "", "", false
		}
		var uuid string
		for _, face := range []string{magic.StripFaceWrapping(faces[i]), magic.CleanFaceName(faces[i])} {
			if cards := b.MatchInSetNumber(face, set, a.number); len(cards) == 1 {
				uuid = cards[0].UUID
				break
			}
		}
		if uuid == "" {
			return "", "", false
		}
		uuids[i] = uuid
	}
	return uuids[0], uuids[1], true
}

// lowerVariantLetter puts a variant letter into the case the datastore spells
// it in. Star City Games writes the letter that demands the variant printing
// either way, and the matcher reads the case, so an upper-case one asks for
// the plain card sharing the number and quietly gets it.
//
// The letter is only ever the one following a digit: the all-letter segments
// ("PRM-FAB_233") name a set instead, and lowering those would lose the code.
func lowerVariantLetter(number string) string {
	cut := len(number)
	for cut > 0 && unicode.IsLetter(rune(number[cut-1])) {
		cut--
	}
	if cut == 0 || cut == len(number) || !unicode.IsDigit(rune(number[cut-1])) {
		return number
	}
	return number[:cut] + strings.ToLower(number[cut:])
}

// fabPrintRunMarkers are the print-run suffixes Star City Games glues onto a
// Flesh and Blood set code in its skus, listed longest first so the trim takes
// the whole marker. The datastore numbers a card with the bare code and crosses
// the run with the treatment instead, so "ARC1036" is its ARC036.
var fabPrintRunMarkers = []string{"12", "1", "2", "U"}

// fabNumbers returns the collector numbers to try for a Flesh and Blood sku,
// most specific first.
//
// The sku carries the datastore's own number split across two segments:
// "SGL-FAB-BVO-005-ENN" is BVO005, which names exactly one printing, while the
// bare "005" is every set's fifth card. That matters because the sets Star City
// Games sells these under are decks and blister packs whose names match no
// datastore set, so nothing else narrows the candidates and the reprints alias.
//
// A number segment that opens with a code of its own ("HER_001" under the
// catch-all PRM) already names its set, and the sku's set segment is only the
// shelf it sits on. The fused pairs join with the separator the datastore
// spells them with, and the letter parts ("019_CC") stay wording.
//
// The bare number stays last rather than being dropped: it is what resolves the
// listings whose set segment is neither a datastore code nor a marked-up one,
// and keeping it last costs nothing since a prefixed number that resolves is
// always the more specific answer.
func fabNumbers(sku string) []string {
	number := lowerVariantLetter(skuNumber(sku))
	bare := strings.ReplaceAll(number, "_", " ")
	if number == "" {
		return []string{bare}
	}

	code := skuSetCode(sku)
	parts := strings.Split(number, "_")
	if len(parts) > 1 && isAllLetters(parts[0]) {
		code, parts = parts[0], parts[1:]
	}

	var candidates []string
	for _, prefix := range []string{code, trimPrintRun(code)} {
		candidate := prefixNumber(prefix, parts)
		if spelled, found := fabNumberRespellings[candidate]; found {
			candidate = spelled
		}
		if label, found := fabWordings[candidate]; found {
			candidate += " " + label
		}
		if candidate != "" && candidate != bare && !slices.Contains(candidates, candidate) {
			candidates = append(candidates, candidate)
		}
	}
	return append(candidates, bare)
}

// fabFacesOnly reports whether a double-sided listing is one the datastore
// holds only as its two faces. SCG sells the card as a front with one of
// several backs, a product with two collector numbers; the datastore has each
// face as its own printing and no row for the pair, so there is no single
// printing to price and neither face's price is the listing's.
func fabFacesOnly(b *matcher.Backend, name, front, back string) bool {
	return !fabHasName(b, name) && fabHasName(b, front) && fabHasName(b, back)
}

// fabHasName reports whether the datastore carries a printing of the name.
func fabHasName(b *matcher.Backend, name string) bool {
	uuids, err := b.SearchEquals(name)
	return err == nil && len(uuids) > 0
}

// fabSingleNumbered reports whether a sku's number segment names exactly one
// collector number, the way "NUU-026" and "PRM-FAB_233" do and the pairs
// ("OMN-048_047") do not.
func fabSingleNumbered(sku string) bool {
	parts := strings.Split(skuNumber(sku), "_")
	if len(parts) > 1 && isAllLetters(parts[0]) {
		parts = parts[1:]
	}
	var count int
	for _, part := range parts {
		if numberedPart(part) {
			count++
		}
	}
	return count == 1
}

// numberedPart reports whether a part of a sku's number segment is a
// collector number rather than the wording the letter parts are.
func numberedPart(part string) bool {
	return part != "" && part[0] >= '0' && part[0] <= '9'
}

// prefixNumber glues the set code onto every numeric part of a sku's number
// segment, pairing them the way the datastore does ("MST002//MST158") and
// leaving the non-numeric parts as the wording they are.
func prefixNumber(code string, parts []string) string {
	if code == "" {
		return ""
	}
	var numbers, words []string
	for _, part := range parts {
		if numberedPart(part) {
			numbers = append(numbers, code+part)
			continue
		}
		words = append(words, part)
	}
	if len(numbers) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.Join(numbers, " // ") + " " + strings.Join(words, " "))
}

// trimPrintRun removes a print-run marker from a sku's set code. The trim is
// only ever a second guess, and it never shortens a code past the three
// letters every Flesh and Blood code is at least: "KSU", "NUU" and "UZU" are
// whole codes that end in a marker letter, so the untrimmed code has to be
// tried first and what the trim leaves has to still look like a code.
func trimPrintRun(code string) string {
	for _, marker := range fabPrintRunMarkers {
		trimmed := strings.TrimSuffix(code, marker)
		if trimmed != code && len(trimmed) >= 3 {
			return trimmed
		}
	}
	return code
}

// fabVariantMarker is the digit Star City Games glues onto a Flesh and Blood
// set code to give the second printing of a collector number a sku of its own:
// EVO056 is War Machine and "SGL-FAB-EVO2-056-ENR" is the Extended Art beside
// it. Nothing else in the catalog divides the two - same set, same number,
// same finish, same rarity - so the marker is the whole of what says a listing
// is not the plain printing.
const fabVariantMarker = "2"

// fabVariantMarked reports whether a sku carries the marker. A code is only
// read as marked where taking the marker off still leaves a code, so that a
// genuine code ending in the digit would be left alone.
func fabVariantMarked(sku string) bool {
	code := skuSetCode(sku)
	if !strings.HasSuffix(code, fabVariantMarker) {
		return false
	}
	return trimPrintRun(code) != code
}

// fabPlainSibling steers a fabMatch result off a Marvel printing when the
// product's own rarity does not say Marvel. fabMatch's rarity argument only
// ever asks for the Marvel printing, through fabTiers appending it to the
// number - nothing steers a product priced at any other rarity away from
// whichever id an unmarked query happens to land on, and a survey of the
// datastore found it lands on the Marvel one. "042b" Aether Ashwing // Ash
// is catalogued rarity "Token" at $9.99, beside its own "042" Marvel twin at
// $69.99, and without this both share the Marvel uuid at every grade.
//
// Only a clean pair is resolved: a (set, number, foilness) carrying more
// than one non-Marvel candidate is left exactly as fabMatch found it, since
// nothing here says which of those the product is either. A survey of the
// whole Flesh and Blood datastore found 36 (set, number, foilness) keys
// holding both a Marvel and a non-Marvel printing, 32 of them this clean.
func fabPlainSibling(b *matcher.Backend, id, rarity string) string {
	if rarity == "Marvel" {
		return id
	}
	co, err := b.GetUUID(id)
	if err != nil || !co.HasPromoType("marvel") {
		return id
	}
	var plain string
	for _, twin := range b.MatchWithNumber("", co.SetCode, co.Number) {
		tco, terr := b.GetUUID(twin.UUID)
		if terr != nil || tco.HasPromoType("marvel") || tco.Foil != co.Foil {
			continue
		}
		if plain != "" {
			return id
		}
		plain = twin.UUID
	}
	if plain == "" {
		return id
	}
	return plain
}

// fabRenamedTwins names the promo type a sku's own printing carries, for the
// one shelf where the catalog sends the same name for two different cards.
// SUP2-009b-ENC is $549.99 beside SUP2-009-ENC's $109.99, both sold as
// "Pleiades, Superstar" - but the dearer one is the Maori printing, which
// the datastore carries under its own card name, "Hinewhitu, Hautipua",
// with "Pleiades, Superstar" folded into a promo type instead of the name.
// Nothing about the product's own fields says to look for that: the sku is
// what marks it. A survey of the whole datastore for a second row at one
// (set, number, foilness) carrying a different name found exactly this one
// pair, so the table is closed rather than a rule guessing at a convention.
var fabRenamedTwins = map[string]string{
	"SGL-FAB-SUP2-009b-ENC": "pleiadessuperstar",
}

// fabRenamedTwin steers a fabMatch result onto the printing fabRenamedTwins
// names, given the one the match landed on.
func fabRenamedTwin(b *matcher.Backend, id, sku string) string {
	want, renamed := fabRenamedTwins[sku]
	if !renamed {
		return id
	}
	co, err := b.GetUUID(id)
	if err != nil {
		return id
	}
	for _, twin := range b.MatchWithNumber("", co.SetCode, co.Number) {
		tco, terr := b.GetUUID(twin.UUID)
		if terr != nil || !tco.HasPromoType(want) || tco.Foil != co.Foil {
			continue
		}
		return twin.UUID
	}
	return id
}

// fabCreditedTwins names the artist a sku's own printing carries, for the
// shelf where two products share a number and, once "(Marvel)" is folded
// into the matched variant, a name: SGL-FAB-ROS2-008a-ENC is sold as "Aurora
// (Asur Misoa)" and SGL-FAB-ROS2-008b-ENC as "Aurora (Ramza Ardyputra)", and
// nothing about either product's own fields says which uuid is which. The
// artist credit does, which datastore-gen publishes from the-fab-cube and
// TCGplayer's own catalog does not carry. The table is closed rather than a
// rule guessing at a convention, the way fabRenamedTwins is: a survey for any
// other (set, number, foilness) with two candidate rows and no other way to
// tell them apart is worth running again if datastore-gen publishes more
// artists.
var fabCreditedTwins = map[string]string{
	"SGL-FAB-ROS2-008a-ENC": "Asur Misoa",
	"SGL-FAB-ROS2-008b-ENC": "Ramza Ardyputra",
}

// fabCreditedTwin steers a fabMatch result onto the printing fabCreditedTwins
// names, given the one the match landed on.
func fabCreditedTwin(b *matcher.Backend, id, sku string) string {
	want, credited := fabCreditedTwins[sku]
	if !credited {
		return id
	}
	co, err := b.GetUUID(id)
	if err != nil {
		return id
	}
	for _, twin := range b.MatchWithNumber("", co.SetCode, co.Number) {
		tco, terr := b.GetUUID(twin.UUID)
		if terr != nil || tco.Artist != want || tco.Foil != co.Foil {
			continue
		}
		return twin.UUID
	}
	return id
}

// fabUnmarkedTreatment reports whether a vendor id lands an unmarked sku on
// a treatment printing. SCG files one id on a card's plain sku and on its
// marked one ("HNT-155" and "HNT2-155"), so only the marker says which
// printing is sold, and an unmarked sku is left to its wording.
func fabUnmarkedTreatment(b *matcher.Backend, p CatalogProduct, uuid string) bool {
	if fabVariantMarked(p.SKU) {
		return false
	}
	co, err := b.GetUUID(uuid)
	return err == nil && len(fabTreatments(co.PromoTypes)) > 0
}

// fabSkuPrinting picks the printing a Flesh and Blood sku names among the
// siblings of the one a match landed on: the plain card the rarity names, a
// renamed or credited twin, or the sibling a marker digit points at.
func fabSkuPrinting(b *matcher.Backend, id string, p CatalogProduct) string {
	id = fabPlainSibling(b, id, p.Rarity)
	id = fabRenamedTwin(b, id, p.SKU)
	id = fabCreditedTwin(b, id, p.SKU)
	return fabMarkedSibling(b, id, p)
}

// fabMarkedSibling returns the printing a marked sku names, given the plain
// one the match landed on. The marker says that a second printing of this
// number exists and not which it is, so the datastore is what names it: where
// exactly one sibling carries a treatment the plain printing does not, that is
// what is being sold. Where several do - a number with a Marvel and an
// Extended Art both - the marker cannot choose between them, and the treatment
// the catalog does spell (through the rarity, see fabTiers) has already had
// its say, so the answer already reached stands.
func fabMarkedSibling(b *matcher.Backend, id string, p CatalogProduct) string {
	if !fabVariantMarked(p.SKU) {
		return id
	}
	co, err := b.GetUUID(id)
	if err != nil || len(fabTreatments(co.PromoTypes)) > 0 {
		return id
	}

	var found string
	for _, card := range b.MatchWithNumber(co.Name, co.SetCode, co.Number) {
		if len(fabTreatments(card.PromoTypes)) == 0 {
			continue
		}
		// A treatment the catalog has a field for is that field's to name.
		// The marker is on the sku of a marvel too, and letting it choose
		// one would price a listing as a marvel on the strength of a digit
		// where the rarity beside it says the ordinary card.
		if fabTiers[b.RarityLabel(card.Rarity)] {
			continue
		}
		sibling, serr := b.MatchIDFinish(card.UUID, p.Finish)
		if serr != nil || sibling == id {
			continue
		}
		if found != "" && found != sibling {
			return id
		}
		found = sibling
	}
	if found == "" {
		return id
	}
	return found
}

// fabTreatments answers the labels on a printing that name a treatment. The
// datastore files a pitch color as a label on the printings of a card sold
// in one pitch, and a pitch is a piece of the name: the plain Drop of
// Dragon Blood is labelled red beside its extended art, and the marker on
// the second sku has to find the extended art from it.
func fabTreatments(promoTypes []string) []string {
	var labels []string
	for _, promoType := range promoTypes {
		switch promoType {
		case "red", "yellow", "blue":
			continue
		}
		labels = append(labels, promoType)
	}
	return labels
}

func isAllLetters(field string) bool {
	if field == "" {
		return false
	}
	for _, r := range field {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

// catalogNames are the names Star City Games spells differently from the
// datastore, one entry per misspelling. The matcher does not forgive a stray
// s on any name, which would read Nest Ball as Net Ball and Swoobat as
// Woobat, so a storefront that misspells a name says so here.
var catalogNames = map[string]string{
	"Bandana of the Blue Beyond": "Bandana of the Blue Beyonds",

	// SCG's own dungeon-card listing spells "Lost Mine of Phandelver"
	// with an extra "the" nowhere in the card's real name; without this,
	// the pairing it names (Dungeon of the Mad Mage // Lost Mine of
	// Phandelver) is unreachable regardless of how well cleanFaceName
	// strips the rest of SCG's own brace-and-suffix wrapping.
	"{Dungeon of the Mad Mage Dungeon} // {Lost Mine of the Phandelver Dungeon}": "{Dungeon of the Mad Mage Dungeon} // {Lost Mine of Phandelver Dungeon}",

	// Star Trek #83: SCG's own typo.
	"Suurok-Class Science Vessek": "Suurok-Class Science Vessel",
}

// prereleaseMissing reports whether a PRE_<set>_<number> sku names a card
// with no prerelease copy: the promo set holds no printing of it, and the
// set's own row at that number is not the prerelease one.
func prereleaseMissing(b *matcher.Backend, cardName, number string) bool {
	fields := strings.Split(number, "_")
	if len(fields) != 3 || fields[0] != "PRE" {
		return false
	}
	if promo, found := b.Sets["P"+fields[1]]; found {
		for _, card := range promo.Cards {
			if matcher.Contains(card.Name, cardName) {
				return false
			}
		}
	}
	cards := b.MatchWithNumber(cardName, fields[1], strings.TrimLeft(fields[2], "0"))
	if len(cards) == 0 {
		return false
	}
	for _, card := range cards {
		if card.HasPromoType(magic.PromoTypePrerelease) {
			return false
		}
	}
	return true
}

// catalogResolvers names the printing a catalog product of each game is,
// once resolveProductID has read what every game shares.
var catalogResolvers = map[int]func(b *matcher.Backend, p CatalogProduct, foil bool) (string, error){
	GameMagic:         resolveMagic,
	GameFleshAndBlood: resolveFleshAndBlood,
	GameLorcana:       resolveLorcana,
	GameRiftbound:     resolveRiftbound,
}

func resolveProductID(b *matcher.Backend, game int, p CatalogProduct) (string, error) {
	// Duel Masters crossover promos are catalogued under Magic but aren't Magic
	// cards, so there's nothing to match; discard them.
	if strings.Contains(p.Name, "(Duel Masters)") {
		return "", matcher.ErrUnsupported
	}

	if spelled, found := catalogNames[p.Name]; found {
		p.Name = spelled
	}

	resolve, found := catalogResolvers[game]
	if !found {
		return "", matcher.ErrUnsupported
	}
	return resolve(b, p, catalogFoil(p))
}

// vendorPrinting resolves the first of the product's vendor ids that names a
// printing, Scryfall first, in the finish the product names (matchVendorID).
// What the printing has to carry besides is the game's to check.
func vendorPrinting(b *matcher.Backend, game int, p CatalogProduct, foil, etched bool) (string, bool) {
	for _, id := range []string{p.ScryfallID, p.TCGPlayerID} {
		if id == "" {
			continue
		}
		out, err := matchVendorID(b, game, id, p, foil, etched)
		if err == nil {
			return out, true
		}
	}
	return "", false
}

// resolveMagic steers by the sku the shelves whose identifiers name another
// printing, then resolves the vendor ids, and reads the wording where neither
// places the product. An id the product's own record contradicts goes to the
// wording.
func resolveMagic(b *matcher.Backend, p CatalogProduct, foil bool) (string, error) {
	etched := strings.Contains(strings.ToLower(p.Finish), "etched")

	// Star City Games lists prerelease skus for cards that were never given
	// a prerelease printing, and there is nothing for those to price.
	if prereleaseMissing(b, p.Name, skuNumber(p.SKU)) {
		return "", matcher.ErrUnsupported
	}

	// Portal printed two versions of six cards. SCG marks them a and b in
	// the sku while mtgjson numbers the second with a d suffix, and sends
	// the same collector number and the same scryfall id for both - an id
	// that names the first version - so the b product has to be steered by
	// its number before the identifiers get a say.
	if p.Set == "Portal" && strings.HasSuffix(skuNumber(p.SKU), "b") {
		number := strings.TrimSuffix(skuNumber(p.SKU), "b") + "d"
		if out := b.MatchWithNumber(p.Name, "POR", number); len(out) == 1 {
			return b.MatchID(out[0].UUID, foil, etched)
		}
	}

	// Star City Games letters a prerelease number that names two printings:
	// the datestamped promo, and the set's own card the stamp was put on.
	// Both products carry the promo's identifiers, so the second shelf has to
	// be steered by its sku before the identifiers get a say - the letter is
	// the whole of what separates them. The set code rides in the sku's own
	// number segment, the product's set saying only "Promo".
	if strings.HasPrefix(skuNumber(p.SKU), "PRE_") && strings.HasSuffix(skuNumber(p.SKU), "b") {
		fields := strings.Split(strings.TrimSuffix(skuNumber(p.SKU), "b"), "_")
		if len(fields) == 3 {
			out := b.MatchWithNumber(p.Name, fields[1], strings.TrimLeft(fields[2], "0"))
			if len(out) == 1 && !out[0].HasPromoType("datestamped") {
				return b.MatchID(out[0].UUID, foil, etched)
			}
		}
	}

	// Duel Decks: Anthology reprints four earlier duel decks, and mtgjson
	// keeps them under their original codes. The product's set name says
	// only "Anthology", so the deck it belongs to is read from the sku.
	if p.Set == "Duel Decks: Anthology" {
		number := strings.TrimLeft(p.CollectorNumber, "0")
		if out := b.MatchWithNumber(p.Name, skuSetCode(p.SKU), number); len(out) == 1 {
			return b.MatchID(out[0].UUID, foil, etched)
		}
	}

	// A two-sided token sheet prints one physical card for a pairing
	// matcher/magic may already carry a combined entity for
	// (tokenpairs.go) - resolve that precisely, by id, before the plain
	// identifier lookup further down, which resolves the id straight
	// through with no regard for what the other half of the listing's
	// own name says, silently pricing the whole two-sided product as if
	// it were just the one face the id happens to anchor. (The
	// promo-shelf table below is unaffected either way: its own
	// MatchWithNumber("", set, number) call already finds mtgjson's own
	// combined "X // Y" printing whole where one exists, the same way
	// MatchNativeTokenPair below does explicitly.) See
	// magic.MatchTokenPairing, shared with cardkingdom's own version of
	// this same problem.
	//
	// "Dungeon" alongside "Token": SCG spells its own dungeon-card
	// listings the same way it spells tokens, wrapped and suffixed with
	// its own type name (AFR's "{Dungeon of the Mad Mage Dungeon} //
	// {Lost Mine of the Phandelver Dungeon}"), and neither face of a
	// dungeon // dungeon pairing ever contains the literal word "Token".
	if strings.Contains(p.Name, " // ") &&
		(strings.Contains(p.Name, "Token") || strings.Contains(p.Name, "Dungeon")) {
		if tcgID := magic.MatchTokenPairing(b, p.ScryfallID, p.Name, foil); tcgID != "" {
			if id, err := b.MatchID(tcgID, foil, etched); err == nil {
				return id, nil
			}
		}

		// A two-sided token listing SCG never publishes a scryfallId for
		// at all, but whose own sku already names the filing set directly
		// (as opposed to the Mystery Booster/List shape further below,
		// which bundles two unrelated sets' own tokens into one sku and
		// has no single filing set to anchor from) - try the set that sku
		// segment names, by both mechanisms a two-sided listing can
		// resolve through: a real printing mtgjson already files under
		// one combined "X // Y" name of its own (magic.MatchNativeTokenPair,
		// e.g. Guild Kit's "Copy // Horror"), or a synthetic
		// tokenProducts-derived pairing anchored by the first face alone
		// (magic.MatchTokenPairingBySetNumber, the same mechanism as
		// cardkingdom's own sku-anchored fallback).
		if p.ScryfallID == "" {
			tokenSet := "T" + skuSetCode(p.SKU)
			if _, err := b.GetSet(tokenSet); err != nil {
				tokenSet = skuSetCode(p.SKU)
			}
			if _, err := b.GetSet(tokenSet); err == nil {
				number := leadingTokenNumber(skuNumber(p.SKU))
				if uuid := magic.MatchNativeTokenPair(b, tokenSet, number, p.Name); uuid != "" {
					if id, err := b.MatchID(uuid, foil, etched); err == nil {
						return id, nil
					}
				}
				if tcgID := magic.MatchTokenPairingBySetNumber(b, tokenSet, number, p.Name, foil); tcgID != "" {
					if id, err := b.MatchID(tcgID, foil, etched); err == nil {
						return id, nil
					}
				}
			}

			// The sku's own number segment sometimes names BOTH faces'
			// own filing set and number directly, composited together
			// rather than bundled under one shared set the way the
			// single-anchor attempt above expects ("AFR_T01_GRN_T02",
			// "T03T15" fused under one shared code, ...). Anchoring both
			// faces independently by identity, rather than the first
			// face by number and the second by name, sidesteps
			// Backend.TokenPairIndex's own name-collision-blanking
			// entirely - two already-known uuids can't collide with each
			// other the way two vendor-spelled names can - via the
			// uuid-pair-keyed magic.MatchTokenPairingByUUIDs.
			if uuidA, uuidB, ok := tokenPairAnchorUUIDs(b, p); ok {
				if tcgID := magic.MatchTokenPairingByUUIDs(b, uuidA, uuidB, foil); tcgID != "" {
					if id, err := b.MatchID(tcgID, foil, etched); err == nil {
						return id, nil
					}
				}
			}
		}
	}

	// A promo-shelf printing is steered by its sku for the same reason the
	// Portal pair is: the identifiers name another printing entirely.
	if printing, found := promoShelfPrintings[skuNumber(p.SKU)]; found {
		out := b.MatchWithNumber("", printing.set, printing.number)
		if len(out) == 1 {
			return b.MatchID(out[0].UUID, foil, etched)
		}
	}

	out, found := vendorPrinting(b, GameMagic, p, foil, etched)
	if found && !idContradictsProduct(b, p, out) {
		return resalePrinting(b, out, p, foil, etched), nil
	}

	// SCG's "-WAR2-" is the War of the Spark Japanese planeswalker
	// (jpwalker), whose Japanese-language Scryfall id isn't in the index and
	// which preprocess rejects as non-english. It maps to WAR #NNN★.
	if strings.Contains(p.SKU, "-WAR2-") {
		num := strings.TrimLeft(p.CollectorNumber, "0") + "★"
		if out := b.MatchWithNumber(p.Name, "WAR", num); len(out) == 1 {
			if id, err := b.MatchID(out[0].UUID, foil, false); err == nil {
				return id, nil
			}
		}
	}

	// A "PP_<CODE>_<NUM>" sku names the promo-pack printing of set
	// P<CODE>, which the datastore numbers <NUM>p. The name is passed as
	// a free self-check: the printing the grammar names carries the
	// product's own name in every case the catalog holds.
	if rest, found := strings.CutPrefix(skuNumber(p.SKU), "PP_"); found {
		code, number, split := strings.Cut(rest, "_")
		if split {
			number = strings.TrimLeft(number, "0")
			out := b.MatchWithNumber(p.Name, "P"+code, number+"p")
			if len(out) == 1 {
				if id, err := b.MatchID(out[0].UUID, foil, false); err == nil {
					return id, nil
				}
			}
		}
	}

	card, err := preprocess(b, catalogHit(p, foil))
	if err != nil {
		return "", err
	}
	// Inherently foreign sets (Foreign Black Border, Rinascimento, ...)
	// store the foreign printing as their canonical card, so a resolved id
	// whose primary language isn't English is the right match. Match's
	// English-only language validation would reject it, so use it directly.
	// English-primary cards fall through so a foreign single isn't wrongly
	// collapsed onto the English printing.
	if card.ID != "" {
		if co, e := b.GetUUID(card.ID); e == nil && co.Language != "" && co.Language != "English" {
			return b.MatchID(card.ID, foil, etched)
		}
	}
	id, err := b.Match(card)
	if err != nil || !etched {
		return id, err
	}
	// Only the catalog's finish name says a product is the etched
	// printing - the sku spells it as a plain foil - and the wording
	// path has no other way to hear it.
	return b.MatchID(id, foil, etched)
}

// resolveFleshAndBlood refuses the closed tables, then resolves the vendor
// ids where the printing carries the sku's number, and reads the sku
// otherwise.
func resolveFleshAndBlood(b *matcher.Backend, p CatalogProduct, foil bool) (string, error) {
	// The closed Flesh and Blood refusals hold whatever ids the product sends
	if fabInserts[p.Name] || fabDuplicateStock[p.SKU] || fabUnmade[p.SKU] {
		return "", matcher.ErrUnsupported
	}
	out, found := vendorPrinting(b, GameFleshAndBlood, p, foil, false)
	if found && carriesSkuNumber(b, GameFleshAndBlood, p, out) && !fabUnmarkedTreatment(b, p, out) && !idContradictsProduct(b, p, out) {
		return resalePrinting(b, fabSkuPrinting(b, out, p), p, foil, false), nil
	}

	// Flesh and Blood reads its number off the sku instead: the segments
	// keep the set code, the fused-card pair ("077_112"), the promo-pack
	// prefix ("JDG_001") and the variant letter ("155b") that the product's
	// bare number field drops. The candidates run most specific first, so
	// the bare number only decides what the set-prefixed one could not.
	// The catalog's own finish name rides beside the flag: a product is one
	// printing in one treatment, and only the name says which.
	name := p.Name
	if spelled, found := fabNames[name]; found {
		name = spelled
	}
	edition, finish := fabPrintRun(p.Set, p.Finish)
	numbers := fabNumbers(p.SKU)
	if position := fabArtPosition(name, p.SKU); position != "" {
		numbers = []string{position}
	}
	id, err := fabMatch(b, name, edition, finish, p.Rarity, foil, numbers)
	if err == nil {
		return fabSkuPrinting(b, id, p), nil
	}
	// A product named by both its faces at a single collector number
	// is one printing plus the token printed on its back, not a
	// two-numbered double-sided card: the catalog writes "Pass Over //
	// Inner Chi" for NUU026, which the datastore files under the front
	// face alone. The number segment is what tells the two apart, and
	// it has to hold exactly one number - the genuine double-sided
	// cards carry a pair ("048_047") and a front-face retry would
	// flatten them onto the ordinary single.
	front, back, twoFaced := strings.Cut(name, " // ")
	if twoFaced && fabSingleNumbered(p.SKU) {
		retry, rerr := fabMatch(b, front, edition, finish, p.Rarity, foil, numbers)
		if rerr == nil {
			return fabMarkedSibling(b, retry, p), nil
		}
	}
	if twoFaced && !fabSingleNumbered(p.SKU) && fabFacesOnly(b, name, front, back) {
		return "", matcher.ErrUnsupported
	}
	return "", err
}

// riftboundEventHeadings names the promo shelf behind the event prefix of a
// promo sku, "NN_SFD_R01b" or "RLS_OGN_202": the catalog's own set is the
// generic "Promotional Cards", which spans every promotional set.
var riftboundEventHeadings = map[string]string{
	"NN":  "Nexus Night Promos",
	"RLS": "Release Event Promos",
}

// resolveRiftbound reads a promo sku's own event, set and number, which the
// product's generic set drops. A lettered number that the named set carries
// exactly is that set's printing: the Spiritforged runes "R01b" are filed in
// Spiritforged itself, not beside the Vendetta runes of the same wording.
// Otherwise the event prefix picks the promo shelf. The vendor ids answer
// first, where the printing they name carries the sku's own set and number.
func resolveRiftbound(b *matcher.Backend, p CatalogProduct, foil bool) (string, error) {
	out, found := vendorPrinting(b, GameRiftbound, p, foil, false)
	if found && carriesSkuNumber(b, GameRiftbound, p, out) && !idContradictsProduct(b, p, out) {
		return resalePrinting(b, out, p, foil, false), nil
	}

	edition := p.Set
	segments := strings.Split(skuNumber(p.SKU), "_")
	if skuSetCode(p.SKU) == "PRM" && len(segments) == 3 {
		id, found := riftboundLetteredPrinting(b, p.Name, segments[1], segments[2], foil)
		if found {
			return id, nil
		}
		heading, found := riftboundEventHeadings[segments[0]]
		if found {
			edition = heading
		}
	}
	return b.Match(&matcher.InputCard{
		Name:      p.Name,
		Edition:   edition,
		Variation: p.CollectorNumber,
		Foil:      foil,
	})
}

// riftboundLetteredPrinting finds the printing a lettered number names in
// the set it is filed under. The letter is what makes the number exact: a
// bare one would also match the set's plain card of that number.
func riftboundLetteredPrinting(b *matcher.Backend, name, setCode, number string, foil bool) (string, bool) {
	set, found := b.Sets[setCode]
	if !found || number == "" || !unicode.IsLetter(rune(number[len(number)-1])) {
		return "", false
	}
	id, err := b.Match(&matcher.InputCard{
		Name:      name,
		Edition:   set.Name,
		Variation: number,
		Foil:      foil,
	})
	if err != nil {
		return "", false
	}
	co, err := b.GetUUID(id)
	if err != nil || co.SetCode != setCode || co.Number == "" {
		return "", false
	}
	return id, unicode.IsLetter(rune(co.Number[len(co.Number)-1]))
}

// promoShelfPrintings names the printing behind a sku that SCG files on its
// generic Promo shelf, for the products that shelf leaves unplaceable.
//
// The shelf loses the printing twice over. A League Tokens promo is numbered
// for the set whose league handed it out, which is not where mtgjson keeps it
// - the tokens are their own yearly sets, L12 through L17 - and twelve of the
// thirteen carry no identifier at all, so nothing but the sku says which. The
// thirteenth is worse: {Monk Token} LEAG_FRF_L01, listed at $34.99 and bought
// at $15.00, carries Fate Reforged's set-token scryfall id and so would be
// priced as that $0.49 common, the higher bid winning the shared id.
//
// The League half is closed rather than derived: the leagues ran from 2012 to
// 2017, and the catalog's thirteen products are exactly the datastore's
// thirteen reachable rows. Two are the same creature in one year, L13's Boros
// Soldier for Gatecrash and its mono-white one for Theros, which no rule
// reading the product could tell apart.
var promoShelfPrintings = map[string]struct{ set, number string }{
	"LEAG_M13_L01":    {"L12", "1"},
	"LEAG_RTR_L01":    {"L12", "2"},
	"LEAG_GTC_L01":    {"L13", "1"},
	"LEAG_DGM_L01":    {"L13", "2"},
	"LEAG_M14_L01":    {"L13", "3"},
	"LEAG_THS_L01":    {"L13", "4"},
	"LEAG_BNG_T01":    {"L14", "1"},
	"LEAG_JOU_L01":    {"L14", "2"},
	"LEAG_M15_L01":    {"L14", "3"},
	"LEAG_KTK_L01":    {"L14", "4"},
	"LEAG_FRF_L01":    {"L15", "1"},
	"LEAG_KLD_T05T09": {"L16", "5"},
	"LEAG_AER_T01T00": {"L17", "1"},

	// Cowboy Bebop's five Standard Showdown promos are SCG's SSD_2024_NNNb,
	// beside the basic lands that take the same numbers unsuffixed. Four
	// carry their own identifiers and place themselves; Disdainful Stroke
	// carries Friday Night Magic 2015's, nine years earlier.
	"SSD_2024_002b": {"PCBB", "2"},

	// Undercity has no standalone printing of its own to name at all -
	// mtgjson only ever files it natively combined, "Undercity // The
	// Initiative" (CLB's fixed two-sided token, TCLB #20; the paired
	// listing this bare one is half of, "{The Initiative} //
	// {Undercity Dungeon}", already resolves there via
	// magic.MatchNativeTokenPair). This anchors the bare listing at the
	// identical uuid rather than leaving it unresolved.
	"CLB_T20": {"TCLB", "20"},

	// Nine more shelves carry another printing's identifier, or none at all
	// and spell their printing only in the sku. No guard general enough to
	// catch the id collisions without also catching a correct id elsewhere
	// was found, so this is closed rather than derived.
	"LYLGS_2022_001": {"PLG22", "1"},
	"FEST_2026_002":  {"PF26", "5"},
	"LNY_2024_005":   {"PL24", "5"},
	"MFCMD_2025_001": {"PF25", "10"},
	"PP_SOI_242":     {"PSOI", "242p"},
	"PLYR_ROE2_004":  {"P10", "4" + magic.SuffixSpecial},
	"RC_LTR_301":     {"LTR", "301"},
	"GBUN_MSH_432":   {"MSH", "432"},
	"DD3_048":        {"PLST", "GVL-48"},

	// Game Edition Cloud carries the Pro Tour product's identifiers, and
	// the MagicCon Tifa has none and reads as the Final Fantasy prerelease.
	"GAME_2025_001": {"PMEI", "2025-21"},
	"PTQ_2025_002":  {"PF25", "9"},

	// Japanese promos with no identifier at all, each the only printing of
	// its name in that language: Final Fantasy's buy-a-box pair and the
	// Pro Tour Eternal Wanderer.
	"BAB_FIN_001a":  {"RFIN", "J2"},
	"BAB_FIN_001b":  {"RFIN", "J1"},
	"FOUR_2025_001": {"PW26", "4"},
}

// matchVendorID resolves a vendor id to the printing sold in the finish the
// product names, print run included, where the game's catalog spells one,
// and through the foil and etched flags otherwise.
func matchVendorID(b *matcher.Backend, game int, id string, p CatalogProduct, foil, etched bool) (string, error) {
	finish := catalogFinish(game, p)
	if matcher.FinishSlug(finish) == "" {
		return b.MatchID(id, foil, etched)
	}
	return b.MatchIDFinish(id, finish)
}

// catalogFinish names the finish a product is sold in the way the datastore
// names it, or "" where the game's catalog does not spell one beyond the
// flags.
func catalogFinish(game int, p CatalogProduct) string {
	switch game {
	case GameFleshAndBlood:
		_, finish := fabPrintRun(p.Set, p.Finish)
		return finish
	case GameLorcana:
		return lorcanaFinish(p.Finish)
	}
	return ""
}

// carriesSkuNumber reports whether the printing a vendor id lands on carries
// the number the sku names. Outside Magic SCG files the id on a sibling of
// the card at times (the other pitch of a pair, one face of a double-faced
// pair, the base of a misprint, the card a signature signs), and the sku's
// number is what tells them apart. A Flesh and Blood number carries its
// set's letters, read the way the wording path reads them (fabNumbers).
// Elsewhere the sku's raw number segment is compared, so a prefixed promo
// sku ("P04_013") never agrees and is left to the wording, which lands it.
func carriesSkuNumber(b *matcher.Backend, game int, p CatalogProduct, uuid string) bool {
	co, err := b.GetUUID(uuid)
	if err != nil {
		return false
	}
	if game == GameFleshAndBlood {
		for _, number := range fabNumbers(p.SKU) {
			if strings.EqualFold(number, co.Number) {
				return true
			}
		}
		return false
	}
	// A Riftbound sku names the set too, which tells same-numbered cards apart
	set := skuSetCode(p.SKU)
	if game == GameRiftbound && set != "PRM" && !strings.EqualFold(co.SetCode, set) {
		return false
	}
	return strings.EqualFold(strings.TrimLeft(co.Number, "0"), strings.TrimLeft(skuNumber(p.SKU), "0"))
}

// idContradictsProduct reports whether the resolved printing lacks what the
// product's own record says of itself; a product saying nothing cannot
// contradict.
func idContradictsProduct(b *matcher.Backend, p CatalogProduct, uuid string) bool {
	co, err := b.GetUUID(uuid)
	if err != nil {
		return false
	}
	if strings.Contains(p.Finish, "Double Rainbow") &&
		!co.HasPromoType("doublerainbow") && !co.HasPromoType("rainbowfoil") &&
		!co.HasPromoType("serialized") {
		return true
	}
	if strings.Contains(p.SKU, "-AMP_") && !co.HasPromoType("embossed") {
		return true
	}
	// And its dual. The shelf and the finish are the only two ways this
	// catalog names an embossed card, so an id landing on one from a product
	// that says neither is the generator keying on the collector number and
	// reaching the Ampersand twin.
	if co.HasPromoType("embossed") &&
		!strings.Contains(p.SKU, "-AMP_") && !strings.Contains(p.Finish, "Embossed") {
		return true
	}
	// Modern Horizons' Timeshifts are a set of their own, mtgjson's H1R, sold
	// on the MH12 shelf and priced apart from the originals - but the catalog
	// hands twenty-six of the forty the original's identifiers. No Modern
	// Horizons printing is booster fun and every Timeshifts one is, so an id
	// landing on a plain printing is naming the very card the shelf exists to
	// tell apart. The wording path already renames MH12 to H1R.
	if skuSetCode(p.SKU) == "MH12" && !co.HasPromoType("boosterfun") {
		return true
	}
	// A World Championship sku names the year's deck set, and the identifiers
	// routinely name the ordinary printing the deck reprinted: a gold-bordered
	// $1.25 Covetous Dragon would be priced as Urza's Destiny's $4.99 rare,
	// leaving the deck printing unlisted. The year segment says which set,
	// so a printing from any other - the original, or another year's deck -
	// is refused and the sku read instead.
	if set, named := worldsSetFromSKU(p.SKU); named && co.SetCode != set {
		return true
	}
	// Standard Showdown is an event, not a set: mtgjson files each year's
	// promos under whichever set ran them, while the shop numbers them by the
	// year and the place in that year's cycle. The 2024 lands carry the 2019
	// promo pack's identifiers and would be priced as them, so an id
	// landing anywhere but on a Standard Showdown printing is naming
	// another event's card. The wording path reads the year and finds the
	// right one.
	if strings.HasPrefix(skuNumber(p.SKU), "SSD_") && !co.HasPromoType(magic.PromoTypeStandardShowdown) {
		return true
	}
	return false
}

// resaleMarker opens the number segment of the sku Star City Games gives a
// WotC resale promo.
const resaleMarker = "RESL_"

// resalePrinting returns the resale printing a RESL sku names, given the
// printing its identifiers landed on. SCG copies the promo-pack product's
// identifiers onto the resale product beside it, so the ids name the right
// card in the right promo set and the wrong treatment - both products then
// answer to the promo pack's uuid and the dearer wins.
//
// The datastore's own promoType is what separates the two, and the set the id
// lands in is what bounds the search: where that set holds exactly one resale
// printing of the name, that is what is being sold. Where it holds none the id
// stands, because the shelf also carries printings mtgjson labels no resale
// and refusing those would lose the only answer there is.
func resalePrinting(b *matcher.Backend, id string, p CatalogProduct, foil, etched bool) string {
	if !strings.HasPrefix(skuNumber(p.SKU), resaleMarker) {
		return id
	}
	co, err := b.GetUUID(id)
	if err != nil || co.HasPromoType(magic.PromoTypeResale) {
		return id
	}
	var found string
	for _, card := range b.MatchInSet(co.Name, co.SetCode) {
		sibling, serr := b.GetUUID(card.UUID)
		if serr != nil || !sibling.HasPromoType(magic.PromoTypeResale) {
			continue
		}
		out, merr := b.MatchID(card.UUID, foil, etched)
		if merr != nil || out == id {
			continue
		}
		if found != "" && found != out {
			return id
		}
		found = out
	}
	if found == "" {
		return id
	}
	return found
}

// worldsSetFromSKU names the World Championship set a sku's year segment asks
// for. The 1996 Pro Tour Collector Set is filed as PTC, every later year as
// WC<yy>; the championships ended in 2004.
func worldsSetFromSKU(sku string) (string, bool) {
	if skuSetCode(sku) != "WCHP" {
		return "", false
	}
	number := skuNumber(sku)
	if len(number) < 2 || !unicode.IsDigit(rune(number[0])) || !unicode.IsDigit(rune(number[1])) {
		return "", false
	}
	if number[:2] == "96" {
		return "PTC", true
	}
	return "WC" + number[:2], true
}

// secondBucketMarker is what Star City Games appends to a sku's number segment
// to give a listing a second product record of its own.
const secondBucketMarker = "_CC"

// secondBucket reports whether a sku is the second record of a listing the
// catalog also carries plainly.
func secondBucket(sku string) bool {
	return strings.HasSuffix(skuNumber(sku), secondBucketMarker)
}

// bucketKey names the listing a record belongs to: the sku with the marker
// taken off its number segment, which both records of a pair answer with and
// no other sku does. It is what lets the two be filed together whichever of
// them the catalog streams first.
func bucketKey(sku string) string {
	fields := strings.Split(sku, "-")
	if len(fields) < 4 {
		return sku
	}
	fields[3] = strings.TrimSuffix(fields[3], secondBucketMarker)
	return strings.Join(fields, "-")
}

// lorcanaNumberRespellings spells the promo numbers two skus misnumber,
// keyed by the sku's number segment: Merlin - Envisioning the Future is PD1 7
// and the Top 128 Challenge Let It Go is DLPC 2, in the datastore and on
// TCGplayer alike.
var lorcanaNumberRespellings = map[string]string{
	"PD1_013":  "7",
	"P01_041a": "2",
}

// lorcanaNumber returns the collector number to match a Lorcana product by.
//
// The sku's number segment is the more specific of the two numbers a product
// carries. It keeps the printing marker the number field drops ("117M" beside
// its "117"), and for a promotional printing it spells the number under the
// promo series that issued it ("P3_031"), where the number field sometimes
// carries the whole undivided segment and sometimes only the tail. The series
// is a heading rather than part of the number - the datastore numbers that
// card 31 in Winterspell - so only what follows the last underscore is the
// number, and a segment whose tail names no number at all (the "T03" of a
// token) leaves the product's own field to answer.
//
// More specific is not the same as more reliable, and the sku only speaks
// where the product's own number does not contradict it: a stray digit
// ("0142" on the 042 it prices) and a marker the datastore numbers as a card
// of its own ("032B" for the 33 it files) each name a different card rather
// than the same card more precisely.
func lorcanaNumber(p CatalogProduct) string {
	number := skuNumber(p.SKU)
	if spelled, found := lorcanaNumberRespellings[number]; found {
		return spelled
	}
	idx := strings.LastIndexByte(number, '_')
	if idx >= 0 {
		number = number[idx+1:]
	}
	if number == "" || number[0] < '0' || number[0] > '9' {
		return p.CollectorNumber
	}
	digits := numberDigits(p.CollectorNumber)
	if digits != "" && digits != numberDigits(number) {
		return p.CollectorNumber
	}
	return number
}

// numberDigits returns the number a collector number opens with, as the
// datastore writes it: the leading run of digits without its padding.
func numberDigits(number string) string {
	end := 0
	for end < len(number) && number[end] >= '0' && number[end] <= '9' {
		end++
	}
	return strings.TrimLeft(number[:end], "0")
}

// lorcanaTotal returns the denominator a Lorcana product's number is one of,
// in the spelling the datastore's own SetTotal carries - the piece
// lorcanaNumber leaves out. A promo sku names its pool before the underscore
// lorcanaNumber cuts on; a main-set sku carries no pool, and its total comes
// from lorcanaMainSetTotal rather than the datastore's BaseSetSize, which
// runs past what Fabled and Into the Inklands print on their own cards.
func lorcanaTotal(b *matcher.Backend, p CatalogProduct) string {
	raw := skuNumber(p.SKU)
	idx := strings.LastIndexByte(raw, '_')
	if idx < 0 {
		return lorcanaMainSetTotal(b, p.Set)
	}
	series, tail := raw[:idx], raw[idx+1:]
	if tail == "" || tail[0] < '0' || tail[0] > '9' {
		return ""
	}
	digits := numberDigits(p.CollectorNumber)
	if digits != "" && digits != numberDigits(tail) {
		return ""
	}
	return lorcanaSeriesTotal(series)
}

// lorcanaSeriesTotal drops the zero-padding Star City Games hangs off a
// single-digit promo pool ("P01", "C02") that the datastore's own SetTotal
// never carries ("P1", "C2"). A pool with no digits to pad in the first place
// - "D23", "PD1" - is returned as given.
func lorcanaSeriesTotal(series string) string {
	if len(series) < 2 {
		return series
	}
	head, digits := series[:1], series[1:]
	if head != "P" && head != "C" {
		return series
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return series
		}
	}
	return head + strings.TrimLeft(digits, "0")
}

// lorcanaMainSetTotal returns the SetTotal printed on the named edition's
// own cards, read off the first one whose SetTotal is plain digits.
func lorcanaMainSetTotal(b *matcher.Backend, edition string) string {
	set, err := b.GetSetByName(edition)
	if err != nil {
		return ""
	}
	for _, card := range set.Cards {
		digits := card.SetTotal != ""
		for _, r := range card.SetTotal {
			if r < '0' || r > '9' {
				digits = false
				break
			}
		}
		if digits {
			return card.SetTotal
		}
	}
	return ""
}

// lorcanaFinish names a Lorcana treatment the way the datastore names it, and
// answers "" for the ones that need no naming.
//
// Star City Games sells a Lorcana treatment under a marketing name of its own,
// and the name is the set's rather than the treatment's: its "Inkwash Foil" is
// the datastore's Lava in one set, Magma in another and VerticalWave in a
// third. None of that has to be translated, because every printing behind
// those names is sold in one foil and the foil flag already reaches it.
//
// One does. A printing sold in its standard foil and in a second one beside it
// has two foils for the flag to choose between, and the flag always picks the
// standard - so on the flag alone the second foil's sku and the standard's land
// on one uuid, and a $16.78 buylist competes with a $12.56 one on the same
// card. That second foil is the Rainbow Pillars printing TCGplayer prices as
// Holofoil, which the catalog names Rainbow Foil, and naming it is what
// separates the two skus. A printing not sold in it is unaffected: the name
// reaches no uuid and the flag decides as before.
func lorcanaFinish(finish string) string {
	if finish == "Rainbow Foil" {
		return "Holofoil"
	}
	return ""
}

// lorcanaMarker returns the letters a Lorcana collector number ends in, which
// name a printing beside the one the digits alone name.
func lorcanaMarker(number string) string {
	return strings.TrimLeft(number, "0123456789")
}

// resolveLorcana returns the mtgban card id for a Lorcana catalog product.
//
// A number ending in letters names a printing beside the base one, and the
// datastore answers that in either of two ways. It numbers the sibling with
// the same letter, the way Into the Inklands numbers the five Dalmatian
// Puppies "4a" through "4e", and then the number resolves it outright. Or it
// numbers both the same and tells them apart in the name instead - "Bucky -
// Squirrel Squeak Tutor (Errata Version)" is its own printing at Rise of the
// Floodborn 73 - and then the number falls back to the base card and the two
// products land on one uuid, where a $12.99 errata printing prices a $0.39
// common.
//
// So a marker the resolved printing does not wear is asked of the name: the
// printing at that set and number whose name is the product's plus a suffix
// is the sibling the marker names. A marker with no such printing behind it
// is skipped rather than folded onto the base card: Star City Games sells a
// printing the datastore does not carry (a misprint, or one not added yet),
// and a missing price beats corrupting the price of a card that is carried.
//
// The vendor ids answer first, where the printing they name carries the
// sku's number.
func resolveLorcana(b *matcher.Backend, p CatalogProduct, foil bool) (string, error) {
	out, found := vendorPrinting(b, GameLorcana, p, foil, false)
	if found && carriesSkuNumber(b, GameLorcana, p, out) && !idContradictsProduct(b, p, out) {
		return resalePrinting(b, out, p, foil, false), nil
	}

	number := lorcanaNumber(p)
	variation := number
	total := lorcanaTotal(b, p)
	if total != "" {
		variation = number + "/" + total
	}
	if suffix, found := lorcanaExclusives[p.Language]; found {
		id, err := b.Match(&matcher.InputCard{
			Name:      p.Name + suffix,
			Edition:   p.Set,
			Variation: variation,
			Foil:      foil,
			Finish:    lorcanaFinish(p.Finish),
		})
		if err == nil {
			return id, nil
		}
	}
	id, err := b.Match(&matcher.InputCard{
		Name:      p.Name,
		Edition:   p.Set,
		Variation: variation,
		Foil:      foil,
		Finish:    lorcanaFinish(p.Finish),
	})
	if err != nil {
		return lorcanaQuest(b, p, number, foil, err)
	}
	if lorcanaMarker(number) == "" {
		return id, nil
	}
	co, cerr := b.GetUUID(id)
	if cerr != nil || strings.EqualFold(co.Number, strings.TrimLeft(number, "0")) {
		return id, nil
	}
	return lorcanaSibling(b, p, co, foil)
}

// lorcanaExclusives are what the datastore adds to a promo's name where the
// catalog sells it in Japanese or Simplified Chinese: the promo struck for
// that market is a card of its own beside the English one, and its name has
// already said which printing a letter on the sku's number would pick. A
// listing in that language the suffixed name does not answer is read by its
// plain name like any other.
var lorcanaExclusives = map[string]string{
	"Japanese":             " (JP Exclusive)",
	"Chinese - Simplified": " (CS Exclusive)",
}

// lorcanaQuest answers an Illumineer's Quest promo, which the catalog shelves
// under the set it promotes while the datastore files it in the Quest set at
// numbers 223 to 225. The set name alone cannot reach it, so the name and
// number are asked of every set and the answer is taken only where it is a
// Quest printing at the number asked; anything else keeps the refusal.
func lorcanaQuest(b *matcher.Backend, p CatalogProduct, number string, foil bool, refusal error) (string, error) {
	id, err := b.Match(&matcher.InputCard{
		Name:      p.Name,
		Variation: number,
		Foil:      foil,
		Finish:    lorcanaFinish(p.Finish),
	})
	if err != nil {
		return "", refusal
	}
	co, err := b.GetUUID(id)
	if err != nil || !strings.HasPrefix(co.Edition, "Illumineer's Quest") ||
		co.Number != strings.TrimLeft(number, "0") {
		return "", refusal
	}
	return id, nil
}

// otherFormats are what the Lorcana datastore adds to a card's name to file
// something that is not that card: a jumbo print of it, and a lot of several
// cards sold as one. Both sit at the base card's own set and number, so the
// set and the number cannot tell them from the printing a marker asks for.
var otherFormats = []string{"(Oversized)", "(Set of "}

// namesAnotherFormat reports whether what a longer name adds to a base one
// names a different physical product rather than another printing of the same
// card. Star City Games sells a jumbo card as a listing of its own, never as a
// marked variant of the card, so a marker must not be routed onto one.
func namesAnotherFormat(extension string) bool {
	for _, format := range otherFormats {
		if strings.Contains(extension, format) {
			return true
		}
	}
	return false
}

// lorcanaSibling answers with the printing the datastore files at the same set
// and number under a longer name, which is where it puts the errata reprint a
// sku marks with a letter. Exactly one such name may answer: two would leave
// the marker naming neither in particular.
func lorcanaSibling(b *matcher.Backend, p CatalogProduct, base *matcher.CardObject, foil bool) (string, error) {
	missing := fmt.Errorf("no printing beside %s %s for the sku marker", base.SetCode, base.Number)
	uuids, err := b.SearchHasPrefix(p.Name)
	if err != nil {
		return "", missing
	}
	var candidates []*matcher.CardObject
	for _, uuid := range uuids {
		co, err := b.GetUUID(uuid)
		if err != nil {
			continue
		}
		if !siblingCandidate(base, co) {
			continue
		}
		candidates = append(candidates, co)
	}
	if len(candidates) == 0 {
		return "", matcher.ErrUnsupported
	}
	found := soleSibling(candidates)
	if found == nil {
		return "", missing
	}
	return b.MatchID(found.UUID, foil, false)
}

// siblingCandidate reports whether a printing can be the one a sku marker
// names beside its base: the same slot in the same set, spelled as the
// base's name extended - and not extended into another physical format,
// which is a different product wearing the same slot.
func siblingCandidate(base, co *matcher.CardObject) bool {
	if co.Sealed || co.Name == base.Name {
		return false
	}
	if co.SetCode != base.SetCode || co.Number != base.Number {
		return false
	}
	return !namesAnotherFormat(strings.TrimPrefix(co.Name, base.Name))
}

// soleSibling answers the printing a set of candidates agrees on, and nothing
// where they name two cards, which would leave the marker naming neither in
// particular. They are counted by name because one printing is spelled once
// per treatment it is sold in, and those are the same card.
func soleSibling(candidates []*matcher.CardObject) *matcher.CardObject {
	var found *matcher.CardObject
	for _, co := range candidates {
		if found != nil && found.Name != co.Name {
			return nil
		}
		found = co
	}
	return found
}

// fabMatch answers the first printing one of the sku's collector numbers
// names under the given name, the candidates running most specific first.
// fabTiers are the rarities that name a printing rather than describe one.
// A marvel is a separate product sharing its card's number and treatment, so
// a listing that does not say which of the two it is has not been told apart:
// the catalog sells the Dynasty marvel of Construct Nitro Mechanoid beside
// the ordinary cold foil at DYN092, and only this word divides them.
var fabTiers = map[string]bool{
	"Marvel": true,
}

func fabMatch(b *matcher.Backend, name, edition, finish, rarity string, foil bool, numbers []string) (string, error) {
	var err error
	for _, number := range numbers {
		if fabTiers[rarity] {
			number = strings.TrimSpace(number + " " + rarity)
		}
		var id string
		id, err = b.Match(&matcher.InputCard{
			Name:      name,
			Edition:   edition,
			Variation: number,
			Finish:    finish,
			Foil:      foil,
		})
		if err == nil {
			return id, nil
		}
	}
	return "", err
}

// fabFinishes spells a finish the way the datastore does where the catalog
// names the same treatment by its color: the cold foil of a promo is gold,
// and "Gold Foil" is what Talismanic Lens FAB066 is sold as.
var fabFinishes = map[string]string{
	"Gold Foil": "Cold Foil",
}

// fabNames respells catalog names that disagree with the datastore.
var fabNames = map[string]string{
	"Deep Recess of Existence": "Deep Recesses of Existence",
	"Robe of Resourcefullness": "Robe of Resourcefulness",
	"Peace Art Card":           "Peace",

	"Gold / Goldkiss Rum // Gold / Goldkiss Rum": "Goldkiss Rum // Gold",
}

// fabNumberRespellings spells the collector numbers the catalog misspells:
// Attention Grabbers is APS006, the sixth card of the Pleiades armory deck,
// on the card and everywhere but this sku. Malice (Marvel) is IAR054; the
// catalog's IAR2-055 sku misnumbers it, and IAR055 is Vox Necropolis.
var fabNumberRespellings = map[string]string{
	"APS056":  "APS006",
	"1HK01":   "1HK012",
	"IAR2055": "IAR054",
}

// fabWordings are the labels the datastore tells the printings of one
// number apart by where the catalog lettered them instead: the three
// marvels of Seismic Surge are its 112a, 112b and 112c, sold as Trees,
// Crystals and Lava on the storefront, and labelled forest, crystal and
// lava in the datastore.
var fabWordings = map[string]string{
	"MPG112a": "forest",
	"MPG112b": "crystal",
	"MPG112c": "lava",
}

// fabInserts are the products the catalog sells as Flesh and Blood singles
// that are not cards, so no datastore will ever carry them.
var fabInserts = map[string]bool{
	"Binder Label": true,
}

// fabDuplicateStock are the skus SCG lists twice for one row the datastore
// does not distinguish further. Sanctuary of Aria's "027a" is titled
// "(Assorted Back)" on the storefront and its "027b" "(Feet)", both selling
// the same ROS027 token at their own price - $0.29 and $1.49 - and neither
// is more the card's price than the other, so resolving lands both on one
// uuid rather than name which is being sold. Both are refused instead of
// let one win by whichever streams first.
//
// A survey of the whole datastore for any other lettered number two skus
// land on found only this pair - the shapes fabPlainSibling, fabRenamedTwin
// and fabCreditedTwin steer are all a letter naming a printing the datastore
// does carry, which this is not - so the table is closed rather than a rule
// that would also catch those.
var fabDuplicateStock = map[string]bool{
	"SGL-FAB-ROS-027a-ENN": true,
	"SGL-FAB-ROS-027b-ENN": true,
}

// fabUnmade are the skus SCG lists for something no datastore row can be:
// a Rainbow Foil of two cards TCGplayer prints only in Normal and Cold Foil
// (Hyper Driver (Red) DYN110, Flat Trackers HVY155), and the Mastery Pack
// sheets of tokens and hero cards sold as one product (MPG H01, MPW 134 to
// 154) that name no single printing.
var fabUnmade = map[string]bool{
	"SGL-FAB-DYN-110-ENR":             true,
	"SGL-FAB-HVY-155-ENR":             true,
	"SGL-FAB-MPG-H01-ENN":             true,
	"SGL-FAB-MPW-134_135_154_154-ENN": true,
}

// fabArtPositions are the nine pieces the Antiquity Pack art cards cut a
// picture into, in the order the catalog numbers them: P01 is the top left
// piece of Eye of Ophidia and P09 the bottom right, then P10 through P18 the
// next picture. The datastore labels each piece by its position and numbers
// none of them, so the position is what the number becomes.
var fabArtPositions = []string{
	"top left", "top center", "top right",
	"middle left", "middle center", "middle right",
	"bottom left", "bottom center", "bottom right",
}

// fabArtPosition answers the piece an art card sku names, empty for any
// other product.
func fabArtPosition(name, sku string) string {
	if !strings.HasSuffix(name, " Art Card") {
		return ""
	}
	number := skuNumber(sku)
	if !strings.HasPrefix(number, "P") {
		return ""
	}
	index, err := strconv.Atoi(number[1:])
	if err != nil || index < 1 {
		return ""
	}
	return fabArtPositions[(index-1)%len(fabArtPositions)]
}

// fabPrintRun moves the print run out of a Flesh and Blood set name and into
// the finish. The catalog spells the run as part of the set, "Tales of Aria
// (1st Edition)", where the datastore keeps one set and crosses the run with
// the treatment, so the two runs of a card are two printings and the set name
// alone reaches neither in particular. A set without a run is left as it is.
func fabPrintRun(set, finish string) (string, string) {
	if translated, found := fabFinishes[finish]; found {
		finish = translated
	}
	var run string
	switch {
	case strings.HasSuffix(set, " (1st Edition)"):
		set, run = strings.TrimSuffix(set, " (1st Edition)"), "1st Edition"
	case strings.HasSuffix(set, " (Unlimited)"):
		set, run = strings.TrimSuffix(set, " (Unlimited)"), "Unlimited Edition"
	default:
		return set, finish
	}
	// The catalog names the plain treatment for what it is not; the
	// datastore names it for what it is.
	if finish == "" || finish == "Non-foil" {
		finish = "Normal"
	}
	return set, matcher.PrintingFinish(run, finish)
}

// catalogCondition maps a catalog condition string to an mtgban grade.
func catalogCondition(condition string) (mtgban.Condition, error) {
	switch condition {
	case "Near Mint":
		return mtgban.NM, nil
	case "Played":
		return mtgban.SP, nil
	case "Heavily Played":
		return mtgban.HP, nil
	default:
		return "", fmt.Errorf("unknown condition %q", condition)
	}
}
