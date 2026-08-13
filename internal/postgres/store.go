package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const catalogReadinessQuery = `select ready, status from discogs_catalog_readiness`

var ErrCatalogNotReady = errors.New("catalog is not ready")

type Store struct {
	pool         *pgxpool.Pool
	serverURL    string
	queryTimeout time.Duration
}

func New(pool *pgxpool.Pool, serverURL string, queryTimeout time.Duration) *Store {
	return &Store{
		pool:         pool,
		serverURL:    strings.TrimRight(serverURL, "/"),
		queryTimeout: queryTimeout,
	}
}

func (s *Store) timeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, s.queryTimeout)
}

// Ready reports serving readiness only after canonical bootstrap finalization succeeds.
func (s *Store) Ready(ctx context.Context) error {
	queryContext, cancel := s.timeout(ctx)
	defer cancel()
	var ready bool
	var status string
	if err := s.pool.QueryRow(queryContext, catalogReadinessQuery).Scan(&ready, &status); err != nil {
		return fmt.Errorf("read catalog readiness: %w", err)
	}
	if !ready {
		return fmt.Errorf("%w: %s", ErrCatalogNotReady, status)
	}
	return nil
}

type itemLoader[T catalog.PageItem] func(context.Context) ([]T, error)

func loadPage[T catalog.PageItem](
	ctx context.Context,
	requestedSize int,
	loadItems itemLoader[T],
) (catalog.Page[T], error) {
	items, err := loadItems(ctx)
	if err != nil {
		return catalog.Page[T]{}, err
	}
	return catalog.NewPage(items, requestedSize), nil
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return catalog.ErrNotFound
	}
	return err
}
