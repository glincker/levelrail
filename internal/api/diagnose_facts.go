package api

import (
	"context"
	"io"
	"log/slog"
	"os"
	goruntime "runtime"
	"sort"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/diagnose"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envDiagnoseOOMFactor   = "APP_DIAGNOSE_OOM_FACTOR"
	envDiagnoseProbeBudget = "APP_DIAGNOSE_PROBE_TIMEOUT"
	defaultProbeBudget     = 4 * time.Second
	maxProcNetBytes        = 256 << 10
)

// diagnoseFacts gathers structured observations for the typed rules. Every
// probe is best effort: a missing runtime or a stopped container just leaves
// that fact unknown.
func (rt *Router) diagnoseFacts(ctx context.Context, svc *store.DesiredService, buildFailed bool) diagnose.Facts {
	f := diagnose.Facts{
		Port:        svc.Port,
		OOMFactor:   envFloatOr(envDiagnoseOOMFactor, 0),
		EnvKeys:     configuredEnvKeys(svc),
		BuildFailed: buildFailed,
	}
	if svc.Health != nil && svc.Health.Readiness != nil {
		f.HealthPath = svc.Health.Readiness.Path
	}
	if svc.Resources != nil {
		f.MemoryBytes = svc.Resources.MemoryBytes
	}
	if rt.isLocalNode(svc.NodeID) {
		f.NodeArch = goruntime.GOARCH
	}
	if rt.execRuntime == nil || buildFailed {
		return f
	}
	runtime, err := rt.execRuntime(svc.NodeID)
	if err != nil {
		rt.logger.Warn("api: diagnose facts: resolve node runtime failed", slog.String("error", err.Error()), slog.String("name", svc.Name))
		return f
	}
	pctx, cancel := context.WithTimeout(ctx, envDurationOr(envDiagnoseProbeBudget, defaultProbeBudget))
	defer cancel()
	container := application.ContainerName(svc.Name, svc.Image, svc.RestartNonce)

	running := false
	if ins, ok := runtime.(docker.ExitStateInspector); ok {
		if st, ierr := ins.InspectExitState(pctx, container); ierr == nil && st != nil {
			running = st.Running
			f.OOMKilled = st.OOMKilled
			if !st.Running {
				f.HasExit, f.ExitCode = true, st.ExitCode
			}
		}
	}
	if running && svc.ExecEnabled {
		f.ListeningPorts = rt.listeningPorts(pctx, runtime, container)
	}
	return f
}

// listeningPorts reads the container's own socket tables, which needs a
// running container and exec access to be enabled for the app.
func (rt *Router) listeningPorts(ctx context.Context, runtime docker.Runtime, container string) []int {
	state, err := runtime.InspectByName(ctx, container)
	if err != nil || state == nil || !state.Running {
		return nil
	}
	var tables []string
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		out, rerr := runtime.Exec(ctx, state.ID, []string{"cat", path})
		if rerr != nil {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(out, maxProcNetBytes))
		_ = out.Close()
		tables = append(tables, string(b))
	}
	return diagnose.ParseListeningPorts(tables...)
}

func (rt *Router) isLocalNode(nodeID string) bool {
	return nodeID == "" || nodeID == rt.localNodeID
}

func configuredEnvKeys(svc *store.DesiredService) []string {
	seen := map[string]bool{}
	for k, v := range svc.Env {
		if v != "" {
			seen[k] = true
		}
	}
	for _, ref := range svc.SecretEnv {
		seen[ref.Name] = true
	}
	for k := range svc.VaultEnv {
		seen[k] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func envFloatOr(key string, def float64) float64 {
	if f, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil && f > 0 {
		return f
	}
	return def
}

func envDurationOr(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d > 0 {
		return d
	}
	return def
}
