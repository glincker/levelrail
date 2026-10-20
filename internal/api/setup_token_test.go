package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// newSetupTestRouter builds a Router with a data dir holding a fresh setup token.
func newSetupTestRouter(t *testing.T) (*Router, *store.DB, string) {
	t.Helper()
	db := openTestDB(t)
	dataDir := t.TempDir()
	token, created, err := EnsureSetupToken(context.Background(), db, dataDir)
	if err != nil || !created || token == "" {
		t.Fatalf("EnsureSetupToken() = (%q, %v, %v), want a new token", token, created, err)
	}
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithDataDir(dataDir)), db, token
}

func TestEnsureSetupToken_Lifecycle(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	dataDir := t.TempDir()

	first, created, err := EnsureSetupToken(ctx, db, dataDir)
	if err != nil || !created || len(first) < 40 {
		t.Fatalf("first EnsureSetupToken() = (%q, %v, %v), want a new 256-bit token", first, created, err)
	}
	info, err := os.Stat(SetupTokenPath(dataDir))
	if err != nil {
		t.Fatalf("stat setup token: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("setup token mode = %o, want 600", perm)
	}

	again, created, err := EnsureSetupToken(ctx, db, dataDir)
	if err != nil || created || again != first {
		t.Errorf("second EnsureSetupToken() = (%q, %v, %v), want the same token reused", again, created, err)
	}

	bootstrapTestAdmin(t, db)
	gone, created, err := EnsureSetupToken(ctx, db, dataDir)
	if err != nil || created || gone != "" {
		t.Errorf("EnsureSetupToken() with a user = (%q, %v, %v), want empty", gone, created, err)
	}
	if _, err := os.Stat(SetupTokenPath(dataDir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("setup token file still present after a user exists: %v", err)
	}
}

func TestEnsureSetupToken_TightensLoosePermissions(t *testing.T) {
	db := openTestDB(t)
	dataDir := t.TempDir()
	if err := os.WriteFile(SetupTokenPath(dataDir), []byte("preexisting\n"), 0o644); err != nil { //nolint:gosec // deliberately loose, the code under test must fix it
		t.Fatal(err)
	}
	token, created, err := EnsureSetupToken(context.Background(), db, dataDir)
	if err != nil || created || token != "preexisting" {
		t.Fatalf("EnsureSetupToken() = (%q, %v, %v)", token, created, err)
	}
	info, err := os.Stat(SetupTokenPath(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600", perm)
	}
}

func TestHandleRegister_SetupToken(t *testing.T) {
	tests := []struct {
		name       string
		token      func(good string) string
		noDataDir  bool
		wantStatus int
	}{
		{name: "missing token", token: func(string) string { return "" }, wantStatus: http.StatusForbidden},
		{name: "wrong token", token: func(good string) string { return good + "x" }, wantStatus: http.StatusForbidden},
		{name: "prefix of token", token: func(good string) string { return good[:10] }, wantStatus: http.StatusForbidden},
		{name: "right token", token: func(good string) string { return good }, wantStatus: http.StatusCreated},
		{name: "no data dir configured", token: func(good string) string { return good }, noDataDir: true, wantStatus: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db, good := newSetupTestRouter(t)
			dataDir := rt.dataDir
			if tt.noDataDir {
				rt.dataDir = ""
			}
			body, _ := json.Marshal(map[string]string{"username": "admin", "password": "a-good-password", "setup_token": tt.token(good)})
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(string(body))))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			n, err := db.CountUsers(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			_, statErr := os.Stat(SetupTokenPath(dataDir))
			if tt.wantStatus == http.StatusCreated {
				if n != 1 {
					t.Errorf("CountUsers() = %d, want 1", n)
				}
				if !errors.Is(statErr, os.ErrNotExist) {
					t.Errorf("setup token file should be deleted after registration, stat err = %v", statErr)
				}
				return
			}
			if n != 0 {
				t.Errorf("CountUsers() = %d, want 0 after a rejected registration", n)
			}
			if statErr != nil {
				t.Errorf("setup token file should survive a rejected registration, stat err = %v", statErr)
			}
		})
	}
}

func TestHandleRegister_TokenCannotBeReused(t *testing.T) {
	rt, _, token := newSetupTestRouter(t)
	for i, want := range []int{http.StatusCreated, http.StatusConflict} {
		body := `{"username":"admin` + string(rune('a'+i)) + `","password":"a-good-password","setup_token":"` + token + `"}`
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body)))
		if rec.Code != want {
			t.Fatalf("attempt %d: status = %d, want %d", i, rec.Code, want)
		}
	}
}

func TestHandleSetupStatus(t *testing.T) {
	rt, db := newTestRouter(t)
	get := func() bool {
		t.Helper()
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/setup-status", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var got setupStatusResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got.NeedsSetup
	}
	if !get() {
		t.Error("needs_setup = false on an empty instance, want true")
	}
	bootstrapTestAdmin(t, db)
	if get() {
		t.Error("needs_setup = true after an admin exists, want false")
	}
}
