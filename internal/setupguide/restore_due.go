package setupguide

import "time"

// RestoreReason says why a restore drill is or is not due.
type RestoreReason string

// Restore drill reasons.
const (
	RestoreNoBackup        RestoreReason = "no_backup"
	RestoreNeverVerified   RestoreReason = "never_verified"
	RestoreVerificationOld RestoreReason = "verification_stale"
	RestoreOK              RestoreReason = "ok"
)

// RestoreDrill is the outcome of RestoreDrillDue.
type RestoreDrill struct {
	Due    bool          `json:"due"`
	Reason RestoreReason `json:"reason"`
	// DaysSince counts whole days since the last verified restore, or since
	// the last backup when none was ever verified. Zero for no_backup.
	DaysSince int `json:"days_since"`
}

// RestoreDrillDue reports whether a restore drill should be prompted. Zero
// times mean "never"; days comes from the caller's config or setting.
func RestoreDrillDue(lastBackup, lastVerified, now time.Time, days int) RestoreDrill {
	if lastBackup.IsZero() || lastBackup.After(now) {
		return RestoreDrill{Reason: RestoreNoBackup}
	}
	if lastVerified.IsZero() {
		return RestoreDrill{Due: true, Reason: RestoreNeverVerified, DaysSince: wholeDays(lastBackup, now)}
	}
	since := wholeDays(lastVerified, now)
	if lastVerified.After(now) {
		since = 0
	}
	if since > days {
		return RestoreDrill{Due: true, Reason: RestoreVerificationOld, DaysSince: since}
	}
	return RestoreDrill{Reason: RestoreOK, DaysSince: since}
}

func wholeDays(from, to time.Time) int {
	return int(to.Sub(from) / (24 * time.Hour))
}
