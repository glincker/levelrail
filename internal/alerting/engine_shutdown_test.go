package alerting

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
)

type failingRuleStore struct {
	*fakeRuleStore
	calls int
}

func (f *failingRuleStore) ListEnabledRules(context.Context) ([]Rule, error) {
	f.calls++
	return nil, errors.New("sql: database is closed")
}

func TestRunTickQuietOnShutdown(t *testing.T) {
	var buf bytes.Buffer
	store := &failingRuleStore{fakeRuleStore: newFakeRuleStore()}
	e := newTestEngine(store.fakeRuleStore, nil, nil, nil, &spyNotifier{})
	e.rules = store
	e.logger = slog.New(slog.NewTextHandler(&buf, nil))
	loop := footprint.Track("alerting shutdown test", time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	e.runTick(ctx, loop)
	if store.calls != 1 || buf.Len() == 0 {
		t.Fatalf("live ctx should tick and log the failure, calls=%d log=%q", store.calls, buf.String())
	}

	buf.Reset()
	cancel()
	e.runTick(ctx, loop)
	if store.calls != 1 || buf.Len() != 0 {
		t.Fatalf("cancelled ctx must not tick or log, calls=%d log=%q", store.calls, buf.String())
	}
}
