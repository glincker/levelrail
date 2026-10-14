package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleUpdateProfile(t *testing.T) {
	h := newCodeHarness(t, false)
	other := store.User{ID: "user_other", Email: "other@example.com", DisplayName: "Other", Abilities: []string{AbilityRead}, CreatedAt: time.Now()}
	if err := h.db.CreateUser(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	own := h.userSession()

	tests := []struct {
		name     string
		body     string
		cookie   *http.Cookie
		wantCode int
		wantName string
	}{
		{name: "renames self", body: `{"display_name":"  New Name "}`, cookie: own, wantCode: http.StatusOK, wantName: "New Name"},
		{name: "empty rejected", body: `{"display_name":"  "}`, cookie: own, wantCode: http.StatusBadRequest},
		{name: "too long rejected", body: `{"display_name":"` + repeatRune('a', maxDisplayNameLen+1) + `"}`, cookie: own, wantCode: http.StatusBadRequest},
		{name: "anonymous rejected", body: `{"display_name":"X"}`, cookie: nil, wantCode: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := h.send(http.MethodPatch, "/api/v1/auth/profile", tt.body, "", tt.cookie)
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantName == "" {
				return
			}
			u, err := h.db.GetUserByID(context.Background(), h.user.ID)
			if err != nil || u.DisplayName != tt.wantName {
				t.Fatalf("display name = %q (%v), want %q", u.DisplayName, err, tt.wantName)
			}
		})
	}

	got, err := h.db.GetUserByID(context.Background(), other.ID)
	if err != nil || got.DisplayName != "Other" {
		t.Fatalf("another user's display name changed: %q (%v)", got.DisplayName, err)
	}
}

func repeatRune(r rune, n int) string {
	out := make([]rune, n)
	for i := range out {
		out[i] = r
	}
	return string(out)
}

func TestHandleMyAccess(t *testing.T) {
	h := newCodeHarness(t, false)
	viewer := store.User{ID: "user_viewer", Email: "viewer@example.com", DisplayName: "V", Abilities: []string{AbilityRead}, CreatedAt: time.Now()}
	if err := h.db.CreateUser(context.Background(), viewer); err != nil {
		t.Fatal(err)
	}
	admin := adminSessionForTest(t, h)
	viewerCookie := sessionCookieForTest(t, h.rt, viewer.ID)

	tests := []struct {
		name        string
		cookie      *http.Cookie
		wantCode    int
		wantTier    string
		wantName    string
		wantAllowed map[string]bool
	}{
		{name: "viewer", cookie: viewerCookie, wantCode: http.StatusOK, wantTier: "viewer", wantName: viewer.Email,
			wantAllowed: map[string]bool{"view": true, "deploy": false, "users_and_access": false}},
		{name: "admin", cookie: admin, wantCode: http.StatusOK, wantTier: "admin", wantName: testAdminUsername,
			wantAllowed: map[string]bool{"view": true, "deploy": true, "users_and_access": true}},
		{name: "anonymous", cookie: nil, wantCode: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := h.send(http.MethodGet, "/api/v1/auth/my-access", "", "", tt.cookie)
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantCode != http.StatusOK {
				return
			}
			var got myAccessResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Tier != tt.wantTier || got.Name != tt.wantName {
				t.Fatalf("tier=%q name=%q, want %q %q", got.Tier, got.Name, tt.wantTier, tt.wantName)
			}
			for _, c := range got.Capabilities {
				want, ok := tt.wantAllowed[c.Key]
				if !ok {
					continue
				}
				if c.Allowed != want {
					t.Errorf("capability %s allowed=%v, want %v", c.Key, c.Allowed, want)
				}
				if !c.Allowed && c.Reason == "" {
					t.Errorf("capability %s denied without a reason", c.Key)
				}
			}
		})
	}
}
