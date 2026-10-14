package demo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Docker answers the API layer's narrow Docker-facing interfaces (pinger,
// image and container listers, disk usage) from seeded rows. It owns no
// socket and cannot reach a host.
type Docker struct {
	db  *store.DB
	now func() time.Time
}

// NewDocker builds the fake; now stamps container creation times.
func NewDocker(db *store.DB, now func() time.Time) *Docker {
	if now == nil {
		now = time.Now
	}
	return &Docker{db: db, now: now}
}

// Ping always succeeds so the doctor and status pages read as healthy.
func (*Docker) Ping(context.Context) error { return nil }

// ListImages returns the tags the seeded deploy history produced for repo.
func (d *Docker) ListImages(ctx context.Context, repo string) ([]docker.ImageInfo, error) {
	svcs, err := d.db.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("demo: list services: %w", err)
	}
	var out []docker.ImageInfo
	for _, svc := range svcs {
		base, _, _ := strings.Cut(svc.Image, ":")
		if repo != "" && base != repo {
			continue
		}
		attempts, err := d.db.ListDeployAttempts(ctx, svc.Name)
		if err != nil {
			return nil, fmt.Errorf("demo: list deploy attempts: %w", err)
		}
		for _, a := range attempts {
			out = append(out, docker.ImageInfo{Tag: a.Image, CreatedAt: a.StartedAt})
		}
	}
	return out, nil
}

// ListByPrefix reports one running container per seeded service whose name matches prefix.
func (d *Docker) ListByPrefix(ctx context.Context, prefix string) ([]docker.ContainerState, error) {
	svcs, err := d.db.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("demo: list services: %w", err)
	}
	var out []docker.ContainerState
	for _, svc := range svcs {
		if !strings.HasPrefix(svc.Name, prefix) {
			continue
		}
		out = append(out, docker.ContainerState{
			ID: "demo" + svc.Name, Name: svc.Name, Image: svc.Image,
			Created: d.now().Add(-72 * time.Hour), Running: !svc.Suspended,
		})
	}
	return out, nil
}

// DiskUsage returns fixed, plausible numbers.
func (*Docker) DiskUsage(context.Context) (docker.DiskUsage, error) {
	return docker.DiskUsage{
		ImagesTotalBytes: 6 << 30, ImagesReclaimableBytes: 1 << 30,
		ContainersTotalBytes: 300 << 20, VolumesTotalBytes: 4 << 30,
		BuildCacheTotalBytes: 2 << 30, BuildCacheReclaimableBytes: 1 << 30,
	}, nil
}
