package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/alerting"
)

const redactedNotifyTarget = alerting.MaskedChannelSecret

// redactNotifyTarget hides a notification destination from a caller
// without read:sensitive: a webhook URL, bot token, routing key or API
// key is a bearer secret, so only where it points is shown.
func redactNotifyTarget(kind alerting.NotifyKind, target string) string {
	return alerting.MaskNotifyTarget(kind, target)
}

// notifyTargetFor returns target as the caller may see it.
func (rt *Router) notifyTargetFor(r *http.Request, kind alerting.NotifyKind, target string) string {
	if rt.callerHasAbility(r, AbilityReadSensitive) {
		return target
	}
	return redactNotifyTarget(kind, target)
}
