package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
)

const (
	identifierField = catalog.FieldID

	searchArtistsBaseSQL = `WITH params AS (
  SELECT $1::text AS name, $2::text AS real_name, $3::text AS profile
)
SELECT a.id::bigint, a.name, a.real_name, a.profile, a.data_quality
FROM artist a
CROSS JOIN params p
WHERE (p.name IS NULL OR a.name ILIKE '%' || p.name || '%')
  AND (p.real_name IS NULL OR a.real_name ILIKE '%' || p.real_name || '%')
  AND (p.profile IS NULL OR a.profile ILIKE '%' || p.profile || '%')`
	searchArtistsPageSQL = ` LIMIT $4 OFFSET $5`
	countArtistsSQL      = `WITH params AS (
  SELECT $1::text AS name, $2::text AS real_name, $3::text AS profile
)
SELECT count(*)::bigint
FROM artist a
CROSS JOIN params p
WHERE (p.name IS NULL OR a.name ILIKE '%' || p.name || '%')
  AND (p.real_name IS NULL OR a.real_name ILIKE '%' || p.real_name || '%')
  AND (p.profile IS NULL OR a.profile ILIKE '%' || p.profile || '%')`

	searchLabelsBaseSQL = `WITH params AS (
  SELECT $1::text AS contact_info, $2::text AS data_quality, $3::text AS name, $4::text AS profile
)
SELECT l.id::bigint, l.contact_info, l.data_quality, l.name, l.profile
FROM label l
CROSS JOIN params p
WHERE (p.contact_info IS NULL OR l.contact_info ILIKE '%' || p.contact_info || '%')
  AND (p.data_quality IS NULL OR l.data_quality ILIKE '%' || p.data_quality || '%')
  AND (p.name IS NULL OR l.name ILIKE '%' || p.name || '%')
  AND (p.profile IS NULL OR l.profile ILIKE '%' || p.profile || '%')`
	searchLabelsPageSQL = ` LIMIT $5 OFFSET $6`
	countLabelsSQL      = `WITH params AS (
  SELECT $1::text AS contact_info, $2::text AS data_quality, $3::text AS name, $4::text AS profile
)
SELECT count(*)::bigint
FROM label l
CROSS JOIN params p
WHERE (p.contact_info IS NULL OR l.contact_info ILIKE '%' || p.contact_info || '%')
  AND (p.data_quality IS NULL OR l.data_quality ILIKE '%' || p.data_quality || '%')
  AND (p.name IS NULL OR l.name ILIKE '%' || p.name || '%')
  AND (p.profile IS NULL OR l.profile ILIKE '%' || p.profile || '%')`

	searchMastersBaseSQL = `WITH params AS (
  SELECT $1::text AS title, $2::integer AS release_year
)
SELECT m.id::bigint, m.data_quality, m.title, m.year::integer
FROM master m
CROSS JOIN params p
WHERE (p.title IS NULL OR m.title ILIKE '%' || p.title || '%')
  AND (p.release_year IS NULL OR m.year = p.release_year)`
	searchMastersPageSQL = ` LIMIT $3 OFFSET $4`
	countMastersSQL      = `WITH params AS (
  SELECT $1::text AS title, $2::integer AS release_year
)
SELECT count(*)::bigint
FROM master m
CROSS JOIN params p
WHERE (p.title IS NULL OR m.title ILIKE '%' || p.title || '%')
  AND (p.release_year IS NULL OR m.year = p.release_year)`

	searchReleasesBaseSQL = `WITH params AS (
  SELECT $1::text AS title, $2::text AS country, $3::date AS start_date,
         $4::date AS end_date, $5::integer AS release_month, $6::boolean AS is_master
)
SELECT r.id::bigint, r.title, r.country, r.data_quality,
  CASE WHEN r.has_valid_year THEN extract(year FROM r.release_date)::integer END,
  CASE WHEN r.has_valid_month THEN extract(month FROM r.release_date)::integer END,
  CASE WHEN r.has_valid_day THEN extract(day FROM r.release_date)::integer END,
  r.listed_release_date, r.is_master, r.master_id::bigint, r.notes, r.status
FROM release_item r
CROSS JOIN params p
WHERE (p.title IS NULL OR r.title ILIKE '%' || p.title || '%')
  AND (p.country IS NULL OR lower(r.country) = lower(p.country))
  AND (p.start_date IS NULL OR (r.has_valid_year IS TRUE AND r.release_date >= p.start_date AND r.release_date < p.end_date))
  AND (p.release_month IS NULL OR (r.has_valid_month IS TRUE AND extract(month FROM r.release_date)::integer = p.release_month))
  AND (p.is_master IS NULL OR r.is_master = p.is_master)`
	searchReleasesPageSQL = ` LIMIT $7 OFFSET $8`
	countReleasesSQL      = `WITH params AS (
  SELECT $1::text AS title, $2::text AS country, $3::date AS start_date,
         $4::date AS end_date, $5::integer AS release_month, $6::boolean AS is_master
)
SELECT count(*)::bigint
FROM release_item r
CROSS JOIN params p
WHERE (p.title IS NULL OR r.title ILIKE '%' || p.title || '%')
  AND (p.country IS NULL OR lower(r.country) = lower(p.country))
  AND (p.start_date IS NULL OR (r.has_valid_year IS TRUE AND r.release_date >= p.start_date AND r.release_date < p.end_date))
  AND (p.release_month IS NULL OR (r.has_valid_month IS TRUE AND extract(month FROM r.release_date)::integer = p.release_month))
  AND (p.is_master IS NULL OR r.is_master = p.is_master)`

	queryArtistsError  = "query artists"
	countArtistsError  = "count artists"
	queryLabelsError   = "query labels"
	countLabelsError   = "count labels"
	queryMastersError  = "query masters"
	countMastersError  = "count masters"
	queryReleasesError = "query releases"
	countReleasesError = "count releases"
)

var artistSortFields = map[string]string{
	catalog.FieldID: "a.id", catalog.FieldName: "a.name", catalog.FieldRealName: "a.real_name", catalog.FieldProfile: "a.profile",
}

var labelSortFields = map[string]string{
	catalog.FieldID: "l.id", catalog.FieldContactInfo: "l.contact_info", catalog.FieldDataQuality: "l.data_quality", catalog.FieldName: "l.name", catalog.FieldProfile: "l.profile",
}

var masterSortFields = map[string]string{
	catalog.FieldID: "m.id", catalog.FieldTitle: "m.title", catalog.FieldReleasedYear: "m.year",
}

var releaseSortFields = map[string]string{
	catalog.FieldID: "r.id", catalog.FieldTitle: "r.title", catalog.FieldCountry: "r.country", catalog.FieldReleasedYear: "r.release_date", catalog.FieldReleasedMonth: "r.release_date",
}

func (s *Store) SearchArtists(ctx context.Context, filter catalog.ArtistFilter, request catalog.PageRequest) (catalog.Page[catalog.Artist], error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	name := optionalText(filter.Name)
	realName := optionalText(filter.RealName)
	profile := optionalText(filter.Profile)
	itemsQuery := searchArtistsBaseSQL + orderBy(request, artistSortFields, "a.id") + searchArtistsPageSQL
	key := fmt.Sprintf("artists|%q|%q|%q", filter.Name, filter.RealName, filter.Profile)
	return loadPage(ctx, s, key, func(loadContext context.Context) ([]catalog.Artist, error) {
		rows, err := s.pool.Query(loadContext, itemsQuery, name, realName, profile, request.Size, request.Offset())
		if err != nil {
			return nil, operationError(queryArtistsError, err)
		}
		return collectRows(rows, queryArtistsError, scanArtist)
	}, func(loadContext context.Context) (int64, error) {
		var total int64
		err := s.pool.QueryRow(loadContext, countArtistsSQL, name, realName, profile).Scan(&total)
		return total, operationError(countArtistsError, err)
	})
}

func (s *Store) SearchLabels(ctx context.Context, filter catalog.LabelFilter, request catalog.PageRequest) (catalog.Page[catalog.Label], error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	contactInfo := optionalText(filter.ContactInfo)
	dataQuality := optionalText(filter.DataQuality)
	name := optionalText(filter.Name)
	profile := optionalText(filter.Profile)
	itemsQuery := searchLabelsBaseSQL + orderBy(request, labelSortFields, "l.id") + searchLabelsPageSQL
	key := fmt.Sprintf("labels|%q|%q|%q|%q", filter.ContactInfo, filter.DataQuality, filter.Name, filter.Profile)
	return loadPage(ctx, s, key, func(loadContext context.Context) ([]catalog.Label, error) {
		rows, err := s.pool.Query(loadContext, itemsQuery, contactInfo, dataQuality, name, profile, request.Size, request.Offset())
		if err != nil {
			return nil, operationError(queryLabelsError, err)
		}
		return collectRows(rows, queryLabelsError, scanLabel)
	}, func(loadContext context.Context) (int64, error) {
		var total int64
		err := s.pool.QueryRow(loadContext, countLabelsSQL, contactInfo, dataQuality, name, profile).Scan(&total)
		return total, operationError(countLabelsError, err)
	})
}

func (s *Store) SearchMasters(ctx context.Context, filter catalog.MasterFilter, request catalog.PageRequest) (catalog.Page[catalog.Master], error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	title := optionalText(filter.Title)
	itemsQuery := searchMastersBaseSQL + orderBy(request, masterSortFields, "m.id") + searchMastersPageSQL
	key := fmt.Sprintf("masters|%q|%v", filter.Title, filter.Year)
	return loadPage(ctx, s, key, func(loadContext context.Context) ([]catalog.Master, error) {
		rows, err := s.pool.Query(loadContext, itemsQuery, title, filter.Year, request.Size, request.Offset())
		if err != nil {
			return nil, operationError(queryMastersError, err)
		}
		return collectRows(rows, queryMastersError, scanMaster)
	}, func(loadContext context.Context) (int64, error) {
		var total int64
		err := s.pool.QueryRow(loadContext, countMastersSQL, title, filter.Year).Scan(&total)
		return total, operationError(countMastersError, err)
	})
}

func (s *Store) SearchReleases(ctx context.Context, filter catalog.ReleaseFilter, request catalog.PageRequest) (catalog.Page[catalog.Release], error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	title := optionalText(filter.Title)
	country := optionalText(filter.Country)
	start, end, month := releaseDateParameters(filter.Year, filter.Month)
	itemsQuery := searchReleasesBaseSQL + orderBy(request, releaseSortFields, "r.id") + searchReleasesPageSQL
	key := fmt.Sprintf("releases|%q|%q|%v|%v|%v", filter.Title, filter.Country, filter.Year, filter.Month, filter.Master)
	return loadPage(ctx, s, key, func(loadContext context.Context) ([]catalog.Release, error) {
		rows, err := s.pool.Query(loadContext, itemsQuery, title, country, start, end, month, filter.Master, request.Size, request.Offset())
		if err != nil {
			return nil, operationError(queryReleasesError, err)
		}
		return collectRows(rows, queryReleasesError, scanRelease)
	}, func(loadContext context.Context) (int64, error) {
		var total int64
		err := s.pool.QueryRow(loadContext, countReleasesSQL, title, country, start, end, month, filter.Master).Scan(&total)
		return total, operationError(countReleasesError, err)
	})
}

func releaseDateParameters(year, month *int) (*time.Time, *time.Time, *int) {
	if year == nil {
		return nil, nil, month
	}
	selectedMonth := time.January
	if month != nil {
		selectedMonth = time.Month(*month)
	}
	start := time.Date(*year, selectedMonth, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(1, 0, 0)
	if month != nil {
		end = start.AddDate(0, 1, 0)
	}
	return &start, &end, nil
}

func optionalText(raw string) *string {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return nil
	}
	return &normalized
}
