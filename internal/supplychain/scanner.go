package supplychain

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
)

const (
	sbomPathInContainer = "/tmp/sbom.json"
	cachePath           = "/cache"
	roleLabelSuffix     = ".role"
	roleValue           = "supply-chain-scan"
	scanPidsLimit       = 256
)

// Runner is the Docker surface scanning uses; *docker.Client satisfies it.
type Runner interface {
	EnsureImageID(ctx context.Context, ref string) (id string, pulled bool, err error)
	RunOneShot(ctx context.Context, spec docker.OneShotSpec) (docker.OneShotResult, error)
	ListContainersByLabel(ctx context.Context, label string) ([]docker.LabeledContainer, error)
	Remove(ctx context.Context, id string, force bool) error
}

// scannerCommand is the container command and output parser for a scanner.
func scannerCommand(scanner string) ([]string, map[string]string, func([]byte) (ScanSummary, error), error) {
	switch scanner {
	case ScannerTrivy:
		return []string{"sbom", "--format", "json", "--quiet", "--cache-dir", cachePath, sbomPathInContainer},
			map[string]string{"TRIVY_NO_PROGRESS": "true"}, ParseTrivy, nil
	case ScannerGrype:
		return []string{"sbom:" + sbomPathInContainer, "-o", "json", "-q"},
			map[string]string{"GRYPE_DB_CACHE_DIR": cachePath, "GRYPE_CHECK_FOR_APP_UPDATE": "false"}, ParseGrype, nil
	}
	return nil, nil, nil, fmt.Errorf("%w: scanner must be trivy or grype", ErrInvalid)
}

// runScan scans sbom in a one-shot scanner container.
func (s *Service) runScan(ctx context.Context, attemptID string, sbom []byte) (ScanSummary, error) {
	cmd, env, parse, err := scannerCommand(s.cfg.Scanner)
	if err != nil {
		return ScanSummary{}, err
	}
	pullCtx, cancelPull := context.WithTimeout(ctx, s.cfg.PullTimeout)
	_, _, err = s.deps.Runner.EnsureImageID(pullCtx, s.cfg.Image)
	cancelPull()
	if err != nil {
		return ScanSummary{}, fmt.Errorf("supplychain: scanner image %q unavailable: %w", s.cfg.Image, err)
	}
	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	runCtx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()
	res, err := s.deps.Runner.RunOneShot(runCtx, docker.OneShotSpec{
		Name:    s.deps.Namespace + "-scan-" + hex.EncodeToString(suffix),
		Image:   s.cfg.Image,
		Command: cmd,
		Env:     env,
		Labels: map[string]string{
			s.roleLabel():                         roleValue,
			s.deps.Namespace + ".scan-deployment": attemptID,
		},
		Files:       map[string][]byte{sbomPathInContainer: sbom},
		Volumes:     map[string]string{s.deps.Namespace + "-scanner-cache": cachePath},
		MemoryBytes: s.cfg.MemoryMB << 20,
		NanoCPUs:    int64(s.cfg.CPUs * 1e9),
		PidsLimit:   scanPidsLimit,
	})
	if err != nil {
		return ScanSummary{}, fmt.Errorf("supplychain: run scanner: %w", err)
	}
	if res.ExitCode != 0 {
		return ScanSummary{}, fmt.Errorf("supplychain: scanner exited %d: %s", res.ExitCode, tail(string(res.Stderr), 400))
	}
	return parse(res.Stdout)
}

func (s *Service) roleLabel() string { return s.deps.Namespace + roleLabelSuffix }

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}
