package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/treehorn/stash-recommendations/server/internal/domain"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repository *Repository) CurrentRatings(ctx context.Context) ([]Rating, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT account_id, endpoint, stash_id, rating
		FROM current_preferences
		ORDER BY account_id, endpoint, stash_id
	`)
	if err != nil {
		return nil, fmt.Errorf("query current ratings: %w", err)
	}
	defer rows.Close()

	var ratings []Rating
	for rows.Next() {
		var rating Rating
		if err := rows.Scan(&rating.AccountID, &rating.ContentKey.Endpoint, &rating.ContentKey.StashID, &rating.Value); err != nil {
			return nil, fmt.Errorf("scan current rating: %w", err)
		}
		ratings = append(ratings, rating)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate current ratings: %w", err)
	}
	return ratings, nil
}

func (repository *Repository) CurrentSessions(ctx context.Context) ([]Session, error) {
	rows, err := repository.pool.Query(ctx, `
		WITH latest AS (
			SELECT account_id, projection_type, MAX(projection_version) AS projection_version
			FROM session_projections
			GROUP BY account_id, projection_type
		)
		SELECT
			items.account_id,
			items.projection_type,
			items.session_order,
			items.item_order,
			items.endpoint,
			items.stash_id,
			items.kind
		FROM session_projection_items AS items
		JOIN latest
			ON latest.account_id = items.account_id
			AND latest.projection_type = items.projection_type
			AND latest.projection_version = items.projection_version
		ORDER BY items.account_id, items.projection_type, items.session_order, items.item_order
	`)
	if err != nil {
		return nil, fmt.Errorf("query current sessions: %w", err)
	}
	defer rows.Close()

	var (
		sessions    []Session
		lastAccount string
		lastType    string
		lastOrder   int
	)
	for rows.Next() {
		var (
			accountID, projectionType, endpoint, stashID, kind string
			sessionOrder, itemOrder                            int
		)
		if err := rows.Scan(&accountID, &projectionType, &sessionOrder, &itemOrder, &endpoint, &stashID, &kind); err != nil {
			return nil, fmt.Errorf("scan current session item: %w", err)
		}
		if len(sessions) == 0 || accountID != lastAccount || projectionType != lastType || sessionOrder != lastOrder {
			sessions = append(sessions, Session{AccountID: accountID, ProjectionType: projectionType})
			lastAccount, lastType, lastOrder = accountID, projectionType, sessionOrder
		}
		sessions[len(sessions)-1].Items = append(sessions[len(sessions)-1].Items, SessionItem{
			ContentKey: domain.ContentKey{Endpoint: endpoint, StashID: stashID},
			Kind:       kind,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate current sessions: %w", err)
	}
	return sessions, nil
}

func (repository *Repository) CatalogCandidates(ctx context.Context) ([]CatalogCandidate, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT source_endpoint, source_stash_id, candidate_endpoint, candidate_stash_id, reason
		FROM (
			SELECT left_scene.scene_endpoint AS source_endpoint, left_scene.scene_stash_id AS source_stash_id,
				right_scene.scene_endpoint AS candidate_endpoint, right_scene.scene_stash_id AS candidate_stash_id,
				'shared_performer' AS reason
			FROM source_scene_performers AS left_scene
			JOIN source_scene_performers AS right_scene
				ON right_scene.performer_endpoint = left_scene.performer_endpoint
				AND right_scene.performer_stash_id = left_scene.performer_stash_id
				AND (right_scene.scene_endpoint, right_scene.scene_stash_id) <> (left_scene.scene_endpoint, left_scene.scene_stash_id)
			UNION ALL
			SELECT left_scene.scene_endpoint, left_scene.scene_stash_id,
				right_scene.scene_endpoint, right_scene.scene_stash_id, 'shared_tag'
			FROM source_scene_tags AS left_scene
			JOIN source_scene_tags AS right_scene
				ON right_scene.tag_endpoint = left_scene.tag_endpoint
				AND right_scene.tag_stash_id = left_scene.tag_stash_id
				AND (right_scene.scene_endpoint, right_scene.scene_stash_id) <> (left_scene.scene_endpoint, left_scene.scene_stash_id)
			UNION ALL
			SELECT left_scene.scene_endpoint, left_scene.scene_stash_id,
				right_scene.scene_endpoint, right_scene.scene_stash_id, 'shared_group'
			FROM source_scene_groups AS left_scene
			JOIN source_scene_groups AS right_scene
				ON right_scene.group_endpoint = left_scene.group_endpoint
				AND right_scene.group_stash_id = left_scene.group_stash_id
				AND (right_scene.scene_endpoint, right_scene.scene_stash_id) <> (left_scene.scene_endpoint, left_scene.scene_stash_id)
			UNION ALL
			SELECT left_scene.endpoint, left_scene.stash_id, right_scene.endpoint, right_scene.stash_id, 'shared_studio'
			FROM source_scenes AS left_scene
			JOIN source_scenes AS right_scene
				ON right_scene.studio_endpoint = left_scene.studio_endpoint
				AND right_scene.studio_stash_id = left_scene.studio_stash_id
				AND (right_scene.endpoint, right_scene.stash_id) <> (left_scene.endpoint, left_scene.stash_id)
			WHERE left_scene.studio_endpoint IS NOT NULL AND left_scene.studio_stash_id IS NOT NULL
			UNION ALL
			SELECT left_scene.endpoint, left_scene.stash_id, right_scene.endpoint, right_scene.stash_id, 'shared_director'
			FROM source_scenes AS left_scene
			JOIN source_scenes AS right_scene
				ON right_scene.director = left_scene.director
				AND (right_scene.endpoint, right_scene.stash_id) <> (left_scene.endpoint, left_scene.stash_id)
			WHERE NULLIF(BTRIM(left_scene.director), '') IS NOT NULL
			UNION ALL
			SELECT left_scene.endpoint, left_scene.stash_id, right_scene.endpoint, right_scene.stash_id, 'shared_code'
			FROM source_scenes AS left_scene JOIN source_scenes AS right_scene
				ON lower(BTRIM(right_scene.code)) = lower(BTRIM(left_scene.code))
				AND (right_scene.endpoint, right_scene.stash_id) <> (left_scene.endpoint, left_scene.stash_id)
			WHERE NULLIF(BTRIM(left_scene.code), '') IS NOT NULL
			UNION ALL
			SELECT left_scene.endpoint, left_scene.stash_id, right_scene.endpoint, right_scene.stash_id, 'shared_title'
			FROM source_scenes AS left_scene JOIN source_scenes AS right_scene
				ON lower(BTRIM(right_scene.title)) = lower(BTRIM(left_scene.title))
				AND (right_scene.endpoint, right_scene.stash_id) <> (left_scene.endpoint, left_scene.stash_id)
			WHERE NULLIF(BTRIM(left_scene.title), '') IS NOT NULL
			UNION ALL
			SELECT left_scene.endpoint, left_scene.stash_id, right_scene.endpoint, right_scene.stash_id, 'shared_date'
			FROM source_scenes AS left_scene JOIN source_scenes AS right_scene
				ON (right_scene.endpoint, right_scene.stash_id) <> (left_scene.endpoint, left_scene.stash_id)
				AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(left_scene.dates) AS left_date JOIN jsonb_array_elements_text(right_scene.dates) AS right_date ON right_date = left_date)
			UNION ALL
			SELECT left_scene.endpoint, left_scene.stash_id, right_scene.endpoint, right_scene.stash_id, 'similar_duration'
			FROM source_scenes AS left_scene JOIN source_scenes AS right_scene
				ON (right_scene.endpoint, right_scene.stash_id) <> (left_scene.endpoint, left_scene.stash_id)
				AND left_scene.duration IS NOT NULL AND right_scene.duration IS NOT NULL
				AND ABS(right_scene.duration - left_scene.duration) <= GREATEST(60, left_scene.duration / 10)
		) AS candidates
	`)
	if err != nil {
		return nil, fmt.Errorf("query catalog candidates: %w", err)
	}
	defer rows.Close()

	var candidates []CatalogCandidate
	for rows.Next() {
		var candidate CatalogCandidate
		if err := rows.Scan(
			&candidate.Source.Endpoint,
			&candidate.Source.StashID,
			&candidate.Candidate.Endpoint,
			&candidate.Candidate.StashID,
			&candidate.Reason,
		); err != nil {
			return nil, fmt.Errorf("scan catalog candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate catalog candidates: %w", err)
	}
	return candidates, nil
}

func (repository *Repository) CatalogedScenes(ctx context.Context) ([]domain.ContentKey, error) {
	rows, err := repository.pool.Query(ctx, `SELECT endpoint, stash_id FROM source_scenes ORDER BY endpoint, stash_id`)
	if err != nil {
		return nil, fmt.Errorf("query cataloged scenes: %w", err)
	}
	defer rows.Close()
	var scenes []domain.ContentKey
	for rows.Next() {
		var scene domain.ContentKey
		if err := rows.Scan(&scene.Endpoint, &scene.StashID); err != nil {
			return nil, fmt.Errorf("scan cataloged scene: %w", err)
		}
		scenes = append(scenes, scene)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cataloged scenes: %w", err)
	}
	return scenes, nil
}

func (repository *Repository) CatalogScenes(ctx context.Context) ([]CatalogScene, error) {
	rows, err := repository.pool.Query(ctx, `
		WITH scene_features AS (
			SELECT endpoint, stash_id, 'studio:' || studio_endpoint || ':' || studio_stash_id AS feature
			FROM source_scenes
			WHERE studio_endpoint IS NOT NULL AND studio_stash_id IS NOT NULL
			UNION ALL
			SELECT links.scene_endpoint, links.scene_stash_id,
				'performer:' || links.performer_endpoint || ':' || links.performer_stash_id
			FROM source_scene_performers AS links
			UNION ALL
			SELECT links.scene_endpoint, links.scene_stash_id,
				'tag:' || links.tag_endpoint || ':' || links.tag_stash_id
			FROM source_scene_tags AS links
			UNION ALL
			SELECT links.scene_endpoint, links.scene_stash_id,
				'performer_gender:' || lower(btrim(performer.gender))
			FROM source_scene_performers AS links
			JOIN source_performers AS performer
				ON performer.endpoint = links.performer_endpoint AND performer.stash_id = links.performer_stash_id
			WHERE NULLIF(btrim(performer.gender), '') IS NOT NULL
			UNION ALL
			SELECT links.scene_endpoint, links.scene_stash_id,
				'performer_ethnicity:' || lower(btrim(performer.ethnicity))
			FROM source_scene_performers AS links
			JOIN source_performers AS performer
				ON performer.endpoint = links.performer_endpoint AND performer.stash_id = links.performer_stash_id
			WHERE NULLIF(btrim(performer.ethnicity), '') IS NOT NULL
			UNION ALL
			SELECT links.scene_endpoint, links.scene_stash_id,
				'performer_country:' || lower(btrim(performer.country))
			FROM source_scene_performers AS links
			JOIN source_performers AS performer
				ON performer.endpoint = links.performer_endpoint AND performer.stash_id = links.performer_stash_id
			WHERE NULLIF(btrim(performer.country), '') IS NOT NULL
		)
		SELECT scenes.endpoint, scenes.stash_id,
			COALESCE(array_agg(scene_features.feature) FILTER (WHERE scene_features.feature IS NOT NULL), ARRAY[]::text[])
		FROM source_scenes AS scenes
		LEFT JOIN scene_features ON scene_features.endpoint = scenes.endpoint AND scene_features.stash_id = scenes.stash_id
		GROUP BY scenes.endpoint, scenes.stash_id
		ORDER BY scenes.endpoint, scenes.stash_id
	`)
	if err != nil {
		return nil, fmt.Errorf("query catalog scene features: %w", err)
	}
	defer rows.Close()

	var scenes []CatalogScene
	for rows.Next() {
		var scene CatalogScene
		if err := rows.Scan(&scene.ContentKey.Endpoint, &scene.ContentKey.StashID, &scene.Features); err != nil {
			return nil, fmt.Errorf("scan catalog scene features: %w", err)
		}
		scenes = append(scenes, scene)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate catalog scene features: %w", err)
	}
	return scenes, nil
}

func (repository *Repository) SaveAndActivateVectors(ctx context.Context, projection VectorProjection) (string, error) {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", fmt.Errorf("begin vector recommendation transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(81102535415121123)); err != nil {
		return "", fmt.Errorf("lock vector recommendation build: %w", err)
	}

	var versionID string
	if err := tx.QueryRow(ctx, `INSERT INTO model_versions (active) VALUES (false) RETURNING id`).Scan(&versionID); err != nil {
		return "", fmt.Errorf("create inactive vector model version: %w", err)
	}
	if err := insertSceneVectors(ctx, tx, versionID, projection.SceneVectors); err != nil {
		return "", err
	}
	if err := insertAccountProfiles(ctx, tx, versionID, projection.Profiles); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE model_versions SET active = false WHERE active`); err != nil {
		return "", fmt.Errorf("deactivate prior model version: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE model_versions SET active = true, activated_at = now() WHERE id = $1`, versionID); err != nil {
		return "", fmt.Errorf("activate vector model version: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM model_scene_vectors WHERE model_version_id <> $1`, versionID); err != nil {
		return "", fmt.Errorf("prune inactive scene vectors: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit vector recommendation projection: %w", err)
	}
	return versionID, nil
}

func insertAccountProfiles(ctx context.Context, tx pgx.Tx, versionID string, profiles []AccountProfile) error {
	for _, profile := range profiles {
		reasons, err := json.Marshal(profile.Reasons)
		if err != nil {
			return fmt.Errorf("encode account profile reasons: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO model_account_profiles (model_version_id, account_id, embedding, reasons)
			VALUES ($1, $2, $3::vector, $4)
		`, versionID, profile.AccountID, vectorLiteral(profile.Embedding), reasons); err != nil {
			return fmt.Errorf("insert account profile: %w", err)
		}
	}
	return nil
}

func insertSceneVectors(ctx context.Context, tx pgx.Tx, versionID string, vectors []SceneEmbedding) error {
	batch := &pgx.Batch{}
	for _, vector := range vectors {
		batch.Queue(`
			INSERT INTO model_scene_vectors (model_version_id, endpoint, stash_id, embedding)
			VALUES ($1, $2, $3, $4::vector)
		`, versionID, vector.ContentKey.Endpoint, vector.ContentKey.StashID, vectorLiteral(vector.Embedding))
	}
	results := tx.SendBatch(ctx, batch)
	defer results.Close()
	for range vectors {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("insert scene vector: %w", err)
		}
	}
	return nil
}

func (repository *Repository) SaveAndActivate(ctx context.Context, projection Projection) (string, error) {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", fmt.Errorf("begin recommendation transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(81102535415121123)); err != nil {
		return "", fmt.Errorf("lock recommendation build: %w", err)
	}

	var versionID string
	if err := tx.QueryRow(ctx, `INSERT INTO model_versions (active) VALUES (false) RETURNING id`).Scan(&versionID); err != nil {
		return "", fmt.Errorf("create inactive model version: %w", err)
	}
	for _, neighbor := range projection.Neighbors {
		reasons, err := json.Marshal(neighbor.Reasons)
		if err != nil {
			return "", fmt.Errorf("encode neighbor reasons: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO item_neighbors (
				model_version_id, source_endpoint, source_stash_id,
				neighbor_endpoint, neighbor_stash_id, score, reasons
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, versionID, neighbor.Source.Endpoint, neighbor.Source.StashID, neighbor.Candidate.Endpoint, neighbor.Candidate.StashID, neighbor.Score, reasons); err != nil {
			return "", fmt.Errorf("insert item neighbor: %w", err)
		}
	}
	for _, recommendation := range projection.UserRecommendations {
		reasons, err := json.Marshal(recommendation.Reasons)
		if err != nil {
			return "", fmt.Errorf("encode user recommendation reasons: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_recommendations (
				model_version_id, account_id, source_endpoint, source_stash_id, score, reasons
			) VALUES ($1, $2, $3, $4, $5, $6)
		`, versionID, recommendation.AccountID, recommendation.ContentKey.Endpoint, recommendation.ContentKey.StashID, recommendation.Score, reasons); err != nil {
			return "", fmt.Errorf("insert user recommendation: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE model_versions SET active = false WHERE active`); err != nil {
		return "", fmt.Errorf("deactivate prior model version: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE model_versions SET active = true, activated_at = now() WHERE id = $1`, versionID); err != nil {
		return "", fmt.Errorf("activate model version: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit recommendation projection: %w", err)
	}
	return versionID, nil
}

func (repository *Repository) Related(ctx context.Context, source domain.ContentKey, limit int) ([]Recommendation, string, error) {
	version, found, err := repository.activeVersion(ctx)
	if err != nil || !found {
		return nil, version, err
	}
	return repository.readRecommendations(ctx, `
		WITH source AS (
			SELECT embedding
			FROM model_scene_vectors
			WHERE model_version_id = $1 AND endpoint = $2 AND stash_id = $3
		)
		SELECT candidates.endpoint, candidates.stash_id,
			1 - (candidates.embedding <=> source.embedding) AS score,
			'["content_similarity"]'::jsonb AS reasons
		FROM model_scene_vectors AS candidates
		CROSS JOIN source
		WHERE candidates.model_version_id = $1
			AND (candidates.endpoint, candidates.stash_id) <> ($2, $3)
		ORDER BY candidates.embedding <=> source.embedding, candidates.endpoint, candidates.stash_id
		LIMIT $4
	`, version, source.Endpoint, source.StashID, normalizeLimit(limit))
}

func (repository *Repository) ForYou(ctx context.Context, accountID string, limit, offset int) ([]Recommendation, string, error) {
	version, found, err := repository.activeVersion(ctx)
	if err != nil || !found {
		return nil, version, err
	}
	return repository.readRecommendations(ctx, `
		SELECT candidates.endpoint, candidates.stash_id,
			1 - (candidates.embedding <=> profile.embedding) AS score,
			profile.reasons
		FROM model_account_profiles AS profile
		JOIN model_scene_vectors AS candidates
			ON candidates.model_version_id = profile.model_version_id
		WHERE profile.model_version_id = $1 AND profile.account_id = $2
		ORDER BY candidates.embedding <=> profile.embedding, candidates.endpoint, candidates.stash_id
		LIMIT $3
		OFFSET $4
	`, version, accountID, normalizeLimit(limit), max(offset, 0))
}

func (repository *Repository) activeVersion(ctx context.Context) (string, bool, error) {
	var version string
	err := repository.pool.QueryRow(ctx, `SELECT id FROM model_versions WHERE active`).Scan(&version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("query active model version: %w", err)
	}
	return version, true, nil
}

func (repository *Repository) readRecommendations(ctx context.Context, query string, arguments ...any) ([]Recommendation, string, error) {
	rows, err := repository.pool.Query(ctx, query, arguments...)
	if err != nil {
		return nil, "", fmt.Errorf("query recommendations: %w", err)
	}
	defer rows.Close()

	// The selected content key is the first two columns. Canonical URLs are
	// fetched separately so absent catalog metadata remains a valid result.
	var raw []struct {
		key     domain.ContentKey
		score   float64
		reasons []byte
	}
	for rows.Next() {
		var row struct {
			key     domain.ContentKey
			score   float64
			reasons []byte
		}
		if err := rows.Scan(&row.key.Endpoint, &row.key.StashID, &row.score, &row.reasons); err != nil {
			return nil, "", fmt.Errorf("scan recommendation: %w", err)
		}
		raw = append(raw, row)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate recommendations: %w", err)
	}

	version := fmt.Sprint(arguments[0])
	items := make([]Recommendation, 0, len(raw))
	for _, row := range raw {
		var reasons []string
		if err := json.Unmarshal(row.reasons, &reasons); err != nil {
			return nil, "", fmt.Errorf("decode recommendation reasons: %w", err)
		}
		canonicalURL, err := repository.canonicalURL(ctx, row.key)
		if err != nil {
			return nil, "", err
		}
		items = append(items, Recommendation{ContentKey: row.key, Score: row.score, Reasons: reasons, ModelVersion: version, CanonicalURL: canonicalURL})
	}
	return items, version, nil
}

func (repository *Repository) canonicalURL(ctx context.Context, key domain.ContentKey) (*string, error) {
	var template *string
	err := repository.pool.QueryRow(ctx, `SELECT canonical_scene_url_template FROM source_configs WHERE endpoint = $1`, key.Endpoint).Scan(&template)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query canonical URL: %w", err)
	}
	if template == nil || strings.TrimSpace(*template) == "" {
		return nil, nil
	}
	value := strings.ReplaceAll(*template, "{stash_id}", key.StashID)
	value = strings.ReplaceAll(value, "{id}", key.StashID)
	return &value, nil
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	return int(math.Min(float64(limit), 100))
}
