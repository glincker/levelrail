package demo

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	fineStep       = 5 * time.Minute
	coarseStep     = time.Hour
	fineWindow     = 24 * time.Hour
	latencySigma   = 0.6
	normalMedian   = 40.0
	slowMedian     = 90.0
	logLinesPerApp = 120
	logHistory     = 3 * 24 * time.Hour
	// crashLogLines must exceed the 200 lines the crashloop panel surfaces.
	crashLogLines = 238
)

// SeedTelemetry writes deterministic metrics, deploy markers and log lines
// into the telemetry store, all anchored on opts.Anchor so the series ends at
// "now". The relational store must already be seeded.
func SeedTelemetry(ctx context.Context, db *store.DB, tdb *telemetry.DB, opts Options) error {
	opts.Anchor = opts.Anchor.UTC()
	t := &telemetrySeeder{ctx: ctx, db: db, tdb: tdb, anchor: opts.Anchor, story: storyFor(opts.Anchor, opts.calm()), rng: newRNG(opts.Seed ^ 0x7e1e)}
	for _, step := range []func() error{t.metrics, t.nodeMetrics, t.markers, t.logs, t.deployLogs} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

type telemetrySeeder struct {
	ctx    context.Context
	db     *store.DB
	tdb    *telemetry.DB
	anchor time.Time
	story  storyTimes
	rng    *rng
}

// grid returns the sample times: fine for the last day and around the
// incident, hourly before that, always ending exactly at the anchor.
func (t *telemetrySeeder) grid() []time.Time {
	seen := map[int64]time.Time{}
	add := func(from, to time.Time, step time.Duration) {
		for at := from; !at.After(to); at = at.Add(step) {
			if at.After(t.anchor) {
				break
			}
			seen[at.Unix()] = at
		}
	}
	add(t.anchor.Add(-fineWindow), t.anchor, fineStep)
	add(t.anchor.AddDate(0, 0, -historyDays), t.anchor.Add(-fineWindow), coarseStep)
	add(t.story.slowDeploy.Add(-30*time.Minute), t.story.slowRecovered.Add(30*time.Minute), fineStep)
	seen[t.anchor.Unix()] = t.anchor
	out := make([]time.Time, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

func (t *telemetrySeeder) inSlowWindow(at time.Time) bool {
	return !at.Before(t.story.slowDeploy) && at.Before(t.story.slowRecovered)
}

func (t *telemetrySeeder) metrics() error {
	times := t.grid()
	var batch []telemetry.Sample
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := t.tdb.WriteSamples(t.ctx, batch)
		batch = batch[:0]
		return err
	}
	for i, a := range appSpecs {
		id := "service:" + a.name
		for n, at := range times {
			step := fineStep
			if n+1 < len(times) {
				step = times[n+1].Sub(at)
			}
			batch = append(batch, t.resourceSamples(i, a, id, at)...)
			if len(a.hosts) > 0 && !a.suspended {
				win := t.requestWindow(i, a.name, at, step)
				batch = append(batch, win.Samples(id, at)...)
			}
		}
		if err := flush(); err != nil {
			return fmt.Errorf("demo: write samples for %q: %w", a.name, err)
		}
	}
	return nil
}

func (t *telemetrySeeder) resourceSamples(i int, a appSpec, id string, at time.Time) []telemetry.Sample {
	if a.suspended {
		return nil
	}
	hour := float64(at.Hour()) + float64(at.Minute())/60
	diurnal := 0.5 + 0.5*math.Sin((hour-8)/24*2*math.Pi)
	cpu := float64(6+(i*5)%18) + 10*diurnal + float64(t.rng.intn(300))/100
	memBase := float64(150+(i*23)%200) * (1 << 20)
	mem := memBase + float64(t.rng.intn(12))*(1<<20)
	if t.inSlowWindow(at) && a.name == incidentApp {
		cpu += 28
	}
	if a.name == incidentApp {
		// Sawtooth leak ending in restarts about five days ago.
		age := t.anchor.Sub(at)
		if age > 5*24*time.Hour && age < 7*24*time.Hour {
			phase := math.Mod(float64(at.Unix()), 8*3600) / (8 * 3600)
			mem += phase * 300 * (1 << 20)
		}
	}
	if a.name == crashloopApp && !at.Before(t.story.crashStart) && at.Before(t.story.crashEnd) {
		mem = 505 * (1 << 20)
		cpu += 35
	}
	return []telemetry.Sample{
		{ResourceID: id, Metric: "cpu_percent", Timestamp: at, Value: cpu},
		{ResourceID: id, Metric: "memory_usage_bytes", Timestamp: at, Value: mem},
	}
}

func (t *telemetrySeeder) requestWindow(i int, name string, at time.Time, step time.Duration) telemetry.RequestWindow {
	hour := float64(at.Hour()) + float64(at.Minute())/60
	rps := (2 + float64((i*3)%9)) * (0.4 + 0.6*(0.5+0.5*math.Sin((hour-9)/24*2*math.Pi)))
	if at.Weekday() == time.Tuesday {
		rps *= 1.8
	}
	total := rps * step.Seconds() * (0.9 + float64(t.rng.intn(20))/100)
	errRate := 0.004
	median := normalMedian
	if name == incidentApp && t.inSlowWindow(at) {
		median, errRate = slowMedian, 0.03
	}
	if name == crashloopApp && !at.Before(t.story.crashStart) && at.Before(t.story.crashEnd) {
		errRate = 0.4
	}
	w := telemetry.RequestWindow{Requests: uint64(total)}
	w.Status5xx = uint64(total * errRate)
	w.Status4xx = uint64(total * 0.02)
	w.Status3xx = uint64(total * 0.03)
	w.Status2xx = w.Requests - w.Status5xx - w.Status4xx - w.Status3xx
	prev := 0.0
	for b := 0; b < telemetry.LatencyBucketCount; b++ {
		cdf := 1.0
		if b < len(telemetry.LatencyBucketsMS) {
			cdf = lognormalCDF(telemetry.LatencyBucketsMS[b], median, latencySigma)
		}
		w.Latency[b] = uint64(total * (cdf - prev))
		prev = cdf
	}
	return w
}

func lognormalCDF(x, median, sigma float64) float64 {
	return 0.5 * (1 + math.Erf((math.Log(x)-math.Log(median))/(sigma*math.Sqrt2)))
}

// nodeMetrics gives the worker disk a rising line that crosses 82 percent at
// the anchor, matching the firing disk alert.
func (t *telemetrySeeder) nodeMetrics() error {
	const total = 80 << 30
	var batch []telemetry.Sample
	for ni, n := range nodeSpecs {
		for _, at := range t.grid() {
			frac := 0.42 + 0.05*float64(ni)
			if n.id == "node_worker_sgp_1" {
				days := t.anchor.Sub(at).Hours() / 24
				frac = 0.82 - 0.04*days
				if frac < 0.5 {
					frac = 0.5
				}
			}
			batch = append(batch,
				telemetry.Sample{ResourceID: "node:" + n.id, Metric: telemetry.MetricDiskTotalBytes, Timestamp: at, Value: total},
				telemetry.Sample{ResourceID: "node:" + n.id, Metric: telemetry.MetricDiskUsedBytes, Timestamp: at, Value: frac * total},
			)
		}
	}
	for ni, n := range nodeSpecs {
		id := "node:" + n.id
		for h := 0; h < 6; h++ {
			at := t.anchor.Add(-time.Duration(h) * time.Hour)
			batch = append(batch,
				telemetry.Sample{ResourceID: id, Metric: telemetry.MetricOSPatchesAvailable, Timestamp: at, Value: float64(3 + ni*2)},
				telemetry.Sample{ResourceID: id, Metric: telemetry.MetricOSSecurityPatchesAvailable, Timestamp: at, Value: float64(ni % 3)},
			)
		}
	}
	return t.tdb.WriteSamples(t.ctx, batch)
}

// markers derives deploy, build and restart samples from the seeded deploy rows.
func (t *telemetrySeeder) markers() error {
	for _, a := range appSpecs {
		attempts, err := t.db.ListDeployAttempts(t.ctx, a.name)
		if err != nil {
			return fmt.Errorf("demo: list attempts for %q: %w", a.name, err)
		}
		for _, d := range attempts {
			if d.Status != store.DeployAttemptStatusSucceeded {
				continue
			}
			if err := t.tdb.RecordDeploy(t.ctx, a.name, d.StartedAt.Add(60*time.Second)); err != nil {
				return err
			}
			dur := 45*time.Second + time.Duration(t.rng.intn(120))*time.Second
			if err := t.tdb.RecordBuildDuration(t.ctx, a.name, dur, d.StartedAt); err != nil {
				return err
			}
		}
	}
	for k := 0; k < 4 && !t.story.calm; k++ {
		at := t.story.crashStart.Add(time.Duration(k) * 3 * time.Minute)
		if err := t.tdb.RecordContainerRestart(t.ctx, crashloopApp, at); err != nil {
			return err
		}
	}
	leak := t.anchor.AddDate(0, 0, -5)
	for k := 0; k < 3; k++ {
		if err := t.tdb.RecordContainerRestart(t.ctx, incidentApp, leak.Add(time.Duration(k)*8*time.Hour)); err != nil {
			return err
		}
	}
	return nil
}

var plainSamples = []string{
	"worker heartbeat ok",
	"cache refresh completed in 38ms",
	"scheduled job tick: reconcile-inventory",
	"connection pool at 40 percent",
	"GET /healthz 200 1ms",
	"flushed 128 buffered events",
}

func (t *telemetrySeeder) logs() error {
	var batch []telemetry.LogEntry
	for _, a := range appSpecs {
		if a.suspended {
			continue
		}
		cid := "demo" + a.name
		for n := 0; n < logLinesPerApp; n++ {
			at := t.anchor.Add(-time.Duration(t.rng.intn(int(logHistory/time.Second))) * time.Second)
			line := t.normalLine(a.name, cid, at)
			if a.name == crashloopApp && !t.story.calm && !at.Before(t.story.crashStart) {
				continue
			}
			batch = append(batch, line)
		}
		if a.name == incidentApp {
			batch = append(batch, t.slowLines(cid)...)
		}
		if a.name == crashloopApp && !t.story.calm {
			batch = append(batch, t.crashLines()...)
		}
	}
	sort.SliceStable(batch, func(i, j int) bool { return batch[i].Timestamp.Before(batch[j].Timestamp) })
	for start := 0; start < len(batch); start += 500 {
		end := min(start+500, len(batch))
		if err := t.tdb.WriteLogBatch(t.ctx, batch[start:end]); err != nil {
			return fmt.Errorf("demo: write logs: %w", err)
		}
	}
	return nil
}

func (t *telemetrySeeder) normalLine(app, cid string, at time.Time) telemetry.LogEntry {
	if t.rng.intn(10) < 7 {
		status := 200
		level := "info"
		if t.rng.intn(40) == 0 {
			status, level = 500, "error"
		}
		msg := fmt.Sprintf(`{"level":%q,"route":"/api/v1/items","status":%d,"ms":%d,"request_id":"req_%s"}`, level, status, 4+t.rng.intn(120), t.rng.hex(8))
		stream := "stdout"
		if status == 500 {
			stream = "stderr"
		}
		return telemetry.LogEntry{ResourceID: "service:" + app, ContainerID: cid, Stream: stream, Timestamp: at, Message: msg, Structured: true, FieldsJSON: msg}
	}
	return telemetry.LogEntry{ResourceID: "service:" + app, ContainerID: cid, Stream: "stdout", Timestamp: at, Message: plainSamples[t.rng.intn(len(plainSamples))]}
}

// slowLines is the error cluster aligned to the slow deploy window.
func (t *telemetrySeeder) slowLines(cid string) []telemetry.LogEntry {
	var out []telemetry.LogEntry
	span := t.story.slowRecovered.Sub(t.story.slowDeploy)
	for n := 0; n < 60; n++ {
		at := t.story.slowDeploy.Add(span * time.Duration(n) / 60)
		msg := fmt.Sprintf(`{"level":"warn","msg":"catalog cache miss storm","route":"/api/v1/catalog","status":200,"ms":%d,"request_id":"req_%s"}`, 400+t.rng.intn(900), t.rng.hex(8))
		stream := "stdout"
		if n%6 == 0 {
			msg = fmt.Sprintf(`{"level":"error","msg":"upstream timeout","route":"/api/v1/catalog","status":500,"ms":%d,"request_id":"req_%s"}`, 2000+t.rng.intn(500), t.rng.hex(8))
			stream = "stderr"
		}
		out = append(out, telemetry.LogEntry{ResourceID: "service:" + incidentApp, ContainerID: cid, Stream: stream, Timestamp: at, Message: msg, Structured: true, FieldsJSON: msg})
	}
	return out
}

// crashLines emits restart cycles of start, warm up, memory pressure and OOM
// kill, enough that the last 200 lines are all crashloop output.
func (t *telemetrySeeder) crashLines() []telemetry.LogEntry {
	var out []telemetry.LogEntry
	span := t.story.crashEnd.Sub(t.story.crashStart)
	cycle := []string{
		"billing-service starting, config loaded",
		"connected to postgres pool=10",
		"invoice batch job started, batch_size=5000",
		"loading customer ledger into memory",
		"heap in use 412MB of 512MB limit",
		"heap in use 498MB of 512MB limit",
		"fatal: runtime out of memory, allocation of 64MB failed",
	}
	for n := 0; n < crashLogLines; n++ {
		at := t.story.crashStart.Add(span * time.Duration(n) / crashLogLines)
		run := n / len(cycle)
		line := cycle[n%len(cycle)]
		stream := "stdout"
		if n%len(cycle) >= 5 {
			stream = "stderr"
		}
		out = append(out, telemetry.LogEntry{
			ResourceID: "service:" + crashloopApp, ContainerID: fmt.Sprintf("demo%s-r%d", crashloopApp, run),
			Stream: stream, Timestamp: at, Message: line,
		})
	}
	return out
}

// deployLogs writes a short build log per seeded attempt, with a failing
// tail for failed builds.
func (t *telemetrySeeder) deployLogs() error {
	steps := []string{
		"#1 [internal] load build definition from Dockerfile",
		"#2 [internal] load metadata for docker.io/library/node:20-slim",
		"#3 [1/5] FROM docker.io/library/node:20-slim",
		"#4 [2/5] WORKDIR /app",
		"#5 [3/5] COPY package*.json ./",
		"#6 [4/5] RUN npm ci --omit=dev",
		"#7 [5/5] COPY . .",
		"#8 exporting to image",
	}
	for _, a := range appSpecs {
		attempts, err := t.db.ListDeployAttempts(t.ctx, a.name)
		if err != nil {
			return err
		}
		for _, d := range attempts {
			if d.Source == store.DeployAttemptSourceAutoRollback || d.Source == store.DeployAttemptSourceImage {
				continue
			}
			var entries []telemetry.DeployLogEntry
			for n, line := range steps {
				entries = append(entries, telemetry.DeployLogEntry{AttemptID: d.ID, Stream: "stdout", Timestamp: d.StartedAt.Add(time.Duration(n) * 4 * time.Second), Message: line})
			}
			if d.Status == store.DeployAttemptStatusFailed {
				entries = append(entries, telemetry.DeployLogEntry{AttemptID: d.ID, Stream: "stderr", Timestamp: d.StartedAt.Add(40 * time.Second), Message: d.Error})
			} else {
				entries = append(entries, telemetry.DeployLogEntry{AttemptID: d.ID, Stream: "stdout", Timestamp: d.StartedAt.Add(40 * time.Second), Message: "deploy complete, container healthy"})
			}
			if err := t.tdb.WriteDeployLogBatch(t.ctx, entries); err != nil {
				return err
			}
		}
	}
	return nil
}
