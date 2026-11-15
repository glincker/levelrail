package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/hostpressure"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	// Bounds on the top consumer lookup: containers inspected and rows reported.
	pressureScanMax = 64
	pressureTopN    = 5
	pressureHostID  = "host"
)

// hostPressureConfig reads every threshold from runtime settings on each sample so changes apply live.
func hostPressureConfig(router *api.Router) func() hostpressure.Config {
	return func() hostpressure.Config {
		rs, c := router.RuntimeSettings(), context.Background()
		f := func(k string) float64 { return float64(rs.Int(c, k)) }
		return hostpressure.Config{
			Enabled: rs.Bool(c, runtimesettings.KeyHostPressureEnabled), ShedEnabled: rs.Bool(c, runtimesettings.KeyHostPressureShedEnabled),
			CPUWarn: f(runtimesettings.KeyHostPressureCPUWarn), CPUCritical: f(runtimesettings.KeyHostPressureCPUCritical),
			MemWarn: f(runtimesettings.KeyHostPressureMemWarn), MemCritical: f(runtimesettings.KeyHostPressureMemCritical),
			MemFullWarn: f(runtimesettings.KeyHostPressureFullWarn), MemFullCritical: f(runtimesettings.KeyHostPressureFullCritical),
			AvailWarn: f(runtimesettings.KeyHostPressureAvailWarn), AvailCritical: f(runtimesettings.KeyHostPressureAvailCrit),
			LoadWarn: f(runtimesettings.KeyHostPressureLoadWarn), LoadCritical: f(runtimesettings.KeyHostPressureLoadCritical),
			ExitPercent: f(runtimesettings.KeyHostPressureExitPercent),
			Hold:        rs.Duration(c, runtimesettings.KeyHostPressureHold), Jitter: rs.Duration(c, runtimesettings.KeyHostPressureJitter),
			Interval:  rs.Duration(c, runtimesettings.KeyHostPressureInterval),
			NoSwapRAM: int64(rs.Int(c, runtimesettings.KeyHostNoSwapRAMMB)) * mibBytes,
		}
	}
}

// topContainersByMemory reads one-shot stats for running containers and returns the biggest users.
func topContainersByMemory(client *docker.Client) hostpressure.ConsumerFunc {
	return func(ctx context.Context) ([]hostpressure.Consumer, error) {
		list, err := client.ListLocalContainers(ctx)
		if err != nil {
			return nil, fmt.Errorf("list containers: %w", err)
		}
		var out []hostpressure.Consumer
		scanned := 0
		for _, c := range list {
			if !c.Running || scanned >= pressureScanMax || ctx.Err() != nil {
				continue
			}
			scanned++
			st, serr := client.Stats(ctx, c.Name)
			if serr != nil {
				continue
			}
			out = append(out, hostpressure.Consumer{Name: c.Name, MemBytes: int64(st.MemoryUsageBytes)}) //nolint:gosec // container memory fits int64
		}
		sort.Slice(out, func(i, j int) bool { return out[i].MemBytes > out[j].MemBytes })
		if len(out) > pressureTopN {
			out = out[:pressureTopN]
		}
		return out, nil
	}
}

// derivedBuildLimits seeds build slots from CPU and RAM when APP_BUILD_MAX_CONCURRENT is unset; saved build limits still override it.
func derivedBuildLimits() build.Limits {
	l := build.LimitsFromEnv(os.LookupEnv)
	if v, ok := os.LookupEnv(build.EnvBuildMaxConcurrent); !ok || v == "" {
		l.MaxConcurrent = build.DerivedMaxConcurrent(runtime.NumCPU(), hostMemoryMB())
	}
	return l
}

// startHostPressure samples the host, feeds gauges and gates new local builds.
func (s *bootState) startHostPressure() {
	g := hostpressure.New(hostpressure.ProcReader{}.Read, hostPressureConfig(s.apiRouter), s.logger)
	if s.client != nil {
		g.SetConsumers(topContainersByMemory(s.client))
	}
	g.Sink = func(st hostpressure.Status) {
		if s.telemetryDB == nil || st.Source == "unavailable" {
			return
		}
		now, sm := time.Now(), st.Sample
		rows := []telemetry.Sample{
			{ResourceID: pressureHostID, Metric: "host_pressure_state", Timestamp: now, Value: float64(st.State.Rank())},
			{ResourceID: pressureHostID, Metric: "host_psi_cpu_some_avg10", Timestamp: now, Value: sm.CPUSome10},
			{ResourceID: pressureHostID, Metric: "host_psi_memory_some_avg10", Timestamp: now, Value: sm.MemSome10},
			{ResourceID: pressureHostID, Metric: "host_psi_memory_full_avg10", Timestamp: now, Value: sm.MemFull10},
			{ResourceID: pressureHostID, Metric: "host_load1", Timestamp: now, Value: sm.Load1},
		}
		if err := s.telemetryDB.WriteSamples(s.ctx, rows); err != nil {
			s.logger.Warn("hostpressure: write gauges failed", "error", err.Error())
		}
	}
	s.apiRouter.SetHostPressure(g)
	build.SetHostGate(g)
	go g.Run(s.ctx)
}
