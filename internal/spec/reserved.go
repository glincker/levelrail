package spec

import (
	"errors"
	"fmt"
	"strings"
)

// ErrReservedNamespace is returned when app scoped code names a control plane namespace.
var ErrReservedNamespace = errors.New("secrets: namespace is reserved for the control plane")

// AppNamespace refuses a reserved name. It lives here so packages that only need the guard do not import internal/secrets.
func AppNamespace(name string) error {
	if IsReservedName(name) {
		return fmt.Errorf("%w: %q", ErrReservedNamespace, name)
	}
	return nil
}

// reservedNames are the control plane secret namespaces that have no slash,
// so an app name could otherwise take one and resolve its secrets. This is the
// single list every service name writer consults; TestReservedNamesCoverSecretNamespaces keeps it complete.
var reservedNames = map[string]bool{
	"build-records": true, "auth-engine": true, "webpush": true, "heartbeat": true,
	"heartbeat-jobs": true, "token-lifecycle": true, "secretconnections": true, "secretproviders": true, "ddns": true,
	"vault": true, "email-settings": true, "mail-system-email": true, "ai-assistant": true,
	"cloudflare-tunnel": true, "cloudflare-dns": true, "google-cloud-dns": true, "route53-dns": true,
	"builtin-registry": true, "traffic-export": true,
	"dns-digitalocean": true, "dns-hetzner": true, "dns-azure": true, "dns-porkbun": true, "dns-desec": true,
	"dns-godaddy": true, "dns-namecheap": true, "dns-cloudflare": true, "dns-route53": true, "dns-googleclouddns": true,
}

// reservedPrefixes are namespace families keyed by an id appended to the prefix.
var reservedPrefixes = []string{"ai-assistant-provider-", "dns-", "monitor-"}

// IsReservedName reports whether name is, or could alias, a control plane secrets namespace.
func IsReservedName(name string) bool {
	if reservedNames[name] {
		return true
	}
	for _, p := range reservedPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// ReservedNameCovers reports whether a namespace literal is reserved exactly or falls under a reserved prefix.
func ReservedNameCovers(lit string) bool {
	return IsReservedName(lit)
}
