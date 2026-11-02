package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func saveVolumeApp(t *testing.T, db *store.DB, name, logical string) {
	t.Helper()
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{
		Name: name, Image: "levelrail/thesvg:abc1234",
		Volumes: []store.ServiceVolume{{Name: store.ServiceVolumeName(name, logical), ContainerPath: "/data"}},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestVolumeNameCollision_BlocksAttachAndTransfer(t *testing.T) {
	f := &transferRuntime{exported: "X"}
	rt, db, cookie := transferRouter(t, f)
	// App "a-b" owns app-a-b-c; app "a" with volume "b-c" maps to the same Docker volume.
	saveVolumeApp(t, db, "a-b", "c")
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "a", Image: "levelrail/thesvg:abc1234"}); err != nil {
		t.Fatal(err)
	}
	rec := do(t, rt, cookie, http.MethodPut, "/api/v1/apps/a/volumes", `{"volumes":[{"name":"b-c","container_path":"/data"}]}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "a-b") {
		t.Fatalf("colliding attach = %d %s, want 409 naming the other app", rec.Code, rec.Body.String())
	}
	if rec := do(t, rt, cookie, http.MethodPut, "/api/v1/apps/a/volumes", `{"volumes":[{"name":"other","container_path":"/data"}]}`); rec.Code != http.StatusOK {
		t.Fatalf("non-colliding attach = %d %s", rec.Code, rec.Body.String())
	}

	// A collision that already exists in the store must stop data movement for both owners.
	saveVolumeApp(t, db, "a", "b-c")
	for _, path := range []string{"/api/v1/apps/a/volumes/b-c/export", "/api/v1/apps/a-b/volumes/c/export"} {
		if rec := do(t, rt, cookie, http.MethodGet, path, ""); rec.Code != http.StatusConflict {
			t.Fatalf("%s = %d, want 409", path, rec.Code)
		}
	}
	if len(f.mounts) != 0 {
		t.Fatalf("a shared volume must never be mounted: %+v", f.mounts)
	}
}

func TestVolumeImport_AppStartedDuringUploadIsRefused(t *testing.T) {
	f := &transferRuntime{}
	rt, db, cookie := transferRouter(t, f)
	if err := db.UpdateServiceSuspended(context.Background(), "web", true); err != nil {
		t.Fatal(err)
	}
	// The app is started while the body is still arriving.
	body := &startOnRead{r: strings.NewReader(gzTar(t, "a.txt", "hello")), start: func() {
		_ = db.UpdateServiceSuspended(context.Background(), "web", false)
	}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/volumes/data/import", body)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("import = %d %s, want 409", rec.Code, rec.Body.String())
	}
	if len(f.mounts) != 0 || f.extracted != nil {
		t.Fatalf("the volume was mounted or written for a running app: mounts=%v", f.mounts)
	}
}

type startOnRead struct {
	r     *strings.Reader
	start func()
	done  bool
}

func (s *startOnRead) Read(p []byte) (int, error) {
	if !s.done {
		s.done = true
		s.start()
	}
	return s.r.Read(p)
}

func TestVolumeImport_InProgressBlocksStartAndSecondImport(t *testing.T) {
	f := &transferRuntime{}
	rt, db, cookie := transferRouter(t, f)
	if err := db.UpdateServiceSuspended(context.Background(), "web", true); err != nil {
		t.Fatal(err)
	}
	release, err := rt.volumeTransfers.beginImport("web", "app-web-data", 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if rec := do(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/start", ""); rec.Code != http.StatusConflict {
		t.Fatalf("start during import = %d, want 409", rec.Code)
	}
	if rec := do(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/import", gzTar(t, "a", "x")); rec.Code != http.StatusConflict {
		t.Fatalf("second import on the same volume = %d, want 409", rec.Code)
	}
	release()
	if rec := do(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/start", ""); rec.Code != http.StatusOK {
		t.Fatalf("start after import = %d %s", rec.Code, rec.Body.String())
	}
}

func TestVolumeImport_CapsConcurrentImports(t *testing.T) {
	f := &transferRuntime{}
	rt, db, cookie := transferRouter(t, f)
	if err := db.UpdateServiceSuspended(context.Background(), "web", true); err != nil {
		t.Fatal(err)
	}
	release, err := rt.volumeTransfers.beginImport("other", "app-other-data", 1, "")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if rec := do(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/import", gzTar(t, "a", "x")); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("import over the cap = %d, want 429", rec.Code)
	}
}

func TestVolumeExport_CutOffAbortsTheResponse(t *testing.T) {
	f := &transferRuntime{exported: "PARTIAL", exportErr: backup.ErrTransferTooLarge}
	rt, _, cookie := transferRouter(t, f)
	rec := httptest.NewRecorder()
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/volumes/data/export", ""))
	}()
	if err, ok := recovered.(error); !ok || !errors.Is(err, http.ErrAbortHandler) {
		t.Fatalf("a cut-off export must abort the response, recovered %v (status %d)", recovered, rec.Code)
	}
}

func TestVolumeImport_SpoolUnderDataDirAndStartupSweep(t *testing.T) {
	data := t.TempDir()
	spool := filepath.Join(data, "volume-transfer")
	if err := os.MkdirAll(spool, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(spool, backup.SpoolFilePrefix+"old.tar")
	if err := os.WriteFile(stale, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithDataDir(data))
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale spool file survived startup")
	}
	if got := rt.transferSpoolDir(context.Background()); got != spool {
		t.Fatalf("spool dir = %q, want %q", got, spool)
	}
}

func TestVolumeImport_SpoolStagesNextToDataDir(t *testing.T) {
	data := t.TempDir()
	var staged atomic.Bool
	f := &spoolProbeRuntime{transferRuntime: transferRuntime{}, dir: filepath.Join(data, "volume-transfer"), staged: &staged}
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithDataDir(data), WithExecRuntime(func(string) (docker.Runtime, error) { return f, nil }))
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	if err := db.UpdateServiceSuspended(context.Background(), "web", true); err != nil {
		t.Fatal(err)
	}
	if rec := do(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/import", gzTar(t, "a", "x")); rec.Code != http.StatusOK {
		t.Fatalf("import = %d %s", rec.Code, rec.Body.String())
	}
	if !staged.Load() {
		t.Fatal("the archive was not staged under the data dir spool folder")
	}
}

// spoolProbeRuntime looks in the spool dir at the moment the volume is mounted.
type spoolProbeRuntime struct {
	transferRuntime
	dir    string
	staged *atomic.Bool
}

func (f *spoolProbeRuntime) Create(ctx context.Context, spec docker.ContainerSpec) (string, error) {
	ents, err := os.ReadDir(f.dir)
	if err == nil && len(ents) == 1 {
		f.staged.Store(true)
	}
	return f.transferRuntime.Create(ctx, spec)
}

func TestImportErrorStatus_Guards(t *testing.T) {
	if s, _ := importErrorStatus(errImportCapacity); s != http.StatusTooManyRequests {
		t.Fatalf("capacity = %d", s)
	}
	if s, _ := importErrorStatus(errors.Join(errImportVolumeBusy)); s != http.StatusConflict {
		t.Fatalf("busy = %d", s)
	}
}

func TestWakeAndStartPaths_RefusedDuringImport(t *testing.T) {
	f := &transferRuntime{}
	rt, _, cookie := transferRouter(t, f)
	release, err := rt.volumeTransfers.beginImport("web", "app-web-data", 1, "")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if rec := do(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/move-with-volumes", `{"node_id":"n2"}`); rec.Code == http.StatusAccepted {
		t.Fatalf("volume move during import = %d, want refusal", rec.Code)
	}
	ex := &moveExecutor{rt: rt}
	if err := ex.Resume(context.Background(), store.NodeMoveKindApp, "web"); !errors.Is(err, backup.ErrImportInProgress) {
		t.Fatalf("node move resume during import = %v", err)
	}
}

func TestCloneRejectsForeignVolumeNameCollision(t *testing.T) {
	f := &transferRuntime{}
	rt, db, _ := transferRouter(t, f)
	saveVolumeApp(t, db, "a-b", "c")
	clash := store.DesiredService{Name: "a", Image: "x:1", Volumes: []store.ServiceVolume{{Name: "app-a-b-c", ContainerPath: "/d"}}}
	other, err := rt.cloneVolumeConflict(context.Background(), clash)
	if err != nil || other != "a-b" {
		t.Fatalf("other=%q err=%v, want a-b", other, err)
	}
}
