package httpapi_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/treehorn/stash-recommendations/server/internal/catalog"
	"github.com/treehorn/stash-recommendations/server/internal/httpapi"
	"github.com/treehorn/stash-recommendations/server/internal/ingest"
)

func TestPreferenceEventFixturesPostThroughHTTPAPI(t *testing.T) {
	repository := openInteractionHTTPStore(t)
	handler := httpapi.NewMux(httpapi.Dependencies{
		AccountRepository:  repository,
		InteractionService: ingest.NewInteractionService(repository),
	})

	for _, fixture := range []struct {
		name       string
		statusCode int
	}{
		{name: "preference-event.valid.json", statusCode: 202},
		{name: "interaction-event.played.valid.json", statusCode: 202},
		{name: "interaction-event.o.valid.json", statusCode: 202},
		{name: "interaction-event.uppercase-endpoint.valid.json", statusCode: 202},
		{name: "preference-event.invalid.json", statusCode: 400},
		{name: "preference-event.boolean-rating.invalid.json", statusCode: 400},
		{name: "interaction-event.with-rating.invalid.json", statusCode: 400},
		{name: "interaction-event.non-utc-timestamp.invalid.json", statusCode: 400},
		{name: "interaction-event.credential-endpoint.invalid.json", statusCode: 400},
		{name: "interaction-event.query-fragment-endpoint.invalid.json", statusCode: 400},
		{name: "interaction-event.http-endpoint.invalid.json", statusCode: 400},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			account, err := repository.CreateAccount(context.Background())
			require.NoError(t, err)

			recorder := postInteraction(t, handler, account.PlaintextKey, readV1Fixture(t, fixture.name))

			require.Equal(t, fixture.statusCode, recorder.Code, recorder.Body.String())
		})
	}
}

func TestSourceSnapshotFixturesPostThroughHTTPAPI(t *testing.T) {
	repository := openInteractionHTTPStore(t)
	handler := httpapi.NewMux(httpapi.Dependencies{
		AccountRepository: repository,
		SnapshotService:   catalog.NewSnapshotService(repository),
	})

	for _, fixture := range []struct {
		name       string
		statusCode int
	}{
		{name: "source-snapshot.valid.json", statusCode: 202},
		{name: "source-snapshot.group.valid.json", statusCode: 202},
		{name: "source-snapshot.missing-appearance-performer.invalid.json", statusCode: 400},
		{name: "source-snapshot.missing-source-updated-at.invalid.json", statusCode: 400},
		{name: "source-snapshot.boolean-duration.invalid.json", statusCode: 400},
		{name: "source-snapshot.scalar-dates.invalid.json", statusCode: 400},
		{name: "source-snapshot.invalid-date.invalid.json", statusCode: 400},
		{name: "source-snapshot.nested-null.invalid.json", statusCode: 400},
		{name: "source-snapshot.boolean-career-years.invalid.json", statusCode: 400},
		{name: "source-snapshot.invalid-remote-url.invalid.json", statusCode: 400},
		{name: "source-snapshot.credential-remote-image.invalid.json", statusCode: 400},
		{name: "source-snapshot.query-fragment-remote-reference.invalid.json", statusCode: 400},
		{name: "source-snapshot.http-remote-reference.invalid.json", statusCode: 400},
		{name: "source-snapshot.empty-query-remote-reference.invalid.json", statusCode: 400},
		{name: "source-snapshot.empty-fragment-remote-reference.invalid.json", statusCode: 400},
		{name: "source-snapshot.blank-group.invalid.json", statusCode: 400},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			account, err := repository.CreateAccount(context.Background())
			require.NoError(t, err)

			recorder := postSnapshot(handler, account.PlaintextKey, readV1Fixture(t, fixture.name))

			require.Equal(t, fixture.statusCode, recorder.Code, recorder.Body.String())
		})
	}
}

func readV1Fixture(t *testing.T, name string) []byte {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	path := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "contracts", "v1", "fixtures", name)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
