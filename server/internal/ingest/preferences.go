package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/treehorn/stash-recommendations/server/internal/domain"
)

type interactionRepository interface {
	AcceptInteractionEvent(ctx context.Context, accountID string, event domain.PreferenceEvent, bodyHash []byte) (bool, error)
}

type Service struct {
	repository interactionRepository
}

func NewInteractionService(repository interactionRepository) *Service {
	return &Service{repository: repository}
}

func (service *Service) Accept(ctx context.Context, accountID string, event domain.PreferenceEvent) (bool, error) {
	if err := event.Validate(); err != nil {
		return false, err
	}
	body, err := json.Marshal(event)
	if err != nil {
		return false, fmt.Errorf("marshal interaction event: %w", err)
	}
	sum := sha256.Sum256(body)
	return service.repository.AcceptInteractionEvent(ctx, accountID, event, sum[:])
}
