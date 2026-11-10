package platformimport

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// NotificationProvider is one alert provider read from the source. It never holds a token,
// webhook URL or password: those are read only at apply time through NotificationTarget.
type NotificationProvider struct {
	SourceID string
	Name     string
	// Kind is the source's provider type, lowercased.
	Kind string
	// Server is the non-secret base URL of a self-hosted provider (gotify, ntfy).
	Server string
	// Recipient is the email address for an email provider; it is not a credential.
	Recipient string
	// ExtraRecipients counts email addresses beyond the first.
	ExtraRecipients int
	// HasSecret is false when the source returned no usable token or masked one.
	HasSecret bool
	// HasHeaders is true for a custom webhook that sends extra headers, which do not carry over.
	HasHeaders bool
	// HasAccessToken is true for an ntfy provider protected by a token.
	HasAccessToken bool
	// Mismatch is true when the sub-object named by the kind is missing.
	Mismatch bool
}

type dokployNotification struct {
	ID    string `json:"notificationId"`
	Name  string `json:"name"`
	Type  string `json:"notificationType"`
	Slack *struct {
		WebhookURL string `json:"webhookUrl"`
	} `json:"slack"`
	Discord *struct {
		WebhookURL string `json:"webhookUrl"`
	} `json:"discord"`
	Lark *struct {
		WebhookURL string `json:"webhookUrl"`
	} `json:"lark"`
	Telegram *struct {
		BotToken        string `json:"botToken"`
		ChatID          string `json:"chatId"`
		MessageThreadID string `json:"messageThreadId"`
	} `json:"telegram"`
	Email *struct {
		ToAddresses []string `json:"toAddresses"`
	} `json:"email"`
	Gotify *struct {
		ServerURL string `json:"serverUrl"`
		AppToken  string `json:"appToken"`
	} `json:"gotify"`
	Ntfy *struct {
		ServerURL   string `json:"serverUrl"`
		Topic       string `json:"topic"`
		AccessToken string `json:"accessToken"`
	} `json:"ntfy"`
	Pushover *struct {
		UserKey  string `json:"userKey"`
		APIToken string `json:"apiToken"`
	} `json:"pushover"`
	Custom *struct {
		Endpoint string            `json:"endpoint"`
		Headers  map[string]string `json:"headers"`
	} `json:"custom"`
}

// ErrNoNotificationSecret means the source did not hand over a usable token or URL for a provider.
var ErrNoNotificationSecret = errors.New("the source returned no usable credentials for this notification provider")

func (s *DokploySource) readNotifications(ctx context.Context) ([]dokployNotification, error) {
	var out []dokployNotification
	if err := s.r.get(ctx, "/api/notification.all", nil, &out); err != nil {
		return nil, err
	}
	if len(out) > MaxPages {
		out = out[:MaxPages]
	}
	return out, nil
}

func (s *DokploySource) collectNotifications(ctx context.Context, d *Discovery) {
	list, err := s.readNotifications(ctx)
	if err != nil {
		d.Warnings = append(d.Warnings, "could not read notification providers: "+err.Error())
		return
	}
	for _, x := range list {
		d.Notifications = append(d.Notifications, describeNotification(x))
	}
}

// describeNotification keeps only what is safe to plan and print with, read from the
// sub-object its kind names; a missing sub-object marks the provider as mismatched.
func describeNotification(x dokployNotification) NotificationProvider {
	p := NotificationProvider{SourceID: x.ID, Name: x.Name, Kind: notificationKind(x)}
	target, err := buildNotificationTarget(x)
	p.HasSecret = err == nil && target.URL != ""
	p.Mismatch = !hasKindObject(x)
	switch p.Kind {
	case "email":
		if x.Email == nil {
			break
		}
		var to []string
		for _, a := range x.Email.ToAddresses {
			if a = strings.TrimSpace(a); a != "" {
				to = append(to, a)
			}
		}
		if len(to) > 0 {
			p.Recipient, p.ExtraRecipients = to[0], len(to)-1
		}
	case "gotify", "ntfy":
		p.Server = notificationServer(x)
		p.HasAccessToken = p.Kind == "ntfy" && x.Ntfy != nil && strings.TrimSpace(x.Ntfy.AccessToken) != ""
	case "custom":
		p.HasHeaders = x.Custom != nil && len(x.Custom.Headers) > 0
	}
	return p
}

func notificationKind(x dokployNotification) string {
	return strings.ToLower(strings.TrimSpace(x.Type))
}

// hasKindObject is true when the sub-object named by the provider's kind is present.
func hasKindObject(x dokployNotification) bool {
	switch notificationKind(x) {
	case "slack":
		return x.Slack != nil
	case "discord":
		return x.Discord != nil
	case "lark":
		return x.Lark != nil
	case "custom":
		return x.Custom != nil
	case "telegram":
		return x.Telegram != nil
	case "email":
		return x.Email != nil
	case "gotify":
		return x.Gotify != nil
	case "ntfy":
		return x.Ntfy != nil
	case "pushover":
		return x.Pushover != nil
	}
	return true
}

// notificationServer is the non-secret base URL of a gotify or ntfy provider.
func notificationServer(x dokployNotification) string {
	raw := ""
	switch notificationKind(x) {
	case "gotify":
		if x.Gotify != nil {
			raw = x.Gotify.ServerURL
		}
	case "ntfy":
		if x.Ntfy != nil {
			raw = x.Ntfy.ServerURL
		}
	}
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

// NotifyTarget is a provider's notify_url plus the non-secret parts an apply run compares
// with its plan. URL is a secret and never leaves the apply call.
type NotifyTarget struct {
	URL    string
	Kind   string
	Server string
}

// buildNotificationTarget packs the secret parts of the sub-object named by the kind into
// the notify_url shape the channel create call expects.
func buildNotificationTarget(x dokployNotification) (NotifyTarget, error) {
	out := NotifyTarget{Kind: notificationKind(x), Server: notificationServer(x)}
	use := func(v string) (string, bool) { v = strings.TrimSpace(v); return v, credentialUsable(v) }
	switch out.Kind {
	case "slack":
		if x.Slack != nil {
			out.URL, _ = use(x.Slack.WebhookURL)
		}
	case "discord":
		if x.Discord != nil {
			out.URL, _ = use(x.Discord.WebhookURL)
		}
	case "lark":
		if x.Lark != nil {
			out.URL, _ = use(x.Lark.WebhookURL)
		}
	case "custom":
		if x.Custom != nil {
			out.URL, _ = use(x.Custom.Endpoint)
		}
	case "telegram":
		out.URL = telegramTarget(x)
	case "gotify":
		if x.Gotify != nil {
			if tok, ok := use(x.Gotify.AppToken); ok {
				out.URL = out.Server + "/message?" + url.Values{"token": {tok}}.Encode()
			}
		}
	case "ntfy":
		out.URL = ntfyTarget(x, out.Server)
	case "pushover":
		if x.Pushover != nil {
			user, okU := use(x.Pushover.UserKey)
			tok, okT := use(x.Pushover.APIToken)
			if okU && okT {
				out.URL = "https://api.pushover.net/1/messages.json?" + url.Values{"token": {tok}, "user": {user}}.Encode()
			}
		}
	}
	if out.URL == "" || !credentialUsable(out.URL) {
		return NotifyTarget{}, ErrNoNotificationSecret
	}
	return out, nil
}

func telegramTarget(x dokployNotification) string {
	if x.Telegram == nil {
		return ""
	}
	tok := strings.TrimSpace(x.Telegram.BotToken)
	chat := strings.TrimSpace(x.Telegram.ChatID)
	if !credentialUsable(tok) || chat == "" || strings.ContainsAny(tok, "/?#") {
		return ""
	}
	q := url.Values{}
	q.Set("chat_id", chat)
	if t := strings.TrimSpace(x.Telegram.MessageThreadID); t != "" {
		q.Set("message_thread_id", t)
	}
	return "https://api.telegram.org/bot" + url.PathEscape(tok) + "/sendMessage?" + q.Encode()
}

func ntfyTarget(x dokployNotification, server string) string {
	if x.Ntfy == nil {
		return ""
	}
	topic := strings.TrimSpace(x.Ntfy.Topic)
	if topic == "" || strings.ContainsAny(topic, "/?#") {
		return ""
	}
	base := server + "/" + url.PathEscape(topic)
	tok := strings.TrimSpace(x.Ntfy.AccessToken)
	switch {
	case tok == "":
		return base
	case !credentialUsable(tok):
		return ""
	}
	return base + "?" + url.Values{"auth": {tok}}.Encode()
}

// NotificationTarget re-reads one provider for an apply run. Callers compare Kind and
// Server with the plan and pass URL straight to the channel create call, never to a log.
func (s *DokploySource) NotificationTarget(ctx context.Context, sourceID string) (NotifyTarget, error) {
	list, err := s.readNotifications(ctx)
	if err != nil {
		return NotifyTarget{}, err
	}
	for _, x := range list {
		if x.ID == sourceID {
			return buildNotificationTarget(x)
		}
	}
	return NotifyTarget{}, ErrNoNotificationSecret
}

// ValidateNotifyEndpoint applies the import network policy to a webhook or provider URL.
// Errors never echo the URL, since the path or query may carry a token.
func ValidateNotifyEndpoint(ctx context.Context, raw string, p NetworkPolicy) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("the URL is not absolute")
	}
	if u.Scheme != "https" {
		return errors.New("the URL must use https")
	}
	if u.User != nil || u.Fragment != "" {
		return errors.New("the URL must not carry credentials before the host or a fragment")
	}
	host := u.Hostname()
	if addr, err := netip.ParseAddr(host); err == nil {
		if p.Check(addr.Unmap()) != nil {
			return errors.New("the URL host is a blocked address")
		}
		return nil
	}
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addrs) == 0 {
		return errors.New("the URL host does not resolve")
	}
	for _, a := range addrs {
		if p.Check(a.Unmap()) != nil {
			return errors.New("the URL host resolves to a blocked address")
		}
	}
	return nil
}
