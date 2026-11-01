package plugin

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/plugin/egress"
)

// fakeSidecar serves a denial tally on a short unix socket and counts reads.
func fakeSidecar(t *testing.T, c *egress.Counters) (string, *int) {
	t.Helper()
	dir, err := os.MkdirTemp("", "lrsc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "p.sock")
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "unix", path)
	if err != nil {
		t.Fatal(err)
	}
	reads := 0
	mux := http.NewServeMux()
	mux.HandleFunc("GET /denials", func(w http.ResponseWriter, r *http.Request) {
		reads++
		c.Handler().ServeHTTP(w, r)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return path, &reads
}

func TestEgressDenialsReadsSidecarAndCaches(t *testing.T) {
	h := newHarness(t)
	h.m.cfg.EgressMode = EgressProxy
	c := egress.NewCounters()
	c.Record("example", egress.ReasonHostNotListed)
	c.Record("example", egress.ReasonHostNotListed)
	c.Record("other", egress.ReasonBlockedAddr)
	sock, reads := fakeSidecar(t, c)
	h.m.denialsFn = func(ctx context.Context, _ string, to time.Duration) ([]egress.DenialCount, error) {
		return egress.FetchDenials(ctx, sock, to)
	}
	got := h.m.EgressDenials(context.Background())
	if len(got["example"]) != 1 || got["example"][0].Count != 2 || got["example"][0].LastDenied == "" || len(got["other"]) != 1 {
		t.Fatalf("denials = %+v", got)
	}
	h.m.EgressDenials(context.Background())
	if *reads != 1 {
		t.Fatalf("reads within ttl = %d, want 1", *reads)
	}
	h.now = h.now.Add(time.Minute)
	h.m.EgressDenials(context.Background())
	if *reads != 2 {
		t.Fatalf("reads after ttl = %d, want 2", *reads)
	}
}

func TestEgressDenialsNoopInOpenModeAndWhenSidecarDown(t *testing.T) {
	h := newHarness(t)
	h.m.denialsFn = func(context.Context, string, time.Duration) ([]egress.DenialCount, error) {
		t.Fatal("sidecar must not be read in open mode")
		return nil, nil
	}
	if got := h.m.EgressDenials(context.Background()); len(got) != 0 {
		t.Fatalf("open mode = %+v", got)
	}
	h.m.cfg.EgressMode = EgressProxy
	h.m.denialsFn = func(ctx context.Context, _ string, to time.Duration) ([]egress.DenialCount, error) {
		return egress.FetchDenials(ctx, filepath.Join(t.TempDir(), "gone.sock"), to)
	}
	if got := h.m.EgressDenials(context.Background()); len(got) != 0 {
		t.Fatalf("sidecar down = %+v", got)
	}
}
