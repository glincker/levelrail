package supplychain

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// DefaultPolicy is the scan policy apps inherit when they have none of their own.
type DefaultPolicy struct {
	Enabled bool     `json:"enabled"`
	Gate    GateMode `json:"gate"`
}

// DefaultPolicySaved holds the saved halves of the instance default; nil means not saved.
type DefaultPolicySaved struct {
	Enabled *bool
	Gate    *GateMode
}

// GetDefaultPolicy reads the saved halves from the registered runtime settings rows.
func (s *SQLStore) GetDefaultPolicy(ctx context.Context) (DefaultPolicySaved, error) {
	rows, err := s.db.ListRuntimeSettings(ctx)
	if err != nil {
		return DefaultPolicySaved{}, fmt.Errorf("store: get supply chain default policy: %w", err)
	}
	var out DefaultPolicySaved
	for _, r := range rows {
		switch r.Key {
		case runtimesettings.KeyScanDefaultEnabled:
			if v, perr := strconv.ParseBool(r.Value); perr == nil {
				out.Enabled = &v
			}
		case runtimesettings.KeyScanDefaultGate:
			if g, perr := ParseGateMode(r.Value); perr == nil {
				out.Gate = &g
			}
		}
	}
	return out, nil
}

// SaveDefaultPolicy writes both halves under the registered keys so the settings API shows the same values.
func (s *SQLStore) SaveDefaultPolicy(ctx context.Context, p DefaultPolicy, actor string, at time.Time) error {
	if err := s.db.SetRuntimeSetting(ctx, runtimesettings.KeyScanDefaultEnabled, strconv.FormatBool(p.Enabled), actor, at); err != nil {
		return fmt.Errorf("store: save supply chain default policy: %w", err)
	}
	if err := s.db.SetRuntimeSetting(ctx, runtimesettings.KeyScanDefaultGate, string(p.Gate), actor, at); err != nil {
		return fmt.Errorf("store: save supply chain default policy: %w", err)
	}
	return nil
}

// DefaultPolicy returns the instance default: each saved half, else the env seed, else off.
func (s *Service) DefaultPolicy(ctx context.Context) (DefaultPolicy, error) {
	saved, err := s.deps.Store.GetDefaultPolicy(ctx)
	if err != nil {
		return DefaultPolicy{}, err
	}
	p := DefaultPolicy{Enabled: s.cfg.DefaultEnabled, Gate: s.cfg.DefaultGate}
	if saved.Enabled != nil {
		p.Enabled = *saved.Enabled
	}
	if saved.Gate != nil {
		p.Gate = *saved.Gate
	}
	return p, nil
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
