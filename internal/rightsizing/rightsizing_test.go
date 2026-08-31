package rightsizing

import (
	"testing"
	"time"
)

func memSamples(now time.Time, n int, bytesPerSample func(i int) float64, spread time.Duration) []Sample {
	out := make([]Sample, n)
	for i := 0; i < n; i++ {
		out[i] = Sample{
			Timestamp: now.Add(-spread + time.Duration(i)*(spread/time.Duration(n))),
			Value:     bytesPerSample(i),
		}
	}
	return out
}

func TestRecommend_InsufficientData_NewApp(t *testing.T) {
	now := time.Now()
	in := Input{
		ServiceName:        "web",
		Now:                now,
		LookbackWindow:     7 * 24 * time.Hour,
		MemorySamples:      memSamples(now, 5, func(_ int) float64 { return 100 * mib }, time.Hour),
		CPUPercentSamples:  memSamples(now, 5, func(_ int) float64 { return 10 }, time.Hour),
		CurrentMemoryBytes: 512 * mib,
	}

	got := Recommend(in)

	if got.Memory.DataSufficient {
		t.Errorf("Memory.DataSufficient = true, want false for only 5 samples spanning 1h")
	}
	if got.Memory.Confidence != ConfidenceLow {
		t.Errorf("Memory.Confidence = %q, want %q", got.Memory.Confidence, ConfidenceLow)
	}
	if got.Memory.Action != "" {
		t.Errorf("Memory.Action = %q, want empty (insufficient data must not suggest a change)", got.Memory.Action)
	}
	if got.Memory.Reason == "" {
		t.Error("Memory.Reason is empty, want an explanation of why data is insufficient")
	}
}

func TestRecommend_NoSamplesAtAll(t *testing.T) {
	now := time.Now()
	got := Recommend(Input{ServiceName: "web", Now: now, LookbackWindow: 7 * 24 * time.Hour, CurrentMemoryBytes: 512 * mib})

	if got.Memory.SampleCount != 0 {
		t.Errorf("SampleCount = %d, want 0", got.Memory.SampleCount)
	}
	if got.Memory.DataSufficient {
		t.Error("DataSufficient = true, want false with zero samples")
	}
	if got.Memory.Action != "" {
		t.Errorf("Action = %q, want empty", got.Memory.Action)
	}
}

func TestRecommend_UnderLimit_SuggestsLower(t *testing.T) {
	now := time.Now()
	// p95 usage well below the current limit, plenty of data.
	in := Input{
		ServiceName:        "web",
		Now:                now,
		LookbackWindow:     7 * 24 * time.Hour,
		MemorySamples:      memSamples(now, 200, func(_ int) float64 { return 100 * mib }, 7*24*time.Hour),
		CurrentMemoryBytes: 1024 * mib,
	}

	got := Recommend(in)

	if !got.Memory.DataSufficient {
		t.Fatalf("DataSufficient = false, want true (200 samples over 7 days)")
	}
	if got.Memory.Confidence != ConfidenceHigh {
		t.Errorf("Confidence = %q, want %q", got.Memory.Confidence, ConfidenceHigh)
	}
	if got.Memory.Action != ActionLower {
		t.Errorf("Action = %q, want %q (p95 100MiB is ~10%% of a 1024MiB limit)", got.Memory.Action, ActionLower)
	}
	if got.Memory.SuggestedLimit >= in.CurrentMemoryBytes {
		t.Errorf("SuggestedLimit = %d, want less than current limit %d", got.Memory.SuggestedLimit, in.CurrentMemoryBytes)
	}
}

func TestRecommend_OverLimit_SuggestsRaise(t *testing.T) {
	now := time.Now()
	in := Input{
		ServiceName:        "web",
		Now:                now,
		LookbackWindow:     7 * 24 * time.Hour,
		MemorySamples:      memSamples(now, 200, func(_ int) float64 { return 480 * mib }, 7*24*time.Hour),
		CurrentMemoryBytes: 512 * mib,
	}

	got := Recommend(in)

	if got.Memory.Action != ActionRaise {
		t.Errorf("Action = %q, want %q (p95 480MiB is ~94%% of a 512MiB limit)", got.Memory.Action, ActionRaise)
	}
	if got.Memory.SuggestedLimit <= in.CurrentMemoryBytes {
		t.Errorf("SuggestedLimit = %d, want more than current limit %d", got.Memory.SuggestedLimit, in.CurrentMemoryBytes)
	}
}

func TestRecommend_WithinBand_SuggestsKeep(t *testing.T) {
	now := time.Now()
	in := Input{
		ServiceName:        "web",
		Now:                now,
		LookbackWindow:     7 * 24 * time.Hour,
		MemorySamples:      memSamples(now, 200, func(_ int) float64 { return 300 * mib }, 7*24*time.Hour),
		CurrentMemoryBytes: 512 * mib,
	}

	got := Recommend(in)

	if got.Memory.Action != ActionKeep {
		t.Errorf("Action = %q, want %q (p95 300MiB is ~59%% of a 512MiB limit, inside the keep band)", got.Memory.Action, ActionKeep)
	}
}

func TestRecommend_NoLimitConfigured(t *testing.T) {
	now := time.Now()
	in := Input{
		ServiceName:        "web",
		Now:                now,
		LookbackWindow:     7 * 24 * time.Hour,
		MemorySamples:      memSamples(now, 200, func(_ int) float64 { return 300 * mib }, 7*24*time.Hour),
		CurrentMemoryBytes: 0,
	}

	got := Recommend(in)

	if got.Memory.Action != "" {
		t.Errorf("Action = %q, want empty when no limit is currently set", got.Memory.Action)
	}
	if got.Memory.SuggestedLimit <= 0 {
		t.Error("SuggestedLimit should still be computed even with no current limit")
	}
}

func TestRecommend_OOMSignal_OverridesToRaise(t *testing.T) {
	now := time.Now()
	oomAt := now.Add(-2 * time.Hour)
	in := Input{
		ServiceName:    "web",
		Now:            now,
		LookbackWindow: 7 * 24 * time.Hour,
		// Usage looks comfortably within the limit, which is exactly the
		// scenario an OOM kill should override: the sampled average
		// understates the spike that actually caused the kill.
		MemorySamples:      memSamples(now, 200, func(_ int) float64 { return 200 * mib }, 7*24*time.Hour),
		CurrentMemoryBytes: 512 * mib,
		OOM:                &OOMEvidence{DetectedAt: oomAt, Excerpt: "container killed: oomkilled"},
	}

	got := Recommend(in)

	if got.Memory.Action != ActionRaise {
		t.Errorf("Action = %q, want %q when an OOM kill was observed", got.Memory.Action, ActionRaise)
	}
	if got.Memory.Confidence != ConfidenceHigh {
		t.Errorf("Confidence = %q, want %q for an OOM-backed recommendation", got.Memory.Confidence, ConfidenceHigh)
	}
	wantFloor := int64(float64(in.CurrentMemoryBytes) * oomLimitFloorMultiplier)
	if got.Memory.SuggestedLimit < wantFloor {
		t.Errorf("SuggestedLimit = %d, want at least %d (current limit * oomLimitFloorMultiplier)", got.Memory.SuggestedLimit, wantFloor)
	}
	if got.OOM == nil {
		t.Fatal("Result.OOM is nil, want the evidence to be echoed back")
	}
	if got.CPU.Action == ActionRaise && got.CPU.Confidence == ConfidenceHigh {
		// Not a hard requirement that CPU stays unaffected in every
		// respect, but an OOM signal must never itself force a CPU raise:
		// it's a memory-specific signal.
		t.Error("OOM evidence must not force the CPU dimension to raise on its own")
	}
}

func TestRecommend_OOMSignal_EvenWithInsufficientData(t *testing.T) {
	now := time.Now()
	oomAt := now.Add(-10 * time.Minute)
	in := Input{
		ServiceName:        "web",
		Now:                now,
		LookbackWindow:     7 * 24 * time.Hour,
		MemorySamples:      memSamples(now, 3, func(_ int) float64 { return 500 * mib }, 15*time.Minute),
		CurrentMemoryBytes: 512 * mib,
		OOM:                &OOMEvidence{DetectedAt: oomAt, Excerpt: "exit code 137"},
	}

	got := Recommend(in)

	if got.Memory.Action != ActionRaise {
		t.Errorf("Action = %q, want %q: a real OOM signal should override an otherwise-insufficient sample count", got.Memory.Action, ActionRaise)
	}
}

func TestRecommend_CPUConvertsPercentToNanoCPUs(t *testing.T) {
	now := time.Now()
	// 150% CPU (1.5 cores) sustained, current limit exactly 1 core.
	in := Input{
		ServiceName:       "web",
		Now:               now,
		LookbackWindow:    7 * 24 * time.Hour,
		CPUPercentSamples: memSamples(now, 200, func(_ int) float64 { return 150 }, 7*24*time.Hour),
		CurrentNanoCPUs:   1_000_000_000,
	}

	got := Recommend(in)

	if got.CPU.Action != ActionRaise {
		t.Errorf("CPU.Action = %q, want %q (150%% usage against a 1-core limit)", got.CPU.Action, ActionRaise)
	}
	if got.CPU.P95Usage < 1_400_000_000 || got.CPU.P95Usage > 1_600_000_000 {
		t.Errorf("CPU.P95Usage = %v, want approximately 1_500_000_000 nanoCPUs (150%% converted)", got.CPU.P95Usage)
	}
}

func TestPercentile_KnownValues(t *testing.T) {
	values := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	if got := percentile(values, 0.95); got != 100 {
		t.Errorf("percentile(0.95) = %v, want 100", got)
	}
	if got := percentile(values, 0.5); got != 50 {
		t.Errorf("percentile(0.5) = %v, want 50", got)
	}
	if got := percentile(nil, 0.95); got != 0 {
		t.Errorf("percentile(nil) = %v, want 0", got)
	}
}

func TestRoundToMiB(t *testing.T) {
	tests := []struct {
		in   float64
		want int64
	}{
		{0, 0},
		{-5, 0},
		{mib, mib},
		{mib * 1.4, mib},
		{mib * 1.6, 2 * mib},
	}
	for _, tt := range tests {
		if got := roundToMiB(tt.in); got != tt.want {
			t.Errorf("roundToMiB(%v) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
