package build

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	dockerclient "github.com/docker/docker/client"
	bkclient "github.com/moby/buildkit/client"
	"golang.org/x/sync/errgroup"
)

// Result is what a successful build produced.
type Result struct {
	Tag      string
	Duration time.Duration

	// ExporterResponse is BuildKit's raw exporter metadata (e.g. the
	// resulting image ID), passed through for callers that need it.
	ExporterResponse map[string]string
}

// ProgressEvent is one structured build progress update, deliberately
// decoupled from BuildKit's own SolveStatus wire type so callers, a
// future SSE handler in particular (the build design's build log
// streaming requirement), don't need to import moby/buildkit/client
// just to format a build log line for a browser.
type ProgressEvent struct {
	// Step is the build step's name, e.g. "[2/4] RUN go build".
	Step string
	// Cached is true when this step was skipped because BuildKit's cache
	// (see WithCacheDir) already had the result.
	Cached bool
	// Completed is true once Step finished, successfully or not.
	Completed bool
	// Error is non-empty when Step failed.
	Error string
	// Log is a raw output line from the step (stdout/stderr from the
	// build), empty for step-lifecycle events.
	Log string
	// Stream is "stdout" or "stderr" when Log is non-empty, and empty
	// for a step-lifecycle event (Log == ""), since those never came
	// from either stream. Added for internal/deploylog.Recorder, so a
	// persisted/live-streamed build log line can carry the same
	// stdout/stderr distinction web/src/hooks/useDeployLogStream.ts's
	// contract already expects (its own doc comment: "distinguishing
	// stdout/stderr is useful for highlighting failed build steps").
	// Populated from BuildKit's own VertexLog.Stream (see
	// relayProgress), 1/2 being BuildKit's stdout/stderr convention; the
	// docker-image-load phase (loadImage) has no equivalent stream
	// signal from the Engine API, so it always reports "stdout".
	Stream string
}

// SlogProgress adapts a *slog.Logger into a progress func, for callers
// that just want build progress logged rather than consumed some other
// way (an SSE stream, a test assertion). log defaults to slog.Default()
// if nil.
func SlogProgress(log *slog.Logger) func(ProgressEvent) {
	if log == nil {
		log = slog.Default()
	}
	return func(ev ProgressEvent) {
		switch {
		case ev.Error != "":
			log.Error("build: step failed", "step", ev.Step, "error", ev.Error)
		case ev.Completed:
			log.Debug("build: step completed", "step", ev.Step, "cached", ev.Cached)
		case ev.Log != "":
			log.Debug("build: step output", "step", ev.Step, "data", ev.Log)
		}
	}
}

// Build runs req end to end: solves the Dockerfile with BuildKit, streams
// the resulting image as a tar into the local Docker Engine's
// /images/load endpoint, and waits for both to finish. It never shells
// out to the docker CLI.
//
// progress, if non-nil, receives every build progress update as it
// happens, including the docker-image-load phase after BuildKit's own
// solve finishes (that phase isn't part of BuildKit's SolveStatus
// stream, but is relayed through the same callback for one unified
// progress feed). Pass SlogProgress(logger) to just log them, or nil to
// discard them entirely.
func (c *Client) Build(ctx context.Context, req Request, progress func(ProgressEvent)) (*Result, error) {
	if progress == nil {
		progress = func(ProgressEvent) {}
	}
	start := time.Now()

	pipeR, pipeW := io.Pipe()

	solveOpt, err := newSolveOpt(req, c.cache, pipeW)
	if err != nil {
		_ = pipeW.Close()
		_ = pipeR.Close()
		return nil, err
	}

	statusCh := make(chan *bkclient.SolveStatus)

	eg, egCtx := errgroup.WithContext(ctx)

	var solveResp *bkclient.SolveResponse
	eg.Go(func() error {
		defer func() { _ = pipeW.Close() }()
		resp, err := c.bk.Solve(egCtx, nil, *solveOpt, statusCh)
		if err != nil {
			return fmt.Errorf("build: solve %q: %w", req.Tag, err)
		}
		solveResp = resp
		return nil
	})

	eg.Go(func() error {
		relayProgress(statusCh, progress)
		return nil
	})

	eg.Go(func() error {
		defer func() { _ = pipeR.Close() }()
		return loadImage(egCtx, c.docker, pipeR, req.Tag, progress)
	})

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	res := &Result{
		Tag:      req.Tag,
		Duration: time.Since(start),
	}
	if solveResp != nil {
		res.ExporterResponse = solveResp.ExporterResponse
	}
	return res, nil
}

// relayProgress drains a SolveStatus channel, converting each vertex and
// log line into a ProgressEvent for progress. BuildKit closes ch when the
// solve finishes (success or failure), so this always returns once the
// build is done.
func relayProgress(ch <-chan *bkclient.SolveStatus, progress func(ProgressEvent)) {
	for st := range ch {
		for _, v := range st.Vertexes {
			switch {
			case v.Error != "":
				progress(ProgressEvent{Step: v.Name, Error: v.Error, Completed: true})
			case v.Completed != nil:
				progress(ProgressEvent{Step: v.Name, Cached: v.Cached, Completed: true})
			}
		}
		for _, l := range st.Logs {
			progress(ProgressEvent{Log: string(l.Data), Stream: vertexLogStream(l.Stream)})
		}
	}
}

// vertexLogStream converts BuildKit's VertexLog.Stream int (1 = stdout,
// 2 = stderr, the same convention as Unix file descriptor numbers) into
// ProgressEvent.Stream's string form. Anything else (0, or a value
// BuildKit's own protocol doesn't currently define) defaults to
// "stdout": a build log line always came from one of exactly two real
// streams, so there is no meaningful third value to represent, only an
// unset one to default away.
func vertexLogStream(bkStream int) string {
	if bkStream == 2 {
		return "stderr"
	}
	return "stdout"
}

// loadImage reads a `docker save`-style tar from r and loads it into the
// local image store via the Docker Engine API, the same endpoint the
// `docker load` CLI command uses internally. r is exhausted, but not
// closed, by this function; the caller owns closing the pipe.
func loadImage(ctx context.Context, docker *dockerclient.Client, r io.Reader, tag string, progress func(ProgressEvent)) error {
	resp, err := docker.ImageLoad(ctx, r, false)
	if err != nil {
		return fmt.Errorf("build: load image %q into docker: %w", tag, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if !resp.JSON {
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			return fmt.Errorf("build: drain image load response for %q: %w", tag, err)
		}
		return nil
	}

	dec := json.NewDecoder(resp.Body)
	for {
		var msg struct {
			Stream string `json:"stream"`
			Error  string `json:"error"`
		}
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("build: read image load response for %q: %w", tag, err)
		}
		if msg.Error != "" {
			return fmt.Errorf("build: docker image load %q: %s", tag, msg.Error)
		}
		if msg.Stream != "" {
			progress(ProgressEvent{Log: msg.Stream, Stream: "stdout"})
		}
	}
}
