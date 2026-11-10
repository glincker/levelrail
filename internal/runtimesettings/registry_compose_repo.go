package runtimesettings

import "time"

// Keys for compose apps deployed from a git repository.
const (
	KeyComposeRepoFileMaxBytes  = "compose.repo_file_max_bytes"
	KeyComposeRepoDeployTimeout = "compose.repo_deploy_timeout"
)

func init() {
	registerLimit(Definition{
		Key: KeyComposeRepoFileMaxBytes, Category: CategoryDeploys, Default: "524288", Env: "APP_COMPOSE_REPO_FILE_MAX_BYTES", Apply: ApplyLive,
		Title:       "Compose file size from git",
		Description: "Largest compose file a compose app deployed from a git repository may read from the checkout, in bytes. From 1024 to 4194304. A larger file fails the deploy.",
	}, 1024, 4194304)
	Register(Definition{
		Key: KeyComposeRepoDeployTimeout, Category: CategoryDeploys, Kind: KindDuration, Default: "10m",
		Env: "APP_COMPOSE_REPO_DEPLOY_TIMEOUT", Apply: ApplyLive, CheckValue: durationRangeCheck(30*time.Second, 2*time.Hour),
		Title:       "Compose deploy from git timeout",
		Description: "How long one push deploy of a compose app from git may take to fetch the repository and apply the file, from 30s to 2h. It runs apart from the webhook request, so a forge timeout never stops it halfway.",
	})
}
