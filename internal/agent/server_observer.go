package agent

import (
	"context"
	"net"

	"google.golang.org/grpc/peer"
)

// WithSessionObserver calls f with each authenticated node's remote IP as
// its session opens, so the mesh can record where to reach that node.
func WithSessionObserver(f func(nodeID, remoteIP string)) Option {
	return func(s *Server) { s.onSession = f }
}

func (s *Server) observeSession(ctx context.Context, nodeID string) {
	if s.onSession == nil {
		return
	}
	if ip := peerIP(ctx); ip != "" {
		s.onSession(nodeID, ip)
	}
}

func peerIP(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok || p.Addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(p.Addr.String())
	if err != nil {
		return ""
	}
	return host
}
