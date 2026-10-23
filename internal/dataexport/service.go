package dataexport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Store is the persistence surface the service needs; *store.DB satisfies it.
type Store interface {
	SaveExportDestination(ctx context.Context, d store.ExportDestination) error
	UpdateExportDestination(ctx context.Context, d store.ExportDestination) error
	GetExportDestination(ctx context.Context, id string) (store.ExportDestination, error)
	ListExportDestinations(ctx context.Context) ([]store.ExportDestination, error)
	DeleteExportDestination(ctx context.Context, id string) error
	SaveExportStatus(ctx context.Context, s store.ExportStatus) error
	GetExportStatus(ctx context.Context, id string) (store.ExportStatus, error)
	ListAuditEntriesAfter(ctx context.Context, afterAt, afterID string, to *time.Time, limit int) ([]store.AuditEntry, error)
}

// Secrets stores destination credentials in envelope encryption.
type Secrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// Deps wires a Service. Logs, Metrics, AppCount and Objects are optional:
// a destination kind that needs a missing one reports a clear error.
type Deps struct {
	Store    Store
	Secrets  Secrets
	Logs     *telemetry.LogBroadcaster
	Metrics  MetricSource
	AppCount func(ctx context.Context) (int, error)
	Platform PlatformSource
	Objects  ObjectPutter
	Config   Config
	// SpoolDir, when set, keeps each log destination's backlog on disk so it
	// survives a restart. Empty keeps the backlog in memory only.
	SpoolDir   string
	Brand      string
	Logger     *slog.Logger
	HTTPClient *http.Client
	Now        func() time.Time
}

// ErrInvalid wraps every validation failure so callers can answer 400.
var ErrInvalid = errors.New("invalid export destination")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Service owns destination CRUD and the background workers.
type Service struct {
	d       Deps
	prefix  string
	hdrPfx  string
	client  *http.Client
	nudge   chan struct{}
	mu      sync.Mutex
	workers map[string]*worker
	// startErrs remembers the last start failure per destination so a
	// persistent one is logged once, not every resync.
	startErrs map[string]string
}

// NewService builds a Service. Call Run to start delivery.
func NewService(d Deps) *Service {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Config.Resync == 0 {
		d.Config = DefaultConfig()
	}
	client := d.HTTPClient
	if client == nil {
		client = newHTTPClient()
	}
	brand := d.Brand
	if brand == "" {
		brand = "platform"
	}
	return &Service{
		d: d, prefix: SanitizePrefix(brand), client: client,
		hdrPfx:  "X-" + strings.ToUpper(brand[:1]) + strings.ToLower(brand[1:]),
		nudge:   make(chan struct{}, 1),
		workers: map[string]*worker{}, startErrs: map[string]string{},
	}
}

// MetricPrefix is the prefix every exported metric name carries.
func (s *Service) MetricPrefix() string { return s.prefix }

// Input is the writable part of a destination. Headers and HMACKey are
// write-only: they are stored encrypted and never returned.
type Input struct {
	Name     string
	Kind     store.ExportKind
	Endpoint string
	Enabled  bool
	Apps     []string
	MinLevel string
	Options  store.ExportOptions
	Headers  map[string]string
	HMACKey  string
	// APIKey is the vendor key for the vendor kinds; it is stored as the
	// header that vendor reads and is never returned.
	APIKey string
}

// Patch is a partial update; nil fields are left unchanged.
type Patch struct {
	Name     *string
	Endpoint *string
	Enabled  *bool
	Apps     *[]string
	MinLevel *string
	Options  *store.ExportOptions
	// Headers replaces the stored secret headers (an empty map clears them).
	Headers *map[string]string
	// HMACKey replaces the signing key (an empty string clears it).
	HMACKey *string
	// APIKey replaces a vendor destination's key.
	APIKey *string
}

type secretBlob struct {
	Headers map[string]string `json:"headers,omitempty"`
	HMACKey string            `json:"hmac_key,omitempty"`
}

var (
	labelName  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	headerName = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")
	reserved   = map[string]bool{"job": true, "app": true, "kind": true, "stream": true, "level": true}
	forbidden  = map[string]bool{"host": true, "content-length": true, "transfer-encoding": true, "connection": true, "content-type": true}
)

func isLogKind(k store.ExportKind) bool {
	return k == store.ExportKindLogsHTTP || k == store.ExportKindLogsSyslog || k == store.ExportKindLogsLoki || k == store.ExportKindLogsOTLP || IsVendorLogKind(k)
}

func isMetricsKind(k store.ExportKind) bool {
	return k == store.ExportKindMetricsOTLP || k == store.ExportKindMetricsRemoteWrite
}

func isAuditStreamKind(k store.ExportKind) bool {
	return k == store.ExportKindAuditSplunk || k == store.ExportKindAuditSyslog || k == store.ExportKindAuditOTLP
}

var hecText = regexp.MustCompile(`^[A-Za-z0-9_.:/-]{1,64}$`)

func (s *Service) validateAuditStream(in Input) error {
	o := in.Options
	if !isAuditStreamKind(in.Kind) {
		if o.Index != "" || o.Source != "" || o.SourceType != "" || o.Gzip || o.InsecureSkipVerify || o.Facility != "" || o.AppName != "" {
			return invalid("index, source, source_type, gzip, insecure_skip_verify, facility and app_name apply to audit_splunk, audit_syslog and audit_otlp only")
		}
		return nil
	}
	for name, v := range map[string]string{"index": o.Index, "source": o.Source, "source_type": o.SourceType} {
		if v != "" && (in.Kind != store.ExportKindAuditSplunk || !hecText.MatchString(v)) {
			return invalid("%s applies to audit_splunk and may contain letters, digits and _.:/- only", name)
		}
	}
	if in.Kind != store.ExportKindAuditSyslog && (o.Facility != "" || o.AppName != "") {
		return invalid("facility and app_name apply to audit_syslog only")
	}
	if in.Kind == store.ExportKindAuditSyslog && !ValidAuditFacility(o.Facility) {
		return invalid("facility must be a syslog facility name such as authpriv or local0")
	}
	if len(o.AppName) > 48 {
		return invalid("app_name is at most 48 characters")
	}
	if o.Gzip && in.Kind == store.ExportKindAuditSyslog {
		return invalid("gzip does not apply to syslog")
	}
	if in.Kind == store.ExportKindAuditSyslog {
		if len(in.Headers) > 0 {
			return invalid("headers do not apply to audit_syslog")
		}
		return nil
	}
	if in.Kind == store.ExportKindAuditSplunk && len(in.Headers) == 0 {
		return invalid("audit_splunk needs an HEC token (api_key)")
	}
	return nil
}

func (s *Service) validate(ctx context.Context, in Input) error {
	if n := strings.TrimSpace(in.Name); n == "" || len(n) > 64 {
		return invalid("name is required and at most 64 characters")
	}
	switch in.Kind {
	case store.ExportKindLogsHTTP, store.ExportKindLogsLoki, store.ExportKindLogsOTLP, store.ExportKindAuditWebhook, store.ExportKindMetricsOTLP, store.ExportKindMetricsRemoteWrite:
		if err := ValidateHTTPEndpoint(ctx, in.Endpoint); err != nil {
			return invalid("%v", err)
		}
	case store.ExportKindLogsSyslog:
		if err := ValidateSyslogEndpoint(ctx, in.Endpoint); err != nil {
			return invalid("%v", err)
		}
	case store.ExportKindAuditSplunk, store.ExportKindAuditOTLP:
		if err := ValidateHTTPEndpoint(ctx, in.Endpoint); err != nil {
			return invalid("%v", err)
		}
	case store.ExportKindAuditSyslog:
		if err := ValidateAuditSyslogEndpoint(ctx, in.Endpoint); err != nil {
			return invalid("%v", err)
		}
	case store.ExportKindLogsNewRelic, store.ExportKindLogsDatadog, store.ExportKindLogsAxiom, store.ExportKindLogsBetterStack:
		ep := in.Endpoint
		if ep == "" {
			var err error
			if ep, err = VendorEndpoint(in.Kind, in.Options); err != nil {
				return invalid("%v", err)
			}
		}
		if err := ValidateHTTPEndpoint(ctx, ep); err != nil {
			return invalid("%v", err)
		}
		if len(in.Headers) == 0 && in.APIKey == "" {
			return invalid("%s needs an API key", in.Kind)
		}
	case store.ExportKindAuditS3:
		if in.Options.StorageTargetID == "" {
			return invalid("audit_s3 needs options.storage_target_id (a connected storage destination)")
		}
	default:
		return invalid("unknown kind %q", in.Kind)
	}
	if err := s.validateAuditStream(in); err != nil {
		return err
	}
	if !isLogKind(in.Kind) && in.MinLevel != "" {
		return invalid("min_level applies to log destinations only")
	}
	if !isLogKind(in.Kind) && !isMetricsKind(in.Kind) && len(in.Apps) > 0 {
		return invalid("apps applies to log and metrics destinations only")
	}
	if !ValidLevel(in.MinLevel) {
		return invalid("min_level must be one of debug, info, warn, error")
	}
	if len(in.Apps) > 100 {
		return invalid("at most 100 apps per destination")
	}
	if len(in.Options.Labels) > 10 {
		return invalid("at most 10 static labels")
	}
	for k, v := range in.Options.Labels {
		if !labelName.MatchString(k) || reserved[k] || len(v) > 128 {
			return invalid("label %q is not allowed", k)
		}
	}
	if iv := in.Options.IntervalSeconds; iv != 0 && (iv < 10 || iv > 3600) {
		return invalid("interval_seconds must be between 10 and 3600")
	}
	if len(in.Headers) > 10 {
		return invalid("at most 10 headers")
	}
	for k, v := range in.Headers {
		if !headerName.MatchString(k) || forbidden[strings.ToLower(k)] {
			return invalid("header %q is not allowed", k)
		}
		if len(v) > 2048 || strings.ContainsAny(v, "\r\n") {
			return invalid("header %q has an invalid value", k)
		}
	}
	if in.Options.Username != "" {
		if in.Kind != store.ExportKindMetricsRemoteWrite {
			return invalid("username applies to metrics_remote_write only")
		}
		if err := validateRemoteWriteAuth(in.Options.Username); err != nil {
			return invalid("%v", err)
		}
	}
	if in.HMACKey != "" {
		if in.Kind != store.ExportKindAuditWebhook {
			return invalid("hmac_key applies to audit_webhook only")
		}
		if len(in.HMACKey) < 16 {
			return invalid("hmac_key must be at least 16 characters")
		}
	}
	return nil
}

func mergeHeaders(a, b map[string]string) map[string]string {
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func headerNames(h map[string]string) []string {
	names := make([]string, 0, len(h))
	for k := range h {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func (s *Service) writeSecrets(ctx context.Context, id string, blob secretBlob) error {
	if s.d.Secrets == nil {
		if len(blob.Headers) > 0 || blob.HMACKey != "" {
			return errors.New("dataexport: secret storage is not configured, so headers and signing keys cannot be saved")
		}
		return nil
	}
	raw, err := json.Marshal(blob)
	if err != nil {
		return fmt.Errorf("dataexport: encode secrets: %w", err)
	}
	if err := s.d.Secrets.SetValue(ctx, store.ExportDestinationSecretsKey(id), "config", string(raw)); err != nil {
		return fmt.Errorf("dataexport: store destination secrets: %w", err)
	}
	return nil
}

func (s *Service) readSecrets(ctx context.Context, d store.ExportDestination) (secretBlob, error) {
	if s.d.Secrets == nil || (len(d.Options.HeaderNames) == 0 && !d.Options.HasHMACKey) {
		return secretBlob{}, nil
	}
	raw, err := s.d.Secrets.Resolve(ctx, store.ExportDestinationSecretsKey(d.ID), "config")
	if err != nil {
		return secretBlob{}, fmt.Errorf("dataexport: resolve destination secrets: %w", err)
	}
	var b secretBlob
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		return secretBlob{}, fmt.Errorf("dataexport: decode destination secrets: %w", err)
	}
	return b, nil
}

// Create validates and stores a destination, then nudges the workers.
func (s *Service) Create(ctx context.Context, in Input) (store.ExportDestination, error) {
	in.Name = strings.TrimSpace(in.Name)
	if IsVendorLogKind(in.Kind) && in.APIKey != "" {
		in.Headers = mergeHeaders(in.Headers, vendorKeyHeaders(in.Kind, in.APIKey))
	}
	if in.Kind == store.ExportKindMetricsRemoteWrite && in.APIKey != "" {
		in.Headers = mergeHeaders(in.Headers, remoteWriteAuthHeaders(in.Options.Username, in.APIKey))
	}
	if in.Kind == store.ExportKindAuditSplunk && in.APIKey != "" {
		in.Headers = mergeHeaders(in.Headers, splunkAuthHeaders(in.APIKey))
	}
	if err := s.validate(ctx, in); err != nil {
		return store.ExportDestination{}, err
	}
	if IsVendorLogKind(in.Kind) && in.Endpoint == "" {
		ep, err := VendorEndpoint(in.Kind, in.Options)
		if err != nil {
			return store.ExportDestination{}, invalid("%v", err)
		}
		in.Endpoint = ep
	}
	id, err := store.NewExportDestinationID()
	if err != nil {
		return store.ExportDestination{}, fmt.Errorf("dataexport: %w", err)
	}
	now := s.d.Now().UTC().Format(time.RFC3339Nano)
	d := store.ExportDestination{
		ID: id, Name: in.Name, Kind: in.Kind, Endpoint: in.Endpoint, Enabled: in.Enabled,
		Apps: in.Apps, MinLevel: in.MinLevel, Options: in.Options, CreatedAt: now, UpdatedAt: now,
	}
	d.Options.HeaderNames = headerNames(in.Headers)
	d.Options.HasHMACKey = in.HMACKey != ""
	if len(in.Headers) > 0 || in.HMACKey != "" {
		if err := s.writeSecrets(ctx, id, secretBlob{Headers: in.Headers, HMACKey: in.HMACKey}); err != nil {
			return store.ExportDestination{}, err
		}
	}
	if err := s.d.Store.SaveExportDestination(ctx, d); err != nil {
		return store.ExportDestination{}, fmt.Errorf("dataexport: %w", err)
	}
	s.poke()
	return d, nil
}

// Update applies a Patch, re-validating the merged result.
func (s *Service) Update(ctx context.Context, id string, p Patch) (store.ExportDestination, error) {
	d, err := s.d.Store.GetExportDestination(ctx, id)
	if err != nil {
		return store.ExportDestination{}, fmt.Errorf("dataexport: %w", err)
	}
	blob, err := s.readSecrets(ctx, d)
	if err != nil {
		return store.ExportDestination{}, err
	}
	if p.Name != nil {
		d.Name = strings.TrimSpace(*p.Name)
	}
	if p.Endpoint != nil {
		d.Endpoint = *p.Endpoint
	}
	if p.Enabled != nil {
		d.Enabled = *p.Enabled
	}
	if p.Apps != nil {
		d.Apps = *p.Apps
	}
	if p.MinLevel != nil {
		d.MinLevel = *p.MinLevel
	}
	if p.Options != nil {
		d.Options = *p.Options
	}
	if p.Headers != nil {
		blob.Headers = *p.Headers
	}
	if IsVendorLogKind(d.Kind) {
		if p.APIKey != nil && *p.APIKey != "" {
			blob.Headers = mergeHeaders(blob.Headers, vendorKeyHeaders(d.Kind, *p.APIKey))
			p.Headers = &blob.Headers
		}
		// A region, site or dataset change moves a preset endpoint with it.
		if p.Options != nil && p.Endpoint == nil {
			if ep, err := VendorEndpoint(d.Kind, d.Options); err == nil {
				d.Endpoint = ep
			}
		}
	}
	if d.Kind == store.ExportKindMetricsRemoteWrite && p.APIKey != nil && *p.APIKey != "" {
		blob.Headers = mergeHeaders(blob.Headers, remoteWriteAuthHeaders(d.Options.Username, *p.APIKey))
		p.Headers = &blob.Headers
	}
	if d.Kind == store.ExportKindAuditSplunk && p.APIKey != nil && *p.APIKey != "" {
		blob.Headers = mergeHeaders(blob.Headers, splunkAuthHeaders(*p.APIKey))
		p.Headers = &blob.Headers
	}
	if p.HMACKey != nil {
		blob.HMACKey = *p.HMACKey
	}
	in := Input{Name: d.Name, Kind: d.Kind, Endpoint: d.Endpoint, Enabled: d.Enabled, Apps: d.Apps,
		MinLevel: d.MinLevel, Options: d.Options, Headers: blob.Headers, HMACKey: blob.HMACKey}
	if err := s.validate(ctx, in); err != nil {
		return store.ExportDestination{}, err
	}
	d.Options.HeaderNames = headerNames(blob.Headers)
	d.Options.HasHMACKey = blob.HMACKey != ""
	if p.Headers != nil || p.HMACKey != nil {
		if err := s.writeSecrets(ctx, id, blob); err != nil {
			return store.ExportDestination{}, err
		}
	}
	d.UpdatedAt = s.d.Now().UTC().Format(time.RFC3339Nano)
	if err := s.d.Store.UpdateExportDestination(ctx, d); err != nil {
		return store.ExportDestination{}, fmt.Errorf("dataexport: %w", err)
	}
	s.poke()
	return d, nil
}

// Delete removes a destination, its status and its stored secrets.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.d.Store.DeleteExportDestination(ctx, id); err != nil {
		return fmt.Errorf("dataexport: %w", err)
	}
	if s.d.SpoolDir != "" {
		name := spoolFileName(id)
		_ = os.Remove(filepath.Join(s.d.SpoolDir, name+".spool"))
		_ = os.Remove(filepath.Join(s.d.SpoolDir, name+".ack"))
	}
	if s.d.Secrets != nil {
		if err := s.d.Secrets.DeleteAll(ctx, store.ExportDestinationSecretsKey(id)); err != nil {
			s.d.Logger.Warn("dataexport: delete destination secrets failed", slog.String("destination_id", id), slog.String("error", err.Error()))
		}
	}
	s.poke()
	return nil
}

// Get returns one destination.
func (s *Service) Get(ctx context.Context, id string) (store.ExportDestination, error) {
	d, err := s.d.Store.GetExportDestination(ctx, id)
	if err != nil {
		return store.ExportDestination{}, fmt.Errorf("dataexport: %w", err)
	}
	return d, nil
}

// List returns every destination.
func (s *Service) List(ctx context.Context) ([]store.ExportDestination, error) {
	out, err := s.d.Store.ListExportDestinations(ctx)
	if err != nil {
		return nil, fmt.Errorf("dataexport: %w", err)
	}
	return out, nil
}

func (s *Service) poke() {
	select {
	case s.nudge <- struct{}{}:
	default:
	}
}
