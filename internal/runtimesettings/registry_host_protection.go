package runtimesettings

import "time"

const (
	// KeyHostUncleanRestartWindow is how long an unclean restart stays in the attention feed and the doctor row.
	KeyHostUncleanRestartWindow = "host.unclean_restart_window"
	// KeyHostProtectedOOMScoreAdj is the OOM score adjustment at or below which the control plane counts as protected.
	KeyHostProtectedOOMScoreAdj = "host.protected_oom_score_adj"
)

func init() {
	Register(Definition{
		Key: KeyHostUncleanRestartWindow, Category: CategoryObservability, Kind: KindDuration, Default: "72h", Env: "APP_UNCLEAN_RESTART_WINDOW", Apply: ApplyLive,
		CheckValue: durationRangeCheck(time.Hour, 720*time.Hour),
		Title:      "Unclean restart memory", Description: "How long an unclean control plane restart stays in the attention feed and the protection check, from 1h to 720h. A change applies on the next refresh.",
	})
	Register(Definition{
		Key: KeyHostProtectedOOMScoreAdj, Category: CategoryObservability, Kind: KindInt, Default: "-500", Env: "APP_PROTECTED_OOM_SCORE_ADJ", Apply: ApplyLive,
		CheckValue: intRangeCheck(-1000, 0),
		Title:      "Protected OOM score", Description: "The OOM score adjustment at or below which the protection check counts the control plane as shielded from the kernel OOM killer, from -1000 to 0. A change applies on the next doctor run.",
	})
}
