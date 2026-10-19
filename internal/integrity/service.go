package integrity

import (
	"context"
	"log/slog"
	"time"
)

// View is everything the health page and CLI show about database integrity.
type View struct {
	Status     Status      `json:"status"`
	Pending    *Pending    `json:"pending,omitempty"`
	Candidates []Candidate `json:"candidates"`
}

// Service ties the checker to the snapshot store and the staged restore marker.
type Service struct {
	*Checker
	dataDir string
	backups BackupLister
}

// NewService returns a Service for the database behind db, with snapshots listed by backups under dataDir.
func NewService(db Querier, dataDir string, backups BackupLister, logger *slog.Logger) *Service {
	return &Service{Checker: NewChecker(db, logger), dataDir: dataDir, backups: backups}
}

// View returns the current status, staged restore and restorable snapshots.
func (s *Service) View() (View, error) {
	pending, err := ReadPending(s.dataDir)
	if err != nil {
		return View{}, err
	}
	cands, err := Candidates(s.backups)
	if err != nil {
		return View{}, err
	}
	return View{Status: s.Status(), Pending: pending, Candidates: cands}, nil
}

// Restore stages the named snapshot, or the newest one that verifies when name is empty. It takes effect at the next boot.
func (s *Service) Restore(ctx context.Context, name, by string) (Pending, error) {
	if name == "" {
		latest, err := LatestVerified(ctx, s.backups)
		if err != nil {
			return Pending{}, err
		}
		name = latest
	}
	return Stage(ctx, s.dataDir, name, by, time.Now())
}

// CancelRestore removes a staged restore.
func (s *Service) CancelRestore() error { return ClearPending(s.dataDir) }
