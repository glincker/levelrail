package meshpath

import (
	"context"
	"fmt"
)

// EndpointResolver reports the mesh address services on a node are
// published on and dialed at, the control plane's own node included.
type EndpointResolver interface {
	Endpoint(ctx context.Context, nodeID string) (Path, error)
}

// Endpoint is Path, except the control plane's own node also resolves to
// its mesh address, since remote nodes reach services there over the mesh.
func (t *Tracker) Endpoint(ctx context.Context, nodeID string) (Path, error) {
	if nodeID != "" && nodeID != t.localID {
		return t.Path(ctx, nodeID)
	}
	if t.mesh == nil {
		return Path{Local: true, Reason: "mesh networking is off on this control plane: set APP_MESH_ENABLED=1 and restart it"}, nil
	}
	st, err := t.mesh.Status(ctx)
	if err != nil {
		return Path{}, fmt.Errorf("meshpath: read mesh status: %w", err)
	}
	if !st.Address.IsValid() || ValidateBindIP(st.Address.Addr().String()) != nil {
		return Path{Local: true, Reason: "this control plane's WireGuard device is not up: check `levelrail-cli nodes mesh`"}, nil
	}
	return Path{Local: true, Usable: true, Address: st.Address.Addr().String()}, nil
}

// LocalID is the control plane's own node ID, "" when mesh is off.
func (t *Tracker) LocalID() string { return t.localID }

var _ EndpointResolver = (*Tracker)(nil)
