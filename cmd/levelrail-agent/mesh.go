package main

// Node-side mesh device for this agent, gated by APP_MESH_ENABLED (unset means off).
// A mesh setup failure must not stop the agent: ApplyMesh and RotateMeshKey answer agent.ErrMeshUnavailable.

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/network"
)

const (
	// meshKeyFilename mirrors cmd/levelrail's own mesh.go constant of the
	// same name: same file format (network.Key.String()'s base64 form),
	// same reasoning, different process's disk.
	meshKeyFilename = "levelrail-agent-mesh.key"
)

// meshAgentSetup is the mesh handle run() passes to agent.WithMesh; nil means mesh is off or unavailable here.
type meshAgentSetup struct {
	sink  *network.LocalSink
	close func()
}

// Same env var as cmd/levelrail's meshEnabled, so one setting opts a control plane and its agents in together.
func meshEnabled() bool {
	return os.Getenv("APP_MESH_ENABLED") == "1"
}

// Defaults to the identity file's directory so the agent keeps a single persisted-state location.
func meshKeyPath() string {
	if p := os.Getenv("APP_MESH_KEY_FILE"); p != "" {
		return p
	}
	return filepath.Join(filepath.Dir(identityFilePath()), meshKeyFilename)
}

// setupAgentMesh brings up this node's WireGuard device behind a network.LocalSink for nodeID.
// Nothing in network assumes a control-plane process, so the agent reuses the same LocalSink.
func setupAgentMesh(ctx context.Context, nodeID string, logger *slog.Logger) (*meshAgentSetup, error) {
	if !meshEnabled() {
		return nil, nil
	}

	keyPath := meshKeyPath()
	key, err := network.LoadOrGenerateKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("mesh key: %w", err)
	}

	var linkOpt network.DeviceOption
	if link := agentLinkConfigurator(); link != nil {
		linkOpt = network.WithLinkConfigurator(link)
	}

	opts := []network.DeviceOption{network.WithLogger(logger), network.WithShortName(meshBrandShortName())}
	if linkOpt != nil {
		opts = append(opts, linkOpt)
	}
	device, err := network.NewDevice(ctx, network.SystemProbe{}, opts...)
	if err != nil {
		return nil, fmt.Errorf("bring up mesh device: %w", err)
	}

	sink, err := network.NewLocalSink(nodeID, device, key,
		network.WithKeyPersistFunc(func(newKey network.Key) error {
			return network.PersistKey(keyPath, newKey)
		}))
	if err != nil {
		_ = device.Close()
		return nil, fmt.Errorf("build local mesh sink: %w", err)
	}

	logger.Info("mesh networking enabled", slog.String("node_id", nodeID))

	return &meshAgentSetup{
		sink: sink,
		close: func() {
			if err := device.Close(); err != nil {
				logger.Error("closing mesh device", slog.String("error", err.Error()))
			}
		},
	}, nil
}

// agentLinkConfigurator returns the system link configurator on supported platforms, or nil elsewhere.
func agentLinkConfigurator() network.LinkConfigurator {
	switch runtime.GOOS {
	case "darwin", "linux":
		return network.SystemLinkConfigurator{}
	default:
		return nil
	}
}

// meshBrandShortName resolves the brand short name the control plane
// also derives its interface name from: brand.yaml (APP_BRAND_FILE or
// ./brand.yaml) with APP_BRAND_* overrides, else the bare override.
func meshBrandShortName() string {
	path := os.Getenv("APP_BRAND_FILE")
	if path == "" {
		path = "./brand.yaml"
	}
	if b, err := brand.Load(path); err == nil {
		return b.ShortName
	}
	return os.Getenv("APP_BRAND_SHORT_NAME")
}
