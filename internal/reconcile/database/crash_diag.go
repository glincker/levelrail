package database

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

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
