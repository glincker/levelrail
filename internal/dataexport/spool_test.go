package dataexport

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"net/http/httptest"
)

func rec(msg string) LogRecord {
	return LogRecord{Time: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC), Kind: "app", App: "web", Stream: "stdout", Level: "info", Message: msg}
}

func TestSpoolReplaysOnlyWhatIsStillOwed(t *testing.T) {
	dir := t.TempDir()
	sp, pending, err := openSpool(dir, "exp_1")
	if err != nil || len(pending) != 0 {
		t.Fatalf("fresh spool: %v %v", err, pending)
	}
	for _, m := range []string{"a", "b", "c", "d"} {
		sp.append(rec(m))
	}
	sp.flush()
	sp.ack(2)
	sp.close()

	_, pending, err = openSpool(dir, "exp_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 || pending[0].Message != "c" || pending[1].Message != "d" {
		t.Fatalf("pending = %+v, want c and d", pending)
	}
}

func TestSpoolIgnoresATornLastLine(t *testing.T) {
	dir := t.TempDir()
	sp, _, _ := openSpool(dir, "exp_1")
	sp.append(rec("whole"))
	sp.flush()
	sp.close()
	f, _ := os.OpenFile(filepath.Join(dir, "exp_1.spool"), os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // test file under t.TempDir
	_, _ = f.WriteString(`{"Message":"torn`)
	_ = f.Close()

	_, pending, err := openSpool(dir, "exp_1")
	if err != nil || len(pending) != 1 || pending[0].Message != "whole" {
		t.Fatalf("pending = %+v err=%v, want only the whole line", pending, err)
	}
}

func TestSpoolResetsWhenNothingIsOwedAndCompacts(t *testing.T) {
	dir := t.TempDir()
	sp, _, _ := openSpool(dir, "exp_1")
	for i := 0; i < spoolCompactAfter+10; i++ {
		sp.append(rec("x"))
	}
	sp.flush()
	sp.ack(spoolCompactAfter + 5) // most are done: compacts
	sp.flush()
	raw, _ := os.ReadFile(filepath.Join(dir, "exp_1.spool")) //nolint:gosec // test file under t.TempDir
	if n := strings.Count(string(raw), "\n"); n != 5 {
		t.Fatalf("compacted file has %d lines, want 5", n)
	}
	sp.ack(5) // nothing owed: truncated
	if st, _ := os.Stat(filepath.Join(dir, "exp_1.spool")); st.Size() != 0 {
		t.Fatalf("file size after full ack = %d, want 0", st.Size())
	}
	sp.close()
}

// TestBacklogSurvivesARestart is the point of the spool: lines that a down
// receiver never got are delivered by the next run of the control plane.
func TestBacklogSurvivesARestart(t *testing.T) {
	db := openDB(t)
	spool := t.TempDir()
	rcv := &sink{}
	srv := httptest.NewServer(rcv)
	defer srv.Close()
	rcv.setFail(true)

	newSvc := func(b *telemetry.LogBroadcaster) *Service {
		t.Setenv(AllowPrivateEnv, "true")
		return NewService(Deps{Store: db, Secrets: &memSecrets{}, Logs: b, Config: fastConfig(), Brand: "plat", SpoolDir: spool})
	}

	b1 := telemetry.NewLogBroadcaster()
	svc1 := newSvc(b1)
	ctx1, cancel1 := context.WithCancel(context.Background())
	if _, err := svc1.Create(ctx1, Input{Name: "http", Kind: store.ExportKindLogsHTTP, Endpoint: srv.URL, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	done1 := make(chan struct{})
	go func() { svc1.Run(ctx1); close(done1) }()
	waitFor(t, "worker start", func() bool { svc1.mu.Lock(); defer svc1.mu.Unlock(); return len(svc1.workers) == 1 })

	b1.Publish(telemetry.LogEntry{ResourceID: "service:web", Stream: "stdout", Timestamp: time.Now(), Message: "survives restart"})
	time.Sleep(200 * time.Millisecond) // spool flush
	cancel1()
	<-done1

	rcv.setFail(false)
	b2 := telemetry.NewLogBroadcaster()
	svc2 := newSvc(b2)
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go svc2.Run(ctx2)

	waitFor(t, "replay after restart", func() bool {
		for _, l := range rcv.lines() {
			if strings.Contains(l, "survives restart") {
				return true
			}
		}
		return false
	})
}
