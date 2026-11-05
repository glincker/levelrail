package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"github.com/GLINCKER/levelrail/internal/fixit"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	now := time.Now()
	tests := []struct {
		name       string
		status     *dockerguard.Status
		decisions  []bool
		rule       string
		sec        *docker.DaemonSecurity
		listErr    bool
		wantStatus string
		wantSubstr string
		wantNot    string
		wantFix    string
		wantCode   string
		wantClass  string
	}{
		{name: "default enforce could not start", status: &dockerguard.Status{Mode: dockerguard.ModeEnforce, Source: dockerguard.SourceDefault, Effective: dockerguard.ModeOff, StartError: "not a unix socket"}, wantStatus: doctorStatusWarn, wantSubstr: "could not start", wantFix: "APP_DOCKER_GUARD", wantClass: attention.ClassIncident},
		{name: "not configured", wantStatus: doctorStatusUnknown, wantSubstr: "no docker guard"},
		{name: "enforce", status: &dockerguard.Status{Mode: dockerguard.ModeEnforce, Effective: dockerguard.ModeEnforce, Running: true}, decisions: []bool{true}, wantStatus: doctorStatusOK, wantSubstr: "1 decision(s) denied", wantFix: "APP_DOCKER_GUARD=audit"},
		{name: "audit with low risk would deny", status: &dockerguard.Status{Mode: dockerguard.ModeAudit, Effective: dockerguard.ModeAudit, Running: true}, decisions: []bool{false, false}, rule: dockerguard.RuleBindUndeclared, wantStatus: doctorStatusOK, wantSubstr: "2 decision(s) would have been denied"},
		{name: "audit with high risk would deny warns", status: &dockerguard.Status{Mode: dockerguard.ModeAudit, Effective: dockerguard.ModeAudit, Running: true}, decisions: []bool{false}, rule: dockerguard.RulePrivileged, wantStatus: doctorStatusWarn, wantSubstr: "host-level", wantCode: "docker_guard_high_risk", wantClass: attention.ClassIncident},
		{name: "audit clean after a full window", status: &dockerguard.Status{Mode: dockerguard.ModeAudit, Effective: dockerguard.ModeAudit, Running: true, AuditSince: now.Add(-8 * 24 * time.Hour)}, wantStatus: doctorStatusOK, wantSubstr: "jobs that run less often were not seen"},
		{name: "audit clean without a window never says likely safe", status: &dockerguard.Status{Mode: dockerguard.ModeAudit, Effective: dockerguard.ModeAudit, Running: true}, wantStatus: doctorStatusOK, wantSubstr: "still watching", wantNot: "likely safe"},
		{name: "audit log read error is not clean", status: &dockerguard.Status{Mode: dockerguard.ModeAudit, Effective: dockerguard.ModeAudit, Running: true, AuditSince: now.Add(-8 * 24 * time.Hour)}, listErr: true, wantStatus: doctorStatusWarn, wantSubstr: "could not read", wantNot: "jobs that run less often"},
		{name: "off rootful userns remap still warns", status: &dockerguard.Status{Mode: dockerguard.ModeOff, Effective: dockerguard.ModeOff}, sec: &docker.DaemonSecurity{UsernsRemap: true}, wantStatus: doctorStatusWarn, wantSubstr: "privileged containers"},
		{name: "off rootless warns softly and names what is reachable", status: &dockerguard.Status{Mode: dockerguard.ModeOff, Effective: dockerguard.ModeOff}, sec: &docker.DaemonSecurity{Rootless: true}, wantStatus: doctorStatusWarn, wantSubstr: "bind_sensitive_path", wantNot: "every Levelrail process"},
		{name: "off", status: &dockerguard.Status{Mode: dockerguard.ModeOff, Effective: dockerguard.ModeOff}, wantStatus: doctorStatusWarn, wantSubstr: "privileged containers"},
		{name: "restart pending", status: &dockerguard.Status{Mode: dockerguard.ModeEnforce, Effective: dockerguard.ModeOff, RestartRequired: true}, wantStatus: doctorStatusWarn, wantSubstr: "restart"},
		{name: "corrupt settings and a failed start is not failing closed", status: &dockerguard.Status{Mode: dockerguard.ModeEnforce, Effective: dockerguard.ModeOff, ConfigError: "bad json", StartError: "no unix socket"}, wantStatus: doctorStatusWarn, wantSubstr: "reach the daemon directly", wantNot: "failing closed", wantClass: attention.ClassIncident},
		{name: "config error", status: &dockerguard.Status{Mode: dockerguard.ModeEnforce, Effective: dockerguard.ModeEnforce, Running: true, ConfigError: "bad"}, wantStatus: doctorStatusWarn, wantSubstr: "failing closed", wantClass: attention.ClassIncident},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			if tt.status != nil {
				rt.SetDockerGuard(&fakeDockerGuard{status: *tt.status})
			}
			if tt.sec != nil {
				rt.dockerPinger = fakeSecurityPinger{sec: *tt.sec}
			}
			rule := tt.rule
			if rule == "" {
				rule = dockerguard.RulePrivileged
			}
			for _, denied := range tt.decisions {
				saveGuardDecision(t, db, rule, denied, now.Add(-time.Minute))
			}
			if tt.listErr {
				rt.auditLog = failingAuditList{AuditStore: rt.auditLog}
			}
			got := rt.doctorCheckDockerGuard(context.Background())
			if tt.wantNot != "" && strings.Contains(got.Message, tt.wantNot) {
				t.Fatalf("message %q must not contain %q", got.Message, tt.wantNot)
			}
			wantCode := tt.wantCode
			if wantCode == "" {
				wantCode = "docker_guard"
			}
			if tt.wantClass != "" && got.Class != tt.wantClass {
				t.Fatalf("class = %q, want %q", got.Class, tt.wantClass)
			}
			if got.Code != wantCode || got.Status != tt.wantStatus || !strings.Contains(got.Message, tt.wantSubstr) {
				t.Fatalf("check = %+v", got)
			}
			if !strings.Contains(got.Fix, tt.wantFix) {
				t.Fatalf("fix = %q, want it to mention %q", got.Fix, tt.wantFix)
			}
		})
	}
}

type failingAuditList struct{ AuditStore }

func (failingAuditList) ListAuditEntries(context.Context, int, *time.Time, store.AuditEntryFilter) ([]store.AuditEntry, error) {
	return nil, errors.New("disk i/o error")
}

type fakeSecurityPinger struct {
	sec docker.DaemonSecurity
}

func (f fakeSecurityPinger) Ping(context.Context) error { return nil }
func (f fakeSecurityPinger) DaemonSecurity(context.Context) (docker.DaemonSecurity, error) {
	return f.sec, nil
}

func TestDoctorCheckDockerPrivilege(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		sec        docker.DaemonSecurity
		mode       dockerguard.Mode
		wantStatus string
		wantSubstr string
	}{
		{name: "rootless daemon", sec: docker.DaemonSecurity{Rootless: true}, mode: dockerguard.ModeAudit, wantStatus: doctorStatusOK, wantSubstr: "daemon rootless=true"},
		{name: "userns remap is not isolation", sec: docker.DaemonSecurity{UsernsRemap: true}, mode: dockerguard.ModeAudit, wantStatus: doctorStatusWarn, wantSubstr: "userns-remap=true"},
		{name: "userns remap guard off warns", sec: docker.DaemonSecurity{UsernsRemap: true}, mode: dockerguard.ModeOff, wantStatus: doctorStatusWarn, wantSubstr: "root equivalent"},
		{name: "rootless guard off is ok", sec: docker.DaemonSecurity{Rootless: true}, mode: dockerguard.ModeOff, wantStatus: doctorStatusOK, wantSubstr: "rootless=true"},
		{name: "rootful enforced", mode: dockerguard.ModeEnforce, wantStatus: doctorStatusOK, wantSubstr: "rootless=false"},
		{name: "rootful audit warns and says audit does not block", mode: dockerguard.ModeAudit, wantStatus: doctorStatusWarn, wantSubstr: "audit logs but does not block, this process stays root equivalent until enforce"},
		{name: "rootful guard off warns", mode: dockerguard.ModeOff, wantStatus: doctorStatusWarn, wantSubstr: "root equivalent"},
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

func TestDockerGuardAuditLowRiskIsOneRow(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	rt.dockerPinger = fakeSecurityPinger{}
	rt.SetDockerGuard(&fakeDockerGuard{status: dockerguard.Status{Mode: dockerguard.ModeOff, Effective: dockerguard.ModeOff}})
	if g, p := rt.doctorCheckDockerGuard(context.Background()), rt.doctorCheckDockerPrivilege(context.Background()); g.Status != doctorStatusWarn || p.Status != doctorStatusWarn {
		t.Fatalf("guard off on rootful daemon must warn on both: %+v %+v", g, p)
	}
	rt.SetDockerGuard(&fakeDockerGuard{status: dockerguard.Status{Mode: dockerguard.ModeAudit, Effective: dockerguard.ModeAudit, Running: true}})
	saveGuardDecision(t, db, dockerguard.RuleBindUndeclared, false, time.Now().Add(-time.Minute))
	if g := rt.doctorCheckDockerGuard(context.Background()); g.Status != doctorStatusOK {
		t.Fatalf("low risk audit must not warn on docker_guard: %+v", g)
	}
	if p := rt.doctorCheckDockerPrivilege(context.Background()); p.Status != doctorStatusWarn {
		t.Fatalf("rootful audit keeps the single doctor warning: %+v", p)
	}
	items := rt.dockerGuardAttentionItems(httptest.NewRequest(http.MethodGet, "/", nil), []string{AbilityRoot}, time.Now())
	if len(items) != 1 || items[0].Severity != attention.Warning || items[0].Class != attention.ClassRecommendation {
		t.Fatalf("items = %+v, want one recommendation", items)
	}
}

func TestDockerGuardHighRiskIsAnIncidentWithItsOwnID(t *testing.T) {
	t.Parallel()
	now := time.Now()
	feed := func(rule string) attention.Item {
		rt, db := newTestRouter(t)
		rt.SetDockerGuard(&fakeDockerGuard{status: dockerguard.Status{Mode: dockerguard.ModeAudit, Effective: dockerguard.ModeAudit, Running: true}})
		saveGuardDecision(t, db, rule, false, now.Add(-time.Minute))
		items := rt.dockerGuardAttentionItems(httptest.NewRequest(http.MethodGet, "/", nil), []string{AbilityRoot}, now)
		if len(items) != 1 {
			t.Fatalf("rule %s: items = %+v", rule, items)
		}
		return items[0]
	}
	high, low := feed(dockerguard.RulePrivileged), feed(dockerguard.RuleBindUndeclared)
	if high.Class != attention.ClassIncident || low.Class != attention.ClassRecommendation {
		t.Fatalf("classes high=%q low=%q", high.Class, low.Class)
	}
	if high.ID == low.ID || high.Params["last_seen"] == "" {
		t.Fatalf("ids must differ by risk and carry last_seen: %q %q %v", high.ID, low.ID, high.Params)
	}
}

func TestDockerGuardAuditReadErrorShowsNoReadyItem(t *testing.T) {
	t.Parallel()
	now := time.Now()
	rt, _ := newTestRouter(t)
	rt.SetDockerGuard(&fakeDockerGuard{status: dockerguard.Status{Mode: dockerguard.ModeAudit, Effective: dockerguard.ModeAudit, Running: true, AuditSince: now.Add(-8 * 24 * time.Hour)}})
	rt.auditLog = failingAuditList{AuditStore: rt.auditLog}
	if items := rt.dockerGuardAttentionItems(httptest.NewRequest(http.MethodGet, "/", nil), []string{AbilityRoot}, now); len(items) != 0 {
		t.Fatalf("a failed audit read must not offer enforce: %+v", items)
	}
}

func TestDockerGuardAttentionItems(t *testing.T) {
	t.Parallel()
	now := time.Now()
	tests := []struct {
		name      string
		status    dockerguard.Status
		decisions []bool
		abilities []string
		wantSev   string
		wantSubj  string
	}{
		{name: "audit would deny", status: dockerguard.Status{Effective: dockerguard.ModeAudit, Running: true}, decisions: []bool{false}, abilities: []string{AbilityRoot}, wantSev: attention.Warning, wantSubj: "would-deny:low:bind_undeclared"},
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
				saveGuardDecision(t, db, dockerguard.RuleBindUndeclared, denied, now.Add(-time.Minute))
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

func TestDockerGuardAuditEntryFilesUnderTheHighRiskRule(t *testing.T) {
	t.Parallel()
	e, err := DockerGuardAuditEntry(dockerguard.Decision{Method: "POST", Path: "/containers/create", At: time.Now(),
		Violations: []dockerguard.Violation{{Rule: dockerguard.RuleBindUndeclared}, {Rule: dockerguard.RuleVolumeBindDriverOpt}}})
	if err != nil {
		t.Fatal(err)
	}
	if e.Ability != dockerguard.RuleVolumeBindDriverOpt {
		t.Fatalf("ability = %q, want the high-risk rule", e.Ability)
	}
}

func TestDockerGuardHighRiskRulesNeverLookLow(t *testing.T) {
	t.Parallel()
	now := time.Now()
	for _, rule := range []string{dockerguard.RuleVolumeBindDriverOpt, dockerguard.RuleDevices, dockerguard.RuleDeviceCgroupRules, dockerguard.RuleNetworkHost, dockerguard.RuleBodyInvalid} {
		rt, db := newTestRouter(t)
		rt.SetDockerGuard(&fakeDockerGuard{status: dockerguard.Status{Mode: dockerguard.ModeAudit, Effective: dockerguard.ModeAudit, Running: true}})
		saveGuardDecision(t, db, rule, false, now.Add(-time.Minute))
		g := rt.doctorCheckDockerGuard(context.Background())
		items := rt.dockerGuardAttentionItems(httptest.NewRequest(http.MethodGet, "/", nil), []string{AbilityRoot}, now)
		if g.Status != doctorStatusWarn || g.Class != attention.ClassIncident || len(items) != 1 || items[0].Class != attention.ClassIncident {
			t.Fatalf("rule %s: doctor %+v items %+v", rule, g, items)
		}
	}
}

func TestDockerGuardItemIDNamesTheRule(t *testing.T) {
	t.Parallel()
	now := time.Now()
	id := func(rule string) string {
		rt, db := newTestRouter(t)
		rt.SetDockerGuard(&fakeDockerGuard{status: dockerguard.Status{Mode: dockerguard.ModeAudit, Effective: dockerguard.ModeAudit, Running: true}})
		saveGuardDecision(t, db, rule, false, now.Add(-time.Minute))
		return rt.dockerGuardAttentionItems(httptest.NewRequest(http.MethodGet, "/", nil), []string{AbilityRoot}, now)[0].ID
	}
	if a, b := id(dockerguard.RulePrivileged), id(dockerguard.RuleDevices); a == b || !strings.HasSuffix(a, ":privileged") {
		t.Fatalf("ids = %q %q", a, b)
	}
}

func TestDockerGuardResourceCarriesWindowError(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.SetDockerGuard(&fakeDockerGuard{status: dockerguard.Status{Mode: dockerguard.ModeAudit, Effective: dockerguard.ModeAudit, Running: true}})
	rt.auditLog = failingAuditList{AuditStore: rt.auditLog}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/docker-guard", ""))
	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res["window_error"] == nil || res["window_error"] == "" {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestDoctorKeepsAClassTheCheckSet(t *testing.T) {
	t.Parallel()
	def := doctorCheckDef{ID: "x", Run: func(context.Context) []doctorCheckResource {
		return []doctorCheckResource{{Code: "docker_guard", Status: doctorStatusWarn, Class: attention.ClassIncident}, {Code: "docker_guard", Status: doctorStatusWarn}}
	}}
	got := runDoctorDef(context.Background(), def, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
	if got[0].Class != attention.ClassIncident || got[1].Class != attention.ClassRecommendation {
		t.Fatalf("classes = %q %q", got[0].Class, got[1].Class)
	}
}

func TestDockerGuardOneItemPerHighRiskRule(t *testing.T) {
	t.Parallel()
	now := time.Now()
	rt, db := newTestRouter(t)
	rt.runtimeSettings = runtimesettings.New(db, func(string) (string, bool) { return "", false })
	rt.SetDockerGuard(&fakeDockerGuard{status: dockerguard.Status{Mode: dockerguard.ModeAudit, Effective: dockerguard.ModeAudit, Running: true}})
	for i := 0; i < 3; i++ {
		saveGuardDecision(t, db, dockerguard.RuleNetworkHost, false, now.Add(-time.Duration(i+1)*time.Minute))
	}
	saveGuardDecision(t, db, dockerguard.RulePrivileged, false, now.Add(-time.Minute))
	saveGuardDecision(t, db, dockerguard.RuleDevices, false, now.Add(-time.Minute))
	saveGuardDecision(t, db, dockerguard.RuleBindUndeclared, false, now.Add(-time.Minute))
	feed := func() []attention.Item {
		return rt.dockerGuardAttentionItems(httptest.NewRequest(http.MethodGet, "/", nil), []string{AbilityRoot}, now)
	}
	items := feed()
	if len(items) != 3 || !strings.HasSuffix(items[0].ID, ":network_host") || items[0].ID == items[1].ID {
		t.Fatalf("want one item per high-risk rule, most frequent first: %+v", items)
	}
	for _, it := range items {
		if it.Class != attention.ClassIncident || strings.Contains(it.ID, "bind_undeclared") {
			t.Fatalf("item = %+v", it)
		}
	}
	if _, err := rt.runtimeSettings.Set(context.Background(), "security.docker_guard_max_items", json.RawMessage("2"), "test"); err != nil {
		t.Fatal(err)
	}
	items = feed()
	if len(items) != 2 || items[1].Params["more"] != "1" {
		t.Fatalf("capped items = %+v, want 2 with the rest counted in the last one", items)
	}
}

func TestDoctorFailedCheckIsAlwaysAnIncident(t *testing.T) {
	t.Parallel()
	def := doctorCheckDef{ID: "x", Run: func(context.Context) []doctorCheckResource {
		return []doctorCheckResource{{Code: "docker_guard", Status: doctorStatusFail, Class: attention.ClassRecommendation}}
	}}
	if got := runDoctorDef(context.Background(), def, slog.New(slog.NewTextHandler(discardWriter{}, nil))); got[0].Class != attention.ClassIncident {
		t.Fatalf("class = %q", got[0].Class)
	}
}

func TestDockerGuardUnsilencedHighRiskRuleIsNeverCutOrHidden(t *testing.T) {
	t.Parallel()
	now := time.Now()
	rt, db := newTestRouter(t)
	rt.runtimeSettings = runtimesettings.New(db, func(string) (string, bool) { return "", false })
	rt.SetDockerGuard(&fakeDockerGuard{status: dockerguard.Status{Mode: dockerguard.ModeAudit, Effective: dockerguard.ModeAudit, Running: true}})
	rules := []string{dockerguard.RuleNetworkHost, dockerguard.RuleDevices, dockerguard.RuleDeviceCgroupRules, dockerguard.RuleSecurityOpt, dockerguard.RuleMaskedPaths, dockerguard.RulePrivileged}
	for i, rule := range rules {
		for n := 0; n < len(rules)-i; n++ {
			saveGuardDecision(t, db, rule, false, now.Add(-time.Duration(n+1)*time.Minute))
		}
	}
	for i, rule := range rules[:5] {
		sil := store.Silence{ID: "sil_noisy" + string(rune('a'+i)), IssueKey: "docker_guard:would-deny:high:" + rule, CreatedBy: "root", CreatedAt: now}
		if err := db.CreateSilence(context.Background(), sil); err != nil {
			t.Fatal(err)
		}
	}
	var guard []attention.Item
	for _, it := range rt.attentionFeedItems(httptest.NewRequest(http.MethodGet, "/", nil), []string{AbilityRoot}, now) {
		if it.Kind == attention.KindDockerGuard {
			guard = append(guard, it)
		}
	}
	if len(guard) != 1 || !strings.HasSuffix(guard[0].ID, ":privileged") || guard[0].Class != attention.ClassIncident {
		t.Fatalf("the one unsilenced high-risk rule must reach the feed: %+v", guard)
	}
}

func TestEveryDockerGuardDoctorCodeHasAnExplanation(t *testing.T) {
	t.Parallel()
	for _, code := range []string{"docker_guard", dockerGuardHighRiskCode, "docker_privilege"} {
		if _, ok := fixit.Explain(code); !ok {
			t.Errorf("doctor code %q has no explain entry", code)
		}
	}
}
