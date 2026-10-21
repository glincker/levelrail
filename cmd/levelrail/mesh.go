package main

// Opt-in via APP_MESH_ENABLED so a default control plane sees no new TUN device or DNS server.
// bootstrapLocalNode must create this node's row, since the mesh planner errors on an unknown self ID.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/profile"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	meshKeyFilename      = "mesh-node.key"
	localNodeIDFilename  = "mesh-node.id"
	defaultMeshDNSAddr   = ":5390"
	defaultLocalNodeName = "local"
)

// meshEnabled resolves whether the mesh should start: APP_MESH_ENABLED pins
// it, else the setting saved from the dashboard or CLI, else off.
func meshEnabled(ctx context.Context, db *store.DB, logger *slog.Logger) bool {
	var saved store.MeshSetting
	if db != nil {
		s, err := db.GetMeshSetting(ctx)
		if err != nil {
			logger.Warn("mesh setting unreadable, treating as off", slog.String("error", err.Error()))
		}
		saved = s
	}
	on, _ := network.ResolveEnabledWithProfile(os.LookupEnv, saved.Enabled, saved.Set, profile.For(profile.Active()).Mesh)
	return on
}

// Defaults to an unprivileged port so the process can bind without root.
func meshDNSAddr() string {
	if v := os.Getenv("APP_MESH_DNS_ADDR"); v != "" {
		return v
	}
	return defaultMeshDNSAddr
}

// An invalid override is treated as absent so the control plane still starts.
func meshCIDR(logger *slog.Logger) netip.Prefix {
	raw := os.Getenv("APP_MESH_CIDR")
	if raw == "" {
		return netip.Prefix{}
	}
	p, err := netip.ParsePrefix(raw)
	if err != nil {
		logger.Warn("ignoring invalid APP_MESH_CIDR", slog.String("value", raw), slog.String("error", err.Error()))
		return netip.Prefix{}
	}
	return p
}

// The key is persisted because regenerating on restart would change this node's mesh identity.
func loadOrGenerateMeshKey(dataDir string) (network.Key, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil { //nolint:gosec // operator-controlled startup config, not user input
		return network.Key{}, err
	}
	path := filepath.Join(dataDir, meshKeyFilename)

	raw, err := os.ReadFile(path) //nolint:gosec // operator-controlled data directory path, not user input
	if err == nil {
		key, parseErr := network.ParseKey(string(raw))
		if parseErr != nil {
			return network.Key{}, fmt.Errorf("parse persisted mesh key: %w", parseErr)
		}
		return key, nil
	}

	key, err := network.GeneratePrivateKey()
	if err != nil {
		return network.Key{}, fmt.Errorf("generate mesh key: %w", err)
	}
	if err := persistMeshKey(dataDir, key); err != nil {
		return network.Key{}, err
	}
	return key, nil
}

// Shared by first-run generation and key rotation so both write the key the same way.
func persistMeshKey(dataDir string, key network.Key) error {
	path := filepath.Join(dataDir, meshKeyFilename)
	if err := os.WriteFile(path, []byte(key.String()), 0o600); err != nil { //nolint:gosec // operator-controlled data directory path, not user input
		return fmt.Errorf("persist mesh key: %w", err)
	}
	return nil
}

// The ID is the nodes table key and DNS label, so it must survive restarts unchanged.
func loadOrGenerateLocalNodeID(dataDir string) (string, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil { //nolint:gosec // operator-controlled startup config, not user input
		return "", err
	}
	path := filepath.Join(dataDir, localNodeIDFilename)

	raw, err := os.ReadFile(path) //nolint:gosec // operator-controlled data directory path, not user input
	if err == nil && len(raw) > 0 {
		return string(raw), nil
	}

	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate local node id: %w", err)
	}
	id := "local_" + hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(id), 0o600); err != nil { //nolint:gosec // operator-controlled data directory path, not user input
		return "", fmt.Errorf("persist local node id: %w", err)
	}
	return id, nil
}

// readLocalNodeID returns the persisted local node ID, or "" before one exists.
func readLocalNodeID(dataDir string) string {
	raw, err := os.ReadFile(filepath.Join(dataDir, localNodeIDFilename)) //nolint:gosec // operator-controlled data directory path, not user input
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// LastSeenAt is set now, or the local node reads Offline until its first heartbeat.
func bootstrapLocalNode(ctx context.Context, db *store.DB, id string) error {
	_, err := db.GetNode(ctx, id)
	if err == nil {
		return recoverLocalNode(ctx, db, id)
	}
	if !errors.Is(err, store.ErrNodeNotFound) {
		return fmt.Errorf("look up local node: %w", err)
	}

	name := defaultLocalNodeName
	if hostname, hostErr := os.Hostname(); hostErr == nil && hostname != "" {
		name = hostname
	}

	now := time.Now()
	n := store.Node{
		ID:         id,
		Name:       name,
		Status:     store.NodeStatusOnline,
		LastSeenAt: &now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := db.SaveNode(ctx, n); err != nil {
		if errors.Is(err, store.ErrNodeNameTaken) {
			n.Name = id
			return db.SaveNode(ctx, n)
		}
		return fmt.Errorf("save local node: %w", err)
	}
	return nil
}

// recoverLocalNode refreshes an existing local node on startup so a restart
// that outlasted the heartbeat timeout does not leave it Offline.
func recoverLocalNode(ctx context.Context, db *store.DB, id string) error {
	if err := db.TouchNodeLastSeen(ctx, id); err != nil {
		return fmt.Errorf("refresh local node heartbeat: %w", err)
	}
	return markLocalNodeOnline(ctx, db, id)
}

// markLocalNodeOnline sets the local node Online when it is not. The node-health
// controller only ever flips Online to Offline, and the local node has no agent
// session that would flip it back, so a fresh heartbeat must restore it here.
func markLocalNodeOnline(ctx context.Context, db *store.DB, id string) error {
	n, err := db.GetNode(ctx, id)
	if err != nil {
		return fmt.Errorf("look up local node: %w", err)
	}
	if n.Status == store.NodeStatusOnline {
		return nil
	}
	if err := db.UpdateNodeStatus(ctx, id, store.NodeStatusOnline); err != nil {
		return fmt.Errorf("mark local node online: %w", err)
	}
	return nil
}

// Without this, nodehealth marks the local node Offline after boot and it never recovers.
func localNodeHeartbeat(ctx context.Context, db *store.DB, id string, interval time.Duration, logger *slog.Logger) {
	loop := footprint.Track("local node heartbeat", interval)
	defer loop.Active()()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			start := time.Now()
			if err := db.TouchNodeLastSeen(ctx, id); err != nil {
				loop.Observe(start, err)
				logger.Error("local node heartbeat failed", slog.String("error", err.Error()))
				continue
			}
			err := markLocalNodeOnline(ctx, db, id)
			loop.Observe(start, err)
			if err != nil {
				logger.Error("local node status recovery failed", slog.String("node_id", id), slog.String("error", err.Error()))
			}
		}
	}
}

// A nil *meshSetup means mesh is disabled and no mesh controller is built.
type meshSetup struct {
	localNodeID string
	coordinator *network.Coordinator
	resolver    *network.Resolver
	// Never nil when meshSetup is non-nil: NewDevice always returns a usable Mesh.
	device network.Mesh
	// Use the actually bound address, which can differ from the configured one.
	dnsAddr netip.AddrPort
	close   func()
}

// Returns (nil, nil) when disabled, so an operator who did not opt in sees nothing new.
// Device failures never fail startup, but config errors (bad key, taken DNS port) do.
func setupMesh(ctx context.Context, db *store.DB, b *brand.Brand, dataDir string, agentRegistry *agent.Registry, logger *slog.Logger) (*meshSetup, error) {
	if !meshEnabled(ctx, db, logger) {
		return nil, nil
	}

	key, err := loadOrGenerateMeshKey(dataDir)
	if err != nil {
		return nil, fmt.Errorf("mesh key: %w", err)
	}
	localNodeID, err := loadOrGenerateLocalNodeID(dataDir)
	if err != nil {
		return nil, fmt.Errorf("local node id: %w", err)
	}
	if err := bootstrapLocalNode(ctx, db, localNodeID); err != nil {
		return nil, fmt.Errorf("bootstrap local node: %w", err)
	}

	device, err := network.NewDevice(ctx, network.SystemProbe{},
		network.WithLogger(logger), network.WithShortName(b.ShortName),
		network.WithLinkConfigurator(network.SystemLinkConfigurator{}))
	if err != nil {
		return nil, fmt.Errorf("bring up mesh device: %w", err)
	}

	localSink, err := network.NewLocalSink(localNodeID, device, key,
		network.WithKeyPersistFunc(func(newKey network.Key) error {
			return persistMeshKey(dataDir, newKey)
		}))
	if err != nil {
		_ = device.Close()
		return nil, fmt.Errorf("build local mesh sink: %w", err)
	}
	// GRPCSink serves remote nodes and LocalSink serves this node, combined via MultiSink.
	sink := network.NewMultiSink(localNodeID, localSink, agent.NewGRPCSink(agentRegistry))
	coordinator := network.NewCoordinator(sink, network.PlanOptions{MeshCIDR: meshCIDR(logger)},
		network.WithCoordinatorLogger(logger))
	if hub := meshHubEndpoint(advertiseHostFromStore(ctx, db), network.DefaultListenPort); hub != "" {
		if err := coordinator.SetObservedEndpoint(localNodeID, hub); err != nil {
			logger.Warn("mesh hub endpoint not recorded", slog.String("endpoint", hub), slog.String("error", err.Error()))
		}
	} else {
		logger.Warn("mesh hub endpoint unknown: set APP_AGENT_ADVERTISE_HOST to this server's public host so agents can reach it on UDP " + strconv.Itoa(network.DefaultListenPort))
	}

	// Lowercased because Resolver.Authoritative compares lowercased query names verbatim.
	zone := strings.ToLower(b.ShortName)
	if zone == "" {
		zone = defaultLocalNodeName
	}
	resolver := network.NewResolver(zone)
	dnsServer := network.NewDNSServer(resolver, logger)
	dnsAddr := meshDNSAddr()
	if err := dnsServer.Start(ctx, dnsAddr); err != nil {
		_ = device.Close()
		return nil, fmt.Errorf("start mesh dns server on %q: %w", dnsAddr, err)
	}

	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	go localNodeHeartbeat(heartbeatCtx, db, localNodeID, nodeHeartbeatInterval(), logger)

	logger.Info("mesh networking enabled",
		slog.String("local_node_id", localNodeID),
		slog.String("dns_addr", dnsAddr))

	return &meshSetup{
		localNodeID: localNodeID,
		coordinator: coordinator,
		resolver:    resolver,
		device:      device,
		dnsAddr:     dnsServer.Addr(),
		close: func() {
			cancelHeartbeat()
			if err := dnsServer.Close(); err != nil {
				logger.Error("closing mesh dns server", slog.String("error", err.Error()))
			}
			if err := device.Close(); err != nil {
				logger.Error("closing mesh device", slog.String("error", err.Error()))
			}
		},
	}, nil
}

// Docker rejects an ip:port nameserver at container start, so containers can only use port 53.
const dockerNameserverPort = 53

// Returns "" when no usable address exists; controllers treat "" as no DNS override.
// The default high DNS port therefore means no override, since containers cannot reach it.
func containerDNSAddr(ctx context.Context, client *docker.Client, meshCfg *meshSetup, logger *slog.Logger) string {
	if meshCfg == nil || !meshCfg.dnsAddr.IsValid() {
		return ""
	}
	if meshCfg.dnsAddr.Port() != dockerNameserverPort {
		logger.Warn("mesh dns: not bound to port 53, so no container can be pointed at it "+
			"(neither Docker's DNS config nor a container's own resolver supports a non-standard nameserver port); "+
			"containers will use Docker's own resolver until APP_MESH_DNS_ADDR is set to bind :53",
			slog.Int("bound_port", int(meshCfg.dnsAddr.Port())))
		return ""
	}

	bctx, bcancel := context.WithTimeout(ctx, bootDockerCallTimeout)
	gateway, err := client.BridgeGatewayIP(bctx)
	bcancel()
	if err != nil {
		logger.Warn("mesh dns: could not resolve a container-reachable address, containers will use Docker's own resolver",
			slog.String("error", err.Error()))
		return ""
	}

	return gateway
}

// meshHubEndpoint is the control plane's own WireGuard endpoint as agents
// must dial it: the advertise host plus the mesh UDP port. Empty for a
// loopback or blank host, which no remote agent can reach.
func meshHubEndpoint(host string, port int) string {
	if host == "" {
		return ""
	}
	if ip, err := netip.ParseAddr(host); err == nil && ip.IsLoopback() {
		return ""
	}
	if host == "localhost" {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}
