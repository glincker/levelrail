package buildkitd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	bkclient "github.com/moby/buildkit/client"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// Runtime is the slice of docker.Runtime the supervisor needs.
type Runtime interface {
	InspectByName(ctx context.Context, name string) (*docker.ContainerState, error)
	Create(ctx context.Context, spec docker.ContainerSpec) (string, error)
	Start(ctx context.Context, id string) error
	Stop(ctx context.Context, id string, timeout time.Duration) error
	Remove(ctx context.Context, id string, force bool) error
	EnsureVolume(ctx context.Context, name string) error
}

// Supervisor keeps one healthy buildkitd at the pinned image. It is not wired
// into the build path yet.
type Supervisor struct {
	cfg Config
	rt  Runtime

	health      func(ctx context.Context, addr string, timeout time.Duration) error
	socketOK    func(path string) bool
	now         func() time.Time
	stopTimeout time.Duration

	mu       sync.Mutex
	inflight int
	lastUse  time.Time
}

// New builds a supervisor and refuses an unpinned image.
func New(cfg Config, rt Runtime) (*Supervisor, error) {
	if err := CheckPinned(cfg.Image); err != nil {
		return nil, err
	}
	return &Supervisor{
		cfg:         cfg,
		rt:          rt,
		health:      workersHealth,
		socketOK:    func(p string) bool { _, err := os.Stat(p); return err == nil },
		now:         time.Now,
		stopTimeout: 30 * time.Second,
	}, nil
}

func workersHealth(ctx context.Context, addr string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	c, err := bkclient.New(ctx, addr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	_, err = c.ListWorkers(ctx)
	return err
}

// Acquire ensures a healthy daemon and returns its socket address. Callers
// Release when the solve ends so idle stop can run.
func (s *Supervisor) Acquire(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reconcile(ctx); err != nil {
		return "", err
	}
	s.inflight++
	s.lastUse = s.now()
	return s.cfg.SocketAddr(), nil
}

// Release marks one build finished.
func (s *Supervisor) Release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight > 0 {
		s.inflight--
	}
	s.lastUse = s.now()
}

// IdleStop stops the daemon once no build ran for Config.IdleStop and reports
// whether it did. An IdleStop of 0 keeps the daemon running.
func (s *Supervisor) IdleStop(ctx context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.IdleStop <= 0 || s.inflight > 0 || s.lastUse.IsZero() || s.now().Sub(s.lastUse) < s.cfg.IdleStop {
		return false, nil
	}
	st, err := s.rt.InspectByName(ctx, ContainerName)
	if err != nil || st == nil || !st.Running {
		return false, err
	}
	if err := s.rt.Stop(ctx, st.ID, s.stopTimeout); err != nil {
		return false, err
	}
	return true, nil
}

// reconcile drives observed state to one running, healthy daemon at the pinned
// image. A running but unhealthy daemon gets exactly one restart.
func (s *Supervisor) reconcile(ctx context.Context) error {
	if err := ensureDir0700(s.cfg.SocketDir); err != nil {
		return fmt.Errorf("buildkitd: socket dir: %w", err)
	}
	if err := s.writeConfig(); err != nil {
		return err
	}
	if err := s.rt.EnsureVolume(ctx, VolumeName); err != nil {
		return fmt.Errorf("buildkitd: state volume: %w", err)
	}
	st, err := s.rt.InspectByName(ctx, ContainerName)
	if err != nil {
		return fmt.Errorf("buildkitd: inspect: %w", err)
	}
	if st != nil && st.Image != s.cfg.Image {
		slog.Info("buildkitd image changed, replacing", "from", st.Image, "to", s.cfg.Image)
		if err := s.remove(ctx, st); err != nil {
			return err
		}
		st = nil
	}
	if st == nil {
		id, err := s.rt.Create(ctx, s.spec())
		if err != nil {
			return fmt.Errorf("buildkitd: create: %w", err)
		}
		if err := s.rt.Start(ctx, id); err != nil {
			return fmt.Errorf("buildkitd: start: %w", err)
		}
		return s.healthy(ctx)
	}
	if !st.Running {
		if err := s.rt.Start(ctx, st.ID); err != nil {
			return fmt.Errorf("buildkitd: start: %w", err)
		}
		return s.healthy(ctx)
	}
	if s.healthy(ctx) == nil {
		return nil
	}
	if err := s.rt.Stop(ctx, st.ID, s.stopTimeout); err != nil {
		return fmt.Errorf("buildkitd: stop: %w", err)
	}
	if err := s.rt.Start(ctx, st.ID); err != nil {
		return fmt.Errorf("buildkitd: restart: %w", err)
	}
	return s.healthy(ctx)
}

func (s *Supervisor) healthy(ctx context.Context) error {
	if !s.socketOK(filepath.Join(s.cfg.SocketDir, socketName)) {
		return fmt.Errorf("%w: socket missing", ErrUnavailable)
	}
	if err := s.health(ctx, s.cfg.SocketAddr(), s.cfg.HealthTimeout); err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return nil
}

func (s *Supervisor) remove(ctx context.Context, st *docker.ContainerState) error {
	if st.Running {
		if err := s.rt.Stop(ctx, st.ID, s.stopTimeout); err != nil {
			return fmt.Errorf("buildkitd: stop old: %w", err)
		}
	}
	if err := s.rt.Remove(ctx, st.ID, false); err != nil {
		return fmt.Errorf("buildkitd: remove old: %w", err)
	}
	return nil
}

func (s *Supervisor) writeConfig() error {
	if err := ensureDir0700(s.cfg.ConfigDir); err != nil {
		return fmt.Errorf("buildkitd: config dir: %w", err)
	}
	return os.WriteFile(filepath.Join(s.cfg.ConfigDir, "buildkitd.toml"), []byte(s.cfg.tomlBody()), 0o600)
}

// spec has no network and no published ports. The socket directory and the
// generated config are bind mounted, state lives on a named volume.
func (s *Supervisor) spec() docker.ContainerSpec {
	return docker.ContainerSpec{
		Name:        ContainerName,
		Image:       s.cfg.Image,
		NetworkMode: "none",
		Labels:      map[string]string{"levelrail.managed": "true", "levelrail.role": "buildkitd"},
		Command: []string{
			"buildkitd", "--addr", "unix://" + containerSocketDir + "/" + socketName,
			"--config", "/etc/buildkit/buildkitd.toml",
		},
		Volumes: []docker.VolumeMount{{Name: VolumeName, ContainerPath: "/home/user/.local/share/buildkit"}},
		BindMounts: []docker.BindMount{
			{HostPath: s.cfg.SocketDir, ContainerPath: containerSocketDir},
			{HostPath: s.cfg.ConfigDir, ContainerPath: "/etc/buildkit", ReadOnly: true},
		},
	}
}
