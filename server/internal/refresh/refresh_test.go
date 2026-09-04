package refresh

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunnerRebuildsEveryAccountBeforeActivatingModel(t *testing.T) {
	var calls []string
	runner := NewRunner(
		accountSourceFunc(func(context.Context) ([]string, error) {
			calls = append(calls, "accounts")
			return []string{"account-b", "account-a"}, nil
		}),
		sessionRebuilderFunc(func(_ context.Context, accountID string) error {
			calls = append(calls, "session:"+accountID)
			return nil
		}),
		modelBuilderFunc(func(context.Context) (string, error) {
			calls = append(calls, "model")
			return "version-1", nil
		}),
	)

	version, err := runner.Refresh(context.Background())

	require.NoError(t, err)
	require.Equal(t, "version-1", version)
	require.Equal(t, []string{"accounts", "session:account-b", "session:account-a", "model"}, calls)
}

func TestRunnerDoesNotActivateModelWhenSessionRebuildFails(t *testing.T) {
	boom := errors.New("session rebuild failed")
	var modelCalled bool
	runner := NewRunner(
		accountSourceFunc(func(context.Context) ([]string, error) { return []string{"account-a"}, nil }),
		sessionRebuilderFunc(func(context.Context, string) error { return boom }),
		modelBuilderFunc(func(context.Context) (string, error) {
			modelCalled = true
			return "", nil
		}),
	)

	_, err := runner.Refresh(context.Background())

	require.ErrorIs(t, err, boom)
	require.False(t, modelCalled)
}

type accountSourceFunc func(context.Context) ([]string, error)

func (fn accountSourceFunc) AccountIDs(ctx context.Context) ([]string, error) { return fn(ctx) }

type sessionRebuilderFunc func(context.Context, string) error

func (fn sessionRebuilderFunc) Rebuild(ctx context.Context, accountID string) error {
	return fn(ctx, accountID)
}

type modelBuilderFunc func(context.Context) (string, error)

func (fn modelBuilderFunc) BuildAndActivate(ctx context.Context) (string, error) { return fn(ctx) }
