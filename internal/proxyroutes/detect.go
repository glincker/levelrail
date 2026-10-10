package proxyroutes

import (
	"path"
	"sort"
	"strings"
)

// Mount is one container mount: host Source at container Destination.
type Mount struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// Container is the Docker data detection reads from a proxy container.
type Container struct {
	Name        string
	Image       string
	Args        []string
	Mounts      []Mount
	ExtraHosts  []string
	NetworkMode string
	Gateway     string
	Published   []int
}

// HostDockerInternal is the name Docker resolves to the host when the
// container was started with host.docker.internal:host-gateway.
const HostDockerInternal = "host.docker.internal"

// Detection is what was learned about a Traefik container. Missing lists,
// in plain language, everything that could not be determined.
type Detection struct {
	Container       string   `json:"container,omitempty"`
	Image           string   `json:"image,omitempty"`
	DynamicDir      string   `json:"dynamic_dir,omitempty"`
	ContainerDir    string   `json:"container_dir,omitempty"`
	EntrypointHTTP  string   `json:"entrypoint_http,omitempty"`
	EntrypointHTTPS string   `json:"entrypoint_https,omitempty"`
	CertResolver    string   `json:"cert_resolver,omitempty"`
	CertResolvers   []string `json:"cert_resolvers,omitempty"`
	// UpstreamHost is how the proxy reaches this host, before the listener
	// address is taken into account.
	UpstreamHost string `json:"upstream_host,omitempty"`
	HostNetwork  bool   `json:"host_network,omitempty"`
	// APIInsecure means Traefik's API answers unauthenticated on :8080.
	APIInsecure bool     `json:"api_insecure,omitempty"`
	APIPort     int      `json:"api_port,omitempty"`
	Missing     []string `json:"missing,omitempty"`
}

// traefikAPIPort is the port of Traefik's built-in "traefik" entrypoint.
const traefikAPIPort = 8080

// parseArgs turns Traefik CLI flags into lowercased keys. Traefik accepts
// "--key=value" and bare boolean "--key"; a bare flag followed by a value
// that is not itself a flag takes that value.
func parseArgs(args []string) map[string]string {
	out := map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			continue
		}
		a = strings.TrimPrefix(a, "--")
		if k, v, ok := strings.Cut(a, "="); ok {
			out[strings.ToLower(k)] = v
			continue
		}
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			out[strings.ToLower(a)] = args[i+1]
			i++
			continue
		}
		out[strings.ToLower(a)] = "true"
	}
	return out
}

func addressPort(addr string) string {
	addr, _, _ = strings.Cut(addr, "/")
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i+1:]
	}
	return ""
}

// hostPath maps a path inside the container to the host through the mount
// with the longest matching destination.
func hostPath(mounts []Mount, inside string) (string, bool) {
	inside = path.Clean(inside)
	best := -1
	var out string
	for _, m := range mounts {
		dst := path.Clean(m.Destination)
		if m.Source == "" || dst == "" {
			continue
		}
		if inside != dst && !strings.HasPrefix(inside, strings.TrimSuffix(dst, "/")+"/") {
			continue
		}
		if len(dst) > best {
			best = len(dst)
			out = path.Join(m.Source, strings.TrimPrefix(inside, dst))
		}
	}
	return out, best >= 0
}

// DetectTraefik reads a Traefik container's command arguments and mounts.
// It never guesses: anything it cannot determine is listed in Missing.
func DetectTraefik(c Container) Detection {
	d := Detection{Container: c.Name, Image: c.Image}
	flags := parseArgs(c.Args)

	resolvers := map[string]bool{}
	for k, v := range flags {
		switch {
		case strings.HasPrefix(k, "entrypoints.") && strings.HasSuffix(k, ".address"):
			name := strings.TrimSuffix(strings.TrimPrefix(k, "entrypoints."), ".address")
			switch addressPort(v) {
			case "80":
				d.EntrypointHTTP = name
			case "443":
				d.EntrypointHTTPS = name
			}
		case strings.HasPrefix(k, "certificatesresolvers."):
			name, _, _ := strings.Cut(strings.TrimPrefix(k, "certificatesresolvers."), ".")
			resolvers[name] = true
		}
	}
	for name := range resolvers {
		d.CertResolvers = append(d.CertResolvers, name)
	}
	sort.Strings(d.CertResolvers)
	switch {
	case resolvers["letsencrypt"]:
		d.CertResolver = "letsencrypt"
	case len(d.CertResolvers) == 1:
		d.CertResolver = d.CertResolvers[0]
	}

	switch dir, file := flags["providers.file.directory"], flags["providers.file.filename"]; {
	case dir != "":
		d.ContainerDir = dir
		if host, ok := hostPath(c.Mounts, dir); ok {
			d.DynamicDir = host
		} else {
			d.Missing = append(d.Missing, "the file provider directory "+dir+" is not on a host mount, so this host cannot write into it")
		}
	case file != "":
		d.Missing = append(d.Missing, "the file provider reads a single file ("+file+"); managed routes need --providers.file.directory")
	default:
		d.Missing = append(d.Missing, "no --providers.file.directory argument (static configuration may be in a traefik.yml); set the directory in settings")
	}
	if flags["providers.file.watch"] == "false" {
		d.Missing = append(d.Missing, "the file provider has watch=false, so Traefik would not load new files without a restart")
	}
	if d.EntrypointHTTP == "" {
		d.Missing = append(d.Missing, "no entrypoint listening on :80 in the arguments")
	}
	if d.EntrypointHTTPS == "" {
		d.Missing = append(d.Missing, "no entrypoint listening on :443 in the arguments")
	}
	if d.CertResolver == "" {
		if len(d.CertResolvers) > 1 {
			d.Missing = append(d.Missing, "several certificate resolvers ("+strings.Join(d.CertResolvers, ", ")+"); choose one in settings")
		} else {
			d.Missing = append(d.Missing, "no certificate resolver in the arguments, so Traefik would serve its default certificate")
		}
	}

	switch {
	case c.NetworkMode == "host":
		d.HostNetwork = true
		d.UpstreamHost = "127.0.0.1"
	case hasHostGateway(c.ExtraHosts):
		d.UpstreamHost = HostDockerInternal
	case c.Gateway != "":
		d.UpstreamHost = c.Gateway
	default:
		d.Missing = append(d.Missing, "cannot tell how the proxy container reaches this host; set an upstream host")
	}

	if flags["api.insecure"] == "true" {
		d.APIInsecure = true
		for _, p := range c.Published {
			if p == traefikAPIPort {
				d.APIPort = traefikAPIPort
			}
		}
	}
	return d
}

func hasHostGateway(extra []string) bool {
	for _, h := range extra {
		name, target, ok := strings.Cut(h, ":")
		if ok && strings.EqualFold(name, HostDockerInternal) && target == "host-gateway" {
			return true
		}
	}
	return false
}
