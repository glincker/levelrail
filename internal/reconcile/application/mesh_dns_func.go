package application

// WithMeshDNSFunc picks the DNS server per target node at container create time.
// The func returns "" for a remote node or an unconfirmed listener, so containers keep Docker's default resolver.
func WithMeshDNSFunc(fn func(nodeID string) string) Option {
	return func(ctrl *Controller) { ctrl.meshDNSFn = fn }
}

func (c *Controller) meshDNSFor(nodeID string) string {
	if c.meshDNSFn != nil {
		return c.meshDNSFn(nodeID)
	}
	return c.meshDNSAddr
}
