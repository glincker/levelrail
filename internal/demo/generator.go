package demo

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// LogPublisher is the slice of telemetry.LogBroadcaster the generator needs.
type LogPublisher interface {
	Publish(entry telemetry.LogEntry)
}

// SampleWriter is the slice of telemetry.DB the generator needs.
type SampleWriter interface {
	WriteSamples(ctx context.Context, samples []telemetry.Sample) error
}

// Generator keeps the demo visibly alive: a synthetic log line per service
// and a metric sample per service, both deterministic given the seed.
type Generator struct {
	Services []string
	Logs     LogPublisher
	Samples  SampleWriter
	Now      func() time.Time
	Seed     int64
	// LogEvery and SampleEvery default to 2s and 15s.
	LogEvery    time.Duration
	SampleEvery time.Duration

	rng *rng
}

var plainLines = []string{
	"worker heartbeat ok",
	"cache refresh completed",
	"scheduled job tick",
	"connection pool at 40 percent",
}

// TickLogs publishes one line per service. Roughly 70 percent are structured JSON.
func (g *Generator) TickLogs(at time.Time) {
	g.init()
	if g.Logs == nil {
		return
	}
	for _, name := range g.Services {
		var msg string
		structured := g.rng.intn(10) < 7
		if structured {
			status := 200
			if g.rng.intn(40) == 0 {
				status = 500
			}
			msg = fmt.Sprintf(`{"level":"info","route":"/api/v1/items","status":%d,"ms":%d,"request_id":"req_%s"}`, status, 4+g.rng.intn(120), g.rng.hex(8))
		} else {
			msg = plainLines[g.rng.intn(len(plainLines))]
		}
		g.Logs.Publish(telemetry.LogEntry{ResourceID: "service:" + name, Stream: "stdout", Timestamp: at, Message: msg, Structured: structured})
	}
}

// TickSamples writes a CPU and memory sample per service, a smooth value around a per-service baseline.
func (g *Generator) TickSamples(ctx context.Context, at time.Time) error {
	g.init()
	if g.Samples == nil {
		return nil
	}
	samples := make([]telemetry.Sample, 0, len(g.Services)*2)
	for i, name := range g.Services {
		id := "service:" + name
		base := float64(8 + (i*7)%30)
		cpu := base + float64(g.rng.intn(600))/100
		mem := float64(150+(i*23)%200)*(1<<20) + float64(g.rng.intn(20))*(1<<20)
		samples = append(samples,
			telemetry.Sample{ResourceID: id, Metric: "cpu_percent", Timestamp: at, Value: cpu},
			telemetry.Sample{ResourceID: id, Metric: "memory_usage_bytes", Timestamp: at, Value: mem},
		)
	}
	return g.Samples.WriteSamples(ctx, samples)
}

// Run ticks until ctx is done.
func (g *Generator) Run(ctx context.Context) {
	g.init()
	logT := time.NewTicker(g.LogEvery)
	sampleT := time.NewTicker(g.SampleEvery)
	defer logT.Stop()
	defer sampleT.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-logT.C:
			g.TickLogs(g.Now())
		case <-sampleT.C:
			_ = g.TickSamples(ctx, g.Now())
		}
	}
}

func (g *Generator) init() {
	if g.rng != nil {
		return
	}
	g.rng = newRNG(g.Seed)
	if g.Now == nil {
		g.Now = time.Now
	}
	if g.LogEvery <= 0 {
		g.LogEvery = 2 * time.Second
	}
	if g.SampleEvery <= 0 {
		g.SampleEvery = 15 * time.Second
	}
}
