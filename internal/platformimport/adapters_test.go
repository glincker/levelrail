package platformimport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", rel)) //nolint:gosec // fixed fixture paths
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var testOpts = ClientOptions{Policy: NetworkPolicy{AllowLoopback: true}}

// readOnlyServer serves fixtures by path and fails the test on any
// non-GET request except the named login path.
func readOnlyServer(t *testing.T, routes map[string]string, loginPath string, wantHeader, wantValue string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.URL.Path != loginPath {
			t.Errorf("non-GET request to source: %s %s", r.Method, r.URL.Path)
			http.Error(w, "forbidden", http.StatusMethodNotAllowed)
			return
		}
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"status":100,"description":"ok","data":{"token":"session-fixture"}}`))
			return
		}
		if wantHeader != "" && r.Header.Get(wantHeader) != wantValue {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		key := r.URL.Path
		if q := r.URL.RawQuery; q != "" {
			key += "?" + q
		}
		f, ok := routes[key]
		if !ok {
			t.Logf("unrouted: %s", key)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(fixture(t, f))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCoolifyDiscover(t *testing.T) {
	srv := readOnlyServer(t, map[string]string{
		"/api/v1/projects":                             "coolify/projects.json",
		"/api/v1/projects/p1/environments":             "coolify/environments.json",
		"/api/v1/projects/p1/production":               "coolify/env_resources.json",
		"/api/v1/applications/app-web/envs":            "coolify/envs_web.json",
		"/api/v1/applications/app-web/storages":        "coolify/storages_web.json",
		"/api/v1/applications/app-web/scheduled-tasks": "coolify/tasks_web.json",
		"/api/v1/applications/app-img/envs":            "coolify/empty_list.json",
		"/api/v1/applications/app-img/storages":        "coolify/empty_storages.json",
		"/api/v1/applications/app-img/scheduled-tasks": "coolify/empty_list.json",
		"/api/v1/applications/app-nix/envs":            "coolify/empty_list.json",
		"/api/v1/applications/app-nix/storages":        "coolify/empty_storages.json",
		"/api/v1/applications/app-nix/scheduled-tasks": "coolify/empty_list.json",
	}, "", "Authorization", "Bearer tok-fixture")
	src, err := NewCoolify(srv.URL, "tok-fixture", testOpts)
	if err != nil {
		t.Fatal(err)
	}
	d, err := src.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Apps) != 3 || len(d.Databases) != 2 || len(d.Unsupported) != 1 || len(d.ComposeApps) != 1 {
		t.Fatalf("apps=%d dbs=%d unsupported=%d compose=%d", len(d.Apps), len(d.Databases), len(d.Unsupported), len(d.ComposeApps))
	}
	web := d.Apps[0]
	checks := []struct {
		name string
		ok   bool
	}{
		{"kind git dockerfile", web.Kind == SourceGit && web.BuildMethod == "dockerfile" && web.BuildPath == "Dockerfile"},
		{"git url", web.GitURL == "https://github.com/acme/web" && web.GitBranch == "main"},
		{"domains", strings.Join(web.Domains, ",") == "web.example.com,www.example.com"},
		{"port", web.Port == 3000},
		{"health", web.Health != nil && web.Health.Path == "/healthz" && web.Health.IntervalSeconds == 10 && web.Health.Retries == 4},
		{"limits", web.MemoryBytes == 512<<20 && web.NanoCPUs == 500000000},
		{"volume", len(web.Volumes) == 1 && web.Volumes[0].Name == "web-uploads" && web.Volumes[0].ContainerPath == "/app/uploads"},
		{"cron enabled only", len(web.Crons) == 1 && web.Crons[0].Schedule == "0 3 * * *"},
		{"preview env dropped, 4 vars", len(web.Env) == 4},
		{"session secret real value", envOf(web, "SESSION_SECRET").Value == "s3cr3t-fixture" && envOf(web, "SESSION_SECRET").Secret},
		{"shown once is secret", envOf(web, "PLAIN_HIDDEN").Secret},
		{"plain env not secret", !envOf(web, "NODE_ENV").Secret},
	}
	for _, c := range checks {
		if !c.ok {
			t.Errorf("check failed: %s (%+v)", c.name, web)
		}
	}
	img := d.Apps[1]
	if img.Kind != SourceImage || img.Image != "ghcr.io/acme/proxy:v2" || img.MemoryBytes != 0 {
		t.Errorf("image app: %+v", img)
	}
	nix := d.Apps[2]
	if nix.BuildMethod != "railpack" || nix.GitURL != "https://gitlab.example.org/acme/api.git" || len(nix.Notes) < 2 {
		t.Errorf("nixpacks app: %+v", nix)
	}
	if d.Databases[0].Engine != "postgres" || d.Databases[0].Version != "16" || d.Databases[1].Engine != "redis" || d.Databases[1].Version != "7.2" {
		t.Errorf("databases: %+v", d.Databases)
	}
}

func envOf(a App, key string) Env {
	for _, e := range a.Env {
		if e.Key == key {
			return e
		}
	}
	return Env{}
}

func TestDokployDiscover(t *testing.T) {
	srv := readOnlyServer(t, map[string]string{
		"/api/project.all":                          "dokploy/project_all.json",
		"/api/application.one?applicationId=a-gh":   "dokploy/app_gh.json",
		"/api/compose.one?composeId=c1":             "dokploy/compose_stack.json",
		"/api/application.one?applicationId=a-img":  "dokploy/app_img.json",
		"/api/application.one?applicationId=a-drop": "dokploy/app_drop.json",
	}, "", "x-api-key", "key-fixture")
	src, err := NewDokploy(srv.URL, "key-fixture", testOpts)
	if err != nil {
		t.Fatal(err)
	}
	d, err := src.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Apps) != 3 || len(d.Databases) != 3 || len(d.Unsupported) != 1 || len(d.ComposeApps) != 1 || d.ComposeApps[0].Verdict != ComposeReady {
		t.Fatalf("apps=%d dbs=%d unsupported=%d compose=%+v", len(d.Apps), len(d.Databases), len(d.Unsupported), d.ComposeApps)
	}
	gh := d.Apps[0]
	if gh.GitURL != "https://github.com/acme/storefront" || gh.BuildMethod != "railpack" || gh.Replicas != 3 {
		t.Errorf("gh app: %+v", gh)
	}
	if strings.Join(gh.Domains, ",") != "shop.example.com,www.shop.example.com" || gh.Port != 3000 {
		t.Errorf("domains/port: %+v", gh)
	}
	if gh.MemoryBytes != 536870912 || gh.NanoCPUs != 500000000 {
		t.Errorf("limits: %+v", gh)
	}
	if len(gh.Env) != 3 || envOf(gh, "DATABASE_URL").Value != "postgres://db/shop" || !envOf(gh, "DATABASE_URL").Secret || envOf(gh, "LOG_LEVEL").Value != "info" {
		t.Errorf("env: %+v", gh.Env)
	}
	if len(gh.Volumes) != 2 || gh.Volumes[0].Name != "uploads" || gh.Volumes[1].HostPath != "/srv/shared" {
		t.Errorf("volumes: %+v", gh.Volumes)
	}
	if gh.Health == nil || gh.Health.Path != "/health" || gh.Health.IntervalSeconds != 15 || gh.Health.TimeoutSeconds != 5 {
		t.Errorf("health: %+v", gh.Health)
	}
	if d.Apps[1].Kind != SourceImage || d.Apps[1].Image != "ghcr.io/acme/worker:1.4" || d.Apps[1].Replicas != 1 {
		t.Errorf("image app: %+v", d.Apps[1])
	}
	engines := d.Databases[0].Engine + d.Databases[1].Engine + d.Databases[2].Engine
	if engines != "postgresmongodbredis" || d.Databases[0].Version != "15" {
		t.Errorf("databases: %+v", d.Databases)
	}
}

func TestCapRoverDiscover(t *testing.T) {
	srv := readOnlyServer(t, map[string]string{
		"/api/v2/user/apps/appDefinitions": "caprover/app_definitions.json",
	}, "/api/v2/login", "x-captain-auth", "session-fixture")
	src, err := NewCapRover(srv.URL, "pw-login", testOpts)
	if err != nil {
		t.Fatal(err)
	}
	d, err := src.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Apps) != 2 {
		t.Fatalf("apps=%d", len(d.Apps))
	}
	blog := d.Apps[0]
	if blog.Replicas != 2 || blog.Port != 2368 || blog.GitURL != "https://github.com/acme/blog" || blog.GitBranch != "prod" {
		t.Errorf("blog: %+v", blog)
	}
	if strings.Join(blog.Domains, ",") != "blog.apps.example.com,blog.example.com" {
		t.Errorf("domains: %v", blog.Domains)
	}
	if len(blog.Volumes) != 2 || blog.Volumes[0].Name != "blog-content" || blog.Volumes[1].HostPath != "/srv/blog" {
		t.Errorf("volumes: %+v", blog.Volumes)
	}
	if !envOf(blog, "DB_PASSWORD").Secret || envOf(blog, "url").Secret {
		t.Errorf("env: %+v", blog.Env)
	}
	proxyNotes := 0
	for _, n := range blog.Notes {
		if strings.Contains(n.Reason, "forced HTTPS") || strings.Contains(n.Reason, "websocket") || strings.Contains(n.Reason, "redirected") || strings.Contains(n.Reason, "basic auth") {
			proxyNotes++
		}
	}
	if proxyNotes != 4 {
		t.Errorf("proxy notes=%d: %+v", proxyNotes, blog.Notes)
	}
	job := d.Apps[1]
	if len(job.Domains) != 0 || job.Replicas != 1 || job.Port != 80 || len(job.Notes) < 2 {
		t.Errorf("job: %+v", job)
	}
}

func TestCapRoverBadLoginRedactsPassword(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":1105,"description":"wrong password pw-login"}`))
	}))
	defer srv.Close()
	src, _ := NewCapRover(srv.URL, "pw-login", testOpts)
	_, err := src.Discover(context.Background())
	if err == nil || strings.Contains(err.Error(), "pw-login") {
		t.Fatalf("want redacted error, got %v", err)
	}
}

func TestSourceRedirectIsNotFollowed(t *testing.T) {
	leaked := make(chan string, 1)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked <- r.Header.Get("x-api-key")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusFound)
	}))
	defer srv.Close()
	src, _ := NewDokploy(srv.URL, "key-fixture", testOpts)
	if _, err := src.Discover(context.Background()); err == nil || !strings.Contains(err.Error(), "redirected") {
		t.Fatalf("want redirect refusal, got %v", err)
	}
	select {
	case v := <-leaked:
		t.Fatalf("redirect target received a request with x-api-key %q", v)
	default:
	}
}

func TestSourceErrorRedactsToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad token tok-secret-fixture", http.StatusUnauthorized)
	}))
	defer srv.Close()
	src, _ := NewCoolify(srv.URL, "tok-secret-fixture", testOpts)
	_, err := src.Discover(context.Background())
	if err == nil || strings.Contains(err.Error(), "tok-secret-fixture") {
		t.Fatalf("want redacted error, got %v", err)
	}
}

func TestComposeAndDokku(t *testing.T) {
	yml := "services:\n  web:\n    image: nginx:1.27\n    ports: ['8080:80']\n    environment:\n      API_KEY: abc\n      MODE: prod\n    volumes: ['data:/var/data']\n  db:\n    image: postgres:16\n  builder:\n    image: ''\n"
	d, err := ComposeSource{Data: []byte(yml)}.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Apps) != 1 || d.Apps[0].Port != 80 || !envOf(d.Apps[0], "API_KEY").Secret || len(d.Apps[0].Volumes) != 1 {
		t.Errorf("compose app: %+v", d.Apps)
	}
	if len(d.Databases) != 1 || d.Databases[0].Engine != "postgres" || len(d.Unsupported) != 1 {
		t.Errorf("compose db/unsupported: %+v %+v", d.Databases, d.Unsupported)
	}
	dk, err := DokkuEnvSource{AppName: "blog", Data: "export FOO=bar\nDOKKU_APP_RESTORE=1\nGIT_REV=abc\nSECRET_KEY='x y'\n"}.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dk.Apps[0].Env) != 2 || envOf(dk.Apps[0], "SECRET_KEY").Value != "x y" {
		t.Errorf("dokku env: %+v", dk.Apps[0].Env)
	}
	if _, err := (DokkuEnvSource{}).Discover(context.Background()); err == nil {
		t.Error("want error for empty app name")
	}
}

func TestCoolifyReportsTasksBackupsAndDestinations(t *testing.T) {
	srv := readOnlyServer(t, map[string]string{
		"/api/v1/projects":                             "coolify/projects.json",
		"/api/v1/projects/p1/environments":             "coolify/environments.json",
		"/api/v1/projects/p1/production":               "coolify/env_resources.json",
		"/api/v1/applications/app-web/envs":            "coolify/envs_web.json",
		"/api/v1/applications/app-web/storages":        "coolify/storages_web.json",
		"/api/v1/applications/app-web/scheduled-tasks": "coolify/tasks_web.json",
		"/api/v1/applications/app-img/envs":            "coolify/empty_list.json",
		"/api/v1/applications/app-img/storages":        "coolify/empty_storages.json",
		"/api/v1/applications/app-img/scheduled-tasks": "coolify/empty_list.json",
		"/api/v1/applications/app-nix/envs":            "coolify/empty_list.json",
		"/api/v1/applications/app-nix/storages":        "coolify/empty_storages.json",
		"/api/v1/applications/app-nix/scheduled-tasks": "coolify/empty_list.json",
		"/api/v1/databases/db-pg/backups":              "coolify/backups_pg.json",
	}, "", "Authorization", "Bearer tok-fixture")
	src, err := NewCoolify(srv.URL, "tok-fixture", testOpts)
	if err != nil {
		t.Fatal(err)
	}
	d, err := src.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var manual []string
	for _, n := range d.Apps[0].Notes {
		manual = append(manual, n.Manual)
	}
	for _, u := range d.Unsupported {
		manual = append(manual, u.Manual)
	}
	all := strings.Join(manual, "\n")
	for _, want := range []string{
		"levelrail-cli jobs create 'my-web-app' 'off' --schedule '* * * * *' -- sh -c 'true'",
		"levelrail-cli backups schedule set 'main-db' --target <TARGET_ID> --cron '0 0 * * *'",
		"levelrail-cli backup-targets create --name 's3-main' --provider custom --endpoint <S3_ENDPOINT> --bucket 'acme-backups'",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
	if strings.Count(all, "backup-targets create") != 1 {
		t.Errorf("destination should be listed once:\n%s", all)
	}
}

func TestCoolifyServiceComposeMasked(t *testing.T) {
	compose := "services:\n  plausible:\n    image: ghcr.io/plausible/community-edition:v2\n    environment:\n      - SECRET_KEY_BASE=hunter2-inline\n      - BASE_URL=${SERVICE_URL_PLAUSIBLE}\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/projects":
			_, _ = w.Write([]byte(`[{"uuid":"p1","name":"Shop"}]`))
		case "/api/v1/projects/p1/environments":
			_, _ = w.Write([]byte(`[{"name":"production"}]`))
		case "/api/v1/projects/p1/production":
			_, _ = w.Write([]byte(`{"services":[{"uuid":"svc1","name":"plausible","service_type":"plausible"}]}`))
		case "/api/v1/services/svc1":
			b, _ := json.Marshal(map[string]string{"docker_compose_raw": compose})
			_, _ = w.Write(b)
		case "/api/v1/services/svc1/envs":
			_, _ = w.Write([]byte(`[{"key":"POSTGRES_PASSWORD","value":"pg-pass-plain"},{"key":"SERVICE_FQDN_PLAUSIBLE","value":"stats.example.com"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	src, err := NewCoolify(srv.URL, "tok", testOpts)
	if err != nil {
		t.Fatal(err)
	}
	d, err := src.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Services) != 1 {
		t.Fatalf("services: %+v", d.Services)
	}
	s := d.Services[0]
	if !strings.Contains(s.Compose, "image: ghcr.io/plausible/community-edition:v2") || !strings.Contains(s.Compose, "${SERVICE_URL_PLAUSIBLE}") {
		t.Errorf("compose not stored: %q", s.Compose)
	}
	if strings.Contains(s.Compose, "hunter2-inline") {
		t.Errorf("inline secret leaked: %q", s.Compose)
	}
	for _, e := range s.Env {
		if e.Key == "POSTGRES_PASSWORD" && (e.Value != SecretMask || !e.Secret) {
			t.Errorf("password env not masked: %+v", e)
		}
		if e.Key == "SERVICE_FQDN_PLAUSIBLE" && e.Value != "stats.example.com" {
			t.Errorf("plain env altered: %+v", e)
		}
	}
	if len(d.Unsupported) != 1 {
		t.Errorf("unsupported entry should remain: %+v", d.Unsupported)
	}
}
