package ingress

import (
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// crossNodeIngressCondition reports every service whose domains can never
// be reached: no mesh path lets this node's Caddy forward to a container
// placed elsewhere. Returns nil when nothing is affected, mirroring
// lbCondition's own "omit when unused" shape.
func crossNodeIngressCondition(services []store.DesiredService, isLocal func(string) bool) *reconcile.Condition {
	var unreachable []string
	for _, svc := range services {
		if len(svc.Domains) == 0 || isLocal(svc.NodeID) {
			continue
		}
		unreachable = append(unreachable, fmt.Sprintf("%s (node %s, domain(s) %s)", svc.Name, svc.NodeID, strings.Join(svc.Domains, ", ")))
	}
	if len(unreachable) == 0 {
		return nil
	}
	return &reconcile.Condition{
		Type:   "CrossNodeIngress",
		Status: reconcile.ConditionFalse,
		Reason: "NoMeshIngressPath",
		Message: fmt.Sprintf(
			"%d service(s) have domains routed by this node's ingress but are placed on a different node with no mesh path to reach them yet: %s",
			len(unreachable), strings.Join(unreachable, "; "),
		),
	}
}
