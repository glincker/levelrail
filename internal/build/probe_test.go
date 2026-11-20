package build

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type probeRemote struct {
	err   error
	files []string
}

func (p *probeRemote) BuildOnNode(_ context.Context, _ string, req RemoteRequest, _ io.Writer, _ func(ProgressEvent)) (*Result, error) {
	entries, _ := os.ReadDir(req.ContextDir)
	for _, e := range entries {
		p.files = append(p.files, filepath.Base(e.Name()))
	}
	if p.err != nil {
		return nil, p.err
	}
	return &Result{ExporterResponse: map[string]string{"buildkit.version": "v0.20.0"}}, nil
}

func TestProbeNodeSendsOnlyMarkerContext(t *testing.T) {
	r := &probeRemote{}
	res, err := ProbeNode(context.Background(), r, "n1", 0)
	if err != nil || !res.OK || res.BuildKit != "v0.20.0" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(r.files) != 2 || r.files[0] != "Dockerfile" || r.files[1] != "probe" {
		t.Fatalf("context must hold only the marker, got %v", r.files)
	}
}

func TestProbeNodeReportsFailureWithoutError(t *testing.T) {
	res, err := ProbeNode(context.Background(), &probeRemote{err: errors.New("builder offline")}, "n1", 0)
	if err != nil || res.OK || res.Error != "builder offline" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestPausedBuilderSkippedRunningBuildsUntouched(t *testing.T) {
	paused := runnerNode("a", 1)
	paused.Draining = true
	open := runnerNode("b", 2)
	d := ChooseBuildNode([]NodeInfo{paused, open}, Query{}, Policy{Strategy: StrategyFirst})
	if d.Node == "a" {
		t.Fatalf("paused builder must not receive new builds, got %+v", d)
	}
}
