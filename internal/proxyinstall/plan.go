// Package proxyinstall plans and applies the start of a managed reverse
// proxy container through the Docker Engine API.
package proxyinstall

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// EnvTraefikImage overrides DefaultTraefikImage, the pinned official image.
const (
	EnvTraefikImage     = "APP_PROXY_TRAEFIK_IMAGE"
	DefaultTraefikImage = "traefik:v3.3.5"
)

// Fixed names of the proxy's own entry points and certificate resolver.
const (
	EntrypointHTTP  = "web"
	EntrypointHTTPS = "websecure"
	CertResolver    = "letsencrypt"
	UpstreamHost    = "host.docker.internal"

	LabelManaged = "managed-by"
	HTTPPort     = 80
	HTTPSPort    = 443

	RestartUnlessStopped = "unless-stopped"
	ProviderTraefik      = "traefik"
)

// Refusal codes a plan can carry.
const (
	CodeUnsupportedProvider = "unsupported_provider"
	CodeInContainer         = "control_plane_in_container"
	CodePortBusy            = "port_in_use"
	CodeOwnIngress          = "port_held_by_builtin_ingress"
	CodeNameTaken           = "container_name_taken"
	CodeNoDataDir           = "no_data_dir"
)

// Input is everything the plan depends on.
type Input struct {
	Provider    string
	Image       string
	Brand       string
	DataDir     string
	PublicCerts bool
	ACMEEmail   string
}

// File is one file the install writes before the container starts.
type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Mount is one bind mount of the container.
type Mount struct {
	Host      string `json:"host"`
	Container string `json:"container"`
	ReadOnly  bool   `json:"read_only"`
}

// Refusal is a reason the install cannot proceed, with what to do instead.
type Refusal struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (r *Refusal) Error() string { return r.Message }

// Plan is the exact install: nothing in it is decided at apply time.
type Plan struct {
	Provider  string            `json:"provider"`
	Container string            `json:"container"`
	Image     string            `json:"image"`
	Ports     []int             `json:"ports"`
	Mounts    []Mount           `json:"mounts"`
	Files     []File            `json:"files"`
	Labels    map[string]string `json:"labels"`
	Restart   string            `json:"restart"`
	RouteDir  string            `json:"route_dir"`
	Resolver  string            `json:"cert_resolver"`
	Upstream  string            `json:"upstream_host"`
	Refusal   *Refusal          `json:"refusal,omitempty"`
}

// BuildPlan is pure: it only renders names, paths and file contents.
func BuildPlan(in Input) (Plan, error) {
	if in.Provider != ProviderTraefik {
		return Plan{Provider: in.Provider}, &Refusal{Code: CodeUnsupportedProvider,
			Message: "Only Traefik can be installed for you. nginx needs certificates you provide, so start it yourself or use the built-in Caddy."}
	}
	if strings.TrimSpace(in.DataDir) == "" || strings.TrimSpace(in.Brand) == "" {
		return Plan{}, &Refusal{Code: CodeNoDataDir, Message: "this control plane has no data directory to keep the proxy files in"}
	}
	image := strings.TrimSpace(in.Image)
	if image == "" {
		image = DefaultTraefikImage
	}
	brand := strings.ToLower(in.Brand)
	root := filepath.Join(in.DataDir, "proxy", ProviderTraefik)
	routeDir := filepath.Join(root, "dynamic")
	p := Plan{
		Provider: ProviderTraefik, Container: brand + "-" + ProviderTraefik, Image: image,
		Ports: []int{HTTPPort, HTTPSPort}, RouteDir: routeDir, Restart: RestartUnlessStopped,
		Upstream: UpstreamHost,
		Labels:   map[string]string{brand + "." + LabelManaged: brand, brand + ".proxy": ProviderTraefik},
		Mounts: []Mount{
			{Host: filepath.Join(root, "traefik.yml"), Container: "/etc/traefik/traefik.yml", ReadOnly: true},
			{Host: routeDir, Container: "/etc/traefik/dynamic"},
		},
	}
	if in.PublicCerts && strings.TrimSpace(in.ACMEEmail) != "" {
		p.Resolver = CertResolver
		p.Mounts = append(p.Mounts, Mount{Host: filepath.Join(root, "acme"), Container: "/etc/traefik/acme"})
	}
	p.Files = []File{{Path: filepath.Join(root, "traefik.yml"), Content: renderTraefik(p, in.ACMEEmail)}}
	return p, nil
}

func renderTraefik(p Plan, email string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "entryPoints:\n  %s:\n    address: \":%d\"\n  %s:\n    address: \":%d\"\n", EntrypointHTTP, HTTPPort, EntrypointHTTPS, HTTPSPort)
	b.WriteString("providers:\n  file:\n    directory: /etc/traefik/dynamic\n    watch: true\n")
	if p.Resolver != "" {
		fmt.Fprintf(&b, "certificatesResolvers:\n  %s:\n    acme:\n      email: %q\n      storage: /etc/traefik/acme/acme.json\n      httpChallenge:\n        entryPoint: %s\n", p.Resolver, email, EntrypointHTTP)
	}
	return b.String()
}

// Spec converts the plan to the Docker container spec.
func (p Plan) Spec() docker.ContainerSpec {
	spec := docker.ContainerSpec{
		Name: p.Container, Image: p.Image, Labels: p.Labels, RestartPolicy: p.Restart,
		ExtraHosts: []string{UpstreamHost + ":host-gateway"},
	}
	for _, port := range p.Ports {
		spec.Ports = append(spec.Ports, docker.PortBinding{ContainerPort: port, HostPort: port})
	}
	for _, m := range p.Mounts {
		spec.BindMounts = append(spec.BindMounts, docker.BindMount{HostPath: m.Host, ContainerPath: m.Container, ReadOnly: m.ReadOnly})
	}
	return spec
}
