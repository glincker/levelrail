package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/diskspace"
	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/netguard"
	"github.com/GLINCKER/levelrail/internal/preflight"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	preflightBodyLimit = 1 << 20
	dockerRootDir      = "/var/lib/docker"
	portDialTimeout    = time.Second
)

var (
	preflightImagesOnce sync.Once
	preflightImages     *preflight.RegistryInspector
)

func preflightImageInspector() *preflight.RegistryInspector {
	preflightImagesOnce.Do(func() {
		preflightImages = &preflight.RegistryInspector{Client: netguard.NewClient(), TTL: preflight.LimitsFromEnv().ImageCacheTTL}
	})
	return preflightImages
}

type preflightRequestBody struct {
	Name        string   `json:"name"`
	NodeID      string   `json:"node_id"`
	Image       string   `json:"image"`
	Port        int      `json:"port"`
	HostPort    int      `json:"host_port"`
	Domains     []string `json:"domains"`
	MemoryBytes int64    `json:"memory_bytes"`
	RequiredEnv []string `json:"required_env"`
	EnvKeys     []string `json:"env_keys"`
	BindMounts  []string `json:"bind_mounts"`
	VolumePaths []string `json:"volume_paths"`
	GPU         bool     `json:"gpu"`
	GitURL      string   `json:"git_url"`
	GitBranch   string   `json:"git_branch"`
}

// handlePreflightNew handles POST /api/v1/preflight: checks for an app that
// does not exist yet, described entirely by the request body.
func (rt *Router) handlePreflightNew(w http.ResponseWriter, r *http.Request) {
	var body preflightRequestBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, preflightBodyLimit)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req := preflight.Request{
		Name: body.Name, NodeID: body.NodeID, Image: body.Image, Port: body.Port, HostPort: body.HostPort,
		Domains: body.Domains, MemoryBytes: body.MemoryBytes, RequiredEnv: body.RequiredEnv, EnvKeys: body.EnvKeys,
		BindMounts: body.BindMounts, VolumePaths: body.VolumePaths, GPU: body.GPU, GitURL: body.GitURL, GitBranch: body.GitBranch,
	}
	writeJSON(w, http.StatusOK, preflight.Run(r.Context(), req, rt.preflightEnv(req, false)))
}

// handlePreflightApp handles POST /api/v1/apps/{name}/preflight: checks an
// existing app's stored configuration. An optional body may pass
// required_env, since the required set is not stored.
func (rt *Router) handlePreflightApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()

	svc, err := rt.apps.GetDesiredService(ctx, name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: preflight app: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	var body preflightRequestBody
	if r.ContentLength != 0 {
		if derr := json.NewDecoder(http.MaxBytesReader(w, r.Body, preflightBodyLimit)).Decode(&body); derr != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	req := rt.preflightRequestFor(ctx, svc)
	req.RequiredEnv = append(req.RequiredEnv, body.RequiredEnv...)
	writeJSON(w, http.StatusOK, preflight.Run(ctx, req, rt.preflightEnv(req, true)))
}

func (rt *Router) preflightRequestFor(ctx context.Context, svc *store.DesiredService) preflight.Request {
	req := preflight.Request{
		Name: svc.Name, NodeID: svc.NodeID, Image: svc.Image, Port: svc.Port,
		Domains: svc.Domains, EnvKeys: configuredEnvKeys(svc),
	}
	if svc.HostPort != nil {
		req.HostPort = *svc.HostPort
	}
	for k, v := range svc.Env {
		if v == "" {
			req.RequiredEnv = append(req.RequiredEnv, k)
		}
	}
	if svc.Resources != nil {
		req.MemoryBytes = svc.Resources.MemoryBytes
		req.GPU = svc.Resources.GPU != nil
	}
	for _, bm := range svc.BindMounts {
		req.BindMounts = append(req.BindMounts, bm.HostPath)
		req.VolumePaths = append(req.VolumePaths, bm.ContainerPath)
	}
	for _, v := range svc.Volumes {
		req.VolumePaths = append(req.VolumePaths, v.ContainerPath)
	}
	if rt.gitSources != nil {
		if gs, err := rt.gitSources.GetGitSource(ctx, svc.Name); err == nil && gs != nil {
			req.GitURL, req.GitBranch = gs.RepoURL, gs.Branch
		}
	}
	return req
}

// preflightEnv wires the probes. Disk and memory are only known for this
// control plane's own node. When existing is true the app's own container may
// legitimately hold its pinned host port, so only other apps count.
func (rt *Router) preflightEnv(req preflight.Request, existing bool) preflight.Env {
	lim := preflight.LimitsFromEnv()
	env := preflight.Env{
		Limits: lim,
		Image:  preflightImageInspector(),
		Git:    &preflight.SmartHTTPChecker{Client: netguard.NewClient()},
		LookupHost: func(ctx context.Context, host string) ([]string, error) {
			return net.DefaultResolver.LookupHost(ctx, host)
		},
		PublicIP: func(ctx context.Context) (string, error) {
			endpoint := rt.doctorPublicIPEndpoint
			if endpoint == "" {
				endpoint = defaultDoctorPublicIPEndpoint
			}
			return doctorFetchPublicIP(ctx, rt.doctorHTTPClientOrDefault(), endpoint)
		},
		HostPortHolder: func(ctx context.Context, nodeID string, port int) (string, bool, error) {
			return rt.hostPortHolder(ctx, req.Name, nodeID, port, existing)
		},
		GPUFit: func(ctx context.Context, nodeID string) error {
			p, err := rt.newGPUPlanner(ctx)
			if err != nil || p == nil {
				return fmt.Errorf("gpu inventory is unavailable")
			}
			return p.fit(nodeID, gpu.Claim{Name: req.Name})
		},
	}
	if rt.isLocalNode(req.NodeID) {
		env.FreeDisk = func(_ context.Context, _ string) (int64, error) {
			free, err := diskspace.Free(dockerRootDir)
			if err != nil {
				return diskspace.Free("/")
			}
			return free, nil
		}
		env.Memory = func(_ context.Context, _ string) (int64, int64, error) { return telemetry.HostMemoryBytes() }
	}
	return env
}

func (rt *Router) hostPortHolder(ctx context.Context, self, nodeID string, port int, existing bool) (string, bool, error) {
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return "", false, fmt.Errorf("list apps: %w", err)
	}
	for _, s := range svcs {
		if s.Name != self && s.HostPort != nil && *s.HostPort == port && s.NodeID == nodeID {
			return "app " + s.Name, true, nil
		}
	}
	if existing || !rt.isLocalNode(nodeID) {
		return "", false, nil
	}
	d := net.Dialer{Timeout: portDialTimeout}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", fmt.Sprint(port)))
	if err != nil {
		return "", false, nil
	}
	_ = conn.Close()
	return "a process on this host", true, nil
}
