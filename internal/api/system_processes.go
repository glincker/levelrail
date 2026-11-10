package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

type hostProcess struct {
	PID      int    `json:"pid"`
	Name     string `json:"name"`
	RSSBytes int64  `json:"rss_bytes"`
	Threads  int    `json:"threads"`
}

type hostDockerDisk struct {
	Images       int64 `json:"images_bytes"`
	Containers   int64 `json:"containers_bytes"`
	Volumes      int64 `json:"volumes_bytes"`
	BuildCache   int64 `json:"build_cache_bytes"`
	ReclaimBytes int64 `json:"reclaimable_bytes"`
}

type systemProcessesResponse struct {
	Supported bool            `json:"supported"`
	Total     int             `json:"total"`
	Processes []hostProcess   `json:"processes"`
	Docker    *hostDockerDisk `json:"docker_disk,omitempty"`
}

// handleGetSystemProcesses lists the largest processes on the control plane host.
// It reads /proc only, never node agents, and returns a bounded row count.
func (rt *Router) handleGetSystemProcesses(w http.ResponseWriter, r *http.Request) {
	res := systemProcessesResponse{Processes: []hostProcess{}}
	if runtime.GOOS == "linux" {
		res.Supported = true
		limit := rt.runtimeSettings.Limit(r.Context(), runtimesettings.KeySystemProcessesRows)
		res.Processes, res.Total = readHostProcesses("/proc", limit)
	}
	if rt.dockerDiskUsage != nil {
		if u, err := rt.dockerDiskUsage.DiskUsage(r.Context()); err == nil {
			res.Docker = &hostDockerDisk{
				Images: u.ImagesTotalBytes, Containers: u.ContainersTotalBytes,
				Volumes: u.VolumesTotalBytes, BuildCache: u.BuildCacheTotalBytes,
				ReclaimBytes: u.ImagesReclaimableBytes + u.ContainersReclaimableBytes + u.BuildCacheReclaimableBytes,
			}
		}
	}
	writeJSON(w, http.StatusOK, res)
}

// readHostProcesses parses status files under root. Command lines are never
// read, since arguments can carry secrets.
func readHostProcesses(root string, limit int) ([]hostProcess, int) {
	dirs, _ := filepath.Glob(filepath.Join(root, "[0-9]*"))
	out := make([]hostProcess, 0, len(dirs))
	for _, d := range dirs {
		pid, err := strconv.Atoi(filepath.Base(d))
		if err != nil {
			continue
		}
		// d comes from a numeric-only glob of the proc root, not from input.
		raw, err := os.ReadFile(filepath.Join(d, "status")) //nolint:gosec // numeric proc entry
		if err != nil {
			continue
		}
		p := hostProcess{PID: pid}
		for _, line := range strings.Split(string(raw), "\n") {
			k, v, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			v = strings.TrimSpace(v)
			switch k {
			case "Name":
				p.Name = v
			case "Threads":
				p.Threads, _ = strconv.Atoi(v)
			case "VmRSS":
				kb, _ := strconv.ParseInt(strings.Fields(v + " ")[0], 10, 64)
				p.RSSBytes = kb << 10
			}
		}
		out = append(out, p)
	}
	total := len(out)
	sort.Slice(out, func(i, j int) bool { return out[i].RSSBytes > out[j].RSSBytes })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, total
}
