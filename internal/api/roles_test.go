package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestRoleAbilities(t *testing.T) {
	tests := []struct {
		name   string
		role   string
		want   []string
		wantOK bool
	}{
		{name: "admin", role: RoleAdmin, want: []string{AbilityRoot}, wantOK: true},
		{name: "operator", role: RoleOperator, want: []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityDeploy}, wantOK: true},
		{name: "viewer", role: RoleViewer, want: []string{AbilityRead}, wantOK: true},
		{name: "unknown", role: "superadmin", want: nil, wantOK: false},
		{name: "empty", role: "", want: nil, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := roleAbilities(tt.role)
			if ok != tt.wantOK {
				t.Fatalf("roleAbilities(%q) ok = %v, want %v", tt.role, ok, tt.wantOK)
			}
			if ok && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("roleAbilities(%q) = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}

func TestRoleForAbilities(t *testing.T) {
	tests := []struct {
		name      string
		abilities []string
		want      string
		wantOK    bool
	}{
		{name: "matches admin", abilities: []string{AbilityRoot}, want: RoleAdmin, wantOK: true},
		{name: "matches operator", abilities: []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityDeploy}, want: RoleOperator, wantOK: true},
		{name: "matches operator, different order", abilities: []string{AbilityDeploy, AbilityWrite, AbilityReadSensitive, AbilityRead}, want: RoleOperator, wantOK: true},
		{name: "matches viewer", abilities: []string{AbilityRead}, want: RoleViewer, wantOK: true},
		{name: "custom subset of operator", abilities: []string{AbilityRead, AbilityWrite}, want: "", wantOK: false},
		{name: "custom superset of viewer", abilities: []string{AbilityRead, AbilityDeploy}, want: "", wantOK: false},
		{name: "empty", abilities: nil, want: "", wantOK: false},
		{name: "write:sensitive alone matches nothing", abilities: []string{AbilityWriteSensitive}, want: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := roleForAbilities(tt.abilities)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("roleForAbilities(%v) = (%q, %v), want (%q, %v)", tt.abilities, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestResolveAbilities(t *testing.T) {
	tests := []struct {
		name      string
		role      string
		abilities []string
		want      []string
		wantErr   bool
	}{
		{name: "role wins over abilities", role: RoleViewer, abilities: []string{AbilityRoot}, want: []string{AbilityRead}},
		{name: "role admin", role: RoleAdmin, want: []string{AbilityRoot}},
		{name: "unknown role rejected", role: "superadmin", wantErr: true},
		{name: "no role falls back to abilities", role: "", abilities: []string{AbilityRead, AbilityDeploy}, want: []string{AbilityRead, AbilityDeploy}},
		{name: "no role, empty abilities rejected", role: "", abilities: nil, wantErr: true},
		{name: "no role, invalid ability rejected", role: "", abilities: []string{"not-real"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAbilities(tt.role, tt.abilities)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveAbilities(%q, %v) error = %v, wantErr %v", tt.role, tt.abilities, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("resolveAbilities(%q, %v) = %v, want %v", tt.role, tt.abilities, got, tt.want)
			}
		})
	}
}

func TestHandleListRoles(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/roles", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []Role
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(roles) = %d, want 3", len(got))
	}
	names := map[string]bool{}
	for _, r := range got {
		names[r.Name] = true
		if len(r.Abilities) == 0 {
			t.Errorf("role %q has no abilities", r.Name)
		}
		if r.Description == "" {
			t.Errorf("role %q has no description", r.Name)
		}
	}
	for _, want := range []string{RoleAdmin, RoleOperator, RoleViewer} {
		if !names[want] {
			t.Errorf("roles response missing %q", want)
		}
	}
}

func TestHandleListRoles_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
