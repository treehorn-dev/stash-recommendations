package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestRecommendationOffsetDefaultsAndRejectsInvalidValues(t *testing.T) {
	request := httptest.NewRequest("GET", "/v1/recommendations/for-you", nil)
	if got, err := recommendationOffset(request); err != nil || got != 0 {
		t.Fatalf("default offset = %d, %v", got, err)
	}
	request = httptest.NewRequest("GET", "/v1/recommendations/for-you?offset=100", nil)
	if got, err := recommendationOffset(request); err != nil || got != 100 {
		t.Fatalf("offset = %d, %v", got, err)
	}
	request = httptest.NewRequest("GET", "/v1/recommendations/for-you?offset=-1", nil)
	if _, err := recommendationOffset(request); err == nil {
		t.Fatal("negative offset accepted")
	}
}
