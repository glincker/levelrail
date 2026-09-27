package models

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	maxEngineMetricPoints = 240
	maxEngineMetricWindow = 7 * 24 * time.Hour
)

// Engine health states.
const (
	EngineHealthOK      = "ok"
	EngineHealthWarn    = "warn"
	EngineHealthUnknown = "unknown"
)

// EngineMetricsReader reads stored samples. *telemetry.DB satisfies it.
type EngineMetricsReader interface {
	Query(ctx context.Context, resourceID, metric string, from, to time.Time) ([]telemetry.Sample, error)
}

// SetEngineMetricsReader enables EngineMetrics. Call once at startup.
func (s *Service) SetEngineMetricsReader(r EngineMetricsReader) { s.engineMetrics = r }

// EngineMetricPoint is one stored reading.
type EngineMetricPoint struct {
	T time.Time `json:"t"`
	V float64   `json:"v"`
}

// EngineMetricSeries is one metric over the window. Supported is false
// when the model's engine cannot expose it.
type EngineMetricSeries struct {
	ID        string              `json:"id"`
	Label     string              `json:"label"`
	Unit      string              `json:"unit"`
	Supported bool                `json:"supported"`
	Latest    *float64            `json:"latest"`
	Points    []EngineMetricPoint `json:"points"`
}

// EngineHealth is a one-line reading of the latest samples.
type EngineHealth struct {
	State   string   `json:"state"`
	Summary string   `json:"summary"`
	Reasons []string `json:"reasons"`
}

// EngineMetricsReport is a model's engine metrics over a window.
type EngineMetricsReport struct {
	Model      string               `json:"model"`
	Engine     string               `json:"engine"`
	From       time.Time            `json:"from"`
	To         time.Time            `json:"to"`
	Collecting bool                 `json:"collecting"`
	Note       string               `json:"note"`
	Health     EngineHealth         `json:"health"`
	Series     []EngineMetricSeries `json:"series"`
}

type engineMetricDef struct {
	id, label, unit, metric string
	engines                 []string
}

var engineMetricDefs = []engineMetricDef{
	{"kv_cache", "KV cache usage", "percent", MetricEngineKVCachePercent, []string{EngineVLLM, EngineLlamaCpp}},
	{"queue", "Queued requests", "count", MetricEngineWaiting, []string{EngineVLLM, EngineLlamaCpp}},
	{"running", "Running requests", "count", MetricEngineRunning, []string{EngineVLLM, EngineLlamaCpp}},
	{"prefix_hits", "Prefix cache hits", "percent", MetricEnginePrefixHitPct, []string{EngineVLLM}},
	{"tokens_per_second", "Tokens per second", "tokens_per_second", MetricEngineGenRate, []string{EngineVLLM, EngineLlamaCpp}},
	{"ttft", "Time to first token", "seconds", MetricEngineTTFTSeconds, []string{EngineVLLM}},
	{"vram", "VRAM in use by the model", "bytes", MetricEngineVRAMBytes, []string{EngineOllama}},
	{"cpu_offload", "Share running on CPU", "percent", MetricEngineOffloadPercent, []string{EngineOllama}},
}

var engineMetricNotes = map[string]string{
	EngineVLLM:     "Scraped from the engine's /metrics endpoint every collection interval. Rates are computed between scrapes.",
	EngineLlamaCpp: "Scraped from the engine's /metrics endpoint, which llama.cpp serves only when started with --metrics. Models deployed before this feature keep running without it until restarted. llama.cpp reports no prefix cache or time to first token metrics.",
	EngineOllama:   "Ollama exposes no Prometheus metrics, so KV cache, queue depth, prefix cache and time to first token are not available for this engine. VRAM residency comes from its /api/ps endpoint. Tokens per second and first-byte latency per key are on the Usage tab, measured at the gateway.",
}

func supportsMetric(d engineMetricDef, engine string) bool {
	for _, e := range d.engines {
		if e == engine {
			return true
		}
	}
	return false
}

func envFloat(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil && v >= 0 {
		return v
	}
	return def
}

// EngineMetrics returns the model's engine metrics over window.
func (s *Service) EngineMetrics(ctx context.Context, name string, window time.Duration) (EngineMetricsReport, error) {
	m, err := s.store.GetModel(ctx, name)
	if err != nil {
		return EngineMetricsReport{}, err
	}
	if window <= 0 {
		window = time.Hour
	}
	window = min(window, maxEngineMetricWindow)
	to := time.Now().UTC()
	from := to.Add(-window)
	rep := EngineMetricsReport{Model: m.Name, Engine: m.Engine, From: from, To: to, Note: engineMetricNotes[m.Engine], Series: []EngineMetricSeries{}}
	for _, d := range engineMetricDefs {
		ser := EngineMetricSeries{ID: d.id, Label: d.label, Unit: d.unit, Supported: supportsMetric(d, m.Engine), Points: []EngineMetricPoint{}}
		if ser.Supported && s.engineMetrics != nil {
			samples, err := s.engineMetrics.Query(ctx, "model:"+m.Name, d.metric, from, to)
			if err != nil {
				return EngineMetricsReport{}, fmt.Errorf("models: query engine metric %s: %w", d.metric, err)
			}
			ser.Points = downsample(samples)
			if n := len(samples); n > 0 {
				v := samples[n-1].Value
				ser.Latest = &v
				rep.Collecting = true
			}
		}
		rep.Series = append(rep.Series, ser)
	}
	rep.Health = engineHealth(rep)
	return rep, nil
}

func downsample(samples []telemetry.Sample) []EngineMetricPoint {
	step := 1
	if len(samples) > maxEngineMetricPoints {
		step = (len(samples) + maxEngineMetricPoints - 1) / maxEngineMetricPoints
	}
	out := make([]EngineMetricPoint, 0, min(len(samples), maxEngineMetricPoints))
	for i := 0; i < len(samples); i += step {
		out = append(out, EngineMetricPoint{T: samples[i].Timestamp, V: samples[i].Value})
	}
	return out
}

func engineHealth(rep EngineMetricsReport) EngineHealth {
	if !rep.Collecting {
		return EngineHealth{State: EngineHealthUnknown, Summary: "No engine samples yet.", Reasons: []string{}}
	}
	kvWarn := envFloat("APP_MODEL_KV_WARN_PERCENT", 90)
	queueWarn := envFloat("APP_MODEL_QUEUE_WARN", 4)
	reasons := []string{}
	for _, s := range rep.Series {
		if s.Latest == nil {
			continue
		}
		switch s.ID {
		case "kv_cache":
			if *s.Latest >= kvWarn {
				reasons = append(reasons, fmt.Sprintf("KV cache is %.0f%% full; long or many requests may queue or be preempted.", *s.Latest))
			}
		case "queue":
			if *s.Latest > queueWarn {
				reasons = append(reasons, fmt.Sprintf("%.0f requests are waiting for a slot.", *s.Latest))
			}
		case "cpu_offload":
			if *s.Latest > 0 {
				reasons = append(reasons, fmt.Sprintf("%.0f%% of the model runs on CPU because it did not fit in VRAM; expect slow generation.", *s.Latest))
			}
		}
	}
	if len(reasons) > 0 {
		return EngineHealth{State: EngineHealthWarn, Summary: reasons[0], Reasons: reasons}
	}
	return EngineHealth{State: EngineHealthOK, Summary: "Engine looks healthy.", Reasons: reasons}
}
