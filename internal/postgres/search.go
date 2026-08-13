package postgres

import (
	"context"
	"strings"
	"time"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
	"github.com/jackc/pgx/v5"
)

const (
	releaseColumnsSQL = `r.id::bigint, r.title, r.country, r.data_quality,
  CASE WHEN r.has_valid_year THEN extract(year FROM r.release_date)::integer END,
  CASE WHEN r.has_valid_month THEN extract(month FROM r.release_date)::integer END,
  CASE WHEN r.has_valid_day THEN extract(day FROM r.release_date)::integer END,
  r.listed_release_date, r.is_master, r.master_id::bigint, r.notes, r.status`

	searchArtistsSQL = `WITH params AS (
  SELECT $1::text AS name, $2::text AS real_name
)
SELECT a.id::bigint, a.name, a.real_name, a.profile, a.data_quality
FROM artist a
CROSS JOIN params p
WHERE (p.name IS NULL OR a.name ILIKE '%' || p.name || '%')
  AND (p.real_name IS NULL OR a.real_name ILIKE '%' || p.real_name || '%')
  AND a.id > $3
ORDER BY a.id
LIMIT $4`

	searchLabelsSQL = `WITH params AS (
  SELECT $1::text AS name
)
SELECT l.id::bigint, l.contact_info, l.data_quality, l.name, l.profile
FROM label l
CROSS JOIN params p
WHERE (p.name IS NULL OR l.name ILIKE '%' || p.name || '%')
  AND l.id > $2
ORDER BY l.id
LIMIT $3`

	searchMastersSQL = `WITH params AS (
  SELECT $1::text AS title, $2::integer AS release_year
)
SELECT m.id::bigint, m.data_quality, m.title, m.year::integer
FROM master m
CROSS JOIN params p
WHERE (p.title IS NULL OR m.title ILIKE '%' || p.title || '%')
  AND (p.release_year IS NULL OR m.year = p.release_year)
  AND m.id > $3
ORDER BY m.id
LIMIT $4`

	searchReleasesSQL = `WITH params AS (
  SELECT $1::text AS title, $2::text AS country, $3::date AS start_date,
         $4::date AS end_date, $5::integer AS release_month, $6::boolean AS is_master
)
SELECT ` + releaseColumnsSQL + `
FROM release_item r
CROSS JOIN params p
WHERE (p.title IS NULL OR r.title ILIKE '%' || p.title || '%')
  AND (p.country IS NULL OR lower(r.country) = lower(p.country))
  AND (p.start_date IS NULL OR (r.has_valid_year IS TRUE AND r.release_date >= p.start_date AND r.release_date < p.end_date))
  AND (p.release_month IS NULL OR (r.has_valid_month IS TRUE AND extract(month FROM r.release_date)::integer = p.release_month))
  AND (p.is_master IS NULL OR r.is_master = p.is_master)
  AND r.id > $7
ORDER BY r.id
LIMIT $8`

	queryArtistsError  = "query artists"
	queryLabelsError   = "query labels"
	queryMastersError  = "query masters"
	queryReleasesError = "query releases"
)

func (s *Store) SearchArtists(ctx context.Context, filter catalog.ArtistFilter, request catalog.PageRequest) (catalog.Page[catalog.Artist], error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	name := optionalText(filter.Name)
	realName := optionalText(filter.RealName)
	return loadPage(ctx, request.Size, func(loadContext context.Context) ([]catalog.Artist, error) {
		rows, err := s.pool.Query(
			loadContext,
			searchArtistsSQL,
			pgx.QueryExecModeExec,
			name,
			realName,
			request.AfterID,
			request.FetchSize(),
		)
		if err != nil {
			return nil, operationError(queryArtistsError, err)
		}
		return collectRows(rows, queryArtistsError, scanArtist)
	})
}

func (s *Store) SearchLabels(ctx context.Context, filter catalog.LabelFilter, request catalog.PageRequest) (catalog.Page[catalog.Label], error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	name := optionalText(filter.Name)
	return loadPage(ctx, request.Size, func(loadContext context.Context) ([]catalog.Label, error) {
		rows, err := s.pool.Query(
			loadContext,
			searchLabelsSQL,
			pgx.QueryExecModeExec,
			name,
			request.AfterID,
			request.FetchSize(),
		)
		if err != nil {
			return nil, operationError(queryLabelsError, err)
		}
		return collectRows(rows, queryLabelsError, scanLabel)
	})
}

func (s *Store) SearchMasters(ctx context.Context, filter catalog.MasterFilter, request catalog.PageRequest) (catalog.Page[catalog.Master], error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	title := optionalText(filter.Title)
	return loadPage(ctx, request.Size, func(loadContext context.Context) ([]catalog.Master, error) {
		rows, err := s.pool.Query(
			loadContext,
			searchMastersSQL,
			pgx.QueryExecModeExec,
			title,
			filter.Year,
			request.AfterID,
			request.FetchSize(),
		)
		if err != nil {
			return nil, operationError(queryMastersError, err)
		}
		return collectRows(rows, queryMastersError, scanMaster)
	})
}

func (s *Store) SearchReleases(ctx context.Context, filter catalog.ReleaseFilter, request catalog.PageRequest) (catalog.Page[catalog.Release], error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	title := optionalText(filter.Title)
	country := optionalText(filter.Country)
	start, end, month := releaseDateParameters(filter.Year, filter.Month)
	return loadPage(ctx, request.Size, func(loadContext context.Context) ([]catalog.Release, error) {
		rows, err := s.pool.Query(
			loadContext,
			searchReleasesSQL,
			pgx.QueryExecModeExec,
			title,
			country,
			start,
			end,
			month,
			filter.Master,
			request.AfterID,
			request.FetchSize(),
		)
		if err != nil {
			return nil, operationError(queryReleasesError, err)
		}
		return collectRows(rows, queryReleasesError, scanRelease)
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
