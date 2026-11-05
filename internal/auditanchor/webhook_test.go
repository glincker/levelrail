package auditanchor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

type fakeSettings map[string]string

func (f fakeSettings) String(_ context.Context, k string) string { return f[k] }

func newTestExporter(t *testing.T, set Settings, src HeadSource, cfg ExportConfig) *Exporter {
	t.Helper()
	t.Setenv(netguard.AllowPrivateEnv, "true")
	e := NewExporter(cfg, time.Minute, src, set, quiet())
	e.sleep = func(context.Context, time.Duration) {}
	return e
}

func drainOne(e *Exporter) {
	select {
	case a := <-e.queue:
		e.deliver(context.Background(), a)
	default:
	}
}

func TestExporter_OffByDefaultMakesNoRequest(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	src := &fakeSource{a: store.AuditAnchor{Version: 1, Seq: 1, Hash: "h", At: "t", Sig: "k:s"}}
	e := newTestExporter(t, fakeSettings{}, src, ExportConfig{Queue: 4, Attempts: 2})
	e.Tick(context.Background())
	drainOne(e)
	if hits.Load() != 0 || e.Status(context.Background()).Enabled {
		t.Fatal("export ran while off")
	}
}

func TestExporter_SignsBodyAndLeaksNothingElse(t *testing.T) {
	var gotSig, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotSig, gotBody = r.Header.Get(SignatureHeader), string(b)
	}))
	defer srv.Close()
	set := fakeSettings{runtimesettings.KeyAuditAnchorWebhookURL: srv.URL, runtimesettings.KeyAuditAnchorWebhookSecret: "s3cret"}
	src := &fakeSource{a: store.AuditAnchor{Version: 1, Seq: 9, Hash: "abc", At: "2026-01-01T00:00:00Z", Sig: "k:sig"}}
	e := newTestExporter(t, set, src, ExportConfig{Queue: 4, Attempts: 2})
	e.Tick(context.Background())
	drainOne(e)
	if gotSig != Sign("s3cret", []byte(gotBody)) || gotSig == "" {
		t.Fatalf("signature does not verify: %q", gotSig)
	}
	if strings.Contains(gotBody, "s3cret") || strings.Contains(gotBody, "actor") || strings.Contains(gotBody, "detail") {
		t.Fatalf("unexpected content in body: %s", gotBody)
	}
	st := e.Status(context.Background())
	if st.LastExportedSeq != 9 || st.Exported != 1 || !st.Enabled {
		t.Fatalf("status = %+v", st)
	}
	e.Tick(context.Background())
	if len(e.queue) != 0 {
		t.Fatal("same head queued twice")
	}
}

func TestExporter_RetryThenSuccess(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n < 3 {
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer srv.Close()
	set := fakeSettings{runtimesettings.KeyAuditAnchorWebhookURL: srv.URL}
	src := &fakeSource{a: store.AuditAnchor{Version: 1, Seq: 2, Hash: "h", At: "t", Sig: "k:s"}}
	e := newTestExporter(t, set, src, ExportConfig{Queue: 4, Attempts: 5})
	e.Tick(context.Background())
	drainOne(e)
	st := e.Status(context.Background())
	if calls != 3 || st.Exported != 1 || st.Failed != 0 {
		t.Fatalf("calls=%d status=%+v", calls, st)
	}
}

func TestExporter_GivesUpAndRecordsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	set := fakeSettings{runtimesettings.KeyAuditAnchorWebhookURL: srv.URL}
	src := &fakeSource{a: store.AuditAnchor{Version: 1, Seq: 3, Hash: "h", At: "t", Sig: "k:s"}}
	e := newTestExporter(t, set, src, ExportConfig{Queue: 4, Attempts: 2})
	e.Tick(context.Background())
	drainOne(e)
	st := e.Status(context.Background())
	if st.Failed != 1 || st.LastError == "" || st.LastExportedSeq != 0 {
		t.Fatalf("status = %+v", st)
	}
}

func TestExporter_QueueIsBounded(t *testing.T) {
	set := fakeSettings{runtimesettings.KeyAuditAnchorWebhookURL: "https://example.invalid/hook"}
	src := &fakeSource{}
	e := newTestExporter(t, set, src, ExportConfig{Queue: 2, Attempts: 1})
	for i := int64(1); i <= 5; i++ {
		src.a = store.AuditAnchor{Version: 1, Seq: i, Hash: "h", At: "t", Sig: "k:s"}
		e.Tick(context.Background())
	}
	st := e.Status(context.Background())
	if st.Queued != 2 || st.Dropped != 3 {
		t.Fatalf("status = %+v", st)
	}
}
