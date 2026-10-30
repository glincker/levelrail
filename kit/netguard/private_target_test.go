package netguard

import (
	"net/netip"
	"testing"
)

func TestIsPrivateTargetAllowed(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1":              true,
		"10.1.2.3":               true,
		"192.168.0.9":            true,
		"::1":                    true,
		"8.8.8.8":                true,
		"169.254.169.254":        false,
		"fe80::1":                false,
		"0.0.0.0":                false,
		"100.100.100.200":        false,
		"fd00:ec2::254":          false,
		"224.0.0.1":              false,
		"fd00:ec2::254%eth0":     false,
		"fe80::1%eth0":           false,
		"fd12::1%eth0":           true,
		"::ffff:169.254.169.254": false,
		"::ffff:100.100.100.200": false,
	}
	for in, want := range cases {
		if got := IsPrivateTargetAllowed(netip.MustParseAddr(in)); got != want {
			t.Errorf("%s: got %v want %v", in, got, want)
		}
	}
}

func TestIsBlockedUnchangedForLoopback(t *testing.T) {
	if !IsBlocked(netip.MustParseAddr("127.0.0.1")) {
		t.Fatal("general guard must still block loopback")
	}
}
