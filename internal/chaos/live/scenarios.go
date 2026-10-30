package live

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/bench"
	"github.com/GLINCKER/levelrail/internal/chaos"
	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	loadRate     = 40
	oomMemory    = 48 * 1024 * 1024
	settleBudget = 90 * time.Second
)

// Run executes one scenario and always returns a result; an environment that cannot run it yields a skipped result.
func Run(ctx context.Context, name, hardwareNote string) (res chaos.Result) {
	res = chaos.Result{Scenario: name, RanAt: time.Now().UTC(), Hardware: Hardware(ctx, hardwareNote)}
	started := time.Now()
	defer func() { res.DurationMS = float64(time.Since(started)) / float64(time.Millisecond) }()

	if err := Available(ctx); err != nil {
		res.Status, res.Summary = chaos.StatusSkipped, err.Error()
		return res
	}
	fn := map[string]func(context.Context, *chaos.Result) error{
		chaos.KillContainer:      killContainer,
		chaos.DaemonUnreachable:  daemonUnreachable,
		chaos.ControlPlaneBounce: controlPlaneRestart,
		chaos.DiskPressure:       diskPressure,
		chaos.OOMDuringReadiness: oomDuringReadiness,
		chaos.IngressSteady:      ingressSteady,
		chaos.DeployLatency:      deployLatency,
		chaos.RolloutUnderLoad:   rolloutUnderTraffic,
		chaos.IngressConcurrency: ingressConcurrency,
		chaos.ReplicaFailover:    replicaFailover,
	}[name]
	if fn == nil {
		res.Status, res.Summary = chaos.StatusFailed, "unknown scenario "+name
		return res
	}
	if err := fn(ctx, &res); err != nil {
		res.Assert("scenario ran to completion", false, err.Error())
		if res.Summary == "" {
			res.Summary = err.Error()
		}
	}
	res.Finish()
	return res
}

// Hardware describes this machine and Docker, plus any caller note.
func Hardware(ctx context.Context, note string) string {
	parts := []string{fmt.Sprintf("%s/%s, %d CPUs", runtime.GOOS, runtime.GOARCH, runtime.NumCPU())}
	if cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation()); err == nil {
		defer func() { _ = cli.Close() }()
		if v, err := cli.ServerVersion(ctx); err == nil {
			parts = append(parts, "Docker "+v.Version)
		}
		if info, err := cli.Info(ctx); err == nil {
			parts = append(parts, fmt.Sprintf("%d GiB RAM visible to Docker", info.MemTotal>>30))
		}
	}
	if note != "" {
		parts = append(parts, note)
	}
	return strings.Join(parts, ", ")
}

func tally(res *chaos.Result, r bench.Result) {
	res.Requests, res.RequestErrors = r.Stats.Sent, r.Stats.Failed
	if r.Event != nil {
		res.ErrorWindowMS = r.Event.ErrorWindowMS
	}
	res.Set("p50_ms", r.Stats.P50MS)
	res.Set("p95_ms", r.Stats.P95MS)
	res.Set("p99_ms", r.Stats.P99MS)
	res.Set("max_ms", r.Stats.MaxMS)
	res.Set("achieved_rps", r.Stats.AchievedRPS)
	res.Set("client_saturated", float64(r.Stats.Saturated))
	res.Assert("load generator was not the bottleneck", r.Stats.Saturated == 0, fmt.Sprintf("%d requests shed by the client in-flight cap", r.Stats.Saturated))
}

func killContainer(ctx context.Context, res *chaos.Result) error {
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
	before := s.Running(ctx)
	if len(before) == 0 {
		return fmt.Errorf("no running container to kill")
	}
	oldID := before[0].ID

	ev := &bench.Event{Name: "kill-container", At: 5 * time.Second, Fire: func(c context.Context) error {
		return s.cli.ContainerKill(c, oldID, "KILL")
	}}
	r, err := bench.Run(ctx, s.Client(), bench.Config{URL: s.URL(), Rate: loadRate, Duration: 30 * time.Second, Timeout: 3 * time.Second}, ev)
	if err != nil {
		return err
	}
	tally(res, r)
	if r.Event != nil {
		res.RecoveryMS = r.Event.RecoveryMS
	}
	after := s.Running(ctx)
	res.Assert("kill was delivered", r.Event != nil && r.Event.FireError == "", fireErr(r))
	res.Assert("the kill was visible to clients", r.Event != nil && r.Event.Dropped > 0, "a single replica cannot hide a kill")
	res.Assert("requests recovered before the run ended", r.Event != nil && r.Event.Recovered, "")
	res.Assert("a container is running and routed again", len(after) > 0 && s.Serving(ctx) == "v1", fmt.Sprintf("%d running", len(after)))
	replacement := "restarted in place"
	if len(after) > 0 && after[0].ID != oldID {
		replacement = "replaced by a new container"
	}
	res.Summary = fmt.Sprintf("healthy again %.0f ms after the kill (%s), %d of %d requests failed", res.RecoveryMS, replacement, res.RequestErrors, res.Requests)
	return nil
}

func fireErr(r bench.Result) string {
	if r.Event == nil {
		return "no event"
	}
	return r.Event.FireError
}

func daemonUnreachable(ctx context.Context, res *chaos.Result) error {
	s, err := NewStack(ctx, true)
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
	const (
		fireAt  = 3 * time.Second
		downAt  = 2 * time.Second
		downFor = 6 * time.Second
	)
	ev := &bench.Event{Name: "daemon-unreachable", At: fireAt, Fire: func(c context.Context) error {
		if err := s.Ship(c, "v2", "ok", 0); err != nil {
			return err
		}
		time.Sleep(downAt)
		s.proxy.setDown(true)
		time.Sleep(downFor)
		s.proxy.setDown(false)
		return nil
	}}
	r, err := bench.Run(ctx, s.Client(), bench.Config{URL: s.URL(), Rate: loadRate, Duration: 30 * time.Second, Timeout: 3 * time.Second}, ev)
	if err != nil {
		return err
	}
	tally(res, r)
	res.Assert("deploy was submitted and the outage injected", r.Event != nil && r.Event.FireError == "", fireErr(r))

	var attempt store.DeployAttempt
	settleErr := poll(ctx, settleBudget, func() bool {
		attempt = s.Attempt(ctx, "v2")
		return attempt.Status == store.DeployAttemptStatusSucceeded || attempt.Status == store.DeployAttemptStatusFailed
	})
	res.Assert("deploy reached a terminal state", settleErr == nil, "status "+attempt.Status)
	want := "v1"
	outcome := "failed cleanly (" + attempt.Reason + ")"
	if attempt.Status == store.DeployAttemptStatusSucceeded {
		want, outcome = "v2", "resumed and succeeded"
	}
	var served string
	_ = poll(ctx, 30*time.Second, func() bool { served = s.Serving(ctx); return served == want })
	res.Assert("the ingress serves the version the deploy outcome implies", served == want, "serving "+served+", want "+want)

	failedInWindow := 0
	lo, hi := fireAt+downAt, fireAt+downAt+downFor
	for _, x := range r.Sample {
		if x.Failed && x.Offset >= lo && x.Offset < hi {
			failedInWindow++
		}
	}
	res.Assert("no request failed while the Docker API was unreachable", failedInWindow == 0, strconv.Itoa(failedInWindow)+" failed")
	res.Summary = fmt.Sprintf("Docker API unreachable %.0f s: deploy %s, %d requests failed in the window", downFor.Seconds(), outcome, failedInWindow)
	return nil
}

func controlPlaneRestart(ctx context.Context, res *chaos.Result) error {
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
	before := s.Running(ctx)
	if len(before) != 1 || len(before[0].Ports) == 0 {
		return fmt.Errorf("expected one running container with a published port, got %d", len(before))
	}
	oldID := before[0].ID
	direct := fmt.Sprintf("http://127.0.0.1:%d/", before[0].Ports[0].HostPort)
	plain := &http.Client{Timeout: 2 * time.Second}
	_, status, err := get(ctx, plain, direct)
	directOK := err == nil && status == http.StatusOK

	probeCtx, stopProbe := context.WithCancel(ctx)
	directFailures := make(chan int, 1)
	go func() {
		failed := 0
		for probeCtx.Err() == nil {
			if _, st, err := get(probeCtx, plain, direct); probeCtx.Err() == nil && (err != nil || st != http.StatusOK) {
				failed++
			}
			time.Sleep(50 * time.Millisecond)
		}
		directFailures <- failed
	}()

	ev := &bench.Event{Name: "control-plane-restart", At: 5 * time.Second, Fire: s.RestartControlPlane}
	r, err := bench.Run(ctx, s.Client(), bench.Config{URL: s.URL(), Rate: loadRate, Duration: 30 * time.Second, Timeout: 3 * time.Second}, ev)
	stopProbe()
	failedDirect := <-directFailures
	if err != nil {
		return err
	}
	tally(res, r)
	after := s.Running(ctx)
	res.Assert("restart completed", r.Event != nil && r.Event.FireError == "", fireErr(r))
	res.Assert("direct probe of the container worked before the restart", directOK, direct)
	res.Assert("the app container was not recreated", len(after) == 1 && after[0].ID == oldID, "")
	res.Assert("the container kept answering on its own port during the restart", failedDirect == 0, strconv.Itoa(failedDirect)+" direct probes failed")
	res.Assert("the ingress came back on its own", r.Event != nil && r.Event.Recovered, "")
	if r.Event != nil {
		res.RecoveryMS = r.Event.RecoveryMS
	}
	res.Summary = fmt.Sprintf("in-process restart: ingress outage %.0f ms, container untouched, %d direct probes failed (socket activation not exercised)", res.RecoveryMS, failedDirect)
	return nil
}

func oomDuringReadiness(ctx context.Context, res *chaos.Result) error {
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
	ev := &bench.Event{Name: "oom-release", At: 3 * time.Second, Fire: func(c context.Context) error {
		return s.Ship(c, "v2", "oom", oomMemory)
	}}
	r, err := bench.Run(ctx, s.Client(), bench.Config{URL: s.URL(), Rate: loadRate, Duration: 30 * time.Second, Timeout: 3 * time.Second}, ev)
	if err != nil {
		return err
	}
	tally(res, r)
	res.Assert("the OOM release was submitted", r.Event != nil && r.Event.FireError == "", fireErr(r))
	var attempt store.DeployAttempt
	_ = poll(ctx, settleBudget, func() bool {
		attempt = s.Attempt(ctx, "v2")
		return attempt.Status == store.DeployAttemptStatusFailed
	})
	res.Assert("the deploy failed with OOMKilledDuringReadiness", attempt.Status == store.DeployAttemptStatusFailed && attempt.Reason == deploysafety.ReasonOOMKilledDuringReadiness,
		"status "+attempt.Status+" reason "+attempt.Reason)
	res.Assert("the old version kept serving", s.Serving(ctx) == "v1", "serving "+s.Serving(ctx))
	res.Assert("no request failed during the failed deploy", r.Stats.Failed == 0, strconv.Itoa(r.Stats.Failed)+" failed")
	res.Summary = fmt.Sprintf("deploy %s (%s), v1 kept serving, %d of %d requests failed", attempt.Status, attempt.Reason, res.RequestErrors, res.Requests)
	return nil
}

type oneNode struct{}

func (oneNode) ListNodes(context.Context) ([]store.Node, error) {
	return []store.Node{{ID: "chaos-node", Name: "chaos-node"}}, nil
}

type diskSamples struct{ used, total float64 }

func (d diskSamples) QueryMetrics(_ context.Context, _, metric string, from, _ time.Time) ([]telemetry.Sample, error) {
	v := d.used
	if metric == telemetry.MetricDiskTotalBytes {
		v = d.total
	}
	return []telemetry.Sample{{Metric: metric, Timestamp: from.Add(time.Second), Value: v}}, nil
}

func diskPressure(ctx context.Context, res *chaos.Result) error {
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close() }()
	prefix := fmt.Sprintf("lrchaos-disk-%d", time.Now().UnixNano()%1_000_000)

	idle, err := scratchUsage(ctx, cli, prefix+"-idle", "true")
	if err != nil {
		return err
	}
	filled, err := scratchUsage(ctx, cli, prefix+"-full", "dd if=/dev/zero of=/scratch/fill bs=64k 2>/dev/null")
	if err != nil {
		return err
	}
	evaluate := func(u [2]float64) (bool, float64) {
		rule, notices, err := alerting.EvaluateNodeDiskSpace(ctx, oneNode{}, diskSamples{used: u[0], total: u[1]},
			alerting.Rule{ID: "chaos-disk", Kind: alerting.KindNodeDiskSpace}, alerting.DefaultNodeDiskSpaceThresholdPercent, time.Now(), nil)
		var pct float64
		if rule.LastValue != nil {
			pct = *rule.LastValue
		}
		return err == nil && len(notices) > 0, pct
	}
	idleFires, idlePct := evaluate(idle)
	fullFires, fullPct := evaluate(filled)
	res.Assert("an idle scratch volume does not raise the disk alert", !idleFires, fmt.Sprintf("%.1f%% used", idlePct))
	res.Assert("a filled scratch volume raises the disk alert", fullFires, fmt.Sprintf("%.1f%% used", fullPct))
	res.Summary = fmt.Sprintf("10 MiB tmpfs scratch volume filled to %.0f%%, node disk rule fired: %t (measured usage fed to the real evaluator, not the host disk)", fullPct, fullFires)
	return nil
}

// scratchUsage runs a throwaway container with a 10 MiB tmpfs, runs fill, and returns used and total bytes from df.
func scratchUsage(ctx context.Context, cli *dockerclient.Client, name, fill string) ([2]float64, error) {
	var zero [2]float64
	cmd := fill + `; df -Pk /scratch | awk 'NR==2{print $3, $2}'`
	created, err := cli.ContainerCreate(ctx, &container.Config{
		Image: BaseImage, Cmd: []string{"sh", "-c", cmd}, Labels: map[string]string{"levelrail.chaos": name},
	}, &container.HostConfig{
		Tmpfs: map[string]string{"/scratch": "size=10m"}, NetworkMode: "none",
		Resources: container.Resources{Memory: 64 << 20},
	}, nil, nil, name)
	if err != nil {
		return zero, fmt.Errorf("create scratch container: %w", err)
	}
	defer func() {
		_ = cli.ContainerRemove(context.Background(), created.ID, container.RemoveOptions{Force: true})
	}()
	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return zero, fmt.Errorf("start scratch container: %w", err)
	}
	waitC, errC := cli.ContainerWait(ctx, created.ID, container.WaitConditionNotRunning)
	select {
	case <-waitC:
	case err := <-errC:
		return zero, err
	case <-time.After(60 * time.Second):
		return zero, fmt.Errorf("scratch container did not finish")
	}
	logs, err := cli.ContainerLogs(ctx, created.ID, container.LogsOptions{ShowStdout: true})
	if err != nil {
		return zero, err
	}
	defer func() { _ = logs.Close() }()
	var out bytes.Buffer
	if _, err := stdcopy.StdCopy(&out, &out, logs); err != nil {
		return zero, err
	}
	f := strings.Fields(out.String())
	if len(f) < 2 {
		return zero, fmt.Errorf("unexpected df output %q", out.String())
	}
	usedK, e1 := strconv.ParseFloat(f[len(f)-2], 64)
	totalK, e2 := strconv.ParseFloat(f[len(f)-1], 64)
	if e1 != nil || e2 != nil {
		return zero, fmt.Errorf("unparsable df output %q", out.String())
	}
	return [2]float64{usedK * 1024, totalK * 1024}, nil
}
