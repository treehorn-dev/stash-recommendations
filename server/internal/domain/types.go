package domain

import (
	"fmt"
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
	return nil
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

type Studio struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Tag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type PerformerAppearance struct {
	PerformerID string `json:"performer_id"`
}
