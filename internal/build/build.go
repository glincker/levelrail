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

// Build runs req end to end: solves the Dockerfile with BuildKit, streams
// the resulting image as a tar into the local Docker Engine's
// /images/load endpoint, and waits for both to finish. It never shells
// out to the docker CLI.
//
// log receives structured progress for the build (per-step completion,
// per-step failure, docker load progress). Phase 1 will replace this with
// streaming SolveStatus to the frontend over SSE and adding remote cache
// via SolveOpt.CacheImports / CacheExports; see
// docs-local/research/buildkit-spike.md.
func (c *Client) Build(ctx context.Context, req Request, log *slog.Logger) (*Result, error) {
	if log == nil {
		log = slog.Default()
	}
	start := time.Now()

	pipeR, pipeW := io.Pipe()

	solveOpt, err := newSolveOpt(req, pipeW)
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
		logProgress(egCtx, log, req.Tag, statusCh)
		return nil
	})

	eg.Go(func() error {
		defer func() { _ = pipeR.Close() }()
		return loadImage(egCtx, c.docker, pipeR, log, req.Tag)
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
	log.InfoContext(ctx, "build: completed", "tag", req.Tag, "duration", res.Duration)
	return res, nil
}

// logProgress drains a SolveStatus channel into structured log lines.
// BuildKit closes ch when the solve finishes (success or failure), so
// this always returns once the build is done.
func logProgress(ctx context.Context, log *slog.Logger, tag string, ch <-chan *bkclient.SolveStatus) {
	for st := range ch {
		for _, v := range st.Vertexes {
			switch {
			case v.Error != "":
				log.ErrorContext(ctx, "build: step failed", "tag", tag, "step", v.Name, "error", v.Error)
			case v.Completed != nil:
				log.DebugContext(ctx, "build: step completed", "tag", tag, "step", v.Name, "cached", v.Cached)
			}
		}
		for _, l := range st.Logs {
			log.DebugContext(ctx, "build: step output", "tag", tag, "data", string(l.Data))
		}
	}
}

// loadImage reads a `docker save`-style tar from r and loads it into the
// local image store via the Docker Engine API, the same endpoint the
// `docker load` CLI command uses internally. r is exhausted, but not
// closed, by this function; the caller owns closing the pipe.
func loadImage(ctx context.Context, docker *dockerclient.Client, r io.Reader, log *slog.Logger, tag string) error {
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
			log.DebugContext(ctx, "build: docker load", "tag", tag, "message", msg.Stream)
		}
	}
}
