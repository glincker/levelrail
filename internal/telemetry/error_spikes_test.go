package telemetry

import (
	"testing"
	"time"
)

func spikePts(base time.Time, reqs int, rates ...float64) []RequestPoint {
	pts := make([]RequestPoint, len(rates))
	for i, r := range rates {
		pts[i] = RequestPoint{Timestamp: base.Add(time.Duration(i) * time.Minute), Requests: float64(reqs), ErrorRate5xx: r}
	}
	return pts
}

func testSpikeCfg() SpikeConfig {
	return SpikeConfig{MinRate: 0.05, Factor: 3, MinRequests: 20, Attribution: 10 * time.Minute, Max: 5}
}

func TestDetectErrorSpikesGroupsRunsAndAttributesDeploy(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	step := time.Minute
	pts := spikePts(base, 100, 0, 0.01, 0.4, 0.5, 0, 0, 0.3, 0)
	deploys := []DeployMarker{
		{ID: "old", At: base.Add(-time.Hour)},
		{ID: "d1", At: base.Add(90 * time.Second), Status: "succeeded", Image: "app:2"},
	}
	got := DetectErrorSpikes(pts, step, deploys, testSpikeCfg())
	if len(got) != 2 {
		t.Fatalf("want 2 spikes, got %d: %+v", len(got), got)
	}
	if !got[0].Start.Equal(base.Add(6*step)) || got[1].PeakRate5xx != 0.5 {
		t.Fatalf("unexpected order or peak: %+v", got)
	}
	if got[1].DeployID != "d1" || got[1].DeployImage != "app:2" || got[1].DeployAt == nil {
		t.Fatalf("spike not tied to deploy: %+v", got[1])
	}
	if got[1].Requests != 200 || got[1].Errors5xx != 90 {
		t.Fatalf("totals wrong: %+v", got[1])
	}
}

func TestDetectErrorSpikesIgnoresLowTrafficAndHighBaseline(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Minute)
	low := spikePts(base, 5, 0, 1, 0)
	if got := DetectErrorSpikes(low, time.Minute, nil, testSpikeCfg()); len(got) != 0 {
		t.Fatalf("low traffic flagged: %+v", got)
	}
	noisy := spikePts(base, 100, 0.2, 0.2, 0.25, 0.2)
	if got := DetectErrorSpikes(noisy, time.Minute, nil, testSpikeCfg()); len(got) != 0 {
		t.Fatalf("steady noise flagged: %+v", got)
	}
}

func TestDetectErrorSpikesDeployTooOldOrAfter(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	pts := spikePts(base, 100, 0, 0, 0.5, 0)
	deploys := []DeployMarker{{ID: "old", At: base.Add(-time.Hour)}, {ID: "later", At: base.Add(30 * time.Minute)}}
	got := DetectErrorSpikes(pts, time.Minute, deploys, testSpikeCfg())
	if len(got) != 1 || got[0].DeployID != "" || got[0].DeployAt != nil {
		t.Fatalf("unexpected attribution: %+v", got)
	}
}

func TestDetectErrorSpikesCapsResults(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	rates := []float64{}
	for i := 0; i < 20; i++ {
		rates = append(rates, 0, 0, 0, 0.5)
	}
	cfg := testSpikeCfg()
	cfg.Max = 3
	got := DetectErrorSpikes(spikePts(base, 100, rates...), time.Minute, nil, cfg)
	if len(got) != 3 || !got[0].Start.After(got[2].Start) {
		t.Fatalf("cap or order wrong: %d", len(got))
	}
}

func TestSpikeConfigFromEnv(t *testing.T) {
	t.Setenv(EnvSpikeMinRate, "0.2")
	t.Setenv(EnvSpikeFactor, "bad")
	t.Setenv(EnvSpikeAttribution, "5m")
	t.Setenv(EnvSpikeMax, "2")
	c := SpikeConfigFromEnv()
	if c.MinRate != 0.2 || c.Factor != defaultSpikeFactor || c.Attribution != 5*time.Minute || c.Max != 2 {
		t.Fatalf("cfg: %+v", c)
	}
}
