package api

import (
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/alerting"
)

const redactedNotifyTarget = "(hidden)"

// redactNotifyTarget hides a notification destination from a caller
// without read:sensitive. A webhook URL, bot token, routing key or API
// key is a bearer secret (anyone holding it can post to the channel or
// page the on-call), so a plain read-tier caller sees only where it
// points, never the credential.
func redactNotifyTarget(kind alerting.NotifyKind, target string) string {
	if target == "" {
		return ""
	}
	if kind == alerting.NotifyEmail {
		return target
	}
	if u, err := url.Parse(target); err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") {
		return u.Scheme + "://" + u.Host + "/" + redactedNotifyTarget
	}
	return redactedNotifyTarget
}

// notifyTargetFor returns target as the caller may see it.
func (rt *Router) notifyTargetFor(r *http.Request, kind alerting.NotifyKind, target string) string {
	if rt.callerHasAbility(r, AbilityReadSensitive) {
		return target
	}
	return redactNotifyTarget(kind, target)
}
