// Package dokployextras plans and applies the parts of a Dokploy import that are not
// apps: S3 backup destinations, database backup schedules and alert channels. The CLI
// and the dashboard import share it, so both create through the same API calls.
package dokployextras

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

// Import outcomes. No type here has a credential field: keys and notify URLs go from the
// source straight into the create call and are never stored, printed or journaled.
const (
	StateWouldCreate = "would_create"
	StateNeedsSecret = "needs_secret"
	StateCreated     = "created"
	StateExists      = "exists"
	StatePending     = "pending"
	StateSkipped     = "skipped"
	StateReview      = "review"
	StateFailed      = "failed"
	StateUnsupported = "unsupported"

	KindBackupTarget   = "backup_target"
	KindBackupSchedule = "backup_schedule"
	KindChannel        = "alert_channel"

	EnvLimit     = "APP_MIGRATE_BACKUP_LIMIT"
	DefaultLimit = 200
	CeilingLimit = 5000

	EndpointWarning   = "the server connects to this endpoint from its own network: confirm it before enabling schedules"
	fingerprintPrefix = "sha256:"
	detailMaxRunes    = 300
)

var (
	// s3BucketRE is the S3 bucket naming rule; anything else is never put into a create call or a hint.
	s3BucketRE   = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
	invalidChars = regexp.MustCompile(`[^a-z0-9-]+`)
	// NameRE is the resource name rule a journal and an import accept.
	NameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
)

// channelKinds maps source provider types onto channel kinds that exist here.
var channelKinds = map[string]string{
	"slack": "slack", "discord": "discord", "telegram": "telegram", "email": "email", "gotify": "gotify",
	"ntfy": "ntfy", "lark": "lark", "pushover": "pushover", "custom": "generic",
}

// BackupImport is the backup half of a Dokploy plan.
type BackupImport struct {
	Targets   []BackupTarget   `json:"targets"`
	Schedules []BackupSchedule `json:"schedules"`
	// UseKeys and Creds are set only for an apply that includes secret values.
	UseKeys bool                                                               `json:"-"`
	Creds   func(ctx context.Context, sourceID string) (string, string, error) `json:"-"`
}

// BackupTarget is one source destination mapped onto a backup target.
type BackupTarget struct {
	SourceName string `json:"source_name"`
	Name       string `json:"name"`
	Provider   string `json:"provider"`
	Endpoint   string `json:"endpoint,omitempty"`
	Region     string `json:"region,omitempty"`
	Bucket     string `json:"bucket"`
	ID         string `json:"id,omitempty"`
	State      string `json:"state"`
	Detail     string `json:"detail,omitempty"`
	// HasKeys says the source holds keys for it; the keys themselves are never read at plan time.
	HasKeys  bool   `json:"has_keys"`
	SourceID string `json:"-"`
}

// BackupSchedule is one source database backup mapped onto a schedule.
type BackupSchedule struct {
	Database string `json:"database"`
	Target   string `json:"target"`
	Cron     string `json:"cron"`
	Retain   int    `json:"retain,omitempty"`
	// Paused is true unless schedules were explicitly enabled and the source schedule was on.
	Paused bool   `json:"paused"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

// ChannelImport is the alert channel half of a Dokploy plan.
type ChannelImport struct {
	Channels    []Channel            `json:"channels"`
	Unsupported []UnsupportedChannel `json:"unsupported,omitempty"`
	// UseSecrets and Target are set only for an apply that includes secret values.
	UseSecrets bool                                                                            `json:"-"`
	Target     func(ctx context.Context, sourceID string) (platformimport.NotifyTarget, error) `json:"-"`
	CheckURL   func(string) error                                                              `json:"-"`
}

// Channel is one source notification provider mapped onto an alert channel.
type Channel struct {
	SourceName string `json:"source_name"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Server     string `json:"server,omitempty"`
	// Enabled is false unless channels were explicitly enabled.
	Enabled bool   `json:"enabled"`
	ID      string `json:"id,omitempty"`
	State   string `json:"state"`
	Detail  string `json:"detail,omitempty"`
	// HasSecret says the source holds a token or webhook URL; the value is never read at plan time.
	HasSecret  bool   `json:"has_secret"`
	SourceID   string `json:"-"`
	SourceKind string `json:"-"`
	Recipient  string `json:"-"`
	HasAuth    bool   `json:"-"`
}

// UnsupportedChannel is a source provider with no matching channel kind here.
type UnsupportedChannel struct {
	SourceName string `json:"source_name"`
	SourceKind string `json:"source_kind"`
	Reason     string `json:"reason"`
}

// BackupOptions tunes PlanBackups. CheckEndpoint is the import network policy check.
type BackupOptions struct {
	EnableSchedules bool
	Limit           int
	CheckEndpoint   func(string) error
}

// ChannelOptions tunes PlanChannels. CheckURL is the import network policy check.
type ChannelOptions struct {
	Enable   bool
	Limit    int
	CheckURL func(string) error
}

// EnvPositiveInt reads a positive int capped at ceiling, falling back to def.
func EnvPositiveInt(lookupEnv func(string) (string, bool), name string, def, ceiling int) int {
	if lookupEnv == nil {
		return def
	}
	if v, ok := lookupEnv(name); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return min(n, ceiling)
		}
	}
	return def
}

// NewBackupOptions wires the per-run cap from env and the endpoint check from policy.
func NewBackupOptions(ctx context.Context, enable bool, policy platformimport.NetworkPolicy, lookupEnv func(string) (string, bool)) BackupOptions {
	return BackupOptions{EnableSchedules: enable, Limit: EnvPositiveInt(lookupEnv, EnvLimit, DefaultLimit, CeilingLimit),
		CheckEndpoint: func(raw string) error { return platformimport.ValidateDestinationEndpoint(ctx, raw, policy) }}
}

// NewChannelOptions wires the per-run cap from env and the server URL check from policy.
func NewChannelOptions(ctx context.Context, enable bool, policy platformimport.NetworkPolicy, lookupEnv func(string) (string, bool)) ChannelOptions {
	return ChannelOptions{Enable: enable, Limit: EnvPositiveInt(lookupEnv, EnvLimit, DefaultLimit, CeilingLimit),
		CheckURL: func(raw string) error { return platformimport.ValidateNotifyEndpoint(ctx, raw, policy) }}
}

// unsafeRune covers control and format characters (bidi overrides, zero-width marks).
func unsafeRune(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
}

// HasControlChar reports control or format characters, which never go into a hint or a create call.
func HasControlChar(s string) bool {
	return strings.IndexFunc(s, unsafeRune) >= 0
}

// CleanDetail keeps a provider or server message printable and short.
func CleanDetail(s string) string {
	s = strings.Map(func(r rune) rune {
		if unsafeRune(r) {
			return ' '
		}
		return r
	}, s)
	if r := []rune(s); len(r) > detailMaxRunes {
		s = string(r[:detailMaxRunes]) + "..."
	}
	return strings.TrimSpace(s)
}

// ShQuote single-quotes a value so a printed hint command is safe to paste into a shell.
func ShQuote(s string) string {
	s = strings.Map(func(r rune) rune {
		if unsafeRune(r) {
			return ' '
		}
		return r
	}, s)
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ServiceName maps free-text source names onto the lowercase, hyphenated form a resource accepts.
func ServiceName(name string) string {
	s := invalidChars.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if s == "" {
		return "app"
	}
	if s[0] < 'a' || s[0] > 'z' {
		s = "app-" + s
	}
	return s
}

// ImportName maps a source name onto a journal-safe name; ok is false for names a person must pick.
func ImportName(raw string) (string, bool) {
	if strings.Trim(invalidChars.ReplaceAllString(strings.ToLower(raw), ""), "-") == "" {
		return "", false
	}
	name := ServiceName(raw)
	return name, NameRE.MatchString(name)
}

// TargetFingerprint identifies a backup target by its non-secret shape.
func TargetFingerprint(provider, endpoint, bucket, region string) string {
	sum := sha256.Sum256([]byte(provider + "\n" + endpoint + "\n" + bucket + "\n" + region))
	return fingerprintPrefix + hex.EncodeToString(sum[:])
}

// MapProvider picks the target provider from the endpoint host, since source provider labels vary.
func MapProvider(endpoint string) (provider, keptEndpoint string) {
	host := ""
	if u, err := url.Parse(endpoint); err == nil {
		host = strings.ToLower(u.Hostname())
	}
	switch {
	case host == "":
		return "aws", ""
	case strings.HasSuffix(host, ".r2.cloudflarestorage.com"):
		return "r2", endpoint
	case host == "s3.amazonaws.com" || strings.HasSuffix(host, ".amazonaws.com"):
		return "aws", ""
	default:
		return "custom", endpoint
	}
}

func planTarget(d platformimport.BackupDestination, o BackupOptions, budget int, used map[string]bool) BackupTarget {
	t := BackupTarget{SourceName: d.Name, Region: d.Region, Bucket: d.Bucket, SourceID: d.SourceID, HasKeys: d.HasCredentials, State: StateNeedsSecret}
	t.Provider, t.Endpoint = MapProvider(d.Endpoint)
	name, nameOK := ImportName(d.Name)
	t.Name = name
	var endpointErr error
	if d.Endpoint != "" && o.CheckEndpoint != nil {
		endpointErr = o.CheckEndpoint(d.Endpoint)
	}
	switch {
	case budget <= 0:
		t.State, t.Detail = StateSkipped, "over the per-run backup import limit ("+EnvLimit+")"
	case !nameOK:
		t.State, t.Detail = StateReview, "the source name is blank, symbols only or over 63 characters; create this target by hand"
	case used[t.Name]:
		t.State, t.Detail = StateSkipped, "another source destination maps to the same name; create this one by hand"
	case !s3BucketRE.MatchString(t.Bucket):
		t.State, t.Detail = StateReview, "the bucket name is not a valid S3 bucket name"
	case HasControlChar(t.Region) || HasControlChar(d.Endpoint):
		t.State, t.Detail = StateReview, "the region or endpoint holds control characters"
	case endpointErr != nil:
		t.State, t.Detail = StateReview, "endpoint refused by the import network policy: "+CleanDetail(endpointErr.Error())
	case t.Provider == "custom" && t.Region == "":
		t.State, t.Detail = StateReview, "a custom endpoint needs a region; create it with backup-targets create --region"
	}
	if t.State == StateReview {
		t.SourceName = CleanDetail(t.SourceName)
		t.Bucket, t.Region, t.Endpoint = CleanDetail(t.Bucket), CleanDetail(t.Region), CleanDetail(t.Endpoint)
	}
	return t
}

// PlanBackups is pure apart from the endpoint check: names, endpoints and buckets only.
func PlanBackups(disc *platformimport.Discovery, o BackupOptions) *BackupImport {
	if len(disc.Destinations) == 0 && !hasBackups(disc) {
		return nil
	}
	imp := &BackupImport{}
	used := map[string]bool{}
	budget := o.Limit
	bySource := map[string]int{}
	for _, d := range disc.Destinations {
		t := planTarget(d, o, budget, used)
		if t.Name != "" {
			used[t.Name] = true
		}
		budget--
		bySource[d.SourceID] = len(imp.Targets)
		imp.Targets = append(imp.Targets, t)
	}
	scheduled := map[string]bool{}
	for _, db := range disc.Databases {
		name, nameOK := ImportName(db.Name)
		for _, b := range db.Backups {
			s := BackupSchedule{Database: name, Cron: b.Schedule, Retain: b.KeepLatest, Paused: !o.EnableSchedules || !b.Enabled, State: StateWouldCreate}
			idx, ok := bySource[b.DestinationID]
			if ok {
				s.Target = imp.Targets[idx].Name
			}
			_, cronErr := cronexpr.Parse(b.Schedule)
			switch {
			case budget <= 0:
				s.State, s.Detail = StateSkipped, "over the per-run backup import limit ("+EnvLimit+")"
			case !nameOK:
				s.State, s.Detail = StateReview, "the database name is blank, symbols only or over 63 characters"
			case !ok:
				s.State, s.Detail = StateReview, "its destination was not found on the source"
			case cronErr != nil:
				s.State, s.Detail = StateReview, "cron expression rejected: "+CleanDetail(cronErr.Error())
			case scheduled[name]:
				s.State, s.Detail = StateSkipped, "a database keeps one schedule here; the first source schedule was kept"
			case imp.Targets[idx].State == StateSkipped || imp.Targets[idx].State == StateReview:
				s.State, s.Detail = StatePending, "its backup target is not imported"
			}
			if s.State == StateWouldCreate {
				scheduled[name] = true
			} else {
				s.Cron = CleanDetail(s.Cron)
			}
			budget--
			imp.Schedules = append(imp.Schedules, s)
		}
	}
	return imp
}

func hasBackups(disc *platformimport.Discovery) bool {
	for _, db := range disc.Databases {
		if len(db.Backups) > 0 {
			return true
		}
	}
	return false
}

// TargetHint is the manual create command; it carries placeholders, never a key.
func TargetHint(t *BackupTarget) string {
	cmd := "keys not imported; run: backup-targets create --name " + ShQuote(t.Name) + " --provider " + ShQuote(t.Provider) + " --bucket " + ShQuote(t.Bucket)
	if t.Endpoint != "" {
		cmd += " --endpoint " + ShQuote(t.Endpoint)
	}
	if t.Region != "" {
		cmd += " --region " + ShQuote(t.Region)
	}
	return cmd + " --access-key-id <id> --secret-access-key <key>"
}

// ScheduleHint is the manual schedule command for a schedule that could not be bound.
func ScheduleHint(s *BackupSchedule, targetID string) string {
	cmd := "backups schedule set " + ShQuote(s.Database) + " --target " + ShQuote(targetID) + " --cron " + ShQuote(s.Cron)
	if s.Retain > 0 {
		cmd += fmt.Sprintf(" --retain %d", s.Retain)
	}
	return cmd
}

// FinishPlan marks which targets an apply would create with keys and sets the manual
// command for the rest.
func (imp *BackupImport) FinishPlan(useKeys bool) {
	if imp == nil {
		return
	}
	imp.UseKeys = useKeys
	for i := range imp.Targets {
		t := &imp.Targets[i]
		if t.State != StateNeedsSecret {
			continue
		}
		if useKeys && t.HasKeys {
			t.State = StateWouldCreate
			continue
		}
		t.Detail = TargetHint(t)
	}
}

// serverProblem checks a self-hosted provider base URL, which is shown in hints and plans.
func serverProblem(raw string, check func(string) error) string {
	u, err := url.Parse(raw)
	switch {
	case raw == "" || err != nil:
		return "the provider has no usable server URL"
	case HasControlChar(raw):
		return "the server URL holds control characters"
	case u.User != nil || u.RawQuery != "" || u.Fragment != "":
		return "the server URL carries credentials or a query; create this channel by hand"
	}
	if check != nil {
		if err := check(raw); err != nil {
			return "server URL refused by the import network policy: " + CleanDetail(err.Error())
		}
	}
	return ""
}

func planChannel(p platformimport.NotificationProvider, o ChannelOptions, budget int, used map[string]bool) Channel {
	c := Channel{SourceName: CleanDetail(p.Name), Kind: channelKinds[p.Kind], Enabled: o.Enable, SourceID: p.SourceID, SourceKind: p.Kind,
		Recipient: p.Recipient, HasSecret: p.HasSecret, HasAuth: p.HasAccessToken, State: StateNeedsSecret}
	if c.Kind == "email" {
		c.State = StateWouldCreate
	}
	name, nameOK := ImportName(p.Name)
	c.Name = name
	server := ""
	if p.Kind == "gotify" || p.Kind == "ntfy" {
		server = serverProblem(p.Server, o.CheckURL)
	}
	switch {
	case budget <= 0:
		c.State, c.Detail = StateSkipped, "over the per-run import limit ("+EnvLimit+")"
	case !nameOK:
		c.State, c.Detail = StateReview, "the source name is blank, symbols only or over 63 characters; create this channel by hand"
	case used[c.Name]:
		c.State, c.Detail = StateSkipped, "another source provider maps to the same name; create this one by hand"
	case p.Mismatch:
		c.State, c.Detail = StateReview, "the provider settings do not match its kind; create this channel by hand"
	case p.HasHeaders:
		c.State, c.Detail = StateReview, "the custom webhook sends extra headers, which a generic channel does not carry; create it by hand"
	case c.Kind == "email" && (p.Recipient == "" || HasControlChar(p.Recipient) || strings.ContainsAny(p.Recipient, " ,;")):
		c.State, c.Detail = StateReview, "the email provider has no usable recipient address"
	case server != "":
		c.State, c.Detail = StateReview, server
	default:
		if p.Kind == "gotify" || p.Kind == "ntfy" {
			c.Server = p.Server
		}
		if p.ExtraRecipients > 0 {
			c.Detail = fmt.Sprintf("only the first recipient is imported; add a channel for each of the other %d", p.ExtraRecipients)
		}
	}
	return c
}

// PlanChannels is pure apart from the server URL check: names, kinds and hosts only.
func PlanChannels(disc *platformimport.Discovery, o ChannelOptions) *ChannelImport {
	if len(disc.Notifications) == 0 {
		return nil
	}
	imp := &ChannelImport{CheckURL: o.CheckURL}
	used := map[string]bool{}
	budget := o.Limit
	for _, p := range disc.Notifications {
		if _, ok := channelKinds[p.Kind]; !ok {
			imp.Unsupported = append(imp.Unsupported, UnsupportedChannel{SourceName: CleanDetail(p.Name), SourceKind: CleanDetail(p.Kind),
				Reason: "no matching channel kind here; create a generic webhook channel or another kind by hand"})
			continue
		}
		c := planChannel(p, o, budget, used)
		if c.Name != "" {
			used[c.Name] = true
		}
		budget--
		imp.Channels = append(imp.Channels, c)
	}
	return imp
}

// ChannelHint is the manual create command. It carries placeholders only, never a token or address.
func ChannelHint(c *Channel) string {
	cmd := "token not imported; run: channels create --name " + ShQuote(c.Name) + " --kind " + ShQuote(c.Kind)
	switch c.Kind {
	case "pushover":
		cmd += " --pushover-user-key '<user-key>' --pushover-api-token '<api-token>'"
	case "telegram":
		cmd += " --notify-url 'https://api.telegram.org/bot<bot-token>/sendMessage?chat_id=<chat-id>'"
	case "gotify":
		cmd += " --notify-url " + ShQuote(c.Server+"/message?token=<app-token>")
	case "ntfy":
		u := c.Server + "/<topic>"
		if c.HasAuth {
			u += "?auth=<access-token>"
		}
		cmd += " --notify-url " + ShQuote(u)
	case "email":
		cmd += " --notify-url '<address>'"
	default:
		cmd += " --notify-url '<" + c.Kind + "-webhook-url>'"
	}
	if !c.Enabled {
		cmd += " --disabled"
	}
	return cmd
}

// FinishPlan marks which channels an apply would create and sets the manual command for the rest.
func (imp *ChannelImport) FinishPlan(useSecrets bool) {
	if imp == nil {
		return
	}
	imp.UseSecrets = useSecrets
	for i := range imp.Channels {
		c := &imp.Channels[i]
		if c.State != StateNeedsSecret {
			continue
		}
		if useSecrets && c.HasSecret {
			c.State = StateWouldCreate
			continue
		}
		c.Detail = ChannelHint(c)
	}
}
