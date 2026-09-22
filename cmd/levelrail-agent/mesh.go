package main

// This file: this agent's own mesh device, the node-side half of
// internal/network.ConfigSink's gRPC arm (see internal/agent.MeshApplier
// and internal/agent.GRPCSink for the transport this crosses). Gated
// behind APP_MESH_ENABLED, the same env var and the same "unset means
// off" default cmd/levelrail's own meshEnabled already establishes: an
// operator opts every node in individually (control plane and each
// agent), not implicitly by enrolling.
//
// A node whose mesh setup fails still runs: this binary's whole job is
// Docker operations, and a bad mesh key file or a TUN device this
// process cannot create (missing CAP_NET_ADMIN, an unsupported sandbox)
// must not stop it from serving container requests, the identical
// "a node that cannot mesh must still run" contract network.NewDevice's
// own doc comment already establishes one layer down for the exact same
// reason. What changes is only that this node's ApplyMesh/RotateMeshKey
// requests answer with agent.ErrMeshUnavailable instead of applying
// anything, which is the accurate state to report, not a crash.

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/GLINCKER/levelrail/internal/network"
)

const (
	// meshKeyFilename mirrors cmd/levelrail's own mesh.go constant of the
	// same name: same file format (network.Key.String()'s base64 form),
	// same reasoning, different process's disk.
	meshKeyFilename = "levelrail-agent-mesh.key"
)

// meshAgentSetup is what setupAgentMesh hands back to run(): the
// MeshApplier to pass to agent.WithMesh, and a close func for shutdown. A
// nil *meshAgentSetup is setupAgentMesh's "mesh not enabled or not
// available on this node" signal, run()'s cue to call agent.RunSession
// with no agent.WithMesh option at all (every ApplyMesh/RotateMeshKey
// request the control plane sends this node then answers with
// agent.ErrMeshUnavailable, per this file's own header).
type meshAgentSetup struct {
	sink  *network.LocalSink
	close func()
}

// meshEnabled mirrors cmd/levelrail's own meshEnabled (mesh.go): the same
// env var name, so one setting governs whether a control plane and one of
// its agents both opt in, without needing two different names an
// operator has to remember are really the same switch.
func meshEnabled() bool {
	return os.Getenv("APP_MESH_ENABLED") == "1"
}

// meshDataDir is where this agent persists its own mesh private key,
// defaulting to the identity file's own directory: an agent already has
// exactly one operator-configured location for its own persisted state
// (APP_AGENT_IDENTITY_FILE), and a second, separate data-directory
// variable for one more small file would be a second thing to configure
// for no real benefit. APP_MESH_KEY_FILE overrides the full path
// directly, for an operator who wants it somewhere else entirely.
func meshKeyPath() string {
	if p := os.Getenv("APP_MESH_KEY_FILE"); p != "" {
		return p
	}
	return filepath.Join(filepath.Dir(identityFilePath()), meshKeyFilename)
}

// setupAgentMesh brings up this node's own WireGuard device and wraps it
// in a network.LocalSink addressed to nodeID (this agent's own enrolled
// node ID, from its Identity), the exact same LocalSink type
// cmd/levelrail's own setupMesh already uses for the control plane's
// local node: MeshApplier (internal/agent/mesh_dispatch.go) is satisfied
// by *network.LocalSink's own ApplyMesh/RotateKey methods with no
// adapter, which is the reuse network.go's own package header promises
// ("everything that makes the kernel agree with that decision... is the
// only part that touches wireguard-go... and no more") and device.go's
// own header promises a second time ("this type owns the WireGuard
// device... it does NOT assign the mesh IP"): nothing in either type
// assumed a control-plane process, so nothing needed decoupling to run
// here.
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

	opts := []network.DeviceOption{network.WithLogger(logger)}
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

// agentLinkConfigurator returns network.SystemLinkConfigurator on the
// platforms it supports (see that type's own doc comment), or nil to
// leave the interface unaddressed elsewhere: an unsupported platform
// getting device.go's own "no LinkConfigurator" warning at Apply time is
// the correct, already-established behavior, not something this function
// needs to duplicate reasoning about.
func agentLinkConfigurator() network.LinkConfigurator {
	switch runtime.GOOS {
	case "darwin", "linux":
		return network.SystemLinkConfigurator{}
	default:
		return nil
	}
}
