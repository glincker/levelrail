package application

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
)

// ReasonNodeOffline is the Ready=False reason for an app whose node has no
// live agent connection.
const ReasonNodeOffline = "NodeOffline"

// NodeOfflineController stands in for an app's Controller on a pass where its
// node cannot be reached, so the stored status stops claiming the app is
// Ready. It touches nothing on the node: containers keep running and the next
// pass with a live agent reports the real state without recreating them.
type NodeOfflineController struct {
	serviceName string
	nodeName    string
	since       time.Time
	now         func() time.Time
}

// NewNodeOfflineController builds the stand-in. since is when the node was last
// seen, zero when unknown.
func NewNodeOfflineController(serviceName, nodeName string, since time.Time) *NodeOfflineController {
	return &NodeOfflineController{serviceName: serviceName, nodeName: nodeName, since: since, now: time.Now}
}

// Name matches the real controller so the condition replaces its status row.
func (c *NodeOfflineController) Name() string { return ControllerName(c.serviceName) }

// Reconcile implements reconcile.Controller.
func (c *NodeOfflineController) Reconcile(context.Context) (reconcile.Result, error) {
	msg := fmt.Sprintf("node %s is offline", c.nodeName)
	if !c.since.IsZero() {
		msg += fmt.Sprintf(" for %s", c.now().Sub(c.since).Round(time.Second))
	}
	msg += ": containers keep running on it, but this app cannot be managed or reached through the mesh until the agent reconnects"
	return notReady(ReasonNodeOffline, fmt.Errorf("%s", msg)), nil
}
