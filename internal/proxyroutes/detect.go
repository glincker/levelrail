package proxyroutes

import (
	"net"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Proxy kinds Detection.Kind reports.
const (
	KindTraefik = "traefik"
	KindNginx   = "nginx"
	KindCaddy   = "caddy"
	KindNone    = "none"
)

// Mount is one container mount: host Source at container Destination.
type Mount struct {
	Source      string
	Destination string
}

// PortMap is one published port: container Private on host Public.
type PortMap struct {
	Private int
	Public  int
}

// Container is the Docker data detection reads from a proxy container.
type Container struct {
	Name        string
	Image       string
	Kind        string
	Args        []string
	Mounts      []Mount
	ExtraHosts  []string
	NetworkMode string
	// Gateway is the host side of the container's first network.
	Gateway  string
	Mappings []PortMap
	// HostGatewayIP is what Docker's "host-gateway" alias resolves to.
	HostGatewayIP string
	// StaticFile and StaticConfig are the static configuration file read
	// from the host, when one exists (see StaticConfigCandidates).
	StaticFile   string
	StaticConfig []byte
}

// HostDockerInternal is the name Docker resolves to the host when the
// container was started with host.docker.internal:host-gateway.
const HostDockerInternal = "host.docker.internal"

// Detection is what was learned about the proxy container. Missing lists,
// in plain language, everything that could not be determined.
type Detection struct {
	Kind            string   `json:"kind"`
	Container       string   `json:"container"`
	Image           string   `json:"image"`
	PublishedPorts  []int    `json:"published_ports"`
	DynamicDir      string   `json:"dynamic_dir"`
	EntrypointHTTP  string   `json:"entrypoint_http"`
	EntrypointHTTPS string   `json:"entrypoint_https"`
	CertResolver    string   `json:"cert_resolver"`
	UpstreamHost    string   `json:"upstream_host"`
	Complete        bool     `json:"complete"`
	Missing         []string `json:"missing"`

	ContainerDir    string   `json:"-"`
	CertResolvers   []string `json:"-"`
	UpstreamIP      string   `json:"-"`
	HostNetwork     bool     `json:"-"`
	PublicHTTPPort  int      `json:"-"`
	PublicHTTPSPort int      `json:"-"`
	// APIPort is the host port of Traefik's unauthenticated API, 0 if none.
	APIPort int `json:"-"`

	// dirMissing are the gaps an explicit dynamic_dir resolves.
	dirMissing []string
}

// traefikAPIEntrypoint is Traefik's built-in API entrypoint name.
const traefikAPIEntrypoint = "traefik"

// traefikAPIDefaultPort is the container port of that entrypoint when it is not set explicitly.
const traefikAPIDefaultPort = 8080

// None is the detection result when no container holds the web ports.
func None() Detection {
	return Detection{Kind: KindNone, PublishedPorts: []int{}, Missing: []string{"no running container publishes port 80 or 443 on this host"}}
}

// parseArgs turns Traefik CLI flags into lowercased keys. Traefik accepts
// "--key=value" and "--key value"; a bare flag is boolean true.
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

func addressPort(addr string) int {
	addr, _, _ = strings.Cut(addr, "/")
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return 0
	}
	p, err := strconv.Atoi(addr[i+1:])
	if err != nil {
		return 0
	}
	return p
}

// hostPath maps a path inside the container to the host through the mount
// with the longest matching destination.
func hostPath(mounts []Mount, inside string) (string, bool) {
	inside = path.Clean(inside)
	best := -1
	var out string
	for _, m := range mounts {
		dst := path.Clean(m.Destination)
		if m.Source == "" || dst == "" || dst == "." {
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

// Detect reads a proxy container's arguments, mounts, ports and extra hosts.
// It never guesses: anything it cannot determine is listed in Missing.
func Detect(c Container) Detection {
	d := Detection{Kind: c.Kind, Container: c.Name, Image: c.Image, HostNetwork: c.NetworkMode == "host"}
	for _, m := range c.Mappings {
		d.PublishedPorts = append(d.PublishedPorts, m.Public)
	}
	d.PublishedPorts = uniqueInts(d.PublishedPorts)
	d.detectUpstream(c)
	if c.Kind != KindTraefik {
		d.Missing = append(d.Missing, "managed routes need Traefik's file provider; "+c.Kind+" is supported through the copy and paste guide only")
		d.finish()
		return d
	}
	flags := parseArgs(c.Args)
	if c.StaticFile != "" {
		// Traefik reads one static source: a found file replaces the flags.
		fileFlags, err := staticFlags(c.StaticFile, c.StaticConfig)
		if err != nil {
			d.Missing = append(d.Missing, err.Error())
			d.finish()
			return d
		}
		flags = fileFlags
	}
	d.detectEntrypoints(c, flags)
	d.detectResolver(flags)
	d.detectDir(c, flags)
	d.finish()
	return d
}

func (d *Detection) finish() {
	d.Missing = append(d.Missing, d.dirMissing...)
	if d.PublishedPorts == nil {
		d.PublishedPorts = []int{}
	}
	if d.Missing == nil {
		d.Missing = []string{}
	}
	d.Complete = len(d.Missing) == 0
}

// OverrideDir uses dir, given by the operator, as the host directory and
// drops the gaps it resolves.
func (d *Detection) OverrideDir(dir string) {
	d.DynamicDir = dir
	drop := map[string]bool{}
	for _, m := range d.dirMissing {
		drop[m] = true
	}
	kept := []string{}
	for _, m := range d.Missing {
		if !drop[m] {
			kept = append(kept, m)
		}
	}
	d.Missing, d.dirMissing = kept, nil
	d.Complete = len(d.Missing) == 0
}

func (d *Detection) publicPort(c Container, private int) (int, bool) {
	if d.HostNetwork {
		return private, private > 0
	}
	for _, m := range c.Mappings {
		if m.Private == private {
			return m.Public, true
		}
	}
	return 0, false
}

type entrypoint struct {
	name    string
	private int
	public  int
}

func (d *Detection) detectEntrypoints(c Container, flags map[string]string) {
	var eps []entrypoint
	apiPort := traefikAPIDefaultPort
	for k, v := range flags {
		if !strings.HasPrefix(k, "entrypoints.") || !strings.HasSuffix(k, ".address") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(k, "entrypoints."), ".address")
		if strings.Contains(name, ".") {
			continue
		}
		port := addressPort(v)
		if name == traefikAPIEntrypoint {
			apiPort = port
			continue
		}
		pub, _ := d.publicPort(c, port)
		eps = append(eps, entrypoint{name: name, private: port, public: pub})
	}
	sort.Slice(eps, func(i, j int) bool { return eps[i].name < eps[j].name })
	httpEP, httpOK := pickEntrypoint(eps, 80)
	httpsEP, httpsOK := pickEntrypoint(eps, 443)
	if httpOK {
		d.EntrypointHTTP, d.PublicHTTPPort = httpEP.name, httpEP.public
	} else {
		d.Missing = append(d.Missing, "no Traefik entrypoint is published on host port 80 (looked for --entrypoints.<name>.address)")
	}
	if httpsOK {
		d.EntrypointHTTPS, d.PublicHTTPSPort = httpsEP.name, httpsEP.public
	} else {
		d.Missing = append(d.Missing, "no Traefik entrypoint is published on host port 443 (looked for --entrypoints.<name>.address)")
	}
	if flags["api.insecure"] == "true" {
		if pub, ok := d.publicPort(c, apiPort); ok {
			d.APIPort = pub
		}
	}
}

// pickEntrypoint prefers the entrypoint published on host port want, then
// the only published one listening on container port want.
func pickEntrypoint(eps []entrypoint, want int) (entrypoint, bool) {
	for _, e := range eps {
		if e.public == want {
			return e, true
		}
	}
	var found []entrypoint
	for _, e := range eps {
		if e.private == want && e.public != 0 {
			found = append(found, e)
		}
	}
	if len(found) == 1 {
		return found[0], true
	}
	return entrypoint{}, false
}

func (d *Detection) detectResolver(flags map[string]string) {
	resolvers := map[string]bool{}
	for k := range flags {
		if strings.HasPrefix(k, "certificatesresolvers.") {
			name, _, _ := strings.Cut(strings.TrimPrefix(k, "certificatesresolvers."), ".")
			resolvers[name] = true
		}
	}
	for name := range resolvers {
		d.CertResolvers = append(d.CertResolvers, name)
	}
	sort.Strings(d.CertResolvers)
	switch len(d.CertResolvers) {
	case 1:
		d.CertResolver = d.CertResolvers[0]
	case 0:
		d.Missing = append(d.Missing, "no certificate resolver in the Traefik arguments (--certificatesresolvers.<name>.acme...), so Traefik would serve its default certificate")
	default:
		d.Missing = append(d.Missing, "several certificate resolvers ("+strings.Join(d.CertResolvers, ", ")+"); choose one in settings")
	}
}

func (d *Detection) detectDir(c Container, flags map[string]string) {
	switch dir, file := flags["providers.file.directory"], flags["providers.file.filename"]; {
	case dir != "":
		d.ContainerDir = dir
		if host, ok := hostPath(c.Mounts, dir); ok {
			d.DynamicDir = host
		} else {
			d.dirMissing = append(d.dirMissing, "the file provider directory "+dir+" is not on a host mount, so this host cannot write into it")
		}
	case file != "":
		d.dirMissing = append(d.dirMissing, "the file provider reads a single file ("+file+"); managed routes need --providers.file.directory")
	default:
		d.dirMissing = append(d.dirMissing, "no --providers.file.directory argument (the static configuration may be in a traefik.yml); pass dynamic_dir")
	}
	if flags["providers.file.watch"] == "false" {
		d.Missing = append(d.Missing, "the file provider has watch=false, so Traefik would not load new files without a restart")
	}
}

func (d *Detection) detectUpstream(c Container) {
	if d.HostNetwork {
		d.UpstreamHost, d.UpstreamIP = "127.0.0.1", "127.0.0.1"
		return
	}
	if target, ok := hostDockerInternalTarget(c.ExtraHosts); ok {
		d.UpstreamHost = HostDockerInternal
		switch {
		case target == "host-gateway" && c.HostGatewayIP != "":
			d.UpstreamIP = c.HostGatewayIP
		case net.ParseIP(target) != nil:
			d.UpstreamIP = target
		default:
			d.Missing = append(d.Missing, "host.docker.internal maps to "+target+", but the address it resolves to could not be read from Docker")
		}
		return
	}
	if c.Gateway != "" {
		d.UpstreamHost, d.UpstreamIP = c.Gateway, c.Gateway
		return
	}
	d.Missing = append(d.Missing, "cannot tell how the proxy container reaches this host (no host.docker.internal entry and no network gateway); set upstream_host")
}

func hostDockerInternalTarget(extra []string) (string, bool) {
	for _, h := range extra {
		name, target, ok := strings.Cut(h, ":")
		if !ok {
			// Docker 25+ also accepts "name=target".
			name, target, ok = strings.Cut(h, "=")
		}
		if ok && strings.EqualFold(name, HostDockerInternal) && target != "" {
			return target, true
		}
	}
	return "", false
}

func uniqueInts(in []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, v := range in {
		if v > 0 && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Ints(out)
	return out
}
