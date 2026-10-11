package appwizard

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"

	"github.com/GLINCKER/levelrail/internal/build"
)

// dirSource serves a directory as a repository file tree, standing in for a
// provider API.
type dirSource struct{ root string }

func (d dirSource) Paths(context.Context) ([]string, bool, error) {
	var out []string
	err := filepath.WalkDir(d.root, func(p string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		rel, rerr := filepath.Rel(d.root, p)
		if rerr != nil {
			return rerr
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out, false, err
}

func (d dirSource) Read(_ context.Context, p string, limit int) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(d.root, filepath.FromSlash(p))) //nolint:gosec // test fixture path
	if err != nil {
		return nil, err
	}
	if len(b) > limit {
		b = b[:limit]
	}
	return b, nil
}

const fixtureServer = `const http = require('http')
http.createServer((req, res) => {
  if (req.url === '/healthz') { res.end('ok'); return }
  res.end('hello')
}).listen(4100)
`

// TestLive_WizardAnalysisBuildsAndRunsFixture analyses a turborepo-shaped
// Node fixture, then builds and runs it with exactly the build settings,
// port and health path the analysis suggested, against a real Docker daemon.
func TestLive_WizardAnalysisBuildsAndRunsFixture(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live Docker test in -short mode")
	}
	docker, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client: %v", err)
	}
	t.Cleanup(func() { _ = docker.Close() })
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 3*time.Second)
	_, err = docker.Ping(pingCtx)
	cancelPing()
	if err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}
	connectCtx, cancelConnect := context.WithTimeout(context.Background(), 10*time.Second)
	bk, err := build.NewClient(connectCtx, docker)
	cancelConnect()
	if err != nil {
		t.Skipf("no buildkit via docker: %v", err)
	}
	t.Cleanup(func() { _ = bk.Close() })

	root := t.TempDir()
	files := map[string]string{
		"turbo.json":                   "{}",
		"package.json":                 `{"private":true,"workspaces":["apps/*"]}`,
		"apps/web/package.json":        `{"name":"web","scripts":{"start":"node server.js"},"dependencies":{"express":"4","pg":"8"}}`,
		"apps/web/server.js":           fixtureServer,
		"apps/web/.env.example":        "DATABASE_URL=\nSESSION_SECRET=changeme\n",
		"apps/web/Dockerfile":          "FROM node:22-alpine\nWORKDIR /app\nCOPY server.js .\nEXPOSE 4100\nHEALTHCHECK CMD wget -qO- http://127.0.0.1:4100/healthz || exit 1\nCMD [\"node\",\"server.js\"]\n",
		"apps/docs/package.json":       `{"name":"docs"}`,
		"packages/shared/package.json": `{"name":"shared"}`,
	}
	for p, body := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	a, err := Analyze(context.Background(), dirSource{root: root}, Options{
		RepoURL: "https://example.com/acme/mono", Branch: "main",
		DefaultVersion: func(string) string { return "16" },
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.Build.Type != "dockerfile" || a.Build.DockerfilePath != "apps/web/Dockerfile" || a.AppDirectory != "apps/web" {
		t.Fatalf("build = %+v dir = %q", a.Build, a.AppDirectory)
	}
	if a.Port.Value != 4100 || a.Health.Value != "/healthz" || len(a.Databases) != 1 {
		t.Fatalf("port = %+v health = %+v dbs = %+v", a.Port, a.Health, a.Databases)
	}

	tag := "levelrail-wizard-live:test"
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	t.Cleanup(func() {
		c, done := context.WithTimeout(context.Background(), 20*time.Second)
		defer done()
		_, _ = docker.ImageRemove(c, tag, image.RemoveOptions{Force: true})
	})
	contextDir := root
	if a.Build.BaseDirectory != "" {
		contextDir = filepath.Join(root, filepath.FromSlash(a.Build.BaseDirectory))
	}
	if _, err := bk.Build(ctx, build.Request{
		ContextDir:     contextDir,
		DockerfilePath: filepath.Join(root, filepath.FromSlash(a.Build.DockerfilePath)),
		Tag:            tag,
	}, func(build.ProgressEvent) {}); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	port := nat.Port(fmt.Sprintf("%d/tcp", a.Port.Value))
	created, err := docker.ContainerCreate(ctx, &container.Config{
		Image: tag, ExposedPorts: nat.PortSet{port: {}},
	}, &container.HostConfig{
		PortBindings: nat.PortMap{port: {{HostIP: "127.0.0.1", HostPort: ""}}},
	}, nil, nil, "")
	if err != nil {
		t.Fatalf("ContainerCreate() error = %v", err)
	}
	t.Cleanup(func() {
		c, done := context.WithTimeout(context.Background(), 20*time.Second)
		defer done()
		_ = docker.ContainerRemove(c, created.ID, container.RemoveOptions{Force: true})
	})
	if err := docker.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		t.Fatalf("ContainerStart() error = %v", err)
	}
	info, err := docker.ContainerInspect(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	bindings := info.NetworkSettings.Ports[port]
	if len(bindings) == 0 {
		t.Fatalf("no host port bound for %s", port)
	}
	target := "http://127.0.0.1:" + bindings[0].HostPort + a.Health.Value

	var body string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, gerr := http.Get(target) //nolint:gosec,noctx // loopback fixture
		if gerr == nil {
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			body = strings.TrimSpace(string(b))
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if body != "ok" {
		t.Fatalf("health probe at %s returned %q, want ok", target, body)
	}
}
