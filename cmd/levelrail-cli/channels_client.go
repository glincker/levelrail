package main

import "net/url"

// pushoverEndpoint is Pushover's fixed Message API endpoint. A pushover
// notify_url is this URL plus token/user query params, matching
// internal/alerting/notify.go's own parsePushoverCreds convention.
const pushoverEndpoint = "https://api.pushover.net/1/messages.json"

// buildPushoverNotifyURL packs a Pushover user key and API token into
// the notify_url shape the control plane expects, so an operator can
// pass the two Pushover credentials as separate flags instead of
// hand-building a query string.
func buildPushoverNotifyURL(userKey, apiToken string) string {
	q := url.Values{}
	q.Set("token", apiToken)
	q.Set("user", userKey)
	return pushoverEndpoint + "?" + q.Encode()
}
