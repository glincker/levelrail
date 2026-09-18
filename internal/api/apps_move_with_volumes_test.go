package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeMoveVolumeRuntime is a hand-written fake docker.Runtime purpose-built
// for exercising runAppVolumeMove end to end, the same "compile-satisfying
// stub except the methods actually exercised" convention fakeExecAppRuntime
// (exec_test.go) already establishes. Exec backs the archive side of
// backup.ContainerVolumeArchiver, ExecWithInput backs the restore side of
// backup.ContainerVolumeRestorer; one instance stands in for however many
// distinct nodes a test resolves to, since none of this needs to be
// node-specific to prove the orchestration sequence is correct.
type fakeMoveVolumeRuntime struct {
	volumeContent string
	archiveErr    error
	restoreErr    error

	mu             sync.Mutex
	restoredBodies []string
}

func (f *fakeMoveVolumeRuntime) InspectByName(context.Context, string) (*docker.ContainerState, error) {
	return nil, nil
}
func (f *fakeMoveVolumeRuntime) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	return "helper-" + spec.Name, nil
}
func (f *fakeMoveVolumeRuntime) Start(context.Context, string) error { return nil }
func (f *fakeMoveVolumeRuntime) Events(context.Context) (<-chan docker.Event, <-chan error) {
	return nil, nil
}
func (f *fakeMoveVolumeRuntime) ListImages(context.Context, string) ([]docker.ImageInfo, error) {
	return nil, nil
}
func (f *fakeMoveVolumeRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return nil, nil
}
func (f *fakeMoveVolumeRuntime) Stop(context.Context, string, time.Duration) error { return nil }
func (f *fakeMoveVolumeRuntime) Remove(context.Context, string, bool) error        { return nil }
func (f *fakeMoveVolumeRuntime) UpdateResources(context.Context, string, docker.Resources) error {
	return nil
}
func (f *fakeMoveVolumeRuntime) EnsureVolume(context.Context, string) error { return nil }
func (f *fakeMoveVolumeRuntime) EnsureNetwork(context.Context, string) (string, error) {
	return "", nil
}
func (f *fakeMoveVolumeRuntime) RemoveNetwork(context.Context, string) error { return nil }
func (f *fakeMoveVolumeRuntime) ListNetworksByPrefix(context.Context, string) ([]docker.NetworkInfo, error) {
	return nil, nil
}

func (f *fakeMoveVolumeRuntime) Exec(_ context.Context, _ string, _ []string) (io.ReadCloser, error) {
	if f.archiveErr != nil {
		return nil, f.archiveErr
	}
	return io.NopCloser(strings.NewReader(f.volumeContent)), nil
}

func (f *fakeMoveVolumeRuntime) ExecWithInput(_ context.Context, _ string, _ []string, in io.Reader) (io.ReadCloser, error) {
	if f.restoreErr != nil {
		return nil, f.restoreErr
	}
	body, err := io.ReadAll(in)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.restoredBodies = append(f.restoredBodies, string(body))
	f.mu.Unlock()
	return io.NopCloser(strings.NewReader("")), nil
}

func (f *fakeMoveVolumeRuntime) gotRestoredBodies() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.restoredBodies...)
}

var _ docker.Runtime = (*fakeMoveVolumeRuntime)(nil)

func newTestRouterWithMoveRuntime(t *testing.T, fake docker.Runtime) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	resolver := func(string) (docker.Runtime, error) { return fake, nil }
	return NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(resolver)), db
}

// awaitAppVolumeMoveDone polls db.GetAppVolumeMove until Status is no
// longer "running" or the deadline passes, the store-level counterpart of
// fakeVolumeRestoreRunner.awaitCall's own "poll instead of sleep a fixed
// guess" reasoning: runAppVolumeMove runs in a detached goroutine, so the
// test has no other signal for when it's actually done.
func awaitAppVolumeMoveDone(t *testing.T, db *store.DB, moveID string) store.AppVolumeMove {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m, err := db.GetAppVolumeMove(context.Background(), moveID)
		if err != nil {
			t.Fatalf("GetAppVolumeMove(%q) error = %v", moveID, err)
		}
		if m.Status != store.BackupStatusRunning {
			return m
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("app volume move %q did not finish within the deadline", moveID)
	return store.AppVolumeMove{}
}

func TestHandleMoveAppWithVolumes_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithExecRuntime
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	seedNode(t, db, "node_1", "worker-1")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/move-with-volumes", `{"node_id":"node_1"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleMoveAppWithVolumes_UnknownNode_Rejected(t *testing.T) {
	fake := &fakeMoveVolumeRuntime{volumeContent: "tar-bytes"}
	rt, db := newTestRouterWithMoveRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/move-with-volumes", `{"node_id":"nonexistent"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.NodeID != "" {
		t.Errorf("stored NodeID = %q, want unchanged (empty): a rejected request must not partially apply", svc.NodeID)
	}
}

// TestHandleMoveAppWithVolumes_NoVolumes_PlainMove proves an app with no
// named volumes takes the synchronous plain-move path: a 200 with an
// already-succeeded move record, not a 202 kicking off a background job
// with nothing for it to actually do.
func TestHandleMoveAppWithVolumes_NoVolumes_PlainMove(t *testing.T) {
	fake := &fakeMoveVolumeRuntime{}
	rt, db := newTestRouterWithMoveRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	seedNode(t, db, "node_1", "worker-1")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/move-with-volumes", `{"node_id":"node_1"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got appVolumeMoveResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != store.BackupStatusSucceeded || got.ToNodeID != "node_1" {
		t.Errorf("got = %+v, want status=succeeded to_node_id=node_1", got)
	}
	if len(got.Steps) != 1 || got.Steps[0].Name != "update_placement" || got.Steps[0].Status != store.BackupStatusSucceeded {
		t.Errorf("steps = %+v, want one succeeded update_placement step", got.Steps)
	}

	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.NodeID != "node_1" {
		t.Errorf("stored NodeID = %q, want node_1", svc.NodeID)
	}

	moves, err := db.ListAppVolumeMoves(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListAppVolumeMoves() error = %v", err)
	}
	if len(moves) != 1 || moves[0].Status != store.BackupStatusSucceeded {
		t.Errorf("ListAppVolumeMoves() = %+v, want exactly one succeeded record", moves)
	}
}

// TestHandleMoveAppWithVolumes_SameNode_NoOp proves requesting a move to
// the node an app is already on is a harmless no-op: no teardown dispatched
// (there is no "old node" to clean up), same synchronous 200 shape as the
// no-volumes case.
func TestHandleMoveAppWithVolumes_SameNode_NoOp(t *testing.T) {
	fake := &fakeMoveVolumeRuntime{volumeContent: "tar-bytes"}
	rt, db := newTestRouterWithMoveRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/move-with-volumes", `{"node_id":""}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got appVolumeMoveResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != store.BackupStatusSucceeded {
		t.Errorf("Status = %q, want succeeded", got.Status)
	}
}

// TestHandleMoveAppWithVolumes_WithVolumes_Success proves the full
// stop -> archive -> restore -> update_placement -> resume sequence: the
// app ends up on the new node, suspended is cleared again, and the
// destination's helper container was restored with the exact bytes the
// source's helper container archived.
func TestHandleMoveAppWithVolumes_WithVolumes_Success(t *testing.T) {
	fake := &fakeMoveVolumeRuntime{volumeContent: "tar-bytes"}
	rt, db := newTestRouterWithMoveRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	seedNode(t, db, "node_1", "worker-1")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/move-with-volumes", `{"node_id":"node_1"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	var accepted appVolumeMoveResource
	if err := json.Unmarshal(rec.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if accepted.Status != store.BackupStatusRunning || accepted.FromNodeID != "" || accepted.ToNodeID != "node_1" {
		t.Fatalf("accepted response = %+v, want status=running from=\"\" to=node_1", accepted)
	}

	final := awaitAppVolumeMoveDone(t, db, accepted.ID)
	if final.Status != store.BackupStatusSucceeded {
		t.Fatalf("final move = %+v, want status=succeeded", final)
	}
	wantSteps := []string{"stop_app", "move_volume:app-web-data", "update_placement", "resume_app"}
	if len(final.Steps) != len(wantSteps) {
		t.Fatalf("steps = %+v, want %d steps: %v", final.Steps, len(wantSteps), wantSteps)
	}
	for i, name := range wantSteps {
		if final.Steps[i].Name != name || final.Steps[i].Status != store.BackupStatusSucceeded {
			t.Errorf("step[%d] = %+v, want name=%q status=succeeded", i, final.Steps[i], name)
		}
	}

	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.NodeID != "node_1" {
		t.Errorf("stored NodeID = %q, want node_1", svc.NodeID)
	}
	if svc.Suspended {
		t.Error("service left Suspended after a successful move, want resumed")
	}

	if bodies := fake.gotRestoredBodies(); len(bodies) != 1 || bodies[0] != "tar-bytes" {
		t.Errorf("restored bodies = %v, want exactly one restore of the archived content", bodies)
	}
}

// TestHandleMoveAppWithVolumes_ArchiveFailure_RecordsPartialFailure proves a
// mid-flight failure is diagnosable rather than a black box: the failing
// step is recorded with its error, node_id is left untouched (never even
// attempted), and the app stays suspended rather than being guessed back
// into a running state on a node whose volume was never actually copied.
func TestHandleMoveAppWithVolumes_ArchiveFailure_RecordsPartialFailure(t *testing.T) {
	archiveErr := errors.New("docker daemon unreachable")
	fake := &fakeMoveVolumeRuntime{archiveErr: archiveErr}
	rt, db := newTestRouterWithMoveRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	seedNode(t, db, "node_1", "worker-1")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/move-with-volumes", `{"node_id":"node_1"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var accepted appVolumeMoveResource
	if err := json.Unmarshal(rec.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("decode: %v", err)
	}

	final := awaitAppVolumeMoveDone(t, db, accepted.ID)
	if final.Status != store.BackupStatusFailed {
		t.Fatalf("final move = %+v, want status=failed", final)
	}
	if final.Error == "" {
		t.Error("Error left empty on a failed move")
	}
	if len(final.Steps) != 2 {
		t.Fatalf("steps = %+v, want exactly [stop_app, move_volume:...] (nothing past the failure)", final.Steps)
	}
	if final.Steps[0].Name != "stop_app" || final.Steps[0].Status != store.BackupStatusSucceeded {
		t.Errorf("step[0] = %+v, want stop_app succeeded", final.Steps[0])
	}
	if final.Steps[1].Name != "move_volume:app-web-data" || final.Steps[1].Status != store.BackupStatusFailed {
		t.Errorf("step[1] = %+v, want move_volume:app-web-data failed", final.Steps[1])
	}

	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.NodeID != "" {
		t.Errorf("stored NodeID = %q, want unchanged (empty): placement must not move until every volume has already copied", svc.NodeID)
	}
	if !svc.Suspended {
		t.Error("service resumed after a failed move, want left stopped so an operator notices before it serves with a missing volume")
	}
}

func TestHandleListAppVolumeMoves_Success(t *testing.T) {
	fake := &fakeMoveVolumeRuntime{volumeContent: "tar-bytes"}
	rt, db := newTestRouterWithMoveRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	seedNode(t, db, "node_1", "worker-1")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/move-with-volumes", `{"node_id":"node_1"}`))
	var accepted appVolumeMoveResource
	if err := json.Unmarshal(rec.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("decode: %v", err)
	}
	awaitAppVolumeMoveDone(t, db, accepted.ID)

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/moves", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var list []appVolumeMoveResource
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 || list[0].ID != accepted.ID {
		t.Fatalf("list = %+v, want exactly the one move just triggered", list)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/moves/"+accepted.ID, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHandleGetAppVolumeMove_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/moves/avm_missing", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleGetAppVolumeMove_WrongApp(t *testing.T) {
	fake := &fakeMoveVolumeRuntime{volumeContent: "tar-bytes"}
	rt, db := newTestRouterWithMoveRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "other", Image: "levelrail/other:1", Port: 3000}); err != nil {
		t.Fatalf("seed other app: %v", err)
	}
	seedNode(t, db, "node_1", "worker-1")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/move-with-volumes", `{"node_id":"node_1"}`))
	var accepted appVolumeMoveResource
	if err := json.Unmarshal(rec.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("decode: %v", err)
	}
	awaitAppVolumeMoveDone(t, db, accepted.ID)

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/other/moves/"+accepted.ID, ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
