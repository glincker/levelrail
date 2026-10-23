package live

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/bench"
	"github.com/GLINCKER/levelrail/internal/chaos"
)

const (
	steadyStepDuration = 10 * time.Second
	steadyMaxErrorRate = 0.01
)

var steadyRates = []float64{100, 400, 800, 1600, 3200, 6400, 12800}

// ingressSteady drives the embedded ingress at rising constant rates and
// records each step; the highest step with under 1 percent errors and no
// client saturation is the reported ceiling for this machine.
func ingressSteady(ctx context.Context, res *chaos.Result) error {
	s, err := NewStack(ctx, false)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.Ship(ctx, "v1", "ok", 0); err != nil {
		return err
	}
	if err := s.WaitServing(ctx, "v1", settleBudget); err != nil {
		return fmt.Errorf("v1 never served: %w", err)
	}
	var ceiling float64
	var firstClean bool
	for i, rate := range steadyRates {
		r, err := bench.Run(ctx, s.Client(), bench.Config{URL: s.URL(), Rate: rate, Duration: steadyStepDuration, Timeout: 3 * time.Second}, nil)
		if err != nil {
			return err
		}
		st := r.Stats
		clean := st.ErrorRate < steadyMaxErrorRate && st.Saturated == 0
		if clean {
			ceiling = rate
		}
		if i == 0 {
			firstClean = clean
		}
		res.Requests += st.Sent
		res.RequestErrors += st.Failed
		key := fmt.Sprintf("step%.0f_", rate)
		res.Set(key+"rps", st.AchievedRPS)
		res.Set(key+"p50_ms", st.P50MS)
		res.Set(key+"p95_ms", st.P95MS)
		res.Set(key+"p99_ms", st.P99MS)
		res.Set(key+"errors", float64(st.Failed))
		res.Assert(fmt.Sprintf("step %.0f req/s", rate), true, fmt.Sprintf(
			"achieved %.0f req/s, errors %d of %d, p50 %.2f ms, p95 %.2f ms, p99 %.2f ms, max %.1f ms, client saturated %d, clean %t",
			st.AchievedRPS, st.Failed, st.Sent, st.P50MS, st.P95MS, st.P99MS, st.MaxMS, st.Saturated, clean))
		if !clean {
			break
		}
	}
	res.Set("highest_clean_rps", ceiling)
	res.Assert("the lowest step ran clean", firstClean, "")
	res.Summary = fmt.Sprintf("highest clean step %.0f req/s through the embedded ingress, one replica, %s per step", ceiling, steadyStepDuration)
	return nil
}
