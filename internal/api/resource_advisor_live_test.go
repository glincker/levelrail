package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// TestResourceAdvisor_ApplyMemoryLimit_Live seeds the screenshot set, applies
// the suggested memory limit to a real running container and reads the
// limit back from the daemon, then checks the flag hides it all.
func TestResourceAdvisor_ApplyMemoryLimit_Live(t *testing.T) {
	dockertest.SkipIfShort(t)
	client, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	name := databaseContainerName("askverdict")
	if state, err := client.InspectByName(ctx, name); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	} else if state != nil {
		_ = client.Remove(ctx, state.ID, true)
	}
	id, err := client.Create(ctx, docker.ContainerSpec{Name: name, Image: "nginx:alpine"})
	if err != nil {
		t.Skipf("could not create the test container: %v", err)
	}
	t.Cleanup(func() { _ = client.Remove(context.Background(), id, true) })
	if err := client.Start(ctx, id); err != nil {
		t.Fatalf("start: %v", err)
	}

	db := openTestDB(t)
	tdb := newTestTelemetryDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db,
		WithTelemetryQuerier(telemetry.NewLocalFederator(tdb)),
		WithExecRuntime(func(string) (docker.Runtime, error) { return client, nil }))
	cookie := loginTestSession(t, rt, db)
	seedAdvisorScreenshot(t, db, tdb)

	findingID := "memory_limit_missing:database:askverdict"
	prev := advisorPost(t, rt, cookie, findingID, "apply", "")
	var preview advisorApplyResponse
	if err := json.Unmarshal(prev.Body.Bytes(), &preview); err != nil || !preview.ConfirmRequired {
		t.Fatalf("preview = %d %s", prev.Code, prev.Body.String())
	}
	before, err := client.Stats(ctx, id)
	if err != nil {
		t.Fatalf("stats before: %v", err)
	}
	if sameBytes(before.MemoryLimitBytes, preview.After.MemoryBytes) {
		t.Fatal("container already had the suggested limit before confirm")
	}

	rec := advisorPost(t, rt, cookie, findingID, "apply", `{"confirm":true}`)
	var done advisorApplyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &done); err != nil || !done.Applied || !done.AppliedLive {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		after, err := client.Stats(ctx, id)
		if err == nil && sameBytes(after.MemoryLimitBytes, preview.After.MemoryBytes) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("daemon limit = %d, want %d (err %v)", after.MemoryLimitBytes, preview.After.MemoryBytes, err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	experimental.Set()
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	off := httptest.NewRecorder()
	rt.Handler().ServeHTTP(off, authedRequest(t, cookie, http.MethodGet, "/api/v1/resource-advisor/findings", ""))
	if off.Code != http.StatusNotFound {
		t.Fatalf("flag off = %d, want 404", off.Code)
	}
}

func sameBytes(daemon uint64, want int64) bool {
	return strconv.FormatUint(daemon, 10) == strconv.FormatInt(want, 10)
}
