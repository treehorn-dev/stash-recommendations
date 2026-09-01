package catalog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/treehorn/stash-recommendations/server/internal/catalog"
	"github.com/treehorn/stash-recommendations/server/internal/domain"
	"github.com/treehorn/stash-recommendations/server/internal/store"
)

func TestSnapshotUpsertKeepsNewestSourceVersion(t *testing.T) {
	repository := openSnapshotStore(t)
	service := catalog.NewSnapshotService(repository)
	ctx := context.Background()

	require.NoError(t, service.Upsert(ctx, snapshotJSON(t, "2026-08-30T10:00:00Z", "Canonical title")))
	require.NoError(t, service.Upsert(ctx, snapshotJSON(t, "2026-08-30T09:00:00Z", "Older title")))

	scene, found, err := repository.CatalogSource(ctx, domain.ContentKey{
		Endpoint: "https://box.example/graphql",
		StashID:  "scene-1",
	})
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "Canonical title", scene.Title)

	err = service.Upsert(ctx, snapshotWithExtraField(t, "paths"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "paths")
}

func TestSnapshotUpsertProjectsRelationsAndCanonicalURL(t *testing.T) {
	repository := openSnapshotStore(t)
	service := catalog.NewSnapshotService(repository)
	ctx := context.Background()

	_, err := repository.Pool().Exec(ctx, `
		INSERT INTO source_configs (endpoint, canonical_scene_url_template)
		VALUES ($1, $2)
	`, "https://box.example/graphql", "https://box.example/scenes/{stash_id}")
	require.NoError(t, err)

	require.NoError(t, service.Upsert(ctx, richSnapshotJSON(t, "2026-08-30T10:00:00Z")))

	scene, found, err := repository.CatalogSource(ctx, domain.ContentKey{
		Endpoint: "https://box.example/graphql",
		StashID:  "scene-1",
	})
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "Example Scene", scene.Title)
	require.Equal(t, []string{"2026-08-30"}, scene.Dates)
	require.Equal(t, []string{"https://example.test/scenes/scene-1"}, scene.URLs)
	require.Equal(t, []string{"https://images.example/scene.jpg"}, scene.RemoteImages)
	require.NotNil(t, scene.Duration)
	require.Equal(t, 120, *scene.Duration)
	require.NotNil(t, scene.Studio)
	require.Equal(t, "studio-1", scene.Studio.ContentKey.StashID)
	require.Equal(t, []catalog.EntityReference{{
		ContentKey: domain.ContentKey{Endpoint: "https://box.example/graphql", StashID: "tag-1"},
		Name:       "Tag",
	}}, scene.Tags)
	require.Equal(t, []catalog.EntityReference{{
		ContentKey: domain.ContentKey{Endpoint: "https://box.example/graphql", StashID: "performer-1"},
		Name:       "Performer",
	}}, scene.Performers)
	require.NotNil(t, scene.CanonicalURL)
	require.Equal(t, "https://box.example/scenes/scene-1", *scene.CanonicalURL)

	var performerAliasesJSON []byte
	require.NoError(t, repository.Pool().QueryRow(ctx, `
		SELECT aliases
		FROM source_performers
		WHERE endpoint = $1 AND stash_id = $2
	`, "https://box.example/graphql", "performer-1").Scan(&performerAliasesJSON))
	var performerAliases []string
	require.NoError(t, json.Unmarshal(performerAliasesJSON, &performerAliases))
	require.Equal(t, []string{"Alias"}, performerAliases)

	var appearanceOrder int
	require.NoError(t, repository.Pool().QueryRow(ctx, `
		SELECT appearance_order
		FROM source_scene_performers
		WHERE scene_endpoint = $1 AND scene_stash_id = $2 AND performer_endpoint = $3 AND performer_stash_id = $4
	`, "https://box.example/graphql", "scene-1", "https://box.example/graphql", "performer-1").Scan(&appearanceOrder))
	require.Equal(t, 1, appearanceOrder)

	var tagOrder int
	require.NoError(t, repository.Pool().QueryRow(ctx, `
		SELECT tag_order
		FROM source_scene_tags
		WHERE scene_endpoint = $1 AND scene_stash_id = $2 AND tag_endpoint = $3 AND tag_stash_id = $4
	`, "https://box.example/graphql", "scene-1", "https://box.example/graphql", "tag-1").Scan(&tagOrder))
	require.Equal(t, 1, tagOrder)
}

func TestSnapshotUpsertIgnoresStaleUpdatesAndAllowsRepeatUpserts(t *testing.T) {
	repository := openSnapshotStore(t)
	service := catalog.NewSnapshotService(repository)
	ctx := context.Background()

	require.NoError(t, service.Upsert(ctx, snapshotJSON(t, "2026-08-30T10:00:00Z", "Canonical title")))
	require.NoError(t, service.Upsert(ctx, snapshotJSON(t, "2026-08-30T10:00:00Z", "Canonical title")))
	require.NoError(t, service.Upsert(ctx, snapshotJSON(t, "2026-08-30T09:00:00Z", "Older title")))

	scene, found, err := repository.CatalogSource(ctx, domain.ContentKey{
		Endpoint: "https://box.example/graphql",
		StashID:  "scene-1",
	})
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "Canonical title", scene.Title)

	var count int
	require.NoError(t, repository.Pool().QueryRow(ctx, `
		SELECT count(*)
		FROM source_snapshots
		WHERE endpoint = $1 AND stash_id = $2
	`, "https://box.example/graphql", "scene-1").Scan(&count))
	require.Equal(t, 1, count)
}

func openSnapshotStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is required for PostgreSQL integration tests")
	}

	ctx := context.Background()
	adminPool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(adminPool.Close)

	schema := fmt.Sprintf("task6_catalog_%d", time.Now().UnixNano())
	_, err = adminPool.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := adminPool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		require.NoError(t, err)
	})

	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()

	repository, err := store.Open(ctx, parsed.String())
	require.NoError(t, err)
	t.Cleanup(func() { repository.Close(context.Background()) })
	require.NoError(t, repository.Migrate(ctx))
	return repository
}

func snapshotJSON(t *testing.T, capturedAt string, title string) []byte {
	t.Helper()
	payload := map[string]any{
		"schema_version": 1,
		"content_key": map[string]any{
			"endpoint": "https://box.example/graphql",
			"stash_id": "scene-1",
		},
		"captured_at": capturedAt,
		"scenes": []map[string]any{{
			"id":            "scene-1",
			"title":         title,
			"urls":          []string{"https://example.test/scenes/scene-1"},
			"remote_images": []string{"https://images.example/scene.jpg"},
		}},
		"performers": []map[string]any{},
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return data
}

func richSnapshotJSON(t *testing.T, capturedAt string) []byte {
	t.Helper()
	payload := map[string]any{
		"schema_version": 1,
		"content_key": map[string]any{
			"endpoint": "https://box.example/graphql",
			"stash_id": "scene-1",
		},
		"captured_at": capturedAt,
		"scenes": []map[string]any{{
			"id":       "scene-1",
			"title":    "Example Scene",
			"details":  "Details",
			"dates":    []string{"2026-08-30"},
			"urls":     []string{"https://example.test/scenes/scene-1"},
			"duration": 120,
			"director": "Director",
			"code":     "CODE-1",
			"studio": map[string]any{
				"id":   "studio-1",
				"name": "Studio",
			},
			"tags": []map[string]any{{
				"id":   "tag-1",
				"name": "Tag",
			}},
			"performer_appearances": []map[string]any{{
				"performer_id": "performer-1",
			}},
			"remote_images": []string{"https://images.example/scene.jpg"},
		}},
		"performers": []map[string]any{{
			"id":           "performer-1",
			"name":         "Performer",
			"aliases":      []string{"Alias"},
			"gender":       "female",
			"country":      "US",
			"ethnicity":    "Example",
			"eye_color":    "blue",
			"hair_color":   "brown",
			"measurements": "34B-24-34",
			"career_years": []int{2020, 2026},
			"urls":         []string{"https://example.test/performers/performer-1"},
			"remote_images": []string{
				"https://images.example/performer.jpg",
			},
		}},
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return data
}

func snapshotWithExtraField(t *testing.T, field string) []byte {
	t.Helper()
	payload := map[string]any{
		"schema_version": 1,
		"content_key": map[string]any{
			"endpoint": "https://box.example/graphql",
			"stash_id": "scene-1",
		},
		"captured_at": "2026-08-30T10:00:00Z",
		"scenes": []map[string]any{{
			"id":    "scene-1",
			field:   []string{"/private/scene.mp4"},
			"title": "Should fail",
		}},
		"performers": []map[string]any{},
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return data
}
