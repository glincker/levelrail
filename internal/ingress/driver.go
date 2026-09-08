package ingress

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"

	"github.com/caddyserver/caddy/v2"

	// Blank-imported for registration side effects only: this pulls in
	// Caddy's standard module set (reverse_proxy, the tls app's internal
	// and ACME issuers, the pki app the internal issuer needs for its
	// local CA, file storage, logging, metrics) exactly the way Caddy's
	// own cmd/caddy binary does. Without this import, caddy.Load fails
	// at Provision time with "unrecognized module" for any handler or
	// issuer name in the JSON config, because Caddy resolves module
	// names from Go's init()-time module registry, not from the config
	// itself.
	_ "github.com/caddyserver/caddy/v2/modules/standard"

	// Blank-imported for the same registration-side-effect reason:
	// dns.providers.cloudflare is a separate Go module from Caddy's own
	// standard set (third-party DNS providers always are), so it needs
	// its own blank import to be resolvable from a wildcard domain's
	// DNS-01 automation policy (see config.go's
	// NewCloudflareDNSACMEIssuer).
	_ "github.com/caddy-dns/cloudflare"
)

// Driver drives an in-process Caddy instance through the same code path as
// its HTTP admin API (caddy.Load calls the identical config-apply logic
// the admin API's POST /load handler calls, see
// docs-local/research/caddy-spike.md), without this package ever shelling
// out to a `caddy` binary or running one as a subprocess or sibling
// container. The embedded-ingress design requires this shape
// specifically: ingress state lives in the control plane's own
// process, not in something else's.
type Driver struct {
	logger *slog.Logger

	mu          sync.Mutex
	listenPorts map[int]bool
}

// New builds a Driver. A nil logger falls back to slog.Default(), matching
// the convention in internal/reconcile.Engine.
func New(logger *slog.Logger) *Driver {
	if logger == nil {
		logger = slog.Default()
	}
	return &Driver{logger: logger}
}

// Apply marshals cfg to Caddy's JSON config format and loads it into the
// running (package-global) Caddy instance, starting it on first call.
// Caddy's config model has no notion of "the config for this Driver"
// versus "the config for some other caller in the process": caddy.Load
// replaces the entire process-wide config, which is a real constraint
// Phase 1 needs to design around once ingress config is built
// incrementally from many apps rather than handed over as one document
// per call (see docs-local/research/caddy-spike.md).
func (d *Driver) Apply(ctx context.Context, cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("ingress: apply: config is nil")
	}

	payload, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("ingress: apply: marshal config: %w", err)
	}

	if err := caddy.Load(payload, true); err != nil {
		return fmt.Errorf("ingress: apply: load config: %w", err)
	}

	d.mu.Lock()
	d.listenPorts = listenPortsOf(cfg)
	d.mu.Unlock()

	d.logger.InfoContext(ctx, "ingress config applied",
		slog.Int("config_bytes", len(payload)),
	)
	return nil
}

// Stop tears down the running Caddy instance, releasing every listener it
// holds. Safe to call even if Apply was never called.
func (d *Driver) Stop(ctx context.Context) error {
	if err := caddy.Stop(); err != nil {
		return fmt.Errorf("ingress: stop: %w", err)
	}
	d.mu.Lock()
	d.listenPorts = nil
	d.mu.Unlock()
	d.logger.InfoContext(ctx, "ingress stopped")
	return nil
}

// OwnsPort reports whether the most recently applied config bound an HTTP
// server to port. Used by GET /api/v1/system/doctor (internal/api) to tell
// this control plane's own ingress apart from an unrelated process holding
// the same port.
func (d *Driver) OwnsPort(port int) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.listenPorts[port]
}

// listenPortsOf collects every port cfg's HTTP servers are configured to
// listen on. Non-numeric or unparseable Listen entries are skipped rather
// than erroring: this is a best-effort signal for a doctor check, not a
// config validator.
func listenPortsOf(cfg *Config) map[int]bool {
	ports := make(map[int]bool)
	if cfg.Apps.HTTP == nil {
		return ports
	}
	for _, server := range cfg.Apps.HTTP.Servers {
		for _, addr := range server.Listen {
			if port, ok := listenAddrPort(addr); ok {
				ports[port] = true
			}
		}
	}
	return ports
}

// listenAddrPort extracts the numeric port from a Caddy listen address
// (e.g. ":443", "0.0.0.0:8080").
func listenAddrPort(addr string) (int, bool) {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, false
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, false
	}
	return port, true
}
