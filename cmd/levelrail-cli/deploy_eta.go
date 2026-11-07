package main

import (
	"context"
	"fmt"
	"time"
)

// previousBuildDuration returns how long the newest finished healthy build of the app took, so a
// running build can show what to expect. Zero means no usable history; errors are not worth surfacing.
func previousBuildDuration(ctx context.Context, client *Client, name, skipID string) time.Duration {
	attempts, err := client.ListDeployAttempts(ctx, name)
	if err != nil {
		return 0
	}
	var best time.Time
	var dur time.Duration
	for _, a := range attempts {
		if a.ID == skipID || a.FinishedAt == nil || a.Error != "" || a.Status == "failed" || a.Status == "cancelled" {
			continue
		}
		d := a.FinishedAt.Sub(a.StartedAt)
		if d <= 0 || !a.StartedAt.After(best) {
			continue
		}
		best, dur = a.StartedAt, d
	}
	return dur
}

// buildProgressLabel is the label for the quiet-period status line, with the previous build's
// duration as the expected time when one is known.
func buildProgressLabel(prev time.Duration) string {
	if prev <= 0 {
		return "building"
	}
	return fmt.Sprintf("building (last build took %s)", prev.Round(time.Second))
}
