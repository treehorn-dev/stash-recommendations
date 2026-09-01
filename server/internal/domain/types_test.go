package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPreferenceEventValidateNormalizesV1RatingSet(t *testing.T) {
	event := PreferenceEvent{
		SchemaVersion: 1,
		EventID:       "550e8400-e29b-41d4-a716-446655440000",
		ClientID:      "550e8400-e29b-41d4-a716-446655440001",
		Sequence:      7,
		OccurredAt:    time.Now().UTC(),
		ContentKey: ContentKey{
			Endpoint: "HTTPS://BOX.EXAMPLE/GRAPHQL/",
			StashID:  "scene-1",
		},
		Kind:   PreferenceEventKindSceneRatingSet,
		Rating: pointerTo(0.75),
		Origin: "hook",
	}

	require.NoError(t, event.Validate())
	require.Equal(t, "https://box.example/GRAPHQL", event.ContentKey.Endpoint)
}

func TestPreferenceEventValidateAcceptsUUIDVersionSeven(t *testing.T) {
	event := PreferenceEvent{
		SchemaVersion: 1,
		EventID:       "018f8d8e-4f35-7c9e-8b4c-7c7b0f4d1f4a",
		ClientID:      "018f8d8e-4f35-7c9e-8b4c-7c7b0f4d1f4b",
		Sequence:      1,
		OccurredAt:    time.Now().UTC(),
		ContentKey:    ContentKey{Endpoint: "https://box.example/graphql", StashID: "scene-1"},
		Kind:          PreferenceEventKindSceneRatingRemove,
		Origin:        "hook",
	}

	require.NoError(t, event.Validate())
}

func pointerTo(value float64) *float64 {
	return &value
}
