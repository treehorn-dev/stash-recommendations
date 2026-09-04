package refresh

import (
	"context"
	"fmt"
)

type accountSource interface {
	AccountIDs(context.Context) ([]string, error)
}

type sessionRebuilder interface {
	Rebuild(context.Context, string) error
}

type modelBuilder interface {
	BuildAndActivate(context.Context) (string, error)
}

// Runner creates current session projections before activating a model.
type Runner struct {
	accounts accountSource
	sessions sessionRebuilder
	model    modelBuilder
}

func NewRunner(accounts accountSource, sessions sessionRebuilder, model modelBuilder) Runner {
	return Runner{accounts: accounts, sessions: sessions, model: model}
}

func (runner Runner) Refresh(ctx context.Context) (string, error) {
	accountIDs, err := runner.accounts.AccountIDs(ctx)
	if err != nil {
		return "", fmt.Errorf("load accounts: %w", err)
	}
	for _, accountID := range accountIDs {
		if err := runner.sessions.Rebuild(ctx, accountID); err != nil {
			return "", fmt.Errorf("rebuild sessions for account %s: %w", accountID, err)
		}
	}
	version, err := runner.model.BuildAndActivate(ctx)
	if err != nil {
		return "", fmt.Errorf("build recommendation model: %w", err)
	}
	return version, nil
}
