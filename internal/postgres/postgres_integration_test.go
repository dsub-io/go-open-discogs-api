package postgres

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
	canonicalschema "github.com/dsub-io/open-discogs-model/schema"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	envTestDatabaseURL              = "TEST_DATABASE_URL"
	defaultTestDatabaseURL          = "postgres://discogs:discogs@127.0.0.1:55432/discogs?sslmode=disable"
	testPublicURL                   = "https://api.example.com"
	testQueryTimeout                = 5 * time.Second
	testSchemaMigrationLockID int64 = 7_803_151_124
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
	request := catalog.PageRequest{Size: 20}

	artists, err := store.SearchArtists(context.Background(), catalog.ArtistFilter{Name: "alpha", RealName: "real"}, request)
	assertPage(t, artists, 1, false, err)
	artists, err = store.SearchArtists(context.Background(), catalog.ArtistFilter{Name: "alpha", RealName: "real"}, request)
	assertPage(t, artists, 1, false, err)

	labels, err := store.SearchLabels(context.Background(), catalog.LabelFilter{Name: "alpha"}, request)
	assertPage(t, labels, 1, false, err)
	masters, err := store.SearchMasters(context.Background(), catalog.MasterFilter{Title: "master", Year: intPointer(2000)}, request)
	assertPage(t, masters, 1, false, err)
	releases, err := store.SearchReleases(context.Background(), catalog.ReleaseFilter{Title: "release", Country: "us", Year: intPointer(2000), Month: intPointer(2), Master: boolPointer(true)}, request)
	assertPage(t, releases, 1, false, err)

	artistReleases, err := store.ArtistReleases(context.Background(), 1, request)
	assertPage(t, artistReleases, 1, false, err)
	if artistReleases.Items[0].ResourceURL != testPublicURL+"/releases/1" {
		t.Fatalf("artist resource URL=%s", artistReleases.Items[0].ResourceURL)
	}
	labelReleases, err := store.LabelReleases(context.Background(), 1, request)
	assertPage(t, labelReleases, 1, false, err)
	masterReleases, err := store.MasterReleases(context.Background(), 1, request)
	assertPage(t, masterReleases, 1, false, err)

	first, err := store.SearchArtists(context.Background(), catalog.ArtistFilter{}, catalog.PageRequest{Size: 1})
	assertPage(t, first, 1, true, err)
	next := first.NextAfterID()
	if next == nil || *next != 1 {
		t.Fatalf("first cursor=%v", next)
	}
	second, err := store.SearchArtists(
		context.Background(),
		catalog.ArtistFilter{},
		catalog.PageRequest{AfterID: *next, Size: 1},
	)
	assertPage(t, second, 1, true, err)
	if second.Items[0].ID != 2 {
		t.Fatalf("second page=%+v", second)
	}
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
	if _, err := store.SearchArtists(ctx, catalog.ArtistFilter{}, catalog.PageRequest{Size: 1}); err == nil {
		t.Fatal("canceled search succeeded")
	}
}

func TestStoreHelpers(t *testing.T) {
	t.Parallel()
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

	page, err := loadPage(context.Background(), 1,
		func(context.Context) ([]catalog.Artist, error) { return []catalog.Artist{{ID: 1}, {ID: 2}}, nil },
	)
	if err != nil || !page.HasMore || len(page.Items) != 1 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	_, err = loadPage(context.Background(), 1,
		func(context.Context) ([]catalog.Artist, error) { return nil, errRepositoryTest },
	)
	if !errors.Is(err, errRepositoryTest) {
		t.Fatalf("item loader error=%v", err)
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
	request := catalog.PageRequest{Size: 1}
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

func TestValidateSchemaBoundaries(t *testing.T) {
	ctx := context.Background()
	if err := ValidateSchema(ctx, integrationPool, "public"); err != nil {
		t.Fatal(err)
	}

	if err := ValidateSchema(ctx, integrationPool, "missing_schema"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing schema error=%v", err)
	}

	const incompleteSchema = "api_incomplete"
	if _, err := integrationPool.Exec(ctx, "DROP SCHEMA IF EXISTS "+incompleteSchema+" CASCADE"); err != nil {
		t.Fatal(err)
	}
	if _, err := integrationPool.Exec(ctx, "CREATE SCHEMA "+incompleteSchema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = integrationPool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+incompleteSchema+" CASCADE")
	})
	if err := ValidateSchema(ctx, integrationPool, incompleteSchema); err == nil || !strings.Contains(err.Error(), "missing required API tables") {
		t.Fatalf("incomplete schema error=%v", err)
	}

	testSchemaUsageDenied(t, ctx)
	testTableSelectDenied(t, ctx)

	closedPool, err := pgxpool.New(ctx, integrationPool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	closedPool.Close()
	if err := ValidateSchema(ctx, closedPool, "public"); err == nil || !strings.Contains(err.Error(), "inspect database schema") {
		t.Fatalf("closed pool error=%v", err)
	}
}

func TestStoreReadsSelectedCustomSchema(t *testing.T) {
	const schemaName = "api_custom"
	ctx := context.Background()
	if _, err := integrationPool.Exec(ctx, "DROP SCHEMA IF EXISTS "+schemaName+" CASCADE"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = integrationPool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schemaName+" CASCADE")
	})
	migrations, err := canonicalschema.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	initialMigration, err := fs.ReadFile(migrations, "V001__initial_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := integrationPool.Exec(ctx, "CREATE SCHEMA "+schemaName); err != nil {
		t.Fatal(err)
	}
	if _, err := integrationPool.Exec(ctx, strings.ReplaceAll(string(initialMigration), "public.", schemaName+".")); err != nil {
		t.Fatal(err)
	}

	poolConfig, err := pgxpool.ParseConfig(integrationPool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = `"` + schemaName + `"`
	customPool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer customPool.Close()
	if err := ValidateSchema(ctx, customPool, schemaName); err != nil {
		t.Fatal(err)
	}
	if _, err := customPool.Exec(ctx, `
INSERT INTO artist (id, created_at, last_modified_at, name)
VALUES (9001, now(), now(), 'Custom Schema Artist')`); err != nil {
		t.Fatal(err)
	}
	page, err := New(customPool, testPublicURL, testQueryTimeout).SearchArtists(
		ctx,
		catalog.ArtistFilter{Name: "custom schema"},
		catalog.PageRequest{Size: 20},
	)
	assertPage(t, page, 1, false, err)
	if page.Items[0].ID != 9001 {
		t.Fatalf("custom schema artist=%+v", page.Items[0])
	}
}

func testSchemaUsageDenied(t *testing.T, ctx context.Context) {
	t.Helper()
	const (
		roleName   = "api_no_schema_usage"
		schemaName = "api_private"
		password   = "schema-test-password"
	)
	for _, statement := range []string{
		"DROP SCHEMA IF EXISTS " + schemaName + " CASCADE",
		"DROP ROLE IF EXISTS " + roleName,
		"CREATE ROLE " + roleName + " LOGIN PASSWORD '" + password + "'",
		"CREATE SCHEMA " + schemaName,
		"REVOKE ALL ON SCHEMA " + schemaName + " FROM PUBLIC",
	} {
		if _, err := integrationPool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	poolConfig, err := pgxpool.ParseConfig(integrationPool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	poolConfig.ConnConfig.User = roleName
	poolConfig.ConnConfig.Password = password
	restrictedPool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSchema(ctx, restrictedPool, schemaName); err == nil || !strings.Contains(err.Error(), "requires USAGE") {
		restrictedPool.Close()
		t.Fatalf("schema usage error=%v", err)
	}
	restrictedPool.Close()
	if _, err := integrationPool.Exec(ctx, "DROP SCHEMA "+schemaName+" CASCADE"); err != nil {
		t.Fatal(err)
	}
	if _, err := integrationPool.Exec(ctx, "DROP ROLE "+roleName); err != nil {
		t.Fatal(err)
	}
}

func testTableSelectDenied(t *testing.T, ctx context.Context) {
	t.Helper()
	const (
		roleName = "api_no_table_select"
		password = "table-test-password"
	)
	for _, statement := range []string{
		"DROP ROLE IF EXISTS " + roleName,
		"CREATE ROLE " + roleName + " LOGIN PASSWORD '" + password + "'",
		"GRANT USAGE ON SCHEMA public TO " + roleName,
	} {
		if _, err := integrationPool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	poolConfig, err := pgxpool.ParseConfig(integrationPool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	poolConfig.ConnConfig.User = roleName
	poolConfig.ConnConfig.Password = password
	restrictedPool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSchema(ctx, restrictedPool, "public"); err == nil || !strings.Contains(err.Error(), "requires SELECT") {
		restrictedPool.Close()
		t.Fatalf("table SELECT error=%v", err)
	}
	restrictedPool.Close()
	if _, err := integrationPool.Exec(ctx, "REVOKE USAGE ON SCHEMA public FROM "+roleName); err != nil {
		t.Fatal(err)
	}
	if _, err := integrationPool.Exec(ctx, "DROP ROLE "+roleName); err != nil {
		t.Fatal(err)
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

func assertPage[T catalog.PageItem](t *testing.T, page catalog.Page[T], itemCount int, hasMore bool, err error) {
	t.Helper()
	if err != nil || len(page.Items) != itemCount || page.HasMore != hasMore {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}

func intPointer(value int) *int { return &value }

func boolPointer(value bool) *bool { return &value }

func prepareDatabase(ctx context.Context, pool *pgxpool.Pool) error {
	if err := ensureCanonicalTestSchema(ctx, pool); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, resetSQL); err != nil {
		return fmt.Errorf("reset integration data: %w", err)
	}
	if _, err := pool.Exec(ctx, seedSQL); err != nil {
		return fmt.Errorf("seed integration data: %w", err)
	}
	return nil
}

func ensureCanonicalTestSchema(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin schema migration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", testSchemaMigrationLockID); err != nil {
		return fmt.Errorf("lock schema migrations: %w", err)
	}
	var schemaExists bool
	if err := tx.QueryRow(ctx, "SELECT to_regclass('public.artist') IS NOT NULL").Scan(&schemaExists); err != nil {
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
			if _, err := tx.Exec(ctx, string(migration)); err != nil {
				return fmt.Errorf("apply %s: %w", file.Name(), err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit schema migrations: %w", err)
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
