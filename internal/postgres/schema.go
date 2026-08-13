package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const requiredAPIRelationNames = "artist,artist_alias,artist_group,artist_member,artist_name_variation,artist_url,discogs_catalog_readiness,label,label_release_item,label_sub_label,label_url,master,master_artist,master_genre,master_style,master_video,release_item,release_item_artist,release_item_credited_artist,release_item_format,release_item_genre,release_item_style,release_item_video,release_item_work"

const validateSchemaQuery = `
WITH schema_presence AS (
  SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname = $1) AS schema_exists
), schema_state AS (
  SELECT schema_exists,
         CASE WHEN schema_exists THEN has_schema_privilege(current_user, $1, 'USAGE') ELSE false END AS can_use
  FROM schema_presence
), required_tables AS (
  SELECT schema_exists,
         can_use,
         table_name,
         CASE
           WHEN schema_exists AND can_use THEN to_regclass(format('%I.%I', $1, table_name))
           ELSE NULL
         END AS relation
  FROM schema_state
  CROSS JOIN unnest(string_to_array($2, ',')) required(table_name)
), table_violations AS (
  SELECT COALESCE(
           array_agg(table_name ORDER BY table_name)
             FILTER (WHERE schema_exists AND can_use AND relation IS NULL),
           ARRAY[]::text[]
         ) AS missing_names,
         COALESCE(
           array_agg(table_name ORDER BY table_name)
             FILTER (WHERE relation IS NOT NULL AND NOT has_table_privilege(current_user, relation, 'SELECT')),
           ARRAY[]::text[]
         ) AS unreadable_names
  FROM required_tables
)
SELECT schema_exists, can_use, missing_names, unreadable_names
FROM schema_state CROSS JOIN table_violations`

type schemaState struct {
	exists           bool
	canUse           bool
	missingTables    []string
	unreadableTables []string
}

// ValidateSchema verifies the complete read schema without creating or migrating database objects.
func ValidateSchema(ctx context.Context, pool *pgxpool.Pool, schemaName string) error {
	var state schemaState
	if err := pool.QueryRow(ctx, validateSchemaQuery, schemaName, requiredAPIRelationNames).Scan(
		&state.exists,
		&state.canUse,
		&state.missingTables,
		&state.unreadableTables,
	); err != nil {
		return fmt.Errorf("inspect database schema %s: %w", schemaName, err)
	}
	if !state.exists {
		return fmt.Errorf("database schema %s does not exist; run an OpenDiscogs batch importer first", schemaName)
	}
	if !state.canUse {
		return fmt.Errorf("current user requires USAGE on database schema %s", schemaName)
	}
	if len(state.missingTables) > 0 {
		return fmt.Errorf(
			"database schema %s is missing required API relations: %s; run an OpenDiscogs batch importer first",
			schemaName,
			strings.Join(state.missingTables, ", "),
		)
	}
	if len(state.unreadableTables) > 0 {
		return fmt.Errorf(
			"current user requires SELECT on database schema %s relations: %s",
			schemaName,
			strings.Join(state.unreadableTables, ", "),
		)
	}
	return nil
}
