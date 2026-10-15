// Package auditstream ships the hash-chained audit log to external sinks
// (syslog, HTTPS webhook, S3-compatible storage) at least once, with a
// durable per-sink cursor, backoff on failure and a delivery history.
package auditstream

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dataexport"
	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

const maxNameLen = 80

// ErrInvalid wraps every validation failure so callers can answer 400.
var ErrInvalid = errors.New("invalid audit sink")

// ErrSecretsUnavailable is returned when no master key is configured.
var ErrSecretsUnavailable = errors.New("auditstream: secret storage is not configured")

// SinkConfig is the non-secret, per-kind settings of a sink.
type SinkConfig struct {
	Address   string `json:"address,omitempty"`
	TLS       bool   `json:"tls,omitempty"`
	URL       string `json:"url,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
	Region    string `json:"region,omitempty"`
	Bucket    string `json:"bucket,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	PathStyle bool   `json:"path_style,omitempty"`
}

func decodeConfig(raw string) (SinkConfig, error) {
	var c SinkConfig
	if raw == "" {
		return c, nil
	}
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return c, fmt.Errorf("auditstream: decode sink config: %w", err)
	}
	return c, nil
}

// Store is the persistence surface; *store.DB satisfies it.
type Store interface {
	SaveAuditSink(ctx context.Context, k store.AuditSink) error
	UpdateAuditSink(ctx context.Context, k store.AuditSink) error
	RecordAuditSinkProgress(ctx context.Context, k store.AuditSink) error
	GetAuditSink(ctx context.Context, id string) (store.AuditSink, error)
	ListAuditSinks(ctx context.Context) ([]store.AuditSink, error)
	DeleteAuditSink(ctx context.Context, id string) error
	SaveAuditSinkDelivery(ctx context.Context, d store.AuditSinkDelivery) error
	ListAuditSinkDeliveries(ctx context.Context, sinkID string, limit int) ([]store.AuditSinkDelivery, error)
	DeleteAuditSinkDeliveriesOlderThan(ctx context.Context, cutoff string) (int64, error)
	ListAuditEntriesAfterSeq(ctx context.Context, afterSeq int64, limit int) ([]store.AuditEntry, error)
	AuditChainHeadSeq(ctx context.Context) (int64, error)
}

// Secrets stores sink credentials in envelope encryption.
type Secrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// Deps wires a Service.
type Deps struct {
	Store      Store
	Secrets    Secrets
	Config     Config
	Brand      string
	Logger     *slog.Logger
	HTTPClient *http.Client
	Now        func() time.Time
}

// Service owns sink CRUD and the streaming worker.
type Service struct {
	d       Deps
	hdrPfx  string
	appName string
	client  *http.Client
}

// NewService builds a Service. Call Run to start streaming.
func NewService(d Deps) *Service {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Config.Tick == 0 {
		d.Config = DefaultConfig()
	}
	client := d.HTTPClient
	if client == nil {
		client = netguard.NewClientAllowing(dataexport.AllowPrivateEnv, netguard.AllowPrivateEnv)
		client.Timeout = d.Config.SendTimeout
	}
	brand := d.Brand
	if brand == "" {
		brand = "platform"
	}
	return &Service{
		d: d, client: client, appName: brand,
		hdrPfx: "X-" + strings.ToUpper(brand[:1]) + strings.ToLower(brand[1:]),
	}
}

// Backoff is the wait after the failures-th consecutive failure.
func Backoff(cfg Config, failures int) time.Duration {
	d := cfg.BackoffBase
	for i := 1; i < failures && d < cfg.BackoffMax; i++ {
		d *= 2
	}
	if d > cfg.BackoffMax {
		d = cfg.BackoffMax
	}
	return d
}

// maxBatchesPerTick bounds catch-up work so one slow sink cannot starve the rest.
const maxBatchesPerTick = 10

// Run streams due sinks until ctx ends.
func (s *Service) Run(ctx context.Context) {
	tick := time.NewTicker(s.d.Config.Tick)
	defer tick.Stop()
	prune := time.NewTicker(time.Hour)
	defer prune.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.StreamOnce(ctx)
		case <-prune.C:
			cutoff := store.FormatAuditTime(s.d.Now().Add(-s.d.Config.Retention))
			if _, err := s.d.Store.DeleteAuditSinkDeliveriesOlderThan(ctx, cutoff); err != nil {
				s.d.Logger.Warn("auditstream: prune deliveries", slog.String("error", err.Error()))
			}
		}
	}
}

// StreamOnce advances every enabled sink that is not backing off.
func (s *Service) StreamOnce(ctx context.Context) {
	sinks, err := s.d.Store.ListAuditSinks(ctx)
	if err != nil {
		s.d.Logger.Warn("auditstream: list sinks", slog.String("error", err.Error()))
		return
	}
	for _, k := range sinks {
		if ctx.Err() != nil {
			return
		}
		if !k.Enabled || k.NextAttemptAt > store.FormatAuditTime(s.d.Now()) {
			continue
		}
		s.drain(ctx, k)
	}
}

func (s *Service) drain(ctx context.Context, k store.AuditSink) {
	for i := 0; i < maxBatchesPerTick; i++ {
		batch, err := s.d.Store.ListAuditEntriesAfterSeq(ctx, k.CursorSeq, s.d.Config.BatchSize)
		if err != nil {
			s.d.Logger.Warn("auditstream: read entries", slog.String("sink", k.ID), slog.String("error", err.Error()))
			return
		}
		if len(batch) == 0 {
			return
		}
		sendErr := s.send(ctx, k, batch)
		now := s.d.Now()
		d := store.AuditSinkDelivery{
			FromSeq: batch[0].Seq, ToSeq: batch[len(batch)-1].Seq, Entries: len(batch),
			State: store.AuditSinkDeliveryDelivered, Attempt: k.ConsecutiveFailures + 1,
			CreatedAt: store.FormatAuditTime(now),
		}
		if sendErr != nil {
			d.State = store.AuditSinkDeliveryFailed
			d.LastError = truncate(sendErr.Error(), 300)
			k.ConsecutiveFailures++
			k.LastError = d.LastError
			k.NextAttemptAt = store.FormatAuditTime(now.Add(Backoff(s.d.Config, k.ConsecutiveFailures)))
		} else {
			k.CursorSeq = d.ToSeq
			k.ConsecutiveFailures = 0
			k.LastError = ""
			k.NextAttemptAt = ""
			k.LastDeliveredAt = store.FormatAuditTime(now)
		}
		s.recordAttempt(ctx, k, d)
		if sendErr != nil || len(batch) < s.d.Config.BatchSize {
			return
		}
	}
}

// recordAttempt persists the cursor first: a crash between send and here
// re-sends the batch, which the at-least-once contract allows.
func (s *Service) recordAttempt(ctx context.Context, k store.AuditSink, d store.AuditSinkDelivery) {
	if err := s.d.Store.RecordAuditSinkProgress(ctx, k); err != nil {
		s.d.Logger.Warn("auditstream: record progress", slog.String("sink", k.ID), slog.String("error", err.Error()))
	}
	id, err := store.NewAuditSinkDeliveryID()
	if err != nil {
		return
	}
	d.ID, d.SinkID = id, k.ID
	if err := s.d.Store.SaveAuditSinkDelivery(ctx, d); err != nil {
		s.d.Logger.Warn("auditstream: record delivery", slog.String("sink", k.ID), slog.String("error", err.Error()))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Input is the writable part of a sink. Credentials are write-only.
type Input struct {
	Name            string
	Kind            string
	Enabled         bool
	Config          SinkConfig
	AccessKeyID     string
	SecretAccessKey string
	// Backfill streams the whole retained chain instead of only new entries.
	Backfill bool
}

// Patch holds the optional changes of an update.
type Patch struct {
	Name            *string
	Enabled         *bool
	Config          *SinkConfig
	AccessKeyID     *string
	SecretAccessKey *string
}

func validateConfig(ctx context.Context, kind string, c SinkConfig) error {
	switch kind {
	case store.AuditSinkSyslog:
		scheme := "tcp"
		if c.TLS {
			scheme = "tls"
		}
		if err := dataexport.ValidateSyslogEndpoint(ctx, scheme+"://"+c.Address); err != nil {
			return invalid("%v", err)
		}
	case store.AuditSinkWebhook:
		if err := dataexport.ValidateHTTPEndpoint(ctx, c.URL); err != nil {
			return invalid("%v", err)
		}
		if u, err := url.Parse(c.URL); err != nil || u.Scheme != "https" {
			return invalid("webhook url must use https")
		}
	case store.AuditSinkS3:
		if c.Bucket == "" {
			return invalid("s3 bucket is required")
		}
		if c.Endpoint != "" {
			if err := objectstore.ValidateEndpoint(c.Endpoint); err != nil {
				return invalid("%v", err)
			}
		}
	default:
		return invalid("kind must be syslog, webhook or s3")
	}
	return nil
}

func validateName(name string) error {
	if name == "" || len(name) > maxNameLen {
		return invalid("name is required and must be at most %d characters", maxNameLen)
	}
	return nil
}

func newHMACSecret() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auditstream: generate signing secret: %w", err)
	}
	return "whsec_" + hex.EncodeToString(buf), nil
}

func (s *Service) storeCredentials(ctx context.Context, id, kind, accessKey, secretKey string) (hmacSecret string, err error) {
	key := store.AuditSinkSecretsKey(id)
	switch kind {
	case store.AuditSinkWebhook:
		if hmacSecret, err = newHMACSecret(); err != nil {
			return "", err
		}
		if err = s.d.Secrets.SetValue(ctx, key, secretHMAC, hmacSecret); err != nil {
			return "", fmt.Errorf("auditstream: store signing secret: %w", err)
		}
	case store.AuditSinkS3:
		if accessKey != "" {
			if err = s.d.Secrets.SetValue(ctx, key, secretAccessKey, accessKey); err != nil {
				return "", fmt.Errorf("auditstream: store access key: %w", err)
			}
		}
		if secretKey != "" {
			if err = s.d.Secrets.SetValue(ctx, key, secretSecretKey, secretKey); err != nil {
				return "", fmt.Errorf("auditstream: store secret key: %w", err)
			}
		}
	}
	return hmacSecret, nil
}

func needsSecrets(kind string) bool {
	return kind == store.AuditSinkWebhook || kind == store.AuditSinkS3
}

// Create validates and stores a sink. For a webhook it also returns the
// HMAC signing secret, shown once and never readable afterwards.
func (s *Service) Create(ctx context.Context, in Input) (store.AuditSink, string, error) {
	in.Name = strings.TrimSpace(in.Name)
	if err := validateName(in.Name); err != nil {
		return store.AuditSink{}, "", err
	}
	if err := validateConfig(ctx, in.Kind, in.Config); err != nil {
		return store.AuditSink{}, "", err
	}
	if in.Kind == store.AuditSinkS3 && (in.AccessKeyID == "" || in.SecretAccessKey == "") {
		return store.AuditSink{}, "", invalid("s3 access key id and secret access key are required")
	}
	if needsSecrets(in.Kind) && s.d.Secrets == nil {
		return store.AuditSink{}, "", ErrSecretsUnavailable
	}
	id, err := store.NewAuditSinkID()
	if err != nil {
		return store.AuditSink{}, "", err
	}
	var secret string
	if needsSecrets(in.Kind) {
		if secret, err = s.storeCredentials(ctx, id, in.Kind, in.AccessKeyID, in.SecretAccessKey); err != nil {
			_ = s.d.Secrets.DeleteAll(ctx, store.AuditSinkSecretsKey(id))
			return store.AuditSink{}, "", err
		}
	}
	cfg, err := json.Marshal(in.Config)
	if err != nil {
		return store.AuditSink{}, "", fmt.Errorf("auditstream: encode config: %w", err)
	}
	var cursor int64
	if !in.Backfill {
		if cursor, err = s.d.Store.AuditChainHeadSeq(ctx); err != nil {
			return store.AuditSink{}, "", err
		}
	}
	now := store.FormatAuditTime(s.d.Now())
	k := store.AuditSink{
		ID: id, Name: in.Name, Kind: in.Kind, Enabled: in.Enabled, Config: string(cfg),
		CursorSeq: cursor, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.d.Store.SaveAuditSink(ctx, k); err != nil {
		if s.d.Secrets != nil {
			_ = s.d.Secrets.DeleteAll(ctx, store.AuditSinkSecretsKey(id))
		}
		return store.AuditSink{}, "", err
	}
	return k, secret, nil
}

// Update applies a patch. Supplied credentials replace the sealed ones.
func (s *Service) Update(ctx context.Context, id string, p Patch) (store.AuditSink, error) {
	k, err := s.d.Store.GetAuditSink(ctx, id)
	if err != nil {
		return store.AuditSink{}, err
	}
	cfg, err := decodeConfig(k.Config)
	if err != nil {
		return store.AuditSink{}, err
	}
	if p.Name != nil {
		k.Name = strings.TrimSpace(*p.Name)
	}
	if p.Enabled != nil {
		k.Enabled = *p.Enabled
	}
	if p.Config != nil {
		cfg = *p.Config
	}
	if err := validateName(k.Name); err != nil {
		return store.AuditSink{}, err
	}
	if err := validateConfig(ctx, k.Kind, cfg); err != nil {
		return store.AuditSink{}, err
	}
	if k.Kind == store.AuditSinkS3 && (p.AccessKeyID != nil || p.SecretAccessKey != nil) {
		if s.d.Secrets == nil {
			return store.AuditSink{}, ErrSecretsUnavailable
		}
		var ak, sk string
		if p.AccessKeyID != nil {
			ak = *p.AccessKeyID
		}
		if p.SecretAccessKey != nil {
			sk = *p.SecretAccessKey
		}
		if _, err := s.storeCredentials(ctx, id, k.Kind, ak, sk); err != nil {
			return store.AuditSink{}, err
		}
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return store.AuditSink{}, fmt.Errorf("auditstream: encode config: %w", err)
	}
	k.Config = string(raw)
	k.UpdatedAt = store.FormatAuditTime(s.d.Now())
	if err := s.d.Store.UpdateAuditSink(ctx, k); err != nil {
		return store.AuditSink{}, err
	}
	return k, nil
}

// RotateSecret issues a new webhook signing secret and returns it once.
func (s *Service) RotateSecret(ctx context.Context, id string) (string, error) {
	k, err := s.d.Store.GetAuditSink(ctx, id)
	if err != nil {
		return "", err
	}
	if k.Kind != store.AuditSinkWebhook {
		return "", invalid("only webhook sinks have a signing secret")
	}
	if s.d.Secrets == nil {
		return "", ErrSecretsUnavailable
	}
	return s.storeCredentials(ctx, id, k.Kind, "", "")
}

// Delete removes a sink, its history and its credentials.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.d.Store.DeleteAuditSink(ctx, id); err != nil {
		return err
	}
	if s.d.Secrets != nil {
		if err := s.d.Secrets.DeleteAll(ctx, store.AuditSinkSecretsKey(id)); err != nil {
			return fmt.Errorf("auditstream: delete credentials: %w", err)
		}
	}
	return nil
}

// Get returns one sink.
func (s *Service) Get(ctx context.Context, id string) (store.AuditSink, error) {
	return s.d.Store.GetAuditSink(ctx, id)
}

// List returns every sink.
func (s *Service) List(ctx context.Context) ([]store.AuditSink, error) {
	return s.d.Store.ListAuditSinks(ctx)
}

// Deliveries lists a sink's newest delivery attempts.
func (s *Service) Deliveries(ctx context.Context, id string, limit int) ([]store.AuditSinkDelivery, error) {
	if _, err := s.d.Store.GetAuditSink(ctx, id); err != nil {
		return nil, err
	}
	return s.d.Store.ListAuditSinkDeliveries(ctx, id, limit)
}

// Test sends one synthetic entry (seq 0, never part of the chain) and
// reports whether the destination accepted it.
func (s *Service) Test(ctx context.Context, id string) error {
	k, err := s.d.Store.GetAuditSink(ctx, id)
	if err != nil {
		return err
	}
	probe := store.AuditEntry{
		ID: "audit_sink_test", ActorType: "system", ActorName: "sink test", Action: "audit_sink.test",
		CreatedAt: store.FormatAuditTime(s.d.Now()),
	}
	return s.send(ctx, k, []store.AuditEntry{probe})
}
