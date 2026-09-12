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

// resendEndpoint is Resend's fixed transactional-email endpoint. A
// resend notify_url is this URL plus key/to/from query params, matching
// internal/alerting/notify.go's own parseResendCreds convention.
const resendEndpoint = "https://api.resend.com/emails"

// buildResendNotifyURL packs a Resend API key and destination address
// (plus an optional from address) into the notify_url shape the control
// plane expects, the same convention buildPushoverNotifyURL uses above.
func buildResendNotifyURL(apiKey, to, from string) string {
	q := url.Values{}
	q.Set("key", apiKey)
	q.Set("to", to)
	if from != "" {
		q.Set("from", from)
	}
	return resendEndpoint + "?" + q.Encode()
}

// opsgenieEndpoint is Opsgenie's fixed Alerts API endpoint. An opsgenie
// notify_url is this URL plus a key query param, matching
// internal/alerting/notify.go's own parseOpsgenieCreds convention.
const opsgenieEndpoint = "https://api.opsgenie.com/v2/alerts"

// buildOpsgenieNotifyURL packs an Opsgenie API key into the notify_url
// shape the control plane expects, so an operator can pass it as its own
// flag instead of hand-building a query string.
func buildOpsgenieNotifyURL(apiKey string) string {
	q := url.Values{}
	q.Set("key", apiKey)
	return opsgenieEndpoint + "?" + q.Encode()
}
