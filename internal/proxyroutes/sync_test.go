package proxyroutes

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeSyncStore struct {
	settings store.ProxyIntegrationSettings
	ingress  store.IngressSettings
	services []store.DesiredService
	sites    []store.StaticSite
	status   map[string]store.ProxyRouteStatus
	audits   []string
}

func newFakeSyncStore(dir string) *fakeSyncStore {
	return &fakeSyncStore{
		settings: store.ProxyIntegrationSettings{Mode: store.ProxyIntegrationTraefikFile, DynamicDir: dir, EntrypointHTTP: "http", EntrypointHTTPS: "https", CertResolver: "letsencrypt", UpstreamHost: HostDockerInternal},
		status:   map[string]store.ProxyRouteStatus{},
	}
}

func (f *fakeSyncStore) GetProxyIntegrationSettings(context.Context) (store.ProxyIntegrationSettings, error) {
	return f.settings, nil
}
func (f *fakeSyncStore) GetIngressSettings(context.Context) (store.IngressSettings, error) {
	return f.ingress, nil
}
func (f *fakeSyncStore) ListDesiredServices(context.Context) ([]store.DesiredService, error) {
	return f.services, nil
}
func (f *fakeSyncStore) ListStaticSites(context.Context) ([]store.StaticSite, error) {
	return f.sites, nil
}
func (f *fakeSyncStore) ListProxyRouteStatus(context.Context) ([]store.ProxyRouteStatus, error) {
	var out []store.ProxyRouteStatus
	for _, s := range f.status {
		out = append(out, s)
	}
	return out, nil
}
func (f *fakeSyncStore) MarkProxyRouteWritten(_ context.Context, domain, target, name string, _ time.Time, _ bool) error {
	f.status[domain] = store.ProxyRouteStatus{Domain: domain, Target: target, FileName: name, Written: true}
	return nil
}
func (f *fakeSyncStore) MarkProxyRouteFailed(_ context.Context, domain, target, msg string) error {
	f.status[domain] = store.ProxyRouteStatus{Domain: domain, Target: target, LastError: msg}
	return nil
}
func (f *fakeSyncStore) DeleteProxyRouteStatus(_ context.Context, domain string) error {
	delete(f.status, domain)
	return nil
}
func (f *fakeSyncStore) SaveAuditEntry(_ context.Context, e store.AuditEntry) error {
	f.audits = append(f.audits, e.Action+" "+strings.TrimPrefix(e.Path, auditPathPrefix))
	return nil
}

func newTestSyncer(st SyncStore) *Syncer {
	return &Syncer{
		Store: st, NS: testNS, Unit: "acme.service",
		Ingress:   Listener{Label: "ingress", Addr: ":8088", EnvVar: "APP_INGRESS_HTTP_ADDR"},
		Dashboard: Listener{Label: "dashboard", Addr: "127.0.0.1:8080", EnvVar: "APP_HTTP_ADDR"},
	}
}

func managedFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

func TestSync_Lifecycle(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "coolify.yaml"), []byte("http: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	st := newFakeSyncStore(dir)
	s := newTestSyncer(st)
	ctx := context.Background()

	steps := []struct {
		name        string
		mutate      func()
		wantFiles   []string
		wantWritten []string
		wantRemoved []string
		wantFailed  []string
	}{
		{
			name: "app domains written, canary skipped",
			mutate: func() {
				st.services = []store.DesiredService{{Name: "web", Domains: []string{"b.example.com", "a.example.com"}}, {Name: "web" + store.CanaryServiceSuffix, Domains: []string{"c.example.com"}}}
			},
			wantFiles:   []string{"acme-managed-a.example.com.yaml", "acme-managed-b.example.com.yaml", "coolify.yaml"},
			wantWritten: []string{"a.example.com", "b.example.com"},
		},
		{name: "repeat is a no-op", mutate: func() {}, wantFiles: []string{"acme-managed-a.example.com.yaml", "acme-managed-b.example.com.yaml", "coolify.yaml"}},
		{
			name:        "upstream change rewrites",
			mutate:      func() { st.settings.UpstreamHost = "10.0.1.1" },
			wantFiles:   []string{"acme-managed-a.example.com.yaml", "acme-managed-b.example.com.yaml", "coolify.yaml"},
			wantWritten: []string{"a.example.com", "b.example.com"},
		},
		{
			name:        "domain removed",
			mutate:      func() { st.services[0].Domains = []string{"a.example.com"} },
			wantFiles:   []string{"acme-managed-a.example.com.yaml", "coolify.yaml"},
			wantRemoved: []string{"b.example.com"},
		},
		{
			name:       "dashboard on loopback is unreachable and nothing is defaulted",
			mutate:     func() { st.ingress.PrimaryDomain = "console.example.com" },
			wantFiles:  []string{"acme-managed-a.example.com.yaml", "coolify.yaml"},
			wantFailed: []string{"console.example.com"},
		},
		{
			name: "wildcard rejected",
			mutate: func() {
				st.ingress.PrimaryDomain = ""
				st.sites = []store.StaticSite{{Name: "docs", Domains: []string{"*.example.com"}}}
			},
			wantFiles:  []string{"acme-managed-a.example.com.yaml", "coolify.yaml"},
			wantFailed: []string{"*.example.com"},
		},
		{
			name:        "disable removes only its own files",
			mutate:      func() { st.sites = nil; st.settings.Mode = store.ProxyIntegrationOff },
			wantFiles:   []string{"coolify.yaml"},
			wantRemoved: []string{"a.example.com"},
		},
	}
	for _, step := range steps {
		step.mutate()
		rep, err := s.Sync(ctx)
		if (err != nil) != (len(step.wantFailed) > 0) {
			t.Fatalf("%s: err = %v", step.name, err)
		}
		if got := managedFiles(t, dir); !reflect.DeepEqual(got, step.wantFiles) {
			t.Errorf("%s: files = %v, want %v", step.name, got, step.wantFiles)
		}
		if !equalSet(rep.Written, step.wantWritten) || !equalSet(rep.Removed, step.wantRemoved) {
			t.Errorf("%s: written %v removed %v", step.name, rep.Written, rep.Removed)
		}
		var failed []string
		for d := range rep.Failed {
			failed = append(failed, d)
		}
		if !equalSet(failed, step.wantFailed) {
			t.Errorf("%s: failed %v", step.name, rep.Failed)
		}
	}
	if len(st.status) != 0 {
		t.Errorf("status rows left after disable: %v", st.status)
	}
	wantAudits := []string{"proxy_route.written a.example.com", "proxy_route.written b.example.com", "proxy_route.written a.example.com", "proxy_route.written b.example.com", "proxy_route.removed b.example.com", "proxy_route.removed a.example.com"}
	if !reflect.DeepEqual(st.audits, wantAudits) {
		t.Errorf("audits = %v", st.audits)
	}
}

func TestSync_KeepsFileWhenUpstreamBreaksAndSparesForeignFile(t *testing.T) {
	dir := t.TempDir()
	st := newFakeSyncStore(dir)
	st.services = []store.DesiredService{{Name: "web", Domains: []string{"a.example.com", "f.example.com"}}}
	foreign := filepath.Join(dir, "acme-managed-f.example.com.yaml")
	if err := os.WriteFile(foreign, []byte("hand written\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := newTestSyncer(st)
	ctx := context.Background()
	if _, err := s.Sync(ctx); err == nil {
		t.Fatal("foreign file: expected an error")
	}
	if b, _ := os.ReadFile(foreign); string(b) != "hand written\n" { //nolint:gosec // test temp dir
		t.Errorf("foreign file changed: %q", b)
	}
	s.Ingress.Addr = "127.0.0.1:8088"
	rep, err := s.Sync(ctx)
	if err == nil || !strings.Contains(rep.Failed["a.example.com"], UpstreamUnreachableCode) {
		t.Fatalf("rep = %+v err = %v", rep, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "acme-managed-a.example.com.yaml")); err != nil {
		t.Error("existing route was removed when its upstream became unreachable")
	}
	p, err := s.Plan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range p.Items {
		if state, _ := p.State(it); state != StateError {
			t.Errorf("%s state = %s", it.Domain, state)
		}
	}
}

func TestSync_DirectoryProblems(t *testing.T) {
	ctx := context.Background()
	st := newFakeSyncStore("/does/not/exist")
	st.services = []store.DesiredService{{Name: "web", Domains: []string{"a.example.com"}}}
	if _, err := newTestSyncer(st).Sync(ctx); err == nil || !strings.Contains(st.status["a.example.com"].LastError, "does not exist") {
		t.Errorf("err = %v status = %+v", err, st.status)
	}
	st.settings.Mode = store.ProxyIntegrationOff
	if _, err := newTestSyncer(st).Sync(ctx); err != nil {
		t.Errorf("disabled with a missing directory: %v", err)
	}
}

func TestPlanState(t *testing.T) {
	dir := t.TempDir()
	st := newFakeSyncStore(dir)
	st.services = []store.DesiredService{{Name: "web", Domains: []string{"a.example.com"}}}
	s := newTestSyncer(st)
	ctx := context.Background()
	state := func() string {
		p, err := s.Plan(ctx)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := p.State(p.Items[0])
		return got
	}
	if got := state(); got != StateMissing {
		t.Errorf("before sync = %s", got)
	}
	if _, err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if got := state(); got != StateWritten {
		t.Errorf("after sync = %s", got)
	}
	st.settings.CertResolver = "other"
	if got := state(); got != StateStale {
		t.Errorf("after settings change = %s", got)
	}
}

func equalSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	a, b = append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	return reflect.DeepEqual(a, b)
}
