package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockerlint"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	maxDockerLintBody = 256 << 10
	dockerHealthProbe = 5 * time.Second

	dockerDiskWarnPercent = 80
)

func (rt *Router) registerDockerIntegrationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/docker/lint", rt.requireAbility(AbilityRead, rt.handleDockerLint))
	mux.HandleFunc("GET /api/v1/docker/health", rt.requireAbility(AbilityRead, rt.handleDockerHealth))
	mux.HandleFunc("GET /api/v1/apps/{name}/image-status", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleImageStatus))
}

type dockerLintRequest struct {
	Dockerfile      string `json:"dockerfile"`
	HasDockerignore bool   `json:"has_dockerignore"`
}

func (rt *Router) handleDockerLint(w http.ResponseWriter, r *http.Request) {
	var req dockerLintRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxDockerLintBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Dockerfile) == "" {
		writeError(w, http.StatusBadRequest, "dockerfile is required")
		return
	}
	writeJSON(w, http.StatusOK, dockerlint.Lint(req.Dockerfile, req.HasDockerignore))
}

type dockerHealthCheck struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Message string `json:"message"`
	// FixAction names a UI action: "free_space" or "recheck". Empty when nothing to do.
	FixAction string `json:"fix_action,omitempty"`
}

type dockerHealthResource struct {
	Reachable     bool                `json:"reachable"`
	Version       string              `json:"version,omitempty"`
	Arch          string              `json:"arch,omitempty"`
	OS            string              `json:"os,omitempty"`
	RuntimeKind   string              `json:"runtime_kind,omitempty"`
	Rootless      bool                `json:"rootless"`
	DiskTotal     int64               `json:"disk_total_bytes"`
	DiskReclaim   int64               `json:"disk_reclaimable_bytes"`
	CheckedAt     time.Time           `json:"checked_at"`
	Checks        []dockerHealthCheck `json:"checks"`
	ProbeDuration int64               `json:"probe_ms"`
	// Unresponsive is true when the daemon accepted no call within the guard timeout.
	Unresponsive      bool       `json:"unresponsive"`
	UnresponsiveSince *time.Time `json:"unresponsive_since,omitempty"`
}

type unresponsiveReporter interface {
	UnresponsiveSince() time.Time
}

const dockerNotRespondingMessage = "Docker is not responding. Deploys and containers are paused."

func (rt *Router) markDockerUnresponsive(res *dockerHealthResource, err error) {
	var since time.Time
	if rep, ok := rt.dockerPinger.(unresponsiveReporter); ok {
		since = rep.UnresponsiveSince()
	}
	if since.IsZero() && !errors.Is(err, docker.ErrDaemonUnresponsive) {
		return
	}
	res.Unresponsive = true
	if !since.IsZero() {
		u := since.UTC()
		res.UnresponsiveSince = &u
	}
}

func (rt *Router) handleDockerHealth(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), dockerHealthProbe)
	defer cancel()

	res := dockerHealthResource{CheckedAt: started.UTC(), Checks: []dockerHealthCheck{}}
	if rt.dockerPinger == nil {
		res.Checks = append(res.Checks, dockerHealthCheck{Code: "api", Status: doctorStatusFail, Message: "No Docker client is configured on this control plane."})
		writeJSON(w, http.StatusOK, res)
		return
	}
	if err := rt.dockerPinger.Ping(ctx); err != nil {
		rt.logger.Warn("api: docker health ping failed", slog.String("error", err.Error()))
		msg := "The Docker Engine API did not answer. Check that the daemon is running, then re-check."
		rt.markDockerUnresponsive(&res, err)
		if res.Unresponsive {
			msg = dockerNotRespondingMessage
		}
		res.Checks = append(res.Checks, dockerHealthCheck{Code: "api", Status: doctorStatusFail, Message: msg, FixAction: "recheck"})
		writeJSON(w, http.StatusOK, res)
		return
	}
	res.Reachable = true
	res.Checks = append(res.Checks, dockerHealthCheck{Code: "api", Status: doctorStatusOK, Message: "The Docker Engine API answers."})

	if v, ok := rt.dockerPinger.(engineVersioner); ok {
		if version, err := v.ServerVersion(ctx); err == nil {
			res.Version = version
			res.Checks = append(res.Checks, dockerHealthCheck{Code: "version", Status: doctorStatusOK, Message: "Engine version " + version + "."})
		}
	}
	if rep, ok := rt.dockerPinger.(runtimeReporter); ok {
		info := rep.Runtime()
		res.RuntimeKind, res.Rootless = string(info.Kind), info.Rootless
	}
	if insp, ok := rt.dockerPinger.(docker.ImageDetailInspector); ok {
		if p, err := insp.NodePlatform(ctx); err == nil {
			res.Arch, res.OS = p.Arch, p.OS
		}
	}
	rt.addDockerDiskCheck(ctx, &res)
	res.ProbeDuration = time.Since(started).Milliseconds()
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) addDockerDiskCheck(ctx context.Context, res *dockerHealthResource) {
	if rt.dockerDiskUsage == nil {
		return
	}
	u, err := rt.dockerDiskUsage.DiskUsage(ctx)
	if err != nil {
		res.Checks = append(res.Checks, dockerHealthCheck{Code: "disk", Status: doctorStatusWarn, Message: "Docker disk usage could not be read.", FixAction: "recheck"})
		return
	}
	res.DiskTotal = u.ImagesTotalBytes + u.ContainersTotalBytes + u.VolumesTotalBytes + u.BuildCacheTotalBytes
	res.DiskReclaim = u.ImagesReclaimableBytes + u.ContainersReclaimableBytes + u.BuildCacheReclaimableBytes
	check := dockerHealthCheck{Code: "disk", Status: doctorStatusOK, Message: "Docker storage is within normal range."}
	if res.DiskTotal > 0 && res.DiskReclaim*100/res.DiskTotal >= dockerDiskWarnPercent {
		check.Status = doctorStatusWarn
		check.Message = "Most of Docker's storage is reclaimable (unused images, stopped containers, build cache)."
		check.FixAction = "free_space"
	}
	res.Checks = append(res.Checks, check)
}

type imageStatusResource struct {
	Image           string `json:"image"`
	Source          string `json:"source"`
	CurrentDigest   string `json:"current_digest,omitempty"`
	LatestDigest    string `json:"latest_digest,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	ImageArch       string `json:"image_arch,omitempty"`
	NodeArch        string `json:"node_arch,omitempty"`
	ArchMismatch    bool   `json:"arch_mismatch"`
	Message         string `json:"message,omitempty"`
}

const (
	imageSourceRegistry = "registry"
	imageSourceBuilt    = "built"
)

// handleImageStatus reports, without deploying, whether the app's image tag moved and
// whether the image architecture matches the node.
func (rt *Router) handleImageStatus(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: image status: load app", err, slog.String("name", name))
		return
	}
	res := imageStatusResource{Image: svc.Image, Source: imageSourceRegistry, CurrentDigest: docker.ImageDigestOf(svc.Image)}
	if svc.ImageIDRef != "" {
		res.Source = imageSourceBuilt
		res.Message = "Built from source, so there is no registry tag to compare."
		writeJSON(w, http.StatusOK, res)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dockerHealthProbe*3)
	defer cancel()
	if insp, ok := rt.dockerPinger.(docker.ImageDetailInspector); ok {
		if d, err := insp.InspectImageDetails(ctx, svc.Image); err == nil {
			res.ImageArch = docker.NormalizeArch(d.Arch)
		}
		if p, err := insp.NodePlatform(ctx); err == nil {
			res.NodeArch = p.Arch
		}
		res.ArchMismatch = res.ImageArch != "" && res.NodeArch != "" && res.ImageArch != res.NodeArch
	}
	if rt.imageResolver != nil {
		tagRef := docker.UnpinImageRef(svc.Image)
		remote, err := rt.imageResolver.ResolveImage(ctx, tagRef, rt.imageTrigger(ctx, *svc, true, "image status").Auth, true)
		switch {
		case err != nil:
			res.Message = "The registry could not be reached, so update status is unknown."
		case remote.Digest != "":
			res.LatestDigest = remote.Digest
			res.UpdateAvailable = res.CurrentDigest != "" && remote.Digest != res.CurrentDigest
		}
	}
	writeJSON(w, http.StatusOK, res)
}
