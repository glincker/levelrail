package docker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testIndexDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	testCacheDigest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

func newFakeDaemonClient(t *testing.T, distStatus int, localStatus int, localBody string) *Client {
	t.Helper()
	return newFakeDaemonClientDist(t, distStatus, `{"message":"registry unreachable"}`, localStatus, localBody)
}

func newFakeDaemonClientDist(t *testing.T, distStatus int, distErrBody string, localStatus int, localBody string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.Header().Set("Api-Version", "1.47")
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/distribution/"):
			w.WriteHeader(distStatus)
			if distStatus == http.StatusOK {
				_, _ = w.Write([]byte(`{"Descriptor":{"mediaType":"application/vnd.oci.image.index.v1+json","digest":"` + testIndexDigest + `","size":1}}`))
			} else {
				_, _ = w.Write([]byte(distErrBody))
			}
		case strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
			w.WriteHeader(localStatus)
			_, _ = w.Write([]byte(localBody))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(srv.URL, "http://"))
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestResolveImage(t *testing.T) {
	notFound := `{"message":"No such image"}`
	cached := `{"Id":"sha256:cfg","RepoDigests":["nginx@` + testCacheDigest + `"]}`
	localOnly := `{"Id":"sha256:localonly","RepoDigests":[]}`
	tests := []struct {
		name         string
		ref          string
		dist, local  int
		body         string
		requireFresh bool
		wantRef      string
		wantDigest   string
		wantLocalID  string
		wantSource   ImageSource
		wantErr      bool
	}{
		{name: "registry resolves the tag", ref: "nginx:latest", dist: 200, local: 404, body: notFound, wantRef: "nginx:latest@" + testIndexDigest, wantDigest: testIndexDigest, wantSource: ImageSourceRegistry},
		{name: "already pinned skips the registry", ref: "nginx:latest@" + testCacheDigest, dist: 500, local: 404, body: notFound, wantRef: "nginx:latest@" + testCacheDigest, wantDigest: testCacheDigest, wantSource: ImageSourcePinned},
		{name: "registry down uses the cached digest", ref: "nginx:latest", dist: 500, local: 200, body: cached, wantRef: "nginx:latest@" + testCacheDigest, wantDigest: testCacheDigest, wantSource: ImageSourceCache},
		{name: "registry down local-only image keeps the tag", ref: "myapp:dev", dist: 500, local: 200, body: localOnly, wantRef: "myapp:dev", wantLocalID: "sha256:localonly", wantSource: ImageSourceCache},
		{name: "registry down and nothing cached", ref: "nginx:latest", dist: 500, local: 404, body: notFound, wantRef: "nginx:latest", wantSource: ImageSourceNone},
		{name: "registry says the tag does not exist and nothing cached", ref: "nginx:nosuchtag", dist: 404, local: 404, body: notFound, wantErr: true},
		{name: "require fresh fails when registry is down", ref: "nginx:latest", dist: 500, local: 200, body: cached, requireFresh: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newFakeDaemonClient(t, tt.dist, tt.local, tt.body)
			got, err := c.ResolveImage(context.Background(), tt.ref, nil, tt.requireFresh)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Ref != tt.wantRef || got.Digest != tt.wantDigest || got.LocalID != tt.wantLocalID || got.Source != tt.wantSource {
				t.Fatalf("got %+v", got)
			}
			if (tt.wantSource == ImageSourceCache || tt.wantSource == ImageSourceNone) && got.RegistryErr == nil {
				t.Fatal("fallback must carry the registry error")
			}
		})
	}
}

// Podman has no /distribution endpoint: its 404 must not read as "tag missing".
func TestResolveImageEngineWithoutDistributionEndpoint(t *testing.T) {
	for _, body := range []string{`{"cause":"","message":"Path /v1.40/distribution/nginx:1.27-alpine/json is not supported","response":0}`, `{"message":"page not found"}`} {
		c := newFakeDaemonClientDist(t, 404, body, 404, `{"message":"No such image"}`)
		got, err := c.ResolveImage(context.Background(), "nginx:1.27-alpine", nil, false)
		if err != nil {
			t.Fatalf("body %q: %v", body, err)
		}
		if got.Source != ImageSourceNone || got.Ref != "nginx:1.27-alpine" {
			t.Fatalf("body %q: got %+v", body, got)
		}
	}
	c := newFakeDaemonClientDist(t, 404, `{"message":"manifest unknown"}`, 404, `{"message":"No such image"}`)
	if _, err := c.ResolveImage(context.Background(), "nginx:nosuchtag", nil, false); !errors.Is(err, ErrImageNotFound) {
		t.Fatalf("registry manifest unknown must still be ErrImageNotFound, got %v", err)
	}
}

func TestPinAndUnpinImageRef(t *testing.T) {
	if got := PinImageRef("nginx:1", "sha256:a"); got != "nginx:1@sha256:a" {
		t.Fatalf("pin = %q", got)
	}
	if got := PinImageRef("nginx:1@sha256:a", "sha256:b"); got != "nginx:1@sha256:a" {
		t.Fatalf("repin = %q", got)
	}
	if got := UnpinImageRef("registry:5000/app:1@sha256:a"); got != "registry:5000/app:1" {
		t.Fatalf("unpin = %q", got)
	}
	if got := ImageDigestOf("app:1@sha256:a"); got != "sha256:a" {
		t.Fatalf("digest = %q", got)
	}
	if got := ImageDigestOf("app:1"); got != "" {
		t.Fatalf("digest of unpinned = %q", got)
	}
}

// On an engine without /distribution, --pull (requireFresh) pulls through the engine instead of failing.
func TestResolveImageRequireFreshPullsOnEngineWithoutDistribution(t *testing.T) {
	pulled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.Header().Set("Api-Version", "1.47")
		case strings.Contains(r.URL.Path, "/distribution/"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Path /distribution/x/json is not supported"}`))
		case strings.HasSuffix(r.URL.Path, "/images/create"):
			pulled = true
			_, _ = w.Write([]byte(`{"status":"Pull complete"}` + "\n"))
		case strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
			_, _ = w.Write([]byte(`{"Id":"sha256:cfg","RepoDigests":["docker.io/library/nginx@` + testCacheDigest + `"]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(srv.URL, "http://"))
	c, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	got, err := c.ResolveImage(context.Background(), "nginx:1.27-alpine", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if !pulled || got.Digest != testCacheDigest || got.Source != ImageSourceRegistry {
		t.Fatalf("pulled=%v got %+v", pulled, got)
	}
}

// Podman 5 answers every /distribution path with a bare text/plain 404 "Not Found".
func TestResolveImageRequireFreshPodmanBareNotFound(t *testing.T) {
	for _, tc := range []struct {
		name      string
		pullCode  int
		pullBody  string
		wantFound bool
	}{
		{"pulls", http.StatusOK, `{"status":"Pull complete"}` + "\n", true},
		{"missing tag", http.StatusNotFound, `{"message":"initializing source docker://traefik/whoami:nope: reading manifest nope: manifest unknown"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/_ping"):
					w.Header().Set("Api-Version", "1.41")
				case strings.Contains(r.URL.Path, "/distribution/"):
					w.Header().Set("Content-Type", "text/plain; charset=utf-8")
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte("Not Found\n"))
				case strings.HasSuffix(r.URL.Path, "/images/create"):
					w.WriteHeader(tc.pullCode)
					_, _ = w.Write([]byte(tc.pullBody))
				case strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
					_, _ = w.Write([]byte(`{"Id":"sha256:cfg","RepoDigests":["docker.io/traefik/whoami@` + testCacheDigest + `"]}`))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(srv.Close)
			t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(srv.URL, "http://"))
			c, err := NewClient()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = c.Close() })
			got, err := c.ResolveImage(context.Background(), "traefik/whoami:v1.10.1", nil, true)
			if tc.wantFound {
				if err != nil || got.Digest != testCacheDigest {
					t.Fatalf("got %+v err %v", got, err)
				}
				return
			}
			if !errors.Is(err, ErrImageNotFound) {
				t.Fatalf("missing tag must be ErrImageNotFound, got %v", err)
			}
		})
	}
}
