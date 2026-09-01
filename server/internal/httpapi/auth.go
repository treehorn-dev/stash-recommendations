package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/treehorn/stash-recommendations/server/internal/store"
)

type accountContextKey struct{}

// RequireAccount rejects requests without a valid bearer API key.
func RequireAccount(repository store.AccountRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			plaintextKey, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				unauthorized(w)
				return
			}
			account, err := repository.Authenticate(r.Context(), plaintextKey)
			if err != nil {
				if errors.Is(err, store.ErrInvalidAPIKey) {
					unauthorized(w)
					return
				}
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), accountContextKey{}, account)))
		})
	}
}

// AccountFromContext returns the account authenticated by RequireAccount.
func AccountFromContext(ctx context.Context) (store.Account, bool) {
	account, ok := ctx.Value(accountContextKey{}).(store.Account)
	return account, ok
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}
