package postgres

import (
	"fmt"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
	"github.com/jackc/pgx/v5"
)

type rowScanner[T catalog.PageItem] func(pgx.CollectableRow) (T, error)

func collectRows[T catalog.PageItem](rows pgx.Rows, operation string, scanner rowScanner[T]) ([]T, error) {
	items, err := pgx.CollectRows(rows, pgx.RowToFunc[T](scanner))
	return items, operationError(operation, err)
}

func operationError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func scanArtist(row pgx.CollectableRow) (catalog.Artist, error) {
	var item catalog.Artist
	err := row.Scan(&item.ID, &item.Name, &item.RealName, &item.Profile, &item.DataQuality)
	return item, err
}

func scanLabel(row pgx.CollectableRow) (catalog.Label, error) {
	var item catalog.Label
	err := row.Scan(&item.ID, &item.ContactInfo, &item.DataQuality, &item.Name, &item.Profile)
	return item, err
}

func scanMaster(row pgx.CollectableRow) (catalog.Master, error) {
	var item catalog.Master
	err := row.Scan(&item.ID, &item.DataQuality, &item.Title, &item.ReleasedYear)
	return item, err
}

func scanRelease(row pgx.CollectableRow) (catalog.Release, error) {
	var item catalog.Release
	err := row.Scan(
		&item.ID, &item.Title, &item.Country, &item.DataQuality,
		&item.ReleasedYear, &item.ReleasedMonth, &item.ReleasedDay,
		&item.ListedReleaseDate, &item.IsMaster, &item.MasterID, &item.Notes, &item.Status,
	)
	return item, err
}

func (s *Store) scanArtistRelease(row pgx.CollectableRow) (catalog.ArtistRelease, error) {
	var item catalog.ArtistRelease
	err := row.Scan(
		&item.ID, &item.Role, &item.Title, &item.Country, &item.DataQuality,
		&item.ReleasedYear, &item.ReleasedMonth, &item.ReleasedDay,
		&item.ListedReleaseDate, &item.IsMaster, &item.MasterID, &item.Notes, &item.Status,
	)
	item.ResourceURL = fmt.Sprintf("%s/releases/%d", s.serverURL, item.ID)
	return item, err
}

func scanLabelRelease(row pgx.CollectableRow) (catalog.LabelRelease, error) {
	var item catalog.LabelRelease
	err := row.Scan(&item.ID, &item.Artist, &item.Title, &item.Year, &item.Status, &item.CatalogNumbers, &item.Format)
	return item, err
}

func (s *Store) scanMasterRelease(row pgx.CollectableRow) (catalog.MasterRelease, error) {
	var item catalog.MasterRelease
	err := row.Scan(&item.ID, &item.Title, &item.Artists, &item.ArtistIDs, &item.Year)
	item.ResourceURL = fmt.Sprintf("%s/releases/%d", s.serverURL, item.ID)
	return item, err
}
