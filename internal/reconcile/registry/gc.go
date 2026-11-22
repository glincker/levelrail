package registry

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync/atomic"
)

const (
	gcConfigPath = "/etc/docker/registry/config.yml"
	gcBlobRoot   = dataPath + "/docker/registry/v2/blobs/sha256"
	gcEligible   = "blob eligible for deletion:"
	gcStatChunk  = 100
)

// ErrNotRunning means no registry container is available for a garbage collection pass.
var ErrNotRunning = errors.New("registry: the built-in registry is not running")

// GCReport is what a garbage collection pass found or removed in the registry data volume.
type GCReport struct {
	DryRun            bool  `json:"dry_run"`
	UnreferencedBlobs int   `json:"unreferenced_blobs"`
	UnreferencedBytes int64 `json:"unreferenced_bytes"`
}

// active is the controller that last reconciled, so other packages can reach the registry container.
var active atomic.Pointer[Controller]

// RunGC runs garbage collection through the last reconciled controller.
func RunGC(ctx context.Context, dryRun bool) (GCReport, error) {
	c := active.Load()
	if c == nil {
		return GCReport{DryRun: dryRun}, ErrNotRunning
	}
	return c.GarbageCollect(ctx, dryRun)
}

// GarbageCollect runs registry garbage-collect inside the container. A dry run only reports
// what is unreferenced. A real run frees it; pushes during that moment can lose blobs, so the
// caller schedules it for quiet times.
func (c *Controller) GarbageCollect(ctx context.Context, dryRun bool) (GCReport, error) {
	state, err := c.runtime.InspectByName(ctx, ContainerName(c.prefix))
	if err != nil {
		return GCReport{DryRun: dryRun}, fmt.Errorf("registry: inspect: %w", err)
	}
	if state == nil || !state.Running {
		return GCReport{DryRun: dryRun}, ErrNotRunning
	}
	digests, err := c.gcCandidates(ctx, state.ID)
	if err != nil {
		return GCReport{DryRun: dryRun}, err
	}
	rep := GCReport{DryRun: dryRun, UnreferencedBlobs: len(digests)}
	// Sizes are read before a real pass, since the blobs are gone afterwards.
	rep.UnreferencedBytes = c.blobBytes(ctx, state.ID, digests)
	if dryRun || len(digests) == 0 {
		return rep, nil
	}
	if _, err := c.execOutput(ctx, state.ID, []string{"registry", "garbage-collect", gcConfigPath}); err != nil {
		return rep, fmt.Errorf("registry: garbage collect: %w", err)
	}
	return rep, nil
}

func (c *Controller) gcCandidates(ctx context.Context, id string) ([]string, error) {
	out, err := c.execOutput(ctx, id, []string{"registry", "garbage-collect", "--dry-run", gcConfigPath})
	if err != nil {
		return nil, fmt.Errorf("registry: garbage collect dry run: %w", err)
	}
	return ParseGCDryRun(out), nil
}

// ParseGCDryRun returns the digests a dry run listed as eligible for deletion.
func ParseGCDryRun(out string) []string {
	var digests []string
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		i := strings.Index(line, gcEligible)
		if i < 0 {
			continue
		}
		d := strings.TrimSpace(line[i+len(gcEligible):])
		if strings.HasPrefix(d, "sha256:") && len(d) > len("sha256:")+2 {
			digests = append(digests, d)
		}
	}
	return digests
}

// blobBytes sums blob sizes with stat; a failed chunk is skipped, so the figure can undercount.
func (c *Controller) blobBytes(ctx context.Context, id string, digests []string) int64 {
	var total int64
	for start := 0; start < len(digests); start += gcStatChunk {
		end := min(start+gcStatChunk, len(digests))
		cmd := []string{"stat", "-c", "%s"}
		for _, d := range digests[start:end] {
			hex := strings.TrimPrefix(d, "sha256:")
			cmd = append(cmd, gcBlobRoot+"/"+hex[:2]+"/"+hex+"/data")
		}
		out, err := c.execOutput(ctx, id, cmd)
		if err != nil {
			continue
		}
		for _, f := range strings.Fields(out) {
			if n, perr := strconv.ParseInt(f, 10, 64); perr == nil {
				total += n
			}
		}
	}
	return total
}

func (c *Controller) execOutput(ctx context.Context, id string, cmd []string) (string, error) {
	rc, err := c.runtime.Exec(ctx, id, cmd)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(rc)
	return string(b), err
}
