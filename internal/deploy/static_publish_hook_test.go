package deploy

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
)

func TestStaticPublishHook_FailureOnlyWarns(t *testing.T) {
	p := &Pipeline{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	p.SetStaticPublishHook(func(_ context.Context, app string) error {
		return errors.New("purge failed for " + app)
	})
	var log []string
	p.runStaticPublishHook(context.Background(), Request{ServiceName: "web"}, func(ev build.ProgressEvent) {
		log = append(log, ev.Log)
	})
	if len(log) == 0 || !strings.Contains(strings.Join(log, ""), "warning: purge failed for web") {
		t.Fatalf("log = %v", log)
	}
}
