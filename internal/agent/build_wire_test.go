package agent

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/build"
)

func TestBuildRequestWireRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		req  build.RemoteRequest
	}{
		{
			name: "dockerfile with every field set",
			req: build.RemoteRequest{
				Kind:           build.RemoteKindDockerfile,
				DockerfilePath: "docker/Dockerfile",
				Tag:            "app:sha",
				Target:         "runtime",
				BuildArgs:      map[string]string{"A": "1", "B": "2"},
				NoCache:        true,
				Cache:          build.CacheConfig{RegistryRef: "reg.example/cache:app", RegistryInsecure: true},
			},
		},
		{
			name: "railpack with nothing but a tag",
			req:  build.RemoteRequest{Kind: build.RemoteKindRailpack, Tag: "app:sha"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pb, err := buildRequestToPB(tt.req)
			if err != nil {
				t.Fatalf("buildRequestToPB() error = %v", err)
			}
			got, err := buildRequestFromPB(pb)
			if err != nil {
				t.Fatalf("buildRequestFromPB() error = %v", err)
			}

			// ContextDir deliberately does not survive: the build node
			// fills in its own unpacked directory.
			want := tt.req
			want.ContextDir = ""
			if got.Kind != want.Kind || got.Tag != want.Tag || got.Target != want.Target ||
				got.DockerfilePath != want.DockerfilePath || got.NoCache != want.NoCache || got.Cache != want.Cache {
				t.Errorf("round trip = %+v, want %+v", got, want)
			}
			if len(got.BuildArgs) != len(want.BuildArgs) {
				t.Fatalf("BuildArgs = %v, want %v", got.BuildArgs, want.BuildArgs)
			}
			for k, v := range want.BuildArgs {
				if got.BuildArgs[k] != v {
					t.Errorf("BuildArgs[%q] = %q, want %q", k, got.BuildArgs[k], v)
				}
			}
		})
	}
}

func TestBuildKindWireErrors(t *testing.T) {
	if _, err := buildRequestToPB(build.RemoteRequest{Kind: "compose", Tag: "app:sha"}); err == nil {
		t.Error("buildRequestToPB() with an undispatchable kind: error = nil, want a non-nil error")
	}
	if _, err := buildRequestFromPB(&agentpb.BuildRequest{Tag: "app:sha"}); err == nil {
		t.Error("buildRequestFromPB() with an unspecified kind: error = nil, want a non-nil error")
	}
	if _, err := buildRequestFromPB(&agentpb.BuildRequest{Kind: agentpb.BuildKind(99), Tag: "app:sha"}); err == nil {
		t.Error("buildRequestFromPB() with an unknown kind: error = nil, want a non-nil error")
	}
}

func TestBuildProgressWireRoundTrip(t *testing.T) {
	events := []build.ProgressEvent{
		{Step: "[2/4] RUN go build", Completed: true},
		{Step: "[1/4] FROM golang", Completed: true, Cached: true},
		{Step: "[3/4] RUN test", Completed: true, Error: "exit code 1"},
		{Log: "compiling\n", Stream: "stdout"},
		{Log: "vet failed\n", Stream: "stderr"},
	}

	for _, want := range events {
		if got := buildProgressFromPB(buildProgressToPB(want)); got != want {
			t.Errorf("round trip = %+v, want %+v", got, want)
		}
	}
}

func TestBuildFailureWire(t *testing.T) {
	t.Run("a plain failure keeps its message", func(t *testing.T) {
		err := buildFailureToError(buildFailureToPB(errors.New("solve failed: no such file")))
		if err.Error() != "solve failed: no such file" {
			t.Errorf("error = %q, want the original message", err)
		}
	})

	t.Run("an unsupported provider stays typed", func(t *testing.T) {
		wrapped := fmt.Errorf("deploy: service %q: %w", "web", &build.UnsupportedProviderError{Provider: "ruby"})
		err := buildFailureToError(buildFailureToPB(wrapped))

		var unsupported *build.UnsupportedProviderError
		if !errors.As(err, &unsupported) {
			t.Fatalf("error = %v, want a *build.UnsupportedProviderError even through a wrap", err)
		}
		if unsupported.Provider != "ruby" {
			t.Errorf("Provider = %q, want %q", unsupported.Provider, "ruby")
		}
	})
}

func TestBuildResultFromPB(t *testing.T) {
	got := buildResultFromPB("app:sha", &agentpb.BuildDone{
		DurationMs:       1500,
		ExporterResponse: map[string]string{"containerimage.digest": "sha256:abc"},
	})
	if got.Tag != "app:sha" || got.Duration != 1500*time.Millisecond {
		t.Errorf("Result = %+v, want app:sha built in 1.5s", got)
	}
	if got.ExporterResponse["containerimage.digest"] != "sha256:abc" {
		t.Errorf("ExporterResponse = %v, want the remote exporter metadata", got.ExporterResponse)
	}
}
