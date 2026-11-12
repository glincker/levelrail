package api

import (
	"net/http"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"github.com/GLINCKER/levelrail/internal/store"
)

// elevatedCapsOf returns the Elevated caps (NET_ADMIN, NET_RAW) among caps.
func elevatedCapsOf(caps []string) []string {
	var out []string
	for _, c := range caps {
		if slices.Contains(dockerguard.GrantableCaps, c) {
			out = append(out, c)
		}
	}
	return out
}

// reviewCapabilityChange runs under the app lock on the translated services. It refuses an
// Elevated cap on a service that has an egress policy (the sidecar shares its network
// namespace), lets the caller gate the newly added caps, and records them for the audit.
func (rt *Router) reviewCapabilityChange(w http.ResponseWriter, app string, previous, services []store.DesiredService, opts composeDeployOpts) bool {
	prevByName := make(map[string]store.DesiredService, len(previous))
	for _, p := range previous {
		prevByName[p.Name] = p
	}
	for _, s := range services {
		if elevated := elevatedCapsOf(s.GrantedCaps); len(elevated) > 0 && prevByName[s.Name].Egress != nil {
			writeError(w, http.StatusConflict, "service "+strings.TrimPrefix(s.Name, app+"-")+" has an egress policy, which "+strings.Join(elevated, " and ")+" could remove; clear the egress policy or drop the capability")
			return false
		}
	}
	held := make(map[string][]string, len(previous))
	for _, p := range previous {
		held[strings.TrimPrefix(p.Name, app+"-")] = p.GrantedCaps
	}
	added := addedCaps(held, servicesCapMap(app, services))
	if opts.capGate != nil && !opts.capGate(added) {
		return false
	}
	if opts.addedCaps != nil {
		*opts.addedCaps = added
	}
	return true
}
