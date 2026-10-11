package main

import (
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"sync"

	"github.com/GLINCKER/levelrail/internal/network"
)

// endpointRecorder is the part of network.Coordinator the book feeds.
type endpointRecorder interface {
	SetObservedEndpoint(nodeID, endpoint string) error
}

// meshEndpointBook records where each agent connected from, so workers get
// each other's WireGuard endpoint, not only the hub's. Agent sessions can
// open before the mesh is set up, hence the buffer.
type meshEndpointBook struct {
	mu      sync.Mutex
	pending map[string]string
	target  endpointRecorder
	skip    string
	logger  *slog.Logger
}

func newMeshEndpointBook(logger *slog.Logger) *meshEndpointBook {
	return &meshEndpointBook{pending: map[string]string{}, logger: logger}
}

// Observe is the agent.WithSessionObserver callback.
func (b *meshEndpointBook) Observe(nodeID, remoteIP string) {
	addr, err := netip.ParseAddr(remoteIP)
	if err != nil || addr.IsLoopback() || addr.IsUnspecified() {
		return
	}
	endpoint := net.JoinHostPort(addr.Unmap().String(), strconv.Itoa(network.DefaultListenPort))
	b.mu.Lock()
	defer b.mu.Unlock()
	if nodeID == b.skip {
		return
	}
	if b.target == nil {
		b.pending[nodeID] = endpoint
		return
	}
	b.record(nodeID, endpoint)
}

// Attach starts forwarding to rec and flushes anything seen before.
// localNodeID is skipped: the hub's endpoint comes from APP_AGENT_ADVERTISE_HOST.
func (b *meshEndpointBook) Attach(rec endpointRecorder, localNodeID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.target, b.skip = rec, localNodeID
	for id, ep := range b.pending {
		if id != localNodeID {
			b.record(id, ep)
		}
	}
	b.pending = map[string]string{}
}

func (b *meshEndpointBook) record(nodeID, endpoint string) {
	if err := b.target.SetObservedEndpoint(nodeID, endpoint); err != nil {
		b.logger.Warn("mesh: record observed agent endpoint failed", slog.String("node_id", nodeID), slog.String("endpoint", endpoint), slog.String("error", err.Error()))
	}
}
