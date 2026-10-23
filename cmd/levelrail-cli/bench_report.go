package main

import (
	"context"
	"fmt"
	"io"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/benchreport"
	"github.com/GLINCKER/levelrail/internal/chaos"
	"github.com/GLINCKER/levelrail/internal/version"
)

const (
	envNoisyLoadRatio = "BENCH_NOISY_LOAD_RATIO"
	envBenchCommit    = "BENCH_COMMIT"
)

type dockerProbeFunc func(ctx context.Context) benchreport.Docker

type benchReportPlan struct {
	jsonPath string
	mdPath   string
	upload   bool
	repeat   int
	note     string
	probe    benchreport.Probe
	docker   dockerProbeFunc
	host     benchreport.Host
}

func (p *benchReportPlan) wanted() bool { return p.jsonPath != "" || p.mdPath != "" || p.upload }

// begin records the host and its load before the scenarios start.
func (p *benchReportPlan) begin(ctx context.Context) {
	p.host = benchreport.DetectHost(ctx, p.probe)
	p.host.Note = p.note
	if p.docker != nil {
		p.host.Docker = p.docker(ctx)
	}
}

// finish samples the load again, builds the report and writes the requested files.
func (p *benchReportPlan) finish(ctx context.Context, results []chaos.Result, lookupEnv func(string) (string, bool), stderr io.Writer) (benchreport.Report, error) {
	p.host.LoadEnd = benchreport.Load1(ctx, p.probe)
	ratio := 0.0
	if v, ok := lookupEnv(envNoisyLoadRatio); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			ratio = f
		}
	}
	rep := benchreport.New(results, benchreport.Options{Build: buildIdentity(lookupEnv), Host: p.host, Repeat: p.repeat, Now: time.Now(), NoisyLoadRatio: ratio})
	if err := rep.Validate(); err != nil {
		return rep, err
	}
	if p.jsonPath != "" {
		if err := benchreport.WriteJSON(p.jsonPath, rep); err != nil {
			return rep, err
		}
		_, _ = fmt.Fprintf(stderr, "wrote %s\n", p.jsonPath)
	}
	if p.mdPath != "" {
		if err := benchreport.WriteMarkdown(p.mdPath, rep); err != nil {
			return rep, err
		}
		_, _ = fmt.Fprintf(stderr, "wrote %s\n", p.mdPath)
	}
	return rep, nil
}

// buildIdentity reads the version and the VCS stamp Go embeds at build time.
// BENCH_COMMIT overrides the stamp: a build from a git worktree can be stamped with another checkout's HEAD.
func buildIdentity(lookupEnv func(string) (string, bool)) benchreport.Build {
	b := benchreport.Build{Version: version.Version, Commit: unknownBuildInfo}
	if c, ok := lookupEnv(envBenchCommit); ok && c != "" {
		b.Commit = c
		return b
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				b.Commit = s.Value
			case "vcs.modified":
				b.Dirty = s.Value == "true"
			}
		}
	}
	return b
}
