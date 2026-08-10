package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
)

const (
	errorIDRange    = "id must be between 1 and %d"
	errorBoolean    = "%s must be true or false"
	errorSortField  = "unsupported sort field %q"
	errorSortOrder  = "sort direction must be asc or desc"
	errorSortFormat = "sort must use field,direction"
)

type SortFieldSet map[string]struct{}

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

func sortFields(names ...string) SortFieldSet {
	result := make(SortFieldSet, len(names))
	for _, name := range names {
		result[name] = struct{}{}
	}
	return result
}

func defaultIDSort() []catalog.Sort {
	return []catalog.Sort{{Field: catalog.FieldID, Direction: catalog.Ascending}}
}
