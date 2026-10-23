package telemetry

import (
	"os"
	"sort"
	"strconv"
	"time"
)

// Env vars that tune error spike detection; unset or invalid values fall back to defaults.
const (
	EnvSpikeMinRate     = "APP_TRAFFIC_SPIKE_MIN_RATE"
	EnvSpikeFactor      = "APP_TRAFFIC_SPIKE_FACTOR"
	EnvSpikeMinRequests = "APP_TRAFFIC_SPIKE_MIN_REQUESTS"
	EnvSpikeAttribution = "APP_TRAFFIC_SPIKE_ATTRIBUTION"
	EnvSpikeMax         = "APP_TRAFFIC_SPIKE_MAX"
	EnvSpikeBuckets     = "APP_TRAFFIC_SPIKE_BUCKETS"

	defaultSpikeMinRate     = 0.05
	defaultSpikeFactor      = 3.0
	defaultSpikeMinRequests = 20
	defaultSpikeAttribution = 15 * time.Minute
	defaultSpikeMax         = 10
	defaultSpikeBuckets     = 60
)

// SpikeConfig tunes error spike detection. MinRate is the 5xx share a bucket
// must reach, Factor how many times the window median it must exceed, and
// MinRequests the traffic a bucket needs so a lone failure is not a spike.
type SpikeConfig struct {
	MinRate     float64
	Factor      float64
	MinRequests float64
	Attribution time.Duration
	Max         int
	Buckets     int
}

// SpikeConfigFromEnv reads the detection thresholds from the environment.
func SpikeConfigFromEnv() SpikeConfig {
	cfg := SpikeConfig{
		MinRate: defaultSpikeMinRate, Factor: defaultSpikeFactor, MinRequests: defaultSpikeMinRequests,
		Attribution: defaultSpikeAttribution, Max: defaultSpikeMax, Buckets: defaultSpikeBuckets,
	}
	if v, err := strconv.ParseFloat(os.Getenv(EnvSpikeMinRate), 64); err == nil && v > 0 && v <= 1 {
		cfg.MinRate = v
	}
	if v, err := strconv.ParseFloat(os.Getenv(EnvSpikeFactor), 64); err == nil && v >= 1 {
		cfg.Factor = v
	}
	if v, err := strconv.Atoi(os.Getenv(EnvSpikeMinRequests)); err == nil && v > 0 {
		cfg.MinRequests = float64(v)
	}
	if v, err := time.ParseDuration(os.Getenv(EnvSpikeAttribution)); err == nil && v > 0 {
		cfg.Attribution = v
	}
	if v, err := strconv.Atoi(os.Getenv(EnvSpikeMax)); err == nil && v > 0 {
		cfg.Max = v
	}
	if v, err := strconv.Atoi(os.Getenv(EnvSpikeBuckets)); err == nil && v >= 2 {
		cfg.Buckets = v
	}
	return cfg
}

// DeployMarker is a deploy the detector may blame a spike on.
type DeployMarker struct {
	ID     string
	At     time.Time
	Status string
	Image  string
	Commit string
}

// ErrorSpike is a run of consecutive buckets whose 5xx share stood out.
type ErrorSpike struct {
	Start        time.Time  `json:"start"`
	End          time.Time  `json:"end"`
	Requests     float64    `json:"requests"`
	Errors5xx    float64    `json:"errors_5xx"`
	PeakRate5xx  float64    `json:"peak_error_rate_5xx"`
	BaselineRate float64    `json:"baseline_error_rate_5xx"`
	DeployID     string     `json:"deploy_id,omitempty"`
	DeployAt     *time.Time `json:"deploy_at,omitempty"`
	DeployStatus string     `json:"deploy_status,omitempty"`
	DeployImage  string     `json:"deploy_image,omitempty"`
	DeployCommit string     `json:"deploy_commit,omitempty"`
}

// DetectErrorSpikes groups buckets of points (each step wide) whose 5xx share
// is at least cfg.MinRate, cfg.Factor times the window median and backed by
// cfg.MinRequests, then ties each spike to the latest deploy that started
// within cfg.Attribution before it. Newest spikes come first, capped at cfg.Max.
func DetectErrorSpikes(points []RequestPoint, step time.Duration, deploys []DeployMarker, cfg SpikeConfig) []ErrorSpike {
	out := []ErrorSpike{}
	if len(points) == 0 || step <= 0 {
		return out
	}
	rates := make([]float64, len(points))
	for i, p := range points {
		rates[i] = p.ErrorRate5xx
	}
	sort.Float64s(rates)
	baseline := rates[len(rates)/2]

	var cur *ErrorSpike
	flush := func() {
		if cur != nil {
			attachDeploy(cur, deploys, cfg.Attribution)
			out = append(out, *cur)
			cur = nil
		}
	}
	for _, p := range points {
		hot := p.Requests >= cfg.MinRequests && p.ErrorRate5xx >= cfg.MinRate && p.ErrorRate5xx >= cfg.Factor*baseline
		if !hot {
			flush()
			continue
		}
		if cur != nil && !p.Timestamp.Equal(cur.End) {
			flush()
		}
		if cur == nil {
			cur = &ErrorSpike{Start: p.Timestamp, BaselineRate: baseline}
		}
		cur.End = p.Timestamp.Add(step)
		cur.Requests += p.Requests
		cur.Errors5xx += p.Requests * p.ErrorRate5xx
		if p.ErrorRate5xx > cur.PeakRate5xx {
			cur.PeakRate5xx = p.ErrorRate5xx
		}
	}
	flush()
	sort.Slice(out, func(i, j int) bool { return out[i].Start.After(out[j].Start) })
	if cfg.Max > 0 && len(out) > cfg.Max {
		out = out[:cfg.Max]
	}
	return out
}

func attachDeploy(s *ErrorSpike, deploys []DeployMarker, within time.Duration) {
	var best *DeployMarker
	for i := range deploys {
		d := &deploys[i]
		if d.At.After(s.End) || s.Start.Sub(d.At) > within {
			continue
		}
		if best == nil || d.At.After(best.At) {
			best = d
		}
	}
	if best == nil {
		return
	}
	at := best.At
	s.DeployID, s.DeployAt, s.DeployStatus, s.DeployImage, s.DeployCommit = best.ID, &at, best.Status, best.Image, best.Commit
}
