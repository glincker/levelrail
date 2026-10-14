package main

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/compliance"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

const complianceScheduleEvery = time.Hour

// setupCompliance enables access reviews and evidence packs. Packs are
// signed with a key derived from the master key, so without one only
// access reviews work.
func setupCompliance(ctx context.Context, logger *slog.Logger, db *store.DB, secretsManager *secrets.Manager, dataDir string, router *api.Router) {
	var signer *compliance.Signer
	if secretsManager != nil {
		signer = compliance.NewSigner(secretsManager.DeriveKey("evidence-pack"))
	}
	svc := compliance.NewService(ctx, db, filepath.Join(dataDir, "evidence"), signer,
		router.ComplianceSecuritySnapshot, router.ComplianceUserRemover(), logger)
	if err := svc.Recover(ctx); err != nil {
		logger.Warn("compliance: recover interrupted packs failed", slog.String("error", err.Error()))
	}
	router.SetCompliance(svc)
	go svc.RunScheduler(ctx, complianceScheduleEvery)
}
