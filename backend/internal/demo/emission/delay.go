package emission

import (
	"context"
	"time"
)

// Wait pauses a demo business flow immediately before it creates a domain
// event. The timer remains cancellation-aware so shutdown and request timeout
// signals do not leave a service blocked in an artificial simulation delay.
func Wait(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
