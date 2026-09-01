package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	PreferenceEventKindSceneRatingSet    = "scene.rating.set"
	PreferenceEventKindSceneRatingRemove = "scene.rating.remove"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type ContentKey struct {
	Endpoint string `json:"endpoint"`
	StashID  string `json:"stash_id"`
}

// Normalize canonicalizes the Stash endpoint while retaining its path.
func (ContentKey) Normalize(endpoint, stashID string) (ContentKey, error) {
	if strings.TrimSpace(stashID) == "" {
		return ContentKey{}, fmt.Errorf("stash ID is required")
	}

	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ContentKey{}, fmt.Errorf("endpoint must be an absolute URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	if strings.HasSuffix(parsed.Path, "/") {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	}

	return ContentKey{Endpoint: parsed.String(), StashID: stashID}, nil
}

func (key *ContentKey) NormalizeInPlace() error {
	normalized, err := (ContentKey{}).Normalize(key.Endpoint, key.StashID)
	if err != nil {
		return err
	}
	*key = normalized
	return nil
}

func (key *ContentKey) UnmarshalJSON(data []byte) error {
	type contentKey ContentKey
	var value contentKey
	if err := strictUnmarshalJSON(data, &value); err != nil {
		return err
	}
	*key = ContentKey(value)
	return nil
}

type PreferenceEvent struct {
	SchemaVersion int        `json:"schema_version"`
	EventID       string     `json:"event_id"`
	ClientID      string     `json:"client_id"`
	Sequence      int64      `json:"sequence"`
	OccurredAt    time.Time  `json:"occurred_at"`
	ContentKey    ContentKey `json:"content_key"`
	Kind          string     `json:"kind"`
	Rating        *float64   `json:"rating,omitempty"`
	Origin        string     `json:"origin"`
}

func (event *PreferenceEvent) Validate() error {
	if event.SchemaVersion != 1 {
		return fmt.Errorf("schema_version must be 1")
	}
	if !uuidPattern.MatchString(event.EventID) {
		return fmt.Errorf("event_id must be a UUID")
	}
	if !uuidPattern.MatchString(event.ClientID) {
		return fmt.Errorf("client_id must be a UUID")
	}
	if event.Sequence < 1 {
		return fmt.Errorf("sequence must be at least 1")
	}
	if event.OccurredAt.IsZero() {
		return fmt.Errorf("occurred_at is required")
	}
	if err := event.ContentKey.NormalizeInPlace(); err != nil {
		return fmt.Errorf("content_key: %w", err)
	}

	switch event.Kind {
	case PreferenceEventKindSceneRatingSet:
		if event.Rating == nil || *event.Rating < 0 || *event.Rating > 1 {
			return fmt.Errorf("scene.rating.set requires rating between 0 and 1")
		}
	case PreferenceEventKindSceneRatingRemove:
		if event.Rating != nil {
			return fmt.Errorf("scene.rating.remove prohibits rating")
		}
	default:
		return fmt.Errorf("unsupported preference event kind %q", event.Kind)
	}
	if strings.TrimSpace(event.Origin) == "" {
		return fmt.Errorf("origin is required")
	}
	return nil
}

type SourceSnapshot struct {
	SchemaVersion int         `json:"schema_version"`
	ContentKey    ContentKey  `json:"content_key"`
	CapturedAt    time.Time   `json:"captured_at"`
	Scenes        []Scene     `json:"scenes"`
	Performers    []Performer `json:"performers"`
}

func (snapshot *SourceSnapshot) Validate() error {
	if snapshot.SchemaVersion != 1 {
		return fmt.Errorf("schema_version must be 1")
	}
	if snapshot.CapturedAt.IsZero() {
		return fmt.Errorf("captured_at is required")
	}
	if err := snapshot.ContentKey.NormalizeInPlace(); err != nil {
		return fmt.Errorf("content_key: %w", err)
	}
	if snapshot.Scenes == nil || snapshot.Performers == nil {
		return fmt.Errorf("scenes and performers must be arrays")
	}
	for index := range snapshot.Scenes {
		if err := snapshot.Scenes[index].Validate(); err != nil {
			return fmt.Errorf("scenes[%d]: %w", index, err)
		}
	}
	for index := range snapshot.Performers {
		if err := snapshot.Performers[index].Validate(); err != nil {
			return fmt.Errorf("performers[%d]: %w", index, err)
		}
	}
	return nil
}

func (snapshot *SourceSnapshot) UnmarshalJSON(data []byte) error {
	type sourceSnapshot SourceSnapshot
	var value sourceSnapshot
	if err := strictUnmarshalJSON(data, &value); err != nil {
		return err
	}
	*snapshot = SourceSnapshot(value)
	return snapshot.Validate()
}

func (snapshot SourceSnapshot) MarshalJSON() ([]byte, error) {
	if err := (&snapshot).Validate(); err != nil {
		return nil, err
	}
	type sourceSnapshot SourceSnapshot
	return json.Marshal(sourceSnapshot(snapshot))
}

type Scene struct {
	ID                   string                `json:"id"`
	Title                string                `json:"title,omitempty"`
	Details              string                `json:"details,omitempty"`
	Dates                []string              `json:"dates,omitempty"`
	URLs                 []string              `json:"urls,omitempty"`
	Duration             *int                  `json:"duration,omitempty"`
	Director             string                `json:"director,omitempty"`
	Code                 string                `json:"code,omitempty"`
	Studio               *Studio               `json:"studio,omitempty"`
	Tags                 []Tag                 `json:"tags,omitempty"`
	PerformerAppearances []PerformerAppearance `json:"performer_appearances,omitempty"`
	RemoteImages         []string              `json:"remote_images,omitempty"`
}

func (scene *Scene) Validate() error {
	if strings.TrimSpace(scene.ID) == "" {
		return fmt.Errorf("scene id is required")
	}
	if scene.Studio != nil && (strings.TrimSpace(scene.Studio.ID) == "" || strings.TrimSpace(scene.Studio.Name) == "") {
		return fmt.Errorf("studio id and name are required")
	}
	for index, tag := range scene.Tags {
		if strings.TrimSpace(tag.ID) == "" || strings.TrimSpace(tag.Name) == "" {
			return fmt.Errorf("tags[%d] id and name are required", index)
		}
	}
	for index, appearance := range scene.PerformerAppearances {
		if strings.TrimSpace(appearance.PerformerID) == "" {
			return fmt.Errorf("performer_appearances[%d] performer_id is required", index)
		}
	}
	return nil
}

func (scene *Scene) UnmarshalJSON(data []byte) error {
	type sceneValue Scene
	var value sceneValue
	if err := strictUnmarshalJSON(data, &value); err != nil {
		return err
	}
	*scene = Scene(value)
	return nil
}

type Performer struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Aliases      []string `json:"aliases,omitempty"`
	Gender       string   `json:"gender,omitempty"`
	Country      string   `json:"country,omitempty"`
	Ethnicity    string   `json:"ethnicity,omitempty"`
	EyeColor     string   `json:"eye_color,omitempty"`
	HairColor    string   `json:"hair_color,omitempty"`
	Measurements string   `json:"measurements,omitempty"`
	CareerYears  []int    `json:"career_years,omitempty"`
	URLs         []string `json:"urls,omitempty"`
	RemoteImages []string `json:"remote_images,omitempty"`
}

func (performer *Performer) Validate() error {
	if strings.TrimSpace(performer.ID) == "" || strings.TrimSpace(performer.Name) == "" {
		return fmt.Errorf("performer id and name are required")
	}
	return nil
}

func (performer *Performer) UnmarshalJSON(data []byte) error {
	type performerValue Performer
	var value performerValue
	if err := strictUnmarshalJSON(data, &value); err != nil {
		return err
	}
	*performer = Performer(value)
	return nil
}

type Studio struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (studio *Studio) UnmarshalJSON(data []byte) error {
	type studioValue Studio
	var value studioValue
	if err := strictUnmarshalJSON(data, &value); err != nil {
		return err
	}
	*studio = Studio(value)
	return nil
}

type Tag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (tag *Tag) UnmarshalJSON(data []byte) error {
	type tagValue Tag
	var value tagValue
	if err := strictUnmarshalJSON(data, &value); err != nil {
		return err
	}
	*tag = Tag(value)
	return nil
}

type PerformerAppearance struct {
	PerformerID string `json:"performer_id"`
}

func (appearance *PerformerAppearance) UnmarshalJSON(data []byte) error {
	type appearanceValue PerformerAppearance
	var value appearanceValue
	if err := strictUnmarshalJSON(data, &value); err != nil {
		return err
	}
	*appearance = PerformerAppearance(value)
	return nil
}

func strictUnmarshalJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("invalid trailing JSON data")
	}
	return nil
}
