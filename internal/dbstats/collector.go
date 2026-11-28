// Package dbstats samples engine statistics by running a read only script inside each database container.
package dbstats

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/glincker/levelrail/internal/dbaccess"
	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/telemetry"
)

// Target is one running database to sample.
type Target struct {
	Name        string
	Engine      string
	ContainerID string
}

// Source lists databases that are running now on this node; stopped ones are simply absent.
type Source interface {
	RunningDatabases(ctx context.Context) ([]Target, error)
}

// Writer is the telemetry store surface the collector needs.
type Writer interface {
	WriteSamples(ctx context.Context, samples []telemetry.Sample) error
}

// Config carries limits the caller reads from settings before every tick.
type Config struct {
	// Timeout bounds one script, both inside psql (statement_timeout) and on the exec.
	Timeout time.Duration
	// MaxDatabases caps per-database rows and size series.
	MaxDatabases int
	// MaxOutputBytes caps the script output read back; 0 derives it from MaxDatabases.
	MaxOutputBytes int64
}

const (
	minTimeout          = time.Second
	defaultMaxDatabases = 20
	maxDatabasesCeiling = 200
	// bytesPerRow is a generous size for one output line, used to derive the output cap.
	bytesPerRow = 256
	workers     = 4
	warnEvery   = 10 * time.Minute
)

// normalize forces a config into safe bounds so a zero value can never disable a limit.
func (c Config) normalize() Config {
	if c.Timeout < minTimeout {
		c.Timeout = minTimeout
	}
	if c.MaxDatabases <= 0 {
		c.MaxDatabases = defaultMaxDatabases
	}
	c.MaxDatabases = min(c.MaxDatabases, maxDatabasesCeiling)
	if c.MaxOutputBytes <= 0 {
		c.MaxOutputBytes = int64(c.MaxDatabases+16) * bytesPerRow
	}
	return c
}

// Status is the last sample outcome for one database.
type Status struct {
	LastOK    time.Time
	LastError string
	LastErrAt time.Time
}

// ResourceID is the telemetry resource id for a database.
func ResourceID(name string) string { return "database:" + name }

// Collector samples every running Postgres database.
type Collector struct {
	src   Source
	ex    dbaccess.Exec
	out   Writer
	cfg   func() Config
	log   *slog.Logger
	now   func() time.Time
	every func() time.Duration

	mu     sync.Mutex
	prev   map[string]map[int64]dbRow
	warned map[string]time.Time
	status map[string]Status
}

// New builds a Collector; cfg and every are read before each tick so saved settings apply live.
func New(src Source, ex dbaccess.Exec, out Writer, cfg func() Config, every func() time.Duration, log *slog.Logger) *Collector {
	if log == nil {
		log = slog.Default()
	}
	return &Collector{src: src, ex: ex, out: out, cfg: cfg, every: every, log: log, now: time.Now,
		prev: map[string]map[int64]dbRow{}, warned: map[string]time.Time{}, status: map[string]Status{}}
}

// Status returns the last outcome per database name.
func (c *Collector) Status() map[string]Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]Status, len(c.status))
	for k, v := range c.status {
		out[k] = v
	}
	return out
}

// Run samples until ctx ends.
func (c *Collector) Run(ctx context.Context) {
	for {
		c.CollectOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(c.every()):
		}
	}
}

// CollectOnce samples each running Postgres database with a small worker pool and a per tick budget.
func (c *Collector) CollectOnce(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, max(c.every(), minTimeout))
	defer cancel()
	targets, err := c.src.RunningDatabases(ctx)
	if err != nil {
		c.log.Warn("db stats: list databases", "err", err)
		return
	}
	cfg := c.cfg().normalize()
	live := map[string]bool{}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, t := range targets {
		if t.Engine != store.EnginePostgres || t.ContainerID == "" {
			continue
		}
		live[t.Name] = true
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(t Target) {
			defer wg.Done()
			defer func() { <-sem }()
			c.collectOne(ctx, cfg, t)
		}(t)
	}
	wg.Wait()
	c.forget(live)
}

// forget drops state of databases no longer running, so a stop and start never diffs across the gap.
func (c *Collector) forget(live map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name := range c.prev {
		if !live[name] {
			delete(c.prev, name)
			delete(c.status, name)
			delete(c.warned, name)
		}
	}
}

func (c *Collector) collectOne(ctx context.Context, cfg Config, t Target) {
	ms := int(cfg.Timeout.Milliseconds())
	raw, err := c.run(ctx, cfg, t, psqlCommand(ms, scalarSQL(cfg.MaxDatabases)))
	if err != nil {
		c.fail(t.Name, err)
		return
	}
	res, err := parseScalars(raw)
	if err != nil {
		c.fail(t.Name, err)
		return
	}
	vals := res.Gauges
	c.mu.Lock()
	prev, had := c.prev[t.Name]
	c.prev[t.Name] = res.Rows
	c.mu.Unlock()
	if had {
		vals = append(vals, counterDeltas(prev, res.Rows)...)
	}
	// Sizes run apart so a slow size scan or a dropped database never loses the cheap metrics.
	if sraw, serr := c.run(ctx, cfg, t, psqlCommand(ms, sizeSQL(cfg.MaxDatabases))); serr != nil {
		c.warn(t.Name, fmt.Errorf("sizes: %w", serr))
	} else if sizes, perr := parseSizes(sraw, cfg.MaxDatabases); perr != nil {
		c.warn(t.Name, fmt.Errorf("sizes: %w", perr))
	} else {
		vals = append(vals, sizes...)
	}
	ts := c.now()
	samples := make([]telemetry.Sample, len(vals))
	for i, v := range vals {
		samples[i] = telemetry.Sample{ResourceID: ResourceID(t.Name), Metric: v.Metric, Timestamp: ts, Value: v.Value}
	}
	if err := c.out.WriteSamples(ctx, samples); err != nil {
		c.fail(t.Name, fmt.Errorf("write: %w", err))
		return
	}
	c.mu.Lock()
	c.status[t.Name] = Status{LastOK: ts}
	c.mu.Unlock()
}

func (c *Collector) run(ctx context.Context, cfg Config, t Target, script string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	rc, err := c.ex.ExecWithInput(ctx, t.ContainerID, []string{"sh", "-c", script}, strings.NewReader(""))
	if err != nil {
		return "", fmt.Errorf("exec stats: %w", err)
	}
	defer func() { _ = rc.Close() }()
	// One byte over the cap proves truncation, so a cut last line is never parsed.
	out, err := io.ReadAll(io.LimitReader(rc, cfg.MaxOutputBytes+1))
	if err != nil {
		return "", fmt.Errorf("read stats: %w", err)
	}
	if int64(len(out)) > cfg.MaxOutputBytes {
		return "", errors.New("stats output over cap")
	}
	return string(out), nil
}

func (c *Collector) fail(name string, err error) {
	c.mu.Lock()
	st := c.status[name]
	st.LastError, st.LastErrAt = err.Error(), c.now()
	c.status[name] = st
	c.mu.Unlock()
	c.warn(name, err)
}

// warn logs at most once per warnEvery per database so a dead container cannot flood the log.
func (c *Collector) warn(name string, err error) {
	c.mu.Lock()
	last, seen := c.warned[name]
	due := !seen || c.now().Sub(last) >= warnEvery
	if due {
		c.warned[name] = c.now()
	}
	c.mu.Unlock()
	if due {
		c.log.Warn("db stats: sample failed", "database", name, "err", err)
	}
}
