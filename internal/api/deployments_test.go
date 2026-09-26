package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestParseDeploymentQuery(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		query   string
		wantErr bool
		check   func(t *testing.T, f store.DeploymentFilter)
	}{
		{"defaults", "", false, func(t *testing.T, f store.DeploymentFilter) {
			if f.Limit != deploymentsDefaultLimit {
				t.Errorf("limit = %d", f.Limit)
			}
		}},
		{"limit capped", "limit=9999", false, func(t *testing.T, f store.DeploymentFilter) {
			if f.Limit != deploymentsMaxLimit {
				t.Errorf("limit = %d", f.Limit)
			}
		}},
		{"limit zero", "limit=0", true, nil},
		{"repeated and comma statuses", "status=failed,ready&status=building", false, func(t *testing.T, f store.DeploymentFilter) {
			if len(f.Statuses) != 3 {
				t.Errorf("statuses = %v", f.Statuses)
			}
		}},
		{"bad status", "status=weird", true, nil},
		{"trigger with space", "trigger=git+push", false, func(t *testing.T, f store.DeploymentFilter) {
			if len(f.Triggers) != 1 || f.Triggers[0] != "git push" {
				t.Errorf("triggers = %v", f.Triggers)
			}
		}},
		{"bad trigger", "trigger=carrier-pigeon", true, nil},
		{"since duration", "since=7d", false, func(t *testing.T, f store.DeploymentFilter) {
			if !f.Since.Equal(now.Add(-7 * 24 * time.Hour)) {
				t.Errorf("since = %v", f.Since)
			}
		}},
		{"since rfc3339", "since=2026-09-01T00:00:00Z", false, func(t *testing.T, f store.DeploymentFilter) {
			if f.Since.Day() != 1 {
				t.Errorf("since = %v", f.Since)
			}
		}},
		{"bad since", "since=yesterday", true, nil},
		{"live", "live=true&pr=12", false, func(t *testing.T, f store.DeploymentFilter) {
			if !f.Live || f.PR != 12 {
				t.Errorf("live=%v pr=%d", f.Live, f.PR)
			}
		}},
		{"bad pr", "pr=abc", true, nil},
		{"bad live", "live=maybe", true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parseDeploymentQuery(q, now)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, f)
			}
		})
	}
}

func TestDeploymentImageRef(t *testing.T) {
	tests := []struct{ name, image, digest, reason, want string }{
		{"resolved digest pins", "reg.io/a/web:1", "sha256:abc", store.DigestReasonResolved, "reg.io/a/web@sha256:abc"},
		{"registry port kept", "reg.io:5000/web:1", "sha256:abc", store.DigestReasonPinned, "reg.io:5000/web@sha256:abc"},
		{"local build id is not a registry digest", "web:1", "sha256:abc", store.DigestReasonLocalBuild, "web:1"},
		{"no digest", "web:1", "", "", "web:1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deploymentImageRef(tt.image, tt.digest, tt.reason); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDeploymentsScopedTokenSeesOnlyItsApps(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	now := time.Now().UTC()
	for i, app := range []string{"web", "api", "worker"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: app, Image: app + ":1", Port: 80}); err != nil {
			t.Fatal(err)
		}
		if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
			ID: "dep_" + app, ServiceName: app, Image: app + ":1", Source: store.DeployAttemptSourceImage,
			Status: store.DeployAttemptStatusFailed, StartedAt: now.Add(-time.Duration(i+1) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	tok := seedMatrixToken(t, db, "scoped-tok", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-api", "Deny", "*", "app:api", store.PrincipalTypeToken, "scoped-tok")
	attachTestPolicy(t, db, "deny-worker", "Deny", "*", "app:worker", store.PrincipalTypeToken, "scoped-tok")

	list := func(mutate func(*http.Request), query string) deploymentListResponse {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments"+query, nil)
		mutate(req)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var out deploymentListResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	admin := list(func(r *http.Request) { r.AddCookie(cookie) }, "")
	if len(admin.Items) != 3 {
		t.Fatalf("admin sees %d deployments, want 3", len(admin.Items))
	}
	scoped := list(bearer(tok), "")
	if len(scoped.Items) != 1 || scoped.Items[0].App != "web" {
		t.Fatalf("scoped token sees %+v, want only web", scoped.Items)
	}
	if got := list(bearer(tok), "?app=api"); len(got.Items) != 0 {
		t.Fatalf("scoped token can read a denied app via the app filter: %+v", got.Items)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/summary?window=24h", nil)
	bearer(tok)(req)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	var sum deploymentSummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &sum); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("summary status = %d err = %v body = %s", rec.Code, err, rec.Body.String())
	}
	if sum.Counts[store.DeploymentFailed] != 1 || len(sum.PerDay) != store.DeploymentSummaryDays {
		t.Fatalf("summary leaks or misses rows: %+v", sum)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/deployments?status=nope", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad status filter code = %d, want 400", rec.Code)
	}
}
