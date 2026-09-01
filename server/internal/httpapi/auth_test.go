package httpapi_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/treehorn/stash-recommendations/server/internal/httpapi"
	"github.com/treehorn/stash-recommendations/server/internal/store"
)

func TestAuthenticateAcceptsOnlyTheOwningAccountKey(t *testing.T) {
	repository := openTestAccountRepository(t)
	firstAccount, err := repository.CreateAccount(context.Background())
	require.NoError(t, err)
	secondAccount, err := repository.CreateAccount(context.Background())
	require.NoError(t, err)

	handler := httpapi.RequireAccount(repository)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authenticatedAccount, ok := httpapi.AccountFromContext(r.Context())
		require.True(t, ok)
		_, err := w.Write([]byte(authenticatedAccount.ID))
		require.NoError(t, err)
	}))

	require.Equal(t, firstAccount.ID, authenticatedAccountID(t, handler, firstAccount.PlaintextKey))
	require.Equal(t, secondAccount.ID, authenticatedAccountID(t, handler, secondAccount.PlaintextKey))

	invalidRequest := httptest.NewRequest(http.MethodGet, "/protected", nil)
	invalidRequest.Header.Set("Authorization", "Bearer not-an-issued-key")
	invalidRecorder := httptest.NewRecorder()
	handler.ServeHTTP(invalidRecorder, invalidRequest)
	require.Equal(t, http.StatusUnauthorized, invalidRecorder.Code)
}

func authenticatedAccountID(t *testing.T, handler http.Handler, plaintextKey string) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer "+plaintextKey)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	body, err := io.ReadAll(recorder.Result().Body)
	require.NoError(t, err)
	return string(body)
}

func openTestAccountRepository(t *testing.T) *store.Store {
	t.Helper()
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is required for PostgreSQL integration tests")
	}

	repository, err := store.Open(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(func() { repository.Close(context.Background()) })
	require.NoError(t, repository.Migrate(context.Background()))
	return repository
}
