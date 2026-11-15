package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/network"
)

func TestMeshDNSDoctorRows(t *testing.T) {
	if rows := meshDNSDoctorRows(network.GatewayStatus{}); rows != nil {
		t.Fatalf("disabled must be silent, got %v", rows)
	}
	st := network.GatewayStatus{
		GatewayReport:   network.GatewayReport{Enabled: true, Listeners: []network.GatewayListener{{Addr: "172.17.0.1:53", Bound: true}}},
		FirewallBackend: "iptables", FirewallPresent: map[string]bool{"docker0": true},
		ContainersUsing: 2, ContainersTotal: 2,
	}
	for _, r := range meshDNSDoctorRows(st) {
		if r.Status != doctorStatusOK {
			t.Fatalf("healthy state row %s is %s", r.Code, r.Status)
		}
	}
	st.FirewallPresent = map[string]bool{"docker0": false}
	st.Listeners = []network.GatewayListener{{Addr: "172.17.0.1:53", Code: network.GatewayReasonInUse}}
	rows := meshDNSDoctorRows(st)
	if rows[0].Status != doctorStatusWarn || rows[1].Status != doctorStatusFail {
		t.Fatalf("unhealthy rows wrong: %+v", rows)
	}
}
