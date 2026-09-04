package refresh

import (
	"context"
	"time"
)

// RunPeriodically invokes refresh at each interval until the context is canceled.
// A failed refresh is reported and the next interval is still attempted.
func RunPeriodically(
	ctx context.Context,
	interval time.Duration,
	refresh func(context.Context) (string, error),
	onSuccess func(string),
	onError func(error),
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			version, err := refresh(ctx)
			if err != nil {
				onError(err)
				continue
			}
			onSuccess(version)
		}
	}
}
