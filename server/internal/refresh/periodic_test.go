package refresh

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunPeriodicallyRefreshesUntilCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	refreshed := make(chan struct{}, 1)
	var calls atomic.Int32
	done := make(chan struct{})
	go func() {
		RunPeriodically(ctx, time.Millisecond, func(context.Context) (string, error) {
			if calls.Add(1) == 1 {
				refreshed <- struct{}{}
			}
			return "version", nil
		}, func(string) {}, func(error) {})
		close(done)
	}()

	require.Eventually(t, func() bool {
		select {
		case <-refreshed:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	cancel()
	require.Eventually(t, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
}
