package httpapi

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
)

const (
	maximumCatalogNumberLength   = 1000
	maximumIdentifierTypeLength  = 255
	maximumIdentifierValueLength = 4096
	errorLookupPair              = "%s and %s must be provided together"
	errorLookupCombination       = "catalog-number and identifier lookups cannot be combined"
	errorLookupFilterCombination = "exact release lookup cannot be combined with title, country, year, month, or master filters"
)

type releaseLookupKind uint8

const (
	releaseLookupNone releaseLookupKind = iota
	releaseLookupCatalogNumber
	releaseLookupIdentifier
)

type releaseLookup struct {
	kind          releaseLookupKind
	catalogNumber catalog.CatalogNumberLookup
	identifier    catalog.IdentifierLookup
}

func parseReleaseLookup(values url.Values) (releaseLookup, error) {
	hasLabelID := hasQueryParameter(values, ParameterLabelID)
	hasCatalogNumber := hasQueryParameter(values, ParameterCatalogNumber)
	hasIdentifierType := hasQueryParameter(values, ParameterIdentifierType)
	hasIdentifierValue := hasQueryParameter(values, ParameterIdentifierValue)
	hasCatalogLookup := hasLabelID || hasCatalogNumber
	hasIdentifierLookup := hasIdentifierType || hasIdentifierValue
	if hasCatalogLookup && hasIdentifierLookup {
		return releaseLookup{}, errors.New(errorLookupCombination)
	}
	if !hasCatalogLookup && !hasIdentifierLookup {
		return releaseLookup{}, nil
	}
	if hasStandardReleaseFilter(values) {
		return releaseLookup{}, errors.New(errorLookupFilterCombination)
	}
	if hasCatalogLookup {
		return parseCatalogNumberLookup(values, hasLabelID, hasCatalogNumber)
	}
	return parseIdentifierLookup(values, hasIdentifierType, hasIdentifierValue)
}

func parseCatalogNumberLookup(values url.Values, hasLabelID, hasCatalogNumber bool) (releaseLookup, error) {
	if !hasLabelID || !hasCatalogNumber {
		return releaseLookup{}, fmt.Errorf(errorLookupPair, ParameterLabelID, ParameterCatalogNumber)
	}
	if !hasText(values.Get(ParameterLabelID)) {
		return releaseLookup{}, fmt.Errorf(errorRequired, ParameterLabelID)
	}
	labelID, err := parseBoundedInt64(values.Get(ParameterLabelID), 0, 1, MaximumResourceID, ParameterLabelID)
	if err != nil {
		return releaseLookup{}, err
	}
	catalogNumber, err := requiredTextFilter(
		values.Get(ParameterCatalogNumber),
		1,
		maximumCatalogNumberLength,
		ParameterCatalogNumber,
	)
	if err != nil {
		return releaseLookup{}, err
	}
	return releaseLookup{
		kind: releaseLookupCatalogNumber,
		catalogNumber: catalog.CatalogNumberLookup{
			LabelID: labelID, CatalogNumber: catalogNumber,
		},
	}, nil
}

func parseIdentifierLookup(values url.Values, hasType, hasValue bool) (releaseLookup, error) {
	if !hasType || !hasValue {
		return releaseLookup{}, fmt.Errorf(errorLookupPair, ParameterIdentifierType, ParameterIdentifierValue)
	}
	identifierType, err := requiredTextFilter(
		values.Get(ParameterIdentifierType),
		1,
		maximumIdentifierTypeLength,
		ParameterIdentifierType,
	)
	if err != nil {
		return releaseLookup{}, err
	}
	identifierValue, err := requiredTextFilter(
		values.Get(ParameterIdentifierValue),
		1,
		maximumIdentifierValueLength,
		ParameterIdentifierValue,
	)
	if err != nil {
		return releaseLookup{}, err
	}
	return releaseLookup{
		kind: releaseLookupIdentifier,
		identifier: catalog.IdentifierLookup{
			Type: identifierType, Value: identifierValue,
		},
	}, nil
}

func hasStandardReleaseFilter(values url.Values) bool {
	for _, name := range []string{ParameterTitle, ParameterCountry, ParameterYear, ParameterMonth, ParameterMaster} {
		if hasText(values.Get(name)) {
			return true
		}
	}
	return false
}

func hasText(value string) bool {
	return strings.TrimSpace(value) != ""
}

func hasQueryParameter(values url.Values, name string) bool {
	_, exists := values[name]
	return exists
}
