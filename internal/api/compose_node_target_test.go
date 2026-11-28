package api

import (
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/dockerguard"
	"github.com/glincker/levelrail/internal/store"
)

func TestRemoteGrantRefusalNamesServiceAndNode(t *testing.T) {
	plain := store.DesiredService{Name: "app-web"}
	if msg := remoteGrantRefusal(plain, "oci1"); msg != "" {
		t.Fatalf("no grant must not refuse, got %q", msg)
	}
	hostNet := store.DesiredService{Name: "app-vpn", GrantedCaps: []string{dockerguard.HostNetworkGrantCap}}
	msg := remoteGrantRefusal(hostNet, "oci1")
	for _, want := range []string{"app-vpn", "host_network", `"oci1"`, "control plane"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("refusal %q misses %q", msg, want)
		}
	}
}
