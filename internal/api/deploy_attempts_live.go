package api

import (
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

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

// isRedeployOfEarlierImage reports an operator or automatic rollback. A manual
// deploy of an older tag is not one, so it keeps the manual label.
func isRedeployOfEarlierImage(a store.DeployAttempt, _ []store.DeployAttempt) bool {
	return a.Source == store.DeployAttemptSourceAutoRollback || strings.Contains(a.Reason, "RollbackTo:")
}
