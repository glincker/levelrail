package runtimesettings

import "time"

// Deploy window keys: the drain, hold, stable and watch timings that decide
// whether a deploy drops requests. Env names stay as the seed.
const (
	KeyDeployDrainTimeout        = "deploy.drain_timeout"
	KeyDeployPreviousReleaseHold = "deploy.previous_release_hold"
	KeyDeployStableWindow        = "deploy.stable_window"
	KeyDeployWatchWindow         = "deploy.watch_window"
	// KeyDeployCIGateMaxWaits caps the CI gate waits one app may hold at once, one per ref.
	KeyDeployCIGateMaxWaits = "deploy.ci_gate_max_waits_per_app"
	// KeyDeployReceiptApprovalWindow is the longest gap between an approval and the deploy it released.
	KeyDeployReceiptApprovalWindow = "deploy.receipt_approval_window"
	// KeyDeployProcfileDefaultPort is the port a Procfile web process gets, since a Procfile cannot declare one.
	KeyDeployProcfileDefaultPort = "deploy.procfile_default_port"
	minDeployDrainTimeout        = time.Second
	maxDeployDrainTimeout        = 10 * time.Minute
	maxDeployPreviousReleaseHold = time.Hour
	minDeployStableWindow        = time.Second
	maxDeployStableWindow        = 10 * time.Minute
	maxDeployWatchWindow         = 24 * time.Hour
)

func init() {
	Register(Definition{
		Key: KeyDeployProcfileDefaultPort, Category: CategoryDeploys, Kind: KindInt, Default: "8080",
		Env: "APP_DEPLOY_PROCFILE_DEFAULT_PORT", Apply: ApplyLive, CheckValue: intRangeCheck(1, 65535),
		Title:       "Procfile web port",
		Description: "The port given to the web process of an app deployed from a Procfile, since a Procfile cannot declare one, from 1 to 65535. The next deploy uses a changed value; apps already deployed keep their port.",
	})
	Register(Definition{
		Key: KeyDeployCIGateMaxWaits, Category: CategoryDeploys, Kind: KindInt, Default: "4",
		Env: "APP_CI_GATE_MAX_WAITS_PER_APP", Apply: ApplyLive, CheckValue: intRangeCheck(1, 50),
		Title:       "CI gate waits per app",
		Description: "How many refs of one app may wait for CI at once. A newer deploy of the same ref replaces the wait; one past the limit is refused and can be retried.",
	})
	Register(Definition{
		Key: KeyDeployReceiptApprovalWindow, Category: CategoryDeploys, Kind: KindDuration, Default: "15m",
		Env: "APP_DEPLOY_RECEIPT_APPROVAL_WINDOW", Apply: ApplyLive, CheckValue: durationRangeCheck(time.Minute, 24*time.Hour),
		Title:       "Deploy receipt approval window",
		Description: "How long before a deploy started an approval may have been decided for the receipt to name it as the approval that released that deploy, from 1m to 24h. A longer window can name an approval meant for an earlier deploy. The next receipt uses a changed value.",
	})
	Register(Definition{
		Key: KeyDeployDrainTimeout, Category: CategoryDeploys, Kind: KindDuration, Default: "15s",
		Env: "APP_DEPLOY_DRAIN_TIMEOUT", Apply: ApplyLive, CheckValue: durationRangeCheck(minDeployDrainTimeout, maxDeployDrainTimeout),
		Title:       "Deploy drain timeout",
		Description: "How long the previous release keeps serving after a cutover so in-flight requests finish, from 1s to 10m. Below the longest request your app serves, a deploy can cut requests short. Read on every reconcile pass, so it applies to the next deploy; a per-app override still wins.",
	})
	Register(Definition{
		Key: KeyDeployPreviousReleaseHold, Category: CategoryDeploys, Kind: KindDuration, Default: "3m",
		Env: "APP_DEPLOY_PREVIOUS_RELEASE_HOLD", Apply: ApplyLive, CheckValue: durationRangeCheck(0, maxDeployPreviousReleaseHold),
		Title:       "Previous release hold",
		Description: "How long the previous release stays running after a cutover as an instant rollback target, from 0 (remove at once) to 1h. The effective hold is never shorter than the drain timeout, and with auto-rollback on it also covers the watch window. Applies on the next deploy.",
	})
	Register(Definition{
		Key: KeyDeployStableWindow, Category: CategoryDeploys, Kind: KindDuration, Default: "10s",
		Env: "APP_DEPLOY_STABLE_WINDOW", Apply: ApplyLive, CheckValue: durationRangeCheck(minDeployStableWindow, maxDeployStableWindow),
		Title:       "Deploy stable window",
		Description: "How long a new container must stay running with no restart or OOM kill before it takes traffic, from 1s to 10m. Applies on the next deploy; a per-app override still wins.",
	})
	Register(Definition{
		Key: KeyDeployWatchWindow, Category: CategoryDeploys, Kind: KindDuration, Default: "5m",
		Env: "APP_DEPLOY_WATCH_WINDOW", Apply: ApplyLive, CheckValue: durationRangeCheck(0, maxDeployWatchWindow),
		Title:       "Post-deploy watch window",
		Description: "How long a new release is watched for restarts and 5xx before it counts as healthy, from 0 (no watch) to 24h. Auto-rollback needs the previous release held for this long. Applies to the next evaluation; a per-app override still wins.",
	})
}
