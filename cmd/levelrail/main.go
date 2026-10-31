// Command levelrail is the control plane binary.
// Controllers are derived from desired state on every reconcile pass (see dynamicSource).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/selfupgrade"
)

const (
	resyncInterval = 30 * time.Second
	// Relative, not /var/lib/levelrail-data: Phase 0 runs locally, not as an installed service.
	defaultDataDir = "./data"
	// defaultBrandFile is the same "relative default, env override"
	// pattern as defaultDataDir.
	defaultBrandFile = "./brand.yaml"
	// Absence is not an error: the What's new panel just shows no entries.
	defaultChangelogFile = "./CHANGELOG.md"
	// Absence is not an error: the built-in manifest config is used instead.
	defaultGitHubAppManifestFile = "./deploy/github-app-manifest.yaml"
	// Only read when APP_DEV_MODE=1.
	defaultDevFixturesFile = "./test/fixtures/dev-fixtures.yml"
	// defaultHTTPAddr is where the HTTP API listens.
	defaultHTTPAddr = ":8080"

	// Overridable so a second control plane on the same host does not fail to bind :443.
	defaultIngressHTTPSAddr = ":443"
	// Overridable via APP_INGRESS_HTTP_ADDR.
	defaultIngressHTTPAddr = ":80"

	// A separate port: agent gRPC uses mTLS with its own CA, unlike the HTTP API's session auth.
	defaultAgentAddr = ":9443"
	// Regenerated on each start; only the CA persists, so agents keep trusting it.
	agentServerCertValidity = 365 * 24 * time.Hour

	httpShutdownTimeout = 10 * time.Second

	// shutdownDrainEnv bounds how long SIGTERM waits for a running reconcile pass and queued deploys.
	shutdownDrainEnv     = "APP_SHUTDOWN_DRAIN"
	defaultShutdownDrain = 20 * time.Second

	// Bounded so a live agent Session stream cannot stall exit until systemd SIGKILLs the process.
	agentGRPCShutdownTimeout = 5 * time.Second

	// Not env-configurable: resolution changes the stored sample shape, retention does not.
	metricsCollectionInterval = 15 * time.Second

	// Separate from resyncInterval: log stream subscriptions are a different concern.
	logTargetsResyncInterval = 30 * time.Second
	// defaultLogsRetention matches the observability design's "default
	// 15 days," the same default metrics uses, applied to the sibling
	// log store.
	defaultLogsRetention = 15 * 24 * time.Hour
	// logsRetentionSweepInterval mirrors metricsRetentionSweepInterval's
	// own reasoning: frequent enough that log_entries never grows much
	// past its steady-state size, infrequent enough to be a non-event.
	logsRetentionSweepInterval = 1 * time.Hour

	// Coarser than metrics collection: a rule's ForDuration already debounces noisy samples.
	alertEvaluationInterval = 30 * time.Second

	// Resync only: start events are handled continuously by the event stream.
	restartTrackerResyncInterval = 30 * time.Second

	// Duplicated rather than imported so this file's env-var convention stays self-contained.
	defaultNodeHeartbeatInterval = 15 * time.Second
	// Three missed heartbeats: long enough to ride out a GC pause, short enough to catch a hang.
	defaultNodeHeartbeatTimeout = 45 * time.Second

	// Must stay under defaultNodeHeartbeatTimeout so the transport notices before nodehealth does.
	defaultNodeKeepaliveTime    = 10 * time.Second
	defaultNodeKeepaliveTimeout = 10 * time.Second

	// Shells out to a package manager, so an hour is a conservative default.
	defaultOSPatchCheckInterval = 1 * time.Hour

	// One minute: standard cron has no finer granularity, and a coarser check would miss minute schedules.
	defaultBackupSchedulerInterval = 1 * time.Minute

	// Same one-minute value as the backup scheduler, for the same cron granularity reason.
	defaultScheduledTaskSchedulerInterval = 1 * time.Minute

	// Same one-minute value as the backup scheduler, for the same cron granularity reason.
	defaultAppScheduleSchedulerInterval = 1 * time.Minute
	defaultAppScheduleOneShotCatchUp    = 24 * time.Hour
	defaultScheduledDeployGrace         = 5 * time.Minute
	defaultScheduledDeployStaleRun      = 30 * time.Minute

	// An hour: a days-scale TTL needs no minute-granularity checks.
	defaultPreviewSweepInterval = 1 * time.Hour

	// An hour: a release check is outbound network cost with no benefit from polling faster.
	defaultUpdateCheckInterval = 1 * time.Hour

	// An hour: a days-scale retention window needs no finer checks.
	defaultAuditLogSweepInterval = 1 * time.Hour

	// Hourly, like the audit log sweep, for a days-scale retention window.
	defaultWebhookDeliverySweepInterval = 1 * time.Hour

	// Minutes, not hourly: a stale pending approval should show as expired promptly.
	defaultDeployApprovalSweepInterval = 5 * time.Minute

	// Budgets sit well above normal dashboard, CLI, and deploy cadence; only runaway loops hit them.
	defaultAPIRateLimitReadRPM  = 600
	defaultAPIRateLimitWriteRPM = 120

	// Well above any real git provider's delivery cadence; only a client hammering this route hits it.
	defaultWebhookRateLimitRPM = 60

	// defaultTokenRedeemRateLimitRPM is the per-IP budget for reset-password
	// and invite-accept when APP_API_RATE_LIMIT_TOKEN_REDEEM_RPM is unset.
	defaultTokenRedeemRateLimitRPM = 10
)

func main() {
	if code, handled := bootArgsExit(os.Args[1:], os.Args[0], os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}
	logger := slog.New(secretmask.Handler(slog.NewTextHandler(os.Stdout, nil)))

	if len(os.Args) > 1 && hostCommands[os.Args[1]] {
		adoptInstalledDataDir(systemdUnitDir)
	}
	if len(os.Args) > 1 && runReleaseCommand(os.Args[1], os.Args[2:]) {
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "demo" {
		if err := runDemo(context.Background(), logger, os.Args[2:], os.Stdout); err != nil {
			logger.Error("demo failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "recover-admin" {
		if err := runRecoverAdmin(context.Background(), logger, os.Args[2:], os.Stdout, openStore); err != nil {
			logger.Error("recover-admin failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "plugin-egress" {
		if err := runPluginEgressSignals(logger); err != nil {
			logger.Error("plugin-egress failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "approve-signin" {
		if err := runApproveSignIn(context.Background(), logger, os.Args[2:], os.Stdout, openStore); err != nil {
			logger.Error("approve-signin failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "emergency-token" {
		if err := runEmergencyToken(context.Background(), logger, os.Args[2:], os.Stdout, openStore); err != nil {
			if errors.Is(err, errEmergencyHelp) {
				fmt.Println(err.Error())
				return
			}
			logger.Error("emergency-token failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "access-policy-reset" {
		if err := runAccessPolicyReset(context.Background(), os.Args[2:], os.Stdout, openStore); err != nil {
			logger.Error("access-policy-reset failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "restore-db" {
		if err := runRestoreDB(context.Background(), os.Args[2:], dataDirFromEnv(), os.Stdout); err != nil {
			logger.Error("restore-db failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "wrap-master-key" {
		if err := runWrapMasterKey(context.Background(), os.Args[2:], dataDirFromEnv(), os.Stdout, os.Getenv); err != nil {
			logger.Error("wrap-master-key failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "restore-snapshot" {
		if err := runRestoreSnapshot(context.Background(), os.Args[2:], dataDirFromEnv(), os.Stdin, os.Stdout); err != nil {
			logger.Error("restore-snapshot failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "restore" {
		if err := runRestore(context.Background(), os.Args[2:], dataDirFromEnv(), os.Stdout, os.LookupEnv); err != nil {
			logger.Error("restore failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "setup-token" {
		if err := runSetupToken(context.Background(), os.Stdout, len(os.Args) > 2 && os.Args[2] == "--plain", openStore); err != nil {
			logger.Error("setup-token failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(context.Background(), os.Stdout, len(os.Args) > 2 && os.Args[2] == "--ready"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if err := run(logger); err != nil {
		var refusal *downgradeRefusal
		if errors.As(err, &refusal) {
			fmt.Fprint(os.Stderr, refusal.msg)
			os.Exit(selfupgrade.ExitDowngradeRefused)
		}
		if errors.Is(err, errRestartRequested) {
			logger.Info("exiting for restart", slog.Int("code", exitRestart))
			os.Exit(exitRestart)
		}
		logger.Error("exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
