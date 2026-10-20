package main

import (
	"context"
	"log/slog"
	"os"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/profile"
	"github.com/GLINCKER/levelrail/internal/store"
)

const loopbackAgentAddr = "127.0.0.1:9443"

// profileRetention is the metrics and log retention the active profile sets when no retention env var does.
var profileRetention time.Duration

// resolveServerProfile picks the profile from env, then the saved choice, and
// records it before any subsystem reads it.
func resolveServerProfile(ctx context.Context, db *store.DB, logger *slog.Logger) {
	saved, err := db.GetServerProfile(ctx)
	if err != nil {
		logger.Warn("server profile unreadable, using standard", slog.String("error", err.Error()))
	}
	p, src := profile.Resolve(os.LookupEnv, saved.Profile)
	profile.SetActive(p, src)
	profileRetention = profile.Retention(p, diskTotalBytes(dataDirFromEnv()), os.LookupEnv)
	logger.Info("server profile", slog.String("profile", string(p)), slog.String("source", src))
}

func diskTotalBytes(dir string) int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0
	}
	return int64(st.Blocks) * int64(st.Bsize) //nolint:gosec // block counts of a real filesystem fit int64
}

// agentListenAddr binds the agent listener to loopback on minimal until a
// remote node exists; APP_AGENT_ADDR always wins.
func agentListenAddr(ctx context.Context, db *store.DB, localNodeID string) string {
	if os.Getenv("APP_AGENT_ADDR") != "" || !profile.For(profile.Active()).LoopbackAgent {
		return agentAddr()
	}
	nodes, err := db.ListNodes(ctx)
	if err != nil {
		return agentAddr()
	}
	for _, n := range nodes {
		if n.ID != localNodeID {
			return agentAddr()
		}
	}
	return loopbackAgentAddr
}

// startWhenWork runs start at once on a profile that does not defer it, and
// otherwise as soon as hasWork reports true, checking on a slow interval.
func startWhenWork(ctx context.Context, logger *slog.Logger, name string, hasWork func(context.Context) (bool, error), start func()) {
	if !profile.For(profile.Active()).LazyLoops {
		start()
		return
	}
	go func() {
		t := time.NewTimer(0)
		defer t.Stop()
		interval := profile.LazyStartInterval(os.LookupEnv)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			ok, err := hasWork(ctx)
			if err != nil {
				logger.Warn("deferred start check failed", slog.String("loop", name), slog.String("error", err.Error()))
			}
			if ok {
				logger.Info("starting deferred loop", slog.String("loop", name))
				start()
				return
			}
			t.Reset(interval)
		}
	}()
}

func hasScheduledBackups(db *store.DB) func(context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		n, err := db.CountScheduledBackups(ctx)
		return n > 0, err
	}
}
