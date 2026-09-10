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

// Ready checks connectivity after startup schema validation, independently of import progress.
func (s *Store) Ready(ctx context.Context) error {
	queryContext, cancel := s.timeout(ctx)
	defer cancel()
	if err := s.pool.Ping(queryContext); err != nil {
		return fmt.Errorf("check database connectivity: %w", err)
	}
	return nil
}

type itemLoader[T catalog.PageItem] func(context.Context) ([]T, error)

type hashItemLoader[T catalog.HashPageItem] func(context.Context) ([]T, error)

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

func loadHashPage[T catalog.HashPageItem](
	ctx context.Context,
	requestedSize int,
	loadItems hashItemLoader[T],
) (catalog.HashPage[T], error) {
	items, err := loadItems(ctx)
	if err != nil {
		return catalog.HashPage[T]{}, err
	}
	return catalog.NewHashPage(items, requestedSize), nil
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return catalog.ErrNotFound
	}
	return err
}
