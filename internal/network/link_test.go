package network

import (
	"context"
	"net/netip"
	"testing"
)

func TestSystemLinkConfigurator_SetAddress_InvalidPrefix_ReturnsError(t *testing.T) {
	var c SystemLinkConfigurator
	if err := c.SetAddress(context.Background(), "wg0", netip.Prefix{}); err == nil {
		t.Fatal("SetAddress() error = nil, want an error for an invalid prefix")
	}
}

func TestMaskString(t *testing.T) {
	tests := []struct {
		bits int
		want string
	}{
		{0, "0.0.0.0"},
		{16, "255.255.0.0"},
		{24, "255.255.255.0"},
		{32, "255.255.255.255"},
	}
	for _, tt := range tests {
		if got := maskString(tt.bits); got != tt.want {
			t.Errorf("maskString(%d) = %q, want %q", tt.bits, got, tt.want)
		}
	}
}
