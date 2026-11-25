package dbbranch

import (
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/diskguard"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ErrQuota is returned when a branch limit is reached.
var ErrQuota = errors.New("dbbranch: branch quota reached")

// Limits are the live settings the checks read.
type Limits struct {
	MaxPerRoot       int
	MaxTotal         int
	MaxTotalBytes    int64
	MaxDepth         int
	SlowPathMaxBytes int64
	Headroom         float64
}

// Request is one branch about to be created.
type Request struct {
	Parent       store.DatabaseBranch
	ParentIsRoot bool
	RootName     string
	ParentBytes  int64
	SlowPath     bool
}

// CheckQuota refuses a branch over a limit. It reads a snapshot, so the API slice must serialize creates
// or insert through store.CreateDatabaseBranchWithinQuota. all is every existing branch row.
func CheckQuota(l Limits, r Request, all []store.DatabaseBranch, guard *diskguard.Guard) error {
	perRoot, total := 0, 0
	var bytes int64
	for _, b := range all {
		if b.State == StateDeleted || b.State == StatePromoted {
			continue
		}
		total++
		bytes += b.SizeBytes
		if b.RootName == r.RootName {
			perRoot++
		}
	}
	if l.MaxPerRoot > 0 && perRoot >= l.MaxPerRoot {
		return fmt.Errorf("%w: %d branches already under %s", ErrQuota, perRoot, r.RootName)
	}
	if l.MaxTotal > 0 && total >= l.MaxTotal {
		return fmt.Errorf("%w: %d branches on the platform", ErrQuota, total)
	}
	if l.MaxTotalBytes > 0 && bytes+r.ParentBytes > l.MaxTotalBytes {
		return fmt.Errorf("%w: branches would exceed the total size limit", ErrQuota)
	}
	if depth := branchDepth(r, all); l.MaxDepth > 0 && depth > l.MaxDepth {
		return fmt.Errorf("%w: depth %d exceeds %d", ErrQuota, depth, l.MaxDepth)
	}
	if r.SlowPath && l.SlowPathMaxBytes > 0 && r.ParentBytes > l.SlowPathMaxBytes {
		return fmt.Errorf("%w: parent is larger than the dump branch limit, use a backup source", ErrQuota)
	}
	return guard.RefuseBranch(r.ParentBytes, l.Headroom, r.SlowPath)
}

// branchDepth is the depth the new branch would have: a branch of a root is depth 1.
func branchDepth(r Request, all []store.DatabaseBranch) int {
	if r.ParentIsRoot {
		return 1
	}
	byName := map[string]store.DatabaseBranch{}
	for _, b := range all {
		byName[b.DatabaseName] = b
	}
	depth, name := 1, r.Parent.DatabaseName
	for i := 0; i < 64 && name != ""; i++ {
		b, ok := byName[name]
		if !ok {
			break
		}
		depth++
		name = b.ParentName
		if name == r.RootName {
			break
		}
	}
	return depth
}

// ExpiryFor returns the expiry time for a new branch: never for a zero ttl, otherwise now plus ttl.
func ExpiryFor(now time.Time, ttl time.Duration) string {
	if ttl <= 0 {
		return ""
	}
	return now.UTC().Add(ttl).Format(time.RFC3339)
}

// SelectExpired picks what the expiry job collects, through the one rule in the store.
func SelectExpired(rows []store.DatabaseBranch, now time.Time, p store.DuePolicy) []store.DatabaseBranch {
	return store.SelectDueBranches(rows, now, p)
}
