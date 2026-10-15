package proxyroutes

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// goldenRoute is the file a Coolify-style Traefik gets for one app domain.
const goldenRoute = "# managed by acme\n" +
	"# Do not edit: rewritten on every reconcile. Remove the domain instead.\n" +
	"http:\n" +
	"  routers:\n" +
	"    acme-managed-app_example_com:\n" +
	"      rule: Host(`app.example.com`)\n" +
	"      entryPoints: [https]\n" +
	"      service: acme-managed-app_example_com\n" +
	"      priority: 100000\n" +
	"      tls:\n" +
	"        certResolver: letsencrypt\n" +
	"    acme-managed-app_example_com-http:\n" +
	"      rule: Host(`app.example.com`)\n" +
	"      entryPoints: [http]\n" +
	"      middlewares: [acme-managed-app_example_com-https]\n" +
	"      service: acme-managed-app_example_com\n" +
	"      priority: 100000\n" +
	"  middlewares:\n" +
	"    acme-managed-app_example_com-https:\n" +
	"      redirectScheme:\n" +
	"        scheme: https\n" +
	"        permanent: true\n" +
	"  services:\n" +
	"    acme-managed-app_example_com:\n" +
	"      loadBalancer:\n" +
	"        passHostHeader: true\n" +
	"        servers:\n" +
	"          - url: http://host.docker.internal:8088\n"

func goldenInput() Route {
	return Route{Domain: "app.example.com", EntrypointHTTP: "http", EntrypointHTTPS: "https", CertResolver: "letsencrypt", Upstream: "host.docker.internal:8088"}
}

func TestRender_Golden(t *testing.T) {
	got, err := testNS.Render(goldenInput())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != goldenRoute {
		t.Errorf("rendered file differs from golden:\n%s", got)
	}
}

func TestRender_Priority(t *testing.T) {
	type router struct {
		Priority int `yaml:"priority"`
	}
	var doc struct {
		HTTP struct {
			Routers map[string]router `yaml:"routers"`
		} `yaml:"http"`
	}
	cases := []struct {
		name    string
		prio    int
		want    int
		wantErr bool
	}{
		{"default", 0, DefaultRouterPriority, false},
		{"custom", 5000, 5000, false},
		{"maximum", MaxRouterPriority, MaxRouterPriority, false},
		{"negative", -1, 0, true},
		{"above traefik internal routers", MaxRouterPriority + 1, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := goldenInput()
			r.Priority = tc.prio
			body, err := testNS.Render(r)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := yaml.Unmarshal(body, &doc); err != nil {
				t.Fatalf("not valid YAML: %v", err)
			}
			if len(doc.HTTP.Routers) != 2 {
				t.Fatalf("routers = %v", doc.HTTP.Routers)
			}
			for name, r := range doc.HTTP.Routers {
				if r.Priority != tc.want {
					t.Errorf("%s priority = %d, want %d", name, r.Priority, tc.want)
				}
			}
		})
	}
}

// The header rule survives a priority change: the file stays ours to rewrite.
func TestRender_PriorityKeepsManagedHeader(t *testing.T) {
	r := goldenInput()
	r.Priority = 42
	body, err := testNS.Render(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), testNS.Header()+"\n") {
		t.Fatalf("missing managed header:\n%s", body)
	}
	dir := t.TempDir()
	d, err := OpenDir(testNS, dir)
	if err != nil {
		t.Fatal(err)
	}
	name, _ := testNS.FileName(r.Domain)
	if err := d.Write(name, body); err != nil {
		t.Fatal(err)
	}
	have, foreign, err := d.Managed()
	if err != nil || len(foreign) != 0 || string(have[name]) != string(body) {
		t.Fatalf("Managed() = %v, %v, %v", have, foreign, err)
	}
}
