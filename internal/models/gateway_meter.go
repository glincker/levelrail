package models

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Environment variables that tune usage metering.
const (
	envUsageFlushInterval = "APP_MODEL_USAGE_FLUSH_INTERVAL"
	envUsageBatchSize     = "APP_MODEL_USAGE_BATCH_SIZE"
	envUsageMaxBuffered   = "APP_MODEL_USAGE_MAX_BUFFERED"
	envUsageRetention     = "APP_MODEL_USAGE_RETENTION"
	envUsageScanBytes     = "APP_MODEL_USAGE_SCAN_BYTES"
)

// MeterConfig bounds usage metering. A non-positive Retention keeps rows
// forever; a non-positive ScanBytes turns off token parsing.
type MeterConfig struct {
	FlushInterval time.Duration
	BatchSize     int
	MaxBuffered   int
	Retention     time.Duration
	ScanBytes     int
}

// DefaultMeterConfig returns the metering defaults.
func DefaultMeterConfig() MeterConfig {
	return MeterConfig{FlushInterval: 30 * time.Second, BatchSize: 200, MaxBuffered: 10000, Retention: 30 * 24 * time.Hour, ScanBytes: 16 << 10}
}

// LoadMeterConfig reads the metering config from the environment.
func LoadMeterConfig() MeterConfig {
	c := DefaultMeterConfig()
	c.FlushInterval = envDuration(envUsageFlushInterval, c.FlushInterval)
	c.BatchSize = int(envInt64(envUsageBatchSize, int64(c.BatchSize)))
	c.MaxBuffered = int(envInt64(envUsageMaxBuffered, int64(c.MaxBuffered)))
	c.Retention = envDuration(envUsageRetention, c.Retention)
	c.ScanBytes = int(envInt64(envUsageScanBytes, int64(c.ScanBytes)))
	return c
}

// UsageStore is the store surface the meter flushes to.
type UsageStore interface {
	AddModelUsage(ctx context.Context, rows []store.ModelUsage) error
	TouchModelKeys(ctx context.Context, used map[string]time.Time) error
	PruneModelUsage(ctx context.Context, before time.Time) (int64, error)
}

type meterKey struct {
	model string
	keyID string
	hour  int64
}

// observation is one finished gateway request.
type observation struct {
	model       string
	keyID       string
	at          time.Time
	status      int
	rateLimited bool
	duration    time.Duration
	ttft        time.Duration
	bytes       int64
	usage       *tokenUsage
}

// meter aggregates observations in memory and writes them out in bounded
// batches, so the request path never waits on the database.
type meter struct {
	cfg    MeterConfig
	logger *slog.Logger

	mu       sync.Mutex
	agg      map[meterKey]*store.ModelUsage
	lastUsed map[string]time.Time
	dropped  int64

	lastPrune time.Time
}

func newMeter(cfg MeterConfig, logger *slog.Logger) *meter {
	return &meter{cfg: cfg, logger: logger, agg: map[meterKey]*store.ModelUsage{}, lastUsed: map[string]time.Time{}}
}

func (m *meter) record(o observation) {
	hour := o.at.UTC().Truncate(time.Hour)
	k := meterKey{o.model, o.keyID, hour.Unix()}
	m.mu.Lock()
	defer m.mu.Unlock()
	if o.at.After(m.lastUsed[o.keyID]) {
		m.lastUsed[o.keyID] = o.at
	}
	u := m.agg[k]
	if u == nil {
		if m.cfg.MaxBuffered > 0 && len(m.agg) >= m.cfg.MaxBuffered {
			m.dropped++
			return
		}
		u = &store.ModelUsage{ModelName: o.model, KeyID: o.keyID, HourStart: hour}
		m.agg[k] = u
	}
	u.Requests++
	switch {
	case o.status >= 500:
		u.Status5xx++
	case o.status >= 400:
		u.Status4xx++
	default:
		u.Status2xx++
	}
	if o.rateLimited {
		u.RateLimited++
	}
	u.BytesOut += o.bytes
	u.DurationMsSum += o.duration.Milliseconds()
	if o.ttft > 0 {
		u.TTFTMsSum += o.ttft.Milliseconds()
		u.TTFTCount++
	}
	if o.usage != nil {
		u.UsageRequests++
		u.InputTokens += o.usage.input
		u.OutputTokens += o.usage.output
	}
}

// flush writes everything buffered. Batches that fail to write are merged
// back for the next flush.
func (m *meter) flush(ctx context.Context, st UsageStore) {
	m.mu.Lock()
	agg, used, dropped := m.agg, m.lastUsed, m.dropped
	m.agg, m.lastUsed, m.dropped = map[meterKey]*store.ModelUsage{}, map[string]time.Time{}, 0
	m.mu.Unlock()
	if dropped > 0 {
		m.logger.Warn("models: usage buffer full, dropped hourly aggregates", slog.Int64("dropped", dropped))
	}
	rows := make([]store.ModelUsage, 0, len(agg))
	for _, u := range agg {
		rows = append(rows, *u)
	}
	batch := m.cfg.BatchSize
	if batch <= 0 {
		batch = len(rows)
	}
	for i := 0; i < len(rows); i += batch {
		chunk := rows[i:min(i+batch, len(rows))]
		if err := st.AddModelUsage(ctx, chunk); err != nil {
			m.logger.Error("models: flush usage failed", slog.String("error", err.Error()), slog.Int("rows", len(chunk)))
			m.requeue(chunk)
		}
	}
	if len(used) > 0 {
		if err := st.TouchModelKeys(ctx, used); err != nil {
			m.logger.Error("models: flush key last-used failed", slog.String("error", err.Error()))
			m.requeueUsed(used)
		}
	}
	m.pruneIfDue(ctx, st)
}

func (m *meter) requeue(rows []store.ModelUsage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range rows {
		k := meterKey{r.ModelName, r.KeyID, r.HourStart.Unix()}
		cur := m.agg[k]
		if cur == nil {
			if m.cfg.MaxBuffered > 0 && len(m.agg) >= m.cfg.MaxBuffered {
				m.dropped++
				continue
			}
			cp := r
			m.agg[k] = &cp
			continue
		}
		cur.Requests += r.Requests
		cur.Status2xx += r.Status2xx
		cur.Status4xx += r.Status4xx
		cur.Status5xx += r.Status5xx
		cur.RateLimited += r.RateLimited
		cur.UsageRequests += r.UsageRequests
		cur.InputTokens += r.InputTokens
		cur.OutputTokens += r.OutputTokens
		cur.BytesOut += r.BytesOut
		cur.DurationMsSum += r.DurationMsSum
		cur.TTFTMsSum += r.TTFTMsSum
		cur.TTFTCount += r.TTFTCount
	}
}

func (m *meter) requeueUsed(used map[string]time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, at := range used {
		if at.After(m.lastUsed[id]) {
			m.lastUsed[id] = at
		}
	}
}

func (m *meter) pruneIfDue(ctx context.Context, st UsageStore) {
	if m.cfg.Retention <= 0 || time.Since(m.lastPrune) < time.Hour {
		return
	}
	m.lastPrune = time.Now()
	if _, err := st.PruneModelUsage(ctx, time.Now().Add(-m.cfg.Retention)); err != nil {
		m.logger.Error("models: prune usage failed", slog.String("error", err.Error()))
	}
}

// run flushes every interval until ctx ends, then once more.
func (m *meter) run(ctx context.Context, st UsageStore) {
	interval := m.cfg.FlushInterval
	if interval <= 0 {
		interval = DefaultMeterConfig().FlushInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			m.flush(flushCtx, st)
			cancel()
			return
		case <-t.C:
			m.flush(ctx, st)
		}
	}
}
