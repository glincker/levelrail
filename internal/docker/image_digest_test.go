package docker

import (
	"context"
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
				_, _ = w.Write([]byte(`{"message":"registry unreachable"}`))
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
