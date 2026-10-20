package envredact

import (
	"net/url"
	"strings"
)

// Marker replaces a known secret found inside free text.
const Marker = "[redacted]"

// Secrets replaces every non-empty secret found in s with Marker.
func Secrets(s string, secrets ...string) string {
	for _, v := range secrets {
		if v != "" {
			s = strings.ReplaceAll(s, v, Marker)
		}
	}
	return s
}

// URLUserinfo replaces the userinfo of raw with username, because a bare
// token username leaks just as a password does. A URL without userinfo is
// returned unchanged; a malformed one is an error.
func URLUserinfo(raw, username string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.User == nil {
		return raw, nil
	}
	u.User = url.User(username)
	return u.String(), nil
}

// URLForLog is URLUserinfo for log lines and error text: it never fails, and
// an unparseable URL is replaced whole because its parse error would echo it.
func URLForLog(raw string) string {
	out, err := URLUserinfo(raw, "redacted")
	if err != nil {
		return "<unparseable url>"
	}
	return out
}

// URLQuery replaces the value of every query key in keys (matched
// case-insensitively, keys lowercase) with mask. A malformed URL is
// returned as is.
func URLQuery(raw string, keys map[string]bool, mask string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	changed := false
	for key := range q {
		if keys[strings.ToLower(key)] {
			q.Set(key, mask)
			changed = true
		}
	}
	if changed {
		u.RawQuery = q.Encode()
	}
	return u.String()
}
