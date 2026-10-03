package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TestHandleBadgeSettings_GetDefaultOffAndSetToggles proves the one
// place this endpoint deliberately differs from GET/PUT .../exec-access:
// default false, not true, so an app that never touches its badge
// setting stays unexposed on GET .../badge.svg.
func TestHandleBadgeSettings_GetDefaultOffAndSetToggles(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	recGet := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recGet, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/badge", ""))
	if recGet.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recGet.Code, http.StatusOK)
	}
	var got badgeSettingsResource
	if err := json.Unmarshal(recGet.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Enabled {
		t.Error("Enabled = true, want false: off by default")
	}

	recSet := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recSet, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/badge", `{"enabled":true}`))
	if recSet.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recSet.Code, http.StatusOK, recSet.Body.String())
	}

	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService: %v", err)
	}
	if !svc.BadgeEnabled {
		t.Error("BadgeEnabled = false after enabling, want true")
	}

	recGetAgain := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recGetAgain, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/badge", ""))
	var gotAgain badgeSettingsResource
	if err := json.Unmarshal(recGetAgain.Body.Bytes(), &gotAgain); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !gotAgain.Enabled {
		t.Error("Enabled = false after enabling, want true")
	}

	recMissing := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recMissing, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/ghost/badge", `{"enabled":true}`))
	if recMissing.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recMissing.Code, http.StatusNotFound)
	}
}

// TestHandleSetBadgeSettings_PlainWriteToken_Forbidden proves PUT
// .../badge sits behind AbilityRoot: a token scoped to nothing but
// AbilityWrite cannot opt an app into public exposure.
func TestHandleSetBadgeSettings_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/badge", strings.NewReader(`{"enabled":true}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not be able to enable the public badge", rec.Code, http.StatusForbidden)
	}
}

// TestHandlePublicAppBadge_404sWhenDisabledOrMissing proves the public
// badge.svg route cannot be used to distinguish "no such app" from
// "app exists, badge off": both answer with a plain 404, unauthenticated.
func TestHandlePublicAppBadge_404sWhenDisabledOrMissing(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	for _, path := range []string{"/api/v1/apps/web/badge.svg", "/api/v1/apps/ghost/badge.svg"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s (unauthenticated) = %d, want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}

// TestHandlePublicAppBadge_RendersStatusAndColor proves the public SVG
// reflects the real latest deploy attempt: success renders green,
// failed renders red, and an app with badge enabled but no deploy
// attempts yet still renders a valid "no deploys" badge rather than
// erroring.
func TestHandlePublicAppBadge_RendersStatusAndColor(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	cases := []struct {
		name       string
		seed       func(t *testing.T)
		wantColor  string
		wantSubstr string
	}{
		{
			name:       "no-deploys",
			seed:       func(*testing.T) {},
			wantColor:  badgeColorNone,
			wantSubstr: "no deploys",
		},
		{
			name: "succeeded",
			seed: func(t *testing.T) {
				if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
					ID: "dep_ok", ServiceName: "succeeded", Image: "levelrail/web:1",
					Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusSucceeded,
					StartedAt: time.Now().Add(-2 * time.Hour),
				}); err != nil {
					t.Fatalf("seed attempt: %v", err)
				}
			},
			wantColor:  badgeColorSuccess,
			wantSubstr: "success",
		},
		{
			name: "failed",
			seed: func(t *testing.T) {
				if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
					ID: "dep_fail", ServiceName: "failed", Image: "levelrail/web:1",
					Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusFailed,
					StartedAt: time.Now().Add(-10 * time.Minute),
				}); err != nil {
					t.Fatalf("seed attempt: %v", err)
				}
			},
			wantColor:  badgeColorFailed,
			wantSubstr: "failed",
		},
		{
			name: "canceled-renders-as-unknown",
			seed: func(t *testing.T) {
				if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
					ID: "dep_cancel", ServiceName: "canceled-renders-as-unknown", Image: "levelrail/web:1",
					Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusCanceled,
					StartedAt: time.Now(),
				}); err != nil {
					t.Fatalf("seed attempt: %v", err)
				}
			},
			wantColor:  badgeColorNone,
			wantSubstr: "unknown",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := db.SaveDesiredService(ctx, store.DesiredService{Name: c.name, Image: "levelrail/web:1", Port: 3000}); err != nil {
				t.Fatalf("seed app: %v", err)
			}
			if err := db.SetServiceBadgeEnabled(ctx, c.name, true); err != nil {
				t.Fatalf("SetServiceBadgeEnabled: %v", err)
			}
			c.seed(t)

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/apps/"+c.name+"/badge.svg", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "image/svg+xml") {
				t.Errorf("Content-Type = %q, want image/svg+xml", ct)
			}
			body := rec.Body.String()
			if !strings.Contains(body, "<svg") {
				t.Errorf("body does not look like SVG: %s", body)
			}
			if !strings.Contains(body, c.wantColor) {
				t.Errorf("body does not contain expected color %q: %s", c.wantColor, body)
			}
			if !strings.Contains(body, c.wantSubstr) {
				t.Errorf("body does not contain expected text %q: %s", c.wantSubstr, body)
			}
		})
	}
}
