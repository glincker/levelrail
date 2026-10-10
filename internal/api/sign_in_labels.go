package api

import (
	"net"
	"strings"
	"unicode"
)

const (
	unknownBrowserLabel = "Unknown browser"
	maxSignInTextLen    = 64
)

// Checked in order: Edge and Opera carry a Chrome token, Chrome carries Safari.
var browserLabelRules = []struct{ token, label string }{
	{"edg/", "Edge"},
	{"opr/", "Opera"},
	{"firefox/", "Firefox"},
	{"fxios/", "Firefox"},
	{"crios/", "Chrome"},
	{"chrome/", "Chrome"},
	{"safari/", "Safari"},
	{"curl/", "curl"},
}

// iPad and iPhone user agents also say "Mac OS X", so iOS comes first.
var osLabelRules = []struct{ token, label string }{
	{"windows", "Windows"},
	{"iphone", "iOS"},
	{"ipad", "iOS"},
	{"android", "Android"},
	{"cros", "ChromeOS"},
	{"mac os x", "macOS"},
	{"macintosh", "macOS"},
	{"linux", "Linux"},
}

// browserLabel turns a raw User-Agent into "Browser on OS" built only from
// fixed words, so a requester cannot put their own text in front of the
// account owner.
func browserLabel(ua string) string {
	lower := strings.ToLower(ua)
	switch clientKindFromUserAgent(ua) {
	case ClientKindCLI:
		return "Command line client"
	case ClientKindMCP:
		return "MCP client"
	}
	browser := unknownBrowserLabel
	for _, r := range browserLabelRules {
		if strings.Contains(lower, r.token) {
			browser = r.label
			break
		}
	}
	for _, r := range osLabelRules {
		if strings.Contains(lower, r.token) {
			return browser + " on " + r.label
		}
	}
	return browser
}

// safeSignInText makes stored requester context safe to show in an email
// or the dashboard: no control characters or newlines, no URLs, capped.
func safeSignInText(s string) string {
	var b strings.Builder
	for _, field := range strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		lower := strings.ToLower(field)
		if strings.Contains(lower, "://") || strings.HasPrefix(lower, "www.") || strings.ContainsAny(field, "<>@") {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(field)
	}
	out := []rune(b.String())
	if len(out) > maxSignInTextLen {
		out = out[:maxSignInTextLen]
	}
	return string(out)
}

// safeSignInIP shows an address only when it parses as one.
func safeSignInIP(ip string) string {
	if parsed := net.ParseIP(strings.TrimSpace(ip)); parsed != nil {
		return parsed.String()
	}
	return "unknown address"
}
