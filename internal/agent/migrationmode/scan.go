package migrationmode

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/GLINCKER/levelrail/internal/agent/migrationpb"
	"github.com/GLINCKER/levelrail/internal/treestat"
)

// ErrUnknownTarget means a scan named something the last inventory did not list.
var ErrUnknownTarget = errors.New("migrationmode: scan target was not in the last inventory")

// Scanner counts files under volumes and bind mounts, but only those the
// agent itself reported in its last inventory.
type Scanner struct {
	mu    sync.RWMutex
	roots map[string]string
}

// NewScanner returns a scanner with no known targets.
func NewScanner() *Scanner { return &Scanner{roots: map[string]string{}} }

// Learn records the scannable targets of inv, replacing earlier ones.
func (s *Scanner) Learn(inv *migrationpb.Inventory) {
	roots := map[string]string{}
	for _, v := range inv.GetVolumes() {
		if v.GetName() != "" && filepath.IsAbs(v.GetMountpoint()) {
			roots[v.GetName()] = filepath.Clean(v.GetMountpoint())
		}
	}
	for _, c := range inv.GetContainers() {
		for _, m := range c.GetMounts() {
			if m.GetType() == "bind" && filepath.IsAbs(m.GetSource()) {
				clean := filepath.Clean(m.GetSource())
				roots[clean] = clean
			}
		}
	}
	s.mu.Lock()
	s.roots = roots
	s.mu.Unlock()
}

func (s *Scanner) root(target string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.roots[target]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownTarget, target)
	}
	return r, nil
}

// Scan measures each target twice. A second pass with different totals marks
// the target changed_during_scan so live writes are never reported as a match.
func (s *Scanner) Scan(ctx context.Context, requestID string, targets []string) *migrationpb.ScanResult {
	res := &migrationpb.ScanResult{RequestId: requestID}
	for _, t := range targets {
		st := &migrationpb.VolumeStat{Target: t}
		root, err := s.root(t)
		if err != nil {
			st.Error = err.Error()
			res.Stats = append(res.Stats, st)
			continue
		}
		first, err := treestat.Walk(ctx, root)
		if err != nil {
			st.Error = err.Error()
			res.Stats = append(res.Stats, st)
			continue
		}
		second, err := treestat.Walk(ctx, root)
		if err != nil {
			st.Error = err.Error()
			res.Stats = append(res.Stats, st)
			continue
		}
		st.Files, st.Bytes = second.Files, second.Bytes
		st.ChangedDuringScan = first != second
		res.Stats = append(res.Stats, st)
	}
	return res
}
