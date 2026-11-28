package api

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/glincker/levelrail/internal/network"
)

const (
	doctorMeshDNSCode = "mesh_dns"
	doctorMeshDNSDocs = "/mesh-dns"
)

// SetMeshDNSStatus wires the bridge gateway resolver's status; nil keeps the check silent.
func (rt *Router) SetMeshDNSStatus(f func(ctx context.Context) network.GatewayStatus) {
	rt.meshDNSStatus = f
}

func (rt *Router) doctorCheckMeshDNS(ctx context.Context) []doctorCheckResource {
	if rt.meshDNSStatus == nil {
		return nil
	}
	return meshDNSDoctorRows(rt.meshDNSStatus(ctx))
}

// meshDNSDoctorRows says nothing while the beta setting is off.
func meshDNSDoctorRows(st network.GatewayStatus) []doctorCheckResource {
	if !st.Enabled {
		return nil
	}
	rows := []doctorCheckResource{meshDNSListenerRow(st)}
	rows = append(rows, meshDNSFirewallRow(st), meshDNSUsageRow(st))
	rows = append(rows, meshDNSFindingRows(st.FirewallFindings)...)
	return rows
}

func meshDNSListenerRow(st network.GatewayStatus) doctorCheckResource {
	row := doctorCheckResource{Code: doctorMeshDNSCode + "_listener", Name: "Mesh DNS listener", DocsPath: doctorMeshDNSDocs}
	var bound, bad []string
	for _, l := range st.Listeners {
		if l.Bound {
			bound = append(bound, l.Addr)
			continue
		}
		bad = append(bad, fmt.Sprintf("%s (%s)", l.Addr, l.Code))
	}
	switch {
	case len(bound) == 0 && len(bad) == 0:
		row.Status, row.Message = doctorStatusWarn, "no managed bridge was found to bind to; containers keep Docker's resolver"
	case len(bad) > 0:
		row.Status = doctorStatusWarn
		row.Message = "not bound: " + strings.Join(bad, ", ") + "; those containers keep Docker's resolver"
		row.Fix = "free port 53 on the bridge gateway address, then wait for the next sync"
	default:
		row.Status, row.Message = doctorStatusOK, "bound on "+strings.Join(bound, ", ")
	}
	return row
}

func meshDNSFirewallRow(st network.GatewayStatus) doctorCheckResource {
	row := doctorCheckResource{Code: doctorMeshDNSCode + "_firewall", Name: "Mesh DNS firewall rule", DocsPath: doctorMeshDNSDocs}
	var missing []string
	for n, ok := range st.FirewallPresent {
		if !ok {
			missing = append(missing, n)
		}
	}
	sort.Strings(missing)
	switch {
	case st.FirewallError != "":
		row.Status = doctorStatusWarn
		row.Message = "could not check the INPUT accept for udp and tcp 53: " + st.FirewallError
		row.Fix = "run the control plane as root, or add an INPUT accept for udp and tcp 53 from the bridge interfaces yourself"
	case len(missing) > 0:
		row.Status = doctorStatusFail
		row.Message = "INPUT accept for udp and tcp 53 is missing on " + strings.Join(missing, ", ") + ", containers cannot reach the resolver"
		row.Fix = "wait for the next sync, or add the accept for those interfaces"
	default:
		row.Status, row.Message = doctorStatusOK, "INPUT accept present via "+st.FirewallBackend+doctorMeshDNSIPv6Note
	}
	return row
}

func meshDNSUsageRow(st network.GatewayStatus) doctorCheckResource {
	row := doctorCheckResource{Code: doctorMeshDNSCode + "_containers", Name: "Containers using mesh DNS", DocsPath: doctorMeshDNSDocs}
	row.Status = doctorStatusOK
	row.Message = fmt.Sprintf("%d of %d app containers on this node use it", st.ContainersUsing, st.ContainersTotal)
	if st.ContainersTotal > 0 && st.ContainersUsing == 0 {
		row.Status = doctorStatusWarn
		row.Message += "; existing containers pick it up when they are recreated"
	}
	return row
}
