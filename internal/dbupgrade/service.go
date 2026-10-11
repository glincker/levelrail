package dbupgrade

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Errors the API maps to status codes.
var (
	ErrNotFound = errors.New("database not found")
	ErrInvalid  = errors.New("invalid upgrade request")
	ErrConflict = errors.New("upgrade conflict")
)

// historyLimit caps the history an overview returns.
const historyLimit = 50

// Overview is everything the Upgrades tab shows for one database.
type Overview struct {
	Database        string
	Engine          string
	Version         string
	Advice          Advice
	Policy          Policy
	PolicyInherited bool
	WindowOpen      bool
	NextWindow      time.Time
	// NextTarget is what the next window would apply, when anything.
	NextTarget *Target
	Blockers   []string
	Active     *store.DBUpgradeRun
	History    []store.DBUpgradeRun
}

// Overview builds the advisor, policy and history view of one database.
func (m *Manager) Overview(ctx context.Context, name string) (Overview, error) {
	db, err := m.loadDatabase(ctx, name)
	if err != nil {
		return Overview{}, err
	}
	now := m.now()
	policy, inherited, err := m.EffectivePolicy(ctx, name)
	if err != nil {
		return Overview{}, err
	}
	out := Overview{
		Database: db.Name, Engine: db.Engine, Version: db.Version, Advice: m.Advisor.Advise(db.Engine, db.Version, now),
		Policy: policy, PolicyInherited: inherited,
	}
	if st, err := policy.WindowState(now); err == nil {
		out.WindowOpen, out.NextWindow = st.Active, st.NextStart
	}
	if policy.AutoUpgrade != store.DBAutoUpgradeOff {
		if t, ok := out.Advice.BestAutomatic(policy.AutoUpgrade); ok {
			out.NextTarget = &t
		}
	}
	out.Blockers = blockers(*db, out.Advice, policy)
	if why := m.unreachable(*db); why != "" {
		out.Blockers = append(out.Blockers, why)
	}
	runs, err := m.Store.ListDBUpgradeRuns(ctx, name, historyLimit)
	if err != nil {
		return Overview{}, fmt.Errorf("dbupgrade: list runs: %w", err)
	}
	out.History = runs
	for i := range runs {
		if !store.DBUpgradeTerminal(runs[i].State) {
			out.Active = &runs[i]
			break
		}
	}
	return out, nil
}

func blockers(db store.DesiredDatabase, a Advice, p Policy) []string {
	var out []string
	if db.BackupTargetID == "" {
		out = append(out, "no backup target is configured: every upgrade starts from a fresh backup, so none can run")
	}
	if db.Suspended {
		out = append(out, "the database is stopped")
	}
	if a.ManualReason != "" && p.AutoUpgrade != store.DBAutoUpgradeOff {
		out = append(out, "automatic upgrades are off for this engine: "+a.ManualReason)
	}
	if p.AutoUpgrade == store.DBAutoUpgradeMinor && a.AutoMax == KindPatch {
		out = append(out, "this engine allows automatic patch upgrades only; minor releases stay manual")
	}
	return out
}

// SetPolicy saves a database's policy, or with inherit drops it so the
// platform default applies again. name may be the platform default.
func (m *Manager) SetPolicy(ctx context.Context, name string, p Policy, inherit bool, by string) error {
	if name != store.DBUpgradePlatformDefault {
		if _, err := m.loadDatabase(ctx, name); err != nil {
			return err
		}
		if inherit {
			if err := m.Store.DeleteDBUpgradePolicy(ctx, name); err != nil {
				return fmt.Errorf("dbupgrade: %w", err)
			}
			return nil
		}
	}
	if err := p.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := m.Store.SaveDBUpgradePolicy(ctx, p.ToStore(name, by, m.now())); err != nil {
		return fmt.Errorf("dbupgrade: %w", err)
	}
	return nil
}

// UpgradeNow starts a run immediately, outside any window, with the same
// backup, verify and revert steps as a scheduled one. Majors are refused.
func (m *Manager) UpgradeNow(ctx context.Context, name, version, by string) (store.DBUpgradeRun, error) {
	db, err := m.loadDatabase(ctx, name)
	if err != nil {
		return store.DBUpgradeRun{}, err
	}
	if db.Suspended {
		return store.DBUpgradeRun{}, fmt.Errorf("%w: the database is stopped; start it first", ErrConflict)
	}
	if db.BackupTargetID == "" {
		return store.DBUpgradeRun{}, fmt.Errorf("%w: configure a backup target first, every upgrade starts from a fresh backup", ErrConflict)
	}
	if why := m.unreachable(*db); why != "" {
		return store.DBUpgradeRun{}, fmt.Errorf("%w: %s", ErrConflict, why)
	}
	target, err := m.manualTarget(*db, version)
	if err != nil {
		return store.DBUpgradeRun{}, err
	}
	active, err := m.Store.ListActiveDBUpgradeRuns(ctx)
	if err != nil {
		return store.DBUpgradeRun{}, fmt.Errorf("dbupgrade: list active runs: %w", err)
	}
	for _, r := range active {
		if r.DatabaseName == name {
			return store.DBUpgradeRun{}, fmt.Errorf("%w: upgrade %s of this database is still running", ErrConflict, r.ID)
		}
	}
	policy, _, err := m.EffectivePolicy(ctx, name)
	if err != nil {
		return store.DBUpgradeRun{}, err
	}
	return m.createRun(ctx, *db, target, store.DBUpgradeSourceManual, by, policy)
}

func (m *Manager) manualTarget(db store.DesiredDatabase, version string) (Target, error) {
	advice := m.Advisor.Advise(db.Engine, db.Version, m.now())
	if t, ok := advice.FindTarget(version); ok {
		if t.Kind == KindMajor {
			return Target{}, fmt.Errorf("%w: %s is a major upgrade; use the guarded major upgrade (Postgres) or restore a backup into a new database", ErrInvalid, version)
		}
		return t, nil
	}
	if err := database.CheckInPlaceVersionChange(db.Engine, db.Version, version); err != nil {
		return Target{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	ec, ok := m.Advisor.Catalog.Engine(db.Engine)
	from, okFrom := parseVersion(db.Version)
	to, okTo := parseVersion(version)
	if !ok || !okFrom || !okTo || from.prefix != to.prefix || from.suffix != to.suffix {
		return Target{}, fmt.Errorf("%w: %q cannot be compared with %q", ErrInvalid, version, db.Version)
	}
	if compareVersions(to, from) <= 0 {
		return Target{}, fmt.Errorf("%w: %s is not newer than %s; downgrades are not supported", ErrInvalid, version, db.Version)
	}
	kind := classify(ec.Levels, from, to)
	if kind == KindMajor {
		return Target{}, fmt.Errorf("%w: %s is a major upgrade; use the guarded major upgrade (Postgres) or restore a backup into a new database", ErrInvalid, version)
	}
	return Target{Version: version, Kind: kind}, nil
}

// SummaryItem is one database's upgrade status for the attention center.
type SummaryItem struct {
	Database   string
	Engine     string
	Version    string
	Support    string
	EOL        string
	Security   bool
	Advisories []string
	Available  int
	Active     *store.DBUpgradeRun
	Last       *store.DBUpgradeRun
}

// Summary lists every database's upgrade status, filtered by visible.
func (m *Manager) Summary(ctx context.Context, visible func(string) bool) ([]SummaryItem, error) {
	dbs, err := m.Store.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("dbupgrade: list databases: %w", err)
	}
	now := m.now()
	out := make([]SummaryItem, 0, len(dbs))
	for _, db := range dbs {
		if visible != nil && !visible(db.Name) {
			continue
		}
		a := m.Advisor.Advise(db.Engine, db.Version, now)
		item := SummaryItem{Database: db.Name, Engine: db.Engine, Version: db.Version, Support: a.Support, EOL: a.EOL,
			Security: a.HasSecurityUpdate(), Advisories: a.Advisories, Available: len(a.Targets)}
		if runs, err := m.Store.ListDBUpgradeRuns(ctx, db.Name, 1); err == nil && len(runs) == 1 {
			run := runs[0]
			if store.DBUpgradeTerminal(run.State) {
				item.Last = &run
			} else {
				item.Active = &run
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func (m *Manager) loadDatabase(ctx context.Context, name string) (*store.DesiredDatabase, error) {
	db, err := m.Store.GetDesiredDatabase(ctx, name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("dbupgrade: load database %q: %w", name, err)
	}
	return db, nil
}
