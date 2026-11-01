package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseRef(t *testing.T) {
	for in, want := range map[string]imageRef{
		"postgres:16":              {"registry-1.docker.io", "library/postgres", "16"},
		"grafana/grafana":          {"registry-1.docker.io", "grafana/grafana", "latest"},
		"ghcr.io/itzg/mc:java21":   {"ghcr.io", "itzg/mc", "java21"},
		"lscr.io/linuxserver/x:v1": {"lscr.io", "linuxserver/x", "v1"},
		"docker.io/library/nginx":  {"registry-1.docker.io", "library/nginx", "latest"},
		"a.io/b@sha256:abc":        {"a.io", "b", "sha256:abc"},
		"localhost:5000/app:2":     {"localhost:5000", "app", "2"},
	} {
		got, err := parseRef(in)
		if err != nil || got != want {
			t.Errorf("%s -> %+v %v, want %+v", in, got, err, want)
		}
	}
}

// fakeRegistry serves a token endpoint and manifests; the client is pointed at it via a rewriting transport.
func fakeRegistry(t *testing.T, handler http.HandlerFunc) *http.Client {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	c := srv.Client()
	base := c.Transport
	c.Transport = rewrite{base: base, to: strings.TrimPrefix(srv.URL, "https://")}
	return c
}

type rewrite struct {
	base http.RoundTripper
	to   string
}

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Host = r.to
	return r.base.RoundTrip(req)
}

const indexBody = `{"manifests":[{"platform":{"os":"linux","architecture":"amd64"}},{"platform":{"os":"linux","architecture":"arm64"}}]}`

func TestRegistryFetcher_IndexWithTokenChallenge(t *testing.T) {
	c := fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/token") {
			if r.URL.Query().Get("scope") != "repository:a/b:pull" {
				t.Errorf("scope %q", r.URL.Query().Get("scope"))
			}
			_, _ = w.Write([]byte(`{"token":"tok"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.Header().Set("Www-Authenticate", `Bearer realm="https://reg.test/token",service="reg",scope="repository:a/b:pull"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(indexBody))
	})
	got, err := registryFetcher(c)(context.Background(), "reg.test/a/b:1")
	if err != nil || len(got) != 2 || got[1] != "linux/arm64" {
		t.Fatalf("got %v %v", got, err)
	}
}

func TestRegistryFetcher_SingleManifestReadsConfigBlob(t *testing.T) {
	c := fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/manifests/"):
			_, _ = w.Write([]byte(`{"config":{"digest":"sha256:cfg"},"layers":[]}`))
		case strings.Contains(r.URL.Path, "/blobs/sha256:cfg"):
			_, _ = w.Write([]byte(`{"os":"linux","architecture":"amd64"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	got, err := registryFetcher(c)(context.Background(), "reg.test/a/b:1")
	if err != nil || len(got) != 1 || got[0] != "linux/amd64" {
		t.Fatalf("got %v %v", got, err)
	}
}

func TestRegistryFetcher_Statuses(t *testing.T) {
	for code, check := range map[int]func(error) bool{
		http.StatusTooManyRequests: func(e error) bool { return errors.Is(e, errRateLimited) },
		http.StatusNotFound:        func(e error) bool { return e != nil && strings.Contains(e.Error(), "404") },
		http.StatusForbidden:       func(e error) bool { return e != nil && strings.Contains(e.Error(), "denied") },
	} {
		c := fakeRegistry(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) })
		_, err := registryFetcher(c)(context.Background(), "reg.test/a/b:1")
		if !check(err) {
			t.Errorf("status %d -> %v", code, err)
		}
	}
}
