package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestOAuthSignIn_ApprovalScope(t *testing.T) {
	tests := []struct {
		name, scope, wantPrefix string
	}{
		{"password only lets a second OAuth browser in", approvalScopePasswordOnly, "/oauth/complete"},
		{"all methods holds a second OAuth browser", approvalScopeAllMethods, "/login?approval=la_"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newOAuthHarness(t, modeLibrary)
			h.rt.newDeviceApproval = true
			h.enable("oidc", "")
			id := fakeIdentity{Sub: "s1", Email: "owner@anywhere.test", Verified: true}
			if loc := h.signIn("oidc", id).Header().Get("Location"); loc != "/oauth/complete" {
				t.Fatalf("first sign-in = %q", loc)
			}
			scope := tt.scope
			if err := h.db.SaveSecurityPolicy(context.Background(), store.SecurityPolicy{ApprovalScope: &scope, UpdatedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
			h.jar = map[string]*http.Cookie{}
			rec := h.signIn("oidc", id)
			if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, tt.wantPrefix) {
				t.Fatalf("second sign-in = %q, want prefix %q", loc, tt.wantPrefix)
			}
			if _, ok := h.sessionUser(rec); ok != (tt.scope == approvalScopePasswordOnly) {
				t.Fatalf("session issued = %v", ok)
			}
		})
	}
}
