\set ON_ERROR_STOP on

set synchronous_commit = off;

insert into artist (id, created_at, last_modified_at, data_quality, name, profile, real_name)
select id,
       timestamp '2026-08-13',
       timestamp '2026-08-13',
       'Correct',
       'Artist ' || id,
       'Deterministic performance fixture artist ' || id,
       'Real Artist ' || id
from generate_series(1, :artist_rows) id;

insert into label (id, created_at, last_modified_at, contact_info, data_quality, name, profile)
select id,
       timestamp '2026-08-13',
       timestamp '2026-08-13',
       'label-' || id || '@example.invalid',
       'Correct',
       'Label ' || id,
       'Deterministic performance fixture label ' || id
from generate_series(1, :label_rows) id;

insert into genre (name)
values ('Electronic'), ('Rock'), ('Jazz');

insert into style (name)
values ('Ambient'), ('House'), ('Techno');

insert into master (id, created_at, last_modified_at, data_quality, title, year)
select id,
       timestamp '2026-08-13',
       timestamp '2026-08-13',
       'Correct',
       'Master ' || id,
       (1950 + id % 76)::smallint
from generate_series(1, :master_rows) id;

insert into release_item (
    id,
    created_at,
    last_modified_at,
    country,
    data_quality,
    has_valid_day,
    has_valid_month,
    has_valid_year,
    is_master,
    master_id,
    listed_release_date,
    notes,
    release_date,
    status,
    title
)
select id,
       timestamp '2026-08-13',
       timestamp '2026-08-13',
       case id % 4 when 0 then 'US' when 1 then 'JP' when 2 then 'GB' else 'DE' end,
       'Correct',
       true,
       true,
       true,
       id % 2 = 0,
       case when id % 100 = 0 then 1 else 1 + id % :master_rows end,
       to_char(make_date(1950 + id % 76, 1 + id % 12, 1 + id % 28), 'YYYY-MM-DD'),
       'Deterministic performance fixture release ' || id,
       make_date(1950 + id % 76, 1 + id % 12, 1 + id % 28),
       'Accepted',
       case when id > :release_rows - 100
            then 'Rare needle release ' || id
            else 'Catalog release ' || id
       end
from generate_series(1, :release_rows) id;

insert into release_item_artist (last_modified_at, artist_id, release_item_id)
select timestamp '2026-08-13',
       case when id % 100 = 0 then 1 else 1 + id % :artist_rows end,
       id
from generate_series(1, :release_rows) id;

insert into release_item_credited_artist (
    last_modified_at,
    hash,
    role,
    artist_id,
    release_item_id,
    identity_sha256
)
select timestamp '2026-08-13', id, 'Producer', 1, id, null
from generate_series(250, :release_rows, 250) id;

insert into label_release_item (
    last_modified_at,
    category_notation,
    label_id,
    release_item_id
)
select timestamp '2026-08-13',
       'CAT-' || id,
       case when id % 100 = 0 then 1 else 1 + id % :label_rows end,
       id
from generate_series(1, :release_rows) id;

insert into artist_alias (last_modified_at, alias_id, artist_id)
select timestamp '2026-08-13', id, 1
from generate_series(2, 31) id;

insert into artist_group (last_modified_at, artist_id, group_id)
select timestamp '2026-08-13', 1, id
from generate_series(2, 31) id;

insert into artist_member (last_modified_at, artist_id, member_id)
select timestamp '2026-08-13', 1, id
from generate_series(2, 31) id;

insert into artist_name_variation (
    hash,
    last_modified_at,
    name_variation,
    artist_id,
    identity_sha256
)
select id, timestamp '2026-08-13', 'Artist One Variation ' || id, 1, null
from generate_series(1, 30) id;

insert into artist_url (last_modified_at, hash, url, artist_id, identity_sha256)
select timestamp '2026-08-13', id, 'https://artist.example.invalid/' || id, 1, null
from generate_series(1, 30) id;

insert into label_sub_label (last_modified_at, parent_label_id, sub_label_id)
select timestamp '2026-08-13', 1, id
from generate_series(2, 31) id;

insert into label_url (last_modified_at, hash, url, label_id, identity_sha256)
select timestamp '2026-08-13', id, 'https://label.example.invalid/' || id, 1, null
from generate_series(1, 30) id;

insert into master_artist (last_modified_at, artist_id, master_id)
select timestamp '2026-08-13', id, 1
from generate_series(2, 31) id;

insert into master_genre (last_modified_at, genre, master_id)
values
    (timestamp '2026-08-13', 'Electronic', 1),
    (timestamp '2026-08-13', 'Rock', 1),
    (timestamp '2026-08-13', 'Jazz', 1);

insert into master_style (last_modified_at, master_id, style)
values
    (timestamp '2026-08-13', 1, 'Ambient'),
    (timestamp '2026-08-13', 1, 'House'),
    (timestamp '2026-08-13', 1, 'Techno');

insert into master_video (
    last_modified_at,
    hash,
    description,
    title,
    url,
    master_id,
    identity_sha256
)
select timestamp '2026-08-13',
       id,
       'Master video ' || id,
       'Master fixture video ' || id,
       'https://video.example.invalid/master/' || id,
       1,
       null
from generate_series(1, 30) id;

insert into release_item_credited_artist (
    last_modified_at,
    hash,
    role,
    artist_id,
    release_item_id,
    identity_sha256
)
select timestamp '2026-08-13', 1000000 + id, 'Remix', id, 1, null
from generate_series(3, 31) id;

insert into label_release_item (
    last_modified_at,
    category_notation,
    label_id,
    release_item_id
)
select timestamp '2026-08-13', 'DETAIL-' || id, id, 1
from generate_series(3, 31) id;

insert into release_item_work (
    last_modified_at,
    hash,
    work,
    label_id,
    release_item_id,
    identity_sha256
)
select timestamp '2026-08-13', id, 'Company role ' || id, id, 1, null
from generate_series(1, 30) id;

insert into release_item_format (
    last_modified_at,
    hash,
    description,
    name,
    quantity,
    quantity_text,
    text,
    release_item_id,
    identity_sha256
)
select timestamp '2026-08-13',
       id,
       'Description ' || id,
       'Format ' || id,
       1,
       '1',
       'Fixture format ' || id,
       1,
       null
from generate_series(1, 30) id;

insert into release_item_genre (last_modified_at, genre, release_item_id)
values
    (timestamp '2026-08-13', 'Electronic', 1),
    (timestamp '2026-08-13', 'Rock', 1),
    (timestamp '2026-08-13', 'Jazz', 1);

insert into release_item_style (last_modified_at, release_item_id, style)
values
    (timestamp '2026-08-13', 1, 'Ambient'),
    (timestamp '2026-08-13', 1, 'House'),
    (timestamp '2026-08-13', 1, 'Techno');

insert into release_item_video (
    last_modified_at,
    hash,
    description,
    title,
    url,
    release_item_id,
    identity_sha256
)
select timestamp '2026-08-13',
       id,
       'Release video ' || id,
       'Release fixture video ' || id,
       'https://video.example.invalid/release/' || id,
       1,
       null
from generate_series(1, 30) id;

analyze;
