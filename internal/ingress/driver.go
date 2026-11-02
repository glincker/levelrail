package ingress

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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
	// dns.providers.cloudflare and dns.providers.route53 are separate Go
	// modules from Caddy's own standard set (third-party DNS providers
	// always are), so each needs its own blank import to be resolvable
	// from a wildcard domain's DNS-01 automation policy (see config.go's
	// NewDNSACMEIssuer and its two DNS01Provider implementations).
	_ "github.com/caddy-dns/cloudflare"
	_ "github.com/caddy-dns/route53"

	// Registers http.handlers.waf (OWASP Coraza, module ID "waf" despite
	// the import path's own "coraza-caddy" name): opt-in per-domain WAF,
	// see routes.go's NewCorazaWAFHandler.
	_ "github.com/corazawaf/coraza-caddy/v2"

	// Registers http.handlers.rate_limit: opt-in per-domain rate
	// limiting, see routes.go's NewRateLimitHandler.
	_ "github.com/mholt/caddy-ratelimit"

	// Registers the "layer4" app and its base route/matcher machinery
	// (caddy-l4's own imports.go pulls in every one of its handler and
	// matcher modules via one bundled package; this package imports only
	// the two it actually uses, layer4 itself and l4proxy below, to keep
	// this binary's dependency footprint to what layer4.go's
	// NewLayer4ProxyHandler actually builds).
	_ "github.com/mholt/caddy-l4/layer4"
	// Registers layer4.handlers.proxy (layer4.go's NewLayer4ProxyHandler):
	// plain TCP passthrough, no TLS termination.
	_ "github.com/mholt/caddy-l4/modules/l4proxy"
)

// loadConfig is caddy.Load, replaceable in tests.
var loadConfig = caddy.Load

// validateConfig provisions payload without starting it, replaceable in tests.
var validateConfig = func(payload []byte) error {
	var c caddy.Config
	if err := json.Unmarshal(payload, &c); err != nil {
		return err
	}
	return caddy.Validate(&c)
}

// CARetryBackoff supplies the base and cap of the wait before an identical
// failed CA switch is retried; nil falls back to one minute and one hour.
type CARetryBackoff func(ctx context.Context) (base, ceiling time.Duration)

// SetCARetryBackoff makes the backoff a live runtime setting.
func (d *Driver) SetCARetryBackoff(f CARetryBackoff) {
	d.mu.Lock()
	d.backoff = f
	d.mu.Unlock()
}

func (d *Driver) backoffFor(ctx context.Context, fails int) time.Duration {
	d.mu.Lock()
	f := d.backoff
	d.mu.Unlock()
	base, ceiling := time.Minute, time.Hour
	if f != nil {
		if b, m := f(ctx); b > 0 {
			base, ceiling = b, m
		}
	}
	if ceiling < base {
		ceiling = base
	}
	delay := base
	for i := 1; i < fails && delay < ceiling; i++ {
		delay *= 2
	}
	return min(delay, ceiling)
}

var (
	caNoticeMu sync.Mutex
	caNotice   string
)

// CASwitchNotice describes a failed CA switch that is waiting to be retried,
// or "" when there is none.
func CASwitchNotice() string {
	caNoticeMu.Lock()
	defer caNoticeMu.Unlock()
	return caNotice
}

func setCASwitchNotice(s string) {
	caNoticeMu.Lock()
	caNotice = s
	caNoticeMu.Unlock()
}

// restorePrevious reloads the last working config after a failed CA switch
// left Caddy on the empty one.
func (d *Driver) restorePrevious(ctx context.Context, prev []byte) bool {
	if err := loadConfig(prev, true); err != nil {
		d.logger.ErrorContext(ctx, "ingress: restore previous config failed", slog.String("error", err.Error()))
		return false
	}
	d.mu.Lock()
	d.lastApplied = prev
	d.mu.Unlock()
	return true
}

// noteSwitchFailure schedules the next retry with exponential backoff. When
// the restore failed nothing is serving, so the next tick retries at once.
func (d *Driver) noteSwitchFailure(ctx context.Context, payload []byte, restored bool) {
	d.mu.Lock()
	if !restored {
		d.failedReset, d.failCount = nil, 0
		d.mu.Unlock()
		return
	}
	if bytes.Equal(d.failedReset, payload) {
		d.failCount++
	} else {
		d.failCount = 1
	}
	fails := d.failCount
	d.mu.Unlock()
	delay := d.backoffFor(ctx, fails)
	d.mu.Lock()
	d.failedReset = append([]byte(nil), payload...)
	d.failedAt, d.retryAfter = time.Now(), delay
	d.mu.Unlock()
	setCASwitchNotice(fmt.Sprintf("a previous CA switch failed, retrying after %s", delay.Round(time.Second)))
}

// Driver drives an in-process Caddy instance through the same code path as
// its HTTP admin API: caddy.Load calls the identical config-apply logic
// the admin API's POST /load handler calls, without this package ever
// shelling out to a `caddy` binary or running one as a subprocess or
// sibling container. The embedded-ingress design requires this shape
// specifically: ingress state lives in the control plane's own
// process, not in something else's.
type Driver struct {
	logger *slog.Logger

	mu          sync.Mutex
	listenPorts map[int]bool
	lastApplied []byte
	lastCASig   string
	failedReset []byte
	failedAt    time.Time
	retryAfter  time.Duration
	failCount   int
	lastGood    []byte
	backoff     CARetryBackoff

	snapshot            *SnapshotStore
	snapshotFingerprint string
	snapStatus          SnapshotStatus
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
// per call.
func (d *Driver) Apply(ctx context.Context, cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("ingress: apply: config is nil")
	}

	payload, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("ingress: apply: marshal config: %w", err)
	}

	d.mu.Lock()
	unchanged := d.lastApplied != nil && bytes.Equal(d.lastApplied, payload)
	d.mu.Unlock()
	if unchanged {
		return nil
	}

	sig := acmeCASignature(cfg)
	d.mu.Lock()
	switchedCA := d.lastCASig != "" && sig != "" && sig != d.lastCASig
	prev := d.lastGood
	failedReset, failedAt, retryAfter := d.failedReset, d.failedAt, d.retryAfter
	d.mu.Unlock()
	// Without a known-good config to restore, a reset could leave nothing serving.
	if switchedCA && prev != nil {
		// One reset attempt per changed payload, so a failing switch is not
		// retried on every reconcile tick.
		if failedReset != nil && bytes.Equal(failedReset, payload) && time.Since(failedAt) < retryAfter {
			wait := (retryAfter - time.Since(failedAt)).Round(time.Second)
			return fmt.Errorf("ingress: apply: a previous CA switch failed, retrying after %s", wait)
		}
		// Validate first: the reset drops every server, so a bad payload must
		// never reach it.
		if err := validateConfig(payload); err != nil {
			return fmt.Errorf("ingress: apply: validate config: %w", err)
		}
		// Caddy treats any ACME CA as the same issuer and keeps the old CA's
		// cached certificate, so the new CA never gets an order. Loading a
		// config without apps drops its in-memory cache; storage is untouched.
		if err := loadConfig(emptyAppsPayload(cfg), true); err != nil {
			return fmt.Errorf("ingress: apply: reset certificate cache: %w", err)
		}
		d.mu.Lock()
		d.lastApplied = nil
		d.mu.Unlock()
		if err := loadConfig(payload, true); err != nil {
			restored := d.restorePrevious(ctx, prev)
			d.noteSwitchFailure(ctx, payload, restored)
			return fmt.Errorf("ingress: apply: load config: %w", err)
		}
	} else if err := loadConfig(payload, true); err != nil {
		d.mu.Lock()
		d.lastApplied = nil
		d.mu.Unlock()
		return fmt.Errorf("ingress: apply: load config: %w", err)
	}

	d.mu.Lock()
	// Keep the last CA when ACME is switched off so turning it back on with
	// a different CA still resets the cache.
	if sig != "" {
		d.lastCASig = sig
	}
	d.failedReset, d.failCount = nil, 0
	d.lastGood = payload
	d.listenPorts = listenPortsOf(cfg)
	d.lastApplied = payload
	d.mu.Unlock()
	setCASwitchNotice("")
	d.saveSnapshot(payload)

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
	d.lastApplied, d.lastGood = nil, nil
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
//
// Any server without automatic_https.disable_redirects also binds
// cfg.Apps.HTTP.HTTPPort (Caddy's default 80) implicitly for its
// HTTP->HTTPS redirect, even with zero explicit Listen entries on that
// port: confirmed live, a fresh install with no domains configured still
// holds port 80 for this reason, which made doctor's port_80 check
// report a false "in use by another process" on every install.
func listenPortsOf(cfg *Config) map[int]bool {
	ports := make(map[int]bool)
	if cfg.Apps.HTTP == nil {
		return ports
	}
	redirects := false
	for _, server := range cfg.Apps.HTTP.Servers {
		for _, addr := range server.Listen {
			if port, ok := listenAddrPort(addr); ok {
				ports[port] = true
			}
		}
		auto := server.AutomaticHTTPS
		if auto == nil || (!auto.Disabled && !auto.DisableRedir) {
			redirects = true
		}
	}
	for _, p := range cfg.ownedPorts {
		if p != 0 {
			ports[p] = true
		}
	}
	if redirects {
		httpPort := cfg.Apps.HTTP.HTTPPort
		if httpPort == 0 {
			httpPort = 80
		}
		ports[httpPort] = true
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

// acmeProductionURL is the CA Caddy uses when an ACME issuer sets none.
const acmeProductionURL = "https://acme-v02.api.letsencrypt.org/directory"

// acmeCASignature is the sorted set of distinct ACME CAs across all policies,
// or "" when no ACME issuer is configured. The default CA and the explicit
// production URL count as one.
func acmeCASignature(cfg *Config) string {
	if cfg == nil || cfg.Apps.TLS == nil || cfg.Apps.TLS.Automation == nil {
		return ""
	}
	set := map[string]bool{}
	for _, p := range cfg.Apps.TLS.Automation.Policies {
		for _, iss := range p.Issuers {
			raw, err := json.Marshal(iss)
			if err != nil {
				continue
			}
			var v struct {
				Module string `json:"module"`
				CA     string `json:"ca"`
			}
			if json.Unmarshal(raw, &v) == nil && v.Module == "acme" {
				ca := v.CA
				if ca == "" {
					ca = acmeProductionURL
				}
				set[ca] = true
			}
		}
	}
	if len(set) == 0 {
		return ""
	}
	parts := make([]string, 0, len(set))
	for ca := range set {
		parts = append(parts, ca)
	}
	sort.Strings(parts)
	return "acme|" + strings.Join(parts, "|")
}

// emptyAppsPayload keeps the admin endpoint and drops every app.
func emptyAppsPayload(cfg *Config) []byte {
	b, _ := json.Marshal(&Config{Admin: cfg.Admin, Apps: Apps{}})
	return b
}
