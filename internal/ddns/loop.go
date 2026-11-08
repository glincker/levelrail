package ddns

import (
	"context"
	"github.com/GLINCKER/levelrail/internal/footprint"
	"log/slog"
	"sync"
	"time"
)

// Hooks connect the loop to the alert path owned elsewhere; both may be nil.
type Hooks struct {
	OnAlert   func(msg string)
	OnRecover func()
}

// Status is a snapshot for later API and CLI slices. It holds no secrets.
type Status struct {
	LastCheck   time.Time
	LastChange  time.Time
	LastIP      string
	LastError   string
	Failures    int
	AlertActive bool
}

// Updater detects the address and writes it through the provider.
type Updater struct {
	cfg      Config
	detector *Detector
	provider Provider
	log      *slog.Logger
	hooks    Hooks
	now      func() time.Time

	mu        sync.Mutex
	st        Status
	lastWrite time.Time
}

// New wires an updater; a nil logger discards output.
func New(cfg Config, d *Detector, p Provider, log *slog.Logger, hooks Hooks) *Updater {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Updater{cfg: cfg, detector: d, provider: p, log: log, hooks: hooks, now: time.Now}
}

// Carry is the write history a replacement loop inherits so the minimum gap and reassert timer keep applying.
type Carry struct {
	LastIP     string
	LastChange time.Time
	LastWrite  time.Time
}

// Carry snapshots the write history; it is zero until a write succeeded.
func (u *Updater) Carry() Carry {
	u.mu.Lock()
	defer u.mu.Unlock()
	return Carry{LastIP: u.st.LastIP, LastChange: u.st.LastChange, LastWrite: u.lastWrite}
}

// Seed restores write history from an earlier loop before Run starts.
func (u *Updater) Seed(c Carry) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.st.LastIP, u.st.LastChange, u.lastWrite = c.LastIP, c.LastChange, c.LastWrite
}

// StartFromEnv launches the loop only when enabled; it returns false when off.
func StartFromEnv(ctx context.Context, getenv func(string) string, log *slog.Logger, hooks Hooks) (bool, error) {
	if !Enabled(getenv) {
		return false, nil
	}
	cfg := ConfigFromEnv(getenv)
	p, err := ProviderFromEnv(getenv, cfg)
	if err != nil {
		return false, err
	}
	u := New(cfg, NewDetector(cfg), p, log, hooks)
	go u.Run(ctx)
	return true, nil
}

// Run ticks until ctx ends.
func (u *Updater) Run(ctx context.Context) {
	u.Tick(ctx)
	_ = footprint.Every(ctx, "ddns update", u.cfg.Interval, u.log, func(ctx context.Context) error {
		u.Tick(ctx)
		return nil
	})
}

// Status returns a copy of the current state.
func (u *Updater) Status() Status {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.st
}

// Tick runs one check. A panic or error here never escapes the loop.
func (u *Updater) Tick(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			u.fail("internal error in updater")
		}
	}()
	ip, err := u.detector.Detect(ctx)
	u.mu.Lock()
	u.st.LastCheck = u.now()
	changed := ip != "" && ip != u.st.LastIP
	reassert := ip != "" && !u.lastWrite.IsZero() && u.now().Sub(u.lastWrite) >= u.cfg.Reassert
	gapOK := u.lastWrite.IsZero() || u.now().Sub(u.lastWrite) >= u.cfg.MinUpdateGap
	u.mu.Unlock()
	if err != nil {
		u.fail(err.Error())
		return
	}
	if !changed && !reassert {
		return
	}
	if !gapOK {
		u.log.Info("ddns update deferred by minimum gap")
		return
	}
	if err := u.provider.Update(ctx, ip); err != nil {
		u.fail(err.Error())
		return
	}
	u.mu.Lock()
	recovered := u.st.AlertActive
	if changed {
		u.st.LastChange = u.now()
	}
	u.st.LastIP, u.st.LastError, u.st.Failures, u.st.AlertActive = ip, "", 0, false
	u.lastWrite = u.now()
	u.mu.Unlock()
	u.log.Info("ddns record updated")
	if recovered && u.hooks.OnRecover != nil {
		u.hooks.OnRecover()
	}
}

func (u *Updater) fail(msg string) {
	u.mu.Lock()
	u.st.Failures++
	u.st.LastError = msg
	raise := u.st.Failures >= u.cfg.FailAlertAfter && !u.st.AlertActive
	if raise {
		u.st.AlertActive = true
	}
	n := u.st.Failures
	u.mu.Unlock()
	u.log.Warn("ddns check failed", "error", msg, "consecutive", n)
	if raise && u.hooks.OnAlert != nil {
		u.hooks.OnAlert("dynamic DNS update failing: " + msg)
	}
}
