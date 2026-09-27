package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// agentLoopGreeting is the value the scripted agent supplies for a missing
// env var; asserting it is served proves the fixed container is the one live.
const agentLoopGreeting = "hello from the agent loop"

var (
	missingDistRe = regexp.MustCompile(`No matching distribution found for ([A-Za-z0-9_.-]+)==(\S+) \(available: ([^)]*)\)`)
	missingEnvRe  = regexp.MustCompile(`missing required env ([A-Z_][A-Z0-9_]*)`)
)

// TestAgentLoop_Live drives the deploy, fail, read, fix, redeploy loop an
// agent runs, against a real control plane, real Docker and a real stdio MCP
// server on the agent-core profile. The agent is scripted: every repair it
// makes is derived from the structured failure the MCP server returns.
func TestAgentLoop_Live(t *testing.T) {
	suffix := randomSuffix(t)
	buildApp := "agent-loop-build-" + suffix
	runtimeApp := "agent-loop-runtime-" + suffix
	env := newAgentLoopEnv(t, buildApp, runtimeApp)

	tools := env.toolNames(t)
	for _, name := range []string{"diagnose_app_failure", "set_app_env", "deploy_app", "get_app_status", "list_apps"} {
		if !tools[name] {
			t.Fatalf("agent-core profile does not expose %s; tools: %v", name, tools)
		}
	}
	if tools["list_failed_deploys"] {
		t.Fatalf("list_failed_deploys is exposed, so the agent-core profile is not active")
	}

	t.Run("build_error_bad_dependency", func(t *testing.T) {
		t.Parallel()
		env.runBuildErrorLoop(t, buildApp)
	})
	t.Run("runtime_error_missing_env", func(t *testing.T) {
		t.Parallel()
		env.runMissingEnvLoop(t, runtimeApp)
	})
}

func (e *agentLoopEnv) runBuildErrorLoop(t *testing.T, app string) {
	dir, url, sha := e.fixtureRepo(t, "agent-loop-build", app)
	e.createApp(t, app)

	first := e.triggerBuild(t, app, url, sha)
	if got := e.waitAttempt(t, app, first); got.Status != "failed" {
		t.Fatalf("first deploy status = %q, want failed (the fixture pins a missing dependency)", got.Status)
	}

	f := e.diagnose(t, app, first)
	if f.Code != "dependency_install_failed" {
		t.Fatalf("failure code = %q, want dependency_install_failed; failure: %+v", f.Code, f)
	}
	if f.DeployID != first || f.App != app {
		t.Fatalf("failure names deploy %q app %q, want %q %q", f.DeployID, f.App, first, app)
	}

	pkg, bad, good := pinFixFor(t, f)
	reqPath := filepath.Join(dir, "requirements.txt")
	if err := os.WriteFile(reqPath, []byte(pkg+"=="+good+"\n"), 0o600); err != nil {
		t.Fatalf("rewrite %s: %v", reqPath, err)
	}
	fixed := commitAll(t, dir, fmt.Sprintf("pin %s %s instead of missing %s", pkg, good, bad))

	second := e.triggerBuild(t, app, url, fixed)
	if got := e.waitAttempt(t, app, second); got.Status != "succeeded" {
		t.Fatalf("redeploy status = %q, want succeeded; error: %s", got.Status, got.Error)
	}
	e.waitServing(t, app, pkg+" "+good+" says hello")
}

func (e *agentLoopEnv) runMissingEnvLoop(t *testing.T, app string) {
	_, url, sha := e.fixtureRepo(t, "agent-loop-runtime", app)
	e.createApp(t, app)

	id := e.triggerBuild(t, app, url, sha)
	if got := e.waitAttempt(t, app, id); got.Status != "succeeded" {
		t.Fatalf("build status = %q, want succeeded (the fixture only fails at runtime); error: %s", got.Status, got.Error)
	}

	// The build succeeds; the failure only appears once the rollout hits the
	// crashing container and its log line reaches the log store.
	var f apiclient.DeployFailure
	pollUntil(t, 3*time.Minute, "a missing_env failure for "+app, func() (bool, string) {
		f = e.diagnose(t, app, "")
		if f.Code != "missing_env" {
			return false, fmt.Sprintf("failure %+v", f)
		}
		return true, ""
	})

	key := envFixFor(t, f)
	var changed struct {
		Key            string `json:"key"`
		RedeployNeeded bool   `json:"redeploy_needed"`
	}
	e.callTool(t, "set_app_env", map[string]any{"name": app, "key": key, "value": agentLoopGreeting}, &changed)
	if changed.Key != key || !changed.RedeployNeeded {
		t.Fatalf("set_app_env result = %+v, want key %q with redeploy_needed", changed, key)
	}

	var apps []apiclient.AppResource
	e.callTool(t, "list_apps", nil, &apps)
	image := ""
	for _, a := range apps {
		if a.Name == app {
			image = a.Image
		}
	}
	if image == "" {
		t.Fatalf("list_apps did not return %s: %+v", app, apps)
	}
	e.callTool(t, "deploy_app", map[string]any{"name": app, "image": image}, nil)
	e.waitServing(t, app, agentLoopGreeting)
}

// diagnose reads the structured failure through the MCP server, failing the
// test when the response carries none. An empty deployID means the newest.
func (e *agentLoopEnv) diagnose(t *testing.T, app, deployID string) apiclient.DeployFailure {
	t.Helper()
	args := map[string]any{"name": app}
	if deployID != "" {
		args["deploy_id"] = deployID
	}
	var d apiclient.DiagnosisResource
	e.callTool(t, "diagnose_app_failure", args, &d)
	if d.Failure == nil {
		return apiclient.DeployFailure{}
	}
	return *d.Failure
}

// pinFixFor is the scripted agent's repair for a failed dependency install:
// the class tells it to fix the version pin, and the log excerpt names the
// package, the missing version and the versions that exist.
func pinFixFor(t *testing.T, f apiclient.DeployFailure) (pkg, bad, good string) {
	t.Helper()
	if !strings.Contains(f.SuggestedFix, "version pin") {
		t.Fatalf("suggested_fix %q no longer points at the version pin the scripted agent rewrites", f.SuggestedFix)
	}
	m := missingDistRe.FindStringSubmatch(f.LogExcerpt)
	if m == nil {
		t.Fatalf("log excerpt does not name the missing dependency:\n%s", f.LogExcerpt)
	}
	good = newestVersion(strings.Fields(m[3]))
	if good == "" {
		t.Fatalf("log excerpt lists no available version for %s: %q", m[1], m[3])
	}
	return m[1], m[2], good
}

// envFixFor is the scripted agent's repair for missing_env: the class says the
// error names the variable, so it reads the name from the log excerpt.
func envFixFor(t *testing.T, f apiclient.DeployFailure) string {
	t.Helper()
	if !strings.Contains(f.SuggestedFix, "Set the missing variable") {
		t.Fatalf("suggested_fix %q no longer tells the agent to set the variable", f.SuggestedFix)
	}
	m := missingEnvRe.FindStringSubmatch(f.LogExcerpt)
	if m == nil {
		t.Fatalf("log excerpt does not name the missing variable:\n%s", f.LogExcerpt)
	}
	return m[1]
}

// newestVersion returns the highest dotted numeric version in vs.
func newestVersion(vs []string) string {
	best, bestParts := "", []int(nil)
	for _, v := range vs {
		parts, ok := versionParts(v)
		if !ok {
			continue
		}
		if bestParts == nil || versionLess(bestParts, parts) {
			best, bestParts = v, parts
		}
	}
	return best
}

func versionParts(v string) ([]int, bool) {
	fields := strings.Split(v, ".")
	out := make([]int, len(fields))
	for i, s := range fields {
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, false
		}
		out[i] = n
	}
	return out, true
}

func versionLess(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

func TestNewestVersion(t *testing.T) {
	tests := []struct {
		in   []string
		want string
	}{
		{[]string{"1.0.0", "1.1.0"}, "1.1.0"},
		{[]string{"1.10.0", "1.9.0"}, "1.10.0"},
		{[]string{"2", "1.9.9"}, "2"},
		{[]string{"x", "0.1"}, "0.1"},
		{nil, ""},
	}
	for _, tt := range tests {
		if got := newestVersion(tt.in); got != tt.want {
			t.Errorf("newestVersion(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
