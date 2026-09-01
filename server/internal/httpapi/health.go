package httpapi

import "net/http"

// NewMux provides the service's HTTP routes.
func NewMux(deps ...Dependencies) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	if len(deps) > 0 && deps[0].AccountRepository != nil {
		if deps[0].InteractionService != nil {
			mux.Handle("POST /v1/events/interactions", RequireAccount(deps[0].AccountRepository)(PostInteractions(deps[0].InteractionService)))
		}
		if deps[0].SnapshotService != nil {
			mux.Handle("POST /v1/catalog/snapshots", RequireAccount(deps[0].AccountRepository)(PostSnapshots(deps[0].SnapshotService)))
		}
	}
	return mux
}
