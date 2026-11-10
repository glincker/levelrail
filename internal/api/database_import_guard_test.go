package api

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestDatabaseImport_SealedExportIs400WithoutRestore(t *testing.T) {
	for name, body := range map[string]string{
		"age binary":  "age-encryption.org/v1\n-> X25519 abc\n---\x00\x01",
		"age armored": "-----BEGIN AGE ENCRYPTED FILE-----\nYWdl\n",
		"zstd":        "\x28\xb5\x2f\xfd\x00\x01binary",
	} {
		t.Run(name, func(t *testing.T) {
			rt, _, fake, tok := newImportFixture(t, "")
			rec := incTokenPost(rt, tok, "/api/v1/databases/main/import", body)
			if rec.Code != http.StatusBadRequest || fake.restores != 0 {
				t.Fatalf("sealed import = %d restores=%d body=%s, want 400 and no restore", rec.Code, fake.restores, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "export") {
				t.Errorf("message does not name the export: %s", rec.Body.String())
			}
		})
	}
}

// failingImportRuntime fails the restore with engine stderr quoting the dump.
type failingImportRuntime struct{ *importFakeRuntime }

func (f *failingImportRuntime) ExecWithInput(_ context.Context, c string, cmd []string, in io.Reader) (io.ReadCloser, error) {
	_, _ = io.Copy(io.Discard, in)
	pr, pw := io.Pipe()
	_ = pw.CloseWithError(&docker.ExecExitError{Cmd: cmd, Container: c, ExitCode: 3, Stderr: "ERROR: syntax error at or near \"CANARY-DUMP-LINE\""})
	return pr, nil
}

func TestDatabaseImport_RestoreFailureLogsNoDumpText(t *testing.T) {
	const canary = "CANARY-DUMP-LINE"
	rt, _, fake, tok := newImportFixture(t, "")
	var logs bytes.Buffer
	rt.logger = slog.New(slog.NewTextHandler(&logs, nil))
	failing := &failingImportRuntime{fake}
	rt.execRuntime = func(string) (docker.Runtime, error) { return failing, nil }

	rec := incTokenPost(rt, tok, "/api/v1/databases/main/import", "insert "+canary+";")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("failed restore = %d, want 502", rec.Code)
	}
	if strings.Contains(logs.String(), canary) || strings.Contains(rec.Body.String(), canary) {
		t.Fatalf("dump text leaked: log=%q body=%q", logs.String(), rec.Body.String())
	}
	if !strings.Contains(logs.String(), "error_class=dump_not_readable") || !strings.Contains(logs.String(), "exit_status=3") {
		t.Errorf("log lacks the error class and exit status: %q", logs.String())
	}
}

// sealedOpenerProtection embeds the interface so only the opener is real.
type sealedOpenerProtection struct {
	BackupProtection
	p *backup.Protection
}

func (s sealedOpenerProtection) OpenSealedExport(r io.Reader) (io.ReadCloser, error) {
	return s.p.OpenSealedExport(r)
}

func sealerFor(t *testing.T) *backup.Sealer {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return &backup.Sealer{Recipients: []age.Recipient{id.Recipient()}, Identities: []age.Identity{id}}
}

func TestDatabaseImport_SealedExportRoundTripAndKeyMismatch(t *testing.T) {
	const dump = "select 1; -- CANARY-PLAIN"
	own, other := sealerFor(t), sealerFor(t)
	seal := func(s *backup.Sealer) string {
		b, err := io.ReadAll(s.Seal(strings.NewReader(dump)))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	rt, _, fake, tok := newImportFixture(t, "")
	rt.backupProtection = sealedOpenerProtection{p: &backup.Protection{Sealer: own}}

	rec := incTokenPost(rt, tok, "/api/v1/databases/main/import", seal(other))
	if rec.Code != http.StatusBadRequest || fake.restores != 0 || !strings.Contains(rec.Body.String(), "different instance key") {
		t.Fatalf("foreign export = %d restores=%d body=%s, want 400 key mismatch and no restore", rec.Code, fake.restores, rec.Body.String())
	}
	rec = incTokenPost(rt, tok, "/api/v1/databases/main/import", seal(own))
	if rec.Code != http.StatusOK || fake.restores != 1 || fake.restoredN != len(dump) {
		t.Fatalf("own export = %d restores=%d bytes=%d body=%s, want 200 with the plain dump restored", rec.Code, fake.restores, fake.restoredN, rec.Body.String())
	}
}
