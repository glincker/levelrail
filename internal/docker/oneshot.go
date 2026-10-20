package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// OneShotSpec describes a run-to-completion container: it is created,
// started, waited on, read and force-removed by RunOneShot.
type OneShotSpec struct {
	Name    string
	Image   string
	Command []string
	Env     map[string]string
	Labels  map[string]string
	// Files are copied into the container before it starts, keyed by absolute path.
	Files map[string][]byte
	// Volumes maps a named volume to its mount path; each is created if missing.
	Volumes     map[string]string
	MemoryBytes int64
	NanoCPUs    int64
	PidsLimit   int64
	// MaxOutput caps captured stdout and stderr each; 0 means 32 MiB.
	MaxOutput int64
}

// OneShotResult is what a one-shot container produced.
type OneShotResult struct {
	ExitCode int64
	Stdout   []byte
	Stderr   []byte
}

const defaultOneShotOutput = 32 << 20

// RunOneShot runs spec to completion with default networking (scanners need
// egress to fetch their database) and every capability dropped. The
// container is force-removed on every path; the labels let an orphan sweep
// find leftovers of a control plane that died mid-run.
func (c *Client) RunOneShot(ctx context.Context, spec OneShotSpec) (OneShotResult, error) {
	binds := make([]string, 0, len(spec.Volumes))
	for vol, path := range spec.Volumes {
		if err := c.EnsureVolume(ctx, vol); err != nil {
			return OneShotResult{}, err
		}
		binds = append(binds, vol+":"+path)
	}
	sort.Strings(binds)
	hc := &container.HostConfig{
		Binds:         binds,
		CapDrop:       []string{"ALL"},
		SecurityOpt:   []string{securityOptNoNewPrivileges},
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyDisabled},
		Resources:     container.Resources{Memory: spec.MemoryBytes, MemorySwap: spec.MemoryBytes, NanoCPUs: spec.NanoCPUs},
	}
	if spec.PidsLimit > 0 {
		pids := spec.PidsLimit
		hc.PidsLimit = &pids
	}
	cfg := &container.Config{Image: spec.Image, Cmd: spec.Command, Env: toDockerEnv(spec.Env), Labels: c.withInstanceLabel(spec.Labels)}
	resp, err := c.cli.ContainerCreate(ctx, cfg, hc, nil, nil, spec.Name)
	if err != nil {
		return OneShotResult{}, fmt.Errorf("docker: create one-shot container %q: %w", spec.Name, err)
	}
	defer c.removeQuietly(resp.ID)

	for path, data := range spec.Files {
		archive, err := singleFileTar(path, data)
		if err != nil {
			return OneShotResult{}, err
		}
		if err := c.cli.CopyToContainer(ctx, resp.ID, "/", archive, container.CopyToContainerOptions{}); err != nil {
			return OneShotResult{}, fmt.Errorf("docker: copy %q into one-shot container %q: %w", path, spec.Name, err)
		}
	}
	waitCh, errCh := c.cli.ContainerWait(ctx, resp.ID, container.WaitConditionNextExit)
	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return OneShotResult{}, fmt.Errorf("docker: start one-shot container %q: %w", spec.Name, err)
	}
	var exit int64
	select {
	case w := <-waitCh:
		exit = w.StatusCode
	case err := <-errCh:
		return OneShotResult{}, fmt.Errorf("docker: wait for one-shot container %q: %w", spec.Name, err)
	case <-ctx.Done():
		return OneShotResult{}, fmt.Errorf("docker: one-shot container %q: %w", spec.Name, ctx.Err())
	}

	logs, err := c.cli.ContainerLogs(ctx, resp.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return OneShotResult{}, fmt.Errorf("docker: read one-shot container %q logs: %w", spec.Name, err)
	}
	defer func() { _ = logs.Close() }()
	limit := spec.MaxOutput
	if limit <= 0 {
		limit = defaultOneShotOutput
	}
	stdout, stderr := &cappedBuffer{limit: int(limit)}, &cappedBuffer{limit: int(limit)}
	if _, err := stdcopy.StdCopy(stdout, stderr, logs); err != nil {
		return OneShotResult{}, fmt.Errorf("docker: demultiplex one-shot container %q logs: %w", spec.Name, err)
	}
	return OneShotResult{ExitCode: exit, Stdout: stdout.buf.Bytes(), Stderr: stderr.buf.Bytes()}, nil
}

func (c *Client) removeQuietly(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = c.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true, RemoveVolumes: true}) // best effort; the orphan sweep is the backstop
}

func singleFileTar(path string, data []byte) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	name := path
	if len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
		return nil, fmt.Errorf("docker: tar header for %q: %w", path, err)
	}
	if _, err := tw.Write(data); err != nil {
		return nil, fmt.Errorf("docker: tar body for %q: %w", path, err)
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("docker: tar close for %q: %w", path, err)
	}
	return &buf, nil
}
