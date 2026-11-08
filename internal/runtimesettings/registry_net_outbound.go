package runtimesettings

// KeyNetOutboundTimeout bounds every shared outbound HTTP request.
const KeyNetOutboundTimeout = "net.outbound_timeout"

func init() {
	Register(Definition{
		Key: KeyNetOutboundTimeout, Category: CategoryNetworking, Kind: KindDuration, Default: "30s",
		Env: "APP_NET_OUTBOUND_TIMEOUT", Apply: ApplyLive,
		Title:       "Outbound request timeout",
		Description: "How long an outbound HTTP request from the control plane (cloud metadata, model probes, upgrade checks) may take before it is cancelled.",
	})
}
