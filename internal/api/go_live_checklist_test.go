package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/fixit"
	"github.com/GLINCKER/levelrail/internal/golive"
	"github.com/GLINCKER/levelrail/internal/hostprovider"
	"github.com/GLINCKER/levelrail/internal/store"
)

const goLiveUFWBlocked = "Status: active\nDefault: deny (incoming), allow (outgoing)\n\nTo  Action  From\n--  ------  ----\n22/tcp                     ALLOW IN    Anywhere\n"
const goLiveUFWOpen = goLiveUFWBlocked + "80,443/tcp                 ALLOW IN    Anywhere\n"

type fakeFirewallHost struct {
	status string
	ran    []string
}

func (f *fakeFirewallHost) run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	f.ran = append(f.ran, key)
	if strings.HasPrefix(key, "ufw allow") {
		f.status = goLiveUFWOpen
	}
	if key == "ufw status verbose" {
		return []byte(f.status), nil
	}
	return nil, nil
}

func (f *fakeFirewallHost) look(n string) (string, error) {
	if n == "ufw" {
		return "/usr/sbin/ufw", nil
	}
	return "", errors.New("absent")
}

func goLiveRouter(t *testing.T, fw *fakeFirewallHost) (*Router, *http.Cookie) {
	t.Helper()
	rt, db, cookie := certFlowRouter(t, doctorFakeDNS{ips: []string{certFlowServerIP}})
	rt.hostFirewallRun, rt.hostFirewallLookPath = fw.run, fw.look
	rt.checklist.provDeps = &hostprovider.Deps{
		ReadFile: func(p string) ([]byte, error) {
			if strings.HasSuffix(p, "sys_vendor") {
				return []byte("Hetzner\n"), nil
			}
			return nil, errors.New("absent")
		},
		Get: func(context.Context, string, http.Header) (int, []byte, error) { return 0, nil, errors.New("none") },
	}
	_ = db
	return rt, cookie
}

func TestGoLiveChecklistEndpoint(t *testing.T) {
	fw := &fakeFirewallHost{status: goLiveUFWBlocked}
	rt, cookie := goLiveRouter(t, fw)
	var cl golive.Checklist
	if code := certFlowDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/go-live/checklist?domain=shop.example.dev", nil, &cl); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if len(cl.Steps) != 6 || cl.Steps[0].ID != golive.StepApp || cl.Steps[5].ID != golive.StepLive {
		t.Fatalf("steps = %+v", cl.Steps)
	}
	fwStep := cl.Steps[2]
	if fwStep.Status != golive.StatusFail || fwStep.FirewallFix == nil || len(fwStep.FirewallFix.Commands) != 2 || len(fwStep.FirewallFix.Undo) != 2 {
		t.Fatalf("firewall step = %+v", fwStep)
	}
	if fwStep.Provider == nil || fwStep.Provider.ID != hostprovider.Hetzner {
		t.Fatalf("provider = %+v", fwStep.Provider)
	}
	if cl.State != golive.StateNeedsAttention || cl.Live {
		t.Fatalf("state = %s", cl.State)
	}
	if cl.Steps[4].Status == golive.StatusOK || cl.Steps[5].Status == golive.StatusOK {
		t.Fatalf("certificate and live must not be ok yet: %+v", cl.Steps[4:])
	}
	for _, c := range fw.ran {
		if strings.HasPrefix(c, "ufw allow") {
			t.Fatalf("a read-only checklist changed the firewall: %v", fw.ran)
		}
	}
}

func TestGoLiveChecklistPicksTheOnlyDomain(t *testing.T) {
	rt, cookie := goLiveRouter(t, &fakeFirewallHost{status: goLiveUFWOpen})
	var cl golive.Checklist
	if code := certFlowDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/go-live/checklist", nil, &cl); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if cl.Domain != "shop.example.dev" {
		t.Fatalf("the app has one domain, so it is picked: %q", cl.Domain)
	}
}

func TestGoLiveChecklistUnknownDomainAndApp(t *testing.T) {
	rt, cookie := goLiveRouter(t, &fakeFirewallHost{status: goLiveUFWOpen})
	if code := certFlowDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/go-live/checklist?domain=other.dev", nil, nil); code != http.StatusNotFound {
		t.Fatalf("unknown domain = %d", code)
	}
	if code := certFlowDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/missing/go-live/checklist", nil, nil); code != http.StatusNotFound {
		t.Fatalf("unknown app = %d", code)
	}
}

func TestGoLiveChecklistRequiresAuth(t *testing.T) {
	rt, _ := goLiveRouter(t, &fakeFirewallHost{})
	for _, p := range []string{"/api/v1/apps/web/go-live/checklist", "/api/v1/apps/web/go-live/stream", "/api/v1/go-live/status"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s without a session = %d", p, rec.Code)
		}
	}
}

func TestGoLiveStreamOrdering(t *testing.T) {
	t.Setenv(envGoLiveStreamRounds, "2")
	t.Setenv(envGoLivePollInterval, "10ms")
	rt, cookie := goLiveRouter(t, &fakeFirewallHost{status: goLiveUFWOpen})
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/go-live/stream?domain=shop.example.dev", ""))
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	var types []string
	sc := bufio.NewScanner(rec.Body)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev checklistEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			t.Fatal(err)
		}
		types = append(types, ev.Type)
		if ev.Type == checklistEventSnapshot && (ev.Checklist == nil || len(ev.Checklist.Steps) != 6) {
			t.Fatalf("snapshot without six steps: %+v", ev)
		}
	}
	want := []string{checklistEventSnapshot, checklistEventSnapshot, checklistEventDone}
	if strings.Join(types, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", types, want)
	}
}

func TestGoLiveBadges(t *testing.T) {
	rt, cookie := goLiveRouter(t, &fakeFirewallHost{status: goLiveUFWOpen})
	var out struct {
		Domains []liveStatusEntry `json:"domains"`
	}
	if code := certFlowDo(t, rt, cookie, http.MethodGet, "/api/v1/go-live/status", nil, &out); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	found := false
	for _, d := range out.Domains {
		if d.Domain == "shop.example.dev" {
			found = d.State == golive.StateWaitingDNS && d.Basis == "stored"
		}
	}
	if !found {
		t.Fatalf("badges = %+v", out.Domains)
	}
}

func TestOpenWebPortsFix(t *testing.T) {
	root := func() fixit.HostEnv { return fixit.HostEnv{Mode: fixit.RunSystemd, Root: true} }
	cases := []struct {
		name     string
		env      func() fixit.HostEnv
		status   string
		wantRan  bool
		wantKind fixit.Kind
	}{
		{"root on the host opens ports", root, goLiveUFWBlocked, true, fixit.KindAuto},
		{"container falls back to guided", func() fixit.HostEnv { return fixit.HostEnv{Mode: fixit.RunDocker, Root: true} }, goLiveUFWBlocked, false, fixit.KindGuided},
		{"not root falls back to guided", func() fixit.HostEnv { return fixit.HostEnv{Mode: fixit.RunSystemd} }, goLiveUFWBlocked, false, fixit.KindGuided},
		{"inactive firewall is left alone", root, "Status: inactive\n", false, fixit.KindAuto},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			rt := NewRouter(discardLogger(), testBrand(), db, WithFixit(db))
			fw := &fakeFirewallHost{status: tc.status}
			rt.hostFirewallRun, rt.hostFirewallLookPath = fw.run, fw.look
			rt.fixit.host = tc.env
			cookie := loginTestSession(t, rt, db)
			var prev fixit.View
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/fixes/"+openWebPortsFixID+"/preview", ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("preview %d %s", rec.Code, rec.Body.String())
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &prev)
			if prev.Kind != tc.wantKind {
				t.Fatalf("kind = %s (%s), want %s", prev.Kind, prev.Reason, tc.wantKind)
			}
			rec = httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/fixes/"+openWebPortsFixID+"/apply", ""))
			ranAllow := false
			for _, c := range fw.ran {
				if strings.HasPrefix(c, "ufw allow") {
					ranAllow = true
				}
				if strings.Contains(c, "enable") || strings.Contains(c, "reset") || strings.Contains(c, "delete") {
					t.Fatalf("forbidden command %q", c)
				}
			}
			if ranAllow != tc.wantRan {
				t.Fatalf("ran allow = %v, want %v (%v, response %s)", ranAllow, tc.wantRan, fw.ran, rec.Body.String())
			}
			entries, err := db.ListAuditEntries(context.Background(), 20, nil, store.AuditEntryFilter{ActionPrefixes: []string{auditActionWebPortsOpened}})
			if err != nil {
				t.Fatal(err)
			}
			if (len(entries) == 1) != tc.wantRan {
				t.Fatalf("audit entries = %d, want one only when ports were opened", len(entries))
			}
		})
	}
}

func TestOpenWebPortsFixRefusesWhenRunnerFails(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithFixit(db))
	fw := &fakeFirewallHost{status: goLiveUFWBlocked}
	rt.hostFirewallLookPath = fw.look
	rt.hostFirewallRun = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if strings.HasPrefix(strings.Join(args, " "), "allow 443") {
			return []byte("boom"), errors.New("exit 1")
		}
		return fw.run(ctx, name, args...)
	}
	rt.fixit.host = func() fixit.HostEnv { return fixit.HostEnv{Mode: fixit.RunSystemd, Root: true} }
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/fixes/"+openWebPortsFixID+"/apply", ""))
	var res fixit.Result
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res.Outcome == fixit.OutcomeFixed {
		t.Fatalf("a failed apply was reported as fixed: %s", rec.Body.String())
	}
	if !slices.Contains(fw.ran, "ufw delete allow 80/tcp") {
		t.Fatalf("partial change was not rolled back: %v", fw.ran)
	}
}
