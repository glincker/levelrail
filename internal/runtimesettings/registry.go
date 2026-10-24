// Package runtimesettings holds the registry of operator-tunable settings and
// resolves each one as saved value, then env var, then registry default.
package runtimesettings

import (
	"fmt"
	"path/filepath"
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
	KeyRevokedTokenDays        = "retention.revoked_token_days" //nolint:gosec // setting key, not a credential
	KeyExpiredTokenDays        = "retention.expired_token_days" //nolint:gosec // setting key, not a credential
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
		Key: KeyTrustLoopbackForwarded, Category: CategoryNetworking, Kind: KindBool, Default: "true",
		Env: "APP_TRUST_LOOPBACK_FORWARDED", Apply: ApplyRestart,
		TitleKey: "runtimeSettings:items.trustLoopbackForwarded.title", DescKey: "runtimeSettings:items.trustLoopbackForwarded.description",
		Title:       "Trust forwarded headers from loopback",
		Description: "Believe X-Forwarded-For when the request arrives from this host. Turn off when nothing local proxies to the control plane.",
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
