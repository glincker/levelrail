package supplychain

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// DefaultPolicyKey is the runtime_settings key holding the instance default policy.
const DefaultPolicyKey = "supply_chain.default_policy"

// DefaultPolicy is the scan policy apps inherit when they have none of their own.
type DefaultPolicy struct {
	Enabled bool     `json:"enabled"`
	Gate    GateMode `json:"gate"`
}

// GetDefaultPolicy returns the saved instance default; ok is false when none was saved.
func (s *SQLStore) GetDefaultPolicy(ctx context.Context) (DefaultPolicy, bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM runtime_settings WHERE key = ?`, DefaultPolicyKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultPolicy{}, false, nil
	}
	if err != nil {
		return DefaultPolicy{}, false, fmt.Errorf("store: get supply chain default policy: %w", err)
	}
	var p DefaultPolicy
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return DefaultPolicy{}, false, fmt.Errorf("store: decode supply chain default policy: %w", err)
	}
	mode, perr := ParseGateMode(string(p.Gate))
	if perr != nil {
		mode = GateOff
	}
	p.Gate = mode
	return p, true, nil
}

// SaveDefaultPolicy stores the instance default in the runtime settings key-value table.
func (s *SQLStore) SaveDefaultPolicy(ctx context.Context, p DefaultPolicy, actor string, at time.Time) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("store: encode supply chain default policy: %w", err)
	}
	return s.db.SetRuntimeSetting(ctx, DefaultPolicyKey, string(raw), actor, at)
}

// DefaultPolicy returns the instance default: the saved value, else the env seed, else off.
func (s *Service) DefaultPolicy(ctx context.Context) (DefaultPolicy, error) {
	p, ok, err := s.deps.Store.GetDefaultPolicy(ctx)
	if err != nil {
		return DefaultPolicy{}, err
	}
	if ok {
		return p, nil
	}
	return DefaultPolicy{Enabled: s.cfg.DefaultEnabled, Gate: s.cfg.DefaultGate}, nil
}

// SaveDefaultPolicy applies patch to the instance default. A gate other than off requires scanning enabled.
func (s *Service) SaveDefaultPolicy(ctx context.Context, patch SettingsPatch, actor string) (DefaultPolicy, error) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	cur, err := s.DefaultPolicy(ctx)
	if err != nil {
		return DefaultPolicy{}, err
	}
	if patch.Enabled != nil {
		cur.Enabled = *patch.Enabled
	}
	if patch.Gate != nil {
		mode, err := ParseGateMode(*patch.Gate)
		if err != nil {
			return DefaultPolicy{}, err
		}
		cur.Gate = mode
	}
	if cur.Gate != GateOff && !cur.Enabled {
		return DefaultPolicy{}, fmt.Errorf("%w: scan_gate %q needs scanning enabled", ErrInvalid, cur.Gate)
	}
	if err := s.deps.Store.SaveDefaultPolicy(ctx, cur, actor, s.now()); err != nil {
		return DefaultPolicy{}, err
	}
	return cur, nil
}

// effectiveSettings is the single fallback call site: apps without a stored policy inherit the instance default.
func (s *Service) effectiveSettings(ctx context.Context, app string) (Settings, error) {
	st, err := s.deps.Store.GetSettings(ctx, app)
	if err != nil || st.Explicit {
		return st, err
	}
	def, err := s.DefaultPolicy(ctx)
	if err != nil {
		return Settings{}, err
	}
	st.Enabled, st.Gate = def.Enabled, def.Gate
	if st.Gate == "" {
		st.Gate = GateOff
	}
	return st, nil
}
