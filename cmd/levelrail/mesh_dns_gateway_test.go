package main

import (
	"reflect"
	"testing"
)

func TestPickContainerDNS(t *testing.T) {
	cases := []struct {
		name                      string
		local                     bool
		gateway, legacy, fallback string
		want                      []string
	}{
		{"remote never gets an address", false, "172.17.0.1", "172.17.0.1", "10.0.0.2", nil},
		{"local confirmed gateway gets the fallback second", true, "172.18.0.1", "172.17.0.1", "10.0.0.2", []string{"172.18.0.1", "10.0.0.2"}},
		{"fail closed keeps only the gateway", true, "172.18.0.1", "", "", []string{"172.18.0.1"}},
		{"local unconfirmed falls back to static", true, "", "172.17.0.1", "10.0.0.2", []string{"172.17.0.1"}},
		{"local with nothing leaves defaults", true, "", "", "", nil},
	}
	for _, c := range cases {
		if got := pickContainerDNS(c.local, c.gateway, c.legacy, c.fallback); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
