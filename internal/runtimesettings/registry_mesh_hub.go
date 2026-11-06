package runtimesettings

import "time"

// Mesh hub hostname resolution keys.
const (
	KeyMeshHubResolveTTL = "mesh.hub_resolve_ttl"
	KeyMeshHubStaleMax   = "mesh.hub_stale_max"
)

func init() {
	Register(Definition{
		Key: KeyMeshHubResolveTTL, Category: CategoryNetworking, Kind: KindDuration, Default: "5m",
		Env: "APP_MESH_HUB_RESOLVE_TTL", Apply: ApplyLive,
		CheckValue: durationRangeCheck(30*time.Second, 24*time.Hour),
		Title:      "Mesh hub hostname re-check time",
		Description: "When the advertise address is a hostname, the control plane resolves it to an IP for the mesh endpoint and checks again after this long. " +
			"A changed DNS answer reaches agents on the next mesh reconcile; a failed lookup keeps the last good IP. " +
			"Each lookup is bounded by the DNS lookup timeout (APP_DNS_STATUS_LOOKUP_TIMEOUT, shared with the DNS status checks).",
	})
	Register(Definition{
		Key: KeyMeshHubStaleMax, Category: CategoryNetworking, Kind: KindDuration, Default: "24h",
		Env: "APP_MESH_HUB_STALE_MAX", Apply: ApplyLive,
		CheckValue: durationRangeCheck(5*time.Minute, 30*24*time.Hour),
		Title:      "Mesh hub last good address lifetime",
		Description: "How long agents keep dialing the last good IP of the advertise hostname while lookups fail. After this the mesh endpoint is dropped, " +
			"so a reused cloud IP does not receive handshakes meant for this server.",
	})
}
