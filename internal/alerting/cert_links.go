package alerting

import (
	"net/url"
	"strings"
)

// CertLinks are the dashboard deep links a cert alert offers; every field is empty when no public base URL is configured.
type CertLinks struct {
	Primary      string
	PrimaryLabel string
	Inbox        string
	Settings     string
}

// BuildCertLinks opens pages only: a signed-in user still has to click the fix. An empty or non-http base yields no links.
func BuildCertLinks(baseURL string, f CertFact, app string) CertLinks {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "localhost" {
		return CertLinks{}
	}
	q := url.Values{}
	if app != "" {
		q.Set("app", app)
	}
	q.Set("tab", "overview")
	q.Set("fix", f.Action)
	label := "Renew now"
	if f.Action == CertActionDNS {
		label = "Check DNS"
	}
	return CertLinks{
		Primary:      base + "/domains/" + url.PathEscape(f.Domain) + "?" + q.Encode(),
		PrimaryLabel: label,
		Inbox:        base + "/inbox",
		Settings:     base + "/settings/notification-channels",
	}
}
