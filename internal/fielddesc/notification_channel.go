package fielddesc

// NotificationChannel is the resource name of the alert and deploy notification channel form.
const NotificationChannel = "notification-channels"

func init() {
	webhook := []string{"slack", "discord", "telegram", "teams", "mattermost", "lark", "rocketchat", "webex", "googlechat", "generic"}
	kinds := []string{"slack", "discord", "telegram", "teams", "mattermost", "lark", "rocketchat", "webex", "googlechat", "generic", "email", "pushover", "pagerduty", "opsgenie", "resend", "ntfy", "gotify", "webpush"}
	when := func(k ...string) []Condition { return []Condition{{Field: "kind", Equals: k}} }
	f := func(id, key string, t Type, req bool, show []Condition, flag, doc string) Field {
		p := "integrations:fields.notificationChannel." + id
		return Field{ID: id, Key: key, Type: t, LabelKey: p + ".label", HelpKey: p + ".help", Required: req, Secret: t == TypeSecret, ShowWhen: show, Flag: flag, Doc: doc}
	}
	ph := func(x Field, placeholder, helpURL string) Field {
		x.Placeholder, x.HelpURL = placeholder, helpURL
		return x
	}
	kind := f("kind", "kind", TypeSelect, true, nil, "kind", "channel kind, one of the listed kinds")
	kind.Options = kinds
	kind.Default = "slack"
	device := f("notify_device_login", "notify_device_login", TypeBool, false, nil, "notify-device-login", "send a link-only notice here when a CLI login is waiting for approval (never includes the code)")
	expired := f("notify_device_login_expired", "notify_device_login_expired", TypeBool, false, nil, "notify-device-login-expired", "send a link-only notice here when a pending CLI login expires unapproved (never includes the code)")
	enabled := f("enabled", "enabled", TypeBool, false, nil, "", "channel delivery on or paused (CLI: --disabled creates it paused)")
	enabled.Default, enabled.FormOnly = "true", true

	Register(Resource{Name: NotificationChannel, Docs: Docs{File: "notification-channel-fields.md", Title: "Notification channel fields", Description: "Every field of a notification channel with its type, requirement, visibility rule and CLI flag."}, Fields: []Field{
		kind,
		ph(f("name", "name", TypeString, true, nil, "name", "display name for the channel"), "e.g. Team Slack", ""),
		ph(f("notify_url_webhook", "notify_url", TypeString, true, when(webhook...), "notify-url", "destination: webhook URL, or a Telegram sendMessage URL with chat_id and an optional message_thread_id topic"), "https://hooks.example.com/...", ""),
		ph(f("notify_url_email", "notify_url", TypeString, true, when("email"), "", "destination email address"), "ops@example.com", ""),
		ph(f("notify_url_ntfy", "notify_url", TypeString, true, when("ntfy"), "", "ntfy topic URL"), "https://ntfy.sh/my-topic", "https://docs.ntfy.sh/publish/"),
		ph(f("notify_url_gotify", "notify_url", TypeString, true, when("gotify"), "", "Gotify message endpoint URL"), "https://gotify.example.com/message?token=...", "https://gotify.net/docs/pushmsg"),
		ph(f("pushover_user_key", "pushover_user_key", TypeString, true, when("pushover"), "pushover-user-key", "Pushover User Key (an alternative to notify-url)"), "uQiRzpo4DXghDmr9QzzfQu27cmVRsG", ""),
		ph(f("pushover_api_token", "pushover_api_token", TypeString, true, when("pushover"), "pushover-api-token", "Pushover Application API Token (an alternative to notify-url)"), "azGDORePK8gMaC0QOYAMyEEuzJnyUi", "https://pushover.net/api#registration"),
		ph(f("pagerduty_routing_key", "pagerduty_routing_key", TypeString, true, when("pagerduty"), "pagerduty-routing-key", "PagerDuty Events API v2 Integration/Routing Key (an alternative to notify-url)"), "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4", "https://support.pagerduty.com/docs/services-and-integrations#events-api-v2"),
		ph(f("resend_api_key", "resend_api_key", TypeString, true, when("resend"), "resend-api-key", "Resend API Key (an alternative to notify-url)"), "re_123456789", ""),
		ph(f("resend_to", "resend_to", TypeString, true, when("resend"), "resend-to", "Resend destination email address"), "ops@example.com", ""),
		ph(f("resend_from", "resend_from", TypeString, false, when("resend"), "resend-from", "Resend from address, defaults to onboarding@resend.dev"), "alerts@yourdomain.com", "https://resend.com/docs/dashboard/domains/introduction"),
		ph(f("opsgenie_api_key", "opsgenie_api_key", TypeString, true, when("opsgenie"), "opsgenie-api-key", "Opsgenie API Key (an alternative to notify-url)"), "00000000-0000-0000-0000-000000000000", "https://support.atlassian.com/opsgenie/docs/api-integration/"),
		enabled,
		device,
		expired,
	}})
}
