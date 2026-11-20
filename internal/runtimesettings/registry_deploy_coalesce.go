package runtimesettings

import "time"

// KeyDeployCoalesceWindow is the window in which a second deploy of the same commit is folded into the running one.
const KeyDeployCoalesceWindow = "deploys.coalesce_window"

func init() {
	Register(Definition{
		Key: KeyDeployCoalesceWindow, Category: CategoryDeploys, Kind: KindDuration, Default: "15s", Env: "APP_DEPLOY_COALESCE_WINDOW", Apply: ApplyLive,
		CheckValue: durationRangeCheck(time.Second, 10*time.Minute),
		Title:      "Deploy coalesce window",
		Description: "When a branch is still deploying a commit and the same commit arrives again inside this window " +
			"(for example a push and a variable change together), the repeat is skipped. From 1s to 10m. Applies to the next push.",
	})
}
