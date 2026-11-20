package runtimesettings

// Registered keys for request size and connection limits on the API server and the ingress edge.
const (
	KeyAPIMaxHeaderBytes         = "api.max_header_bytes"
	KeyIngressBodyReadTimeoutSec = "ingress.body_read_timeout_seconds"
	KeyIngressMaxRequestsPerIP   = "ingress.max_requests_per_ip"
)

func init() {
	registerLimit(Definition{
		Key: KeyAPIMaxHeaderBytes, Category: CategoryNetworking, Default: "65536", Env: "APP_API_MAX_HEADER_BYTES", Apply: ApplyRestart,
		Title:       "API request header limit",
		Description: "Largest request header block in bytes the API server reads. From 8192 to 1048576. The server reads it at start, so a change needs a restart.",
	}, 8192, 1<<20)
	registerLimit(Definition{
		Key: KeyIngressBodyReadTimeoutSec, Category: CategoryNetworking, Default: "1800", Env: "APP_INGRESS_BODY_READ_TIMEOUT_SECONDS", Apply: ApplyLive,
		Title:       "Ingress body read deadline",
		Description: "Seconds a request body may take to arrive on routes that are not websockets or event streams. From 60 to 86400. A saved change re-renders the ingress config at once.",
	}, 60, 86400)
	registerLimit(Definition{
		Key: KeyIngressMaxRequestsPerIP, Category: CategoryNetworking, Default: "256", Env: "APP_INGRESS_MAX_REQUESTS_PER_IP", Apply: ApplyLive,
		Title:       "Ingress concurrent requests per address",
		Description: "Most requests one client address may have in flight at once. Websockets and event streams are not counted. From 8 to 100000. A saved change re-renders the ingress config at once.",
	}, 8, 100000)
}
