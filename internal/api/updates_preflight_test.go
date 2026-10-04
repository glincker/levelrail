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
	"github.com/GLINCKER/levelrail/internal/upgrade"
)

func releaseWithAssets(names ...string) *githubRelease {
	r := &githubRelease{TagName: "v9.9.9", HTMLURL: "https://example.test/r", Body: "notes"}
	for _, n := range names {
		r.Assets = append(r.Assets, struct {
			Name string `json:"name"`
		}{Name: n})
	}
	return r
}

func TestHandleUpdatePreflight(t *testing.T) {
	setVersion(t, "v1.0.0")
	tests := []struct {
		name        string
		release     *githubRelease
		fetchErr    error
		wantBlocked bool
		wantSigCode string
		wantCommand string
	}{
		{"signed release", releaseWithAssets(upgrade.ChecksumsAsset, upgrade.SignatureAsset), nil, false, upgrade.StatusOK, "LEVELRAIL_VERSION=v9.9.9"},
		{"unsigned release blocks", releaseWithAssets(upgrade.ChecksumsAsset), nil, true, upgrade.StatusFail, "LEVELRAIL_VERSION=v9.9.9"},
		{"github unreachable", nil, errors.New("offline"), false, upgrade.StatusUnknown, "sh -s upgrade"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newTestRouterWithFetchLatestRelease(t, func(context.Context) (*githubRelease, error) {
				return tc.release, tc.fetchErr
			})
			cookie := loginTestSession(t, rt, db)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/updates/preflight", ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			var got updatePreflightResource
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.Blocked != tc.wantBlocked {
				t.Errorf("blocked = %v, want %v (%+v)", got.Blocked, tc.wantBlocked, got.Checks)
			}
			var sig string
			for _, c := range got.Checks {
				if c.Code == "release_signature" {
					sig = c.Status
				}
			}
			if sig != tc.wantSigCode {
				t.Errorf("release_signature = %q, want %q", sig, tc.wantSigCode)
			}
			if !strings.Contains(got.UpgradeCommand, tc.wantCommand) {
				t.Errorf("upgrade command = %q, want containing %q", got.UpgradeCommand, tc.wantCommand)
			}
		})
	}
}

// TestHandleUpdatePreflight_RespectsConfiguredChannel is the regression
// test for the bug found live: an operator on beta/edge saw
// GET /api/v1/updates report an upgrade available while this endpoint
// still reported "latest release could not be fetched", because it
// always checked the stable release list regardless of the configured
// channel.
func TestHandleUpdatePreflight_RespectsConfiguredChannel(t *testing.T) {
	setVersion(t, "v1.0.0")
	rt, db := newTestRouterWithFetchLatestRelease(t, func(context.Context) (*githubRelease, error) {
		return nil, errors.New("stable channel should not be queried when the configured channel is beta")
	})
	if err := db.UpdateUpdateSettings(context.Background(), store.UpdateSettings{Channel: upgrade.ChannelBeta}); err != nil {
		t.Fatalf("UpdateUpdateSettings: %v", err)
	}
	rt.upgradeFetchers = upgrade.Fetchers{
		Beta: func(context.Context) (*upgrade.Release, error) {
			return &upgrade.Release{Tag: "v1.1.0-beta.1", URL: "https://example.test/beta", Body: "beta notes"}, nil
		},
	}

	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/updates/preflight", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got updatePreflightResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.LatestVersion == nil || *got.LatestVersion != "v1.1.0-beta.1" {
		t.Errorf("LatestVersion = %v, want v1.1.0-beta.1", got.LatestVersion)
	}
	if !got.UpdateAvailable {
		t.Error("UpdateAvailable = false, want true for a newer beta release")
	}
}
