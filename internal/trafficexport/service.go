package trafficexport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
)

// Sink kinds.
const (
	SinkPostHog  = "posthog"
	SinkOTLP     = "otlp"
	SinkWebhook  = "webhook"
	hostRegionUS = "us"
	hostRegionEU = "eu"
)

// Secret slots stored per sink. They are write-only through the API.
const (
	SecretURL        = "url"
	SecretToken      = "token"
	SecretAuthHeader = "auth_header"
)

// SinkKinds lists the supported sink kinds.
func SinkKinds() []string { return []string{SinkPostHog, SinkOTLP, SinkWebhook} }

// SecretSlots returns the secret slots a kind uses.
func SecretSlots(kind string) []string {
	switch kind {
	case SinkPostHog:
		return []string{SecretToken}
	case SinkOTLP, SinkWebhook:
		return []string{SecretURL, SecretAuthHeader}
	}
	return nil
}

// SinkSpec is one configured sink with its resolved secrets. Secrets must
// never be logged or returned to a client.
type SinkSpec struct {
	ID         string
	Name       string
	Kind       string
	Enabled    bool
	Host       string
	Fields     []string
	IPTruncate bool
	Sample     float64
	Secrets    map[string]string
}

// Policy is the field policy of the spec.
func (s SinkSpec) Policy() Policy { return Policy{Fields: s.Fields, IPTruncate: s.IPTruncate} }

func (s SinkSpec) fingerprint() string {
	b, _ := json.Marshal(s)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (s SinkSpec) secretValues() []string {
	out := make([]string, 0, len(s.Secrets))
	for _, v := range s.Secrets {
		out = append(out, v)
	}
	return out
}

// BuildSink validates spec and returns its sink.
func BuildSink(spec SinkSpec, o HTTPOptions) (Sink, error) {
	if err := ValidateFields(spec.Fields); err != nil {
		return nil, err
	}
	if spec.Sample < 0 || spec.Sample > 1 {
		return nil, fmt.Errorf("sample must be between 0 and 1")
	}
	var headers map[string]string
	if v := spec.Secrets[SecretAuthHeader]; v != "" {
		headers = map[string]string{"Authorization": v}
	}
	switch spec.Kind {
	case SinkPostHog:
		host := strings.TrimSpace(spec.Host)
		switch strings.ToLower(host) {
		case "", hostRegionUS:
			host = PostHogHostUS
		case hostRegionEU:
			host = PostHogHostEU
		}
		return NewPostHogSink(PostHogConfig{Host: host, Token: spec.Secrets[SecretToken], HTTP: o})
	case SinkOTLP:
		return NewOTLPSink(OTLPConfig{URL: spec.Secrets[SecretURL], Headers: headers, HTTP: o})
	case SinkWebhook:
		return NewWebhookSink(WebhookConfig{URL: spec.Secrets[SecretURL], Headers: headers, HTTP: o})
	}
	return nil, fmt.Errorf("unknown sink kind %q", spec.Kind)
}

// ValidateSpec checks spec against the sink rules and the environment's transport policy.
func ValidateSpec(spec SinkSpec) error {
	_, err := BuildSink(spec, HTTPOptionsFromEnv())
	return err
}

// Source loads the stored sinks with their secrets resolved.
type Source interface {
	LoadSpecs(ctx context.Context) ([]SinkSpec, error)
}

// SinkStatus is the live view of one stored sink.
type SinkStatus struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Kind          string    `json:"kind"`
	Enabled       bool      `json:"enabled"`
	Active        bool      `json:"active"`
	LastSuccess   time.Time `json:"last_success"`
	LastErrorKind string    `json:"last_error_kind,omitempty"`
	Queued        int       `json:"queued"`
	Sent          uint64    `json:"sent"`
	Dropped       uint64    `json:"dropped"`
	Failed        uint64    `json:"failed"`
	SampledOut    uint64    `json:"sampled_out"`
}

// TestResult is the outcome of one synthetic delivery.
type TestResult struct {
	OK         bool    `json:"ok"`
	ErrorKind  string  `json:"error_kind,omitempty"`
	DurationMS float64 `json:"duration_ms"`
}

type entry struct {
	fp     string
	policy Policy
	pipe   *Pipeline
}

type snapshot struct {
	specs   []SinkSpec
	entries map[string]*entry
	invalid map[string]string
}

// Service owns one pipeline per enabled sink and taps the access log. Every
// failure stays inside it: a request is never slowed or failed by export.
type Service struct {
	src     Source
	http    HTTPOptions
	opts    Options
	resolve func(key string) string
	salt    func(ctx context.Context) ([]byte, error)
	logger  *slog.Logger
	log     atomic.Pointer[ingress.AccessLog]

	mu       sync.Mutex
	snap     atomic.Pointer[snapshot]
	wantID   atomic.Bool
	wantVis  atomic.Bool
	saltOnce atomic.Pointer[[]byte]
}

// NewService returns a Service. resolve maps a route key to an app name; salt
// returns the server secret used for visitor ids (nil disables them).
func NewService(src Source, httpOpts HTTPOptions, opts Options, resolve func(string) string, salt func(context.Context) ([]byte, error), logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Service{src: src, http: httpOpts.withDefaults(), opts: opts.withDefaults(), resolve: resolve, salt: salt, logger: logger}
	s.snap.Store(&snapshot{entries: map[string]*entry{}, invalid: map[string]string{}})
	return s
}

// Install subscribes the service to log and returns the unsubscribe func.
func (s *Service) Install(log *ingress.AccessLog) func() {
	s.log.Store(log)
	log.CaptureIdentity(s.wantID.Load())
	unsub := log.Subscribe(s.Tap)
	return func() {
		unsub()
		log.CaptureIdentity(false)
	}
}

// IdentityWanted reports whether any active sink opted into identity or referrer data.
func (s *Service) IdentityWanted() bool { return s.wantID.Load() }

// Reload rebuilds pipelines for changed sinks, keeps unchanged ones running
// and retires removed ones.
func (s *Service) Reload(ctx context.Context) error {
	specs, err := s.src.LoadSpecs(ctx)
	if err != nil {
		return fmt.Errorf("trafficexport: load sinks: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.snap.Load()
	next := &snapshot{specs: specs, entries: map[string]*entry{}, invalid: map[string]string{}}
	wantID, wantVis := false, false
	for _, sp := range specs {
		if !sp.Enabled {
			continue
		}
		fp := sp.fingerprint()
		if e, ok := old.entries[sp.ID]; ok && e.fp == fp {
			next.entries[sp.ID] = e
		} else {
			sink, err := BuildSink(sp, s.http)
			if err != nil {
				next.invalid[sp.ID] = KindConfig
				s.logger.Warn("trafficexport: sink config rejected", "sink", sp.ID, "kind", sp.Kind)
				continue
			}
			sample := sp.Sample
			next.entries[sp.ID] = &entry{fp: fp, policy: sp.Policy(), pipe: NewPipeline(s.opts, SinkConfig{Sink: sink, Sample: sample, Secrets: sp.secretValues()})}
		}
		if HasIdentity(sp.Fields) || containsField(sp.Fields, FieldReferrerHost) {
			wantID = true
		}
		if containsField(sp.Fields, FieldVisitorID) {
			wantVis = true
		}
	}
	s.snap.Store(next)
	s.wantID.Store(wantID)
	s.wantVis.Store(wantVis)
	if l := s.log.Load(); l != nil {
		l.CaptureIdentity(wantID)
	}
	for id, e := range old.entries {
		if cur, ok := next.entries[id]; !ok || cur != e {
			go s.retire(e) //nolint:gosec // a replaced pipeline must flush after the reload call returns
		}
	}
	return nil
}

func containsField(fields []string, f string) bool {
	for _, x := range fields {
		if x == f {
			return true
		}
	}
	return false
}

// retire flushes a replaced pipeline off the caller's path.
func (s *Service) retire(e *entry) {
	ctx, cancel := context.WithTimeout(context.Background(), s.http.Timeout)
	defer cancel()
	_ = e.pipe.Close(ctx)
}

// Tap is the access-log subscriber. It never blocks and never panics out.
func (s *Service) Tap(key string, rec ingress.AccessRecord) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("trafficexport: tap panic recovered")
		}
	}()
	snap := s.snap.Load()
	if len(snap.entries) == 0 {
		return
	}
	app := ""
	if s.resolve != nil {
		app = s.resolve(key)
	}
	ev := NewEvent(app, rec)
	if id := rec.Identity; id != nil {
		ev.ClientIP, ev.UserAgent, ev.ReferrerHost = id.ClientIP, id.UserAgent, id.ReferrerHost
		if s.wantVis.Load() {
			ev.VisitorID = VisitorID(s.visitorSalt(), rec.Time, id.ClientIP, id.UserAgent)
		}
	}
	for _, e := range snap.entries {
		e.pipe.Enqueue(e.policy.Apply(ev))
	}
}

func (s *Service) visitorSalt() []byte {
	if p := s.saltOnce.Load(); p != nil {
		return *p
	}
	if s.salt == nil {
		return nil
	}
	b, err := s.salt(context.Background())
	if err != nil {
		return nil
	}
	s.saltOnce.Store(&b)
	return b
}

// Status returns one entry per stored sink, ordered by name then id.
func (s *Service) Status() []SinkStatus {
	snap := s.snap.Load()
	out := make([]SinkStatus, 0, len(snap.specs))
	for _, sp := range snap.specs {
		st := SinkStatus{ID: sp.ID, Name: sp.Name, Kind: sp.Kind, Enabled: sp.Enabled}
		if e, ok := snap.entries[sp.ID]; ok {
			ps := e.pipe.Stats()[0]
			st.Active = true
			st.LastSuccess, st.LastErrorKind, st.Queued = ps.LastSuccess, ps.LastKind, ps.Queued
			st.Sent, st.Dropped, st.Failed, st.SampledOut = ps.Sent, ps.Dropped, ps.Failed, ps.SampledOut
		} else if k, bad := snap.invalid[sp.ID]; bad {
			st.LastErrorKind = k
		}
		out = append(out, st)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Test sends one synthetic record through a fresh sink built from spec, shaped
// by the spec's own field policy. It does not touch the live pipelines.
func (s *Service) Test(ctx context.Context, spec SinkSpec) TestResult {
	start := time.Now()
	res := TestResult{}
	sink, err := BuildSink(spec, s.http)
	if err != nil {
		res.ErrorKind = KindConfig
		return res
	}
	cctx, cancel := context.WithTimeout(ctx, s.http.Timeout)
	defer cancel()
	ev := syntheticEvent(start)
	err = sink.Send(cctx, []Event{spec.Policy().Apply(ev)})
	res.DurationMS = float64(time.Since(start)) / float64(time.Millisecond)
	if err != nil {
		res.ErrorKind = ErrorKind(err)
		return res
	}
	res.OK = true
	return res
}

func syntheticEvent(now time.Time) Event {
	return Event{
		Time: now, App: "export-test", Host: "example.test", Method: "GET", Path: "/export-test",
		Status: 200, DurationMS: 1, RequestID: "export-test", ReferrerHost: "example.test",
		ClientIP: "192.0.2.10", UserAgent: "export-test", VisitorID: "0000000000000000export-test0000",
	}
}

// Close flushes and stops every pipeline.
func (s *Service) Close(ctx context.Context) {
	s.mu.Lock()
	snap := s.snap.Swap(&snapshot{entries: map[string]*entry{}, invalid: map[string]string{}})
	s.mu.Unlock()
	for _, e := range snap.entries {
		_ = e.pipe.Close(ctx)
	}
}
