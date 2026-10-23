package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/exposure"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
)

// Shared with cmd/levelrail-agent through the same env var, so one setting governs both cadences by default.
func nodeHeartbeatInterval() time.Duration {
	raw := os.Getenv("APP_NODE_HEARTBEAT_INTERVAL")
	if raw == "" {
		return defaultNodeHeartbeatInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return defaultNodeHeartbeatInterval
	}
	return d
}

// nodeHeartbeatTimeout reads APP_NODE_HEARTBEAT_TIMEOUT,
// how long since last_seen_at internal/reconcile/nodehealth treats as
// stale.
func nodeHeartbeatTimeout() time.Duration {
	raw := os.Getenv("APP_NODE_HEARTBEAT_TIMEOUT")
	if raw == "" {
		return defaultNodeHeartbeatTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return defaultNodeHeartbeatTimeout
	}
	return d
}

// nodeKeepaliveTime reads APP_NODE_KEEPALIVE_TIME, how often the agent
// gRPC server sends an HTTP/2 PING on an otherwise-idle connection.
func nodeKeepaliveTime() time.Duration {
	raw := os.Getenv("APP_NODE_KEEPALIVE_TIME")
	if raw == "" {
		return defaultNodeKeepaliveTime
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return defaultNodeKeepaliveTime
	}
	return d
}

// nodeKeepaliveTimeout reads APP_NODE_KEEPALIVE_TIMEOUT, how long the
// agent gRPC server waits for a PING ack before considering the
// connection dead.
func nodeKeepaliveTimeout() time.Duration {
	raw := os.Getenv("APP_NODE_KEEPALIVE_TIMEOUT")
	if raw == "" {
		return defaultNodeKeepaliveTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return defaultNodeKeepaliveTimeout
	}
	return d
}

// osPatchCheckInterval reads APP_OS_PATCH_CHECK_INTERVAL, the same
// env-var-with-default shape as nodeHeartbeatInterval above.
func osPatchCheckInterval() time.Duration {
	raw := os.Getenv("APP_OS_PATCH_CHECK_INTERVAL")
	if raw == "" {
		return defaultOSPatchCheckInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return defaultOSPatchCheckInterval
	}
	return d
}

// Resolved here, not in the scheduler, which takes an interval with no fallback of its own.
func backupSchedulerInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_BACKUP_SCHEDULER_INTERVAL")
	if raw == "" {
		return defaultBackupSchedulerInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_BACKUP_SCHEDULER_INTERVAL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return defaultBackupSchedulerInterval
	}
	return d
}

func scheduledTaskSchedulerInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_SCHEDULED_TASK_SCHEDULER_INTERVAL")
	if raw == "" {
		return defaultScheduledTaskSchedulerInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_SCHEDULED_TASK_SCHEDULER_INTERVAL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return defaultScheduledTaskSchedulerInterval
	}
	return d
}

// appScheduleOneShotCatchUp bounds how late a one-time scheduled deploy may
// still fire after its run_at (default 24h); 0 disables the bound.
func appScheduleOneShotCatchUp(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_SCHEDULE_ONESHOT_CATCHUP")
	if raw == "" {
		return defaultAppScheduleOneShotCatchUp
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		logger.Warn("invalid APP_SCHEDULE_ONESHOT_CATCHUP, using the default", slog.String("value", raw))
		return defaultAppScheduleOneShotCatchUp
	}
	return d
}

func appScheduleSchedulerInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_SCHEDULE_DEPLOY_SCHEDULER_INTERVAL")
	if raw == "" {
		return defaultAppScheduleSchedulerInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_SCHEDULE_DEPLOY_SCHEDULER_INTERVAL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return defaultAppScheduleSchedulerInterval
	}
	return d
}

func httpAddr() string {
	return resolveHTTPAddr(os.Getenv, runningInContainer())
}

// ingressHTTPSAddr reads APP_INGRESS_HTTPS_ADDR, defaulting to
// defaultIngressHTTPSAddr, and is passed to
// ingressreconcile.WithListenAddr.
func ingressHTTPSAddr() string {
	addr := os.Getenv("APP_INGRESS_HTTPS_ADDR")
	if addr == "" {
		addr = defaultIngressHTTPSAddr
	}
	return addr
}

// ingressHTTPAddr reads APP_INGRESS_HTTP_ADDR, defaulting to
// defaultIngressHTTPAddr, and is passed to
// ingressreconcile.WithHTTPListenAddr.
func ingressHTTPAddr() string {
	addr := os.Getenv("APP_INGRESS_HTTP_ADDR")
	if addr == "" {
		addr = defaultIngressHTTPAddr
	}
	return addr
}

// An unparseable address degrades to 0, which means the default ports.
func ingressPortFromAddr(addr string) int {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0
	}
	return port
}

// Passed to both the API and the firewall reconciler so no firewall rule can close a platform port.
func newExposureManager(prefix string) *exposure.Manager {
	protected := append(platformRequiredPorts(), api.HostFirewallSSHPorts()...)
	return exposure.NewManager(prefix, protected)
}

func platformRequiredPorts() []int {
	return []int{
		ingressPortFromAddr(httpAddr()),
		ingressPortFromAddr(agentAddr()),
		ingressPortFromAddr(ingressHTTPAddr()),
		ingressPortFromAddr(ingressHTTPSAddr()),
	}
}

// Caddy and the control plane share a loopback interface, so a wildcard bind address is not a dial target.
// An explicit host set by the operator is passed through unchanged.
func dashboardDialAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return ""
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// Returns 0 when unset or invalid so api.NewRouter applies its default.
func sessionTTL(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_SESSION_TTL")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_SESSION_TTL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func slowRequestThreshold(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_SLOW_REQUEST_THRESHOLD")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_SLOW_REQUEST_THRESHOLD, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// criticalRequestThreshold reads APP_CRITICAL_REQUEST_THRESHOLD, the
// same shape as slowRequestThreshold but for the Error band.
func criticalRequestThreshold(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_CRITICAL_REQUEST_THRESHOLD")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_CRITICAL_REQUEST_THRESHOLD, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Defaults to enabled; an omitted node_id is auto-placed onto the least-loaded node.
func autoPlacementEnabled(logger *slog.Logger) bool {
	raw := os.Getenv("APP_AUTO_PLACEMENT")
	if raw == "" {
		return true
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		logger.Warn("invalid APP_AUTO_PLACEMENT, defaulting to enabled", slog.String("value", raw), slog.String("error", err.Error()))
		return true
	}
	return v
}

// Opt-in: HSTS is only safe with browser-trusted certificates, which this process cannot verify.
func hstsEnabled(logger *slog.Logger) bool {
	raw := os.Getenv("APP_ENABLE_HSTS")
	if raw == "" {
		return false
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		logger.Warn("invalid APP_ENABLE_HSTS, defaulting to disabled", slog.String("value", raw), slog.String("error", err.Error()))
		return false
	}
	return v
}

// Shared by the API and the cert-expiry alert rule so they agree on when a certificate becomes a concern.
func certExpiryWarningWindow(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_CERT_EXPIRY_WARNING_WINDOW")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_CERT_EXPIRY_WARNING_WINDOW, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so alerting applies its default.
func certRenewalStalledThreshold(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_CERT_RENEWAL_STALLED_THRESHOLD")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_CERT_RENEWAL_STALLED_THRESHOLD, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so alerting applies its default.
func patchStatusThreshold(logger *slog.Logger) float64 {
	raw := os.Getenv("APP_ALERT_PATCH_STATUS_THRESHOLD")
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		logger.Warn("invalid APP_ALERT_PATCH_STATUS_THRESHOLD, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return v
}

// Returns 0 when unset or invalid so alerting applies its default.
func nodeDiskSpaceThreshold(logger *slog.Logger) float64 {
	raw := os.Getenv("APP_ALERT_NODE_DISK_SPACE_THRESHOLD_PERCENT")
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		logger.Warn("invalid APP_ALERT_NODE_DISK_SPACE_THRESHOLD_PERCENT, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return v
}

// Returns 0 when unset or invalid so alerting applies its default.
func nodeCPUThreshold(logger *slog.Logger) float64 {
	raw := os.Getenv("APP_ALERT_NODE_CPU_THRESHOLD_PERCENT")
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		logger.Warn("invalid APP_ALERT_NODE_CPU_THRESHOLD_PERCENT, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return v
}

// Bytes, not percent: memory has no honest node-capacity percentage. Returns 0 when unset or invalid.
func nodeMemoryThreshold(logger *slog.Logger) float64 {
	raw := os.Getenv("APP_ALERT_NODE_MEMORY_THRESHOLD_BYTES")
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		logger.Warn("invalid APP_ALERT_NODE_MEMORY_THRESHOLD_BYTES, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return v
}

// Each check is a real DNS lookup, so this runs on its own interval, not the evaluation tick.
func domainHealthCheckInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_ALERT_DOMAIN_HEALTH_CHECK_INTERVAL")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_ALERT_DOMAIN_HEALTH_CHECK_INTERVAL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so alerting applies its default.
func backupMissingGracePeriod(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_ALERT_BACKUP_MISSING_GRACE_PERIOD")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_ALERT_BACKUP_MISSING_GRACE_PERIOD, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func previewTTL(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_PREVIEW_TTL")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_PREVIEW_TTL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func inviteTTL(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_INVITE_TTL")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_INVITE_TTL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// updateCheckInterval reads APP_UPDATE_CHECK_INTERVAL as a Go duration
// string, the same env-var-with-default shape backupSchedulerInterval
// already uses.
func updateCheckInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_UPDATE_CHECK_INTERVAL")
	if raw == "" {
		return defaultUpdateCheckInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_UPDATE_CHECK_INTERVAL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return defaultUpdateCheckInterval
	}
	return d
}

// Resolved here: RunPreviewSweeper takes its interval with no fallback of its own.
func previewSweepInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_PREVIEW_SWEEP_INTERVAL")
	if raw == "" {
		return defaultPreviewSweepInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_PREVIEW_SWEEP_INTERVAL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return defaultPreviewSweepInterval
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func auditLogRetention(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_AUDIT_LOG_RETENTION_DAYS")
	if raw == "" {
		return 0
	}
	days, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		logger.Warn("invalid APP_AUDIT_LOG_RETENTION_DAYS, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return time.Duration(days) * 24 * time.Hour
}

// Returns 0 when unset or invalid so api applies its default.
func webhookDeliveryRetention(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_WEBHOOK_DELIVERY_RETENTION_DAYS")
	if raw == "" {
		return 0
	}
	days, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		logger.Warn("invalid APP_WEBHOOK_DELIVERY_RETENTION_DAYS, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return time.Duration(days) * 24 * time.Hour
}

// positiveIntEnv returns the env var as a positive int, or 0 (the api
// package's signal to use its own default) when unset or invalid.
func positiveIntEnv(logger *slog.Logger, name string) int {
	raw := os.Getenv(name)
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		logger.Warn("invalid "+name+", using the default", slog.String("value", raw))
		return 0
	}
	return n
}

// Resolved here: RunWebhookDeliverySweeper takes its interval with no fallback of its own.
func webhookDeliverySweepInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_WEBHOOK_DELIVERY_SWEEP_INTERVAL")
	if raw == "" {
		return defaultWebhookDeliverySweepInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_WEBHOOK_DELIVERY_SWEEP_INTERVAL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return defaultWebhookDeliverySweepInterval
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func secretRotationWarnAge(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_SECRET_ROTATION_WARN_DAYS")
	if raw == "" {
		return 0
	}
	days, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		logger.Warn("invalid APP_SECRET_ROTATION_WARN_DAYS, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return time.Duration(days) * 24 * time.Hour
}

// Returns 0 when unset or invalid so database applies its default.
func databaseSlowQueryThresholdMs(logger *slog.Logger) int {
	raw := os.Getenv("APP_DATABASE_SLOW_QUERY_THRESHOLD_MS")
	if raw == "" {
		return 0
	}
	ms, err := strconv.Atoi(raw)
	if err != nil {
		logger.Warn("invalid APP_DATABASE_SLOW_QUERY_THRESHOLD_MS, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return ms
}

// Resolved here: RunAuditLogSweeper takes its interval with no fallback of its own.
func auditLogSweepInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_AUDIT_LOG_SWEEP_INTERVAL")
	if raw == "" {
		return defaultAuditLogSweepInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_AUDIT_LOG_SWEEP_INTERVAL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return defaultAuditLogSweepInterval
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func deployApprovalTTL(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_DEPLOY_APPROVAL_TTL")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_DEPLOY_APPROVAL_TTL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Resolved here: RunDeployApprovalExpirySweep takes its interval with no fallback of its own.
func deployApprovalSweepInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_DEPLOY_APPROVAL_SWEEP_INTERVAL")
	if raw == "" {
		return defaultDeployApprovalSweepInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_DEPLOY_APPROVAL_SWEEP_INTERVAL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return defaultDeployApprovalSweepInterval
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func resourceRecommendationLookback(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_RESOURCE_RECOMMENDATION_LOOKBACK")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_RESOURCE_RECOMMENDATION_LOOKBACK, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func capacityForecastLookback(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_CAPACITY_FORECAST_LOOKBACK")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_CAPACITY_FORECAST_LOOKBACK, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func doctorDiskWarningBytes(logger *slog.Logger) int64 {
	raw := os.Getenv("APP_DOCTOR_DISK_WARNING_BYTES")
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		logger.Warn("invalid APP_DOCTOR_DISK_WARNING_BYTES, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return n
}

// Returns 0 when unset or invalid so api applies its default.
func doctorMasterKeyRotationWarnAge(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_DOCTOR_MASTER_KEY_ROTATION_WARN_DAYS")
	if raw == "" {
		return 0
	}
	days, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		logger.Warn("invalid APP_DOCTOR_MASTER_KEY_ROTATION_WARN_DAYS, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return time.Duration(days) * 24 * time.Hour
}

// Returns 0 when unset or invalid so api applies its default.
func doctorNetworkTimeout(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_DOCTOR_NETWORK_TIMEOUT")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_DOCTOR_NETWORK_TIMEOUT, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func doctorClockSkewWarnAge(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_DOCTOR_CLOCK_SKEW_WARN")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_DOCTOR_CLOCK_SKEW_WARN, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func doctorDiskIOWarnLatency(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_DOCTOR_DISK_IO_WARN_MS")
	if raw == "" {
		return 0
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		logger.Warn("invalid APP_DOCTOR_DISK_IO_WARN_MS, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

// Returns 0 when unset or invalid so api applies its default.
func doctorMinRAMBytes(logger *slog.Logger) int64 {
	raw := os.Getenv("APP_DOCTOR_MIN_RAM_BYTES")
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		logger.Warn("invalid APP_DOCTOR_MIN_RAM_BYTES, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return n
}

// doctorMinCPUCount reads APP_DOCTOR_MIN_CPU_COUNT, applied to
// api.WithDoctorMinCPUCount. Returns 0 (api's own signal to fall back to
// its internal default) when unset or unparseable.
func doctorMinCPUCount(logger *slog.Logger) int {
	raw := os.Getenv("APP_DOCTOR_MIN_CPU_COUNT")
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		logger.Warn("invalid APP_DOCTOR_MIN_CPU_COUNT, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return n
}

// Resolved here: api.WithAPIRateLimit treats 0 as disabled, not as the default.
func apiRateLimitReadRPM(logger *slog.Logger) int {
	return apiRateLimitRPM(logger, "APP_API_RATE_LIMIT_READ_RPM", defaultAPIRateLimitReadRPM)
}

func apiRateLimitWriteRPM(logger *slog.Logger) int {
	return apiRateLimitRPM(logger, "APP_API_RATE_LIMIT_WRITE_RPM", defaultAPIRateLimitWriteRPM)
}

// Keyed per (client IP, app name) since this unauthenticated route has no actor identity.
func webhookRateLimitRPM(logger *slog.Logger) int {
	return apiRateLimitRPM(logger, "APP_WEBHOOK_RATE_LIMIT_RPM", defaultWebhookRateLimitRPM)
}

func apiRateLimitRPM(logger *slog.Logger, envVar string, fallback int) int {
	raw := os.Getenv(envVar)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		logger.Warn("invalid "+envVar+", using the default", slog.String("value", raw), slog.Int("default", fallback))
		return fallback
	}
	return n
}

// publicHost is the address apps are reachable at: APP_PUBLIC_HOST, else
// the server's detected public IP (see ingressdriver.ResolvePublicHost).
// Resolved once per process; "" when neither is available.
func publicHost() string {
	host, _ := resolvedPublicHost()
	return host
}

var (
	publicHostOnce   sync.Once
	publicHostValue  string
	publicHostOrigin string
)

func resolvedPublicHost() (host, source string) {
	if v := strings.TrimSpace(os.Getenv("APP_PUBLIC_HOST")); v != "" {
		return v, ingressdriver.PublicHostSourceEnv
	}
	publicHostOnce.Do(func() {
		publicHostValue, publicHostOrigin = ingressdriver.ResolvePublicHost(context.Background())
	})
	return publicHostValue, publicHostOrigin
}
