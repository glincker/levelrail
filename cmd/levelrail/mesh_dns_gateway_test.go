package main

import "testing"

func TestPickContainerDNS(t *testing.T) {
	cases := []struct {
		name            string
		local           bool
		gateway, legacy string
		want            string
	}{
		{"remote never gets an address", false, "172.17.0.1", "172.17.0.1", ""},
		{"local confirmed gateway wins", true, "172.18.0.1", "172.17.0.1", "172.18.0.1"},
		{"local unconfirmed falls back to static", true, "", "172.17.0.1", "172.17.0.1"},
		{"local with nothing leaves defaults", true, "", "", ""},
	}
	for _, c := range cases {
		if got := pickContainerDNS(c.local, c.gateway, c.legacy); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}
