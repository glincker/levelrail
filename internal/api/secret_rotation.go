package api

import "time"

// defaultSecretRotationWarnAge is how old a secret's last-set value can
// get before GET /apps/{name}/secrets, GET .../env/all, and the doctor's
// stale_secrets check all flag it as due for rotation. Overridable via
// APP_SECRET_ROTATION_WARN_DAYS (cmd/levelrail/main.go's
// secretRotationWarnAge, WithSecretRotationWarnAge), the project's "no
// hardcoded thresholds" rule, the same env-var-with-default shape
// defaultAuditLogRetention (audit_retention.go) already establishes.
const defaultSecretRotationWarnAge = 90 * 24 * time.Hour

// effectiveSecretRotationWarnAge returns rt.secretRotationWarnAge if set,
// else defaultSecretRotationWarnAge, the same "0 means use the default"
// convention effectiveAuditLogRetention already establishes.
func (rt *Router) effectiveSecretRotationWarnAge() time.Duration {
	if rt.secretRotationWarnAge > 0 {
		return rt.secretRotationWarnAge
	}
	return defaultSecretRotationWarnAge
}

// secretIsStale reports whether updatedAt is older than the effective
// secret rotation warning threshold. A zero updatedAt (a value that was
// never actually set, which callers should not be passing here in the
// first place) is never stale: there is nothing yet to warn about.
func (rt *Router) secretIsStale(updatedAt time.Time) bool {
	if updatedAt.IsZero() {
		return false
	}
	return time.Since(updatedAt) > rt.effectiveSecretRotationWarnAge()
}
