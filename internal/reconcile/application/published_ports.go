package application

import (
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ReasonPublishedPortsRemoteNode marks a service whose direct host ports were
// refused because it is placed on a remote node.
const ReasonPublishedPortsRemoteNode = "PublishedPortsRemoteNode"

// publishedPortsRemoteError fails closed: node firewall and exposure rules
// are only managed on the control plane's own node today.
type publishedPortsRemoteError struct {
	port   store.PublishedPort
	nodeID string
}

func (e *publishedPortsRemoteError) Error() string {
	return fmt.Sprintf("host port %s can only be published on the control plane's own node, this service is placed on node %q; move it to the control plane node or remove the port", e.port, e.nodeID)
}

func (c *Controller) refuseRemotePublishedPorts(desired *store.DesiredService) error {
	if len(desired.PublishedPorts) == 0 {
		return nil
	}
	local := ""
	if c.meshDB != nil {
		local = c.meshDB.LocalNodeID
	}
	if SameNode(desired.NodeID, "", local) {
		return nil
	}
	return &publishedPortsRemoteError{port: desired.PublishedPorts[0], nodeID: desired.NodeID}
}
