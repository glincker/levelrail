package api

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/kit/diskspace"
)

const (
	envDoctorDiskWarnFreePct     = "APP_DOCTOR_DISK_WARN_FREE_PCT"
	defaultDoctorDiskWarnFreePct = 10
)

type diskConsumer struct {
	name        string
	total       int64
	reclaimable int64
	cleanup     string
}

// doctorCheckDiskHygiene is read-only: it reports free space on the data dir and the Docker root,
// Docker's own storage sizes, and a cleanup command for the operator to run. It never deletes.
func (rt *Router) doctorCheckDiskHygiene(ctx context.Context) doctorCheckResource {
	const code, name = "disk_hygiene", "Disk hygiene"
	warnPct := envInt64(envDoctorDiskWarnFreePct, defaultDoctorDiskWarnFreePct)

	var parts []string
	low := false
	note := func(label, path string) {
		var st syscall.Statfs_t
		if path == "" || syscall.Statfs(path, &st) != nil || st.Blocks == 0 {
			return
		}
		free := int64(st.Bavail) * int64(st.Bsize) //nolint:gosec // statfs fields are non-negative
		pct := int64(st.Bavail * 100 / st.Blocks)  //nolint:gosec // percent is at most 100
		parts = append(parts, fmt.Sprintf("%s %s free (%d%%)", label, diskspace.HumanBytes(free), pct))
		if pct < warnPct {
			low = true
		}
	}
	note("data dir", rt.dataDir)
	if d, ok := rt.dockerPinger.(dockerRootDirer); ok {
		if root, err := d.DockerRootDir(ctx); err == nil {
			note("docker root", root)
		}
	}

	var consumers []diskConsumer
	if rt.dockerDiskUsage != nil {
		if u, err := rt.dockerDiskUsage.DiskUsage(ctx); err == nil {
			consumers = diskConsumers(u)
		}
	}
	if len(parts) == 0 && len(consumers) == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "no disk or Docker storage figures available"}
	}
	for _, c := range consumers {
		parts = append(parts, fmt.Sprintf("%s %s (%s reclaimable)", c.name, diskspace.HumanBytes(c.total), diskspace.HumanBytes(c.reclaimable)))
	}
	res := doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Evidence: strings.Join(parts, "; "), Message: "free space is above " + strconv.FormatInt(warnPct, 10) + "%"}
	if low {
		res.Status = doctorStatusWarn
		res.Message = "free space is below " + strconv.FormatInt(warnPct, 10) + "%"
		res.Fix = suggestedCleanup(consumers)
		res.DocsPath = "/troubleshooting#disk-space"
	}
	return res
}

// diskConsumers orders Docker's categories by size, largest first, so the top consumers lead.
func diskConsumers(u docker.DiskUsage) []diskConsumer {
	c := []diskConsumer{
		{"images", u.ImagesTotalBytes, u.ImagesReclaimableBytes, "docker image prune"},
		{"build cache", u.BuildCacheTotalBytes, u.BuildCacheReclaimableBytes, "docker builder prune --filter until=168h"},
		{"volumes", u.VolumesTotalBytes, u.VolumesReclaimableBytes, "docker volume ls -f dangling=true"},
		{"containers", u.ContainersTotalBytes, u.ContainersReclaimableBytes, "docker container prune"},
	}
	sort.SliceStable(c, func(i, j int) bool { return c[i].total > c[j].total })
	return c
}

// suggestedCleanup picks the safest command for the largest reclaimable category. Volumes are only listed, never pruned.
func suggestedCleanup(c []diskConsumer) string {
	best := -1
	for i, x := range c {
		if x.reclaimable > 0 && (best < 0 || x.reclaimable > c[best].reclaimable) {
			best = i
		}
	}
	if best < 0 {
		return "free space by moving Docker's data-root or the data directory to a larger disk"
	}
	return fmt.Sprintf("review then run (nothing is deleted by Levelrail): %s, to reclaim up to %s of %s", c[best].cleanup, diskspace.HumanBytes(c[best].reclaimable), c[best].name)
}
