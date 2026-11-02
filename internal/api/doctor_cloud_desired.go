package api

import (
	"github.com/GLINCKER/levelrail/internal/bindaddr"
	"github.com/GLINCKER/levelrail/internal/store"
)

// desiredCloudPorts lists ports the platform intends to expose that a container
// scan misses: ingress stream listeners (bound by the control plane on every
// interface) and direct host ports, even while their container is not running.
func desiredCloudPorts(services []store.DesiredService, streams []store.AppStream) []cloudPort {
	var out []cloudPort
	for _, s := range streams {
		out = append(out, cloudPort{Port: s.HostPort, Proto: s.Protocol, Container: "stream:" + s.ServiceName})
	}
	for _, svc := range services {
		for _, p := range svc.PublishedPorts {
			ip, err := bindaddr.Resolve(p.EffectiveBind(svc.BindAddress))
			out = append(out, cloudPort{
				Port: p.HostPort, Proto: p.EffectiveProtocol(), Container: svc.Name,
				// An unresolvable bind counts as reachable, the safe side.
				Loopback: err == nil && cloudBindsLoopback([]string{ip}),
			})
		}
	}
	return out
}
