package build

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ProbeResult is the outcome of a hermetic probe build on one node.
type ProbeResult struct {
	OK       bool
	Latency  time.Duration
	Error    string
	BuildKit string
}

// ProbeNode runs a tiny scratch-image build on nodeID through the real dispatch path.
// The context holds only a fixed marker file, so no app source or secret is ever sent.
func ProbeNode(ctx context.Context, remote NodeBuilder, nodeID string, timeout time.Duration) (ProbeResult, error) {
	if remote == nil {
		return ProbeResult{}, fmt.Errorf("build: remote builds are not available on this control plane")
	}
	dir, err := os.MkdirTemp("", "levelrail-probe-")
	if err != nil {
		return ProbeResult{}, fmt.Errorf("build: probe context: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\nCOPY probe /probe\n"), 0o600); err != nil {
		return ProbeResult{}, fmt.Errorf("build: probe dockerfile: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "probe"), []byte("levelrail-probe\n"), 0o600); err != nil {
		return ProbeResult{}, fmt.Errorf("build: probe marker: %w", err)
	}
	var salt [4]byte
	_, _ = rand.Read(salt[:])
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	start := time.Now()
	res, err := remote.BuildOnNode(ctx, nodeID, RemoteRequest{
		Kind: RemoteKindDockerfile, ContextDir: dir, Tag: "levelrail-probe:" + hex.EncodeToString(salt[:]), NoCache: true,
	}, io.Discard, func(ProgressEvent) {})
	out := ProbeResult{Latency: time.Since(start)}
	if err != nil {
		out.Error = err.Error()
		return out, nil
	}
	out.OK = true
	if res != nil {
		out.BuildKit = res.ExporterResponse["buildkit.version"]
	}
	return out, nil
}
