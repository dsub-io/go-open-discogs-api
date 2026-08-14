package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	errorIDRange           = "id must be between 1 and %d"
	errorBoolean           = "%s must be true or false"
	errorTextEncoding      = "%s must be valid UTF-8"
	errorTextLength        = "%s must contain between %d and %d characters"
	errorMonthRequiresYear = "month requires year"
	errorRequired          = "%s is required"
	minimumSearchLength    = 3
	maximumSearchLength    = 200
	maximumCountryLength   = 255
)

func requiredTextFilter(raw string, minimum, maximum int, name string) (string, error) {
	value, err := optionalTextFilter(raw, minimum, maximum, name)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf(errorRequired, name)
	}
	return value, nil
}

func pathID(request *http.Request) (int64, error) {
	id, err := strconv.ParseInt(request.PathValue(ParameterID), 10, 64)
	if err != nil || id < 1 || id > MaximumResourceID {
		return 0, fmt.Errorf(errorIDRange, MaximumResourceID)
	}
	return id, nil
}

func optionalInt(raw string, minimum, maximum int, name string) (*int, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := parseBoundedInt(raw, 0, minimum, maximum, name)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func optionalBool(raw, name string) (*bool, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, fmt.Errorf(errorBoolean, name)
	}
	return &value, nil
}

func optionalSearchTerm(raw, name string) (string, error) {
	return optionalTextFilter(raw, minimumSearchLength, maximumSearchLength, name)
}

func optionalCountry(raw string) (string, error) {
	return optionalTextFilter(raw, 1, maximumCountryLength, ParameterCountry)
}

func optionalTextFilter(raw string, minimum, maximum int, name string) (string, error) {
	if !utf8.ValidString(raw) {
		return "", fmt.Errorf(errorTextEncoding, name)
	}
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return "", nil
	}
	length := utf8.RuneCountInString(normalized)
	if length < minimum || length > maximum {
		return "", fmt.Errorf(errorTextLength, name, minimum, maximum)
	}
	return normalized, nil
}
