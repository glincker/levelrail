package changes

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeLogSignatures struct {
	n     int
	err   error
	calls int
}

func (f *fakeLogSignatures) NewErrorTypes(context.Context, string, time.Time, time.Time) (int, error) {
	f.calls++
	return f.n, f.err
}

func TestCollectAnnotatesNewestDeployWithNewErrorTypes(t *testing.T) {
	dp := fakeDeploys{list: []store.DeployAttempt{
		{ID: "d2", Image: "app:v2", Status: store.DeployAttemptStatusSucceeded, StartedAt: ago(5), FinishedAt: timePtr(ago(4))},
		{ID: "d1", Image: "app:v1", Status: store.DeployAttemptStatusSucceeded, StartedAt: ago(20), FinishedAt: timePtr(ago(19))},
	}}
	logs := &fakeLogSignatures{n: 3}
	a := agg(nil, dp, nil)
	a.Logs = logs

	res := a.Collect(context.Background(), "web", now)
	if logs.calls != 1 {
		t.Fatalf("log source called %d times, want 1 (newest deploy only)", logs.calls)
	}
	if res.Changes[0].NewErrorTypes != 3 || res.Changes[1].NewErrorTypes != 0 {
		t.Fatalf("annotations = %d, %d", res.Changes[0].NewErrorTypes, res.Changes[1].NewErrorTypes)
	}
	if !strings.Contains(res.Changes[0].Line(), "3 new error types") {
		t.Fatalf("line = %q", res.Changes[0].Line())
	}
}

func TestCollectLogSourceErrorIsSkipped(t *testing.T) {
	dp := fakeDeploys{list: []store.DeployAttempt{
		{ID: "d1", Image: "app:v1", Status: store.DeployAttemptStatusSucceeded, StartedAt: ago(5), FinishedAt: timePtr(ago(4))},
	}}
	a := agg(nil, dp, nil)
	a.Logs = &fakeLogSignatures{err: errors.New("telemetry down")}
	res := a.Collect(context.Background(), "web", now)
	if len(res.Changes) != 1 || res.Changes[0].NewErrorTypes != 0 {
		t.Fatalf("a failing log source must not hide the deploy: %+v", res.Changes)
	}
}
