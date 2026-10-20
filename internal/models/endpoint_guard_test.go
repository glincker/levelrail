package models

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

func fixedLookup(addrs ...string) HostLookup {
	return func(context.Context, string) ([]netip.Addr, error) {
		var out []netip.Addr
		for _, a := range addrs {
			out = append(out, netip.MustParseAddr(a))
		}
		return out, nil
	}
}

func TestNormalizeEndpointURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		lookup  HostLookup
		allow   string
		want    string
		wantErr error
	}{
		{name: "private refused without opt in", raw: "http://192.168.1.20:11434", allow: "false", wantErr: ErrEndpointPrivate},
		{name: "private allowed with opt in", raw: "http://192.168.1.20:11434/", allow: "true", want: "http://192.168.1.20:11434"},
		{name: "mesh cgnat allowed with opt in", raw: "http://100.64.0.7:11434", allow: "true", want: "http://100.64.0.7:11434"},
		{name: "v1 suffix trimmed", raw: "http://10.0.0.5:1234/v1", allow: "true", want: "http://10.0.0.5:1234"},
		{name: "loopback needs opt in", raw: "http://127.0.0.1:11434", allow: "false", wantErr: ErrEndpointPrivate},
		{name: "aws metadata refused even with opt in", raw: "http://169.254.169.254/latest", allow: "true", wantErr: ErrEndpointBlocked},
		{name: "link local v6 refused even with opt in", raw: "http://[fe80::1]:11434", allow: "true", wantErr: ErrEndpointBlocked},
		{name: "aws v6 metadata refused", raw: "http://[fd00:ec2::254]", allow: "true", wantErr: ErrEndpointBlocked},
		{name: "alibaba metadata refused", raw: "http://100.100.100.200", allow: "true", wantErr: ErrEndpointBlocked},
		{name: "oracle metadata refused", raw: "http://192.0.0.192", allow: "true", wantErr: ErrEndpointBlocked},
		{name: "unspecified refused", raw: "http://0.0.0.0:11434", allow: "true", wantErr: ErrEndpointBlocked},
		{name: "mapped v4 metadata refused", raw: "http://[::ffff:169.254.169.254]", allow: "true", wantErr: ErrEndpointBlocked},
		{name: "nat64 refused", raw: "http://[64:ff9b::a9fe:a9fe]", allow: "true", wantErr: ErrEndpointBlocked},
		{name: "name resolving to metadata refused", raw: "http://sneaky.example.com", lookup: fixedLookup("169.254.169.254"), allow: "true", wantErr: ErrEndpointBlocked},
		{name: "name resolving to private refused without opt in", raw: "http://mac.example.com", lookup: fixedLookup("10.1.1.1"), allow: "false", wantErr: ErrEndpointPrivate},
		{name: "public https ok", raw: "https://api.example.com/v1", lookup: fixedLookup("93.184.216.34"), allow: "false", want: "https://api.example.com"},
		{name: "public http refused", raw: "http://api.example.com", lookup: fixedLookup("93.184.216.34"), allow: "false", wantErr: ErrInvalid},
		{name: "userinfo refused", raw: "http://user:pw@10.0.0.5", allow: "true", wantErr: ErrInvalid}, //nolint:gosec // fake userinfo in a test URL
		{name: "query refused", raw: "http://10.0.0.5/?x=1", allow: "true", wantErr: ErrInvalid},
		{name: "scheme refused", raw: "file:///etc/passwd", allow: "true", wantErr: ErrInvalid},
		{name: "empty refused", raw: "", allow: "true", wantErr: ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EndpointAllowPrivateEnv, tt.allow)
			t.Setenv("APP_NOTIFY_ALLOW_PRIVATE_NETWORKS", "false")
			got, err := NormalizeEndpointURL(context.Background(), tt.raw, tt.lookup)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v, want %q", got, err, tt.want)
			}
		})
	}
}

func TestEndpointDialControl(t *testing.T) {
	t.Setenv(EndpointAllowPrivateEnv, "true")
	for addr, wantBlocked := range map[string]bool{
		"169.254.169.254:80": true, "[fe80::1]:80": true, "10.0.0.5:80": false, "127.0.0.1:80": false, "93.184.216.34:443": false,
	} {
		err := endpointDialControl("tcp", addr, nil)
		if (err != nil) != wantBlocked {
			t.Errorf("%s: err = %v, want blocked = %v", addr, err, wantBlocked)
		}
	}
	t.Setenv(EndpointAllowPrivateEnv, "false")
	t.Setenv("APP_NOTIFY_ALLOW_PRIVATE_NETWORKS", "false")
	if err := endpointDialControl("tcp", "10.0.0.5:80", nil); !errors.Is(err, ErrEndpointPrivate) {
		t.Errorf("private without opt in: %v", err)
	}
}
