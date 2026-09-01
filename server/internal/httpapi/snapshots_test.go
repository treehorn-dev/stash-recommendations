package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/treehorn/stash-recommendations/server/internal/catalog"
	"github.com/treehorn/stash-recommendations/server/internal/httpapi"
)

func TestPostSnapshotsReturnsAcceptedForAuthenticatedValidPayload(t *testing.T) {
	repository := openInteractionHTTPStore(t)
	account, err := repository.CreateAccount(context.Background())
	require.NoError(t, err)

	handler := httpapi.NewMux(httpapi.Dependencies{
		AccountRepository: repository,
		SnapshotService:   catalog.NewSnapshotService(repository),
	})

	recorder := postSnapshot(handler, account.PlaintextKey, snapshotRequestJSON(t, "2026-08-30T10:00:00Z", "Example Scene"))
	require.Equal(t, http.StatusAccepted, recorder.Code)
}

func TestPostSnapshotsRejectsMalformedAndUnauthenticatedRequests(t *testing.T) {
	repository := openInteractionHTTPStore(t)
	account, err := repository.CreateAccount(context.Background())
	require.NoError(t, err)

	handler := httpapi.NewMux(httpapi.Dependencies{
		AccountRepository: repository,
		SnapshotService:   catalog.NewSnapshotService(repository),
	})

	recorder := postSnapshot(handler, account.PlaintextKey, []byte(`{"schema_version":1,"content_key":{"endpoint":"https://box.example/graphql","stash_id":"scene-1"},"captured_at":"2026-08-30T10:00:00Z","source_updated_at":"2026-08-30T10:00:00Z","scenes":[{"id":"scene-1","paths":["/private/scene.mp4"]}],"performers":[]}`))
	require.Equal(t, http.StatusBadRequest, recorder.Code)

	recorder = postSnapshot(handler, "", snapshotRequestJSON(t, "2026-08-30T10:00:00Z", "Example Scene"))
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Equal(t, "Bearer", recorder.Header().Get("WWW-Authenticate"))
}

func TestPostSnapshotsRejectsOversizedBodies(t *testing.T) {
	repository := openInteractionHTTPStore(t)
	account, err := repository.CreateAccount(context.Background())
	require.NoError(t, err)

	handler := httpapi.NewMux(httpapi.Dependencies{
		AccountRepository: repository,
		SnapshotService:   catalog.NewSnapshotService(repository),
	})
	body := bytes.Repeat([]byte("x"), 2<<20)

	t.Run("declared length", func(t *testing.T) {
		recorder := postSnapshot(handler, account.PlaintextKey, body)
		require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	})

	t.Run("chunked", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/v1/catalog/snapshots", io.NopCloser(bytes.NewReader(body)))
		request.ContentLength = -1
		request.TransferEncoding = []string{"chunked"}
		request.Header.Set("Authorization", "Bearer "+account.PlaintextKey)
		recorder := httptest.NewRecorder()

		handler.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	})
}

func postSnapshot(handler http.Handler, plaintextKey string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/v1/catalog/snapshots", bytes.NewReader(body))
	if plaintextKey != "" {
		request.Header.Set("Authorization", "Bearer "+plaintextKey)
	}
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func snapshotRequestJSON(t *testing.T, capturedAt string, title string) []byte {
	t.Helper()
	payload := map[string]any{
		"schema_version": 1,
		"content_key": map[string]any{
			"endpoint": "https://box.example/graphql",
			"stash_id": "scene-1",
		},
		"captured_at":       capturedAt,
		"source_updated_at": capturedAt,
		"scenes": []map[string]any{{
			"id":    "scene-1",
			"title": title,
		}},
		"performers": []map[string]any{},
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return data
}
