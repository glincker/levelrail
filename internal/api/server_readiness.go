package api

import (
	"context"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/serverready"
)

var readinessDomainPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// daemonSecurer is implemented by the Docker client behind DockerPinger.
type daemonSecurer interface {
	DaemonSecurity(ctx context.Context) (docker.DaemonSecurity, error)
}

// readinessProbes returns the host probes with Docker answered by the
// control plane's own engine client, so no CLI or second socket is needed.
func (rt *Router) readinessProbes() serverready.Probes {
	p := serverready.HostProbes()
	p.DockerInfo = func(ctx context.Context) (serverready.DockerInfo, error) {
		var info serverready.DockerInfo
		versioner := rt.dockerEngineVersion()
		if versioner == nil {
			return info, os.ErrNotExist
		}
		v, err := versioner(ctx)
		if err != nil {
			return info, err
		}
		info.Version = v
		if sec, ok := rt.dockerPinger.(daemonSecurer); ok {
			if s, err := sec.DaemonSecurity(ctx); err == nil {
				info.Rootless = s.Rootless
			}
		}
		return info, nil
	}
	p.Published = func(ctx context.Context) ([]serverready.Listener, error) {
		var out []serverready.Listener
		for _, h := range rt.portHolders(ctx) {
			out = append(out, serverready.Listener{Addr: "0.0.0.0", Port: h.Port, Container: h.Container, Image: h.Image})
		}
		return out, nil
	}
	return p
}

// handleServerReadiness handles GET /api/v1/system/readiness: port owners,
// Docker, disk, memory, firewall and (with ?domain=) DNS, with the install mode
// and next step. Read-only.
func (rt *Router) handleServerReadiness(w http.ResponseWriter, r *http.Request) {
	domain := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain")))
	if domain != "" && !readinessDomainPattern.MatchString(domain) {
		writeError(w, http.StatusBadRequest, "domain must be a hostname such as console.example.com")
		return
	}
	facts := serverready.Collect(r.Context(), rt.readinessProbes(), serverready.Request{
		Domain:        domain,
		DataDir:       rt.dataDir,
		SelfProcesses: []string{rt.brand.BinaryName},
	})
	report := serverready.Assess(facts, serverready.Thresholds{
		MinDiskGB: envFloat("APP_READINESS_MIN_DISK_GB", 10), MinMemoryMB: int(envFloat("APP_READINESS_MIN_RAM_MB", 1024)),
		MinDockerMajor:  int(envFloat("APP_READINESS_MIN_DOCKER_MAJOR", 24)),
		InstallerPrefix: "curl -fsSL https://raw.githubusercontent.com/" + githubRepo + "/main/install.sh | sudo",
	})
	writeJSON(w, http.StatusOK, report)
}
