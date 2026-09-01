package httpapi_test

import (
	"context"
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
	account, err := repository.CreateAccount(context.Background())
	require.NoError(t, err)

	handler := httpapi.RequireAccount(repository)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authenticatedAccount, ok := httpapi.AccountFromContext(r.Context())
		require.True(t, ok)
		require.Equal(t, account.ID, authenticatedAccount.ID)
		w.WriteHeader(http.StatusNoContent)
	}))

	validRequest := httptest.NewRequest(http.MethodGet, "/protected", nil)
	validRequest.Header.Set("Authorization", "Bearer "+account.PlaintextKey)
	validRecorder := httptest.NewRecorder()
	handler.ServeHTTP(validRecorder, validRequest)
	require.Equal(t, http.StatusNoContent, validRecorder.Code)

	invalidRequest := httptest.NewRequest(http.MethodGet, "/protected", nil)
	invalidRequest.Header.Set("Authorization", "Bearer not-an-issued-key")
	invalidRecorder := httptest.NewRecorder()
	handler.ServeHTTP(invalidRecorder, invalidRequest)
	require.Equal(t, http.StatusUnauthorized, invalidRecorder.Code)
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
