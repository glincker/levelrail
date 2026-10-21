package spec

import (
	"fmt"
	"net/http"
	"strings"
)

// Header sides for HeaderNameProblem.
const (
	HeaderSideRequest  = "request"
	HeaderSideResponse = "response"
)

var (
	hopByHopHeaders = map[string]bool{
		"Connection": true, "Keep-Alive": true, "Proxy-Connection": true, "Te": true,
		"Trailer": true, "Transfer-Encoding": true, "Upgrade": true, "Content-Length": true,
		"Host": true,
	}
	requestForbiddenHeaders  = map[string]bool{"Forwarded": true, "X-Real-Ip": true, "Proxy-Authorization": true}
	responseForbiddenHeaders = map[string]bool{"X-Cache": true, "Cache-Status": true}
)

// IsToken reports whether s is an RFC 9110 token, the grammar for a header name.
func IsToken(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			continue
		}
		if !strings.ContainsRune("!#$%&'*+-.^_`|~", rune(c)) {
			return false
		}
	}
	return true
}

// SafeHeaderValue reports whether v can be written into a header without
// splitting it: no CR, LF or NUL, and no other control byte except tab.
func SafeHeaderValue(v string) bool {
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == '\t' {
			continue
		}
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}

// HeaderNameProblem returns why name cannot be used on side, or "".
func HeaderNameProblem(side, name string) string {
	if !IsToken(name) {
		return "must be a valid header name (letters, digits and !#$%&'*+-.^_`|~)"
	}
	canon := http.CanonicalHeaderKey(name)
	if hopByHopHeaders[canon] {
		return fmt.Sprintf("%s is managed by the ingress and cannot be changed here", canon)
	}
	if strings.HasPrefix(canon, "X-Forwarded-") {
		return "X-Forwarded-* headers are set by the ingress; use forwarded_prefix for X-Forwarded-Prefix"
	}
	if side == HeaderSideRequest && requestForbiddenHeaders[canon] {
		return fmt.Sprintf("%s is set by the ingress and cannot be changed here", canon)
	}
	if side == HeaderSideResponse && responseForbiddenHeaders[canon] {
		return fmt.Sprintf("%s is reserved for the ingress cache", canon)
	}
	return ""
}
