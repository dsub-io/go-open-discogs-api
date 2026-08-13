package postgres

import (
	"context"
	"fmt"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
)

func (s *Store) Artist(ctx context.Context, id int64) (catalog.ArtistDetail, error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	const query = `
SELECT a.id::bigint, a.name, a.real_name, a.profile, a.data_quality,
  COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'id', related.id::bigint,
      'name', related.name,
      'resource_url', $2 || '/artists/' || related.id
    ))
    FROM artist_member relation
    JOIN artist related ON related.id = relation.member_id
    WHERE relation.artist_id = a.id
  ), '[]'::jsonb),
  COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'id', related.id::bigint,
      'name', related.name,
      'resource_url', $2 || '/artists/' || related.id
    ))
    FROM artist_group relation
    JOIN artist related ON related.id = relation.group_id
    WHERE relation.artist_id = a.id
  ), '[]'::jsonb),
  COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'id', related.id::bigint,
      'name', related.name,
      'resource_url', $2 || '/artists/' || related.id
    ))
    FROM artist_alias relation
    JOIN artist related ON related.id = relation.alias_id
    WHERE relation.artist_id = a.id
  ), '[]'::jsonb),
  COALESCE((
    SELECT jsonb_agg(variation.name_variation)
    FROM artist_name_variation variation
    WHERE variation.artist_id = a.id
  ), '[]'::jsonb),
  COALESCE((
    SELECT jsonb_agg(artist_url.url)
    FROM artist_url
    WHERE artist_url.artist_id = a.id
  ), '[]'::jsonb)
FROM artist a
WHERE a.id = $1`

	var detail catalog.ArtistDetail
	var members, groups, aliases, variations, urls []byte
	err := s.pool.QueryRow(ctx, query, id, s.serverURL).Scan(
		&detail.ID, &detail.Name, &detail.RealName, &detail.Profile, &detail.DataQuality,
		&members, &groups, &aliases, &variations, &urls,
	)
	if err != nil {
		return catalog.ArtistDetail{}, notFound(err)
	}
	detail.URI = fmt.Sprintf("%s/artists/%d", s.serverURL, id)
	detail.ReleaseURL = detail.URI + "/releases"
	return artistDetailResult(detail, artistDetailPayload{members, groups, aliases, variations, urls})
}

func (s *Store) Label(ctx context.Context, id int64) (catalog.LabelDetail, error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	const query = `
SELECT l.id::bigint, l.contact_info, l.data_quality, l.name, l.profile,
  (
    SELECT jsonb_build_object(
      'id', parent.id::bigint,
      'name', parent.name,
      'resource_url', $2 || '/labels/' || parent.id
    )
    FROM label_sub_label relation
    JOIN label parent ON parent.id = relation.parent_label_id
    WHERE relation.sub_label_id = l.id
    ORDER BY parent.id
    LIMIT 1
  ),
  COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'id', child.id::bigint,
      'name', child.name,
      'resource_url', $2 || '/labels/' || child.id
    ))
    FROM label_sub_label relation
    JOIN label child ON child.id = relation.sub_label_id
    WHERE relation.parent_label_id = l.id
  ), '[]'::jsonb),
  COALESCE((
    SELECT jsonb_agg(label_url.url)
    FROM label_url
    WHERE label_url.label_id = l.id
  ), '[]'::jsonb)
FROM label l
WHERE l.id = $1`

	var detail catalog.LabelDetail
	var parent, sublabels, urls []byte
	err := s.pool.QueryRow(ctx, query, id, s.serverURL).Scan(
		&detail.ID, &detail.ContactInfo, &detail.DataQuality, &detail.Name, &detail.Profile,
		&parent, &sublabels, &urls,
	)
	if err != nil {
		return catalog.LabelDetail{}, notFound(err)
	}
	detail.URI = fmt.Sprintf("%s/labels/%d", s.serverURL, id)
	detail.ReleaseURL = detail.URI + "/releases"
	return labelDetailResult(detail, labelDetailPayload{parent, sublabels, urls})
}

func (s *Store) Master(ctx context.Context, id int64) (catalog.MasterDetail, error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	const query = `
SELECT m.id::bigint, m.title, m.data_quality, m.main_release_id::bigint, m.year::integer,
  COALESCE((
    SELECT jsonb_agg(master_genre.genre)
    FROM master_genre
    WHERE master_genre.master_id = m.id
  ), '[]'::jsonb),
  COALESCE((
    SELECT jsonb_agg(master_style.style)
    FROM master_style
    WHERE master_style.master_id = m.id
  ), '[]'::jsonb),
  COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'id', artist.id::bigint,
      'name', artist.name,
      'resource_url', $2 || '/artists/' || artist.id
    ))
    FROM master_artist
    JOIN artist ON artist.id = master_artist.artist_id
    WHERE master_artist.master_id = m.id
  ), '[]'::jsonb),
  COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'url', master_video.url,
      'description', master_video.description,
      'title', master_video.title
    ))
    FROM master_video
    WHERE master_video.master_id = m.id
  ), '[]'::jsonb)
FROM master m
WHERE m.id = $1`

	var detail catalog.MasterDetail
	var genres, styles, artists, videos []byte
	err := s.pool.QueryRow(ctx, query, id, s.serverURL).Scan(
		&detail.ID, &detail.Title, &detail.DataQuality, &detail.MainRelease, &detail.Year,
		&genres, &styles, &artists, &videos,
	)
	if err != nil {
		return catalog.MasterDetail{}, notFound(err)
	}
	return masterDetailResult(detail, masterDetailPayload{genres, styles, artists, videos})
}

func (s *Store) Release(ctx context.Context, id int64) (catalog.ReleaseDetail, error) {
	ctx, cancel := s.timeout(ctx)
	defer cancel()
	const query = `
SELECT r.id::bigint, r.title, r.country, r.data_quality,
  CASE WHEN r.has_valid_year THEN extract(year FROM r.release_date)::integer END,
  CASE WHEN r.has_valid_month THEN extract(month FROM r.release_date)::integer END,
  CASE WHEN r.has_valid_day THEN extract(day FROM r.release_date)::integer END,
  r.listed_release_date, r.is_master, r.master_id::bigint, r.notes, r.status,
  COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'id', related.id::bigint,
      'name', related.name,
      'role', related.role,
      'resource_url', $2 || '/artists/' || related.id
    ))
    FROM (
      SELECT artist.id, artist.name,
             string_agg(DISTINCT trim(relation.role), ',' ORDER BY trim(relation.role)) AS role
      FROM (
        SELECT release_item_artist.artist_id, 'Main'::text AS role
        FROM release_item_artist
        WHERE release_item_artist.release_item_id = r.id
        UNION ALL
        SELECT release_item_credited_artist.artist_id, release_item_credited_artist.role
        FROM release_item_credited_artist
        WHERE release_item_credited_artist.release_item_id = r.id
      ) relation
      JOIN artist ON artist.id = relation.artist_id
      GROUP BY artist.id, artist.name
    ) related
  ), '[]'::jsonb),
  COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'id', label.id::bigint,
      'name', label.name,
      'catno', relation.category_notation,
      'resource_url', $2 || '/labels/' || label.id
    ))
    FROM label_release_item relation
    JOIN label ON label.id = relation.label_id
    WHERE relation.release_item_id = r.id
  ), '[]'::jsonb),
  COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'id', label.id::bigint,
      'name', label.name,
      'catno', relation.work,
      'resource_url', $2 || '/labels/' || label.id
    ))
    FROM release_item_work relation
    JOIN label ON label.id = relation.label_id
    WHERE relation.release_item_id = r.id
  ), '[]'::jsonb),
  COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'name', release_item_format.name,
      'qty', COALESCE(
        release_item_format.quantity_text,
        release_item_format.quantity::text
      ),
      'descriptions', CASE
        WHEN release_item_format.description IS NULL THEN '[]'::jsonb
        ELSE to_jsonb(string_to_array(release_item_format.description, ','))
      END
    ))
    FROM release_item_format
    WHERE release_item_format.release_item_id = r.id
  ), '[]'::jsonb),
  COALESCE((
    SELECT jsonb_agg(release_item_style.style)
    FROM release_item_style
    WHERE release_item_style.release_item_id = r.id
  ), '[]'::jsonb),
  COALESCE((
    SELECT jsonb_agg(release_item_genre.genre)
    FROM release_item_genre
    WHERE release_item_genre.release_item_id = r.id
  ), '[]'::jsonb),
  COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'title', release_item_video.title,
      'url', release_item_video.url,
      'description', release_item_video.description
    ))
    FROM release_item_video
    WHERE release_item_video.release_item_id = r.id
  ), '[]'::jsonb)
FROM release_item r
WHERE r.id = $1`

	var detail catalog.ReleaseDetail
	var artists, labels, companies, formats, styles, genres, videos []byte
	err := s.pool.QueryRow(ctx, query, id, s.serverURL).Scan(
		&detail.ID, &detail.Title, &detail.Country, &detail.DataQuality,
		&detail.ReleasedYear, &detail.ReleasedMonth, &detail.ReleasedDay,
		&detail.ListedReleaseDate, &detail.IsMaster, &detail.MasterID, &detail.Notes, &detail.Status,
		&artists, &labels, &companies, &formats, &styles, &genres, &videos,
	)
	if err != nil {
		return catalog.ReleaseDetail{}, notFound(err)
	}
	return releaseDetailResult(detail, releaseDetailPayload{artists, labels, companies, formats, styles, genres, videos})
}
