package postgres

import (
	"context"
	"errors"
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
