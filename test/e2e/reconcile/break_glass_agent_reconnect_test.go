// Package e2e: the node-agent half of docs/resilience.md, alongside
// break_glass_control_plane_death_test.go's control-plane half. A real
// levelrail-agent process, enrolled against a real control plane, with a
// real container placed on it through node placement
// (cmd/levelrail/main.go's resolveNodeTransport). This proves the agent
// never takes a destructive local action just because its connection to
// the control plane drops, keeps retrying with backoff
// (cmd/levelrail-agent's own runReconnectLoop), logs the disconnect
// clearly, and picks the connection back up once the control plane
// returns, without disturbing the container it already placed.
package reconcile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/test/e2e/testenv"
)

func waitNodeStatus(t *testing.T, client *apiclient.Client, nodeID, want string, timeout time.Duration) {
	t.Helper()
	pollUntil(t, timeout, fmt.Sprintf("node %s to reach status %q", nodeID, want), func() (bool, string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		nodes, err := client.ListNodes(ctx)
		if err != nil {
			return false, err.Error()
		}
		for _, n := range nodes {
			if n.ID == nodeID {
				if n.Status == want {
					return true, ""
				}
				return false, "status " + n.Status
			}
		}
		return false, "node not listed yet"
	})
}

func TestBreakGlass_Live_AgentSurvivesControlPlaneDeath(t *testing.T) {
	testenv.RequireFullLive(t)
	env := newLiveBuildEnv(t)

	const serviceName = "levelrail-breakglass-agent"
	tag := "levelrail/breakglass-agent:1"

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = env.DockerCli.ImageRemove(ctx, tag, image.RemoveOptions{Force: true})
	})
	cleanupContainers(context.Background(), t, env.Runtime, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, env.Runtime, serviceName) })

	buildCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	res, err := env.BuildClient.Build(buildCtx, build.Request{ContextDir: "../../fixtures/hello-e2e", Tag: tag}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	repoRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	binDir := t.TempDir()
	cpBin := goBuild(t, repoRoot, binDir, "./cmd/levelrail")
	agentBin := goBuild(t, repoRoot, binDir, "./cmd/levelrail-agent")

	workDir := t.TempDir()
	dataDir := filepath.Join(workDir, "data")
	homeDir := filepath.Join(workDir, "home")
	httpAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	agentAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	apiURL := "http://" + httpAddr

	cpEnv := envWithOverrides(hermeticEnv(
		"APP_DEV_MODE=1",
		"APP_BRAND_FILE="+filepath.Join(repoRoot, "brand.yaml"),
		"APP_DEV_FIXTURES_FILE="+filepath.Join(repoRoot, "dev-fixtures.yml"),
		"APP_DATA_DIR="+dataDir,
		"APP_HTTP_ADDR="+httpAddr,
		"APP_AGENT_ADDR="+agentAddr,
		"APP_INGRESS_HTTP_ADDR=127.0.0.1:0",
		"APP_INGRESS_HTTPS_ADDR=127.0.0.1:0",
	),
		"HOME="+homeDir,
		"XDG_DATA_HOME="+filepath.Join(homeDir, ".local", "share"),
		"XDG_CONFIG_HOME="+filepath.Join(homeDir, ".config"),
	)

	proc1 := startBreakGlassProcess(t, cpBin, cpEnv, filepath.Join(workDir, "cp1.log"))
	client := apiclient.NewClient(apiURL, breakGlassToken)
	waitAPIUp(t, client, proc1.exited)

	tokenCtx, cancelToken := context.WithTimeout(context.Background(), 10*time.Second)
	joinToken, err := client.CreateNodeJoinToken(tokenCtx)
	cancelToken()
	if err != nil {
		t.Fatalf("CreateNodeJoinToken() error = %v", err)
	}

	const nodeName = "breakglass-agent-node"
	agentEnv := envWithOverrides(hermeticEnv(
		"APP_CONTROL_PLANE_ADDR="+agentAddr,
		"APP_JOIN_TOKEN="+joinToken.Token,
		"APP_NODE_NAME="+nodeName,
		"APP_AGENT_IDENTITY_FILE="+filepath.Join(workDir, "agent-identity.json"),
		// Accelerates the node's Online transition below: the control
		// plane's own default (15s, cmd/levelrail's defaultNodeHeartbeatInterval)
		// would work too, just slower to observe in a test.
		"APP_NODE_HEARTBEAT_INTERVAL=2s",
	),
		"HOME="+filepath.Join(workDir, "agent-home"),
	)
	agentProc := startBreakGlassProcess(t, agentBin, agentEnv, filepath.Join(workDir, "agent1.log"))

	var nodeID string
	pollUntil(t, 30*time.Second, "enrolled node to appear", func() (bool, string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		nodes, err := client.ListNodes(ctx)
		if err != nil {
			return false, err.Error()
		}
		for _, n := range nodes {
			if n.Name == nodeName {
				nodeID = n.ID
				return true, ""
			}
		}
		return false, fmt.Sprintf("nodes %+v", nodes)
	})
	waitNodeStatus(t, client, nodeID, "online", 30*time.Second)

	createCtx, cancelCreate := context.WithTimeout(context.Background(), 30*time.Second)
	_, err = client.CreateApp(createCtx, apiclient.AppResource{
		Name:   serviceName,
		Image:  res.Tag,
		Port:   8080,
		NodeID: nodeID,
		Health: &apiclient.ServiceHealth{Readiness: &apiclient.ServiceProbe{Path: "/"}},
	})
	cancelCreate()
	if err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}
	waitAppReady(t, client, serviceName)

	before := breakGlassInspect(t, env, serviceName)
	if !before.State.Running {
		t.Fatalf("container not running before kill: %+v", before.State)
	}

	// Kill the control plane: the agent's own gRPC connection to it is
	// severed the same way a real network partition or a crashed control
	// plane would sever it.
	proc1.kill(t)

	// The agent must not do anything destructive just because it lost
	// the connection: the container it placed stays exactly as it was.
	stillRunning := breakGlassInspect(t, env, serviceName)
	if !stillRunning.State.Running {
		t.Fatalf("container not running after control plane death: %+v", stillRunning.State)
	}
	if stillRunning.RestartCount != before.RestartCount {
		t.Fatalf("container RestartCount changed (%d -> %d) just because the control plane died", before.RestartCount, stillRunning.RestartCount)
	}

	// The agent process itself must not have exited: it retries with
	// backoff and never gives up on its own
	// (cmd/levelrail-agent's runReconnectLoop doc comment).
	select {
	case <-agentProc.exited:
		t.Fatal("agent process exited when the control plane died, want it to keep retrying")
	default:
	}

	// It must be logging the disconnect clearly, not silently spinning:
	// runReconnectLoop's own "session ended, reconnecting" warning.
	pollUntil(t, 20*time.Second, "agent log to show a reconnect attempt", func() (bool, string) {
		b, err := os.ReadFile(agentProc.logPath) //nolint:gosec // path under t.TempDir
		if err != nil {
			return false, err.Error()
		}
		if strings.Contains(string(b), "reconnecting") {
			return true, ""
		}
		return false, "no reconnect message yet"
	})

	// Restart the control plane on the exact same address and data dir:
	// the agent's own backoff loop should find it again on its own, no
	// agent restart needed.
	proc2 := startBreakGlassProcess(t, cpBin, cpEnv, filepath.Join(workDir, "cp2.log"))
	client2 := apiclient.NewClient(apiURL, breakGlassToken)
	waitAPIUp(t, client2, proc2.exited)
	waitNodeStatus(t, client2, nodeID, "online", 60*time.Second)

	// The reconnected control plane reconciles the placed app back to
	// Ready without recreating or restarting its already-running
	// container.
	waitAppReady(t, client2, serviceName)
	final := breakGlassInspect(t, env, serviceName)
	if final.ID != before.ID {
		t.Fatalf("container was recreated across control plane restart: before id=%s after id=%s", before.ID, final.ID)
	}
	if final.RestartCount != before.RestartCount {
		t.Fatalf("container RestartCount = %d, want unchanged from %d: it must never be restarted just because the control plane came back", final.RestartCount, before.RestartCount)
	}
}
