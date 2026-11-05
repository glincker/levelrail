package api

import (
	"context"
	"fmt"
	"time"

	meshnet "github.com/GLINCKER/levelrail/internal/network"
)

// SetMeshHubResolver lets a hostname advertise host become the mesh endpoint IP.
func (rt *Router) SetMeshHubResolver(r *meshnet.HubHostResolver) { rt.meshHubResolver = r }

// meshHubHostProblem is "" when the mesh can use host as an endpoint (an IP
// literal, or a hostname that resolves), else why it cannot.
func (rt *Router) meshHubHostProblem(ctx context.Context, host string) string {
	if !meshAdvertiseHostNeedsIP(host) {
		return ""
	}
	if rt.meshHubResolver == nil {
		return fmt.Sprintf(meshAdvertiseHostMessage, host)
	}
	if _, err := rt.meshHubResolver.Resolve(ctx, host); err != nil {
		return fmt.Sprintf("the mesh cannot use the advertise host %q: %s", host, err)
	}
	return ""
}

const meshHubSettingFix = "Fix the DNS record for %s so it points at this server's public IP, or save the IP as the advertise address with the \"Save the address as a setting\" fix on the public address check (no restart needed)."

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
	switch {
	case res.IP.IsValid():
		msg += fmt.Sprintf("; agents keep using the last good address %s, stale since %s, for at most %s more", res.IP, res.ResolvedAt.UTC().Format(time.RFC3339), res.ResolvedAt.Add(rt.meshHubResolver.StaleMax()).Sub(rt.meshHubResolver.Now()).Round(time.Minute))
	case res.LastGood.IsValid():
		msg += fmt.Sprintf("; the last good address %s (since %s) is no longer used after %s, so agents cannot set the endpoint", res.LastGood, res.LastGoodAt.UTC().Format(time.RFC3339), rt.meshHubResolver.StaleMax())
	default:
		msg += "; agents cannot set the endpoint and never handshake"
	}
	return doctorCheckResource{
		Code: code, Name: name, Status: doctorStatusWarn,
		Message:  msg,
		Fix:      fmt.Sprintf(meshHubSettingFix, host),
		DocsPath: docsPath,
	}
}
