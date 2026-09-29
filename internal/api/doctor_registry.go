package api

import (
	"context"
	"fmt"
	"net"
	"strconv"
)

// defaultDoctorRegistryPort is the TLS port every OCI registry this
// check probes (Docker Hub, a configured external credential's host, the
// built-in registry) serves its API on.
const defaultDoctorRegistryPort = 443

// defaultDoctorRegistryHost is probed whenever no registry credential and
// no built-in registry are configured: apps still pull public images from
// Docker Hub by default, so this is the one outbound dependency that
// exists even on an otherwise unconfigured instance.
const defaultDoctorRegistryHost = "registry-1.docker.io"

// doctorCheckRegistryReachability probes outbound TCP reachability to
// every registry host this control plane actually pulls or pushes
// against: each configured external registry credential
// (RegistryCredential.RegistryHost), the built-in registry's own host
// when enabled, and the Docker Hub default when neither is configured.
// This is the gap the real 2-node verification run into: "is Docker
// running" says nothing about whether the registry a build or deploy
// needs is actually reachable from this host.
func (rt *Router) doctorCheckRegistryReachability(ctx context.Context) []doctorCheckResource {
	hosts := rt.doctorRegistryHosts(ctx)
	out := make([]doctorCheckResource, 0, len(hosts))
	for _, host := range hosts {
		out = append(out, rt.doctorCheckOneRegistry(ctx, host))
	}
	return out
}

// doctorRegistryHosts collects every distinct registry host worth
// probing, falling back to defaultDoctorRegistryHost when no credential
// and no built-in registry are configured.
func (rt *Router) doctorRegistryHosts(ctx context.Context) []string {
	seen := make(map[string]bool)
	var hosts []string
	add := func(h string) {
		if h == "" || seen[h] {
			return
		}
		seen[h] = true
		hosts = append(hosts, h)
	}

	if rt.registryCredentials != nil {
		if creds, err := rt.registryCredentials.ListRegistryCredentials(ctx); err == nil {
			for _, c := range creds {
				add(c.RegistryHost)
			}
		}
	}
	if rt.registry != nil {
		if settings, err := rt.registry.GetRegistrySettings(ctx); err == nil && settings.Enabled {
			add(settings.Host)
		}
	}
	if len(hosts) == 0 {
		add(defaultDoctorRegistryHost)
	}
	return hosts
}

// doctorCheckOneRegistry dials host:443 from this host itself. Degrades
// to warn, never fail: a registry an operator has genuinely no plans to
// pull from soon (a private registry that's temporarily down, for
// instance) is a heads-up, not a broken control plane.
func (rt *Router) doctorCheckOneRegistry(ctx context.Context, host string) doctorCheckResource {
	code := "registry_reachability_" + host
	name := fmt.Sprintf("Registry reachability (%s)", host)
	const docsPath = "/troubleshooting#registry-reachability-failed"

	addr := net.JoinHostPort(host, strconv.Itoa(defaultDoctorRegistryPort))
	conn, err := rt.doctorDialContextOrDefault()(ctx, "tcp", addr)
	if err != nil {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn,
			Message:  fmt.Sprintf("could not reach %s: %s", addr, err),
			Fix:      fmt.Sprintf("Confirm this host can make outbound HTTPS connections to %s (egress firewall rules, any HTTP(S)_PROXY settings). Builds and deploys that pull or push images here will fail until it's reachable.", host),
			DocsPath: docsPath,
		}
	}
	_ = conn.Close()
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("%s is reachable", addr)}
}
