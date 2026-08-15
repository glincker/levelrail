package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeDockerPruner is a hand-written fake for DockerPruner, the same
// "not a mocking framework" convention fakeDockerPinger (status_test.go)
// already establishes in this package. It records the keep list it was
// called with, so tests can assert desiredContainerNames actually
// reaches Prune unmodified.
type fakeDockerPruner struct {
	result  docker.PruneResult
	gotKeep []string
	callCnt int
}

func (f *fakeDockerPruner) Prune(_ context.Context, keep []string) docker.PruneResult {
	f.callCnt++
	f.gotKeep = keep
	return f.result
}

func newTestRouterWithDockerPruner(t *testing.T, p DockerPruner) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	return NewRouter(discardLogger(), testBrand(), db, WithDockerPruner(p)), db
}

func TestHandleSystemPrune_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithDockerPruner
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/prune", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestSystemPruneRoute_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/prune", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestHandleSystemPrune_PlainWriteToken_Forbidden proves POST
// /system/prune sits behind AbilityRoot, not AbilityWrite: this is a
// fleet-wide, no-undo action touching every stopped container/dangling
// image/anonymous volume on the daemon, the same boundary
// handleDrainNode already draws for its own fleet-wide action (see
// TestHandleSetAppNode_PlainWriteToken_Forbidden in apps_test.go for the
// identical pattern applied to node placement).
func TestHandleSystemPrune_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouterWithDockerPruner(t, &fakeDockerPruner{})
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/prune", strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach a fleet-wide destructive action", rec.Code, http.StatusForbidden)
	}
}

func TestHandleSystemPrune_Success(t *testing.T) {
	fake := &fakeDockerPruner{
		result: docker.PruneResult{
			ContainersRemoved:        []string{"leftover"},
			ContainersReclaimedBytes: 123,
			ImagesRemoved:            []string{"sha256:abc"},
			ImagesReclaimedBytes:     456,
			VolumesRemoved:           []string{"anonvol"},
			VolumesReclaimedBytes:    789,
			BuildCacheReclaimedBytes: 111,
		},
	}
	rt, db := newTestRouterWithDockerPruner(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/prune", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got systemPruneResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.ContainersRemoved) != 1 || got.ContainersRemoved[0] != "leftover" {
		t.Errorf("ContainersRemoved = %v, want [leftover]", got.ContainersRemoved)
	}
	if got.ContainersReclaimedBytes != 123 {
		t.Errorf("ContainersReclaimedBytes = %d, want 123", got.ContainersReclaimedBytes)
	}
	if got.ImagesReclaimedBytes != 456 || got.VolumesReclaimedBytes != 789 || got.BuildCacheReclaimedBytes != 111 {
		t.Errorf("reclaimed bytes = %+v, want images=456 volumes=789 buildcache=111", got)
	}
	if len(got.Errors) != 0 {
		t.Errorf("Errors = %v, want none", got.Errors)
	}
	if fake.callCnt != 1 {
		t.Errorf("Prune called %d times, want 1", fake.callCnt)
	}
}

func TestHandleSystemPrune_SurfacesPartialStageErrors(t *testing.T) {
	fake := &fakeDockerPruner{
		result: docker.PruneResult{
			ImagesRemoved: []string{"sha256:def"},
			Errors:        []string{"containers: daemon hiccup"},
		},
	}
	rt, db := newTestRouterWithDockerPruner(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/prune", ""))
	// A partial stage failure is still a real, reportable outcome, not a
	// 5xx: the same shape handleDrainNode's own partial-failure test
	// (TestHandleDrainNode_PartialFailure) already establishes, except
	// this handler reports it inside a 200 body (Errors), not 207,
	// because there's exactly one call here, not N independent resource
	// mutations to enumerate a status per.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got systemPruneResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Errors) != 1 || got.Errors[0] != "containers: daemon hiccup" {
		t.Errorf("Errors = %v, want [\"containers: daemon hiccup\"]", got.Errors)
	}
	if len(got.ImagesRemoved) != 1 {
		t.Errorf("ImagesRemoved = %v, want the one successful stage's result preserved", got.ImagesRemoved)
	}
}

// TestHandleSystemPrune_KeepListProtectsDesiredContainers is this
// handler's actual safety-relevant behavior: every currently desired
// application replica and database container name must reach
// DockerPruner.Prune as the keep list, so a real Client.PruneContainers
// (internal/docker/prune.go) never treats a stopped-but-desired
// container as garbage. Uses two replicas on one service and one
// database to prove both name shapes (base + "-rN", and "db-<name>")
// are included.
func TestHandleSystemPrune_KeepListProtectsDesiredContainers(t *testing.T) {
	fake := &fakeDockerPruner{}
	rt, db := newTestRouterWithDockerPruner(t, fake)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	svc := store.DesiredService{Name: "web", Image: "levelrail/web:v1", Port: 3000, Replicas: 2}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatalf("seed service: %v", err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/prune", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	base := application.ContainerName("web", "levelrail/web:v1", "")
	want := []string{base, base + "-r1", "db-main"}
	sort.Strings(want)
	got := append([]string(nil), fake.gotKeep...)
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("keep list = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("keep list = %v, want %v", got, want)
			break
		}
	}
}
