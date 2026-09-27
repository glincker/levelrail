package models

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	defaultEngineMetricsInterval = 15 * time.Second
	engineScrapeConcurrency      = 8
)

// EngineMetricsInterval reads APP_MODEL_ENGINE_METRICS_INTERVAL.
func EngineMetricsInterval() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("APP_MODEL_ENGINE_METRICS_INTERVAL")); err == nil && d > 0 {
		return d
	}
	return defaultEngineMetricsInterval
}

// EngineMetricsStore is the model listing the collector needs.
type EngineMetricsStore interface {
	ListModels(ctx context.Context) ([]store.Model, error)
}

// SampleWriter persists telemetry samples.
type SampleWriter interface {
	WriteSamples(ctx context.Context, samples []telemetry.Sample) error
}

type scrapeMemo struct {
	scrape engineScrape
	at     time.Time
}

// EngineMetricsCollector scrapes each running engine's metrics endpoint
// and writes the readings to the telemetry store. It reaches engines the
// same way the readiness prober does.
type EngineMetricsCollector struct {
	store  EngineMetricsStore
	sink   SampleWriter
	client *http.Client
	logger *slog.Logger

	mu   sync.Mutex
	prev map[string]scrapeMemo
}

// NewEngineMetricsCollector builds a collector. client may be nil.
func NewEngineMetricsCollector(st EngineMetricsStore, sink SampleWriter, client *http.Client, logger *slog.Logger) *EngineMetricsCollector {
	if client == nil {
		client = &http.Client{Timeout: probeTimeout}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &EngineMetricsCollector{store: st, sink: sink, client: client, logger: logger, prev: map[string]scrapeMemo{}}
}

// Run collects every interval until ctx ends.
func (c *EngineMetricsCollector) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			c.Collect(ctx, now)
		}
	}
}

// Collect scrapes every model once and writes the resulting samples.
func (c *EngineMetricsCollector) Collect(ctx context.Context, now time.Time) {
	list, err := c.store.ListModels(ctx)
	if err != nil {
		c.logger.Warn("models: engine metrics list failed", slog.String("error", err.Error()))
		return
	}
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		samples []telemetry.Sample
		sem     = make(chan struct{}, engineScrapeConcurrency)
		live    = map[string]bool{}
	)
	for _, m := range list {
		if m.Deleting || m.EndpointDial == "" {
			continue
		}
		live[m.Name] = true
		wg.Add(1)
		sem <- struct{}{}
		go func(m store.Model) {
			defer func() { <-sem; wg.Done() }()
			got := c.scrapeOne(ctx, m, now)
			mu.Lock()
			samples = append(samples, got...)
			mu.Unlock()
		}(m)
	}
	wg.Wait()
	c.mu.Lock()
	for name := range c.prev {
		if !live[name] {
			delete(c.prev, name)
		}
	}
	c.mu.Unlock()
	if len(samples) == 0 {
		return
	}
	if err := c.sink.WriteSamples(ctx, samples); err != nil && ctx.Err() == nil {
		c.logger.Warn("models: engine metrics write failed", slog.String("error", err.Error()))
	}
}

func (c *EngineMetricsCollector) scrapeOne(ctx context.Context, m store.Model, now time.Time) []telemetry.Sample {
	cur, err := c.fetch(ctx, m)
	if err != nil {
		c.logger.Debug("models: engine metrics scrape skipped", slog.String("model", m.Name), slog.String("error", err.Error()))
		return nil
	}
	c.mu.Lock()
	prev, hadPrev := c.prev[m.Name]
	c.prev[m.Name] = scrapeMemo{scrape: cur, at: now}
	c.mu.Unlock()
	var rates engineRates
	if hadPrev {
		rates = computeRates(prev.scrape, cur, now.Sub(prev.at).Seconds())
	}
	return samplesFor("model:"+m.Name, now, cur, rates)
}

func (c *EngineMetricsCollector) fetch(ctx context.Context, m store.Model) (engineScrape, error) {
	switch m.Engine {
	case EngineVLLM, EngineLlamaCpp:
		p, err := c.getProm(ctx, "http://"+m.EndpointDial+"/metrics")
		if err != nil {
			return engineScrape{}, err
		}
		if m.Engine == EngineVLLM {
			return scrapeFromVLLM(p), nil
		}
		return scrapeFromLlamaCpp(p), nil
	case EngineOllama:
		resp, err := c.get(ctx, "http://"+m.EndpointDial+"/api/ps")
		if err != nil {
			return engineScrape{}, err
		}
		defer func() { _ = resp.Body.Close() }()
		return scrapeFromOllamaPS(resp.Body)
	}
	return engineScrape{}, fmt.Errorf("models: no metrics scraper for engine %q", m.Engine)
}

func (c *EngineMetricsCollector) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("models: build metrics request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("models: scrape %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("models: scrape %s: status %d", url, resp.StatusCode)
	}
	return resp, nil
}

func (c *EngineMetricsCollector) getProm(ctx context.Context, url string) (promSeries, error) {
	resp, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return parseProm(resp.Body)
}

func samplesFor(resource string, at time.Time, s engineScrape, r engineRates) []telemetry.Sample {
	var out []telemetry.Sample
	add := func(metric string, v *float64) {
		if v != nil {
			out = append(out, telemetry.Sample{ResourceID: resource, Metric: metric, Timestamp: at, Value: *v})
		}
	}
	add(MetricEngineKVCachePercent, s.kvPercent)
	add(MetricEngineRunning, s.running)
	add(MetricEngineWaiting, s.waiting)
	add(MetricEngineVRAMBytes, s.vramBytes)
	add(MetricEngineOffloadPercent, s.offloadPercent)
	add(MetricEngineGenRate, r.tokensPerSecond)
	add(MetricEnginePrefixHitPct, r.prefixHitPct)
	add(MetricEngineTTFTSeconds, r.ttftSeconds)
	return out
}
