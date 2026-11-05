package runtimesettings

import "time"

// KeyMeshHubResolveTTL is how long a resolved hub hostname is reused.
const KeyMeshHubResolveTTL = "mesh.hub_resolve_ttl"

func init() {
	Register(Definition{
		Key: KeyMeshHubResolveTTL, Category: CategoryNetworking, Kind: KindDuration, Default: "5m",
		Env: "APP_MESH_HUB_RESOLVE_TTL", Apply: ApplyLive,
		CheckValue: durationRangeCheck(30*time.Second, 24*time.Hour),
		Title:      "Mesh hub hostname re-check time",
		Description: "When the advertise address is a hostname, the control plane resolves it to an IP for the mesh endpoint and checks again after this long. " +
			"A changed DNS answer reaches agents on the next mesh reconcile; a failed lookup keeps the last good IP.",
	})
}
