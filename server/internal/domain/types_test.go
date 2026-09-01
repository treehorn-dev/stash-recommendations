package domain

import (
	"encoding/json"
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

func TestSourceSnapshotDecodeRejectsPrivacyFields(t *testing.T) {
	for _, field := range []string{"paths", "files", "rating100", "play_count", "custom_fields"} {
		t.Run(field, func(t *testing.T) {
			var snapshot SourceSnapshot
			err := json.Unmarshal([]byte(`{
				"schema_version": 1,
				"content_key": {"endpoint": "https://box.example/graphql", "stash_id": "scene-1"},
				"captured_at": "2026-08-30T00:00:00Z",
				"scenes": [{"id": "scene-1", "`+field+`": "private"}],
				"performers": []
			}`), &snapshot)

			require.Error(t, err)
			require.Contains(t, err.Error(), field)
		})
	}
}

func TestSourceSnapshotValidateRequiresArraysAndNestedRequiredFields(t *testing.T) {
	snapshot := SourceSnapshot{
		SchemaVersion: 1,
		ContentKey:    ContentKey{Endpoint: "https://box.example/graphql", StashID: "scene-1"},
		CapturedAt:    time.Now().UTC(),
	}
	require.Error(t, snapshot.Validate())

	snapshot.Scenes = []Scene{{}}
	snapshot.Performers = []Performer{{ID: "performer-1"}}
	require.Error(t, snapshot.Validate())
}

func TestSourceSnapshotMarshalRejectsNilArrays(t *testing.T) {
	snapshot := SourceSnapshot{
		SchemaVersion: 1,
		ContentKey:    ContentKey{Endpoint: "https://box.example/graphql", StashID: "scene-1"},
		CapturedAt:    time.Now().UTC(),
	}

	_, err := json.Marshal(snapshot)

	require.Error(t, err)
}

func pointerTo(value float64) *float64 {
	return &value
}
