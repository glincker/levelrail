package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/release"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestListAppReleases(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	release.Install(db, release.ModeShadow, nil)
	ctx := context.Background()
	for i, id := range []string{"a1", "a2"} {
		if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: id, ServiceName: "web", Image: "reg.example/web:v1", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusRunning, StartedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		digest := "sha256:" + strings.Repeat(string(rune('a'+i)), 64)
		if err := db.SetDeployAttemptDigest(ctx, id, digest, store.DigestReasonStaticTree); err != nil {
			t.Fatal(err)
		}
		if err := db.FinishDeployAttempt(ctx, id, store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
			t.Fatal(err)
		}
		time.Sleep(3 * time.Millisecond)
	}

	get := func() (*httptest.ResponseRecorder, appReleasesResource) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/releases", ""))
		var got appReleasesResource
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
		}
		return rec, got
	}

	t.Setenv(release.EnvBindings, "off")
	if rec, got := get(); rec.Code != http.StatusOK || len(got.Releases) != 0 || !strings.Contains(rec.Body.String(), `"releases":[]`) {
		t.Fatalf("off: code %d body %s", rec.Code, rec.Body.String())
	}

	t.Setenv(release.EnvBindings, "shadow")
	rec, got := get()
	if rec.Code != http.StatusOK || len(got.Releases) != 2 {
		t.Fatalf("shadow: code %d body %s", rec.Code, rec.Body.String())
	}
	if got.Releases[0].SourceAttemptID != "a2" || got.Releases[1].SourceAttemptID != "a1" {
		t.Errorf("order = %s, %s, want a2 then a1", got.Releases[0].SourceAttemptID, got.Releases[1].SourceAttemptID)
	}
	if got.CurrentReleaseID != got.Releases[0].ID {
		t.Errorf("current = %q, want newest %q", got.CurrentReleaseID, got.Releases[0].ID)
	}
	if got.Releases[0].ShortDigest != "sha256:bbbbbbbbbbbb" {
		t.Errorf("short digest = %q", got.Releases[0].ShortDigest)
	}
	var raw struct {
		Releases []map[string]any `json:"releases"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &raw)
	for _, k := range []string{"config", "commit_sha", "created_by"} {
		if _, has := raw.Releases[0][k]; has {
			t.Errorf("response exposes %q", k)
		}
	}
}
