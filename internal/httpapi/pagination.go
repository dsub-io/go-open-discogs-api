package httpapi

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
)

const (
	defaultPageSize = 20
	maximumPageSize = 30
)

type PageResponse[T catalog.PageItem] struct {
	Items       []T    `json:"items"`
	NextAfterID *int64 `json:"next_after_id"`
	HasMore     bool   `json:"has_more"`
	PageSize    int    `json:"page_size"`
	ResourceURI string `json:"resource_uri"`
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
