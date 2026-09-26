package preflight

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeImage struct {
	info ImageInfo
	err  error
}

func (f fakeImage) Inspect(context.Context, string) (ImageInfo, error) { return f.info, f.err }

type fakeGit struct{ err error }

func (f fakeGit) Check(context.Context, string, string) error { return f.err }

func find(t *testing.T, rep Report, id string) Check {
	t.Helper()
	for _, c := range rep.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no check %q in %+v", id, rep.Checks)
	return Check{}
}

func TestDomainChecks(t *testing.T) {
	lookup := func(m map[string][]string) func(context.Context, string) ([]string, error) {
		return func(_ context.Context, h string) ([]string, error) {
			if a, ok := m[h]; ok {
				return a, nil
			}
			return nil, errors.New("no such host")
		}
	}
	tests := []struct {
		name   string
		dns    map[string][]string
		pubErr bool
		want   Status
		frag   string
	}{
		{"match", map[string][]string{"a.example.com": {"203.0.113.9"}}, false, StatusPass, "resolves to this server"},
		{"mismatch", map[string][]string{"a.example.com": {"198.51.100.4"}}, false, StatusFail, "203.0.113.9"},
		{"cloudflare v4", map[string][]string{"a.example.com": {"104.21.5.5"}}, false, StatusWarn, "Cloudflare"},
		{"cloudflare v6", map[string][]string{"a.example.com": {"2606:4700::6810:1"}}, false, StatusWarn, "Cloudflare"},
		{"unresolved", map[string][]string{}, false, StatusWarn, "does not resolve"},
		{"ip unknown", map[string][]string{"a.example.com": {"198.51.100.4"}}, true, StatusWarn, "could not be detected"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := Env{
				LookupHost: lookup(tc.dns),
				PublicIP: func(context.Context) (string, error) {
					if tc.pubErr {
						return "", errors.New("offline")
					}
					return "203.0.113.9", nil
				},
			}
			rep := Run(context.Background(), Request{Domains: []string{"a.example.com"}}, env)
			c := find(t, rep, "dns:a.example.com")
			if c.Status != tc.want || !strings.Contains(c.Reason, tc.frag) {
				t.Fatalf("got %s %q, want %s containing %q", c.Status, c.Reason, tc.want, tc.frag)
			}
		})
	}
}

func TestHostPort(t *testing.T) {
	env := Env{HostPortHolder: func(_ context.Context, _ string, p int) (string, bool, error) {
		return "app web", p == 8080, nil
	}}
	rep := Run(context.Background(), Request{HostPort: 8080}, env)
	if c := find(t, rep, "host_port"); c.Status != StatusFail || !strings.Contains(c.Reason, "app web") {
		t.Fatalf("got %+v", c)
	}
	rep = Run(context.Background(), Request{HostPort: 9090}, env)
	if c := find(t, rep, "host_port"); c.Status != StatusPass {
		t.Fatalf("got %+v", c)
	}
	if rep := Run(context.Background(), Request{}, env); len(rep.Checks) != 0 {
		t.Fatalf("unpinned port should skip: %+v", rep.Checks)
	}
}

func TestImageAndDisk(t *testing.T) {
	gib := int64(1 << 30)
	tests := []struct {
		name      string
		img       fakeImage
		free      int64
		wantImage Status
		wantDisk  Status
	}{
		{"ok", fakeImage{info: ImageInfo{SizeBytes: 100 << 20}}, 50 * gib, StatusPass, StatusPass},
		{"not found", fakeImage{err: ErrImageNotFound}, 50 * gib, StatusFail, StatusPass},
		{"private", fakeImage{err: ErrImageUnauthorized}, 50 * gib, StatusWarn, StatusPass},
		{"rate limited", fakeImage{err: ErrImageRateLimited}, 50 * gib, StatusWarn, StatusPass},
		{"registry down", fakeImage{err: errors.New("dial tcp: timeout")}, 50 * gib, StatusWarn, StatusPass},
		{"disk too small for image", fakeImage{info: ImageInfo{SizeBytes: 2 * gib}}, 3 * gib, StatusPass, StatusFail},
		{"disk low", fakeImage{info: ImageInfo{SizeBytes: 10 << 20}}, gib, StatusPass, StatusWarn},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := Env{Image: tc.img, FreeDisk: func(context.Context, string) (int64, error) { return tc.free, nil }}
			rep := Run(context.Background(), Request{Image: "acme/web:1"}, env)
			if got := find(t, rep, "image").Status; got != tc.wantImage {
				t.Errorf("image = %s, want %s", got, tc.wantImage)
			}
			if got := find(t, rep, "disk").Status; got != tc.wantDisk {
				t.Errorf("disk = %s, want %s", got, tc.wantDisk)
			}
		})
	}
}

func TestMemory(t *testing.T) {
	gib := int64(1 << 30)
	env := Env{Memory: func(context.Context, string) (int64, int64, error) { return 4 * gib, gib, nil }}
	tests := []struct {
		name string
		mem  int64
		want Status
	}{
		{"tiny", 16 << 20, StatusWarn},
		{"fits", 512 << 20, StatusPass},
		{"over free", 2 * gib, StatusWarn},
		{"over total", 8 * gib, StatusFail},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rep := Run(context.Background(), Request{MemoryBytes: tc.mem}, env)
			if got := find(t, rep, "memory").Status; got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestGitEnvMountsGPU(t *testing.T) {
	tests := []struct {
		name string
		req  Request
		env  Env
		id   string
		want Status
	}{
		{"git ok", Request{GitURL: "https://x/y.git", GitBranch: "main"}, Env{Git: fakeGit{}}, "git", StatusPass},
		{"git branch missing", Request{GitURL: "https://x/y.git", GitBranch: "nope"}, Env{Git: fakeGit{ErrGitBranchMissing}}, "git", StatusFail},
		{"git auth", Request{GitURL: "https://x/y.git"}, Env{Git: fakeGit{ErrGitAuth}}, "git", StatusWarn},
		{"git ssh", Request{GitURL: "git@x:y.git"}, Env{Git: fakeGit{ErrGitUnverifiable}}, "git", StatusWarn},
		{"git down", Request{GitURL: "https://x/y.git"}, Env{Git: fakeGit{errors.New("no route")}}, "git", StatusFail},
		{"env missing", Request{RequiredEnv: []string{"A", "B"}, EnvKeys: []string{"A"}}, Env{}, "env", StatusFail},
		{"env ok", Request{RequiredEnv: []string{"A"}, EnvKeys: []string{"A"}}, Env{}, "env", StatusPass},
		{"bind forbidden", Request{BindMounts: []string{"/proc/x"}}, Env{}, "mounts", StatusFail},
		{"bind relative", Request{BindMounts: []string{"data"}}, Env{}, "mounts", StatusFail},
		{"volume relative", Request{VolumePaths: []string{"data"}}, Env{}, "mounts", StatusFail},
		{"mounts ok", Request{BindMounts: []string{"/srv/data"}, VolumePaths: []string{"/data"}}, Env{}, "mounts", StatusPass},
		{"gpu none", Request{GPU: true}, Env{GPUFit: func(context.Context, string) error { return errors.New("no GPU on this node") }}, "gpu", StatusFail},
		{"gpu ok", Request{GPU: true}, Env{GPUFit: func(context.Context, string) error { return nil }}, "gpu", StatusPass},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rep := Run(context.Background(), tc.req, tc.env)
			if got := find(t, rep, tc.id).Status; got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestRunBoundsSlowProbe(t *testing.T) {
	env := Env{
		Limits: Limits{Timeout: 50 * time.Millisecond},
		LookupHost: func(ctx context.Context, _ string) ([]string, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	start := time.Now()
	rep := Run(context.Background(), Request{Domains: []string{"slow.example.com"}}, env)
	if time.Since(start) > 2*time.Second {
		t.Fatal("slow probe not bounded")
	}
	if rep.Status != StatusWarn {
		t.Fatalf("status = %s", rep.Status)
	}
}

func TestReportWorstStatus(t *testing.T) {
	rep := Run(context.Background(), Request{RequiredEnv: []string{"X"}}, Env{})
	if rep.Status != StatusFail {
		t.Fatalf("status = %s", rep.Status)
	}
	if rep := Run(context.Background(), Request{}, Env{}); rep.Status != StatusPass || rep.Checks == nil {
		t.Fatalf("empty report = %+v", rep)
	}
}

func TestParseImageRef(t *testing.T) {
	tests := []struct {
		in   string
		want imageRef
	}{
		{"nginx", imageRef{"registry-1.docker.io", "library/nginx", "latest"}},
		{"acme/web:1.2", imageRef{"registry-1.docker.io", "acme/web", "1.2"}},
		{"ghcr.io/acme/web:v1", imageRef{"ghcr.io", "acme/web", "v1"}},
		{"localhost:5000/app", imageRef{"localhost:5000", "app", "latest"}},
		{"docker.io/library/redis:7", imageRef{"registry-1.docker.io", "library/redis", "7"}},
		{"nginx:1.27@sha256:abc", imageRef{"registry-1.docker.io", "library/nginx", "sha256:abc"}},
		{"ghcr.io/acme/web@sha256:def", imageRef{"ghcr.io", "acme/web", "sha256:def"}},
		{"localhost:5000/app:v2@sha256:0a", imageRef{"localhost:5000", "app", "sha256:0a"}},
	}
	for _, tc := range tests {
		if got := parseImageRef(tc.in); got != tc.want {
			t.Errorf("%s = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestRegistryInspectorTokenFlowAndCache(t *testing.T) {
	hits := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			_, _ = w.Write([]byte(`{"token":"t"}`))
		case strings.HasPrefix(r.URL.Path, "/v2/acme/web/manifests/"):
			hits++
			if r.Header.Get("Authorization") != "Bearer t" {
				w.Header().Set("Www-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="reg",scope="repository:acme/web:pull"`, srv.URL))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if strings.HasSuffix(r.URL.Path, "/missing") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`{"config":{"size":100},"layers":[{"size":1000},{"size":2000}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	insp := &RegistryInspector{Client: srv.Client(), TTL: time.Minute}
	info, err := insp.Inspect(context.Background(), host+"/acme/web:1")
	if err != nil || info.SizeBytes != 3100 {
		t.Fatalf("info=%+v err=%v", info, err)
	}
	before := hits
	if _, err := insp.Inspect(context.Background(), host+"/acme/web:1"); err != nil || hits != before {
		t.Fatalf("second call should be cached, hits %d -> %d, err %v", before, hits, err)
	}
	if _, err := insp.Inspect(context.Background(), host+"/acme/web:missing"); !errors.Is(err, ErrImageNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestSmartHTTPChecker(t *testing.T) {
	refs := "001e# service=git-upload-pack\n0000abc refs/heads/main\x00multi_ack\nabc refs/heads/main-2\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok/info/refs":
			_, _ = w.Write([]byte(refs))
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()
	c := &SmartHTTPChecker{Client: srv.Client()}
	ctx := context.Background()
	if err := c.Check(ctx, srv.URL+"/ok", "main"); err != nil {
		t.Fatalf("main: %v", err)
	}
	if err := c.Check(ctx, srv.URL+"/ok", "main-2"); err != nil {
		t.Fatalf("main-2: %v", err)
	}
	if err := c.Check(ctx, srv.URL+"/ok", "mai"); !errors.Is(err, ErrGitBranchMissing) {
		t.Fatalf("prefix branch should be missing: %v", err)
	}
	if err := c.Check(ctx, srv.URL+"/private", ""); !errors.Is(err, ErrGitAuth) {
		t.Fatalf("private: %v", err)
	}
	if err := c.Check(ctx, "git@github.com:a/b.git", ""); !errors.Is(err, ErrGitUnverifiable) {
		t.Fatalf("ssh: %v", err)
	}
}
