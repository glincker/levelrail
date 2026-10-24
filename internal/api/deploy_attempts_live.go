package api

import "github.com/GLINCKER/levelrail/internal/store"

// newestSucceededID is the one attempt that is live: attempts are newest
// first, and a rollback re-deploys an older image, so image equality alone
// would mark every attempt of that image as live.
func newestSucceededID(attempts []store.DeployAttempt) string {
	for _, a := range attempts {
		if a.Status == store.DeployAttemptStatusSucceeded {
			return a.ID
		}
	}
	return ""
}

// isRedeployOfEarlierImage mirrors the deployments list rule: an image deploy that
// re-deployed an image which succeeded earlier, after a different image
// succeeded in between. attempts must be newest first.
func isRedeployOfEarlierImage(a store.DeployAttempt, attempts []store.DeployAttempt) bool {
	if a.Source == store.DeployAttemptSourceAutoRollback {
		return true
	}
	if a.Source != store.DeployAttemptSourceImage {
		return false
	}
	for _, e := range attempts {
		if e.ID == a.ID || e.Status != store.DeployAttemptStatusSucceeded || e.Image != a.Image || !e.StartedAt.Before(a.StartedAt) {
			continue
		}
		for _, m := range attempts {
			if m.Status == store.DeployAttemptStatusSucceeded && m.Image != a.Image && m.StartedAt.After(e.StartedAt) && m.StartedAt.Before(a.StartedAt) {
				return true
			}
		}
	}
	return false
}
