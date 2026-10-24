package datamigrate

import (
	"fmt"
	"strings"
)

// providerOnlyExtensions exist only on one hosted provider. They are left out
// of the restore with a warning instead of blocking the move.
var providerOnlyExtensions = toSet("neon", "neon_utils", "pg_session_jwt")

// HostedPlan is the caller's request to move one hosted database.
type HostedPlan struct {
	// URL is the source connection URL. It prints and marshals redacted.
	URL SecretURL `json:"url"`
	// TargetName is the new managed database name; TargetVersion defaults to
	// the source's major (with the pgvector variant when needed).
	TargetName    string `json:"target_name"`
	TargetVersion string `json:"target_version,omitempty"`
	TargetExists  bool   `json:"target_exists,omitempty"`
	OwnTarget     bool   `json:"own_target,omitempty"`
	// WritesFrozen is the caller's acknowledgement that the old app is in
	// maintenance mode (Heroku: `heroku maintenance:on -a <app>`) and nothing
	// writes to the source. A dump taken while it changes is not a copy.
	WritesFrozen bool `json:"writes_frozen"`
	// AllowLarge overrides the size ceiling after reading the replication advice.
	AllowLarge bool `json:"allow_large,omitempty"`
}

// HostedPreflightInput is the gathered facts for HostedPreflight.
type HostedPreflightInput struct {
	Plan        HostedPlan
	Source      HostedSource
	Probe       HostedProbe
	Config      HostedConfig
	DiskMargin  float64
	MBPerSecond float64
}

// HostedPreflight runs the standard Postgres preflight on the probe, then adds
// the hosted checks: quiesced writes, size ceiling, TLS, roles. Nothing has
// been dumped or written when it runs.
func HostedPreflight(in HostedPreflightInput) PreflightResult {
	version := in.Plan.TargetVersion
	if version == "" && in.Probe.Major > 0 {
		version = fmt.Sprint(in.Probe.Major)
		if s, _ := requiredVariant(in.Probe.Extensions); s != "" {
			version = variantVersion(in.Probe.Major, s)
		}
	}
	var carried, provider []string
	for _, e := range in.Probe.Extensions {
		if providerOnlyExtensions[strings.ToLower(e)] {
			provider = append(provider, e)
			continue
		}
		carried = append(carried, e)
	}
	res := Preflight(PreflightInput{
		Engine: EnginePostgres,
		DB: DatabaseInfo{Name: in.Source.Source.Database, SizeBytes: in.Probe.SizeBytes, Tables: in.Probe.Tables,
			Extensions: carried},
		SourceMajor: in.Probe.Major, TargetName: in.Plan.TargetName, TargetVersion: version,
		TargetExists: in.Plan.TargetExists, OwnTarget: in.Plan.OwnTarget,
		FreeBytes: in.Probe.FreeBytes, DiskMargin: in.DiskMargin, MBPerSecond: in.MBPerSecond,
	})
	add := func(id, sev, msg, next string) {
		res.Checks = append(res.Checks, PreflightCheck{ID: id, Severity: sev, Message: msg, NextAction: next})
		if sev == SeverityBlock {
			res.Blocked = true
		}
	}

	if in.Plan.WritesFrozen {
		add("writes_frozen", SeverityOK, "Writes to the source are acknowledged as stopped.", "")
	} else {
		add("writes_frozen", SeverityBlock, "The source is still taking writes, so a dump would miss them.",
			"Put the old app in maintenance mode (Heroku: `heroku maintenance:on -a <app>`), stop workers and schedulers, then confirm writes are frozen.")
	}

	switch {
	case in.Config.MaxBytes > 0 && in.Probe.SizeBytes > in.Config.MaxBytes && in.Plan.AllowLarge:
		add("size_limit", SeverityWarn, fmt.Sprintf("The database is %s, above the %s dump and restore ceiling. The override was set.", HumanBytes(in.Probe.SizeBytes), HumanBytes(in.Config.MaxBytes)),
			"Expect long downtime while the source stays frozen.")
	case in.Config.MaxBytes > 0 && in.Probe.SizeBytes > in.Config.MaxBytes:
		add("size_limit", SeverityBlock, fmt.Sprintf("The database is %s, above the %s ceiling for a dump and restore, which keeps the source frozen the whole time.", HumanBytes(in.Probe.SizeBytes), HumanBytes(in.Config.MaxBytes)),
			"Prefer continuous replication from the provider to cut downtime, or set the large database override to accept the downtime.")
	default:
		add("size_limit", SeverityOK, fmt.Sprintf("%s is within the dump and restore ceiling.", HumanBytes(in.Probe.SizeBytes)), "")
	}

	switch {
	case in.Source.Source.TLS:
		add("tls", SeverityOK, "The connection to the source is encrypted ("+in.Source.Source.SSLMode+").", "")
	case in.Source.Private:
		add("tls", SeverityWarn, "The source is on a private address and the connection may be unencrypted.", "Use sslmode=require if the server supports it.")
	default:
		add("tls", SeverityWarn, "TLS was disabled by operator policy for a public host.", "Use sslmode=require.")
	}

	if len(provider) > 0 {
		add("provider_extensions", SeverityWarn, "Provider-only extension(s) will be skipped: "+strings.Join(provider, ", ")+".",
			"The data is copied; features that depend on these extensions will not work on the target.")
	}
	if len(in.Probe.PolicyRoles) > 0 {
		add("policy_roles", SeverityBlock, "Row level security policies name role(s) that do not exist on the target: "+strings.Join(in.Probe.PolicyRoles, ", ")+".",
			"Create these roles on the target database first, or drop the policies on the source. Hosted providers do not export their roles.")
	}
	add("ownership", SeverityOK, "Owners and grants are not copied (hosted providers forbid superuser); every object is owned by the managed database user.", "")
	return res
}
