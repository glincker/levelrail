package trafficpolicy

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Header rule sides and operations.
const (
	SideRequest  = "request"
	SideResponse = "response"

	OpSet    = "set"
	OpAdd    = "add"
	OpRemove = "remove"
)

// HeaderRule is one ordered header change. Response rules are applied after
// the app answered, so they win over the app's own value.
type HeaderRule struct {
	Side  string `json:"side"`
	Op    string `json:"op"`
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
}

// SecurityPreset is the one click set of browser hardening headers.
type SecurityPreset struct {
	// HSTS needs a publicly trusted certificate; validation refuses it otherwise.
	HSTS                  bool   `json:"hsts"`
	HSTSMaxAge            int    `json:"hsts_max_age,omitempty"`
	HSTSIncludeSubdomains bool   `json:"hsts_include_subdomains,omitempty"`
	ContentTypeOptions    bool   `json:"content_type_options"`
	FrameOptions          string `json:"frame_options,omitempty"`
	ReferrerPolicy        string `json:"referrer_policy,omitempty"`
	PermissionsPolicy     string `json:"permissions_policy,omitempty"`
}

// CORSPreset answers cross origin requests from Origins at the ingress.
type CORSPreset struct {
	Origins       []string `json:"origins"`
	Methods       []string `json:"methods,omitempty"`
	Headers       []string `json:"headers,omitempty"`
	ExposeHeaders []string `json:"expose_headers,omitempty"`
	Credentials   bool     `json:"credentials,omitempty"`
	MaxAgeSeconds int      `json:"max_age_seconds,omitempty"`
	// Preflight answers OPTIONS preflight requests at the ingress instead of
	// passing them to the app.
	Preflight bool `json:"preflight"`
}

// Headers is a domain's header policy.
type Headers struct {
	Rules    []HeaderRule    `json:"rules"`
	Security *SecurityPreset `json:"security,omitempty"`
	CORS     *CORSPreset     `json:"cors,omitempty"`
	// HideServer removes Server and X-Powered-By from every response.
	HideServer bool `json:"hide_server,omitempty"`
	// ForwardedPrefix sets X-Forwarded-Prefix on requests, the only
	// X-Forwarded-* header an operator may set.
	ForwardedPrefix string `json:"forwarded_prefix,omitempty"`
}

// DefaultHSTSMaxAge is one year, the preload list minimum.
const DefaultHSTSMaxAge = 31536000

// DefaultSecurityPreset is what the dashboard's "Security headers" button applies.
func DefaultSecurityPreset(tlsReal bool) SecurityPreset {
	return SecurityPreset{
		HSTS:               tlsReal,
		HSTSMaxAge:         DefaultHSTSMaxAge,
		ContentTypeOptions: true,
		FrameOptions:       "SAMEORIGIN",
		ReferrerPolicy:     "strict-origin-when-cross-origin",
		PermissionsPolicy:  "camera=(), microphone=(), geolocation=()",
	}
}

var (
	hopByHop = map[string]bool{
		"Connection": true, "Keep-Alive": true, "Proxy-Connection": true, "Te": true,
		"Trailer": true, "Transfer-Encoding": true, "Upgrade": true, "Content-Length": true,
		"Host": true,
	}
	requestForbidden = map[string]bool{
		"Forwarded": true, "X-Real-Ip": true, "Proxy-Authorization": true,
	}
	responseForbidden = map[string]bool{"X-Cache": true, "Cache-Status": true}
	referrerPolicies  = map[string]bool{
		"no-referrer": true, "no-referrer-when-downgrade": true, "origin": true,
		"origin-when-cross-origin": true, "same-origin": true, "strict-origin": true,
		"strict-origin-when-cross-origin": true, "unsafe-url": true,
	}
	corsMethods = map[string]bool{
		"GET": true, "HEAD": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "OPTIONS": true,
	}
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

// headerNameProblem returns why name cannot be used on side, or "".
func headerNameProblem(side, name string) string {
	if !IsToken(name) {
		return "must be a valid header name (letters, digits and !#$%&'*+-.^_`|~)"
	}
	canon := http.CanonicalHeaderKey(name)
	if hopByHop[canon] {
		return fmt.Sprintf("%s is managed by the ingress and cannot be changed here", canon)
	}
	if strings.HasPrefix(canon, "X-Forwarded-") {
		return "X-Forwarded-* headers are set by the ingress; use forwarded_prefix for X-Forwarded-Prefix"
	}
	if side == SideRequest && requestForbidden[canon] {
		return fmt.Sprintf("%s is set by the ingress and cannot be changed here", canon)
	}
	if side == SideResponse && responseForbidden[canon] {
		return fmt.Sprintf("%s is reserved for the ingress cache", canon)
	}
	return ""
}

// Validate checks every rule and preset.
func (h *Headers) Validate(l Limits, c Context) error {
	col := &collector{kind: KindHeaders}
	if len(h.Rules) > l.MaxHeaderRules {
		col.add("rules", "at most %d header rules per domain", l.MaxHeaderRules)
	}
	for i, r := range h.Rules {
		f := fmt.Sprintf("rules[%d]", i)
		if r.Side != SideRequest && r.Side != SideResponse {
			col.add(f+".side", "must be request or response")
		}
		switch r.Op {
		case OpSet, OpAdd:
			if r.Value == "" {
				col.add(f+".value", "is required for %s", r.Op)
			}
		case OpRemove:
			if r.Value != "" {
				col.add(f+".value", "must be empty for remove")
			}
		default:
			col.add(f+".op", "must be set, add or remove")
		}
		if p := headerNameProblem(r.Side, r.Name); p != "" {
			col.add(f+".name", "%s", p)
		}
		if len(r.Value) > l.MaxHeaderValueLen {
			col.add(f+".value", "must be at most %d bytes", l.MaxHeaderValueLen)
		}
		if !SafeHeaderValue(r.Value) {
			col.add(f+".value", "must not contain line breaks or control characters")
		}
	}
	if s := h.Security; s != nil {
		validateSecurity(col, s, c)
	}
	if co := h.CORS; co != nil {
		validateCORS(col, co, l)
	}
	if h.ForwardedPrefix != "" && (!strings.HasPrefix(h.ForwardedPrefix, "/") || !SafeHeaderValue(h.ForwardedPrefix) || len(h.ForwardedPrefix) > 256) {
		col.add("forwarded_prefix", "must be a path starting with / (at most 256 bytes)")
	}
	return col.err()
}

func validateSecurity(col *collector, s *SecurityPreset, c Context) {
	if s.HSTS && !c.TLSReal {
		col.add("security.hsts", "needs a publicly trusted certificate for this domain (enable ACME, upload a certificate, or terminate TLS at your proxy); browsers would otherwise lock visitors out")
	}
	if s.HSTSMaxAge < 0 || s.HSTSMaxAge > 2*DefaultHSTSMaxAge {
		col.add("security.hsts_max_age", "must be between 0 and %d seconds", 2*DefaultHSTSMaxAge)
	}
	if s.FrameOptions != "" && s.FrameOptions != "DENY" && s.FrameOptions != "SAMEORIGIN" {
		col.add("security.frame_options", "must be DENY, SAMEORIGIN or empty")
	}
	if s.ReferrerPolicy != "" && !referrerPolicies[s.ReferrerPolicy] {
		col.add("security.referrer_policy", "is not a valid Referrer-Policy value")
	}
	if !SafeHeaderValue(s.PermissionsPolicy) || len(s.PermissionsPolicy) > 1024 {
		col.add("security.permissions_policy", "must be a single line of at most 1024 bytes")
	}
}

func validateCORS(col *collector, co *CORSPreset, l Limits) {
	if len(co.Origins) == 0 {
		col.add("cors.origins", "list at least one origin, or * for any")
	}
	if len(co.Origins) > l.MaxListEntries {
		col.add("cors.origins", "at most %d origins", l.MaxListEntries)
	}
	for i, o := range co.Origins {
		if o == "*" {
			if co.Credentials {
				col.add(fmt.Sprintf("cors.origins[%d]", i), "* cannot be combined with credentials; list the origins instead")
			}
			continue
		}
		if !ValidOrigin(o) {
			col.add(fmt.Sprintf("cors.origins[%d]", i), "must be an origin like https://app.example.com (scheme and host, no path)")
		}
	}
	for i, m := range co.Methods {
		if !corsMethods[m] {
			col.add(fmt.Sprintf("cors.methods[%d]", i), "must be one of GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
		}
	}
	for i, h := range append(append([]string{}, co.Headers...), co.ExposeHeaders...) {
		if !IsToken(h) {
			col.add(fmt.Sprintf("cors.headers[%d]", i), "must be a valid header name")
		}
	}
	if len(co.Headers)+len(co.ExposeHeaders) > l.MaxListEntries {
		col.add("cors.headers", "at most %d headers", l.MaxListEntries)
	}
	if co.MaxAgeSeconds < 0 || co.MaxAgeSeconds > 86400 {
		col.add("cors.max_age_seconds", "must be between 0 and 86400")
	}
}

// ValidOrigin reports whether o is a bare http(s) origin.
func ValidOrigin(o string) bool {
	u, err := url.Parse(o)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	return (u.Path == "" || u.Path == "/") && u.RawQuery == "" && u.Fragment == "" && u.User == nil &&
		!strings.ContainsAny(o, "'\"\\ {}")
}
