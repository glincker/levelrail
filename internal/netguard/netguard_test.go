package netguard

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestIsBlocked(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"10.1.2.3", true},
		{"172.16.0.1", true},
		{"192.168.1.10", true},
		{"169.254.169.254", true},
		{"fd00:ec2::254", true},
		{"fe80::1", true},
		{"0.0.0.0", true},
		{"::", true},
		{"100.100.100.200", true},
		{"224.0.0.1", true},
		{"255.255.255.255", true},
		{"::ffff:127.0.0.1", true},
		{"::ffff:169.254.169.254", true},
		{"64:ff9b::a9fe:a9fe", true},
		{"2002:a9fe:a9fe::1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"2606:4700:4700::1111", false},
		{"64:ff9b::808:808", false},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			if got := IsBlocked(netip.MustParseAddr(tt.addr)); got != tt.want {
				t.Errorf("IsBlocked(%s) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}

func TestNewClient_RefusesLoopbackAndMetadata(t *testing.T) {
	t.Setenv(AllowPrivateEnv, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	for _, target := range []string{srv.URL, "http://169.254.169.254/latest/meta-data/", "http://localhost:1/"} {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := NewClient().Do(req)
		if err == nil {
			_ = resp.Body.Close()
			t.Fatalf("GET %s succeeded, want a blocked-address error", target)
		}
		if !errors.Is(err, ErrBlockedAddress) {
			t.Fatalf("GET %s error = %v, want ErrBlockedAddress", target, err)
		}
	}
}

func TestNewClient_OptInTogglesInternalAccess(t *testing.T) {
	t.Setenv(AllowPrivateEnv, "true")
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer internal.Close()
	client := NewClient()

	// With the opt-in on, the loopback server is reachable, proving the
	// check (not the server) is what refuses it once the opt-in is off.
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, internal.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET with %s=true: %v", AllowPrivateEnv, err)
	}
	_ = resp.Body.Close()

	t.Setenv(AllowPrivateEnv, "false")
	client.CloseIdleConnections()
	req, _ = http.NewRequestWithContext(context.Background(), http.MethodGet, internal.URL, nil)
	if resp, err := client.Do(req); !errors.Is(err, ErrBlockedAddress) {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatalf("GET after disabling opt-in: err = %v, want ErrBlockedAddress", err)
	}
}
