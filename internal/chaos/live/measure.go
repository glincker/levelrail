package live

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/bench"
	"github.com/GLINCKER/levelrail/internal/chaos"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	deployRuns       = 6
	rolloutRate      = 100
	concurrencyStep  = 8 * time.Second
	failoverRate     = 100
	failoverDuration = 30 * time.Second
)

var concurrencies = []int{1, 8, 32, 128}

// deployLatency redeploys alternating image tags and times Deploy to the first
// request answered by the new version through the ingress. The first run is a
// cold start and is reported apart from the warm p50 and p95.
func deployLatency(ctx context.Context, res *chaos.Result) error {
	s, err := NewStack(ctx, false)
	if err != nil {
		return err
	}
	defer s.Close()

	var cold time.Duration
	var warm []time.Duration
	for i := 0; i < deployRuns; i++ {
		v := fmt.Sprintf("r%d", i+1)
		if err := s.cli.ImageTag(ctx, BaseImage, s.repo()+":"+v); err != nil {
			return fmt.Errorf("tag %s: %w", v, err)
		}
		t0 := time.Now()
		if err := s.Ship(ctx, v, "ok", 0); err != nil {
			return err
		}
		if err := s.WaitServing(ctx, v, settleBudget); err != nil {
			return fmt.Errorf("run %d: %s never served: %w", i+1, v, err)
		}
		d := time.Since(t0)
		var attempt store.DeployAttempt
		_ = poll(ctx, settleBudget, func() bool {
			attempt = s.Attempt(ctx, v)
			return attempt.Status == store.DeployAttemptStatusSucceeded || attempt.Status == store.DeployAttemptStatusFailed
		})
		gate := time.Since(t0)
		res.Assert(fmt.Sprintf("run %d (%s)", i+1, v), attempt.Status == store.DeployAttemptStatusSucceeded,
			fmt.Sprintf("first answer from new version %s, attempt %s after %s", d.Round(time.Millisecond), attempt.Status, gate.Round(time.Millisecond)))
		if i == 0 {
			cold = d
			continue
		}
		warm = append(warm, d)
	}
	sort.Slice(warm, func(i, j int) bool { return warm[i] < warm[j] })
	p50, p95 := bench.Percentile(warm, 50), bench.Percentile(warm, 95)
	res.Assert("every deploy reached serving", len(warm) == deployRuns-1, fmt.Sprintf("%d of %d", len(warm)+1, deployRuns))
	res.Assert("cold deploy", true, cold.Round(time.Millisecond).String())
	res.Assert("warm deploys", true, fmt.Sprintf("n=%d p50 %s p95 %s min %s max %s", len(warm),
		p50.Round(time.Millisecond), p95.Round(time.Millisecond), warm[0].Round(time.Millisecond), warm[len(warm)-1].Round(time.Millisecond)))
	res.Summary = fmt.Sprintf("image to serving: cold %.0f ms, warm p50 %.0f ms, p95 %.0f ms over %d redeploys (includes the 3 s stability gate)",
		ms(cold), ms(p50), ms(p95), len(warm))
	return nil
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// rolloutUnderTraffic ships v2 over a two replica v1 while an open-loop
// generator runs, and counts failed requests.
func rolloutUnderTraffic(ctx context.Context, res *chaos.Result) error {
	s, err := NewStack(ctx, false)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.ShipReplicas(ctx, "v1", "ok", 0, 2); err != nil {
		return err
	}
	if err := s.WaitServing(ctx, "v1", settleBudget); err != nil {
		return fmt.Errorf("v1 never served: %w", err)
	}
	if err := poll(ctx, settleBudget, func() bool { return len(s.Running(ctx)) == 2 }); err != nil {
		return fmt.Errorf("two replicas never ran: %w", err)
	}

	ev := &bench.Event{Name: "rollout", At: 5 * time.Second, Fire: func(c context.Context) error {
		return s.ShipReplicas(c, "v2", "ok", 0, 2)
	}}
	sampleCtx, stopSampling := context.WithCancel(ctx)
	seen := make(chan map[string]int, 1)
	go func() {
		counts := map[string]int{}
		c := s.Client()
		for sampleCtx.Err() == nil {
			if body, st, err := get(sampleCtx, c, s.URL()); err == nil && st == http.StatusOK {
				counts[body]++
			}
			time.Sleep(20 * time.Millisecond)
		}
		seen <- counts
	}()
	r, err := bench.Run(ctx, s.Client(), bench.Config{URL: s.URL(), Rate: rolloutRate, Duration: 40 * time.Second, Timeout: 3 * time.Second}, ev)
	stopSampling()
	counts := <-seen
	if err != nil {
		return err
	}
	tally(res, r)
	res.Assert("both versions answered during the run, so traffic overlapped the swap", counts["ok-v1"] > 0 && counts["ok-v2"] > 0,
		fmt.Sprintf("probe answers: v1 %d, v2 %d", counts["ok-v1"], counts["ok-v2"]))
	res.Assert("deploy was submitted", r.Event != nil && r.Event.FireError == "", fireErr(r))
	attempt := s.Attempt(ctx, "v2")
	_ = poll(ctx, settleBudget, func() bool {
		attempt = s.Attempt(ctx, "v2")
		return attempt.Status == store.DeployAttemptStatusSucceeded || attempt.Status == store.DeployAttemptStatusFailed
	})
	res.Assert("the rollout succeeded", attempt.Status == store.DeployAttemptStatusSucceeded, "status "+attempt.Status)
	res.Assert("the ingress serves v2 after the rollout", s.Serving(ctx) == "v2", "serving "+s.Serving(ctx))
	res.Assert("zero failed requests during the rollout", r.Stats.Failed == 0, fmt.Sprintf("%d of %d failed, p99 %.1f ms, max %.1f ms",
		r.Stats.Failed, r.Stats.Sent, r.Stats.P99MS, r.Stats.MaxMS))
	res.Summary = fmt.Sprintf("%d of %d requests failed at %d req/s across a two replica rollout, p50 %.2f ms, p95 %.2f ms, p99 %.2f ms",
		r.Stats.Failed, r.Stats.Sent, rolloutRate, r.Stats.P50MS, r.Stats.P95MS, r.Stats.P99MS)
	return nil
}

// ingressConcurrency drives the ingress with closed-loop workers at several
// concurrencies; each step is recorded as an assertion detail.
func ingressConcurrency(ctx context.Context, res *chaos.Result) error {
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
	var parts []string
	for _, n := range concurrencies {
		st, err := bench.RunClosed(ctx, s.ClientIdle(n), s.URL(), n, concurrencyStep, 3*time.Second)
		if err != nil {
			return err
		}
		res.Requests += st.Sent
		res.RequestErrors += st.Failed
		res.Assert(fmt.Sprintf("concurrency %d", n), n > concurrencies[0] || st.Failed == 0, fmt.Sprintf(
			"%.0f req/s, errors %d of %d, p50 %.2f ms, p95 %.2f ms, p99 %.2f ms, max %.1f ms",
			st.AchievedRPS, st.Failed, st.Sent, st.P50MS, st.P95MS, st.P99MS, st.MaxMS))
		parts = append(parts, fmt.Sprintf("c%d %.0f req/s", n, st.AchievedRPS))
	}
	res.Summary = "closed loop, one replica, " + concurrencyStep.String() + " per step: " + strings.Join(parts, ", ")
	return nil
}

// replicaFailover kills one of two replicas under traffic and times both the
// request recovery and the return to two running containers.
func replicaFailover(ctx context.Context, res *chaos.Result) error {
	s, err := NewStack(ctx, false)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.ShipReplicas(ctx, "v1", "ok", 0, 2); err != nil {
		return err
	}
	if err := s.WaitServing(ctx, "v1", settleBudget); err != nil {
		return fmt.Errorf("v1 never served: %w", err)
	}
	if err := poll(ctx, settleBudget, func() bool { return len(s.Running(ctx)) == 2 }); err != nil {
		return fmt.Errorf("two replicas never ran: %w", err)
	}
	victim := s.Running(ctx)[0].ID

	var killedAt time.Time
	restored := make(chan time.Duration, 1)
	ev := &bench.Event{Name: "kill-replica", At: 5 * time.Second, Fire: func(c context.Context) error {
		killedAt = time.Now()
		if err := s.cli.ContainerKill(c, victim, "KILL"); err != nil {
			return err
		}
		go func() {
			if poll(ctx, 5*time.Second, func() bool { return len(s.Running(ctx)) < 2 }) != nil {
				restored <- -1
				return
			}
			if poll(ctx, 20*time.Second, func() bool { return len(s.Running(ctx)) == 2 }) != nil {
				restored <- -1
				return
			}
			restored <- time.Since(killedAt)
		}()
		return nil
	}}
	r, err := bench.Run(ctx, s.Client(), bench.Config{URL: s.URL(), Rate: failoverRate, Duration: failoverDuration, Timeout: 3 * time.Second}, ev)
	if err != nil {
		return err
	}
	tally(res, r)
	if r.Event != nil {
		res.RecoveryMS = r.Event.RecoveryMS
	}
	var back time.Duration
	select {
	case back = <-restored:
	case <-time.After(25 * time.Second):
		back = -1
	}
	res.Assert("kill was delivered", r.Event != nil && r.Event.FireError == "", fireErr(r))
	res.Assert("requests recovered before the run ended", r.Event != nil && r.Event.Recovered, "")
	res.Assert("two containers running again after the kill", back > 0, fmt.Sprintf("replacement visible %.0f ms after the kill", ms(back)))
	res.Summary = fmt.Sprintf("%d of %d requests failed at %d req/s, error window %.0f ms, second replica back %.0f ms after the kill (container list poll, 200 ms resolution)",
		res.RequestErrors, res.Requests, failoverRate, res.ErrorWindowMS, ms(back))
	return nil
}
