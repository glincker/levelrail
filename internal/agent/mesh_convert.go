package agent

// Pure conversions between internal/network and agentpb mesh types.
// Kept in its own file so the private-key boundary is easy to review.

import (
	"fmt"
	"math"
	"net/netip"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/network"
)

// PrivateKey is never read: the DeviceConfig message has no field for it, so it is structurally dropped.
func deviceConfigToPB(cfg network.DeviceConfig) *agentpb.DeviceConfig {
	out := &agentpb.DeviceConfig{
		NodeId:     cfg.NodeID,
		ListenPort: int32(cfg.ListenPort), //nolint:gosec // a UDP listen port fits int32 by construction (0-65535)
		Peers:      make([]*agentpb.PeerConfig, len(cfg.Peers)),
	}
	if cfg.Address.IsValid() {
		out.Address = cfg.Address.String()
	}
	for i, p := range cfg.Peers {
		out.Peers[i] = peerConfigToPB(p)
	}
	return out
}

// Malformed peer addresses are skipped, not fatal, so one bad peer cannot make the whole config unreachable.
func deviceConfigFromPB(cfg *agentpb.DeviceConfig) (network.DeviceConfig, error) {
	if cfg == nil {
		return network.DeviceConfig{}, fmt.Errorf("network: apply mesh: request carried no config")
	}
	out := network.DeviceConfig{
		NodeID:     cfg.GetNodeId(),
		ListenPort: int(cfg.GetListenPort()),
		Peers:      make([]network.PeerConfig, 0, len(cfg.GetPeers())),
	}
	if raw := cfg.GetAddress(); raw != "" {
		addr, err := netip.ParsePrefix(raw)
		if err != nil {
			return network.DeviceConfig{}, fmt.Errorf("network: apply mesh: parse own address %q: %w", raw, err)
		}
		out.Address = addr
	}
	for _, p := range cfg.GetPeers() {
		peer, err := peerConfigFromPB(p)
		if err != nil {
			return network.DeviceConfig{}, fmt.Errorf("network: apply mesh: peer %q: %w", p.GetNodeId(), err)
		}
		out.Peers = append(out.Peers, peer)
	}
	return out, nil
}

func peerConfigToPB(p network.PeerConfig) *agentpb.PeerConfig {
	out := &agentpb.PeerConfig{
		NodeId:                p.NodeID,
		PublicKey:             p.PublicKey.String(),
		Endpoint:              p.Endpoint,
		AllowedIps:            make([]string, len(p.AllowedIPs)),
		PersistentKeepaliveMs: p.PersistentKeepalive.Milliseconds(),
	}
	for i, prefix := range p.AllowedIPs {
		out.AllowedIps[i] = prefix.String()
	}
	return out
}

func peerConfigFromPB(p *agentpb.PeerConfig) (network.PeerConfig, error) {
	key, err := network.ParseKey(p.GetPublicKey())
	if err != nil {
		return network.PeerConfig{}, fmt.Errorf("parse public key: %w", err)
	}
	out := network.PeerConfig{
		NodeID:              p.GetNodeId(),
		PublicKey:           key,
		Endpoint:            p.GetEndpoint(),
		AllowedIPs:          make([]netip.Prefix, 0, len(p.GetAllowedIps())),
		PersistentKeepalive: time.Duration(p.GetPersistentKeepaliveMs()) * time.Millisecond,
	}
	for _, raw := range p.GetAllowedIps() {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			return network.PeerConfig{}, fmt.Errorf("parse allowed IP %q: %w", raw, err)
		}
		out.AllowedIPs = append(out.AllowedIPs, prefix)
	}
	return out, nil
}

// nodeIdentityToPB converts what a node reports back about itself after
// applying a config. An out-of-range listen port is reported as 0
// (unknown) rather than wrapped into a different, wrong port.
func nodeIdentityToPB(id network.NodeIdentity) *agentpb.NodeIdentity {
	var port int32
	if id.ListenPort > 0 && id.ListenPort <= math.MaxUint16 {
		port = int32(id.ListenPort)
	}
	return &agentpb.NodeIdentity{
		PublicKey:  id.PublicKey.String(),
		ListenPort: port,
		Endpoint:   id.Endpoint,
	}
}

func nodeIdentityFromPB(id *agentpb.NodeIdentity) (network.NodeIdentity, error) {
	key, err := network.ParseKey(id.GetPublicKey())
	if err != nil {
		return network.NodeIdentity{}, fmt.Errorf("parse public key: %w", err)
	}
	return network.NodeIdentity{
		PublicKey:  key,
		ListenPort: int(id.GetListenPort()),
		Endpoint:   id.GetEndpoint(),
	}, nil
}
