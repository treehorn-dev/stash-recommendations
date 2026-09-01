package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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
				"source_updated_at": "2026-08-30T00:00:00Z",
				"scenes": [{"id": "scene-1", "`+field+`": "private"}],
				"performers": []
			}`), &snapshot)

			require.Error(t, err)
			require.Contains(t, err.Error(), field)
		})
	}
}

func TestSourceSnapshotDecodeAcceptsNamedGroups(t *testing.T) {
	var snapshot SourceSnapshot
	err := json.Unmarshal([]byte(`{
		"schema_version": 1,
		"content_key": {"endpoint": "https://box.example/graphql", "stash_id": "scene-1"},
		"captured_at": "2026-08-30T00:00:00Z",
		"source_updated_at": "2026-08-30T00:00:00Z",
		"scenes": [{"id": "scene-1", "groups": [{"id": "group-1", "name": "Series"}]}],
		"performers": []
	}`), &snapshot)

	require.NoError(t, err)
	require.Equal(t, []Group{{ID: "group-1", Name: "Series"}}, snapshot.Scenes[0].Groups)
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

func TestPreferenceEventValidateSupportsInteractionKinds(t *testing.T) {
	for _, kind := range []string{"scene.played", "scene.o"} {
		t.Run(kind, func(t *testing.T) {
			event := PreferenceEvent{
				SchemaVersion: 1,
				EventID:       "550e8400-e29b-41d4-a716-446655440000",
				ClientID:      "550e8400-e29b-41d4-a716-446655440001",
				Sequence:      7,
				OccurredAt:    time.Now().UTC(),
				ContentKey:    ContentKey{Endpoint: "https://box.example/graphql", StashID: "scene-1"},
				Kind:          kind,
				Origin:        "hook",
			}

			require.NoError(t, event.Validate())
			event.Rating = pointerTo(0.5)
			require.Error(t, event.Validate())
		})
	}
}

func TestPreferenceEventDecodeRejectsLocalFields(t *testing.T) {
	for _, field := range []string{"local_id", "duration", "resume_time", "file", "player"} {
		t.Run(field, func(t *testing.T) {
			var event PreferenceEvent
			err := json.Unmarshal([]byte(`{
				"schema_version": 1,
				"event_id": "550e8400-e29b-41d4-a716-446655440000",
				"client_id": "550e8400-e29b-41d4-a716-446655440001",
				"sequence": 7,
				"occurred_at": "2026-08-30T00:00:00Z",
				"content_key": {"endpoint": "https://box.example/graphql", "stash_id": "scene-1"},
				"kind": "scene.played",
				"origin": "history",
				"`+field+`": "private"
			}`), &event)

			require.Error(t, err)
			require.Contains(t, err.Error(), field)
		})
	}
}

func TestV1FixturesHaveCrossLanguageContractParity(t *testing.T) {
	for _, fixture := range []struct {
		name  string
		valid bool
	}{
		{name: "preference-event.valid.json", valid: true},
		{name: "preference-event.invalid.json", valid: false},
		{name: "preference-event.boolean-rating.invalid.json", valid: false},
		{name: "interaction-event.played.valid.json", valid: true},
		{name: "interaction-event.o.valid.json", valid: true},
		{name: "interaction-event.with-rating.invalid.json", valid: false},
		{name: "interaction-event.non-utc-timestamp.invalid.json", valid: false},
		{name: "interaction-event.credential-endpoint.invalid.json", valid: false},
		{name: "interaction-event.query-fragment-endpoint.invalid.json", valid: false},
		{name: "interaction-event.http-endpoint.invalid.json", valid: false},
		{name: "interaction-event.uppercase-endpoint.valid.json", valid: true},
		{name: "interaction-event.empty-query-endpoint.invalid.json", valid: false},
		{name: "interaction-event.empty-fragment-endpoint.invalid.json", valid: false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			data, err := os.ReadFile(v1FixturePath(t, fixture.name))
			require.NoError(t, err)

			var event PreferenceEvent
			err = json.Unmarshal(data, &event)
			if fixture.valid {
				require.NoError(t, err)
				require.NoError(t, event.Validate())
			} else {
				require.Error(t, err)
			}
		})
	}

	for _, fixture := range []struct {
		name  string
		valid bool
	}{
		{name: "source-snapshot.valid.json", valid: true},
		{name: "source-snapshot.group.valid.json", valid: true},
		{name: "source-snapshot.missing-appearance-performer.invalid.json", valid: false},
		{name: "source-snapshot.missing-source-updated-at.invalid.json", valid: false},
		{name: "source-snapshot.boolean-duration.invalid.json", valid: false},
		{name: "source-snapshot.scalar-dates.invalid.json", valid: false},
		{name: "source-snapshot.invalid-date.invalid.json", valid: false},
		{name: "source-snapshot.nested-null.invalid.json", valid: false},
		{name: "source-snapshot.boolean-career-years.invalid.json", valid: false},
		{name: "source-snapshot.invalid-remote-url.invalid.json", valid: false},
		{name: "source-snapshot.credential-remote-image.invalid.json", valid: false},
		{name: "source-snapshot.query-fragment-remote-reference.invalid.json", valid: false},
		{name: "source-snapshot.http-remote-reference.invalid.json", valid: false},
		{name: "source-snapshot.empty-query-remote-reference.invalid.json", valid: false},
		{name: "source-snapshot.empty-fragment-remote-reference.invalid.json", valid: false},
		{name: "source-snapshot.blank-group.invalid.json", valid: false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			data, err := os.ReadFile(v1FixturePath(t, fixture.name))
			require.NoError(t, err)

			var snapshot SourceSnapshot
			err = json.Unmarshal(data, &snapshot)
			if fixture.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestUppercaseEndpointFixtureNormalizes(t *testing.T) {
	data, err := os.ReadFile(v1FixturePath(t, "interaction-event.uppercase-endpoint.valid.json"))
	require.NoError(t, err)

	var event PreferenceEvent
	require.NoError(t, json.Unmarshal(data, &event))
	require.Equal(t, "https://box.example/GRAPHQL", event.ContentKey.Endpoint)
}

func v1FixturePath(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "contracts", "v1", "fixtures", name)
}

func pointerTo(value float64) *float64 {
	return &value
}
