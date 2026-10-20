package fixit

import (
	"context"
	"fmt"
)

// DiskReclaim is what a cleanup would free, by category.
type DiskReclaim struct {
	ContainersBytes int64
	ImagesBytes     int64
	VolumesBytes    int64
	BuildCacheBytes int64
	OrphanNetworks  []string
}

// DiskDeps are the facts and the pruner the disk fix uses.
type DiskDeps struct {
	Free        func(ctx context.Context) (free, total int64, err error)
	Reclaimable func(ctx context.Context) (DiskReclaim, error)
	Prune       func(ctx context.Context) (freed int64, err error)
	// WarnFreePercent is the free-space percentage below which the check fails.
	WarnFreePercent float64
}

// DiskFix routes the "Clean up now" action through the engine.
func DiskFix(d DiskDeps) Fix {
	return Fix{
		ID: "disk.reclaim", CheckID: "disk_space", Subject: "disk", Kind: KindAuto,
		Title: "Clean up unused Docker data",
		Why:   "Builds and deploys fail when the data volume fills. Unreferenced images, build cache and stopped containers can be removed safely.",
		Risk:  RiskSafe, Reversible: false, Needs: "root",
		Detect: func(ctx context.Context) (Applicability, error) {
			free, total, err := d.Free(ctx)
			if err != nil {
				return Applicability{}, err
			}
			if total <= 0 {
				return Applicability{Detail: "disk size unknown", Changes: []Change{}}, nil
			}
			pct := float64(free) * 100 / float64(total)
			a := Applicability{Failing: pct < d.WarnFreePercent, Changes: []Change{}}
			a.Detail = fmt.Sprintf("%d MiB free (%.1f%%) on the data volume", free>>20, pct)
			if !a.Failing {
				return a, nil
			}
			a.Signature = "disk_low"
			r, err := d.Reclaimable(ctx)
			if err != nil {
				return Applicability{}, err
			}
			add := func(label string, b int64) {
				if b > 0 {
					a.Changes = append(a.Changes, Change{Label: label, Bytes: b})
				}
			}
			add("Stopped containers not managed by an app", r.ContainersBytes)
			add("Dangling images (rollback targets are kept)", r.ImagesBytes)
			add("Anonymous volumes", r.VolumesBytes)
			add("Build cache", r.BuildCacheBytes)
			if n := len(r.OrphanNetworks); n > 0 {
				a.Changes = append(a.Changes, Change{Label: "Orphaned networks", Detail: fmt.Sprintf("%d network(s)", n)})
			}
			if len(a.Changes) == 0 {
				a.Kind = KindManual
				a.Reason = "nothing is reclaimable by cleanup; the space is used by volumes, logs or backups"
			}
			return a, nil
		},
		Run: func(ctx context.Context, _ Input) error {
			_, err := d.Prune(ctx)
			return err
		},
	}
}

// NetworkPoolDeps are the facts and the pruner the pool fix uses.
type NetworkPoolDeps struct {
	Usage func(ctx context.Context) (enabled bool, inUse, capacity int, err error)
	// Orphans lists networks a prune would remove.
	Orphans func(ctx context.Context) ([]string, error)
	Prune   func(ctx context.Context) (removed []string, err error)
	// WarnPercent is the usage above which the check fails.
	WarnPercent int
	// PoolEnv names the env var that widens the pool.
	PoolEnv string
}

// NetworkPoolFix prunes orphaned app networks when the subnet pool runs high.
func NetworkPoolFix(d NetworkPoolDeps) Fix {
	return Fix{
		ID: "docker.network_pool.prune", CheckID: "network_pool", Subject: "docker", Kind: KindAuto,
		Title: "Remove orphaned app networks",
		Why:   "Each app network takes a subnet from a finite pool. Networks whose app is gone still hold theirs.",
		Risk:  RiskSafe, Reversible: false, Needs: "root",
		Detect: func(ctx context.Context) (Applicability, error) {
			en, in, capy, err := d.Usage(ctx)
			if err != nil {
				return Applicability{}, err
			}
			if !en || capy <= 0 {
				return Applicability{Detail: "explicit subnet pool disabled", Changes: []Change{}}, nil
			}
			pct := in * 100 / capy
			a := Applicability{Failing: pct > d.WarnPercent, Changes: []Change{}}
			a.Detail = fmt.Sprintf("%d of %d networks in use (%d%%)", in, capy, pct)
			if !a.Failing {
				return a, nil
			}
			a.Signature = "pool_high"
			names, err := d.Orphans(ctx)
			if err != nil {
				return Applicability{}, err
			}
			for _, n := range names {
				a.Changes = append(a.Changes, Change{Label: "Remove network", Detail: n})
			}
			if len(names) == 0 {
				a.Kind = KindManual
				a.Reason = "no orphaned networks to remove; widen the pool with " + d.PoolEnv
				a.Command = "# set " + d.PoolEnv + " to a larger CIDR, then restart the control plane"
			}
			return a, nil
		},
		Run: func(ctx context.Context, _ Input) error {
			_, err := d.Prune(ctx)
			return err
		},
	}
}
