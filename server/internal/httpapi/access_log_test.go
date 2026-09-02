package httpapi_test

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/treehorn/stash-recommendations/server/internal/httpapi"
)

func TestAccessLogRecordsRequestMetadataWithoutCredentialsOrQuery(t *testing.T) {
	var output bytes.Buffer
	logger := log.New(&output, "", 0)
	handler := httpapi.AccessLog(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}), logger)

	request := httptest.NewRequest(http.MethodPost, "/v1/events/interactions?token=secret", nil)
	request.Header.Set("Authorization", "Bearer secret-api-key")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	line := output.String()
	if !strings.Contains(line, "method=POST path=/v1/events/interactions status=202") {
		t.Fatalf("access log = %q", line)
	}
	if strings.Contains(line, "secret") {
		t.Fatalf("access log leaked secret: %q", line)
	}
}
