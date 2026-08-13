package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
	"github.com/jackc/pgx/v5"
)

const (
	snapshotSQL = `SELECT readiness.ready, readiness.status, readiness.updated_at,
  state.entity_type, state.status,
  to_char(checkpoint.dump_date, 'YYYY-MM-DD'),
  checkpoint.applied_at, checkpoint.processor, checkpoint.processor_version
FROM discogs_catalog_readiness readiness
CROSS JOIN discogs_catalog_entity_state state
LEFT JOIN discogs_import_checkpoint checkpoint
  ON checkpoint.entity_type = state.entity_type
ORDER BY CASE state.entity_type
  WHEN 'artist' THEN 1
  WHEN 'label' THEN 2
  WHEN 'master' THEN 3
  WHEN 'release' THEN 4
END`

	querySnapshotError = "query catalog snapshot"
)

func (s *Store) Snapshot(ctx context.Context) (catalog.CatalogSnapshot, error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	rows, err := s.pool.Query(ctx, snapshotSQL, pgx.QueryExecModeExec)
	if err != nil {
		return catalog.CatalogSnapshot{}, operationError(querySnapshotError, err)
	}
	return collectSnapshot(rows)
}

type snapshotRow struct {
	ready     bool
	status    string
	updatedAt time.Time
	entity    catalog.SnapshotEntity
}

func collectSnapshot(rows pgx.Rows) (catalog.CatalogSnapshot, error) {
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (snapshotRow, error) {
		var item snapshotRow
		err := row.Scan(
			&item.ready,
			&item.status,
			&item.updatedAt,
			&item.entity.EntityType,
			&item.entity.Status,
			&item.entity.DumpDate,
			&item.entity.AppliedAt,
			&item.entity.Processor,
			&item.entity.ProcessorVersion,
		)
		return item, err
	})
	if err != nil {
		return catalog.CatalogSnapshot{}, operationError(querySnapshotError, err)
	}
	if len(items) == 0 {
		return catalog.CatalogSnapshot{}, fmt.Errorf("%s: canonical catalog state is empty", querySnapshotError)
	}
	snapshot := catalog.CatalogSnapshot{
		Ready:     items[0].ready,
		Status:    items[0].status,
		UpdatedAt: items[0].updatedAt,
		Entities:  make([]catalog.SnapshotEntity, 0, len(items)),
	}
	for _, item := range items {
		snapshot.Entities = append(snapshot.Entities, item.entity)
	}
	return snapshot, nil
}
