package runtimesettings

import "time"

// Mesh DNS resolver on the bridge gateways (beta, off by default).
const (
	KeyDNSMeshResolverEnabled  = "dns.mesh_resolver_enabled"
	KeyDNSMeshResolverInterval = "dns.mesh_resolver_interval"
)

func init() {
	Register(Definition{
		Key: KeyDNSMeshResolverEnabled, Category: CategoryNetworking, Kind: KindBool, Default: "false",
		Env: "LEVELRAIL_DNS_MESH_RESOLVER_ENABLED", Apply: ApplyLive,
		Title: "Mesh DNS on bridge gateways (beta)",
		Description: "Bind the mesh DNS server to each managed bridge gateway on port 53, open the host firewall for it, " +
			"and point new app containers on this node at it. Needs mesh networking. Off by default.",
	})
	Register(Definition{
		Key: KeyDNSMeshResolverInterval, Category: CategoryNetworking, Kind: KindDuration, Default: "15s",
		Env: "LEVELRAIL_DNS_MESH_RESOLVER_INTERVAL", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(time.Second, time.Hour),
		Title:       "Mesh DNS bridge sync interval",
		Description: "How often bridge gateway listeners and the firewall rule are reconciled with Docker's networks.",
	})
}
