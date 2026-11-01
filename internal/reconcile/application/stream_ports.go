package application

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// effectiveDesired returns desired with two private publishes moved aside
// so they cannot squat a host port someone else needs: one a stream's
// ingress listener binds (Linux refuses a wildcard bind while a loopback
// bind holds the same number), and one the main port already binds as a
// direct exposure. The first becomes an ephemeral loopback publish, the
// second is dropped. The stored desired state is never changed.
func effectiveDesired(desired *store.DesiredService, streamHostPorts map[int]bool) *store.DesiredService {
	if len(desired.PublishedPorts) == 0 {
		return desired
	}
	out := *desired
	out.PublishedPorts = make([]store.PublishedPort, 0, len(desired.PublishedPorts))
	for _, p := range desired.PublishedPorts {
		private := p.Bind == spec.PortBindPrivate
		tcp := p.EffectiveProtocol() == store.PortProtocolTCP
		if private && tcp {
			if desired.Port != 0 && desired.HostPort != nil && *desired.HostPort == p.HostPort && desired.Port == p.ContainerPort {
				continue
			}
			if streamHostPorts[p.HostPort] {
				p.HostPort = 0
			}
		}
		out.PublishedPorts = append(out.PublishedPorts, p)
	}
	return &out
}

// streamHostPorts lists the host ports this service's streams listen on.
func (c *Controller) streamHostPorts(ctx context.Context, serviceName string) (map[int]bool, error) {
	streams, err := c.store.ListAppStreamsForService(ctx, serviceName)
	if err != nil {
		return nil, fmt.Errorf("list app streams for %q: %w", serviceName, err)
	}
	if len(streams) == 0 {
		return nil, nil
	}
	ports := make(map[int]bool, len(streams))
	for _, s := range streams {
		ports[s.HostPort] = true
	}
	return ports, nil
}

// ephemeralPublishDrifted judges a published port left at host port 0: it
// is satisfied by a live binding of the container port that does not hold
// a host port a stream listens on, so a container made before the stream
// existed is recreated once.
func ephemeralPublishDrifted(ports []docker.PortBinding, p store.PublishedPort, streamHostPorts map[int]bool) bool {
	found := false
	for _, b := range ports {
		if b.ContainerPort != p.ContainerPort || bindingProtocol(b) != p.EffectiveProtocol() {
			continue
		}
		if streamHostPorts[b.HostPort] {
			return true
		}
		found = true
	}
	return !found
}
