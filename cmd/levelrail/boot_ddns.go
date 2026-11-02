package main

import (
	"log/slog"
	"os"

	"github.com/GLINCKER/levelrail/internal/ddns"
)

// startDDNS runs the dynamic DNS updater from the stored setting; env only seeds the defaults.
func (s *bootState) startDDNS() {
	// No alert rule source exists for a loop-raised event yet, so alerts go to the log.
	hooks := ddns.Hooks{
		OnAlert:   func(msg string) { s.logger.Error("ddns alert", slog.String("message", msg)) },
		OnRecover: func() { s.logger.Info("ddns recovered") },
	}
	var sec ddns.Secrets
	if s.secretsManager != nil {
		sec = s.secretsManager
	}
	m := ddns.NewManager(s.ctx, sec, os.Getenv, s.logger, hooks)
	s.apiRouter.SetDDNS(m)
	if err := m.Reconcile(s.ctx); err != nil {
		s.logger.Warn("ddns not started", slog.String("error", err.Error()))
	}
}
