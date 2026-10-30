package main

import (
	"log/slog"
	"os"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/analysis"
	"github.com/GLINCKER/levelrail/internal/store"
)

// wireAnalysisHook returns the runner (nil when disabled) and attaches report generation to alert firings. With
// APP_ANALYSIS_ENABLED unset the runner is nil and nothing is attached.
func wireAnalysisHook(n *alerting.NoiseControl, db *store.DB, tel analysis.TelemetryReader, adb *alerting.DB, logger *slog.Logger) *analysis.Runner {
	runner := analysis.NewRunner(analysis.ConfigFromEnv(os.LookupEnv), analysis.Sources{Telemetry: tel, Deploys: db, Alerts: adb, Services: db}, db, logger)
	if runner == nil {
		return nil
	}
	n.SetFiredHook(runner.OnAlert)
	return runner
}
