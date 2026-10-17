package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GLINCKER/levelrail/kit/upgrade"
)

type fakeTrustRoot struct {
	state string
	warms atomic.Int32
}

func (f *fakeTrustRoot) State() string { return f.state }
func (f *fakeTrustRoot) Warm()         { f.warms.Add(1) }

func TestUpdatePreflightVerifierReflectsTrustRoot(t *testing.T) {
	setVersion(t, "v1.0.0")
	tests := []struct {
		name       string
		state      string
		wantStatus string
		wantFix    string
	}{
		{"ready", upgrade.TrustRootReady, upgrade.StatusOK, ""},
		{"cached", upgrade.TrustRootCached, upgrade.StatusOK, ""},
		{"checking", upgrade.TrustRootChecking, upgrade.StatusWarn, upgrade.FixRecheck},
		{"unavailable without cache", upgrade.TrustRootUnavailable, upgrade.StatusWarn, upgrade.FixRecheck},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newTestRouterWithFetchLatestRelease(t, func(context.Context) (*githubRelease, error) {
				return releaseWithAssets(upgrade.ChecksumsAsset, upgrade.SignatureAsset), nil
			})
			probe := &fakeTrustRoot{state: tc.state}
			WithTrustRoot(probe)(rt)
			cookie := loginTestSession(t, rt, db)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/updates/preflight", ""))
			if strings.Contains(rec.Body.String(), "cosign") {
				t.Fatalf("response must not mention cosign: %s", rec.Body.String())
			}
			var got updatePreflightResource
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			var c *upgrade.Check
			for i := range got.Checks {
				if got.Checks[i].Code == "release_verifier" {
					c = &got.Checks[i]
				}
			}
			if c == nil || c.Status != tc.wantStatus {
				t.Fatalf("verifier check = %+v, want %s", c, tc.wantStatus)
			}
			if (tc.wantFix == "") != (c.Fix == nil) || (c.Fix != nil && c.Fix.Action != tc.wantFix) {
				t.Fatalf("fix = %+v, want %q", c.Fix, tc.wantFix)
			}
			if got.Blocked {
				t.Fatalf("trust root state must never block an unpinned host: %+v", got.Checks)
			}
			if probe.warms.Load() == 0 {
				t.Fatal("a newer release must warm the trust root")
			}
		})
	}
}
