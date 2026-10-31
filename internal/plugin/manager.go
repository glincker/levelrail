package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// Containers is the slice of the Docker wrapper the runtime uses.
type Containers interface {
	InspectByName(ctx context.Context, name string) (*docker.ContainerState, error)
	ListByPrefix(ctx context.Context, prefix string) ([]docker.ContainerState, error)
	Create(ctx context.Context, spec docker.ContainerSpec) (string, error)
	Start(ctx context.Context, id string) error
	Stop(ctx context.Context, id string, timeout time.Duration) error
	Remove(ctx context.Context, id string, force bool) error
	EnsureNetwork(ctx context.Context, name string) (string, error)
}

// Manager owns plugin installation, approval and supervision.
type Manager struct {
	store  Store
	docker Containers
	audit  Auditor
	cfg    Config
	log    *slog.Logger
	now    func() time.Time
	dial   dialFunc

	mu        sync.Mutex
	instances map[string]*instance
}

// NewManager builds a Manager. audit and docker must be non-nil.
func NewManager(store Store, dk Containers, audit Auditor, cfg Config, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{
		store: store, docker: dk, audit: audit, cfg: cfg.withDefaults(), log: log,
		now: time.Now, dial: dialUnix, instances: map[string]*instance{},
	}
}

// InstallRequest is everything an operator submits to install or upgrade a plugin.
// Approve, when non-nil, must list exactly the abilities the manifest requests.
type InstallRequest struct {
	Manifest       []byte
	Signature      string
	KeyID          string
	ExpectedDigest string
	ConfigJSON     string
	Approve        []string
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// Install verifies and records a plugin. It starts nothing, and approves only
// when the request carries the exact requested ability set.
func (m *Manager) Install(ctx context.Context, req InstallRequest, actor string) (Record, error) {
	man, err := ParseManifest(req.Manifest)
	if err != nil {
		return Record{}, err
	}
	ver, keyID, err := m.verify(ctx, req, man)
	if err != nil {
		return Record{}, err
	}
	approved, err := exactApproval(req.Approve, man.Abilities)
	if err != nil {
		return Record{}, err
	}
	cfgJSON, err := m.checkConfig(req.ConfigJSON)
	if err != nil {
		return Record{}, err
	}
	effective, err := EffectiveOutboundHosts(man, cfgJSON)
	if err != nil {
		return Record{}, err
	}
	now := stamp(m.now())
	rec := Record{
		Name: man.Name, Version: man.Version, Kind: man.Kind, Publisher: man.Publisher, Description: man.Description,
		Image: man.Image, Digest: man.Digest(), Manifest: string(req.Manifest), Signature: req.Signature, KeyID: keyID,
		RequestedAbilities: man.Abilities, ApprovedAbilities: []string{}, OutboundHosts: effective, ConfigJSON: cfgJSON,
		State: StateInstalled, InstalledAt: now, InstalledBy: actor, UpdatedAt: now,
	}
	if approved != nil {
		rec.ApprovedAbilities, rec.ApprovedAt, rec.ApprovedBy = approved, now, actor
	}
	if err := m.store.InsertPlugin(ctx, rec); err != nil {
		return Record{}, err
	}
	err = m.audit.Record(ctx, AuditEvent{Action: AuditInstall, Actor: actor, Plugin: rec.Name, Detail: map[string]string{
		"version": rec.Version, "digest": rec.Digest, "key_id": rec.KeyID, "verification": ver.Status, "requested": strings.Join(rec.RequestedAbilities, ","),
	}})
	if err == nil && approved != nil {
		err = m.audit.Record(ctx, AuditEvent{Action: AuditApprove, Actor: actor, Plugin: rec.Name, Detail: map[string]string{
			"approved": strings.Join(approved, ","), "requested": strings.Join(rec.RequestedAbilities, ","),
		}})
	}
	return rec, err
}

// exactApproval returns the normalized approval list, nil when none was offered,
// and an error unless the offer equals the requested set exactly.
func exactApproval(offered, requested []string) ([]string, error) {
	if offered == nil {
		return nil, nil
	}
	got, err := normalizeAbilities(offered)
	if err != nil {
		return nil, err
	}
	if !slices.Equal(got, requested) {
		return nil, invalid("approval must list exactly the requested abilities (%s)", strings.Join(requested, ", "))
	}
	return got, nil
}

func (m *Manager) checkConfig(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}", nil
	}
	if len(raw) > m.cfg.MaxConfigBytes {
		return "", invalid("config is larger than %d bytes", m.cfg.MaxConfigBytes)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "", invalid("config must be a JSON object")
	}
	return raw, nil
}

// Approve records the abilities the operator grants. They must be a subset of
// what the manifest requested; nothing is ever granted implicitly.
func (m *Manager) Approve(ctx context.Context, name string, abilities []string, actor string) (Record, error) {
	rec, err := m.store.GetPlugin(ctx, name)
	if err != nil {
		return Record{}, err
	}
	if rec.State == StateRemoving {
		return Record{}, ErrState
	}
	approved, err := normalizeAbilities(abilities)
	if err != nil {
		return Record{}, err
	}
	if len(approved) == 0 {
		return Record{}, invalid("approve at least one ability, or leave the plugin unapproved")
	}
	for _, a := range approved {
		if !slices.Contains(rec.RequestedAbilities, a) {
			return Record{}, invalid("ability %q was not requested by the manifest", a)
		}
	}
	now := stamp(m.now())
	rec.ApprovedAbilities, rec.ApprovedAt, rec.ApprovedBy, rec.UpdatedAt = approved, now, actor, now
	if err := m.store.UpdatePlugin(ctx, rec); err != nil {
		return Record{}, err
	}
	err = m.audit.Record(ctx, AuditEvent{Action: AuditApprove, Actor: actor, Plugin: name, Detail: map[string]string{
		"approved": strings.Join(approved, ","), "requested": strings.Join(rec.RequestedAbilities, ","),
	}})
	return rec, err
}

// Enable allows the supervisor to run the plugin. It needs a prior approval that includes read.
func (m *Manager) Enable(ctx context.Context, name, actor string) (Record, error) {
	rec, err := m.store.GetPlugin(ctx, name)
	if err != nil {
		return Record{}, err
	}
	if rec.State == StateRemoving {
		return Record{}, ErrState
	}
	if rec.ApprovedAt == "" || !slices.Contains(rec.ApprovedAbilities, AbilityRead) {
		return Record{}, ErrNotApproved
	}
	rec.State, rec.LastError, rec.UpdatedAt = StateEnabled, "", stamp(m.now())
	if err := m.store.UpdatePlugin(ctx, rec); err != nil {
		return Record{}, err
	}
	err = m.audit.Record(ctx, AuditEvent{Action: AuditEnable, Actor: actor, Plugin: name})
	return rec, err
}

// Disable stops the plugin and keeps its record and approval.
func (m *Manager) Disable(ctx context.Context, name, actor string) (Record, error) {
	rec, err := m.store.GetPlugin(ctx, name)
	if err != nil {
		return Record{}, err
	}
	if rec.State == StateRemoving {
		return Record{}, ErrState
	}
	rec.State, rec.UpdatedAt = StateDisabled, stamp(m.now())
	if err := m.store.UpdatePlugin(ctx, rec); err != nil {
		return Record{}, err
	}
	m.stopPlugin(ctx, name)
	err = m.audit.Record(ctx, AuditEvent{Action: AuditDisable, Actor: actor, Plugin: name})
	return rec, err
}

// Remove marks a plugin for removal and tries to clean up at once. If the
// container cannot be removed now the row stays in the removing state and the
// supervisor retries on its next pass.
func (m *Manager) Remove(ctx context.Context, name, actor string) error {
	rec, err := m.store.GetPlugin(ctx, name)
	if err != nil {
		return err
	}
	rec.State, rec.UpdatedAt = StateRemoving, stamp(m.now())
	if err := m.store.UpdatePlugin(ctx, rec); err != nil {
		return err
	}
	if err := m.audit.Record(ctx, AuditEvent{Action: AuditRemove, Actor: actor, Plugin: name}); err != nil {
		return err
	}
	return m.finishRemove(ctx, name)
}

func (m *Manager) finishRemove(ctx context.Context, name string) error {
	if err := m.removeContainer(ctx, name); err != nil {
		return fmt.Errorf("plugin: %q is marked for removal, container cleanup will be retried: %w", name, err)
	}
	m.dropInstance(name)
	return m.store.DeletePlugin(ctx, name)
}

// List returns every installed plugin.
func (m *Manager) List(ctx context.Context) ([]Record, error) { return m.store.ListPlugins(ctx) }

// Get returns one plugin.
func (m *Manager) Get(ctx context.Context, name string) (Record, error) {
	return m.store.GetPlugin(ctx, name)
}

// TrustKeys returns the operator trust list.
func (m *Manager) TrustKeys(ctx context.Context) ([]TrustedKey, error) { return m.store.ListKeys(ctx) }

// TrustAdd adds a publisher key to the trust list.
func (m *Manager) TrustAdd(ctx context.Context, name, publicKeyB64, actor string) (TrustedKey, error) {
	pub, err := ParsePublicKey(publicKeyB64)
	if err != nil {
		return TrustedKey{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return TrustedKey{}, invalid("a key name of 1 to 80 characters is required")
	}
	k := TrustedKey{KeyID: Fingerprint(pub), Name: name, PublicKey: strings.TrimSpace(publicKeyB64), AddedAt: stamp(m.now()), AddedBy: actor}
	if err := m.store.AddKey(ctx, k); err != nil {
		return TrustedKey{}, err
	}
	err = m.audit.Record(ctx, AuditEvent{Action: AuditTrustAdd, Actor: actor, Detail: map[string]string{"key_id": k.KeyID, "name": name}})
	return k, err
}

// TrustRemove removes a key. Plugins signed by it stop at the next supervisor pass.
func (m *Manager) TrustRemove(ctx context.Context, keyID, actor string) error {
	if err := m.store.RemoveKey(ctx, keyID); err != nil {
		return err
	}
	return m.audit.Record(ctx, AuditEvent{Action: AuditTrustRemove, Actor: actor, Detail: map[string]string{"key_id": keyID}})
}
