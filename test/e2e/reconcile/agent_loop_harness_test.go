package reconcile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cgi" //nolint:gosec // httpoxy only affects Go < 1.6.3; this serves local test repos
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	agentLoopToken    = "dev-root-token" //nolint:gosec // dev-mode fixture token from dev-fixtures.yml
	agentLoopImageNS  = "levelrail-e2e-agent-loop"
	agentLoopPollTick = 250 * time.Millisecond
)

// agentLoopEnv is one real control plane process, a real stdio MCP server
// process on the agent-core profile, and a smart-HTTP git server for fixtures.
type agentLoopEnv struct {
	live    liveBuildEnv
	api     *apiclient.Client
	mcp     *mcp.ClientSession
	gitRoot string
	gitURL  string
}

// newAgentLoopEnv starts the stack; apps names the apps the test will create
// so their Docker leftovers are removed after the control plane stops.
func newAgentLoopEnv(t *testing.T, apps ...string) *agentLoopEnv {
	t.Helper()
	live := newLiveBuildEnv(t)
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("git not on PATH, needed to serve fixture repos: %v", err)
	}
	cleanupDocker(t, live, apps...)

	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	binDir := t.TempDir()
	cpBin := goBuild(t, repoRoot, binDir, "./cmd/levelrail")
	mcpBin := goBuild(t, repoRoot, binDir, "./cmd/levelrail-mcp")

	apiURL := startControlPlane(t, repoRoot, cpBin)
	env := &agentLoopEnv{
		live: live,
		api:  apiclient.NewClient(apiURL, agentLoopToken),
		mcp:  startMCP(t, mcpBin, apiURL),
	}
	env.gitRoot = t.TempDir()
	env.gitURL = serveGit(t, gitBin, env.gitRoot)
	return env
}

func goBuild(t *testing.T, repoRoot, outDir, pkg string) string {
	t.Helper()
	out := filepath.Join(outDir, filepath.Base(pkg))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", out, pkg) //nolint:gosec // fixed package paths from this test
	cmd.Dir = repoRoot
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, b)
	}
	return out
}

// hermeticEnv drops every APP_ variable so the developer's own shell config
// cannot leak into the processes under test.
func hermeticEnv(extra ...string) []string {
	var out []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "APP_") {
			out = append(out, kv)
		}
	}
	return append(out, extra...)
}

func startControlPlane(t *testing.T, repoRoot, bin string) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "server.log")
	logFile, err := os.Create(logPath) //nolint:gosec // path under t.TempDir
	if err != nil {
		t.Fatalf("create server log: %v", err)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	cmd := exec.Command(bin) //nolint:gosec,noctx // binary built by this test; lifetime is managed by the cleanup below
	cmd.Env = hermeticEnv(
		"APP_DEV_MODE=1",
		"APP_BRAND_FILE="+filepath.Join(repoRoot, "brand.yaml"),
		"APP_DEV_FIXTURES_FILE="+filepath.Join(repoRoot, "dev-fixtures.yml"),
		"APP_DATA_DIR="+filepath.Join(dir, "data"),
		"APP_HTTP_ADDR="+addr,
		"APP_AGENT_ADDR=127.0.0.1:0",
		"APP_INGRESS_HTTP_ADDR=127.0.0.1:0",
		"APP_INGRESS_HTTPS_ADDR=127.0.0.1:0",
	)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		t.Fatalf("start control plane: %v", err)
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-exited:
		case <-time.After(20 * time.Second):
			_ = cmd.Process.Kill()
			<-exited
		}
		_ = logFile.Close()
		if t.Failed() {
			dumpTail(t, "control plane log", logPath, 60)
		}
	})

	apiURL := "http://" + addr
	client := apiclient.NewClient(apiURL, agentLoopToken)
	pollUntil(t, 60*time.Second, "control plane to answer /api/v1/apps", func() (bool, string) {
		select {
		case <-exited:
			t.Fatalf("control plane exited during startup")
		default:
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err := client.ListApps(ctx)
		if err != nil {
			return false, err.Error()
		}
		return true, ""
	})
	return apiURL
}

func dumpTail(t *testing.T, label, path string, lines int) {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // path under t.TempDir
	if err != nil {
		return
	}
	all := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	t.Logf("%s (last %d lines):\n%s", label, len(all), strings.Join(all, "\n"))
}

func startMCP(t *testing.T, bin, apiURL string) *mcp.ClientSession {
	t.Helper()
	cmd := exec.Command(bin) //nolint:gosec,noctx // binary built by this test; the transport owns its lifetime
	cmd.Env = hermeticEnv(
		"APP_API_URL="+apiURL,
		"APP_API_TOKEN="+agentLoopToken,
		"APP_MCP_TOOL_PROFILE=agent-core",
		"HOME="+t.TempDir(),
	)
	client := mcp.NewClient(&mcp.Implementation{Name: "agent-loop-e2e", Version: "test"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect to MCP server: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// callTool calls an MCP tool and decodes its structured result into out.
func (e *agentLoopEnv) callTool(t *testing.T, name string, args map[string]any, out any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := e.mcp.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("MCP %s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("MCP %s returned a tool error: %s", name, toolText(res))
	}
	if out == nil {
		return
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("MCP %s: marshal structured content: %v", name, err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("MCP %s: decode structured content %s: %v", name, b, err)
	}
}

func toolText(res *mcp.CallToolResult) string {
	var parts []string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func (e *agentLoopEnv) toolNames(t *testing.T) map[string]bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := e.mcp.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("MCP tools/list: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	return names
}

// serveGit serves every repo under root over git's smart HTTP protocol, the
// only one the control plane's go-git clone speaks.
func serveGit(t *testing.T, gitBin, root string) string {
	t.Helper()
	ts := httptest.NewServer(&cgi.Handler{
		Path: gitBin,
		Args: []string{"http-backend"},
		Env: []string{
			"GIT_PROJECT_ROOT=" + root,
			"GIT_HTTP_EXPORT_ALL=1",
			"GIT_CONFIG_NOSYSTEM=1",
			"HOME=" + root,
		},
	})
	t.Cleanup(ts.Close)
	return ts.URL
}

// fixtureRepo copies a fixture directory into a fresh git repo under the git
// server root and commits it.
func (e *agentLoopEnv) fixtureRepo(t *testing.T, fixture, name string) (dir, url, sha string) {
	t.Helper()
	dir = filepath.Join(e.gitRoot, name)
	src := filepath.Join("..", "fixtures", fixture)
	if err := os.CopyFS(dir, os.DirFS(src)); err != nil {
		t.Fatalf("copy fixture %s: %v", fixture, err)
	}
	if _, err := git.PlainInit(dir, false); err != nil {
		t.Fatalf("git init %s: %v", dir, err)
	}
	return dir, e.gitURL + "/" + name, commitAll(t, dir, "initial fixture")
}

func commitAll(t *testing.T, dir, msg string) string {
	t.Helper()
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("git open %s: %v", dir, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("git worktree %s: %v", dir, err)
	}
	if err := wt.AddGlob("."); err != nil {
		t.Fatalf("git add %s: %v", dir, err)
	}
	hash, err := wt.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{Name: "agent-loop", Email: "agent-loop@example.invalid", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("git commit %s: %v", dir, err)
	}
	return hash.String()
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("random suffix: %v", err)
	}
	return hex.EncodeToString(b)
}

// createApp creates app with a pending image and an HTTP readiness probe.
func (e *agentLoopEnv) createApp(t *testing.T, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := e.api.CreateApp(ctx, apiclient.AppResource{
		Name:  name,
		Image: agentLoopImageNS + "/" + name + ":pending",
		Port:  8080,
		Health: &apiclient.ServiceHealth{
			Readiness: &apiclient.ServiceProbe{Path: "/"},
		},
	})
	if err != nil {
		t.Fatalf("create app %s: %v", name, err)
	}
}

// cleanupDocker removes every container and image the control plane made for
// name. Registered before the control plane starts so it runs after it stops.
func cleanupDocker(t *testing.T, live liveBuildEnv, names ...string) {
	t.Helper()
	sweep := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		for _, name := range names {
			cleanupContainers(ctx, t, live.Runtime, name)
			imgs, err := live.DockerCli.ImageList(ctx, image.ListOptions{
				Filters: filters.NewArgs(filters.Arg("reference", agentLoopImageNS+"/"+name)),
			})
			if err != nil {
				continue
			}
			for _, img := range imgs {
				_, _ = live.DockerCli.ImageRemove(ctx, img.ID, image.RemoveOptions{Force: true, PruneChildren: true})
			}
			// Leaked per-app networks exhaust Docker's address pools after a
			// few dozen runs.
			nets, err := live.DockerCli.NetworkList(ctx, network.ListOptions{
				Filters: filters.NewArgs(filters.Arg("name", name)),
			})
			if err != nil {
				continue
			}
			for _, n := range nets {
				if strings.HasSuffix(n.Name, "-app-"+name) {
					_ = live.DockerCli.NetworkRemove(ctx, n.ID)
				}
			}
		}
	}
	sweep()
	t.Cleanup(func() {
		if t.Failed() {
			logContainers(t, live, names...)
		}
		sweep()
	})
}

func logContainers(t *testing.T, live liveBuildEnv, names ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, name := range names {
		list, err := live.DockerCli.ContainerList(ctx, container.ListOptions{
			All:     true,
			Filters: filters.NewArgs(filters.Arg("name", name)),
		})
		if err != nil {
			continue
		}
		for _, c := range list {
			t.Logf("container %v image=%s state=%s status=%q ports=%+v", c.Names, c.Image, c.State, c.Status, c.Ports)
		}
	}
}

// triggerBuild starts a build of url at sha and returns the deploy attempt id.
func (e *agentLoopEnv) triggerBuild(t *testing.T, app, url, sha string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := e.api.TriggerBuild(ctx, app, apiclient.BuildTriggerRequest{
		RepoURL:   url,
		Ref:       sha,
		ImageRepo: agentLoopImageNS + "/" + app,
	})
	if err != nil {
		t.Fatalf("trigger build of %s at %s: %v", app, sha, err)
	}
	if res.ID == "" {
		t.Fatalf("trigger build of %s returned no deploy attempt id: %+v", app, res)
	}
	return res.ID
}

// waitAttempt polls until the deploy attempt reaches a terminal status.
func (e *agentLoopEnv) waitAttempt(t *testing.T, app, id string) apiclient.DeployAttemptResource {
	t.Helper()
	var got apiclient.DeployAttemptResource
	pollUntil(t, 5*time.Minute, "deploy attempt "+id+" to finish", func() (bool, string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		attempts, err := e.api.ListDeployAttempts(ctx, app)
		if err != nil {
			return false, err.Error()
		}
		for _, a := range attempts {
			if a.ID != id {
				continue
			}
			got = a
			switch a.Status {
			case "succeeded", "failed", "canceled", "superseded":
				return true, ""
			}
			return false, "status " + a.Status
		}
		return false, "attempt not listed yet"
	})
	return got
}

// waitServing polls MCP get_app_status for a True Ready condition and the
// app's published host port for a body containing want.
func (e *agentLoopEnv) waitServing(t *testing.T, app, want string) {
	t.Helper()
	httpClient := &http.Client{Timeout: 3 * time.Second}
	pollUntil(t, 3*time.Minute, app+" to be Ready and serve "+want, func() (bool, string) {
		var conds []apiclient.ConditionResource
		e.callTool(t, "get_app_status", map[string]any{"name": app}, &conds)
		ready := false
		for _, c := range conds {
			if c.Type == "Ready" && c.Status == "True" {
				ready = true
			}
		}
		if !ready {
			return false, fmt.Sprintf("conditions %+v", conds)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		netw, err := e.api.GetAppNetwork(ctx, app)
		if err != nil || !netw.Running || netw.HostPort == 0 {
			return false, fmt.Sprintf("network %+v err %v", netw, err)
		}
		url := fmt.Sprintf("http://127.0.0.1:%d/", netw.HostPort)
		body, status, err := doGet(httpClient, url)
		if err != nil {
			return false, err.Error()
		}
		if status != http.StatusOK || !strings.Contains(body, want) {
			return false, fmt.Sprintf("GET %s = %d %q", url, status, body)
		}
		return true, ""
	})
}

// pollUntil calls check on a fixed tick until it reports done or the deadline
// passes, then fails with the last reason check gave.
func pollUntil(t *testing.T, timeout time.Duration, what string, check func() (bool, string)) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(agentLoopPollTick)
	defer tick.Stop()
	start := time.Now()
	last := ""
	for {
		done, reason := check()
		if done {
			t.Logf("%s after %s", what, time.Since(start).Round(time.Second))
			return
		}
		last = reason
		select {
		case <-deadline.C:
			t.Fatalf("timed out after %s waiting for %s, last: %s", timeout, what, last)
		case <-tick.C:
		}
	}
}
