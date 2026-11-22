package ingress

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	reasonAgentTunnel = "AgentTunnel"
	// defaultTunnelMaxLifetime caps one tunnel's life; APP_INGRESS_TUNNEL_MAX_LIFETIME overrides it.
	defaultTunnelMaxLifetime = 24 * time.Hour
	tunnelMaxLifetimeEnv     = "APP_INGRESS_TUNNEL_MAX_LIFETIME"
	maxContainerPort         = 65535
)

// AgentTunnels routes mesh-less nodes through the agent session. Capable says
// whether a node's agent advertises ingress_tunnel.
type AgentTunnels struct {
	Gateway *ingress.TunnelGateway
	Capable func(nodeID string) bool
	// Connected says whether the node has a live agent session; nil skips the offline wording.
	Connected func(nodeID string) bool
}

const (
	tunnelOfflineWhy  = "the node is offline and its agent reconnects automatically"
	tunnelUpgradeHint = "the node's agent does not advertise ingress_tunnel: upgrade the agent and set APP_EXPERIMENTAL=agent-ingress-tunnel in its environment too"
)

// tunnelBlockWhy refines a no-route reason while the tunnel flag is on, because
// mesh advice is wrong for an offline or tunnel-capable-with-upgrade node.
func (c *Controller) tunnelBlockWhy(nodeID, why string, noMesh bool) string {
	if c.tunnels == nil || c.tunnels.Gateway == nil || !experimental.Enabled(experimental.AgentIngressTunnel) {
		return why
	}
	if c.tunnels.Connected != nil && !c.tunnels.Connected(nodeID) {
		if noMesh {
			return tunnelOfflineWhy
		}
		return why
	}
	return why + "; " + tunnelUpgradeHint
}

// WithAgentTunnels enables the tunnel path for nodes without a usable mesh path.
func WithAgentTunnels(t *AgentTunnels) Option {
	return func(c *Controller) { c.tunnels = t }
}

// AgentTunnelDialer adapts the control plane's tunnel opener to the gateway's dialer.
type AgentTunnelDialer struct {
	Opener agent.TunnelOpener
}

var _ ingress.TunnelDialer = AgentTunnelDialer{}

// OpenTunnel opens one tunnel and maps the typed agent errors the gateway turns into a 503.
func (d AgentTunnelDialer) OpenTunnel(ctx context.Context, nodeID, service string, containerPort int) (ingress.TunnelStream, error) {
	if containerPort <= 0 || containerPort > maxContainerPort {
		return nil, fmt.Errorf("ingress: container port %d out of range", containerPort)
	}
	var deadline time.Time
	if life := tunnelMaxLifetime(); life > 0 {
		deadline = time.Now().Add(life)
	}
	conn, err := d.Opener.OpenTunnel(ctx, nodeID, service, uint16(containerPort), deadline)
	if err != nil {
		return nil, translateTunnelError(err)
	}
	return conn, nil
}

func translateTunnelError(err error) error {
	switch {
	case errors.Is(err, agent.ErrTunnelUnsupported), errors.Is(err, agent.ErrAgentLacksCapability):
		return fmt.Errorf("%w: %w", ingress.ErrTunnelUnsupported, err)
	case errors.Is(err, agent.ErrNodeOffline):
		return fmt.Errorf("%w: %w", ingress.ErrTunnelOffline, err)
	case errors.Is(err, agent.ErrTunnelTimeout):
		return fmt.Errorf("%w: %w", ingress.ErrTunnelTimeout, err)
	}
	return err
}

func tunnelMaxLifetime() time.Duration {
	if raw := os.Getenv(tunnelMaxLifetimeEnv); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d >= 0 {
			return d
		}
	}
	return defaultTunnelMaxLifetime
}

// agentTunnelDial reports used=true when the tunnel path applies to svc's
// node. An empty dial with used=true is the usual transient skip.
func (c *Controller) agentTunnelDial(ctx context.Context, svc store.DesiredService, targetReady bool) (dial string, used bool) {
	if c.tunnels == nil || c.tunnels.Gateway == nil || !experimental.Enabled(experimental.AgentIngressTunnel) {
		return "", false
	}
	if c.tunnels.Capable == nil || !c.tunnels.Capable(svc.NodeID) {
		return "", false
	}
	if !targetReady {
		return "", true
	}
	sock, err := c.tunnels.Gateway.Register(ingress.TunnelTarget{NodeID: svc.NodeID, Service: svc.Name, ContainerPort: svc.Port})
	if err != nil {
		c.logger.WarnContext(ctx, "ingress: tunnel socket unavailable, skipping for this pass",
			slog.String("service", svc.Name), slog.String("node", svc.NodeID), slog.String("error", err.Error()))
		return "", true
	}
	c.tunnelTargets = append(c.tunnelTargets, ingress.TunnelTarget{NodeID: svc.NodeID, Service: svc.Name, ContainerPort: svc.Port})
	return ingress.TunnelUpstreamDial(sock), true
}

// syncTunnels closes sockets no route used this pass, so gone targets lose theirs.
func (c *Controller) syncTunnels(ctx context.Context) {
	if c.tunnels == nil || c.tunnels.Gateway == nil {
		return
	}
	if err := c.tunnels.Gateway.Sync(c.tunnelTargets); err != nil {
		c.logger.WarnContext(ctx, "ingress: syncing tunnel sockets failed", slog.String("error", err.Error()))
	}
}

// agentTunnelCondition clears NoMeshIngressPath for tunneled services and says how they are reached.
func agentTunnelCondition(targets []ingress.TunnelTarget) *reconcile.Condition {
	if len(targets) == 0 {
		return nil
	}
	names := make([]string, 0, len(targets))
	for _, t := range targets {
		names = append(names, fmt.Sprintf("%s (node %s)", t.Service, t.NodeID))
	}
	return &reconcile.Condition{
		Type:    conditionCrossNodeIngress,
		Status:  reconcile.ConditionTrue,
		Reason:  reasonAgentTunnel,
		Message: fmt.Sprintf("%d service(s) reach their node through the agent tunnel instead of the mesh: %s", len(targets), strings.Join(names, ", ")),
	}
}
