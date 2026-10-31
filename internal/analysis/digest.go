package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/costestimate"
	"github.com/GLINCKER/levelrail/internal/rightsizing"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/kit/untrusted"
)

const (
	metricMemory = "memory_usage_bytes"
	metricCPU    = "cpu_percent"
)

// TelemetryReader is the metrics and logs surface the dashboard already uses.
type TelemetryReader interface {
	QueryMetrics(ctx context.Context, resourceID, metric string, from, to time.Time) ([]telemetry.Sample, error)
	QueryLogs(ctx context.Context, resourceID string, from, to time.Time, query string) ([]telemetry.LogEntry, error)
}

// DeployReader lists an app's deploy attempts.
type DeployReader interface {
	ListDeployAttempts(ctx context.Context, serviceName string) ([]store.DeployAttempt, error)
}

// AlertReader reads alert history.
type AlertReader interface {
	ListHistory(ctx context.Context, f alerting.HistoryFilter) ([]alerting.HistoryEntry, error)
}

// ServiceReader reads the declared resource limits.
type ServiceReader interface {
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
}

// Sources groups the existing readers the digest uses; nil ones are skipped.
type Sources struct {
	Telemetry TelemetryReader
	Deploys   DeployReader
	Alerts    AlertReader
	Services  ServiceReader
}

// Collect builds the digest for app ending at end. A failing reader only
// drops its own section, so partial data still yields a digest.
func Collect(ctx context.Context, src Sources, cfg Config, app string, end time.Time) Digest {
	start := end.Add(-cfg.Window)
	d := Digest{App: app, WindowStart: start, WindowEnd: end}
	rid := "service:" + app

	var memAll, cpuAll []telemetry.Sample
	if src.Telemetry != nil {
		from := start.Add(-cfg.Baseline)
		memAll, _ = src.Telemetry.QueryMetrics(ctx, rid, metricMemory, from, end)
		cpuAll, _ = src.Telemetry.QueryMetrics(ctx, rid, metricCPU, from, end)
		d.Metrics = []MetricStat{
			metricStat(metricMemory, memAll, start, cfg),
			metricStat(metricCPU, cpuAll, start, cfg),
		}
		restarts, _ := src.Telemetry.QueryMetrics(ctx, rid, telemetry.MetricContainerRestartCount, start, end)
		sort.Slice(restarts, func(i, j int) bool { return restarts[i].Timestamp.Before(restarts[j].Timestamp) })
		d.Restarts = len(restarts)
		for _, r := range restarts {
			d.Timeline = append(d.Timeline, Event{At: r.Timestamp.UTC(), Kind: "restart", Ref: fmt.Sprintf("restart:%d", r.Timestamp.Unix()), Detail: "container restarted"})
		}
		logs, _ := src.Telemetry.QueryLogs(ctx, rid, start, end, "")
		d.LogLines = len(logs)
		d.Logs = groupLogs(logs, cfg)
	}

	if src.Deploys != nil {
		attempts, _ := src.Deploys.ListDeployAttempts(ctx, app)
		for _, a := range attempts {
			if a.StartedAt.Before(start) || a.StartedAt.After(end) {
				continue
			}
			detail := fmt.Sprintf("deploy %s", a.Status)
			if a.Error != "" {
				detail += ": " + untrusted.Sanitize(a.Error, cfg.LogSampleBytes)
			}
			d.Timeline = append(d.Timeline, Event{At: a.StartedAt.UTC(), Kind: "deploy", Ref: "deploy:" + a.ID, Detail: detail})
		}
	}
	if src.Alerts != nil {
		hist, _ := src.Alerts.ListHistory(ctx, alerting.HistoryFilter{App: app, Since: start, Event: alerting.EventFired, Limit: cfg.MaxEvents})
		for _, h := range hist {
			if h.At.After(end) {
				continue
			}
			d.Timeline = append(d.Timeline, Event{At: h.At.UTC(), Kind: "alert", Ref: "alert:" + h.ID, Detail: untrusted.Sanitize(h.RuleName+" ("+h.Severity+")", cfg.LogSampleBytes)})
		}
	}
	sort.SliceStable(d.Timeline, func(i, j int) bool {
		a, b := d.Timeline[i], d.Timeline[j]
		if !a.At.Equal(b.At) {
			return a.At.Before(b.At)
		}
		return a.Ref < b.Ref
	})
	if len(d.Timeline) > cfg.MaxEvents {
		d.Timeline = d.Timeline[len(d.Timeline)-cfg.MaxEvents:]
	}

	var memLimit, nanoCPU int64
	if src.Services != nil {
		if svc, err := src.Services.GetDesiredService(ctx, app); err == nil && svc != nil && svc.Resources != nil {
			memLimit, nanoCPU = svc.Resources.MemoryBytes, svc.Resources.NanoCPUs
		}
	}
	d.MemoryLimit = memLimit
	d.CPUPercentLimit = float64(nanoCPU) / 1e7
	d.Sizing, d.MonthlySavingUSD, d.SavingProvider = sizing(app, end, cfg, memAll, cpuAll, memLimit, nanoCPU)
	return d
}

func sizing(app string, end time.Time, cfg Config, mem, cpu []telemetry.Sample, memLimit, nanoCPU int64) ([]Sizing, float64, string) {
	if memLimit <= 0 && nanoCPU <= 0 {
		return nil, 0, ""
	}
	conv := func(in []telemetry.Sample) []rightsizing.Sample {
		out := make([]rightsizing.Sample, len(in))
		for i, s := range in {
			out[i] = rightsizing.Sample{Timestamp: s.Timestamp, Value: s.Value}
		}
		return out
	}
	res := rightsizing.Recommend(rightsizing.Input{
		ServiceName: app, Now: end, LookbackWindow: cfg.Window + cfg.Baseline,
		MemorySamples: conv(mem), CPUPercentSamples: conv(cpu),
		CurrentMemoryBytes: memLimit, CurrentNanoCPUs: nanoCPU, Tuning: cfg.Sizing,
	})
	out := []Sizing{
		{Dimension: "memory", Action: res.Memory.Action, Confidence: res.Memory.Confidence, DataSufficient: res.Memory.DataSufficient, CurrentLimit: res.Memory.CurrentLimit, P95Usage: round(res.Memory.P95Usage), SuggestedLimit: res.Memory.SuggestedLimit, SuggestedText: rightsizing.FormatMemory(res.Memory.SuggestedLimit)},
		{Dimension: "cpu", Action: res.CPU.Action, Confidence: res.CPU.Confidence, DataSufficient: res.CPU.DataSufficient, CurrentLimit: res.CPU.CurrentLimit, P95Usage: round(res.CPU.P95Usage), SuggestedLimit: res.CPU.SuggestedLimit, SuggestedText: rightsizing.FormatCPU(res.CPU.SuggestedLimit)},
	}
	sugMem, sugCPU := memLimit, nanoCPU
	if res.Memory.DataSufficient && res.Memory.Action == rightsizing.ActionLower {
		sugMem = res.Memory.SuggestedLimit
	}
	if res.CPU.DataSufficient && res.CPU.Action == rightsizing.ActionLower {
		sugCPU = res.CPU.SuggestedLimit
	}
	if s, ok := costestimate.Saving(memLimit, nanoCPU, sugMem, sugCPU, costestimate.RateTableFromEnv()); ok {
		return out, s.USD, s.ProviderKey
	}
	return out, 0, ""
}

func metricStat(metric string, all []telemetry.Sample, windowStart time.Time, cfg Config) MetricStat {
	st := MetricStat{Evidence: "metric:" + metric, Metric: metric, Trend: "flat"}
	var win, base []float64
	var winSamples []telemetry.Sample
	for _, s := range all {
		if s.Timestamp.Before(windowStart) {
			base = append(base, s.Value)
		} else {
			win = append(win, s.Value)
			winSamples = append(winSamples, s)
		}
	}
	st.Samples = len(win)
	if len(win) == 0 {
		st.Trend = "none"
		return st
	}
	sorted := append([]float64(nil), win...)
	sort.Float64s(sorted)
	st.Min, st.Max = round(sorted[0]), round(sorted[len(sorted)-1])
	st.P50, st.P95 = round(pct(sorted, 50)), round(pct(sorted, 95))
	st.Mean = round(mean(win))
	if len(base) > 0 {
		bm := mean(base)
		st.BaselineMean = round(bm)
		if bm != 0 {
			st.TrendPct = round((mean(win) - bm) / math.Abs(bm) * 100)
		}
		switch {
		case st.TrendPct >= cfg.TrendPct:
			st.Trend = "rising"
		case st.TrendPct <= -cfg.TrendPct:
			st.Trend = "falling"
		}
	}
	ref := base
	if len(ref) < 5 {
		ref = win
	}
	med, mad := medianMAD(ref)
	if mad == 0 {
		return st
	}
	for _, s := range winSamples {
		z := 0.6745 * (s.Value - med) / mad
		if math.Abs(z) < cfg.AnomalyZ {
			continue
		}
		st.AnomalyCount++
		if math.Abs(z) > st.MaxZ {
			st.MaxZ = round(math.Abs(z))
		}
		if st.FirstAnomaly == nil || s.Timestamp.Before(*st.FirstAnomaly) {
			t := s.Timestamp.UTC()
			st.FirstAnomaly = &t
		}
	}
	return st
}

var (
	reTime  = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[t ]\d{2}:\d{2}:\d{2}(\.\d+)?(z|[+-]\d{2}:?\d{2})?`)
	reUUID  = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	reHex   = regexp.MustCompile(`\b(0x)?[0-9a-f]{8,}\b`)
	reNum   = regexp.MustCompile(`\d+`)
	reSpace = regexp.MustCompile(`\s+`)
)

// fingerprint masks the parts of a line that vary between occurrences.
func fingerprint(msg string) string {
	s := strings.ToLower(msg)
	s = reTime.ReplaceAllString(s, "<ts>")
	s = reUUID.ReplaceAllString(s, "<id>")
	s = reHex.ReplaceAllString(s, "<hex>")
	s = reNum.ReplaceAllString(s, "<n>")
	return strings.TrimSpace(reSpace.ReplaceAllString(s, " "))
}

func logLevel(e telemetry.LogEntry) string {
	if e.Structured && e.FieldsJSON != "" {
		var f map[string]any
		if json.Unmarshal([]byte(e.FieldsJSON), &f) == nil {
			for _, k := range []string{"level", "severity", "lvl"} {
				if v, ok := f[k].(string); ok && v != "" {
					return normLevel(v)
				}
			}
		}
	}
	l := strings.ToLower(e.Message)
	switch {
	case strings.Contains(l, "panic") || strings.Contains(l, "fatal"):
		return "fatal"
	case strings.Contains(l, "error") || strings.Contains(l, "exception") || strings.Contains(l, "level=err"):
		return "error"
	case strings.Contains(l, "warn"):
		return "warn"
	}
	return "info"
}

func normLevel(v string) string {
	switch l := strings.ToLower(v); {
	case strings.HasPrefix(l, "fatal"), strings.HasPrefix(l, "crit"), strings.HasPrefix(l, "panic"):
		return "fatal"
	case strings.HasPrefix(l, "err"):
		return "error"
	case strings.HasPrefix(l, "warn"):
		return "warn"
	case strings.HasPrefix(l, "debug"), strings.HasPrefix(l, "trace"):
		return "debug"
	}
	return "info"
}

var levelRank = map[string]int{"fatal": 0, "error": 1, "warn": 2, "info": 3, "debug": 4}

func groupLogs(entries []telemetry.LogEntry, cfg Config) []LogGroup {
	groups := map[string]*LogGroup{}
	for _, e := range entries {
		msg := untrusted.Redact(untrusted.Clean(e.Message))
		level := logLevel(e)
		fp := fingerprint(msg)
		key := level + "|" + fp
		g := groups[key]
		ts := e.Timestamp.UTC()
		if g == nil {
			sum := sha256.Sum256([]byte(key))
			g = &LogGroup{Evidence: "log:" + level + ":" + hex.EncodeToString(sum[:4]), Level: level, Fingerprint: untrusted.Truncate(fp, cfg.LogSampleBytes), Sample: untrusted.Truncate(msg, cfg.LogSampleBytes), FirstSeen: ts, LastSeen: ts}
			groups[key] = g
		}
		g.Count++
		if ts.Before(g.FirstSeen) {
			g.FirstSeen = ts
		}
		if ts.After(g.LastSeen) {
			g.LastSeen = ts
		}
	}
	out := make([]LogGroup, 0, len(groups))
	for _, g := range groups {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if levelRank[a.Level] != levelRank[b.Level] {
			return levelRank[a.Level] < levelRank[b.Level]
		}
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		return a.Evidence < b.Evidence
	})
	if len(out) > cfg.LogGroups {
		out = out[:cfg.LogGroups]
	}
	return out
}

func round(v float64) float64 { return math.Round(v*100) / 100 }

func mean(v []float64) float64 {
	var s float64
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func pct(sorted []float64, p float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	pos := p / 100 * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	return sorted[lo] + (sorted[hi]-sorted[lo])*(pos-float64(lo))
}

func medianMAD(v []float64) (float64, float64) {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	med := pct(s, 50)
	dev := make([]float64, len(s))
	for i, x := range s {
		dev[i] = math.Abs(x - med)
	}
	sort.Float64s(dev)
	return med, pct(dev, 50)
}
