package catalog

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/treehorn/stash-recommendations/server/internal/domain"
)

type EntityReference struct {
	ContentKey domain.ContentKey
	Name       string
}

type Source struct {
	ContentKey   domain.ContentKey
	Title        string
	Details      string
	Dates        []string
	URLs         []string
	Duration     *int
	Director     string
	Code         string
	Studio       *EntityReference
	Tags         []EntityReference
	Performers   []EntityReference
	RemoteImages []string
	CanonicalURL *string
}

type SnapshotRepository interface {
	UpsertSnapshot(ctx context.Context, snapshot domain.SourceSnapshot, raw json.RawMessage) error
	CatalogSource(ctx context.Context, key domain.ContentKey) (Source, bool, error)
}

type Service struct {
	repository SnapshotRepository
}

func NewSnapshotService(repository SnapshotRepository) *Service {
	return &Service{repository: repository}
}

func (service *Service) Upsert(ctx context.Context, snapshotJSON []byte) error {
	var snapshot domain.SourceSnapshot
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return fmt.Errorf("decode source snapshot: %w", err)
	}
	return service.repository.UpsertSnapshot(ctx, snapshot, append(json.RawMessage(nil), snapshotJSON...))
}

func (service *Service) Source(ctx context.Context, key domain.ContentKey) (Source, bool, error) {
	normalized, err := (domain.ContentKey{}).Normalize(key.Endpoint, key.StashID)
	if err != nil {
		return Source{}, false, err
	}
	return service.repository.CatalogSource(ctx, normalized)
}
