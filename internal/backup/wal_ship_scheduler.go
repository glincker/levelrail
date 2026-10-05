package backup

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// WALShipStatus is the outcome of the most recent shipping pass for one
// database, shown by "pitr status".
type WALShipStatus struct {
	LastAttemptAt time.Time
	LastSuccessAt time.Time
	LastError     string
	// Shipped counts segments uploaded since the control plane started.
	Shipped  int
	TargetID string
}

// WALShipStore is the store surface WALShipScheduler needs.
type WALShipStore interface {
	ListDesiredDatabases(ctx context.Context) ([]store.DesiredDatabase, error)
	ListBaseBackupHistory(ctx context.Context, databaseName string) ([]store.BaseBackupHistory, error)
}

// WALShipScheduler ships the WAL of every local PITR-enabled database to the
// target of its newest base backup on a fixed interval, bounding the data lost
// if the database host's disk dies to one interval of WAL.
type WALShipScheduler struct {
	Store         WALShipStore
	Shipper       WALShipping
	Resolve       func(ctx context.Context, targetID string) (Destination, error)
	ContainerName func(databaseName string) string
	// IsLocal reports whether a database's node is the one this control plane's
	// Docker client talks to; remote nodes are skipped.
	IsLocal func(nodeID string) bool
	Logger  *slog.Logger

	mu     sync.Mutex
	status map[string]WALShipStatus
}

// WALShipStatus returns the last shipping outcome for a database.
func (s *WALShipScheduler) WALShipStatus(databaseName string) (WALShipStatus, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.status[databaseName]
	return st, ok
}

func (s *WALShipScheduler) record(databaseName string, update func(*WALShipStatus)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status == nil {
		s.status = map[string]WALShipStatus{}
	}
	st := s.status[databaseName]
	update(&st)
	s.status[databaseName] = st
}

// Run ticks until ctx ends.
func (s *WALShipScheduler) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return nil
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		s.Tick(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// Tick runs one shipping pass over every eligible database.
func (s *WALShipScheduler) Tick(ctx context.Context) {
	dbs, err := s.Store.ListDesiredDatabases(ctx)
	if err != nil {
		s.log().Error("backup: wal ship: list databases failed", slog.String("error", err.Error()))
		return
	}
	for _, d := range dbs {
		if ctx.Err() != nil {
			return
		}
		if !d.PITREnabled || d.Engine != store.EnginePostgres || (s.IsLocal != nil && !s.IsLocal(d.NodeID)) {
			continue
		}
		s.shipOne(ctx, d.Name)
	}
}

func (s *WALShipScheduler) shipOne(ctx context.Context, name string) {
	history, err := s.Store.ListBaseBackupHistory(ctx, name)
	if err != nil {
		s.log().Error("backup: wal ship: list base backups failed", slog.String("database", name), slog.String("error", err.Error()))
		return
	}
	targetID := ""
	for _, h := range history {
		if h.Status == store.BackupStatusSucceeded {
			targetID = h.TargetID
			break
		}
	}
	if targetID == "" {
		return
	}

	now := time.Now().UTC()
	dest, err := s.Resolve(ctx, targetID)
	var n int
	if err == nil {
		n, err = s.Shipper.Ship(ctx, dest, name, s.ContainerName(name))
	}
	s.record(name, func(st *WALShipStatus) {
		st.LastAttemptAt = now
		st.TargetID = targetID
		if err != nil {
			st.LastError = err.Error()
			return
		}
		st.LastError = ""
		st.LastSuccessAt = now
		st.Shipped += n
	})
	if err != nil {
		s.log().Warn("backup: wal ship failed", slog.String("database", name), slog.String("error", err.Error()))
	}
}

func (s *WALShipScheduler) log() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}
