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

func TestRecommendationFiltersParseNumericPredicates(t *testing.T) {
	request := httptest.NewRequest("GET", "/v1/recommendations/for-you?rating_operator=gte&rating_value=4&o_count_operator=not_null", nil)
	filters, err := recommendationFilters(request)
	if err != nil {
		t.Fatal(err)
	}
	if filters.Rating.Operator != "gte" || filters.Rating.Value != 4 || filters.OCount.Operator != "not_null" {
		t.Fatalf("unexpected filters: %#v", filters)
	}
}

func TestRecommendationFiltersRejectValuesForNullPredicates(t *testing.T) {
	request := httptest.NewRequest("GET", "/v1/recommendations/for-you?rating_operator=is_null&rating_value=4", nil)
	_, err := recommendationFilters(request)
	if err == nil {
		t.Fatal("null predicate accepted a numeric value")
	}
}
