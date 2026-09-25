package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/gpu"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/models"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const defaultGPUCollectInterval = 60 * time.Second

func gpuCollectInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_GPU_COLLECT_INTERVAL")
	if raw == "" {
		return defaultGPUCollectInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		logger.Warn("invalid APP_GPU_COLLECT_INTERVAL, using default", slog.String("value", raw))
		return defaultGPUCollectInterval
	}
	return d
}

// modelImageOverrides reads APP_MODEL_IMAGE_OLLAMA, APP_MODEL_IMAGE_VLLM
// and APP_MODEL_IMAGE_LLAMACPP.
func modelImageOverrides() map[string]string {
	out := map[string]string{}
	for _, name := range models.EngineNames() {
		if v := os.Getenv("APP_MODEL_IMAGE_" + strings.ToUpper(name)); v != "" {
			out[name] = v
		}
	}
	return out
}

func newModelHostResolver() *models.HostResolver {
	return models.NewHostResolver(publicHost(), ingressdriver.FallbackDomain)
}

// modelWiring builds the model service and gateway used by the HTTP layer.
// secretsManager may be nil (no master key).
func modelWiring(db *store.DB, secretsManager *secrets.Manager) (*models.Service, *models.Gateway, *models.HostResolver) {
	hosts := newModelHostResolver()
	var writer models.SecretWriter
	if secretsManager != nil {
		writer = secretsManager
	}
	return models.NewService(db, writer, hosts, ""), models.NewGateway(db, hosts, nil), hosts
}

// modelNodes resolves node GPU state and reachability for the model and
// application controllers.
type modelNodes struct {
	db          *store.DB
	localNodeID string
}

func (n modelNodes) isLocal(nodeID string) bool { return nodeID == "" || nodeID == n.localNodeID }

// NodeGPU implements application.NodeGPUChecker.
func (n modelNodes) NodeGPU(ctx context.Context, nodeID string) (gpu.Info, error) {
	key := nodeID
	if n.isLocal(nodeID) {
		key = store.LocalNodeGPUKey
	}
	snap, _, err := n.db.GetNodeGPU(ctx, key)
	if err != nil {
		return gpu.Info{}, fmt.Errorf("read node gpu: %w", err)
	}
	return snap.Info, nil
}

// NodeInfo implements models.Nodes.
func (n modelNodes) NodeInfo(ctx context.Context, nodeID string) (models.NodeInfo, error) {
	info, err := n.NodeGPU(ctx, nodeID)
	if err != nil {
		return models.NodeInfo{}, err
	}
	out := models.NodeInfo{GPU: info, BindIP: "127.0.0.1", DialHost: "127.0.0.1"}
	if n.isLocal(nodeID) {
		return out, nil
	}
	node, err := n.db.GetNode(ctx, nodeID)
	if err != nil {
		return models.NodeInfo{}, fmt.Errorf("look up node %q: %w", nodeID, err)
	}
	if node.MeshAddress == "" {
		out.DialHost = ""
		return out, nil
	}
	out.BindIP, out.DialHost = node.MeshAddress, node.MeshAddress
	return out, nil
}

// modelDeps is what dynamicSource needs to build model controllers. The
// prober is shared across reconcile passes because it tracks in-flight
// downloads.
type modelDeps struct {
	prober *models.HTTPProber
	hosts  *models.HostResolver
	images map[string]string
}

func newModelDeps() *modelDeps {
	return &modelDeps{prober: models.NewHTTPProber(), hosts: newModelHostResolver(), images: modelImageOverrides()}
}

// modelControllersFor builds one models.Controller per model, skipping
// (with a warning) any whose node transport is unavailable this pass.
func modelControllersFor(deps dynamicSourceDeps, list []store.Model) []reconcile.Controller {
	nodes := modelNodes{db: deps.db, localNodeID: localNodeIDOf(deps)}
	opts := []models.Option{models.WithContainerPrefix(deps.networkPrefix), models.WithImages(deps.models.images)}
	if deps.secretsManager != nil {
		opts = append(opts, models.WithSecrets(deps.secretsManager))
	}
	controllers := make([]reconcile.Controller, 0, len(list))
	for _, m := range list {
		rt, err := resolveNodeTransport(deps.runtime, deps.agentRegistry, modelRuntimeNode(nodes, m.NodeID))
		if err != nil {
			deps.logger.Warn("skipping model for this reconcile pass: node transport unavailable",
				slog.String("model", m.Name), slog.String("node_id", m.NodeID), slog.String("error", err.Error()))
			continue
		}
		controllers = append(controllers, models.New(m.Name, deps.db, nodes, rt, deps.models.prober, opts...))
	}
	return controllers
}

func modelRuntimeNode(n modelNodes, nodeID string) string {
	if n.isLocal(nodeID) {
		return ""
	}
	return nodeID
}

func localNodeIDOf(deps dynamicSourceDeps) string {
	if deps.meshCfg == nil {
		return ""
	}
	return deps.meshCfg.localNodeID
}

// withModelTelemetryTargets adds every running local model container to
// base's collection targets so model logs and metrics land in the
// node-local stores under resource "model:<name>".
func withModelTelemetryTargets(base func(context.Context) ([]telemetry.Target, error), db *store.DB, rt docker.Runtime, prefix string) func(context.Context) ([]telemetry.Target, error) {
	return func(ctx context.Context) ([]telemetry.Target, error) {
		targets, err := base(ctx)
		if err != nil {
			return nil, err
		}
		list, err := db.ListModels(ctx)
		if err != nil {
			return nil, fmt.Errorf("list models: %w", err)
		}
		for _, m := range list {
			if m.NodeID != "" {
				continue
			}
			containers, err := rt.ListByPrefix(ctx, models.ContainerPrefix(prefix, m.Name))
			if err != nil {
				return nil, fmt.Errorf("list containers for model %s: %w", m.Name, err)
			}
			for _, c := range containers {
				if c.Running {
					targets = append(targets, telemetry.Target{ResourceID: "model:" + m.Name, ContainerID: c.ID})
				}
			}
		}
		return targets, nil
	}
}

func startLocalGPUCollector(ctx context.Context, db *store.DB, rl gpu.RuntimeLister, logger *slog.Logger) {
	go gpu.RunCollector(ctx, db, gpu.ExecRunner{}, rl, store.LocalNodeGPUKey, gpuCollectInterval(logger), logger)
}
