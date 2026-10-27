package application

import (
	"net"

	"github.com/GLINCKER/levelrail/internal/bindaddr"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

const reasonPortDriftRecreate = "PortDriftRecreateFailed"

// publishedPortDrifted reports a running container whose published main
// port no longer matches desired: the container port changed, or the bind
// interface did. Container names hash only image and nonce, so without this
// an exposure edit would read as AlreadyRunning forever. A stopped container is judged by its configured
// bindings, since a restart would republish the stale port. A container that
// reports no bindings or no host IP is unknown, not drifted.
func publishedPortDrifted(state *docker.ContainerState, desired *store.DesiredService, meshIP string) bool {
	if state == nil || desired.Port == 0 {
		return false
	}
	ports := state.Ports
	if !state.Running {
		ports = state.ConfiguredPorts
	}
	if len(ports) == 0 {
		return false
	}
	want := meshIP
	if want == "" {
		ip, err := bindaddr.Resolve(desired.BindAddress)
		if err != nil {
			return false
		}
		want = ip
	}
	for _, p := range ports {
		if p.ContainerPort != desired.Port || bindingProtocol(p) != store.PortProtocolTCP {
			continue
		}
		if meshFlapBind(p.HostIP, want) || p.HostIP == "" || p.HostIP == want || (want == "0.0.0.0" && p.HostIP == "::") {
			return false
		}
	}
	return true
}

// meshFlapBind spots a loopback-desired port bound to one specific
// non-wildcard address, which is the mesh bind surviving a path flap.
func meshFlapBind(have, want string) bool {
	ip := net.ParseIP(have)
	return isLoopbackIP(want) && ip != nil && !ip.IsLoopback() && !ip.IsUnspecified()
}

// publishedPortsDrifted reports a container whose direct host ports differ
// from desired by (container port, host port, protocol), in either
// direction. No reported bindings at all is unknown, not drifted.
func publishedPortsDrifted(state *docker.ContainerState, desired *store.DesiredService) bool {
	if state == nil {
		return false
	}
	ports := state.Ports
	if !state.Running {
		ports = state.ConfiguredPorts
	}
	if len(ports) == 0 {
		return false
	}
	have := map[store.PublishedPort]bool{}
	for _, p := range ports {
		have[store.PublishedPort{ContainerPort: p.ContainerPort, HostPort: p.HostPort, Protocol: bindingProtocol(p)}] = true
	}
	want := map[store.PublishedPort]bool{}
	for _, p := range desired.PublishedPorts {
		key := store.PublishedPort{ContainerPort: p.ContainerPort, HostPort: p.HostPort, Protocol: p.EffectiveProtocol()}
		want[key] = true
		if !have[key] {
			return true
		}
	}
	// Only non-tcp leftovers count: tcp bindings also come from the main port and app streams.
	for k := range have {
		if k.Protocol != store.PortProtocolTCP && !want[k] {
			return true
		}
	}
	return false
}
