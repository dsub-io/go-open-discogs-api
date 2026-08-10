package postgres

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

const (
	countCacheTTL         = 5 * time.Minute
	countCacheEntries     = 1024
	defaultOrderDirection = "ASC"
	descendingDirection   = "DESC"
)

type Store struct {
	pool         *pgxpool.Pool
	serverURL    string
	queryTimeout time.Duration
	counts       *countCache
}

func New(pool *pgxpool.Pool, serverURL string, queryTimeout time.Duration) *Store {
	return &Store{
		pool:         pool,
		serverURL:    strings.TrimRight(serverURL, "/"),
		queryTimeout: queryTimeout,
		counts:       newCountCache(countCacheTTL, countCacheEntries),
	}
}

func (s *Store) timeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, s.queryTimeout)
}

type itemLoader[T catalog.PageItem] func(context.Context) ([]T, error)
type countLoader func(context.Context) (int64, error)

func loadPage[T catalog.PageItem](
	ctx context.Context,
	store *Store,
	cacheKey string,
	loadItems itemLoader[T],
	loadCount countLoader,
) (catalog.Page[T], error) {
	items := make([]T, 0)
	var total int64
	group, groupContext := errgroup.WithContext(ctx)
	group.Go(func() error {
		loaded, err := loadItems(groupContext)
		if err != nil {
			return err
		}
		items = loaded
		return nil
	})

	if cached, ok := store.counts.Get(cacheKey); ok {
		total = cached
	} else {
		group.Go(func() error {
			loaded, err := loadCount(groupContext)
			if err != nil {
				return err
			}
			total = loaded
			store.counts.Put(cacheKey, total)
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return catalog.Page[T]{}, err
	}
	return catalog.Page[T]{Items: items, Total: total}, nil
}

func orderBy(request catalog.PageRequest, allowed map[string]string, defaultField string) string {
	parts := make([]string, 0, len(request.Sort)+1)
	hasID := false
	for _, sort := range request.Sort {
		column, ok := allowed[sort.Field]
		if !ok {
			continue
		}
		direction := defaultOrderDirection
		if sort.Direction == catalog.Descending {
			direction = descendingDirection
		}
		parts = append(parts, column+" "+direction)
		if sort.Field == identifierField {
			hasID = true
		}
	}
	if len(parts) == 0 {
		parts = append(parts, defaultField+" "+defaultOrderDirection)
		hasID = true
	}
	if !hasID {
		parts = append(parts, defaultField+" "+defaultOrderDirection)
	}
	return " ORDER BY " + strings.Join(parts, ", ")
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return catalog.ErrNotFound
	}
	return err
}

type countCache struct {
	mu         sync.Mutex
	ttl        time.Duration
	maxEntries int
	values     map[string]countEntry
}

type countEntry struct {
	value     int64
	expiresAt time.Time
}

func newCountCache(ttl time.Duration, maxEntries int) *countCache {
	return &countCache{ttl: ttl, maxEntries: maxEntries, values: make(map[string]countEntry)}
}

func (c *countCache) Get(key string) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.values[key]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(c.values, key)
		return 0, false
	}
	return entry.value, true
}

func (c *countCache) Put(key string, value int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if len(c.values) >= c.maxEntries {
		for existingKey, entry := range c.values {
			if now.After(entry.expiresAt) {
				delete(c.values, existingKey)
			}
		}
	}
	if len(c.values) >= c.maxEntries {
		for existingKey := range c.values {
			delete(c.values, existingKey)
			break
		}
	}
	c.values[key] = countEntry{value: value, expiresAt: now.Add(c.ttl)}
}
