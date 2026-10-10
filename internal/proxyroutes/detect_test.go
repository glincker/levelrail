package proxyroutes

import (
	"reflect"
	"strings"
	"testing"
)

// coolifyArgs is the command Coolify's coolify-proxy compose file starts
// Traefik with (bootstrap/helpers/proxy.php).
var coolifyArgs = []string{
	"--ping=true", "--ping.entrypoint=http", "--api.dashboard=true",
	"--entrypoints.http.address=:80", "--entrypoints.https.address=:443",
	"--entrypoints.http.http.encodequerysemicolons=true", "--entryPoints.http.http2.maxConcurrentStreams=250",
	"--entrypoints.https.http.encodequerysemicolons=true", "--entryPoints.https.http2.maxConcurrentStreams=250",
	"--entrypoints.https.http3",
	"--providers.file.directory=/traefik/dynamic/", "--providers.file.watch=true",
	"--certificatesresolvers.letsencrypt.acme.httpchallenge=true",
	"--certificatesresolvers.letsencrypt.acme.httpchallenge.entrypoint=http",
	"--certificatesresolvers.letsencrypt.acme.storage=/traefik/acme.json",
	"--api.insecure=false",
}

func coolify() Container {
	return Container{
		Name: "coolify-proxy", Image: "traefik:v3.6", Args: append([]string{"traefik"}, coolifyArgs...),
		Mounts:     []Mount{{Source: "/var/run/docker.sock", Destination: "/var/run/docker.sock"}, {Source: "/data/coolify/proxy", Destination: "/traefik"}},
		ExtraHosts: []string{"host.docker.internal:host-gateway"},
		Gateway:    "10.0.1.1", Published: []int{80, 443, 8080},
	}
}

func TestDetectTraefik(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*Container)
		want        Detection
		missingHint []string
	}{
		{
			name:   "coolify",
			mutate: func(*Container) {},
			want: Detection{
				Container: "coolify-proxy", Image: "traefik:v3.6", DynamicDir: "/data/coolify/proxy/dynamic", ContainerDir: "/traefik/dynamic/",
				EntrypointHTTP: "http", EntrypointHTTPS: "https", CertResolver: "letsencrypt", CertResolvers: []string{"letsencrypt"},
				UpstreamHost: HostDockerInternal,
			},
		},
		{
			name: "plain traefik, space separated values, nested mount wins",
			mutate: func(c *Container) {
				c.Args = []string{"--entrypoints.web.address", ":80", "--entrypoints.websecure.address=0.0.0.0:443/tcp",
					"--providers.file.directory", "/etc/traefik/dynamic", "--certificatesresolvers.le.acme.tlschallenge=true",
					"--api.insecure=true"}
				c.Mounts = []Mount{{Source: "/srv/traefik", Destination: "/etc/traefik"}, {Source: "/srv/dyn", Destination: "/etc/traefik/dynamic"}}
				c.ExtraHosts = nil
			},
			want: Detection{
				Container: "coolify-proxy", Image: "traefik:v3.6", DynamicDir: "/srv/dyn", ContainerDir: "/etc/traefik/dynamic",
				EntrypointHTTP: "web", EntrypointHTTPS: "websecure", CertResolver: "le", CertResolvers: []string{"le"},
				UpstreamHost: "10.0.1.1", APIInsecure: true, APIPort: 8080,
			},
		},
		{
			name:   "host network",
			mutate: func(c *Container) { c.NetworkMode = "host"; c.ExtraHosts = nil },
			want: Detection{
				Container: "coolify-proxy", Image: "traefik:v3.6", DynamicDir: "/data/coolify/proxy/dynamic", ContainerDir: "/traefik/dynamic/",
				EntrypointHTTP: "http", EntrypointHTTPS: "https", CertResolver: "letsencrypt", CertResolvers: []string{"letsencrypt"},
				UpstreamHost: "127.0.0.1", HostNetwork: true,
			},
		},
		{
			name:        "directory not mounted",
			mutate:      func(c *Container) { c.Mounts = nil },
			missingHint: []string{"not on a host mount"},
		},
		{
			name:        "single file provider",
			mutate:      func(c *Container) { c.Args = []string{"--providers.file.filename=/traefik/dyn.yaml"} },
			missingHint: []string{"single file", ":80", ":443", "certificate resolver"},
		},
		{
			name:        "static config file",
			mutate:      func(c *Container) { c.Args = []string{"traefik"}; c.ExtraHosts = nil; c.Gateway = "" },
			missingHint: []string{"traefik.yml", "upstream host"},
		},
		{
			name: "two resolvers, none preferred",
			mutate: func(c *Container) {
				c.Args = append(c.Args, "--certificatesresolvers.a.acme.tlschallenge=true", "--certificatesresolvers.b.acme.tlschallenge=true")
				for i, a := range c.Args {
					if strings.Contains(a, "letsencrypt") {
						c.Args[i] = "--noop"
					}
				}
			},
			missingHint: []string{"several certificate resolvers (a, b)"},
		},
		{
			name:        "watch disabled",
			mutate:      func(c *Container) { c.Args = append(c.Args, "--providers.file.watch=false") },
			missingHint: []string{"watch=false"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := coolify()
			c.Args = append([]string(nil), c.Args...)
			tc.mutate(&c)
			got := DetectTraefik(c)
			if tc.missingHint == nil {
				if !reflect.DeepEqual(got, tc.want) {
					t.Errorf("got  %+v\nwant %+v", got, tc.want)
				}
				return
			}
			joined := strings.Join(got.Missing, "\n")
			for _, h := range tc.missingHint {
				if !strings.Contains(joined, h) {
					t.Errorf("missing %q in %q", h, joined)
				}
			}
		})
	}
}
