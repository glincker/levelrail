package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// transferRuntime fakes the helper container: Exec lists an empty volume or tars fixed bytes, ExecWithInput records the stream.
type transferRuntime struct {
	bareRuntime
	listing   string
	exported  string
	exportErr error // when set, the export stream fails after exported is read
	extracted []byte
	mounts    []docker.VolumeMount
}

func (f *transferRuntime) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	f.mounts = append(f.mounts, spec.Volumes...)
	return "helper", nil
}
func (f *transferRuntime) Start(context.Context, string) error        { return nil }
func (f *transferRuntime) Remove(context.Context, string, bool) error { return nil }
func (f *transferRuntime) Exec(_ context.Context, _ string, cmd []string) (io.ReadCloser, error) {
	if cmd[0] == "tar" {
		if f.exportErr != nil {
			return io.NopCloser(io.MultiReader(strings.NewReader(f.exported), errReader{f.exportErr})), nil
		}
		return io.NopCloser(strings.NewReader(f.exported)), nil
	}
	return io.NopCloser(strings.NewReader(f.listing)), nil
}
func (f *transferRuntime) ExecWithInput(_ context.Context, _ string, _ []string, in io.Reader) (io.ReadCloser, error) {
	f.extracted, _ = io.ReadAll(in)
	return io.NopCloser(strings.NewReader("")), nil
}

func transferRouter(t *testing.T, f *transferRuntime) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(func(string) (docker.Runtime, error) { return f, nil }))
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	return rt, db, cookie
}

func gzTar(t *testing.T, name, body string) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	_, _ = tw.Write([]byte(body))
	_ = tw.Close()
	_ = gz.Close()
	return buf.String()
}

func TestVolumeTransferRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/volumes/data/export"},
		{http.MethodPost, "/api/v1/apps/web/volumes/data/import"},
	})
}

func TestVolumeExport_StreamsReadOnlyAndAudits(t *testing.T) {
	f := &transferRuntime{exported: "ARCHIVE-BYTES"}
	rt, db, cookie := transferRouter(t, f)
	rec := do(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/volumes/data/export", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "ARCHIVE-BYTES" {
		t.Fatalf("export = %d %q", rec.Code, rec.Body.String())
	}
	if len(f.mounts) != 1 || !f.mounts[0].ReadOnly {
		t.Fatalf("export must mount read-only: %+v", f.mounts)
	}
	entries, err := db.ListAuditEntries(context.Background(), 20, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Action == AuditActionVolumeExport {
			found = true
			if !strings.Contains(e.Detail, `"bytes":13`) || strings.Contains(e.Detail, "ARCHIVE") {
				t.Fatalf("audit detail = %s", e.Detail)
			}
		}
	}
	if !found {
		t.Fatal("no volume.export audit entry")
	}
}

func TestVolumeImport_NeedsStoppedAppAndEmptyVolume(t *testing.T) {
	f := &transferRuntime{}
	rt, db, cookie := transferRouter(t, f)
	body := gzTar(t, "a.txt", "hello")
	const target = "/api/v1/apps/web/volumes/data/import"

	if rec := do(t, rt, cookie, http.MethodPost, target, body); rec.Code != http.StatusConflict {
		t.Fatalf("running app = %d, want 409", rec.Code)
	}
	if err := db.UpdateServiceSuspended(context.Background(), "web", true); err != nil {
		t.Fatal(err)
	}
	rec := do(t, rt, cookie, http.MethodPost, target, body)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"entries":1`) {
		t.Fatalf("import = %d %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(f.extracted, []byte("hello")) {
		t.Fatal("extractor never got the file")
	}

	f.listing = "old.txt\n"
	if rec := do(t, rt, cookie, http.MethodPost, target, body); rec.Code != http.StatusConflict {
		t.Fatalf("populated volume = %d, want 409", rec.Code)
	}
	if rec := do(t, rt, cookie, http.MethodPost, target+"?force=true", body); rec.Code != http.StatusForbidden {
		t.Fatalf("force with setting off = %d, want 403", rec.Code)
	}
}

func TestVolumeImport_RejectsTraversal(t *testing.T) {
	f := &transferRuntime{}
	rt, db, cookie := transferRouter(t, f)
	if err := db.UpdateServiceSuspended(context.Background(), "web", true); err != nil {
		t.Fatal(err)
	}
	rec := do(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/import", gzTar(t, "../../etc/cron.d/x", "boom"))
	if rec.Code != http.StatusBadRequest || f.extracted != nil {
		t.Fatalf("traversal = %d extracted=%q", rec.Code, f.extracted)
	}
}
