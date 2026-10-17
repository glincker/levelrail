package ai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Vault errors the API maps to status codes.
var (
	ErrUnknownProvider = errors.New("ai: unknown provider")
	ErrNoUsableChain   = errors.New("ai: active provider has no usable key")
)

// SaveError is a typed, user-presentable failure from Vault.Set. It never
// contains the API key.
type SaveError struct {
	Code    string
	Message string
	Fix     string
}

func (e *SaveError) Error() string { return e.Message }

// VaultStore is the internal/store surface Vault needs.
type VaultStore interface {
	GetAIAssistantSettings(ctx context.Context) (store.AIAssistantSettings, error)
	UpdateAIAssistantSettings(ctx context.Context, s store.AIAssistantSettings) error
	GetAIProviderConfig(ctx context.Context, id string) (store.AIProviderConfig, error)
	UpsertAIProviderConfig(ctx context.Context, c store.AIProviderConfig) error
	ListAIProviderConfigs(ctx context.Context) ([]store.AIProviderConfig, error)
	DeleteAIProviderConfig(ctx context.Context, id string) error
	SetAIProviderStatus(ctx context.Context, id, status, detail string, at time.Time) error
}

// VaultSecrets is the internal/secrets.Manager surface Vault needs. Keys are
// written here and read back only to build a provider call.
type VaultSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// BackendFactory builds an adapter; tests swap it to point at fake servers.
type BackendFactory func(info ProviderInfo, cfg AdapterConfig) Backend

// DefaultBackendFactory picks the adapter by catalog kind.
func DefaultBackendFactory(info ProviderInfo, cfg AdapterConfig) Backend {
	switch info.Kind {
	case KindAnthropic:
		return NewAnthropic(cfg)
	case KindGemini:
		return NewGemini(cfg)
	default:
		return NewOpenAICompat(cfg)
	}
}

// Vault manages per-provider keys and settings and builds the provider chain.
type Vault struct {
	store    VaultStore
	secrets  VaultSecrets
	factory  BackendFactory
	resolve  HostResolver
	logger   *slog.Logger
	now      func() time.Time
	policy   RetryPolicy
	defaults int

	mu    sync.Mutex
	cache map[string]modelsEntry
}

type modelsEntry struct {
	at     time.Time
	models []ModelInfo
}

// VaultOption configures a Vault.
type VaultOption func(*Vault)

// WithBackendFactory overrides adapter construction.
func WithBackendFactory(f BackendFactory) VaultOption { return func(v *Vault) { v.factory = f } }

// WithHostResolver overrides DNS resolution in the SSRF check.
func WithHostResolver(r HostResolver) VaultOption { return func(v *Vault) { v.resolve = r } }

// WithRetryPolicy overrides the retry policy used for chain calls.
func WithRetryPolicy(p RetryPolicy) VaultOption { return func(v *Vault) { v.policy = p } }

// WithClock overrides the vault clock.
func WithClock(now func() time.Time) VaultOption { return func(v *Vault) { v.now = now } }

// NewVault builds a Vault.
func NewVault(s VaultStore, sec VaultSecrets, logger *slog.Logger, opts ...VaultOption) *Vault {
	if logger == nil {
		logger = slog.Default()
	}
	v := &Vault{
		store: s, secrets: sec, factory: DefaultBackendFactory, logger: logger, now: time.Now,
		policy: RetryPolicyFromEnv(), defaults: LimitsFromEnv().DefaultContext, cache: map[string]modelsEntry{},
	}
	for _, o := range opts {
		o(v)
	}
	return v
}

// ProviderView is the API shape of one provider.
type ProviderView struct {
	ID              string `json:"id"`
	Label           string `json:"label"`
	Kind            Kind   `json:"kind"`
	DefaultBaseURL  string `json:"default_base_url"`
	BaseURLEditable bool   `json:"base_url_editable"`
	KeyURL          string `json:"key_url"`
	DefaultModel    string `json:"default_model"`
	Brand           string `json:"brand"`
	HasKey          bool   `json:"has_key"`
	BaseURL         string `json:"base_url,omitempty"`
	Status          string `json:"status"`
	StatusDetail    string `json:"status_detail,omitempty"`
	LastCheckedAt   string `json:"last_checked_at,omitempty"`
}

// SetInput is what PUT /providers/{id} accepts. Empty fields are unchanged.
type SetInput struct {
	APIKey       string
	BaseURL      string
	DefaultModel string
}

// SetResult reports facts the API layer audits.
type SetResult struct {
	PrivateBaseURL bool
}

func (v *Vault) view(info ProviderInfo, cfg store.AIProviderConfig, found, hasKey bool) ProviderView {
	pv := ProviderView{
		ID: info.ID, Label: info.Label, Kind: info.Kind, DefaultBaseURL: info.DefaultBaseURL,
		BaseURLEditable: info.BaseURLEditable, KeyURL: info.KeyURL, DefaultModel: info.DefaultModel,
		Brand: info.Brand, HasKey: hasKey, Status: store.AIProviderStatusUntested,
	}
	if found {
		pv.BaseURL = cfg.BaseURL
		if cfg.DefaultModel != "" {
			pv.DefaultModel = cfg.DefaultModel
		}
		pv.Status = cfg.Status
		pv.StatusDetail = cfg.StatusDetail
		if !cfg.LastCheckedAt.IsZero() {
			pv.LastCheckedAt = cfg.LastCheckedAt.UTC().Format(time.RFC3339)
		}
	}
	return pv
}

// Get returns one provider's view.
func (v *Vault) Get(ctx context.Context, id string) (ProviderView, error) {
	info, ok := LookupProvider(id)
	if !ok {
		return ProviderView{}, ErrUnknownProvider
	}
	cfg, err := v.store.GetAIProviderConfig(ctx, id)
	found := err == nil
	if err != nil && !errors.Is(err, store.ErrAIProviderConfigNotFound) {
		return ProviderView{}, fmt.Errorf("ai: get provider %s: %w", id, err)
	}
	has, err := v.secrets.Exists(ctx, store.AIProviderSecretsKey(id), SecretsAPIKeyEnvKey)
	if err != nil {
		return ProviderView{}, fmt.Errorf("ai: check key for %s: %w", id, err)
	}
	return v.view(info, cfg, found, has), nil
}

// List returns every catalog provider with its saved state.
func (v *Vault) List(ctx context.Context) ([]ProviderView, error) {
	out := make([]ProviderView, 0, len(Catalog()))
	for _, info := range Catalog() {
		pv, err := v.Get(ctx, info.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, pv)
	}
	return out, nil
}

func statusForClass(c ErrorClass) string {
	switch c {
	case ClassInvalidKey:
		return store.AIProviderStatusInvalid
	case ClassQuotaExhausted:
		return store.AIProviderStatusExhausted
	default:
		return store.AIProviderStatusUnreach
	}
}

func (v *Vault) backendFor(info ProviderInfo, baseURL, key string, private bool) Backend {
	return v.factory(info, AdapterConfig{
		ProviderID: info.ID, BaseURL: baseURL, APIKey: key,
		HTTP: NewHTTPClient(private && info.Kind == KindOpenAICompat),
	})
}

func (v *Vault) validate(ctx context.Context, info ProviderInfo, baseURL, key string, private bool) *ProviderError {
	vctx, cancel := context.WithTimeout(ctx, ValidateTimeout())
	defer cancel()
	err := v.backendFor(info, baseURL, key, private).Validate(vctx)
	if err == nil {
		return nil
	}
	if pe, ok := AsProviderError(err); ok {
		return pe
	}
	return ClassifyTransport(info.ID, err, key)
}

func saveErrorFor(pe *ProviderError) *SaveError {
	msg, fix := pe.Explain()
	code := string(pe.Class)
	switch pe.Class {
	case ClassNetwork, ClassOverloaded, ClassRateLimited, ClassBlockedAddress, ClassBadRequest, ClassModelNotFound, ClassStreamCut:
		code = "unreachable"
	}
	return &SaveError{Code: code, Message: msg, Fix: fix}
}

// Set validates and stores a provider's key, base URL and default model.
// privateAllowed says the caller is root and may point an openai_compat
// provider at a private or loopback host. A key that fails validation is
// not stored.
func (v *Vault) Set(ctx context.Context, id string, in SetInput, privateAllowed bool) (ProviderView, SetResult, error) {
	info, ok := LookupProvider(id)
	if !ok {
		return ProviderView{}, SetResult{}, ErrUnknownProvider
	}
	log := v.logger.With(slog.String("provider", id))
	cfg, err := v.store.GetAIProviderConfig(ctx, id)
	if err != nil && !errors.Is(err, store.ErrAIProviderConfigNotFound) {
		return ProviderView{}, SetResult{}, fmt.Errorf("ai: set provider %s: %w", id, err)
	}
	cfg.ProviderID = id

	if in.BaseURL != "" && !info.BaseURLEditable {
		return ProviderView{}, SetResult{}, &SaveError{Code: "base_url_fixed", Message: "This provider's base URL cannot be changed.", Fix: "Use the Other (OpenAI compatible) provider for a custom endpoint."}
	}
	baseURL := firstNonEmpty(in.BaseURL, cfg.BaseURL, info.DefaultBaseURL)
	if baseURL == "" {
		return ProviderView{}, SetResult{}, &SaveError{Code: "base_url_required", Message: "A base URL is required for this provider.", Fix: "Enter the endpoint URL, for example https://my-resource.openai.azure.com/openai/v1."}
	}
	check, err := CheckBaseURL(ctx, baseURL, info.Kind, privateAllowed, v.resolve)
	if err != nil {
		log.Warn("ai: provider base url rejected")
		return ProviderView{}, SetResult{}, &SaveError{Code: "blocked_base_url", Message: err.Error(), Fix: "Use a public https URL. Private addresses are only allowed for OpenAI compatible servers, set by a root user."}
	}
	if in.BaseURL != "" {
		cfg.BaseURL = check.URL
	}
	cfg.PrivateURLOK = check.Private
	if in.DefaultModel != "" {
		cfg.DefaultModel = in.DefaultModel
	}

	key := in.APIKey
	needsCheck := in.APIKey != "" || in.BaseURL != ""
	if needsCheck {
		if key == "" {
			key, err = v.existingKey(ctx, id)
			if err != nil {
				return ProviderView{}, SetResult{}, err
			}
		}
		if key == "" && info.KeyRequired {
			return ProviderView{}, SetResult{}, &SaveError{Code: "key_required", Message: "An API key is required for this provider.", Fix: "Paste a key from " + info.KeyURL}
		}
		if pe := v.validate(ctx, info, check.URL, key, check.Private); pe != nil {
			cfg.Status, cfg.StatusDetail, cfg.LastCheckedAt = statusForClass(pe.Class), pe.Detail, v.now()
			log.Warn("ai: provider validation failed", slog.String("class", string(pe.Class)))
			if pe.Class != ClassQuotaExhausted {
				return ProviderView{}, SetResult{}, saveErrorFor(pe)
			}
		} else {
			cfg.Status, cfg.StatusDetail, cfg.LastCheckedAt = store.AIProviderStatusOK, "", v.now()
		}
		if in.APIKey != "" {
			if err := v.secrets.SetValue(ctx, store.AIProviderSecretsKey(id), SecretsAPIKeyEnvKey, in.APIKey); err != nil {
				return ProviderView{}, SetResult{}, fmt.Errorf("ai: store key for %s: %w", id, err)
			}
		}
	}
	if err := v.store.UpsertAIProviderConfig(ctx, cfg); err != nil {
		return ProviderView{}, SetResult{}, fmt.Errorf("ai: save provider %s: %w", id, err)
	}
	v.dropModels(id)
	log.Info("ai: provider saved", slog.Bool("key_changed", in.APIKey != ""), slog.Bool("private_base_url", check.Private))
	pv, err := v.Get(ctx, id)
	return pv, SetResult{PrivateBaseURL: check.Private}, err
}

func (v *Vault) existingKey(ctx context.Context, id string) (string, error) {
	has, err := v.secrets.Exists(ctx, store.AIProviderSecretsKey(id), SecretsAPIKeyEnvKey)
	if err != nil || !has {
		return "", err
	}
	key, err := v.secrets.Resolve(ctx, store.AIProviderSecretsKey(id), SecretsAPIKeyEnvKey)
	if err != nil {
		return "", fmt.Errorf("ai: read key for %s: %w", id, err)
	}
	return key, nil
}

func firstNonEmpty(vals ...string) string {
	for _, s := range vals {
		if s != "" {
			return s
		}
	}
	return ""
}

// TestResult is the outcome of a key test.
type TestResult struct {
	OK           bool   `json:"ok"`
	Status       string `json:"status"`
	StatusDetail string `json:"status_detail,omitempty"`
	LatencyMS    int64  `json:"latency_ms"`
}

// Test re-validates the saved key and records the result.
func (v *Vault) Test(ctx context.Context, id string) (TestResult, error) {
	info, ok := LookupProvider(id)
	if !ok {
		return TestResult{}, ErrUnknownProvider
	}
	cfg, key, baseURL, err := v.load(ctx, info)
	if err != nil {
		return TestResult{}, err
	}
	start := v.now()
	pe := v.validate(ctx, info, baseURL, key, cfg.PrivateURLOK)
	res := TestResult{OK: pe == nil, Status: store.AIProviderStatusOK, LatencyMS: v.now().Sub(start).Milliseconds()}
	if pe != nil {
		res.Status, res.StatusDetail = statusForClass(pe.Class), pe.Detail
		if res.StatusDetail == "" {
			res.StatusDetail, _ = pe.Explain()
		}
	}
	if _, gerr := v.store.GetAIProviderConfig(ctx, id); gerr == nil {
		if err := v.store.SetAIProviderStatus(ctx, id, res.Status, res.StatusDetail, v.now()); err != nil {
			v.logger.Warn("ai: record provider status failed", slog.String("provider", id), slog.String("error", err.Error()))
		}
	}
	return res, nil
}

// load resolves a provider's saved config and key. A missing key for a
// provider that requires one fails closed.
func (v *Vault) load(ctx context.Context, info ProviderInfo) (store.AIProviderConfig, string, string, error) {
	cfg, err := v.store.GetAIProviderConfig(ctx, info.ID)
	if err != nil && !errors.Is(err, store.ErrAIProviderConfigNotFound) {
		return cfg, "", "", fmt.Errorf("ai: load provider %s: %w", info.ID, err)
	}
	key, err := v.existingKey(ctx, info.ID)
	if err != nil {
		return cfg, "", "", err
	}
	if key == "" && info.KeyRequired {
		return cfg, "", "", &SaveError{Code: "key_required", Message: "No API key is saved for this provider.", Fix: "Add a key in Settings, AI assistant."}
	}
	baseURL := firstNonEmpty(cfg.BaseURL, info.DefaultBaseURL)
	if baseURL == "" {
		return cfg, "", "", &SaveError{Code: "base_url_required", Message: "No base URL is saved for this provider.", Fix: "Enter the endpoint URL."}
	}
	return cfg, key, baseURL, nil
}

// Remove deletes a provider's key and settings, and drops it from the active
// choice and fallback chain.
func (v *Vault) Remove(ctx context.Context, id string) error {
	if _, ok := LookupProvider(id); !ok {
		return ErrUnknownProvider
	}
	if err := v.secrets.DeleteAll(ctx, store.AIProviderSecretsKey(id)); err != nil {
		return fmt.Errorf("ai: delete key for %s: %w", id, err)
	}
	if err := v.store.DeleteAIProviderConfig(ctx, id); err != nil {
		return fmt.Errorf("ai: delete provider %s: %w", id, err)
	}
	s, err := v.store.GetAIAssistantSettings(ctx)
	if err != nil {
		return fmt.Errorf("ai: remove provider %s: %w", id, err)
	}
	if s.Provider == id {
		s.Provider, s.Model = "", ""
	}
	s.Fallback = slices.DeleteFunc(slices.Clone(s.Fallback), func(r store.AIModelRef) bool { return r.Provider == id })
	if err := v.store.UpdateAIAssistantSettings(ctx, s); err != nil {
		return fmt.Errorf("ai: remove provider %s: %w", id, err)
	}
	v.dropModels(id)
	v.logger.Info("ai: provider key removed", slog.String("provider", id))
	return nil
}

func (v *Vault) dropModels(id string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.cache, id)
}

func (v *Vault) cachedModels(id string) ([]ModelInfo, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	e, ok := v.cache[id]
	if !ok || v.now().Sub(e.at) > ModelsCacheTTL() {
		return nil, false
	}
	return e.models, true
}

// Models lists a provider's models, cached for ModelsCacheTTL.
func (v *Vault) Models(ctx context.Context, id string) ([]ModelInfo, error) {
	info, ok := LookupProvider(id)
	if !ok {
		return nil, ErrUnknownProvider
	}
	if m, ok := v.cachedModels(id); ok {
		return m, nil
	}
	cfg, key, baseURL, err := v.load(ctx, info)
	if err != nil {
		return nil, err
	}
	lctx, cancel := context.WithTimeout(ctx, ValidateTimeout())
	defer cancel()
	models, err := v.backendFor(info, baseURL, key, cfg.PrivateURLOK).ListModels(lctx)
	if err != nil {
		return nil, err
	}
	if models == nil {
		models = []ModelInfo{}
	}
	sort.SliceStable(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	v.mu.Lock()
	v.cache[id] = modelsEntry{at: v.now(), models: models}
	v.mu.Unlock()
	return models, nil
}

// AssistantSettings is the API shape of GET /settings/ai-assistant.
type AssistantSettings struct {
	Configured bool
	Active     *store.AIModelRef
	Fallback   []store.AIModelRef
}

func (v *Vault) usable(ctx context.Context, info ProviderInfo) (bool, error) {
	has, err := v.secrets.Exists(ctx, store.AIProviderSecretsKey(info.ID), SecretsAPIKeyEnvKey)
	if err != nil {
		return false, fmt.Errorf("ai: check key for %s: %w", info.ID, err)
	}
	if has {
		return true, nil
	}
	if info.KeyRequired {
		return false, nil
	}
	_, err = v.store.GetAIProviderConfig(ctx, info.ID)
	if errors.Is(err, store.ErrAIProviderConfigNotFound) {
		return false, nil
	}
	return err == nil, err
}

// Settings reports the active choice and fallback chain.
func (v *Vault) Settings(ctx context.Context) (AssistantSettings, error) {
	s, err := v.store.GetAIAssistantSettings(ctx)
	if err != nil {
		return AssistantSettings{}, fmt.Errorf("ai: settings: %w", err)
	}
	out := AssistantSettings{Fallback: s.Fallback}
	if out.Fallback == nil {
		out.Fallback = []store.AIModelRef{}
	}
	if s.Provider == "" || s.Model == "" {
		return out, nil
	}
	out.Active = &store.AIModelRef{Provider: s.Provider, Model: s.Model}
	if info, ok := LookupProvider(s.Provider); ok {
		out.Configured, err = v.usable(ctx, info)
	}
	return out, err
}

// MaxFallback bounds the fallback chain length.
const MaxFallback = 5

func (v *Vault) checkRef(ctx context.Context, r store.AIModelRef) error {
	info, ok := LookupProvider(r.Provider)
	if !ok {
		return &SaveError{Code: "unknown_provider", Message: fmt.Sprintf("Unknown provider %q.", r.Provider)}
	}
	if strings.TrimSpace(r.Model) == "" {
		return &SaveError{Code: "model_required", Message: "A model is required for " + info.Label + "."}
	}
	ok, err := v.usable(ctx, info)
	if err != nil {
		return err
	}
	if !ok {
		return &SaveError{Code: "key_required", Message: info.Label + " has no saved key.", Fix: "Add a key for " + info.Label + " first."}
	}
	return nil
}

// SetChoice replaces the active choice and/or fallback chain; nil leaves a part unchanged.
func (v *Vault) SetChoice(ctx context.Context, active *store.AIModelRef, fallback *[]store.AIModelRef) error {
	s, err := v.store.GetAIAssistantSettings(ctx)
	if err != nil {
		return fmt.Errorf("ai: set choice: %w", err)
	}
	if active != nil {
		if err := v.checkRef(ctx, *active); err != nil {
			return err
		}
		s.Provider, s.Model = active.Provider, active.Model
	}
	if fallback != nil {
		if len(*fallback) > MaxFallback {
			return &SaveError{Code: "fallback_too_long", Message: fmt.Sprintf("The fallback chain holds at most %d entries.", MaxFallback)}
		}
		seen := map[store.AIModelRef]bool{}
		for _, r := range *fallback {
			if err := v.checkRef(ctx, r); err != nil {
				return err
			}
			if seen[r] {
				return &SaveError{Code: "duplicate_fallback", Message: "The fallback chain lists " + r.Provider + ":" + r.Model + " twice."}
			}
			seen[r] = true
		}
		s.Fallback = *fallback
	}
	if err := v.store.UpdateAIAssistantSettings(ctx, s); err != nil {
		return fmt.Errorf("ai: set choice: %w", err)
	}
	return nil
}

// Configured reports whether the active provider can serve a turn.
func (v *Vault) Configured(ctx context.Context) (bool, error) {
	s, err := v.Settings(ctx)
	return s.Configured, err
}

// Chain builds the provider chain for one turn: the active choice first,
// then each usable fallback. A fallback with a missing key is skipped; a
// missing key on the active choice fails closed.
func (v *Vault) Chain(ctx context.Context) (Provider, error) {
	s, err := v.store.GetAIAssistantSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("ai: chain: %w", err)
	}
	if s.Provider == "" || s.Model == "" {
		return nil, ErrNotConfigured
	}
	refs := append([]store.AIModelRef{{Provider: s.Provider, Model: s.Model}}, s.Fallback...)
	chain := &ChainProvider{DefaultContext: v.defaults, Logger: v.logger, Now: v.now}
	for i, r := range refs {
		info, ok := LookupProvider(r.Provider)
		if !ok {
			continue
		}
		cfg, key, baseURL, lerr := v.load(ctx, info)
		if lerr != nil {
			if i == 0 {
				return nil, ErrNoUsableChain
			}
			v.logger.Warn("ai: skipping fallback provider", slog.String("provider", r.Provider))
			continue
		}
		be := v.backendFor(info, baseURL, key, cfg.PrivateURLOK)
		window := 0
		if m, ok := v.cachedModels(r.Provider); ok {
			for _, mi := range m {
				if mi.ID == r.Model {
					window = mi.ContextWindow
				}
			}
		}
		chain.Entries = append(chain.Entries, ChainEntry{
			Provider: r.Provider, Model: r.Model, ContextWindow: window,
			Backend: &RetryProvider{Inner: be, Policy: v.policy},
		})
	}
	if len(chain.Entries) == 0 {
		return nil, ErrNoUsableChain
	}
	return chain, nil
}

// HTTPStatusFor maps a Vault error to an HTTP status for the API layer.
func HTTPStatusFor(err error) int {
	var se *SaveError
	switch {
	case errors.Is(err, ErrUnknownProvider):
		return http.StatusNotFound
	case errors.As(err, &se):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}
