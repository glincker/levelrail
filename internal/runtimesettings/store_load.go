package runtimesettings

import (
	"context"
	"log/slog"
	"sync"

	"github.com/GLINCKER/levelrail/internal/store"
)

var warnedStoredValue sync.Map

// upgradeRows makes an older stored set safe to boot with: retired keys are renamed
// (and persisted best effort), unknown keys are kept as they are, and a known key whose
// value no longer validates is dropped from the view so the default applies, with one warning.
func (s *Service) upgradeRows(ctx context.Context, rows []store.RuntimeSettingRow) map[string]store.RuntimeSettingRow {
	m := make(map[string]store.RuntimeSettingRow, len(rows))
	for _, r := range rows {
		m[r.Key] = r
	}
	for old, cur := range legacyKeys {
		row, ok := m[old]
		if !ok {
			continue
		}
		delete(m, old)
		if _, taken := m[cur]; taken {
			continue
		}
		row.Key = cur
		m[cur] = row
		if s.store != nil {
			if err := s.store.SetRuntimeSetting(ctx, cur, row.Value, row.Actor, row.UpdatedAt); err == nil {
				_ = s.store.DeleteRuntimeSetting(ctx, old)
			}
		}
	}
	// A FailClosed setting keeps its bad row so StringChecked can refuse instead of using the default.
	for key, row := range m {
		d, known := s.definition(key)
		if !known || d.FailClosed || savedValueUsable(d, row.Value) {
			continue
		}
		if _, seen := warnedStoredValue.LoadOrStore(key, struct{}{}); !seen {
			slog.Warn("runtime setting holds a stored value that is out of range, using the default", slog.String("key", key))
		}
		delete(m, key)
	}
	return m
}

func savedValueUsable(d Definition, raw string) bool {
	c, err := canonicalFromString(d, raw)
	if err != nil {
		return false
	}
	return d.CheckValue == nil || d.CheckValue(c) == nil
}
