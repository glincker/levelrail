package supplychain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
)

// Store persists supply chain metadata; *SQLStore satisfies it.
type Store interface {
	GetSettings(ctx context.Context, app string) (Settings, error)
	SaveSettings(ctx context.Context, s Settings) error
	// ConsumeOverride returns and clears the armed override if it is newer than notBefore.
	ConsumeOverride(ctx context.Context, app string, notBefore time.Time) (string, bool, error)
	SaveRecord(ctx context.Context, r Record) error
	GetRecord(ctx context.Context, attemptID string) (Record, error)
	ListRecords(ctx context.Context, app string) ([]Record, error)
	ListAllRecords(ctx context.Context) ([]Record, error)
	RecordsByAttempt(ctx context.Context, ids []string) (map[string]Record, error)
	DeleteRecords(ctx context.Context, ids []string) error
	DeleteApp(ctx context.Context, app string) error
}

// Resolver answers questions about apps and their deploy attempts.
type Resolver interface {
	AppExists(ctx context.Context, app string) (bool, error)
	AttemptExists(ctx context.Context, attemptID string) (bool, error)
}

// Deps are the collaborators a Service needs.
type Deps struct {
	Store    Store
	Resolver Resolver
	Runner   Runner
	// DataDir is the platform data directory; SBOMs live in DataDir/supplychain.
	DataDir string
	// Namespace prefixes labels and names of scan containers (the brand short name).
	Namespace string
	Logger    *slog.Logger
}

// Service records SBOMs, scans them on demand and gates releases. It holds no
// goroutine while idle apart from the optional sweep ticker.
type Service struct {
	cfg  Config
	deps Deps
	log  *slog.Logger
	fs   *FileStore
	now  func() time.Time

	scanMu sync.Mutex
	opMu   sync.Mutex
}

// Option customizes a Service.
type Option func(*Service)

// WithClock overrides the clock, for tests.
func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

// New builds a Service.
func New(cfg Config, deps Deps, opts ...Option) *Service {
	log := deps.Logger
	if log == nil {
		log = slog.Default()
	}
	s := &Service{cfg: cfg, deps: deps, log: log, now: time.Now, fs: NewFileStore(filepath.Join(deps.DataDir, "supplychain"))}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Config returns the effective configuration.
func (s *Service) Config() Config { return s.cfg }

// BlockedError is returned to the deploy pipeline when the gate blocks a release.
type BlockedError struct{ Reason string }

func (e *BlockedError) Error() string { return "supply chain gate blocked this release: " + e.Reason }

// ErrNotEnabled means scanning is not enabled for the app.
var ErrNotEnabled = errors.New("supplychain: scanning is not enabled for this app")

// Settings returns the app's settings, or the default (off) when none exist.
func (s *Service) Settings(ctx context.Context, app string) (Settings, error) {
	return s.deps.Store.GetSettings(ctx, app)
}

// SettingsPatch changes the fields that are non-nil.
type SettingsPatch struct {
	Enabled *bool
	Gate    *string
}

// SaveSettings applies patch. A gate other than off requires scanning enabled.
func (s *Service) SaveSettings(ctx context.Context, app string, patch SettingsPatch) (Settings, error) {
	cur, err := s.deps.Store.GetSettings(ctx, app)
	if err != nil {
		return Settings{}, err
	}
	if patch.Enabled != nil {
		cur.Enabled = *patch.Enabled
	}
	if patch.Gate != nil {
		mode, err := ParseGateMode(*patch.Gate)
		if err != nil {
			return Settings{}, err
		}
		cur.Gate = mode
	}
	if cur.Gate != GateOff && !cur.Enabled {
		return Settings{}, fmt.Errorf("%w: scan_gate %q needs scanning enabled", ErrInvalid, cur.Gate)
	}
	if err := s.deps.Store.SaveSettings(ctx, cur); err != nil {
		return Settings{}, err
	}
	return cur, nil
}

// ArmOverride lets the next blocked release of app through once, for reason.
func (s *Service) ArmOverride(ctx context.Context, app, reason string) (Settings, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Settings{}, fmt.Errorf("%w: override reason is required", ErrInvalid)
	}
	cur, err := s.deps.Store.GetSettings(ctx, app)
	if err != nil {
		return Settings{}, err
	}
	if cur.Gate != GateBlockOnCritical {
		return Settings{}, fmt.Errorf("%w: override only applies to scan_gate block_on_critical", ErrInvalid)
	}
	cur.OverrideReason, cur.OverrideArmedAt = reason, s.now()
	if err := s.deps.Store.SaveSettings(ctx, cur); err != nil {
		return Settings{}, err
	}
	return cur, nil
}

// AfterBuild is the deploy pipeline hook: it stores the build's SBOM and, when
// the app opted in, scans it and applies the gate. Only a gate block returns
// an error; every other failure is logged and the release proceeds.
func (s *Service) AfterBuild(ctx context.Context, app, attemptID string, att build.Attestations) error {
	if attemptID == "" {
		return nil
	}
	rec, err := s.storeSBOM(ctx, app, attemptID, att)
	if err != nil {
		s.log.Warn("supplychain: record sbom failed", slog.String("app", app), slog.String("attempt_id", attemptID), slog.String("error", err.Error()))
	}
	settings, err := s.deps.Store.GetSettings(ctx, app)
	if err != nil {
		s.log.Warn("supplychain: load settings failed", slog.String("app", app), slog.String("error", err.Error()))
		return nil
	}
	if !s.cfg.Enabled || !settings.Enabled || rec == nil {
		return nil
	}
	sbom, err := s.fs.Read(app, attemptID)
	if err != nil {
		return nil
	}
	scan, scanErr := s.scanAndStore(ctx, rec, sbom)
	if scanErr != nil {
		s.log.Warn("supplychain: scan failed, release allowed", slog.String("app", app), slog.String("attempt_id", attemptID), slog.String("error", scanErr.Error()))
	}
	return s.applyGate(ctx, settings, rec, scan)
}

func (s *Service) applyGate(ctx context.Context, settings Settings, rec *Record, scan *ScanSummary) error {
	override := ""
	if settings.Gate == GateBlockOnCritical && scan != nil && scan.Counts.Critical > 0 {
		reason, ok, err := s.deps.Store.ConsumeOverride(ctx, rec.App, s.now().Add(-s.cfg.OverrideTTL))
		if err != nil {
			s.log.Warn("supplychain: consume override failed", slog.String("app", rec.App), slog.String("error", err.Error()))
		} else if ok {
			override = reason
		}
	}
	d := Decide(settings.Gate, scan, override)
	rec.GateAction, rec.GateReason = d.Action, d.Reason
	if err := s.deps.Store.SaveRecord(ctx, *rec); err != nil {
		s.log.Warn("supplychain: save gate decision failed", slog.String("app", rec.App), slog.String("attempt_id", rec.AttemptID), slog.String("error", err.Error()))
	}
	if d.Blocked() {
		return &BlockedError{Reason: d.Reason}
	}
	return nil
}

func (s *Service) storeSBOM(ctx context.Context, app, attemptID string, att build.Attestations) (*Record, error) {
	if len(att.SBOM) == 0 {
		return nil, nil
	}
	format, pkgs, err := ParseSBOM(att.SBOM)
	if err != nil {
		return nil, err
	}
	if err := s.fs.Write(app, attemptID, att.SBOM); err != nil {
		return nil, err
	}
	rec := Record{
		AttemptID: attemptID, App: app, Summary: Summarize(format, pkgs), SBOMBytes: int64(len(att.SBOM)),
		HasProvenance: len(att.Provenance) > 0, GeneratedAt: s.now(),
	}
	if err := s.deps.Store.SaveRecord(ctx, rec); err != nil {
		_ = s.fs.Remove(app, attemptID)
		return nil, err
	}
	return &rec, nil
}

// scanAndStore runs the scanner and persists the outcome on rec.
func (s *Service) scanAndStore(ctx context.Context, rec *Record, sbom []byte) (*ScanSummary, error) {
	s.scanMu.Lock()
	sum, err := s.runScan(ctx, rec.AttemptID, sbom)
	s.scanMu.Unlock()
	rec.ScannedAt, rec.Scanner = s.now(), s.cfg.Scanner
	if err != nil {
		rec.ScanStatus, rec.ScanError, rec.Scan = ScanFailed, err.Error(), nil
		if serr := s.deps.Store.SaveRecord(ctx, *rec); serr != nil {
			s.log.Warn("supplychain: save scan failure failed", slog.String("attempt_id", rec.AttemptID), slog.String("error", serr.Error()))
		}
		return nil, err
	}
	rec.ScanStatus, rec.ScanError, rec.Scan = ScanOK, "", &sum
	if err := s.deps.Store.SaveRecord(ctx, *rec); err != nil {
		return &sum, fmt.Errorf("supplychain: save scan result: %w", err)
	}
	return &sum, nil
}

// ScanNow scans one deployment's stored SBOM and records the result. It never
// changes a release that is already live.
func (s *Service) ScanNow(ctx context.Context, app, attemptID string) (Record, error) {
	if !s.cfg.Enabled {
		return Record{}, ErrDisabled
	}
	settings, err := s.deps.Store.GetSettings(ctx, app)
	if err != nil {
		return Record{}, err
	}
	if !settings.Enabled {
		return Record{}, ErrNotEnabled
	}
	rec, err := s.Get(ctx, app, attemptID)
	if err != nil {
		return Record{}, err
	}
	sbom, err := s.fs.Read(app, attemptID)
	if errors.Is(err, ErrNotFound) {
		return Record{}, ErrNoSBOM
	}
	if err != nil {
		return Record{}, err
	}
	if _, err := s.scanAndStore(ctx, &rec, sbom); err != nil {
		return rec, err
	}
	return rec, nil
}

// Get returns the record of one deployment of app.
func (s *Service) Get(ctx context.Context, app, attemptID string) (Record, error) {
	rec, err := s.deps.Store.GetRecord(ctx, attemptID)
	if err != nil {
		return Record{}, err
	}
	if rec.App != app {
		return Record{}, ErrNotFound
	}
	return rec, nil
}

// OpenSBOM returns the stored SBOM document of one deployment.
func (s *Service) OpenSBOM(ctx context.Context, app, attemptID string) ([]byte, Record, error) {
	rec, err := s.Get(ctx, app, attemptID)
	if err != nil {
		return nil, Record{}, err
	}
	data, err := s.fs.Read(app, attemptID)
	if errors.Is(err, ErrNotFound) {
		return nil, rec, ErrNoSBOM
	}
	return data, rec, err
}

// List returns app's records, newest first.
func (s *Service) List(ctx context.Context, app string) ([]Record, error) {
	return s.deps.Store.ListRecords(ctx, app)
}

// Lookup maps deploy attempt IDs to their records.
func (s *Service) Lookup(ctx context.Context, ids []string) (map[string]Record, error) {
	return s.deps.Store.RecordsByAttempt(ctx, ids)
}

// DeleteApp removes every record, file and setting of a deleted app.
func (s *Service) DeleteApp(ctx context.Context, app string) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if err := s.deps.Store.DeleteApp(ctx, app); err != nil {
		return err
	}
	return s.fs.RemoveApp(app)
}
