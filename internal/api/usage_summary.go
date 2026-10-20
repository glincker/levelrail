package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	usageOwnerApp      = "app"
	usageOwnerDatabase = "database"
	usageOwnerOther    = "other"
)

// backupUsageSource is the optional store capability the usage summary
// needs; a BackupHistoryStore without it simply reports no backup usage.
type backupUsageSource interface {
	SumBackupUsage(ctx context.Context) ([]store.BackupUsage, error)
}

type databaseUsageResource struct {
	Name             string   `json:"name"`
	CPUPercent       *float64 `json:"cpu_percent,omitempty"`
	MemoryUsageBytes *float64 `json:"memory_usage_bytes,omitempty"`
	MemoryLimitBytes *float64 `json:"memory_limit_bytes,omitempty"`
}

type volumeUsageItem struct {
	Name      string `json:"name"`
	OwnerKind string `json:"owner_kind"`
	Owner     string `json:"owner,omitempty"`
	SizeBytes *int64 `json:"size_bytes,omitempty"`
}

// volumeUsageResource covers the control plane's own Docker daemon only;
// volumes on other nodes are not measured, and the UI says so.
type volumeUsageResource struct {
	Scope         string            `json:"scope"`
	TotalBytes    int64             `json:"total_bytes"`
	UnknownCount  int               `json:"unknown_count"`
	Items         []volumeUsageItem `json:"items"`
	NotConfigured bool              `json:"not_configured,omitempty"`
}

type backupUsageItem struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Count int64  `json:"count"`
	Bytes int64  `json:"bytes"`
}

type backupUsageResource struct {
	TotalBytes int64             `json:"total_bytes"`
	Count      int64             `json:"count"`
	Items      []backupUsageItem `json:"items"`
}

type usageSummaryResponse struct {
	// LocalCPUCores is the control plane host's core count; other nodes do
	// not report cores, so it is a local capacity, not a fleet total.
	LocalCPUCores int                     `json:"local_cpu_cores"`
	Databases     []databaseUsageResource `json:"databases"`
	Volumes       volumeUsageResource     `json:"volumes"`
	Backups       backupUsageResource     `json:"backups"`
}

// handleUsageSummary handles GET /api/v1/usage/summary: the numbers the
// dashboard needs that no per-resource route returns in one call, the
// latest database CPU and memory readings, local named-volume sizes, and
// stored backup bytes per resource.
func (rt *Router) handleUsageSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	canSeeApp, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: usage summary: app visibility", err)
		return
	}
	canSeeDB, err := rt.databaseVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: usage summary: database visibility", err)
		return
	}
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		rt.internalError(w, "api: usage summary: list databases", err)
		return
	}

	cores := rt.doctorNumCPU
	if cores == nil {
		cores = runtime.NumCPU
	}
	out := usageSummaryResponse{
		LocalCPUCores: cores(),
		Databases:     rt.databaseUsage(ctx, dbs, canSeeDB),
		Volumes:       volumeUsageResource{Scope: "local", Items: []volumeUsageItem{}},
		Backups:       backupUsageResource{Items: []backupUsageItem{}},
	}
	if err := rt.fillVolumeUsage(ctx, &out.Volumes, canSeeApp, canSeeDB); err != nil {
		rt.logger.Warn("api: usage summary: volumes unavailable", slog.String("error", err.Error()))
		out.Volumes.NotConfigured = true
	}
	if err := rt.fillBackupUsage(ctx, &out.Backups, canSeeApp, canSeeDB); err != nil {
		rt.internalError(w, "api: usage summary: backups", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) databaseUsage(ctx context.Context, dbs []store.DesiredDatabase, canSee func(string) bool) []databaseUsageResource {
	byName := make(map[string]*databaseUsageResource, len(dbs))
	out := make([]databaseUsageResource, 0, len(dbs))
	for _, d := range dbs {
		if canSee(d.Name) {
			out = append(out, databaseUsageResource{Name: d.Name})
		}
	}
	for i := range out {
		byName[out[i].Name] = &out[i]
	}
	if rt.telemetry == nil {
		return out
	}
	for _, metric := range resourceUsageMetrics {
		samples, err := rt.telemetry.LatestByMetric(ctx, metric)
		if err != nil {
			rt.logger.Warn("api: usage summary: query metric failed", slog.String("metric", metric), slog.String("error", err.Error()))
			continue
		}
		for _, s := range samples {
			name, ok := strings.CutPrefix(s.ResourceID, resourcePrefixDatabase)
			if !ok {
				continue
			}
			u := byName[name]
			if u == nil {
				continue
			}
			v := s.Value
			switch metric {
			case "cpu_percent":
				u.CPUPercent = &v
			case "memory_usage_bytes":
				u.MemoryUsageBytes = &v
			case "memory_limit_bytes":
				u.MemoryLimitBytes = &v
			}
		}
	}
	return out
}

func (rt *Router) fillVolumeUsage(ctx context.Context, res *volumeUsageResource, canSeeApp, canSeeDB func(string) bool) error {
	if rt.orphanedVolumes == nil {
		res.NotConfigured = true
		return nil
	}
	vols, err := rt.orphanedVolumes.ListNamedVolumes(ctx)
	if err != nil {
		return fmt.Errorf("list named volumes: %w", err)
	}
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return fmt.Errorf("list desired services: %w", err)
	}
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return fmt.Errorf("list desired databases: %w", err)
	}
	owners := map[string][2]string{}
	for _, svc := range services {
		for _, v := range svc.Volumes {
			owners[v.Name] = [2]string{usageOwnerApp, svc.Name}
		}
	}
	for _, d := range dbs {
		for _, suffix := range []string{"-data", "-certs", "-wal-archive"} {
			owners["db-"+d.Name+suffix] = [2]string{usageOwnerDatabase, d.Name}
		}
	}
	for _, v := range vols {
		o, known := owners[v.Name]
		kind, owner := usageOwnerOther, ""
		if known {
			kind, owner = o[0], o[1]
			if kind == usageOwnerApp && !canSeeApp(owner) || kind == usageOwnerDatabase && !canSeeDB(owner) {
				continue
			}
		}
		item := volumeUsageItem{Name: v.Name, OwnerKind: kind, Owner: owner}
		if v.SizeBytes >= 0 {
			size := v.SizeBytes
			item.SizeBytes = &size
			res.TotalBytes += size
		} else {
			res.UnknownCount++
		}
		res.Items = append(res.Items, item)
	}
	sort.Slice(res.Items, func(i, j int) bool {
		return sizeOrZero(res.Items[i].SizeBytes) > sizeOrZero(res.Items[j].SizeBytes)
	})
	return nil
}

func sizeOrZero(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func (rt *Router) fillBackupUsage(ctx context.Context, res *backupUsageResource, canSeeApp, canSeeDB func(string) bool) error {
	src, ok := rt.backupHistory.(backupUsageSource)
	if !ok {
		return nil
	}
	rows, err := src.SumBackupUsage(ctx)
	if err != nil {
		return fmt.Errorf("sum backup usage: %w", err)
	}
	for _, u := range rows {
		item := backupUsageItem{Count: u.Count, Bytes: u.Bytes}
		if u.ResourceKind == store.BackupResourceKindVolume {
			if !canSeeApp(u.ServiceName) {
				continue
			}
			item.Kind, item.Name = usageOwnerApp, u.ServiceName+"/"+u.VolumeName
		} else {
			if !canSeeDB(u.DatabaseName) {
				continue
			}
			item.Kind, item.Name = usageOwnerDatabase, u.DatabaseName
		}
		res.TotalBytes += u.Bytes
		res.Count += u.Count
		res.Items = append(res.Items, item)
	}
	sort.Slice(res.Items, func(i, j int) bool { return res.Items[i].Bytes > res.Items[j].Bytes })
	return nil
}
