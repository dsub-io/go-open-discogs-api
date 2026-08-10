package postgres

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
	canonicalschema "github.com/dsub-io/open-discogs-model/schema"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	envTestDatabaseURL     = "TEST_DATABASE_URL"
	defaultTestDatabaseURL = "postgres://discogs:discogs@127.0.0.1:55432/discogs?sslmode=disable"
	testPublicURL          = "https://api.example.com"
	testQueryTimeout       = 5 * time.Second
)

var integrationPool *pgxpool.Pool

func TestMain(main *testing.M) {
	ctx := context.Background()
	databaseURL := os.Getenv(envTestDatabaseURL)
	if databaseURL == "" {
		databaseURL = defaultTestDatabaseURL
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		fmt.Fprintf(os.Stderr, "PostgreSQL integration database is required: %v\n", err)
		os.Exit(1)
	}
	if err := prepareDatabase(ctx, pool); err != nil {
		pool.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	integrationPool = pool
	code := main.Run()
	pool.Close()
	os.Exit(code)
}

func TestStoreSearchAndRelatedQueries(t *testing.T) {
	t.Parallel()
	store := New(integrationPool, testPublicURL+"/", testQueryTimeout)
	request := catalog.PageRequest{Page: 1, Size: 20, Sort: []catalog.Sort{{Field: catalog.FieldID, Direction: catalog.Ascending}}}

	artists, err := store.SearchArtists(context.Background(), catalog.ArtistFilter{Name: "alpha", RealName: "real", Profile: "profile"}, request)
	assertPage(t, artists.Total, len(artists.Items), err)
	artists, err = store.SearchArtists(context.Background(), catalog.ArtistFilter{Name: "alpha", RealName: "real", Profile: "profile"}, request)
	assertPage(t, artists.Total, len(artists.Items), err)

	labels, err := store.SearchLabels(context.Background(), catalog.LabelFilter{ContactInfo: "contact", DataQuality: "correct", Name: "label", Profile: "profile"}, request)
	assertPage(t, labels.Total, len(labels.Items), err)
	masters, err := store.SearchMasters(context.Background(), catalog.MasterFilter{Title: "master", Year: intPointer(2000)}, request)
	assertPage(t, masters.Total, len(masters.Items), err)
	releases, err := store.SearchReleases(context.Background(), catalog.ReleaseFilter{Title: "release", Country: "us", Year: intPointer(2000), Month: intPointer(2), Master: boolPointer(true)}, request)
	assertPage(t, releases.Total, len(releases.Items), err)

	artistReleases, err := store.ArtistReleases(context.Background(), 1, request)
	assertPage(t, artistReleases.Total, len(artistReleases.Items), err)
	if artistReleases.Items[0].ResourceURL != testPublicURL+"/releases/1" {
		t.Fatalf("artist resource URL=%s", artistReleases.Items[0].ResourceURL)
	}
	labelReleases, err := store.LabelReleases(context.Background(), 1, request)
	assertPage(t, labelReleases.Total, len(labelReleases.Items), err)
	masterReleases, err := store.MasterReleases(context.Background(), 1, request)
	assertPage(t, masterReleases.Total, len(masterReleases.Items), err)
}

func TestStoreDetailQueries(t *testing.T) {
	t.Parallel()
	store := New(integrationPool, testPublicURL, testQueryTimeout)
	artist, err := store.Artist(context.Background(), 1)
	if err != nil || artist.ID != 1 || len(artist.Members) != 1 || len(artist.Groups) != 1 || len(artist.Aliases) != 1 || len(artist.NameVariations) != 1 || len(artist.URLs) != 1 {
		t.Fatalf("artist=%+v err=%v", artist, err)
	}
	label, err := store.Label(context.Background(), 2)
	if err != nil || label.ParentLabel == nil || label.ParentLabel.ID != 1 || len(label.URLs) != 1 {
		t.Fatalf("label=%+v err=%v", label, err)
	}
	parent, err := store.Label(context.Background(), 1)
	if err != nil || parent.ParentLabel != nil || len(parent.Sublabels) != 1 {
		t.Fatalf("parent=%+v err=%v", parent, err)
	}
	master, err := store.Master(context.Background(), 1)
	if err != nil || master.ID != 1 || len(master.Genres) != 1 || len(master.Styles) != 1 || len(master.Artists) != 1 || len(master.Videos) != 1 {
		t.Fatalf("master=%+v err=%v", master, err)
	}
	release, err := store.Release(context.Background(), 1)
	if err != nil || release.ID != 1 || len(release.Artists) != 2 || len(release.Labels) != 1 || len(release.Companies) != 1 || len(release.Formats) != 1 || len(release.Styles) != 1 || len(release.Genres) != 1 || len(release.Videos) != 1 {
		t.Fatalf("release=%+v err=%v", release, err)
	}
}

func TestStoreNotFoundAndCancellation(t *testing.T) {
	t.Parallel()
	store := New(integrationPool, testPublicURL, testQueryTimeout)
	if _, err := store.Artist(context.Background(), 999); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("artist error=%v", err)
	}
	if _, err := store.Label(context.Background(), 999); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("label error=%v", err)
	}
	if _, err := store.Master(context.Background(), 999); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("master error=%v", err)
	}
	if _, err := store.Release(context.Background(), 999); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("release error=%v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.SearchArtists(ctx, catalog.ArtistFilter{}, catalog.PageRequest{Page: 1, Size: 1}); err == nil {
		t.Fatal("canceled search succeeded")
	}
}

func TestStoreHelpers(t *testing.T) {
	t.Parallel()
	request := catalog.PageRequest{Sort: []catalog.Sort{
		{Field: catalog.FieldTitle, Direction: catalog.Descending},
		{Field: "unsupported", Direction: catalog.Ascending},
	}}
	ordered := orderBy(request, map[string]string{catalog.FieldTitle: "r.title"}, "r.id")
	if ordered != " ORDER BY r.title DESC, r.id ASC" {
		t.Fatalf("order=%s", ordered)
	}
	if orderBy(catalog.PageRequest{}, map[string]string{}, "r.id") != " ORDER BY r.id ASC" {
		t.Fatal("default ordering changed")
	}
	if notFound(errRepositoryTest) != errRepositoryTest || !errors.Is(notFound(pgx.ErrNoRows), catalog.ErrNotFound) {
		t.Fatal("notFound mapping changed")
	}
	if optionalText("   ") != nil || *optionalText(" value ") != "value" {
		t.Fatal("optional text normalization changed")
	}
	start, end, month := releaseDateParameters(nil, intPointer(2))
	if start != nil || end != nil || month == nil || *month != 2 {
		t.Fatal("month-only parameters changed")
	}
	start, end, month = releaseDateParameters(intPointer(2000), intPointer(2))
	if start == nil || end == nil || month != nil || start.Year() != 2000 || start.Month() != 2 || end.Month() != 3 {
		t.Fatal("year-month parameters changed")
	}

	cache := newCountCache(time.Hour, 1)
	if _, ok := cache.Get("missing"); ok {
		t.Fatal("missing cache entry found")
	}
	cache.Put("first", 1)
	cache.Put("second", 2)
	if value, ok := cache.Get("second"); !ok || value != 2 {
		t.Fatalf("cache value=%d ok=%t", value, ok)
	}
	expired := newCountCache(-time.Second, 1)
	expired.Put("expired", 1)
	if _, ok := expired.Get("expired"); ok {
		t.Fatal("expired cache entry found")
	}
	partiallyExpired := newCountCache(time.Hour, 2)
	partiallyExpired.values["expired"] = countEntry{value: 1, expiresAt: time.Now().Add(-time.Second)}
	partiallyExpired.values["active"] = countEntry{value: 2, expiresAt: time.Now().Add(time.Hour)}
	partiallyExpired.Put("replacement", 3)
	if _, exists := partiallyExpired.values["expired"]; exists {
		t.Fatal("Put retained expired entry")
	}

	store := &Store{counts: newCountCache(time.Hour, 1)}
	page, err := loadPage(context.Background(), store, "key",
		func(context.Context) ([]catalog.Artist, error) { return []catalog.Artist{{ID: 1}}, nil },
		func(context.Context) (int64, error) { return 1, nil },
	)
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	_, err = loadPage(context.Background(), store, "cached",
		func(context.Context) ([]catalog.Artist, error) { return nil, errRepositoryTest },
		func(context.Context) (int64, error) { return 1, nil },
	)
	if !errors.Is(err, errRepositoryTest) {
		t.Fatalf("item loader error=%v", err)
	}
	_, err = loadPage(context.Background(), store, "count-error",
		func(context.Context) ([]catalog.Artist, error) { return nil, nil },
		func(context.Context) (int64, error) { return 0, errRepositoryTest },
	)
	if !errors.Is(err, errRepositoryTest) {
		t.Fatalf("count loader error=%v", err)
	}

	rows, err := integrationPool.Query(context.Background(), "SELECT 1::bigint")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := collectRows(rows, queryArtistsError, scanArtist); err == nil {
		t.Fatal("invalid row shape was accepted")
	}
}

func TestStoreQueryErrorsFromClosedPool(t *testing.T) {
	databaseURL := os.Getenv(envTestDatabaseURL)
	if databaseURL == "" {
		databaseURL = defaultTestDatabaseURL
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	store := New(pool, testPublicURL, testQueryTimeout)
	request := catalog.PageRequest{Page: 1, Size: 1, Sort: []catalog.Sort{{Field: catalog.FieldID, Direction: catalog.Ascending}}}
	tests := []struct {
		name string
		run  func() error
	}{
		{"artists", func() error {
			_, err := store.SearchArtists(context.Background(), catalog.ArtistFilter{}, request)
			return err
		}},
		{"labels", func() error {
			_, err := store.SearchLabels(context.Background(), catalog.LabelFilter{}, request)
			return err
		}},
		{"masters", func() error {
			_, err := store.SearchMasters(context.Background(), catalog.MasterFilter{}, request)
			return err
		}},
		{"releases", func() error {
			_, err := store.SearchReleases(context.Background(), catalog.ReleaseFilter{}, request)
			return err
		}},
		{"artist releases", func() error { _, err := store.ArtistReleases(context.Background(), 1, request); return err }},
		{"label releases", func() error { _, err := store.LabelReleases(context.Background(), 1, request); return err }},
		{"master releases", func() error { _, err := store.MasterReleases(context.Background(), 1, request); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); err == nil {
				t.Fatal("closed pool query succeeded")
			}
		})
	}
}

func TestDetailDecodersRejectMalformedJSON(t *testing.T) {
	invalid := []byte("not-json")
	emptyArray := []byte("[]")
	if _, err := artistDetailResult(catalog.ArtistDetail{}, artistDetailPayload{invalid, emptyArray, emptyArray, emptyArray, emptyArray}); err == nil {
		t.Fatal("invalid artist detail was accepted")
	}
	if _, err := labelDetailResult(catalog.LabelDetail{}, labelDetailPayload{invalid, emptyArray, emptyArray}); err == nil {
		t.Fatal("invalid label detail was accepted")
	}
	if _, err := masterDetailResult(catalog.MasterDetail{}, masterDetailPayload{invalid, emptyArray, emptyArray, emptyArray}); err == nil {
		t.Fatal("invalid master detail was accepted")
	}
	if _, err := releaseDetailResult(catalog.ReleaseDetail{}, releaseDetailPayload{invalid, emptyArray, emptyArray, emptyArray, emptyArray, emptyArray, emptyArray}); err == nil {
		t.Fatal("invalid release detail was accepted")
	}
}

var errRepositoryTest = errors.New("test repository failure")

func assertPage(t *testing.T, total int64, itemCount int, err error) {
	t.Helper()
	if err != nil || total != 1 || itemCount != 1 {
		t.Fatalf("total=%d items=%d err=%v", total, itemCount, err)
	}
}

func intPointer(value int) *int { return &value }

func boolPointer(value bool) *bool { return &value }

func prepareDatabase(ctx context.Context, pool *pgxpool.Pool) error {
	var schemaExists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.artist') IS NOT NULL").Scan(&schemaExists); err != nil {
		return err
	}
	if !schemaExists {
		migrations, err := canonicalschema.Migrations()
		if err != nil {
			return err
		}
		files, err := fs.ReadDir(migrations, ".")
		if err != nil {
			return err
		}
		for _, file := range files {
			migration, err := fs.ReadFile(migrations, file.Name())
			if err != nil {
				return err
			}
			if _, err := pool.Exec(ctx, string(migration)); err != nil {
				return fmt.Errorf("apply %s: %w", file.Name(), err)
			}
		}
	}
	if _, err := pool.Exec(ctx, resetSQL); err != nil {
		return fmt.Errorf("reset integration data: %w", err)
	}
	if _, err := pool.Exec(ctx, seedSQL); err != nil {
		return fmt.Errorf("seed integration data: %w", err)
	}
	return nil
}

const resetSQL = `
TRUNCATE TABLE
  artist, label, master, release_item, genre, style
RESTART IDENTITY CASCADE`

const seedSQL = `
INSERT INTO artist (id, created_at, last_modified_at, data_quality, name, profile, real_name) VALUES
  (1, now(), now(), 'Correct', 'Alpha Artist', 'Artist Profile', 'Alpha Real'),
  (2, now(), now(), 'Correct', 'Member Artist', NULL, NULL),
  (3, now(), now(), 'Correct', 'Group Artist', NULL, NULL),
  (4, now(), now(), 'Correct', 'Alias Artist', NULL, NULL);
INSERT INTO label (id, created_at, last_modified_at, contact_info, data_quality, name, profile) VALUES
  (1, now(), now(), 'Label Contact', 'Correct', 'Alpha Label', 'Label Profile'),
  (2, now(), now(), NULL, 'Correct', 'Child Label', NULL);
INSERT INTO release_item (
  id, created_at, last_modified_at, country, data_quality, has_valid_day, has_valid_month,
  has_valid_year, is_master, listed_release_date, notes, release_date, status, title
) VALUES
  (1, now(), now(), 'US', 'Correct', true, true, true, true, '2000-02-03', 'Notes', DATE '2000-02-03', 'Accepted', 'Alpha Release');
INSERT INTO master (id, created_at, last_modified_at, data_quality, title, year, main_release_id) VALUES
  (1, now(), now(), 'Correct', 'Alpha Master', 2000, 1);
UPDATE release_item SET master_id = 1 WHERE id = 1;
INSERT INTO genre (name) VALUES ('Electronic');
INSERT INTO style (name) VALUES ('Ambient');
INSERT INTO artist_member (created_at, last_modified_at, artist_id, member_id) VALUES (now(), now(), 1, 2);
INSERT INTO artist_group (created_at, last_modified_at, artist_id, group_id) VALUES (now(), now(), 1, 3);
INSERT INTO artist_alias (created_at, last_modified_at, artist_id, alias_id) VALUES (now(), now(), 1, 4);
INSERT INTO artist_name_variation (created_at, last_modified_at, hash, name_variation, artist_id) VALUES (now(), now(), 1, 'A. Artist', 1);
INSERT INTO artist_url (created_at, last_modified_at, hash, url, artist_id) VALUES (now(), now(), 1, 'https://artist.example.com', 1);
INSERT INTO label_sub_label (created_at, last_modified_at, parent_label_id, sub_label_id) VALUES (now(), now(), 1, 2);
INSERT INTO label_url (created_at, last_modified_at, hash, url, label_id) VALUES (now(), now(), 1, 'https://label.example.com', 2);
INSERT INTO master_artist (created_at, last_modified_at, artist_id, master_id) VALUES (now(), now(), 1, 1);
INSERT INTO master_genre (created_at, last_modified_at, genre, master_id) VALUES (now(), now(), 'Electronic', 1);
INSERT INTO master_style (created_at, last_modified_at, master_id, style) VALUES (now(), now(), 1, 'Ambient');
INSERT INTO master_video (created_at, last_modified_at, hash, description, title, url, master_id) VALUES (now(), now(), 1, 'Video', 'Master Video', 'https://video.example.com/master', 1);
INSERT INTO release_item_artist (created_at, last_modified_at, artist_id, release_item_id) VALUES (now(), now(), 1, 1);
INSERT INTO release_item_credited_artist (created_at, last_modified_at, hash, role, artist_id, release_item_id) VALUES (now(), now(), 1, 'Producer', 2, 1);
INSERT INTO label_release_item (created_at, last_modified_at, category_notation, label_id, release_item_id) VALUES (now(), now(), 'CAT-1', 1, 1);
INSERT INTO release_item_work (created_at, last_modified_at, hash, work, label_id, release_item_id) VALUES (now(), now(), 1, 'Pressed By', 2, 1);
INSERT INTO release_item_format (created_at, last_modified_at, hash, description, name, quantity, text, release_item_id) VALUES (now(), now(), 1, 'Album,Limited Edition', 'Vinyl', 1, NULL, 1);
INSERT INTO release_item_style (created_at, last_modified_at, release_item_id, style) VALUES (now(), now(), 1, 'Ambient');
INSERT INTO release_item_genre (created_at, last_modified_at, genre, release_item_id) VALUES (now(), now(), 'Electronic', 1);
INSERT INTO release_item_video (created_at, last_modified_at, hash, description, title, url, release_item_id) VALUES (now(), now(), 1, 'Video', 'Release Video', 'https://video.example.com/release', 1);`
