package build

import (
	"context"
	"fmt"
	"strings"

	bkclient "github.com/moby/buildkit/client"
)

// CacheBudget is the builder cache size a post build prune keeps the cache
// under. It mirrors cachebudget.Budget so this package stays free of that import.
type CacheBudget struct {
	ReservedBytes int64
	MaxUsedBytes  int64
}

// CacheBudgetEngine describes the builder's engine for sizing the cache budget.
type CacheBudgetEngine struct {
	UsedBytes int64
	// Desktop is true for Docker Desktop, whose VM disk the host cannot stat.
	Desktop bool
	RootDir string
}

// CacheBudgetPrune reports one prune down to the cache budget.
type CacheBudgetPrune struct {
	UsedBytes  int64
	FreedBytes int64
	Removed    int
}

// CacheBudgetEngine reads the cache size and engine kind in one call.
func (c *Client) CacheBudgetEngine(ctx context.Context) (CacheBudgetEngine, error) {
	if c == nil || c.bk == nil {
		return CacheBudgetEngine{}, ErrAppCacheNoBuilder
	}
	du, err := c.bk.DiskUsage(ctx)
	if err != nil {
		return CacheBudgetEngine{}, fmt.Errorf("build: disk usage: %w", err)
	}
	var out CacheBudgetEngine
	for _, r := range du {
		out.UsedBytes += r.Size
	}
	if c.docker != nil {
		if info, ierr := c.docker.Info(ctx); ierr == nil {
			out.Desktop = IsDesktopEngine(info.OperatingSystem)
			out.RootDir = info.DockerRootDir
		}
	}
	return out, nil
}

// IsDesktopEngine matches the operating system name Docker Desktop reports.
func IsDesktopEngine(osName string) bool {
	return strings.Contains(strings.ToLower(osName), "docker desktop")
}

// PruneCacheToBudget prunes the builder cache down to b. Min free stays 0
// because the engine's disk may not be the one this process can stat, and
// records held by a running build are never removed by BuildKit.
func (c *Client) PruneCacheToBudget(ctx context.Context, b CacheBudget) (CacheBudgetPrune, error) {
	if c == nil || c.bk == nil {
		return CacheBudgetPrune{}, ErrAppCacheNoBuilder
	}
	return pruneToBudget(ctx, c.bk, b)
}

type budgetPruner interface {
	Prune(ctx context.Context, ch chan bkclient.UsageInfo, opts ...bkclient.PruneOption) error
}

func pruneToBudget(ctx context.Context, bk budgetPruner, b CacheBudget) (CacheBudgetPrune, error) {
	var res CacheBudgetPrune
	if b.MaxUsedBytes <= 0 {
		return res, nil
	}
	ch := make(chan bkclient.UsageInfo)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for u := range ch {
			res.FreedBytes += u.Size
			res.Removed++
		}
	}()
	err := bk.Prune(ctx, ch, bkclient.PruneAll, bkclient.WithKeepOpt(0, b.ReservedBytes, b.MaxUsedBytes, 0))
	close(ch)
	<-done
	if err != nil {
		return res, fmt.Errorf("build: prune cache to budget: %w", err)
	}
	return res, nil
}
