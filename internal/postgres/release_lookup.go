package postgres

import (
	"context"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
	"github.com/jackc/pgx/v5"
)

const (
	releasesByCatalogNumberSQL = `WITH matching_release AS (
  SELECT release_item_id
  FROM label_release_item
  WHERE label_id = $1
    AND category_notation = $2
    AND release_item_id > $3
  ORDER BY release_item_id
  LIMIT $4
)
SELECT ` + releaseColumnsSQL + `
FROM matching_release matching
JOIN release_item r ON r.id = matching.release_item_id
ORDER BY r.id`

	releasesByIdentifierSQL = `WITH matching_release AS (
  SELECT DISTINCT release_item_id
  FROM release_item_identifier
  WHERE lower(type) = lower($1)
    AND decode(md5(value), 'hex') = decode(md5($2), 'hex')
    AND value = $2
    AND release_item_id > $3
  ORDER BY release_item_id
  LIMIT $4
)
SELECT ` + releaseColumnsSQL + `
FROM matching_release matching
JOIN release_item r ON r.id = matching.release_item_id
ORDER BY r.id`

	queryReleasesByCatalogNumberError = "query releases by catalog number"
	queryReleasesByIdentifierError    = "query releases by identifier"
)

func (s *Store) ReleasesByCatalogNumber(
	ctx context.Context,
	lookup catalog.CatalogNumberLookup,
	request catalog.PageRequest,
) (catalog.Page[catalog.Release], error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	return loadPage(ctx, request.Size, func(loadContext context.Context) ([]catalog.Release, error) {
		rows, err := s.pool.Query(
			loadContext,
			releasesByCatalogNumberSQL,
			pgx.QueryExecModeExec,
			lookup.LabelID,
			lookup.CatalogNumber,
			request.AfterID,
			request.FetchSize(),
		)
		if err != nil {
			return nil, operationError(queryReleasesByCatalogNumberError, err)
		}
		return collectRows(rows, queryReleasesByCatalogNumberError, scanRelease)
	})
}

func (s *Store) ReleasesByIdentifier(
	ctx context.Context,
	lookup catalog.IdentifierLookup,
	request catalog.PageRequest,
) (catalog.Page[catalog.Release], error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	return loadPage(ctx, request.Size, func(loadContext context.Context) ([]catalog.Release, error) {
		rows, err := s.pool.Query(
			loadContext,
			releasesByIdentifierSQL,
			pgx.QueryExecModeExec,
			lookup.Type,
			lookup.Value,
			request.AfterID,
			request.FetchSize(),
		)
		if err != nil {
			return nil, operationError(queryReleasesByIdentifierError, err)
		}
		return collectRows(rows, queryReleasesByIdentifierError, scanRelease)
	})
}
