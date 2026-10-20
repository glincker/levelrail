package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

func edgeRouter(t *testing.T, container bool) (*Router, *http.Cookie) {
	t.Helper()
	setVersion(t, "v1.0.0")
	old := runningInContainer
	runningInContainer = func() bool { return container }
	t.Cleanup(func() { runningInContainer = old })
	rt, db := newTestRouterWithFetchLatestRelease(t, func(context.Context) (*githubRelease, error) {
		return nil, errors.New("stable must not be queried on the edge channel")
	})
	if err := db.UpdateUpdateSettings(context.Background(), store.UpdateSettings{Channel: upgrade.ChannelEdge}); err != nil {
		t.Fatal(err)
	}
	WithSelfUpgrade(db, func(context.Context, []string) error { return nil })(rt)
	rt.upgradeFetchers = upgrade.Fetchers{
		Edge: func(context.Context) (*upgrade.Release, error) {
			return &upgrade.Release{
				Tag: "main-07cd3bf", URL: "https://github.com/x/y/commit/07cd3bf", Kind: upgrade.KindEdgeImage,
				Commit: "07cd3bf9a1b2", Image: "ghcr.io/x/y:edge",
			}, nil
		},
	}
	return rt, loginTestSession(t, rt, db)
}

func TestEdgeChannel_PreflightGuidance(t *testing.T) {
	tests := []struct {
		name          string
		container     bool
		wantInstall   bool
		wantPull      string
		wantSwitchLen int
	}{
		{"container install gets pull and recreate", true, true, "docker pull ghcr.io/x/y:edge", 0},
		{"binary install is offered a channel switch", false, false, "", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, cookie := edgeRouter(t, tt.container)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/updates/preflight", ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body)
			}
			var got updatePreflightResource
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.ReleaseKind != upgrade.KindEdgeImage || got.Channel != upgrade.ChannelEdge || got.Edge == nil {
				t.Fatalf("resource = %+v", got)
			}
			if got.Blocked {
				t.Errorf("edge must not be blocked: %+v", got.Checks)
			}
			for _, c := range got.Checks {
				if c.Code == "release_signature" && (c.Status != upgrade.StatusSkipped || c.Message != upgrade.EdgeNoSignatureNote) {
					t.Errorf("signature check = %+v", c)
				}
			}
			if got.Edge.Installable != tt.wantInstall || got.Edge.PullCommand != tt.wantPull || len(got.Edge.SwitchTo) != tt.wantSwitchLen {
				t.Errorf("edge = %+v", got.Edge)
			}
			if strings.Contains(got.UpgradeCommand, "install.sh") || strings.Contains(got.UpgradeCommand, "main-07cd3bf") {
				t.Errorf("upgrade command must not be a release install: %q", got.UpgradeCommand)
			}
		})
	}
}

func TestEdgeChannel_StatusAndSelfUpgradePlan(t *testing.T) {
	rt, cookie := edgeRouter(t, true)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/updates", ""))
	var st updateStatusResource
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.ReleaseKind != upgrade.KindEdgeImage || st.Channel != upgrade.ChannelEdge || st.Image == nil || !st.UpdateAvailable {
		t.Errorf("status = %+v", st)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/updates/self-upgrade/plan", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("plan status = %d: %s", rec.Code, rec.Body)
	}
	var plan selfUpgradePlanResource
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.CanApply || plan.Edge == nil || !strings.Contains(plan.Command, "docker pull ghcr.io/x/y:edge") {
		t.Errorf("plan = %+v", plan)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/updates/self-upgrade", `{"target":"main-07cd3bf"}`))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "container images") {
		t.Errorf("start edge = %d: %s", rec.Code, rec.Body)
	}
}
