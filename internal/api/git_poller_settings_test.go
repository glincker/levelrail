package api

import (
	"context"
	"testing"
	"time"
)

func TestGitPollConfigDefaultsWithoutSettings(t *testing.T) {
	rt := &Router{}
	c := rt.gitPollConfig(context.Background())
	if !c.Enabled || c.Interval != 5*time.Minute || c.Concurrency != 4 || c.MaxBackoff != time.Hour {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}
