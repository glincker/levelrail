package main

import (
	"net/netip"
	"testing"
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
