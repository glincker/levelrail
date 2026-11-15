package connhub

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeGit struct {
	statuses map[string]store.GitConnectionStatus
}

func (f fakeGit) ListGitConnections(context.Context, string) ([]store.GitConnection, error) {
	return []store.GitConnection{{ID: "gc_1", Provider: "gitlab", Label: "team gitlab", Host: "gitlab.com", AccountName: "ana"}}, nil
}
func (f fakeGit) GetGitConnectionStatus(_ context.Context, k string) (store.GitConnectionStatus, bool, error) {
	s, ok := f.statuses[k]
	return s, ok, nil
}
func (f fakeGit) GetGitHubAppConnection(context.Context) (store.GitHubAppConnection, error) {
	return store.GitHubAppConnection{AppID: 7, InstanceURL: "https://github.com"}, nil
}
func (f fakeGit) ListGitHubAppInstallations(context.Context) ([]store.GitHubAppInstallation, error) {
	return []store.GitHubAppInstallation{{InstallationID: 42, AccountLogin: "acme"}}, nil
}
func (f fakeGit) ListGitSourcesByConnection(context.Context, string) ([]string, error) {
	return []string{"web", "api"}, nil
}
func (f fakeGit) GetGitSource(_ context.Context, name string) (*store.GitSource, error) {
	if name != "web" {
		return nil, store.ErrGitSourceNotFound
	}
	return &store.GitSource{ServiceName: "web", ConnectionID: "gc_1"}, nil
}

type fakeRegistry struct{ list []store.RegistryCredential }

func (f fakeRegistry) ListRegistryCredentials(context.Context) ([]store.RegistryCredential, error) {
	return f.list, nil
}

type fakeNotify struct{}

func (fakeNotify) ListNotificationChannels(context.Context) ([]alerting.NotificationChannel, error) {
	return []alerting.NotificationChannel{{ID: "chn_1", Name: "ops", Kind: alerting.NotifySlack, NotifyURL: "https://hooks.example/SECRET-TOKEN", InteractiveSecret: "SECRET-SIGN", Enabled: true}}, nil
}

type fakeWebhooks struct{}

func (fakeWebhooks) ListEventWebhooks(context.Context) ([]store.EventWebhook, error) {
	return []store.EventWebhook{{ID: "ew1", Name: "ci", URL: "https://x.example/SECRET-TOKEN", Enabled: true}}, nil
}
func (fakeWebhooks) ListStatusWebhooks(context.Context) ([]store.StatusWebhook, error) {
	return []store.StatusWebhook{{ID: "sw1", Label: "pager", Paused: true, PausedReason: "too many failures"}}, nil
}

type fakeMeta struct {
	meta map[string]store.ConnectionMeta
}

func (f fakeMeta) ListConnectionMeta(context.Context) (map[string]store.ConnectionMeta, error) {
	return f.meta, nil
}
func (f fakeMeta) CountConnectionBindingsByRef(context.Context) (map[string]int, error) {
	return map[string]int{"git:gc_1": 1}, nil
}
func (f fakeMeta) ListConnectionBindingsByTarget(context.Context, string, string) ([]store.ConnectionBinding, error) {
	return []store.ConnectionBinding{{ID: "cb_1", Ref: "registry:rc_1", TargetType: "app", TargetID: "web", Role: "registry_pull"}}, nil
}

func newTestService(now time.Time) *Service {
	past, soon := now.Add(-time.Hour), now.Add(24*time.Hour)
	s := New(Sources{
		Git: fakeGit{statuses: map[string]store.GitConnectionStatus{
			"github": {Status: store.GitConnOK, UpdatedAt: now},
			"gc_1":   {Status: store.GitConnNeedsReauthorize, Detail: "token revoked", UpdatedAt: now},
		}},
		Registry: fakeRegistry{list: []store.RegistryCredential{
			{ID: "rc_old", Name: "old", RegistryHost: "ghcr.io", ExpiresAt: &past},
			{ID: "rc_ecr", Name: "ecr", RegistryHost: "x.dkr.ecr.eu-west-1.amazonaws.com", Preset: "ecr", LastRefreshError: "sts denied"},
			{ID: "rc_soon", Name: "soon", RegistryHost: "quay.io", ExpiresAt: &soon},
		}},
		Notify:   fakeNotify{},
		Webhooks: fakeWebhooks{},
		Meta: fakeMeta{meta: map[string]store.ConnectionMeta{
			"notify:chn_1": {Ref: "notify:chn_1", ScopeType: "org", ScopeID: "o1", Health: HealthDegraded, HealthDetail: "last test failed", ExpiresAt: soon},
		}},
	}, func(context.Context) time.Duration { return 48 * time.Hour })
	s.now = func() time.Time { return now }
	return s
}

func byRef(list []Connection) map[string]Connection {
	m := map[string]Connection{}
	for _, c := range list {
		m[c.Ref] = c
	}
	return m
}

func TestListMapsHealthAndExpiry(t *testing.T) {
	list, err := newTestService(time.Now()).List(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	m := byRef(list)
	cases := map[string]string{
		"github:install:42": HealthOK, "git:gc_1": HealthNeedsReconnect, "registry:rc_old": HealthExpired,
		"registry:rc_ecr": HealthDegraded, "registry:rc_soon": HealthUnknown, "notify:chn_1": HealthDegraded,
		"webhook:event:ew1": HealthUnknown, "webhook:status:sw1": HealthDegraded,
	}
	for ref, want := range cases {
		if got := m[ref].Health; got != want {
			t.Errorf("%s health = %q, want %q", ref, got, want)
		}
	}
	if !m["registry:rc_soon"].ExpiringSoon || m["registry:rc_old"].ExpiringSoon {
		t.Errorf("expiring_soon wrong: %+v %+v", m["registry:rc_soon"], m["registry:rc_old"])
	}
	if c := m["notify:chn_1"]; c.ScopeType != "org" || c.ScopeID != "o1" || !c.ExpiringSoon {
		t.Errorf("sidecar overlay missing: %+v", c)
	}
	if m["git:gc_1"].BoundCount != 3 || m["registry:rc_old"].ScopeType != "instance" {
		t.Errorf("bound count or default scope wrong: %+v", m["git:gc_1"])
	}
}

func TestListNeverCarriesSecrets(t *testing.T) {
	list, _ := newTestService(time.Now()).List(context.Background(), Filter{})
	raw, _ := json.Marshal(list)
	if strings.Contains(string(raw), "SECRET") {
		t.Fatalf("projection leaked a secret: %s", raw)
	}
}

func TestFilterAndGet(t *testing.T) {
	s := newTestService(time.Now())
	list, _ := s.List(context.Background(), Filter{Kind: KindRegistry, Health: HealthExpired})
	if len(list) != 1 || list[0].Ref != "registry:rc_old" {
		t.Fatalf("filter = %+v", list)
	}
	if _, err := s.Get(context.Background(), "registry:nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	if c, err := s.Get(context.Background(), "git:gc_1"); err != nil || c.Account != "ana" {
		t.Fatalf("get = %+v, %v", c, err)
	}
}

func TestAppBindingsMergeStoredAndLegacy(t *testing.T) {
	got, err := newTestService(time.Now()).AppBindings(context.Background(), "web")
	if err != nil || len(got) != 2 || got[0].Origin != "binding" || got[1].Origin != "legacy" || got[1].Ref != "git:gc_1" {
		t.Fatalf("bindings = %+v, %v", got, err)
	}
}
