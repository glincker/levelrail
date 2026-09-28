package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeSecretBinder struct {
	status    secrets.BindingStatus
	result    secrets.RebindResult
	rebindErr error
	rebinds   int
}

func (f *fakeSecretBinder) BindingStatus(context.Context) (secrets.BindingStatus, error) {
	return f.status, nil
}

func (f *fakeSecretBinder) Rebind(context.Context) (secrets.RebindResult, error) {
	f.rebinds++
	return f.result, f.rebindErr
}

func TestSecretBindingRoutes_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/system/secrets/binding"},
		{http.MethodPost, "/api/v1/system/secrets/rebind"},
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, tc.method, tc.path, ""))
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("%s %s status = %d, want 501", tc.method, tc.path, rec.Code)
		}
	}
}

func TestHandleGetSecretBinding(t *testing.T) {
	fake := &fakeSecretBinder{status: secrets.BindingStatus{Total: 5, Bound: 3, Legacy: 2}}
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithSecretBinding(fake))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/secrets/binding", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got secretBindingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got != (secretBindingResponse{Total: 5, Bound: 3, Legacy: 2}) {
		t.Errorf("response = %+v", got)
	}
}

func TestHandleRebindSecrets(t *testing.T) {
	tests := []struct {
		name     string
		fake     *fakeSecretBinder
		wantCode int
		wantBody string
	}{
		{
			name: "success",
			fake: &fakeSecretBinder{result: secrets.RebindResult{Scanned: 4, Rebound: 3, AlreadyBound: 1,
				FailedCount: 1, Failed: []secrets.RebindFailure{{Owner: "web", Key: "K", Reason: "ciphertext is bound to a different slot"}}}},
			wantCode: http.StatusOK,
			wantBody: `"rebound":3`,
		},
		{
			name:     "store failure mid-run",
			fake:     &fakeSecretBinder{result: secrets.RebindResult{Rebound: 2}, rebindErr: errors.New("disk full")},
			wantCode: http.StatusInternalServerError,
			wantBody: "stopped after 2 values",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			rt := NewRouter(discardLogger(), testBrand(), db, WithSecretBinding(tt.fake))
			cookie := loginTestSession(t, rt, db)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/secrets/rebind", ""))
			if rec.Code != tt.wantCode || !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("status = %d body = %s, want %d containing %q", rec.Code, rec.Body.String(), tt.wantCode, tt.wantBody)
			}
		})
	}
}

func TestHandleRebindSecrets_PlainWriteToken_Forbidden(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithSecretBinding(&fakeSecretBinder{}))
	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/secrets/rebind", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestHandleRotateMasterKey_RebindsAfterRotation(t *testing.T) {
	tests := []struct {
		name        string
		binder      *fakeSecretBinder
		wantRebind  bool
		wantWarning string
	}{
		{"rebind succeeds", &fakeSecretBinder{result: secrets.RebindResult{Rebound: 7}}, true, "APP_MASTER_KEY"},
		{"rebind fails, rotation still ok", &fakeSecretBinder{rebindErr: errors.New("boom")}, false, "secrets rebind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			rt := NewRouter(discardLogger(), testBrand(), db,
				WithMasterKeyRotation(&fakeMasterKeyRotator{rotatedAt: time.Now().UTC()}, ""), WithSecretBinding(tt.binder))
			cookie := loginTestSession(t, rt, db)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/master-key/rotate", `{"newMasterKey":"k"}`))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			var got rotateMasterKeyResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if tt.binder.rebinds != 1 {
				t.Errorf("Rebind called %d times, want 1", tt.binder.rebinds)
			}
			if (got.Rebind != nil) != tt.wantRebind || (got.Rebind != nil && got.Rebind.Rebound != 7) {
				t.Errorf("Rebind = %+v, want present=%t", got.Rebind, tt.wantRebind)
			}
			if !strings.Contains(got.Warning, tt.wantWarning) {
				t.Errorf("Warning = %q, want it to contain %q", got.Warning, tt.wantWarning)
			}
		})
	}
}

func TestDoctorCheckSecretBinding(t *testing.T) {
	tests := []struct {
		name   string
		binder SecretBinder
		want   string
	}{
		{"not configured", nil, doctorStatusUnknown},
		{"all bound", &fakeSecretBinder{status: secrets.BindingStatus{Total: 3, Bound: 3}}, doctorStatusOK},
		{"legacy remain", &fakeSecretBinder{status: secrets.BindingStatus{Total: 3, Bound: 1, Legacy: 2}}, doctorStatusWarn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &Router{secretBinder: tt.binder}
			got := rt.doctorCheckSecretBinding(context.Background())
			if got.Status != tt.want {
				t.Errorf("status = %q, want %q (%s)", got.Status, tt.want, got.Message)
			}
			if got.Status == doctorStatusWarn && got.Fix == "" {
				t.Error("warn result has no Fix command")
			}
		})
	}
}
