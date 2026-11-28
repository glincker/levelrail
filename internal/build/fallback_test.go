package build

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
	bkclient "github.com/moby/buildkit/client"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/glincker/levelrail/internal/build/buildkitd"
)

type nopWC struct{ bytes.Buffer }

func (*nopWC) Close() error { return nil }

func TestStdioConnDemuxesAndWrites(t *testing.T) {
	var frames bytes.Buffer
	_, _ = stdcopy.NewStdWriter(&frames, stdcopy.Stdout).Write([]byte("hello"))
	_, _ = stdcopy.NewStdWriter(&frames, stdcopy.Stderr).Write([]byte("noise"))
	_, _ = stdcopy.NewStdWriter(&frames, stdcopy.Stdout).Write([]byte(" world"))

	w := &nopWC{}
	c := newStdioConn(w, &frames)
	got, err := io.ReadAll(c)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world" {
		t.Fatalf("got %q", got)
	}
	if _, err := c.Write([]byte("req")); err != nil || w.String() != "req" {
		t.Fatalf("write: %v %q", err, w.String())
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestEnvDurationOverride(t *testing.T) {
	t.Setenv(EnvBuildkitProbeTimeout, "3s")
	if envDuration(EnvBuildkitProbeTimeout, time.Minute) != 3*time.Second {
		t.Fatal("duration override ignored")
	}
	t.Setenv(EnvBuildkitProbeTimeout, "junk")
	if envDuration(EnvBuildkitProbeTimeout, time.Minute) != time.Minute {
		t.Fatal("bad value must fall back to default")
	}
}

const pinnedRootless = "docker.io/moby/buildkit:v0.34.1-rootless@sha256:0000000000000000000000000000000000000000000000000000000000000000"

func envGetter(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestMaybeFallbackDecisions(t *testing.T) {
	noGRPC := status.Error(grpccodes.Unavailable, `connection error: desc = "transport: Error while dialing: unable to upgrade to h2c, received 404"`)
	pinned := envGetter(map[string]string{"APP_BUILDKIT_IMAGE": pinnedRootless})
	tests := []struct {
		name      string
		err       error
		get       func(string) string
		podman    bool
		wantStart bool
		wantErr   error
	}{
		{"transient deadline fails closed", context.DeadlineExceeded, pinned, false, false, context.DeadlineExceeded},
		{"transient unavailable fails closed", status.Error(grpccodes.Unavailable, "connection refused"), pinned, false, false, nil},
		{"no pinned image means no fallback", noGRPC, envGetter(nil), false, false, ErrBuildEngineUnsupported},
		{"unpinned tag refused", noGRPC, envGetter(map[string]string{"APP_BUILDKIT_IMAGE": "moby/buildkit:v1-rootless"}), false, false, ErrBuildEngineUnsupported},
		{"unimplemented with pin starts", status.Error(grpccodes.Unimplemented, "unknown service"), pinned, false, true, nil},
		{"upgrade 404 with pin starts", noGRPC, pinned, true, true, nil},
		{"podman needs the rootless variant", noGRPC, envGetter(map[string]string{"APP_BUILDKIT_IMAGE": "moby/buildkit:v1@sha256:" + strings.Repeat("0", 64)}), true, false, ErrBuildEngineUnsupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			started := false
			_, err := maybeFallback(context.Background(), tt.err, tt.get, tt.podman, func(context.Context, buildkitd.Config) (*bkclient.Client, error) {
				started = true
				return nil, nil
			})
			if started != tt.wantStart {
				t.Fatalf("started=%v want %v (err %v)", started, tt.wantStart, err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("err %v want %v", err, tt.wantErr)
			}
			if !tt.wantStart && err == nil {
				t.Fatal("expected an error when nothing starts")
			}
		})
	}
}

func TestStdioConnSurfacesStderrWhenNoPayload(t *testing.T) {
	var frames bytes.Buffer
	_, _ = stdcopy.NewStdWriter(&frames, stdcopy.Stderr).Write([]byte("exec: buildctl: not found"))
	c := newStdioConn(&nopWC{}, &frames)
	_, err := io.ReadAll(c)
	if err == nil || !strings.Contains(err.Error(), "buildctl: not found") {
		t.Fatalf("got %v", err)
	}
}
