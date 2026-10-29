// Package release writes the shadow copy of ADR 050 release snapshots and
// environment bindings. Nothing here is read by a deploy decision.
package release

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// EnvBindings selects the rollout mode: off (default), shadow or on.
const EnvBindings = "APP_RELEASE_BINDINGS"

// Mode is the parsed APP_RELEASE_BINDINGS value.
type Mode string

// Rollout modes. Only off, the default, leaves the store untouched.
const (
	ModeOff    Mode = "off"
	ModeShadow Mode = "shadow"
	ModeOn     Mode = "on"
)

// ModeFromEnv reads the flag; unknown values count as off.
func ModeFromEnv() Mode {
	switch Mode(strings.ToLower(strings.TrimSpace(os.Getenv(EnvBindings)))) {
	case ModeShadow:
		return ModeShadow
	case ModeOn:
		return ModeOn
	default:
		return ModeOff
	}
}

// Writes reports whether the mode records shadow rows. In this phase on
// behaves like shadow.
func (m Mode) Writes() bool { return m == ModeShadow || m == ModeOn }

// Config is the environment neutral subset of a deploy snapshot. It holds no
// env values, domains, ports, volumes or placement.
type Config struct {
	Health    *store.ServiceHealth    `json:"health,omitempty"`
	Replicas  int                     `json:"replicas,omitempty"`
	Strategy  string                  `json:"strategy,omitempty"`
	Resources *store.ServiceResources `json:"resources,omitempty"`
	Labels    map[string]string       `json:"labels,omitempty"`
}

// Shadow records releases and bindings next to the existing deploy path.
type Shadow struct {
	db     *store.DB
	mode   Mode
	logger *slog.Logger
}

// NewShadow builds a writer for mode.
func NewShadow(db *store.DB, mode Mode, logger *slog.Logger) *Shadow {
	if logger == nil {
		logger = slog.Default()
	}
	return &Shadow{db: db, mode: mode, logger: logger}
}

// Install hooks the writer onto db. Off mode installs nothing.
func Install(db *store.DB, mode Mode, logger *slog.Logger) {
	if !mode.Writes() {
		return
	}
	db.SetAttemptObserver(NewShadow(db, mode, logger).OnAttempt)
}

// OnAttempt is the store hook. A failure only logs a warning, never the deploy.
func (s *Shadow) OnAttempt(ctx context.Context, attemptID string) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Warn("release shadow write panicked", slog.String("attempt", attemptID), slog.Any("panic", r))
		}
	}()
	if err := s.Record(ctx, attemptID); err != nil {
		s.logger.Warn("release shadow write failed", slog.String("attempt", attemptID), slog.String("error", err.Error()))
	}
}

// Record mints the release for a succeeded attempt that has a digest, then
// notes the binding it would imply. Anything else is a quiet no-op.
func (s *Shadow) Record(ctx context.Context, attemptID string) error {
	if !s.mode.Writes() {
		return nil
	}
	a, err := s.db.GetDeployAttempt(ctx, attemptID)
	if err != nil {
		return fmt.Errorf("load attempt: %w", err)
	}
	if a.Status != store.DeployAttemptStatusSucceeded || a.ImageDigest == "" || a.Image == "" {
		return nil
	}
	cfg, err := json.Marshal(Config{
		Health: a.Snapshot.Health, Replicas: a.Snapshot.Replicas, Strategy: a.Snapshot.Strategy,
		Resources: a.Snapshot.Resources, Labels: a.Snapshot.Labels,
	})
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	var projectID, envID string
	if svc, serr := s.db.GetDesiredService(ctx, a.ServiceName); serr == nil {
		projectID, envID = svc.ProjectID, svc.EnvironmentID
	}
	id, err := store.NewReleaseID()
	if err != nil {
		return err
	}
	if _, err := s.db.InsertRelease(ctx, store.Release{
		ID: id, ProjectID: projectID, SourceApp: a.ServiceName, SourceDeployAttemptID: a.ID,
		ImageRef: ImageRef(a.Image, a.ImageDigest), ImageTag: a.Image, Config: string(cfg), CommitSHA: a.CommitSHA,
	}); err != nil {
		return err
	}
	rel, err := s.db.GetReleaseByAttempt(ctx, a.ID)
	if err != nil || rel == nil {
		return err
	}
	_, err = s.db.RecordShadowBinding(ctx, a.ServiceName, envID, rel.ID, a.Source, a.Author)
	return err
}

// ImageRef pins a tag reference to its digest, replacing any existing tag.
func ImageRef(image, digest string) string {
	if i := strings.Index(image, "@"); i >= 0 {
		image = image[:i]
	}
	if i := strings.LastIndex(image, ":"); i > strings.LastIndex(image, "/") {
		image = image[:i]
	}
	return image + "@" + digest
}
