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
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

func TestHandleGetUpdateSettings_Default(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/updates/settings", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got updateSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := updateSettingsResource{Channel: upgrade.ChannelStable, AutoUpdateEnabled: false}
	if got != want {
		t.Errorf("GET /updates/settings = %+v, want the seeded default %+v", got, want)
	}
}

func TestHandleUpdateSettings_RejectsInvalidChannel(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/updates/settings", `{"channel":"nightly"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	getRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/updates/settings", ""))
	var got updateSettingsResource
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Channel != upgrade.ChannelStable {
		t.Errorf("channel = %q after a rejected update, want the row left unchanged at %q", got.Channel, upgrade.ChannelStable)
	}
}

func TestHandleUpdateSettings_RoundTrip(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"channel":"beta","auto_update_enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/updates/settings", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	want := updateSettingsResource{Channel: "beta", AutoUpdateEnabled: true}
	var got updateSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != want {
		t.Errorf("PUT response = %+v, want %+v", got, want)
	}

	getRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/updates/settings", ""))
	var reread updateSettingsResource
	if err := json.Unmarshal(getRec.Body.Bytes(), &reread); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if reread != want {
		t.Errorf("GET after PUT = %+v, want %+v", reread, want)
	}
}

func TestHandleUpdateSettings_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/updates/settings", nil)
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d for an unauthenticated request", rec.Code, http.StatusUnauthorized)
	}
}

// TestHandleUpdateSettings_PlainWriteToken_Forbidden proves this sits
// behind AbilityRoot, not merely AbilityWrite, matching PUT
// /api/v1/settings/ingress's own precedent (ingress_settings_test.go).
func TestHandleUpdateSettings_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	const plaintext = "write-scoped-token-updates" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write_updates", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/updates/settings", strings.NewReader(`{"channel":"beta"}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach the channel toggle", rec.Code, http.StatusForbidden)
	}
}

// TestHandleGetUpdates_ChannelAware proves GET /api/v1/updates dispatches
// to the configured channel's own fetcher: stable keeps using the
// pre-existing fetchLatestRelease/updatesCache path (already covered by
// TestHandleGetUpdates_* in updates_test.go), beta and edge go through
// upgradeFetchers instead.
func TestHandleGetUpdates_ChannelAware(t *testing.T) {
	setVersion(t, "v1.0.0")

	tests := []struct {
		channel    string
		wantLatest string
	}{
		{upgrade.ChannelBeta, "v1.1.0-beta.1"},
		{upgrade.ChannelEdge, "main-abc1234"},
	}
	for _, tt := range tests {
		t.Run(tt.channel, func(t *testing.T) {
			rt, db := newTestRouter(t)
			rt.upgradeFetchers = upgrade.Fetchers{
				Stable: func(context.Context) (*upgrade.Release, error) { return nil, nil },
				Beta: func(context.Context) (*upgrade.Release, error) {
					return &upgrade.Release{Tag: "v1.1.0-beta.1", URL: "https://example.com/beta"}, nil
				},
				Edge: func(context.Context) (*upgrade.Release, error) {
					return &upgrade.Release{Tag: "main-abc1234", URL: "https://example.com/edge"}, nil
				},
			}
			cookie := loginTestSession(t, rt, db)

			putRec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(putRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/updates/settings", `{"channel":"`+tt.channel+`"}`))
			if putRec.Code != http.StatusOK {
				t.Fatalf("set channel: status = %d, body = %s", putRec.Code, putRec.Body.String())
			}

			got := getUpdatesOK(t, rt, cookie)
			if got.LatestVersion == nil || *got.LatestVersion != tt.wantLatest {
				t.Errorf("latest_version = %v, want %q", got.LatestVersion, tt.wantLatest)
			}
			if !got.UpdateAvailable {
				t.Error("update_available = false, want true")
			}
		})
	}
}
