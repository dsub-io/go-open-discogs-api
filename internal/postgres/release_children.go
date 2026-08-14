package postgres

import (
	"context"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
	"github.com/jackc/pgx/v5"
)

const (
	releaseTracksSQL = `SELECT hash, duration, position, title
FROM release_item_track
WHERE release_item_id = $1
  AND ($2::integer IS NULL OR hash > $2)
ORDER BY hash
LIMIT $3`

	releaseIdentifiersSQL = `SELECT hash, description, type, value
FROM release_item_identifier
WHERE release_item_id = $1
  AND ($2::integer IS NULL OR hash > $2)
ORDER BY hash
LIMIT $3`

	releaseExistsSQL = `SELECT EXISTS (
  SELECT 1
  FROM release_item
  WHERE id = $1
)`

	queryReleaseTracksError      = "query release tracks"
	queryReleaseIdentifiersError = "query release identifiers"
	queryReleaseExistenceError   = "query release existence"
)

func (s *Store) ReleaseTracks(
	ctx context.Context,
	releaseID int64,
	request catalog.HashPageRequest,
) (catalog.HashPage[catalog.ReleaseTrack], error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	page, err := loadHashPage(ctx, request.Size, func(loadContext context.Context) ([]catalog.ReleaseTrack, error) {
		rows, err := s.pool.Query(
			loadContext,
			releaseTracksSQL,
			pgx.QueryExecModeExec,
			releaseID,
			request.AfterHash,
			request.FetchSize(),
		)
		if err != nil {
			return nil, operationError(queryReleaseTracksError, err)
		}
		return collectHashRows(rows, queryReleaseTracksError, scanReleaseTrack)
	})
	if err != nil || len(page.Items) > 0 {
		return page, err
	}
	if err := s.requireRelease(ctx, releaseID); err != nil {
		return catalog.HashPage[catalog.ReleaseTrack]{}, err
	}
	return page, nil
}

func (s *Store) ReleaseIdentifiers(
	ctx context.Context,
	releaseID int64,
	request catalog.HashPageRequest,
) (catalog.HashPage[catalog.ReleaseIdentifier], error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	page, err := loadHashPage(ctx, request.Size, func(loadContext context.Context) ([]catalog.ReleaseIdentifier, error) {
		rows, err := s.pool.Query(
			loadContext,
			releaseIdentifiersSQL,
			pgx.QueryExecModeExec,
			releaseID,
			request.AfterHash,
			request.FetchSize(),
		)
		if err != nil {
			return nil, operationError(queryReleaseIdentifiersError, err)
		}
		return collectHashRows(rows, queryReleaseIdentifiersError, scanReleaseIdentifier)
	})
	if err != nil || len(page.Items) > 0 {
		return page, err
	}
	if err := s.requireRelease(ctx, releaseID); err != nil {
		return catalog.HashPage[catalog.ReleaseIdentifier]{}, err
	}
	return page, nil
}

func (s *Store) requireRelease(ctx context.Context, releaseID int64) error {
	var exists bool
	if err := s.pool.QueryRow(ctx, releaseExistsSQL, pgx.QueryExecModeExec, releaseID).Scan(&exists); err != nil {
		return operationError(queryReleaseExistenceError, err)
	}
	if !exists {
		return catalog.ErrNotFound
	}
	return nil
}
