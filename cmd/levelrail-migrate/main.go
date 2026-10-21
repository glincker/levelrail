// Read-only migration agent for the server being migrated away from. It dials
// out to the control plane, reports what is running and counts files in
// volumes. It never starts, stops, changes or execs into anything.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/GLINCKER/levelrail/internal/agent/migrationmode"
	"github.com/GLINCKER/levelrail/internal/version"
)

type settings struct {
	addr, token, caFingerprint string
	stateDir, socket           string
	heartbeat, scanTimeout     time.Duration
	reconnectMax               time.Duration
	maxResponse                int64
	scanLimits                 migrationmode.ScanLimits
}

func envInt(key string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return def
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d > 0 {
		return d
	}
	return def
}

func exeName() string {
	exe, err := os.Executable()
	if err != nil {
		return "migrate-agent"
	}
	return filepath.Base(exe)
}

func loadSettings() settings {
	maxResp := int64(32 << 20)
	if n, err := strconv.ParseInt(os.Getenv("APP_MIGRATE_MAX_RESPONSE"), 10, 64); err == nil && n > 0 {
		maxResp = n
	}
	return settings{
		addr:          os.Getenv("APP_CONTROL_PLANE_ADDR"),
		token:         os.Getenv("APP_MIGRATION_TOKEN"),
		caFingerprint: os.Getenv("APP_CA_FINGERPRINT"),
		stateDir:      envOr("APP_MIGRATE_STATE_DIR", filepath.Join("/var/lib", exeName())),
		socket:        envOr("APP_MIGRATE_DOCKER_SOCKET", "/var/run/docker.sock"),
		heartbeat:     envDuration("APP_MIGRATE_HEARTBEAT", 15*time.Second),
		scanTimeout:   envDuration("APP_MIGRATE_SCAN_TIMEOUT", 10*time.Minute),
		scanLimits: migrationmode.ScanLimits{MaxTargets: envInt("APP_MIGRATE_SCAN_MAX_TARGETS", 64),
			MaxEntries: int64(envInt("APP_MIGRATE_SCAN_MAX_ENTRIES", 20_000_000)), MaxDepth: envInt("APP_MIGRATE_SCAN_MAX_DEPTH", 128)},
		reconnectMax: envDuration("APP_MIGRATE_RECONNECT_MAX", 30*time.Second),
		maxResponse:  maxResp,
	}
}

func (s settings) identityPath() string { return filepath.Join(s.stateDir, "identity.json") }

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(logger, os.Args[1:]); err != nil {
		logger.Error("exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(logger *slog.Logger, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if len(args) == 0 {
		return errors.New("usage: " + exeName() + " install|run|remove|version")
	}
	cfg := loadSettings()
	switch args[0] {
	case "version":
		fmt.Println(version.Version)
		return nil
	case "install":
		return install(ctx, cfg, logger)
	case "run":
		return serve(ctx, cfg, logger)
	case "remove":
		return remove(ctx, cfg, logger)
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func (s settings) requireDial() error {
	if s.addr == "" {
		return errors.New("APP_CONTROL_PLANE_ADDR must be set")
	}
	return nil
}

func (s settings) enrollAndSave(ctx context.Context, logger *slog.Logger) (*migrationmode.Identity, error) {
	if err := s.requireDial(); err != nil {
		return nil, err
	}
	if s.token == "" {
		return nil, errors.New("APP_MIGRATION_TOKEN must be set to enroll")
	}
	id, err := migrationmode.Enroll(ctx, s.addr, s.token, s.caFingerprint, version.Version)
	if err != nil {
		return nil, err
	}
	if err := migrationmode.SaveIdentity(s.identityPath(), id); err != nil {
		return nil, err
	}
	logger.Info("enrolled", slog.String("session_id", id.SessionID), slog.Time("session_expires_at", id.ExpiresAt))
	return id, nil
}

func (s settings) handler(logger *slog.Logger) *migrationmode.Handler {
	onDeny := func(method, path, reason string) {
		logger.Warn("docker call denied", slog.String("method", method), slog.String("path", path), slog.String("reason", reason))
	}
	return &migrationmode.Handler{
		Docker: migrationmode.NewDocker(s.socket, s.maxResponse, onDeny), Scanner: migrationmode.NewScannerWithLimits(s.scanLimits),
		ScanTimeout: s.scanTimeout, Now: time.Now, Log: logger,
	}
}

func serve(ctx context.Context, cfg settings, logger *slog.Logger) error {
	if err := cfg.requireDial(); err != nil {
		return err
	}
	id, err := migrationmode.LoadIdentity(cfg.identityPath())
	if errors.Is(err, os.ErrNotExist) {
		id, err = cfg.enrollAndSave(ctx, logger)
	}
	if err != nil {
		return err
	}
	h := cfg.handler(logger)
	delay := time.Second
	for ctx.Err() == nil {
		if time.Now().After(id.ExpiresAt) || time.Now().After(id.NotAfter) {
			logger.Info("session cap reached, removing identity")
			return wipe(cfg)
		}
		err := migrationmode.Connect(ctx, cfg.addr, id, h, version.Version, cfg.heartbeat)
		if ended(err) {
			logger.Info("session ended by the control plane, removing identity", slog.String("reason", err.Error()))
			return wipe(cfg)
		}
		logger.Warn("session dropped", slog.String("error", fmt.Sprint(err)))
		select {
		case <-ctx.Done():
		case <-time.After(delay):
		}
		delay = min(delay*2, cfg.reconnectMax)
	}
	return nil
}

func ended(err error) bool {
	if errors.Is(err, migrationmode.ErrGoAway) || errors.Is(err, migrationmode.ErrUnknownControl) {
		return true
	}
	s, ok := status.FromError(err)
	return ok && (s.Code() == codes.PermissionDenied || s.Code() == codes.Unauthenticated)
}

func wipe(cfg settings) error {
	if err := os.Remove(cfg.identityPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove identity: %w", err)
	}
	return nil
}
