// Package matcher resolves the loose card descriptions storefronts
// publish, a name and whatever qualifiers they chose, to the one printing
// they meant, and gives every printing a stable uuid to price against.
//
// Each game registers its own datastore loader and matching rules; see the
// matcher/<game> packages, or matcher/games to link them all in.
package matcher

import (
	"errors"
	"slices"
	"strings"
)

// finishTwins reports whether two set-mates are one card filed as two
// finish-split entries: the same collector number - the foil twin only adds
// a suffix - in the same language, with no primary finish sold by both,
// which is what tells a twin apart from a promo that happens to share the
// number. The three finishes named are the axis the flags can ask for,
// which every loader files its game's vocabulary into, not a magic idiom: a
// secondary treatment both entries sell - a signed art card - must not hide
// that the pair splits on the axis.
func finishTwins(co, altCo *CardObject) bool {
	if ExtractNumberValue(co.Number) != ExtractNumberValue(altCo.Number) ||
		co.Language != altCo.Language {
		return false
	}
	sameFinish := (co.HasFinish(FinishNonfoil) && altCo.HasFinish(FinishNonfoil)) ||
		(co.HasFinish(FinishFoil) && altCo.HasFinish(FinishFoil)) ||
		(co.HasFinish(FinishEtched) && altCo.HasFinish(FinishEtched))
	return !sameFinish
}

// IsFinishTwin reports whether the printing is one of a pair the set filed
// as two entries for the one card, the twin's number differing only by a
// suffix. The star such a number ends in is the twin's own, and says nothing
// about a misprint.
func (b *Backend) IsFinishTwin(inputID string) bool {
	co, err := b.cardObject4Id(inputID)
	if err != nil {
		return false
	}
	for _, variation := range co.Variations {
		altCo, found := b.UUIDs[variation]
		if !found {
			continue
		}
		if finishTwins(co, altCo) {
			return true
		}
	}
	return false
}

// FinishSiblings answers every uuid the card behind the id is sold under,
// itself included: the printing's registered finishes first - the base, the
// shared ones, then the game's own vocabulary in sorted order - and, for the
// sets that filed a foil as a card of its own, the set-mates that are its
// finish twins. A sealed product or an unknown id answers with what it is.
func (b *Backend) FinishSiblings(inputID string) []string {
	co, err := b.cardObject4Id(inputID)
	if err != nil {
		return nil
	}

	var siblings []string
	seen := map[string]bool{}
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		_, found := b.UUIDs[id]
		if !found {
			return
		}
		seen[id] = true
		siblings = append(siblings, id)
	}

	addFinishes := func(co *CardObject) {
		for _, finish := range []string{FinishNonfoil, FinishFoil, FinishEtched} {
			add(co.FoilUUIDs[finish])
		}
		var extra []string
		for finish := range co.FoilUUIDs {
			switch finish {
			case FinishNonfoil, FinishFoil, FinishEtched:
				continue
			}
			extra = append(extra, finish)
		}
		slices.Sort(extra)
		for _, finish := range extra {
			add(co.FoilUUIDs[finish])
		}
		// A card without a registered map is its own only finish
		add(co.UUID)
	}
	addFinishes(co)

	// A twin is a whole card entry, so its registered finishes come along:
	// entering from the etched twin must still surface the base's foil.
	for _, variation := range co.Variations {
		altCo, found := b.UUIDs[variation]
		if !found {
			continue
		}
		if finishTwins(co, altCo) {
			addFinishes(altCo)
		}
	}
	return siblings
}

// cardObject4Id resolves whatever identifier a caller sends - one of the
// matcher's own uuids, or an external product id - to the entry it names.
// The maps decide what an id is: only Magic spells its uuids the way mtgjson
// does and only some games number their products, so an id is looked up as
// it was sent rather than measured against either shape first.
func (b *Backend) cardObject4Id(inputID string) (*CardObject, error) {
	if inputID == "" {
		return nil, ErrCardUnknownID
	}

	// Look up as one of the matcher's own uuids first, then through the id
	// spaces in their fixed order
	co, found := b.UUIDs[inputID]
	for _, space := range idSpaceOrder {
		if found {
			break
		}
		co, found = b.UUIDs[b.ExternalIdentifiers[space][inputID]]
	}
	if !found {
		return nil, ErrCardUnknownID
	}
	return co, nil
}

// FinishUUID resolves a finish name, spelled the way any source spells it, to
// the uuid of the printing's sibling sold in it, and "" when the printing is
// not sold in it at all. The name is read through FinishSlug, and a finish
// the printing is sold in under another print run answers for it (otherRun).
func (b *Backend) FinishUUID(card *Card, finish string) string {
	if b.rules == nil {
		return ""
	}
	canonical := FinishSlug(finish)
	if canonical == "" {
		return ""
	}
	if uuid, found := card.FoilUUIDs[canonical]; found {
		return uuid
	}
	return b.otherRun(card, canonical)
}

// MatchIDFinish answers an id with the uuid of the printing's sibling sold in
// the named finish, whichever sibling the id itself names: it promotes and
// demotes over the game's whole finish vocabulary the way the flags do over
// the three finishes they can name, so a plain uuid reaches its holofoil and
// a holofoil uuid reaches the plain one.
//
// A finish the printing is not sold in is an error, never another finish's
// uuid: the caller is pricing one sku, and answering with a sibling files two
// sku prices under one uuid, which is the whole point of a uuid per finish.
// It reports a datastore that does not carry a printing the vendor sells,
// where the flag form would have quietly clamped the price onto the finish it
// does carry.
//
// One of the matcher's own uuids names a printing and answers from its
// siblings alone. A vendor's id names a product, which can hold more than
// one printing: a finish the printing it files at is not sold in comes from a
// set-mate sold under the same product (soldUnder), and a product filed at an
// etched printing answers foil with it, since TCGplayer sells an etched foil
// as a product of its own and calls it Foil. Unlike the flag form it never
// reaches a twin the product does not hold.
func (b *Backend) MatchIDFinish(inputID, finish string) (string, error) {
	co, err := b.cardObject4Id(inputID)
	if err != nil {
		return "", err
	}
	// A sealed product is one product in no finish at all, so a caller
	// naming one is describing the singles it holds, not the box.
	if co.Sealed {
		return co.UUID, nil
	}
	if b.rules == nil {
		return "", ErrDatastoreEmpty
	}
	canonical := FinishSlug(finish)
	if canonical == "" {
		b.Logf("Finish %q is not one this game names", finish)
		return "", ErrCardUnnamedFinish
	}
	var outID string
	tags := b.idTags(&co.Card, inputID)
	if len(tags) > 0 && co.Etched && canonical == FinishFoil {
		outID = co.UUID
	} else {
		outID = b.FinishUUID(&co.Card, finish)
	}
	if outID == "" && len(tags) > 0 {
		outID = b.productSibling(co, inputID, tags, finish)
	}
	if outID == "" {
		if !b.knownFinishes[canonical] {
			b.Logf("Finish %q is not one this datastore sells", finish)
			return "", ErrCardUnnamedFinish
		}
		b.Logf("Printing %s is not sold in finish %q", co.UUID, finish)
		return "", ErrCardWrongFinish
	}
	// Validate that what we found is correct
	if _, found := b.UUIDs[outID]; !found {
		return "", ErrCardUnknownID
	}
	return outID, nil
}

// idTags names the identifiers a printing carries a vendor's id under, and
// none for one of the matcher's own uuids, which names the printing alone.
func (b *Backend) idTags(card *Card, inputID string) []string {
	_, isUUID := b.UUIDs[inputID]
	if isUUID {
		return nil
	}
	var tags []string
	for tag, id := range card.Identifiers {
		if id == inputID {
			tags = append(tags, tag)
		}
	}
	return tags
}

// productSibling answers a finish from the set-mates of a printing sold under
// the product a vendor's id names, and "" where none is sold in it or two
// different printings are.
func (b *Backend) productSibling(co *CardObject, inputID string, tags []string, finish string) string {
	var answer string
	for _, variation := range co.Variations {
		altCo, found := b.UUIDs[variation]
		if !found || !soldUnder(co, altCo, inputID, tags) {
			continue
		}
		outID := b.FinishUUID(&altCo.Card, finish)
		if outID == "" {
			continue
		}
		if answer != "" && answer != outID {
			return ""
		}
		answer = outID
	}
	return answer
}

// soldUnder reports whether a set-mate is sold under the vendor's id the
// printing carries under tags: the set-mate carries it too, and no other id
// of those kinds, or it is the printing's foil twin numbered with a star and
// carries none of them, a foil sold under its card's product (FRF 65★ under
// #65's). A misprint the catalog stars shares a finish with its card, which
// finishTwins rules out.
func soldUnder(co, altCo *CardObject, inputID string, tags []string) bool {
	carries := false
	for _, tag := range tags {
		id := altCo.Identifiers[tag]
		if id == "" {
			continue
		}
		if id != inputID {
			return false
		}
		carries = true
	}
	if carries {
		return true
	}
	return altCo.Foil && altCo.Number == co.Number+"★" && finishTwins(co, altCo)
}

// matchIDFor answers an id the way the caller asked about it. The two forms
// differ in reach, not just in spelling: the flags may land on any foil Magic
// files as a printing of its own, while a named finish reaches only the
// printings the id itself names - a uuid's own, or those a vendor's product
// is sold as (MatchIDFinish) - which is what lets it be loud about a finish
// none of them is sold in.
func (b *Backend) matchIDFor(inCard *InputCard) (string, error) {
	if inCard.Finish != "" {
		return b.MatchIDFinish(inCard.ID, inCard.Finish)
	}
	return b.MatchID(inCard.ID, inCard.Foil, inCard.IsEtched())
}

// MatchID resolves an identifier a storefront already knows, one of the
// matcher's uuids or an external product id, to the uuid of a printing. The
// optional flags ask for the foil or etched sibling, and are answered only
// where the printing was sold in one.
func (b *Backend) MatchID(inputID string, finishes ...bool) (string, error) {
	co, err := b.cardObject4Id(inputID)
	if err != nil {
		return "", err
	}

	isEtched := len(finishes) > 1 && finishes[1]
	isFoil := len(finishes) > 0 && finishes[0] && !isEtched

	// If the loaded card already matches the requested finishes
	// return the found id straight away
	if (co.Foil && isFoil) || (co.Etched && isEtched) ||
		(!co.Foil && !co.Etched && !isFoil && !isEtched) {
		return co.UUID, nil
	}

	outID := b.output(co.Card, finishes...)

	// Validate that what we found is correct
	co, found := b.UUIDs[outID]
	if !found {
		return "", ErrCardUnknownID
	}

	// If the input card was requested as foil, we should double check
	// if the original card has a foil under a separate id
	if co.Foil != isFoil || co.Etched != isEtched {
		// So we iterate over the Variations array and try outputting ids
		// until we find a perfect match in foiling status
		for _, variation := range co.Variations {
			// A missing key yields a nil pointer, not an empty card, so
			// every read of this map has to be checked before use
			altCo, found := b.UUIDs[variation]
			if !found {
				continue
			}
			if !finishTwins(co, altCo) {
				continue
			}
			maybeID := b.output(altCo.Card, isFoil, isEtched)
			altCo, found = b.UUIDs[maybeID]
			if !found {
				continue
			}

			// If the alt card finish matches the expected one
			// then replace the final output uuid
			if altCo.Foil == isFoil && altCo.Etched == isEtched {
				outID = maybeID
				break
			}
		}
		// Nothing is sold etched, neither this printing nor a twin of it:
		// the etched flag named a finish the card does not come in, and
		// the foil flag beside it is what the caller asked for next.
		if isEtched && len(finishes) > 0 && finishes[0] && !b.UUIDs[outID].Etched {
			return b.MatchID(inputID, true, false)
		}
	}
	return outID, nil
}

// MatchIDLanguage returns the printing of the card behind uuid in language,
// in the same finish: the card itself when it is in that language, or else
// the one its foreignData names for it. ErrCardUnknownID answers a language
// the datastore loads no printing of the card in.
func (b *Backend) MatchIDLanguage(uuid, language string) (string, error) {
	co, err := b.GetUUID(uuid)
	if err != nil {
		return "", err
	}
	if co.Language == language {
		return co.UUID, nil
	}
	for _, fd := range co.ForeignData {
		if fd.Language == language {
			return b.MatchID(fd.UUID, co.Foil, co.Etched)
		}
	}
	return "", ErrCardUnknownID
}

// Match resolves a storefront's description of a card to the uuid of the one
// printing it names, reporting ErrAliasing when the description fits more than
// one. The input is normalized in place, so a caller can see what the matcher
// made of it.
func (b *Backend) Match(inCard *InputCard) (cardID string, err error) {
	if b.Sets == nil {
		return "", ErrDatastoreEmpty
	}

	// Adjust flag as needed
	if inCard.IsFoil() {
		inCard.Foil = true
	}

	inCard.normalizeLanguage()

	// Look up by uuid
	if inCard.ID != "" {
		b.Logf("Performing id lookup for %s", inCard.ID)
		outID, err := b.matchIDFor(inCard)
		// The wording cannot improve on a finish the printing does not
		// carry: it would answer from the same printing, and the only
		// answer it has is another finish's uuid. A name the game could not
		// place says nothing about the printing, so that one falls through.
		if errors.Is(err, ErrCardWrongFinish) {
			return "", err
		}
		if err == nil {
			co := b.UUIDs[outID]
			b.Logf("Id found: %v", b.describe(inCard))

			// A product id names a card sold in every language, which the
			// listing's language narrows to its printing where one is loaded
			if inCard.Language != "" && !matchesLanguage(co.Language, inCard.Language) {
				id, err := b.MatchIDLanguage(outID, inCard.Language)
				if err == nil {
					outID, co = id, b.UUIDs[id]
				}
			}

			// Validation step
			switch {
			// No printing of the id is loaded in the requested language
			case inCard.Language != "" && !matchesLanguage(co.Language, inCard.Language):
				b.Log("Language validation failed, resetting card")
				inCard.Name = co.Name
				inCard.Edition = co.Edition
				inCard.Variation = co.Number
				inCard.Foil = co.Foil
				if co.Etched {
					inCard.AddToVariant("etched")
				}
			// Tokens are unsupported for broken ids in different languages
			case inCard.Language != "" && co.Layout == "token":
				return "", ErrUnsupported
			// This runs before b.rules is known non-nil, hence the check
			case b.rules != nil && b.rules.IsUnsupported(b, inCard, co, StageAnswer):
				b.Log("Missing necessary tag")
				return "", ErrUnsupported
			// Actually found id
			default:
				return outID, nil
			}
		}
		b.Log("Id lookup failed, attempting full match")
	}

	// In case id lookup failed, an no more data is present
	if inCard.Name == "" {
		return "", ErrCardDoesNotExist
	}
	ogName := inCard.Name

	// A Backend without attached GameRules cannot match anything; check before
	// the prefilter below, which runs the game's name preprocessing.
	rules := b.rules
	if rules == nil {
		return "", ErrDatastoreEmpty
	}

	// Prefilter runs the game-specific name/variant preprocessing before the
	// canonical-name lookup: Magic splits bracketed editions and parenthesized
	// or dashed variants off the name, Lorcana only the parenthetical (its
	// names are "Character - Title"), plus each game's token/name fixups.
	rules.Prefilter(b, inCard)

	// Re-check foil in case prefilter moved a finish hint into the variant.
	if inCard.IsFoil() {
		inCard.Foil = true
	}
	if ogName != inCard.Name {
		b.Logf("Pre-adjusted name from '%s' to '%s' '%s'", ogName, inCard.Name, inCard.Variation)
	}

	// Skip unsupported sets
	if rules.IsUnsupported(b, inCard, nil, StageWording) {
		return "", ErrUnsupported
	}

	// Get the card basic info to retrieve the Printings array
	canonicalName, found := b.CanonicalNames[Normalize(inCard.Name)]
	// A token carrying no Token in its name is filed under a key that says
	// it, leaving the plain one to whatever card normalizes the same way -
	// the Unsanctioned "Bat-" answers for "Bat". An edition that files
	// tokens is asking for the token, so let its key answer first; so is a
	// variation that says the word under a plain edition name, or the card
	// sharing the bucket would answer for the token beside it.
	var viaTokenKey bool
	if tokenName, ok := b.CanonicalNames[Normalize(inCard.Name)+"token"]; ok &&
		(b.editionFilesTokens(inCard.Edition) || Contains(inCard.Variation, "Token")) {
		canonicalName, found, viaTokenKey = tokenName, true, true
	}
	if !found {
		ogName := inCard.Name
		// Fixup up the name and try again
		rules.AdjustName(b, inCard)
		if ogName != inCard.Name {
			inCard.OriginalName = ogName
			b.Logf("Adjusted name from '%s' to '%s'", ogName, inCard.Name)
		}

		canonicalName, found = b.CanonicalNames[Normalize(inCard.Name)]
		if !found {
			// Return a safe error if it's a token
			if b.IsToken(ogName) || Contains(inCard.Variation, "Oversize") {
				return "", ErrUnsupported
			}
			return "", ErrCardDoesNotExist
		}
	}

	// Restore the card to the canonical MTGJSON name
	ogName = inCard.Name
	inCard.Name = canonicalName

	// Fix up edition
	ogEdition := inCard.Edition
	rules.AdjustEdition(b, inCard)
	if ogName != inCard.Name {
		b.Logf("Re-adjusted name from '%s' to '%s'", ogName, inCard.Name)
	}
	if ogEdition != inCard.Edition {
		b.Logf("Adjusted edition from '%s' to '%s'", ogEdition, inCard.Edition)
	}

	// Extra check, after any possible edition adjustment has been done
	switch {
	// For any unsupported set that wasn't processed previously
	case inCard.Contains("Oversize") && !b.hasOversizedPrinting(inCard.Name):
		return "", ErrUnsupported
	// For any specific missing card
	case rules.IsUnsupported(b, inCard, nil, StageEdition):
		return "", ErrUnsupported
	}

	printings, err := b.Printings4Card(inCard.Name)
	if err != nil {
		b.Logf("Printings error: %v", err)
		return "", err
	}

	// If there are multiple printings of the card, filter out to the
	// minimum common elements, using the rules defined.
	// Given that many tokens are not supported, make sure to filter
	// out unrelated editions.
	b.Logf("Processing %v %v", b.describe(inCard), printings)
	// A name answered by the token key never passed through AdjustName, which
	// is what would have suffixed it and asked for the filter below. Ask for
	// it here instead, or a token carrying a single printing would be served
	// for whatever edition the listing named.
	if len(printings) > 1 || viaTokenKey || strings.HasSuffix(ogName, "Token") {
		printings = rules.FilterPrintings(b, inCard, printings)
		b.Logf("Filtered printings: %v", printings)

		// Filtering was too aggressive or wrong data fed,
		// in either case, nothing else to be done here.
		if len(printings) == 0 {
			// Return a safe error if it's a token
			if b.IsToken(ogName) || Contains(inCard.Variation, "Oversize") {
				return "", ErrUnsupported
			}
			return "", ErrCardNotInEdition
		}
	}

	// Every printing filed under the name in a set the game admits, once
	// whatever finishes it is sold in. The index is keyed by Normalize, so
	// printings spelled apart by case or punctuation stay reachable.
	codes := rules.CandidateSets(b, inCard, printings)
	candidates := make([]CandidateSet, len(codes))
	for i, code := range codes {
		candidates[i].Code = code
	}
	slices.SortFunc(candidates, func(x, y CandidateSet) int { return strings.Compare(x.Code, y.Code) })
	candidates = slices.CompactFunc(candidates, func(x, y CandidateSet) bool { return x.Code == y.Code })
	seen := map[string]bool{}
	for _, uuid := range b.Hashes[Normalize(inCard.Name)] {
		co, found := b.UUIDs[uuid]
		if !found || co.Sealed {
			continue
		}
		i, admitted := slices.BinarySearchFunc(candidates, co.SetCode, func(set CandidateSet, code string) int {
			return strings.Compare(set.Code, code)
		})
		if !admitted {
			continue
		}
		key := PrintingKey(co.Card)
		if seen[key] {
			continue
		}
		seen[key] = true
		candidates[i].Cards = append(candidates[i].Cards, co.Card)
	}

	b.Log("Found these possible matches")
	for _, set := range candidates {
		for _, card := range set.Cards {
			b.Logf("%s %s %s", card.SetCode, card.Name, card.Number)
		}
	}

	// Filter the candidates using all the input card details. The game's rules
	// own this step, so even a single candidate is validated rather than used
	// blindly (Lorcana enforces the collector number here; a lone candidate
	// taken as is could be a wrong-numbered card).
	b.Log("Now filtering...")
	outCards := rules.FilterCards(b, inCard, candidates)

	b.Log("Post filtering status...")
	for _, card := range outCards {
		b.Logf("%s %s %s", card.SetCode, card.Name, card.Number)
	}

	// Final game policy runs before language filtering: Magic
	// trims World Championship candidates here, even across languages.
	outCards = rules.FinalizeCandidates(b, inCard, outCards)

	// Language check - out of filterCards to catch single cases too
	if inCard.Language != "" || len(outCards) > 1 {
		var filteredOutCards []Card
		for _, card := range outCards {
			if (inCard.Language == "" && card.Language != "English") ||
				!matchesLanguage(card.Language, inCard.Language) {
				b.Log("Dropping different language prints...")
				b.Logf("%s %s %s %s", card.SetCode, card.Name, card.Number, card.Language)
				continue
			}
			filteredOutCards = append(filteredOutCards, card)
		}
		outCards = filteredOutCards
	}

	// Finish line
	switch len(outCards) {
	// Not found, rip
	case 0:
		b.Log("No matches...")
		err = ErrCardWrongVariant
		if inCard.Variation == "" {
			err = ErrCardMissingVariant
		}
		if inCard.Language != "" {
			err = ErrUnsupported
		}
	// Victory
	case 1:
		b.Log("Found it!")

		cardID = b.output(outCards[0], inCard.Foil, inCard.IsEtched())

		co := b.UUIDs[cardID]
		b.Logf("%v -> %v", b.describe(inCard), co)

		// Validation step
		if rules.IsUnsupported(b, inCard, co, StageAnswer) {
			b.Log("...but it's invalid")
			return "", ErrUnsupported
		}
	// FOR SHAME
	default:
		b.Log("Aliasing...")
		alias := NewAliasingError()
		for i := range outCards {
			alias.Dupes = append(alias.Dupes, b.output(outCards[i], inCard.Foil, inCard.IsEtched()))
		}
		err = alias
	}

	return
}

// MatchInSet returns every printing in the set whose name is exactly the one
// given. A combined name is matched on its first half alone.
func (b *Backend) MatchInSet(cardName string, setCode string) (outCards []Card) {
	set, found := b.Sets[setCode]
	if !found {
		return
	}
	for _, card := range set.Cards {
		// Cut rather than Split: only the front half is ever read, and
		// Split allocates a slice for every card in the set to hand it
		// over, whether or not the name has two halves at all
		front, _, _ := strings.Cut(card.Name, " // ")
		if cardName == card.Name || cardName == front {
			outCards = append(outCards, card)
		}
	}
	return
}

// MatchInSetNumber returns every printing in the set with exactly this name
// and collector number.
func (b *Backend) MatchInSetNumber(cardName, setCode, number string) (outCards []Card) {
	set, found := b.Sets[setCode]
	if !found {
		return
	}
	for _, card := range set.Cards {
		if cardName == card.Name && card.Number == number {
			outCards = append(outCards, card)
		}
	}
	return
}

// MatchWithNumber returns every printing with this set code and collector
// number. The name only narrows the result and may be empty.
func (b *Backend) MatchWithNumber(cardName, setCode, number string) (outCards []Card) {
	set, found := b.Sets[setCode]
	if !found {
		return
	}
	for _, card := range set.Cards {
		if Contains(card.Name, cardName) && card.Number == number {
			outCards = append(outCards, card)
		}
	}
	return
}
