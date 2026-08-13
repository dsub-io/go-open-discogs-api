package postgres

import (
	"context"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
	"github.com/jackc/pgx/v5"
)

const (
	artistReleasesSQL = `WITH matching_release AS (
  SELECT release_item_id
  FROM (
    (
      SELECT release_item_id
      FROM release_item_artist
      WHERE artist_id = $1
        AND release_item_id > $2
      ORDER BY release_item_id
      LIMIT $3
    )
    UNION
    (
      SELECT DISTINCT release_item_id
      FROM release_item_credited_artist
      WHERE artist_id = $1
        AND release_item_id > $2
      ORDER BY release_item_id
      LIMIT $3
    )
  ) candidate
  ORDER BY release_item_id
  LIMIT $3
), aggregated AS (
  SELECT matching.release_item_id,
         string_agg(DISTINCT trim(related.role), ',' ORDER BY trim(related.role)) AS role
  FROM matching_release matching
  CROSS JOIN LATERAL (
    SELECT 'Main'::text AS role
    FROM release_item_artist
    WHERE artist_id = $1
      AND release_item_id = matching.release_item_id
    UNION ALL
    SELECT role
    FROM release_item_credited_artist
    WHERE artist_id = $1
      AND release_item_id = matching.release_item_id
  ) related
  GROUP BY matching.release_item_id
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

	labelReleasesSQL = `WITH matching_release AS (
  SELECT DISTINCT release_item_id
  FROM label_release_item
  WHERE label_id = $1
    AND release_item_id > $2
  ORDER BY release_item_id
  LIMIT $3
)
SELECT r.id::bigint,
  (
    SELECT string_agg(DISTINCT artist.name, ',' ORDER BY artist.name)
    FROM release_item_artist relation
    JOIN artist ON artist.id = relation.artist_id
    WHERE relation.release_item_id = r.id
  ) AS artist,
  r.title,
  CASE WHEN r.has_valid_year THEN extract(year FROM r.release_date)::integer END AS year,
  r.status,
  COALESCE(
    (
      SELECT array_agg(catalog_number ORDER BY catalog_number)
      FROM (
        SELECT relation.category_notation AS catalog_number
        FROM label_release_item relation
        WHERE relation.label_id = $1
          AND relation.release_item_id = r.id
          AND relation.category_notation IS NOT NULL
        GROUP BY relation.category_notation
      ) ordered_catalog_number
    ),
    ARRAY[]::text[]
  ) AS catalog_numbers,
  (
    SELECT string_agg(DISTINCT format.description, ',' ORDER BY format.description)
    FROM release_item_format format
    WHERE format.release_item_id = r.id
  ) AS format
FROM matching_release matching
JOIN release_item r ON r.id = matching.release_item_id
ORDER BY r.id
`

	masterReleasesSQL = `SELECT r.id::bigint, r.title,
  COALESCE(
    array_agg(a.name ORDER BY a.id) FILTER (WHERE a.id IS NOT NULL),
    ARRAY[]::text[]
  ),
  COALESCE(
    array_agg(a.id::bigint ORDER BY a.id) FILTER (WHERE a.id IS NOT NULL),
    ARRAY[]::bigint[]
  ),
  CASE WHEN r.has_valid_year THEN extract(year FROM r.release_date)::integer END
FROM release_item r
LEFT JOIN release_item_artist release_artist ON r.id = release_artist.release_item_id
LEFT JOIN artist a ON a.id = release_artist.artist_id
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
