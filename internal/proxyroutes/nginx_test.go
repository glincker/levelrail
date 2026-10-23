package proxyroutes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

const nginxGoldenHead = "# managed by acme\n" +
	"# Do not edit: rewritten on every reconcile. Remove the domain instead.\n" +
	"server {\n" +
	"\tlisten 80;\n" +
	"\tlisten [::]:80;\n" +
	"\tserver_name app.example.com;\n"

const nginxGoldenRedirect = "\tlocation / {\n\t\treturn 301 https://$host$request_uri;\n\t}\n}\n"

const nginxGoldenTLS = "server {\n" +
	"\tlisten 443 ssl;\n" +
	"\tlisten [::]:443 ssl;\n" +
	"\tserver_name app.example.com;\n" +
	"\tssl_certificate /etc/letsencrypt/live/app.example.com/fullchain.pem;\n" +
	"\tssl_certificate_key /etc/letsencrypt/live/app.example.com/privkey.pem;\n" +
	"\tlocation / {\n" +
	"\t\tset $acme_connection upgrade;\n" +
	"\t\tif ($http_upgrade = \"\") {\n" +
	"\t\t\tset $acme_connection \"\";\n" +
	"\t\t}\n" +
	"\t\tproxy_pass http://host.docker.internal:8088;\n" +
	"\t\tproxy_http_version 1.1;\n" +
	"\t\tproxy_set_header Host $host;\n" +
	"\t\tproxy_set_header X-Real-IP $remote_addr;\n" +
	"\t\tproxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n" +
	"\t\tproxy_set_header X-Forwarded-Proto $scheme;\n" +
	"\t\tproxy_set_header X-Forwarded-Host $host;\n" +
	"\t\tproxy_set_header Upgrade $http_upgrade;\n" +
	"\t\tproxy_set_header Connection $acme_connection;\n" +
	"\t}\n" +
	"}\n"

func nginxRoute() Route {
	return Route{Domain: "app.example.com", Upstream: "host.docker.internal:8088", CertDir: "/etc/letsencrypt/live"}
}

func TestNginxRender(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Route)
		want   string
		errHas string
	}{
		{name: "certbot layout", mutate: func(*Route) {}, want: nginxGoldenHead + nginxGoldenRedirect + nginxGoldenTLS},
		{
			name:   "webroot adds the ACME challenge location",
			mutate: func(r *Route) { r.Webroot = "/var/www/certbot" },
			want: nginxGoldenHead + "\tlocation /.well-known/acme-challenge/ {\n\t\troot /var/www/certbot;\n\t}\n" +
				nginxGoldenRedirect + nginxGoldenTLS,
		},
		{name: "trailing slash on the cert dir", mutate: func(r *Route) { r.CertDir += "/" }, want: nginxGoldenHead + nginxGoldenRedirect + nginxGoldenTLS},
		{name: "no cert dir", mutate: func(r *Route) { r.CertDir = "" }, errHas: "certificate directory"},
		{name: "cert dir with a directive break", mutate: func(r *Route) { r.CertDir = "/etc/x; include /etc/passwd" }, errHas: "certificate directory"},
		{name: "cert dir traversal", mutate: func(r *Route) { r.CertDir = "/etc/../root" }, errHas: "certificate directory"},
		{name: "webroot injection", mutate: func(r *Route) { r.Webroot = "/x}\nserver{" }, errHas: "webroot"},
		{name: "uppercase domain", mutate: func(r *Route) { r.Domain = "App.example.com" }, errHas: "lowercase"},
		{name: "wildcard domain", mutate: func(r *Route) { r.Domain = "*.example.com" }, errHas: "wildcard"},
		{name: "bad upstream", mutate: func(r *Route) { r.Upstream = "host;rm" }, errHas: "upstream"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := nginxRoute()
			tc.mutate(&r)
			got, err := nginxProvider{}.Render(testNS, r)
			if tc.errHas != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errHas) {
					t.Fatalf("err = %v, want it to mention %q", err, tc.errHas)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("rendered file differs from golden:\n%s", got)
			}
		})
	}
}

func TestSnippetRender(t *testing.T) {
	r := Route{Domain: "app.example.com", Upstream: "10.0.0.5:8088"}
	caddy, err := snippetProvider{kind: KindCaddy}.Render(testNS, r)
	if err != nil || string(caddy) != "app.example.com {\n\treverse_proxy 10.0.0.5:8088\n}\n" {
		t.Errorf("caddy = %q, %v", caddy, err)
	}
	ha, err := snippetProvider{kind: KindHAProxy}.Render(testNS, r)
	if err != nil || !strings.Contains(string(ha), "backend acme_app_example_com") || !strings.Contains(string(ha), "server upstream 10.0.0.5:8088") {
		t.Errorf("haproxy = %q, %v", ha, err)
	}
	if Managed(snippetProvider{kind: KindCaddy}) || !Managed(nginxProvider{}) {
		t.Error("Managed must be true for nginx and false for snippet providers")
	}
}

func officialNginx() Container {
	return Container{
		Name: "edge-nginx", Image: "nginx:1.27", Kind: KindNginx,
		Mounts: []Mount{
			{Source: "/srv/nginx/conf.d", Destination: "/etc/nginx/conf.d"},
			{Source: "/etc/letsencrypt", Destination: "/etc/letsencrypt"},
			{Source: "/srv/certbot-www", Destination: "/var/www/certbot"},
		},
		ExtraHosts: []string{"host.docker.internal:host-gateway"}, HostGatewayIP: "172.17.0.1",
		Mappings: []PortMap{{80, 80}, {443, 443}},
	}
}

func TestNginxDetect(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Container)
		missing []string
		check   func(*testing.T, Detection)
	}{
		{name: "conf.d with certbot", mutate: func(*Container) {}, check: func(t *testing.T, d Detection) {
			if !d.Complete || d.DynamicDir != "/srv/nginx/conf.d" || d.ContainerDir != "/etc/nginx/conf.d" ||
				d.CertDir != "/etc/letsencrypt/live" || d.Webroot != "/var/www/certbot" || d.PublicHTTPSPort != 443 || d.UpstreamIP != "172.17.0.1" {
				t.Errorf("unexpected detection %+v", d)
			}
		}},
		{name: "no webroot is not a gap", mutate: func(c *Container) { c.Mounts = c.Mounts[:2] }, check: func(t *testing.T, d Detection) {
			if !d.Complete || d.Webroot != "" {
				t.Errorf("unexpected detection %+v", d)
			}
		}},
		{
			name: "sites-enabled fallback",
			mutate: func(c *Container) {
				c.Mounts[0] = Mount{Source: "/srv/nginx/sites", Destination: "/etc/nginx/sites-enabled"}
			},
			check: func(t *testing.T, d Detection) {
				if d.DynamicDir != "/srv/nginx/sites" || d.ContainerDir != "/etc/nginx/sites-enabled" {
					t.Errorf("unexpected detection %+v", d)
				}
			},
		},
		{name: "no config mount", mutate: func(c *Container) { c.Mounts = c.Mounts[1:] }, missing: []string{"is on a host mount"}},
		{name: "no certificate mount", mutate: func(c *Container) { c.Mounts = []Mount{c.Mounts[0]} }, missing: []string{"nothing is mounted at /etc/letsencrypt"}},
		{
			name:    "nginx.conf does not include conf.d",
			mutate:  func(c *Container) { c.NginxConf = []byte("http {\n\tinclude /etc/nginx/other/*.conf;\n}\n") },
			missing: []string{"nginx.conf does not include /etc/nginx/conf.d"},
		},
		{
			name:   "nginx.conf includes conf.d",
			mutate: func(c *Container) { c.NginxConf = []byte("http {\n\tinclude /etc/nginx/conf.d/*.conf;\n}\n") },
			check: func(t *testing.T, d Detection) {
				if !d.Complete {
					t.Errorf("unexpected detection %+v", d)
				}
			},
		},
		{name: "port 443 not published", mutate: func(c *Container) { c.Mappings = c.Mappings[:1] }, missing: []string{"container port 443 is not published"}},
		{name: "no way to reach the host", mutate: func(c *Container) { c.ExtraHosts = nil }, missing: []string{"cannot tell how the proxy container reaches this host"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := officialNginx()
			c.Mounts = append([]Mount(nil), c.Mounts...)
			c.Mappings = append([]PortMap(nil), c.Mappings...)
			tc.mutate(&c)
			got := Detect(c)
			if tc.missing != nil {
				if got.Complete {
					t.Fatalf("Complete = true, want gaps %v", tc.missing)
				}
				joined := strings.Join(got.Missing, "\n")
				for _, m := range tc.missing {
					if !strings.Contains(joined, m) {
						t.Errorf("missing %q in %q", m, joined)
					}
				}
				return
			}
			tc.check(t, got)
		})
	}
}

func TestNginxDetect_Overrides(t *testing.T) {
	c := officialNginx()
	c.Mounts = nil
	d := Detect(c)
	if d.Complete || len(d.Missing) != 2 {
		t.Fatalf("want two gaps, got %v", d.Missing)
	}
	d.OverrideDir("/srv/nginx/conf.d")
	d.OverrideCertDir("/certs")
	if !d.Complete || d.DynamicDir != "/srv/nginx/conf.d" || d.CertDir != "/certs" || !reflect.DeepEqual(d.Missing, []string{}) {
		t.Errorf("after overrides: %+v", d)
	}
}

type fakeExec struct {
	calls   [][]string
	failOn  string
	message string
}

func (f *fakeExec) Exec(_ context.Context, _ string, cmd []string) (string, error) {
	f.calls = append(f.calls, cmd)
	if f.failOn != "" && strings.Join(cmd, " ") == f.failOn {
		return "", &ExecError{Output: f.message, Err: errors.New("exited 1")}
	}
	return "", nil
}

func nginxSyncer(t *testing.T, ex Exec) (*Syncer, *fakeSyncStore, string) {
	t.Helper()
	dir := t.TempDir()
	st := newFakeSyncStore(dir)
	st.settings = store.ProxyIntegrationSettings{
		Mode: store.ProxyIntegrationNginxFile, DynamicDir: dir, UpstreamHost: HostDockerInternal,
		Container: "edge-nginx", CertDir: "/etc/letsencrypt/live",
	}
	st.services = []store.DesiredService{{Name: "web", Domains: []string{"a.example.com"}}}
	s := newTestSyncer(st)
	s.Exec = ex
	return s, st, dir
}

func TestSync_NginxWritesValidatesAndReloads(t *testing.T) {
	ex := &fakeExec{}
	s, _, dir := nginxSyncer(t, ex)
	if err := os.WriteFile(filepath.Join(dir, "default.conf"), []byte("server { listen 8081; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := s.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rep.Written, []string{"a.example.com"}) {
		t.Errorf("written = %v", rep.Written)
	}
	if got := managedFiles(t, dir); !reflect.DeepEqual(got, []string{"acme-managed-a.example.com.conf", "default.conf"}) {
		t.Errorf("files = %v", got)
	}
	want := [][]string{{"nginx", "-t"}, {"nginx", "-s", "reload"}}
	if !reflect.DeepEqual(ex.calls, want) {
		t.Errorf("exec calls = %v, want %v", ex.calls, want)
	}
	ex.calls = nil
	if _, err := s.Sync(context.Background()); err != nil || len(ex.calls) != 0 {
		t.Errorf("an unchanged pass must not reload: err=%v calls=%v", err, ex.calls)
	}
}

func TestSync_NginxRollsBackWhenValidationFails(t *testing.T) {
	ex := &fakeExec{}
	s, st, dir := nginxSyncer(t, ex)
	ctx := context.Background()
	if _, err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	aPath := filepath.Join(dir, "acme-managed-a.example.com.conf")
	before, err := os.ReadFile(aPath) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}

	ex.calls = nil
	ex.failOn = "nginx -t"
	ex.message = `nginx: [emerg] cannot load certificate "/etc/letsencrypt/live/b.example.com/fullchain.pem"`
	st.services = []store.DesiredService{{Name: "web", Domains: []string{"b.example.com"}}}
	rep, err := s.Sync(ctx)
	if err == nil || !strings.Contains(err.Error(), ex.message) {
		t.Fatalf("err = %v, want the exact nginx error", err)
	}
	if !strings.Contains(rep.Failed["b.example.com"], ex.message) {
		t.Errorf("failed = %v", rep.Failed)
	}
	if got := managedFiles(t, dir); !reflect.DeepEqual(got, []string{"acme-managed-a.example.com.conf"}) {
		t.Errorf("files after rollback = %v", got)
	}
	after, err := os.ReadFile(aPath) //nolint:gosec // test temp dir
	if err != nil || string(after) != string(before) {
		t.Errorf("a.example.com was not restored: %v", err)
	}
	for _, c := range ex.calls {
		if strings.Contains(strings.Join(c, " "), "reload") {
			t.Errorf("reloaded after a failed validation: %v", ex.calls)
		}
	}
	if !strings.Contains(st.status["b.example.com"].LastError, ex.message) {
		t.Errorf("status = %+v", st.status["b.example.com"])
	}
}

func TestSync_NginxNeverTouchesUnmanagedFiles(t *testing.T) {
	ex := &fakeExec{}
	s, st, dir := nginxSyncer(t, ex)
	foreign := filepath.Join(dir, "acme-managed-a.example.com.conf")
	const body = "server { listen 80; server_name a.example.com; }\n"
	if err := os.WriteFile(foreign, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := s.Sync(context.Background())
	if err == nil {
		t.Fatal("want an error for the file without the managed header")
	}
	if got, _ := os.ReadFile(foreign); string(got) != body { //nolint:gosec // test temp dir
		t.Errorf("foreign file was modified: %q", got)
	}
	if rep.Failed["a.example.com"] == "" || st.status["a.example.com"].LastError == "" {
		t.Errorf("failure not reported: %v", rep.Failed)
	}
	for _, c := range ex.calls {
		if strings.Join(c, " ") == "nginx -s reload" {
			t.Errorf("reloaded with nothing written: %v", ex.calls)
		}
	}
}

func TestSync_NginxWithoutDockerWritesNothing(t *testing.T) {
	s, _, dir := nginxSyncer(t, nil)
	if _, err := s.Sync(context.Background()); err == nil {
		t.Fatal("want an error")
	}
	if got := managedFiles(t, dir); len(got) != 0 {
		t.Errorf("wrote %v without a way to validate", got)
	}
}

func TestSync_NginxDisableRemovesOwnFiles(t *testing.T) {
	ex := &fakeExec{}
	s, st, dir := nginxSyncer(t, ex)
	ctx := context.Background()
	if _, err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	st.settings.Mode = store.ProxyIntegrationOff
	if _, err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if got := managedFiles(t, dir); len(got) != 0 {
		t.Errorf("files left after disable: %v", got)
	}
}
