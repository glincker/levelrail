package main

import (
	"log/slog"
	"path/filepath"

	"github.com/glincker/levelrail/internal/agent"
	"github.com/glincker/levelrail/internal/experimental"
	"github.com/glincker/levelrail/internal/ingress"
	ingressreconcile "github.com/glincker/levelrail/internal/reconcile/ingress"
)

// tunnelSocketDirName sits under the data dir, which is private to the service user.
const tunnelSocketDirName = "ingress-tunnels"

// startAgentTunnels builds the tunnel gateway once, only while the flag is on,
// so a default install opens no listener. It returns nil when disabled.
func startAgentTunnels(server *agent.Server, dataDir string, logger *slog.Logger, onClose func(func())) *ingressreconcile.AgentTunnels {
	if server == nil || !experimental.Enabled(experimental.AgentIngressTunnel) {
		return nil
	}
	gw, err := ingress.NewTunnelGateway(filepath.Join(dataDir, tunnelSocketDirName), ingressreconcile.AgentTunnelDialer{Opener: server}, logger)
	if err != nil {
		logger.Warn("ingress: agent tunnel gateway not started", slog.String("error", err.Error()))
		return nil
	}
	onClose(func() { _ = gw.Close() })
	return &ingressreconcile.AgentTunnels{Gateway: gw, Capable: server.NodeTunnels, Connected: server.NodeConnected}
}
