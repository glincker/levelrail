package mailtemplates

import "sort"

// Event categories. System mail is security or account related and ignores
// the opt-out list; announcements honor it and carry an unsubscribe link.
const (
	CategorySystem       = "system"
	CategoryAnnouncement = "announcement"
)

// Template is the editable part of an email.
type Template struct {
	Subject   string `json:"subject"`
	Preheader string `json:"preheader"`
	Body      string `json:"body"`
}

// Event is one kind of email the platform sends.
type Event struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Tokens      []string `json:"tokens"`
	Default     Template `json:"default"`
}

func eventTokens(extra ...string) []string {
	return append(tokensByScope(ScopeGlobal, ScopeRecipient), extra...)
}

// Event keys.
const (
	EventPasswordReset      = "password_reset"
	EventUserInvite         = "user_invite"
	EventLoginCode          = "login_code"
	EventSignInAlert        = "signin_alert"
	EventEmailChanged       = "email_changed"
	EventEmailChangeConfirm = "email_change_confirm"
	EventAlertNotification  = "alert_notification"
	EventCertExpiry         = "cert_expiry_alert"
	EventCertExpiryPlain    = "cert_expiry_alert_plain"
	EventDeployNotification = "deploy_notification"
	EventDeployApproval     = "deploy_approval"
	EventTestEmail          = "test_email"
	EventAnnouncement       = "announcement"
	EventMaintenanceNotice  = "maintenance_notice"
)

var events = []Event{
	{
		Key: EventPasswordReset, Label: "Password reset", Category: CategorySystem,
		Description: "Sent when someone asks to reset their password.",
		Tokens:      eventTokens("reset_link", "expires_in"),
		Default: Template{
			Subject:   "[{{brand_name}}] Reset your password",
			Preheader: "Use this link to choose a new password.",
			Body:      "Someone asked to reset the password for your {{brand_name}} account.\n\n!button Choose a new password|{{reset_link}}\n\nThe link works once and expires in {{expires_in}}. If this was not you, ignore this email.",
		},
	},
	{
		Key: EventUserInvite, Label: "Invite", Category: CategorySystem,
		Description: "Sent when an administrator invites someone to this instance.",
		Tokens:      eventTokens("invite_link", "expires_in"),
		Default: Template{
			Subject:   "[{{brand_name}}] You're invited",
			Preheader: "Accept the invite and set a password.",
			Body:      "You have been invited to join {{brand_name}}.\n\n!button Accept the invite|{{invite_link}}\n\nThis link expires in {{expires_in}}.",
		},
	},
	{
		Key: EventLoginCode, Label: "Sign-in code", Category: CategorySystem,
		Description: "Sent with a one-time code when someone signs in by email.",
		Tokens:      eventTokens("sign_in_code", "browser", "ip_address", "occurred_at", "expires_in"),
		Default: Template{
			Subject:   "[{{brand_name}}] Your sign-in code",
			Preheader: "Your one-time code is {{sign_in_code}}.",
			Body:      "Someone asked to sign in to your {{brand_name}} account.\n\n# {{sign_in_code}}\n\nRequested from {{browser}} ({{ip_address}}) at {{occurred_at}}. The code expires in {{expires_in}} and only works in the browser that asked for it.\n\nIf this was not you, ignore this email and consider changing your password.",
		},
	},
	{
		Key: EventSignInAlert, Label: "New sign-in alert", Category: CategorySystem,
		Description: "Sent when an account signs in from a browser it has not used before.",
		Tokens:      eventTokens("browser", "ip_address", "occurred_at", "disown_link", "expires_in"),
		Default: Template{
			Subject:   "[{{brand_name}}] New sign-in from {{browser}}",
			Preheader: "Your account signed in from a new browser.",
			Body:      "Your {{brand_name}} account signed in from a browser it has not used before.\n\n- Browser: {{browser}}\n- Address: {{ip_address}}\n- Time: {{occurred_at}}\n\nIf this was you, there is nothing to do.\n\nIf it was not you, end that session and flag the account for a new password:\n\n!button This was not me|{{disown_link}}\n\nThe link works once and expires in {{expires_in}}.",
		},
	},
	{
		Key: EventEmailChanged, Label: "Email address changed", Category: CategorySystem,
		Description: "Sent to the old address when an account's email changes.",
		Tokens:      eventTokens("new_email"),
		Default: Template{
			Subject:   "[{{brand_name}}] Your email address changed",
			Preheader: "The address on your account was changed.",
			Body:      "The email address on your {{brand_name}} account was changed to {{new_email}}.\n\nIf this was not you, contact an administrator.",
		},
	},
	{
		Key: EventEmailChangeConfirm, Label: "Confirm new email address", Category: CategorySystem,
		Description: "Sent to a new address to confirm an email change.",
		Tokens:      eventTokens("confirm_link", "expires_in"),
		Default: Template{
			Subject:   "[{{brand_name}}] Confirm your new email",
			Preheader: "Confirm that this is your new address.",
			Body:      "Confirm that this is your new {{brand_name}} email address.\n\n!button Confirm this address|{{confirm_link}}\n\nThe link expires in {{expires_in}} and works only while you are signed in. If you did not ask for this, ignore this message.",
		},
	},
	{
		Key: EventAlertNotification, Label: "Alert notification", Category: CategorySystem,
		Description: "Sent when an alert rule fires or resolves.",
		Tokens:      eventTokens("alert_name", "severity", "alert_summary", "app_name"),
		Default: Template{
			Subject:   "[{{brand_name}}] {{alert_name}}",
			Preheader: "{{severity}}: {{alert_summary}}",
			Body:      "> **{{alert_name}}** ({{severity}})\n\n{{alert_summary}}\n\nApp: {{app_name}}\n\n!button Open the dashboard|{{dashboard_url}}",
		},
	},
	{
		Key: EventCertExpiry, Label: "Certificate expiry alert", Category: CategorySystem,
		Description: "Sent when a certificate needs attention and the rule has email enabled.",
		Tokens:      eventTokens("domain", "issuer", "days_left", "reason", "primary_label", "primary_url", "inbox_url", "settings_url"),
		Default: Template{
			Subject:   "[{{brand_name}}] Certificate for {{domain}} {{days_left}}",
			Preheader: "{{reason}}",
			Body:      "> **{{domain}}** {{days_left}}\n\nIssuer: {{issuer}}\n\nWhy: {{reason}}\n\n!button {{primary_label}}|{{primary_url}}\n\n!button Open in Inbox|{{inbox_url}}\n\n!button Notification settings|{{settings_url}}",
		},
	},
	{
		Key: EventCertExpiryPlain, Label: "Certificate expiry alert (no links)", Category: CategorySystem,
		Description: "Used when the instance has no public address, so the email carries no dashboard links.",
		Tokens:      eventTokens("domain", "issuer", "days_left", "reason"),
		Default: Template{
			Subject:   "[{{brand_name}}] Certificate for {{domain}} {{days_left}}",
			Preheader: "{{reason}}",
			Body:      "> **{{domain}}** {{days_left}}\n\nIssuer: {{issuer}}\n\nWhy: {{reason}}\n\nOpen the dashboard to renew or check DNS.",
		},
	},
	{
		Key: EventDeployNotification, Label: "Deploy notification", Category: CategorySystem,
		Description: "Sent when a deploy finishes.",
		Tokens:      eventTokens("app_name", "deploy_status", "deploy_url"),
		Default: Template{
			Subject:   "[{{brand_name}}] {{app_name}} deploy {{deploy_status}}",
			Preheader: "The deploy of {{app_name}} {{deploy_status}}.",
			Body:      "The deploy of **{{app_name}}** {{deploy_status}}.\n\n!button View the deploy|{{deploy_url}}",
		},
	},
	{
		Key: EventDeployApproval, Label: "Deploy approval request", Category: CategorySystem,
		Description: "Sent to approvers when a deploy waits for approval.",
		Tokens:      eventTokens("app_name", "requester", "approve_link"),
		Default: Template{
			Subject:   "[{{brand_name}}] Approval needed for {{app_name}}",
			Preheader: "{{requester}} is waiting for an approval.",
			Body:      "{{requester}} asked to deploy **{{app_name}}** and needs an approval.\n\n!button Review the request|{{approve_link}}",
		},
	},
	{
		Key: EventTestEmail, Label: "Test email", Category: CategorySystem,
		Description: "Sent when an operator tests the email settings.",
		Tokens:      eventTokens(),
		Default: Template{
			Subject:   "[{{brand_name}}] Test email",
			Preheader: "Email delivery works.",
			Body:      "This is a test message from {{brand_name}} ({{instance_name}}). If you can read it, email delivery works.",
		},
	},
	{
		Key: EventAnnouncement, Label: "Announcement", Category: CategoryAnnouncement,
		Description: "A general message to a group of users.",
		Tokens:      eventTokens(),
		Default: Template{
			Subject:   "[{{brand_name}}] An update from {{instance_name}}",
			Preheader: "A short update for {{first_name}}.",
			Body:      "Hi {{first_name}},\n\nWrite your update here.\n\nThe {{brand_name}} team",
		},
	},
	{
		Key: EventMaintenanceNotice, Label: "Maintenance notice", Category: CategoryAnnouncement,
		Description: "Plan ahead: tell users about upcoming maintenance.",
		Tokens:      eventTokens(),
		Default: Template{
			Subject:   "[{{brand_name}}] Planned maintenance on {{instance_name}}",
			Preheader: "Short interruptions are possible.",
			Body:      "Hi {{first_name}},\n\nWe are doing planned maintenance on {{instance_name}}. Apps keep running, but the dashboard may be unavailable for a few minutes.\n\n- When: (add a time)\n- Expected impact: (describe it)\n\nQuestions? Write to {{support_email}}.",
		},
	},
}

// Events returns every event definition, in a stable order.
func Events() []Event {
	return append([]Event(nil), events...)
}

// LookupEvent finds an event by key.
func LookupEvent(key string) (Event, bool) {
	for _, e := range events {
		if e.Key == key {
			return e, true
		}
	}
	return Event{}, false
}

// EventKeys returns the sorted event keys.
func EventKeys() []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Key
	}
	sort.Strings(out)
	return out
}

// ValidateTemplate checks the subject, preheader and body against the
// tokens e supports.
func ValidateTemplate(e Event, t Template) error {
	if t.Subject == "" {
		return errEmptySubject
	}
	if len(t.Subject) > 300 || len(t.Preheader) > 300 || len(t.Body) > 50000 {
		return errTooLong
	}
	for _, text := range []string{t.Subject, t.Preheader, t.Body} {
		if err := ValidateTokens(text, e.Tokens); err != nil {
			return err
		}
	}
	return nil
}
