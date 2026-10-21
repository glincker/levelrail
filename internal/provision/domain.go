package provision

import "strings"

// FailureDomain maps the region id a server was created in to the region and
// zone labels stored on its node. Providers whose location is a single
// datacenter (hetzner, digitalocean) report it as both; gcp ids are zones.
func FailureDomain(provider, regionID string) (region, zone string) {
	switch provider {
	case "gcp":
		if i := strings.LastIndex(regionID, "-"); i > 0 {
			return regionID[:i], regionID
		}
		return regionID, ""
	case "hetzner", "digitalocean":
		return regionID, regionID
	default:
		return regionID, ""
	}
}
