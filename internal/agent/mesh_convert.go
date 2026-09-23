package agent

// This file: pure conversions between internal/network's Go types and
// internal/agent/agentpb's generated mesh proto types, the same
// field-for-field mirror convert.go already establishes for
// internal/docker. Kept in its own file rather than appended to
// convert.go: network.Key and netip.Prefix round-trip through text
// (base64, CIDR string) rather than convert.go's largely numeric/struct
// fields, different enough conversion shape to read better on its own,
// and this file is where a reviewer checking "does a private key ever
// cross this boundary" looks first, which convert.go's much longer
// docker-shape list would bury it in.

import (
	"fmt"
	"math"
	"net/netip"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/network"
)

// deviceConfigToPB converts cfg for the wire. cfg.PrivateKey is never
// read here: ApplyMeshRequest's own DeviceConfig message has no field to
// put it in (see that message's doc comment), so a caller that
// accidentally populated PrivateKey before calling this has it silently
// and structurally dropped, not merely conventionally omitted.
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

// deviceConfigFromPB converts a wire DeviceConfig back to network's own
// type. Malformed address/prefix text (a peer past a protocol version
// mismatch, or a bug on the sender) is skipped rather than failing the
// whole config: one bad peer entry must not make every other, valid peer
// in the same config unreachable, the same "one node's problem, never the
// fleet's" principle Coordinator's own NodeResult.Err doc comment states
// for a different failure mode at a different layer.
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
