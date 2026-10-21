// Package mailtemplates renders the platform's emails: a token registry, a
// {{token}} substitution engine with no code execution, a small safe markup
// and one shared branded layout around every event template.
package mailtemplates

import "sort"

// Token scopes.
const (
	ScopeGlobal    = "global"
	ScopeRecipient = "recipient"
	ScopeEvent     = "event"
)

// Token is one substitutable value.
type Token struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Sample      string `json:"sample"`
	Scope       string `json:"scope"`
}

// registry is the single place tokens are defined; validation, the editor
// panel, preview samples and CLI help all read it.
var registry = []Token{
	{"brand_name", "Product name", "Acme Deploy", ScopeGlobal},
	{"support_email", "Support contact address", "support@example.com", ScopeGlobal},
	{"dashboard_url", "Dashboard address", "https://deploy.example.com", ScopeGlobal},
	{"instance_name", "Name of this instance", "deploy.example.com", ScopeGlobal},
	{"year", "Current year", "2026", ScopeGlobal},
	{"first_name", "Recipient first name", "Sam", ScopeRecipient},
	{"display_name", "Recipient display name", "Sam Rivera", ScopeRecipient},
	{"email", "Recipient email address", "sam@example.com", ScopeRecipient},
	{"role", "Recipient role", "admin", ScopeRecipient},
	{"app_name", "App name", "web", ScopeEvent},
	{"deploy_url", "Link to the deploy", "https://deploy.example.com/apps/web", ScopeEvent},
	{"deploy_status", "Deploy outcome", "succeeded", ScopeEvent},
	{"alert_name", "Alert rule name", "High memory", ScopeEvent},
	{"severity", "Alert severity", "warning", ScopeEvent},
	{"alert_summary", "What the alert saw", "Memory above 90% for 5 minutes", ScopeEvent},
	{"reset_link", "Password reset link", "https://deploy.example.com/reset-password?token=abc", ScopeEvent},
	{"invite_link", "Invite acceptance link", "https://deploy.example.com/accept-invite?token=abc", ScopeEvent},
	{"approve_link", "Approval link", "https://deploy.example.com/deploy-approvals", ScopeEvent},
	{"requester", "Who asked for the approval", "Alex Kim", ScopeEvent},
	{"sign_in_code", "One-time sign-in code", "482913", ScopeEvent},
	{"browser", "Browser or client description", "Firefox on macOS", ScopeEvent},
	{"ip_address", "Network address", "203.0.113.7", ScopeEvent},
	{"occurred_at", "When it happened", "Tue, 20 Oct 2026 09:30 UTC", ScopeEvent},
	{"disown_link", "Link to report a sign-in as not yours", "https://deploy.example.com/sign-in-alert?token=abc", ScopeEvent},
	{"new_email", "New email address", "sam.new@example.com", ScopeEvent},
	{"confirm_link", "Link to confirm a new email address", "https://deploy.example.com/settings/account#confirm_email=abc", ScopeEvent},
	{"domain", "Domain name", "app.example.com", ScopeEvent},
	{"issuer", "Certificate issuer", "Let's Encrypt", ScopeEvent},
	{"days_left", "Time left, as a phrase", "expires in 2 days", ScopeEvent},
	{"reason", "Why attention is needed", "Renewal is failing", ScopeEvent},
	{"primary_label", "Label of the main action", "Renew now", ScopeEvent},
	{"primary_url", "Dashboard link for the main action", "https://deploy.example.com/domains/app.example.com", ScopeEvent},
	{"inbox_url", "Dashboard link to the Inbox", "https://deploy.example.com/inbox", ScopeEvent},
	{"settings_url", "Dashboard link to notification settings", "https://deploy.example.com/settings/notification-channels", ScopeEvent},
	{"expires_in", "How long a link or code stays valid", "7 days", ScopeEvent},
}

// Tokens returns every registered token, sorted by scope then name.
func Tokens() []Token {
	out := append([]Token(nil), registry...)
	order := map[string]int{ScopeGlobal: 0, ScopeRecipient: 1, ScopeEvent: 2}
	sort.SliceStable(out, func(i, j int) bool {
		if order[out[i].Scope] != order[out[j].Scope] {
			return order[out[i].Scope] < order[out[j].Scope]
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// LookupToken finds a token by name.
func LookupToken(name string) (Token, bool) {
	for _, t := range registry {
		if t.Name == name {
			return t, true
		}
	}
	return Token{}, false
}

func tokensByScope(scopes ...string) []string {
	var out []string
	for _, t := range registry {
		for _, s := range scopes {
			if t.Scope == s {
				out = append(out, t.Name)
			}
		}
	}
	return out
}

// SampleValues returns every token's sample value, for previews.
func SampleValues() map[string]string {
	out := make(map[string]string, len(registry))
	for _, t := range registry {
		out[t.Name] = t.Sample
	}
	return out
}
