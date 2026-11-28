package api

import (
	"fmt"
	"sort"
	"strings"

	"github.com/glincker/levelrail/internal/bindaddr"
	"github.com/glincker/levelrail/internal/spec"
	"github.com/glincker/levelrail/internal/store"
)

// noticeLevelFirewall tags the install notice that lists host ports to open.
const noticeLevelFirewall = "firewall"

// publicHostPorts lists the host ports of services that bind beyond loopback
// and the private mesh, sorted, as "port/proto".
func publicHostPorts(services []store.DesiredService) []string {
	seen := map[string]bool{}
	for _, svc := range services {
		for _, p := range svc.PublishedPorts {
			bind := p.EffectiveBind(svc.BindAddress)
			if bind == "" || bind == bindaddr.Private || bind == spec.PortBindPrivate || bind == "127.0.0.1" {
				continue
			}
			seen[p.String()] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// firewallNotice says which host ports a deploy publishes publicly so the
// operator can open them upstream; nil when there are none.
func firewallNotice(services []store.DesiredService) *composeNoticeResult {
	ports := publicHostPorts(services)
	if len(ports) == 0 {
		return nil
	}
	return &composeNoticeResult{Level: noticeLevelFirewall,
		Message: fmt.Sprintf("open these host ports in your cloud or network firewall: %s", strings.Join(ports, ", "))}
}

// withoutShadowedPublishedPorts drops published entries that use hostPort over
// tcp, so a direct exposure of that port is not blocked by a loopback map.
func withoutShadowedPublishedPorts(ports []store.PublishedPort, hostPort int) []store.PublishedPort {
	var kept []store.PublishedPort
	for _, p := range ports {
		if p.HostPort == hostPort && p.EffectiveProtocol() == spec.PortProtocolTCP {
			continue
		}
		kept = append(kept, p)
	}
	return kept
}
