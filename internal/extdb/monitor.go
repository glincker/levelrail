package extdb

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envProbeInterval      = "APP_EXTERNAL_DB_PROBE_INTERVAL"
	defaultProbeInterval  = 5 * time.Minute
	healthWriteTimeout    = 15 * time.Second
	attentionKindExternal = "external_database"
)

// PasswordSource reads a record's stored password.
type PasswordSource interface {
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// ConnFromRecord builds the in-memory connection for a stored record. A
// record saved without a password resolves with an empty one.
func ConnFromRecord(ctx context.Context, rec store.ExternalDatabase, secrets PasswordSource) (Conn, error) {
	c := Conn{
		Engine: rec.Engine, Host: rec.Host, Port: rec.Port, User: rec.Username, Database: rec.DatabaseName,
		TLSMode: rec.TLSMode, Network: rec.Network, SourceContainer: rec.SourceContainer,
	}
	if rec.Engine == EngineMongoDB {
		c.AuthDatabase = defaultAuthDB
	}
	if secrets == nil {
		return c, nil
	}
	svc := store.ExternalDatabaseSecretsKey(rec.Name)
	exists, err := secrets.Exists(ctx, svc, store.ExternalDatabasePasswordKey)
	if err != nil {
		return Conn{}, fmt.Errorf("extdb: check password for %q: %w", rec.Name, err)
	}
	if !exists {
		return c, nil
	}
	if c.Password, err = secrets.Resolve(ctx, svc, store.ExternalDatabasePasswordKey); err != nil {
		return Conn{}, fmt.Errorf("extdb: resolve password for %q: %w", rec.Name, err)
	}
	return c, nil
}

// HealthStore is what the monitor needs from the store.
type HealthStore interface {
	ListExternalDatabases(ctx context.Context) ([]store.ExternalDatabase, error)
	SetExternalDatabaseHealth(ctx context.Context, name, status, reason string, latencyMs int, checkedAt time.Time) error
	UpdateExternalDatabase(ctx context.Context, d store.ExternalDatabase) error
}

// Monitor probes every external database on an interval, one at a time, each
// through a short-lived helper container.
type Monitor struct {
	Store   HealthStore
	Secrets PasswordSource
	Runtime func(nodeID string) (docker.Runtime, error)
	Logger  *slog.Logger
}

// IntervalFromEnv returns the probe interval, zero meaning disabled.
func IntervalFromEnv() time.Duration {
	raw, ok := os.LookupEnv(envProbeInterval)
	if !ok || raw == "" {
		return defaultProbeInterval
	}
	if raw == "0" {
		return 0
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	return defaultProbeInterval
}

// Run blocks until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.Sweep(ctx)
		}
	}
}

// Sweep probes every record once.
func (m *Monitor) Sweep(ctx context.Context) {
	recs, err := m.Store.ListExternalDatabases(ctx)
	if err != nil {
		m.Logger.Error("extdb: list external databases failed", slog.String("error", err.Error()))
		return
	}
	for _, rec := range recs {
		if ctx.Err() != nil {
			return
		}
		m.CheckOne(ctx, rec)
	}
}

// CheckOne probes rec and records the outcome.
func (m *Monitor) CheckOne(ctx context.Context, rec store.ExternalDatabase) Result {
	log := m.Logger.With(slog.String("database", rec.Name), slog.String("node_id", rec.NodeID))
	rt, err := m.Runtime(rec.NodeID)
	if err != nil {
		log.Warn("extdb: resolve node runtime failed", slog.String("error", err.Error()))
		return m.record(ctx, rec.Name, Result{Status: StatusUnknown, Reason: "the node is not currently reachable"})
	}
	rec = m.refresh(ctx, rt, rec)
	conn, err := ConnFromRecord(ctx, rec, m.Secrets)
	if err != nil {
		log.Warn("extdb: load connection failed", slog.String("error", err.Error()))
		return m.record(ctx, rec.Name, Result{Status: StatusUnknown, Reason: "the stored password could not be read"})
	}
	res := (&Prober{Runtime: rt, Logger: m.Logger}).Probe(ctx, rec.Name, conn)
	log.Info("extdb: probe finished", slog.String("status", res.Status), slog.Int("latency_ms", res.LatencyMs))
	return m.record(ctx, rec.Name, res)
}

func (m *Monitor) record(ctx context.Context, name string, res Result) Result {
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), healthWriteTimeout)
	defer cancel()
	if err := m.Store.SetExternalDatabaseHealth(wctx, name, res.Status, res.Reason, res.LatencyMs, time.Now()); err != nil {
		m.Logger.Warn("extdb: record health failed", slog.String("database", name), slog.String("error", err.Error()))
	}
	return res
}

// AttentionItem is a record that needs an operator, in a shape the attention
// center can adopt without this package importing it.
type AttentionItem struct {
	Severity string `json:"severity"`
	Kind     string `json:"kind"`
	Subject  string `json:"subject"`
	Detail   string `json:"detail"`
}

// AttentionItems lists records whose last probe was not healthy.
func AttentionItems(recs []store.ExternalDatabase) []AttentionItem {
	var out []AttentionItem
	for _, r := range recs {
		var sev string
		switch r.HealthStatus {
		case StatusAuthFailed, StatusTLSError, StatusUnreachable:
			sev = "critical"
		case StatusSlow:
			sev = "warning"
		default:
			continue
		}
		out = append(out, AttentionItem{Severity: sev, Kind: attentionKindExternal, Subject: r.Name, Detail: r.HealthStatus + ": " + r.HealthReason})
	}
	return out
}

// refresh re-resolves an adopted container's network and address and stores
// them when they moved, so the record follows the container across recreates.
func (m *Monitor) refresh(ctx context.Context, rt docker.Runtime, rec store.ExternalDatabase) store.ExternalDatabase {
	next, changed := Refresh(ctx, rt, rec)
	if !changed {
		return rec
	}
	if err := m.Store.UpdateExternalDatabase(ctx, next); err != nil {
		m.Logger.Warn("extdb: store refreshed address failed", slog.String("database", rec.Name), slog.String("error", err.Error()))
		return rec
	}
	m.Logger.Info("extdb: adopted container moved, record updated", slog.String("database", rec.Name), slog.String("host", next.Host), slog.String("network", next.Network))
	return next
}
