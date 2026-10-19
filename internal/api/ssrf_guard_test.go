package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

var blockedTargets = []string{
	"http://169.254.169.254/latest/meta-data/",
	"http://10.0.0.1/",
	"http://192.168.1.1/",
	"http://172.16.0.1/",
	"http://[::1]/",
	"http://[fe80::1]/",
}

func TestProbePackageRegistry_RefusesInternalAddresses(t *testing.T) {
	t.Setenv("APP_NOTIFY_ALLOW_PRIVATE_NETWORKS", "false")
	var hits atomic.Int32
	loop := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(loop.Close)

	for _, target := range append([]string{loop.URL}, blockedTargets...) {
		ok, msg := probePackageRegistry(context.Background(), store.PackageRegistry{URL: target, Username: "u"}, "tok")
		if ok {
			t.Errorf("probe of %s succeeded, want refusal (msg %q)", target, msg)
		}
	}
	if hits.Load() != 0 {
		t.Errorf("loopback server was contacted %d times, want 0", hits.Load())
	}
}

func TestFetchSAMLIdPMetadata_RefusesInternalAddresses(t *testing.T) {
	t.Setenv("APP_NOTIFY_ALLOW_PRIVATE_NETWORKS", "false")
	var hits atomic.Int32
	loop := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(loop.Close)

	targets := []string{loop.URL, "https://169.254.169.254/", "https://10.0.0.1/", "https://192.168.1.1/", "https://[::1]/"}
	for _, target := range targets {
		if _, err := authengine.FetchSAMLIdPMetadata(context.Background(), nil, target); err == nil {
			t.Errorf("fetch of %s succeeded, want refusal", target)
		}
	}
	if hits.Load() != 0 {
		t.Errorf("loopback server was contacted %d times, want 0", hits.Load())
	}
}
