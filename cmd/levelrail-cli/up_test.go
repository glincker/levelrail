package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func initRepo(t *testing.T, root, remote string) {
	t.Helper()
	repo, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if remote == "" {
		return
	}
	if _, err := repo.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{remote}}); err != nil {
		t.Fatal(err)
	}
}

func TestInferUp(t *testing.T) {
	const remote = "https://github.com/acme/shop.git"
	tests := []struct {
		name      string
		files     map[string]string
		subdir    string
		flags     upFlags
		wantErr   string
		wantName  string
		wantPort  int
		wantBuild string
		wantBase  string
		wantSrc   map[string]string
		static    bool
	}{
		{
			name:     "dockerfile repo uses EXPOSE",
			files:    map[string]string{"Dockerfile": "FROM alpine\nEXPOSE 4000\n"},
			wantPort: 4000, wantBuild: "dockerfile", wantSrc: map[string]string{"port": srcDetected, "image repo": srcDefault, "repo": srcGit},
		},
		{
			name:     "node repo without a Dockerfile builds with railpack",
			files:    map[string]string{"package.json": `{"dependencies":{"express":"4"},"scripts":{"start":"node ."}}`},
			wantPort: 3000, wantBuild: "railpack",
		},
		{
			name:     "go repo",
			files:    map[string]string{"go.mod": "module x\n"},
			wantPort: 8080, wantBuild: "railpack",
		},
		{
			name:     "next.js",
			files:    map[string]string{"package.json": `{"dependencies":{"next":"15"},"scripts":{"start":"next start"}}`},
			wantPort: 3000, wantBuild: "railpack",
		},
		{
			name:  "static site has no port and a synthesized spec",
			files: map[string]string{"index.html": "<html></html>"}, static: true, wantBuild: "static",
		},
		{
			name:     "monorepo subdirectory sets the base directory",
			files:    map[string]string{"apps/web/package.json": `{"scripts":{"start":"x"}}`},
			subdir:   "apps/web",
			wantPort: 3000, wantBuild: "railpack", wantBase: "apps/web", wantName: "web",
		},
		{
			name:     "flags beat detection",
			files:    map[string]string{"package.json": `{"scripts":{"start":"x"}}`},
			flags:    upFlags{name: "mine", port: 9999},
			wantName: "mine", wantPort: 9999, wantBuild: "railpack", wantSrc: map[string]string{"name": srcFlag, "port": srcFlag},
		},
		{
			name:     "spec beats detection, flag beats spec",
			files:    map[string]string{"package.json": `{}`, "app.yaml": "version: 1\nservices:\n  api:\n    build: {type: railpack}\n    port: 9000\n    replicas: 2\n"},
			flags:    upFlags{replicas: 3},
			wantName: "api", wantPort: 9000, wantBuild: "railpack", wantSrc: map[string]string{"port": srcSpec, "name": srcSpec, "replicas": srcFlag},
		},
		{
			name:     "spec port beats detection when no flag",
			files:    map[string]string{"Dockerfile": "FROM x\nEXPOSE 1111\n", "app.yaml": "version: 1\nservices:\n  api:\n    build: {type: dockerfile}\n    port: 2222\n"},
			wantName: "api", wantPort: 2222, wantBuild: "dockerfile", wantSrc: map[string]string{"port": srcSpec},
		},
		{
			name:    "compose project is pointed at compose deploy",
			files:   map[string]string{"compose.yaml": "services: {}\n"},
			wantErr: "Docker Compose",
		},
		{
			name:    "nothing detected",
			files:   map[string]string{"README.md": "x"},
			wantErr: "could not tell how to build",
		},
		{
			name:    "multi service spec is refused",
			files:   map[string]string{"app.yaml": "version: 1\nservices:\n  a:\n    build: {type: railpack}\n    port: 1\n  b:\n    build: {type: railpack}\n    port: 2\n"},
			wantErr: "declares 2 services",
		},
		{
			name:    "unknown database engine",
			files:   map[string]string{"go.mod": "module x\n"},
			flags:   upFlags{with: []string{"cobol"}},
			wantErr: "engine must be one of",
		},
		{
			name:    "bad memory",
			files:   map[string]string{"go.mod": "module x\n"},
			flags:   upFlags{memory: "lots"},
			wantErr: "--memory",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			initRepo(t, root, remote)
			writeTree(t, root, tc.files)
			dir := filepath.Join(root, tc.subdir)
			inf, err := inferUp(dir, tc.flags, detectGitAt(dir))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("inferUp: %v", err)
			}
			if tc.wantName != "" && inf.Flags.name != tc.wantName {
				t.Errorf("name = %q, want %q", inf.Flags.name, tc.wantName)
			}
			if inf.Flags.port != tc.wantPort {
				t.Errorf("port = %d, want %d", inf.Flags.port, tc.wantPort)
			}
			if buildLabel(inf) != tc.wantBuild {
				t.Errorf("build = %q, want %q", buildLabel(inf), tc.wantBuild)
			}
			if inf.Flags.baseDirectory != tc.wantBase {
				t.Errorf("base directory = %q, want %q", inf.Flags.baseDirectory, tc.wantBase)
			}
			if inf.Static != tc.static || (tc.static && inf.FileSpec == nil) {
				t.Errorf("static = %v, filespec nil = %v", inf.Static, inf.FileSpec == nil)
			}
			if inf.Flags.repo != remote || (!tc.static && inf.Flags.imageRepo != inf.Flags.name) {
				t.Errorf("repo = %q, image repo = %q", inf.Flags.repo, inf.Flags.imageRepo)
			}
			for k, v := range tc.wantSrc {
				if inf.Sources[k] != v {
					t.Errorf("source of %s = %q, want %q", k, inf.Sources[k], v)
				}
			}
			if _, err := inf.createPlan(); err != nil {
				t.Errorf("inferred settings do not plan: %v", err)
			}
		})
	}
}

func TestInferUpNeedsRemote(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, "")
	writeTree(t, root, map[string]string{"go.mod": "module x\n"})
	_, err := inferUp(root, upFlags{}, detectGitAt(root))
	if err == nil || !strings.Contains(err.Error(), "git remote") || !strings.Contains(err.Error(), "levelrail-cli deploy") {
		t.Fatalf("err = %v, want a git remote hint", err)
	}
	inf, err := inferUp(root, upFlags{repo: "https://example.com/x.git"}, detectGitAt(root))
	if err != nil || inf.Sources["repo"] != srcFlag {
		t.Fatalf("--repo should satisfy it: err=%v src=%q", err, inf.Sources["repo"])
	}
}

func TestInferUpDatabasesAndResources(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, "https://example.com/a/b.git")
	writeTree(t, root, map[string]string{"go.mod": "module x\n"})
	inf, err := inferUp(root, upFlags{with: []string{"postgres:16", "redis", "mysql", "postgres"}, memory: "512Mi", cpu: 0.5}, detectGitAt(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(inf.DBs) != 3 {
		t.Fatalf("databases = %+v, want postgres, redis, mysql once each", inf.DBs)
	}
	if inf.DBs[0].Version != "16" || inf.DBs[0].EnvVar != "DATABASE_URL" || inf.DBs[1].EnvVar != "REDIS_URL" || inf.DBs[2].EnvVar != "" {
		t.Errorf("databases = %+v", inf.DBs)
	}
	if inf.Resources == nil || inf.Resources.MemoryBytes != 512<<20 || inf.Resources.NanoCPUs != 5e8 {
		t.Errorf("resources = %+v", inf.Resources)
	}
	cmds := strings.Join(inf.upCommands("lr", false), "\n")
	for _, want := range []string{"lr apps create --name", "lr apps scale " + inf.Flags.name + " --memory 512Mi --cpu 0.5", "lr databases create --name " + inf.Flags.name + "-postgres --engine postgres --version 16", "lr apps connect " + inf.Flags.name + " " + inf.Flags.name + "-redis --env-var REDIS_URL"} {
		if !strings.Contains(cmds, want) {
			t.Errorf("plan missing %q in:\n%s", want, cmds)
		}
	}
}

func TestRelUnderRoot(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct{ rel, want string }{{".", ""}, {"apps/web", "apps/web"}} {
		dir := filepath.Join(root, tc.rel)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		if got := relUnderRoot(root, dir); got != tc.want {
			t.Errorf("relUnderRoot(%q) = %q, want %q", tc.rel, got, tc.want)
		}
	}
}

type fakeUp struct {
	mu          sync.Mutex
	app         *appResource
	dbs         map[string]bool
	conns       []string
	calls       map[string]int
	failConnect bool
	// outcome is what GET .../deploys/{id} reports; empty means healthy.
	outcome string
	domains []string
}

func newFakeUp(t *testing.T) (*fakeUp, *httptest.Server) {
	t.Helper()
	f := &fakeUp{dbs: map[string]bool{}, calls: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeUp) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := r.Method + " " + r.URL.Path
	f.calls[key]++
	w.Header().Set("Content-Type", "application/json")
	reply := func(code int, v any) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	switch {
	case key == "POST /api/v1/apps":
		var a appResource
		_ = json.NewDecoder(r.Body).Decode(&a)
		f.app = &a
		reply(http.StatusCreated, a)
	case strings.HasPrefix(key, "GET /api/v1/apps/") && strings.Count(r.URL.Path, "/") == 4:
		if f.app == nil {
			reply(http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		out := *f.app
		out.Domains = f.domains
		out.FallbackURL = "https://shop.1-2-3-4.sslip.io"
		reply(http.StatusOK, out)
	case strings.HasPrefix(key, "PUT /api/v1/apps/"):
		var a appResource
		_ = json.NewDecoder(r.Body).Decode(&a)
		f.app = &a
		reply(http.StatusOK, a)
	case strings.HasSuffix(key, "/builds"):
		reply(http.StatusAccepted, map[string]string{"id": "b1"})
	case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/deploys/"):
		d := map[string]any{"id": "b1", "outcome": orDefault(f.outcome, "healthy")}
		if f.outcome == "failed" {
			d["error"] = "build exploded"
		}
		reply(http.StatusOK, d)
	case key == "GET /api/v1/databases":
		out := []map[string]string{}
		for n := range f.dbs {
			out = append(out, map[string]string{"name": n})
		}
		reply(http.StatusOK, out)
	case key == "POST /api/v1/databases":
		var d databaseResource
		_ = json.NewDecoder(r.Body).Decode(&d)
		f.dbs[d.Name] = true
		reply(http.StatusCreated, d)
	case key == "GET /api/v1/database-engines":
		reply(http.StatusOK, []map[string]string{{"id": "postgres", "default_version": "17"}})
	case strings.HasSuffix(key, "/connections") && r.Method == http.MethodGet:
		out := []map[string]string{}
		for _, c := range f.conns {
			out = append(out, map[string]string{"database_name": c})
		}
		reply(http.StatusOK, out)
	case strings.HasSuffix(key, "/connections"):
		if f.failConnect {
			reply(http.StatusInternalServerError, map[string]string{"error": "boom"})
			return
		}
		var c struct{ Database string }
		_ = json.NewDecoder(r.Body).Decode(&c)
		f.conns = append(f.conns, c.Database)
		reply(http.StatusCreated, map[string]string{"database_name": c.Database})
	case strings.HasSuffix(key, "/domains"):
		var d struct{ Add []string }
		_ = json.NewDecoder(r.Body).Decode(&d)
		f.domains = append(f.domains, d.Add...)
		reply(http.StatusOK, map[string]any{"domains": f.domains, "changed": true})
	default:
		reply(http.StatusNotFound, map[string]string{"error": "unexpected " + key})
	}
}

func runUpCLI(t *testing.T, srv *httptest.Server, dir string, extra ...string) (int, string, string) {
	t.Helper()
	args := append([]string{"up", dir, "--api-url", srv.URL, "--token", "t", "--yes"}, extra...)
	var out, errb bytes.Buffer
	code := run("levelrail-cli-test", args, &out, &errb, envMap())
	return code, out.String(), errb.String()
}

func newUpProject(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "shop")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	initRepo(t, root, "https://github.com/acme/shop.git")
	writeTree(t, root, map[string]string{"package.json": `{"dependencies":{"express":"4"},"scripts":{"start":"node ."}}`})
	return root
}

func TestUpIsIdempotent(t *testing.T) {
	f, srv := newFakeUp(t)
	dir := newUpProject(t)
	args := []string{"--with", "postgres", "--domain", "shop.example.com", "--memory", "256Mi"}

	for i := 0; i < 2; i++ {
		code, _, stderr := runUpCLI(t, srv, dir, args...)
		if code != exitOK {
			t.Fatalf("run %d exit = %d, stderr:\n%s", i+1, code, stderr)
		}
	}
	for key, want := range map[string]int{"POST /api/v1/apps": 1, "POST /api/v1/databases": 1, "PATCH /api/v1/apps/" + dirName(dir) + "/domains": 1} {
		if f.calls[key] != want {
			t.Errorf("%s called %d times, want %d", key, f.calls[key], want)
		}
	}
	if len(f.conns) != 1 {
		t.Errorf("connections = %v, want one", f.conns)
	}
	if n := f.calls["PUT /api/v1/apps/"+dirName(dir)]; n != 0 {
		t.Errorf("second run changed settings: %d PUTs", n)
	}
	if n := f.calls["POST /api/v1/apps/"+dirName(dir)+"/builds"]; n != 2 {
		t.Errorf("builds = %d, want a rebuild per run (2)", n)
	}
}

func dirName(dir string) string { return strings.ToLower(filepath.Base(dir)) }

func TestUpUpdatesChangedSettings(t *testing.T) {
	f, srv := newFakeUp(t)
	dir := newUpProject(t)
	if code, _, e := runUpCLI(t, srv, dir); code != exitOK {
		t.Fatalf("first run: %s", e)
	}
	if code, _, e := runUpCLI(t, srv, dir, "--replicas", "3", "--strategy", "rolling"); code != exitOK {
		t.Fatalf("second run: %s", e)
	}
	if f.calls["POST /api/v1/apps"] != 1 || f.calls["PUT /api/v1/apps/"+dirName(dir)] != 1 {
		t.Errorf("calls = %v, want one create and one update", f.calls)
	}
	if f.app.Replicas != 3 || f.app.Strategy != "rolling" {
		t.Errorf("app = %+v", f.app)
	}
}

func TestUpDryRunChangesNothing(t *testing.T) {
	f, srv := newFakeUp(t)
	dir := newUpProject(t)
	code, stdout, stderr := runUpCLI(t, srv, dir, "--dry-run", "--with", "postgres,redis", "--domain", "a.example.com", "--cpu", "1")
	if code != exitOK {
		t.Fatalf("exit = %d, stderr:\n%s", code, stderr)
	}
	for _, want := range []string{"apps create --name", "databases create --name", "apps connect", "apps domains add", "apps scale", "(detected)", "(default)"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("dry run output missing %q:\n%s", want, stderr)
		}
	}
	if stdout != "" && !strings.Contains(stdout, "dry_run") && !strings.Contains(stdout, "App") {
		t.Errorf("unexpected stdout %q", stdout)
	}
	for key, n := range f.calls {
		if strings.HasPrefix(key, "POST") || strings.HasPrefix(key, "PUT") || strings.HasPrefix(key, "PATCH") {
			t.Errorf("dry run sent %s x%d", key, n)
		}
	}
}

func TestUpConnectFailureIsReportedBeforeBuild(t *testing.T) {
	f, srv := newFakeUp(t)
	f.failConnect = true
	dir := newUpProject(t)
	code, _, stderr := runUpCLI(t, srv, dir, "--with", "postgres")
	if code == exitOK {
		t.Fatal("expected a failure")
	}
	if !strings.Contains(stderr, "apps connect") || !strings.Contains(stderr, "no build was started") {
		t.Errorf("stderr lacks a fix hint:\n%s", stderr)
	}
	if n := f.calls["POST /api/v1/apps/"+dirName(dir)+"/builds"]; n != 0 {
		t.Errorf("build started %d times despite the failed connect", n)
	}
	f.failConnect = false
	if code, _, e := runUpCLI(t, srv, dir, "--with", "postgres"); code != exitOK {
		t.Fatalf("retry failed: %s", e)
	}
	if f.calls["POST /api/v1/apps"] != 1 || f.calls["POST /api/v1/databases"] != 1 || len(f.conns) != 1 {
		t.Errorf("retry duplicated work: %v conns=%v", f.calls, f.conns)
	}
}

func TestAppsCreateDefaults(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"package.json": `{"scripts":{"start":"x"}}`})
	tests := []struct {
		name    string
		in      createFlags
		detect  detectedGit
		remote  func(string, string) (int, bool)
		want    createFlags
		wantErr string
	}{
		{
			name:   "node repo without Dockerfile gets railpack, name, port and image repo",
			detect: detectedGit{RepoURL: "git@github.com:acme/My_Shop.git"},
			want:   createFlags{name: "my-shop", imageRepo: "my-shop", buildType: "railpack", port: 3000},
		},
		{
			name:   "explicit flags win",
			in:     createFlags{name: "x", port: 1, buildType: "dockerfile", imageRepo: "r/x"},
			detect: detectedGit{RepoURL: "https://h/o/n.git"},
			want:   createFlags{name: "x", imageRepo: "r/x", buildType: "dockerfile", port: 1},
		},
		{
			name:   "remote repo takes its port from the server scan",
			in:     createFlags{repo: "https://h/o/other.git"},
			remote: func(string, string) (int, bool) { return 5000, true },
			want:   createFlags{repo: "https://h/o/other.git", name: "other", imageRepo: "other", port: 5000},
		},
		{
			name: "image path untouched",
			in:   createFlags{image: "x:1"},
			want: createFlags{image: "x:1"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.in
			applyGitBuildDefaults(&f, tc.detect, root, tc.remote)
			f.buildArgs, tc.want.buildArgs = nil, nil
			if f.name != tc.want.name || f.port != tc.want.port || f.buildType != tc.want.buildType || f.imageRepo != tc.want.imageRepo || f.repo != tc.want.repo || f.image != tc.want.image {
				t.Errorf("got %+v, want %+v", f, tc.want)
			}
		})
	}
}

func TestRepoBaseName(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/acme/Shop.git": "shop", "git@github.com:acme/my_app": "my-app", "https://h/o/n/": "n", "https://h/o/1abc": "",
	} {
		if got := repoBaseName(in); got != want {
			t.Errorf("repoBaseName(%q) = %q, want %q", in, got, want)
		}
	}
}
