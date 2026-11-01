package ingress

import (
	"context"
	"testing"
)

func TestDNS01AllHostsFollowsLiveSwitch(t *testing.T) {
	on := false
	c := New(nil, nil, nil, WithDNS01AllHosts(func(context.Context) bool { return on }))
	if c.dns01AllHostsOn(context.Background()) {
		t.Fatal("want off before the switch")
	}
	on = true
	if !c.dns01AllHostsOn(context.Background()) {
		t.Fatal("a changed setting must reach the next pass without a restart")
	}
	if New(nil, nil, nil).dns01AllHostsOn(context.Background()) {
		t.Fatal("no switch wired means off")
	}
}
