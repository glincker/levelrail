package api

import (
	"net"
	"strconv"
	"strings"
	"time"
)

// Join token lifetime bounds: short by default, never longer than a day,
// since a leaked unredeemed token enrolls an arbitrary machine.
const (
	minNodeJoinTokenTTL = time.Minute
	maxNodeJoinTokenTTL = 24 * time.Hour
)

// WithNodeJoinTokenTTL sets how long a dashboard or CLI join token stays
// redeemable, clamped to [1m, 24h]. Zero keeps the 15 minute default.
func WithNodeJoinTokenTTL(d time.Duration) Option {
	return func(rt *Router) { rt.joinTokenTTL = clampJoinTokenTTL(d) }
}

func clampJoinTokenTTL(d time.Duration) time.Duration {
	switch {
	case d <= 0:
		return nodeJoinTokenTTL
	case d < minNodeJoinTokenTTL:
		return minNodeJoinTokenTTL
	case d > maxNodeJoinTokenTTL:
		return maxNodeJoinTokenTTL
	}
	return d
}

func (rt *Router) effectiveJoinTokenTTL() time.Duration {
	if rt.joinTokenTTL > 0 {
		return rt.joinTokenTTL
	}
	return nodeJoinTokenTTL
}

// createNodeJoinTokenRequest is the optional body of POST /api/v1/nodes/join-tokens.
type createNodeJoinTokenRequest struct {
	ControlPlaneAddr string `json:"control_plane_addr"`
	NodeName         string `json:"node_name"`
	Mesh             bool   `json:"mesh"`
}

// joinCommands are ready-to-paste commands that enroll a server with one token.
type joinCommands struct {
	Docker string `json:"docker"`
	Binary string `json:"binary"`
}

// defaultControlPlaneAddr is where agents dial, from APP_AGENT_ADVERTISE_HOST, or "".
func (rt *Router) defaultControlPlaneAddr() string {
	host := rt.advertiseHost()
	if host == "" || rt.doctorAgentAdvertisePort == 0 {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(rt.doctorAgentAdvertisePort))
}

// buildJoinCommands renders the docker and binary join commands. Every
// interpolated value is validated by the caller or generated server side,
// so nothing here can inject shell syntax.
func (rt *Router) buildJoinCommands(addr, token, caFingerprint, nodeName string, mesh bool) joinCommands {
	env := []string{"APP_CONTROL_PLANE_ADDR=" + addr, "APP_JOIN_TOKEN=" + token}
	if caFingerprint != "" {
		env = append(env, "APP_CA_FINGERPRINT="+caFingerprint)
	}
	if nodeName != "" {
		env = append(env, "APP_NODE_NAME="+nodeName)
	}
	agent := rt.brand.AgentName()
	dataDir := "/var/lib/" + agent + "-data"
	env = append(env, "APP_AGENT_IDENTITY_FILE="+dataDir+"/identity.json")
	if mesh {
		env = append(env, "APP_MESH_ENABLED=1")
	}

	docker := []string{"docker run -d --restart unless-stopped --name " + agent, "--user 0:0 --network host",
		"-v /var/run/docker.sock:/var/run/docker.sock", "-v " + dataDir + ":" + dataDir}
	if mesh {
		docker = append(docker, "--cap-add NET_ADMIN --device /dev/net/tun")
	}
	for _, e := range env {
		docker = append(docker, "-e "+e)
	}
	docker = append(docker, sshProvisionAgentImage(agent))

	return joinCommands{
		Docker: strings.Join(docker, " "),
		Binary: "sudo " + strings.Join(env, " ") + " " + agent,
	}
}
