package application

// WithMeshDNSFunc picks the DNS servers per target node at container create time.
// The func returns nil for a remote node or an unconfirmed listener, so containers keep Docker's default resolver.
func WithMeshDNSFunc(fn func(nodeID string) []string) Option {
	return func(ctrl *Controller) { ctrl.meshDNSFn = fn }
}

func (c *Controller) meshDNSFor(nodeID string) []string {
	if c.meshDNSFn != nil {
		return c.meshDNSFn(nodeID)
	}
	if c.meshDNSAddr != "" {
		return []string{c.meshDNSAddr}
	}
	return nil
}

// prependMeshDNS puts the mesh servers first and drops user entries that repeat one.
func prependMeshDNS(mesh, user []string) []string {
	out := append([]string(nil), mesh...)
	for _, u := range user {
		dup := false
		for _, m := range mesh {
			dup = dup || m == u
		}
		if !dup {
			out = append(out, u)
		}
	}
	return out
}
