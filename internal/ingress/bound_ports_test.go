package ingress

import (
	"context"
	"testing"
	"time"
)

// A reload with a changed route set must keep the ephemeral listener, so a
// URL handed out earlier in dev stays reachable.
func TestDriver_EphemeralPortSurvivesReload(t *testing.T) {
	skipUnderLoad(t)
	d := New(testLogger(t))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	t.Cleanup(func() { _ = d.Stop(ctx) })

	var first int
	storage := t.TempDir()
	for i, backend := range []string{"127.0.0.1:9", "127.0.0.1:10"} {
		cfg, err := BuildProxyConfig(ProxyOptions{ServerName: "stable", ListenAddr: "127.0.0.1:0", BackendDial: backend, StorageDir: storage})
		if err != nil {
			t.Fatal(err)
		}
		if err := d.Apply(ctx, cfg); err != nil {
			t.Fatalf("apply %d: %v", i, err)
		}
		_, https := d.BoundPorts()
		if https == 0 {
			t.Fatalf("apply %d: no bound port", i)
		}
		if i == 0 {
			first = https
		} else if https != first {
			t.Fatalf("port changed across reload: %d then %d", first, https)
		}
	}
}

func TestIsPlainHTTPServer(t *testing.T) {
	for name, want := range map[string]bool{
		autoRedirectServer: true,
		"srv-http":         true,
		"srv-plain":        true,
		"srv":              false,
		"levelrail":        false,
	} {
		if got := isPlainHTTPServer(name); got != want {
			t.Errorf("isPlainHTTPServer(%q) = %v, want %v", name, got, want)
		}
	}
}
