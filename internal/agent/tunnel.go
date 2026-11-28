package agent

import (
	"errors"
	"log/slog"
	"strings"
	"sync"

	"github.com/glincker/levelrail/internal/experimental"
)

// CapIngressTunnel means the agent can carry ingress traffic to its own
// service ports over the existing outbound connection. It is advertised only
// while the agent-ingress-tunnel feature is on.
const CapIngressTunnel = "ingress_tunnel"

// Wire bounds for a tunnel, fixed by the protocol so both ends agree.
const (
	// TunnelMaxFrameData is the largest data frame; a bigger one fails its own tunnel.
	TunnelMaxFrameData = 32 * 1024
	// TunnelMaxResetReason bounds a reset reason so an error cannot carry a payload.
	TunnelMaxResetReason = 256
	// TunnelIDLen is the byte length of a tunnel id.
	TunnelIDLen = 16
)

// ErrTunnelUnsupported is returned while the flag is off or the agent lacks the capability.
var ErrTunnelUnsupported = errors.New("agent: ingress tunnel unsupported")

// agentCapabilities lists what this agent binary advertises; the tunnel
// capability follows the feature flag so a default install never offers it.
func agentCapabilities() []string {
	caps := []string{CapContainerSpecSecurity, CapExitTimes, CapContainerSpecFiles, CapBuildNetwork, CapContainerSpecWorkdir, CapContainerInspect, CapContainerSpecRuntime, CapBuildPush, CapRegistryBearer, CapHostProbe, CapNodeCleanup, CapImagePull, CapFenceToken, CapVolumeList}
	if experimental.Enabled(experimental.AgentIngressTunnel) {
		caps = append(caps, CapIngressTunnel)
	}
	return caps
}

var capsLogOnce sync.Once

// logAgentCapabilities logs the advertised names once per process so an operator can see why
// a feature like the ingress tunnel is not offered, without a line per reconnect.
func logAgentCapabilities(logger *slog.Logger, once *sync.Once) {
	once.Do(func() {
		logger.Info("agent: advertised capabilities", slog.String("capabilities", strings.Join(agentCapabilities(), ",")))
	})
}
