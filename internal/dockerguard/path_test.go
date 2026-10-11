package dockerguard

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestCanonicalize(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantVersion string
		wantPath    string
		wantErr     bool
	}{
		{name: "plain", raw: "/containers/json", wantPath: "/containers/json"},
		{name: "version prefix kept apart", raw: "/v1.43/containers/json", wantVersion: "v1.43", wantPath: "/containers/json"},
		{name: "double slashes collapse", raw: "//containers//abc///json", wantPath: "/containers/abc/json"},
		{name: "trailing slash dropped", raw: "/v1.47/info/", wantVersion: "v1.47", wantPath: "/info"},
		{name: "image ref with slashes", raw: "/images/ghcr.io/org/app:1.2/json", wantPath: "/images/ghcr.io/org/app:1.2/json"},
		{name: "digest ref", raw: "/distribution/alpine@sha256:abc/json", wantPath: "/distribution/alpine@sha256:abc/json"},
		{name: "dot dot rejected", raw: "/containers/../plugins/pull", wantErr: true},
		{name: "dot rejected", raw: "/containers/./json", wantErr: true},
		{name: "versioned dot dot rejected", raw: "/v1.43/../v1.43/swarm/init", wantErr: true},
		{name: "encoded slash rejected", raw: "/containers/a%2Fb/json", wantErr: true},
		{name: "encoded lower slash rejected", raw: "/images/a%2fb/json", wantErr: true},
		{name: "encoded backslash rejected", raw: "/images/a%5cb/json", wantErr: true},
		{name: "encoded dot dot rejected", raw: "/containers/%2e%2e/plugins", wantErr: true},
		{name: "encoded nul rejected", raw: "/containers/a%00/json", wantErr: true},
		{name: "only version", raw: "/v1.43", wantErr: true},
		{name: "empty", raw: "/", wantErr: true},
		{name: "version lookalike is a segment", raw: "/v1/containers/json", wantPath: "/v1/containers/json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse("http://docker" + tt.raw)
			if err != nil {
				t.Fatalf("parse %q: %v", tt.raw, err)
			}
			got, err := canonicalize(u.EscapedPath(), u.Path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("canonicalize(%q) err = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Version != tt.wantVersion || got.Path != tt.wantPath {
				t.Fatalf("canonicalize(%q) = %+v, want version %q path %q", tt.raw, got, tt.wantVersion, tt.wantPath)
			}
		})
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern, path string
		want          bool
	}{
		{"/containers/*/json", "/containers/abc/json", true},
		{"/containers/*/json", "/containers/json", false},
		{"/containers/*/json", "/containers/a/b/json", false},
		{"/containers/*", "/containers/abc", true},
		{"/containers/*", "/containers/abc/json", false},
		{"/images/**/json", "/images/json", false},
		{"/images/**/json", "/images/alpine/json", true},
		{"/images/**/json", "/images/ghcr.io/org/app:1/json", true},
		{"/images/**/json", "/images/ghcr.io/org/app:1/history", false},
		{"/images/**", "/images/a/b/c", true},
		{"/images/**", "/images", false},
		{"/_ping", "/_ping", true},
		{"/_ping", "/_pingx", false},
	}
	for _, tt := range tests {
		if got := matchPattern(tt.pattern, tt.path); got != tt.want {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

func TestMatchEndpoint(t *testing.T) {
	allowed := []struct{ method, path string }{
		{http.MethodGet, "/_ping"}, {http.MethodHead, "/_ping"}, {http.MethodGet, "/version"},
		{http.MethodGet, "/info"}, {http.MethodGet, "/events"}, {http.MethodGet, "/system/df"},
		{http.MethodPost, "/auth"}, {http.MethodGet, "/containers/json"}, {http.MethodPost, "/containers/create"},
		{http.MethodGet, "/containers/web-1/json"}, {http.MethodPost, "/containers/abc/start"},
		{http.MethodPost, "/containers/abc/stop"}, {http.MethodPost, "/containers/abc/kill"},
		{http.MethodPost, "/containers/abc/wait"}, {http.MethodPost, "/containers/abc/update"},
		{http.MethodDelete, "/containers/abc"}, {http.MethodGet, "/containers/abc/logs"},
		{http.MethodGet, "/containers/abc/stats"}, {http.MethodGet, "/containers/abc/archive"},
		{http.MethodHead, "/containers/abc/archive"}, {http.MethodPut, "/containers/abc/archive"},
		{http.MethodPost, "/containers/abc/exec"}, {http.MethodPost, "/exec/e1/start"},
		{http.MethodPost, "/exec/e1/resize"}, {http.MethodGet, "/exec/e1/json"},
		{http.MethodGet, "/images/json"}, {http.MethodPost, "/images/create"}, {http.MethodPost, "/images/load"},
		{http.MethodGet, "/images/get"}, {http.MethodPost, "/images/prune"},
		{http.MethodGet, "/images/ghcr.io/o/a:1/json"}, {http.MethodDelete, "/images/ghcr.io/o/a:1"},
		{http.MethodPost, "/images/a/tag"}, {http.MethodGet, "/distribution/docker.io/library/alpine:3/json"},
		{http.MethodPost, "/build/prune"}, {http.MethodPost, "/grpc"}, {http.MethodPost, "/session"},
		{http.MethodGet, "/networks"}, {http.MethodPost, "/networks/create"}, {http.MethodGet, "/networks/n1"},
		{http.MethodPost, "/networks/n1/connect"}, {http.MethodPost, "/networks/n1/disconnect"},
		{http.MethodDelete, "/networks/n1"}, {http.MethodGet, "/volumes"}, {http.MethodPost, "/volumes/create"},
		{http.MethodGet, "/volumes/v1"}, {http.MethodDelete, "/volumes/v1"},
	}
	for _, a := range allowed {
		if _, ok := matchEndpoint(a.method, a.path); !ok {
			t.Errorf("%s %s should be allowlisted", a.method, a.path)
		}
	}
	denied := []struct{ method, path string }{
		{http.MethodPost, "/containers/abc/attach"}, {http.MethodGet, "/containers/abc/attach/ws"},
		{http.MethodPost, "/containers/prune"}, {http.MethodPost, "/containers/abc/rename"},
		{http.MethodPost, "/containers/abc/commit"}, {http.MethodGet, "/containers/abc/export"},
		{http.MethodPost, "/build"}, {http.MethodPost, "/plugins/pull"}, {http.MethodGet, "/plugins"},
		{http.MethodPost, "/swarm/init"}, {http.MethodPost, "/services/create"}, {http.MethodPost, "/secrets/create"},
		{http.MethodPost, "/configs/create"}, {http.MethodPost, "/volumes/prune"}, {http.MethodPost, "/networks/prune"},
		{http.MethodPost, "/images/a/push"}, {http.MethodPost, "/commit"}, {http.MethodPost, "/exec/e1/stop"},
		{http.MethodPut, "/containers/create"}, {http.MethodDelete, "/volumes"}, {http.MethodGet, "/nodes"},
	}
	for _, d := range denied {
		if _, ok := matchEndpoint(d.method, d.path); ok {
			t.Errorf("%s %s must not be allowlisted", d.method, d.path)
		}
	}
}

// FuzzCanonicalize checks that whatever canonicalize accepts is something
// the matcher and the daemon read identically: no dot segments, no empty
// segments, no encoded separators.
func FuzzCanonicalize(f *testing.F) {
	for _, s := range []string{"/v1.43/containers/create", "//containers/..//x", "/images/a%2Fb/json", "/containers/%2e%2e/x", "/v1.43/v1.43/info", "/images/a/b/c/json"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		u, err := url.Parse("http://docker" + raw)
		if err != nil || !strings.HasPrefix(raw, "/") {
			return
		}
		got, err := canonicalize(u.EscapedPath(), u.Path)
		if err != nil {
			return
		}
		if strings.Contains(got.Path, "//") || !strings.HasPrefix(got.Path, "/") {
			t.Fatalf("non-canonical result %q from %q", got.Path, raw)
		}
		for _, seg := range strings.Split(got.Path, "/")[1:] {
			if seg == "" || seg == "." || seg == ".." || strings.ContainsAny(seg, "\\\x00") {
				t.Fatalf("bad segment %q in %q from %q", seg, got.Path, raw)
			}
		}
		fwd := &url.URL{Path: got.Forward()}
		if again, err := canonicalize(fwd.EscapedPath(), fwd.Path); err != nil || again != got {
			t.Fatalf("canonicalize not idempotent for %q: %+v vs %+v (%v)", raw, again, got, err)
		}
	})
}
