// Package proxycoexist helps run the dashboard behind a reverse proxy that
// already owns ports 80 and 443 (Traefik, Caddy, nginx), which is the normal
// situation on a server migrating off another platform.
package proxycoexist

import (
	"fmt"
	"net"
	"strings"
)

// Kind is a reverse proxy family we can generate configuration for.
type Kind string

// Supported proxy kinds.
const (
	Traefik Kind = "traefik"
	Nginx   Kind = "nginx"
	Caddy   Kind = "caddy"
)

// ParseKind validates a --proxy value.
func ParseKind(s string) (Kind, error) {
	switch Kind(strings.ToLower(strings.TrimSpace(s))) {
	case Traefik:
		return Traefik, nil
	case Nginx:
		return Nginx, nil
	case Caddy:
		return Caddy, nil
	}
	return "", fmt.Errorf("proxy must be traefik, nginx or caddy")
}

// KindFromImage guesses the proxy family from a container image name.
func KindFromImage(image string) (Kind, bool) {
	i := strings.ToLower(image)
	switch {
	case strings.Contains(i, "traefik"):
		return Traefik, true
	case strings.Contains(i, "nginx"):
		return Nginx, true
	case strings.Contains(i, "caddy"):
		return Caddy, true
	}
	return "", false
}

// Holder is a container publishing a host port the ingress would want.
type Holder struct {
	Port      int    `json:"port"`
	Container string `json:"container"`
	Image     string `json:"image"`
	Kind      Kind   `json:"kind,omitempty"`
	// NetworkGateway is the host's address on the holder's first network,
	// reachable from inside that container.
	NetworkGateway string `json:"network_gateway,omitempty"`
}

// Plan is everything an operator needs to put the dashboard behind a proxy.
type Plan struct {
	Domain      string   `json:"domain"`
	Proxy       Kind     `json:"proxy"`
	UpstreamURL string   `json:"upstream_url"`
	NeedsRebind bool     `json:"needs_rebind"`
	Rebind      string   `json:"rebind,omitempty"`
	Snippet     string   `json:"snippet"`
	SnippetPath string   `json:"snippet_path,omitempty"`
	Steps       []string `json:"steps"`
}

// Build returns the plan. listenAddr is the dashboard's current listen
// address (APP_HTTP_ADDR). gateway is the proxy container's host-side address,
// empty when the proxy runs on the host itself.
func Build(domain string, kind Kind, listenAddr, gateway string) (Plan, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" || strings.ContainsAny(domain, " /:") || !strings.Contains(domain, ".") {
		return Plan{}, fmt.Errorf("domain must be a hostname such as console.example.com")
	}
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil || port == "" {
		return Plan{}, fmt.Errorf("cannot read the dashboard listen address %q", listenAddr)
	}
	loopback := host == "127.0.0.1" || host == "localhost" || host == "::1"
	p := Plan{Domain: domain, Proxy: kind}
	switch gateway {
	case "":
		p.UpstreamURL = "http://127.0.0.1:" + port
		if !loopback && host != "" {
			p.UpstreamURL = "http://" + net.JoinHostPort(host, port)
		}
	default:
		p.UpstreamURL = "http://" + net.JoinHostPort(gateway, port)
		if loopback {
			p.NeedsRebind = true
			p.Rebind = "APP_HTTP_ADDR=" + net.JoinHostPort(gateway, port)
		}
	}
	p.Snippet, p.SnippetPath = snippet(kind, domain, p.UpstreamURL)
	p.Steps = steps(p)
	return p, nil
}

func steps(p Plan) []string {
	var s []string
	s = append(s, fmt.Sprintf("Point an A record for %s at this server's IPv4 address.", p.Domain))
	if p.NeedsRebind {
		s = append(s, "Make the dashboard listen where the proxy can reach it, using a systemd drop-in so upgrades keep it: "+
			"/etc/systemd/system/levelrail.service.d/dashboard-bind.conf with [Service] and Environment="+p.Rebind+", then systemctl daemon-reload and systemctl restart levelrail. "+
			"That address is internal to the host and is not reachable from the internet.")
	}
	if p.SnippetPath != "" {
		s = append(s, "Save the configuration below as "+p.SnippetPath+".")
	} else {
		s = append(s, "Add the configuration below to your proxy and reload it.")
	}
	s = append(s, "Check the result with the verify button, or: levelrail-cli proxy --domain "+p.Domain+" --verify.")
	return s
}

func snippet(kind Kind, domain, upstream string) (text, path string) {
	switch kind {
	case Traefik:
		return fmt.Sprintf(`http:
  routers:
    levelrail-dashboard:
      rule: Host(`+"`%[1]s`"+`)
      entryPoints: [https]
      service: levelrail-dashboard
      tls:
        certResolver: letsencrypt
    levelrail-dashboard-http:
      rule: Host(`+"`%[1]s`"+`)
      entryPoints: [http]
      middlewares: [levelrail-https]
      service: levelrail-dashboard
  middlewares:
    levelrail-https:
      redirectScheme:
        scheme: https
        permanent: true
  services:
    levelrail-dashboard:
      loadBalancer:
        servers:
          - url: %[2]s
`, domain, upstream), "/data/coolify/proxy/dynamic/levelrail.yaml (Coolify) or your Traefik dynamic configuration directory"
	case Nginx:
		return fmt.Sprintf(`server {
    listen 80;
    server_name %[1]s;
    location / {
        proxy_pass %[2]s;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
# Then: certbot --nginx -d %[1]s
`, domain, upstream), "/etc/nginx/conf.d/levelrail.conf"
	default:
		return fmt.Sprintf("%s {\n    reverse_proxy %s\n}\n", domain, strings.TrimPrefix(upstream, "http://")), "your Caddyfile"
	}
}
