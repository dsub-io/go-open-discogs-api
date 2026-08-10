package httpapi

import (
	"fmt"
	"math"
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
	Items         []T    `json:"items"`
	TotalElements int64  `json:"total_elements"`
	TotalPages    int    `json:"total_pages"`
	PageNumber    int    `json:"page_number"`
	PageSize      int    `json:"page_size"`
	Last          bool   `json:"last"`
	First         bool   `json:"first"`
	Sorted        bool   `json:"sorted"`
	ResourceURI   string `json:"resource_uri"`
}

func pageResponse[T catalog.PageItem](page catalog.Page[T], request catalog.PageRequest, resourceURI string) PageResponse[T] {
	totalPages := 0
	if page.Total > 0 {
		totalPages = int((page.Total + int64(request.Size) - 1) / int64(request.Size))
	}
	pageNumber := request.Page
	if totalPages == 0 {
		pageNumber = 0
	}
	items := page.Items
	if items == nil {
		items = make([]T, 0)
	}
	return PageResponse[T]{
		Items:         items,
		TotalElements: page.Total,
		TotalPages:    totalPages,
		PageNumber:    pageNumber,
		PageSize:      len(items),
		Last:          totalPages == 0 || request.Page >= totalPages,
		First:         request.Page == 1,
		Sorted:        len(request.Sort) > 0,
		ResourceURI:   resourceURI,
	}
}

func parsePage(values url.Values, allowed map[string]struct{}, defaults []catalog.Sort) (catalog.PageRequest, error) {
	page, err := parseBoundedInt(values.Get(ParameterPage), 1, 1, math.MaxInt32, ParameterPage)
	if err != nil {
		return catalog.PageRequest{}, err
	}
	size, err := parseBoundedInt(values.Get(ParameterSize), defaultPageSize, 1, math.MaxInt32, ParameterSize)
	if err != nil {
		return catalog.PageRequest{}, err
	}
	if size > maximumPageSize {
		size = maximumPageSize
	}

	sorts := make([]catalog.Sort, 0, len(values[ParameterSort]))
	for _, raw := range values[ParameterSort] {
		parts := strings.Split(raw, ",")
		field := strings.TrimSpace(parts[0])
		if _, ok := allowed[field]; !ok {
			return catalog.PageRequest{}, fmt.Errorf(errorSortField, field)
		}
		direction := catalog.Ascending
		if len(parts) > 1 {
			switch strings.ToLower(strings.TrimSpace(parts[1])) {
			case "asc":
			case "desc":
				direction = catalog.Descending
			default:
				return catalog.PageRequest{}, fmt.Errorf(errorSortOrder)
			}
		}
		if len(parts) > 2 {
			return catalog.PageRequest{}, fmt.Errorf(errorSortFormat)
		}
		sorts = append(sorts, catalog.Sort{Field: field, Direction: direction})
	}
	if len(sorts) == 0 {
		sorts = append(sorts, defaults...)
	}
	return catalog.PageRequest{Page: page, Size: size, Sort: sorts}, nil
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
