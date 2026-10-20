package proxyroutes

import (
	"reflect"
	"strings"
	"testing"
)

const staticYAML = `
entryPoints:
  web:
    address: ":80"
  websecure:
    address: ":443"
providers:
  file:
    directory: /etc/traefik/dynamic
    watch: true
certificatesResolvers:
  myresolver:
    acme:
      email: ops@example.com
      httpChallenge:
        entryPoint: web
api:
  insecure: true
`

func TestDetectStaticFile(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		body    string
		want    func(*Detection)
		missing string
	}{
		{
			name: "yaml replaces the flags",
			file: "/srv/traefik/traefik.yml", body: staticYAML,
			want: func(d *Detection) {
				d.DynamicDir, d.ContainerDir = "/srv/traefik/dynamic", "/etc/traefik/dynamic"
				d.EntrypointHTTP, d.EntrypointHTTPS = "web", "websecure"
				d.CertResolver, d.CertResolvers, d.APIPort = "myresolver", []string{"myresolver"}, 8080
			},
		},
		{name: "toml is reported", file: "/srv/traefik/traefik.toml", body: "[api]", missing: "not YAML"},
		{name: "broken yaml", file: "/srv/traefik/traefik.yml", body: "entryPoints: [", missing: "parse"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := coolify()
			c.Args = []string{"traefik"}
			c.Mounts = []Mount{{Source: "/srv/traefik", Destination: "/etc/traefik"}}
			c.StaticFile, c.StaticConfig = tc.file, []byte(tc.body)
			got := Detect(c)
			if tc.missing != "" {
				if got.Complete || !strings.Contains(strings.Join(got.Missing, "\n"), tc.missing) {
					t.Fatalf("Missing = %v", got.Missing)
				}
				return
			}
			want := coolifyDetection()
			tc.want(&want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got  %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestOverrideDir(t *testing.T) {
	c := coolify()
	c.Args = append([]string{"traefik"}, coolifyArgs...)
	c.Mounts = nil
	d := Detect(c)
	if d.Complete {
		t.Fatal("unmounted directory should be incomplete")
	}
	d.OverrideDir("/srv/dyn")
	if !d.Complete || d.DynamicDir != "/srv/dyn" || len(d.Missing) != 0 {
		t.Errorf("after override %+v", d)
	}
	c.Args = append(c.Args, "--certificatesresolvers.b.acme.tlschallenge=true")
	d = Detect(c)
	d.OverrideDir("/srv/dyn")
	if d.Complete || !strings.Contains(strings.Join(d.Missing, "\n"), "several certificate resolvers") {
		t.Errorf("override must keep unrelated gaps: %+v", d.Missing)
	}
}

func TestStaticConfigCandidates(t *testing.T) {
	c := Container{Mounts: []Mount{{Source: "/srv/traefik", Destination: "/etc/traefik"}}}
	if got := StaticConfigCandidates(c); !reflect.DeepEqual(got, []string{"/srv/traefik/traefik.yml", "/srv/traefik/traefik.yaml", "/srv/traefik/traefik.toml"}) {
		t.Errorf("defaults = %v", got)
	}
	c.Args = []string{"--configFile=/etc/traefik/conf/static.yaml"}
	if got := StaticConfigCandidates(c); !reflect.DeepEqual(got, []string{"/srv/traefik/conf/static.yaml"}) {
		t.Errorf("configFile = %v", got)
	}
	if got := StaticConfigCandidates(Container{}); got != nil {
		t.Errorf("no mounts = %v", got)
	}
}
