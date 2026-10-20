package ingress

import (
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/reconcile"
)

const (
	conditionCrossNodeIngress = "CrossNodeIngress"
	reasonNoMeshIngressPath   = "NoMeshIngressPath"
)

// remoteBlock is one remote service whose hosts cannot be routed this pass,
// with the reason and next action for the operator.
type remoteBlock struct {
	Service string
	NodeID  string
	Hosts   []string
	Why     string
}

// crossNodeIngressCondition reports every remote service this node's Caddy
// cannot reach over the mesh. Returns nil when none is blocked, which
// clears the condition on the next pass.
func crossNodeIngressCondition(blocks []remoteBlock) *reconcile.Condition {
	if len(blocks) == 0 {
		return nil
	}
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		parts = append(parts, fmt.Sprintf("%s (node %s, host(s) %s): %s", b.Service, b.NodeID, strings.Join(b.Hosts, ", "), b.Why))
	}
	return &reconcile.Condition{
		Type:   conditionCrossNodeIngress,
		Status: reconcile.ConditionFalse,
		Reason: reasonNoMeshIngressPath,
		Message: fmt.Sprintf(
			"%d service(s) are placed on a remote node this node's ingress cannot reach over the WireGuard mesh: %s",
			len(blocks), strings.Join(parts, "; "),
		),
	}
}
