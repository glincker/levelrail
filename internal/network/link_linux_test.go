package network

import (
	"context"
	"net/netip"
	"strings"
	"testing"
)

// The agent image has no ip(8); the Linux path must never exec it.
func TestSetAddressLinux_DoesNotNeedIPBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	tests := []struct {
		name   string
		prefix netip.Prefix
		want   string
	}{
		{"missing interface reports ioctl error", netip.MustParsePrefix("10.181.0.2/16"), "lvlrail-nope"},
		{"ipv6 refused", netip.MustParsePrefix("fd00::2/64"), "only IPv4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (SystemLinkConfigurator{}).SetAddress(context.Background(), "lvlrail-nope", tt.prefix)
			if err == nil {
				t.Fatal("SetAddress() = nil, want error")
			}
			if strings.Contains(err.Error(), "executable file not found") {
				t.Fatalf("exec path used without ip binary: %v", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestNetipMask4(t *testing.T) {
	for bits, want := range map[int][4]byte{0: {}, 16: {255, 255, 0, 0}, 24: {255, 255, 255, 0}, 30: {255, 255, 255, 252}} {
		if got := netipMask4(bits); got != want {
			t.Errorf("netipMask4(%d) = %v, want %v", bits, got, want)
		}
	}
}
