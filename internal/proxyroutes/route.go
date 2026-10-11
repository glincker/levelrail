// Package proxyroutes writes and maintains route files in an external
// reverse proxy's watched configuration directory (Traefik's file provider),
// so domains reach this instance through a proxy that owns ports 80 and 443.
package proxyroutes

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

var (
	hostnameRE = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)
	nameRE     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,62}$`)
	labelRE    = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
)

// maxHostnameLen is the DNS limit on a full hostname.
const maxHostnameLen = 253

// Namespace scopes every file name and Traefik object this package owns.
// It is the lowercased brand short name, never a literal.
type Namespace string

// FilePrefix is the name prefix of every file this package may touch.
func (n Namespace) FilePrefix() string { return string(n) + "-managed-" }

// Header is the first line of every file this package writes. A file
// without it is never modified or removed, whatever its name.
func (n Namespace) Header() string { return "# managed by " + string(n) }

// ValidateDomain returns domain lowercased when it is a plain hostname that
// is safe to embed in a file name and a Traefik rule. Wildcards, ports,
// paths and anything that would need rewriting are rejected.
func ValidateDomain(domain string) (string, error) {
	d := strings.ToLower(domain)
	switch {
	case d == "":
		return "", errors.New("domain is empty")
	case strings.HasPrefix(d, "*."):
		return "", fmt.Errorf("wildcard domain %q is not supported for managed proxy routes", domain)
	case strings.ContainsAny(d, "/\\:` \t\r\n\x00"):
		return "", fmt.Errorf("domain %q contains a character that is not allowed in a hostname", domain)
	case len(d) > maxHostnameLen || !hostnameRE.MatchString(d):
		return "", fmt.Errorf("domain %q is not a valid hostname", domain)
	}
	return d, nil
}

// FileName is the file domain's route lives in. domain must already be
// validated; anything that would sanitise to a different name is refused.
func (n Namespace) FileName(domain string) (string, error) {
	d, err := ValidateDomain(domain)
	if err != nil {
		return "", err
	}
	if d != domain {
		return "", fmt.Errorf("domain %q must be lowercase", domain)
	}
	return n.FilePrefix() + d + ".yaml", nil
}

// objectName is the Traefik router, service and middleware stem. Dots become
// underscores, which is injective because hostnames never contain "_".
func (n Namespace) objectName(domain string) string {
	return n.FilePrefix() + strings.ReplaceAll(domain, ".", "_")
}

// RouterName is the Traefik name of domain's HTTPS router, as the Traefik
// API lists it (with the @file provider suffix).
func (n Namespace) RouterName(domain string) string {
	return n.objectName(domain) + "@file"
}

// Route is one domain's desired proxy route.
type Route struct {
	Domain          string
	EntrypointHTTP  string
	EntrypointHTTPS string
	// CertResolver empty means Traefik serves its default certificate.
	CertResolver string
	// Upstream is host:port of this instance's listener for the domain.
	Upstream string
}

// ValidateName checks an entrypoint or resolver name before it is written
// into YAML.
func ValidateName(kind, name string) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("%s %q must be letters, digits, '-' or '_'", kind, name)
	}
	return nil
}

// ValidateUpstream checks host:port before it is written into a URL.
func ValidateUpstream(hostport string) error {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return fmt.Errorf("upstream %q is not host:port", hostport)
	}
	if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("upstream port %q is not 1-65535", port)
	}
	if ValidateUpstreamHost(host) != nil {
		return fmt.Errorf("upstream host %q is not an IP address or hostname", host)
	}
	return nil
}

// ValidateUpstreamHost accepts an IP address or a hostname, including a
// single label such as "host.docker.internal" or "localhost".
func ValidateUpstreamHost(host string) error {
	if net.ParseIP(host) != nil {
		return nil
	}
	h := strings.ToLower(host)
	if h == "" || len(h) > maxHostnameLen || strings.HasPrefix(h, "*") {
		return fmt.Errorf("upstream host %q is not valid", host)
	}
	for _, label := range strings.Split(h, ".") {
		if !labelRE.MatchString(label) {
			return fmt.Errorf("upstream host %q is not valid", host)
		}
	}
	return nil
}

// Render returns the Traefik dynamic configuration file for r: an HTTPS
// router, an HTTP router that redirects to HTTPS, and one service.
func (n Namespace) Render(r Route) ([]byte, error) {
	d, err := ValidateDomain(r.Domain)
	if err != nil {
		return nil, err
	}
	if d != r.Domain {
		return nil, fmt.Errorf("domain %q must be lowercase", r.Domain)
	}
	if err := ValidateName("HTTP entrypoint", r.EntrypointHTTP); err != nil {
		return nil, err
	}
	if err := ValidateName("HTTPS entrypoint", r.EntrypointHTTPS); err != nil {
		return nil, err
	}
	if r.CertResolver != "" {
		if err := ValidateName("certificate resolver", r.CertResolver); err != nil {
			return nil, err
		}
	}
	if err := ValidateUpstream(r.Upstream); err != nil {
		return nil, err
	}
	obj := n.objectName(d)
	var b bytes.Buffer
	fmt.Fprintf(&b, "%s\n# Do not edit: rewritten on every reconcile. Remove the domain instead.\n", n.Header())
	b.WriteString("http:\n  routers:\n")
	fmt.Fprintf(&b, "    %s:\n      rule: Host(`%s`)\n      entryPoints: [%s]\n      service: %s\n", obj, d, r.EntrypointHTTPS, obj)
	if r.CertResolver != "" {
		fmt.Fprintf(&b, "      tls:\n        certResolver: %s\n", r.CertResolver)
	} else {
		b.WriteString("      tls: {}\n")
	}
	fmt.Fprintf(&b, "    %s-http:\n      rule: Host(`%s`)\n      entryPoints: [%s]\n      middlewares: [%s-https]\n      service: %s\n",
		obj, d, r.EntrypointHTTP, obj, obj)
	fmt.Fprintf(&b, "  middlewares:\n    %s-https:\n      redirectScheme:\n        scheme: https\n        permanent: true\n", obj)
	fmt.Fprintf(&b, "  services:\n    %s:\n      loadBalancer:\n        passHostHeader: true\n        servers:\n          - url: http://%s\n", obj, r.Upstream)
	return b.Bytes(), nil
}
