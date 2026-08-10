package postgres

import (
	"context"
	"fmt"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
)

const (
	artistReleasesBaseSQL = `WITH related AS (
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
JOIN release_item r ON r.id = aggregated.release_item_id`
	artistReleasesPageSQL  = ` LIMIT $2 OFFSET $3`
	countArtistReleasesSQL = `WITH related AS (
  SELECT release_item_id FROM release_item_artist WHERE artist_id = $1
  UNION
  SELECT release_item_id FROM release_item_credited_artist WHERE artist_id = $1
)
SELECT count(*)::bigint FROM related`

	labelReleasesBaseSQL = `SELECT r.id::bigint,
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
GROUP BY r.id, relation.category_notation`
	labelReleasesPageSQL  = ` LIMIT $2 OFFSET $3`
	countLabelReleasesSQL = `SELECT count(DISTINCT release_item_id)::bigint FROM label_release_item WHERE label_id = $1`

	masterReleasesBaseSQL = `SELECT r.id::bigint, r.title,
  array_agg(a.name ORDER BY a.id),
  array_agg(a.id::bigint ORDER BY a.id),
  CASE WHEN r.has_valid_year THEN extract(year FROM r.release_date)::integer END
FROM release_item r
JOIN release_item_artist release_artist ON r.id = release_artist.release_item_id
JOIN artist a ON a.id = release_artist.artist_id
WHERE r.master_id = $1
GROUP BY r.id`
	masterReleasesPageSQL  = ` LIMIT $2 OFFSET $3`
	countMasterReleasesSQL = `SELECT count(*)::bigint FROM release_item WHERE master_id = $1`

	queryArtistReleasesError = "query artist releases"
	countArtistReleasesError = "count artist releases"
	queryLabelReleasesError  = "query label releases"
	countLabelReleasesError  = "count label releases"
	queryMasterReleasesError = "query master releases"
	countMasterReleasesError = "count master releases"
)

var artistReleaseSortFields = map[string]string{
	catalog.FieldID: "r.id", catalog.FieldTitle: "r.title", catalog.FieldCountry: "r.country", catalog.FieldMasterID: "r.master_id",
	catalog.FieldReleasedYear: "r.release_date", catalog.FieldReleasedMonth: "r.release_date", catalog.FieldReleasedDay: "r.release_date",
}

var labelReleaseSortFields = map[string]string{
	catalog.FieldID: "r.id", catalog.FieldTitle: "r.title", catalog.FieldYear: "r.release_date",
}

var masterReleaseSortFields = map[string]string{
	catalog.FieldID: "r.id", catalog.FieldTitle: "r.title", catalog.FieldYear: "r.release_date",
}

func (s *Store) ArtistReleases(ctx context.Context, id int64, request catalog.PageRequest) (catalog.Page[catalog.ArtistRelease], error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	itemsQuery := artistReleasesBaseSQL + orderBy(request, artistReleaseSortFields, "r.id") + artistReleasesPageSQL
	return loadPage(ctx, s, fmt.Sprintf("artist-releases|%d", id), func(loadContext context.Context) ([]catalog.ArtistRelease, error) {
		rows, err := s.pool.Query(loadContext, itemsQuery, id, request.Size, request.Offset())
		if err != nil {
			return nil, operationError(queryArtistReleasesError, err)
		}
		return collectRows(rows, queryArtistReleasesError, s.scanArtistRelease)
	}, func(loadContext context.Context) (int64, error) {
		var total int64
		err := s.pool.QueryRow(loadContext, countArtistReleasesSQL, id).Scan(&total)
		return total, operationError(countArtistReleasesError, err)
	})
}

func (s *Store) LabelReleases(ctx context.Context, id int64, request catalog.PageRequest) (catalog.Page[catalog.LabelRelease], error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	itemsQuery := labelReleasesBaseSQL + orderBy(request, labelReleaseSortFields, "r.id") + labelReleasesPageSQL
	return loadPage(ctx, s, fmt.Sprintf("label-releases|%d", id), func(loadContext context.Context) ([]catalog.LabelRelease, error) {
		rows, err := s.pool.Query(loadContext, itemsQuery, id, request.Size, request.Offset())
		if err != nil {
			return nil, operationError(queryLabelReleasesError, err)
		}
		return collectRows(rows, queryLabelReleasesError, scanLabelRelease)
	}, func(loadContext context.Context) (int64, error) {
		var total int64
		err := s.pool.QueryRow(loadContext, countLabelReleasesSQL, id).Scan(&total)
		return total, operationError(countLabelReleasesError, err)
	})
}

func (s *Store) MasterReleases(ctx context.Context, id int64, request catalog.PageRequest) (catalog.Page[catalog.MasterRelease], error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	itemsQuery := masterReleasesBaseSQL + orderBy(request, masterReleaseSortFields, "r.id") + masterReleasesPageSQL
	return loadPage(ctx, s, fmt.Sprintf("master-releases|%d", id), func(loadContext context.Context) ([]catalog.MasterRelease, error) {
		rows, err := s.pool.Query(loadContext, itemsQuery, id, request.Size, request.Offset())
		if err != nil {
			return nil, operationError(queryMasterReleasesError, err)
		}
		return collectRows(rows, queryMasterReleasesError, s.scanMasterRelease)
	}, func(loadContext context.Context) (int64, error) {
		var total int64
		err := s.pool.QueryRow(loadContext, countMasterReleasesSQL, id).Scan(&total)
		return total, operationError(countMasterReleasesError, err)
	})
}
