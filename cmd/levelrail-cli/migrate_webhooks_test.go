package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const fakeHookSecret = "fake-hook-secret-value"

// fakeGitTarget serves the connection list and the register route; it counts
// register calls per app so a duplicate registration is caught.
type fakeGitTarget struct {
	mu         sync.Mutex
	registered map[string]int
	failApp    string
}

func (f *fakeGitTarget) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps":
			http.Error(w, `{"error":"an app with this name already exists"}`, http.StatusConflict)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/github-app":
			_, _ = w.Write([]byte(`{"connected":true,"installed":true}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/git-connections":
			_, _ = w.Write([]byte(`[{"id":"c1","provider":"gitlab","host":"gitlab.example.org","instance_url":"https://gitlab.example.org","authorized":true},
				{"id":"c2","provider":"gitea","host":"gitea.example.org","instance_url":"https://gitea.example.org:3000","authorized":true},
				{"id":"c3","provider":"gitea","host":"plain.example.org","instance_url":"http://plain.example.org","authorized":true}]`))
		case strings.HasSuffix(r.URL.Path, "/git-source/webhook/register") && r.Method == http.MethodPost:
			name := strings.Split(r.URL.Path, "/")[4]
			if name == f.failApp {
				http.Error(w, `{"error":"provider said no\u001b[31m"}`, http.StatusBadGateway)
				return
			}
			f.registered[name]++
			_ = json.NewEncoder(w).Encode(map[string]any{"service_name": name, "webhook_secret": fakeHookSecret,
				"webhook_registration": map[string]string{"state": webhookRegisteredState}})
		case strings.HasSuffix(r.URL.Path, "/git-source") && r.Method == http.MethodGet:
			name := strings.Split(r.URL.Path, "/")[4]
			state := ""
			if f.registered[name] > 0 {
				state = webhookRegisteredState
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"service_name": name, "webhook_registration": map[string]string{"state": state}})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}
}

func webhookTestApps() []mappedApp {
	mk := func(name, repo string) mappedApp {
		return mappedApp{SourceName: name, ServiceName: name, RepoURL: repo, Applied: true, gitSourceSet: true}
	}
	return []mappedApp{
		mk("gh", "https://github.com/acme/web"),
		mk("gl", "https://gitlab.example.org/acme/api.git"),
		mk("gitea-port", "https://gitea.example.org:3000/acme/svc"),
		mk("wrong-port", "https://gitea.example.org/acme/svc"),
		mk("unconnected", "https://bitbucket.org/acme/x"),
		mk("plain-http", "http://plain.example.org/acme/x"),
		mk("ssh", "ssh://git@github.com/acme/web"),
		mk("ctrl", "https://github.com/acme/\x1bweb"),
		mk("odd-host", "https://gith%75b.com/acme/web"),
		mk("fails", "https://github.com/acme/fails"),
		mk("evil", "https://evil.com/acme/web"),
		mk("upper", "https://GitHub.COM/acme/upper"),
		mk("trailing-dot", "https://github.com./acme/web"),
		{SourceName: "not-created", ServiceName: "not-created", RepoURL: "https://github.com/acme/nc"},
		{SourceName: "image", ServiceName: "image"},
	}
}

func outcomeByName(apps []mappedApp) map[string]*webhookOutcome {
	out := map[string]*webhookOutcome{}
	for i := range apps {
		out[apps[i].ServiceName] = apps[i].Webhook
	}
	return out
}

func TestApplyMigrationWebhooksConnectedOnlyAndIdempotent(t *testing.T) {
	fake := &fakeGitTarget{registered: map[string]int{}, failApp: "fails"}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()
	client := NewClient(srv.URL, "fake-token")

	apps := webhookTestApps()
	applyMigrationWebhooks(context.Background(), client, apps, func(string) (string, bool) { return "", false })
	got := outcomeByName(apps)
	want := map[string]string{
		"gh": webhookCreated, "gl": webhookCreated, "gitea-port": webhookCreated,
		"wrong-port": webhookSkipped, "unconnected": webhookSkipped, "plain-http": webhookSkipped,
		"ssh": webhookSkipped, "ctrl": webhookSkipped, "odd-host": webhookSkipped,
		"fails": webhookFailed, "not-created": webhookSkipped,
		"evil": webhookSkipped, "upper": webhookCreated, "trailing-dot": webhookSkipped,
	}
	for name, state := range want {
		if got[name] == nil || got[name].State != state {
			t.Errorf("%s: want %s, got %+v", name, state, got[name])
		}
	}
	if got["image"] != nil {
		t.Errorf("app without a repo got a webhook outcome: %+v", got["image"])
	}
	if strings.ContainsRune(got["fails"].Detail, 0x1b) {
		t.Errorf("control character kept in detail %q", got["fails"].Detail)
	}

	// A real re-run: the app exists, so create answers 409 and the app is skipped without a register call.
	rerun := migrationReport{Apps: []mappedApp{{SourceName: "gh", ServiceName: "gh", RepoURL: "https://github.com/acme/web", Service: mappedService{Port: 80}}}, Applied: true}
	applyMigration(context.Background(), client, &rerun)
	applyMigrationWebhooks(context.Background(), client, rerun.Apps, func(string) (string, bool) { return "", false })
	if w := rerun.Apps[0].Webhook; w == nil || w.State != webhookSkipped {
		t.Errorf("re-run outcome %+v", w)
	}
	// An app marked created whose hook already exists is caught by the stored registration state.
	again := webhookTestApps()
	applyMigrationWebhooks(context.Background(), client, again, func(string) (string, bool) { return "", false })
	if s := outcomeByName(again)["gh"].State; s != webhookAlready {
		t.Errorf("second pass state %s", s)
	}
	for name, n := range fake.registered {
		if n != 1 {
			t.Errorf("%s registered %d times", name, n)
		}
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(migrationReport{Apps: apps, Applied: true}); err != nil {
		t.Fatal(err)
	}
	printMigrationReportHuman(&buf, migrationReport{Apps: apps, Applied: true}, "lr", "./migrated")
	if strings.Contains(buf.String(), fakeHookSecret) {
		t.Error("webhook secret leaked into the report")
	}
}

func TestApplyMigrationWebhooksLimitFromEnv(t *testing.T) {
	fake := &fakeGitTarget{registered: map[string]int{}}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()
	apps := webhookTestApps()[:3]
	env := func(k string) (string, bool) { return "1", k == envMigrateWebhookLimit }
	applyMigrationWebhooks(context.Background(), NewClient(srv.URL, "fake-token"), apps, env)
	if len(fake.registered) != 1 {
		t.Errorf("limit not honoured: %+v", fake.registered)
	}
}

func TestPlanMigrationWebhooksWritesNothing(t *testing.T) {
	apps := webhookTestApps()
	planMigrationWebhooks(apps, func(string) (string, bool) { return "", false })
	got := outcomeByName(apps)
	if got["gh"].State != webhookWouldCreate || got["plain-http"].State != webhookSkipped || got["image"] != nil {
		t.Errorf("plan: gh=%+v plain=%+v", got["gh"], got["plain-http"])
	}
}

func TestMigrationPipelineWebhooksOffByDefault(t *testing.T) {
	var mu sync.Mutex
	var gitCalls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only the app create belongs to a run without the flag; every other request counts.
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/apps" {
			mu.Lock()
			gitCalls = append(gitCalls, r.Method+" "+r.URL.Path)
			mu.Unlock()
		}
		http.Error(w, `{"error":"exists"}`, http.StatusConflict)
	}))
	defer srv.Close()
	report := migrationReport{CoolifyURL: "https://coolify.example.com", Apps: webhookTestApps()[:1], Applied: true}
	var out, errOut bytes.Buffer
	runMigrationPipeline(context.Background(), &report, migratePipelineFlags{apply: true, targetToken: "fake-token", targetAPIURL: srv.URL, jsonOut: true},
		"lr", &out, &errOut, func(string) (string, bool) { return "", false })
	for _, a := range report.Apps {
		if a.Webhook != nil {
			t.Errorf("webhook outcome without the flag: %+v", a.Webhook)
		}
	}
	if len(gitCalls) != 0 {
		t.Errorf("requests other than app create without the flag: %v", gitCalls)
	}
}

func TestWebhookLimitsClampedToCeiling(t *testing.T) {
	env := func(string) (string, bool) { return "999999999", true }
	if hooks, url := webhookLimits(env); hooks != ceilingMigrateWebhookLimit || url != ceilingMigrateRepoURLMax {
		t.Errorf("limits not clamped: %d %d", hooks, url)
	}
}
