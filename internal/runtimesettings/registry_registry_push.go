package runtimesettings

import "time"

// Registry push webhook keys (GHCR and Docker Hub deliveries to the image-update hook).
const (
	KeyDeploysRegistryPushMaxBody = "deploys.registry_push_max_body_bytes"
	KeyDeploysRegistryPushTimeout = "deploys.registry_push_timeout"
)

func init() {
	Register(Definition{
		Key: KeyDeploysRegistryPushMaxBody, Category: CategoryDeploys, Kind: KindInt, Default: "262144",
		Env: "APP_REGISTRY_PUSH_MAX_BODY_BYTES", Apply: ApplyLive, CheckValue: intRangeCheck(1024, 4<<20),
		Title:       "Registry push webhook: max body",
		Description: "Largest registry push payload, in bytes, the image-update webhook reads before refusing it, from 1024 to 4194304.",
	})
	Register(Definition{
		Key: KeyDeploysRegistryPushTimeout, Category: CategoryDeploys, Kind: KindDuration, Default: "5m",
		Env: "APP_REGISTRY_PUSH_TIMEOUT", Apply: ApplyLive, CheckValue: durationRangeCheck(10*time.Second, time.Hour),
		Title:       "Registry push webhook: deploy timeout",
		Description: "How long one registry push may spend resolving and deploying its digest before it is abandoned.",
	})
}
