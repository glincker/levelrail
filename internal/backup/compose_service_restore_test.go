package backup

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type restoreOrder struct{ steps []string }

type orderSafety struct {
	o   *restoreOrder
	err error
}

func (s *orderSafety) RunComposeServiceBackup(_ context.Context, _, _, _, _, _ string) error {
	s.o.steps = append(s.o.steps, "safety")
	return s.err
}

type orderRestorer struct{ o *restoreOrder }

func (r *orderRestorer) Restore(_ context.Context, _, _ string, dump io.Reader) error {
	b, _ := io.ReadAll(dump)
	r.o.steps = append(r.o.steps, "restore:"+string(b))
	return nil
}

type fakeDests struct{}

func (fakeDests) ResolveDestination(context.Context, string) (Destination, error) {
	return Destination{Bucket: "b"}, nil
}

func newComposeRestorer(o *restoreOrder, safetyErr error) *ComposeServiceRestorer {
	return &ComposeServiceRestorer{
		Safety:       &orderSafety{o: o, err: safetyErr},
		Destinations: fakeDests{},
		Containers:   &fakeComposeContainers{name: "shop-db-1"},
		Downloader:   &fakeDownloader{content: "SELECT 1;"},
		Restorer:     &orderRestorer{o: o},
	}
}

func TestComposeServiceRestore_SafetyDumpFirst(t *testing.T) {
	for _, engine := range []string{store.EnginePostgres, store.EngineMySQL, store.EngineMariaDB} {
		t.Run(engine, func(t *testing.T) {
			o := &restoreOrder{}
			run := store.ComposeServiceBackupRun{ID: "csr_1", App: "shop", Service: "db", TargetID: "t", Status: store.BackupStatusSucceeded, ObjectKey: "compose/shop/db/x.dump"}
			if err := newComposeRestorer(o, nil).RunComposeServiceRestore(context.Background(), "csr_s", "shop", "db", engine, run); err != nil {
				t.Fatalf("restore: %v", err)
			}
			if got := strings.Join(o.steps, ","); got != "safety,restore:SELECT 1;" {
				t.Fatalf("steps = %q, want safety dump before restore", got)
			}
		})
	}
}

func TestComposeServiceRestore_AbortsWhenSafetyDumpFails(t *testing.T) {
	o := &restoreOrder{}
	run := store.ComposeServiceBackupRun{ID: "csr_1", App: "shop", Service: "db", TargetID: "t", Status: store.BackupStatusSucceeded, ObjectKey: "k.dump"}
	err := newComposeRestorer(o, errors.New("disk full")).RunComposeServiceRestore(context.Background(), "csr_s", "shop", "db", store.EnginePostgres, run)
	if err == nil || strings.Join(o.steps, ",") != "safety" {
		t.Fatalf("err = %v steps = %v, want abort after the failed safety dump", err, o.steps)
	}
}

func TestComposeServiceRestore_RefusesBadRuns(t *testing.T) {
	cases := map[string]struct {
		run    store.ComposeServiceBackupRun
		engine string
	}{
		"failed":       {store.ComposeServiceBackupRun{App: "shop", Service: "db", Status: store.BackupStatusFailed, ObjectKey: "k"}, store.EnginePostgres},
		"running":      {store.ComposeServiceBackupRun{App: "shop", Service: "db", Status: store.BackupStatusRunning, ObjectKey: "k"}, store.EnginePostgres},
		"other-svc":    {store.ComposeServiceBackupRun{App: "shop", Service: "cache", Status: store.BackupStatusSucceeded, ObjectKey: "k"}, store.EnginePostgres},
		"engine":       {store.ComposeServiceBackupRun{App: "shop", Service: "db", Status: store.BackupStatusSucceeded, ObjectKey: "k"}, store.EngineRedis},
		"no-objectkey": {store.ComposeServiceBackupRun{App: "shop", Service: "db", Status: store.BackupStatusSucceeded}, store.EnginePostgres},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			o := &restoreOrder{}
			err := newComposeRestorer(o, nil).RunComposeServiceRestore(context.Background(), "csr_s", "shop", "db", tc.engine, tc.run)
			if !errors.Is(err, ErrComposeRestoreRefused) || len(o.steps) != 0 {
				t.Fatalf("err = %v steps = %v, want refusal with nothing run", err, o.steps)
			}
		})
	}
}

func TestComposeRestoreCmd(t *testing.T) {
	for _, engine := range []string{store.EnginePostgres, store.EngineMySQL, store.EngineMariaDB} {
		if cmd, err := ComposeRestoreCmd(engine); err != nil || len(cmd) != 3 || cmd[0] != "sh" {
			t.Errorf("%s cmd = %v, %v", engine, cmd, err)
		}
	}
	if _, err := ComposeRestoreCmd(store.EngineMongoDB); err == nil {
		t.Error("mongodb accepted")
	}
}

type cutReader struct{ sent bool }

func (c *cutReader) Read(p []byte) (int, error) {
	if c.sent {
		return 0, errors.New("connection reset")
	}
	c.sent = true
	return copy(p, "SELECT 1;\n"), nil
}

func TestGuardedReaderCommitsOnlyOnCleanEOF(t *testing.T) {
	clean := &guardedReader{src: strings.NewReader("SELECT 1;"), prelude: "BEGIN;\n", suffix: "\nCOMMIT;\n"}
	got, err := io.ReadAll(clean)
	if err != nil || !strings.HasSuffix(string(got), "COMMIT;\n") {
		t.Fatalf("clean stream = %q, %v, want trailing COMMIT", got, err)
	}
	cut := &guardedReader{src: &cutReader{}, prelude: "BEGIN;\n", suffix: "\nCOMMIT;\n"}
	got, err = io.ReadAll(cut)
	if err == nil || cut.err == nil || strings.Contains(string(got), "COMMIT") {
		t.Fatalf("cut stream = %q, %v, want error and no COMMIT", got, err)
	}
}
