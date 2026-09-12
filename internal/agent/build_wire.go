package agent

// This file: the internal/build <-> agentpb conversions for a dispatched
// build, the build counterpart to convert.go's docker-type conversions.

import (
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/build"
)

func buildRequestToPB(req build.RemoteRequest) (*agentpb.BuildRequest, error) {
	kind, err := buildKindToPB(req.Kind)
	if err != nil {
		return nil, err
	}
	return &agentpb.BuildRequest{
		Kind:           kind,
		Tag:            req.Tag,
		DockerfilePath: req.DockerfilePath,
		Target:         req.Target,
		BuildArgs:      req.BuildArgs,
		NoCache:        req.NoCache,
		Cache: &agentpb.BuildCache{
			RegistryRef:      req.Cache.RegistryRef,
			RegistryInsecure: req.Cache.RegistryInsecure,
		},
	}, nil
}

// buildRequestFromPB rebuilds the dispatched request agent-side. It
// leaves ContextDir empty: the build node fills in its own unpacked
// context directory, which is the one thing that cannot travel over the
// wire as a path.
func buildRequestFromPB(req *agentpb.BuildRequest) (build.RemoteRequest, error) {
	kind, err := buildKindFromPB(req.GetKind())
	if err != nil {
		return build.RemoteRequest{}, err
	}
	return build.RemoteRequest{
		Kind:           kind,
		DockerfilePath: req.GetDockerfilePath(),
		Tag:            req.GetTag(),
		Target:         req.GetTarget(),
		BuildArgs:      req.GetBuildArgs(),
		NoCache:        req.GetNoCache(),
		Cache: build.CacheConfig{
			RegistryRef:      req.GetCache().GetRegistryRef(),
			RegistryInsecure: req.GetCache().GetRegistryInsecure(),
		},
	}, nil
}

func buildKindToPB(kind build.RemoteKind) (agentpb.BuildKind, error) {
	switch kind {
	case build.RemoteKindDockerfile:
		return agentpb.BuildKind_BUILD_KIND_DOCKERFILE, nil
	case build.RemoteKindRailpack:
		return agentpb.BuildKind_BUILD_KIND_RAILPACK, nil
	default:
		return agentpb.BuildKind_BUILD_KIND_UNSPECIFIED, fmt.Errorf("agent: build kind %q cannot be dispatched", kind)
	}
}

func buildKindFromPB(kind agentpb.BuildKind) (build.RemoteKind, error) {
	switch kind {
	case agentpb.BuildKind_BUILD_KIND_DOCKERFILE:
		return build.RemoteKindDockerfile, nil
	case agentpb.BuildKind_BUILD_KIND_RAILPACK:
		return build.RemoteKindRailpack, nil
	case agentpb.BuildKind_BUILD_KIND_UNSPECIFIED:
		return "", errors.New("agent: dispatched build did not say which kind of build to run")
	default:
		return "", fmt.Errorf("agent: dispatched build has an unknown kind %v", kind)
	}
}

func buildProgressToPB(ev build.ProgressEvent) *agentpb.BuildProgress {
	return &agentpb.BuildProgress{
		Step:      ev.Step,
		Cached:    ev.Cached,
		Completed: ev.Completed,
		Error:     ev.Error,
		Log:       ev.Log,
		Stream:    ev.Stream,
	}
}

func buildProgressFromPB(p *agentpb.BuildProgress) build.ProgressEvent {
	return build.ProgressEvent{
		Step:      p.GetStep(),
		Cached:    p.GetCached(),
		Completed: p.GetCompleted(),
		Error:     p.GetError(),
		Log:       p.GetLog(),
		Stream:    p.GetStream(),
	}
}

// buildFailureToPB carries an unsupported-provider failure structurally,
// not just as a message, so the control plane can rebuild the typed error
// internal/deploy branches on with errors.As.
func buildFailureToPB(err error) *agentpb.BuildFailure {
	failure := &agentpb.BuildFailure{Message: err.Error()}
	var unsupported *build.UnsupportedProviderError
	if errors.As(err, &unsupported) {
		failure.UnsupportedProvider = true
		failure.Provider = unsupported.Provider
	}
	return failure
}

func buildFailureToError(f *agentpb.BuildFailure) error {
	if f.GetUnsupportedProvider() {
		return &build.UnsupportedProviderError{Provider: f.GetProvider()}
	}
	return errors.New(f.GetMessage())
}

func buildResultFromPB(tag string, done *agentpb.BuildDone) *build.Result {
	return &build.Result{
		Tag:              tag,
		Duration:         time.Duration(done.GetDurationMs()) * time.Millisecond,
		ExporterResponse: done.GetExporterResponse(),
	}
}
