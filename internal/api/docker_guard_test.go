package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeDockerGuard struct {
	status dockerguard.Status
	setErr error
	gotSet dockerguard.Mode
}

func (f *fakeDockerGuard) Status() dockerguard.Status { return f.status }

func (f *fakeDockerGuard) SetMode(m dockerguard.Mode, _ string, _ time.Time) (dockerguard.Status, error) {
	if f.setErr != nil {
		return dockerguard.Status{}, f.setErr
	}
	f.gotSet = m
	f.status.Mode, f.status.Effective, f.status.Source = m, m, dockerguard.SourceSettings
	return f.status, nil
}

func saveGuardDecision(t *testing.T, db *store.DB, rule string, denied bool, at time.Time) {
	t.Helper()
	e, err := DockerGuardAuditEntry(dockerguard.Decision{Denied: denied, Method: http.MethodPost, Path: "/containers/create", Container: "web",
		Violations: []dockerguard.Violation{{Rule: rule, Reason: "x"}}, At: at})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAuditEntry(context.Background(), e); err != nil {
		t.Fatal(err)
	}
}

func TestDockerGuardEndpoints(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	guard := &fakeDockerGuard{status: dockerguard.Status{Mode: dockerguard.ModeAudit, Effective: dockerguard.ModeAudit, Running: true, Source: dockerguard.SourceDefault}}

	do := func(method, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, "/api/v1/system/docker-guard", body))
		return rec
	}
	if rec := do(http.MethodPut, `{"mode":"enforce"}`); rec.Code != http.StatusNotImplemented {
		t.Fatalf("PUT without guard = %d", rec.Code)
	}
	rec := do(http.MethodGet, "")
	var res dockerGuardResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res.Configured {
		t.Fatalf("GET without guard = %s (%v)", rec.Body.String(), err)
	}

	rt.SetDockerGuard(guard)
	now := time.Now()
	saveGuardDecision(t, db, dockerguard.RulePrivileged, false, now.Add(-time.Hour))
	saveGuardDecision(t, db, dockerguard.RulePrivileged, false, now.Add(-2*time.Hour))
	saveGuardDecision(t, db, dockerguard.RuleBindSensitive, true, now.Add(-time.Minute))
	saveGuardDecision(t, db, dockerguard.RuleCapAdd, false, now.Add(-30*24*time.Hour))

	rec = do(http.MethodGet, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.Configured || res.WouldDeny != 2 || res.Denied != 1 || len(res.Window) != 2 || res.Window[0].Rule != dockerguard.RulePrivileged {
		t.Fatalf("summary = %+v", res)
	}

	tests := []struct {
		name   string
		body   string
		setErr error
		want   int
	}{
		{name: "invalid mode", body: `{"mode":"loose"}`, want: http.StatusBadRequest},
		{name: "empty mode", body: `{}`, want: http.StatusBadRequest},
		{name: "pinned by env", body: `{"mode":"enforce"}`, setErr: dockerguard.ErrPinnedByEnv, want: http.StatusConflict},
		{name: "enforce", body: `{"mode":"enforce"}`, want: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guard.setErr = tt.setErr
			if rec := do(http.MethodPut, tt.body); rec.Code != tt.want {
				t.Fatalf("PUT %s = %d: %s", tt.body, rec.Code, rec.Body.String())
			}
		})
	}
	if guard.gotSet != dockerguard.ModeEnforce {
		t.Fatalf("controller got %q", guard.gotSet)
	}
}

func TestDockerGuardAuditEntryCarriesNoBody(t *testing.T) {
	e, err := DockerGuardAuditEntry(dockerguard.Decision{Denied: true, Method: "POST", Path: "/containers/create", Container: "web",
		Violations: []dockerguard.Violation{{Rule: dockerguard.RulePrivileged, Reason: "HostConfig.Privileged is never allowed"}}, At: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if e.Action != dockerguard.ActionDenied || e.Ability != dockerguard.RulePrivileged || e.StatusCode != http.StatusForbidden || e.Path != "docker:/containers/create?name=web" {
		t.Fatalf("entry = %+v", e)
	}
}

func TestDoctorCheckDockerGuard(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name       string
		status     *dockerguard.Status
		decisions  []bool
		wantStatus string
		wantSubstr string
	}{
		{name: "not configured", wantStatus: doctorStatusUnknown, wantSubstr: "no docker guard"},
		{name: "enforce", status: &dockerguard.Status{Mode: dockerguard.ModeEnforce, Effective: dockerguard.ModeEnforce, Running: true}, decisions: []bool{true}, wantStatus: doctorStatusOK, wantSubstr: "1 request(s) denied"},
		{name: "audit with would deny", status: &dockerguard.Status{Mode: dockerguard.ModeAudit, Effective: dockerguard.ModeAudit, Running: true}, decisions: []bool{false, false}, wantStatus: doctorStatusWarn, wantSubstr: "2 request(s) would have been denied"},
		{name: "audit clean", status: &dockerguard.Status{Mode: dockerguard.ModeAudit, Effective: dockerguard.ModeAudit, Running: true}, wantStatus: doctorStatusWarn, wantSubstr: "enforce is likely safe"},
		{name: "off", status: &dockerguard.Status{Mode: dockerguard.ModeOff, Effective: dockerguard.ModeOff}, wantStatus: doctorStatusWarn, wantSubstr: "privileged containers"},
		{name: "restart pending", status: &dockerguard.Status{Mode: dockerguard.ModeEnforce, Effective: dockerguard.ModeOff, RestartRequired: true}, wantStatus: doctorStatusWarn, wantSubstr: "restart"},
		{name: "config error", status: &dockerguard.Status{Mode: dockerguard.ModeEnforce, Effective: dockerguard.ModeEnforce, Running: true, ConfigError: "bad"}, wantStatus: doctorStatusWarn, wantSubstr: "failing closed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			if tt.status != nil {
				rt.SetDockerGuard(&fakeDockerGuard{status: *tt.status})
			}
			for _, denied := range tt.decisions {
				saveGuardDecision(t, db, dockerguard.RulePrivileged, denied, now.Add(-time.Minute))
			}
			got := rt.doctorCheckDockerGuard(context.Background())
			if got.Code != "docker_guard" || got.Status != tt.wantStatus || !strings.Contains(got.Message, tt.wantSubstr) {
				t.Fatalf("check = %+v", got)
			}
		})
	}
}

type fakeSecurityPinger struct {
	sec docker.DaemonSecurity
}

func (f fakeSecurityPinger) Ping(context.Context) error { return nil }
func (f fakeSecurityPinger) DaemonSecurity(context.Context) (docker.DaemonSecurity, error) {
	return f.sec, nil
}

func TestDoctorCheckDockerPrivilege(t *testing.T) {
	tests := []struct {
		name       string
		sec        docker.DaemonSecurity
		mode       dockerguard.Mode
		wantStatus string
		wantSubstr string
	}{
		{name: "rootless daemon", sec: docker.DaemonSecurity{Rootless: true}, mode: dockerguard.ModeAudit, wantStatus: doctorStatusOK, wantSubstr: "daemon rootless=true"},
		{name: "userns remap", sec: docker.DaemonSecurity{UsernsRemap: true}, mode: dockerguard.ModeAudit, wantStatus: doctorStatusOK, wantSubstr: "userns-remap=true"},
		{name: "rootful enforced", mode: dockerguard.ModeEnforce, wantStatus: doctorStatusOK, wantSubstr: "rootless=false"},
		{name: "rootful audit warns", mode: dockerguard.ModeAudit, wantStatus: doctorStatusWarn, wantSubstr: "root equivalent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &Router{dockerPinger: fakeSecurityPinger{sec: tt.sec}, dockerGuard: &fakeDockerGuard{status: dockerguard.Status{Effective: tt.mode}}}
			got := rt.doctorCheckDockerPrivilege(context.Background())
			if got.Status != tt.wantStatus || !strings.Contains(got.Message, tt.wantSubstr) || !strings.Contains(got.Message, "service user") {
				t.Fatalf("check = %+v", got)
			}
		})
	}
}

func TestDockerGuardAttentionItems(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		status    dockerguard.Status
		decisions []bool
		abilities []string
		wantSev   string
		wantSubj  string
	}{
		{name: "audit would deny", status: dockerguard.Status{Effective: dockerguard.ModeAudit, Running: true}, decisions: []bool{false}, abilities: []string{AbilityRoot}, wantSev: attention.Warning, wantSubj: "would-deny"},
		{name: "clean week offers enforce", status: dockerguard.Status{Effective: dockerguard.ModeAudit, Running: true, AuditSince: now.Add(-8 * 24 * time.Hour)}, abilities: []string{AbilityRoot}, wantSev: attention.Info, wantSubj: "ready"},
		{name: "clean but too new", status: dockerguard.Status{Effective: dockerguard.ModeAudit, Running: true, AuditSince: now.Add(-time.Hour)}, abilities: []string{AbilityRoot}},
		{name: "enforce denied", status: dockerguard.Status{Effective: dockerguard.ModeEnforce, Running: true}, decisions: []bool{true}, abilities: []string{AbilityRoot}, wantSev: attention.Warning, wantSubj: "denied"},
		{name: "non admin sees nothing", status: dockerguard.Status{Effective: dockerguard.ModeAudit, Running: true}, decisions: []bool{false}, abilities: []string{AbilityRead}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			rt.SetDockerGuard(&fakeDockerGuard{status: tt.status})
			for _, denied := range tt.decisions {
				saveGuardDecision(t, db, dockerguard.RuleNetworkHost, denied, now.Add(-time.Minute))
			}
			items := rt.dockerGuardAttentionItems(httptest.NewRequest(http.MethodGet, "/", nil), tt.abilities, now)
			if tt.wantSev == "" {
				if len(items) != 0 {
					t.Fatalf("want no items, got %+v", items)
				}
				return
			}
			if len(items) != 1 || items[0].Severity != tt.wantSev || items[0].Subject != tt.wantSubj || items[0].Link != "/settings/security" {
				t.Fatalf("items = %+v", items)
			}
		})
	}
}
