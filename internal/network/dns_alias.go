package network

import (
	"net/netip"
	"strings"
)

// aliasRecords adds a record per alias. A name already taken is left to its
// first owner and reported, so one bad alias never drops the whole pass.
func aliasRecords(set *RecordSet, seen map[string]string, zone string, p Placement, nodeID string, addr netip.Addr) {
	suffix := "." + zone
	for _, a := range p.Aliases {
		name := strings.ToLower(strings.TrimSuffix(a, "."))
		if !strings.HasSuffix(name, suffix) {
			name += suffix
		}
		if other, dup := seen[name]; dup {
			set.Unresolved = append(set.Unresolved, Unresolved{
				Service: p.Service, NodeID: nodeID,
				Reason: "alias " + name + " is already used by " + other,
			})
			continue
		}
		seen[name] = p.Service
		set.Records = append(set.Records, Record{Name: name, Address: addr, Service: p.Service, NodeID: nodeID})
	}
}
