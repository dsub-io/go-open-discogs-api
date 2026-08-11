package postgres

import (
	"context"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
	"github.com/jackc/pgx/v5"
)

const (
	artistReleasesSQL = `WITH related AS (
  SELECT release_item_id, 'Main'::text AS role
  FROM release_item_artist
  WHERE artist_id = $1
  UNION ALL
  SELECT release_item_id, role
  FROM release_item_credited_artist
  WHERE artist_id = $1
), aggregated AS (
  SELECT release_item_id,
         string_agg(DISTINCT trim(role), ',' ORDER BY trim(role)) AS role
  FROM related
  GROUP BY release_item_id
)
SELECT r.id::bigint, aggregated.role, r.title, r.country, r.data_quality,
  CASE WHEN r.has_valid_year THEN extract(year FROM r.release_date)::integer END,
  CASE WHEN r.has_valid_month THEN extract(month FROM r.release_date)::integer END,
  CASE WHEN r.has_valid_day THEN extract(day FROM r.release_date)::integer END,
  r.listed_release_date, r.is_master, r.master_id::bigint, r.notes, r.status
FROM aggregated
JOIN release_item r ON r.id = aggregated.release_item_id
WHERE r.id > $2
ORDER BY r.id
LIMIT $3`

	labelReleasesSQL = `SELECT r.id::bigint,
  string_agg(DISTINCT a.name, ',' ORDER BY a.name) AS artist,
  r.title,
  CASE WHEN r.has_valid_year THEN extract(year FROM r.release_date)::integer END AS year,
  r.status,
  relation.category_notation,
  string_agg(DISTINCT format.description, ',' ORDER BY format.description) AS format
FROM label_release_item relation
JOIN release_item r ON r.id = relation.release_item_id
LEFT JOIN release_item_artist release_artist ON r.id = release_artist.release_item_id
LEFT JOIN artist a ON a.id = release_artist.artist_id
LEFT JOIN release_item_format format ON r.id = format.release_item_id
WHERE relation.label_id = $1
  AND r.id > $2
GROUP BY r.id, relation.category_notation
ORDER BY r.id
LIMIT $3`

	masterReleasesSQL = `SELECT r.id::bigint, r.title,
  array_agg(a.name ORDER BY a.id),
  array_agg(a.id::bigint ORDER BY a.id),
  CASE WHEN r.has_valid_year THEN extract(year FROM r.release_date)::integer END
FROM release_item r
JOIN release_item_artist release_artist ON r.id = release_artist.release_item_id
JOIN artist a ON a.id = release_artist.artist_id
WHERE r.master_id = $1
  AND r.id > $2
GROUP BY r.id
ORDER BY r.id
LIMIT $3`

	queryArtistReleasesError = "query artist releases"
	queryLabelReleasesError  = "query label releases"
	queryMasterReleasesError = "query master releases"
)

func (s *Store) ArtistReleases(ctx context.Context, id int64, request catalog.PageRequest) (catalog.Page[catalog.ArtistRelease], error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	return loadPage(ctx, request.Size, func(loadContext context.Context) ([]catalog.ArtistRelease, error) {
		rows, err := s.pool.Query(
			loadContext,
			artistReleasesSQL,
			pgx.QueryExecModeExec,
			id,
			request.AfterID,
			request.FetchSize(),
		)
		if err != nil {
			return nil, operationError(queryArtistReleasesError, err)
		}
		return collectRows(rows, queryArtistReleasesError, s.scanArtistRelease)
	})
}

func (s *Store) LabelReleases(ctx context.Context, id int64, request catalog.PageRequest) (catalog.Page[catalog.LabelRelease], error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	return loadPage(ctx, request.Size, func(loadContext context.Context) ([]catalog.LabelRelease, error) {
		rows, err := s.pool.Query(
			loadContext,
			labelReleasesSQL,
			pgx.QueryExecModeExec,
			id,
			request.AfterID,
			request.FetchSize(),
		)
		if err != nil {
			return nil, operationError(queryLabelReleasesError, err)
		}
		return collectRows(rows, queryLabelReleasesError, scanLabelRelease)
	})
}

func (s *Store) MasterReleases(ctx context.Context, id int64, request catalog.PageRequest) (catalog.Page[catalog.MasterRelease], error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	return loadPage(ctx, request.Size, func(loadContext context.Context) ([]catalog.MasterRelease, error) {
		rows, err := s.pool.Query(
			loadContext,
			masterReleasesSQL,
			pgx.QueryExecModeExec,
			id,
			request.AfterID,
			request.FetchSize(),
		)
		if err != nil {
			return nil, operationError(queryMasterReleasesError, err)
		}
		return collectRows(rows, queryMasterReleasesError, s.scanMasterRelease)
	})
}
