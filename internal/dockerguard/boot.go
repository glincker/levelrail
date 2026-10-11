package dockerguard

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// BootConfig is what a binary passes to Boot.
type BootConfig struct {
	// DataDir holds the socket and settings file and is itself protected.
	DataDir string
	// ExtraProtected are further host paths no bind may cover.
	ExtraProtected []string
	// ResolveSymlinks is false when this process does not share the
	// daemon's view of the host filesystem.
	ResolveSymlinks bool
	Sink            Sink
	Logger          *slog.Logger
	Lookup          func(string) (string, bool)
}

// Booted is Boot's result. Host is empty when clients should dial Docker
// directly (mode off, or audit mode whose socket failed to start).
type Booted struct {
	Server     *Server
	Controller *Controller
	Grants     *Grants
	Host       string
}

// SocketPath is where the guard listens: APP_DOCKER_GUARD_SOCKET, or a
// private directory under dataDir.
func SocketPath(lookup func(string) (string, bool), dataDir string) string {
	if v, ok := envValue(lookup, EnvSocket); ok {
		return v
	}
	return filepath.Join(dataDir, "guard", SocketName)
}

// Boot resolves the mode and, unless off, starts the guard in front of the
// detected daemon socket. In enforce mode any failure is returned so the
// caller refuses to start rather than run unguarded.
func Boot(ctx context.Context, cfg BootConfig) (Booted, error) {
	lookup := cfg.Lookup
	if lookup == nil {
		lookup = os.LookupEnv
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	settingsPath := filepath.Join(cfg.DataDir, SettingsFile)
	settings, serr := LoadSettings(settingsPath)
	mode, source, merr := Resolve(lookup, settings)
	if merr != nil {
		logger.Error("dockerguard: invalid mode, failing closed to enforce", slog.String("error", merr.Error()))
	}
	if serr != nil {
		logger.Error("dockerguard: settings unreadable, failing closed to enforce", slog.String("error", serr.Error()))
		mode = ModeEnforce
	}
	runtime := docker.DetectRuntimeSocket(lookup)
	grants := NewGrants()
	out := Booted{Grants: grants}
	if mode == ModeOff {
		out.Controller = NewController(settingsPath, lookup, nil, runtime.Host, nil)
		logger.Warn("dockerguard: off, Docker clients use the daemon socket directly", slog.String("source", source))
		return out, nil
	}
	server, err := start(ctx, cfg, lookup, logger, mode, runtime, grants)
	if err != nil && mode == ModeEnforce {
		return Booted{}, fmt.Errorf("docker guard is in enforce mode but could not start: %w", err)
	}
	out.Controller = NewController(settingsPath, lookup, server, runtime.Host, err)
	if err != nil {
		logger.Warn("dockerguard: audit mode could not start, Docker clients use the daemon socket directly", slog.String("error", err.Error()))
		return out, nil
	}
	out.Server, out.Host = server, server.Host()
	if err := out.Controller.NoteAuditStart(time.Now()); err != nil {
		logger.Warn("dockerguard: could not record audit start", slog.String("error", err.Error()))
	}
	logger.Info("dockerguard: listening", slog.String("mode", string(mode)), slog.String("source", source), slog.String("socket", server.Socket), slog.String("upstream", runtime.Host))
	return out, nil
}

func start(ctx context.Context, cfg BootConfig, lookup func(string) (string, bool), logger *slog.Logger, mode Mode, runtime docker.RuntimeInfo, grants *Grants) (*Server, error) {
	upstream, err := UpstreamSocket(runtime.Host)
	if err != nil {
		return nil, err
	}
	socket := SocketPath(lookup, cfg.DataDir)
	if strings.TrimSuffix(socket, "/") == upstream {
		return nil, fmt.Errorf("dockerguard: guard socket %s is the daemon socket", socket)
	}
	tunables, terr := TunablesFromEnv(lookup)
	if terr != nil {
		logger.Warn("dockerguard: tunable invalid, using its default", slog.String("error", terr.Error()))
	}
	hardening, _ := docker.HardeningFromEnv(runtime)
	policy := PolicyFromHardening(hardening, tunables, append([]string{cfg.DataDir}, cfg.ExtraProtected...)...)
	policy.ResolveSymlinks = cfg.ResolveSymlinks
	g := New(Config{Mode: mode, Upstream: upstream, Policy: policy, Grants: grants, Tunables: tunables, Sink: cfg.Sink, Logger: logger})
	return Listen(ctx, g, socket)
}

// RunningInContainer reports whether this process sees a container's
// filesystem rather than the host's, where host path checks cannot resolve.
func RunningInContainer() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

// LogPrivilege records how much a compromise of this process would hold.
func LogPrivilege(ctx context.Context, logger *slog.Logger, client *docker.Client) {
	su := docker.CurrentServiceUser()
	rt := client.Runtime()
	attrs := []slog.Attr{
		slog.String("user", su.Name), slog.Bool("root", su.Root), slog.Bool("docker_group", su.DockerGroup),
		slog.String("runtime", string(rt.Kind)), slog.Bool("rootless_socket", rt.Rootless),
	}
	if sec, err := client.DaemonSecurity(ctx); err == nil {
		attrs = append(attrs, slog.Bool("daemon_rootless", sec.Rootless), slog.Bool("userns_remap", sec.UsernsRemap))
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "docker privilege", attrs...)
}
