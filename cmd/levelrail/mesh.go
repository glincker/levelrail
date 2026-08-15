package main

// mesh.go wires TASKS.md 3.4's WireGuard mesh into the control plane,
// gated behind APP_MESH_ENABLED (default off).
//
// It is opt-in rather than always-on for a concrete reason, not general
// caution: internal/network.ConfigSink's gRPC arm, the piece that would
// carry a remote node's config over the agent wire contract, is
// deliberately not built yet (see that interface's own doc comment for
// the exact messages it needs). Until it exists, enabling this only
// brings up *this* node's own device, DNS zone, and self-peer entry;
// a remote, enrolled node never receives a config to apply, so it is
// real but not yet a working multi-node mesh. That is exactly the
// "feature flags for anything half-built" case this project's own
// process rules call for, not a permanent switch: once the wire
// contract extension lands, this flag's default can flip.
//
// A node running the control plane needs a real row in the nodes table
// for any of this to work at all: internal/reconcile/mesh.Controller
// reads its whole inventory from Store.ListNodes, and
// internal/network.Plan errors with ErrUnknownNode for a selfID absent
// from that inventory. There is nothing elsewhere in this codebase that
// creates such a row today (desired_services.node_id's own "" sentinel
// means "local" without ever needing a nodes row to back it), so
// bootstrapLocalNode below is new, not a reuse of an existing path.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	meshKeyFilename      = "mesh-node.key"
	localNodeIDFilename  = "mesh-node.id"
	defaultMeshDNSAddr   = ":5390"
	defaultLocalNodeName = "local"
)

// meshEnabled reads APP_MESH_ENABLED, the same "unset or anything but 1
// means off" shape devmode_debug.go's own APP_DEV_MODE check uses.
func meshEnabled() bool {
	return os.Getenv("APP_MESH_ENABLED") == "1"
}

// meshDNSAddr reads APP_MESH_DNS_ADDR, defaulting to a high, unprivileged
// port: :53 would need root on most systems, and this server is not yet
// what containers actually query (see this file's own top-of-file note
// on what "opt-in" currently gets you), so defaulting to something a
// developer can bind without sudo matters more than matching the
// well-known DNS port.
func meshDNSAddr() string {
	if v := os.Getenv("APP_MESH_DNS_ADDR"); v != "" {
		return v
	}
	return defaultMeshDNSAddr
}

// meshCIDR reads APP_MESH_CIDR, or leaves the zero value for
// network.NewCoordinator to fill in with network.DefaultMeshCIDR: an
// operator with no opinion about addressing should get a working mesh,
// not an error about a field they never set, the same reasoning
// NewCoordinator's own doc comment gives for accepting a zero PlanOptions
// in the first place. An invalid override is treated as absent rather
// than fatal: this control plane must still start.
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

// loadOrGenerateMeshKey loads this node's persisted WireGuard private key
// from dataDir, or generates and persists a fresh one if none exists yet,
// the identical read-or-generate-then-persist shape
// loadOrGenerateAgentCA uses for the agent CA. Regenerating on every
// restart instead of persisting would change this node's mesh identity
// (and therefore its address and every peer's record of it) on every
// reboot for no reason.
//
// Stored as network.Key.String()'s base64 form, the same form the
// control plane already stores and ships elsewhere (Key.String's own doc
// comment), not the UAPI hex form.
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
	if err := os.WriteFile(path, []byte(key.String()), 0o600); err != nil { //nolint:gosec // operator-controlled data directory path, not user input
		return network.Key{}, fmt.Errorf("persist mesh key: %w", err)
	}
	return key, nil
}

// loadOrGenerateLocalNodeID loads this node's persisted node ID from
// dataDir, or generates and persists a fresh one, same shape as
// loadOrGenerateMeshKey immediately above and for the same reason: this
// ID is this node's primary key in the nodes table and the DNS zone's
// ".node." label, both of which must survive a restart unchanged.
//
// A dedicated generator rather than reusing internal/agent's own
// (unexported) node-ID generator, the same "long-lived and
// externally-visible deserves its own generator" reasoning that
// function's own doc comment gives for not sharing with a request ID.
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

// bootstrapLocalNode ensures id has a row in the nodes table, the same
// idempotent "GetNode, SaveNode only if missing" shape bootstrapAdmin
// uses for the admin account: safe to call on every start, a no-op once
// the row exists.
//
// The name prefers the machine's hostname, for the same reason an
// operator-facing node list should say something recognizable rather
// than an opaque ID; a hostname collision with an already-enrolled
// remote node (store.ErrNodeNameTaken) falls back to the ID itself,
// which is always unique by construction, rather than failing startup
// over a cosmetic conflict.
//
// LastSeenAt is set to now, not left nil. A remote node's Status only
// ever reaches Online through Server.Session, which always touches
// LastSeenAt first, so a nil LastSeenAt on an Online node is exactly the
// "not expected in practice" case lastSeenAge's own doc comment
// (internal/reconcile/nodehealth) says it treats as stale: without this,
// the local node reads Offline from the moment it is created until
// localNodeHeartbeat's first tick, which is a real, user-visible
// regression for a control plane that was never actually down.
func bootstrapLocalNode(ctx context.Context, db *store.DB, id string) error {
	_, err := db.GetNode(ctx, id)
	if err == nil {
		return nil
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

// localNodeHeartbeat touches id's last_seen_at on interval until ctx is
// canceled, the local-node equivalent of Server.heartbeatLoop for a
// remote agent's session. Without this, internal/reconcile/nodehealth
// (nothing else touches last_seen_at for a node with no gRPC session)
// flips the local node Offline heartbeatTimeout after boot and it never
// recovers, since nothing else about this node's health is transient the
// way a real connectivity problem is.
func localNodeHeartbeat(ctx context.Context, db *store.DB, id string, interval time.Duration, logger *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := db.TouchNodeLastSeen(ctx, id); err != nil {
				logger.Error("local node heartbeat failed", slog.String("error", err.Error()))
			}
		}
	}
}

// meshSetup is what setupMesh hands back to run: the pieces
// dynamicSource needs to add a mesh controller to every reconcile pass,
// plus a close func that tears down the device and DNS server on
// shutdown. A nil *meshSetup (setupMesh's error-free "disabled" return)
// is dynamicSource's signal to build no mesh controller at all, the same
// nil-means-absent shape secretsManager already uses for
// application.WithSecretResolver.
type meshSetup struct {
	localNodeID string
	coordinator *network.Coordinator
	resolver    *network.Resolver
	// dnsAddr is the mesh DNS server's actual bound address (host, not
	// container-reachable, port only meaningful once combined with a
	// resolved bridge gateway, see containerDNSAddr): DNSServer.Addr's
	// own doc comment on why a caller needs the port it actually bound
	// rather than assuming it matches the configured addr verbatim.
	dnsAddr netip.AddrPort
	close   func()
}

// setupMesh brings up this node's mesh device, DNS server, and
// coordinator, and bootstraps its own nodes row, all gated by
// meshEnabled. Returns (nil, nil) when disabled, not an error: an
// operator who has not opted in should see nothing different at all, not
// a log line about a feature they did not ask for.
//
// The device itself never fails startup even when this process cannot
// actually mesh: network.NewDevice's own doc comment establishes that an
// unprivileged process gets a *network.Disabled, not an error, "a node
// that cannot mesh must still run." Every other step here can fail the
// whole control plane, though, because they are this function's own
// responsibility, not the mesh library's: a bad mesh key file, a taken
// DNS port, or a broken local-node bootstrap are configuration problems
// worth failing loudly on rather than starting in a half-wired state.
func setupMesh(ctx context.Context, db *store.DB, b *brand.Brand, dataDir string, logger *slog.Logger) (*meshSetup, error) {
	if !meshEnabled() {
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
		network.WithLogger(logger), network.WithShortName(b.ShortName))
	if err != nil {
		return nil, fmt.Errorf("bring up mesh device: %w", err)
	}

	sink, err := network.NewLocalSink(localNodeID, device, key)
	if err != nil {
		_ = device.Close()
		return nil, fmt.Errorf("build local mesh sink: %w", err)
	}
	coordinator := network.NewCoordinator(sink, network.PlanOptions{MeshCIDR: meshCIDR(logger)},
		network.WithCoordinatorLogger(logger))

	// Lowercased explicitly: Resolver.Authoritative compares a normalized
	// (lowercased) query name against this zone verbatim, so a
	// brand.yaml short_name with any uppercase (the common case, brand
	// names are typically title-cased) would never match a single real
	// query otherwise.
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

// dockerNameserverPort is the only port a container's resolver ever
// actually queries. Docker's DNS HostConfig field (--dns) and every
// container's resulting /etc/resolv.conf "nameserver" line both carry a
// bare IP, never IP:port: confirmed against a real daemon while building
// this function, ContainerCreate accepts an "ip:port" string without
// complaint (it isn't validated at create time), but the *start* call
// that follows rejects it outright ("bad nameserver address ...:5390:
// ParseAddr(...): unexpected character (at \":5390\")"), which would fail
// every single container this control plane creates the moment mesh is
// enabled with the default (non-53) DNS port. There is no resolv.conf
// syntax or Docker flag that requests a non-standard nameserver port
// either, so a mesh DNS server bound to anything but 53 cannot be wired
// into a container's resolver at all, full stop; see containerDNSAddr's
// own doc comment for what that means in practice.
const dockerNameserverPort = 53

// containerDNSAddr computes the nameserver IP the reconcile controllers
// should point every container they create at, so it resolves this
// node's mesh DNS zone (internal/network/dns_server.go), or "" if that
// isn't possible right now.
//
// No WireGuard mesh interface IP is bound to anything yet
// (internal/network/device.go's configureLink no-ops without a
// LinkConfigurator, a separate, out-of-scope gap), and docker.Client.Create
// attaches every container to Docker's default "bridge" network with no
// NetworkingConfig, so the mesh DNS server's own wildcard bind address is
// never itself reachable from inside a container. The address that is
// reachable is that bridge network's own gateway IP, queried live from
// the Docker daemon rather than hardcoded (dockerd's bip config can
// change it).
//
// That gateway IP is only useful, though, if the mesh DNS server is
// actually listening on port 53 at it: dockerNameserverPort's own doc
// comment is the live-verified reason there is no way to express "query
// port 5390 instead" anywhere a container's resolver looks. APP_MESH_DNS_ADDR
// defaults to :5390 specifically so an unprivileged process can bind it
// (meshDNSAddr's own doc comment), which means in the common,
// zero-configuration case this function's real job is refusing to wire
// anything up, correctly, rather than handing every container a
// nameserver entry that fails to start it. An operator who wants this
// feature live has to set APP_MESH_DNS_ADDR to bind :53 (and grant the
// process permission to do so), a real operational tradeoff this
// function does not paper over.
//
// Every other failure (no mesh running, no bound DNS address yet, the
// bridge gateway lookup itself failing) logs a warning and returns "",
// never fails the caller: every controller this feeds
// (application.WithMeshDNSAddr, database.WithMeshDNSAddr,
// nginxdemo.WithMeshDNSAddr) already treats an empty address as "no DNS
// override at all," so the mesh-disabled and lookup-failed paths both
// stay byte-identical to this code's behavior before the mesh DNS field
// existed: containers fall back to Docker's own embedded resolver.
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

	gateway, err := client.BridgeGatewayIP(ctx)
	if err != nil {
		logger.Warn("mesh dns: could not resolve a container-reachable address, containers will use Docker's own resolver",
			slog.String("error", err.Error()))
		return ""
	}

	return gateway
}
