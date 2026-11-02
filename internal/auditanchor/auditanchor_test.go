package auditanchor

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

type fakeSource struct {
	a   store.AuditAnchor
	err error
}

func (f *fakeSource) LatestAuditAnchor(context.Context) (store.AuditAnchor, bool, error) {
	return f.a, f.err == nil, f.err
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestConfigFromEnv_UnsetIsDisabled(t *testing.T) {
	c := ConfigFromEnv(func(string) (string, bool) { return "", false })
	if c.Enabled() || c.Interval <= 0 || c.Timeout <= 0 {
		t.Fatalf("cfg = %+v", c)
	}
}

func TestTick_AppendsToFileOncePerHead(t *testing.T) {
	dir := t.TempDir()
	src := &fakeSource{a: store.AuditAnchor{Version: 1, Seq: 7, Hash: "abc", At: "t", Sig: "k:x"}}
	p := New(Config{Dir: dir, Interval: time.Minute, Timeout: time.Second}, src, quiet())
	p.Tick(context.Background())
	p.Tick(context.Background())
	src.a.Seq = 8
	p.Tick(context.Background())
	f, err := os.Open(filepath.Join(dir, FileName)) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var seqs []int64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var a store.AuditAnchor
		if err := json.Unmarshal(sc.Bytes(), &a); err != nil {
			t.Fatal(err)
		}
		seqs = append(seqs, a.Seq)
	}
	if len(seqs) != 2 || seqs[0] != 7 || seqs[1] != 8 {
		t.Fatalf("seqs = %v", seqs)
	}
}

func TestTick_WebhookPostsAndFailuresAreSwallowed(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "true")
	var got store.AuditAnchor
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(status)
	}))
	defer srv.Close()
	src := &fakeSource{a: store.AuditAnchor{Version: 1, Seq: 3, Hash: "h", At: "t", Sig: "k:s"}}
	p := New(Config{URL: srv.URL, Interval: time.Minute, Timeout: time.Second}, src, quiet())
	status = http.StatusInternalServerError
	p.Tick(context.Background())
	if p.lastSeq != 0 {
		t.Fatal("a failed post must be retried, not recorded as sent")
	}
	status = http.StatusOK
	p.Tick(context.Background())
	if got.Seq != 3 || p.lastSeq != 3 {
		t.Fatalf("got %+v last=%d", got, p.lastSeq)
	}
}

func TestTick_SourceAndSinkErrorsDoNotPanic(t *testing.T) {
	p := New(Config{Dir: filepath.Join(t.TempDir(), "f", FileName+"/x"), Timeout: time.Second}, &fakeSource{err: errors.New("db down")}, quiet())
	p.Tick(context.Background())
	p2 := New(Config{URL: "http://127.0.0.1:1/x", Timeout: time.Second}, &fakeSource{a: store.AuditAnchor{Seq: 1}}, quiet())
	p2.Tick(context.Background())
}
