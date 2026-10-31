package plugin

import (
	"context"
	"errors"
	"slices"
	"strings"
)

// Verification statuses reported to operators.
const (
	VerifyVerified      = "verified"
	VerifyUnsigned      = "unsigned"
	VerifyUnsignedBlock = "unsigned_refused"
	VerifyUntrustedKey  = "untrusted_key"
	VerifyBadSignature  = "bad_signature"
	VerifyDigest        = "digest_mismatch"
	VerifyUnverified    = "unverified"
)

// Health values reported for an installed plugin.
const (
	HealthHealthy   = "healthy"
	HealthUnhealthy = "unhealthy"
	HealthStopped   = "stopped"
	HealthRemoving  = "removing"
)

// Fix kinds the dashboard turns into a button.
const (
	FixRestart   = "restart"
	FixApprove   = "approve"
	FixTrustKey  = "trust_key"
	FixReinstall = "reinstall"
)

// Verification is what the host could prove about a package before running it.
type Verification struct {
	Status       string `json:"status"`
	DigestPinned bool   `json:"digest_pinned"`
	Digest       string `json:"digest"`
	Signed       bool   `json:"signed"`
	KeyID        string `json:"key_id"`
	KeyName      string `json:"key_name"`
	Installable  bool   `json:"installable"`
	Reason       string `json:"reason"`
}

// Preview is the review an operator sees before approving an install or upgrade.
type Preview struct {
	Manifest           Manifest     `json:"manifest"`
	Verification       Verification `json:"verification"`
	Action             string       `json:"action"`
	InstalledVersion   string       `json:"installed_version"`
	ApprovedAbilities  []string     `json:"approved_abilities"`
	AddedAbilities     []string     `json:"added_abilities"`
	RemovedAbilities   []string     `json:"removed_abilities"`
	AddedHosts         []string     `json:"added_hosts"`
	RemovedHosts       []string     `json:"removed_hosts"`
	ConfigHosts        []string     `json:"config_hosts"`
	ReapprovalRequired bool         `json:"reapproval_required"`
}

// Status is the live health, verification and suggested fix of an installed plugin.
type Status struct {
	Health       string `json:"health"`
	Verification string `json:"verification"`
	Fix          string `json:"fix"`
}

// verify checks digest pin and signature. The Verification is filled even when
// an error is returned so Inspect can show why a package was refused.
func (m *Manager) verify(ctx context.Context, req InstallRequest, man Manifest) (Verification, string, error) {
	digest := man.Digest()
	v := Verification{DigestPinned: digest != "", Digest: digest}
	if req.ExpectedDigest != "" && req.ExpectedDigest != digest {
		v.Status, v.Reason = VerifyDigest, ErrDigestMismatch.Error()
		return v, "", ErrDigestMismatch
	}
	sig, keyID := strings.TrimSpace(req.Signature), strings.TrimSpace(req.KeyID)
	if sig == "" {
		if keyID != "" {
			v.Status, v.Reason = VerifyBadSignature, ErrBadSignature.Error()
			return v, "", ErrBadSignature
		}
		if !m.cfg.AllowUnsigned {
			v.Status, v.Reason = VerifyUnsignedBlock, ErrUnsigned.Error()
			return v, "", ErrUnsigned
		}
		v.Status = VerifyUnsigned
		v.Installable = true
		return v, "", nil
	}
	keys, err := m.store.ListKeys(ctx)
	if err != nil {
		return v, "", err
	}
	v.Signed = true
	if keyID == "" {
		keyID = findSigningKey(keys, req.Manifest, digest, sig)
	}
	if err := Verify(keys, keyID, req.Manifest, digest, sig); err != nil {
		v.Status, v.Reason = VerifyBadSignature, err.Error()
		if errors.Is(err, ErrUntrusted) {
			v.Status = VerifyUntrustedKey
		}
		return v, "", err
	}
	v.Status, v.KeyID, v.Installable = VerifyVerified, keyID, true
	for _, k := range keys {
		if k.KeyID == keyID {
			v.KeyName = k.Name
		}
	}
	return v, keyID, nil
}

func findSigningKey(keys []TrustedKey, manifest []byte, digest, sig string) string {
	for _, k := range keys {
		if Verify(keys, k.KeyID, manifest, digest, sig) == nil {
			return k.KeyID
		}
	}
	return ""
}

// Inspect parses and verifies a package and reports what installing or
// upgrading it would change. It writes nothing.
func (m *Manager) Inspect(ctx context.Context, req InstallRequest) (Preview, error) {
	man, err := ParseManifest(req.Manifest)
	if err != nil {
		return Preview{}, err
	}
	ver, _, verr := m.verify(ctx, req, man)
	if verr != nil && ver.Status == "" {
		return Preview{}, verr
	}
	existing, err := m.store.GetPlugin(ctx, man.Name)
	found := err == nil
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Preview{}, err
	}
	cfg := req.ConfigJSON
	if strings.TrimSpace(cfg) == "" && found {
		cfg = existing.ConfigJSON
	}
	cfgHosts, err := ConfigHosts(man, cfg)
	if err != nil {
		return Preview{}, err
	}
	effective, _ := EffectiveOutboundHosts(man, cfg)
	p := Preview{
		Manifest: man, Verification: ver, Action: "install", ApprovedAbilities: []string{},
		AddedAbilities: man.Abilities, RemovedAbilities: []string{}, AddedHosts: effective, RemovedHosts: []string{},
		ConfigHosts: cfgHosts,
	}
	if !found {
		return p, nil
	}
	p.Action, p.InstalledVersion, p.ApprovedAbilities = "upgrade", existing.Version, existing.ApprovedAbilities
	p.AddedAbilities, p.RemovedAbilities = diff(existing.RequestedAbilities, man.Abilities)
	p.AddedHosts, p.RemovedHosts = diff(existing.OutboundHosts, effective)
	p.ReapprovalRequired = len(p.AddedAbilities) > 0 || len(p.AddedHosts) > 0
	return p, nil
}

func diff(old, next []string) (added, removed []string) {
	added, removed = []string{}, []string{}
	for _, s := range next {
		if !slices.Contains(old, s) {
			added = append(added, s)
		}
	}
	for _, s := range old {
		if !slices.Contains(next, s) {
			removed = append(removed, s)
		}
	}
	return added, removed
}

func intersect(a, b []string) []string {
	out := []string{}
	for _, s := range a {
		if slices.Contains(b, s) {
			out = append(out, s)
		}
	}
	return out
}

// Upgrade replaces an installed plugin's package with a newer verified one.
// Abilities or outbound hosts that were not requested before need a fresh
// approval: the plugin is stopped and unapproved until the operator gives it,
// unless the request carries the exact new ability set.
func (m *Manager) Upgrade(ctx context.Context, req InstallRequest, actor string) (Record, error) {
	man, err := ParseManifest(req.Manifest)
	if err != nil {
		return Record{}, err
	}
	rec, err := m.store.GetPlugin(ctx, man.Name)
	if err != nil {
		return Record{}, err
	}
	if rec.State == StateRemoving {
		return Record{}, ErrState
	}
	ver, keyID, err := m.verify(ctx, req, man)
	if err != nil {
		return Record{}, err
	}
	if rec.KeyID != "" && keyID == "" {
		return Record{}, invalid("a signed plugin cannot be replaced by an unsigned package")
	}
	if man.Digest() == rec.Digest && man.Version == rec.Version {
		return Record{}, invalid("%s %s is already installed", rec.Name, rec.Version)
	}
	approved, err := exactApproval(req.Approve, man.Abilities)
	if err != nil {
		return Record{}, err
	}
	if strings.TrimSpace(req.ConfigJSON) != "" {
		if rec.ConfigJSON, err = m.checkConfig(req.ConfigJSON); err != nil {
			return Record{}, err
		}
	}
	effective, err := EffectiveOutboundHosts(man, rec.ConfigJSON)
	if err != nil {
		return Record{}, err
	}
	addedA, _ := diff(rec.RequestedAbilities, man.Abilities)
	addedH, _ := diff(rec.OutboundHosts, effective)
	widened := len(addedA) > 0 || len(addedH) > 0
	from, now := rec.Version, stamp(m.now())

	rec.Version, rec.Publisher, rec.Description, rec.Image, rec.Digest = man.Version, man.Publisher, man.Description, man.Image, man.Digest()
	rec.Manifest, rec.Signature, rec.KeyID = string(req.Manifest), req.Signature, keyID
	rec.RequestedAbilities, rec.OutboundHosts, rec.LastError, rec.UpdatedAt = man.Abilities, effective, "", now
	switch {
	case approved != nil:
		rec.ApprovedAbilities, rec.ApprovedAt, rec.ApprovedBy = approved, now, actor
	case widened:
		rec.ApprovedAbilities, rec.ApprovedAt, rec.ApprovedBy = []string{}, "", ""
		if rec.State == StateEnabled {
			rec.State = StateDisabled
		}
	default:
		rec.ApprovedAbilities = intersect(rec.ApprovedAbilities, man.Abilities)
	}
	if err := m.store.UpdatePlugin(ctx, rec); err != nil {
		return Record{}, err
	}
	m.stopPlugin(ctx, rec.Name)
	err = m.audit.Record(ctx, AuditEvent{Action: AuditUpgrade, Actor: actor, Plugin: rec.Name, Detail: map[string]string{
		"from": from, "to": rec.Version, "digest": rec.Digest, "verification": ver.Status,
		"requested": strings.Join(rec.RequestedAbilities, ","), "reapproval": boolString(widened && approved == nil),
	}})
	return rec, err
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// Restart clears a plugin's failure backoff and breaker and starts it again now.
func (m *Manager) Restart(ctx context.Context, name, actor string) (Record, error) {
	rec, err := m.store.GetPlugin(ctx, name)
	if err != nil {
		return Record{}, err
	}
	if rec.State != StateEnabled {
		return Record{}, ErrState
	}
	in := m.inst(name)
	in.opMu.Lock()
	in.closeClient()
	if err := m.removeContainer(ctx, name); err != nil {
		m.log.Warn("plugin restart: container cleanup failed", "plugin", name, "error", err)
	}
	in.mu.Lock()
	in.failures, in.healthFails, in.breakerFails = 0, 0, 0
	in.nextAttempt, in.breakerUntil = m.now().Add(-1), m.now().Add(-1)
	in.mu.Unlock()
	in.opMu.Unlock()
	m.setLastError(ctx, name, "")
	m.ensureRunning(ctx, rec)
	if err := m.audit.Record(ctx, AuditEvent{Action: AuditRestart, Actor: actor, Plugin: name}); err != nil {
		return Record{}, err
	}
	return m.store.GetPlugin(ctx, name)
}

// StatusOf reports health, verification and the fix that would help, if any.
func (m *Manager) StatusOf(ctx context.Context, rec Record) Status {
	s := Status{Verification: VerifyVerified}
	if err := m.verifyRecord(ctx, rec); err != nil {
		s.Verification = VerifyUnverified
		if errors.Is(err, ErrUntrusted) {
			s.Verification = VerifyUntrustedKey
		}
	} else if rec.KeyID == "" {
		s.Verification = VerifyUnsigned
	}
	switch {
	case rec.State == StateRemoving:
		s.Health = HealthRemoving
	case rec.State != StateEnabled:
		s.Health = HealthStopped
	case m.Running(rec.Name) && rec.LastError == "":
		s.Health = HealthHealthy
	default:
		s.Health = HealthUnhealthy
	}
	switch {
	case s.Verification == VerifyUntrustedKey:
		s.Fix = FixTrustKey
	case s.Verification == VerifyUnverified:
		s.Fix = FixReinstall
	case rec.ApprovedAt == "" && rec.State != StateRemoving:
		s.Fix = FixApprove
	case s.Health == HealthUnhealthy:
		s.Fix = FixRestart
	}
	return s
}
