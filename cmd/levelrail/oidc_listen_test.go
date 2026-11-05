package main

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHostAddrAssigned(t *testing.T) {
	local := []netip.Prefix{
		netip.MustParsePrefix("192.168.1.5/24"),
		netip.MustParsePrefix("10.88.0.1/16"),
	}
	cases := []struct {
		name  string
		ip    string
		local []netip.Prefix
		want  bool
	}{
		{"podman gateway present", "10.88.0.1", local, true},
		{"podman gateway absent", "10.88.0.1", local[:1], false},
		{"same subnet other host", "10.88.0.2", local, false},
		{"no interfaces", "10.88.0.1", nil, false},
		{"invalid ip", "nope", local, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hostAddrAssigned(tc.ip, tc.local); got != tc.want {
				t.Fatalf("hostAddrAssigned(%q) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

// A zero or negative retry must not turn the bridge wait into a busy loop.
func TestListenWhenAssigned_NonPositiveRetryDoesNotSpin(t *testing.T) {
	for _, tt := range []struct {
		name  string
		retry time.Duration
	}{
		{"zero", 0},
		{"negative", -time.Second},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			var calls atomic.Int64
			local := func() []netip.Prefix { calls.Add(1); return nil }
			ln, err := listenWhenAssigned(ctx, "10.88.0.1:0", tt.retry, local, slog.New(slog.DiscardHandler))
			if ln != nil || err != nil {
				t.Fatalf("listenWhenAssigned() = %v, %v, want nil, nil", ln, err)
			}
			if n := calls.Load(); n > 2 {
				t.Errorf("address checks = %d, want the default retry interval between checks", n)
			}
		})
	}
}

func TestServeOIDCTokenRequests_AddrInUseWarnsOnce(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no loopback listener")
	}
	t.Cleanup(func() { _ = busy.Close() })
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	done := make(chan struct{})
	go func() {
		serveOIDCTokenRequests(context.Background(), busy.Addr().String(), nil, logger)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return on a busy address")
	}
	out := buf.String()
	if strings.Count(out, "level=WARN") != 1 || strings.Contains(out, "level=ERROR") || !strings.Contains(out, "pipelines.oidc_listen_port") {
		t.Fatalf("want a single warn naming the setting, got %s", out)
	}
}

func TestLoadBrand_MissingDefaultFileFallsBack(t *testing.T) {
	t.Setenv("APP_BRAND_FILE", "")
	t.Chdir(t.TempDir())
	b, err := loadBrand()
	if err != nil || b == nil || b.Name == "" {
		t.Fatalf("want the built-in brand, got %v %v", b, err)
	}
	t.Setenv("APP_BRAND_FILE", "/nonexistent/brand.yaml")
	if _, err := loadBrand(); err == nil {
		t.Fatal("an explicit missing APP_BRAND_FILE should still fail")
	}
}
