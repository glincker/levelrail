package database

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbprovision"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
)

// EnvCrashLoopRestarts is how many container restarts inside the ready window count as a crash loop.
const EnvCrashLoopRestarts = "APP_DB_CRASH_LOOP_RESTARTS"

const (
	defaultCrashLoopRestarts = 3
	crashTailLines           = 30
)

// logTailer is what the local Docker client offers for reading a container's last output.
type logTailer interface {
	TailLogs(ctx context.Context, name string, lines int) (string, error)
}

func crashLoopThreshold() int {
	if n, err := strconv.Atoi(os.Getenv(EnvCrashLoopRestarts)); err == nil && n > 0 {
		return n
	}
	return defaultCrashLoopRestarts
}

// crashLoopReasons are the not-ready reasons where the container's own output explains the stop.
func crashLoopReasons(reason string) bool {
	switch reason {
	case "VanishedAfterStart", reasonNotAccepting, dbprovision.ReasonCrashLoop:
		return true
	}
	return false
}

// restartsOf reads the container's restart count, 0 when the runtime cannot report it.
func (c *Controller) restartsOf(ctx context.Context, name string) int {
	insp, ok := c.runtime.(docker.ExitStateInspector)
	if !ok {
		return 0
	}
	st, err := insp.InspectExitState(ctx, name)
	if err != nil || st == nil {
		return 0
	}
	return st.RestartCount
}

// captureCrash records the container's last output and exit facts on the attempt.
func (c *Controller) captureCrash(ctx context.Context) {
	target := containerName(c.dbName)
	info := dbprovision.CrashInfo{}
	if insp, ok := c.runtime.(docker.ExitStateInspector); ok {
		if st, err := insp.InspectExitState(ctx, target); err == nil && st != nil {
			info.ExitCode, info.OOMKilled, info.Restarts = st.ExitCode, st.OOMKilled, st.RestartCount
		}
	}
	if lt, ok := c.runtime.(logTailer); ok {
		tail, err := lt.TailLogs(ctx, target, crashTailLines)
		if err != nil {
			slog.Warn("database: read crash output failed", slog.String("database", c.dbName), slog.String("error", err.Error()))
		}
		info.Tail = tail
	}
	if info.Tail == "" && info.ExitCode == 0 && !info.OOMKilled {
		return
	}
	c.prov.LogTail(info)
}

// crashLoopResult stops the wait early when the container keeps restarting; ok is false when it does not.
func (c *Controller) crashLoopResult(ctx context.Context, target string) (reconcile.Result, bool) {
	n := c.restartsOf(ctx, target)
	if n < crashLoopThreshold() {
		return reconcile.Result{}, false
	}
	return notReady(dbprovision.ReasonCrashLoop, fmt.Errorf("the database container restarted %d times while starting", n)), true
}

// EnvCrashLoopWindow is how long quick crashes are counted, in seconds.
const EnvCrashLoopWindow = "APP_DB_CRASH_LOOP_WINDOW_SECONDS" //nolint:gosec // env var name

const (
	defaultCrashLoopWindow = 60 * time.Second
	crashBackoffBase       = 10 * time.Second
	crashBackoffMax        = 5 * time.Minute
)

// crashRecord counts restarts of a stopped container inside the window and
// holds the exponential back-off that keeps the reconciler from redeploying every pass.
type crashRecord struct {
	times []time.Time
	until time.Time
}

// crashGate runs before the reconciler restarts a stopped container. Past the
// threshold it reports CrashLoop with the last log line and waits longer each time.
func (c *Controller) crashGate(ctx context.Context, target string) (reconcile.Result, bool) {
	now := c.clock()
	window := durationFromEnv(EnvCrashLoopWindow, defaultCrashLoopWindow)
	n, wait := c.shared.noteCrash(c.dbName, now, window, crashLoopThreshold())
	if n < crashLoopThreshold() {
		return reconcile.Result{}, false
	}
	if wait <= 0 {
		return reconcile.Result{}, false
	}
	line := ""
	if lt, ok := c.runtime.(logTailer); ok {
		tail, _ := lt.TailLogs(ctx, target, 5)
		line = lastLine(tail)
	}
	res := notReady(dbprovision.ReasonCrashLoop, fmt.Errorf("the database container exited %d times within %s; retrying in %s. Last output: %s", n, window, wait.Round(time.Second), line))
	res.RequeueAfter = wait
	return res, true
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// noteCrash records one restart and returns the count in the window plus how
// long to hold off. A zero wait means the next restart may go ahead now.
func (s *SharedState) noteCrash(name string, now time.Time, window time.Duration, threshold int) (int, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.crashes[name]
	if r == nil {
		r = &crashRecord{}
		s.crashes[name] = r
	}
	if now.Before(r.until) {
		return len(r.times), r.until.Sub(now)
	}
	kept := r.times[:0]
	for _, t := range r.times {
		if now.Sub(t) < window {
			kept = append(kept, t)
		}
	}
	r.times = append(kept, now)
	over := len(r.times) - threshold
	if over < 0 {
		return len(r.times), 0
	}
	wait := crashBackoffBase << min(over, 6)
	wait = min(wait, crashBackoffMax)
	r.until = now.Add(wait)
	return len(r.times), wait
}
