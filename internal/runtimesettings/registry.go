// Package runtimesettings holds the registry of operator-tunable settings and
// resolves each one as saved value, then env var, then registry default.
package runtimesettings

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Kind is the value type of a setting.
type Kind string

// Setting kinds.
const (
	KindBool       Kind = "bool"
	KindDuration   Kind = "duration"
	KindStringList Kind = "string_list"
	KindInt        Kind = "int"
	KindString     Kind = "string"
)

// Category groups settings in the dashboard.
type Category string

// Setting categories.
const (
	CategorySecurity      Category = "security"
	CategoryNetworking    Category = "networking"
	CategoryDeploys       Category = "deploys"
	CategoryObservability Category = "observability"
	CategoryExperimental  Category = "experimental"
	CategoryRetention     Category = "retention"
)

// Apply says when a changed value takes effect.
type Apply string

// Apply modes.
const (
	ApplyLive    Apply = "live"
	ApplyRestart Apply = "restart"
)

// Risk marks settings where a weaker value (off, wider, shorter) needs confirmation.
type Risk string

// Risk levels.
const (
	RiskNormal          Risk = "normal"
	RiskWeakensSecurity Risk = "weakens_security"
)

// Source says where an effective value came from.
const (
	SourceSaved   = "saved"
	SourceEnv     = "env"
	SourceDefault = "default"
)

// Registered setting keys.
const (
	KeyWebhookReplayProtection = "security.webhook_replay_protection"
	KeyWebhookReplayWindow     = "security.webhook_replay_window"
	KeyAuditSensitiveReads     = "security.audit_sensitive_reads"
	KeyBindBlockCredentialDirs = "security.bind_block_credential_dirs"
	KeyBindAllowedRoots        = "security.bind_allowed_roots"
	KeyTrustLoopbackForwarded  = "security.trust_loopback_forwarded_headers"
	KeyIngressDNS01AllHosts    = "ingress.dns01_all_hosts"
	KeyIngressCARetryBase      = "ingress.ca_switch_retry_base"
	KeyIngressCARetryMax       = "ingress.ca_switch_retry_max"
	KeyHeartbeatJobAttempts    = "heartbeat.job_max_attempts"
	KeyHeartbeatJobBackoffBase = "heartbeat.job_backoff_base"
	KeyHeartbeatJobBackoffMax  = "heartbeat.job_backoff_max"
	KeyHeartbeatJobQueue       = "heartbeat.job_queue"
	KeyContainersRestartPolicy = "containers.restart_policy"
	KeyGameStatusTimeout       = "games.status_timeout"
	KeyGameStatusCache         = "games.status_cache"
	KeyRevokedTokenDays        = "retention.revoked_token_days" //nolint:gosec // setting key, not a credential
	KeyExpiredTokenDays        = "retention.expired_token_days" //nolint:gosec // setting key, not a credential
	KeyDesktopSessionTTL       = "desktop.session_ttl"
	KeyDomainsDNSWaitBudget    = "domains.dns_wait_budget"
	KeyDatabasesPoolerDefault  = "databases.pooler_default"
)

// Definition describes one setting. Default and values are in string form
// (bool "true", duration "10m", int "5", string_list as colon separated).
type Definition struct {
	Key        string
	Category   Category
	Kind       Kind
	Default    string
	Env        string
	Apply      Apply
	Risk       Risk
	Onboarding bool
	// Sensitive masks the value in API responses and audit rows; required for credential-like settings.
	Sensitive   bool
	TitleKey    string
	DescKey     string
	Title       string
	Description string
	// AllowedInts limits an int setting to these values; empty accepts any integer.
	AllowedInts []int
	// CheckItem validates one string_list entry; nil accepts any non-empty entry.
	CheckItem func(item string) error
	// AllowedStrings limits a string setting to these values.
	AllowedStrings []string
	// CheckValue vets a canonical value on API writes only (floors and minimums); env and saved rows are not rechecked.
	CheckValue func(canonical string) error
	// EnvConvert rewrites the env value into this setting's unit before validation (for example a Go duration into days).
	EnvConvert func(raw string) (string, error)
	// EnvLoose lets an int setting take any non-negative env value, not only the presets, so existing deployments keep working.
	EnvLoose bool
	// FailClosed makes StringChecked fail instead of falling back to the default when a saved or env value is invalid.
	FailClosed bool
	// HigherIsWeaker marks an int limit where raising the value weakens a protection, so a raise asks for confirmation.
	HigherIsWeaker bool
	// StrengthOrder lists a string setting's allowed values from weakest to strongest, so WeakensTo can tell a downgrade.
	StrengthOrder []string
	// RemovalIsWeaker marks a string_list of protections, where dropping an entry weakens and adding one does not.
	RemovalIsWeaker bool
}

var (
	regMu sync.RWMutex
	reg   []Definition
)

// Register adds a setting to the registry. It panics on a malformed or
// duplicate definition so a bad registration fails at process start.
func Register(d Definition) {
	if d.Risk == "" {
		d.Risk = RiskNormal
	}
	if d.Apply == "" {
		d.Apply = ApplyLive
	}
	if strings.TrimSpace(d.Key) == "" {
		panic("runtimesettings: empty key")
	}
	if _, err := canonicalFromString(d, d.Default); err != nil {
		panic(fmt.Sprintf("runtimesettings: default of %s: %v", d.Key, err))
	}
	regMu.Lock()
	defer regMu.Unlock()
	for _, e := range reg {
		if e.Key == d.Key {
			panic("runtimesettings: duplicate key " + d.Key)
		}
	}
	reg = append(reg, d)
}

func registryLen() int {
	regMu.RLock()
	defer regMu.RUnlock()
	return len(reg)
}

// Definitions returns a copy of the registry in registration order.
func Definitions() []Definition {
	regMu.RLock()
	defer regMu.RUnlock()
	return append([]Definition(nil), reg...)
}

func absRootCheck(item string) error {
	if !filepath.IsAbs(item) || filepath.Clean(item) == "/" {
		return fmt.Errorf("%q must be an absolute directory other than /", item)
	}
	return nil
}

func init() {
	Register(Definition{
		Key: KeyWebhookReplayProtection, Category: CategorySecurity, Kind: KindBool, Default: "true",
		Env: "APP_WEBHOOK_REPLAY_PROTECTION", Apply: ApplyLive, Risk: RiskWeakensSecurity, Onboarding: true,
		TitleKey: "runtimeSettings:items.webhookReplayProtection.title", DescKey: "runtimeSettings:items.webhookReplayProtection.description",
		Title:       "Webhook replay protection",
		Description: "Ignore a signed git webhook body that was already delivered within the replay window.",
	})
	Register(Definition{
		Key: KeyWebhookReplayWindow, Category: CategorySecurity, Kind: KindDuration, Default: "10m",
		Env: "APP_WEBHOOK_REPLAY_WINDOW", Apply: ApplyLive, Risk: RiskWeakensSecurity,
		TitleKey: "runtimeSettings:items.webhookReplayWindow.title", DescKey: "runtimeSettings:items.webhookReplayWindow.description",
		Title:       "Webhook replay window",
		Description: "How long an identical signed webhook body is remembered.",
	})
	Register(Definition{
		Key: KeyAuditSensitiveReads, Category: CategorySecurity, Kind: KindBool, Default: "true",
		Env: "APP_AUDIT_SENSITIVE_READS", Apply: ApplyLive, Risk: RiskWeakensSecurity, Onboarding: true,
		TitleKey: "runtimeSettings:items.auditSensitiveReads.title", DescKey: "runtimeSettings:items.auditSensitiveReads.description",
		Title:       "Audit sensitive reads",
		Description: "Record an audit entry when logs, env exports and other sensitive data are read.",
	})
	Register(Definition{
		Key: KeyBindBlockCredentialDirs, Category: CategorySecurity, Kind: KindBool, Default: "true",
		Env: "APP_BIND_BLOCK_CREDENTIAL_DIRS", Apply: ApplyLive, Risk: RiskWeakensSecurity, Onboarding: true,
		TitleKey: "runtimeSettings:items.bindBlockCredentialDirs.title", DescKey: "runtimeSettings:items.bindBlockCredentialDirs.description",
		Title:       "Block credential directories in bind mounts",
		Description: "Refuse bind mounts of .ssh, .aws, .kube and similar credential directories.",
	})
	Register(Definition{
		Key: KeyBindAllowedRoots, Category: CategorySecurity, Kind: KindStringList, Default: "",
		Env: "APP_BIND_ALLOWED_ROOTS", Apply: ApplyLive, Risk: RiskWeakensSecurity, Onboarding: true, CheckItem: absRootCheck,
		TitleKey: "runtimeSettings:items.bindAllowedRoots.title", DescKey: "runtimeSettings:items.bindAllowedRoots.description",
		Title:       "Allowed bind-mount roots",
		Description: "When not empty, bind mounts must sit inside one of these directories.",
	})
	Register(Definition{
		Key: KeyContainersRestartPolicy, Category: CategoryDeploys, Kind: KindString, Default: "unless-stopped",
		Env: "APP_CONTAINERS_RESTART_POLICY", Apply: ApplyLive, AllowedStrings: []string{"unless-stopped", "on-failure", "no"},
		TitleKey: "runtimeSettings:items.containersRestartPolicy.title", DescKey: "runtimeSettings:items.containersRestartPolicy.description",
		Title:       "Container restart policy",
		Description: "Docker restart policy for long-running app containers, so apps come back after a host reboot or Docker restart. On a small host (1 GB) many apps starting at boot at once compete for memory; use no to rely on the control plane's sequential start. Applies to containers created on the next deploy or recreate; existing containers keep their old policy.",
	})
	Register(Definition{
		Key: KeyDatabasesPoolerDefault, Category: CategoryDeploys, Kind: KindString, Default: "off",
		Env: "APP_DATABASES_POOLER_DEFAULT", Apply: ApplyLive, AllowedStrings: []string{"off", "on"},
		Title:       "Pooler for new Postgres databases",
		Description: "When on, new Postgres databases get the connection pooler enabled at creation. Existing databases never change. Apps that rely on session features through the pooled url (prepared statements, SET, temp tables) should use direct_url.",
	})
	Register(Definition{
		Key: KeyGameStatusTimeout, Category: CategoryNetworking, Kind: KindDuration, Default: "3s",
		Env: "APP_GAME_STATUS_TIMEOUT", Apply: ApplyLive,
		Title:       "Game status ping timeout",
		Description: "How long the game server status ping waits for a game port to answer before reporting it offline. Values under 200ms are raised to 200ms.",
	})
	Register(Definition{
		Key: KeyGameStatusCache, Category: CategoryNetworking, Kind: KindDuration, Default: "5s",
		Env: "APP_GAME_STATUS_CACHE", Apply: ApplyLive,
		Title:       "Game status cache time",
		Description: "How long one game server status answer is reused, so many dashboards and CLI calls ping a game port at most once in this time. 0s turns the cache off.",
	})
	Register(Definition{
		Key: KeyTrustLoopbackForwarded, Category: CategoryNetworking, Kind: KindBool, Default: "true",
		Env: "APP_TRUST_LOOPBACK_FORWARDED", Apply: ApplyRestart,
		TitleKey: "runtimeSettings:items.trustLoopbackForwarded.title", DescKey: "runtimeSettings:items.trustLoopbackForwarded.description",
		Title:       "Trust forwarded headers from loopback",
		Description: "Believe X-Forwarded-For when the request arrives from this host. Turn off when nothing local proxies to the control plane.",
	})
	Register(Definition{
		Key: KeyIngressDNS01AllHosts, Category: CategoryNetworking, Kind: KindBool, Default: "false",
		Env: "APP_INGRESS_DNS01_ALL_HOSTS", Apply: ApplyLive,
		Title:       "DNS-01 for every host",
		Description: "Issue certificates for single hosts over DNS-01 as well as wildcards, so servers with port 80 closed can still get one. Needs a connected DNS provider; not yet verified against a real CA.",
	})
	Register(Definition{
		Key: KeyIngressCARetryBase, Category: CategoryNetworking, Kind: KindDuration, Default: "1m",
		Env: "APP_INGRESS_CA_RETRY_BACKOFF", Apply: ApplyLive,
		Title:       "CA switch retry delay",
		Description: "First wait before retrying a certificate authority switch that failed to load; it doubles on each failure up to the maximum.",
	})
	Register(Definition{
		Key: KeyIngressCARetryMax, Category: CategoryNetworking, Kind: KindDuration, Default: "1h",
		Env: "APP_INGRESS_CA_RETRY_BACKOFF_MAX", Apply: ApplyLive,
		Title:       "CA switch maximum retry delay",
		Description: "Upper bound on the wait between retries of a failed certificate authority switch.",
	})
	registerHeartbeatJobSettings()
	Register(Definition{
		Key: KeyDomainsDNSWaitBudget, Category: CategoryNetworking, Kind: KindDuration, Default: "10m",
		Env: "APP_DOMAINS_DNS_WAIT_BUDGET", Apply: ApplyLive,
		Title:       "DNS wait budget",
		Description: "How long a domain may wait for DNS before the Domains page stops saying it is fine and lists concrete checks. It only changes the wording; it never blocks onboarding.",
	})
	Register(Definition{
		Key: KeyDesktopSessionTTL, Category: CategorySecurity, Kind: KindDuration, Default: "12h",
		Env: "APP_DESKTOP_SESSION_TTL", Apply: ApplyLive,
		Title:       "Desktop app session lifetime",
		Description: "How long a dashboard session issued to the desktop app stays valid before it must exchange its token again.",
	})
	Register(Definition{
		Key: KeyRevokedTokenDays, Category: CategoryRetention, Kind: KindInt, Default: "90", AllowedInts: TokenRetentionPresets,
		Env: "APP_RETENTION_REVOKED_TOKEN_DAYS", Apply: ApplyLive,
		TitleKey: "runtimeSettings:items.revokedTokenDays.title", DescKey: "runtimeSettings:items.revokedTokenDays.description",
		Title:       "Delete revoked API tokens after",
		Description: "Days a revoked API token stays listed before the cleanup removes it. 0 keeps them forever.",
	})
	Register(Definition{
		Key: KeyExpiredTokenDays, Category: CategoryRetention, Kind: KindInt, Default: "90", AllowedInts: TokenRetentionPresets,
		Env: "APP_RETENTION_EXPIRED_TOKEN_DAYS", Apply: ApplyLive,
		TitleKey: "runtimeSettings:items.expiredTokenDays.title", DescKey: "runtimeSettings:items.expiredTokenDays.description",
		Title:       "Delete expired API tokens after",
		Description: "Days an expired API token stays listed before the cleanup removes it. 0 keeps them forever.",
	})
}

// TokenRetentionPresets are the day counts an operator may pick; 0 keeps tokens forever.
var TokenRetentionPresets = []int{0, 30, 60, 90, 365}

// ParseDuration parses a positive duration.
func ParseDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("%q is not a duration such as 10m", s)
	}
	if d <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	return d, nil
}

// JobQueueMax bounds the job ping queue so its backing channel has a fixed size while the limit stays live.
const JobQueueMax = 10000

func intRangeCheck(floor, ceil int) func(string) error {
	return func(canonical string) error {
		n, err := strconv.Atoi(canonical)
		if err != nil || n < floor || n > ceil {
			return fmt.Errorf("value must be between %d and %d", floor, ceil)
		}
		return nil
	}
}

func registerHeartbeatJobSettings() {
	Register(Definition{
		Key: KeyHeartbeatJobAttempts, Category: CategoryObservability, Kind: KindInt, Default: "3",
		Env: "APP_HEARTBEAT_JOB_MAX_ATTEMPTS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 20),
		Title:       "Job ping attempts",
		Description: "How many times a per-job heartbeat ping is tried before it is given up.",
	})
	Register(Definition{
		Key: KeyHeartbeatJobBackoffBase, Category: CategoryObservability, Kind: KindDuration, Default: "2s",
		Env: "APP_HEARTBEAT_JOB_BACKOFF_BASE", Apply: ApplyLive,
		Title:       "Job ping retry delay",
		Description: "First wait before retrying a failed job ping; it doubles on each retry up to the maximum.",
	})
	Register(Definition{
		Key: KeyHeartbeatJobBackoffMax, Category: CategoryObservability, Kind: KindDuration, Default: "30s",
		Env: "APP_HEARTBEAT_JOB_BACKOFF_MAX", Apply: ApplyLive,
		Title:       "Job ping maximum retry delay",
		Description: "Upper bound on the wait between job ping retries.",
	})
	Register(Definition{
		Key: KeyHeartbeatJobQueue, Category: CategoryObservability, Kind: KindInt, Default: "64",
		Env: "APP_HEARTBEAT_JOB_QUEUE", Apply: ApplyLive, CheckValue: intRangeCheck(1, JobQueueMax),
		Title:       "Job ping queue size",
		Description: "How many job pings may wait to be sent before new ones are dropped. A change applies at once.",
	})
}
