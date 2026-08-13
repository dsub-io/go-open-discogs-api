package httpapi

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
)

const (
	defaultPageSize = 20
	maximumPageSize = 30
	errorCursor     = "cursor is invalid"
)

type PageResponse[T catalog.PageItem] struct {
	Items       []T    `json:"items"`
	NextAfterID *int64 `json:"next_after_id"`
	HasMore     bool   `json:"has_more"`
	PageSize    int    `json:"page_size"`
	ResourceURI string `json:"resource_uri"`
}

type HashPageResponse[T catalog.HashPageItem] struct {
	Items       []T     `json:"items"`
	NextCursor  *string `json:"next_cursor"`
	HasMore     bool    `json:"has_more"`
	PageSize    int     `json:"page_size"`
	ResourceURI string  `json:"resource_uri"`
}

func pageResponse[T catalog.PageItem](page catalog.Page[T], resourceURI string) PageResponse[T] {
	items := page.Items
	if items == nil {
		items = make([]T, 0)
	}
	return PageResponse[T]{
		Items:       items,
		NextAfterID: page.NextAfterID(),
		HasMore:     page.HasMore,
		PageSize:    len(items),
		ResourceURI: resourceURI,
	}
}

func hashPageResponse[T catalog.HashPageItem](page catalog.HashPage[T], resourceURI string) HashPageResponse[T] {
	items := page.Items
	if items == nil {
		items = make([]T, 0)
	}
	return HashPageResponse[T]{
		Items:       items,
		NextCursor:  encodeHashCursor(page.NextAfterHash()),
		HasMore:     page.HasMore,
		PageSize:    len(items),
		ResourceURI: resourceURI,
	}
}

func parseCursorPage(values url.Values) (catalog.PageRequest, error) {
	afterID, err := parseBoundedInt64(
		values.Get(ParameterAfterID),
		0,
		0,
		MaximumResourceID,
		ParameterAfterID,
	)
	if err != nil {
		return catalog.PageRequest{}, err
	}
	size, err := parseBoundedInt(values.Get(ParameterSize), defaultPageSize, 1, maximumPageSize, ParameterSize)
	if err != nil {
		return catalog.PageRequest{}, err
	}
	return catalog.PageRequest{AfterID: afterID, Size: size}, nil
}

func parseHashCursorPage(values url.Values) (catalog.HashPageRequest, error) {
	afterHash, err := decodeHashCursor(values.Get(ParameterCursor))
	if err != nil {
		return catalog.HashPageRequest{}, err
	}
	size, err := parseBoundedInt(values.Get(ParameterSize), defaultPageSize, 1, maximumPageSize, ParameterSize)
	if err != nil {
		return catalog.HashPageRequest{}, err
	}
	return catalog.HashPageRequest{AfterHash: afterHash, Size: size}, nil
}

func encodeHashCursor(hash *int32) *string {
	if hash == nil {
		return nil
	}
	encoded := strconv.FormatInt(int64(*hash), 10)
	value := base64.RawURLEncoding.EncodeToString([]byte(encoded))
	return &value
}

func decodeHashCursor(raw string) (*int32, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s", errorCursor)
	}
	parsed, err := strconv.ParseInt(string(decoded), 10, 32)
	if err != nil {
		return nil, fmt.Errorf("%s", errorCursor)
	}
	value := int32(parsed)
	return &value, nil
}

func parseBoundedInt(raw string, fallback, minimum, maximum int, name string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be between %d and %d", name, minimum, maximum)
	}
	return value, nil
}

func parseBoundedInt64(raw string, fallback, minimum, maximum int64, name string) (int64, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be between %d and %d", name, minimum, maximum)
	}
	return value, nil
}
