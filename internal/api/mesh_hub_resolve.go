package api

import (
	"context"
	"errors"
	"fmt"

	meshnet "github.com/GLINCKER/levelrail/internal/network"
)

// SetMeshHubResolver lets a hostname advertise host become the mesh endpoint IP.
func (rt *Router) SetMeshHubResolver(r *meshnet.HubHostResolver) { rt.meshHubResolver = r }

// meshHubHostUsable is true when the mesh can use host as an endpoint: an IP
// literal, or a hostname the resolver turns into a reachable IP.
func (rt *Router) meshHubHostUsable(ctx context.Context, host string) bool {
	if !meshAdvertiseHostNeedsIP(host) {
		return true
	}
	if rt.meshHubResolver == nil {
		return false
	}
	_, err := rt.meshHubResolver.Resolve(ctx, host)
	return err == nil
}

const meshHubSettingFix = "Fix the DNS record for %s so it points at this server's public IP, or save the public IP as the advertise address with the \"Save the address as a setting\" fix on the public address check (no restart needed)."

// doctorMeshHubHostname judges a hostname advertise host by resolving it, the
// same way the mesh endpoint for agents is built.
func (rt *Router) doctorMeshHubHostname(ctx context.Context, code, name, host, docsPath string) doctorCheckResource {
	if rt.meshHubResolver == nil {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn,
			Message:  fmt.Sprintf("mesh is enabled but APP_AGENT_ADVERTISE_HOST is the hostname %q and this control plane cannot resolve it for the mesh endpoint, so agents cannot set the endpoint and never handshake", host),
			Fix:      fmt.Sprintf(meshHubSettingFix, host),
			DocsPath: docsPath,
		}
	}
	res, err := rt.meshHubResolver.Resolve(ctx, host)
	if err == nil {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusOK,
			Message:  fmt.Sprintf("resolved %s to %s for the mesh endpoint, re-checked every %s; agents dial it on UDP %d", host, res.IP, rt.meshHubResolver.TTL(), meshnet.DefaultListenPort),
			Fix:      fmt.Sprintf("Allow inbound UDP %d on this server's firewall and cloud security group.", meshnet.DefaultListenPort),
			DocsPath: docsPath,
		}
	}
	msg := fmt.Sprintf("mesh is enabled but the advertise hostname %q could not be turned into a mesh endpoint: %s", host, err)
	if res.IP.IsValid() {
		msg += fmt.Sprintf("; agents keep using the last good address %s", res.IP)
	} else {
		msg += "; agents cannot set the endpoint and never handshake"
	}
	var he *meshnet.HubHostError
	if errors.As(err, &he) && he.Reason == meshnet.HubHostPrivate {
		msg = fmt.Sprintf("mesh is enabled but the advertise hostname %q resolves to %s, which is private, loopback or link-local, so remote agents cannot reach it", host, he.Resolved)
	}
	return doctorCheckResource{
		Code: code, Name: name, Status: doctorStatusWarn,
		Message:  msg,
		Fix:      fmt.Sprintf(meshHubSettingFix, host),
		DocsPath: docsPath,
	}
}
