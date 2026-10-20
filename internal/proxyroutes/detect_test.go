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
		Name: "coolify-proxy", Image: "traefik:v3.6", Kind: KindTraefik, Args: append([]string{"traefik"}, coolifyArgs...),
		Mounts:     []Mount{{Source: "/var/run/docker.sock", Destination: "/var/run/docker.sock"}, {Source: "/data/coolify/proxy", Destination: "/traefik"}},
		ExtraHosts: []string{"host.docker.internal:host-gateway"}, HostGatewayIP: "172.17.0.1",
		Gateway: "10.0.1.1", Mappings: []PortMap{{80, 80}, {443, 443}, {443, 443}, {8080, 8080}},
	}
}

func coolifyDetection() Detection {
	return Detection{
		Kind: KindTraefik, Container: "coolify-proxy", Image: "traefik:v3.6", PublishedPorts: []int{80, 443, 8080},
		DynamicDir: "/data/coolify/proxy/dynamic", ContainerDir: "/traefik/dynamic/",
		EntrypointHTTP: "http", EntrypointHTTPS: "https", CertResolver: "letsencrypt", CertResolvers: []string{"letsencrypt"},
		UpstreamHost: HostDockerInternal, UpstreamIP: "172.17.0.1", PublicHTTPPort: 80, PublicHTTPSPort: 443,
		Complete: true, Missing: []string{},
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*Container)
		want        func(*Detection)
		missingHint []string
	}{
		{name: "coolify", mutate: func(*Container) {}, want: func(*Detection) {}},
		{
			name: "plain traefik, remapped host ports, nested mount wins, insecure api",
			mutate: func(c *Container) {
				c.Args = []string{"--entrypoints.web.address", ":8000", "--entrypoints.websecure.address=0.0.0.0:8443/tcp",
					"--entrypoints.traefik.address=:9000", "--providers.file.directory", "/etc/traefik/dynamic",
					"--certificatesresolvers.le.acme.tlschallenge=true", "--api.insecure=true"}
				c.Mounts = []Mount{{Source: "/srv/traefik", Destination: "/etc/traefik"}, {Source: "/srv/dyn", Destination: "/etc/traefik/dynamic"}}
				c.ExtraHosts = nil
				c.Mappings = []PortMap{{8000, 80}, {8443, 443}, {9000, 19000}}
			},
			want: func(d *Detection) {
				d.PublishedPorts = []int{80, 443, 19000}
				d.DynamicDir, d.ContainerDir = "/srv/dyn", "/etc/traefik/dynamic"
				d.EntrypointHTTP, d.EntrypointHTTPS, d.CertResolver, d.CertResolvers = "web", "websecure", "le", []string{"le"}
				d.UpstreamHost, d.UpstreamIP, d.APIPort = "10.0.1.1", "10.0.1.1", 19000
			},
		},
		{
			name: "https published on a non standard host port",
			mutate: func(c *Container) {
				c.Mappings = []PortMap{{80, 8080}, {443, 8443}}
			},
			want: func(d *Detection) {
				d.PublishedPorts = []int{8080, 8443}
				d.PublicHTTPPort, d.PublicHTTPSPort = 8080, 8443
			},
		},
		{
			name:   "host network",
			mutate: func(c *Container) { c.NetworkMode = "host"; c.ExtraHosts = nil; c.Mappings = nil },
			want: func(d *Detection) {
				d.PublishedPorts = []int{}
				d.UpstreamHost, d.UpstreamIP, d.HostNetwork = "127.0.0.1", "127.0.0.1", true
			},
		},
		{
			name:   "extra host with an explicit address",
			mutate: func(c *Container) { c.ExtraHosts = []string{"host.docker.internal=192.168.5.1"} },
			want:   func(d *Detection) { d.UpstreamIP = "192.168.5.1" },
		},
		{
			name:        "host-gateway address unreadable",
			mutate:      func(c *Container) { c.HostGatewayIP = "" },
			missingHint: []string{"host.docker.internal maps to host-gateway"},
		},
		{name: "directory not mounted", mutate: func(c *Container) { c.Mounts = nil }, missingHint: []string{"not on a host mount"}},
		{
			name:        "single file provider",
			mutate:      func(c *Container) { c.Args = []string{"--providers.file.filename=/traefik/dyn.yaml"} },
			missingHint: []string{"single file", "host port 80", "host port 443", "certificate resolver"},
		},
		{
			name:        "static config file, no way to reach the host",
			mutate:      func(c *Container) { c.Args = []string{"traefik"}; c.ExtraHosts = nil; c.Gateway = "" },
			missingHint: []string{"traefik.yml", "upstream_host"},
		},
		{
			name:        "entrypoint not published",
			mutate:      func(c *Container) { c.Mappings = []PortMap{{80, 80}} },
			missingHint: []string{"host port 443"},
		},
		{
			name: "two resolvers",
			mutate: func(c *Container) {
				c.Args = append(c.Args, "--certificatesresolvers.a.acme.tlschallenge=true")
			},
			missingHint: []string{"several certificate resolvers (a, letsencrypt)"},
		},
		{name: "watch disabled", mutate: func(c *Container) { c.Args = append(c.Args, "--providers.file.watch=false") }, missingHint: []string{"watch=false"}},
		{name: "nginx", mutate: func(c *Container) { c.Kind = KindNginx }, missingHint: []string{"nginx is supported through the copy and paste guide only"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := coolify()
			c.Args = append([]string(nil), c.Args...)
			tc.mutate(&c)
			got := Detect(c)
			if tc.missingHint == nil {
				want := coolifyDetection()
				tc.want(&want)
				if !reflect.DeepEqual(got, want) {
					t.Errorf("got  %+v\nwant %+v", got, want)
				}
				return
			}
			if got.Complete {
				t.Errorf("Complete = true with missing hints expected")
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

func TestNone(t *testing.T) {
	d := None()
	if d.Kind != KindNone || d.Complete || len(d.Missing) != 1 || d.PublishedPorts == nil {
		t.Errorf("None() = %+v", d)
	}
}

func TestUpstreamResolve(t *testing.T) {
	bridge := UpstreamPath{Host: HostDockerInternal, IP: "172.17.0.1", Unit: "acme.service"}
	ingress := Listener{Label: "ingress", EnvVar: "APP_INGRESS_HTTP_ADDR"}
	tests := []struct {
		name    string
		path    UpstreamPath
		addr    string
		want    string
		wantErr []string
	}{
		{"wildcard", bridge, ":8088", "host.docker.internal:8088", nil},
		{"unspecified v4", bridge, "0.0.0.0:80", "host.docker.internal:80", nil},
		{"bound to the gateway", bridge, "172.17.0.1:8088", "host.docker.internal:8088", nil},
		{"loopback", bridge, "127.0.0.1:8088", "", []string{"upstream_unreachable", "cannot reach", "/etc/systemd/system/acme.service.d/proxy-upstream-ingress.conf", "Environment=APP_INGRESS_HTTP_ADDR=172.17.0.1:8088", "systemctl restart acme.service"}},
		{"localhost", bridge, "localhost:8088", "", []string{"cannot reach"}},
		{"other address", bridge, "10.9.9.9:8088", "", []string{"10.9.9.9:8088 only", "172.17.0.1"}},
		{"host network loopback", UpstreamPath{Host: "127.0.0.1", IP: "127.0.0.1", HostNetwork: true}, "127.0.0.1:8088", "127.0.0.1:8088", nil},
		{"host network specific", UpstreamPath{Host: "127.0.0.1", IP: "127.0.0.1", HostNetwork: true}, "10.0.0.2:8088", "10.0.0.2:8088", nil},
		{"unknown host", UpstreamPath{}, ":8088", "", []string{"set upstream_host"}},
		{"bad addr", bridge, "8088", "", []string{"not host:port"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := ingress
			l.Addr = tc.addr
			got, err := tc.path.Resolve(l)
			if got != tc.want {
				t.Errorf("Resolve = %q, want %q", got, tc.want)
			}
			if (err != nil) != (tc.wantErr != nil) {
				t.Fatalf("err = %v", err)
			}
			for _, h := range tc.wantErr {
				if !strings.Contains(err.Error(), h) {
					t.Errorf("error %q lacks %q", err, h)
				}
			}
		})
	}
}
