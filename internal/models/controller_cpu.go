package models

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	cpuWeightsMount = "/models"
	bytesPerMiB     = 1 << 20
)

// Weights is the control plane managed weights downloader the controller
// drives for local CPU models.
type Weights interface {
	Ensure(spec WeightsSpec) DownloadState
	Dir(name string) string
	Forget(name string)
}

// createCPU creates the CPU engine container with hard memory and CPU caps,
// no swap, low CPU weight, a high OOM score and a pids limit. On the local
// node it mounts the weights this control plane downloaded, read-only.
func (c *Controller) createCPU(ctx context.Context, m *store.Model, node NodeInfo, target, image, hfToken string) (reconcile.Result, error) {
	if m.CPUMilli <= 0 || m.MemoryMiB <= 0 {
		return notReady("CPULimitsMissing", "this CPU model has no cpu or memory cap; redeploy it with limits"), nil
	}
	lim := CPULimits{CPUMilli: m.CPUMilli, MemoryMiB: m.MemoryMiB, Context: m.ContextLength}
	if lim.Context <= 0 {
		lim.Context = c.cpu.DefaultContext
	}
	spec := SpecFromModel(m)
	cs := docker.ContainerSpec{
		Name:  target,
		Image: image,
		Ports: []docker.PortBinding{{ContainerPort: engines[m.Engine].Port, HostIP: node.BindIP}},
		Resources: &docker.Resources{
			MemoryBytes:     int64(m.MemoryMiB) * bytesPerMiB,
			SwapMemoryBytes: int64(m.MemoryMiB) * bytesPerMiB,
			NanoCPUs:        int64(m.CPUMilli) * 1_000_000,
		},
		CPUShares:   c.cpu.CPUShares,
		OomScoreAdj: c.cpu.OomScoreAdj,
		PidsLimit:   c.cpu.PidsLimit,
	}
	embedding := false
	if cat, err := LocalCatalog(); err == nil {
		if e, ok := cat.FindWeights(m.WeightsRepo, m.WeightsFile); ok {
			embedding = e.Kind == KindEmbedding
		}
	}

	if node.Local && c.weights != nil && m.WeightsFile != "" {
		st := c.weights.Ensure(WeightsSpec{Name: m.Name, Repo: m.WeightsRepo, File: m.WeightsFile, SHA256: m.WeightsSHA256, Token: hfToken,
			Key: fmt.Sprintf("%d|%t", m.RestartNonce, hfToken != "")})
		if st.Phase != DLDone {
			return downloadResult(st), nil
		}
		cs.Env = Env(spec, "")
		cs.BindMounts = []docker.BindMount{{HostPath: c.weights.Dir(m.Name), ContainerPath: cpuWeightsMount, ReadOnly: true}}
		cs.Command = CPUCommand(spec, lim, c.cpu, cpuWeightsMount+"/"+m.WeightsFile, st.Total, embedding)
	} else {
		eng := engines[m.Engine]
		vol := VolumeName(c.prefix, m.Name)
		if err := c.runtime.EnsureVolume(ctx, vol); err != nil {
			return notReady("VolumeFailed", err.Error()), fmt.Errorf("models/%s: ensure volume %q: %w", c.name, vol, err)
		}
		cs.Env = Env(spec, hfToken)
		cs.Volumes = []docker.VolumeMount{{Name: vol, ContainerPath: eng.CachePath}}
		cs.Command = CPUCommand(spec, lim, c.cpu, "", 0, embedding)
	}

	id, err := c.runtime.Create(ctx, cs)
	if err != nil {
		slog.Warn("cpu model container create failed", slog.String("model", m.Name), slog.String("error", err.Error()))
		return notReady("CreateFailed", "the weights are on disk and are reused on retry: "+err.Error()), fmt.Errorf("models/%s: create %q: %w", c.name, target, err)
	}
	if err := c.runtime.Start(ctx, id); err != nil {
		slog.Warn("cpu model container start failed", slog.String("model", m.Name), slog.String("error", err.Error()))
		return notReady("StartFailed", "the weights are on disk and are reused on retry: "+err.Error()), fmt.Errorf("models/%s: start %q: %w", c.name, target, err)
	}
	return notReady("Starting", "engine container created and starting"), nil
}

func downloadResult(st DownloadState) reconcile.Result {
	if st.Err != nil && st.Phase == DLFailed {
		msg := st.Err.Message
		if st.Err.Retryable {
			msg += " (" + progressText(st.Done, st.Total) + " kept, retrying)"
		}
		return notReady("DownloadFailed", msg)
	}
	if st.Phase == DLVerifying {
		return notReady("Downloading", "verifying the download checksum")
	}
	return notReady("Downloading", "downloading weights from Hugging Face: "+progressText(st.Done, st.Total))
}

func progressText(done, total int64) string {
	if total <= 0 {
		return formatGiB(done)
	}
	return fmt.Sprintf("%s of %s (%d%%)", formatGiB(done), formatGiB(total), done*100/total)
}
