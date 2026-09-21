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

// TestHandleExecAccess_GetDefaultOnAndSetToggles proves the one place
// this endpoint deliberately differs from GET/PUT .../auto-rollback:
// default true, not false, so an app that never touches exec-access
// keeps working exactly as it does today.
func TestHandleExecAccess_GetDefaultOnAndSetToggles(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	recGet := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recGet, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/exec-access", ""))
	if recGet.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recGet.Code, http.StatusOK)
	}
	var got execAccessResource
	if err := json.Unmarshal(recGet.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Enabled {
		t.Error("Enabled = false, want true: on by default")
	}

	recSet := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recSet, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/exec-access", `{"enabled":false}`))
	if recSet.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recSet.Code, http.StatusOK, recSet.Body.String())
	}

	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService: %v", err)
	}
	if svc.ExecEnabled {
		t.Error("ExecEnabled = true after disabling, want false")
	}

	recGetAgain := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recGetAgain, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/exec-access", ""))
	var gotAgain execAccessResource
	if err := json.Unmarshal(recGetAgain.Body.Bytes(), &gotAgain); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if gotAgain.Enabled {
		t.Error("Enabled = true after disabling, want false")
	}

	recMissing := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recMissing, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/ghost/exec-access", `{"enabled":false}`))
	if recMissing.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recMissing.Code, http.StatusNotFound)
	}
}

// TestHandleExecAccess_ReadableWhileDisabled proves reading (and
// re-enabling) the setting keeps working even while it's off: the
// exec-access route itself must never be gated by its own flag, or an
// operator who disabled it could never turn it back on.
func TestHandleExecAccess_ReadableWhileDisabled(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SetServiceExecEnabled(ctx, "web", false); err != nil {
		t.Fatalf("SetServiceExecEnabled(false): %v", err)
	}

	recGet := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recGet, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/exec-access", ""))
	if recGet.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", recGet.Code, http.StatusOK)
	}

	recReenable := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recReenable, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/exec-access", `{"enabled":true}`))
	if recReenable.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d; body = %s", recReenable.Code, http.StatusOK, recReenable.Body.String())
	}
}

// TestHandleSetExecAccess_PlainWriteToken_Forbidden proves PUT
// .../exec-access sits behind AbilityRoot, the same tier exec/terminal
// themselves sit behind, not a lower ordinary-app-write tier: a token
// scoped to nothing but AbilityWrite cannot re-enable exec access any
// more than it can run exec itself.
func TestHandleSetExecAccess_PlainWriteToken_Forbidden(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/exec-access", strings.NewReader(`{"enabled":false}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not be able to toggle exec access", rec.Code, http.StatusForbidden)
	}
}
