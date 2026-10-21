package alerting

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DefaultRuleResource is the ResourceID of every seeded rule; they are platform-wide, not tied to an app.
const DefaultRuleResource = "platform"

const defaultRulePrefix = "default-"

// DefaultCertExpiryDays are the escalating lead times, in days, for the seeded cert_expiry rules.
var DefaultCertExpiryDays = []int{14, 7, 3, 1}

// DefaultsConfig tunes which default rules are seeded.
type DefaultsConfig struct {
	CertExpiryDays []int
}

// DefaultsConfigFromEnv reads APP_DEFAULT_ALERT_CERT_DAYS (comma-separated days, e.g. "14,7,3,1"); bad or empty input keeps the defaults.
func DefaultsConfigFromEnv(lookup func(string) string) DefaultsConfig {
	cfg := DefaultsConfig{CertExpiryDays: DefaultCertExpiryDays}
	if days := parseDays(lookup("APP_DEFAULT_ALERT_CERT_DAYS")); len(days) > 0 {
		cfg.CertExpiryDays = days
	}
	return cfg
}

func parseDays(raw string) []int {
	var out []int
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			return nil
		}
		out = append(out, n)
	}
	return out
}

// DefaultRuleSpec is one rule the seeder creates once; Key is stable and doubles as the rule ID.
type DefaultRuleSpec struct {
	Key  string
	Rule Rule
}

// DefaultRuleSpecs lists the rules seeded on first boot. Only kinds the engine already evaluates are used.
func DefaultRuleSpecs(cfg DefaultsConfig) []DefaultRuleSpec {
	days := append([]int(nil), cfg.CertExpiryDays...)
	if len(days) == 0 {
		days = DefaultCertExpiryDays
	}
	sort.Sort(sort.Reverse(sort.IntSlice(days)))

	var specs []DefaultRuleSpec
	for i, d := range days {
		sev := SeverityWarning
		switch {
		case i == 0:
			sev = SeverityInfo
		case d <= 3:
			sev = SeverityCritical
		}
		specs = append(specs, DefaultRuleSpec{Key: fmt.Sprintf("%scert-expiry-%dd", defaultRulePrefix, d), Rule: Rule{
			Name: fmt.Sprintf("Certificate expires within %d days", d), Kind: KindCertExpiry,
			ForDuration: time.Duration(d) * 24 * time.Hour, Severity: sev,
		}})
	}

	platform := []struct {
		key, name string
		kind      Kind
		severity  string
	}{
		{"restore-drill-failed", "Restore drill failed", KindRestoreDrillFailed, SeverityCritical},
		{"control-plane-backup-stale", "Control plane backup is stale", KindControlPlaneBackupStale, SeverityWarning},
		{"node-offline", "Node offline", KindNodeOffline, SeverityCritical},
		{"node-disk-space", "Node disk space critical", KindNodeDiskSpace, SeverityCritical},
		{"email-delivery-failing", "Outbound email is failing", KindEmailDeliveryFailing, SeverityWarning},
		{"node-cert-expiring", "Node certificate expiring", KindNodeCertExpiring, SeverityWarning},
		{"export-behind", "Export destination failing", KindExportBehind, SeverityWarning},
		{"log-archive-stale", "Log archive failing", KindLogArchiveStale, SeverityWarning},
		{"backup-failed", "A database backup failed", KindBackupFailed, SeverityCritical},
		{"deploy-failed", "A deploy failed", KindDeployFailed, SeverityWarning},
		{"database-down", "A database is down", KindDatabaseDown, SeverityCritical},
		{"database-backup-stale", "A database backup is stale", KindDatabaseBackupStale, SeverityWarning},
	}
	for _, p := range platform {
		specs = append(specs, DefaultRuleSpec{Key: defaultRulePrefix + p.key, Rule: Rule{Name: p.name, Kind: p.kind, Severity: p.severity}})
	}
	return specs
}

// SeedDefaults creates each missing default rule once. A seed key is recorded after its rule exists, so
// operator edits, disables and deletes are never undone, and a crash between the two steps heals on the next call.
func (db *DB) SeedDefaults(ctx context.Context, cfg DefaultsConfig) error {
	var errs []error
	for _, spec := range DefaultRuleSpecs(cfg) {
		if err := db.seedOne(ctx, spec); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (db *DB) seedOne(ctx context.Context, spec DefaultRuleSpec) error {
	var seeded int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM alert_default_seeds WHERE key = ?`, spec.Key).Scan(&seeded); err != nil {
		return fmt.Errorf("alerting: read default seed %q: %w", spec.Key, err)
	}
	if seeded > 0 {
		return nil
	}
	_, err := db.GetRule(ctx, spec.Key)
	switch {
	case errors.Is(err, ErrRuleNotFound):
		r := spec.Rule
		r.ID, r.ResourceID, r.Enabled = spec.Key, DefaultRuleResource, true
		if err := db.SaveRule(ctx, r); err != nil {
			return fmt.Errorf("alerting: seed default rule %q: %w", spec.Key, err)
		}
	case err != nil:
		return fmt.Errorf("alerting: seed default rule %q: %w", spec.Key, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO alert_default_seeds (key, rule_id, seeded_at) VALUES (?, ?, ?)`,
		spec.Key, spec.Key, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("alerting: record default seed %q: %w", spec.Key, err)
	}
	return nil
}

// ListDefaultRules returns the seeded rules that still exist.
func (db *DB) ListDefaultRules(ctx context.Context) ([]Rule, error) {
	all, err := db.ListRulesForResource(ctx, DefaultRuleResource)
	if err != nil {
		return nil, err
	}
	var out []Rule
	for _, r := range all {
		if strings.HasPrefix(r.ID, defaultRulePrefix) {
			out = append(out, r)
		}
	}
	return out, nil
}

// SetDefaultRuleEnabled flips only the enabled flag of a seeded rule, leaving every other operator edit alone.
func (db *DB) SetDefaultRuleEnabled(ctx context.Context, id string, enabled bool) (*Rule, error) {
	if !strings.HasPrefix(id, defaultRulePrefix) {
		return nil, ErrRuleNotFound
	}
	r, err := db.GetRule(ctx, id)
	if err != nil {
		return nil, err
	}
	if r.ResourceID != DefaultRuleResource {
		return nil, ErrRuleNotFound
	}
	r.Enabled = enabled
	if err := db.SaveRule(ctx, *r); err != nil {
		return nil, err
	}
	return r, nil
}

// ResetDefaultRules rewrites every default rule to its shipped definition, re-creating deleted ones, and returns them.
func (db *DB) ResetDefaultRules(ctx context.Context) ([]Rule, error) {
	var errs []error
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, spec := range DefaultRuleSpecs(DefaultsConfigFromEnv(os.Getenv)) {
		r := spec.Rule
		r.ID, r.ResourceID, r.Enabled = spec.Key, DefaultRuleResource, true
		if existing, err := db.GetRule(ctx, spec.Key); err == nil {
			r.ChannelID, r.NotifyURL, r.NotifyKind = existing.ChannelID, existing.NotifyURL, existing.NotifyKind
		}
		if err := db.SaveRule(ctx, r); err != nil {
			errs = append(errs, fmt.Errorf("alerting: reset default rule %q: %w", spec.Key, err))
			continue
		}
		if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO alert_default_seeds (key, rule_id, seeded_at) VALUES (?, ?, ?)`, spec.Key, spec.Key, now); err != nil {
			errs = append(errs, fmt.Errorf("alerting: record default seed %q: %w", spec.Key, err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return db.ListDefaultRules(ctx)
}
