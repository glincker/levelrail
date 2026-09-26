package supplychain

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"
)

const orphanGrace = 5 * time.Minute

// HasSBOM reports whether the SBOM document is still on disk.
func (r Record) HasSBOM() bool { return r.SBOMBytes > 0 }

// Sweep runs every cleanup pass: records of deleted apps and deploys, SBOM
// retention, orphan files and orphan scanner containers.
func (s *Service) Sweep(ctx context.Context) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	steps := []struct {
		name string
		run  func() error
	}{
		{"deleted apps and deploys", func() error { return s.removeGone(ctx) }},
		{"retention", func() error { return s.enforceRetention(ctx) }},
		{"orphan files", func() error { return s.removeOrphanFiles(ctx) }},
		{"orphan containers", func() error { return s.sweepContainers(ctx) }},
	}
	for _, st := range steps {
		if err := st.run(); err != nil {
			s.log.Warn("supplychain: sweep step failed", slog.String("step", st.name), slog.String("error", err.Error()))
		}
	}
}

// removeGone deletes records, and their files, whose app or deploy attempt no
// longer exists.
func (s *Service) removeGone(ctx context.Context) error {
	recs, err := s.deps.Store.ListAllRecords(ctx)
	if err != nil {
		return err
	}
	appOK := map[string]bool{}
	var gone []Record
	for _, r := range recs {
		ok, seen := appOK[r.App]
		if !seen {
			if ok, err = s.deps.Resolver.AppExists(ctx, r.App); err != nil {
				return fmt.Errorf("supplychain: check app %q: %w", r.App, err)
			}
			appOK[r.App] = ok
		}
		if !ok {
			gone = append(gone, r)
			continue
		}
		exists, err := s.deps.Resolver.AttemptExists(ctx, r.AttemptID)
		if err != nil {
			return fmt.Errorf("supplychain: check deploy %q: %w", r.AttemptID, err)
		}
		if !exists {
			gone = append(gone, r)
		}
	}
	return s.dropRecords(ctx, gone)
}

func (s *Service) dropRecords(ctx context.Context, victims []Record) error {
	if len(victims) == 0 {
		return nil
	}
	ids := make([]string, 0, len(victims))
	for _, v := range victims {
		if err := s.fs.Remove(v.App, v.AttemptID); err != nil {
			return err
		}
		ids = append(ids, v.AttemptID)
	}
	return s.deps.Store.DeleteRecords(ctx, ids)
}

// enforceRetention keeps the newest KeepPerApp SBOM documents of each app. The
// record, with its package and vulnerability counts, stays; only the file goes.
func (s *Service) enforceRetention(ctx context.Context) error {
	recs, err := s.deps.Store.ListAllRecords(ctx)
	if err != nil {
		return err
	}
	byApp := map[string][]Record{}
	for _, r := range recs {
		if r.HasSBOM() {
			byApp[r.App] = append(byApp[r.App], r)
		}
	}
	for _, list := range byApp {
		sort.Slice(list, func(i, j int) bool { return list[i].GeneratedAt.After(list[j].GeneratedAt) })
		if len(list) <= s.cfg.KeepPerApp {
			continue
		}
		for _, old := range list[s.cfg.KeepPerApp:] {
			if err := s.fs.Remove(old.App, old.AttemptID); err != nil {
				return err
			}
			old.SBOMBytes = 0
			if err := s.deps.Store.SaveRecord(ctx, old); err != nil {
				return err
			}
		}
	}
	return nil
}

// removeOrphanFiles deletes SBOM files without a record and demotes records
// whose file is gone.
func (s *Service) removeOrphanFiles(ctx context.Context) error {
	files, err := s.fs.List()
	if err != nil {
		return err
	}
	recs, err := s.deps.Store.ListAllRecords(ctx)
	if err != nil {
		return err
	}
	withFile := map[string]bool{}
	for _, r := range recs {
		if r.HasSBOM() {
			withFile[r.AttemptID] = true
		}
	}
	onDisk := map[string]bool{}
	for _, f := range files {
		if !withFile[f.AttemptID] {
			if s.now().Sub(f.ModTime) < orphanGrace {
				continue
			}
			if err := s.fs.RemoveEntry(f); err != nil {
				return err
			}
			continue
		}
		onDisk[f.AttemptID] = true
	}
	for _, r := range recs {
		if r.HasSBOM() && !onDisk[r.AttemptID] {
			r.SBOMBytes = 0
			if err := s.deps.Store.SaveRecord(ctx, r); err != nil {
				return err
			}
		}
	}
	return nil
}

// sweepContainers force-removes scanner containers older than twice the scan
// timeout: leftovers of a control plane that died mid-scan.
func (s *Service) sweepContainers(ctx context.Context) error {
	list, err := s.deps.Runner.ListContainersByLabel(ctx, s.roleLabel()+"="+roleValue)
	if err != nil {
		return fmt.Errorf("supplychain: list scanner containers: %w", err)
	}
	cutoff := s.now().Add(-2 * s.cfg.Timeout)
	var firstErr error
	for _, c := range list {
		if c.Created.After(cutoff) {
			continue
		}
		if err := s.deps.Runner.Remove(ctx, c.ID, true); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("supplychain: remove orphan container %q: %w", c.Name, err)
		}
	}
	return firstErr
}

// Start runs a startup sweep and then one every SweepInterval until ctx ends.
func (s *Service) Start(ctx context.Context) {
	go func() {
		s.Sweep(ctx)
		t := time.NewTicker(s.cfg.SweepInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.Sweep(ctx)
			}
		}
	}()
}
